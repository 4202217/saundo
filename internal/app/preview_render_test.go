//go:build preview && !js

package app

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/render"
	"github.com/egoist/mygo/ui"
)

// minPreviewColors rejects a blank frame. A settled view draws many colors;
// the enter pose's empty panel draws one.
const minPreviewColors = 4

// TestPreviewRender renders the catalog through the platform renderer: one
// PNG per scenario variant, each from a fresh fixture. Run with
// SAUNDO_PREVIEW_RENDER set; it is not part of the regular suite.
func TestPreviewRender(t *testing.T) {
	dir := os.Getenv("SAUNDO_PREVIEW_RENDER")
	if dir == "" {
		t.Skip("set SAUNDO_PREVIEW_RENDER to write preview PNGs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := clearPNGs(dir); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, sc := range PreviewScenarios() {
		// The CPU matrix owns the interaction states: motion probes, the
		// hover move, and the scrolled pages.
		if sc.Frames > 0 || sc.Move != nil || sc.Scroll != 0 {
			continue
		}
		for _, v := range sc.variants() {
			a := Fixture(testLocale)
			if sc.usesSettingsPage() {
				a.page = sc.Page
			}
			sc.Apply(a)
			// The still is the first frame, so every enter pose the views
			// read must already be settled, or the PNG is the invisible
			// start of the animation.
			a.flyoutEntered = true
			a.settingsEntered = true
			a.pinEntered = true
			a.passwordEntered = true
			a.credentialEntered = true
			a.logEntered = true
			scene := sc.view(a)
			if scene == nil {
				t.Fatalf("%s: surface %d has no view", sc.ID, sc.Surface)
			}
			w, h := sc.Size(a)
			// The large-text scenarios grow the window the way the desktop's
			// text-size setting does. The composition must use the same window
			// size and text scale, or the PNG would be the normal-size state
			// under a large-text name.
			ww, hh := w, h
			var prefs ui.Preferences
			if sc.Scale > 1 {
				ww, hh = scaledSize(w, h, sc.Scale)
				prefs = ui.Preferences{TextScale: sc.Scale}
			}
			img, err := render.Still(render.Composition{
				Width:       ww * 2,
				Height:      hh * 2,
				FPS:         30,
				Frames:      1,
				Scale:       2,
				Dark:        v.dark,
				Preferences: prefs,
				Locale:      testLocale,
				View: func(c *ui.Context, f render.Frame) {
					scene(c)
				},
			}, 0, render.Options{})
			if err != nil {
				t.Fatalf("%s (%s): %v", sc.ID, v.name, err)
			}
			if n := distinctColors(img); n < minPreviewColors {
				t.Fatalf("%s (%s): %d distinct colors, want at least %d; the render is blank", sc.ID, v.name, n, minPreviewColors)
			}
			writePNG(t, filepath.Join(dir, v.name+".png"), img)
			count++
		}
	}
	t.Logf("wrote %d platform PNGs", count)
}

// distinctColors counts the RGB values in a rendered frame, sampling every
// fourth pixel so the check stays cheap.
func distinctColors(img *image.RGBA) int {
	seen := make(map[uint32]struct{}, 64)
	for i := 0; i+3 < len(img.Pix); i += 16 {
		seen[uint32(img.Pix[i])<<24|uint32(img.Pix[i+1])<<16|uint32(img.Pix[i+2])<<8|uint32(img.Pix[i+3])] = struct{}{}
	}
	return len(seen)
}

// clearPNGs removes an earlier render's images so a removed scenario cannot
// survive in the directory.
func clearPNGs(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".png") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
