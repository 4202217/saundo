// Package lamp renders the ambient-light images a HomePod shows on its top
// display: nineteen named presets, each a base color under three soft radial
// blobs and a white wave mark.
package lamp

import "image/color"

// DefaultID is the preset the settings fall back to, and the one an unknown
// or empty ID normalizes to. Sand matches the old dune default.
const DefaultID = "sand"

// Blob is one radial highlight. X and Y are percentages of the image's width
// and height from its top-left corner; Radius is a percentage of the width.
type Blob struct {
	Color  color.RGBA
	X, Y   float64
	Radius float64
}

// Preset is one ambient-light color. The ID is the persisted value and never
// changes with the interface language.
type Preset struct {
	ID    string
	Base  color.RGBA
	Blobs [3]Blob
}

// presets mirrors Unisson's nineteen-light catalog. X, Y, and Radius are
// stored in percent; the catalog's normalized values are multiplied by 100.
var presets = []Preset{
	{
		ID:   "dim",
		Base: rgb(0xd2, 0xce, 0xf4),
		Blobs: [3]Blob{
			{rgb(0xb4, 0xae, 0xf0), 50, 50, 62},
			{rgb(0x5a, 0x5c, 0xf6), 50, 50, 42},
			{rgb(0x31, 0x34, 0xee), 50, 50, 26},
		},
	},
	{
		ID:   "dawn",
		Base: rgb(0xff, 0x4d, 0x6d),
		Blobs: [3]Blob{
			{rgb(0xff, 0xb0, 0x00), 18, 12, 72},
			{rgb(0xff, 0x2e, 0x93), 92, 86, 70},
			{rgb(0xff, 0xe1, 0x4d), 10, 95, 48},
		},
	},
	{
		ID:   "lagoon",
		Base: rgb(0x00, 0xb4, 0xd8),
		Blobs: [3]Blob{
			{rgb(0x00, 0xf5, 0xa0), 8, 88, 72},
			{rgb(0x1e, 0x5b, 0xff), 92, 10, 72},
			{rgb(0x7d, 0xf9, 0xff), 55, 50, 36},
		},
	},
	{
		ID:   "neon",
		Base: rgb(0xff, 0x1f, 0x8e),
		Blobs: [3]Blob{
			{rgb(0xff, 0x7a, 0x00), 8, 10, 66},
			{rgb(0x2b, 0x5b, 0xff), 96, 96, 72},
			{rgb(0xff, 0x5c, 0xf0), 82, 18, 42},
		},
	},
	{
		ID:   "citrus",
		Base: rgb(0xb8, 0xf0, 0x1f),
		Blobs: [3]Blob{
			{rgb(0xff, 0xe5, 0x00), 14, 16, 72},
			{rgb(0x00, 0xc4, 0x6a), 88, 90, 72},
			{rgb(0x00, 0xe5, 0xc3), 92, 12, 40},
		},
	},
	{
		ID:   "ember",
		Base: rgb(0xe0, 0x10, 0x2f),
		Blobs: [3]Blob{
			{rgb(0xff, 0x7a, 0x00), 16, 88, 76},
			{rgb(0xff, 0x2e, 0x63), 88, 16, 62},
			{rgb(0xff, 0xc3, 0x00), 40, 62, 34},
		},
	},
	{
		ID:   "coral",
		Base: rgb(0xff, 0x6b, 0x5b),
		Blobs: [3]Blob{
			{rgb(0x00, 0xd4, 0xc0), 88, 90, 76},
			{rgb(0xff, 0xc2, 0xa8), 12, 12, 62},
			{rgb(0xff, 0x3b, 0x7f), 94, 8, 44},
		},
	},
	{
		ID:   "glacier",
		Base: rgb(0x2f, 0x6b, 0xff),
		Blobs: [3]Blob{
			{rgb(0xa8, 0xf0, 0xff), 14, 14, 70},
			{rgb(0x00, 0xd9, 0xff), 90, 88, 70},
			{rgb(0xe6, 0xfb, 0xff), 85, 15, 38},
		},
	},
	{
		ID:   "mango",
		Base: rgb(0xff, 0x9f, 0x1c),
		Blobs: [3]Blob{
			{rgb(0xff, 0x4e, 0x00), 12, 86, 74},
			{rgb(0xff, 0xe0, 0x66), 88, 12, 68},
			{rgb(0xff, 0x2d, 0x6f), 92, 92, 40},
		},
	},
	{
		ID:   "mint",
		Base: rgb(0x00, 0xd6, 0x8f),
		Blobs: [3]Blob{
			{rgb(0xc6, 0xff, 0x4d), 12, 12, 70},
			{rgb(0x00, 0xa3, 0xff), 90, 90, 70},
			{rgb(0x7d, 0xff, 0xd6), 50, 55, 36},
		},
	},
	{
		ID:   "cherry",
		Base: rgb(0xd9, 0x04, 0x52),
		Blobs: [3]Blob{
			{rgb(0xff, 0x5c, 0x8a), 16, 14, 72},
			{rgb(0xff, 0x9e, 0x00), 88, 90, 64},
			{rgb(0x8a, 0x00, 0x30), 90, 12, 42},
		},
	},
	{
		ID:   "abyss",
		Base: rgb(0x05, 0x30, 0x6b),
		Blobs: [3]Blob{
			{rgb(0x00, 0xc2, 0xff), 14, 88, 74},
			{rgb(0x1e, 0xff, 0xc4), 90, 14, 62},
			{rgb(0x00, 0x52, 0xff), 60, 50, 40},
		},
	},
	{
		ID:   "volcano",
		Base: rgb(0x2b, 0x0a, 0x00),
		Blobs: [3]Blob{
			{rgb(0xff, 0x3d, 0x00), 14, 86, 76},
			{rgb(0xff, 0xb3, 0x00), 86, 16, 60},
			{rgb(0xff, 0x00, 0x6a), 90, 90, 42},
		},
	},
	{
		ID:   "melon",
		Base: rgb(0xff, 0x3f, 0x5e),
		Blobs: [3]Blob{
			{rgb(0x22, 0xd3, 0x6b), 10, 90, 66},
			{rgb(0xff, 0xd1, 0xdc), 88, 12, 62},
			{rgb(0xff, 0x7a, 0x8a), 60, 60, 36},
		},
	},
	{
		ID:   "sand",
		Base: rgb(0xf2, 0xb8, 0x80),
		Blobs: [3]Blob{
			{rgb(0xff, 0x8a, 0x5c), 86, 86, 70},
			{rgb(0xff, 0xe8, 0xb0), 14, 14, 70},
			{rgb(0x2e, 0xc4, 0xb6), 10, 92, 42},
		},
	},
	{
		ID:   "storm",
		Base: rgb(0x1b, 0x2a, 0x4a),
		Blobs: [3]Blob{
			{rgb(0xff, 0xd4, 0x00), 86, 14, 56},
			{rgb(0x4d, 0x7c, 0xff), 12, 88, 76},
			{rgb(0x9a, 0xd0, 0xff), 50, 50, 34},
		},
	},
	{
		ID:   "pop",
		Base: rgb(0x1f, 0xd1, 0xff),
		Blobs: [3]Blob{
			{rgb(0xff, 0x2b, 0xd6), 14, 14, 68},
			{rgb(0xff, 0xf2, 0x00), 88, 88, 70},
			{rgb(0xff, 0x5a, 0x1f), 90, 12, 40},
		},
	},
	{
		ID:   "forest",
		Base: rgb(0x0b, 0x6e, 0x4f),
		Blobs: [3]Blob{
			{rgb(0xa7, 0xf4, 0x32), 86, 14, 66},
			{rgb(0x00, 0xb3, 0xa4), 12, 88, 72},
			{rgb(0xff, 0xd2, 0x3f), 50, 55, 34},
		},
	},
	{
		ID:   "aurora",
		Base: rgb(0x06, 0x2a, 0x3a),
		Blobs: [3]Blob{
			{rgb(0x2b, 0xff, 0x88), 12, 20, 72},
			{rgb(0x00, 0xe5, 0xff), 90, 86, 68},
			{rgb(0xff, 0x4f, 0xd8), 88, 12, 40},
		},
	},
}

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 0xff} }

// All returns the presets in display order.
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

// Normalize maps an unknown or empty preset to the default.
func Normalize(id string) string {
	if _, ok := ByID(id); ok {
		return id
	}
	return DefaultID
}
