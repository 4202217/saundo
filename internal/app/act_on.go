//go:build preview

package app

// act runs the fake controller. An action with no handler, or a handler that
// rejects its argument, panics: a preview click never falls through to a real
// engine or OS call.
func (a *App) act(action Action, arg any, real func()) {
	if !a.previewAction(action, arg) {
		panic("preview: unhandled action " + string(action))
	}
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}
