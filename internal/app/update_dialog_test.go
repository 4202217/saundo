package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestNotesThemeRebuildsWhenContrastChanges pins the notes theme cache key:
// high contrast recolors the palette without changing the appearance, so a
// cache keyed on dark alone would keep the previous colors.
func TestNotesThemeRebuildsWhenContrastChanges(t *testing.T) {
	a := &App{}
	plain := paletteFor(false)
	high := plain.withPreferences(ui.Preferences{HighContrast: true})
	if plain.contrast || !high.contrast {
		t.Fatalf("contrast = %t/%t, want false/true", plain.contrast, high.contrast)
	}

	first := a.mdThemeCache.forPalette(plain)
	if again := a.mdThemeCache.forPalette(plain); again != first {
		t.Fatal("an unchanged palette rebuilt the cached notes theme")
	}
	contrast := a.mdThemeCache.forPalette(high)
	if contrast == first {
		t.Fatal("a high-contrast palette kept the old notes theme; the cache key ignores contrast")
	}
	if back := a.mdThemeCache.forPalette(plain); back == contrast {
		t.Fatal("returning to the plain palette kept the contrast notes theme")
	}
}
