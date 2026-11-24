package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/engine"
)

// The 连接 settings page: the devices in the current session, the session's
// measured state, and the connection switches. Every value comes from the
// snapshot; the only setting the page echoes is the effective latency, with
// the family floor its guard needs.

// deviceUnhealthyRetransmit is the retransmit share that makes one device's
// link worth naming. Below it the counters stay hidden, so a healthy session
// is not noisy. Any underrun in the stability window is notable by itself.
const deviceUnhealthyRetransmit = 0.5

func (a *App) connectionPage(c *ui.Context, p palette) {
	a.connectionSession(c, p)
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

// connectionSession draws the device list, a lost member's notice, and the
// measured session card. Nothing is drawn without a session.
func (a *App) connectionSession(c *ui.Context, p palette) {
	members := a.sessionMembers()
	if len(members) == 0 {
		return
	}
	groupHead(c, p, locales.SettingsConnectionDevices(), len(members), 6, nil)
	card := ui.Column(c).Margin(8, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip().Shrink(0)
	card.Children(func() {
		for i, m := range members {
			a.connectionDeviceRow(c, p, m, i == len(members)-1)
		}
	})
	a.connectionLostNotice(c, p)
	if phaseOf(a.snap) == phaseStreaming && a.snap.Diagnostics.HasStats {
		a.connectionThisHead(c, p)
		a.connectionSessionCard(c, p)
	}
}

// connectionThisHead is the measured card's section head. The session's age
// is context, not a KPI, so it rides the right edge at note size.
func (a *App) connectionThisHead(c *ui.Context, p palette) {
	ui.Row(c).Margin(12, 0, 0, 0).FillWidth().Height(26).Gap(7).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, locales.SettingsConnectionThis()).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted)
		ui.Spacer(c)
		ui.Text(c, locales.SummaryConnected()+" "+connectionUptimeText(a.snap.Diagnostics.SessionUptime)).
			FontSize(10.5).TextColor(p.muted).SingleLine()
	})
}

// connectionDeviceRow is one connected target on two lines: the name with a
// notable event (and its age) over the live ring headroom and the send rate.
// Channel and volume belong to the tray panel's mixer, not this page.
func (a *App) connectionDeviceRow(c *ui.Context, p palette, m sessionMember, last bool) {
	row := ui.Row(c).Padding(8, 14).Gap(10).AlignItems(ui.Center)
	if !last {
		row.BorderWidth(0, 0, 1, 0).BorderColor(p.line2)
	}
	row.Children(func() {
		connectionGlyph(c, p, m.Device)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Row(c).Height(20).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, m.Device.Name).FontSize(13).FontWeight(500).Shrink(1).SingleLine().Ellipsis("…")
				if event := a.connectionDeviceEvent(m); event != "" {
					ui.Text(c, event).FontSize(11.5).TextColor(p.warning).Shrink(0).SingleLine()
				}
			})
			ui.Row(c).Height(14).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.connectionDeviceHead(m)).FontSize(10.5).TextColor(p.muted).
					FontFeatures("tnum").SingleLine()
				ui.Text(c, "·").FontSize(10.5).TextColor(p.faint).SingleLine()
				ui.Text(c, a.connectionDeviceRate(m)).FontSize(10.5).TextColor(p.muted).
					FontFeatures("tnum").SingleLine()
			})
		})
		text, tone := a.connectionMemberState(m, p)
		ui.Row(c).Shrink(0).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(tone)
			ui.Text(c, text).FontSize(11.5).TextColor(p.muted).SingleLine()
		})
	})
}

// connectionDeviceEvent names the member's current trouble: an underrun in
// the window with its age, or a retransmit share worth seeing.
func (a *App) connectionDeviceEvent(m sessionMember) string {
	if !m.HasTarget {
		return ""
	}
	h := m.Target.Health
	if h.RecentUnderruns > 0 {
		if h.UnderrunAge > 0 {
			seconds := int(h.UnderrunAge.Seconds() + 0.5)
			if seconds < 1 {
				seconds = 1
			}
			return locales.SettingsConnectionUnderrunsAgo(int(h.RecentUnderruns), seconds)
		}
		return locales.SettingsUnderrunsShort(int(h.RecentUnderruns))
	}
	if rate, ok := retransmitRate(h.Retransmits, h.SentBlocks); ok && rate >= deviceUnhealthyRetransmit {
		return locales.SettingsRetransmitShort(fmt.Sprintf("%.1f", rate))
	}
	return ""
}

// connectionDeviceHead is one member's current ring headroom. A silent source
// leaves the ring dry by design, so it reads as idle instead of zero.
func (a *App) connectionDeviceHead(m sessionMember) string {
	if a.snap.AudioIdle {
		return locales.SettingsConnectionHeadroomDeviceIdle()
	}
	if !m.HasTarget || !m.Target.Health.BufferValid {
		return locales.SettingsConnectionHeadroomDeviceDash()
	}
	return locales.SettingsConnectionHeadroomDevice(int(math.Round(m.Target.Health.BufferMS)))
}

// connectionDeviceRate is one member's send rate at L4.
func (a *App) connectionDeviceRate(m sessionMember) string {
	if !m.HasTarget || !m.Target.Health.TrafficValid {
		return "—"
	}
	return connectionTrafficText(m.Target.Health.TrafficBPS)
}

func connectionTrafficText(bps float64) string {
	kb := bps / 1000
	if kb < 10 {
		return fmt.Sprintf("%.1f KB/s", kb)
	}
	return fmt.Sprintf("%.0f KB/s", kb)
}

func connectionPacketText(bytes int) string {
	return fmt.Sprintf("%d B", bytes)
}

func connectionGlyph(c *ui.Context, p palette, d device.Device) {
	g := ui.Box(c).Size(24, 24).Radius(12).Center().Shrink(0).Background(p.pill)
	g.Children(func() {
		ui.Icon(c, stereoIcon(d, d.IsStereoGroup())).Size(13, 13).TextColor(p.fg2)
		if d.Online {
			ui.Box(c).Size(8, 8).Radius(4).Background(p.green).
				Shadow(0, 0, 0, 2, p.card).
				Attach(ui.AnchorBottomRight, ui.AnchorBottomRight).Right(-1).Bottom(-1)
		}
	})
}

// connectionMemberState names one member's role in the session, not the
// session phase: a member of a re-establishing cohort is still connected.
func (a *App) connectionMemberState(m sessionMember, p palette) (string, ui.Color) {
	switch m.Target.State {
	case engine.TargetJoining:
		return locales.StatusConnecting(), p.accent
	case engine.TargetReopening:
		return locales.StatusReconnecting(), p.accent
	case engine.TargetFailed:
		return locales.StatusConnectionFailed(), p.red
	}
	if !m.HasTarget {
		return locales.StatusDisconnected(), p.faint
	}
	if phaseOf(a.snap) == phaseStreaming {
		return locales.StatusStreaming(), p.green
	}
	return locales.SummaryConnected(), p.muted
}

// connectionLostNotice explains a member the engine dropped while the rest
// kept playing. It reads the engine's notice, not the flyout's transient one,
// so it stays until a user action clears it.
func (a *App) connectionLostNotice(c *ui.Context, p palette) {
	n := a.snap.Notice
	if n.Kind != engine.NoticeMemberLost || n.DeviceID == "" || a.snap.Active(n.DeviceID) {
		return
	}
	row := ui.Row(c).Margin(8, 0, 0, 0).MinHeight(44).Padding(8, 10).Radius(9).Gap(9).
		AlignItems(ui.Center).Background(p.accentSoft).Border(1, p.line)
	row.Children(func() {
		ui.Icon(c, iconInfo).Size(16, 16).TextColor(p.accent)
		ui.Text(c, locales.SettingsConnectionLost(n.Name)).FontSize(11.5).TextColor(p.fg2).
			Grow(1).SingleLine().Ellipsis("…")
		b := ui.ButtonBase(c.Key("lost-reconnect")).Height(26).Radius(7).Padding(0, 8).
			Center().Shrink(0).Label(locales.ActionReconnect()).Cursor(ui.CursorPointer)
		b.OnClick(func() { a.connectDevice(n.DeviceID) })
		hoverFill(b, p.pill, 7, 0)
		b.Children(func() {
			ui.Text(c, locales.ActionReconnect()).FontSize(11.5).FontWeight(500).
				TextColor(p.accent).SingleLine()
		})
	})
}

func connectionUptimeText(d time.Duration) string {
	minutes := int(d.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	if minutes < 60 {
		return locales.SettingsUptimeMinutes(minutes)
	}
	h, m := minutes/60, minutes%60
	if m == 0 {
		return locales.SettingsUptimeHoursWhole(h)
	}
	return locales.SettingsUptimeHours(h, m)
}

func retransmitRate(retransmits, sent uint64) (float64, bool) {
	if sent == 0 {
		return 0, false
	}
	return float64(retransmits) / float64(sent) * 100, true
}

// sidebarFooterColumn is the settings sidebar's footer text: the identity
// name with the joined session name as its tooltip when the count folded
// members away, over the link-state line.
func sidebarFooterColumn(c *ui.Context, p palette, name, sub, tip string) {
	ui.Column(c).Gap(1).Grow(1).Children(func() {
		text := ui.Text(c, name).FontSize(11.5).SingleLine().Ellipsis("…")
		if tip != "" {
			text = text.Tooltip(tip)
		}
		ui.Text(c, sub).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
	})
}

func (a *App) sidebarIdentity(p palette) (name, sub string, dot ui.Color, tip string) {
	members := a.sessionMembers()
	d, ok := a.currentDevice()
	name = locales.SidebarNoDevice()
	group := false
	combo := false
	switch {
	case len(members) > 1:
		name = locales.SidebarSessionCount(members[0].Device.Name, len(members)-1)
		tip = a.sessionName()
	case len(members) == 1:
		name, group = members[0].Device.Name, members[0].Device.IsStereoGroup()
		if v, found := a.receiverViewFor(members[0].Device.ID); found && v.Entry.IsCombo() {
			combo = true
		}
	case ok:
		name, group = d.Name, d.IsStereoGroup()
		if v, found := a.receiverViewFor(d.ID); found && v.Entry.IsCombo() {
			combo = true
		}
	}
	switch phaseOf(a.snap) {
	case phaseStreaming:
		sub, dot = locales.SummaryConnected(), p.green
		if group {
			sub = locales.SidebarConnectedStereo()
		}
		if combo {
			// A software combo is not a native pair: the sidebar says so in
			// the combo's own words.
			sub = locales.SummaryConnected() + " · " + locales.ComboTag()
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
	return name, sub, dot, tip
}
