package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestDesignCarriesTextScaleAsZoom pins the accessibility path: the design
// keeps its own pixel sizes, and the window's content zoom carries the
// desktop's text scale, so text grows without clipping the fixed rows.
func TestDesignCarriesTextScaleAsZoom(t *testing.T) {
	prefs := ui.Preferences{TextScale: 1.25}
	var zoom, fontSize float32
	view := func(c *ui.Context) {
		design(c)
		zoom, fontSize = c.Zoom(), c.Theme().FontSize
	}
	tt := ui.NewTesterWith(view, 320, 240, ui.TesterOptions{Preferences: prefs})
	tt.Frame()
	if zoom != 1.25 {
		t.Fatalf("content zoom = %v, want 1.25", zoom)
	}
	if fontSize != 13 {
		t.Fatalf("theme font size = %v, want 13: the zoom must carry the scale alone", fontSize)
	}
}

// TestPaletteFollowsHighContrast pins the contrast preference on the design's
// own palette, which the stock widgets' theme cannot reach.
func TestPaletteFollowsHighContrast(t *testing.T) {
	prefs := ui.Preferences{HighContrast: true}
	plain := paletteFor(false)
	contrast := plain.withPreferences(prefs)
	if contrast.line == plain.line || contrast.muted == plain.muted {
		t.Fatalf("high contrast left line/muted unchanged: %v/%v", contrast.line, contrast.muted)
	}
	theme := contrast.theme(ui.LightTheme(), prefs)
	if theme.Border != contrast.line {
		t.Fatalf("theme border = %v, want the palette's %v", theme.Border, contrast.line)
	}
	if theme.Focus.A != 255 {
		t.Fatalf("focus alpha = %d, want opaque", theme.Focus.A)
	}
}

// TestScaledSizes pins the window DIPs the content zoom adds, and the clamps
// on a scale the platform reports as zero or absurd.
func TestScaledSizes(t *testing.T) {
	for _, tc := range []struct {
		scale        float32
		wantW, wantH int
	}{
		{1, flyoutWidth, 439},
		{1.25, 525, 549},
		{1.5, 630, 659},
	} {
		w, h := scaledSize(flyoutWidth, 439, tc.scale)
		if w != tc.wantW || h != tc.wantH {
			t.Errorf("scaledSize(%d, 439, %v) = %d×%d, want %d×%d", flyoutWidth, tc.scale, w, h, tc.wantW, tc.wantH)
		}
	}
	if got := normalizeScale(0); got != 1 {
		t.Errorf("normalizeScale(0) = %v, want 1", got)
	}
	if got := normalizeScale(9); got != 3 {
		t.Errorf("normalizeScale(9) = %v, want 3", got)
	}
}
