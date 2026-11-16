package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// TestSettingsPageSwitchStartsAtTop pins the page-keyed scroll container: the
// audio page's offset used to follow the user to another page and back.
func TestSettingsPageSwitchStartsAtTop(t *testing.T) {
	withLocale(t, "zh-CN")
	a := previewApp()
	a.page = pageAudio
	tt := ui.NewTester(a.settingsView, 720, 420)
	tt.Frame()
	tt.Advance(300 * time.Millisecond) // settle the window's entrance
	top, ok := tt.Find(locales.SettingsLatencyLow())
	if !ok {
		t.Fatalf("the low-latency tile is missing: %q", tt.Texts())
	}
	tt.Scroll(500, 300, 0, 220)
	tt.Frame()
	scrolled, ok := tt.Find(locales.SettingsLatencyLow())
	if !ok || scrolled.Y >= top.Y {
		t.Fatalf("the audio page did not scroll: %v -> %v", top, scrolled)
	}
	a.page = pageAbout
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	a.page = pageAudio
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	back, ok := tt.Find(locales.SettingsLatencyLow())
	if !ok {
		t.Fatalf("the low-latency tile is missing after switching back: %q", tt.Texts())
	}
	// The first frame still carries the window's one-shot entrance rise, so
	// the settled position sits a few DIPs above it. What matters is that the
	// tile is back in view instead of at the old scroll offset.
	if back.Y < 0 || back.Y > top.Y+12 {
		t.Fatalf("the audio page did not start at the top: %v, want about %v", back, top)
	}
}
