package app

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/audio/analyze"
	"saundo/internal/engine"
	"saundo/internal/nowplaying"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

func (a *App) view(c *ui.Context) {
	p := design(c)
	a.adoptTextScale(p.scale, a.win)
	ui.Column(c).Fill().Background(p.panel).Children(func() {
		content := ui.Column(c).Fill()
		enterPose(c, content, &a.flyoutEntered)
		content.Children(func() {
			a.flyoutHeader(c, p)
			ui.Column(c).Grow(1).Padding(2, 12, 8, 12).Gap(8).Children(func() {
				a.deviceSection(c, p)
				a.playbackSection(c, p)
				a.lampSection(c, p)
			})
			a.flyoutFooter(c, p)
		})
	})
	a.pinDialog(c, p)
	a.passwordDialog(c, p)
	a.credentialDialog(c, p)
}

func (a *App) flyoutHeader(c *ui.Context, p palette) {
	ui.Row(c).Height(46).Padding(0, 10, 0, 14).Gap(8).Children(func() {
		ui.Icon(c, iconMark).Size(21, 21).TextColor(p.fg)
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, locales.AppName()).FontSize(13).FontWeight(600).SingleLine()
			ui.Text(c, locales.AppSubtitle()).FontSize(8.5).FontWeight(600).LetterSpacing(1.1).TextColor(p.muted).SingleLine()
		})
		ui.Spacer(c)
		iconButton(c, p, iconRefresh, locales.FlyoutRefreshDevices(), 14, func() { a.actRefresh() })
		iconButton(c, p, iconGear, locales.CommonSettings(), 15, func() { a.actSettings() })
	})
}

// deviceActionWidth is the row's trailing column: wide enough for the longest
// action (断开连接 or 重新连接 with its glyph), fixed across phases.
const deviceActionWidth = 92

func (a *App) deviceSection(c *ui.Context, p palette) {
	devices := a.snap.Devices
	// The head keeps the 26-DIP action slot whether or not a session gives it
	// the 全部断开 button, so starting or ending a session never moves the
	// panel. The empty slot draws nothing and takes no width.
	action := func() {
		if a.disconnectAllShown() {
			rowAction(c, p, "disconnect-all", locales.DisconnectAll(), iconUnlink, p.fg2, func() { a.actDisconnect() })
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

// hasNowMedia reports whether the media card has a session to draw.
func (a *App) hasNowMedia() bool {
	if !a.cfg.ShowNowPlayingPanel {
		return false
	}
	_, ok := a.nowSession()
	return ok
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
		b.OnClick(func() { a.actRefresh() })
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
					ui.Row(c).Height(26).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…").Grow(1)
						a.comboTag(c, p, d)
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
// says so, in the wording the session state already uses; a stereo combo
// names its two sides; every other row keeps its model and address.
func (a *App) rowSubtitle(d device.Device, t engine.TargetControl, has bool) string {
	if has {
		switch t.State {
		case engine.TargetJoining:
			return locales.StatusConnecting()
		case engine.TargetReopening:
			return locales.StatusReconnecting()
		}
	}
	if d.IsStereoGroup() {
		if v, ok := a.receiverViewFor(d.ID); ok && v.Entry.IsCombo() {
			return comboMemberSubtitle(d.Name, d.Members, deviceSubtitle(d))
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
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…").Grow(1)
					a.comboTag(c, p, d)
				})
			})
		})
		ui.Row(c).Height(20).Gap(8).AlignItems(ui.Center).Children(func() {
			a.volumeButton(c, p, d, t)
			slider(c, p, a.targetVolume(t), 0, 100, locales.VolumeFor(t.Name), func(v float64) {
				a.actTargetVolume(t.ID, v)
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
	if t.Volume > 0 {
		if a.mutedVolume == nil {
			a.mutedVolume = map[string]float64{}
		}
		a.mutedVolume[t.ID] = t.Volume
		a.actTargetVolume(t.ID, 0)
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
	a.actTargetMute(t.ID, restore)
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
// or a combo cannot change, so its label says why.
func channelLabelText(t engine.TargetControl) string {
	current := channelDisplay(t.Channel)
	if t.Split {
		return locales.ComboChannelLocked()
	}
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
	next := nextChannel(t.Channel)
	a.actTargetChannel(t.ID, next)
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

// actionFor is the legacy phase-based mapping, kept for callers that reason
// about the session phase rather than a published target role.
func actionFor(ph phase, active, online bool) rowActionKind {
	switch {
	case active && ph == phaseConnecting:
		return actionCancel
	case active && ph == phaseError:
		return actionRetry
	case active:
		return actionDisconnect
	case !online:
		return actionOffline
	case ph == phaseConnecting:
		return actionConnectDisabled
	}
	return actionConnect
}

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
		textButton(c, p, locales.ActionCancel(), iconClose, p.ctl, func() { a.actDisconnectTarget(d.ID) })
	case actionRetry:
		label := locales.ActionRetry()
		if a.snap.TakenOver {
			label = locales.ActionReconnect()
		}
		textButton(c, p, label, iconRefresh, p.accent, func() { a.actConnect(d.ID) })
	case actionDisconnect:
		textButton(c, p, locales.ActionDisconnect(), iconUnlink, p.fg2, func() { a.actDisconnectTarget(d.ID) })
	case actionOffline:
		textButton(c, p, locales.ActionOffline(), iconSlash, p.muted, nil)
	case actionConnectDisabled:
		textButton(c, p, locales.ActionConnect(), iconLink, p.faint, nil)
	default:
		textButton(c, p, locales.ActionConnect(), iconLink, p.accent, func() { a.actConnect(d.ID) })
	}
}

func (a *App) playbackSection(c *ui.Context, p palette) {
	// Volume lives on the device rows now; with no media and no latency row
	// the section would only hold its own head, so it is left out.
	if !a.hasNowMedia() && !a.showLatencySwitch {
		return
	}
	note := locales.FlyoutFormat()
	if phaseOf(a.snap) == phaseStreaming {
		note += " · " + a.syncText()
	}
	note += " · " + locales.FlyoutLatency(a.latencyDisplay())
	sectionHead(c, p, locales.FlyoutPlayback(), note, a.latencyApplying, nil)

	card := ui.Column(c).Radius(10).Background(p.card).Border(1, p.line).Clip()
	card.Children(func() {
		if a.hasNowMedia() {
			a.nowPlayingCard(c, p)
		}
		if a.hasNowMedia() && a.showLatencySwitch {
			rowLine(c, p)
		}
		if a.showLatencySwitch {
			ui.Column(c).Padding(8, 12, 8, 12).Children(func() { a.latencySwitch(c, p) })
		}
	})
}

// latencySwitch is the four-segment mode control. It carries no value of its
// own; the playback head's ALAC line shows the current latency.
func (a *App) latencySwitch(c *ui.Context, p palette) {
	seg := a.latencyModeIndex()
	items := []segmentItem{
		{locales.SettingsLatencyLow(), iconLatencyLow},
		{locales.SettingsLatencyStandard(), iconLatencyStandard},
		{locales.SettingsLatencyStable(), iconLatencyStable},
		{locales.SettingsLatencyCustom(), iconSliders},
	}
	track := segmented(c, p, &seg, items, 0, false, segmentLeading, locales.SettingsLatency())
	if track.Changed() {
		a.setLatencyMode(latencyModeAt(seg))
	}
}

func (a *App) nowPlayingCard(c *ui.Context, p palette) {
	if !a.cfg.ShowNowPlayingPanel {
		return
	}
	s, ok := a.nowSession()
	if !ok {
		return
	}
	field := a.nowFieldLight
	if p.dark {
		field = a.nowFieldDark
	}
	delay := time.Duration(a.latencyMS()) * time.Millisecond
	level := func() analyze.Frame {
		if a.levelFrame != nil {
			return a.levelFrame(delay)
		}
		return analyze.Frame{}
	}
	a.nowPlayingCardBody(c, p, s, false, true, a.nowArt, field, level, len(a.nowSnap.Sessions) > 1)
}

// nowPlayingCardBody draws the card. preview is the settings page's copy:
// its transport and session switch are read-only, and animateMeter says
// whether its meter asks for continuous frames.
func (a *App) nowPlayingCardBody(c *ui.Context, p palette, s nowplaying.Session, preview, animateMeter bool, art *ui.Bitmap, field *image.RGBA, level func() analyze.Frame, showSwitch bool) {
	ui.Column(c).Margin(8, 12, 10, 12).Padding(10, 10, 8, 10).Radius(12).Background(p.pill).Children(func() {
		ui.Row(c).Height(24).Gap(7).AlignItems(ui.Start).Children(func() {
			// The app icon is optional: the shell knows one for most apps,
			// and the name alone carries the rest.
			if a.nowIcon != nil {
				ui.Image(c, a.nowIcon).Size(18, 18).Shrink(0)
			}
			name := s.AppName
			if name == "" {
				name = locales.FlyoutUnknownApp()
			}
			ui.Text(c, name).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…").Grow(1)
			if showSwitch {
				btn := ui.ButtonBase(c).Size(20, 20).Radius(6).Center().Background(p.card).
					Cursor(ui.CursorPointer).Label(locales.FlyoutSwitchMedia()).Disabled(preview)
				if btn.Hovered() {
					btn.Background(p.cardHover)
				}
				btn.OnClick(a.switchNowMedia)
				btn.Children(func() {
					col := p.muted
					if btn.Hovered() {
						col = p.fg
					}
					ui.Icon(c, iconChevronRight).Size(14, 14).TextColor(col)
				})
			}
		})
		ui.Row(c).Height(62).Gap(10).AlignItems(ui.Center).Padding(6, 0, 4, 0).Margin(2, 0, 0, 0).Children(func() {
			if a.cfg.CoverMode != settings.CoverNone {
				a.nowPlayingPoster(c, p, art, a.cfg.CoverMode)
			}
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				title := s.Title
				if title == "" {
					title = s.AppName
				}
				if title == "" {
					title = locales.FlyoutNowPlayingNoMedia()
				}
				ui.Text(c, title).FontSize(15).FontWeight(500).SingleLine().Ellipsis("…")
				ui.Text(c, a.nowPlayingSubFor(s)).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			})
			if a.cfg.AudioLevel {
				a.nowPlayingMeter(c, p, s, field, level, animateMeter)
			}
		})
		a.nowPlayingProgress(c, p, s, preview, field)
	})
}

// nowPlayingPoster keeps the cover's own aspect ratio around a fixed height,
// so switching sessions never changes the card's height: a 16:9 cover gets a
// wider poster, not a taller one.
func (a *App) nowPlayingPoster(c *ui.Context, p palette, art *ui.Bitmap, mode settings.CoverMode) {
	h := float32(52)
	w := h
	// The placeholder is a raised tile on the media card's pill surface, like
	// the transport circles, so it reads as a cover rather than empty space.
	if mode == settings.CoverPlaceholder || art == nil {
		box := ui.Box(c).Size(w, h).Radius(9).Clip().Shrink(0).Background(p.card)
		box.Center().Children(func() {
			ui.Icon(c, iconNote).Size(18, 18).TextColor(p.muted)
		})
		return
	}
	if bw, bh := art.Size(); bw > 0 && bh > 0 {
		w = h * float32(bw) / float32(bh)
	}
	// The artwork's tile keeps the card's pill, so a corner sliver never
	// shows as a light edge around the image.
	box := ui.Box(c).Size(w, h).Radius(9).Clip().Shrink(0).Background(p.pill)
	box.Children(func() {
		ui.Image(c, art).Size(w, h).Fit(ui.Cover)
	})
}

// nowTransport draws the enabled transport controls as bare glyphs on
// 24-DIP hit areas, 2 DIPs apart. The bare icons sit on the content column;
// hovering one draws its circle back, so the rest of the row stays light.
func (a *App) nowTransport(c *ui.Context, p palette, s nowplaying.Session, preview bool) {
	if !a.cfg.PlaybackControls || !a.nowHasControls(s) {
		return
	}
	ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Margin(0, 0, 0, -6).Children(func() {
		if s.CanPrev {
			a.nowTransportButton(c, p, iconPrev, locales.FlyoutPrevious(), nowplaying.ActionPrevious, s.AUMID, preview)
		}
		if s.Playing && s.CanPause {
			a.nowTransportButton(c, p, iconPause, locales.FlyoutPause(), nowplaying.ActionPause, s.AUMID, preview)
		} else if s.CanPlay {
			a.nowTransportButton(c, p, iconPlay, locales.FlyoutPlay(), nowplaying.ActionPlay, s.AUMID, preview)
		}
		if s.CanNext {
			a.nowTransportButton(c, p, iconNext, locales.FlyoutNext(), nowplaying.ActionNext, s.AUMID, preview)
		}
	})
}

func (a *App) nowTransportButton(c *ui.Context, p palette, icon *ui.SVG, label string, action nowplaying.Action, aumid string, preview bool) {
	btn := ui.ButtonBase(c).Size(24, 24).Radius(12).Center().
		Cursor(ui.CursorPointer).Label(label).Disabled(preview)
	hoverFill(btn, p.card, 12, 0)
	btn.OnClick(func() {
		if a.controlNow != nil {
			a.controlNow(aumid, action)
		}
	})
	btn.Children(func() {
		ui.Icon(c, icon).Size(12, 12).TextColor(p.fg)
	})
}

// meterAnimates reports whether the meter asks for continuous paint frames:
// only a playing meter on a real window, and only when motion is allowed.
// A headless tester has no window and would wait forever for frames to stop.
// reduceMotion runs last: without a running app the theme's preferences block.
func meterAnimates(animate, playing, windowed bool, reduceMotion func() bool) bool {
	return animate && playing && windowed && !reduceMotion()
}

// previewMeterAnimates keeps the settings sample still: the preview follows
// the live level only while a session plays.
func previewMeterAnimates(live bool) bool { return live }

// nowPlayingMeter draws five bars centered on one horizontal axis. Each bar is
// a window into the cover's color field at its own column, so the spectrum
// follows the art's colors as well as its level.
func (a *App) nowPlayingMeter(c *ui.Context, p palette, s nowplaying.Session, field *image.RGBA, level func() analyze.Frame, animate bool) {
	tint := a.cfg.CoverColors
	color := func(x, y int) ui.Color {
		if !tint {
			return p.fg
		}
		return fieldColor(field, x, y)
	}
	playing := s.Playing
	ui.Box(c).Size(fieldW, fieldH).Shrink(0).Draw(func(pt *ui.Painter, r ui.Rect) {
		frame := analyze.Frame{}
		if level != nil {
			frame = level()
		}
		for i := 0; i < analyze.BandCount; i++ {
			h := float32(3)
			if playing {
				h = 8 + frame.Bands[i]*16
			}
			if h > fieldH {
				h = fieldH
			}
			top := int(12 - h/2)
			bottom := int(12 + h/2)
			from := color(i*5+1, top)
			to := color(i*5+1, bottom)
			if !playing {
				alpha := float32(0.45)
				if p.dark {
					alpha = 0.58
				}
				from, to = from.Alpha(alpha), to.Alpha(alpha)
			}
			bar := ui.Rect{X: r.X + float32(i*5), Y: r.Y + (r.H-h)/2, W: 3, H: h}
			pt.FillGradient(bar, ui.LinearGradient{From: from, To: to, Angle: 180}, 1.5)
		}
		windowed := a.win != nil || a.settingsWin != nil
		if meterAnimates(animate, playing, windowed, func() bool {
			return mygo.Theme.Preferences().ReduceMotion
		}) {
			pt.AnimationFrameRate(30)
		}
	})
}

// nowPlayingProgress is the transport row. A session with a timeline shows
// elapsed, the cover-field fill and thumb, and remaining; a session without a
// timeline keeps the same row as an empty track, with no fill, thumb, or copy.
func (a *App) nowPlayingProgress(c *ui.Context, p palette, s nowplaying.Session, preview bool, field *image.RGBA) {
	ui.Row(c).Gap(7).Height(28).AlignItems(ui.Center).Padding(4, 0, 0, 0).Children(func() {
		a.nowTransport(c, p, s, preview)
		if s.Duration <= 0 {
			ui.Box(c).Grow(1).Height(3).Radius(1.5).Background(p.line)
			return
		}
		pos := s.Position
		if s.Playing && !preview && !a.nowSnap.ReadAt.IsZero() {
			pos += time.Since(a.nowSnap.ReadAt)
		}
		if pos < 0 {
			pos = 0
		}
		if pos > s.Duration {
			pos = s.Duration
		}
		frac := float32(pos) / float32(s.Duration)
		ui.Text(c, formatClock(pos)).FontSize(10.5).TextColor(p.muted).FontFeatures("tnum").SingleLine()
		ui.Box(c).Grow(1).Height(3).Draw(func(pt *ui.Painter, r ui.Rect) {
			pt.Fill(r, p.line, 1.5)
			if frac > 0 {
				fill := ui.Rect{X: r.X, Y: r.Y, W: r.W * frac, H: r.H}
				if a.cfg.CoverColors {
					pt.FillGradient(fill, ui.LinearGradient{
						From:  fieldColor(field, 0, fieldH/2),
						To:    fieldColor(field, fieldW-1, fieldH/2),
						Angle: 90,
					}, 1.5)
				} else {
					pt.Fill(fill, p.fg, 1.5)
				}
			}
			x := r.X + r.W*frac
			thumb := ui.Rect{X: x - 4.5, Y: r.Y - 3, W: 9, H: 9}
			pt.Shadow(thumb, 4.5, 0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.18))
			pt.Fill(thumb, p.card, 4.5)
			pt.Stroke(thumb, p.line, 4.5, 1)
		})
		ui.Text(c, "-"+formatClock(s.Duration-pos)).FontSize(10.5).TextColor(p.muted).FontFeatures("tnum").SingleLine()
		if s.Playing && !preview {
			// The clock advances by itself while it plays, without waiting
			// for the two-second media poll. After coalesces, so a frame
			// that also animates the meter adds no timer.
			if d := time.Second - pos%time.Second; d > 0 {
				c.After(d)
			}
		}
	})
}

func (a *App) nowHasControls(s nowplaying.Session) bool {
	return s.AUMID != "" && (s.CanPlay || s.CanPause || s.CanNext || s.CanPrev)
}

func fieldColor(f *image.RGBA, x, y int) ui.Color {
	if f == nil {
		return ui.RGBA(128, 128, 128, 1)
	}
	b := f.Bounds()
	x = clampInt(x, b.Min.X, b.Max.X-1)
	y = clampInt(y, b.Min.Y, b.Max.Y-1)
	c := f.RGBAAt(x, y)
	return ui.RGBA(c.R, c.G, c.B, 1)
}

func (a *App) nowPlayingSubFor(s nowplaying.Session) string {
	switch {
	case a.nowErr != "":
		return locales.FlyoutNowPlayingSessionFailed()
	case s.Artist != "":
		return s.Artist
	case s.Album != "":
		return locales.FlyoutNowPlayingAlbum(s.Album)
	default:
		return locales.FlyoutNowPlayingSession()
	}
}

func formatClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d / time.Second)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func (a *App) flyoutFooter(c *ui.Context, p palette) {
	ph := phaseOf(a.snap)
	color := phaseColor(ph, p)
	ui.Row(c).Height(34).Padding(4, 12, 6, 14).Gap(6).Children(func() {
		ui.Icon(c, footerFace(a.snap)).Size(16, 16).TextColor(color)
		ui.Text(c, a.phaseLabel()).FontSize(11.5).TextColor(color).Key("flyout-status").Role(ui.RoleStatus).Grow(1).SingleLine().Ellipsis("…")
		if a.updateChipVisible() {
			chip := ui.ButtonBase(c).Height(22).Radius(11).Padding(0, 9).Center().Shrink(0).
				Cursor(ui.CursorPointer).Label(a.updateChipLabel())
			chip.Background(p.accentSoft)
			hoverFill(chip, p.accentSoft.Mix(p.accent, 0.1), 11, 0)
			chip.OnClick(func() { a.actUpdateSettings() })
			chip.Children(func() {
				ui.Text(c, a.updateChipLabel()).FontSize(11.5).FontWeight(500).TextColor(p.accent).SingleLine()
			})
		}
	})
}

// footerFace picks the flyout footer's state face, which follows the tray.
func footerFace(s engine.Snapshot) *ui.SVG {
	return faceIcon(faceOf(s))
}

func (a *App) syncText() string {
	if !a.snap.PTPActive {
		return locales.FlyoutSyncNTP()
	}
	if a.snap.PTPReady {
		return locales.FlyoutSyncPTPReady()
	}
	return locales.FlyoutSyncPTPPending()
}

func (a *App) phaseLabel() string {
	if a.snap.TakenOver {
		return locales.StatusTakenOver()
	}
	if phaseOf(a.snap) == phaseError {
		return locales.StatusConnectionFailed()
	}
	return a.statusText()
}

// currentDevice is the device the interface speaks for: the one connecting
// or streaming, else the one last chosen.
func (a *App) currentDevice() (device.Device, bool) {
	id := a.snap.SelectedID
	if id == "" {
		id = a.selID
	}
	for _, d := range a.snap.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return device.Device{}, false
}

func formatRate(rate int) string {
	switch {
	case rate%1000 == 0:
		return fmt.Sprintf("%d kHz", rate/1000)
	case rate%100 == 0:
		return fmt.Sprintf("%.1f kHz", float64(rate)/1000)
	}
	return fmt.Sprintf("%d Hz", rate)
}

func deviceNameOr(name, fallback string) string {
	if strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}

func (a *App) pinDialog(c *ui.Context, p palette) {
	if !a.snap.PinRequired && !a.pinOpen {
		return
	}
	modal := ui.DialogBase(c.Key("pin-dialog"), &a.pinOpen, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(380).Padding(20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.pinEntered)
		dialogHead(c, p, iconKey, locales.PinTitle(),
			locales.PinInstruction(deviceNameOr(a.snap.PinDevice, locales.PinFallbackDevice())), false, nil)
		ui.Text(c, locales.PinLabel()).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted).Margin(16, 0, 6, 0)
		submitted := ui.TextInput(c.Key("pin-code"), &a.pinCode).Height(46).Radius(8).PaddingX(12).FontSize(13).
			Label(locales.PinLabel()).AutoFocus().Submitted()
		ui.Text(c, locales.PinHint()).FontSize(11.5).TextColor(p.muted).Margin(6, 0, 0, 0)
		ui.Row(c).Justify(ui.End).Gap(8).Margin(20, 0, 0, 0).Children(func() {
			quietButton(c, p, locales.ActionCancel(), func() { a.cancelPin() })
			primaryButton(c, p, locales.PinSubmit(), func() { a.submitPin() })
		})
		if submitted {
			a.submitPin()
		}
	})
	if modal.Dismissed() {
		a.cancelPin()
	}
}

func (a *App) passwordDialog(c *ui.Context, p palette) {
	if !a.snap.PasswordRequired && !a.passwordOpen {
		return
	}
	modal := ui.DialogBase(c.Key("password-dialog"), &a.passwordOpen, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(380).Padding(20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.passwordEntered)
		dialogHead(c, p, iconKey, locales.PasswordTitle(),
			locales.PasswordInstruction(deviceNameOr(a.snap.PasswordDevice, locales.PasswordFallbackDevice())), false, nil)
		ui.Text(c, locales.PasswordLabel()).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted).Margin(16, 0, 6, 0)
		submitted := ui.TextInput(c.Key("password-input"), &a.passwordPrompt).Height(46).Radius(8).PaddingX(12).FontSize(13).
			Label(locales.PasswordLabel()).Password().AutoFocus().Submitted()
		ui.Text(c, locales.PasswordHint()).FontSize(11.5).TextColor(p.muted).Margin(6, 0, 0, 0)
		ui.Row(c).Justify(ui.End).Gap(8).Margin(20, 0, 0, 0).Children(func() {
			quietButton(c, p, locales.ActionCancel(), func() { a.cancelPassword() })
			primaryButton(c, p, locales.ActionConnect(), func() { a.submitPassword() })
		})
		if submitted {
			a.submitPassword()
		}
	})
	if modal.Dismissed() {
		a.cancelPassword()
	}
}
