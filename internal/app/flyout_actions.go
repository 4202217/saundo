package app

// The flyout's actions go through act, so a preview build can replace the
// engine with a fake reducer without changing the views. Keeping them here
// leaves flyout.go at its line budget.
func (a *App) actRefresh() { a.act("refresh", nil, a.eng.Refresh) }

func (a *App) actSettings() { a.act("settings", nil, a.showSettings) }

func (a *App) actUpdateSettings() {
	a.act("settings", nil, func() {
		a.page = pageUpdate
		a.showSettings()
	})
}

func (a *App) actDisconnect() { a.act("disconnect", nil, a.disconnect) }

func (a *App) actConnect(id string) {
	a.act("connect", id, func() { a.connectDevice(id) })
}

func (a *App) actVolume(v float64) {
	a.act("volume", v, func() { a.eng.SetVolume(v) })
}
