package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsWindowUsesOneSurface(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := previewApp()
		a.page = pageGeneral
		tt := ui.NewTester(a.settingsView, 720, 500)
		tt.SetDark(dark)
		tt.Frame()
		img := tt.Image()
		// Title bar, sidebar, and content background, in empty areas.
		points := [][2]int{{600, 12}, {20, 400}, {700, 480}}
		r0, g0, b0, _ := img.At(points[0][0], points[0][1]).RGBA()
		for _, pt := range points[1:] {
			r, g, b, _ := img.At(pt[0], pt[1]).RGBA()
			if r != r0 || g != g0 || b != b0 {
				t.Fatalf("dark=%v: surface at %v = #%02x%02x%02x, title bar = #%02x%02x%02x",
					dark, pt, r>>8, g>>8, b>>8, r0>>8, g0>>8, b0>>8)
			}
		}
	}
}
