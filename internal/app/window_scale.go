package app

import (
	"github.com/egoist/mygo"
)

// fitFlyout sizes the tray flyout to the visible device rows and the current
// media state. The device card and the playback card touch directly, and the
// media card adds its height only while a session exists. The flyout is
// anchored to the tray icon by its bottom edge, so a resize grows or shrinks
// it upward. Main thread only.
func (a *App) fitFlyout() {
	if a.win == nil && a.resizeWindow == nil {
		return
	}
	h := flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, a.hasNowMedia(), a.lampCover, a.lampExpanded, a.errorBarShown(), a.snap.CredentialReset, a.noticeShown())
	scale := a.textScale()
	if h == a.flyoutH && scale == a.flyoutScale {
		return
	}
	a.flyoutH, a.flyoutScale = h, scale
	w, wh := scaledSize(flyoutWidth, h, scale)
	if a.resizeWindow != nil {
		a.resizeWindow(w, wh)
		return
	}
	a.win.SetSize(w, wh)
}

func (a *App) textScale() float32 {
	if a.scale <= 0 {
		return 1
	}
	return a.scale
}

// syncTextScale adopts a text-scale change outside a view, before a window
// shows, so the first frame already has the room its content needs.
func (a *App) syncTextScale() {
	s := normalizeScale(mygo.Theme.Preferences().TextScale)
	if s == a.textScale() {
		return
	}
	a.scale = s
	a.flyoutScale = 0
	a.fitFlyout()
	a.resizeSettingsWindow()
}

// adoptTextScale takes the scale a view reports. Resizing from inside the
// build would show a stretched frame on mygo 0.9.0, so the resize is posted.
func (a *App) adoptTextScale(scale float32, w *mygo.Window) {
	if scale == a.textScale() {
		return
	}
	a.scale = scale
	if w == nil {
		return
	}
	w.Update(func() {
		a.flyoutScale = 0
		a.fitFlyout()
		a.resizeSettingsWindow()
	})
}

func (a *App) resizeSettingsWindow() {
	if a.settingsWin == nil {
		return
	}
	// The settings window describes its page area, so the scaled size goes
	// through SetContentSize; the zoom then keeps the view at 720×500.
	w, h := scaledSize(settingsContentWidth, settingsContentHeight, a.textScale())
	a.settingsWin.SetContentSize(w, h)
}

func (a *App) showWindow() {
	if a.win == nil {
		return
	}
	a.syncTextScale()
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
	focusAppOnShow()
}

func (a *App) toggleWindow() {
	if a.win == nil {
		return
	}
	if a.win.IsVisible() {
		a.win.Hide()
		return
	}
	a.showWindow()
}
