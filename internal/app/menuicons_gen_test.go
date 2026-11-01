package app

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestGenerateMenuIcons renders the tray menu's glyphs to the black and
// transparent PNGs a native menu tints, from the same SVGs the interface
// uses. Run with SAUNDO_MENU_ICON_DIR=internal/app/assets; the app embeds
// the files it writes.
func TestGenerateMenuIcons(t *testing.T) {
	dir := os.Getenv("SAUNDO_MENU_ICON_DIR")
	if dir == "" {
		t.Skip("set SAUNDO_MENU_ICON_DIR to write the menu icons")
	}
	icons := []struct {
		name string
		svg  *ui.SVG
	}{
		{"open", iconMark},
		{"settings", iconGear},
		{"connect", iconLink},
		{"disconnect", iconUnlink},
		{"quit", iconPower},
	}
	for _, it := range icons {
		img := ui.Render(func(c *ui.Context) {
			ui.Column(c).Fill().Background(ui.RGB(255, 255, 255)).Center().Children(func() {
				ui.Icon(c, it.svg).Size(16, 16).TextColor(ui.RGB(0, 0, 0))
			})
		}, 16, 16, 4)
		b := img.Bounds()
		out := image.NewNRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, _, _, _ := img.At(x, y).RGBA()
				// A black glyph on white: red's inverse is its coverage.
				out.SetNRGBA(x, y, color.NRGBA{A: uint8(255 - r>>8)})
			}
		}
		f, err := os.Create(filepath.Join(dir, "menu-"+it.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, out); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
