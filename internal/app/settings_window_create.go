package app

// The settings window's creation and visibility. Its view is in
// settings_window.go.

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// showSettings opens the configuration window. The flyout keeps the playback
// controls, so it hides while the user works in this window.
func (a *App) showSettings() {
	if a.settingsWin == nil {
		return
	}
	a.syncTextScale()
	if a.win != nil {
		a.win.Hide()
	}
	if a.settingsWin.IsMinimized() {
		a.settingsWin.Restore()
	}
	a.appMenuOpen = false
	a.settingsWin.Show()
	a.settingsWin.Focus()
	mygo.App.Focus()
}

// hideSettings hides the settings window; the app stays in the tray.
func (a *App) hideSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Hide()
	}
}

// The design's page area. The layout's narrow branch reads it, not the
// window, so the content zoom must keep it exact at every text scale.
const (
	settingsContentWidth  = 720
	settingsContentHeight = 500
)

// newSettingsWindow builds the configuration window hidden; showSettings
// reveals it. Closing it hides it, and the app keeps running in the tray.
func (a *App) newSettingsWindow() *mygo.Window {
	winW, winH := scaledSize(settingsContentWidth, settingsContentHeight, a.textScale())
	minW, minH := scaledSize(640, 420, a.textScale())
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:          locales.SettingsWindowTitle(),
		Width:          winW,
		Height:         winH,
		UseContentSize: true,
		MinWidth:       minW,
		MinHeight:      minH,
		Hidden:         true,
		StateKey:       "settings",
		// Fills the window before the first frame paints, so a dark window
		// does not flash white as it opens.
		BackgroundColor: "light-dark(#f4f4f6, #1a1a1e)",
		// The view draws the title bar so it blends with the window's
		// background; the system keeps its buttons over it.
		TitleBarStyle: mygo.TitleBarHidden,
		Content:       ui.View(a.settingsView),
	})
	w.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		w.Hide()
	})
	// The log dialog belongs to this window; a reopened window starts without it.
	w.OnShow(func() {
		a.settingsEntered = false
		a.showLog = false
	})
	return w
}
