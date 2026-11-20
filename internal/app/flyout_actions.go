package app

// The flyout's actions go through act, so a preview build can replace the
// engine with a fake reducer without changing the views. Keeping them here
// leaves flyout.go at its line budget.
func (a *App) actRefresh() { a.act(ActionRefresh, nil, a.eng.Refresh) }

func (a *App) actSettings() { a.act(ActionSettings, nil, a.showSettings) }

// actUpdateSettings opens the Updates page, not the last page shown.
func (a *App) actUpdateSettings() {
	a.act(ActionSettings, pageUpdate, func() {
		a.page = pageUpdate
		a.showSettings()
	})
}

func (a *App) actDisconnect() { a.act(ActionDisconnect, nil, a.disconnect) }

func (a *App) actConnect(id string) {
	a.act(ActionConnect, id, func() { a.connectDevice(id) })
}

func (a *App) actVolume(v float64) {
	a.act(ActionVolume, v, func() { a.eng.SetVolume(v) })
}
