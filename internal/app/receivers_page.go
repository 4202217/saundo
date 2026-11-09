package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// The 接收器 settings page: the merged receiver list per mockup-final, with
// per-receiver auto-connect, visibility, delay, and password, and the add,
// edit, and password dialogs. The engine owns the registry; the page sends
// commands and renders the snapshot's receiver views.

type receiverDialogKind uint8

const (
	receiverDialogNone receiverDialogKind = iota
	receiverDialogAdd
	receiverDialogEdit
	receiverDialogPassword
	receiverDialogRemove
)

// receiverDraft is the add/edit dialog's fields. The password never lives
// in the registry; the engine moves it to the DPAPI store.
type receiverDraft struct {
	name             string
	host             string
	port             string
	typeIdx          int
	typeLabel        string
	authLabel        string
	wantAuth         bool
	hasSavedPassword bool
	password         string
	errorMsg         string
}

// syncAuthLabel keeps the auth select's value on wantAuth when the dialog
// opens before the user touches the select.
func (d *receiverDraft) syncAuthLabel() {
	if d.authLabel == "" {
		if d.wantAuth {
			d.authLabel = locales.ReceiverAuthPassword()
		} else {
			d.authLabel = locales.ReceiverAuthNone()
		}
	}
}

// typeLabelInit names the type select's initial value.
func (d *receiverDraft) typeLabelInit() {
	if d.typeLabel == "" {
		labels := receiverTypeLabels()
		if d.typeIdx < len(labels) {
			d.typeLabel = labels[d.typeIdx]
		}
	}
}

var receiverTypes = []receivers.Type{
	receivers.TypeAirPortExpress,
	receivers.TypeAirPlay1,
	receivers.TypeAirPlay2,
}

func receiverTypeLabels() []string {
	return []string{
		locales.ReceiverTypeExpress(),
		locales.ReceiverTypeAP1(),
		locales.ReceiverTypeAP2(),
	}
}

// receiverDialogState is one open dialog over the settings window.
type receiverDialogState struct {
	kind receiverDialogKind
	key  string
	// entered drives the one-shot panel entrance pose.
	entered bool
}

func (a *App) receiversPage(c *ui.Context, p palette) {
	var visible, hidden []receivers.View
	for _, v := range a.snap.Receivers {
		if v.Hidden {
			hidden = append(hidden, v)
		} else {
			visible = append(visible, v)
		}
	}
	// Collapsing the group also collapses every row inside it, so reopening
	// starts from a clean list rather than restoring stale expanded panels.
	if !a.receiverHiddenOpen {
		for _, v := range hidden {
			delete(a.receiverOpen, v.Key)
		}
	}

	// The 已启用 group head renders only when the list has rows, matching the
	// hidden group's own non-empty rule. An empty page is plain prose where
	// the list would be; the add row below keeps its margin in every state.
	if len(visible) > 0 {
		ui.Row(c).Margin(6, 0, 0, 0).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ReceiverGroupEnabled()).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted)
			countBadge(c, p, len(visible))
		})

		card := ui.Column(c).Margin(8, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip().Shrink(0)
		card.Children(func() {
			for i, v := range visible {
				a.receiverRow(c, p, v, false, i < len(visible)-1)
			}
		})
	} else if len(hidden) > 0 {
		ui.Column(c).Margin(2, 0, 0, 0).Shrink(0).Children(func() {
			emptyCopy(c, p, locales.ReceiverEmptyAllHiddenTitle(), locales.ReceiverEmptyHiddenHint(), false)
		})
	} else {
		ui.Column(c).Margin(2, 0, 0, 0).Shrink(0).Children(func() {
			emptyCopy(c, p, locales.ReceiverEmptyTitle(), locales.ReceiverDiscoveryHint(), false)
		})
	}

	a.receiverAddRow(c, p)

	if len(hidden) > 0 {
		ui.Column(c).Margin(12, 0, 0, 0).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(p.line2).Shrink(0).Children(func() {
			parts := ui.CollapsibleBase(c, &a.receiverHiddenOpen)
			trigger := parts.Trigger.Height(28).
				AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).
				Label(locales.ReceiverGroupHidden())
			trigger.Children(func() {
				// The hover pill wraps the chevron, label, and count, not the
				// full-width trigger, so the empty right side stays flat.
				cluster := ui.Row(c).Height(28).Radius(7).Padding(0, 8, 0, 4).Gap(6).
					AlignItems(ui.Center).Shrink(0)
				if trigger.Hovered() {
					cluster.Background(p.pill)
				}
				cluster.Children(func() {
					chev := ui.Icon(c, iconChevron).Size(12, 12).TextColor(p.muted)
					chev.Rotate((1 - parts.Progress()) * -90)
					ui.Text(c, locales.ReceiverGroupHidden()).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted)
					countBadge(c, p, len(hidden))
				})
			})
			if panel := parts.Panel(func() {
				list := ui.Column(c).Margin(8, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip().Shrink(0)
				list.Children(func() {
					for i, v := range hidden {
						a.receiverRow(c, p, v, true, i < len(hidden)-1)
					}
				})
			}); panel.Valid() {
				panel.Margin(0, 0, 4, 0)
			}
		})
	}

	a.receiverDialogs(c, p)
}

// receiverAddRow is the dashed manual-add affordance under the card.
func (a *App) receiverAddRow(c *ui.Context, p palette) {
	row := ui.ButtonBase(c).Margin(12, 0, 0, 0).MinHeight(44).Padding(9, 14).Gap(10).Shrink(0).
		Radius(12).Cursor(ui.CursorPointer).Justify(ui.Start).AlignItems(ui.Center).
		Label(locales.ReceiverAddTitle()).Border(1, p.line.Mix(p.muted, 0.2)).
		BorderStyle(ui.BorderDashed)
	row.Background(ui.Transparent)
	hovered := row.Hovered()
	if hovered {
		row.Background(p.pill)
	}
	row.OnClick(func() {
		a.openAddReceiver()
	})
	row.Children(func() {
		tile := ui.Box(c).Size(24, 24).Radius(12).Center().Shrink(0).Background(p.accentSoft)
		if hovered {
			tile.Background(p.accentSoft.Mix(p.accent, 0.2))
		}
		tile.Children(func() {
			ui.Icon(c, iconPlus).Size(12, 12).TextColor(p.accent)
		})
		ui.Column(c).Gap(1).Grow(1).Children(func() {
			ui.Text(c, locales.ReceiverAddTitle()).FontSize(12.5).FontWeight(500).TextColor(p.accent)
			ui.Text(c, locales.ReceiverAddSub()).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
	})
}

// receiverRow is one card row: glyph, name with chips, enable switch,
// chevron, and the expanded config. Hidden rows grey their parts.
func (a *App) receiverRow(c *ui.Context, p palette, v receivers.View, hiddenGroup, divider bool) {
	open := a.receiverOpen[v.Key]
	row := ui.Column(c).Key(v.Key).Shrink(0).
		Transition(ui.ElementTransition{Colors: true, Duration: 140 * time.Millisecond})
	if divider {
		row.BorderWidth(0, 0, 1, 0).BorderColor(p.line2)
	}
	row.Children(func() {
		a.receiverRowHead(c, p, v, open, hiddenGroup)
		if open {
			a.receiverRowBody(c, p, v, hiddenGroup)
		}
	})
}

// receiverRowHead is the card's clickable head: glyph, name column, edit
// button (expanded manual rows), the enable switch, and the chevron.
func (a *App) receiverRowHead(c *ui.Context, p palette, v receivers.View, open, hiddenGroup bool) {
	head := ui.ButtonBase(c.Key("head:"+v.Key)).MinHeight(56).Padding(9, 14).Gap(12).
		AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).
		Label(v.Name).FillWidth()
	switch {
	case open && !hiddenGroup:
		head.Background(p.sel)
	case open && hiddenGroup:
		head.Background(p.pill)
	case head.Hovered():
		head.Background(p.cardHover)
	}
	head.OnClick(func() {
		if a.receiverOpen == nil {
			a.receiverOpen = map[string]bool{}
		}
		a.receiverOpen[v.Key] = !a.receiverOpen[v.Key]
	})
	head.Children(func() {
		a.receiverGlyph(c, p, v, open, hiddenGroup)
		a.receiverNameCol(c, p, v, open)
		ui.Row(c).Gap(8).Shrink(0).AlignItems(ui.Center).Children(func() {
			shown := !v.Hidden
			toggle(c, p, &shown, locales.ReceiverEnableSwitch(), func(next bool) {
				a.eng.SetReceiverHidden(v.Key, !next)
				if !next {
					a.receiverHiddenOpen = true
				}
			})
			chev := ui.Icon(c, iconChevron).Size(12, 12).TextColor(p.muted)
			if open {
				chev.TextColor(p.fg2)
			} else {
				chev.Rotate(-90)
			}
		})
		// The edit action overlays the gap before the fixed row tail, so the
		// enable switch keeps one x across every row, as the mockup pins it.
		if v.IsManual() && open {
			a.receiverEditButton(c, p, v, hiddenGroup)
		}
	})
}

// receiverRowBody is the expanded config: auto-connect, delay, password.
func (a *App) receiverRowBody(c *ui.Context, p palette, v receivers.View, hiddenGroup bool) {
	titleColor, subColor := p.fg, p.muted
	if hiddenGroup {
		titleColor, subColor = p.muted, p.faint
	}
	body := ui.Column(c).Padding(0, 14, 4, 14).BorderWidth(1, 0, 0, 0).BorderColor(p.line2).Shrink(0)
	body.Children(func() {
		auto := v.AutoConnect
		settingRowColors(c, p, locales.ReceiverAutoToggle(), locales.ReceiverAutoToggleDesc(),
			titleColor, subColor, func() {
				toggle(c, p, &auto, locales.ReceiverAutoToggle(), func(next bool) {
					a.eng.SetReceiverAutoConnect(v.Key, next)
				})
			})
		rowLine(c, p)
		a.receiverDelayRow(c, p, v, hiddenGroup)
		if receiverPasswordEditable(v) {
			rowLine(c, p)
			a.receiverPasswordRow(c, p, v, hiddenGroup)
		}
	})
}

// receiverGlyph is the round silhouette with the online dot.
func (a *App) receiverGlyph(c *ui.Context, p palette, v receivers.View, open, hiddenGroup bool) {
	g := ui.Box(c).Size(30, 30).Radius(15).Center().Shrink(0).Background(p.pill)
	if hiddenGroup {
		g.Background(p.bg)
	} else if open {
		g.Background(p.card)
	}
	g.Children(func() {
		color := p.fg2
		if hiddenGroup {
			color = p.faint
		}
		ui.Icon(c, deviceIcon(v.Model)).Size(16, 16).TextColor(color)
		if v.Online && !hiddenGroup {
			ring := p.card
			if open {
				ring = p.sel
			}
			ui.Box(c).Size(9, 9).Radius(4.5).Background(p.green).
				Shadow(0, 0, 0, 2, ring).
				Attach(ui.AnchorBottomRight, ui.AnchorBottomRight).Right(-1).Bottom(-1)
		}
	})
}

func (a *App) receiverNameCol(c *ui.Context, p palette, v receivers.View, open bool) {
	col := ui.Column(c).Grow(1).Gap(2)
	if v.IsManual() && open {
		// The attached edit button overlays this reserved gap.
		col.Padding(0, 58, 0, 0)
	}
	col.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			nameColor := p.fg
			if v.Hidden {
				nameColor = p.fg2
			}
			ui.Text(c, v.Name).FontSize(13).FontWeight(500).TextColor(nameColor).
				SingleLine().Ellipsis("…").Shrink(1)
			if v.IsManual() {
				inlineChip(c, p, locales.ReceiverChipManual(), chipNeutral, v.Hidden)
			}
			if v.AutoConnect {
				inlineChip(c, p, locales.ReceiverChipAuto(), chipOK, v.Hidden)
			}
			if v.Hidden {
				inlineChip(c, p, locales.ReceiverChipHidden(), chipNeutral, true)
			}
		})
		sub := receiverSubtitle(v)
		ui.Text(c, sub).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
	})
}

// receiverSubtitle names the model and either its address or its absence.
// The mockup keeps the address visible on a row, which the auth-oriented
// flyout subtitle cannot, so the receiver page has its own line.
func receiverSubtitle(v receivers.View) string {
	model := receiverModelLabel(v)
	if !v.Device.Online {
		return model + " · " + locales.ActionOffline()
	}
	if v.Stereo {
		return locales.DeviceStereoGroup(model, len(v.Device.Members))
	}
	address := v.Address
	if address == "" && v.Device.Host != "" {
		address = v.Device.Host + ":" + receivers.PortText(v.Device.Port)
	}
	if address == "" {
		return model
	}
	return model + " · " + address
}

// receiverModelLabel prefers the localized manual type name over the
// internal fallback model a manual row carries.
func receiverModelLabel(v receivers.View) string {
	if v.IsManual() {
		switch v.Type {
		case receivers.TypeAirPortExpress:
			return locales.ReceiverTypeExpress()
		case receivers.TypeAirPlay1:
			return locales.ReceiverTypeAP1()
		case receivers.TypeAirPlay2:
			return locales.ReceiverTypeAP2()
		}
	}
	if v.Model != "" {
		return v.Model
	}
	return locales.DeviceDefaultModel()
}

// receiverEditButton is the side button on an expanded manual row. It is
// attached to the head instead of laid out in the tail so the switch and
// chevron do not move between rows.
func (a *App) receiverEditButton(c *ui.Context, p palette, v receivers.View, hiddenGroup bool) {
	color := p.accent
	if hiddenGroup {
		color = p.muted
	}
	b := rowAction(c, p, "edit:"+v.Key, locales.ReceiverEditButton(), iconPencil, color, func() {
		a.beginEditReceiver(v)
	})
	b.Attach(ui.AnchorRight, ui.AnchorRight).Right(receiverTailInset)
}

// receiverTailInset clears the fixed tail (32 switch + 8 gap + 12 chevron),
// the head's 11 gap, and 6 more before the edit button.
const receiverTailInset = 32 + 8 + 12 + 11 + 6

// receiverDelayRow is the DSP/DAC slider with the 0/2000 scale and the
// value on the subtitle line.
func (a *App) receiverDelayRow(c *ui.Context, p palette, v receivers.View, hiddenGroup bool) {
	delay := v.DelayMS
	if draft, ok := a.receiverDelayPend[v.Key]; ok {
		delay = draft
	}
	valueColor := p.fg2
	if hiddenGroup {
		valueColor = p.muted
	}
	ui.Column(c).Padding(8, 14).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				title := p.fg
				sub := p.muted
				if hiddenGroup {
					title, sub = p.muted, p.faint
				}
				ui.Text(c, locales.ReceiverDelayTitle()).FontSize(12.5).FontWeight(500).TextColor(title)
				ui.Text(c, locales.ReceiverDelayDesc()).FontSize(11.5).TextColor(sub)
			})
			ui.Text(c, settings.LatencyText(delay)).FontSize(11.5).TextColor(valueColor)
		})
		value := float64(delay)
		sl := slider(c, p, &value, 0, receivers.MaxDelayMS, locales.ReceiverDelayTitle(), func(next float64) {
			ms := receiverDelayValue(next)
			if ms == delay {
				return
			}
			a.scheduleReceiverDelay(v.Key, ms)
		}).Step(10)
		if hiddenGroup {
			sl.Opacity(0.55)
		}
		ui.Row(c).Margin(2, 0, 0, 0).Children(func() {
			ui.Text(c, "0 ms").FontSize(10.5).TextColor(p.faint)
			ui.Spacer(c)
			ui.Text(c, "2 s").FontSize(10.5).TextColor(p.faint)
		})
	})
}

// receiverDelayValue quantizes a slider position to the mockup's 10 ms
// steps and the sender's 0..2000 ms window.
func receiverDelayValue(v float64) int {
	ms := int(v + 0.5)
	if ms < 0 {
		ms = 0
	}
	if ms > receivers.MaxDelayMS {
		ms = receivers.MaxDelayMS
	}
	return (ms + 5) / 10 * 10
}

// scheduleReceiverDelay debounces the slider like the global latency apply:
// one restart with the final value once the user stops dragging. The draft
// stays in receiverDelayPend until the engine's row carries it.
func (a *App) scheduleReceiverDelay(key string, ms int) {
	if a.receiverDelayPend == nil {
		a.receiverDelayPend = map[string]int{}
	}
	a.receiverDelayPend[key] = ms
	a.receiverDelaySeq++
	seq := a.receiverDelaySeq
	if a.receiverDelayTimer != nil {
		a.receiverDelayTimer.Stop()
	}
	a.receiverDelayTimer = time.AfterFunc(500*time.Millisecond, func() {
		a.updateUI(func() {
			if seq != a.receiverDelaySeq {
				return
			}
			pending := make(map[string]int, len(a.receiverDelayPend))
			for k, v := range a.receiverDelayPend {
				pending[k] = v
			}
			// Only a change to the receiver the engine is working with
			// restarts the session; that restart is what the hold masks.
			if _, ok := pending[a.snap.SelectedID]; ok && phaseOf(a.snap) != phaseIdle {
				a.beginLatencyApply()
			}
			for k, v := range pending {
				a.eng.SetReceiverDelay(k, v)
			}
		})
	})
}

// receiverPasswordEditable reports whether the receiver can use a user-set
// password. A HomePod or Apple TV pairs with a PIN, not a receiver password;
// a manual row always offers the control so the user can set or clear one. A
// saved password also keeps the row reachable so it can be cleared.
func receiverPasswordEditable(v receivers.View) bool {
	if v.Kind == receivers.KindManual || v.PasswordSet {
		return true
	}
	family := v.Device.Family
	if family == device.FamilyUnknown {
		family = device.FamilyFor(v.Model, v.Device.AM)
	}
	switch family {
	case device.FamilyHomePod, device.FamilyAppleTV:
		return false
	}
	return v.Device.Auth == device.AuthHAPPassword || v.Device.Auth == device.AuthPassword
}

func (a *App) receiverPasswordRow(c *ui.Context, p palette, v receivers.View, hiddenGroup bool) {
	controls := func() {
		ui.Row(c).Gap(8).Shrink(0).AlignItems(ui.Center).Children(func() {
			if v.PasswordSet {
				ui.Box(c).MaxWidth(140).Height(22).Radius(6).Padding(0, 8).Center().
					Background(p.pill).Children(func() {
					ui.Text(c, "••••••").FontSize(11.5).TextColor(p.fg2)
				})
			}
			color := p.accent
			if hiddenGroup {
				color = p.muted
			}
			label := locales.ReceiverPasswordSet()
			if v.PasswordSet {
				label = locales.ReceiverPasswordChange()
			}
			rowAction(c, p, "pw:"+v.Key, label, iconKey, color, func() {
				a.receiverDialog = receiverDialogState{kind: receiverDialogPassword, key: v.Key, entered: false}
				a.receiverDialogOpen = true
				a.receiverDraft = receiverDraft{}
			})
		})
	}
	sub := locales.ReceiverPasswordUnset()
	if v.PasswordSet {
		sub = locales.ReceiverPasswordSaved()
	}
	titleColor, subColor := p.fg, p.muted
	if hiddenGroup {
		titleColor, subColor = p.muted, p.faint
	}
	settingRowColors(c, p, locales.ReceiverPasswordTitle(), sub, titleColor, subColor, controls)
}
