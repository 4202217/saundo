package app

import (
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/settings"
)

// The 连接 settings page's session card and the settings sidebar's identity.
// Both read the session member set, so a multi-target session shows the joined
// name instead of one selected device.

func (a *App) connectionPage(c *ui.Context, p palette) {
	if m, ok := a.sessionPrimary(); ok {
		a.connectionHealthCard(c, p, m.Device)
	} else if d, ok := a.currentDevice(); ok {
		a.connectionHealthCard(c, p, d)
	}
	pageCard(c, p, func() {
		settingRow(c, p, locales.SettingsReconnect(), locales.SettingsReconnectDesc(), func() {
			toggle(c, p, &a.reconnect, locales.SettingsReconnect(), func(v bool) { a.eng.SetReconnect(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsStrictAuth(), locales.SettingsStrictAuthDesc(), func() {
			toggle(c, p, &a.strict, locales.SettingsStrictAuth(), func(v bool) { a.eng.SetStrictAuth(v) })
		})
		rowLine(c, p)
		title := locales.SettingsMuteLocal()
		if a.processMuteMode() {
			title = locales.SettingsMuteLocalApp()
		}
		settingRow(c, p, title, a.muteLocalSub(), func() {
			toggle(c, p, &a.cfg.MuteLocalOnConnect, title, a.setMuteLocal)
		})
	})
	if a.cfg.MuteLocalOnConnect {
		note := locales.SettingsMuteLocalNote()
		if a.processMuteMode() {
			note = locales.SettingsMuteLocalAppNote()
		}
		ui.Column(c).Margin(8, 0, 0, 2).Children(func() {
			infoNote(c, p, note)
		})
	}
}

// processMuteMode reports whether the mute switch targets the captured app

// connectionHealthCard is the connection page's leading card. It keeps the
// receiver's identity and state, and adds the session's latency and stability
// only while streaming. The two latency values are separate measurements, not
// a total, so they are drawn as two independent stats.
func (a *App) connectionHealthCard(c *ui.Context, p palette, d device.Device) {
	connected := phaseOf(a.snap) == phaseStreaming
	members := a.sessionMembers()
	name, sub := d.Name, summarySub(d)
	group := d.IsStereoGroup()
	if len(members) > 1 {
		// The card speaks for the whole session, not one of its members.
		name, sub, group = a.sessionName(), locales.SummarySessionReceivers(len(members)), true
	}
	card := ui.Column(c).Margin(12, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip()
	card.Children(func() {
		ui.Row(c).Height(56).Padding(0, 14).Gap(10).Children(func() {
			deviceGlyph(c, p, d, false)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
				ui.Text(c, sub).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			})
			a.connectionStateChip(c, p)
		})
		if connected {
			rowLine(c, p)
			a.connectionHealthStats(c, p, group)
		}
	})
}

func (a *App) connectionStateChip(c *ui.Context, p palette) {
	switch phaseOf(a.snap) {
	case phaseStreaming:
		chip(c, p, locales.SummaryConnected(), chipOK)
	case phaseConnecting:
		chip(c, p, locales.SummaryConnecting(), chipAccent)
	case phaseError:
		text := locales.StatusConnectionFailed()
		if a.snap.TakenOver {
			text = locales.StatusTakenOver()
		}
		chip(c, p, text, chipDanger)
	default:
		chip(c, p, locales.StatusDisconnected(), chipNeutral)
	}
}

func (a *App) connectionHealthStats(c *ui.Context, p palette, group bool) {
	ui.Column(c).Padding(12, 14).Children(func() {
		ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
			healthStat(c, p, locales.SettingsLatency(), settings.LatencyText(a.latencyMS()),
				locales.SettingsHealthLatencySub())
			value, sub := "—", locales.SettingsHealthReceiverUnknown()
			if a.snap.ReceiverLatencyMS > 0 {
				value = settings.LatencyText(a.snap.ReceiverLatencyMS)
				if group {
					value = locales.SettingsHealthReceiverGroupValue(value)
				}
				sub = locales.SettingsHealthReceiverSub()
			}
			healthStat(c, p, locales.SettingsHealthReceiver(), value, sub)
			status, tone := locales.SettingsHealthDropsGood(), p.green
			if a.snap.DroppedSamples > 0 {
				status, tone = locales.SettingsHealthDropsBad(), p.red
			}
			healthStatusStat(c, p, locales.SettingsHealthDrops(), status,
				locales.SettingsHealthDropsSub(), tone)
		})
		ui.Text(c, locales.SettingsHealthNote()).FontSize(10.5).TextColor(p.muted).Margin(10, 0, 0, 0)
	})
}

func healthStat(c *ui.Context, p palette, label, value, sub string) {
	ui.Column(c).Grow(1).Gap(2).Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		ui.Text(c, value).FontSize(15).FontWeight(600).FontFeatures("tnum").SingleLine()
		ui.Text(c, sub).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
	})
}

func healthStatusStat(c *ui.Context, p palette, label, value, sub string, tone ui.Color) {
	ui.Column(c).Grow(1).Gap(2).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Shrink(0).Background(tone)
			ui.Text(c, label).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
		ui.Text(c, value).FontSize(15).FontWeight(600).TextColor(tone).SingleLine()
		ui.Text(c, sub).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
	})
}

func summarySub(d device.Device) string {
	if d.IsStereoGroup() {
		return locales.SummaryStereoGroup(len(d.Members))
	}
	if d.Model != "" {
		return locales.SummaryModelCurrentDevice(d.Model)
	}
	return locales.SummaryCurrentDevice()
}

func (a *App) sidebarIdentity(p palette) (name, sub string, dot ui.Color) {
	members := a.sessionMembers()
	d, ok := a.currentDevice()
	name = locales.SidebarNoDevice()
	group := false
	switch {
	case len(members) > 1:
		name = a.sessionName()
	case len(members) == 1:
		name, group = members[0].Device.Name, members[0].Device.IsStereoGroup()
	case ok:
		name, group = d.Name, d.IsStereoGroup()
	}
	switch phaseOf(a.snap) {
	case phaseStreaming:
		sub, dot = locales.SummaryConnected(), p.green
		if group {
			sub = locales.SidebarConnectedStereo()
		}
	case phaseConnecting:
		sub, dot = a.statusText(), p.accent
	case phaseError:
		sub, dot = locales.StatusConnectionFailed(), p.red
		if a.snap.TakenOver {
			sub = locales.StatusTakenOver()
		}
	default:
		sub, dot = locales.StatusDisconnected(), p.faint
	}
	return name, sub, dot
}
