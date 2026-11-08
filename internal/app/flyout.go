package app

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/audio/analyze"
	"saundo/internal/engine"
	"saundo/internal/lamp"
	"saundo/internal/nowplaying"
	"saundo/internal/settings"
)

const (
	// flyoutHeight is the height with three device rows, no latency row, no
	// media card, and no error bar; the no-media panel is compact and the
	// media card adds its own height when a session exists.
	flyoutWidth  = 420
	flyoutHeight = 459

	// errorBarHeight is the connection error bar under the device card.
	errorBarHeight = 48

	// The media card fills the space the old 62-DIP Now Playing row used, and
	// adds its own height when shown.
	nowPlayingRowHeight  = 62
	nowPlayingCardHeight = 150

	// The device card shows one row per device, up to deviceRows, and the
	// empty state's box when there is none.
	deviceRows        = 3
	deviceRowHeight   = 56
	deviceEmptyHeight = 141

	// latencySwitchHeight is the four-segment row and its divider.
	latencySwitchHeight = 44
)

// deviceCardHeight is the height of the device card for n devices.
func deviceCardHeight(n int) float32 {
	if n <= 0 {
		return deviceEmptyHeight
	}
	return float32(min(n, deviceRows))*deviceRowHeight + 2
}

// flyoutHeightFor is the window height for the visible device count, the
// current media state, and the permanent ambient section. The device card and
// the playback card touch through the body's gap; there is no flexible
// spacer. A shown media card adds its height, and the no-media panel is
// shorter by the same amount. The ambient section is always present.
func flyoutHeightFor(n int, showLatencySwitch, hasMedia bool) int {
	h := flyoutHeight - int(deviceCardHeight(deviceRows)-deviceCardHeight(n)) - nowPlayingRowHeight
	if hasMedia {
		h += nowPlayingCardHeight
	}
	if showLatencySwitch {
		h += latencySwitchHeight
	}
	h += lampSectionHeight
	return h
}

// flyoutHeightForState adds the error bar and the template grid while they
// are shown. The window is anchored by its bottom edge, so this grows it
// upward.
func flyoutHeightForState(n int, showLatencySwitch, hasMedia, lampCover, lampExpanded, errorBar bool) int {
	h := flyoutHeightFor(n, showLatencySwitch, hasMedia)
	if errorBar {
		h += errorBarHeight
	}
	if !lampCover && lampExpanded {
		h += lampGridHeight(len(lamp.All()))
	}
	return h
}

func (a *App) view(c *ui.Context) {
	p := design(c)
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
	a.logDialog(c, p)
	a.pinDialog(c, p)
	a.passwordDialog(c, p)
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

func (a *App) deviceSection(c *ui.Context, p palette) {
	devices := a.snap.Devices
	sectionHead(c, p, locales.FlyoutDevices(), locales.FlyoutDeviceCount(len(devices)), false, nil)
	a.deviceCard(c, p, devices)
	if a.errorBarShown() {
		ui.Row(c).Margin(8, 0, 0, 0).Padding(8, 10).Radius(8).Background(p.redSoft).Children(func() {
			ui.Text(c, locales.FlyoutConnectFailed(a.snap.Error)).FontSize(11.5).TextColor(p.red).SingleLine().Ellipsis("…")
		})
	}
}

// errorBarShown reports whether the device section carries the connection
// error bar.
func (a *App) errorBarShown() bool {
	return phaseOf(a.snap) == phaseError && a.snap.Error != ""
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
		if b.Hovered() {
			b.Background(p.pill)
		}
		b.Children(func() {
			ui.Text(c, locales.FlyoutSearchAgain()).FontSize(12.5).TextColor(p.accent).SingleLine()
		})
	})
}

func (a *App) deviceRow(c *ui.Context, p palette, d device.Device, divider bool) {
	ph := phaseOf(a.snap)
	selected := d.ID == a.selID
	active := d.ID == a.snap.SelectedID && ph != phaseIdle

	row := ui.Row(c).Height(56).Padding(9, 12).Gap(10).
		Key(d.ID).Transition(ui.ElementTransition{Colors: true, Duration: 140 * time.Millisecond})
	if divider {
		row.BorderWidth(0, 0, 1, 0).BorderColor(p.line2)
	}
	switch {
	case selected:
		row.Background(p.sel)
	case row.Hovered():
		row.Background(p.cardHover)
	default:
		row.Background(ui.Transparent)
	}

	row.Children(func() {
		pick := ui.ButtonBase(c.Key("device:" + d.ID)).Grow(1).Gap(10).
			AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(d.Name)
		pick.OnClick(func() { a.selID = d.ID })
		pick.Children(func() {
			deviceGlyph(c, p, d, selected)

			ui.Column(c).Grow(1).Gap(1).Children(func() {
				ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
				ui.Text(c, deviceSubtitle(d)).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			})
		})

		a.rowAction(c, p, d, ph, active)
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
		ui.Icon(c, deviceIcon(d.Model)).Size(16, 16).TextColor(color)
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

func (a *App) rowAction(c *ui.Context, p palette, d device.Device, ph phase, active bool) {
	switch {
	case active && ph == phaseConnecting:
		textButton(c, p, locales.ActionCancel(), nil, p.ctl, func() { a.disconnect() })
	case active && ph == phaseError:
		textButton(c, p, locales.ActionRetry(), iconRefresh, p.accent, func() { a.connectDevice(d.ID) })
	case active:
		textButton(c, p, locales.ActionDisconnect(), iconUnlink, p.fg2, func() { a.disconnect() })
	case !d.Online:
		textButton(c, p, locales.ActionOffline(), iconSlash, p.muted, nil)
	case ph != phaseIdle:
		textButton(c, p, locales.ActionConnect(), iconLink, p.faint, nil)
	default:
		textButton(c, p, locales.ActionConnect(), iconLink, p.accent, func() { a.connectDevice(d.ID) })
	}
}

func (a *App) playbackSection(c *ui.Context, p palette) {
	note := locales.FlyoutFormat()
	if phaseOf(a.snap) == phaseStreaming {
		note += " · " + a.syncText()
	}
	note += " · " + locales.FlyoutLatency(a.latencyDisplay())
	sectionHead(c, p, locales.FlyoutPlayback(), note, a.latencyApplying, nil)

	card := ui.Column(c).Radius(10).Background(p.card).Border(1, p.line).Clip()
	card.Children(func() {
		ui.Column(c).Padding(10, 12, 10, 12).Children(func() {
			ui.Row(c).Children(func() {
				ui.Text(c, locales.VolumeLabel()).FontSize(13).FontWeight(500)
				ui.Spacer(c)
				ui.Text(c, locales.FlyoutVolumeValue(a.volume)).FontSize(11.5).TextColor(p.muted)
			})
			slider(c, p, &a.volume, 0, 100, locales.VolumeLabel(), func(v float64) { a.eng.SetVolume(v) }).Margin(5, 0, 0, 0)
		})
		rowLine(c, p)
		if a.showLatencySwitch {
			ui.Column(c).Padding(6, 12, 10, 12).Children(func() { a.latencySwitch(c, p) })
			rowLine(c, p)
		}
		a.nowPlayingCard(c, p)
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
	track := segmented(c, p, &seg, items, 0, false, locales.SettingsLatency())
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
	ui.Column(c).Margin(8, 12, 10, 12).Padding(8, 10, 8, 10).Radius(12).Background(p.pill).Children(func() {
		ui.Row(c).Height(28).Gap(7).AlignItems(ui.Center).Children(func() {
			name := s.AppName
			if name == "" {
				name = locales.FlyoutUnknownApp()
			}
			ui.Text(c, name).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…").Grow(1)
			if showSwitch {
				btn := ui.ButtonBase(c).Size(24, 24).Radius(7).Center().Background(p.card).
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
					ui.Icon(c, iconChevronRight).Size(12, 12).TextColor(col)
				})
			}
		})
		ui.Row(c).Height(62).Gap(10).AlignItems(ui.Center).Padding(6, 0, 4, 0).Children(func() {
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

// nowPlayingPreview is the settings page's card. It shows the live session,
// or a sample card when nothing plays, so the preview never disappears; the
// master off dims it and the page disables its rows.
func (a *App) nowPlayingPreview(c *ui.Context, p palette) {
	s, ok := a.nowSession()
	var art *ui.Bitmap
	var light, dark *image.RGBA
	level := func() analyze.Frame { return analyze.Frame{} }
	if ok {
		art, light, dark = a.nowArt, a.nowFieldLight, a.nowFieldDark
		delay := time.Duration(a.latencyMS()) * time.Millisecond
		level = func() analyze.Frame {
			if a.levelFrame != nil {
				return a.levelFrame(delay)
			}
			return analyze.Frame{}
		}
	} else {
		s, art, light, dark, level = previewSession()
	}
	field := light
	if p.dark {
		field = dark
	}
	wrap := ui.Column(c)
	if !a.cfg.ShowNowPlayingPanel {
		wrap.Opacity(0.45)
	}
	wrap.Children(func() {
		a.nowPlayingCardBody(c, p, s, true, previewMeterAnimates(ok, a.cfg.ShowNowPlayingPanel), art, field, level, true)
	})
}

// previewSession is the settings page's sample card when nothing plays.
func previewSession() (nowplaying.Session, *ui.Bitmap, *image.RGBA, *image.RGBA, func() analyze.Frame) {
	sample := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			t := float64(x+y) / 126
			sample.SetRGBA(x, y, color.RGBA{
				R: uint8(135 + (47-135)*t),
				G: uint8(121 + (142-121)*t),
				B: uint8(221 + (154-221)*t),
				A: 255,
			})
		}
	}
	s := nowplaying.Session{
		AUMID: "settings.preview", AppName: "Apple Music",
		Title:  locales.SettingsNowPlayingSampleTitle(),
		Artist: locales.SettingsNowPlayingSampleArtist(),
		Status: nowplaying.StatusPlaying, Playing: true,
		CanPlay: true, CanPause: true, CanPrev: true, CanNext: true,
		Position: 128 * time.Second, Duration: 244 * time.Second,
	}
	frame := analyze.Frame{Bands: [analyze.BandCount]float32{0.35, 0.6, 0.85, 0.55, 0.4}}
	return s, ui.NewBitmap(sample), coverField(sample, false), coverField(sample, true), func() analyze.Frame { return frame }
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

// nowTransport draws the enabled transport controls in the session arrow's
// card style: 24-DIP circles 4 DIPs apart, with the same fill and muted glyph.
func (a *App) nowTransport(c *ui.Context, p palette, s nowplaying.Session, preview bool) {
	if !a.cfg.PlaybackControls || !a.nowHasControls(s) {
		return
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
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
		Background(p.card).Cursor(ui.CursorPointer).Label(label).Disabled(preview)
	if btn.Hovered() {
		btn.Background(p.cardHover)
	}
	btn.OnClick(func() {
		if a.controlNow != nil {
			a.controlNow(aumid, action)
		}
	})
	btn.Children(func() {
		col := p.muted
		if btn.Hovered() {
			col = p.fg
		}
		ui.Icon(c, icon).Size(12, 12).TextColor(col)
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
// the live level only while a session plays and the feature is on.
func previewMeterAnimates(live, enabled bool) bool { return live && enabled }

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
		ui.Text(c, a.phaseLabel()).FontSize(11.5).TextColor(color).Grow(1).SingleLine().Ellipsis("…")
		if a.updateChipVisible() {
			chip := ui.ButtonBase(c).Height(22).Radius(11).Padding(0, 9).Center().Shrink(0).
				Cursor(ui.CursorPointer).Label(a.updateChipLabel())
			chip.Background(p.accentSoft)
			if chip.Hovered() {
				chip.Background(p.accentSoft.Mix(p.accent, 0.1))
			}
			chip.OnClick(func() {
				a.page = pageUpdate
				a.showSettings()
			})
			chip.Children(func() {
				ui.Text(c, a.updateChipLabel()).FontSize(11.5).FontWeight(500).TextColor(p.accent).SingleLine()
			})
		}
		b := ui.ButtonBase(c).Gap(4).Padding(3, 7).Radius(6).Cursor(ui.CursorPointer).Label(locales.LogTitle())
		b.OnClick(func() {
			a.showLog = true
			a.logEntered = false
		})
		if b.Hovered() {
			b.Background(p.pill)
		}
		b.Children(func() {
			ui.Icon(c, iconDoc).Size(13, 13).TextColor(p.ctl)
			ui.Text(c, locales.LogTitle()).FontSize(12.5).TextColor(p.ctl).SingleLine()
		})
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
	if phaseOf(a.snap) == phaseError {
		return locales.StatusConnectionFailed()
	}
	return a.statusText()
}

func (a *App) connectDevice(id string) {
	a.cancelLatencyApply()
	a.finishLatencyApply()
	a.selID = id
	a.eng.Connect(id)
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
	if rate%1000 == 0 {
		return fmt.Sprintf("%d kHz", rate/1000)
	}
	return fmt.Sprintf("%d Hz", rate)
}

func deviceNameOr(name, fallback string) string {
	if strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}

func (a *App) logDialog(c *ui.Context, p palette) {
	if !a.showLog {
		return
	}
	lines := a.snap.LogLines
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	ui.DialogBase(c, &a.showLog, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(430).Padding(20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.logEntered)
		dialogHead(c, p, iconDoc, locales.LogTitle(), locales.LogRecentLines(len(lines)), false, nil)
		ui.Scroll(c).Height(300).Margin(14, 0, 0, 0).Padding(10).Radius(10).Background(p.pill).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				if len(lines) == 0 {
					ui.Text(c, locales.LogEmpty()).FontSize(11.5).TextColor(p.muted)
				}
				for _, line := range lines {
					ui.Text(c, line).Font("Cascadia Mono").FontSize(10.5).TextColor(p.fg2)
				}
			})
		})
		ui.Row(c).Justify(ui.End).Gap(8).Margin(16, 0, 0, 0).Children(func() {
			quietButton(c, p, locales.LogCopy(), func() {
				c.Services().WriteClipboard(strings.Join(lines, "\n"))
			})
			primaryButton(c, p, locales.LogClose(), func() { a.showLog = false })
		})
	})
}

func (a *App) pinDialog(c *ui.Context, p palette) {
	if !a.snap.PinRequired && !a.pinOpen {
		return
	}
	modal := ui.DialogBase(c, &a.pinOpen, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(430).Padding(20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.pinEntered)
		dialogHead(c, p, iconKey, locales.PinTitle(),
			locales.PinInstruction(deviceNameOr(a.snap.PinDevice, locales.PinFallbackDevice())), false, nil)
		ui.Text(c, locales.PinLabel()).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted).Margin(16, 0, 6, 0)
		submitted := ui.TextInput(c, &a.pinCode).Height(46).Radius(8).PaddingX(12).FontSize(13).
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
	modal := ui.DialogBase(c, &a.passwordOpen, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(430).Padding(20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.passwordEntered)
		dialogHead(c, p, iconKey, locales.PasswordTitle(),
			locales.PasswordInstruction(deviceNameOr(a.snap.PasswordDevice, locales.PasswordFallbackDevice())), false, nil)
		ui.Text(c, locales.PasswordLabel()).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted).Margin(16, 0, 6, 0)
		submitted := ui.TextInput(c, &a.passwordPrompt).Height(46).Radius(8).PaddingX(12).FontSize(13).
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
