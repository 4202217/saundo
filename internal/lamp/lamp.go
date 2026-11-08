package lamp

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
)

// Size is the square image's side in pixels.
const Size = 600

// Quality is the JPEG quality the live probe verified on the top display.
const Quality = 88

// Generator renders presets lazily and keeps each encoded image for the
// process lifetime. Images are deterministic, so a preset is rendered once.
type Generator struct {
	mu     sync.Mutex
	cache  map[string][]byte
	builds int
}

// NewGenerator returns an empty cache.
func NewGenerator() *Generator {
	return &Generator{cache: make(map[string][]byte)}
}

var shared = NewGenerator()

// JPEG returns the encoded 600x600 image for a preset, normalizing an
// unknown or empty ID to the default template. The bytes are shared and must
// not be modified.
func JPEG(preset string) ([]byte, error) {
	return shared.JPEG(preset)
}

// JPEG returns the encoded image for a preset, rendering it on first use.
// The bytes are shared and must not be modified.
func (g *Generator) JPEG(preset string) ([]byte, error) {
	preset = Normalize(preset)
	g.mu.Lock()
	defer g.mu.Unlock()
	if data, ok := g.cache[preset]; ok {
		return data, nil
	}
	p, _ := ByID(preset)
	data, err := render(p)
	if err != nil {
		return nil, err
	}
	g.builds++
	g.cache[preset] = data
	return data, nil
}

// render paints the carrier the receiver clusters. The receiver never shows
// this image: it runs a Lab k-means over it and drives the LEDs from the three
// colors, so the carrier is three flat color fields with a narrow feather,
// not a picture. The dark base and overlapping gradients the old renderer
// used pulled every template toward one muddy hue, and the receiver reduced
// all four colors to three near neighbours.
func render(p Preset) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	var cuts [4]int
	for i, w := range p.Weights {
		cuts[i+1] = cuts[i] + int(w*Size+0.5)
	}
	cuts[3] = Size
	const feather = Size / 24
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			img.SetRGBA(x, y, carrierColor(p.Colors, cuts, feather, x))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: Quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// carrierColor blends across a band boundary over 2*feather pixels. The flat
// cores stay exact, so the receiver's k-means recovers the three colors.
func carrierColor(colors [3]color.RGBA, cuts [4]int, feather, x int) color.RGBA {
	for i := 0; i < 2; i++ {
		cut := cuts[i+1]
		if x < cut-feather {
			return colors[i]
		}
		if x <= cut+feather {
			t := float64(x-(cut-feather)) / float64(2*feather)
			return blendColor(colors[i], colors[i+1], t)
		}
	}
	return colors[2]
}

func blendColor(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-t) + float64(y)*t + 0.5)
	}
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 0xff}
}
