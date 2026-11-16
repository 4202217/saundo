package app

import "testing"

// TestSampleCardIsMadeOncePerLocale pins the Now Playing settings preview:
// the view runs every frame, so the sample bitmap and color fields must be
// made once and remade only when the language changes.
func TestSampleCardIsMadeOncePerLocale(t *testing.T) {
	withLocale(t, "zh-CN")
	a := previewApp()
	_, art, light, _, _ := a.sampleCard()
	_, art2, light2, _, _ := a.sampleCard()
	if art != art2 || light != light2 {
		t.Fatal("the sample card was rebuilt across frames")
	}
	withLocale(t, "en")
	_, art3, _, _, _ := a.sampleCard()
	if art3 == art {
		t.Fatal("the sample card kept the previous language")
	}
}
