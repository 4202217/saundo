package app

import (
	"embed"
	"log"

	"github.com/egoist/mygo"
)

// The tray menu's glyphs are rendered from the interface's SVGs by
// TestGenerateMenuIcons: black on transparent, which a native menu tints
// with its text color.
//
//go:embed assets/menu-*.png
var menuIconFS embed.FS

func loadMenuIcons() map[string]*mygo.MenuIcon {
	names := []string{"open", "settings", "connect", "disconnect", "quit"}
	icons := make(map[string]*mygo.MenuIcon, len(names))
	for _, name := range names {
		data, err := menuIconFS.ReadFile("assets/menu-" + name + ".png")
		if err != nil {
			continue
		}
		icon, err := mygo.NewMenuIcon(data, true)
		if err != nil {
			log.Printf("menu icon %s: %v", name, err)
			continue
		}
		icons[name] = icon
	}
	return icons
}
