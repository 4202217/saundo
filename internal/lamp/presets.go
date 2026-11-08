// Package lamp renders the ambient-light carrier images a HomePod clusters
// for its top display: six named three-color palettes. The receiver never
// shows the image; it runs a Lab k-means over it and drives the LEDs from the
// three colors. Cover mode is not rendered here; it reuses the media cover.
package lamp

import "image/color"

const (
	// DefaultID is the template the settings fall back to, and the one an
	// unknown or empty ID normalizes to.
	DefaultID = "dune"
)

// Blob is one radial highlight. X and Y are percentages of the image's width
// and height from its top-left corner; Radius is a percentage of the width;
// Alpha scales the blend over the base.
type Blob struct {
	Color  color.RGBA
	X, Y   float64
	Radius float64
	Alpha  float64
}

// Preset is one ambient-light palette. The receiver never displays the
// carrier image: it clusters it in Lab and drives the LEDs from the three
// colors, so Colors and Weights are the design. Base and Seed only shape the
// local preview glow. The ID is the persisted value and never changes with
// the interface language.
type Preset struct {
	ID      string
	Colors  [3]color.RGBA
	Weights [3]float64
	Base    color.RGBA
	Seed    uint32
}

const blobCount = 6

var presets = []Preset{
	{
		ID: "ember",
		Colors: [3]color.RGBA{
			rgb(0xff, 0x5a, 0x1f), rgb(0xff, 0xc0, 0x20), rgb(0xb3, 0x26, 0x1e),
		},
		Weights: [3]float64{0.45, 0.30, 0.25},
		Base:    scaleColor(rgb(0xff, 0x5a, 0x1f), 0.20),
		Seed:    11,
	},
	{
		ID: "dune",
		Colors: [3]color.RGBA{
			rgb(0xc0, 0x8a, 0x00), rgb(0xc4, 0x7a, 0x1e), rgb(0x7a, 0x3f, 0x14),
		},
		Weights: [3]float64{0.45, 0.30, 0.25},
		Base:    scaleColor(rgb(0xc0, 0x8a, 0x00), 0.20),
		Seed:    23,
	},
	{
		ID: "moss",
		Colors: [3]color.RGBA{
			rgb(0x4e, 0x8a, 0x42), rgb(0xb8, 0xd4, 0x2e), rgb(0x2f, 0x7f, 0x86),
		},
		Weights: [3]float64{0.45, 0.30, 0.25},
		Base:    scaleColor(rgb(0x4e, 0x8a, 0x42), 0.20),
		Seed:    37,
	},
	{
		ID: "tide",
		Colors: [3]color.RGBA{
			rgb(0x2e, 0x7f, 0xd0), rgb(0x3f, 0xc9, 0xc0), rgb(0x4f, 0xd1, 0x8a),
		},
		Weights: [3]float64{0.45, 0.30, 0.25},
		Base:    scaleColor(rgb(0x2e, 0x7f, 0xd0), 0.20),
		Seed:    41,
	},
	{
		ID: "orchid",
		Colors: [3]color.RGBA{
			rgb(0xa8, 0x3e, 0xd0), rgb(0x4a, 0x5f, 0xe0), rgb(0xc8, 0x5a, 0x9e),
		},
		Weights: [3]float64{0.45, 0.30, 0.25},
		Base:    scaleColor(rgb(0xa8, 0x3e, 0xd0), 0.20),
		Seed:    53,
	},
	{
		ID: "moon",
		Colors: [3]color.RGBA{
			rgb(0xc4, 0xd6, 0xee), rgb(0x6e, 0x84, 0xa8), rgb(0xe0, 0xc4, 0x8a),
		},
		Weights: [3]float64{0.45, 0.35, 0.20},
		Base:    scaleColor(rgb(0xc4, 0xd6, 0xee), 0.20),
		Seed:    67,
	},
}

// Blobs returns the preset's six seeded highlights for the local preview. The
// same preset always produces the same composition; different presets scatter
// differently. The carrier image does not use these; the receiver's own
// renderer drives the light from the three colors and its audio envelope.
func (p Preset) Blobs() []Blob {
	if p.Colors == ([3]color.RGBA{}) {
		return nil
	}
	rand := mulberry32(p.Seed)
	out := make([]Blob, blobCount)
	for i := range out {
		out[i] = Blob{
			Color:  p.Colors[i%len(p.Colors)],
			X:      4 + rand()*92,
			Y:      4 + rand()*92,
			Radius: 58 + rand()*55,
			Alpha:  0.42 + rand()*0.48,
		}
	}
	return out
}

// mulberry32 matches the JS generator in the approved mockup, so the native
// preview and the mockup scatter the same blobs.
func mulberry32(seed uint32) func() float64 {
	a := seed
	return func() float64 {
		a += 0x6D2B79F5
		t := a
		t = (t ^ (t >> 15)) * (t | 1)
		t ^= t + (t^(t>>7))*(t|61)
		return float64(t^(t>>14)) / 4294967296
	}
}

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 0xff} }

// All returns the six rendered palettes in display order.
func All() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// ByID looks a preset up by its persisted ID.
func ByID(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Normalize maps an unknown or empty preset to the default template. The old
// media/origin values are handled by the settings migration, not here.
func Normalize(id string) string {
	if _, ok := ByID(id); ok {
		return id
	}
	return DefaultID
}
