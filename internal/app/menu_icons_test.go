package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"saundo/internal/lamp"
)

func TestMenuIconsLoad(t *testing.T) {
	icons := loadMenuIcons()
	for _, it := range menuIconSources {
		if icons[it.name] == nil {
			t.Fatalf("menu icon %q missing; got %v", it.name, icons)
		}
	}
	for _, p := range lamp.All() {
		if icons["lamp:"+p.ID] == nil {
			t.Fatalf("lamp preview %q missing", p.ID)
		}
	}
	if icons["lamp:cover"] == nil {
		t.Fatal("cover preview missing")
	}
}

// TestMenuPreviewPNG pins the round preview: transparent outside, opaque at
// the center, and distinct per template.
func TestMenuPreviewPNG(t *testing.T) {
	presets := lamp.All()
	first := previewMenuPNG(presets[0].Colors, presets[0].Weights)
	second := previewMenuPNG(presets[1].Colors, presets[1].Weights)
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("preview png is empty")
	}
	if bytes.Equal(first, second) {
		t.Fatal("two templates produced the same preview")
	}
	img, err := png.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("preview is %v, want 32x32", img.Bounds())
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Fatalf("corner alpha is %d, want 0", a)
	}
	if _, _, _, a := img.At(16, 16).RGBA(); a == 0 {
		t.Fatal("center is transparent")
	}
}

func TestCoverPreviewColors(t *testing.T) {
	colors, weights := coverPreviewColors(nil, nil, false)
	if colors != neutralPreviewColors || weights != coverPreviewWeights {
		t.Fatalf("empty cover colors = %v/%v, want the neutral palette", colors, weights)
	}
	field := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			field.SetRGBA(x, y, color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
		}
	}
	colors, _ = coverPreviewColors(field, nil, false)
	if colors[0] != (color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}) {
		t.Fatalf("cover preview color = %v, want the field color", colors[0])
	}
}

// TestMenuIconPNGsRemoved keeps the generated rasters from creeping back:
// the menu now draws the same SVGs the interface does, and the lamp previews
// are generated in memory.
func TestMenuIconPNGsRemoved(t *testing.T) {
	for _, it := range menuIconSources {
		path := filepath.Join("assets", "menu-"+it.name+".png")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists (err=%v)", path, err)
		}
	}
}
