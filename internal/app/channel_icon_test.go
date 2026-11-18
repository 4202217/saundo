package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestChannelIconFadesTheOtherSide pins the icon's state: the solid side
// carries the current channel and the other side stays visible at a quarter
// ink. Image renders the SVG itself; Icon's mask would drop fill-opacity.
func TestChannelIconFadesTheOtherSide(t *testing.T) {
	for _, tc := range []struct {
		name      string
		icon      *ui.SVG
		solidLeft bool
	}{
		{"left", iconChannelLeft, true},
		{"right", iconChannelRight, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := func(c *ui.Context) {
				ui.Row(c).Fill().Background(ui.RGB(255, 255, 255)).Center().Children(func() {
					ui.Image(c, tc.icon).Size(40, 40).TextColor(ui.RGB(0, 0, 0))
				})
			}
			tt := ui.NewTester(view, 64, 64)
			tt.SetDark(false)
			tt.Frame()
			img := tt.Image()
			var left, right int
			b := img.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					c := img.RGBAAt(x, y)
					gray := (int(c.R) + int(c.G) + int(c.B)) / 3
					if x < b.Dx()/2 {
						left += 255 - gray
					} else {
						right += 255 - gray
					}
				}
			}
			solid, faded := left, right
			if !tc.solidLeft {
				solid, faded = right, left
			}
			if faded == 0 || solid < faded*2 {
				t.Fatalf("solid side ink %d, faded side ink %d: the faded side is not distinguishable", solid, faded)
			}
		})
	}
}
