package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/engine"
	"saundo/internal/settings"
)

type settingsPage uint8

const (
	pageGeneral settingsPage = iota
	pageConnection
	pageReceivers
	pageAudio
	pageLamp
	pageCapture
	pageNowPlaying
	pageUpdate
	pageAbout
)

var settingsNav = []struct {
	page settingsPage
	icon *ui.SVG
}{
	{pageGeneral, iconSliders},
	{pageConnection, iconLink},
	{pageReceivers, iconSpeaker},
	{pageAudio, iconWave},
	{pageLamp, iconLamp},
	{pageCapture, iconScreen},
	{pageNowPlaying, iconNote},
	{pageUpdate, iconRefresh},
	{pageAbout, iconInfo},
}

func pageTitle(page settingsPage) string {
	switch page {
	case pageConnection:
		return locales.SettingsNavConnection()
	case pageReceivers:
		return locales.SettingsNavReceivers()
	case pageAudio:
		return locales.SettingsNavAudio()
	case pageLamp:
		return locales.SettingsNavLamp()
	case pageCapture:
		return locales.SettingsNavCapture()
	case pageNowPlaying:
		return locales.SettingsNavNowPlaying()
	case pageUpdate:
		return locales.SettingsNavUpdates()
	case pageAbout:
		return locales.SettingsAbout()
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
		Title: locales.SettingsWindowTitle(),
		// The design's 720×500 is the page area the view lays out in; the
		// window frame adds a few DIPs around it. The layout's narrow branch
		// reads that area, not the window, so the two must agree.
		Width:          720,
		Height:         500,
		UseContentSize: true,
		MinWidth:       640,
		MinHeight:      420,
		Hidden:         true,
		StateKey:       "settings",
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

// settingsColumnWidth caps the settings page's content column: on a wide
// window the title, cards, and notes stop at 720 DIP and center, so rows keep
// their rhythm instead of stretching. The default 720 window's column is 489,
// so the cap engages only above it.
const settingsColumnWidth = 720

func (a *App) settingsBody(c *ui.Context, p palette) {
	title, desc := pageCopy(a.page)
	// The body runs to the window edge so the overlay scrollbar sits there;
	// the header and cards keep their 20 DIP right margin, giving the bar a
	// gutter it cannot draw over. Both the header and the scrolling content
	// are capped to the same column and centered, so a wide window grows the
	// margins instead of stretching every row.
	body := ui.Column(c).Grow(1).FillHeight().Padding(14, 0, 10, 20)
	enterPose(c, body, &a.settingsEntered)
	body.Children(func() {
		ui.Column(c).Padding(0, 20, 0, 0).Children(func() {
			ui.Column(c).WidthPercent(100).MaxWidth(settingsColumnWidth).Margin(0, ui.Auto).Children(func() {
				ui.Text(c, title).FontSize(18).FontWeight(650)
				ui.Text(c, desc).FontSize(13).TextColor(p.muted).Margin(4, 0, 0, 0)
				ui.Box(c).Height(1).Background(p.line).Margin(10, 0, 0, 0)
			})
		})
		// The page body scrolls under the fixed title and divider so a short
		// window (the 420 DIP minimum) reaches the whole latency card and the
		// read-only rows.
		ui.Scroll(c).Grow(1).Padding(4, 20, 0, 0).Children(func() {
			content := ui.Column(c).Key(a.page).Shrink(0).WidthPercent(100).
				MaxWidth(settingsColumnWidth).Margin(0, ui.Auto).Transition(ui.ElementTransition{
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
				case pageReceivers:
					a.receiversPage(c, p)
				case pageAudio:
					a.audioPage(c, p)
				case pageLamp:
					a.lampPage(c, p)
				case pageCapture:
					a.capturePage(c, p)
				case pageNowPlaying:
					a.nowPlayingPage(c, p)
				case pageUpdate:
					a.updatePage(c, p)
				case pageAbout:
					a.aboutPage(c, p)
				}
			})
		})
	})
}

func pageCopy(page settingsPage) (title, desc string) {
	switch page {
	case pageConnection:
		return pageTitle(page), locales.PageConnectionDesc()
	case pageReceivers:
		return pageTitle(page), locales.PageReceiversDesc()
	case pageAudio:
		return pageTitle(page), locales.PageAudioDesc()
	case pageLamp:
		return pageTitle(page), locales.PageLampDesc()
	case pageCapture:
		return pageTitle(page), locales.PageCaptureDesc()
	case pageNowPlaying:
		return pageTitle(page), locales.PageNowPlayingDesc()
	case pageUpdate:
		return pageTitle(page), locales.PageUpdatesDesc()
	case pageAbout:
		return pageTitle(page), locales.PageAboutDesc()
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
		settingRow(c, p, locales.SettingsTrayAnimation(), locales.SettingsTrayAnimationDesc(), func() {
			toggle(c, p, &a.trayAnimation, locales.SettingsTrayAnimation(), func(v bool) { a.setTrayAnimation(v) })
		})
		if flyoutStyleSetting {
			rowLine(c, p)
			settingRow(c, p, locales.SettingsFlyoutPopover(), locales.SettingsFlyoutPopoverDesc(), func() {
				toggle(c, p, &a.flyoutPopover, locales.SettingsFlyoutPopover(), func(v bool) { a.setFlyoutPopover(v) })
			})
		}
		rowLine(c, p)
		settingRow(c, p, locales.SettingsLanguage(), locales.SettingsLanguageDesc(), func() {
			choice := a.languageLabel()
			options := []string{locales.LanguageSystem(), locales.LanguageEnglish(), locales.LanguageChinese()}
			if fieldSelect(c, p, &choice, options, locales.SettingsLanguage()).Changed() {
				a.setLanguage(choice)
			}
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsTheme(), locales.SettingsThemeDesc(), func() {
			choice := a.themeLabel()
			options := []string{locales.ThemeSystem(), locales.ThemeLight(), locales.ThemeDark()}
			if fieldSelect(c, p, &choice, options, locales.SettingsTheme()).Changed() {
				a.setTheme(choice)
			}
		})
	})
}

// nowPlayingPage is the settings page for the panel's media card. The display
// switch only controls the flyout card; the options below it stay active.
func (a *App) nowPlayingPage(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		cardHead(c, p, iconNote, locales.SettingsShowNowPlaying(), 13, 14, func() {
			chip(c, p, locales.SettingsShowInPanel(), chipNeutral)
			toggle(c, p, &a.cfg.ShowNowPlayingPanel, locales.SettingsShowInPanel(), func(v bool) {
				a.showNowPlayingPanel.Store(v)
				a.persistNowPlaying()
			})
		})
		a.nowPlayingPreview(c, p)
		rowLine(c, p)
		a.nowPlayingCoverRow(c, p)
		rowLine(c, p)
		a.nowPlayingOptionRow(c, p, locales.SettingsPlaybackControls(), locales.SettingsPlaybackControlsDesc(),
			&a.cfg.PlaybackControls, func(bool) { a.persistNowPlaying() })
		rowLine(c, p)
		a.nowPlayingOptionRow(c, p, locales.SettingsCoverColors(), locales.SettingsCoverColorsDesc(),
			&a.cfg.CoverColors, func(bool) { a.persistNowPlaying() })
		rowLine(c, p)
		a.nowPlayingOptionRow(c, p, locales.SettingsAudioLevel(), locales.SettingsAudioLevelDesc(),
			&a.cfg.AudioLevel, func(bool) { a.persistNowPlaying() })
	})
}

// nowPlayingCoverRow is the three-way cover control: none, the default
// placeholder, or the media's own artwork. The options read 无 / 默认 / 媒体
// under the 封面 title, so the whole choice fits the row's right-hand column.
func (a *App) nowPlayingCoverRow(c *ui.Context, p palette) {
	settingRow(c, p, locales.SettingsCover(), locales.SettingsCoverDesc(), func() {
		seg := a.coverModeIndex()
		items := []segmentItem{
			{locales.SettingsCoverNone(), iconSlash},
			{locales.SettingsCoverPlaceholder(), iconNote},
			{locales.SettingsCoverMedia(), iconImage},
		}
		if segmented(c, p, &seg, items, 200, false, locales.SettingsCover()).Changed() {
			a.cfg.CoverMode = coverModeAt(seg)
			a.persistNowPlaying()
		}
	})
}

func (a *App) coverModeIndex() int {
	switch a.cfg.CoverMode {
	case settings.CoverNone:
		return 0
	case settings.CoverPlaceholder:
		return 1
	default:
		return 2
	}
}

func coverModeAt(i int) settings.CoverMode {
	switch i {
	case 0:
		return settings.CoverNone
	case 1:
		return settings.CoverPlaceholder
	default:
		return settings.CoverMedia
	}
}

func (a *App) nowPlayingOptionRow(c *ui.Context, p palette, title, sub string, on *bool, changed func(bool)) {
	settingRow(c, p, title, sub, func() {
		toggle(c, p, on, title, changed)
	})
}

// persistNowPlaying stores the panel display options and resizes the flyout.
func (a *App) persistNowPlaying() {
	a.showNowPlayingPanel.Store(a.cfg.ShowNowPlayingPanel)
	if a.sets != nil {
		cfg := a.sets.Load()
		cfg.ShowNowPlayingPanel = a.cfg.ShowNowPlayingPanel
		cfg.PlaybackControls = a.cfg.PlaybackControls
		cfg.CoverColors = a.cfg.CoverColors
		cfg.CoverMode = a.cfg.CoverMode
		cfg.AudioLevel = a.cfg.AudioLevel
		_ = a.sets.Save(cfg)
	}
	a.fitFlyout()
	a.updateUI(func() {})
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
				ui.Box(c).Size(34, 34).Radius(10).Center().Shrink(0).Background(p.accentSoft).Children(func() {
					ui.Icon(c, iconDownload).Size(16, 16).TextColor(p.accent)
				})
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, a.updateCardTitle()).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…")
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
			if a.upd.phase == updateDownloading {
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
	return locales.UpdateAvailableMeta(a.currentVersionLabel(), updateDate(a.upd.date))
}

// updateCardPrimary is the card's one filled action.
func (a *App) updateCardPrimary(c *ui.Context, p palette) {
	switch a.upd.phase {
	case updateDownloading:
		linkButton(c, p, locales.UpdateCancel(), p.muted, func() { a.cancelInstall() })
	case updateReady:
		cardAction(c, p, locales.UpdateRestartNow(), func() { a.relaunchIntoUpdate() })
	case updateFailed:
		cardAction(c, p, locales.UpdateRetry(), func() { a.installUpdate() })
	default:
		cardAction(c, p, locales.UpdateInstall(), func() { a.installUpdate() })
	}
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
		ui.Text(c, a.currentVersionLabel()+" · "+a.channelLabel()).FontSize(11.5).TextColor(p.muted).SingleLine()
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
		settingRow(c, p, locales.SettingsReconnect(), locales.SettingsReconnectDesc(), func() {
			toggle(c, p, &a.reconnect, locales.SettingsReconnect(), func(v bool) { a.eng.SetReconnect(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsStrictAuth(), locales.SettingsStrictAuthDesc(), func() {
			toggle(c, p, &a.strict, locales.SettingsStrictAuth(), func(v bool) { a.eng.SetStrictAuth(v) })
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsMuteLocal(), a.muteLocalSub(), func() {
			toggle(c, p, &a.cfg.MuteLocalOnConnect, locales.SettingsMuteLocal(), a.setMuteLocal)
		})
	})
	if a.cfg.MuteLocalOnConnect {
		ui.Column(c).Margin(8, 0, 0, 2).Children(func() {
			infoNote(c, p, locales.SettingsMuteLocalNote())
		})
	}
}

// muteLocalSub follows the engine's effective state, not just the switch: on
// means muted now, waiting means it applies on the next connection, failed
// means the endpoint refused the call.
func (a *App) muteLocalSub() string {
	switch a.snap.MuteLocal {
	case engine.MuteLocalActive:
		return locales.SettingsMuteLocalDescOn()
	case engine.MuteLocalWaiting:
		return locales.SettingsMuteLocalDescWaiting()
	case engine.MuteLocalFailed:
		return locales.SettingsMuteLocalDescFailed()
	default:
		return locales.SettingsMuteLocalDesc()
	}
}

// setMuteLocal saves the preference and forwards it to the engine. The write
// is field-targeted, like the other options that share settings.json with the
// engine's debounced save.
func (a *App) setMuteLocal(on bool) {
	a.cfg.MuteLocalOnConnect = on
	if a.eng != nil {
		a.eng.SetMuteLocal(on)
	}
	if a.sets != nil {
		cfg := a.sets.Load()
		cfg.MuteLocalOnConnect = on
		_ = a.sets.Save(cfg)
	}
}

func (a *App) audioPage(c *ui.Context, p palette) {
	mode := a.latencyMode
	card := ui.Column(c).Margin(12, 0, 0, 0).Radius(12).
		Background(p.card).Border(1, p.line).Clip()
	card.Children(func() {
		cardHead(c, p, iconStopwatch, locales.SettingsLatency(), 13, 14, func() {
			if a.latencyApplying {
				ui.Spinner(c).Size(10, 10).TextColor(p.accent)
			}
			chip(c, p, locales.SettingsShowInPanel(), chipNeutral)
			toggle(c, p, &a.showLatencySwitch, locales.SettingsShowInPanel(), func(v bool) {
				a.setShowLatencySwitch(v)
			})
		})

		width, _ := c.Size()
		ui.RadioGroup(c, func() {
			if width > 0 && width < latencyStackWidth {
				for _, preset := range latencyPresets() {
					a.latencyStackRow(c, p, &mode, preset)
				}
			} else {
				ui.Row(c).Padding(0, 14).Gap(8).Children(func() {
					for _, preset := range latencyPresets() {
						a.latencyModeTile(c, p, &mode, preset)
					}
				})
			}
			a.latencyCustomTile(c, p, &mode)
		}).Label(locales.SettingsLatency())
	})

	ui.Column(c).Margin(8, 0, 0, 2).Children(func() {
		infoNote(c, p, locales.SettingsLatencyNote())
	})
	pageCard(c, p, func() {
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

// latencyStackWidth is the narrowest window whose three preset cards fit one
// row: at the 720 default each card is 148 wide and its name box 72, exactly
// the width of "Low latency". A narrower settings window stacks the presets as
// full-width rows, so no name or description is cut.
const latencyStackWidth = 714

// latencyPreset is one of the three named latency choices.
type latencyPreset struct {
	value               settings.LatencyMode
	icon                *ui.SVG
	name, latency, desc string
}

func latencyPresets() []latencyPreset {
	return []latencyPreset{
		{settings.LatencyLow, iconLatencyLow,
			locales.SettingsLatencyLow(), settings.LatencyText(settings.LatencyLowMS),
			locales.SettingsLatencyLowDesc()},
		{settings.LatencyStandard, iconLatencyStandard,
			locales.SettingsLatencyStandard(), settings.LatencyText(settings.LatencyStandardMS),
			locales.SettingsLatencyStandardDesc()},
		{settings.LatencyStable, iconLatencyStable,
			locales.SettingsLatencyStable(), settings.LatencyText(settings.LatencyStableMS),
			locales.SettingsLatencyStableDesc()},
	}
}

// latencyChoiceRow is one full-width latency choice: the leading icon, the
// name and description, the latency value, and the check ring. The custom
// tile builds its header with it, and a narrow window builds each preset with
// it. The description wraps instead of clipping, so the row grows with it.
func latencyChoiceRow(c *ui.Context, p palette, mode *settings.LatencyMode,
	value settings.LatencyMode, icon *ui.SVG, name, latency, desc string) ui.Element {
	selected := *mode == value
	row := ui.RadioBase(c, mode, value).
		MinHeight(52).Padding(10, 12).Gap(10).AlignItems(ui.Center).Justify(ui.Start).
		Cursor(ui.CursorPointer).Label(name + " · " + latency)
	row.Children(func() {
		ui.Box(c).Size(28, 28).Radius(8).Shrink(0).Center().Background(p.card).Children(func() {
			iv := ui.Icon(c, icon).Size(16, 16).TextColor(p.muted)
			if selected {
				iv.TextColor(p.accent)
			}
		})
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, name).FontSize(13).FontWeight(500).SingleLine()
			ui.Text(c, desc).FontSize(11.5).TextColor(p.muted)
		})
		val := ui.Text(c, latency).FontSize(12.5).TextColor(p.muted)
		if selected {
			val.TextColor(p.accent).FontWeight(500)
		}
		checkDot(c, p, selected)
	})
	return row
}

// latencyModeTile is one of the three preset cards. The whole card is a radio
// so the group keeps keyboard and assistive access. Its height follows its
// content, so a wrapped description grows the card instead of leaving it.
func (a *App) latencyModeTile(c *ui.Context, p palette, mode *settings.LatencyMode, preset latencyPreset) {
	selected := *mode == preset.value
	tile := ui.RadioBase(c, mode, preset.value).
		Grow(1).MinHeight(99).Padding(10).Radius(10).Gap(0).
		AlignItems(ui.Start).Justify(ui.Start).Cursor(ui.CursorPointer).
		Label(preset.name+" · "+preset.latency).Border(1, p.line).Background(p.pill)
	if selected {
		tile.Background(p.accentSoft).BorderColor(p.accent)
	}
	tile.Children(func() {
		ui.Column(c).Grow(1).Gap(0).Children(func() {
			ui.Row(c).Gap(6).Children(func() {
				ui.Box(c).Size(28, 28).Radius(8).Shrink(0).Center().Background(p.card).Children(func() {
					iv := ui.Icon(c, preset.icon).Size(16, 16).TextColor(p.muted)
					if selected {
						iv.TextColor(p.accent)
					}
				})
				ui.Text(c, preset.name).FontSize(12.5).FontWeight(500).Grow(1).SingleLine()
				checkDot(c, p, selected)
			})
			valueText := ui.Text(c, preset.latency).FontSize(11.5).TextColor(p.muted).Margin(6, 0, 2, 0)
			if selected {
				valueText.TextColor(p.accent).FontWeight(500)
			}
			ui.Text(c, preset.desc).FontSize(11.5).TextColor(p.muted)
		})
	})
	if tile.Changed() {
		a.setLatencyMode(preset.value)
	}
}

// latencyStackRow is a preset as a full-width row, the custom tile's shape,
// for a window too narrow for three cards.
func (a *App) latencyStackRow(c *ui.Context, p palette, mode *settings.LatencyMode, preset latencyPreset) {
	selected := *mode == preset.value
	row := latencyChoiceRow(c, p, mode, preset.value, preset.icon,
		preset.name, preset.latency, preset.desc).
		Margin(0, 14, 0, 14).Radius(10).Border(1, p.line).Background(p.pill)
	if selected {
		row.Background(p.accentSoft).BorderColor(p.accent)
	}
	if row.Changed() {
		a.setLatencyMode(preset.value)
	}
}

// latencyCustomTile is the full-width custom entry; its slider expands only
// while custom is the selected mode.
func (a *App) latencyCustomTile(c *ui.Context, p palette, mode *settings.LatencyMode) {
	selected := *mode == settings.LatencyCustom
	latency := settings.LatencyText(int(a.latencyCustom + 0.5))
	// The radio group already spaces its children by 8 DIP; a top margin here
	// would double that gap above the custom tile.
	outer := ui.Column(c).Margin(0, 14, 8, 14).Radius(10).Clip().
		Border(1, p.line).Background(p.pill)
	if selected {
		outer.Background(p.accentSoft).BorderColor(p.accent)
	}
	outer.Children(func() {
		top := latencyChoiceRow(c, p, mode, settings.LatencyCustom, iconSliders,
			locales.SettingsLatencyCustom(), latency, locales.SettingsLatencyCustomDesc())
		if top.Changed() {
			a.setLatencyMode(settings.LatencyCustom)
		}
		if selected {
			ui.Column(c).Padding(0, 12, 10, 38).Children(func() {
				slider(c, p, &a.latencyCustom, 20, 2000, locales.SettingsLatencyCustom(), func(v float64) {
					a.setLatencyCustom(v)
				}).Margin(4, 0, 0, 0)
				ui.Row(c).Children(func() {
					ui.Text(c, "20 ms").FontSize(10.5).TextColor(p.muted)
					ui.Spacer(c)
					ui.Text(c, "2 s").FontSize(10.5).TextColor(p.muted)
				})
			})
		}
	})
}

// checkDot is the selected-state ring on a latency card.
func checkDot(c *ui.Context, p palette, selected bool) {
	box := ui.Box(c).Size(14, 14).Radius(7).Shrink(0).Center().Border(1.5, p.faint)
	if selected {
		box.Background(p.accent).BorderWidth(0, 0, 0, 0)
		box.Children(func() {
			ui.Icon(c, iconCheck).Size(9, 9).TextColor(p.accentFg)
		})
	}
}

// captureSelectWidth holds a device or app name in the capture pickers; the
// shared 150-DIP field is too narrow for the longer names.
const captureSelectWidth = 220

func (a *App) capturePage(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		settingRow(c, p, locales.SettingsSystemOutput(), locales.SettingsSystemOutputDesc(), func() {
			if fieldSelect(c, p, &a.captureChoice, a.captureNames, locales.SettingsSystemOutput()).Width(captureSelectWidth).Changed() {
				a.applyCaptureEndpoint()
			}
		})
		rowLine(c, p)
		settingRow(c, p, locales.SettingsAppCapture(), locales.SettingsAppCaptureDesc(), func() {
			if fieldSelect(c, p, &a.appChoice, a.appNames, locales.SettingsAppCapture()).Width(captureSelectWidth).Changed() {
				a.applyCaptureProcess()
			}
		})
		rowLine(c, p)
		// The status names the capture device, so its length follows the
		// device. It keeps the row's right-hand column, capped so the title
		// keeps its width; the full name is in the tooltip.
		settingRow(c, p, locales.SettingsCaptureStatus(), locales.SettingsCaptureStatusDesc(), func() {
			status := captureStatusText(a.snap)
			ui.Row(c).Gap(6).Shrink(0).MaxWidthPercent(55).AlignItems(ui.Center).Children(func() {
				dot := p.faint
				switch {
				case a.snap.CaptureError != "":
					dot = p.red
				case a.snap.CaptureRunning:
					dot = p.green
				}
				ui.Box(c).Size(6, 6).Radius(3).Shrink(0).Background(dot)
				ui.Text(c, status).FontSize(12.5).TextColor(p.muted).SingleLine().Ellipsis("…").Tooltip(status)
			})
		})
	})
}

func (a *App) deviceSummary(c *ui.Context, p palette, d device.Device) {
	ph := phaseOf(a.snap)
	e := ui.Row(c).Height(56).Margin(12, 0, 0, 0).Padding(0, 14).Gap(10).
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
			chip(c, p, locales.StatusConnectionFailed(), chipDanger)
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
