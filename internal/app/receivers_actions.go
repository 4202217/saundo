package app

import "saundo/internal/receivers"

// The receivers page's actions go through act, so a preview build can fake
// them. The production closures reach the engine as before.
type receiverUpdateArg struct {
	key   string
	patch receivers.Patch
}

type receiverBoolArg struct {
	key string
	on  bool
}

type receiverDelayArg struct {
	key string
	ms  int
}

type receiverPasswordArg struct {
	key      string
	password string
}

func (a *App) actReceiverAdd(spec receivers.Spec) {
	a.act(ActionReceiverAdd, spec, func() { a.eng.AddReceiver(spec) })
}

func (a *App) actReceiverUpdate(key string, patch receivers.Patch) {
	a.act(ActionReceiverUpdate, receiverUpdateArg{key: key, patch: patch}, func() {
		a.eng.UpdateReceiver(key, patch)
	})
}

func (a *App) actReceiverRemove(key string) {
	a.act(ActionReceiverRemove, key, func() {
		delete(a.mutedVolume, key)
		a.eng.RemoveReceiver(key)
	})
}

func (a *App) actReceiverPassword(key, password string) {
	a.act(ActionReceiverPassword, receiverPasswordArg{key: key, password: password}, func() {
		a.eng.SetReceiverPassword(key, password)
	})
}

func (a *App) actReceiverHidden(key string, on bool) {
	a.act(ActionReceiverHidden, receiverBoolArg{key: key, on: on}, func() {
		a.eng.SetReceiverHidden(key, on)
	})
}

func (a *App) actReceiverAuto(key string, on bool) {
	a.act(ActionReceiverAuto, receiverBoolArg{key: key, on: on}, func() {
		a.eng.SetReceiverAutoConnect(key, on)
	})
}

func (a *App) actReceiverDelay(key string, ms int) {
	a.act(ActionReceiverDelay, receiverDelayArg{key: key, ms: ms}, func() {
		a.eng.SetReceiverDelay(key, ms)
	})
}
