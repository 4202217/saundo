package app

import (
	"fmt"
	"log"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// menuIconSources are the tray menu's glyphs: the interface's own SVGs,
// drawn by the native menu as template images tinted with its text color.
var menuIconSources = []struct {
	name string
	svg  *ui.SVG
}{
	{"open", iconMark},
	{"settings", iconGear},
	{"connect", iconLink},
	{"quit", iconPower},
	{"update", iconRefresh},
	{"bell", iconBell},
	{"bellOff", iconBellOff},
	{"lamp", iconLamp},
	{"lampOff", iconLampOff},
}

// menuDeviceFamilies are the receiver glyphs a tray row can show.
var menuDeviceFamilies = []string{
	"homepod",
	"homepod-mini",
	"homepod-pair",
	"homepod-mini-pair",
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
	icons := make(map[string]*mygo.MenuIcon, len(menuIconSources)+3*len(menuDeviceFamilies))
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
	return icons
}
