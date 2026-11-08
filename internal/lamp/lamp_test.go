package lamp

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
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

// TestPresetCarriersRecoverTheirPalette is the design's acceptance test: the
// receiver clusters the carrier in Lab, so each carrier must yield the three
// colors it was designed around.
func TestPresetCarriersRecoverTheirPalette(t *testing.T) {
	for _, p := range All() {
		data, err := JPEG(p.ID)
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		got, ok := CoverPreset(img)
		if !ok {
			t.Fatalf("%s: CoverPreset returned no palette", p.ID)
		}
		for _, want := range p.Colors {
			best := math.Inf(1)
			for _, c := range got.Colors {
				if d := labDistance(want, c); d < best {
					best = d
				}
			}
			if best > 12 {
				t.Errorf("%s: %v is %.1f away from the recovered palette %v", p.ID, want, best, got.Colors)
			}
		}
	}
}

// TestPresetColorsAreDistinct keeps each template from collapsing into the
// one-hue-family result the old dark base and blended blobs produced. The
// old dune collapse's closest pair was ΔE 7.1; the golden dune sits at 14.5
// and above.
func TestPresetColorsAreDistinct(t *testing.T) {
	for _, p := range All() {
		for i := 0; i < len(p.Colors); i++ {
			for j := i + 1; j < len(p.Colors); j++ {
				if d := labDistance(p.Colors[i], p.Colors[j]); d < 12 {
					t.Errorf("%s: %v and %v are only %.1f apart", p.ID, p.Colors[i], p.Colors[j], d)
				}
			}
		}
	}
}

func TestPresetWeightsSumToOne(t *testing.T) {
	for _, p := range All() {
		sum := p.Weights[0] + p.Weights[1] + p.Weights[2]
		if math.Abs(sum-1) > 1e-6 {
			t.Errorf("%s weights sum to %v", p.ID, sum)
		}
	}
}

func labDistance(a, b color.RGBA) float64 {
	l1, a1, b1 := rgbToLab(a.R, a.G, a.B)
	l2, a2, b2 := rgbToLab(b.R, b.G, b.B)
	return math.Sqrt((l1-l2)*(l1-l2) + (a1-a2)*(a1-a2) + (b1-b2)*(b1-b2))
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
	if p.ID != "cover" {
		t.Fatalf("preset = %+v, want cover", p)
	}
	var red, green, blue bool
	for _, c := range p.Colors {
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
		t.Fatalf("palette = %v, want red, green and blue", p.Colors)
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
