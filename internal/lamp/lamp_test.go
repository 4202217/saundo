package lamp

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"reflect"
	"testing"
)

func TestPresetsMatchCatalog(t *testing.T) {
	want := []string{"ember", "dune", "moss", "tide", "orchid", "moon"}
	all := All()
	if len(all) != len(want) {
		t.Fatalf("presets = %d, want %d", len(all), len(want))
	}
	for i, p := range all {
		if p.ID != want[i] {
			t.Errorf("preset %d = %q, want %q", i, p.ID, want[i])
		}
	}
	seen := map[string]bool{}
	for _, p := range all {
		if seen[p.ID] {
			t.Errorf("duplicate preset ID %q", p.ID)
		}
		seen[p.ID] = true
	}
	if DefaultID != "dune" {
		t.Errorf("DefaultID = %q, want dune", DefaultID)
	}
	if _, ok := ByID("media"); ok {
		t.Error("media must not be a rendered preset")
	}
}

func TestNormalizeUnknownAndEmpty(t *testing.T) {
	for _, id := range []string{"", "bogus", "Sand", " sand ", "origin", "media"} {
		if got := Normalize(id); got != DefaultID {
			t.Errorf("Normalize(%q) = %q, want %q", id, got, DefaultID)
		}
	}
	if got := Normalize("tide"); got != "tide" {
		t.Errorf("Normalize(tide) = %q, want tide", got)
	}
}

func TestGeneratedJPEGsAre600Square(t *testing.T) {
	g := NewGenerator()
	for _, p := range All() {
		data, err := g.JPEG(p.ID)
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 || data[len(data)-2] != 0xFF || data[len(data)-1] != 0xD9 {
			t.Fatalf("%s: not a JPEG, first bytes % x", p.ID, data[:min(4, len(data))])
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		if cfg.Width != Size || cfg.Height != Size {
			t.Fatalf("%s: %dx%d, want %dx%d", p.ID, cfg.Width, cfg.Height, Size, Size)
		}
	}
}

// TestPresetPixelsFollowTheBlobs samples the first seeded blob's center. The
// composition is deterministic per preset, and the center must leave the base.
func TestPresetPixelsFollowTheBlobs(t *testing.T) {
	p, ok := ByID("ember")
	if !ok {
		t.Fatal("ember is missing")
	}
	blobs := p.Blobs()
	if len(blobs) == 0 {
		t.Fatal("ember has no blobs")
	}
	data, err := JPEG("ember")
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := blobs[0]
	x, y := int(b.X/100*Size), int(b.Y/100*Size)
	r, g, bl, _ := img.At(x, y).RGBA()
	cr, cg, cb := r>>8, g>>8, bl>>8
	if abs(int(cr)-int(p.Base.R)) < 12 && abs(int(cg)-int(p.Base.G)) < 12 && abs(int(cb)-int(p.Base.B)) < 12 {
		t.Fatalf("blob center = #%02x%02x%02x, still the base", cr, cg, cb)
	}
}

// TestPresetCompositionCovers pins the whole-tile coverage: most of a 10x10
// sample grid must differ from the base, not just one centered circle.
func TestPresetCompositionCovers(t *testing.T) {
	for _, p := range All() {
		if len(p.Palette) == 0 {
			continue
		}
		data, err := JPEG(p.ID)
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		changed := 0
		for y := 0; y < Size; y += 60 {
			for x := 0; x < Size; x += 60 {
				r, g, b, _ := img.At(x, y).RGBA()
				if abs(int(r>>8)-int(p.Base.R)) > 10 ||
					abs(int(g>>8)-int(p.Base.G)) > 10 ||
					abs(int(b>>8)-int(p.Base.B)) > 10 {
					changed++
				}
			}
		}
		if changed < 50 {
			t.Fatalf("%s: %d/100 samples differ from the base, want at least 50", p.ID, changed)
		}
	}
}

func TestGeneratorCachesPerPreset(t *testing.T) {
	g := NewGenerator()
	first, err := g.JPEG("tide")
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.JPEG("tide")
	if err != nil {
		t.Fatal(err)
	}
	if g.builds != 1 {
		t.Fatalf("builds = %d, want 1", g.builds)
	}
	if &first[0] != &second[0] {
		t.Fatal("cached image was copied instead of reused")
	}
	if _, err := g.JPEG("unknown"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.JPEG("unknown"); err != nil {
		t.Fatal(err)
	}
	if g.builds != 2 {
		t.Fatalf("builds = %d after the default render, want 2", g.builds)
	}
}

func TestCoverPresetClustersTheCover(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 60; x++ {
			var c color.RGBA
			switch {
			case x < 20:
				c = color.RGBA{R: 255, A: 255}
			case x < 40:
				c = color.RGBA{G: 255, A: 255}
			default:
				c = color.RGBA{B: 255, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	p, ok := CoverPreset(img)
	if !ok {
		t.Fatal("CoverPreset returned no palette")
	}
	if p.ID != "cover" || len(p.Palette) < 3 {
		t.Fatalf("preset = %+v, want cover with three colors", p)
	}
	var red, green, blue bool
	for _, c := range p.Palette {
		switch {
		case c.R > c.G+40 && c.R > c.B+40:
			red = true
		case c.G > c.R+40 && c.G > c.B+40:
			green = true
		case c.B > c.R+40 && c.B > c.G+40:
			blue = true
		}
	}
	if !red || !green || !blue {
		t.Fatalf("palette = %v, want red, green and blue", p.Palette)
	}
	if p.Base.R > 80 || p.Base.G > 80 || p.Base.B > 80 {
		t.Fatalf("base = %v, want a dark background", p.Base)
	}
}

func TestCoverPresetRejectsEmptyImages(t *testing.T) {
	if _, ok := CoverPreset(nil); ok {
		t.Fatal("nil image produced a palette")
	}
	if _, ok := CoverPreset(image.NewRGBA(image.Rect(0, 0, 0, 0))); ok {
		t.Fatal("empty image produced a palette")
	}
}

func TestCoverPresetIsDeterministic(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(40 + x*4), G: uint8(80 + y*3), B: 140, A: 255})
		}
	}
	first, ok := CoverPreset(img)
	if !ok {
		t.Fatal("first call returned no palette")
	}
	second, ok := CoverPreset(img)
	if !ok {
		t.Fatal("second call returned no palette")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("palette changed between calls: %+v vs %+v", first, second)
	}
}

func TestPresetsRenderDistinctImages(t *testing.T) {
	g := NewGenerator()
	seen := map[string]string{}
	for _, p := range All() {
		data, err := g.JPEG(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		sum := string(data)
		if other, ok := seen[sum]; ok {
			t.Fatalf("%s renders like %s", p.ID, other)
		}
		seen[sum] = p.ID
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
