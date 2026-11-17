//go:build preview

package app

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// TestTrayMenuScenarioRenders drives the catalog's tray row through the same
// panel the browser shows: the rows come from the fixture and the size from
// the menu's own height.
func TestTrayMenuScenarioRenders(t *testing.T) {
	withLocale(t, "en")
	sc := previewScenario(t, "tray.menu")
	a := Fixture("en")
	sc.Apply(a)
	w, h := sc.Size(a)
	if w != previewMenuWidth {
		t.Fatalf("menu width = %d, want %d", w, previewMenuWidth)
	}
	if h <= 0 {
		t.Fatalf("menu height = %d", h)
	}
	tt := ui.NewTester(sc.view(a), w, h)
	tt.SetScale(1)
	tt.SetLocale("en")
	tt.Frame()
	for _, want := range []string{"HomePods", "Apple TV", locales.ActionDisconnect()} {
		if !tt.HasText(want) {
			t.Fatalf("the tray menu is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

// TestPreviewMenuHeightCoversItsRows pins the panel height to the rows the
// view draws. The separator is a 1-DIP line with 6 DIP above and below, so a
// 9-DIP count clips the last row.
func TestPreviewMenuHeightCoversItsRows(t *testing.T) {
	a := Fixture(previewLocale)
	items := a.trayMenuItems(a.trayModel("en", false))
	want := 2 * previewMenuPad
	for _, item := range items {
		if item == nil || item.Hidden {
			continue
		}
		switch {
		case item.Type == mygo.MenuItemSeparator:
			want += previewMenuSeparatorDIP
		case item.Subtitle != "":
			want += previewMenuRowSubtitleDIP
		default:
			want += previewMenuRowDIP
		}
	}
	if got := previewMenuHeight(items); got != want {
		t.Fatalf("menu height = %d, want %d", got, want)
	}
}

func TestPreviewMenuLampSubmenuHeight(t *testing.T) {
	a := Fixture(previewLocale)
	items := a.trayMenuItems(a.trayModel("zh-CN", true))
	previewMenuLampOpen = false
	defer func() { previewMenuLampOpen = false }()
	closed := previewMenuHeight(items)
	previewMenuLampOpen = true
	open := previewMenuHeight(items)
	if open <= closed {
		t.Fatalf("lamp submenu height = %d, closed = %d", open, closed)
	}
	found := false
	for _, item := range items {
		if item != nil && item.Label == locales.FlyoutLampSection() {
			found = len(item.Submenu) > 0
			break
		}
	}
	if !found {
		t.Fatal("the tray menu has no ambient submenu")
	}
}
