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
	pageUpdate
)

var settingsNav = []struct {
	page settingsPage
	icon *ui.SVG
}{
	{pageGeneral, iconSliders},
	{pageConnection, iconLink},
	{pageAudio, iconWave},
	{pageCapture, iconScreen},
	{pageUpdate, iconRefresh},
}

func pageTitle(page settingsPage) string {
	switch page {
	case pageConnection:
		return locales.SettingsNavConnection()
	case pageAudio:
		return locales.SettingsNavAudio()
	case pageCapture:
		return locales.SettingsNavCapture()
	case pageUpdate:
		return locales.SettingsNavUpdates()
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
	a.updateNotesDialog(c, p)
}

// settingsTitleBar draws the window's title bar under the system's window
// buttons, which take the band TitleBar reports. It is the window's handle.
func (a *App) settingsTitleBar(c *ui.Context, p palette) {
	bar := c.TitleBar()
	title := ui.Row(c).Height(max(bar.Height, 32)).
		Padding(0, bar.Right, 0, 12+bar.Left).
		Gap(8).Background(p.bg).BorderWidth(0, 0, 1, 0).BorderColor(p.line2).
		DragWindow()
	title.Children(func() {
		ui.Icon(c, iconMark).Size(14, 14).TextColor(p.fg)
		ui.Text(c, locales.SettingsWindowTitle()).FontSize(11.5).TextColor(p.fg2).SingleLine()
	})
}

func (a *App) settingsSidebar(c *ui.Context, p palette) {
	name, sub, dot := a.sidebarIdentity(p)
	ui.Column(c).Width(190).FillHeight().Background(p.bg).Padding(12, 10, 10, 10).
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
		if page == pageUpdate && a.updateChipVisible() {
			ui.Spacer(c)
			ui.Box(c).Size(6, 6).Radius(3).Shrink(0).Background(p.accent)
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
			case pageUpdate:
				a.updatePage(c, p)
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
	case pageUpdate:
		return pageTitle(page), locales.PageUpdatesDesc()
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
			staticText(c, p, a.currentVersion())
		})
	})
}

// updatePage is the settings page for versions and automatic checks.
func (a *App) updatePage(c *ui.Context, p palette) {
	a.updateSettingsCard(c, p)
	pageCard(c, p, func() {
		a.updateVersionRow(c, p)
		rowLine(c, p)
		settingRow(c, p, locales.SettingsAutoUpdate(), locales.SettingsAutoUpdateDesc(), func() {
			toggle(c, p, &a.autoUpdate, locales.SettingsAutoUpdate(), func(v bool) { a.setAutoUpdate(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsAutoDownload(), locales.SettingsAutoDownloadDesc(), func() {
			toggle(c, p, &a.autoDownload, locales.SettingsAutoDownload(), func(v bool) { a.setAutoDownload(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsChannel(), locales.SettingsChannelDesc(), func() {
			choice := a.channelLabel()
			options := []string{locales.SettingsChannelStable(), locales.SettingsChannelBeta()}
			if fieldSelect(c, p, &choice, options, locales.SettingsChannel()).Changed() {
				a.setBeta(choice == locales.SettingsChannelBeta())
			}
		})
	})
}

// updateSettingsCard is the S1 card: the update's state and actions at the
// top of the page, in place of the list.
func (a *App) updateSettingsCard(c *ui.Context, p palette) {
	if !a.updateCardVisible() {
		return
	}
	card := ui.Column(c).Margin(12, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line)
	card.Children(func() {
		ui.Column(c).Padding(12, 14).Gap(8).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				ui.Box(c).Size(34, 34).Radius(9).Center().Shrink(0).Background(p.accentSoft).Children(func() {
					ui.Icon(c, iconDownload).Size(16, 16).TextColor(p.accent)
				})
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, a.updateCardTitle()).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
					ui.Text(c, a.updateCardDesc()).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
				})
				a.updateCardPrimary(c, p)
			})
			// The secondary actions line up with the title, past the icon
			// and the link's own 8-DIP padding.
			ui.Row(c).Gap(12).Padding(0, 0, 0, 36).Children(func() {
				linkButton(c, p, locales.UpdateFullNotes(), p.accent, func() { a.openUpdateNotes() })
				switch a.upd.phase {
				case updateAvailable:
					if !a.updateSkipped() {
						linkButton(c, p, locales.UpdateSkip(), p.muted, func() { a.skipUpdate() })
					}
				case updateReady:
					linkButton(c, p, locales.UpdateLater(), p.muted, func() { a.hideSettings() })
				}
			})
			if a.upd.phase == updateDownloading || a.upd.phase == updateReady {
				updateProgressBar(c, p, float32(a.updateProgress()))
				if a.upd.phase == updateDownloading {
					ui.Row(c).Children(func() {
						ui.Text(c, updateBytes(a.upd.got, a.upd.total)).FontSize(11.5).TextColor(p.muted).Grow(1)
						ui.Textf(c, "%d%%", int(a.updateProgress()*100)).FontSize(11.5).TextColor(p.fg)
					})
				}
			}
		})
	})
}

// updateCardVisible reports whether the S1 card has something to say.
func (a *App) updateCardVisible() bool {
	switch a.upd.phase {
	case updateAvailable, updateDownloading, updateReady:
		return true
	case updateFailed:
		return a.upd.up != nil && !a.upd.disabled
	}
	return false
}

func (a *App) updateCardTitle() string {
	switch a.upd.phase {
	case updateDownloading:
		return locales.UpdateDownloadingTitle(a.upd.version)
	case updateReady:
		return locales.UpdateReadyTitle(a.upd.version)
	case updateFailed:
		return locales.UpdateDownloadFailedTitle()
	}
	return locales.UpdateAvailableTitle(a.upd.version)
}

func (a *App) updateCardDesc() string {
	switch a.upd.phase {
	case updateDownloading:
		return locales.UpdateDownloadingMeta()
	case updateReady:
		return locales.UpdateReadyDesc()
	case updateFailed:
		return a.upd.err
	}
	return locales.UpdateAvailableMeta(a.currentVersion(), updateDate(a.upd.date))
}

// updateCardPrimary is the card's one filled action.
func (a *App) updateCardPrimary(c *ui.Context, p palette) {
	switch a.upd.phase {
	case updateDownloading:
		quietSmall(c, p, locales.UpdateCancel(), func() { a.cancelInstall() })
	case updateReady:
		primarySmall(c, p, locales.UpdateRestartNow(), func() { a.relaunchIntoUpdate() })
	case updateFailed:
		primarySmall(c, p, locales.UpdateRetry(), func() { a.installUpdate() })
	default:
		primarySmall(c, p, locales.UpdateInstall(), func() { a.installUpdate() })
	}
}

// primarySmall is the card's 30-DIP variant of the ink button.
func primarySmall(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(30).Radius(15).Padding(0, 14).Center().Cursor(ui.CursorPointer).Label(label)
	b.Background(p.fg)
	if b.Hovered() {
		b.Background(p.fg.Mix(p.bg, 0.12))
	}
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).FontWeight(500).TextColor(p.bg).SingleLine()
	})
}

// updateVersionRow shows the running version and the check's result in
// place: checking, up to date, or the failure, with the action beside it.
func (a *App) updateVersionRow(c *ui.Context, p palette) {
	ui.Row(c).MinHeight(56).Padding(8, 14).Gap(12).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, locales.SettingsCurrentVersion()).FontSize(13).FontWeight(500).SingleLine()
			a.updateVersionSub(c, p)
		})
		a.updateVersionAction(c, p)
	})
}

func (a *App) updateVersionSub(c *ui.Context, p palette) {
	if a.upd.disabled {
		ui.Text(c, locales.UpdateDisabledTitle()).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		return
	}
	ui.Row(c).Gap(5).Children(func() {
		ui.Text(c, a.currentVersion()+" · "+a.channelLabel()).FontSize(11.5).TextColor(p.muted).SingleLine()
		switch {
		case a.upd.phase == updateFailed && a.upd.up == nil:
			ui.Text(c, locales.SettingsCheckFailed()+" · "+a.upd.err).FontSize(11.5).TextColor(p.red).SingleLine().Ellipsis("…")
		case a.upd.checked:
			ok := ui.Row(c).Gap(3).Shrink(0)
			ok.Children(func() {
				ui.Icon(c, iconCheck).Size(11, 11).TextColor(p.green)
				ui.Text(c, locales.UpdateUpToDateShort()).FontSize(11.5).TextColor(p.green).SingleLine()
			})
		}
	})
}

func (a *App) updateVersionAction(c *ui.Context, p palette) {
	if a.upd.disabled {
		return
	}
	if a.upd.phase == updateChecking {
		busy := ui.Row(c).Height(26).Radius(13).Padding(0, 10).Gap(6).Center().Shrink(0).Background(p.pill)
		busy.Children(func() {
			ui.Spinner(c).Size(12, 12).TextColor(p.muted)
			ui.Text(c, locales.UpdateCheckingShort()).FontSize(11.5).TextColor(p.muted).SingleLine()
		})
		return
	}
	label := locales.SettingsCheckNow()
	if a.upd.phase == updateFailed && a.upd.up == nil {
		label = locales.UpdateRetry()
	}
	chipButton(c, p, label, func() { a.checkForUpdates(true) })
}

func (a *App) channelLabel() string {
	if a.cfg.Beta {
		return locales.SettingsChannelBeta()
	}
	return locales.SettingsChannelStable()
}

// setAutoUpdate saves the automatic-check preference.
func (a *App) setAutoUpdate(on bool) {
	a.autoUpdate = on
	a.cfg.AutoUpdate = on
	a.saveSettings()
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
