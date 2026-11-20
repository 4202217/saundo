package app

// The settings actions that would reach the engine or the OS go through act,
// so a preview build can fake them. The rest are already nil-safe.
func (a *App) actName() {
	a.act(ActionName, a.nameDraft, func() { a.eng.SetName(a.nameDraft) })
}

func (a *App) actLaunch(on bool) {
	a.act(ActionLaunch, on, func() { a.setLaunchAtStart(on) })
}

func (a *App) actReconnect(on bool) {
	a.act(ActionReconnect, on, func() { a.eng.SetReconnect(on) })
}

func (a *App) actStrict(on bool) {
	a.act(ActionStrict, on, func() { a.eng.SetStrictAuth(on) })
}

func (a *App) actLanguage(label string) {
	a.act(ActionLanguage, label, func() { a.setLanguage(label) })
}
