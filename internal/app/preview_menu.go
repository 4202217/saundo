//go:build preview

package app

import (
	"bytes"
	"image/png"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

const previewMenuWidth = 260

// previewMenuWin stands in for the tray menu in the browser, which has no
// tray. The model and its items are the product's; the panel is preview-only.
var previewMenuWin *mygo.Window

var (
	previewMenuItemsCache []*mygo.MenuItem
	previewMenuLampOpen   bool
	previewMenuGlyphCache = map[*mygo.MenuIcon]*ui.Bitmap{}
	previewMenuSVGIcons   = map[*mygo.MenuIcon]*ui.SVG{}
)

// openPreviewMenu builds the tray menu's model and shows it as a panel.
func (a *App) openPreviewMenu() {
	a.closePreviewMenu()
	if a.settingsWin != nil {
		a.settingsWin.Hide()
	}
	if a.menuIcons == nil {
		a.menuIcons = loadMenuIcons()
	}
	previewMenuSVGIcons = map[*mygo.MenuIcon]*ui.SVG{}
	for _, it := range menuIconSources {
		if ic := a.menuIcons[it.name]; ic != nil {
			previewMenuSVGIcons[ic] = it.svg
		}
	}
	for _, family := range menuDeviceFamilies {
		if ic := a.menuIcons["dev:"+family]; ic != nil {
			previewMenuSVGIcons[ic] = sfIcons[family]
		}
	}
	items := a.trayMenuItems(a.trayModel(mygo.App.Locale(), mygo.Theme.IsDark()))
	previewMenuItemsCache = items
	previewMenuLampOpen = false
	h := previewMenuHeight(items)
	previewResizeMenu(h)
	previewMenuWin = mygo.NewWindow(mygo.WindowOptions{
		Title:  "saundo menu",
		Width:  previewMenuWidth,
		Height: h,
		Content: ui.View(func(c *ui.Context) {
			a.previewMenuView(c, items)
		}),
	})
	if a.win != nil {
		a.win.Hide()
	}
	previewMenuWin.Show()
}

func (a *App) closePreviewMenu() {
	if previewMenuWin != nil {
		previewMenuWin.Close()
		previewMenuWin = nil
	}
}

func (a *App) previewMenuView(c *ui.Context, items []*mygo.MenuItem) {
	p := design(c)
	ui.Column(c).Fill().Background(p.card).Padding(6).Children(func() {
		for _, item := range items {
			if item == nil || item.Hidden {
				continue
			}
			if item.Type == mygo.MenuItemSeparator {
				ui.Box(c).Height(1).Margin(6, 0, 6, 0).Background(p.line2)
				continue
			}
			a.previewMenuRow(c, p, item, 0, false)
			if previewMenuLampOpen && len(item.Submenu) > 0 {
				for _, sub := range item.Submenu {
					if sub == nil || sub.Hidden {
						continue
					}
					if sub.Type == mygo.MenuItemSeparator {
						ui.Box(c).Height(1).Margin(6, 0, 6, 0).Background(p.line2)
						continue
					}
					a.previewMenuRow(c, p, sub, 18, true)
				}
			}
		}
	})
}

func (a *App) previewMenuRow(c *ui.Context, p palette, item *mygo.MenuItem, indent float32, submenu bool) {
	h := float32(30)
	if item.Subtitle != "" {
		h = 40
	}
	row := ui.ButtonBase(c).FillWidth().Height(h).Padding(0, 10, 0, 10+indent).Gap(8).Radius(6).
		AlignItems(ui.Center).Justify(ui.Start).Label(item.Label).Background(ui.Transparent)
	if item.Disabled {
		row.Disabled(true)
	} else {
		row.Cursor(ui.CursorPointer)
		hoverFill(row, p.cardHover, 6, 0)
	}
	row.Children(func() {
		fg := p.fg
		if item.Disabled {
			fg = p.faint
		}
		if svg := previewMenuSVGIcons[item.Icon]; svg != nil {
			ui.Icon(c, svg).Size(16, 16).TextColor(fg)
		} else if bmp := previewMenuGlyph(item.Icon); bmp != nil {
			ui.Image(c, bmp).Size(16, 16).Shrink(0)
		} else {
			ui.Box(c).Size(16, 16)
		}
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, item.Label).FontSize(12.5).TextColor(fg).SingleLine().Ellipsis("…")
			if item.Subtitle != "" {
				ui.Text(c, item.Subtitle).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			}
		})
		if len(item.Submenu) > 0 {
			chev := ui.Icon(c, iconChevronRight).Size(10, 10).TextColor(p.muted)
			if !submenu && previewMenuLampOpen {
				chev.Rotate(90)
			}
		}
	})
	if !item.Disabled {
		if submenu {
			row.OnClick(func() { a.previewMenuSubChoose(item) })
		} else {
			row.OnClick(func() { a.previewMenuChoose(item) })
		}
	}
}

func previewMenuRowHeight(item *mygo.MenuItem) int {
	if item == nil || item.Hidden {
		return 0
	}
	switch {
	case item.Type == mygo.MenuItemSeparator:
		return 9
	case item.Subtitle != "":
		return 40
	default:
		return 30
	}
}

func previewMenuHeight(items []*mygo.MenuItem) int {
	h := 12
	for _, item := range items {
		h += previewMenuRowHeight(item)
		if previewMenuLampOpen && item != nil && len(item.Submenu) > 0 {
			for _, sub := range item.Submenu {
				h += previewMenuRowHeight(sub)
			}
		}
	}
	return h
}

// previewMenuGlyph decodes a colored menu icon's PNG. Template icons take
// the SVG path above; their mono PNG is a mask, not a picture.
func previewMenuGlyph(icon *mygo.MenuIcon) *ui.Bitmap {
	if icon == nil {
		return nil
	}
	if bmp, ok := previewMenuGlyphCache[icon]; ok {
		return bmp
	}
	p := icon.PlatformMenuIcon()
	if p == nil || len(p.PNG) == 0 || p.Template {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(p.PNG))
	if err != nil {
		return nil
	}
	bmp := ui.NewBitmap(img)
	previewMenuGlyphCache[icon] = bmp
	return bmp
}

// previewMenuChoose maps the top-level labels to the typed preview actions or
// opens the submenu. The item's real Click reaches the engine for the rows
// the preview does not fake.
func (a *App) previewMenuChoose(item *mygo.MenuItem) {
	if len(item.Submenu) > 0 {
		previewMenuLampOpen = !previewMenuLampOpen
		if previewMenuWin != nil {
			previewResizeMenu(previewMenuHeight(previewMenuItemsCache))
			previewMenuWin.Invalidate()
		}
		return
	}
	for _, d := range a.snap.Devices {
		if d.Name == item.Label && d.Online {
			a.closePreviewMenu()
			a.act(ActionConnect, d.ID, nil)
			a.showPreviewFlyout()
			return
		}
	}
	switch item.Label {
	case locales.ActionDisconnect(), locales.TrayMenuCancelConnect():
		a.closePreviewMenu()
		a.act(ActionDisconnect, nil, nil)
		a.showPreviewFlyout()
	case locales.TrayMenuRetry():
		id := a.snap.SelectedID
		if id == "" {
			id = a.selID
		}
		if id != "" {
			a.closePreviewMenu()
			a.act(ActionConnect, id, nil)
			a.showPreviewFlyout()
		}
	case locales.TraySettings():
		a.closePreviewMenu()
		a.showPreviewSettings()
	case locales.TrayShowPanel(), locales.TrayQuit():
		a.closePreviewMenu()
		a.showPreviewFlyout()
	}
}

func (a *App) previewMenuSubChoose(item *mygo.MenuItem) {
	a.closePreviewMenu()
	if item.Click != nil {
		item.Click(item, nil)
	}
	a.showPreviewFlyout()
}

func (a *App) showPreviewFlyout() {
	if a.settingsWin != nil {
		a.settingsWin.Hide()
	}
	if a.win != nil {
		a.win.Show()
	}
	previewResize(a)
	if a.win != nil {
		a.win.Invalidate()
	}
}
