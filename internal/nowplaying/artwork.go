package nowplaying

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// usableArtwork reports whether a session thumbnail is a cover rather than a
// decorative placeholder. Some players publish a placeholder in the cover
// slot: Apple Music's idle session is a white rounded frame and a note at a
// quarter alpha over a transparent field, which reaches the card as a light
// line around the poster. A real cover has at least one pixel at half alpha
// or more, so a thumbnail without one is not shown as a cover. Data that does
// not decode is not a cover either.
func usableArtwork(data []byte) bool {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a >= 0x8000 {
				return true
			}
		}
	}
	return false
}
