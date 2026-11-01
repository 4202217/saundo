package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
)

// TestRenderPreviews writes the design's states for a visual check. Run with
// SAUNDO_PREVIEW_DIR set; it is not part of the regular suite.
func TestRenderPreviews(t *testing.T) {
	dir := os.Getenv("SAUNDO_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SAUNDO_PREVIEW_DIR to write preview images")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	save := func(name string, view func(*ui.Context), w, h int, dark bool) {
		tt := ui.NewTester(view, w, h)
		tt.SetDark(dark)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, name+".png"))
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
		for i, step := range []time.Duration{0, 70 * time.Millisecond, 250 * time.Millisecond} {
			if step > 0 {
				tt.Advance(step)
			}
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%d.png", name, i)))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}

	stream := previewApp()
	stream.nowOK = true
	stream.nowTrack.Title = "夜曲"
	stream.nowTrack.Artist = "周杰伦"
	stream.nowTrack.Album = "十一月的萧邦"
	stream.showNowPlaying = true
	stream.snap.ShowNowPlaying = true

	save("flyout-stream-light", stream.view, flyoutWidth, flyoutHeight, false)
	save("flyout-stream-dark", stream.view, flyoutWidth, flyoutHeight, true)

	idle := previewApp()
	idle.snap.State = sender.StateIdle
	idle.snap.ConnectedName = ""
	idle.snap.PTPActive = false
	save("flyout-idle", idle.view, flyoutWidth, flyoutHeight, false)

	connecting := previewApp()
	connecting.snap.State = sender.StateHandshake
	connecting.snap.ConnectedName = ""
	save("flyout-connecting", connecting.view, flyoutWidth, flyoutHeight, false)

	failed := previewApp()
	failed.snap.State = sender.StateIdle
	failed.snap.Error = "接收器没有响应。"
	failed.snap.ConnectedName = ""
	save("flyout-error", failed.view, flyoutWidth, flyoutHeight, false)

	empty := previewApp()
	empty.snap.Devices = nil
	empty.snap.State = sender.StateIdle
	empty.snap.SelectedID = ""
	empty.selID = ""
	save("flyout-empty", empty.view, flyoutWidth, flyoutHeightFor(0), false)

	stereo := previewApp()
	stereo.snap.Devices = nil
	stereo.snap.SelectedID = ""
	stereo.selID = ""
	save("flyout-no-device", stereo.view, flyoutWidth, flyoutHeightFor(0), false)

	pages := []settingsPage{pageGeneral, pageConnection, pageAudio, pageCapture}
	for _, page := range pages {
		a := previewApp()
		a.page = page
		name := []string{"settings-general", "settings-connection", "settings-audio", "settings-capture"}[page]
		save(name+"-light", a.settingsView, 720, 500, false)
	}
	darkSettings := previewApp()
	darkSettings.page = pageConnection
	save("settings-connection-dark", darkSettings.settingsView, 720, 500, true)

	pin := previewApp()
	pin.snap.PinRequired = true
	pin.snap.PinDevice = "客厅 Apple TV"
	pin.pinOpen = true
	pin.pinCode = "3939"
	save("dialog-pin", pin.view, flyoutWidth, flyoutHeight, false)

	password := previewApp()
	password.snap.PasswordRequired = true
	password.snap.PasswordDevice = "MacBook Air"
	password.passwordOpen = true
	password.passwordPrompt = "hunter2"
	save("dialog-password", password.view, flyoutWidth, flyoutHeight, false)

	log := previewApp()
	log.showLog = true
	log.snap.LogLines = []string{
		"2026-11-01 12:00:01 saundo 0.3.0",
		"2026-11-01 12:00:02 discovery: 3 devices",
		"2026-11-01 12:00:03 connect AA:BB:CC:DD:EE:01",
		"2026-11-01 12:00:03 pair-verify ok",
		"2026-11-01 12:00:04 ptp: follower ready",
		"2026-11-01 12:00:05 capture open: PHL 27E2F7901 (NVIDIA High Definition Audio) (48000 Hz)",
	}
	save("dialog-log", log.view, flyoutWidth, flyoutHeight, false)

	flyoutProbe := previewApp()
	frames("probe-flyout-enter", flyoutProbe.view, flyoutWidth, flyoutHeight)
	pinProbe := previewApp()
	pinProbe.snap.PinRequired = true
	pinProbe.snap.PinDevice = "客厅 Apple TV"
	pinProbe.pinOpen = true
	pinProbe.pinCode = "3939"
	frames("probe-pin-enter", pinProbe.view, flyoutWidth, flyoutHeight)
	settingsProbe := previewApp()
	settingsProbe.page = pageConnection
	frames("probe-settings-enter", settingsProbe.settingsView, 720, 500)

	mac := previewApp()
	mac.snap.Devices = []device.Device{{
		ID: "AA:BB:CC:DD:EE:09", Name: "MacBook Air", Model: "Mac14,2",
		Auth: device.AuthHAPPassword, AirPlay2: true, Online: true,
	}}
	mac.snap.SelectedID = ""
	mac.snap.State = sender.StateIdle
	mac.snap.ConnectedName = ""
	mac.selID = ""
	save("flyout-mac", mac.view, flyoutWidth, flyoutHeightFor(1), false)

	hover := previewApp()
	tt := ui.NewTester(hover.view, flyoutWidth, flyoutHeight)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	tt.Move(200, 99)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, "flyout-hover.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
	f.Close()
}
