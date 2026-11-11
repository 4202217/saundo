package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMenuIconsLoad(t *testing.T) {
	icons := loadMenuIcons()
	for _, it := range menuIconSources {
		if icons[it.name] == nil {
			t.Fatalf("menu icon %q missing; got %v", it.name, icons)
		}
	}
}

// TestMenuIconPNGsRemoved keeps the generated rasters from creeping back:
// the menu now draws the same SVGs the interface does.
func TestMenuIconPNGsRemoved(t *testing.T) {
	for _, it := range menuIconSources {
		path := filepath.Join("assets", "menu-"+it.name+".png")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists (err=%v)", path, err)
		}
	}
}
