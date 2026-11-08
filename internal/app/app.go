// Package app builds the mygo native user interface and owns the application
// lifecycle. UI copy is Chinese-first; code and diagnostics are English.
package app

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"image"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/analyze"
	"saundo/internal/capture"
	"saundo/internal/diagnostics"
	"saundo/internal/engine"
	"saundo/internal/lamp"
	"saundo/internal/nowplaying"
	"saundo/internal/receivers"
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
	trayAnim    *trayAnimator
	trayTip     string
	menuKey     trayMenuKey
	menuIcons   map[string]*mygo.MenuIcon
	quitting    bool
	flyoutH     int

	updates       updateEngine
	upd           updateState
	updCancel     context.CancelFunc
	autoDownload  bool
	quit          chan struct{}
	md            markdown.Options
	notesOpen     bool
	notesEntered  bool
	notesRange    bool
	notesExpanded map[string]bool
	history       []releaseNote
	historyKey    string
	historyLocale string
	historyState  updateHistoryState
	// async runs a job off the UI thread: a goroutine, or a test's direct
	// call that makes the update states observable at once.
	async func(func())

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

	latency            float64
	latencyMode        settings.LatencyMode
	latencyCustom      float64
	showLatencySwitch  bool
	latencyTimer       *time.Timer
	latencySeq         uint64
	latencyApplying    bool
	latencySawChange   bool
	latencyPendingSnap engine.Snapshot
	latencyHasPending  bool
	latencyHoldTimer   *time.Timer

	receiverOpen       map[string]bool
	receiverHiddenOpen bool
	receiverDialog     receiverDialogState
	receiverDialogOpen bool
	receiverDraft      receiverDraft
	receiverDelaySeq   uint64
	receiverDelayTimer *time.Timer
	receiverDelayPend  map[string]int

	readNowPlaying      func() (nowplaying.Snapshot, error)
	pushNowPlaying      func(title, artist, album string, cover []byte, coverMIME string)
	nowPlayingStreaming func() bool
	// setThemeSource applies the appearance; a test replaces it, because
	// mygo.Theme.SetSource needs the running application's main thread.
	setThemeSource func(mygo.ThemeSource)
	nowSnap        nowplaying.Snapshot
	nowMediaAUMID  string
	nowArt         *ui.Bitmap
	nowArtKey      string
	nowIcon        *ui.Bitmap
	nowIconKey     string
	nowFieldLight  *image.RGBA
	nowFieldDark   *image.RGBA
	nowLamp        lamp.Preset
	nowLampOK      bool
	nowLampKey     string
	nowPinned      bool
	nowCurrent     string
	levelFrame     func(delay time.Duration) analyze.Frame
	controlNow     func(aumid string, action nowplaying.Action)
	nowTrack       nowplaying.Track
	nowOK          bool
	nowErr         string
	nowSent        string
	lastStreaming  bool

	nameDraft           string
	strict              bool
	reconnect           bool
	launchAtStart       bool
	showNowPlayingPanel atomic.Bool
	lampOff             bool
	lampCover           bool
	lampPreset          string
	lampExpanded        bool
	trayAnimation       bool
	flyoutPopover       bool
	autoUpdate          bool
	beta                bool
	showLog             bool
	page                settingsPage

	flyoutEntered   bool
	settingsEntered bool
	pinEntered      bool
	passwordEntered bool
	logEntered      bool
}

// Run starts the application and blocks until it quits.
func Run() error {
	// The updater compares against the build stamp, not the version resource,
	// so a local build past a release still sees that release offered.
	if v := buildVersion(); v != "" {
		mygo.App.SetVersion(v)
	}
	migrateLegacyData()
	if !mygo.App.RequestSingleInstanceLock() {
		// Another saundo is running; it shows itself instead.
		return nil
	}
	mygo.App.SetName("saundo")

	a := &App{
		sets:           settings.NewStore(),
		setThemeSource: mygo.Theme.SetSource,
		log:            diagnostics.New(""),
		updates:        mygoUpdates{},
		quit:           make(chan struct{}),
		captureNames:   []string{locales.CaptureFollowSystem()},
		captureIDs:     []string{""},
		captureChoice:  locales.CaptureFollowSystem(),
		volume:         80,
	}
	a.log.Printf("saundo %s", Version)

	cfg := a.sets.Load()
	if cfg.Name == legacyName && !cfg.NameMigrated {
		// The rename owns the sender name; receivers should show 声渡. Run it
		// once so a later deliberate rename back to the legacy word sticks.
		cfg.Name = "声渡"
		cfg.NameMigrated = true
		_ = a.sets.Save(cfg)
	}
	a.cfg = cfg
	mygo.App.SetLocale(cfg.Language)
	// Set before Run so the first window paints in the saved appearance.
	mygo.Theme.SetSource(appearanceSource(cfg.Theme))
	a.beta = cfg.Beta
	mygo.Updater.SetPrereleases(cfg.Beta)
	a.volume = cfg.Volume
	a.selID = cfg.LastDeviceID
	a.nameDraft = cfg.Name
	a.strict = cfg.StrictReceiverAuth
	a.reconnect = cfg.Reconnect
	a.launchAtStart = cfg.LaunchAtStart
	a.trayAnimation = cfg.TrayAnimation
	a.flyoutPopover = cfg.FlyoutPopover
	a.captureChoice = cfg.CaptureEndpointName
	if a.captureChoice == "" {
		a.captureChoice = locales.CaptureFollowSystem()
	}
	a.latency = float64(cfg.LatencyMS)
	if a.latency <= 0 {
		a.latency = 200
	}
	a.latencyMode = cfg.LatencyMode
	if a.latencyMode == "" {
		a.latencyMode = settings.LatencyModeForMS(int(a.latency + 0.5))
	}
	a.latencyCustom = float64(cfg.LatencyCustomMS)
	if a.latencyCustom <= 0 {
		a.latencyCustom = float64(settings.DefaultLatencyCustomMS)
	}
	a.showLatencySwitch = cfg.ShowLatencySwitch
	a.lampOff = cfg.Lamp.Off
	a.lampCover = cfg.Lamp.Cover
	a.lampPreset = lamp.Normalize(cfg.Lamp.Preset)
	a.appNames = []string{locales.CaptureAllSystem()}
	a.appIDs = []string{"0"}
	a.appChoice = locales.CaptureAllSystem()

	// Move the legacy per-receiver state before the engine reads either
	// store: plaintext passwords into DPAPI, and one auto-connect entry
	// seeded from the old flag and last device. Reload the app's settings
	// copy afterwards, so a later whole-file save cannot write the moved
	// plaintext or the cleared flag back.
	credsStore := creds.NewStore()
	recvStore := receivers.NewStore()
	if res, err := receivers.MigrateSettings(a.sets, credsStore, recvStore); err != nil {
		a.log.Printf("receiver migration: %v", err)
	} else if res.MovedPasswords > 0 || res.SeededAuto || res.KeptPasswords > 0 {
		a.log.Printf("receiver migration: %d passwords moved, %d kept (DPAPI unavailable), auto-connect seeded=%v",
			res.MovedPasswords, res.KeptPasswords, res.SeededAuto)
	}
	a.cfg = a.sets.Load()
	a.showNowPlayingPanel.Store(a.cfg.ShowNowPlayingPanel)

	a.eng = engine.NewWithReceivers(a.sets, credsStore, recvStore, a.log, a.onEngineChange)

	mygo.App.OnSecondInstance(func(_ []string, _ string) { a.showWindow() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		if a.latencyTimer != nil {
			a.latencyTimer.Stop()
		}
		if a.latencyHoldTimer != nil {
			a.latencyHoldTimer.Stop()
		}
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
	mygo.App.OnQuit(func() { close(a.quit) })
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
		// state's height and grows as devices and a media session arrive.
		a.createFlyout()
		// A flyout names no title of its own; give it the app's, so window
		// metadata and accessibility do not fall back to the executable name.
		a.win.SetTitle(locales.AppName())
	} else {
		// Without a tray icon the app still needs a reachable window.
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     locales.AppName(),
			Width:     flyoutWidth,
			Height:    flyoutHeightFor(0, a.showLatencySwitch, false),
			MinWidth:  380,
			MinHeight: 460,
			Hidden:    false,
			Content:   ui.View(a.view),
		})
	}
	// The flyout's content rises in once per show; the window itself is the
	// system's to animate. Hiding the flyout folds the template list, so the
	// next open starts collapsed.
	a.bindWindowLifecycle()

	// Created before the engine starts, so its field stays immutable while
	// engine callbacks read it from other goroutines.
	a.settingsWin = a.newSettingsWindow()
	a.autoUpdate = a.cfg.AutoUpdate
	a.autoDownload = a.cfg.AutoDownload
	a.startUpdateChecks()
	a.rebuildTray()
	a.syncTrayAnimator()

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

	a.readNowPlaying = nowplaying.Read
	a.pushNowPlaying = a.eng.SetNowPlaying
	a.levelFrame = a.eng.LevelFrame
	a.controlNow = func(aumid string, action nowplaying.Action) {
		go func() {
			if err := nowplaying.Control(aumid, action, 0); err != nil && a.log != nil {
				a.log.Printf("now playing control: %v", err)
			}
		}()
	}
	a.nowPlayingStreaming = func() bool {
		return a.eng.Snapshot().State == sender.StateStreaming
	}
	a.eng.Start()
	if !silent {
		a.showWindow()
	}
	go a.pollNowPlaying()
}

// pollNowPlaying reads the system media session every two seconds, displays
// it, and pushes track changes to a streaming receiver. The push is
// unconditional; the card's display switch does not gate it.
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

// refreshNowPlaying updates the card and pushes the current track to a
// streaming receiver. The push does not depend on the card's display switch;
// a new stream resends the current track so a reconnect does not leave the
// receiver without metadata.
func (a *App) refreshNowPlaying() {
	if a.readNowPlaying == nil {
		return
	}
	snap, err := a.readNowPlaying()
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	track, ok := nowPlayingTrack(snap)
	streaming := false
	if a.nowPlayingStreaming != nil {
		streaming = a.nowPlayingStreaming()
	}
	if streaming && !a.lastStreaming {
		a.nowSent = ""
	}
	a.lastStreaming = streaming
	if !ok {
		a.nowSent = ""
	}
	a.updateUI(func() {
		a.setNowSnapshot(snap, errText)
	})
	if !ok || !streaming || a.pushNowPlaying == nil {
		return
	}
	s := snap.Sessions[snapIndex(snap)]
	key := nowPlayingPushKey(track, s)
	if key == a.nowSent {
		return
	}
	a.nowSent = key
	a.pushNowPlaying(track.Title, track.Artist, track.Album, s.Artwork, s.ArtworkMIME)
	if a.log != nil {
		a.log.Printf("now playing: %s - %s", track.Title, track.Artist)
	}
}

func snapIndex(snap nowplaying.Snapshot) int {
	if snap.Current < 0 || snap.Current >= len(snap.Sessions) {
		return 0
	}
	return snap.Current
}

func nowPlayingTrack(snap nowplaying.Snapshot) (nowplaying.Track, bool) {
	if len(snap.Sessions) == 0 {
		return nowplaying.Track{}, false
	}
	s := snap.Sessions[snapIndex(snap)]
	if s.Title == "" {
		return nowplaying.Track{}, false
	}
	return nowplaying.Track{Title: s.Title, Artist: s.Artist, Album: s.Album}, true
}

// setNowSnapshot adopts a read on the UI thread, keeping the selected session
// by AUMID and decoding the cover only when the selected track changes.
func (a *App) setNowSnapshot(snap nowplaying.Snapshot, errText string) {
	a.nowSnap = snap
	a.nowErr = errText
	a.nowTrack, a.nowOK = nowPlayingTrack(snap)
	current := ""
	if len(snap.Sessions) > 0 {
		current = snap.Sessions[snapIndex(snap)].AUMID
	}
	if current != a.nowCurrent {
		// A new session became the system's current one. The card follows
		// it unless the user picks another one again from the switch arrow.
		a.nowCurrent = current
		a.nowPinned = false
	}
	switch {
	case len(snap.Sessions) == 0:
		a.nowMediaAUMID = ""
		a.nowPinned = false
	case !a.hasNowSession(a.nowMediaAUMID):
		a.nowMediaAUMID = current
		a.nowPinned = false
	case !a.nowPinned:
		a.nowMediaAUMID = current
	}
	a.refreshNowArtwork()
	a.refreshNowAppIcon()
	a.refreshNowLamp()
	a.fitFlyout()
}

func (a *App) hasNowSession(aumid string) bool {
	if aumid == "" {
		return false
	}
	for _, s := range a.nowSnap.Sessions {
		if s.AUMID == aumid {
			return true
		}
	}
	return false
}

func (a *App) nowMediaIndex() int {
	if len(a.nowSnap.Sessions) == 0 {
		return 0
	}
	for i, s := range a.nowSnap.Sessions {
		if s.AUMID != "" && s.AUMID == a.nowMediaAUMID {
			return i
		}
	}
	return snapIndex(a.nowSnap)
}

func (a *App) nowSession() (nowplaying.Session, bool) {
	if len(a.nowSnap.Sessions) > 0 {
		return a.nowSnap.Sessions[a.nowMediaIndex()], true
	}
	if a.nowOK && a.nowTrack.Title != "" {
		return nowplaying.Session{Title: a.nowTrack.Title, Artist: a.nowTrack.Artist, Album: a.nowTrack.Album}, true
	}
	return nowplaying.Session{}, false
}

func (a *App) refreshNowArtwork() {
	s, ok := a.nowSession()
	key := ""
	if ok {
		key = nowArtworkKey(s)
	}
	if key == a.nowArtKey {
		return
	}
	a.nowArtKey = key
	a.nowArt = nil
	a.nowFieldLight = nil
	a.nowFieldDark = nil
	if !ok || len(s.Artwork) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(s.Artwork))
	if err != nil {
		if a.log != nil {
			a.log.Printf("now playing artwork: %v", err)
		}
		return
	}
	a.nowFieldLight = coverField(img, false)
	a.nowFieldDark = coverField(img, true)
	a.nowArt = ui.NewBitmap(img)
}

// refreshNowLamp keeps the light preview on the session the engine pushes:
// the system's current session, not the card's manual selection. The top
// display follows that same cover, so switching the card to a paused session
// must not change the preview.
func (a *App) refreshNowLamp() {
	if len(a.nowSnap.Sessions) == 0 {
		return // nothing is pushed; the light keeps its last cover
	}
	s := a.nowSnap.Sessions[snapIndex(a.nowSnap)]
	if s.Title == "" {
		return
	}
	key := nowArtworkKey(s)
	if key == a.nowLampKey {
		return
	}
	a.nowLampKey = key
	a.nowLamp = lamp.Preset{}
	a.nowLampOK = false
	if len(s.Artwork) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(s.Artwork))
	if err != nil {
		return
	}
	a.nowLamp, a.nowLampOK = lamp.CoverPreset(img)
}

// refreshNowAppIcon decodes the session app's icon when the selected app
// changes. The shell may have no icon for an app; the head then shows only
// the name.
func (a *App) refreshNowAppIcon() {
	s, ok := a.nowSession()
	key := ""
	if ok && len(s.AppIcon) > 0 {
		key = fmt.Sprintf("%s\x00%x", s.AUMID, fnv64(s.AppIcon))
	}
	if key == a.nowIconKey {
		return
	}
	a.nowIconKey = key
	a.nowIcon = nil
	if !ok || len(s.AppIcon) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(s.AppIcon))
	if err != nil {
		if a.log != nil {
			a.log.Printf("now playing app icon: %v", err)
		}
		return
	}
	a.nowIcon = ui.NewBitmap(img)
}

// nowArtworkKey includes the thumbnail bytes so a cover that arrives late —
// a placeholder logo first, the real artwork a poll later — refreshes under
// the same session title.
func nowArtworkKey(s nowplaying.Session) string {
	if len(s.Artwork) == 0 {
		return s.AUMID + "\x00" + s.Title + "\x00" + s.Album
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%x", s.AUMID, s.Title, s.Album, fnv64(s.Artwork))
}

// nowPlayingPushKey changes when the receiver should see new metadata: the
// text fields or the cover bytes. SMTC can supply the thumbnail a poll after
// the text, so the bytes belong in the key.
func nowPlayingPushKey(track nowplaying.Track, s nowplaying.Session) string {
	key := track.Title + "\x00" + track.Artist + "\x00" + track.Album + "\x00" + s.ArtworkMIME
	if len(s.Artwork) == 0 {
		return key
	}
	return fmt.Sprintf("%s\x00%x", key, fnv64(s.Artwork))
}

func fnv64(b []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}

// switchNowMedia cycles the card between the available SMTC sessions.
func (a *App) switchNowMedia() {
	if len(a.nowSnap.Sessions) <= 1 {
		return
	}
	next := (a.nowMediaIndex() + 1) % len(a.nowSnap.Sessions)
	a.nowMediaAUMID = a.nowSnap.Sessions[next].AUMID
	a.nowPinned = a.nowMediaAUMID != a.nowCurrent
	a.refreshNowArtwork()
	a.refreshNowAppIcon()
	a.updateUI(func() {})
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

func (a *App) applySnapshot(snap engine.Snapshot) {
	a.snap = snap
	// A delay draft stays visible until the engine's row carries it, so the
	// slider never snaps back to the old value between the command and the
	// next snapshot.
	for key, ms := range a.receiverDelayPend {
		for _, v := range snap.Receivers {
			if v.Key == key && v.DelayMS == ms {
				delete(a.receiverDelayPend, key)
				break
			}
		}
	}
	a.strict = snap.StrictAuth
	a.reconnect = snap.Reconnect
	a.lampOff = snap.LampOff
	a.lampCover = snap.LampCover
	a.lampPreset = lamp.Normalize(snap.LampPreset)
	// The engine owns the lamp fields. Mirror them here so an app-only save
	// (theme, update preferences, launch at login) cannot write a stale lamp
	// choice back over the engine's.
	a.cfg.Lamp = settings.LampSettings{Off: a.lampOff, Cover: a.lampCover, Preset: a.lampPreset}
	a.flyoutPopover = snap.FlyoutPopover
	a.cfg.FlyoutPopover = snap.FlyoutPopover
	if a.trayAnimation != snap.TrayAnimation {
		a.trayAnimation = snap.TrayAnimation
		a.syncTrayAnimator()
	}
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

// fitFlyout sizes the tray flyout to the visible device rows and the current
// media state. The device card and the playback card touch directly, and the
// media card adds its height only while a session exists. The flyout is
// anchored to the tray icon by its bottom edge, so a resize grows or shrinks
// it upward. Main thread only.
func (a *App) fitFlyout() {
	if a.tray == nil || a.win == nil {
		return
	}
	h := flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, a.hasNowMedia(), a.lampCover, a.lampExpanded, a.errorBarShown())
	if h == a.flyoutH {
		return
	}
	a.flyoutH = h
	a.win.SetSize(flyoutWidth, h)
}

// hasNowMedia reports whether the media card has a session to draw.
func (a *App) hasNowMedia() bool {
	if !a.cfg.ShowNowPlayingPanel {
		return false
	}
	_, ok := a.nowSession()
	return ok
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
	focusAppOnShow()
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
			{Label: locales.TrayCheckUpdates(), Icon: a.menuIcons["update"], Click: func(*mygo.MenuItem, *mygo.Window) { a.openUpdateCheck() }},
			mygo.Separator(),
		}
		if connecting {
			items = append(items, &mygo.MenuItem{Label: locales.TrayDisconnect(), Icon: a.menuIcons["disconnect"], Click: func(*mygo.MenuItem, *mygo.Window) {
				a.disconnect()
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
	if a.updateChipVisible() {
		if a.upd.phase == updateReady {
			tip += " · " + locales.UpdateReadyShort()
		} else {
			tip += " · " + locales.FlyoutUpdateAvailable(a.upd.version)
		}
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
	a.saveSettings()
	if err := mygo.App.SetOpenAtLogin(on); err != nil {
		a.log.Printf("set open at login: %v", err)
		a.launchAtStart = !on
		a.cfg.LaunchAtStart = !on
		a.saveSettings()
	}
}

// setTrayAnimation persists the tray animation preference and starts or
// stops the animator at once.
func (a *App) setTrayAnimation(on bool) {
	a.trayAnimation = on
	a.cfg.TrayAnimation = on
	if a.sets != nil {
		cfg := a.sets.Load()
		cfg.TrayAnimation = on
		_ = a.sets.Save(cfg)
	}
	if a.eng != nil {
		a.eng.SetTrayAnimation(on)
	}
	a.syncTrayAnimator()
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
	target := a.snap.PasswordTargetID
	if target == "" {
		target = id
	}
	if password == "" || id == "" {
		return
	}
	a.eng.SetReceiverPassword(id, password)
	a.eng.Connect(target)
	a.selID = target
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
	case sender.StateConnecting, sender.StateHandshake:
		return locales.StatusConnecting()
	case sender.StatePairing:
		return locales.StatusPairing()
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

// onLocaleChanged relabels the sentinel capture options, retitles the
// windows, and rebuilds the tray menu. Both windows redraw in the new
// language on their own.
func (a *App) onLocaleChanged() {
	if len(a.captureNames) > 0 {
		a.captureNames[0] = locales.CaptureFollowSystem()
	}
	if len(a.appNames) > 0 {
		a.appNames[0] = locales.CaptureAllSystem()
	}
	if a.win != nil {
		a.win.SetTitle(locales.AppName())
	}
	if a.settingsWin != nil {
		a.settingsWin.SetTitle(locales.SettingsWindowTitle())
	}
	a.syncCaptureChoice()
	a.syncAppChoice()
	a.rebuildTray()
	a.updateUI(func() {})
}

// setBeta chooses whether update checks offer prereleases too.
func (a *App) setBeta(on bool) {
	a.beta = on
	a.cfg.Beta = on
	a.saveSettings()
	if a.updates != nil {
		a.updates.SetPrereleases(on)
	}
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
	a.cfg = cfg
	_ = a.sets.Save(cfg)
	mygo.App.SetLocale(tag)
}

// themeLabel is the settings select's value for the saved appearance.
func (a *App) themeLabel() string {
	switch a.cfg.Theme {
	case settings.ThemeLight:
		return locales.ThemeLight()
	case settings.ThemeDark:
		return locales.ThemeDark()
	default:
		return locales.ThemeSystem()
	}
}

// setTheme saves the chosen appearance and applies it at once.
func (a *App) setTheme(label string) {
	mode := settings.ThemeSystem
	switch label {
	case locales.ThemeLight():
		mode = settings.ThemeLight
	case locales.ThemeDark():
		mode = settings.ThemeDark
	}
	a.cfg.Theme = mode
	if a.sets != nil {
		cfg := a.sets.Load()
		cfg.Theme = mode
		_ = a.sets.Save(cfg)
	}
	if a.setThemeSource != nil {
		a.setThemeSource(appearanceSource(mode))
	}
}
