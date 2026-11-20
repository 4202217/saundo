package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/internal/lamp"
)

// menuIconSources are the tray menu's glyphs: the interface's own SVGs,
// drawn by the native menu as template images tinted with its text color.
var menuIconSources = []struct {
	name string
	svg  *ui.SVG
}{
	{"open", iconMark},
	{"settings", iconGear},
	{"quit", iconPower},
	{"update", iconRefresh},
	{"lampOff", iconLampOff},
	{"unlink", iconUnlink},
	{"close", iconClose},
	{"prev", iconPrev},
	{"play", iconPlay},
	{"pause", iconPause},
	{"next", iconNext},
}

// menuDeviceFamilies are the receiver glyphs a tray row can show.
var menuDeviceFamilies = []string{
	"homepod",
	"homepod-mini",
	"homepod-pair",
	"homepod-mini-pair",
	"homepod-mini-combo",
	"appletv",
	"airport-express",
	"macbook",
	"macmini",
	"macstudio",
	"desktop",
	"hifispeaker",
}

// menuAccentVariants are the in-session device's colored glyphs, in the
// palette's accent for each appearance.
var menuAccentVariants = []struct {
	prefix string
	color  string
}{
	{"devon:", accentHex(paletteFor(false).accent)},
	{"devond:", accentHex(paletteFor(true).accent)},
}

// accentHex is a palette color as the #rrggbb string an SVG source takes.
func accentHex(c ui.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func loadMenuIcons() map[string]*mygo.MenuIcon {
	icons := make(map[string]*mygo.MenuIcon, len(menuIconSources)+3*len(menuDeviceFamilies)+len(lamp.All())+1)
	for _, it := range menuIconSources {
		icon, err := mygo.NewMenuIconSVG(it.svg, true)
		if err != nil {
			log.Printf("menu icon %s: %v", it.name, err)
			continue
		}
		icons[it.name] = icon
	}
	for _, family := range menuDeviceFamilies {
		icon, err := mygo.NewMenuIconSVG(sfIcons[family], true)
		if err != nil {
			log.Printf("menu icon dev:%s: %v", family, err)
		} else {
			icons["dev:"+family] = icon
		}
		// The colored variant must keep its accent instead of following the
		// menu's text color, so it replaces currentColor in the source and
		// loads as a non-template image.
		for _, v := range menuAccentVariants {
			name := v.prefix + family
			svg, err := ui.ParseSVG([]byte(strings.ReplaceAll(sfIconSVG[family], "currentColor", v.color)))
			if err != nil {
				log.Printf("menu icon %s: %v", name, err)
				continue
			}
			icon, err := mygo.NewMenuIconSVG(svg, false)
			if err != nil {
				log.Printf("menu icon %s: %v", name, err)
				continue
			}
			icons[name] = icon
		}
	}
	for _, p := range lamp.All() {
		if icon := previewMenuIcon(p.Colors, p.Weights); icon != nil {
			icons["lamp:"+p.ID] = icon
		}
	}
	if icon := previewMenuIcon(neutralPreviewColors, coverPreviewWeights); icon != nil {
		icons["lamp:cover"] = icon
	}
	return icons
}

// neutralPreviewColors stand in for the cover preview until a cover exists.
var neutralPreviewColors = [3]color.RGBA{
	{R: 0x8f, G: 0x8f, B: 0x9a, A: 0xff},
	{R: 0xb9, G: 0xb9, B: 0xc2, A: 0xff},
	{R: 0x62, G: 0x62, B: 0x70, A: 0xff},
}

var coverPreviewWeights = [3]float64{0.40, 0.35, 0.25}

// coverPreviewColors samples the light's color field for the cover choice's
// round preview. Without a cover, the neutral palette stands in.
func coverPreviewColors(light, dark *image.RGBA, darkMode bool) ([3]color.RGBA, [3]float64) {
	field := light
	if darkMode {
		field = dark
	}
	if field == nil || field.Bounds().Empty() {
		return neutralPreviewColors, coverPreviewWeights
	}
	at := func(fx, fy float64) color.RGBA {
		b := field.Bounds()
		x := b.Min.X + int(fx*float64(b.Dx()))
		y := b.Min.Y + int(fy*float64(b.Dy()))
		if x >= b.Max.X {
			x = b.Max.X - 1
		}
		if y >= b.Max.Y {
			y = b.Max.Y - 1
		}
		c := field.RGBAAt(x, y)
		c.A = 0xff
		return c
	}
	return [3]color.RGBA{at(0.30, 0.30), at(0.70, 0.38), at(0.50, 0.82)}, coverPreviewWeights
}

// previewMenuIcon draws a round 32px preview from the light's three colors.
// The menu scales it to its own icon size; non-template images keep the
// colors under both menu appearances.
func previewMenuIcon(colors [3]color.RGBA, weights [3]float64) *mygo.MenuIcon {
	data := previewMenuPNG(colors, weights)
	if data == nil {
		return nil
	}
	icon, err := mygo.NewMenuIcon(data, false)
	if err != nil {
		log.Printf("menu preview icon: %v", err)
		return nil
	}
	return icon
}

func previewMenuPNG(colors [3]color.RGBA, weights [3]float64) []byte {
	const (
		size   = 32
		radius = 15.5
		sigma  = 0.28
	)
	centers := [3][2]float64{{0.30, 0.30}, {0.70, 0.38}, {0.50, 0.82}}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			d := math.Hypot(dx, dy)
			if d > radius {
				continue
			}
			nx := (float64(x) + 0.5) / size
			ny := (float64(y) + 0.5) / size
			var ws [3]float64
			var sum float64
			for i, c := range centers {
				dd := (nx-c[0])*(nx-c[0]) + (ny-c[1])*(ny-c[1])
				ws[i] = weights[i] * math.Exp(-dd/(2*sigma*sigma))
				sum += ws[i]
			}
			var r, g, b float64
			for i, w := range ws {
				if sum > 0 {
					w /= sum
				}
				r += float64(colors[i].R) * w
				g += float64(colors[i].G) * w
				b += float64(colors[i].B) * w
			}
			// A faint inner ring keeps the circle legible on a white menu.
			if d > radius-1.2 {
				r *= 0.80
				g *= 0.80
				b *= 0.80
			}
			alpha := 1.0
			if d > radius-1 {
				alpha = radius - d
			}
			if alpha < 0 {
				alpha = 0
			}
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(clampFloat(r, 0, 255)),
				G: uint8(clampFloat(g, 0, 255)),
				B: uint8(clampFloat(b, 0, 255)),
				A: uint8(clampFloat(alpha*255, 0, 255)),
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Printf("menu preview png: %v", err)
		return nil
	}
	return buf.Bytes()
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
