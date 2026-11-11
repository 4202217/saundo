package app

import (
	"log"

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
	{"disconnect", iconUnlink},
	{"quit", iconPower},
	{"update", iconRefresh},
}

func loadMenuIcons() map[string]*mygo.MenuIcon {
	icons := make(map[string]*mygo.MenuIcon, len(menuIconSources))
	for _, it := range menuIconSources {
		icon, err := mygo.NewMenuIconSVG(it.svg, true)
		if err != nil {
			log.Printf("menu icon %s: %v", it.name, err)
			continue
		}
		icons[it.name] = icon
	}
	return icons
}
