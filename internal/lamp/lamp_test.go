package lamp

import (
	"bytes"
	"image/jpeg"
	"slices"
	"testing"
)

func TestPresetsMatchCatalog(t *testing.T) {
	want := []string{"dim", "dawn", "lagoon", "neon", "citrus", "ember", "coral",
		"glacier", "mango", "mint", "cherry", "abyss", "volcano", "melon",
		"sand", "storm", "pop", "forest", "aurora"}
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
	if got := Normalize("aurora"); got != "aurora" {
		t.Errorf("Normalize(aurora) = %q, want aurora", got)
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

// TestPresetPixelsFollowTheBlobs samples ember's first blob center, where the
// warm highlight lands, and a corner no blob reaches, which keeps the base.
func TestPresetPixelsFollowTheBlobs(t *testing.T) {
	data, err := JPEG("ember")
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(96, 528).RGBA()
	cr, cg, cb := r>>8, g>>8, b>>8
	if cr < 190 || cg < 90 || int(cr) < int(cb)+100 {
		t.Fatalf("blob center = #%02x%02x%02x, want a warm highlight", cr, cg, cb)
	}
	r, g, b, _ = img.At(595, 595).RGBA()
	br, bg, bb := r>>8, g>>8, b>>8
	// JPEG is lossy, so the base is matched within a few levels.
	if abs(int(br)-0xe0) > 3 || abs(int(bg)-0x10) > 3 || abs(int(bb)-0x2f) > 3 {
		t.Fatalf("corner = #%02x%02x%02x, want the ember base #e0102f", br, bg, bb)
	}
}

func TestGeneratorCachesPerPreset(t *testing.T) {
	g := NewGenerator()
	first, err := g.JPEG("lagoon")
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.JPEG("lagoon")
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
