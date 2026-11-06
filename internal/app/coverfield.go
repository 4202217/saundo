package app

import (
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
)

// The card's color field is small on purpose: the meter and the progress fill
// sample it at a few points, so a 23x24 grid holds every color they need.
const (
	fieldW = 23
	fieldH = 24
)

// coverField turns a cover into the card's color field. The center square is
// averaged into a small grid, blurred, blended toward its average color, and
// lightness-compressed, so the meter and the progress fill follow the cover's
// colors without inheriting its contrast.
func coverField(src image.Image, dark bool) *image.RGBA {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	if b.Empty() {
		return nil
	}
	side := min(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2

	grid := make([]float64, fieldW*fieldH*3)
	for y := 0; y < fieldH; y++ {
		sy0 := y0 + y*side/fieldH
		sy1 := y0 + (y+1)*side/fieldH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < fieldW; x++ {
			sx0 := x0 + x*side/fieldW
			sx1 := x0 + (x+1)*side/fieldW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			stepX := max(1, (sx1-sx0)/4)
			stepY := max(1, (sy1-sy0)/4)
			var r, g, bl, n float64
			for yy := sy0; yy < sy1; yy += stepY {
				for xx := sx0; xx < sx1; xx += stepX {
					cr, cg, cb, _ := src.At(xx, yy).RGBA()
					r += float64(cr) / 65535
					g += float64(cg) / 65535
					bl += float64(cb) / 65535
					n++
				}
			}
			i := (y*fieldW + x) * 3
			if n > 0 {
				grid[i], grid[i+1], grid[i+2] = r/n, g/n, bl/n
			}
		}
	}

	grid = blurGrid(grid, 1.8)

	var avgR, avgG, avgB float64
	for i := 0; i < len(grid); i += 3 {
		avgR += grid[i]
		avgG += grid[i+1]
		avgB += grid[i+2]
	}
	n := float64(len(grid) / 3)
	avgR, avgG, avgB = avgR/n, avgG/n, avgB/n

	// Light and dark sit at different places on the lightness scale so the
	// same cover reads on both card backgrounds.
	blend, center, scale := 0.55, 0.56, 0.45
	lo, hi, bright, sat := 0.40, 0.70, 1.12, 1.15
	if dark {
		blend, center, scale = 0.50, 0.58, 0.35
		lo, hi, bright, sat = 0.42, 0.72, 1.18, 1.10
	}

	out := image.NewRGBA(image.Rect(0, 0, fieldW, fieldH))
	for i := 0; i < len(grid); i += 3 {
		r := mix(grid[i], avgR, blend)
		g := mix(grid[i+1], avgG, blend)
		bl := mix(grid[i+2], avgB, blend)
		h, s, l := rgbToHSL(r, g, bl)
		l = clamp01(center + (l-center)*scale)
		l = math.Min(hi, math.Max(lo, l*bright))
		s = clamp01(s * sat)
		r, g, bl = hslToRGB(h, s, l)
		p := i / 3
		out.SetRGBA(p%fieldW, p/fieldW, color.RGBA{
			R: uint8(clamp01(r)*255 + 0.5),
			G: uint8(clamp01(g)*255 + 0.5),
			B: uint8(clamp01(bl)*255 + 0.5),
			A: 255,
		})
	}
	return out
}

// blurGrid runs a separable Gaussian over the interleaved RGB grid.
func blurGrid(src []float64, sigma float64) []float64 {
	const w, h = fieldW, fieldH
	radius := int(math.Ceil(3 * sigma))
	kernel := make([]float64, 2*radius+1)
	var sum float64
	for i := -radius; i <= radius; i++ {
		v := math.Exp(-float64(i*i) / (2 * sigma * sigma))
		kernel[i+radius] = v
		sum += v
	}
	for i := range kernel {
		kernel[i] /= sum
	}
	tmp := make([]float64, len(src))
	out := make([]float64, len(src))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b float64
			for k := -radius; k <= radius; k++ {
				xx := clampInt(x+k, 0, w-1)
				i := (y*w + xx) * 3
				kv := kernel[k+radius]
				r += src[i] * kv
				g += src[i+1] * kv
				b += src[i+2] * kv
			}
			i := (y*w + x) * 3
			tmp[i], tmp[i+1], tmp[i+2] = r, g, b
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b float64
			for k := -radius; k <= radius; k++ {
				yy := clampInt(y+k, 0, h-1)
				i := (yy*w + x) * 3
				kv := kernel[k+radius]
				r += tmp[i] * kv
				g += tmp[i+1] * kv
				b += tmp[i+2] * kv
			}
			i := (y*w + x) * 3
			out[i], out[i+1], out[i+2] = r, g, b
		}
	}
	return out
}

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	maxv := math.Max(r, math.Max(g, b))
	minv := math.Min(r, math.Min(g, b))
	l = (maxv + minv) / 2
	if maxv == minv {
		return 0, 0, l
	}
	d := maxv - minv
	if l > 0.5 {
		s = d / (2 - maxv - minv)
	} else {
		s = d / (maxv + minv)
	}
	switch maxv {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return hueToRGB(p, q, h+1.0/3), hueToRGB(p, q, h), hueToRGB(p, q, h-1.0/3)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 0.5:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	default:
		return p
	}
}

func mix(a, b, t float64) float64 { return a*(1-t) + b*t }

func clamp01(v float64) float64 { return math.Min(1, math.Max(0, v)) }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
