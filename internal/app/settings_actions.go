package app

// The settings actions that would reach the engine or the OS go through act,
// so a preview build can fake them. The rest are already nil-safe.
func (a *App) actName() {
	a.act("name", a.nameDraft, func() { a.eng.SetName(a.nameDraft) })
}

func (a *App) actLaunch(on bool) {
	a.act("launch", on, func() { a.setLaunchAtStart(on) })
}

func (a *App) actReconnect(on bool) {
	a.act("reconnect", on, func() { a.eng.SetReconnect(on) })
}

func (a *App) actStrict(on bool) {
	a.act("strict", on, func() { a.eng.SetStrictAuth(on) })
}

func (a *App) actLanguage(label string) {
	a.act("language", label, func() { a.setLanguage(label) })
}
