//go:build preview

package app

import (
	"testing"

	"saundo/cmd/saundo/locales"
)

func TestPreviewMenuLampSubmenuHeight(t *testing.T) {
	a := PreviewApp()
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
