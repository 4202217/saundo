package app

import "saundo/internal/receivers"

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

func (a *App) actDisconnectTarget(id string) {
	a.act(ActionDisconnectTarget, id, func() { a.disconnectTarget(id) })
}

type targetVolumeArg struct {
	id string
	v  float64
}

func (a *App) actTargetVolume(id string, v float64) {
	a.act(ActionTargetVolume, targetVolumeArg{id: id, v: v}, func() {
		if a.eng != nil {
			a.eng.SetTargetVolume(id, v)
		}
	})
}

type targetMuteArg struct {
	id      string
	restore float64
}

func (a *App) actTargetMute(id string, restore float64) {
	a.act(ActionTargetMute, targetMuteArg{id: id, restore: restore}, func() {
		if a.eng != nil {
			a.eng.SetTargetVolume(id, restore)
		}
	})
}

type targetChannelArg struct {
	id      string
	channel string
}

func (a *App) actTargetChannel(id, channel string) {
	a.act(ActionTargetChannel, targetChannelArg{id: id, channel: channel}, func() {
		if a.eng != nil {
			a.eng.UpdateReceiver(id, receivers.Patch{Channel: &channel})
		}
	})
}
