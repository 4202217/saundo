package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// idleFrames counts the frames a view asks for over a second at 60 a
// second, once its entrance has settled.
func idleFrames(t *testing.T, tt *ui.Tester) int {
	t.Helper()
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	asked := 0
	for range 60 {
		tt.Advance(time.Second / 60)
		if tt.FramePending() {
			asked++
			tt.Frame()
		}
	}
	return asked
}

// TestFlyoutIdleRequestsNoFrames pins the static flyout: with devices listed
// and nothing moving, it asks for no frames.
func TestFlyoutIdleRequestsNoFrames(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	if n := idleFrames(t, tt); n > 1 {
		t.Fatalf("the idle flyout asked for %d frames a second", n)
	}
}

// TestSettingsIdleRequestsNoFrames does the same for the settings window.
func TestSettingsIdleRequestsNoFrames(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.settingsView, 720, 500)
	if n := idleFrames(t, tt); n > 1 {
		t.Fatalf("the idle settings page asked for %d frames a second", n)
	}
}

// TestEmptyStatePulsesUntilDevicesArrive pins the searching indicator: it
// keeps frames coming while no device shows, and stops once one does.
func TestEmptyStatePulsesUntilDevicesArrive(t *testing.T) {
	a := previewApp()
	devices := a.snap.Devices
	a.snap.Devices = nil
	a.snap.SelectedID = ""
	a.selID = ""
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(0))
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if n := idleFrames(t, tt); n == 0 {
		t.Fatal("the searching state asked for no frames")
	}

	a.snap.Devices = devices
	a.snap.SelectedID = "AA:BB:CC:DD:EE:01"
	a.selID = "AA:BB:CC:DD:EE:01"
	tt.Frame()
	if n := idleFrames(t, tt); n > 1 {
		t.Fatalf("the flyout still asked for %d frames after devices arrived", n)
	}
}
