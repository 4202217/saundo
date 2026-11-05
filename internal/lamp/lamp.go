package lamp

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
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
// unknown or empty ID to the default. The bytes are shared and must not be
// modified.
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

// render paints the base color, then blends each blob with the linear alpha
// falloff the live probe verified on hardware: a blob is fully its color at
// the center and fades to the base at its radius.
func render(p Preset) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: p.Base}, image.Point{}, draw.Src)
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			c := p.Base
			for _, b := range p.Blobs {
				dx := float64(x)/Size - b.X/100
				dy := float64(y)/Size - b.Y/100
				d := math.Hypot(dx, dy) / (b.Radius / 100)
				if d >= 1 {
					continue
				}
				a := 1 - d
				c.R = uint8(float64(b.Color.R)*a + float64(c.R)*(1-a) + 0.5)
				c.G = uint8(float64(b.Color.G)*a + float64(c.G)*(1-a) + 0.5)
				c.B = uint8(float64(b.Color.B)*a + float64(c.B)*(1-a) + 0.5)
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: Quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
