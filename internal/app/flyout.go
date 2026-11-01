package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/engine"
)

const (
	flyoutWidth  = 420
	flyoutHeight = 500

	// The device card shows one row per device, up to deviceRows, and the
	// empty state's box when there is none.
	deviceRows        = 3
	deviceRowHeight   = 56
	deviceEmptyHeight = 140
)

// deviceCardHeight is the height of the device card for n devices.
func deviceCardHeight(n int) float32 {
	if n <= 0 {
		return deviceEmptyHeight
	}
	return float32(min(n, deviceRows))*deviceRowHeight + 2
}

// flyoutHeightFor is the window height that keeps the spacing between the
// device card and the playback card the same for any device count: the
// design's height fits deviceRows rows, and each missing row, or the empty
// state, shortens the window by as much as the card lost.
func flyoutHeightFor(n int) int {
	return flyoutHeight - int(deviceCardHeight(deviceRows)-deviceCardHeight(n))
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
				ui.Spacer(c)
				a.playbackSection(c, p)
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
			ui.Text(c, locales.AppName()).FontSize(13.5).FontWeight(600).SingleLine()
			ui.Text(c, locales.AppSubtitle()).FontSize(8.5).FontWeight(600).LetterSpacing(1.1).TextColor(p.muted).SingleLine()
		})
		ui.Spacer(c)
		iconButton(c, p, iconRefresh, locales.FlyoutRefreshDevices(), 14, func() { a.eng.Refresh() })
		iconButton(c, p, iconGear, locales.CommonSettings(), 15, func() { a.showSettings() })
	})
}

func (a *App) deviceSection(c *ui.Context, p palette) {
	devices := a.snap.Devices
	sectionHead(c, p, locales.FlyoutDevices(), locales.FlyoutDeviceCount(len(devices)))
	a.deviceCard(c, p, devices)
	if phaseOf(a.snap) == phaseError && a.snap.Error != "" {
		ui.Row(c).Margin(8, 0, 0, 0).Padding(8, 10).Radius(8).Background(p.redSoft).Children(func() {
			ui.Text(c, locales.FlyoutConnectFailed(a.snap.Error)).FontSize(11.5).TextColor(p.red).SingleLine().Ellipsis("…")
		})
	}
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
		ui.Text(c, locales.FlyoutSearching()).FontSize(12.5).TextColor(p.fg2).TextAlign(ui.Center)
		ui.Text(c, locales.FlyoutSearchHint()).FontSize(11.5).
			TextColor(p.muted).TextAlign(ui.Center)
		b := ui.ButtonBase(c).Padding(2, 6).Radius(6).Cursor(ui.CursorPointer).Label(locales.FlyoutSearchAgain())
		b.OnClick(func() { a.eng.Refresh() })
		if b.Hovered() {
			b.Background(p.pill)
		}
		b.Children(func() {
			ui.Text(c, locales.FlyoutSearchAgain()).FontSize(12).TextColor(p.accent).SingleLine()
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
				ui.Text(c, d.Name).FontSize(13.5).FontWeight(500).SingleLine().Ellipsis("…")
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
		textButton(c, p, locales.ActionCancel(), nil, p.ctl, func() { a.eng.Disconnect() })
	case active && ph == phaseError:
		textButton(c, p, locales.ActionRetry(), iconRefresh, p.accent, func() { a.connectDevice(d.ID) })
	case active:
		textButton(c, p, locales.ActionDisconnect(), iconUnlink, p.fg2, func() { a.eng.Disconnect() })
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
	sectionHead(c, p, locales.FlyoutPlayback(), note)

	card := ui.Column(c).Radius(10).Background(p.card).Border(1, p.line).Clip()
	card.Children(func() {
		ui.Column(c).Padding(10, 12, 8, 12).Children(func() {
			ui.Row(c).Children(func() {
				ui.Text(c, locales.VolumeLabel()).FontSize(13).FontWeight(500)
				ui.Spacer(c)
				ui.Text(c, locales.FlyoutVolumeValue(a.volume)).FontSize(11.5).TextColor(p.muted)
			})
			slider(c, p, &a.volume, 0, 100, locales.VolumeLabel(), func(v float64) { a.eng.SetVolume(v) }).Margin(5, 0, 0, 0)
		})
		rowLine(c, p)
		a.nowPlayingRow(c, p)
		// The error bar already cost the room the capture line took.
		if phaseOf(a.snap) != phaseError {
			rowLine(c, p)
			ui.Row(c).Padding(5, 12, 6, 12).Children(func() {
				ui.Text(c, captureMeta(a.snap)).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			})
		}
	})
}

func (a *App) nowPlayingRow(c *ui.Context, p palette) {
	ui.Row(c).Height(62).Padding(9, 12).Gap(10).Children(func() {
		ui.Box(c).Size(34, 34).Radius(9).Center().Shrink(0).Background(p.accentSoft).Children(func() {
			ui.Icon(c, iconNote).Size(16, 16).TextColor(p.accent)
		})
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, a.nowPlayingText()).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
			ui.Text(c, a.nowPlayingSub()).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
		ui.Row(c).Gap(7).Children(func() {
			ui.Text(c, locales.FlyoutPush()).FontSize(11.5).TextColor(p.muted).SingleLine()
			toggle(c, p, &a.showNowPlaying, locales.FlyoutPushLabel(), func(v bool) { a.eng.SetShowNowPlaying(v) })
		})
	})
}

// nowPlayingSub is the line under the track in the playback card.
func (a *App) nowPlayingSub() string {
	switch {
	case a.nowErr != "":
		return locales.FlyoutNowPlayingSessionFailed()
	case !a.nowOK || a.nowTrack.Title == "":
		return locales.FlyoutNowPlayingSession()
	case a.nowTrack.Album != "":
		return locales.FlyoutNowPlayingAlbum(a.nowTrack.Album)
	}
	return locales.FlyoutNowPlaying()
}

func (a *App) flyoutFooter(c *ui.Context, p palette) {
	ph := phaseOf(a.snap)
	color := phaseColor(ph, p)
	ui.Row(c).Height(34).Padding(4, 12, 6, 14).Gap(6).Children(func() {
		ui.Icon(c, faceFor(ph)).Size(16, 16).TextColor(color)
		ui.Text(c, a.phaseLabel()).FontSize(11.5).TextColor(color).Grow(1).SingleLine().Ellipsis("…")
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
			ui.Text(c, locales.LogTitle()).FontSize(12).TextColor(p.ctl).SingleLine()
		})
	})
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

func captureMeta(s engine.Snapshot) string {
	if s.CaptureError != "" {
		return locales.FlyoutCaptureError(s.CaptureError)
	}
	if !s.CaptureRunning {
		return locales.FlyoutCaptureNotRunning()
	}
	name := s.CaptureName
	if name == "" {
		name = locales.CaptureSystemDefault()
	}
	out := locales.FlyoutCapture(name)
	if s.CaptureRate > 0 {
		out += " · " + formatRate(s.CaptureRate)
	}
	return out + " · " + locales.FlyoutDropped(int(s.DroppedSamples))
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
		back.Background(ui.RGBA(20, 22, 30, 0.34))
		panel.Width(380).Padding(20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.logEntered)
		ui.Text(c, locales.LogTitle()).FontSize(18).FontWeight(650)
		ui.Text(c, locales.LogRecentLines(len(lines))).FontSize(12).TextColor(p.muted).Margin(4, 0, 0, 0)
		ui.Scroll(c).Height(300).Margin(14, 0, 0, 0).Padding(10).Radius(10).Background(p.pill).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				if len(lines) == 0 {
					ui.Text(c, locales.LogEmpty()).FontSize(11).TextColor(p.muted)
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
		back.Background(ui.RGBA(20, 22, 30, 0.34))
		panel.Width(380).Padding(22).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.pinEntered)
		ui.Text(c, locales.PinTitle()).FontSize(18).FontWeight(650)
		ui.Text(c, locales.PinInstruction(deviceNameOr(a.snap.PinDevice, locales.PinFallbackDevice()))).
			FontSize(12.5).TextColor(p.muted).Margin(5, 0, 0, 0)
		ui.Text(c, locales.PinLabel()).FontSize(11).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted).Margin(16, 0, 6, 0)
		submitted := ui.TextInput(c, &a.pinCode).Height(46).Radius(8).PaddingX(12).FontSize(14).
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
		back.Background(ui.RGBA(20, 22, 30, 0.34))
		panel.Width(380).Padding(22).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.passwordEntered)
		ui.Text(c, locales.PasswordTitle()).FontSize(18).FontWeight(650)
		ui.Text(c, locales.PasswordInstruction(deviceNameOr(a.snap.PasswordDevice, locales.PasswordFallbackDevice()))).
			FontSize(12.5).TextColor(p.muted).Margin(5, 0, 0, 0)
		ui.Text(c, locales.PasswordLabel()).FontSize(11).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted).Margin(16, 0, 6, 0)
		submitted := ui.TextInput(c, &a.passwordPrompt).Height(46).Radius(8).PaddingX(12).FontSize(14).
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
