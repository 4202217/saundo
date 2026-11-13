package app

import (
	"testing"

	"github.com/egoist/mygo"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/sender"
)

func trayItem(items []*mygo.MenuItem, label string) (*mygo.MenuItem, int) {
	for i, it := range items {
		if it.Label == label {
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

// assertTrayPlain fails if any row uses a check column: the menu must have a
// single icon column, so no row is a radio, a checkbox, or checked.
func assertTrayPlain(t *testing.T, items []*mygo.MenuItem) {
	t.Helper()
	for i, it := range items {
		if it.Type == mygo.MenuItemRadio || it.Type == mygo.MenuItemCheckbox || it.Checked {
			t.Fatalf("item %d (%q) uses a check column: type %q, checked %t", i, it.Label, it.Type, it.Checked)
		}
	}
}

func TestTrayMenuShapeSelected(t *testing.T) {
	a := previewApp()
	a.snap.State = sender.StateIdle
	m := a.trayModel("zh-CN", false)
	items := a.trayMenuItems(m)
	if len(items) != 12 {
		t.Fatalf("menu has %d items, want 12: %v", len(items), trayLabels(items))
	}
	assertTrayPlain(t, items)
	for i, d := range a.snap.Devices {
		it := items[i]
		if it.Label != d.Name {
			t.Fatalf("device row %d is %q, want %q", i, it.Label, d.Name)
		}
	}
	if got := m.deviceIcon(m.devices[0]); got != "dev:homepod-mini" {
		t.Fatalf("HomePod row icon key is %q, want dev:homepod-mini", got)
	}
	if got := m.deviceIcon(m.devices[1]); got != "dev:appletv" {
		t.Fatalf("Apple TV row icon key is %q, want dev:appletv", got)
	}
	assertTraySeparator(t, items[3], "after device rows")
	if it := items[4]; it.Type == mygo.MenuItemCheckbox || it.Label != locales.TrayMuteLocal() {
		t.Fatalf("item 4 is %+v, want a normal mute row", it)
	}
	if it := items[5]; it.Label != m.lampLabel() {
		t.Fatalf("item 5 is %q, want %q", it.Label, m.lampLabel())
	}
	assertTraySeparator(t, items[6], "after the toggles")
	if got, want := items[7].Label, locales.TrayShowPanel(); got != want {
		t.Fatalf("item 7 is %q, want %q", got, want)
	}
	if got, want := items[8].Label, locales.TraySettings(); got != want {
		t.Fatalf("item 8 is %q, want %q", got, want)
	}
	assertTraySeparator(t, items[9], "before updates")
	if got, want := items[10].Label, locales.TrayCheckUpdates(); got != want {
		t.Fatalf("item 10 is %q, want %q", got, want)
	}
	if items[11].Role != mygo.RoleQuit || items[11].Label != locales.TrayQuit() {
		t.Fatalf("item 11 is %+v, want the quit role", items[11])
	}
	for _, label := range []string{locales.ActionDisconnect(), locales.TrayConnectLast()} {
		if _, i := trayItem(items, label); i >= 0 {
			t.Fatalf("%q is in the idle menu at %d", label, i)
		}
	}
}

func TestTrayMenuHasNoDisconnectRow(t *testing.T) {
	a := previewApp()
	if a.snap.State != sender.StateStreaming {
		t.Fatalf("fixture state is %v, want streaming", a.snap.State)
	}
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	if _, i := trayItem(items, locales.ActionDisconnect()); i >= 0 {
		t.Fatalf("disconnect row is at %d: %v", i, trayLabels(items))
	}
}

func TestTrayMenuLampRow(t *testing.T) {
	a := previewApp()
	a.snap.LampOff = false
	m := a.trayModel("zh-CN", false)
	if it, _ := trayItem(a.trayMenuItems(m), locales.LampTurnOff()); it == nil {
		t.Fatal("menu has no lamp row while the light is on")
	}
	if got, want := m.lampLabel(), locales.LampTurnOff(); got != want {
		t.Fatalf("lamp label while lit is %q, want %q", got, want)
	}
	if got := m.lampIcon(); got != "lamp" {
		t.Fatalf("lamp icon while lit is %q, want lamp", got)
	}
	a.snap.LampOff = true
	m = a.trayModel("zh-CN", false)
	if got, want := m.lampLabel(), locales.LampTurnOn(); got != want {
		t.Fatalf("lamp label while dark is %q, want %q", got, want)
	}
	if got := m.lampIcon(); got != "lampOff" {
		t.Fatalf("lamp icon while dark is %q, want lampOff", got)
	}
	if _, i := trayItem(a.trayMenuItems(m), locales.LampTurnOn()); i < 0 {
		t.Fatal("menu has no lamp row while the light is off")
	}
}

func TestTrayMenuDeviceIconFollowsSession(t *testing.T) {
	a := previewApp()
	if a.snap.State != sender.StateStreaming {
		t.Fatalf("fixture state is %v, want streaming", a.snap.State)
	}
	m := a.trayModel("zh-CN", false)
	if got := m.deviceIcon(m.devices[0]); got != "devon:homepod-mini" {
		t.Fatalf("in-session row icon key is %q, want devon:homepod-mini", got)
	}
	if got := m.deviceIcon(m.devices[1]); got != "dev:appletv" {
		t.Fatalf("idle row icon key is %q, want dev:appletv", got)
	}
	dark := a.trayModel("zh-CN", true)
	if got := dark.deviceIcon(dark.devices[0]); got != "devond:homepod-mini" {
		t.Fatalf("dark in-session row icon key is %q, want devond:homepod-mini", got)
	}
	a.snap.State = sender.StateIdle
	m = a.trayModel("zh-CN", false)
	if got := m.deviceIcon(m.devices[0]); got != "dev:homepod-mini" {
		t.Fatalf("selected-only row icon key is %q, want the receiver glyph", got)
	}
}

func TestTrayMenuFallbackWithNoDevices(t *testing.T) {
	a := previewApp()
	a.snap.Devices = nil
	a.snap.SelectedID = ""
	a.selID = ""
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	it, i := trayItem(items, locales.TrayConnectLast())
	if i != 0 {
		t.Fatalf("fallback row is at %d, want 0: %v", i, trayLabels(items))
	}
	if !it.Disabled {
		t.Fatal("fallback row is enabled without a remembered device")
	}
	assertTraySeparator(t, items[1], "after fallback")

	a.selID = "AA:BB:CC:DD:EE:01"
	items = a.trayMenuItems(a.trayModel("zh-CN", false))
	it, _ = trayItem(items, locales.TrayConnectLast())
	if it == nil {
		t.Fatal("menu has no fallback row with a remembered device")
	}
	if it.Disabled {
		t.Fatal("fallback row is disabled with a remembered device")
	}
}

func TestTrayMenuFallbackForHiddenTarget(t *testing.T) {
	a := previewApp()
	a.snap.SelectedID = "AA:BB:CC:DD:EE:99"
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	it, i := trayItem(items, locales.TrayConnectLast())
	if i != len(a.snap.Devices) {
		t.Fatalf("fallback row is at %d, want %d: %v", i, len(a.snap.Devices), trayLabels(items))
	}
	if it.Disabled {
		t.Fatal("fallback row is disabled while the remembered device exists")
	}
}

func TestTrayMenuMuteIcon(t *testing.T) {
	a := previewApp()
	a.cfg.MuteLocalOnConnect = false
	it, _ := trayItem(a.trayMenuItems(a.trayModel("zh-CN", false)), locales.TrayMuteLocal())
	if it == nil {
		t.Fatal("menu has no mute row")
	}
	if it.Type != "" {
		t.Fatalf("mute row has type %q, want a normal row", it.Type)
	}
	if got := a.trayModel("zh-CN", false).muteIcon(); got != "bell" {
		t.Fatalf("mute icon with the preference off is %q, want bell", got)
	}
	a.cfg.MuteLocalOnConnect = true
	if got := a.trayModel("zh-CN", false).muteIcon(); got != "bellOff" {
		t.Fatalf("mute icon with the preference on is %q, want bellOff", got)
	}
}

func TestTrayMenuKey(t *testing.T) {
	base := previewApp()
	key := base.trayModel("zh-CN", false).key()
	if got := previewApp().trayModel("zh-CN", false).key(); got != key {
		t.Fatalf("identical state keys differ: %+v and %+v", key, got)
	}
	online := previewApp()
	online.snap.Devices[2].Online = true
	if online.trayModel("zh-CN", false).key() == key {
		t.Fatal("flipping a device online left the key unchanged")
	}
	renamed := previewApp()
	renamed.snap.Devices[1].Name = "Living Room"
	if renamed.trayModel("zh-CN", false).key() == key {
		t.Fatal("renaming a device left the key unchanged")
	}
	lamp := previewApp()
	lamp.snap.LampOff = true
	if lamp.trayModel("zh-CN", false).key() == key {
		t.Fatal("flipping the light left the key unchanged")
	}
}

func TestTrayMenuDeviceSubtitle(t *testing.T) {
	a := previewApp()
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	online, offline := 0, 0
	for i, d := range a.snap.Devices {
		want := ""
		if !d.Online {
			want = locales.ActionOffline()
			offline++
		} else {
			online++
		}
		if got := items[i].Subtitle; got != want {
			t.Fatalf("device row %d subtitle is %q, want %q", i, got, want)
		}
	}
	if online == 0 || offline == 0 {
		t.Fatalf("fixture has %d online and %d offline rows, need both", online, offline)
	}
}
