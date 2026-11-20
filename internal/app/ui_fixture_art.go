package app

import (
	"image"
	"image/color"
	"math"
)

const coverSize, markSize = 160, 64

var (
	// The demo covers and marks are drawn here for both the preview and the page.
	coverArtOrder = [...]string{"夜航", "落日公路", "玻璃海", "绯红", "雪松与雾", "Open Water Sessions"}
	appMarkOrder  = [...]string{"Longwave", "Marlin", "Cinder"}
	coverDrawers  = map[string]func(*image.RGBA) uint32{
		"夜航":                  drawNightFerry,
		"落日公路":                drawSunsetHighway,
		"玻璃海":                 drawGlassSea,
		"绯红":                  drawCrimson,
		"雪松与雾":                drawCedarFog,
		"Open Water Sessions": drawOpenWater,
	}
	markDrawers = map[string]func(*image.RGBA){
		"Longwave": drawLongwave,
		"Marlin":   drawMarlin,
		"Cinder":   drawCinder,
	}
	// The English fixture localizes the first demo track's title. The art
	// belongs to the track, so both titles draw the same cover.
	coverAliases = map[string]string{
		"Night Ferry": "夜航",
	}
)

func coverArtTitles() []string { return append([]string(nil), coverArtOrder[:]...) }
func appMarkNames() []string   { return append([]string(nil), appMarkOrder[:]...) }

func coverArt(title string) image.Image {
	if canonical, ok := coverAliases[title]; ok {
		title = canonical
	}
	img := image.NewRGBA(image.Rect(0, 0, coverSize, coverSize))
	draw, ok := coverDrawers[title]
	if !ok {
		draw = drawNeutralCover
	}
	artGrain(img, draw(img))
	return img
}

func appMark(name string) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, markSize, markSize))
	draw := markDrawers[name]
	if draw == nil {
		draw = drawNeutralMark
	}
	draw(img)
	return img
}

func artHex(h uint32) color.RGBA {
	return color.RGBA{R: uint8(h >> 16), G: uint8(h >> 8), B: uint8(h), A: 255}
}
func artClamp(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v)) + 0.5) }
func artMix(a, b color.RGBA, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	mix := func(x, y uint8) uint8 { return artClamp(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}
func artLight(c color.RGBA, t float64) color.RGBA {
	return artMix(c, color.RGBA{R: 255, G: 255, B: 255, A: 255}, t)
}
func artDark(c color.RGBA, t float64) color.RGBA { return artMix(c, color.RGBA{A: 255}, t) }

func artBlend(img *image.RGBA, x, y int, c color.RGBA, a float64) {
	if a <= 0 || !image.Pt(x, y).In(img.Bounds()) {
		return
	}
	a = math.Min(1, a)
	i := img.PixOffset(x, y)
	p := img.Pix[i : i+4]
	da := float64(p[3]) / 255
	oa := a + da*(1-a)
	p[0] = artClamp((float64(c.R)*a + float64(p[0])*da*(1-a)) / oa)
	p[1] = artClamp((float64(c.G)*a + float64(p[1])*da*(1-a)) / oa)
	p[2] = artClamp((float64(c.B)*a + float64(p[2])*da*(1-a)) / oa)
	p[3] = artClamp(oa * 255)
}

func artGradient(img *image.RGBA, y0, y1 int, from, to color.RGBA) {
	span := math.Max(1, float64(y1-y0))
	for y := max(y0, 0); y < min(y1, img.Bounds().Dy()); y++ {
		c := artMix(from, to, float64(y-y0)/span)
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetRGBA(x, y, c)
		}
	}
}
func artRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, a float64) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			artBlend(img, x, y, c, a)
		}
	}
}
func artCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA, a float64) {
	for y := int(math.Floor(cy - r - 1)); y <= int(math.Ceil(cy+r+1)); y++ {
		for x := int(math.Floor(cx - r - 1)); x <= int(math.Ceil(cx+r+1)); x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if cov := math.Min(1, r-d+0.5); cov > 0 {
				artBlend(img, x, y, c, a*cov)
			}
		}
	}
}
func artGlow(img *image.RGBA, cx, cy, r float64, c color.RGBA, a float64) {
	for y := int(math.Floor(cy - r)); y <= int(math.Ceil(cy+r)); y++ {
		for x := int(math.Floor(cx - r)); x <= int(math.Ceil(cx+r)); x++ {
			if d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) / r; d < 1 {
				artBlend(img, x, y, c, a*(1-d)*(1-d))
			}
		}
	}
}

type artPt struct{ x, y float64 }

// artPoly fills a convex polygon with one scanline span per row.
func artPoly(img *image.RGBA, pts []artPt, c color.RGBA, a float64) {
	minY, maxY := pts[0].y, pts[0].y
	for _, p := range pts {
		minY, maxY = math.Min(minY, p.y), math.Max(maxY, p.y)
	}
	for y := int(math.Floor(minY)); y <= int(math.Ceil(maxY)); y++ {
		sy := float64(y) + 0.5
		x0, x1 := float64(coverSize), -1.0
		for i := range pts {
			p, q := pts[i], pts[(i+1)%len(pts)]
			if (p.y <= sy) != (q.y <= sy) {
				x := p.x + (sy-p.y)*(q.x-p.x)/(q.y-p.y)
				x0, x1 = math.Min(x0, x), math.Max(x1, x)
			}
		}
		for x := int(math.Ceil(x0 - 0.5)); x1 >= x0 && x <= int(math.Floor(x1-0.5)); x++ {
			artBlend(img, x, y, c, a)
		}
	}
}
func artEllipse(img *image.RGBA, cx, cy, rx, ry, angle float64, c color.RGBA, a float64) {
	sin, cos := math.Sincos(angle)
	r := math.Max(rx, ry) + 1
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx - r); x <= int(cx+r); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			u, v := dx*cos+dy*sin, -dx*sin+dy*cos
			if u*u/(rx*rx)+v*v/(ry*ry) <= 1 {
				artBlend(img, x, y, c, a)
			}
		}
	}
}

// artWaveBand fills from a sine crest down, lighting each crest.
func artWaveBand(img *image.RGBA, base, amp, freq, phase float64, c, crest color.RGBA) {
	for x := 0; x < img.Bounds().Dx(); x++ {
		top := int(math.Round(base + amp*math.Sin(freq*float64(x)+phase)))
		for y := max(top, 0); y < img.Bounds().Dy(); y++ {
			artBlend(img, x, y, c, 1)
		}
		if top >= 1 {
			artBlend(img, x, top-1, crest, 0.55)
		}
	}
}
func artArc(img *image.RGBA, cx, cy, r, width, from, to float64, c color.RGBA, a float64) {
	rr := r + width
	for y := int(cy - rr); y <= int(cy+rr); y++ {
		for x := int(cx - rr); x <= int(cx+rr); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Hypot(dx, dy)
			ang := math.Atan2(-dy, dx)
			if ang < 0 {
				ang += 2 * math.Pi
			}
			if d < r-width/2 || d > r+width/2 || ang < from || ang > to {
				continue
			}
			if cov := math.Min(1, width/2-math.Abs(d-r)+0.5); cov > 0 {
				artBlend(img, x, y, c, a*cov)
			}
		}
	}
}
func artConifer(img *image.RGBA, x, base, h, spread float64, c color.RGBA, a float64) {
	for i := 0; i < 3; i++ {
		top := base - h + float64(i)*h*0.2
		half := spread * (0.45 + 0.27*float64(i))
		artPoly(img, []artPt{{x, top}, {x - half, top + h*0.42}, {x + half, top + h*0.42}}, c, a)
	}
}
func artGrain(img *image.RGBA, seed uint32) {
	b := img.Bounds()
	for i := 0; i < len(img.Pix); i += 4 {
		n := artHash(seed, i/4%b.Dx(), i/4/b.Dx())
		for c := 0; c < 3; c++ {
			img.Pix[i+c] = artClamp(float64(img.Pix[i+c]) + float64(int((n>>(uint(c)*8))&15)-7)/2)
		}
	}
}
func artHash(seed uint32, x, y int) uint32 {
	h := seed*0x9e3779b9 + uint32(x)*0x85ebca6b + uint32(y)*0xc2b2ae35
	h = (h ^ h>>16) * 0x7feb352d
	h = (h ^ h>>15) * 0x846ca68b
	return h ^ h>>16
}

func drawNightFerry(img *image.RGBA) uint32 {
	top, mid, sea := artHex(0x5b5be0), artHex(0x8f6fd8), artHex(0x3d9be0)
	artGradient(img, 0, 100, top, mid)
	artGradient(img, 100, coverSize, artMix(sea, mid, 0.35), artDark(sea, 0.55))
	for i := 0; i < 26; i++ {
		h := artHash(7, i, 3)
		artBlend(img, int(h%coverSize), int((h>>8)%78), artLight(mid, 0.65), 0.35+float64((h>>16)&3)*0.12)
	}
	moon := artLight(mid, 0.85)
	artGlow(img, 116, 38, 40, artLight(mid, 0.55), 0.22)
	artCircle(img, 116, 38, 15, moon, 0.96)
	ink, lit := artDark(top, 0.78), artHex(0xffd9a0)
	artPoly(img, []artPt{{40, 108}, {122, 108}, {112, 121}, {48, 121}}, ink, 1)
	artPoly(img, []artPt{{64, 94}, {100, 94}, {100, 108}, {64, 108}}, ink, 1)
	artRect(img, 70, 99, 74, 103, lit, 0.95)
	artRect(img, 82, 99, 86, 103, lit, 0.8)
	artRect(img, 93, 99, 96, 102, lit, 0.7)
	for i := 0; i < 8; i++ {
		half := 18 - i*2
		artRect(img, 116-half, 126+i*4, 116+half, 127+i*4, artLight(sea, 0.55), 0.18)
	}
	return 101
}
func drawSunsetHighway(img *image.RGBA) uint32 {
	gold, pink, plum := artHex(0xff9a3d), artHex(0xef5d7a), artHex(0x9a4fa8)
	artGradient(img, 0, 58, plum, pink)
	artGradient(img, 58, 102, pink, gold)
	artGlow(img, 80, 98, 46, artLight(gold, 0.2), 0.35)
	artCircle(img, 80, 97, 19, artLight(gold, 0.55), 0.95)
	artRect(img, 0, 102, coverSize, coverSize, artDark(plum, 0.72), 1)
	artPoly(img, []artPt{{14, 160}, {146, 160}, {86, 103}, {74, 103}}, artDark(pink, 0.55), 1)
	line := artLight(gold, 0.6)
	for i := 0; i < 6; i++ {
		t0 := 0.1 + float64(i)*0.15
		t1 := t0 + 0.06
		y0, y1 := 158-54*t0, 158-54*t1
		w0, w1 := 3.2*(1-t0)+0.8, 3.2*(1-t1)+0.8
		artPoly(img, []artPt{{80 - w0, y0}, {80 + w0, y0}, {80 + w1, y1}, {80 - w1, y1}}, line, 0.9)
	}
	return 211
}
func drawGlassSea(img *image.RGBA) uint32 {
	aqua, blue, deep := artHex(0x35d0c5), artHex(0x3aa6d0), artHex(0x1b3d8f)
	artGradient(img, 0, 64, aqua, blue)
	artGradient(img, 64, coverSize, artMix(deep, blue, 0.55), artDark(deep, 0.35))
	artGlow(img, 88, 42, 34, artLight(aqua, 0.7), 0.45)
	artCircle(img, 88, 42, 8, artLight(aqua, 0.9), 0.95)
	bands := []struct {
		base, amp, freq, phase float64
		c                      color.RGBA
	}{
		{62, 3, 0.09, 0.4, artMix(blue, aqua, 0.45)},
		{78, 4, 0.07, 2.1, blue},
		{96, 5, 0.055, 1.2, artMix(deep, blue, 0.5)},
		{116, 4, 0.08, 3.4, deep},
		{136, 3, 0.06, 0.8, artDark(deep, 0.4)},
	}
	crest := artLight(aqua, 0.6)
	for _, b := range bands {
		artWaveBand(img, b.base, b.amp, b.freq, b.phase, b.c, crest)
	}
	for i := 0; i < 12; i++ {
		h := artHash(13, i, 0)
		x, half := 86+int(h%5), 7+int(h%5)-i/4
		artRect(img, x-half, 70+i*7, x+half, 71+i*7, artLight(aqua, 0.8), 0.14)
	}
	return 307
}
func drawCrimson(img *image.RGBA) uint32 {
	deep, rose, bright := artHex(0x7a1f3d), artHex(0xc22a55), artHex(0xe34b6f)
	artGradient(img, 0, coverSize, artMix(deep, rose, 0.35), artDark(deep, 0.35))
	artGlow(img, 80, 84, 84, rose, 0.4)
	petals := []struct {
		cx, cy, rx, ry, angle float64
		c                     color.RGBA
		a                     float64
	}{
		{64, 70, 24, 52, -0.45, bright, 0.72},
		{96, 70, 24, 52, 0.45, bright, 0.72},
		{80, 58, 20, 48, 0, rose, 0.65},
		{80, 96, 28, 44, 0, artMix(bright, rose, 0.4), 0.78},
	}
	for _, p := range petals {
		artEllipse(img, p.cx, p.cy, p.rx, p.ry, p.angle, p.c, p.a)
	}
	artGlow(img, 80, 100, 16, artLight(bright, 0.5), 0.28)
	return 401
}
func drawCedarFog(img *image.RGBA) uint32 {
	deep, light, tan := artHex(0x2f6b4f), artHex(0x7fae5a), artHex(0xb08954)
	artGradient(img, 0, coverSize, artLight(tan, 0.12), artMix(light, tan, 0.4))
	fog := func(y, h float64) {
		for yy := int(y); yy < int(y+h) && yy < coverSize; yy++ {
			k := 1 - math.Abs((float64(yy)+0.5-y-h/2)/(h/2))
			artRect(img, 0, yy, coverSize, yy+1, artLight(tan, 0.55), 0.42*k)
		}
	}
	fog(42, 34)
	artConifer(img, 84, 126, 46, 13, artMix(deep, tan, 0.6), 0.75)
	fog(72, 24)
	artConifer(img, 126, 140, 68, 19, artMix(deep, light, 0.3), 0.85)
	fog(102, 20)
	artRect(img, 0, 148, coverSize, coverSize, artDark(deep, 0.6), 1)
	artConifer(img, 52, 154, 100, 28, deep, 1)
	return 503
}
func drawOpenWater(img *image.RGBA) uint32 {
	gold, brown, dark := artHex(0xd8b25a), artHex(0x8a5a2b), artHex(0x4a2f1a)
	artGradient(img, 0, 92, artDark(gold, 0.2), artLight(gold, 0.25))
	artGradient(img, 92, coverSize, brown, artDark(dark, 0.25))
	artGlow(img, 80, 92, 60, artLight(gold, 0.45), 0.35)
	artRect(img, 0, 91, coverSize, 93, artLight(gold, 0.6), 0.7)
	for i := 0; i < 24; i++ {
		h := artHash(11, i, 5)
		x, y := 6+int(h%130), 100+int((h>>8)%52)
		w := 5 + int((h>>16)%20)
		artRect(img, x, y, x+w, y+1, artLight(gold, 0.35), 0.2+float64((h>>24)&3)*0.05)
	}
	sail := artLight(gold, 0.65)
	artPoly(img, []artPt{{104, 46}, {104, 82}, {86, 82}}, sail, 0.96)
	artPoly(img, []artPt{{107, 56}, {107, 82}, {119, 82}}, artMix(sail, brown, 0.25), 0.9)
	artRect(img, 104, 44, 106, 84, dark, 1)
	artPoly(img, []artPt{{94, 84}, {124, 84}, {119, 91}, {99, 91}}, dark, 1)
	artRect(img, 96, 95, 120, 97, artLight(gold, 0.5), 0.25)
	artRect(img, 100, 101, 116, 102, artLight(gold, 0.5), 0.2)
	return 601
}
func drawNeutralCover(img *image.RGBA) uint32 {
	top, bottom := artHex(0x55627a), artHex(0x2f3646)
	artGradient(img, 0, coverSize, top, bottom)
	artGlow(img, 80, 70, 70, artHex(0x9aa8bf), 0.18)
	artCircle(img, 80, 72, 38, artHex(0xb8c2d4), 0.85)
	artCircle(img, 94, 62, 34, bottom, 1)
	return 701
}

func artRoundedCoverage(x, y, half, radius float64) float64 {
	qx := math.Abs(x-markSize/2) - (half - radius)
	qy := math.Abs(y-markSize/2) - (half - radius)
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
	return math.Max(0, math.Min(1, 0.5-d))
}
func artMarkBase(img *image.RGBA, top, bottom color.RGBA) {
	for y := 0; y < markSize; y++ {
		for x := 0; x < markSize; x++ {
			cov := artRoundedCoverage(float64(x)+0.5, float64(y)+0.5, 26, 9)
			if cov <= 0 {
				continue
			}
			c := artLight(artMix(top, bottom, float64(y)/float64(markSize-1)), math.Max(0, 1-(float64(x)+float64(y))/90)*0.14)
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = c.R, c.G, c.B
			img.Pix[i+3] = artClamp(cov * 255)
		}
	}
}

func drawLongwave(img *image.RGBA) {
	artMarkBase(img, artHex(0x2c2f7e), artHex(0x14163f))
	white := artHex(0xf2f4ff)
	artCircle(img, 32, 47, 2.6, white, 1)
	for i := 0; i < 3; i++ {
		artArc(img, 32, 47, 9+7*float64(i), 2.2, math.Pi/6, 5*math.Pi/6, white, 0.95-float64(i)*0.16)
	}
}
func drawMarlin(img *image.RGBA) {
	artMarkBase(img, artHex(0x157e7c), artHex(0x093b45))
	white := artHex(0xf2fffb)
	artPoly(img, []artPt{{33, 14}, {33, 42}, {14, 42}}, white, 0.96)
	artPoly(img, []artPt{{36, 22}, {36, 42}, {48, 42}}, artMix(white, artHex(0x157e7c), 0.18), 0.9)
	for row := 0; row < 2; row++ {
		for x := 12; x <= 52; x++ {
			yy := int(math.Round(47.5 + float64(row)*6 + 2*math.Sin(float64(x)*0.22+float64(row)*1.4)))
			artRect(img, x, yy, x+1, yy+2, white, 0.85-float64(row)*0.25)
		}
	}
}
func drawCinder(img *image.RGBA) {
	artMarkBase(img, artHex(0x6d1c1c), artHex(0x2f0a0e))
	amber, core := artHex(0xffa63d), artHex(0xffdd8a)
	artEllipse(img, 32, 43, 10, 12, 0, amber, 0.95)
	artPoly(img, []artPt{{32, 13}, {44, 42}, {20, 42}}, amber, 0.95)
	artEllipse(img, 32, 43, 5, 7, 0, core, 0.9)
	artPoly(img, []artPt{{32, 26}, {38, 42}, {26, 42}}, core, 0.9)
}
func drawNeutralMark(img *image.RGBA) {
	artMarkBase(img, artHex(0x4a5262), artHex(0x252b36))
	white := artHex(0xeef1f4)
	artArc(img, 32, 32, 13, 2.4, 0, 2*math.Pi, white, 0.9)
	artCircle(img, 32, 32, 3, white, 0.9)
}
