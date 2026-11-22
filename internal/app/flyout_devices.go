package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/engine"
	"saundo/internal/receivers"
)

// deviceActionWidth is the row's trailing column: wide enough for the longest
// action (断开连接 or 重新连接 with its glyph), fixed across phases.
const deviceActionWidth = 92

// The flyout's device card: one row per receiver, at one height whether or not
// the row is a session member. A member adds the device's own volume and a
// channel button; every other row keeps a plain second line.

func (a *App) deviceSection(c *ui.Context, p palette) {
	devices := a.snap.Devices
	// The head keeps the 26-DIP action slot whether or not a session gives it
	// the 全部断开 button, so starting or ending a session never moves the
	// panel. The empty slot draws nothing and takes no width.
	action := func() {
		if a.disconnectAllShown() {
			rowAction(c, p, "disconnect-all", locales.DisconnectAll(), iconUnlink, p.fg2, func() { a.disconnect() })
			return
		}
		ui.Row(c).Width(0).Height(26)
	}
	// The head and card share one child column, like the ambient section, so
	// the head-to-card gap is the card's own margin rather than the body's
	// section gap. This keeps all three flyout heads equally close to their
	// cards.
	ui.Column(c).Children(func() {
		sectionHead(c, p, locales.FlyoutDevices(), locales.FlyoutDeviceCount(len(devices)), false, action)
		a.deviceCard(c, p, devices)
	})
	a.sessionNoticeBar(c, p)
	if a.errorBarShown() {
		statusBar(c, p, barDanger, iconAlert, locales.FlyoutConnectFailed(a.errorText()), nil)
	}
	if a.snap.CredentialReset {
		a.credentialBar(c, p)
	}
}

func (a *App) hasNowMedia() bool {
	if !a.cfg.ShowNowPlayingPanel {
		return false
	}
	_, ok := a.nowSession()
	return ok
}

// disconnectAllShown reports a session with at least two targets, when the
// device head's one-click disconnect applies. One target's row already carries
// 断开连接, so the head's action would only duplicate it.
func (a *App) disconnectAllShown() bool {
	return len(a.snap.ActiveTargets) > 1
}

// errorBarShown carries the error bar in a failed phase, or for a connect or
// refused-add error raised while the stream is healthy.
func (a *App) errorBarShown() bool {
	return errorBarNeeded(a.snap)
}

func (a *App) deviceCard(c *ui.Context, p palette, devices []device.Device) {
	card := ui.Column(c).Margin(2, 0, 0, 0).Height(deviceCardHeight(len(devices))).Radius(10).
		Background(p.card).Border(1, p.line).Clip().Label(locales.DeviceListLabel())
	card.Children(func() {
		if len(devices) == 0 {
			a.emptyDevices(c, p)
			return
		}
		rows := func() {
			ui.Column(c).Children(func() {
				for i, d := range devices {
					a.deviceRow(c, p, d, i < len(devices)-1)
				}
			})
		}
		if len(devices) > deviceRows {
			ui.Scroll(c).Fill().Children(rows)
			return
		}
		rows()
	})
}

func (a *App) emptyDevices(c *ui.Context, p palette) {
	ui.Column(c).Fill().Center().Gap(4).Padding(18).Children(func() {
		face := ui.Icon(c, faceSearch).Size(28, 28).TextColor(p.faint)
		t := face.Loop("pulse", 1200*time.Millisecond, ui.EaseInOut)
		face.Opacity(0.55 + 0.45*(1-abs32(2*t-1)))
		emptyCopy(c, p, locales.FlyoutSearching(), locales.FlyoutSearchHint(), true)
		b := ui.ButtonBase(c).Padding(2, 6).Radius(6).Cursor(ui.CursorPointer).Label(locales.FlyoutSearchAgain())
		b.OnClick(func() { a.eng.Refresh() })
		hoverFill(b, p.pill, 6, 0)
		b.Children(func() {
			ui.Text(c, locales.FlyoutSearchAgain()).FontSize(12.5).TextColor(p.accent).SingleLine()
		})
	})
}

// deviceRow is one row of the device card. An established target's row is a
// compact mixer: a channel button before the name, the target's volume under
// it, and the connect action in its own centered column. A row that is not a
// member keeps its name and a second line, centered in the same height.
func (a *App) deviceRow(c *ui.Context, p palette, d device.Device, divider bool) {
	target, hasTarget := a.snap.Target(d.ID)

	row := ui.Row(c).Height(deviceRowHeight).Padding(9, 12).Gap(10).
		Key(d.ID).Transition(ui.ElementTransition{Colors: true, Duration: 140 * time.Millisecond})
	if divider {
		row.BorderWidth(0, 0, 1, 0).BorderColor(p.line2)
	}
	// The action column keeps one width whatever its label says (取消, 连接,
	// 断开连接, 重新连接), so the name, slider, and percent never move when
	// the session phase changes.
	action := func() {
		ui.Row(c).Width(deviceActionWidth).Justify(ui.End).AlignItems(ui.Center).Shrink(0).Children(func() {
			a.rowAction(c, p, d)
		})
	}

	// The row's 140 ms color transition needs the background in the build;
	// a hover drawn in Paint would snap. A session target is never tinted: the
	// card's rows are peers, and the action tells the membership.
	if row.Hovered() {
		row.Background(p.cardHover)
	} else {
		row.Background(ui.Transparent)
	}

	row.Children(func() {
		// A member keeps its mixer while the cohort re-establishes, so adding
		// a device never takes the first device's controls away. A target that
		// is joining, reopening, or failed is plain: nothing offers volume
		// before playback, and the row's height does not change.
		if hasTarget && target.State == engine.TargetMember {
			deviceGlyph(c, p, d, a.deviceSeen(d))
			a.deviceMixer(c, p, d, target)
		} else {
			pick := ui.ButtonBase(c.Key("device:" + d.ID)).Grow(1).Gap(10).
				AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(d.Name)
			pick.OnClick(func() { a.selectDevice(d.ID) })
			pick.Children(func() {
				deviceGlyph(c, p, d, a.deviceSeen(d))
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Row(c).Height(26).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…").Shrink(1)
						a.comboTag(c, p, d)
						// A combo carries one password per member, so only a
						// single receiver shows the gate on this row.
						if !d.IsStereoGroup() {
							if v, ok := a.receiverViewFor(d.ID); ok {
								if pill := a.receiverAuthPill(v); pill.ok {
									iconChip(c, p, pill.icon, pill.text, pill.tone, false)
								}
							}
						}
					})
					ui.Row(c).Height(20).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.rowSubtitle(d, target, hasTarget)).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
					})
				})
			})
		}
		action()
	})
}

// deviceIsCombo reports whether the row's stereo group is a user-created
// combo rather than a native pair; the flyout marks it with the combo tag.
func (a *App) deviceIsCombo(d device.Device) bool {
	if !d.IsStereoGroup() {
		return false
	}
	v, ok := a.receiverViewFor(d.ID)
	return ok && v.Entry.IsCombo()
}

// comboTag renders the 立体声组合 tag a combo row carries beside its name,
// on both the plain and the mixer layout.
func (a *App) comboTag(c *ui.Context, p palette, d device.Device) {
	if a.deviceIsCombo(d) {
		inlineChip(c, p, locales.ComboTag(), chipNeutral, false)
	}
}

// rowSubtitle is a plain row's second line: a target the engine is working on
// says so, in the wording the session state already uses; every other row
// keeps the channel, the family, and the system version; a combo names its
// members in playback order.
func (a *App) rowSubtitle(d device.Device, t engine.TargetControl, has bool) string {
	if has {
		switch t.State {
		case engine.TargetJoining:
			return locales.StatusConnecting()
		case engine.TargetReopening:
			return locales.StatusReconnecting()
		}
	}
	if v, ok := a.receiverViewFor(d.ID); ok {
		return flyoutDetail(v, a.cfg.ModelDetail)
	}
	return flyoutDetail(receivers.View{Device: d, Stereo: d.IsStereoGroup()}, a.cfg.ModelDetail)
}

// deviceMixer is a member row's two-line content: the channel button and name,
// then volume and percent. A stereo target draws no channel button — its two
// members play one side each — so its name starts at the row's lead. The 6 DIP
// gap after the 16 DIP marks keeps the name and the rail one column, at the
// pill's icon-to-text rhythm.
func (a *App) deviceMixer(c *ui.Context, p palette, d device.Device, t engine.TargetControl) {
	ui.Column(c).Grow(1).Gap(2).Children(func() {
		ui.Row(c).Height(26).Gap(6).AlignItems(ui.Center).Children(func() {
			if t.Stereo {
				stereoLead(c, p)
			} else {
				a.channelButton(c, p, d, t)
			}
			name := ui.ButtonBase(c.Key("device:" + d.ID)).Grow(1).Justify(ui.Start).
				Cursor(ui.CursorPointer).Label(d.Name)
			name.OnClick(func() { a.selectDevice(d.ID) })
			name.Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…").Shrink(1)
				})
			})
		})
		ui.Row(c).Height(20).Gap(6).AlignItems(ui.Center).Children(func() {
			a.volumeButton(c, p, d, t)
			slider(c, p, a.targetVolume(t), 0, 100, locales.VolumeFor(t.Name), func(v float64) {
				if a.eng != nil {
					a.eng.SetTargetVolume(t.ID, v)
				}
			}).Grow(1)
			ui.Text(c, locales.FlyoutVolumeValue(t.Volume)).FontSize(11.5).TextColor(p.muted).
				FontFeatures("tnum").Width(34).TextAlign(ui.End).SingleLine()
		})
	})
}

// volumeButton is the speaker glyph. One click mutes the target by taking its
// volume to zero; the next click returns the volume it muted from. A target
// already at zero without a remembered level comes back at the launch volume.
// The 16 DIP ink keeps the column's left edge, and the glyph rides a full DIP
// down to meet the slider's rail, which sits a DIP below the line's center.
// Like the channel button, the body rides 3 DIP wider each side for the hover
// pill, and negative margins hand the width back to the row.
func (a *App) volumeButton(c *ui.Context, p palette, d device.Device, t engine.TargetControl) {
	muted := t.Volume <= 0
	label, icon := locales.MuteFor(t.Name), iconVolume
	if muted {
		label, icon = locales.UnmuteFor(t.Name), iconVolumeOff
	}
	b := ui.ButtonBase(c.Key("mute:"+d.ID)).Size(22, 20).Margin(0, -3, 0, -3).
		Radius(6).Center().Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	b.OnClick(func() { a.toggleTargetMute(t) })
	hoverFill(b, p.pill, 6, 0)
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(p.fg2).Margin(2, 0, 0, 0)
	})
}

func (a *App) toggleTargetMute(t engine.TargetControl) {
	if a.eng == nil {
		return
	}
	if t.Volume > 0 {
		if a.preMuteVolume == nil {
			a.preMuteVolume = map[string]float64{}
		}
		a.preMuteVolume[t.ID] = t.Volume
		a.eng.SetTargetVolume(t.ID, 0)
		return
	}
	restore, ok := a.preMuteVolume[t.ID]
	if !ok || restore <= 0 {
		restore = a.cfg.Volume
		if restore <= 0 {
			restore = 50
		}
	}
	delete(a.preMuteVolume, t.ID)
	a.eng.SetTargetVolume(t.ID, restore)
}

// targetVolume is the slider's bound value. The snapshot's own field is used
// when present, so a drag survives the view rebuilds between engine commands.
func (a *App) targetVolume(t engine.TargetControl) *float64 {
	for i := range a.snap.Targets {
		if a.snap.Targets[i].ID == t.ID {
			return &a.snap.Targets[i].Volume
		}
	}
	v := t.Volume
	return &v
}

// stereoLead is a stereo member's non-interactive mark, aligned with the
// single receiver's channel button. A stereo target has no side to choose, so
// the mark never takes focus or a click; the combo chip yields to it while the
// row is a member. The 16 DIP body puts the mark's ink on the content column's
// left edge, where the plain rows' names start, and the full-DIP drop levels
// it with the name's low-riding glyphs.
func stereoLead(c *ui.Context, p palette) {
	ui.Box(c).Size(16, 16).Shrink(0).Margin(2, 0, 0, 0).
		Label(locales.ChannelStereo()).Children(func() {
		ui.Image(c, channelIcon(receivers.ChannelStereo)).Size(16, 16).TextColor(p.fg)
	})
}

// channelButton is the icon-only channel control on a single receiver: the
// solid side is the current side and a click cycles stereo → left → right. A
// stereo target has no side to choose, so deviceMixer leaves it out. The 16
// DIP ink keeps the mark on the content column's edge, like stereoLead's; the
// button rides 3 DIP wider each side for a hover pill worth pointing at, and
// the negative margins hand that width back to the row, so the name holds its
// column.
func (a *App) channelButton(c *ui.Context, p palette, d device.Device, t engine.TargetControl) {
	label := channelLabelText(t.Channel)
	b := ui.ButtonBase(c.Key("channel:"+d.ID)).Size(22, 16).Margin(0, -3, 0, -3).
		Radius(8).Center().Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	b.OnClick(func() { a.cycleChannel(t) })
	hoverFill(b, p.pill, 8, 0)
	b.Children(func() {
		// Image, not Icon: the icon mask drops fill-opacity, and the faded
		// side is the state. Image renders the SVG in the palette color. The
		// full-DIP drop is stereoLead's, leveling the glyph with the name's.
		ui.Image(c, channelIcon(t.Channel)).Size(16, 16).TextColor(p.fg).Margin(2, 0, 0, 0)
	})
}

// channelLabelText names the current side and what a click does next. Only a
// single receiver draws the control; a stereo target has no side to choose.
func channelLabelText(channel string) string {
	return locales.ChannelTooltip(channelDisplay(channel), channelDisplay(nextChannel(channel)))
}

func channelDisplay(channel string) string {
	switch receivers.ParseChannel(channel) {
	case receivers.ChannelLeft:
		return locales.ChannelLeft()
	case receivers.ChannelRight:
		return locales.ChannelRight()
	}
	return locales.ChannelStereo()
}

func channelIcon(channel string) *ui.SVG {
	switch receivers.ParseChannel(channel) {
	case receivers.ChannelLeft:
		return iconChannelLeft
	case receivers.ChannelRight:
		return iconChannelRight
	}
	return iconChannelStereo
}

func nextChannel(channel string) string {
	switch receivers.ParseChannel(channel) {
	case receivers.ChannelLeft:
		return receivers.ChannelRight
	case receivers.ChannelRight:
		return receivers.ChannelStereo
	}
	return receivers.ChannelLeft
}

func (a *App) cycleChannel(t engine.TargetControl) {
	if a.eng == nil {
		return
	}
	next := nextChannel(t.Channel)
	a.eng.UpdateReceiver(t.ID, receivers.Patch{Channel: &next})
}

// deviceSeen reports a live sighting for the row. A manual receiver is
// Online because its address is attemptable, so its dot waits for discovery
// instead of claiming it is there.
func (a *App) deviceSeen(d device.Device) bool {
	if v, ok := a.receiverViewFor(d.ID); ok {
		return v.Seen
	}
	return d.Online
}

// deviceGlyph is the flyout row's round silhouette with the online dot; the
// settings row draws its online mark beside the name.
func deviceGlyph(c *ui.Context, p palette, d device.Device, seen bool) {
	g := ui.Box(c).Size(30, 30).Radius(15).Center().Shrink(0).Background(p.pill)
	g.Children(func() {
		ui.Icon(c, stereoIcon(d, d.IsStereoGroup())).Size(16, 16).TextColor(p.fg)
		if seen {
			ui.Box(c).Size(9, 9).Radius(4.5).Background(p.green).
				Shadow(0, 0, 0, 2, p.card).
				Attach(ui.AnchorBottomRight, ui.AnchorBottomRight).Right(-1).Bottom(-1)
		}
	})
}

// rowActionKind is the control a receiver row offers. It is a pure function of
// the target's published role and the row's reachability, so a rebuild never
// offers 取消 to a member that is merely re-establishing.
type rowActionKind uint8

const (
	actionConnect rowActionKind = iota
	actionConnectDisabled
	actionCancel
	actionRetry
	actionDisconnect
	actionOffline
)

// rowActionFor maps one row to its control. The engine names the target the
// user's connect is joining; every other session row is a member, and only a
// device with no role falls back to the phase.
func rowActionFor(s engine.Snapshot, d device.Device) rowActionKind {
	if t, ok := s.Target(d.ID); ok {
		switch t.State {
		case engine.TargetFailed:
			return actionRetry
		case engine.TargetJoining:
			return actionCancel
		case engine.TargetMember, engine.TargetReopening:
			return actionDisconnect
		}
	}
	if !d.Online {
		return actionOffline
	}
	if phaseOf(s) == phaseConnecting {
		// One session at a time. An attempt in progress must finish or be
		// cancelled before another receiver can take the session.
		return actionConnectDisabled
	}
	// While streaming, another row's connect adds that receiver; the engine
	// rebuilds the cohort once so every member shares the new start epoch.
	return actionConnect
}

func (a *App) connectDevice(id string) {
	a.cancelLatencyApply()
	a.finishLatencyApply()
	a.selectDevice(id)
	if a.eng == nil {
		return
	}
	a.eng.Connect(id)
}

func (a *App) rowAction(c *ui.Context, p palette, d device.Device) {
	switch rowActionFor(a.snap, d) {
	case actionCancel:
		textButton(c, p, locales.ActionCancel(), iconClose, p.ctl, func() { a.disconnectTarget(d.ID) })
	case actionRetry:
		label := locales.ActionRetry()
		if a.snap.TakenOver {
			label = locales.ActionReconnect()
		}
		textButton(c, p, label, iconRefresh, p.accent, func() { a.connectDevice(d.ID) })
	case actionDisconnect:
		textButton(c, p, locales.ActionDisconnect(), iconUnlink, p.fg2, func() { a.disconnectTarget(d.ID) })
	case actionOffline:
		textButton(c, p, locales.ActionOffline(), iconSlash, p.muted, nil)
	case actionConnectDisabled:
		textButton(c, p, locales.ActionConnect(), iconLink, p.faint, nil)
	default:
		textButton(c, p, locales.ActionConnect(), iconLink, p.accent, func() { a.connectDevice(d.ID) })
	}
}
