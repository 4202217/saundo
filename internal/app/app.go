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
	eng                 *engine.Engine
	sets                *settings.Store
	cfg                 settings.Settings
	settingsErrorLogged string
	log                 *diagnostics.Log
	win                 *mygo.Window
	settingsWin         *mygo.Window
	tray                *mygo.Tray
	trayMenu            *mygo.Menu
	trayKey             trayIconKey
	trayAnim            *trayAnimator
	trayTip             string
	menuKey             trayMenuKey
	menuIcons           map[string]*mygo.MenuIcon
	quitting            bool
	power               powerHold
	flyoutH             int
	// resizeWindow replaces the flyout window resize in tests; production
	// uses win.SetSize.
	resizeWindow func(width, height int)

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

	credentialOpen bool

	volume float64

	captureNames  []string
	captureIDs    []string
	captureLabels []string
	captureChoice string

	appNames      []string
	appPIDs       []uint32
	appIDs        []string
	appIcons      map[uint32]*ui.Bitmap
	appNameByID   map[string]string
	appSelection  map[string]bool
	appMenuOpen   bool
	appApplyTimer *time.Timer

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
	standbyEnabled      bool
	standbyMinutes      int
	trayAnimation       bool
	autoUpdate          bool
	beta                bool
	showLog             bool
	page                settingsPage

	flyoutEntered     bool
	settingsEntered   bool
	pinEntered        bool
	passwordEntered   bool
	credentialEntered bool
	logEntered        bool
}

// senderNameForFirstRun gives a new profile the locale's app name and keeps
// a stored name. An English install then advertises "Saundo" instead of the
// Chinese brand.
func senderNameForFirstRun(firstRun bool, stored string) string {
	if firstRun {
		return locales.AppName()
	}
	return stored
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
		captureLabels:  []string{locales.CaptureFollowSystem()},
		captureChoice:  locales.CaptureFollowSystem(),
		volume:         80,
		power:          powerHold{acquire: mygo.Power.KeepRunning},
	}
	a.log.Printf("saundo %s", Version)

	firstRun := !a.sets.Exists()
	cfg := a.sets.Load()
	if firstRun {
		cfg.Name = senderNameForFirstRun(firstRun, cfg.Name)
		if err := a.sets.Save(cfg); err != nil {
			a.log.Printf("settings save failed: %v", err)
		}
	}
	if cfg.Name == legacyName && !cfg.NameMigrated {
		// The rename owns the sender name; use the app's name in the current
		// locale so an English install does not advertise the Chinese brand.
		// Run it once so a later deliberate rename back to the legacy word
		// sticks.
		cfg.Name = locales.AppName()
		cfg.NameMigrated = true
		if err := a.sets.Save(cfg); err != nil {
			a.log.Printf("settings save failed: %v", err)
		}
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
	a.standbyEnabled = cfg.Standby.Enabled
	a.standbyMinutes = settings.NormalizeStandbyMinutes(cfg.Standby.IdleMinutes)
	a.appNames = []string{locales.CaptureAllSystem()}
	a.appPIDs = []uint32{0}

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
		if a.receiverDelayTimer != nil {
			a.receiverDelayTimer.Stop()
		}
		a.power.stop()
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
		a.tray.OnAppearanceChange(a.rebuildTray)
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
		appPIDs := []uint32{0}
		appIDs := []string{""}
		appNameByID := map[string]string{}
		appIcons := make(map[uint32]image.Image)
		for _, ap := range apps {
			label := ap.Name
			if ap.Title != "" && ap.Title != ap.Name {
				label = fmt.Sprintf("%s · %s", ap.Name, ap.Title)
			}
			appNames = append(appNames, label)
			appPIDs = append(appPIDs, ap.PID)
			appIDs = append(appIDs, ap.ID)
			if ap.ID != "" {
				appNameByID[ap.ID] = label
			}
			if len(ap.Icon) > 0 {
				img, _, err := image.Decode(bytes.NewReader(ap.Icon))
				if err != nil {
					a.log.Printf("capture: app icon for %s: %v", ap.Name, err)
					continue
				}
				appIcons[ap.PID] = img
			}
		}
		a.updateUI(func() {
			a.captureNames, a.captureIDs = names, ids
			a.rebuildCaptureLabels()
			a.appNames, a.appPIDs, a.appIDs = appNames, appPIDs, appIDs
			a.appNameByID = appNameByID
			a.appIcons = make(map[uint32]*ui.Bitmap, len(appIcons))
			for pid, img := range appIcons {
				a.appIcons[pid] = ui.NewBitmap(img)
			}
			a.syncCaptureChoice()
			a.syncAppChoice()
			a.migrateLegacyAppSelection()
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
	a.updateUI(func() {
		a.setNowSnapshot(snap, errText)
		if streaming && !a.lastStreaming {
			a.nowSent = ""
		}
		a.lastStreaming = streaming
		if !ok {
			a.nowSent = ""
		}
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
	})
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
	a.standbyEnabled = snap.StandbyEnabled
	a.standbyMinutes = settings.NormalizeStandbyMinutes(snap.StandbyMinutes)
	// The engine owns the lamp fields. Mirror them here so an app-only save
	// (theme, update preferences, launch at login) cannot write a stale lamp
	// choice back over the engine's.
	a.cfg.Lamp = settings.LampSettings{Off: a.lampOff, Cover: a.lampCover, Preset: a.lampPreset}
	a.cfg.Standby = settings.StandbySettings{Enabled: a.standbyEnabled, IdleMinutes: a.standbyMinutes}
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
	a.syncCaptureChoice()
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
	if a.win == nil && a.resizeWindow == nil {
		return
	}
	h := flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, a.hasNowMedia(), a.lampCover, a.lampExpanded, a.errorBarShown(), a.snap.CredentialReset)
	if h == a.flyoutH {
		return
	}
	a.flyoutH = h
	if a.resizeWindow != nil {
		a.resizeWindow(flyoutWidth, h)
		return
	}
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

// rebuildCaptureLabels gives each capture option a unique display label. The
// select binds to the label, while the endpoint ID stays the identity the
// engine reports; a duplicate name, or one equal to the follow-system
// sentinel, gets a numbered suffix.
func (a *App) rebuildCaptureLabels() {
	if len(a.captureNames) == 0 {
		a.captureLabels = nil
		return
	}
	labels := make([]string, len(a.captureNames))
	used := make(map[string]bool, len(a.captureNames))
	for i, name := range a.captureNames {
		label := name
		if i > 0 {
			for n := 1; used[label]; n++ {
				label = fmt.Sprintf("%s (%d)", name, n)
			}
		}
		labels[i] = label
		used[label] = true
	}
	a.captureLabels = labels
}

// captureEndpointFor resolves a display label to the endpoint's ID and real
// name. Matching by label is safe because the labels are unique.
func (a *App) captureEndpointFor(label string) (id, name string, ok bool) {
	for i, l := range a.captureLabels {
		if l == label && i < len(a.captureIDs) && i < len(a.captureNames) {
			return a.captureIDs[i], a.captureNames[i], true
		}
	}
	return "", "", false
}

func (a *App) syncCaptureChoice() {
	if a.snap.CaptureEndpoint == "" {
		a.captureChoice = locales.CaptureFollowSystem()
		return
	}
	for i, id := range a.captureIDs {
		if id != a.snap.CaptureEndpoint {
			continue
		}
		if i < len(a.captureLabels) {
			a.captureChoice = a.captureLabels[i]
		}
		return
	}
	if a.snap.CaptureName != "" {
		a.captureChoice = a.snap.CaptureName
	}
}

func (a *App) syncAppChoice() {
	if a.appMenuOpen {
		return
	}
	a.appSelection = make(map[string]bool, len(a.snap.CaptureAppIDs))
	for _, id := range a.snap.CaptureAppIDs {
		if id != "" {
			a.appSelection[id] = true
		}
	}
}

// appSelectionLabel names the multi-select choice for the field.
func (a *App) appSelectionLabel() string {
	switch len(a.appSelection) {
	case 0:
		// A legacy saved PID whose app was not running at launch still owns
		// capture until the user picks again.
		if a.snap.CaptureProcessPID != 0 && a.snap.CaptureProcessName != "" {
			return a.snap.CaptureProcessName
		}
		return locales.CaptureAllSystem()
	case 1:
		for id := range a.appSelection {
			return a.appNameForID(id)
		}
	}
	return locales.SettingsAppCaptureCount(len(a.appSelection))
}

func (a *App) appNameForID(id string) string {
	if name, ok := a.appNameByID[id]; ok && name != "" {
		return name
	}
	for i, sid := range a.snap.CaptureAppIDs {
		if sid == id && i < len(a.snap.CaptureAppNames) && a.snap.CaptureAppNames[i] != "" {
			return a.snap.CaptureAppNames[i]
		}
	}
	return id
}

func (a *App) appBitmapForID(id string) *ui.Bitmap {
	for i, sid := range a.appIDs {
		if sid == id && i < len(a.appPIDs) {
			return a.appIcons[a.appPIDs[i]]
		}
	}
	return nil
}

// selectionBitmap is the icon of the first selected app in list order, so the
// field stays stable when several are selected.
func (a *App) selectionBitmap() *ui.Bitmap {
	for i, id := range a.appIDs {
		if id != "" && a.appSelection[id] && i < len(a.appPIDs) {
			return a.appIcons[a.appPIDs[i]]
		}
	}
	return nil
}

func (a *App) perAppSelected() bool {
	return a.snap.CaptureProcessPID != 0 || len(a.snap.CaptureAppIDs) > 0
}

// selectedAppIDs returns the chosen IDs in list order, then saved IDs whose
// apps are no longer running.
func (a *App) selectedAppIDs() (ids, names []string) {
	seen := make(map[string]bool, len(a.appSelection))
	for i, id := range a.appIDs {
		if id == "" || !a.appSelection[id] || seen[id] || i >= len(a.appNames) {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		names = append(names, a.appNames[i])
	}
	for i, id := range a.snap.CaptureAppIDs {
		if id == "" || !a.appSelection[id] || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		name := id
		if i < len(a.snap.CaptureAppNames) && a.snap.CaptureAppNames[i] != "" {
			name = a.snap.CaptureAppNames[i]
		}
		names = append(names, name)
	}
	return ids, names
}

// scheduleAppSelection applies the live selection after a short pause, so a
// burst of checks restarts capture once instead of per click.
func (a *App) scheduleAppSelection() {
	ids, names := a.selectedAppIDs()
	label := ""
	switch len(ids) {
	case 0:
	case 1:
		label = names[0]
	default:
		label = locales.SettingsAppCaptureCount(len(ids))
	}
	if a.appApplyTimer != nil {
		a.appApplyTimer.Stop()
	}
	a.appApplyTimer = time.AfterFunc(250*time.Millisecond, func() {
		if a.eng != nil {
			a.eng.SetCaptureApps(ids, names, label)
		}
	})
}

// migrateLegacyAppSelection turns the old single PID saved before bundle-ID
// persistence into the same app's stable ID once the list is loaded.
func (a *App) migrateLegacyAppSelection() {
	if !capture.MultiAppSupported() || len(a.snap.CaptureAppIDs) > 0 || a.snap.CaptureProcessPID == 0 {
		return
	}
	for i, id := range a.appIDs {
		if id == "" || i >= len(a.appPIDs) || i >= len(a.appNames) || a.appPIDs[i] != a.snap.CaptureProcessPID {
			continue
		}
		a.appSelection = map[string]bool{id: true}
		if a.eng != nil {
			a.eng.SetCaptureApps([]string{id}, []string{a.appNames[i]}, a.appNames[i])
		}
		return
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

// trayMenuModel is everything the tray menu shows, derived from one snapshot.
// Rebuilding compares the model, so an unchanged model keeps the cached menu.
type trayMenuModel struct {
	locale     string
	dark       bool
	connecting bool
	selected   string
	connected  string
	muteOn     bool
	lampOff    bool
	devices    []trayMenuDevice
}

// trayMenuDevice is one receiver row.
type trayMenuDevice struct {
	id     string
	name   string
	family string
	online bool
}

// trayMenuKey is what the tray menu's shape depends on. The tooltip follows
// more of the snapshot and is compared on its own.
type trayMenuKey struct {
	locale     string
	dark       bool
	connecting bool
	selected   string
	connected  string
	muteOn     bool
	lampOff    bool
	devices    string
}

func (a *App) trayModel(locale string, dark bool) trayMenuModel {
	m := trayMenuModel{
		locale:     locale,
		dark:       dark,
		connecting: a.snap.State != sender.StateIdle,
		selected:   a.snap.SelectedID,
		muteOn:     a.cfg.MuteLocalOnConnect,
		lampOff:    a.snap.LampOff,
		devices:    make([]trayMenuDevice, len(a.snap.Devices)),
	}
	if m.connecting {
		m.connected = a.snap.SelectedID
	}
	if m.selected == "" {
		m.selected = a.selID
	}
	for i, d := range a.snap.Devices {
		m.devices[i] = trayMenuDevice{id: d.ID, name: d.Name, family: deviceIconKey(d.Model, d.IsStereoGroup()), online: d.Online}
	}
	return m
}

func (m trayMenuModel) key() trayMenuKey {
	var b strings.Builder
	for _, d := range m.devices {
		fmt.Fprintf(&b, "%d:%s|%d:%s|%s|%t\n", len(d.id), d.id, len(d.name), d.name, d.family, d.online)
	}
	return trayMenuKey{
		locale:     m.locale,
		dark:       m.dark,
		connecting: m.connecting,
		selected:   m.selected,
		connected:  m.connected,
		muteOn:     m.muteOn,
		lampOff:    m.lampOff,
		devices:    b.String(),
	}
}

// deviceIcon is the key of the receiver's glyph: the device in session takes
// the accent-colored variant for the appearance, the other rows the template
// in the menu's text color.
func (m trayMenuModel) deviceIcon(d trayMenuDevice) string {
	if d.id != m.connected {
		return "dev:" + d.family
	}
	if m.dark {
		return "devond:" + d.family
	}
	return "devon:" + d.family
}

func (m trayMenuModel) muteIcon() string {
	if m.muteOn {
		return "bellOff"
	}
	return "bell"
}

// muteLabel names the mute row's action: unmuting while the preference is on.
func (m trayMenuModel) muteLabel() string {
	if m.muteOn {
		return locales.TrayUnmuteLocal()
	}
	return locales.TrayMuteLocal()
}

// lampLabel names the ambient light's action: turning off while it is lit.
func (m trayMenuModel) lampLabel() string {
	if m.lampOff {
		return locales.LampTurnOn()
	}
	return locales.LampTurnOff()
}

// lampIcon matches the light's state, as the bell does for the mute row.
func (m trayMenuModel) lampIcon() string {
	if m.lampOff {
		return "lampOff"
	}
	return "lamp"
}

func (a *App) trayMenuItems(m trayMenuModel) []*mygo.MenuItem {
	items := make([]*mygo.MenuItem, 0, len(m.devices)+10)
	targeted := false
	for _, d := range m.devices {
		item := &mygo.MenuItem{
			Label: d.name,
			Icon:  a.menuIcons[m.deviceIcon(d)],
			Click: func(*mygo.MenuItem, *mygo.Window) { a.connectDevice(d.id) },
		}
		if !d.online {
			item.Subtitle = locales.ActionOffline()
		}
		targeted = targeted || d.id == m.selected
		items = append(items, item)
	}
	// No targeted row means the current target is not among the visible
	// receivers, so offer the remembered one instead.
	if !targeted {
		items = append(items, &mygo.MenuItem{
			Label:    locales.TrayConnectLast(),
			Icon:     a.menuIcons["connect"],
			Disabled: a.selID == "",
			Click:    func(*mygo.MenuItem, *mygo.Window) { a.connectSelected() },
		})
	}
	items = append(items, mygo.Separator())
	items = append(items,
		&mygo.MenuItem{Label: m.muteLabel(), Icon: a.menuIcons[m.muteIcon()], Click: func(*mygo.MenuItem, *mygo.Window) {
			a.setMuteLocal(!m.muteOn)
		}},
		&mygo.MenuItem{Label: m.lampLabel(), Icon: a.menuIcons[m.lampIcon()], Click: func(*mygo.MenuItem, *mygo.Window) {
			a.setLampOff(!m.lampOff)
		}},
		mygo.Separator(),
		&mygo.MenuItem{Label: locales.TrayShowPanel(), Icon: a.menuIcons["open"], Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		&mygo.MenuItem{Label: locales.TraySettings(), Icon: a.menuIcons["settings"], Click: func(*mygo.MenuItem, *mygo.Window) { a.showSettings() }},
		mygo.Separator(),
		&mygo.MenuItem{Label: locales.TrayCheckUpdates(), Icon: a.menuIcons["update"], Click: func(*mygo.MenuItem, *mygo.Window) { a.openUpdateCheck() }},
		&mygo.MenuItem{Role: mygo.RoleQuit, Label: locales.TrayQuit(), Icon: a.menuIcons["quit"]},
	)
	return items
}

func (a *App) rebuildTray() {
	if a.tray == nil {
		return
	}
	if a.menuIcons == nil {
		a.menuIcons = loadMenuIcons()
	}
	m := a.trayModel(mygo.App.Locale(), mygo.Theme.IsDark())
	key := m.key()
	if a.trayMenu == nil || key != a.menuKey {
		a.menuKey = key
		a.trayMenu = mygo.NewMenu(a.trayMenuItems(m))
	}
	connecting := a.snap.State != sender.StateIdle
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
	a.saveSettings()
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
	if a.snap.TakenOver {
		return locales.StatusTakenOver()
	}
	if a.snap.Reconnecting {
		return locales.StatusReconnecting()
	}
	switch a.snap.State {
	case sender.StateConnecting, sender.StateHandshake:
		return locales.StatusConnecting()
	case sender.StatePairing:
		return locales.StatusPairing()
	case sender.StateStreaming:
		if a.snap.Standby {
			return locales.StatusStandby()
		}
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
	a.rebuildCaptureLabels()
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
	a.saveSettings()
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
	a.saveSettings()
	if a.setThemeSource != nil {
		a.setThemeSource(appearanceSource(mode))
	}
	a.rebuildTray()
}
