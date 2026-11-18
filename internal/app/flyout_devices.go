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
	sectionHead(c, p, locales.FlyoutDevices(), locales.FlyoutDeviceCount(len(devices)), false, action)
	a.deviceCard(c, p, devices)
	a.sessionNoticeBar(c, p)
	if a.errorBarShown() {
		statusBar(c, p, barDanger, iconAlert, locales.FlyoutConnectFailed(a.errorText()), nil)
	}
	if a.snap.CredentialReset {
		a.credentialBar(c, p)
	}
}

// hasNowMedia reports whether the media card has a session to draw.
func (a *App) hasNowMedia() bool {
	if !a.cfg.ShowNowPlayingPanel {
		return false
	}
	_, ok := a.nowSession()
	return ok
}

// disconnectAllShown reports a live or wanted session, when the device head's
// one-click disconnect applies. A per-target row action leaves the rest of
// the cohort alone.
func (a *App) disconnectAllShown() bool {
	return len(a.snap.ActiveTargets) > 0
}

// errorBarShown carries the error bar in a failed phase, or for a connect or
// refused-add error raised while the stream is healthy.
func (a *App) errorBarShown() bool {
	return errorBarNeeded(a.snap)
}

func (a *App) deviceCard(c *ui.Context, p palette, devices []device.Device) {
	card := ui.Column(c).Height(deviceCardHeight(len(devices))).Radius(10).
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
	selected := d.ID == a.selID
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
	// a hover drawn in Paint would snap.
	switch {
	case selected:
		row.Background(p.sel)
	case row.Hovered():
		row.Background(p.cardHover)
	default:
		row.Background(ui.Transparent)
	}

	row.Children(func() {
		// A member keeps its mixer while the cohort re-establishes, so adding
		// a device never takes the first device's controls away. A target that
		// is joining, reopening, or failed is plain: nothing offers volume
		// before playback, and the row's height does not change.
		if hasTarget && target.State == engine.TargetMember {
			deviceGlyph(c, p, d, selected)
			a.deviceMixer(c, p, d, target, selected)
		} else {
			pick := ui.ButtonBase(c.Key("device:" + d.ID)).Grow(1).Gap(10).
				AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(d.Name)
			pick.OnClick(func() { a.selectDevice(d.ID) })
			pick.Children(func() {
				deviceGlyph(c, p, d, selected)
				// The plain row keeps the live row's line rhythm: a 26 name
				// line, a 2 gap, and a 20 second line, so the name and model
				// breathe the same way as the name and volume bar above.
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Row(c).Height(26).AlignItems(ui.Center).Children(func() {
						ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
					})
					ui.Row(c).Height(20).AlignItems(ui.Center).Children(func() {
						ui.Text(c, rowSubtitle(d, target, hasTarget)).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
					})
				})
			})
		}
		action()
	})
}

// rowSubtitle is a plain row's second line: a target the engine is working on
// says so, in the wording the session state already uses; every other row
// keeps its model and address.
func rowSubtitle(d device.Device, t engine.TargetControl, has bool) string {
	if has {
		switch t.State {
		case engine.TargetJoining:
			return locales.StatusConnecting()
		case engine.TargetReopening:
			return locales.StatusReconnecting()
		}
	}
	return deviceSubtitle(d)
}

// deviceMixer is a member row's two-line content: the channel button and name,
// then the device's own volume and percent. The channel button and the volume
// glyph share a 26 wide lead, so the name and the rail start on one x.
func (a *App) deviceMixer(c *ui.Context, p palette, d device.Device, t engine.TargetControl, selected bool) {
	ui.Column(c).Grow(1).Gap(2).Children(func() {
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
			a.channelButton(c, p, d, t)
			name := ui.ButtonBase(c.Key("device:" + d.ID)).Grow(1).Justify(ui.Start).
				Cursor(ui.CursorPointer).Label(d.Name)
			name.OnClick(func() { a.selectDevice(d.ID) })
			name.Children(func() {
				ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
			})
		})
		ui.Row(c).Height(20).Gap(8).AlignItems(ui.Center).Children(func() {
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
func (a *App) volumeButton(c *ui.Context, p palette, d device.Device, t engine.TargetControl) {
	muted := t.Volume <= 0
	label, icon := locales.MuteFor(t.Name), iconVolume
	if muted {
		label, icon = locales.UnmuteFor(t.Name), iconVolumeOff
	}
	b := ui.ButtonBase(c.Key("mute:" + d.ID)).Width(26).Height(20).Radius(6).Center().
		Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	b.OnClick(func() { a.toggleTargetMute(t) })
	hoverFill(b, p.pill, 6, 0)
	b.Children(func() {
		ui.Icon(c, icon).Size(14, 14).TextColor(p.fg2)
	})
}

func (a *App) toggleTargetMute(t engine.TargetControl) {
	if a.eng == nil {
		return
	}
	if t.Volume > 0 {
		if a.mutedVolume == nil {
			a.mutedVolume = map[string]float64{}
		}
		a.mutedVolume[t.ID] = t.Volume
		a.eng.SetTargetVolume(t.ID, 0)
		return
	}
	restore, ok := a.mutedVolume[t.ID]
	if !ok || restore <= 0 {
		restore = a.cfg.Volume
		if restore <= 0 {
			restore = 50
		}
	}
	delete(a.mutedVolume, t.ID)
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

// channelButton is the icon-only channel control. The solid side of the
// symbol is the current side; a click cycles stereo → left → right. A stereo
// pair always plays stereo, so its button is disabled.
func (a *App) channelButton(c *ui.Context, p palette, d device.Device, t engine.TargetControl) {
	label := channelLabelText(t)
	b := ui.ButtonBase(c.Key("channel:"+d.ID)).Size(26, 26).Radius(13).Center().
		Label(label).Tooltip(label)
	if t.Stereo {
		b.Disabled(true).Opacity(0.4)
	} else {
		b.Cursor(ui.CursorPointer)
		b.OnClick(func() { a.cycleChannel(t) })
		hoverFill(b, p.pill, 13, 0)
	}
	b.Children(func() {
		// Image, not Icon: the icon mask drops fill-opacity, and the faded
		// side is the state. Image renders the SVG in the palette color.
		ui.Image(c, channelIcon(t.Channel)).Size(16, 16).TextColor(p.fg2)
	})
}

// channelLabelText names the current side and what a click does next. A pair
// cannot change, so its label says why.
func channelLabelText(t engine.TargetControl) string {
	current := channelDisplay(t.Channel)
	if t.Stereo {
		return locales.ChannelPairTooltip(current)
	}
	return locales.ChannelTooltip(current, channelDisplay(nextChannel(t.Channel)))
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

// nextChannel is the cycle order the channel button walks.
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

// deviceGlyph is the round silhouette with the online dot, shared by the
// flyout rows and the settings summary.
func deviceGlyph(c *ui.Context, p palette, d device.Device, selected bool) {
	color := p.fg2
	if selected {
		color = p.accent
	}
	g := ui.Box(c).Size(30, 30).Radius(15).Center().Shrink(0).Background(p.pill)
	if selected {
		g.Background(p.card)
	}
	g.Children(func() {
		ui.Icon(c, deviceIcon(d.Model, d.IsStereoGroup())).Size(16, 16).TextColor(color)
		if d.Online {
			ring := p.card
			if selected {
				ring = p.sel
			}
			ui.Box(c).Size(9, 9).Radius(4.5).Background(p.green).
				Shadow(0, 0, 0, 2, ring).
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
