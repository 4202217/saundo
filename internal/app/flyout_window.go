package app

// The tray flyout window: creation, lifecycle bindings, and the hot-swap
// between the rounded flyout and the system-popover look. macOS-only knobs
// (docs/platforms.md); on Windows 11 the system rounds the flyout itself.

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// bindWindowLifecycle attaches the shared window-lifecycle handlers to the
// current window. It runs again after rebuildFlyout.
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

// createFlyout builds the tray flyout window in the configured style: a
// system popover, or the rounded flyout whose corners macOS needs set
// explicitly (Windows 11 rounds its own). Run on the main thread.
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
	if a.cfg.FlyoutPopover {
		opts.Popover = true
	} else {
		opts.CornerRadius = 12
	}
	a.win = mygo.NewFlyout(opts)
	// A flyout names no title of its own; give it the app's, so window
	// metadata and accessibility do not fall back to the executable name.
	a.win.SetTitle(locales.AppName())
}

// rebuildFlyout swaps the flyout window for one in the new style. The old
// window is destroyed outright: its state is transient and the handlers are
// rebound by createFlyout's caller.
func (a *App) rebuildFlyout() {
	if a.win == nil || a.tray == nil {
		return
	}
	a.win.Destroy()
	a.createFlyout()
	a.bindWindowLifecycle()
}

// setFlyoutPopover persists the popover-style preference and hot-swaps the
// flyout window.
func (a *App) setFlyoutPopover(on bool) {
	a.flyoutPopover = on
	a.cfg.FlyoutPopover = on
	if a.sets != nil {
		cfg := a.sets.Load()
		cfg.FlyoutPopover = on
		_ = a.sets.Save(cfg)
	}
	if a.eng != nil {
		a.eng.SetFlyoutPopover(on)
	}
	a.rebuildFlyout()
}
