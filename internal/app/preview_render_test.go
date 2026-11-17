//go:build preview && !js

package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/plugins/render"
	"github.com/egoist/mygo/ui"
)

// TestPreviewRender writes one PNG per PreviewStates entry, from the same
// fixture and views the wasm preview runs. Run with SAUNDO_PREVIEW_RENDER
// set; it is not part of the regular suite.
func TestPreviewRender(t *testing.T) {
	dir := os.Getenv("SAUNDO_PREVIEW_RENDER")
	if dir == "" {
		t.Skip("set SAUNDO_PREVIEW_RENDER to write preview PNGs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, st := range PreviewStates {
		a := PreviewApp()
		st.Apply(a)
		comp := render.Composition{
			Width:  flyoutWidth * 2,
			Height: previewHeight(a) * 2,
			FPS:    30,
			Frames: 1,
			Scale:  2,
			Dark:   true,
			Locale: previewLocale,
			View: func(c *ui.Context, f render.Frame) {
				a.view(c)
			},
		}
		img, err := render.Still(comp, 0, render.Options{})
		if err != nil {
			t.Fatalf("%s: %v", st.Name, err)
		}
		f, err := os.Create(filepath.Join(dir, st.Name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
