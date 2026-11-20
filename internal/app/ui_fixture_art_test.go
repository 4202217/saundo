package app

import (
	"bytes"
	"hash/fnv"
	"image"
	"slices"
	"testing"
)

func TestCoverArtDeterministic(t *testing.T) {
	for _, title := range coverArtTitles() {
		a := artTestRGBA(t, coverArt(title))
		b := artTestRGBA(t, coverArt(title))
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Errorf("coverArt(%q) differs between runs", title)
		}
	}
}

func TestCoverArtDistinct(t *testing.T) {
	seen := map[uint64]string{}
	for _, title := range coverArtTitles() {
		h := artTestHash(artTestRGBA(t, coverArt(title)))
		if prev, ok := seen[h]; ok {
			t.Errorf("coverArt(%q) has the same pixels as coverArt(%q)", title, prev)
		}
		seen[h] = title
	}
}

// TestCoverArtNotGradient is the regression test for the report that the demo
// covers all looked like flat gradients. A gradient has one color per row, so
// it cannot reach 500 distinct values, and its few-color field cannot carry
// this much channel spread.
func TestCoverArtNotGradient(t *testing.T) {
	const (
		minDistinct = 500
		minVariance = 500.0
	)
	for _, title := range coverArtTitles() {
		distinct, variance := artTestStats(artTestRGBA(t, coverArt(title)))
		t.Logf("%s: %d distinct RGB values, channel variance %.0f", title, distinct, variance)
		if distinct < minDistinct {
			t.Errorf("coverArt(%q) has %d distinct RGB values, want at least %d", title, distinct, minDistinct)
		}
		if variance < minVariance {
			t.Errorf("coverArt(%q) has channel variance %.0f, want at least %.0f", title, variance, minVariance)
		}
	}
}

// TestCoverArtFollowsTheLocalizedTitle keeps the English fixture's track on
// the same artwork: the cover belongs to the track, not the language.
func TestCoverArtFollowsTheLocalizedTitle(t *testing.T) {
	a := artTestRGBA(t, coverArt("夜航"))
	b := artTestRGBA(t, coverArt("Night Ferry"))
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("the English title draws different art than its Chinese title")
	}
}

func TestCoverArtCatalog(t *testing.T) {
	want := []string{"夜航", "落日公路", "玻璃海", "绯红", "雪松与雾", "Open Water Sessions"}
	if got := coverArtTitles(); !slices.Equal(got, want) {
		t.Fatalf("coverArtTitles() = %q, want %q", got, want)
	}
}

func TestCoverArtSize(t *testing.T) {
	for _, title := range coverArtTitles() {
		if b := coverArt(title).Bounds(); b != image.Rect(0, 0, coverSize, coverSize) {
			t.Errorf("coverArt(%q) bounds = %v, want 160x160", title, b)
		}
	}
}

func TestCoverArtUnknownNeutral(t *testing.T) {
	a := artTestRGBA(t, coverArt("no such album"))
	b := artTestRGBA(t, coverArt("另一张不存在的专辑"))
	if b := a.Bounds(); b != image.Rect(0, 0, coverSize, coverSize) {
		t.Errorf("unknown cover bounds = %v, want 160x160", b)
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Error("unknown titles share one neutral composition, got different pixels")
	}
}

func TestAppMarkDeterministic(t *testing.T) {
	for _, name := range appMarkNames() {
		a := artTestRGBA(t, appMark(name))
		b := artTestRGBA(t, appMark(name))
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Errorf("appMark(%q) differs between runs", name)
		}
	}
}

func TestAppMarkDistinct(t *testing.T) {
	seen := map[uint64]string{}
	for _, name := range appMarkNames() {
		h := artTestHash(artTestRGBA(t, appMark(name)))
		if prev, ok := seen[h]; ok {
			t.Errorf("appMark(%q) has the same pixels as appMark(%q)", name, prev)
		}
		seen[h] = name
	}
}

func TestAppMarkCatalog(t *testing.T) {
	want := []string{"Longwave", "Marlin", "Cinder"}
	if got := appMarkNames(); !slices.Equal(got, want) {
		t.Fatalf("appMarkNames() = %q, want %q", got, want)
	}
}

func TestAppMarkRounded(t *testing.T) {
	corners := []image.Point{{0, 0}, {markSize - 1, 0}, {0, markSize - 1}, {markSize - 1, markSize - 1}}
	for _, name := range appMarkNames() {
		img := artTestRGBA(t, appMark(name))
		for _, p := range corners {
			if a := img.RGBAAt(p.X, p.Y).A; a != 0 {
				t.Errorf("appMark(%q) corner %v alpha = %d, want 0", name, p, a)
			}
		}
		if a := img.RGBAAt(markSize/2, markSize/2).A; a != 255 {
			t.Errorf("appMark(%q) centre alpha = %d, want 255", name, a)
		}
	}
}

func TestAppMarkSize(t *testing.T) {
	for _, name := range appMarkNames() {
		if b := appMark(name).Bounds(); b != image.Rect(0, 0, markSize, markSize) {
			t.Errorf("appMark(%q) bounds = %v, want 64x64", name, b)
		}
	}
}

func TestAppMarkUnknownNeutral(t *testing.T) {
	a := artTestRGBA(t, appMark("no such player"))
	b := artTestRGBA(t, appMark("另一个不存在的播放器"))
	if b := a.Bounds(); b != image.Rect(0, 0, markSize, markSize) {
		t.Errorf("unknown mark bounds = %v, want 64x64", b)
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Error("unknown names share one neutral mark, got different pixels")
	}
}

func artTestRGBA(t *testing.T, img image.Image) *image.RGBA {
	t.Helper()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("art returns %T, want *image.RGBA", img)
	}
	return rgba
}

func artTestHash(img *image.RGBA) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(img.Pix)
	return h.Sum64()
}

func artTestStats(img *image.RGBA) (distinct int, variance float64) {
	seen := make(map[uint32]struct{}, len(img.Pix)/4)
	var sum, square [3]float64
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
		seen[uint32(r)<<16|uint32(g)<<8|uint32(b)] = struct{}{}
		for c, v := range [3]uint8{r, g, b} {
			sum[c] += float64(v)
			square[c] += float64(v) * float64(v)
		}
		n++
	}
	for c := 0; c < 3; c++ {
		mean := sum[c] / float64(n)
		variance += square[c]/float64(n) - mean*mean
	}
	return len(seen), variance / 3
}
