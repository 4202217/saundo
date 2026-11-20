//go:build preview

package app

import (
	"time"

	"github.com/egoist/mygo"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// previewHandlers is the fake controller: one transition per typed action,
// updating the state the views read. A handler returns false for an argument
// of the wrong type, and act turns that into a panic. The transitions follow
// docs/protocol.md and docs/devices.md, so a fake state is one the engine can
// reach. It is filled in init because a handler reaches act, which reads the
// map.
var previewHandlers map[Action]func(*App, any) bool

func init() {
	previewHandlers = map[Action]func(*App, any) bool{
		ActionRefresh: func(*App, any) bool { return true },
		ActionSettings: func(a *App, arg any) bool {
			switch page := arg.(type) {
			case nil:
			case settingsPage:
				a.page = page
			default:
				return false
			}
			previewShowHost(a, SurfaceSettings)
			return true
		},
		ActionDisconnect: func(a *App, _ any) bool {
			a.cancelLatencyApply()
			a.finishLatencyApply()
			a.snap.State = sender.StateIdle
			a.snap.ConnectedName = ""
			a.snap.SelectedID = ""
			a.snap.Error = ""
			return true
		},
		ActionConnect: func(a *App, arg any) bool {
			id, ok := arg.(string)
			if !ok {
				return false
			}
			if id == "" {
				return true
			}
			a.cancelLatencyApply()
			a.finishLatencyApply()
			previewConnect(a, id)
			return true
		},
		ActionVolume: func(a *App, arg any) bool {
			v, ok := arg.(float64)
			if !ok {
				return false
			}
			a.volume = v
			a.snap.Volume = v
			return true
		},
		ActionReceiverAdd: func(a *App, arg any) bool {
			spec, ok := arg.(receivers.Spec)
			if !ok {
				return false
			}
			previewReceiverAdd(a, spec)
			return true
		},
		ActionReceiverUpdate: func(a *App, arg any) bool {
			u, ok := arg.(receiverUpdateArg)
			if !ok {
				return false
			}
			previewReceiverUpdate(a, u.key, u.patch)
			return true
		},
		ActionReceiverRemove: func(a *App, arg any) bool {
			key, ok := arg.(string)
			if !ok {
				return false
			}
			previewReceiverRemove(a, key)
			return true
		},
		ActionReceiverPassword: func(a *App, arg any) bool {
			p, ok := arg.(receiverPasswordArg)
			if !ok {
				return false
			}
			previewReceiverPassword(a, p.key, p.password)
			return true
		},
		ActionReceiverHidden: func(a *App, arg any) bool {
			b, ok := arg.(receiverBoolArg)
			if !ok {
				return false
			}
			previewReceiverHidden(a, b.key, b.on)
			return true
		},
		ActionReceiverAuto: func(a *App, arg any) bool {
			b, ok := arg.(receiverBoolArg)
			if !ok {
				return false
			}
			previewReceiverAuto(a, b.key, b.on)
			return true
		},
		ActionReceiverDelay: func(a *App, arg any) bool {
			d, ok := arg.(receiverDelayArg)
			if !ok {
				return false
			}
			previewReceiverDelay(a, d.key, d.ms)
			return true
		},
		ActionName: func(a *App, arg any) bool {
			name, ok := arg.(string)
			if !ok {
				return false
			}
			a.cfg.Name = name
			a.snap.Name = name
			return true
		},
		ActionLaunch: func(a *App, arg any) bool {
			on, ok := arg.(bool)
			if !ok {
				return false
			}
			a.launchAtStart = on
			a.cfg.LaunchAtStart = on
			return true
		},
		ActionReconnect: func(a *App, arg any) bool {
			on, ok := arg.(bool)
			if !ok {
				return false
			}
			a.reconnect = on
			a.cfg.Reconnect = on
			return true
		},
		ActionStrict: func(a *App, arg any) bool {
			on, ok := arg.(bool)
			if !ok {
				return false
			}
			a.strict = on
			a.cfg.StrictReceiverAuth = on
			return true
		},
		ActionLanguage: func(a *App, arg any) bool {
			label, ok := arg.(string)
			if !ok {
				return false
			}
			switch label {
			case locales.LanguageEnglish():
				previewLocale = "en"
				a.cfg.Language = "en"
			case locales.LanguageChinese():
				previewLocale = "zh-CN"
				a.cfg.Language = "zh-CN"
			default:
				a.cfg.Language = ""
			}
			return true
		},
	}
}

// previewAction runs the handler for one action. act adds the invalidation
// and the fail-closed panic.
func (a *App) previewAction(action Action, arg any) bool {
	h := previewHandlers[action]
	return h != nil && h(a, arg)
}

// previewAfter and previewOnMain are seams for the connect test.
var (
	previewAfter  = time.AfterFunc
	previewOnMain = mygo.RunOnMain
)

// previewConnect runs the fake handshake: connecting now, streaming after a
// beat, as the engine's snapshot would.
func previewConnect(a *App, id string) {
	name := ""
	for _, d := range a.snap.Devices {
		if d.ID == id {
			name = d.Name
			break
		}
	}
	a.selID = id
	a.snap.SelectedID = id
	a.snap.State = sender.StateHandshake
	a.snap.ConnectedName = ""
	a.snap.Error = ""
	previewAfter(650*time.Millisecond, func() {
		previewOnMain(func() {
			if a.snap.SelectedID != id || a.snap.State != sender.StateHandshake {
				return
			}
			a.snap.State = sender.StateStreaming
			a.snap.ConnectedName = name
			if a.win != nil {
				a.win.Invalidate()
			}
			if a.settingsWin != nil {
				a.settingsWin.Invalidate()
			}
		})
	})
}
