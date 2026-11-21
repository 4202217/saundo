package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
)

// The flyout row's marks sit below their line's geometric center: text glyphs
// ride low in their line box, so a box-centered chip or icon reads high beside
// them. The drops live in iconChip, inlineChip, countChip, stereoLead,
// channelButton, and volumeButton; these tests pin the layout they produce.

func rowAlignCenter(r ui.Rect) float32 { return r.Y + r.H/2 }

// needsPasswordApp is a flyout whose third device is idle with password auth
// and nothing saved, so its plain row carries the needs-password pill.
func needsPasswordApp() *App {
	a := previewApp()
	const id = "AA:BB:CC:DD:EE:03"
	a.snap.Devices[2].Auth = device.AuthHAPPassword
	a.snap.Receivers = []receivers.View{{
		Key: id, Name: "HomePod mini", Model: "AudioAccessory5,1",
		Device: a.snap.Devices[2],
		Entry:  receivers.Entry{Key: id, Kind: receivers.KindDiscovered, Name: "HomePod mini"},
	}}
	return a
}

// TestAuthPillDropsBelowTheNameLine pins the pill's half-DIP drop. The plain
// row's pick button starts at the name line, whose 26 DIP carry the 20-DIP
// pill at (26-20)/2; the pill's own margin adds the 0.5 DIP that levels it
// with the name's low-riding glyphs.
func TestAuthPillDropsBelowTheNameLine(t *testing.T) {
	a := needsPasswordApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	pill, ok := tt.Find(locales.DeviceNeedsPassword())
	if !ok {
		t.Fatalf("the %s pill is missing; texts: %q", locales.DeviceNeedsPassword(), tt.Texts())
	}
	pick, ok := tt.Find("HomePod mini")
	if !ok {
		t.Fatalf("the idle row is missing; texts: %q", tt.Texts())
	}
	want := pick.Y + (26-pill.H)/2 + 0.5
	if diff := pill.Y - want; diff > 0.01 || diff < -0.01 {
		t.Fatalf("the pill sits at %.2f, want %.2f: the 0.5 DIP drop beside the name is gone", pill.Y, want)
	}
}

// TestStereoLeadDropsBelowTheNameLine pins the stereo mark's full-DIP drop:
// its 26-DIP box centers a DIP below the 26-DIP name line, where the glyphs
// of the name sit.
func TestStereoLeadDropsBelowTheNameLine(t *testing.T) {
	a := previewApp()
	a.snap.Targets[0].Stereo = true
	a.snap.ConnectedName = "客厅"
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	name, ok := tt.Find("HomePods")
	if !ok {
		t.Fatalf("the member name is missing; texts: %q", tt.Texts())
	}
	lead, ok := tt.Find(locales.ChannelStereo())
	if !ok {
		t.Fatalf("the stereo mark is missing; texts: %q", tt.Texts())
	}
	if diff := rowAlignCenter(lead) - rowAlignCenter(name); diff > 1.01 || diff < 0.99 {
		t.Fatalf("the stereo mark centers %.2f DIP from the name line, want 1.0", diff)
	}
}

// TestMixerControlsShareOneCenter pins the volume line's boxes: the mute
// button and the slider keep one center, so the glyph's drop inside the
// button is what levels it with the rail, and a later edit cannot fix the
// glyph by moving the whole button off the slider.
func TestMixerControlsShareOneCenter(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	mute, ok := tt.Find(locales.MuteFor("HomePods"))
	if !ok {
		t.Fatalf("the mute button is missing; texts: %q", tt.Texts())
	}
	slider, ok := tt.Find(locales.VolumeFor("HomePods"))
	if !ok {
		t.Fatalf("the volume slider is missing; texts: %q", tt.Texts())
	}
	if diff := rowAlignCenter(mute) - rowAlignCenter(slider); diff > 0.01 || diff < -0.01 {
		t.Fatalf("the mute button centers %.2f DIP off the slider, want 0", diff)
	}
}

// TestMixerLeadInTightened pins the mixer's lead-in: the mark boxes are 16 DIP
// and sit on the content column's left edge, the name and the rail start one
// 6 DIP gap after them, and the name and the rail share that one column, so
// the marks read as a tight lead-in instead of floating in dead space.
func TestMixerLeadInTightened(t *testing.T) {
	a := previewApp()
	a.snap.Targets[0].Stereo = true
	a.snap.ConnectedName = "客厅"
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	lead, ok := tt.Find(locales.ChannelStereo())
	if !ok {
		t.Fatalf("the stereo mark is missing; texts: %q", tt.Texts())
	}
	name, ok := tt.Find("HomePods")
	if !ok {
		t.Fatalf("the member name is missing; texts: %q", tt.Texts())
	}
	mute, ok := tt.Find(locales.MuteFor("HomePods"))
	if !ok {
		t.Fatalf("the mute button is missing; texts: %q", tt.Texts())
	}
	slider, ok := tt.Find(locales.VolumeFor("HomePods"))
	if !ok {
		t.Fatalf("the volume slider is missing; texts: %q", tt.Texts())
	}
	if lead.W != 16 {
		t.Fatalf("the stereo mark is %.1f DIP wide, want 16: the lead-in loosened", lead.W)
	}
	// The mute button rides 3 DIP wider each side for its hover pill, so its
	// box starts a hover-overhang left of the mark's ink column.
	if mute.X != lead.X-3 {
		t.Fatalf("the mute button starts at %.1f, want %.1f (one 3 DIP hover overhang)", mute.X, lead.X-3)
	}
	if diff := name.X - (lead.X + lead.W); diff > 6.01 || diff < 5.99 {
		t.Fatalf("the name starts %.2f DIP after the mark, want the 6 DIP gap", diff)
	}
	if name.X != slider.X {
		t.Fatalf("the name and the rail start at %.1f and %.1f, want one column", name.X, slider.X)
	}
}
