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
		iconButton(c, p, iconRefresh, locales.FlyoutRefreshDevices(), 14, func() { a.eng.Refresh() })
		iconButton(c, p, iconGear, locales.CommonSettings(), 15, func() { a.showSettings() })
	})
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
			chip.OnClick(func() {
				a.page = pageUpdate
				a.showSettings()
			})
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
