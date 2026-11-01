package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/engine"
)

type settingsPage uint8

const (
	pageGeneral settingsPage = iota
	pageConnection
	pageAudio
	pageCapture
)

var settingsNav = []struct {
	page settingsPage
	icon *ui.SVG
}{
	{pageGeneral, iconSliders},
	{pageConnection, iconLink},
	{pageAudio, iconWave},
	{pageCapture, iconScreen},
}

func pageTitle(page settingsPage) string {
	switch page {
	case pageConnection:
		return locales.SettingsNavConnection()
	case pageAudio:
		return locales.SettingsNavAudio()
	case pageCapture:
		return locales.SettingsNavCapture()
	}
	return locales.SettingsNavGeneral()
}

// showSettings opens the configuration window. The flyout keeps the playback
// controls, so it hides while the user works in this window.
func (a *App) showSettings() {
	if a.settingsWin == nil {
		return
	}
	if a.win != nil {
		a.win.Hide()
	}
	if a.settingsWin.IsMinimized() {
		a.settingsWin.Restore()
	}
	a.settingsWin.Show()
	a.settingsWin.Focus()
	mygo.App.Focus()
}

// hideSettings hides the settings window; the app stays in the tray.
func (a *App) hideSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Hide()
	}
}

// newSettingsWindow builds the configuration window hidden; showSettings
// reveals it. Closing it hides it, and the app keeps running in the tray.
func (a *App) newSettingsWindow() *mygo.Window {
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     locales.SettingsWindowTitle(),
		Width:     720,
		Height:    500,
		MinWidth:  640,
		MinHeight: 420,
		Hidden:    true,
		StateKey:  "settings",
		// Fills the window before the first frame paints, so a dark window
		// does not flash white as it opens.
		BackgroundColor: "light-dark(#f4f4f6, #1a1a1e)",
		// The view draws the title bar so it blends with the window's
		// background; the system keeps its buttons over it.
		TitleBarStyle: mygo.TitleBarHidden,
		Content:       ui.View(a.settingsView),
	})
	w.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		w.Hide()
	})
	w.OnShow(func() { a.settingsEntered = false })
	return w
}

func (a *App) settingsView(c *ui.Context) {
	p := design(c)
	c.OnShortcut(ui.Cmd, ui.KeyW, a.hideSettings)
	ui.Column(c).Fill().Background(p.bg).Children(func() {
		a.settingsTitleBar(c, p)
		ui.Row(c).Grow(1).Children(func() {
			a.settingsSidebar(c, p)
			a.settingsBody(c, p)
		})
	})
}

// settingsTitleBar draws the window's title bar under the system's window
// buttons, which take the band TitleBar reports. It is the window's handle.
func (a *App) settingsTitleBar(c *ui.Context, p palette) {
	bar := c.TitleBar()
	title := ui.Row(c).Height(max(bar.Height, 32)).
		Padding(0, bar.Right, 0, 12+bar.Left).
		Gap(8).Background(p.panel).BorderWidth(0, 0, 1, 0).BorderColor(p.line2).
		DragWindow()
	title.Children(func() {
		ui.Icon(c, iconMark).Size(14, 14).TextColor(p.fg)
		ui.Text(c, locales.SettingsWindowTitle()).FontSize(11.5).TextColor(p.fg2).SingleLine()
	})
}

func (a *App) settingsSidebar(c *ui.Context, p palette) {
	name, sub, dot := a.sidebarIdentity(p)
	ui.Column(c).Width(190).FillHeight().Background(p.panel).Padding(12, 10, 10, 10).
		BorderWidth(0, 1, 0, 0).BorderColor(p.line2).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			for _, item := range settingsNav {
				a.sidebarItem(c, p, item.page, item.icon, pageTitle(item.page))
			}
		})
		ui.Spacer(c)
		ui.Box(c).Height(1).Background(p.line2)
		ui.Row(c).Gap(8).Margin(10, 6, 0, 6).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Shrink(0).Background(dot)
			ui.Column(c).Gap(1).Grow(1).Children(func() {
				ui.Text(c, name).FontSize(11.5).SingleLine().Ellipsis("…")
				ui.Text(c, sub).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			})
		})
	})
}

func (a *App) sidebarItem(c *ui.Context, p palette, page settingsPage, icon *ui.SVG, label string) {
	on := a.page == page
	e := ui.ButtonBase(c.Key(page)).Height(32).Padding(0, 9).Gap(8).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(label).
		Transition(ui.ElementTransition{Colors: true, Duration: 140 * time.Millisecond})
	switch {
	case on && p.dark:
		e.Background(ui.RGBA(255, 255, 255, 0.15))
	case on:
		e.Background(p.card)
		e.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.14))
	case e.Hovered():
		e.Background(p.pill)
	}
	e.OnClick(func() { a.page = page })
	e.Children(func() {
		ic := ui.Icon(c, icon).Size(15, 15)
		tx := ui.Text(c, label).FontSize(12.5).SingleLine()
		if on {
			ic.TextColor(p.fg)
			tx.TextColor(p.fg).FontWeight(500)
		} else {
			ic.TextColor(p.muted)
			tx.TextColor(p.ctl)
		}
	})
}

func (a *App) settingsBody(c *ui.Context, p palette) {
	title, desc := pageCopy(a.page)
	body := ui.Column(c).Grow(1).FillHeight().Padding(14, 20, 10, 20)
	enterPose(c, body, &a.settingsEntered)
	body.Children(func() {
		ui.Text(c, title).FontSize(18).FontWeight(650)
		ui.Text(c, desc).FontSize(13).TextColor(p.muted).Margin(4, 0, 0, 0)
		content := ui.Column(c).Key(a.page).Grow(1).Transition(ui.ElementTransition{
			Duration: 160 * time.Millisecond,
			Enter:    &ui.Motion{X: -6, Opacity: 0},
			Exit:     &ui.Motion{X: 6, Opacity: 0},
		})
		content.Children(func() {
			switch a.page {
			case pageGeneral:
				a.generalPage(c, p)
			case pageConnection:
				a.connectionPage(c, p)
			case pageAudio:
				a.audioPage(c, p)
			case pageCapture:
				a.capturePage(c, p)
			}
		})
	})
}

func pageCopy(page settingsPage) (title, desc string) {
	switch page {
	case pageConnection:
		return pageTitle(page), locales.PageConnectionDesc()
	case pageAudio:
		return pageTitle(page), locales.PageAudioDesc()
	case pageCapture:
		return pageTitle(page), locales.PageCaptureDesc()
	}
	return pageTitle(page), locales.PageGeneralDesc()
}

func pageCard(c *ui.Context, p palette, body func()) {
	card := ui.Column(c).Margin(12, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip()
	card.Children(body)
}

func (a *App) generalPage(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		settingRow(c, p, locales.SettingsSenderName(), locales.SettingsSenderNameDesc(), func() {
			ui.Row(c).Gap(6).Children(func() {
				if fieldInput(c, p, &a.nameDraft, locales.AppName(), locales.SettingsSenderName()).Submitted() {
					a.eng.SetName(a.nameDraft)
				}
				chipButton(c, p, locales.SettingsSave(), func() { a.eng.SetName(a.nameDraft) })
			})
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsLaunchAtStart(), locales.SettingsLaunchAtStartDesc(), func() {
			toggle(c, p, &a.launchAtStart, locales.SettingsLaunchAtStart(), func(v bool) { a.setLaunchAtStart(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsLanguage(), locales.SettingsLanguageDesc(), func() {
			choice := a.languageLabel()
			options := []string{locales.LanguageSystem(), locales.LanguageEnglish(), locales.LanguageChinese()}
			if fieldSelect(c, p, &choice, options, locales.SettingsLanguage()).Changed() {
				a.setLanguage(choice)
			}
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsAbout(), locales.SettingsAboutBrand(), func() {
			staticText(c, p, Version)
		})
	})
}

func (a *App) connectionPage(c *ui.Context, p palette) {
	if d, ok := a.currentDevice(); ok {
		a.deviceSummary(c, p, d)
	}
	pageCard(c, p, func() {
		settingRow(c, p, locales.SettingsAutoConnect(), locales.SettingsAutoConnectDesc(), func() {
			toggle(c, p, &a.autoConn, locales.SettingsAutoConnect(), func(v bool) { a.eng.SetAutoConnect(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsReconnect(), locales.SettingsReconnectDesc(), func() {
			toggle(c, p, &a.reconnect, locales.SettingsReconnect(), func(v bool) { a.eng.SetReconnect(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsStrictAuth(), locales.SettingsStrictAuthDesc(), func() {
			toggle(c, p, &a.strict, locales.SettingsStrictAuth(), func(v bool) { a.eng.SetStrictAuth(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsReceiverPassword(), a.passwordTargetText(), func() {
			ui.Row(c).Gap(6).Children(func() {
				field := fieldInput(c, p, &a.passwordDraft, locales.SettingsReceiverPasswordDesc(), locales.SettingsReceiverPassword()).Password()
				if field.Submitted() {
					a.saveReceiverPassword()
				}
				chipButton(c, p, locales.SettingsSave(), func() { a.saveReceiverPassword() })
			})
		})
	})
}

func (a *App) audioPage(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		ui.Column(c).Padding(9, 14, 10, 14).Children(func() {
			ui.Row(c).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, locales.SettingsLatency()).FontSize(13).FontWeight(500)
					ui.Text(c, locales.SettingsLatencyDesc()).FontSize(11.5).
						TextColor(p.muted).SingleLine().Ellipsis("…")
				})
				ui.Text(c, locales.SettingsLatencyValue(int(a.latency+0.5))).FontSize(12).TextColor(p.muted)
			})
			slider(c, p, &a.latency, 20, 2000, locales.SettingsLatency(), func(v float64) {
				a.eng.SetLatency(int(v))
			}).Margin(4, 0, 0, 0)
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsAudioFormat(), locales.SettingsAudioFormatDesc(), func() {
			staticText(c, p, locales.AudioFormatValue())
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsSyncMode(), locales.SettingsSyncModeDesc(), func() {
			mode := locales.SettingsAuto()
			if phaseOf(a.snap) == phaseStreaming {
				mode = a.syncText()
			}
			staticText(c, p, mode)
		})
	})
}

func (a *App) capturePage(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		settingRow(c, p, locales.SettingsSystemOutput(), locales.SettingsSystemOutputDesc(), func() {
			if fieldSelect(c, p, &a.captureChoice, a.captureNames, locales.SettingsSystemOutput()).Changed() {
				a.applyCaptureEndpoint()
			}
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsAppCapture(), locales.SettingsAppCaptureDesc(), func() {
			if fieldSelect(c, p, &a.appChoice, a.appNames, locales.SettingsAppCapture()).Changed() {
				a.applyCaptureProcess()
			}
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsCaptureStatus(), locales.SettingsCaptureStatusDesc(), func() {
			ui.Row(c).Gap(6).Shrink(0).Children(func() {
				dot := p.faint
				switch {
				case a.snap.CaptureError != "":
					dot = p.red
				case a.snap.CaptureRunning:
					dot = p.green
				}
				ui.Box(c).Size(6, 6).Radius(3).Shrink(0).Background(dot)
				ui.Text(c, captureStatusText(a.snap)).FontSize(12).TextColor(p.muted).SingleLine()
			})
		})
	})
}

func (a *App) deviceSummary(c *ui.Context, p palette, d device.Device) {
	ph := phaseOf(a.snap)
	e := ui.Row(c).Height(54).Margin(12, 0, 0, 0).Padding(0, 14).Gap(10).
		Radius(12).Background(p.card).Border(1, p.line)
	e.Children(func() {
		deviceGlyph(c, p, d, false)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
			ui.Text(c, summarySub(d)).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
		switch ph {
		case phaseStreaming:
			chip(c, p, locales.SummaryConnected(), chipOK)
		case phaseConnecting:
			chip(c, p, locales.SummaryConnecting(), chipAccent)
		case phaseError:
			chip(c, p, locales.StatusConnectionFailed(), chipNeutral)
		default:
			chip(c, p, locales.StatusDisconnected(), chipNeutral)
		}
	})
}

func summarySub(d device.Device) string {
	if d.IsStereoGroup() {
		return locales.SummaryStereoGroup(len(d.Members))
	}
	if d.Model != "" {
		return locales.SummaryModelCurrentDevice(d.Model)
	}
	return locales.SummaryCurrentDevice()
}

func (a *App) sidebarIdentity(p palette) (name, sub string, dot ui.Color) {
	d, ok := a.currentDevice()
	name = locales.SidebarNoDevice()
	if ok {
		name = d.Name
	}
	group := ok && d.IsStereoGroup()
	switch phaseOf(a.snap) {
	case phaseStreaming:
		sub, dot = locales.SummaryConnected(), p.green
		if group {
			sub = locales.SidebarConnectedStereo()
		}
	case phaseConnecting:
		sub, dot = a.statusText(), p.accent
	case phaseError:
		sub, dot = locales.StatusConnectionFailed(), p.red
	default:
		sub, dot = locales.StatusDisconnected(), p.faint
	}
	return name, sub, dot
}

// applyCaptureEndpoint switches capture to the chosen output endpoint.
func (a *App) applyCaptureEndpoint() {
	for i, name := range a.captureNames {
		if name == a.captureChoice && i < len(a.captureIDs) {
			a.eng.SetCaptureEndpoint(a.captureIDs[i], a.captureChoice)
			return
		}
	}
}

// applyCaptureProcess narrows capture to one application.
func (a *App) applyCaptureProcess() {
	for i, name := range a.appNames {
		if name == a.appChoice && i < len(a.appIDs) {
			var pid uint32
			fmt.Sscanf(a.appIDs[i], "%d", &pid)
			a.eng.SetCaptureProcess(pid, name)
			return
		}
	}
}

func (a *App) saveReceiverPassword() {
	if a.selID == "" {
		return
	}
	a.eng.SetReceiverPassword(a.selID, strings.TrimSpace(a.passwordDraft))
}

// passwordTargetText names the device the receiver password is saved to.
func (a *App) passwordTargetText() string {
	for _, d := range a.snap.Devices {
		if d.ID == a.selID {
			return locales.SettingsPasswordSavedTo(d.Name)
		}
	}
	return locales.SettingsChooseDeviceFirst()
}

func captureStatusText(s engine.Snapshot) string {
	if s.CaptureError != "" {
		return locales.SettingsCaptureError()
	}
	if !s.CaptureRunning {
		return locales.SettingsCaptureNotRunning()
	}
	name := s.CaptureName
	if name == "" {
		name = locales.CaptureSystemDefault()
	}
	if s.CaptureRate > 0 {
		return name + " · " + formatRate(s.CaptureRate)
	}
	return name
}
