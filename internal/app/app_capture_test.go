package app

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/capture"
	"saundo/internal/engine"
)

// TestAppCapturePickerChoosesMultipleApps drives the checked menu: two apps
// on, one off, all applied live without a Done step.
func TestAppCapturePickerChoosesMultipleApps(t *testing.T) {
	if !capture.MultiAppSupported() {
		t.Skip("multi-select is unsupported on this platform")
	}
	a := previewApp()
	a.page = pageCapture
	a.appIcons = map[uint32]*ui.Bitmap{
		1234: ui.NewBitmap(testIcon()),
		5678: ui.NewBitmap(testIcon()),
	}
	t.Cleanup(func() {
		if a.appApplyTimer != nil {
			a.appApplyTimer.Stop()
		}
	})
	tt := ui.NewTester(a.settingsView, 720, 500)
	if err := tt.Click(a.appSelectionLabel()); err != nil {
		t.Fatalf("open picker: %v", err)
	}
	if err := tt.Click("Spotify"); err != nil {
		t.Fatalf("choose Spotify: %v", err)
	}
	if err := tt.Click("Music"); err != nil {
		t.Fatalf("choose Music: %v", err)
	}
	if !a.appSelection["com.spotify.client"] || !a.appSelection["com.apple.Music"] {
		t.Fatalf("selection = %v, want both apps", a.appSelection)
	}
	if got := a.appSelectionLabel(); got != locales.SettingsAppCaptureCount(2) {
		t.Fatalf("label = %q, want the count", got)
	}
	if err := tt.Click("Spotify"); err != nil {
		t.Fatalf("deselect Spotify: %v", err)
	}
	if a.appSelection["com.spotify.client"] {
		t.Fatalf("selection = %v, want Spotify cleared", a.appSelection)
	}
	if got := a.appSelectionLabel(); got != "Music" {
		t.Fatalf("label = %q, want the remaining app", got)
	}
}

// TestMigrateLegacyAppSelection turns the old single PID into the app's
// stable bundle ID once the list is loaded.
func TestMigrateLegacyAppSelection(t *testing.T) {
	if !capture.MultiAppSupported() {
		t.Skip("multi-select is unsupported on this platform")
	}
	a := previewApp()
	a.snap.CaptureProcessPID = 1234
	a.migrateLegacyAppSelection()
	if !a.appSelection["com.spotify.client"] {
		t.Fatalf("selection = %v, want the Spotify bundle ID", a.appSelection)
	}
}

// TestConnectionMuteCopyNamesTheSelectedApp pins the per-app wording: the
// switch mutes the captured app, not the whole output device.
func TestConnectionMuteCopyNamesTheSelectedApp(t *testing.T) {
	if !capture.ProcessMuteSupported() {
		t.Skip("process mute is unsupported on this platform")
	}
	a := previewApp()
	a.page = pageConnection
	a.cfg.MuteLocalOnConnect = true
	a.snap.CaptureProcessPID = 1234
	a.snap.CaptureProcessName = "Music"
	a.snap.MuteLocal = engine.MuteLocalActive
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		locales.SettingsMuteLocalApp(),
		locales.SettingsMuteLocalDescAppOn(),
		locales.SettingsMuteLocalAppNote(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("connection page is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func testIcon() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: 0x40, G: 0x80, B: 0xc0, A: 0xff})
		}
	}
	return img
}

// TestCapturePageHidesPerAppWhenUnsupported pins the product decision: a
// platform without per-app capture shows only the system source and status.
func TestCapturePageHidesPerAppWhenUnsupported(t *testing.T) {
	if capture.AppCaptureSupported() {
		t.Skip("per-app capture is offered on this platform")
	}
	a := previewApp()
	a.page = pageCapture
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, copy := range []string{locales.SettingsAppCapture(), locales.SettingsAppCaptureDesc()} {
		if tt.HasText(copy) {
			t.Fatalf("the capture page still shows %q", copy)
		}
	}
}
