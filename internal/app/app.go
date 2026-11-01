// Package app builds the mygo native user interface and owns the application
// lifecycle. UI copy is Chinese-first; code and diagnostics are English.
package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/capture"
	"saundo/internal/diagnostics"
	"saundo/internal/engine"
	"saundo/internal/nowplaying"
	"saundo/internal/settings"
)

// Version is stamped at build time with -ldflags -X; plain builds keep the default.
var Version = "dev"

// App is the application's UI state, read and changed on the main thread.
type App struct {
	eng         *engine.Engine
	sets        *settings.Store
	cfg         settings.Settings
	log         *diagnostics.Log
	win         *mygo.Window
	settingsWin *mygo.Window
	tray        *mygo.Tray
	trayMenu    *mygo.Menu
	trayKey     trayIconKey
	trayTip     string
	menuKey     trayMenuKey
	menuIcons   map[string]*mygo.MenuIcon
	quitting    bool
	flyoutH     int

	snap  engine.Snapshot
	selID string

	pinOpen bool
	pinCode string

	passwordOpen   bool
	passwordPrompt string

	volume float64

	captureNames  []string
	captureIDs    []string
	captureChoice string

	appNames  []string
	appIDs    []string
	appChoice string

	latency       float64
	passwordDraft string

	readNowPlaying    func() (nowplaying.Track, bool, error)
	pushNowPlaying    func(title, artist, album string)
	nowPlayingSetting func() (show, streaming bool)
	nowTrack          nowplaying.Track
	nowOK             bool
	nowErr            string
	nowSent           string
	lastStreaming     bool

	nameDraft      string
	strict         bool
	autoConn       bool
	reconnect      bool
	launchAtStart  bool
	showNowPlaying bool
	autoUpdate     bool
	beta           bool
	showLog        bool
	page           settingsPage

	flyoutEntered   bool
	settingsEntered bool
	pinEntered      bool
	passwordEntered bool
	logEntered      bool
}

// Run starts the application and blocks until it quits.
func Run() error {
	migrateLegacyData()
	if !mygo.App.RequestSingleInstanceLock() {
		// Another saundo is running; it shows itself instead.
		return nil
	}
	mygo.App.SetName("saundo")
	mygo.App.SetVersion(Version)

	a := &App{
		sets:          settings.NewStore(),
		log:           diagnostics.New(""),
		captureNames:  []string{locales.CaptureFollowSystem()},
		captureIDs:    []string{""},
		captureChoice: locales.CaptureFollowSystem(),
		volume:        80,
	}
	a.log.Printf("saundo %s", Version)

	cfg := a.sets.Load()
	if cfg.Name == legacyName {
		// The rename owns the sender name; receivers should show 声渡.
		cfg.Name = "声渡"
		_ = a.sets.Save(cfg)
	}
	a.cfg = cfg
	mygo.App.SetLocale(cfg.Language)
	a.beta = cfg.Beta
	mygo.Updater.SetPrereleases(cfg.Beta)
	a.volume = cfg.Volume
	a.selID = cfg.LastDeviceID
	a.nameDraft = cfg.Name
	a.strict = cfg.StrictReceiverAuth
	a.autoConn = cfg.AutoConnect
	a.reconnect = cfg.Reconnect
	a.launchAtStart = cfg.LaunchAtStart
	a.captureChoice = cfg.CaptureEndpointName
	if a.captureChoice == "" {
		a.captureChoice = locales.CaptureFollowSystem()
	}
	a.latency = float64(cfg.LatencyMS)
	if a.latency <= 0 {
		a.latency = 200
	}
	a.appNames = []string{locales.CaptureAllSystem()}
	a.appIDs = []string{"0"}
	a.appChoice = locales.CaptureAllSystem()

	a.eng = engine.New(a.sets, creds.NewStore(), a.log, a.onEngineChange)

	mygo.App.OnSecondInstance(func(_ []string, _ string) { a.showWindow() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.eng.Stop()
	})
	// Windows leaves a tray icon behind after its app until the pointer
	// passes over it.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnLocaleChanged(func(string) { a.onLocaleChanged() })
	updater.OnChange(func() {
		a.autoUpdate = updater.AutomaticChecks()
		a.updateUI(func() {})
	})
	mygo.App.WhenReady(a.ready)
	return mygo.App.Run()
}

func (a *App) ready() {
	// Started at login stays quietly in the tray; the user opens the panel
	// from there. --tray forces the same behavior for manual testing.
	silent := mygo.App.WasOpenedAtLogin() || hasArg("--tray")

	a.tray, _ = mygo.NewTray(mygo.TrayOptions{
		ID:             "main",
		Icon:           trayPNGs[trayIconKey{face: "smile"}],
		IconIsTemplate: false,
		ToolTip:        locales.TrayTooltip(),
	})
	if a.tray != nil {
		a.tray.OnAppearanceChange(a.refreshTrayIcon)
		a.refreshTrayIcon()
		a.tray.OnClick(a.toggleWindow)
		a.tray.OnRightClick(func() {
			if a.win != nil {
				a.win.Hide()
			}
			if a.trayMenu != nil {
				a.tray.SetMenu(a.trayMenu)
				defer a.tray.SetMenu(nil)
				a.tray.PopUpMenu()
			}
		})
		// Discovery has not reported yet, so the flyout opens at the empty
		// state's height and grows as devices arrive.
		a.flyoutH = flyoutHeightFor(0)
		a.win = mygo.NewFlyout(mygo.FlyoutOptions{
			Tray:      a.tray,
			Placement: mygo.PlacementBottom,
			Gap:       6,
			Width:     flyoutWidth,
			Height:    a.flyoutH,
			Focusable: true,
			Shadow:    true,
			Hidden:    true,
			Content:   ui.View(a.view),
		})
	} else {
		// Without a tray icon the app still needs a reachable window.
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     locales.AppName(),
			Width:     flyoutWidth,
			Height:    flyoutHeight,
			MinWidth:  380,
			MinHeight: 460,
			Hidden:    false,
			Content:   ui.View(a.view),
		})
	}
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		if a.tray == nil {
			a.quitting = true
			mygo.App.Quit()
			return
		}
		e.PreventDefault()
		a.win.Hide()
	})
	// The flyout's content rises in once per show; the window itself is the
	// system's to animate.
	a.win.OnShow(func() { a.flyoutEntered = false })
	// Created before the engine starts, so its field stays immutable while
	// engine callbacks read it from other goroutines.
	a.settingsWin = a.newSettingsWindow()
	a.autoUpdate = updater.AutomaticChecks()
	a.rebuildTray()

	if a.cfg.LaunchAtStart {
		// Keep the system registration in step with the saved setting.
		_ = mygo.App.SetOpenAtLogin(true)
	}

	go func() {
		eps, err := capture.Endpoints()
		if err != nil {
			a.log.Printf("enumerate capture endpoints: %v", err)
			return
		}
		names := []string{locales.CaptureFollowSystem()}
		ids := []string{""}
		for _, ep := range eps {
			names = append(names, ep.Name)
			ids = append(ids, ep.ID)
		}
		var apps []capture.AppInfo
		if capture.AppCaptureSupported() {
			apps, err = capture.AudioApplications()
			if err != nil {
				a.log.Printf("enumerate applications: %v", err)
			}
		}
		appNames := []string{locales.CaptureAllSystem()}
		appIDs := []string{"0"}
		for _, ap := range apps {
			label := ap.Name
			if ap.Title != "" && ap.Title != ap.Name {
				label = fmt.Sprintf("%s · %s", ap.Name, ap.Title)
			}
			appNames = append(appNames, label)
			appIDs = append(appIDs, fmt.Sprint(ap.PID))
		}
		a.updateUI(func() {
			a.captureNames, a.captureIDs = names, ids
			a.appNames, a.appIDs = appNames, appIDs
			a.syncCaptureChoice()
			a.syncAppChoice()
		})
	}()

	a.readNowPlaying = nowplaying.Current
	a.pushNowPlaying = a.eng.SetNowPlaying
	a.nowPlayingSetting = func() (show, streaming bool) {
		snap := a.eng.Snapshot()
		return snap.ShowNowPlaying, snap.State == sender.StateStreaming
	}
	a.eng.Start()
	if !silent {
		a.showWindow()
	}
	go a.pollNowPlaying()
}

// pollNowPlaying reads the system media session every two seconds, displays
// it, and pushes track changes while 推送正在播放 is on. The setting comes from
// the live engine snapshot, not the settings copy loaded at startup.
func (a *App) pollNowPlaying() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	quit := make(chan struct{})
	mygo.App.OnQuit(func() { close(quit) })
	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
		}
		a.refreshNowPlaying()
	}
}

// refreshNowPlaying updates the visible track and pushes it to a streaming
// receiver when the toggle is on. A new stream resends the current track so a
// reconnect does not leave the receiver without metadata.
func (a *App) refreshNowPlaying() {
	if a.readNowPlaying == nil {
		return
	}
	track, ok, err := a.readNowPlaying()
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	show, streaming := false, false
	if a.nowPlayingSetting != nil {
		show, streaming = a.nowPlayingSetting()
	}
	if streaming && !a.lastStreaming {
		a.nowSent = ""
	}
	a.lastStreaming = streaming
	if !ok {
		a.nowSent = ""
	}
	a.updateUI(func() {
		a.nowTrack, a.nowOK, a.nowErr = track, ok, errText
	})
	if !show || !ok || !streaming || a.pushNowPlaying == nil {
		return
	}
	key := track.Title + "\x00" + track.Artist + "\x00" + track.Album
	if key == a.nowSent {
		return
	}
	a.nowSent = key
	a.pushNowPlaying(track.Title, track.Artist, track.Album)
	if a.log != nil {
		a.log.Printf("now playing: %s - %s", track.Title, track.Artist)
	}
}

// nowPlayingText is the line under 正在播放 in the playback card.
func (a *App) nowPlayingText() string {
	if a.nowErr != "" {
		return locales.FlyoutNowPlayingReadFailed()
	}
	if !a.nowOK || a.nowTrack.Title == "" {
		return locales.FlyoutNowPlayingNoMedia()
	}
	text := a.nowTrack.Title
	if a.nowTrack.Artist != "" {
		text += " · " + a.nowTrack.Artist
	}
	return text
}

func hasArg(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}

// updateUI runs fn on the main thread and redraws both windows. The settings
// window reads the same App state as the flyout, so it follows every change.
func (a *App) updateUI(fn func()) {
	switch {
	case a.win != nil:
		a.win.Update(fn)
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	case a.settingsWin != nil:
		a.settingsWin.Update(fn)
	default:
		fn()
	}
}

func (a *App) onEngineChange(snap engine.Snapshot) {
	if a.win == nil && a.settingsWin == nil {
		return
	}
	a.updateUI(func() { a.apply(snap) })
}

func (a *App) apply(snap engine.Snapshot) {
	a.snap = snap
	a.strict = snap.StrictAuth
	a.autoConn = snap.AutoConnect
	a.reconnect = snap.Reconnect
	a.showNowPlaying = snap.ShowNowPlaying
	a.latency = float64(snap.LatencyMS)
	if a.latency <= 0 {
		a.latency = 200
	}
	if snap.Name != "" {
		a.nameDraft = snap.Name
	}
	a.volume = snap.Volume
	if snap.CaptureEndpoint == "" {
		if a.captureChoice == "" {
			a.captureChoice = locales.CaptureFollowSystem()
		}
	} else {
		a.syncCaptureChoice()
	}
	if snap.PinRequired && !a.pinOpen {
		a.pinOpen = true
		a.pinEntered = false
		a.pinCode = ""
	} else if !snap.PinRequired && a.pinOpen {
		a.pinOpen = false
	}
	if snap.PasswordRequired && !a.passwordOpen {
		a.passwordOpen = true
		a.passwordEntered = false
		a.passwordPrompt = ""
	} else if !snap.PasswordRequired && a.passwordOpen {
		a.passwordOpen = false
	}
	if a.selID == "" {
		a.selID = snap.SelectedID
	}
	a.syncSelection()
	a.rebuildTray()
	a.fitFlyout()
}

// fitFlyout sizes the tray flyout to the visible device rows, so the playback
// card keeps the same spacing under the device card for any device count. The
// flyout is anchored to the tray icon by its bottom edge, so a resize grows or
// shrinks it upward and leaves the playback card where it is. Main thread only.
func (a *App) fitFlyout() {
	if a.tray == nil || a.win == nil {
		return
	}
	h := flyoutHeightFor(len(a.snap.Devices))
	if h == a.flyoutH {
		return
	}
	a.flyoutH = h
	a.win.SetSize(flyoutWidth, h)
}

func (a *App) syncCaptureChoice() {
	if a.snap.CaptureEndpoint == "" {
		a.captureChoice = locales.CaptureFollowSystem()
		return
	}
	for i, id := range a.captureIDs {
		if id == a.snap.CaptureEndpoint {
			a.captureChoice = a.captureNames[i]
			return
		}
	}
	if a.snap.CaptureName != "" {
		a.captureChoice = a.snap.CaptureName
	}
}

func (a *App) syncAppChoice() {
	if a.snap.CaptureProcessPID == 0 {
		a.appChoice = locales.CaptureAllSystem()
		return
	}
	for i, id := range a.appIDs {
		if id == fmt.Sprint(a.snap.CaptureProcessPID) {
			a.appChoice = a.appNames[i]
			return
		}
	}
	if a.snap.CaptureProcessName != "" {
		a.appChoice = a.snap.CaptureProcessName
	}
}

func (a *App) syncSelection() {
	for _, d := range a.snap.Devices {
		if d.ID == a.selID {
			return
		}
	}
	if len(a.snap.Devices) > 0 {
		a.selID = a.snap.Devices[0].ID
	}
}

func (a *App) showWindow() {
	if a.win == nil {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
	mygo.App.Focus()
}

// toggleWindow is the tray icon's primary click: the panel opens next to the
// icon, and a second click closes it.
func (a *App) toggleWindow() {
	if a.win == nil {
		return
	}
	if a.win.IsVisible() {
		a.win.Hide()
		return
	}
	a.showWindow()
}

// trayMenuKey is what the tray menu's shape depends on. The tooltip follows
// more of the snapshot and is compared on its own.
type trayMenuKey struct {
	connecting   bool
	hasSelection bool
	locale       string
}

func (a *App) rebuildTray() {
	if a.tray == nil {
		return
	}
	if a.menuIcons == nil {
		a.menuIcons = loadMenuIcons()
	}
	connecting := a.snap.State != sender.StateIdle
	key := trayMenuKey{connecting: connecting, hasSelection: a.selID != "", locale: mygo.App.Locale()}
	if a.trayMenu == nil || key != a.menuKey {
		a.menuKey = key
		items := []*mygo.MenuItem{
			{Label: locales.TrayShowPanel(), Icon: a.menuIcons["open"], Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			{Label: locales.TraySettings(), Icon: a.menuIcons["settings"], Click: func(*mygo.MenuItem, *mygo.Window) { a.showSettings() }},
			updater.MenuItem(),
			mygo.Separator(),
		}
		if connecting {
			items = append(items, &mygo.MenuItem{Label: locales.TrayDisconnect(), Icon: a.menuIcons["disconnect"], Click: func(*mygo.MenuItem, *mygo.Window) {
				a.eng.Disconnect()
			}})
		} else {
			items = append(items, &mygo.MenuItem{Label: locales.TrayConnectLast(), Icon: a.menuIcons["connect"], Disabled: a.selID == "", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.connectSelected()
			}})
		}
		items = append(items, mygo.Separator(), &mygo.MenuItem{Role: mygo.RoleQuit, Label: locales.TrayQuit(), Icon: a.menuIcons["quit"]})
		a.trayMenu = mygo.NewMenu(items)
	}
	tip := locales.TrayTooltip()
	if connecting {
		tip = locales.TrayTooltip() + " · " + a.statusText()
	}
	if tip != a.trayTip {
		a.trayTip = tip
		a.tray.SetToolTip(tip)
	}
	a.refreshTrayIcon()
}

func (a *App) connectSelected() {
	if a.selID == "" {
		return
	}
	a.eng.Connect(a.selID)
}

// setLaunchAtStart persists the toggle and asks the system to start the app
// at login. A failed registration reverts the switch.
func (a *App) setLaunchAtStart(on bool) {
	a.cfg.LaunchAtStart = on
	_ = a.sets.Save(a.cfg)
	if err := mygo.App.SetOpenAtLogin(on); err != nil {
		a.log.Printf("set open at login: %v", err)
		a.launchAtStart = !on
		a.cfg.LaunchAtStart = !on
		_ = a.sets.Save(a.cfg)
	}
}

func (a *App) submitPin() {
	// The receiver decides the code's length: an Apple TV shows four
	// digits, a Mac's password can be any length.
	code := strings.TrimSpace(a.pinCode)
	if code == "" {
		return
	}
	a.eng.SubmitPIN(code)
	a.pinOpen = false
	a.pinCode = ""
}

func (a *App) submitPassword() {
	password := strings.TrimSpace(a.passwordPrompt)
	id := a.snap.PasswordDeviceID
	if password == "" || id == "" {
		return
	}
	a.eng.SetReceiverPassword(id, password)
	a.eng.Connect(id)
	a.selID = id
	a.passwordOpen = false
	a.passwordPrompt = ""
}

func (a *App) cancelPassword() {
	a.passwordOpen = false
	a.passwordPrompt = ""
	a.eng.DismissPasswordPrompt()
}

func (a *App) cancelPin() {
	a.pinOpen = false
	a.pinCode = ""
	a.eng.Disconnect()
}

func (a *App) statusText() string {
	if a.snap.Reconnecting {
		return locales.StatusReconnecting()
	}
	switch a.snap.State {
	case sender.StateConnecting:
		return locales.StatusConnecting()
	case sender.StatePairing:
		return locales.StatusPairing()
	case sender.StateHandshake:
		return locales.StatusHandshaking()
	case sender.StateStreaming:
		if a.snap.ConnectedName != "" {
			return locales.StatusStreamingTo(a.snap.ConnectedName)
		}
		return locales.StatusStreaming()
	default:
		return locales.StatusDisconnected()
	}
}

func deviceSubtitle(d device.Device) string {
	model := d.Model
	if model == "" {
		model = locales.DeviceDefaultModel()
	}
	if d.IsStereoGroup() {
		return locales.DeviceStereoGroup(model, len(d.Members))
	}
	switch d.Auth {
	case device.AuthHAPPIN:
		return locales.DevicePinPairing(model)
	case device.AuthHAPPassword, device.AuthPassword:
		return locales.DeviceNeedsPassword(model)
	case device.AuthLegacyPin:
		return locales.DeviceUnsupported(model)
	case device.AuthSetup:
		return locales.DeviceAuthSetup(model)
	}
	if d.AirPlay2 {
		return locales.DeviceAirPlay2(model)
	}
	return locales.DeviceAirPlay1(model)
}

// onLocaleChanged relabels the sentinel capture options and rebuilds the
// tray menu. Both windows redraw in the new language on their own.
func (a *App) onLocaleChanged() {
	if len(a.captureNames) > 0 {
		a.captureNames[0] = locales.CaptureFollowSystem()
	}
	if len(a.appNames) > 0 {
		a.appNames[0] = locales.CaptureAllSystem()
	}
	a.syncCaptureChoice()
	a.syncAppChoice()
	a.rebuildTray()
	a.updateUI(func() {})
}

// updateText is the settings row's status line for the update check.
func (a *App) updateText() string {
	if !mygo.Updater.Enabled() {
		return locales.SettingsUpdateUnavailable()
	}
	if v := updater.AvailableVersion(); v != "" {
		return locales.SettingsUpdateAvailable(v)
	}
	return locales.SettingsUpdatesDesc()
}

// setBeta chooses whether update checks offer prereleases too.
func (a *App) setBeta(on bool) {
	a.beta = on
	a.cfg.Beta = on
	cfg := a.sets.Load()
	cfg.Beta = on
	_ = a.sets.Save(cfg)
	mygo.Updater.SetPrereleases(on)
}

// languageLabel is the settings select's value for the saved language tag.
func (a *App) languageLabel() string {
	switch a.cfg.Language {
	case "en":
		return locales.LanguageEnglish()
	case "zh-CN":
		return locales.LanguageChinese()
	default:
		return locales.LanguageSystem()
	}
}

// setLanguage saves the chosen tag and switches the interface at once.
func (a *App) setLanguage(label string) {
	tag := ""
	switch label {
	case locales.LanguageEnglish():
		tag = "en"
	case locales.LanguageChinese():
		tag = "zh-CN"
	}
	a.cfg.Language = tag
	cfg := a.sets.Load()
	cfg.Language = tag
	_ = a.sets.Save(cfg)
	mygo.App.SetLocale(tag)
}
