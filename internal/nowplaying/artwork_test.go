package nowplaying

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// encodeArtwork encodes an image of size 8x8 with the given color.
func encodeArtwork(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

// TestUsableArtworkRejectsAppleMusicIdlePlaceholder pins the captured SMTC
// thumbnail of an idle Apple Music session. The image is a white rounded
// frame and note at a quarter alpha over a transparent field; drawn as the
// card's cover it showed as a light line around the poster.
func TestUsableArtworkRejectsAppleMusicIdlePlaceholder(t *testing.T) {
	data, err := os.ReadFile("testdata/apple-music-idle.png")
	if err != nil {
		t.Fatal(err)
	}
	if usableArtwork(data) {
		t.Fatal("idle Apple Music thumbnail reported as a usable cover")
	}
}

func TestUsableArtworkAcceptsAnOpaqueCover(t *testing.T) {
	if !usableArtwork(encodeArtwork(t, color.NRGBA{R: 200, G: 40, B: 40, A: 255})) {
		t.Fatal("opaque cover reported as unusable")
	}
}

func TestUsableArtworkRejectsATransparentThumbnail(t *testing.T) {
	if usableArtwork(encodeArtwork(t, color.NRGBA{R: 255, G: 255, B: 255, A: 0})) {
		t.Fatal("fully transparent thumbnail reported as a usable cover")
	}
}

func TestUsableArtworkRejectsUndecodableData(t *testing.T) {
	if usableArtwork([]byte("\x89PNG\r\n\x1a\nnot really a png")) {
		t.Fatal("undecodable data reported as a usable cover")
	}
	if usableArtwork(nil) {
		t.Fatal("empty data reported as a usable cover")
	}
}
