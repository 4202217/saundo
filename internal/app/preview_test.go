package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/nowplaying"
	"saundo/internal/settings"
)

// TestRenderPreviews writes the design's states for a visual check, in both
// languages. Run with SAUNDO_PREVIEW_DIR set; it is not part of the regular
// suite.
func TestRenderPreviews(t *testing.T) {
	dir := os.Getenv("SAUNDO_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SAUNDO_PREVIEW_DIR to write preview images")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ locale, suffix string }{
		{"en", "en"},
		{"zh-CN", "zh"},
	} {
		// The English README uses the en images and the Chinese README the zh
		// ones, so each language gets its own set.
		testLocale = tc.locale
		renderPreviews(t, dir, tc.locale, tc.suffix)
	}
	testLocale = "zh-CN"
}

// sampleTrack is a fictional track, so no real listening shows in a preview.
func sampleTrack(locale string) nowplaying.Track {
	if locale == "en" {
		return nowplaying.Track{Title: "Night Ferry", Artist: "Signal Coast", Album: "Open Water"}
	}
	return nowplaying.Track{Title: "夜航", Artist: "白鲸电台", Album: "远洋信号"}
}

func renderPreviews(t *testing.T, dir, locale, suffix string) {
	t.Helper()
	// A local build stamps git describe, the widest version the About pill
	// has to hold.
	oldVersion := Version
	Version = "0.4.2-5-g1e8e94c-dirty"
	defer func() { Version = oldVersion }()
	save := func(name string, view func(*ui.Context), w, h int, dark bool) {
		tt := ui.NewTester(view, w, h)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, name+"-"+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	// saveScrolled shows the page scrolled with the pointer over it, so the
	// scrollbar and the fixed title divider are visible.
	saveScrolled := func(name string, view func(*ui.Context), w, h int, dark bool) {
		tt := ui.NewTester(view, w, h)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		tt.Advance(400 * time.Millisecond)
		tt.Move(600, 300)
		tt.Scroll(600, 300, 0, 80)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, name+"-"+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	// frames writes the first frame and two more as the motion runs, to
	// check an enter animation instead of its end state.
	frames := func(name string, view func(*ui.Context), w, h int) {
		tt := ui.NewTester(view, w, h)
		tt.SetScale(2)
		tt.SetLocale(locale)
		for i, step := range []time.Duration{0, 70 * time.Millisecond, 250 * time.Millisecond} {
			if step > 0 {
				tt.Advance(step)
			}
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s-%d.png", name, suffix, i)))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
	// fixture fills the device names that come from the environment rather
	// than the catalog, so a preview reads naturally in its language.
	fixture := func() *App {
		a := previewApp()
		a.nameDraft = locales.AppName()
		a.snap.Name = locales.AppName()
		captureName := "扬声器"
		if locale == "en" {
			captureName = "Speakers"
		}
		a.captureNames[1] = captureName
		a.snap.CaptureName = captureName
		return a
	}

	stream := fixture()
	stream.nowOK = true
	stream.nowTrack = sampleTrack(locale)
	stream.showNowPlaying = true
	stream.snap.ShowNowPlaying = true

	save("flyout-stream-light", stream.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)
	save("flyout-stream-dark", stream.view, flyoutWidth, flyoutHeightFor(deviceRows, true), true)

	idle := fixture()
	idle.snap.State = sender.StateIdle
	idle.snap.ConnectedName = ""
	idle.snap.PTPActive = false
	save("flyout-idle", idle.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)

	connecting := fixture()
	connecting.snap.State = sender.StateHandshake
	connecting.snap.ConnectedName = ""
	save("flyout-connecting", connecting.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)

	failed := fixture()
	failed.snap.State = sender.StateIdle
	failed.snap.Error = "The receiver did not respond."
	if locale != "en" {
		failed.snap.Error = "接收器没有响应。"
	}
	failed.snap.ConnectedName = ""
	save("flyout-error", failed.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)

	customFly := fixture()
	customFly.latencyMode = settings.LatencyCustom
	customFly.latencyCustom = 700
	customFly.latency = 700
	customFly.snap.LatencyMS = 700
	save("flyout-custom-light", customFly.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)
	save("flyout-custom-dark", customFly.view, flyoutWidth, flyoutHeightFor(deviceRows, true), true)

	offFly := fixture()
	offFly.showLatencySwitch = false
	save("flyout-switch-off", offFly.view, flyoutWidth, flyoutHeightFor(deviceRows, false), false)

	applying := fixture()
	applying.latencyMode = settings.LatencyStandard
	applying.latency = 1500
	applying.latencyApplying = true
	save("flyout-latency-applying", applying.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)
	save("flyout-latency-applying-dark", applying.view, flyoutWidth, flyoutHeightFor(deviceRows, true), true)

	empty := fixture()
	empty.snap.Devices = nil
	empty.snap.State = sender.StateIdle
	empty.snap.SelectedID = ""
	empty.selID = ""
	save("flyout-empty", empty.view, flyoutWidth, flyoutHeightFor(0, true), false)

	stereo := fixture()
	stereo.snap.Devices = nil
	stereo.snap.SelectedID = ""
	stereo.selID = ""
	save("flyout-no-device", stereo.view, flyoutWidth, flyoutHeightFor(0, true), false)

	pageNames := map[settingsPage]string{
		pageGeneral:    "settings-general",
		pageConnection: "settings-connection",
		pageReceivers:  "settings-receivers",
		pageAudio:      "settings-audio",
		pageLamp:       "settings-lamp",
		pageCapture:    "settings-capture",
		pageAbout:      "settings-about",
	}
	pages := []settingsPage{pageGeneral, pageConnection, pageReceivers, pageAudio, pageLamp, pageCapture, pageAbout}
	for _, page := range pages {
		a := fixture()
		a.page = page
		if page == pageReceivers {
			a.applyReceiverFixtures()
		}
		save(pageNames[page]+"-light", a.settingsView, 720, 500, false)
	}
	darkSettings := fixture()
	darkSettings.page = pageConnection
	save("settings-connection-dark", darkSettings.settingsView, 720, 500, true)
	darkAbout := fixture()
	darkAbout.page = pageAbout
	save("settings-about-dark", darkAbout.settingsView, 720, 500, true)

	// The receivers page: an expanded row, and the hidden group open.
	receiversPage := fixture()
	receiversPage.page = pageReceivers
	receiversPage.applyReceiverFixtures()
	receiversPage.receiverOpen = map[string]bool{"m:ab12": true}
	receiversPage.receiverHiddenOpen = true
	save("settings-receivers-expanded-light", receiversPage.settingsView, 720, 500, false)
	save("settings-receivers-expanded-dark", receiversPage.settingsView, 720, 500, true)

	receiversHidden := fixture()
	receiversHidden.page = pageReceivers
	receiversHidden.applyReceiverFixtures()
	receiversHidden.receiverHiddenOpen = true
	save("settings-receivers-hidden-light", receiversHidden.settingsView, 720, 500, false)

	receiversEmpty := fixture()
	receiversEmpty.page = pageReceivers
	receiversEmpty.snap.Receivers = nil
	receiversEmpty.snap.Devices = nil
	receiversEmpty.snap.SelectedID = ""
	receiversEmpty.snap.ConnectedName = ""
	receiversEmpty.snap.State = sender.StateIdle
	receiversEmpty.selID = ""
	save("settings-receivers-empty-light", receiversEmpty.settingsView, 720, 500, false)
	save("settings-receivers-empty-dark", receiversEmpty.settingsView, 720, 500, true)

	receiversAllHidden := receiverFixture()
	receiversAllHidden.page = pageReceivers
	receiversAllHidden.snap.Receivers = receiversAllHidden.snap.Receivers[1:2]
	receiversAllHidden.snap.SelectedID = ""
	receiversAllHidden.snap.ConnectedName = ""
	receiversAllHidden.snap.State = sender.StateIdle
	receiversAllHidden.selID = ""
	save("settings-receivers-all-hidden-light", receiversAllHidden.settingsView, 720, 500, false)
	save("settings-receivers-all-hidden-dark", receiversAllHidden.settingsView, 720, 500, true)
	receiversPage.openAddReceiver()
	save("dialog-receiver-add", receiversPage.settingsView, 720, 500, false)
	receiversPage.closeReceiverDialog()
	if v, ok := receiversPage.receiverViewFor("m:ab12"); ok {
		receiversPage.beginEditReceiver(v)
		save("dialog-receiver-edit", receiversPage.settingsView, 720, 500, false)
		receiversPage.openRemoveReceiver(v)
		save("dialog-receiver-remove", receiversPage.settingsView, 720, 500, false)
		receiversPage.closeReceiverDialog()
		receiversPage.receiverDialog = receiverDialogState{kind: receiverDialogPassword, key: v.Key, entered: false}
		receiversPage.receiverDialogOpen = true
		save("dialog-receiver-password", receiversPage.settingsView, 720, 500, false)
	}

	customSettings := fixture()
	customSettings.page = pageAudio
	customSettings.latencyMode = settings.LatencyCustom
	customSettings.latencyCustom = 700
	customSettings.latency = 700
	save("settings-audio-custom-light", customSettings.settingsView, 720, 500, false)
	save("settings-audio-custom-dark", customSettings.settingsView, 720, 500, true)
	save("settings-audio-compact-light", customSettings.settingsView, 720, 420, false)
	save("settings-audio-compact-dark", customSettings.settingsView, 720, 420, true)
	saveScrolled("settings-audio-compact-scrolled-light", customSettings.settingsView, 720, 420, false)
	saveScrolled("settings-audio-compact-scrolled-dark", customSettings.settingsView, 720, 420, true)

	lampOff := fixture()
	lampOff.page = pageLamp
	lampOff.lampEnabled = false
	lampOff.snap.LampEnabled = false
	save("settings-lamp-off-light", lampOff.settingsView, 720, 500, false)
	save("settings-lamp-off-dark", lampOff.settingsView, 720, 500, true)

	lampFly := fixture()
	lampFly.lampExpanded = true
	save("flyout-lamp-expanded-light", lampFly.view, flyoutWidth,
		flyoutHeightForState(deviceRows, true, true), false)
	save("flyout-lamp-expanded-dark", lampFly.view, flyoutWidth,
		flyoutHeightForState(deviceRows, true, true), true)
	lampFly.lampEnabled = false
	lampFly.snap.LampEnabled = false
	save("flyout-lamp-off-light", lampFly.view, flyoutWidth,
		flyoutHeightForState(deviceRows, true, true), false)

	pin := fixture()
	pin.snap.PinRequired = true
	pin.snap.PinDevice = "Apple TV"
	pin.pinOpen = true
	pin.pinCode = "3939"
	save("dialog-pin", pin.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)

	password := fixture()
	password.snap.PasswordRequired = true
	password.snap.PasswordDevice = "MacBook"
	password.passwordOpen = true
	password.passwordPrompt = "hunter2"
	save("dialog-password", password.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)

	log := fixture()
	log.showLog = true
	log.snap.LogLines = []string{
		"2026-11-01 12:00:01 saundo 0.3.0",
		"2026-11-01 12:00:02 discovery: 3 devices",
		"2026-11-01 12:00:03 connect AA:BB:CC:DD:EE:01",
		"2026-11-01 12:00:03 pair-verify ok",
		"2026-11-01 12:00:04 ptp: follower ready",
		"2026-11-01 12:00:05 capture open: PHL 27E2F7901 (NVIDIA High Definition Audio) (48000 Hz)",
	}
	save("dialog-log", log.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)

	frames("probe-flyout-enter", fixture().view, flyoutWidth, flyoutHeightFor(deviceRows, true))
	pinProbe := fixture()
	pinProbe.snap.PinRequired = true
	pinProbe.snap.PinDevice = "Apple TV"
	pinProbe.pinOpen = true
	pinProbe.pinCode = "3939"
	frames("probe-pin-enter", pinProbe.view, flyoutWidth, flyoutHeightFor(deviceRows, true))
	settingsProbe := fixture()
	settingsProbe.page = pageConnection
	frames("probe-settings-enter", settingsProbe.settingsView, 720, 500)

	mac := fixture()
	mac.snap.Devices = []device.Device{{
		ID: "AA:BB:CC:DD:EE:09", Name: "MacBook", Model: "Mac",
		Auth: device.AuthHAPPassword, AirPlay2: true, Online: true,
	}}
	mac.snap.SelectedID = ""
	mac.snap.State = sender.StateIdle
	mac.snap.ConnectedName = ""
	mac.selID = ""
	save("flyout-mac", mac.view, flyoutWidth, flyoutHeightFor(1, true), false)

	hover := fixture()
	tt := ui.NewTester(hover.view, flyoutWidth, flyoutHeightFor(deviceRows, true))
	tt.SetScale(2)
	tt.SetLocale(locale)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	tt.Move(200, 99)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, "flyout-hover-"+suffix+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
	f.Close()

	upd := fixture()
	upd.cfg = settings.Default()
	upd.autoUpdate = true
	upd.upd = updateState{
		phase:   updateAvailable,
		version: "0.4.3",
		date:    time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC),
		notes: `### Added

- Per-app capture remembers the last application you streamed.

### Fixed

- The tray menu no longer stays checked after a device drops.
`,
	}
	upd.history = []releaseNote{
		{Version: "0.4.2", Date: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
			Notes: `### Changed

- The update dialog now lists every version since your last update.
`},
		{Version: "0.4.1", Date: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
			Notes: `### Fixed

- The volume readout now shows 0% to 100%.
- The settings window uses one background.
`},
	}
	upd.historyState = historyReady
	upd.page = pageUpdate
	save("settings-update-available", upd.settingsView, 720, 500, false)
	upd.notesOpen = true
	upd.notesRange = true
	upd.notesExpanded = map[string]bool{"0.4.2": true}
	save("settings-update-notes", upd.settingsView, 720, 500, false)
	upd.notesExpanded = map[string]bool{}
	upd.notesRange = false
	save("settings-update-notes-latest", upd.settingsView, 720, 500, false)
	upd.notesRange = true
	upd.notesOpen = false
	upd.upd.phase = updateDownloading
	upd.upd.got, upd.upd.total = 12_400_000, 20_100_000
	save("settings-update-downloading", upd.settingsView, 720, 500, false)
	upd.upd.phase = updateReady
	save("settings-update-ready", upd.settingsView, 720, 500, false)
	upd.upd.phase = updateAvailable
	save("flyout-update-chip", upd.view, flyoutWidth, flyoutHeightFor(deviceRows, true), false)

	upd.upd = updateState{phase: updateChecking}
	save("settings-update-checking", upd.settingsView, 720, 500, false)
	upd.upd = updateState{phase: updateIdle, checked: true}
	save("settings-update-uptodate", upd.settingsView, 720, 500, false)
	upd.upd = updateState{phase: updateFailed, err: "network unreachable"}
	save("settings-update-failed", upd.settingsView, 720, 500, false)
}
