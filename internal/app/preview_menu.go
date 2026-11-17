//go:build preview

package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

const previewMenuWidth = 260

// previewMenuWin stands in for the tray menu in the browser, which has no
// tray. The model and its items are the product's; the panel is preview-only.
var previewMenuWin *mygo.Window

// openPreviewMenu builds the tray menu's model and shows it as a panel.
func (a *App) openPreviewMenu() {
	a.closePreviewMenu()
	if a.settingsWin != nil {
		a.settingsWin.Hide()
	}
	items := a.trayMenuItems(a.trayModel(mygo.App.Locale(), mygo.Theme.IsDark()))
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
			a.previewMenuRow(c, p, item)
		}
	})
}

func (a *App) previewMenuRow(c *ui.Context, p palette, item *mygo.MenuItem) {
	h := float32(30)
	if item.Subtitle != "" {
		h = 40
	}
	row := ui.ButtonBase(c).FillWidth().Height(h).Padding(0, 10).Gap(8).Radius(6).
		AlignItems(ui.Center).Justify(ui.Start).Label(item.Label).Background(ui.Transparent)
	if item.Disabled {
		row.Disabled(true)
	} else {
		row.Cursor(ui.CursorPointer)
		hoverFill(row, p.cardHover, 6, 0)
	}
	row.Children(func() {
		if item.Checked {
			ui.Icon(c, iconCheck).Size(11, 11).TextColor(p.accent)
		} else {
			ui.Box(c).Size(11, 11)
		}
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			color := p.fg
			if item.Disabled {
				color = p.faint
			}
			ui.Text(c, item.Label).FontSize(12.5).TextColor(color).SingleLine().Ellipsis("…")
			if item.Subtitle != "" {
				ui.Text(c, item.Subtitle).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			}
		})
		if len(item.Submenu) > 0 {
			ui.Icon(c, iconChevronRight).Size(10, 10).TextColor(p.muted)
		}
	})
	if !item.Disabled {
		row.OnClick(func() { a.previewMenuChoose(item) })
	}
}

func previewMenuHeight(items []*mygo.MenuItem) int {
	h := 12
	for _, item := range items {
		if item == nil || item.Hidden {
			continue
		}
		switch {
		case item.Type == mygo.MenuItemSeparator:
			h += 9
		case item.Subtitle != "":
			h += 40
		default:
			h += 30
		}
	}
	return h
}

// previewMenuChoose maps the menu's labels to the fake reducer. The item's
// real Click reaches the engine, which the preview does not run.
func (a *App) previewMenuChoose(item *mygo.MenuItem) {
	for _, d := range a.snap.Devices {
		if d.Name == item.Label && d.Online {
			a.closePreviewMenu()
			previewReduce(a, "connect", d.ID)
			a.showPreviewFlyout()
			return
		}
	}
	switch item.Label {
	case locales.ActionDisconnect(), locales.TrayMenuCancelConnect():
		a.closePreviewMenu()
		previewReduce(a, "disconnect", nil)
		a.showPreviewFlyout()
	case locales.TrayMenuRetry():
		id := a.snap.SelectedID
		if id == "" {
			id = a.selID
		}
		if id != "" {
			a.closePreviewMenu()
			previewReduce(a, "connect", id)
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
