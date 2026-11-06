package lamp

import (
	"bytes"
	"image/jpeg"
	"slices"
	"testing"
)

func TestPresetsMatchCatalog(t *testing.T) {
	want := []string{"origin", "ember", "dune", "moss", "tide", "orchid", "moon"}
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
	if !slices.Contains(want, DefaultID) {
		t.Errorf("DefaultID %q is not a preset", DefaultID)
	}
}

func TestNormalizeUnknownAndEmpty(t *testing.T) {
	for _, id := range []string{"", "bogus", "Sand", " sand "} {
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

// TestOffIsDark pins the black image the engine sends when the light is off.
func TestOffIsDark(t *testing.T) {
	data := Off()
	if len(data) == 0 {
		t.Fatal("Off returned no image")
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(Size/2, Size/2).RGBA()
	if r>>8 > 0x10 || g>>8 > 0x10 || b>>8 > 0x10 {
		t.Fatalf("off center = #%02x%02x%02x, want black", r>>8, g>>8, b>>8)
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
	unknown, err := g.JPEG("unknown")
	if err != nil {
		t.Fatal(err)
	}
	direct, err := g.JPEG(DefaultID)
	if err != nil {
		t.Fatal(err)
	}
	if g.builds != 2 {
		t.Fatalf("builds = %d after unknown preset, want 2", g.builds)
	}
	if !bytes.Equal(unknown, direct) {
		t.Fatal("unknown preset did not resolve to the default")
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
