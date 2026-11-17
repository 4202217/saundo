//go:build preview && !js

package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/render"
	"github.com/egoist/mygo/ui"
)

// minPreviewColors rejects a blank frame. A settled view draws many colors;
// the enter pose's empty panel draws one.
const minPreviewColors = 4

// previewManifest records what the platform renderer wrote: the renderer that
// drew the frames and the PNG files with their bytes. Each file keeps its own
// renderer name because every Still starts a new render.
type previewManifest struct {
	Renderer  string              `json:"renderer"`
	Generated string              `json:"generated"`
	Files     []previewFileRecord `json:"files"`
}

type previewFileRecord struct {
	Name     string `json:"name"`
	SHA256   string `json:"sha256"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Renderer string `json:"renderer"`
}

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
	var files []previewFileRecord
	renderer := ""
	count := 0
	wanted := previewWanted()
	for _, sc := range PreviewScenarios() {
		// The CPU matrix owns the interaction states: motion probes, the
		// hover move, and the scrolled pages.
		if sc.Frames > 0 || sc.Move != nil || sc.Scroll != 0 {
			continue
		}
		if !sc.anyWanted(wanted) {
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
			drew := ""
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
			}, 0, render.Options{Progress: func(p render.Progress) {
				drew = p.Renderer
			}})
			if err != nil {
				t.Fatalf("%s (%s): %v", sc.ID, v.name, err)
			}
			if n := distinctColors(img); n < minPreviewColors {
				t.Fatalf("%s (%s): %d distinct colors, want at least %d; the render is blank", sc.ID, v.name, n, minPreviewColors)
			}
			path := filepath.Join(dir, v.name+".png")
			writePNG(t, path, img)
			rec, err := previewRecord(path, drew)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, rec)
			if drew != "" {
				renderer = drew
			}
			count++
		}
	}
	m := previewManifest{
		Renderer:  rendererLabel(renderer),
		Generated: time.Now().UTC().Format(time.RFC3339),
		Files:     files,
	}
	if err := writePreviewManifest(filepath.Join(dir, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d platform PNGs, renderer %s", count, m.Renderer)
}

// previewRecord hashes the written PNG and reads its size back, so the
// manifest describes the bytes on disk rather than the image in memory.
func previewRecord(path, renderer string) (previewFileRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return previewFileRecord{}, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return previewFileRecord{}, err
	}
	sum := sha256.Sum256(data)
	return previewFileRecord{
		Name:     filepath.Base(path),
		SHA256:   fmt.Sprintf("%x", sum),
		Width:    cfg.Width,
		Height:   cfg.Height,
		Renderer: rendererLabel(renderer),
	}, nil
}

// rendererLabel never invents a name. An empty Progress.Renderer means the
// renderer did not report what drew the frame, and the record says so.
func rendererLabel(name string) string {
	if name == "" {
		return "unknown"
	}
	return name
}

func TestRendererLabelNeverInventsAName(t *testing.T) {
	if got := rendererLabel(""); got != "unknown" {
		t.Fatalf("rendererLabel(\"\") = %q, want unknown", got)
	}
	if got := rendererLabel("Direct3D 11 (NVIDIA GeForce RTX 5070)"); got != "Direct3D 11 (NVIDIA GeForce RTX 5070)" {
		t.Fatalf("rendererLabel changed a reported name to %q", got)
	}
}

func writePreviewManifest(path string, m previewManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
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

// clearPNGs removes an earlier render's images and manifest so a removed
// scenario cannot survive in the directory.
func clearPNGs(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".png") && name != "manifest.json" {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}
