package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
	"saundo/internal/receivers"
)

// targetFixture gives the preview app two live targets and selects the second,
// the case the per-device rows must keep apart.
func targetFixture() *App {
	a := previewApp()
	a.snap.Devices = []device.Device{
		{ID: "AA:BB:CC:DD:EE:01", Name: "HomePods", Model: "AudioAccessory5,1",
			Auth: device.AuthHAPTransient, AirPlay2: true, Online: true},
		{ID: "AA:BB:CC:DD:EE:04", Name: "HomePod L", Model: "AudioAccessory5,1",
			Auth: device.AuthHAPTransient, AirPlay2: true, Online: true},
	}
	a.snap.ActiveTargets = []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:04"}
	a.snap.Targets = []engine.TargetControl{
		{ID: "AA:BB:CC:DD:EE:01", Name: "HomePods", Volume: 40, Channel: "stereo", Stereo: true, State: engine.TargetMember},
		{ID: "AA:BB:CC:DD:EE:04", Name: "HomePod L", Volume: 70, Channel: "left", State: engine.TargetMember},
	}
	a.selID = "AA:BB:CC:DD:EE:04"
	a.applySnapshot(a.snap)
	return a
}

// TestFlyoutDeviceRowCarriesItsOwnVolume pins the per-device slider: every live
// row shows its own percent and a slider named for its device, and a change
// writes that target only.
func TestFlyoutDeviceRowCarriesItsOwnVolume(t *testing.T) {
	a := targetFixture()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	for _, want := range []string{
		locales.VolumeFor("HomePods"), locales.VolumeFor("HomePod L"), "40%", "70%",
	} {
		if !tt.HasText(want) {
			t.Fatalf("flyout is missing %q; texts: %q", want, tt.Texts())
		}
	}
	label := locales.VolumeFor("HomePod L")
	if _, ok := tt.Find(label); !ok {
		t.Fatal("the HomePod L slider has no accessible name")
	}
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	var left, pods float64
	for _, tc := range a.snap.Targets {
		switch tc.ID {
		case "AA:BB:CC:DD:EE:04":
			left = tc.Volume
		case "AA:BB:CC:DD:EE:01":
			pods = tc.Volume
		}
	}
	if left == 70 {
		t.Fatalf("HomePod L volume did not move: %v", left)
	}
	if pods != 40 {
		t.Fatalf("HomePods volume moved with another row: %v", pods)
	}
}

// TestFlyoutVolumeButtonNamesItsAction pins the speaker button: its label is
// the mute action at a normal level and the unmute action at zero.
func TestFlyoutVolumeButtonNamesItsAction(t *testing.T) {
	a := targetFixture()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if !tt.HasText(locales.MuteFor("HomePods")) {
		t.Fatalf("flyout is missing the mute action for HomePods; texts: %q", tt.Texts())
	}
	for i := range a.snap.Targets {
		if a.snap.Targets[i].ID == "AA:BB:CC:DD:EE:01" {
			a.snap.Targets[i].Volume = 0
		}
	}
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if !tt.HasText(locales.UnmuteFor("HomePods")) {
		t.Fatalf("muted row is missing the unmute action; texts: %q", tt.Texts())
	}
}

// setTargetState moves one fixture target to a published role.
func setTargetState(a *App, id string, state engine.TargetState) {
	for i := range a.snap.Targets {
		if a.snap.Targets[i].ID == id {
			a.snap.Targets[i].State = state
		}
	}
}

func countText(tt *ui.Tester, want string) int {
	n := 0
	for _, text := range tt.Texts() {
		if text == want {
			n++
		}
	}
	return n
}

// TestFlyoutJoiningRowHidesMixer pins the row shapes while a target joins: the
// established member keeps its own mixer, and the joining row is plain with
// the connecting state and the cancel action.
func TestFlyoutJoiningRowHidesMixer(t *testing.T) {
	a := targetFixture()
	a.snap.State = sender.StateHandshake
	setTargetState(a, "AA:BB:CC:DD:EE:04", engine.TargetJoining)
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	for _, want := range []string{
		locales.VolumeFor("HomePods"), locales.MuteFor("HomePods"),
	} {
		if !tt.HasText(want) {
			t.Fatalf("the member lost %q during a join; texts: %q", want, tt.Texts())
		}
	}
	for _, absent := range []string{locales.VolumeFor("HomePod L"), locales.MuteFor("HomePod L")} {
		if tt.HasText(absent) {
			t.Fatalf("the joining row offers %q; texts: %q", absent, tt.Texts())
		}
	}
	if !tt.HasText(locales.ActionCancel()) {
		t.Fatalf("the joining row is missing its cancel action; texts: %q", tt.Texts())
	}
	// The row subtitle and the footer both read the connecting state.
	if got := countText(tt, locales.StatusConnecting()); got != 2 {
		t.Fatalf("connecting copy appears %d times, want the row and the footer; texts: %q", got, tt.Texts())
	}
}

// TestRebuildRowsOfferNoCancel pins the rebuild: every session row is a member
// and keeps its mixer, so no row offers 取消.
func TestRebuildRowsOfferNoCancel(t *testing.T) {
	a := targetFixture()
	a.snap.State = sender.StateHandshake
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if tt.HasText(locales.ActionCancel()) {
		t.Fatalf("a rebuild offered cancel; texts: %q", tt.Texts())
	}
	if got := countText(tt, locales.ActionDisconnect()); got != 4 {
		t.Fatalf("disconnect labels = %d, want two members (label and text); texts: %q", got, tt.Texts())
	}
	if !tt.HasText(locales.VolumeFor("HomePods")) || !tt.HasText(locales.VolumeFor("HomePod L")) {
		t.Fatalf("a member lost its mixer during the rebuild; texts: %q", tt.Texts())
	}
}

// TestReopeningRowSaysReconnecting pins the automatic-retry wait: each wanted
// row is plain, reads the reconnecting state, and offers disconnect, never
// cancel.
func TestReopeningRowSaysReconnecting(t *testing.T) {
	a := targetFixture()
	a.snap.State = sender.StateHandshake
	setTargetState(a, "AA:BB:CC:DD:EE:01", engine.TargetReopening)
	setTargetState(a, "AA:BB:CC:DD:EE:04", engine.TargetReopening)
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if !tt.HasText(locales.StatusReconnecting()) {
		t.Fatalf("reopening rows do not say reconnecting; texts: %q", tt.Texts())
	}
	if tt.HasText(locales.ActionCancel()) {
		t.Fatalf("an automatic retry offered cancel; texts: %q", tt.Texts())
	}
	if !tt.HasText(locales.ActionDisconnect()) {
		t.Fatalf("reopening rows are missing disconnect; texts: %q", tt.Texts())
	}
}

// TestRowActionFollowsTargetState pins the row policy: the published state
// names the verb, and a device with no role falls back to the phase.
func TestRowActionFollowsTargetState(t *testing.T) {
	a := targetFixture()
	join := device.Device{ID: "AA:BB:CC:DD:EE:04", Online: true}
	other := device.Device{ID: "AA:BB:CC:DD:EE:01", Online: true}

	setTargetState(a, join.ID, engine.TargetJoining)
	if got := rowActionFor(a.snap, join); got != actionCancel {
		t.Fatalf("joining row action = %v, want cancel", got)
	}
	if got := rowActionFor(a.snap, other); got != actionDisconnect {
		t.Fatalf("member row action = %v, want disconnect", got)
	}
	setTargetState(a, join.ID, engine.TargetMember)
	if got := rowActionFor(a.snap, join); got != actionDisconnect {
		t.Fatalf("reopened member action = %v, want disconnect", got)
	}
	setTargetState(a, join.ID, engine.TargetReopening)
	if got := rowActionFor(a.snap, join); got != actionDisconnect {
		t.Fatalf("automatic reconnect action = %v, want disconnect", got)
	}
	setTargetState(a, join.ID, engine.TargetFailed)
	if got := rowActionFor(a.snap, join); got != actionRetry {
		t.Fatalf("failed row action = %v, want retry", got)
	}

	plain := device.Device{ID: "AA:00", Online: true}
	if got := rowActionFor(a.snap, plain); got != actionConnect {
		t.Fatalf("plain row action = %v, want connect", got)
	}
	a.snap.State = sender.StateHandshake
	if got := rowActionFor(a.snap, plain); got != actionConnectDisabled {
		t.Fatalf("plain row during an attempt = %v, want disabled connect", got)
	}
	if got := rowActionFor(a.snap, device.Device{ID: "AA:01"}); got != actionOffline {
		t.Fatalf("offline row action = %v, want offline", got)
	}
}

// TestFlyoutDisconnectAllButtonFollowsSession pins the device head's
// one-click disconnect: shown while two or more targets are live, absent for
// a single target (its row already carries 断开连接) and when idle.
func TestFlyoutDisconnectAllButtonFollowsSession(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if tt.HasText(locales.DisconnectAll()) {
		t.Fatalf("one-target flyout offers disconnect all; texts: %q", tt.Texts())
	}
	a.snap.ActiveTargets = []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02"}
	a.snap.Targets = append(a.snap.Targets, engine.TargetControl{
		ID: "AA:BB:CC:DD:EE:02", Name: "Apple TV", Volume: 50, Channel: "stereo", State: engine.TargetMember,
	})
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if !tt.HasText(locales.DisconnectAll()) {
		t.Fatalf("two-target flyout is missing the disconnect-all action; texts: %q", tt.Texts())
	}
	a.snap.ActiveTargets = nil
	a.snap.Targets = nil
	a.snap.State = sender.StateIdle
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if tt.HasText(locales.DisconnectAll()) {
		t.Fatalf("idle flyout still offers disconnect all; texts: %q", tt.Texts())
	}
}

// TestFlyoutChannelButtonNamesItsState pins the icon-only button's accessible
// copy: the current side and the side a click reaches.
func TestFlyoutChannelButtonNamesItsState(t *testing.T) {
	a := targetFixture()
	label := locales.ChannelTooltip(locales.ChannelLeft(), locales.ChannelRight())
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if !tt.HasText(label) {
		t.Fatalf("flyout is missing the channel label %q; texts: %q", label, tt.Texts())
	}
}

// TestNextChannelCycles pins the click order the button walks.
func TestNextChannelCycles(t *testing.T) {
	if got := nextChannel(receivers.ChannelStereo); got != receivers.ChannelLeft {
		t.Fatalf("stereo -> %q", got)
	}
	if got := nextChannel(receivers.ChannelLeft); got != receivers.ChannelRight {
		t.Fatalf("left -> %q", got)
	}
	if got := nextChannel(receivers.ChannelRight); got != receivers.ChannelStereo {
		t.Fatalf("right -> %q", got)
	}
}

// TestFlyoutPairChannelButtonIsFixed pins the pair rule in the flyout: a
// stereo target keeps its volume but offers no channel control, because its
// two members play one side each and there is no side for the user to switch.
func TestFlyoutPairChannelButtonIsFixed(t *testing.T) {
	a := previewApp()
	pair := device.Device{
		ID: "stereo:1", Name: "客厅立体声", Model: "AudioAccessory5,1", StereoID: "1", Online: true,
		Members: []device.Device{{ID: "AA:10", Name: "L"}, {ID: "AA:11", Name: "R"}},
	}
	a.snap.Devices = []device.Device{pair}
	a.snap.ActiveTargets = []string{pair.ID}
	a.snap.Targets = []engine.TargetControl{
		{ID: pair.ID, Name: pair.Name, Volume: 52, Channel: receivers.ChannelStereo, Stereo: true, State: engine.TargetMember},
	}
	a.selectDevice(pair.ID)
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	for _, absent := range []string{
		locales.ChannelTooltip(locales.ChannelStereo(), locales.ChannelLeft()),
	} {
		if tt.HasText(absent) {
			t.Fatalf("pair row still offers %q; texts: %q", absent, tt.Texts())
		}
	}
	if !tt.HasText(locales.VolumeFor(pair.Name)) {
		t.Fatalf("pair row lost its volume; texts: %q", tt.Texts())
	}
}

// TestFlyoutPlaybackSectionHidesWhenEmpty pins the removed volume row: the
// section is only drawn when it has a media card or the latency row.
func TestFlyoutPlaybackSectionHidesWhenEmpty(t *testing.T) {
	a := previewApp()
	a.showLatencySwitch = false
	a.cfg.ShowNowPlayingPanel = false
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if _, ok := tt.Find(locales.FlyoutPlayback()); ok {
		t.Fatalf("empty playback section is drawn; texts: %q", tt.Texts())
	}
	a.showLatencySwitch = true
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if _, ok := tt.Find(locales.FlyoutPlayback()); !ok {
		t.Fatal("the latency row does not bring the playback section back")
	}
}

// TestFlyoutRowsShareOneHeight pins the uniform device card: a card of three
// devices is the same height with no target as with one.
func TestFlyoutRowsShareOneHeight(t *testing.T) {
	a := previewApp()
	a.snap.ActiveTargets = nil
	plain := deviceCardRect(t, a)
	a.snap.ActiveTargets = []string{"AA:BB:CC:DD:EE:01"}
	a.snap.Targets = []engine.TargetControl{{ID: "AA:BB:CC:DD:EE:01", Name: "HomePods", Volume: 40, Channel: "stereo", Stereo: true}}
	live := deviceCardRect(t, a)
	if plain.H != live.H {
		t.Fatalf("device card height changed with a live target: %.1f vs %.1f", plain.H, live.H)
	}
	if want := float32(3*deviceRowHeight + 2); plain.H != want {
		t.Fatalf("device card height = %.1f, want %.1f", plain.H, want)
	}
}

func deviceCardRect(t *testing.T, a *App) ui.Rect {
	t.Helper()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	r, ok := tt.Find(locales.DeviceListLabel())
	if !ok {
		t.Fatalf("device card is missing; texts: %q", tt.Texts())
	}
	return r
}

// TestReceiverRowShowsVolumeAndChannel pins the settings audio group: the
// volume slider and the three channel choices for a single receiver. The
// per-device delay control stays out of the page.
func TestReceiverRowShowsVolumeAndChannel(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"AA:00": true}
	tt := ui.NewTester(a.settingsView, 720, 700)
	for _, want := range []string{
		locales.VolumeLabel(), locales.ChannelLabel(), locales.ChannelStereo(),
		locales.ChannelLeft(), locales.ChannelRight(), locales.ReceiverChannelDesc(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("receiver row is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

// TestReceiverPairChannelIsDisabledStereo pins the pair rule in the page: a
// stereo pair shows its one shared volume, a disabled 立体声, and no side.
func TestReceiverPairChannelIsDisabledStereo(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"stereo:1": true}
	tt := ui.NewTester(a.settingsView, 720, 700)
	for _, want := range []string{locales.VolumeLabel(), locales.ChannelLabel(), locales.ChannelStereo()} {
		if !tt.HasText(want) {
			t.Fatalf("pair row is missing %q; texts: %q", want, tt.Texts())
		}
	}
	for _, absent := range []string{locales.ChannelLeft(), locales.ChannelRight(), locales.ReceiverChannelDesc()} {
		if hasExactText(tt, absent) {
			t.Fatalf("pair row offers the single-receiver option %q; texts: %q", absent, tt.Texts())
		}
	}
}
