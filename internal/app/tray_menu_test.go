package app

import (
	"testing"

	"github.com/egoist/mygo"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/sender"
	"saundo/internal/lamp"
	"saundo/internal/nowplaying"
)

func trayMenuApp() *App {
	a := previewApp()
	a.menuIcons = loadMenuIcons()
	a.cfg.PlaybackControls = true
	a.nowSnap = nowplaying.Snapshot{Sessions: []nowplaying.Session{{
		AUMID:    "com.apple.Music",
		AppName:  "Music",
		Title:    "夜航",
		Artist:   "白鲸电台",
		Playing:  true,
		CanPlay:  true,
		CanPause: true,
		CanNext:  true,
		CanPrev:  true,
	}}}
	return a
}

func trayItem(items []*mygo.MenuItem, label string) (*mygo.MenuItem, int) {
	for i, it := range items {
		if it.Label == label {
			return it, i
		}
	}
	return nil, -1
}

func trayLampRow(items []*mygo.MenuItem, label string) (*mygo.MenuItem, int) {
	for i, it := range items {
		if it.Label == label || it.Label == label+" ✔" {
			return it, i
		}
	}
	return nil, -1
}

func trayLabels(items []*mygo.MenuItem) []string {
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.Label
	}
	return labels
}

func assertTraySeparator(t *testing.T, item *mygo.MenuItem, at string) {
	t.Helper()
	if item.Type != mygo.MenuItemSeparator {
		t.Fatalf("%s: item is %q, want a separator", at, item.Label)
	}
}

// assertTrayPlain fails if any row uses a check column. The ambient selection
// is a subtitle suffix, so the whole menu stays one icon column.
func assertTrayPlain(t *testing.T, items []*mygo.MenuItem) {
	t.Helper()
	for i, it := range items {
		if it.Type == mygo.MenuItemRadio || it.Type == mygo.MenuItemCheckbox || it.Checked {
			t.Fatalf("item %d (%q) uses a check column: type %q, checked %t", i, it.Label, it.Type, it.Checked)
		}
	}
}

func TestTrayMenuShape(t *testing.T) {
	a := trayMenuApp()
	a.snap.LampCover = false /* this shape test pins the template row; cover follows below */
	m := a.trayModel("zh-CN", false)
	items := a.trayMenuItems(m)
	if len(items) != 15 {
		t.Fatalf("menu has %d items, want 15: %v", len(items), trayLabels(items))
	}
	assertTrayPlain(t, items)
	for i, d := range a.snap.Devices {
		if items[i].Label != d.Name {
			t.Fatalf("device row %d is %q, want %q", i, items[i].Label, d.Name)
		}
	}
	if !items[0].Disabled || items[0].Subtitle != locales.StatusStreaming() {
		t.Fatalf("active row is %+v, want disabled and %q", items[0], locales.StatusStreaming())
	}
	if items[1].Disabled {
		t.Fatal("another online receiver must stay clickable to switch")
	}
	if items[2].Disabled || items[2].Subtitle != locales.ActionOffline() {
		t.Fatalf("offline row is %+v, want enabled with the offline subtitle", items[2])
	}
	if got, want := items[3].Label, locales.ActionDisconnect(); got != want {
		t.Fatalf("session row is %q, want %q", got, want)
	}
	assertTraySeparator(t, items[4], "after the session row")
	if got, want := items[5].Label, locales.FlyoutPrevious(); got != want {
		t.Fatalf("media row 0 is %q, want %q", got, want)
	}
	if got, want := items[6].Label, locales.FlyoutPause(); got != want {
		t.Fatalf("media row 1 is %q, want %q", got, want)
	}
	if got, want := items[7].Label, locales.FlyoutNext(); got != want {
		t.Fatalf("media row 2 is %q, want %q", got, want)
	}
	assertTraySeparator(t, items[8], "after the media rows")
	if got, want := items[9].Label, locales.FlyoutLampSection(); got != want {
		t.Fatalf("ambient row is %q, want %q", got, want)
	}
	if items[9].Submenu == nil {
		t.Fatal("ambient row has no submenu")
	}
	if got, want := items[9].Subtitle, locales.LampPresetDune(); got != want {
		t.Fatalf("ambient subtitle is %q, want %q", got, want)
	}
	assertTraySeparator(t, items[10], "after the ambient row")
	if got, want := items[11].Label, locales.TrayShowPanel(); got != want {
		t.Fatalf("show panel row is %q, want %q", got, want)
	}
	if got, want := items[12].Label, locales.TraySettings(); got != want {
		t.Fatalf("settings row is %q, want %q", got, want)
	}
	assertTraySeparator(t, items[13], "before quit")
	if items[14].Role != mygo.RoleQuit || items[14].Label != locales.TrayQuit() {
		t.Fatalf("quit row is %+v, want the quit role", items[14])
	}
	if got := m.deviceIcon(m.devices[0]); got != "devon:homepod-mini-pair" {
		t.Fatalf("HomePod row icon key is %q, want devon:homepod-mini-pair", got)
	}
	if got := m.deviceIcon(m.devices[1]); got != "dev:appletv" {
		t.Fatalf("Apple TV row icon key is %q, want dev:appletv", got)
	}
}

func TestTrayMenuSessionRow(t *testing.T) {
	a := trayMenuApp()

	a.snap.State = sender.StateConnecting
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.TrayMenuCancelConnect()); i < 0 {
		t.Fatalf("connecting menu has no cancel row: %v", trayLabels(items))
	}
	for i := range a.snap.Devices {
		if !items[i].Disabled {
			t.Fatalf("receiver row %d is enabled while connecting", i)
		}
	}

	a.snap.State = sender.StateStreaming
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.ActionDisconnect()); i < 0 {
		t.Fatalf("streaming menu has no disconnect row: %v", trayLabels(items))
	}
	if !items[0].Disabled {
		t.Fatal("the in-session row must not offer a second connect")
	}
	if items[1].Disabled {
		t.Fatal("another row must stay clickable to switch while streaming")
	}

	a.snap.State = sender.StateIdle
	a.snap.Error = "timeout"
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.TrayMenuRetry()); i < 0 {
		t.Fatalf("failed menu has no retry row: %v", trayLabels(items))
	}
	if got, want := items[0].Subtitle, locales.StatusConnectionFailed(); got != want {
		t.Fatalf("failed row subtitle is %q, want %q", got, want)
	}

	a.snap.Error = ""
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	for _, label := range []string{locales.ActionDisconnect(), locales.TrayMenuCancelConnect(), locales.TrayMenuRetry()} {
		if _, i := trayItem(items, label); i >= 0 {
			t.Fatalf("%q is in the idle menu at %d", label, i)
		}
	}
	for i := range a.snap.Devices {
		if items[i].Disabled {
			t.Fatalf("receiver row %d is disabled while idle", i)
		}
	}
}

func TestTrayMenuMediaRowsFollowSession(t *testing.T) {
	a := trayMenuApp()

	m := a.trayModel("zh-CN", false)
	items := a.trayMenuItems(m)
	if _, i := trayItem(items, locales.FlyoutPause()); i < 0 {
		t.Fatal("playing session has no pause row")
	}
	a.nowSnap.Sessions[0].Playing = false
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.FlyoutPlay()); i < 0 {
		t.Fatal("paused session has no play row")
	}
	if _, i := trayItem(items, locales.FlyoutPause()); i >= 0 {
		t.Fatal("paused session still has a pause row")
	}

	a.nowSnap.Sessions[0].CanPrev = false
	a.nowSnap.Sessions[0].CanNext = false
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.FlyoutPrevious()); i >= 0 {
		t.Fatal("session without CanPrev still has a previous row")
	}
	if _, i := trayItem(items, locales.FlyoutNext()); i >= 0 {
		t.Fatal("session without CanNext still has a next row")
	}

	a.nowSnap.Sessions[0].CanPlay = false
	a.nowSnap.Sessions[0].CanPause = false
	a.nowSnap.Sessions[0].CanNext = true
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.FlyoutNext()); i < 0 {
		t.Fatal("session with only CanNext has no next row")
	}

	a.cfg.PlaybackControls = false
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	for _, label := range []string{locales.FlyoutPrevious(), locales.FlyoutPlay(), locales.FlyoutPause(), locales.FlyoutNext()} {
		if _, i := trayItem(items, label); i >= 0 {
			t.Fatalf("%q is in the menu while playback controls are off", label)
		}
	}

	a.cfg.PlaybackControls = true
	a.nowSnap = nowplaying.Snapshot{}
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.FlyoutPause()); i >= 0 {
		t.Fatal("media row is in the menu without a media session")
	}
}

func TestTrayMenuMediaControlSendsAction(t *testing.T) {
	a := trayMenuApp()
	var gotAUMID string
	var gotAction nowplaying.Action
	a.controlNow = func(aumid string, action nowplaying.Action) {
		gotAUMID, gotAction = aumid, action
	}
	pause, _ := trayItem(a.trayMenuItems(a.trayModel("zh-CN", false)), locales.FlyoutPause())
	if pause == nil {
		t.Fatal("no pause row")
	}
	pause.Click(pause, nil)
	if gotAUMID != "com.apple.Music" || gotAction != nowplaying.ActionPause {
		t.Fatalf("control got (%q, %v), want (com.apple.Music, pause)", gotAUMID, gotAction)
	}
}

func TestTrayMenuLampSubmenu(t *testing.T) {
	a := trayMenuApp()
	a.snap.LampCover = false /* the first half pins template mode; the second switches to cover */
	m := a.trayModel("zh-CN", false)
	items := a.trayMenuItems(m)
	parent, _ := trayItem(items, locales.FlyoutLampSection())
	if parent == nil || parent.Submenu == nil {
		t.Fatal("ambient row has no submenu")
	}
	if got, want := parent.Subtitle, locales.LampPresetDune(); got != want {
		t.Fatalf("ambient subtitle is %q, want %q", got, want)
	}
	if got, want := m.lampIconKey(), "lamp:dune"; got != want {
		t.Fatalf("ambient icon key is %q, want %q", got, want)
	}
	if parent.Icon != a.menuIcons["lamp:dune"] {
		t.Fatal("ambient row does not use the dune preview icon")
	}
	power, _ := trayItem(parent.Submenu, locales.LampTurnOff())
	if power == nil {
		t.Fatal("lit menu has no turn-off row")
	}
	assertTrayPlain(t, parent.Submenu)
	cover, _ := trayLampRow(parent.Submenu, locales.LampPresetCover())
	if cover == nil {
		t.Fatal("no cover row")
	}
	if cover.Subtitle != "" {
		t.Fatalf("cover row subtitle is %q in template mode, want empty", cover.Subtitle)
	}
	if cover.Icon == nil {
		t.Fatal("cover row has no preview icon")
	}
	dune, _ := trayLampRow(parent.Submenu, locales.LampPresetDune())
	if dune == nil {
		t.Fatal("no dune row")
	}
	if !dune.Disabled || dune.Subtitle != "" {
		t.Fatalf("dune row is %+v, want disabled with no suffix", dune)
	}
	if got, want := dune.Label, locales.LampPresetDune()+" ✔"; got != want {
		t.Fatalf("current dune label is %q, want %q", got, want)
	}
	if got, want := cover.Label, locales.LampPresetCover(); got != want {
		t.Fatalf("cover label is %q, want %q", got, want)
	}
	if dune.Icon == nil {
		t.Fatal("dune row has no preview icon")
	}
	ember, _ := trayLampRow(parent.Submenu, locales.LampPresetEmber())
	if ember == nil || ember.Disabled || ember.Subtitle != "" {
		t.Fatalf("ember row is %+v, want enabled with no suffix", ember)
	}

	a.snap.LampCover = true
	m = a.trayModel("zh-CN", false)
	parent, _ = trayItem(a.trayMenuItems(m), locales.FlyoutLampSection())
	if got, want := parent.Subtitle, locales.LampPresetCover(); got != want {
		t.Fatalf("cover-mode subtitle is %q, want %q", got, want)
	}
	if got, want := m.lampIconKey(), "lamp:cover"; got != want {
		t.Fatalf("cover-mode icon key is %q, want %q", got, want)
	}
	if parent.Icon != a.menuIcons["lamp:cover"] {
		t.Fatal("ambient row does not use the cover preview icon")
	}
	cover, _ = trayLampRow(parent.Submenu, locales.LampPresetCover())
	if !cover.Disabled || cover.Subtitle != "" {
		t.Fatalf("cover row is %+v, want disabled with no suffix", cover)
	}
	if got, want := cover.Label, locales.LampPresetCover()+" ✔"; got != want {
		t.Fatalf("current cover label is %q, want %q", got, want)
	}
	dune, _ = trayLampRow(parent.Submenu, locales.LampPresetDune())
	if dune.Disabled || dune.Subtitle != "" {
		t.Fatalf("dune row is %+v, want enabled with no suffix", dune)
	}
	if got, want := dune.Label, locales.LampPresetDune(); got != want {
		t.Fatalf("dune label is %q, want %q", got, want)
	}

	a.snap.LampOff = true
	m = a.trayModel("zh-CN", false)
	parent, _ = trayItem(a.trayMenuItems(m), locales.FlyoutLampSection())
	if got, want := parent.Subtitle, locales.LampOffStatus(); got != want {
		t.Fatalf("off subtitle is %q, want %q", got, want)
	}
	if got, want := m.lampIconKey(), "lampOff"; got != want {
		t.Fatalf("off icon key is %q, want %q", got, want)
	}
	if parent.Icon != a.menuIcons["lampOff"] {
		t.Fatal("dark ambient row does not use the lamp-off icon")
	}
	power, _ = trayItem(parent.Submenu, locales.LampTurnOn())
	if power == nil {
		t.Fatal("dark menu has no turn-on row")
	}
	cover, _ = trayLampRow(parent.Submenu, locales.LampPresetCover())
	if !cover.Disabled {
		t.Fatal("cover row is enabled while the light is off")
	}
	if cover.Subtitle != locales.TrayMenuLastUsed() {
		t.Fatalf("cover row subtitle is %q, want the last-used suffix", cover.Subtitle)
	}
	dune, _ = trayLampRow(parent.Submenu, locales.LampPresetDune())
	if !dune.Disabled {
		t.Fatal("template row is enabled while the light is off")
	}
	if dune.Subtitle != "" {
		t.Fatalf("dune row subtitle is %q, want empty", dune.Subtitle)
	}
}

func TestTrayMenuLampOffDisablesChoices(t *testing.T) {
	a := trayMenuApp()
	a.snap.LampOff = true
	a.snap.LampCover = false
	a.snap.LampPreset = "dune"
	m := a.trayModel("zh-CN", false)
	parent, _ := trayItem(a.trayMenuItems(m), locales.FlyoutLampSection())
	if got, want := m.lampIconKey(), "lampOff"; got != want {
		t.Fatalf("off icon key is %q, want %q", got, want)
	}
	cover, _ := trayLampRow(parent.Submenu, locales.LampPresetCover())
	if !cover.Disabled || cover.Subtitle != "" {
		t.Fatalf("cover row is %+v, want disabled and no suffix", cover)
	}
	ember, _ := trayLampRow(parent.Submenu, locales.LampPresetEmber())
	if !ember.Disabled || ember.Subtitle != "" {
		t.Fatalf("ember row is %+v, want disabled and no suffix", ember)
	}
	dune, _ := trayLampRow(parent.Submenu, locales.LampPresetDune())
	if !dune.Disabled || dune.Subtitle != locales.TrayMenuLastUsed() {
		t.Fatalf("dune row is %+v, want disabled with the last-used suffix", dune)
	}
	power, _ := trayItem(parent.Submenu, locales.LampTurnOn())
	if power == nil {
		t.Fatal("dark menu has no turn-on row")
	}
	power.Click(power, nil)
	if a.lampOff || a.snap.LampOff {
		t.Fatal("the turn-on row did not turn the light on")
	}
	m = a.trayModel("zh-CN", false)
	parent, _ = trayItem(a.trayMenuItems(m), locales.FlyoutLampSection())
	dune, _ = trayLampRow(parent.Submenu, locales.LampPresetDune())
	if !dune.Disabled || dune.Subtitle != "" {
		t.Fatalf("dune row is %+v after turn-on, want disabled as the current choice", dune)
	}
	if got, want := dune.Label, locales.LampPresetDune()+" ✔"; got != want {
		t.Fatalf("current dune label is %q, want %q", got, want)
	}
}

func TestTrayMenuUpdateRow(t *testing.T) {
	a := trayMenuApp()
	a.upd.phase = updateAvailable
	a.upd.version = "9.9.9"
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	download, _ := trayItem(items, locales.TrayMenuDownload("9.9.9"))
	if download == nil || download.Disabled {
		t.Fatalf("available update row is %+v, want enabled", download)
	}

	a.upd.phase = updateDownloading
	a.upd.got, a.upd.total = 50, 100
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	downloading, _ := trayItem(items, locales.FlyoutUpdateDownloading(50))
	if downloading == nil || !downloading.Disabled {
		t.Fatalf("downloading update row is %+v, want disabled", downloading)
	}

	a.upd.phase = updateReady
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	install, _ := trayItem(items, locales.TrayMenuInstall("9.9.9"))
	if install == nil || install.Disabled {
		t.Fatalf("ready update row is %+v, want enabled", install)
	}

	a.upd.phase = updateAvailable
	a.cfg.SkippedVersion = "9.9.9"
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.TrayMenuDownload("9.9.9")); i >= 0 {
		t.Fatal("skipped update still has a menu row")
	}
	a.cfg.SkippedVersion = ""

	a.upd.phase = updateIdle
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	for _, label := range []string{locales.TrayMenuDownload("9.9.9"), locales.TrayMenuInstall("9.9.9"), locales.FlyoutUpdateDownloading(50)} {
		if _, i := trayItem(items, label); i >= 0 {
			t.Fatalf("%q is in the idle menu", label)
		}
	}
}

func TestTrayMenuMoreReceivers(t *testing.T) {
	a := trayMenuApp()
	for i := 0; i < 4; i++ {
		d := a.snap.Devices[0]
		d.ID = "extra-" + string(rune('a'+i))
		d.Name = "Extra " + string(rune('A'+i))
		d.Online = true
		a.snap.Devices = append(a.snap.Devices, d)
	}
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	more, i := trayItem(items, locales.TrayMenuMore())
	if i != trayMenuDeviceRows {
		t.Fatalf("more row is at %d, want %d: %v", i, trayMenuDeviceRows, trayLabels(items))
	}
	if more == nil {
		t.Fatal("menu with seven receivers has no more row")
	}
	if _, i := trayItem(items, "Extra D"); i >= 0 {
		t.Fatal("the seventh receiver is still a flat row")
	}
}

func TestTrayMenuEmptyState(t *testing.T) {
	a := trayMenuApp()
	a.snap.Devices = nil
	a.snap.State = sender.StateIdle
	a.snap.SelectedID = ""
	a.selID = ""
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	search, i := trayItem(items, locales.FlyoutSearchAgain())
	if i != 0 || search == nil {
		t.Fatalf("search row is at %d, want 0: %v", i, trayLabels(items))
	}
	if got, want := search.Subtitle, locales.ReceiverEmptyTitle(); got != want {
		t.Fatalf("search subtitle is %q, want %q", got, want)
	}
	if search.Disabled {
		t.Fatal("search row is disabled")
	}
	for _, label := range []string{locales.ActionDisconnect(), locales.TrayMenuCancelConnect(), locales.TrayMenuRetry(), locales.TrayMenuMore()} {
		if _, i := trayItem(items, label); i >= 0 {
			t.Fatalf("%q is in the empty menu", label)
		}
	}
}

func TestTrayMenuDeviceSubtitle(t *testing.T) {
	a := trayMenuApp()
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	if got, want := items[0].Subtitle, locales.StatusStreaming(); got != want {
		t.Fatalf("active subtitle is %q, want %q", got, want)
	}
	if got, want := items[1].Subtitle, ""; got != want {
		t.Fatalf("idle online subtitle is %q, want empty", got)
	}
	if got, want := items[2].Subtitle, locales.ActionOffline(); got != want {
		t.Fatalf("offline subtitle is %q, want %q", got, want)
	}
	a.snap.AudioIdle = true
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	if got, want := items[0].Subtitle, locales.SummaryConnected(); got != want {
		t.Fatalf("silent session subtitle is %q, want %q", got, want)
	}
}

func TestTrayMenuKey(t *testing.T) {
	/* The key test flips one field at a time. Every variant starts from the
	   same template light, so the fixture's cover default cannot mask it. */
	templateMode := func() *App {
		a := trayMenuApp()
		a.snap.LampCover = false
		return a
	}
	base := templateMode()
	key := base.trayModel("zh-CN", false).key()
	if got := templateMode().trayModel("zh-CN", false).key(); got != key {
		t.Fatalf("identical state keys differ: %+v and %+v", key, got)
	}
	online := templateMode()
	online.snap.Devices[2].Online = true
	if online.trayModel("zh-CN", false).key() == key {
		t.Fatal("flipping a device online left the key unchanged")
	}
	renamed := templateMode()
	renamed.snap.Devices[1].Name = "Living Room"
	if renamed.trayModel("zh-CN", false).key() == key {
		t.Fatal("renaming a device left the key unchanged")
	}
	lampOff := templateMode()
	lampOff.snap.LampOff = true
	if lampOff.trayModel("zh-CN", false).key() == key {
		t.Fatal("flipping the light left the key unchanged")
	}
	lampCover := trayMenuApp()
	if lampCover.trayModel("zh-CN", false).key() == key {
		t.Fatal("changing the light source left the key unchanged")
	}
	lampPreset := templateMode()
	lampPreset.snap.LampPreset = "moss"
	if lampPreset.trayModel("zh-CN", false).key() == key {
		t.Fatal("changing the light template left the key unchanged")
	}
	playing := templateMode()
	playing.nowSnap.Sessions[0].Playing = false
	if playing.trayModel("zh-CN", false).key() == key {
		t.Fatal("pausing the media left the key unchanged")
	}
	track := templateMode()
	track.nowSnap.Sessions[0].Title = "Another"
	if track.trayModel("zh-CN", false).key() != key {
		t.Fatal("a track title the menu no longer shows changed the key")
	}
	cover := templateMode()
	cover.nowArtKey = "cover"
	if cover.trayModel("zh-CN", false).key() == key {
		t.Fatal("changing the cover left the key unchanged")
	}
	update := templateMode()
	update.upd.phase = updateAvailable
	update.upd.version = "9.9.9"
	if update.trayModel("zh-CN", false).key() == key {
		t.Fatal("offering an update left the key unchanged")
	}
	idle := templateMode()
	idle.snap.State = sender.StateIdle
	if idle.trayModel("zh-CN", false).key() == key {
		t.Fatal("ending the session left the key unchanged")
	}
}

// TestTrayMenuLampPresetOrder pins the template submenu to lamp.All().
func TestTrayMenuLampPresetOrder(t *testing.T) {
	a := trayMenuApp()
	a.snap.LampCover = false /* this order test pins the template rows */
	parent, _ := trayItem(a.trayMenuItems(a.trayModel("zh-CN", false)), locales.FlyoutLampSection())
	sub := parent.Submenu
	presets := lamp.All()
	if len(sub) != len(presets)+3 {
		t.Fatalf("submenu has %d rows, want %d", len(sub), len(presets)+3)
	}
	if got, want := sub[0].Label, locales.LampTurnOff(); got != want {
		t.Fatalf("power row is %q, want %q", got, want)
	}
	assertTraySeparator(t, sub[1], "after the power row")
	if got, want := sub[2].Label, locales.LampPresetCover(); got != want {
		t.Fatalf("cover row is %q, want %q", got, want)
	}
	for i, p := range presets {
		want := lampPresetName(p.ID)
		if p.ID == "dune" {
			want += " ✔"
		}
		if got := sub[3+i].Label; got != want {
			t.Fatalf("template %d is %q, want %q", i, got, want)
		}
	}
	assertTrayPlain(t, sub)
}
