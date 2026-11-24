package app

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
)

// The measured card: four live numbers instead of a verdict. Latency answers
// whether the setting is safe; headroom, underruns, and traffic answer whether
// the send side is healthy now. Retransmits, drops, and reconnects are
// exception counters, so they join one footer row only when non-zero.

func (a *App) connectionSessionCard(c *ui.Context, p palette) {
	floor := a.sessionLatencyFloor()
	low := floor > 0 && a.snap.LatencyMS > 0 && a.snap.LatencyMS < floor
	card := ui.Column(c).Margin(8, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip().Shrink(0)
	card.Children(func() {
		if low {
			a.connectionLatencyBanner(c, p, floor)
		}
		ui.Row(c).Padding(12, 14).Gap(18).Children(func() {
			a.connectionLatencyCell(c, p, floor, low)
			a.connectionHeadroomCell(c, p)
		})
		rowLine(c, p)
		ui.Row(c).Padding(12, 14).Gap(18).Children(func() {
			a.connectionUnderrunCell(c, p)
			a.connectionTrafficCell(c, p)
		})
		a.connectionExceptionRow(c, p)
		a.connectionStrip(c, p)
	})
}

func (a *App) connectionLatencyBanner(c *ui.Context, p palette, floor int) {
	ui.Column(c).FillWidth().Background(p.warningSoft).
		BorderWidth(0, 0, 1, 0).BorderColor(p.line2).Children(func() {
		ui.Row(c).Padding(10, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconAlert).Size(14, 14).TextColor(p.warning).Shrink(0)
			ui.Text(c, locales.SettingsConnectionLatencyWarn(a.snap.LatencyMS, floor)).
				FontSize(11.5).TextColor(p.fg2).Grow(1)
		})
	})
}

func (a *App) connectionLatencyCell(c *ui.Context, p palette, floor int, low bool) {
	note := locales.SettingsConnectionFloor(floor)
	tone, noteTone := p.fg, p.faint
	if low {
		note = locales.SettingsConnectionBelowFloor(floor)
		tone, noteTone = p.warning, p.warning
	}
	connectionCell(c, p, locales.SettingsConnectionLatency(),
		fmt.Sprintf("%d ms", a.snap.LatencyMS), note, tone, noteTone)
}

func (a *App) connectionHeadroomCell(c *ui.Context, p palette) {
	diag := a.snap.Diagnostics
	value, note := "—", ""
	tone, noteTone := p.fg, p.faint
	if a.snap.AudioIdle {
		note = locales.SettingsConnectionHeadroomIdle()
	} else {
		if diag.BufferValid {
			value = fmt.Sprintf("%d ms", int(math.Round(diag.SendBufferMS)))
		}
		if diag.BufferMinValid {
			min := int(math.Round(diag.BufferMinMS))
			if diag.GroupSize > 1 {
				note = locales.SettingsConnectionHeadroomMinWorst(min)
			} else {
				note = locales.SettingsConnectionHeadroomMin(min)
			}
			if min == 0 {
				noteTone = p.warning
			}
		}
	}
	connectionCell(c, p, locales.SettingsConnectionHeadroom(), value, note, tone, noteTone)
}

func (a *App) connectionUnderrunCell(c *ui.Context, p palette) {
	n := int(a.snap.Diagnostics.RecentUnderruns)
	tone := p.fg
	if n > 0 {
		tone = p.warning
	}
	connectionCell(c, p, locales.SettingsConnectionUnderrun(), fmt.Sprintf("%d", n),
		a.connectionUnderrunNote(), tone, p.faint)
}

// connectionUnderrunNote splits the window's count by the combo's two sides
// when the session has exactly a left and a right target; otherwise it names
// the window alone.
func (a *App) connectionUnderrunNote() string {
	members := a.sessionMembers()
	if len(members) == 2 {
		var left, right *sessionMember
		for i := range members {
			switch receivers.ParseChannel(members[i].Target.Channel) {
			case receivers.ChannelLeft:
				left = &members[i]
			case receivers.ChannelRight:
				right = &members[i]
			}
		}
		if left != nil && right != nil {
			return locales.SettingsConnectionUnderrunSplit(locales.ChannelLeft(),
				int(left.Target.Health.RecentUnderruns), locales.ChannelRight(),
				int(right.Target.Health.RecentUnderruns))
		}
	}
	return locales.SettingsConnectionUnderrunWindow()
}

func (a *App) connectionTrafficCell(c *ui.Context, p palette) {
	diag := a.snap.Diagnostics
	value := "—"
	if diag.TrafficValid {
		value = connectionTrafficText(diag.TrafficBPS)
	}
	note := ""
	if diag.PacketBytes > 0 {
		note = locales.SettingsConnectionPacketNote(connectionPacketText(diag.PacketBytes))
	}
	connectionCell(c, p, locales.SettingsConnectionTraffic(), value, note, p.fg, p.faint)
}

// connectionExceptionRow carries the counters that only matter when they are
// not zero. They are session totals, so the row names the session span.
func (a *App) connectionExceptionRow(c *ui.Context, p palette) {
	diag := a.snap.Diagnostics
	var items []string
	var warn []bool
	if diag.Retransmits > 0 {
		if rate, ok := retransmitRate(diag.Retransmits, diag.SentBlocks); ok {
			items = append(items, locales.SettingsConnectionRetransmit()+" "+fmt.Sprintf("%.1f%%", rate))
			warn = append(warn, true)
		}
	}
	if diag.RecentDropped > 0 {
		items = append(items, locales.SettingsConnectionDropped(int(diag.RecentDropped)))
		warn = append(warn, true)
	}
	if diag.Reconnects > 0 {
		items = append(items, locales.SettingsConnectionReconnects()+" "+
			locales.SettingsReconnectsValue(diag.Reconnects))
		warn = append(warn, false)
	}
	if len(items) == 0 {
		return
	}
	ui.Column(c).Children(func() {
		rowLine(c, p)
		ui.Row(c).Padding(9, 14).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SettingsConnectionThis()).FontSize(11.5).TextColor(p.faint).SingleLine()
			for i, item := range items {
				if i > 0 {
					ui.Text(c, "·").FontSize(11.5).TextColor(p.faint).SingleLine()
				}
				color := p.muted
				if warn[i] {
					color = p.warning
				}
				ui.Text(c, item).FontSize(11.5).TextColor(color).SingleLine()
			}
		})
	})
}

func connectionCell(c *ui.Context, p palette, label, value, note string, tone, noteTone ui.Color) {
	ui.Column(c).Grow(1).Gap(2).Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(p.muted).SingleLine().Shrink(0)
			if note != "" {
				ui.Text(c, " · "+note).FontSize(10.5).TextColor(noteTone).
					SingleLine().Ellipsis("…").Shrink(1)
			}
		})
		ui.Text(c, value).FontSize(15).FontWeight(600).TextColor(tone).
			FontFeatures("tnum").SingleLine()
	})
}

// receiverLatencyFloorMS is the measured presentation floor per receiver
// family. HomePod: 100 ms severe, 160 frequent, 180 mild, 200 clean
// (docs/verification.md, 2026-11-10). Mac receivers are known only as worse
// than 250 ms. An unknown family has no floor: the page must not invent one.
func receiverLatencyFloorMS(d device.Device) int {
	family := d.Family
	if family == device.FamilyUnknown && d.Model != "" {
		family = device.FamilyFor(d.Model, d.AM)
	}
	switch family {
	case device.FamilyHomePod:
		return 200
	case device.FamilyMac:
		return 250
	}
	return 0
}

// sessionLatencyFloor is the strictest known floor among the session's
// targets; zero when no family has a measured one.
func (a *App) sessionLatencyFloor() int {
	floor := 0
	for _, m := range a.sessionMembers() {
		if f := receiverLatencyFloorMS(m.Device); f > floor {
			floor = f
		}
	}
	return floor
}
