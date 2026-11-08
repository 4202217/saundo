package lamp

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// coverSeed keeps the derived composition stable for every cover.
const coverSeed = 71

// coverSample is one downsampled cover pixel in RGB and Lab.
type coverSample struct {
	r, g, b  uint8
	l, a, bb float64
}

// CoverPreset derives the three-color palette HomePod uses for its Sound
// Glow. The receiver clusters the cover in Lab and renders a dark base with
// that palette (MediaColors); this is the local preview of that result, not
// the device's live light. The bool is false for a nil or empty image.
func CoverPreset(src image.Image) (Preset, bool) {
	if src == nil {
		return Preset{}, false
	}
	b := src.Bounds()
	if b.Empty() {
		return Preset{}, false
	}
	side := min(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	step := max(1, side/24)
	samples := make([]coverSample, 0, 24*24)
	for y := y0; y < y0+side; y += step {
		for x := x0; x < x0+side; x += step {
			r, g, bl, a := src.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			cr, cg, cb := uint8(r>>8), uint8(g>>8), uint8(bl>>8)
			l, aa, bb := rgbToLab(cr, cg, cb)
			samples = append(samples, coverSample{r: cr, g: cg, b: cb, l: l, a: aa, bb: bb})
		}
	}
	if len(samples) == 0 {
		return Preset{}, false
	}
	sort.SliceStable(samples, func(i, j int) bool { return samples[i].l < samples[j].l })
	centers := [3]coverSample{
		samples[0],
		samples[len(samples)/2],
		samples[len(samples)-1],
	}
	var counts [3]int
	for iter := 0; iter < 8; iter++ {
		var sums [3]coverSum
		counts = [3]int{}
		for _, s := range samples {
			k := nearestCover(s, centers)
			counts[k]++
			sums[k].add(s)
		}
		for k := range centers {
			if counts[k] > 0 {
				centers[k] = sums[k].mean()
			}
		}
	}

	type cluster struct {
		c      color.RGBA
		orig   color.RGBA
		l      float64
		weight float64
	}
	var clusters []cluster
	for k := range centers {
		if counts[k] == 0 {
			continue
		}
		orig := color.RGBA{R: centers[k].r, G: centers[k].g, B: centers[k].b, A: 0xff}
		clusters = append(clusters, cluster{
			c:      normalizeGlow(orig),
			orig:   orig,
			l:      centers[k].l,
			weight: float64(counts[k]) / float64(len(samples)),
		})
	}
	if len(clusters) == 0 {
		return Preset{}, false
	}
	sort.SliceStable(clusters, func(i, j int) bool { return clusters[i].l < clusters[j].l })
	var colors [3]color.RGBA
	var weights [3]float64
	for i, c := range clusters {
		if i == len(colors) {
			break
		}
		colors[i] = c.c
		weights[i] = c.weight
	}
	// A cover with one dominant color can leave fewer than three clusters.
	// Fill from the default template so the preview still has a composition.
	if len(clusters) < len(colors) {
		if def, ok := ByID(DefaultID); ok {
			for i := len(clusters); i < len(colors); i++ {
				colors[i] = def.Colors[i]
			}
		}
	}
	base := scaleColor(clusters[0].orig, 0.22)
	return Preset{ID: "cover", Colors: colors, Weights: weights, Base: base, Seed: coverSeed}, true
}

// coverSum accumulates a cluster in RGB and Lab.
type coverSum struct {
	r, g, b, l, a, bb float64
	n                 int
}

func (s *coverSum) add(v coverSample) {
	s.r += float64(v.r)
	s.g += float64(v.g)
	s.b += float64(v.b)
	s.l += v.l
	s.a += v.a
	s.bb += v.bb
	s.n++
}

func (s coverSum) mean() coverSample {
	n := float64(s.n)
	return coverSample{
		r:  uint8(s.r/n + 0.5),
		g:  uint8(s.g/n + 0.5),
		b:  uint8(s.b/n + 0.5),
		l:  s.l / n,
		a:  s.a / n,
		bb: s.bb / n,
	}
}

func nearestCover(s coverSample, centers [3]coverSample) int {
	best, bestD := 0, math.Inf(1)
	for k, c := range centers {
		dl, da, db := s.l-c.l, s.a-c.a, s.bb-c.bb
		d := dl*dl + da*da + db*db
		if d < bestD {
			best, bestD = k, d
		}
	}
	return best
}

// normalizeGlow lifts only very dark clusters so they remain visible. The
// receiver also normalizes its background color stops; applying that 1/min
// scale to the primary colors turns every pastel cover white, so the preview
// keeps the cluster's own hue.
func normalizeGlow(c color.RGBA) color.RGBA {
	m := max(c.R, max(c.G, c.B))
	if m == 0 || m >= 96 {
		return c
	}
	scale := 96 / float64(m)
	return color.RGBA{
		R: clamp8(float64(c.R) * scale),
		G: clamp8(float64(c.G) * scale),
		B: clamp8(float64(c.B) * scale),
		A: 0xff,
	}
}

func scaleColor(c color.RGBA, scale float64) color.RGBA {
	return color.RGBA{
		R: clamp8(float64(c.R) * scale),
		G: clamp8(float64(c.G) * scale),
		B: clamp8(float64(c.B) * scale),
		A: 0xff,
	}
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// rgbToLab converts an sRGB triplet to CIE Lab under D65.
func rgbToLab(r8, g8, b8 uint8) (l, a, b float64) {
	r := srgbToLinear(float64(r8) / 255)
	g := srgbToLinear(float64(g8) / 255)
	bl := srgbToLinear(float64(b8) / 255)
	x := (0.4124564*r + 0.3575761*g + 0.1804375*bl) / 0.95047
	y := 0.2126729*r + 0.7151522*g + 0.0721750*bl
	z := (0.0193339*r + 0.1191920*g + 0.9503041*bl) / 1.08883
	fx, fy, fz := labF(x), labF(y), labF(z)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}

func labF(t float64) float64 {
	if t > 0.008856 {
		return math.Cbrt(t)
	}
	return 7.787*t + 16.0/116
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
