package app

// The tray flyout window: creation and lifecycle bindings. macOS needs the
// rounded corners set explicitly; on Windows 11 the system rounds the flyout
// itself.

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// bindWindowLifecycle attaches the shared window-lifecycle handlers to the
// current window.
func (a *App) bindWindowLifecycle() {
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		if a.tray == nil {
			a.quitting = true
			mygo.App.Quit()
			return
		}
		e.PreventDefault()
		a.win.Hide()
	})
	a.win.OnShow(func() { a.flyoutEntered = false })
	a.win.OnHide(func() { a.collapseLampList() })
}

// createFlyout builds the tray flyout window with the rounded corners macOS
// needs set explicitly (Windows 11 rounds its own). Run on the main thread.
func (a *App) createFlyout() {
	a.flyoutH = flyoutHeightFor(0, a.showLatencySwitch, false)
	opts := mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Gap:       6,
		Width:     flyoutWidth,
		Height:    a.flyoutH,
		Focusable: true,
		Hidden:    true,
		Content:   ui.View(a.view),
	}
	opts.CornerRadius = 12
	opts.Shadow = true // macOS follows the corners; Windows keeps its system shadow
	a.win = mygo.NewFlyout(opts)
	// A flyout names no title of its own; give it the app's, so window
	// metadata and accessibility do not fall back to the executable name.
	a.win.SetTitle(locales.AppName())
}
