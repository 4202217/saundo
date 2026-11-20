package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
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
	receiverDialogComboCreate
	receiverDialogComboUnlink
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

// receiverDialogState is one open dialog over the settings window. The combo
// create dialog keeps its two slots and the name field here, so one dialog
// shell serves every kind.
type receiverDialogState struct {
	kind receiverDialogKind
	key  string
	// entered drives the one-shot panel entrance pose.
	entered bool
	// comboLeft and comboRight are the two slot keys; comboName is the
	// suggestion the user may edit, and comboError its validation line.
	comboLeft  string
	comboRight string
	comboName  string
	comboError string
}

func (a *App) receiversPage(c *ui.Context, p palette) {
	// A save or link refusal lands here: the dialogs close on submit and the
	// flyout's error bar is hidden while the settings window is open.
	if settingsErrorKey(a.snap.ErrorKey) {
		ui.Column(c).Margin(6, 0, 8, 0).Children(func() {
			statusBar(c, p, barDanger, iconAlert,
				userErrorText(a.snap.ErrorKey, a.snap.ErrorArgs, a.snap.Error), nil)
		})
	}
	var visible, hidden, singles, combos []receivers.View
	for _, v := range a.snap.Receivers {
		if v.Hidden {
			hidden = append(hidden, v)
			continue
		}
		visible = append(visible, v)
		if v.Entry.IsCombo() {
			combos = append(combos, v)
		} else {
			singles = append(singles, v)
		}
	}
	// Collapsing the group also collapses every row inside it, so reopening
	// starts from a clean list rather than restoring stale expanded panels.
	if !a.receiverHiddenOpen {
		for _, v := range hidden {
			delete(a.receiverOpen, v.Key)
		}
	}

	// One section per type. A section's create action rides its head while the
	// section has rows; an empty section replaces it with its dashed row. The
	// combo section needs two eligible receivers to offer creation at all: with
	// fewer, the dialog could never commit. It still renders when a combo
	// exists, so its rows stay reachable.
	candidates := len(a.comboCandidates())
	canCreateCombo := candidates >= 2
	a.receiverSectionHead(c, p, locales.ReceiverGroupEnabled(), len(singles), 6, len(singles) > 0,
		locales.ReceiverAddAction(), func() { a.openAddReceiver() })
	if len(singles) > 0 {
		card := ui.Column(c).Margin(8, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip().Shrink(0)
		card.Children(func() {
			for i, v := range singles {
				a.receiverRow(c, p, v, false, i < len(singles)-1)
			}
		})
	} else {
		a.receiverAddRow(c, p)
	}
	if len(combos) > 0 || (len(visible) > 0 && canCreateCombo) {
		a.receiverSectionHead(c, p, locales.ReceiverGroupCombo(), len(combos), 12, len(combos) > 0 && canCreateCombo,
			locales.ComboCreateAction(), func() { a.openComboCreate() })
		if len(combos) > 0 {
			card := ui.Column(c).Margin(8, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip().Shrink(0)
			card.Children(func() {
				for i, v := range combos {
					a.receiverComboRow(c, p, v, false, i < len(combos)-1)
				}
			})
		} else {
			a.receiverPairAddRow(c, p)
		}
	}

	if len(hidden) > 0 {
		ui.Column(c).Margin(12, 0, 0, 0).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(p.line2).Shrink(0).Children(func() {
			parts := ui.CollapsibleBase(c.Key("receivers-hidden"), &a.receiverHiddenOpen)
			trigger := parts.Trigger.Height(28).
				AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).
				Label(locales.ReceiverGroupHidden())
			trigger.Children(func() {
				// The hover pill wraps the chevron, label, and count, not the
				// full-width trigger, so the empty right side stays flat.
				cluster := ui.Row(c).Height(28).Radius(7).Padding(0, 8, 0, 4).Gap(6).
					AlignItems(ui.Center).Shrink(0)
				cluster.Draw(func(pt *ui.Painter, r ui.Rect) {
					if trigger.Hovered() {
						pt.Fill(r, p.pill, 7)
					}
				})
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

// receiverSectionHead is one list section's label with its count and, while
// the section has rows, its create action at the right. A section without
// rows leaves the action to its dashed row, so no head offers an action twice.
func (a *App) receiverSectionHead(c *ui.Context, p palette, label string, count int, topMargin float32, withAction bool, actionLabel string, fn func()) {
	var action func()
	if withAction {
		action = func() {
			rowAction(c, p, "section:"+label, actionLabel, iconPlus, p.accent, fn)
		}
	}
	groupHead(c, p, label, count, topMargin, action)
}

// openComboCreate opens the create-combo dialog over the page.
func (a *App) openComboCreate() {
	a.receiverDialog = receiverDialogState{kind: receiverDialogComboCreate, entered: false}
	a.receiverDialogOpen = true
}

// receiverAddRow is the dashed manual-add affordance under the card.
func (a *App) receiverAddRow(c *ui.Context, p palette) {
	row := ui.ButtonBase(c).Margin(12, 0, 0, 0).MinHeight(44).Padding(9, 14).Gap(10).Shrink(0).
		Radius(12).Cursor(ui.CursorPointer).Justify(ui.Start).AlignItems(ui.Center).
		Label(locales.ReceiverAddTitle()).Border(1, p.line.Mix(p.muted, 0.2)).
		BorderStyle(ui.BorderDashed)
	row.Background(ui.Transparent)
	hoverFill(row, p.pill, 11, 1)
	row.OnClick(func() {
		a.openAddReceiver()
	})
	row.Children(func() {
		tile := ui.Box(c).Size(24, 24).Radius(12).Center().Shrink(0).Background(p.accentSoft)
		tile.Draw(func(pt *ui.Painter, r ui.Rect) {
			if row.Hovered() {
				pt.Fill(r, p.accentSoft.Mix(p.accent, 0.2), 12)
			}
		})
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
// chevron, and the expanded config. Hidden rows grey their parts. A combo
// row owns its own renderer.
func (a *App) receiverRow(c *ui.Context, p palette, v receivers.View, hiddenGroup, divider bool) {
	if v.Entry.IsCombo() {
		a.receiverComboRow(c, p, v, hiddenGroup, divider)
		return
	}
	open := !v.Local && a.receiverOpen[v.Key]
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
	// The local receiver is not a target: its row is a label, with no
	// switch and no expander, so it cannot be enabled or configured.
	if v.Local {
		ui.Row(c.Key("head:"+v.Key)).MinHeight(56).Padding(9, 14).Gap(12).
			AlignItems(ui.Center).Justify(ui.Start).FillWidth().Children(func() {
			a.receiverGlyph(c, p, v, false, hiddenGroup)
			a.receiverNameCol(c, p, v, false)
		})
		return
	}
	head := ui.ButtonBase(c.Key("head:"+v.Key)).MinHeight(56).Padding(9, 14).Gap(12).
		AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).
		Label(v.Name).FillWidth()
	switch {
	case open && !hiddenGroup:
		head.Background(p.sel)
	case open && hiddenGroup:
		head.Background(p.pill)
	default:
		hoverFill(head, p.cardHover, 0, 0)
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

// receiverRowBody is the expanded config: auto-connect, volume, channel,
// password.
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
		a.receiverAudioRows(c, p, v, hiddenGroup)
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
		ui.Icon(c, stereoIcon(v.Device, v.Stereo || v.Device.IsStereoGroup())).Size(16, 16).TextColor(color)
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
			if v.Local {
				inlineChip(c, p, locales.ReceiverChipLocal(), chipNeutral, v.Hidden)
			}
			if v.IsManual() {
				inlineChip(c, p, locales.ReceiverChipManual(), chipNeutral, v.Hidden)
			}
			if v.Entry.IsCombo() {
				inlineChip(c, p, locales.ComboTag(), chipNeutral, v.Hidden)
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

// receiverSubtitle is the row's second line: the one fact its title and
// chips do not already carry. A combo names its sides or their online count;
// every other row is offline, an auth gate, a stereo pair's shape, a manual
// row's address, or nothing at all.
func receiverSubtitle(v receivers.View) string {
	if v.Entry.IsCombo() {
		return comboSubtitle(v.Name, v.Device.Members)
	}
	if !v.Device.Online {
		return locales.ActionOffline()
	}
	if line := authSubtitle(v.Device.Auth, v.PasswordSet); line != "" {
		return line
	}
	if v.Stereo {
		return locales.DeviceStereoGroup(len(v.Device.Members))
	}
	if v.IsManual() {
		if v.Address != "" {
			return v.Address
		}
		if v.Device.Host != "" {
			return v.Device.Host + ":" + receivers.PortText(v.Device.Port)
		}
	}
	return ""
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

// receiverAudioRows is the per-receiver audio group: the volume row and the
// channel row, separated by the page's hairline like every other settings row.
// A stereo pair always plays stereo, so its segment is a disabled 立体声.
func (a *App) receiverAudioRows(c *ui.Context, p palette, v receivers.View, hiddenGroup bool) {
	titleColor, subColor := p.fg, p.muted
	if hiddenGroup {
		titleColor, subColor = p.muted, p.faint
	}
	a.receiverVolumeRow(c, p, v, titleColor, subColor, hiddenGroup)
	rowLine(c, p)
	desc := ""
	if !v.Stereo {
		desc = locales.ReceiverChannelDesc()
	}
	settingRowColors(c, p, locales.ChannelLabel(), desc, titleColor, subColor, func() {
		a.receiverChannelSegment(c, p, v, hiddenGroup)
	})
}

// receiverVolumeRow is the volume line: the row title and its percent on the
// title line, the slider under them at the row's own inset.
func (a *App) receiverVolumeRow(c *ui.Context, p palette, v receivers.View, titleColor, subColor ui.Color, hiddenGroup bool) {
	// The slider writes into the snapshot's row, so a drag survives the view
	// rebuild between engine commands.
	volume := v.Volume
	volumeValue := &volume
	for i := range a.snap.Receivers {
		if a.snap.Receivers[i].Key == v.Key {
			volumeValue = &a.snap.Receivers[i].Volume
			break
		}
	}
	ui.Column(c).Padding(8, 14).Gap(2).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.VolumeLabel()).FontSize(13).FontWeight(500).TextColor(titleColor).
				Grow(1).SingleLine().Ellipsis("…")
			ui.Text(c, locales.FlyoutVolumeValue(*volumeValue)).FontSize(12.5).TextColor(subColor)
		})
		sl := slider(c, p, volumeValue, 0, 100, locales.VolumeFor(v.Name), func(next float64) {
			if a.eng != nil {
				a.eng.SetTargetVolume(v.Key, next)
			}
		})
		if hiddenGroup {
			sl.Opacity(0.55)
		}
	})
}

// receiverChannelSegment is the stereo/left/right control. A pair shows only
// stereo and cannot change it.
func (a *App) receiverChannelSegment(c *ui.Context, p palette, v receivers.View, hiddenGroup bool) {
	if v.Stereo {
		idx := 0
		segmented(c, p, &idx, []segmentItem{{label: locales.ChannelStereo()}}, 120, true, segmentCenter, locales.ChannelLabel()+" · "+v.Name)
		return
	}
	idx := 0
	switch v.Channel {
	case receivers.ChannelLeft:
		idx = 1
	case receivers.ChannelRight:
		idx = 2
	}
	items := []segmentItem{
		{label: locales.ChannelStereo()},
		{label: locales.ChannelLeft()},
		{label: locales.ChannelRight()},
	}
	track := segmented(c, p, &idx, items, 200, hiddenGroup, segmentCenter, locales.ChannelLabel()+" · "+v.Name)
	if track.Changed() && a.eng != nil {
		mode := receivers.ChannelStereo
		switch idx {
		case 1:
			mode = receivers.ChannelLeft
		case 2:
			mode = receivers.ChannelRight
		}
		a.eng.UpdateReceiver(v.Key, receivers.Patch{Channel: &mode})
	}
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
