// Package lamp renders the ambient-light palette images a HomePod shows on
// its top display: six named palettes scattered into a seeded composition.
// Cover mode is not rendered here; it reuses the media cover.
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

// Preset is one ambient-light palette. The ID is the persisted value and
// never changes with the interface language. The seed makes the scatter
// stable for the preset without turning the six compositions into a template.
type Preset struct {
	ID      string
	Base    color.RGBA
	Palette []color.RGBA
	Seed    uint32
}

const blobCount = 6

var presets = []Preset{
	{
		ID:   "ember",
		Base: rgb(0x14, 0x08, 0x06),
		Palette: []color.RGBA{
			rgb(0xff, 0x6a, 0x00), rgb(0xff, 0xc2, 0x47),
			rgb(0xb2, 0x3a, 0x5e), rgb(0xff, 0x3d, 0x1f),
		},
		Seed: 11,
	},
	{
		ID:   "dune",
		Base: rgb(0x1e, 0x17, 0x10),
		Palette: []color.RGBA{
			rgb(0xe7, 0xb3, 0x6a), rgb(0x6f, 0x86, 0xb8),
			rgb(0xff, 0x9a, 0x62), rgb(0xf2, 0xdf, 0xb8),
		},
		Seed: 23,
	},
	{
		ID:   "moss",
		Base: rgb(0x0a, 0x17, 0x10),
		Palette: []color.RGBA{
			rgb(0x78, 0xb8, 0x6a), rgb(0xd8, 0xd9, 0x6a),
			rgb(0x2f, 0x7f, 0x86), rgb(0xb8, 0xe6, 0xc8),
		},
		Seed: 37,
	},
	{
		ID:   "tide",
		Base: rgb(0x05, 0x17, 0x24),
		Palette: []color.RGBA{
			rgb(0x1f, 0x7f, 0xd4), rgb(0x62, 0xe0, 0xd8),
			rgb(0xff, 0x7e, 0x8a), rgb(0x8f, 0xd8, 0xff),
		},
		Seed: 41,
	},
	{
		ID:   "orchid",
		Base: rgb(0x14, 0x0a, 0x20),
		Palette: []color.RGBA{
			rgb(0xb4, 0x4d, 0xe0), rgb(0xf0, 0xb8, 0xff),
			rgb(0x4a, 0x6b, 0xff), rgb(0x9b, 0x5c, 0xd6),
		},
		Seed: 53,
	},
	{
		ID:   "moon",
		Base: rgb(0x0e, 0x16, 0x20),
		Palette: []color.RGBA{
			rgb(0xe8, 0xee, 0xf8), rgb(0x8f, 0xa6, 0xc8),
			rgb(0xf0, 0xd9, 0xa8), rgb(0xff, 0xff, 0xff),
		},
		Seed: 67,
	},
}

// Blobs returns the preset's six seeded highlights. The same preset always
// produces the same composition; different presets scatter differently.
func (p Preset) Blobs() []Blob {
	if len(p.Palette) == 0 {
		return nil
	}
	rand := mulberry32(p.Seed)
	out := make([]Blob, blobCount)
	for i := range out {
		out[i] = Blob{
			Color:  p.Palette[i%len(p.Palette)],
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
