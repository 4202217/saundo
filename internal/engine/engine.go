// Package engine is the UI-agnostic session controller. It owns discovery,
// capture, the audio ring, and the sender session, and funnels every input
// through one event-loop goroutine.
package engine

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/analyze"
	"saundo/internal/audio/ring"
	"saundo/internal/capture"
	"saundo/internal/diagnostics"
	"saundo/internal/discovery"
	"saundo/internal/lamp"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// Snapshot is the complete state the UI draws.
type Snapshot struct {
	Devices       []device.Device
	Receivers     []receivers.View
	SelectedID    string
	State         sender.State
	ConnectedName string
	// ActiveTargets lists the live or opening cohort's registry keys in target
	// order; the first is the primary target (SelectedID). While a failed or
	// retrying session waits, it lists the wanted set instead. Empty when idle.
	ActiveTargets []string
	// Targets carries each live target's per-device audio controls. Empty when
	// idle.
	Targets   []TargetControl
	Notice    SessionNotice
	Error     string
	ErrorKey  UserErrorKey
	ErrorArgs []string
	// CredentialReset is true when the saved credentials could not be read
	// and were moved aside this run. The flyout explains the re-pair.
	CredentialReset  bool
	TakenOver        bool
	PinRequired      bool
	PinDevice        string
	PasswordRequired bool
	PasswordDevice   string
	PasswordDeviceID string
	// PasswordTargetID is the target the UI reconnects after the password is
	// saved. It differs from PasswordDeviceID for a stereo member, whose
	// password is stored under the member key but whose pair is the target.
	PasswordTargetID string
	Name             string
	Reconnect        bool
	Reconnecting     bool
	// AudioIdle is true while the source has been silent past the idle
	// threshold, either with the lamp lit or dark. The session stays
	// connected and, inside the pipe window, the compact silence stream keeps
	// the receiver's renderer fed.
	AudioIdle bool
	// Standby is true while the top display is dark and the session is alive.
	// StandbyEnabled and StandbyMinutes carry the setting itself.
	Standby        bool
	StandbyEnabled bool
	StandbyMinutes int
	MuteLocal      MuteLocalState
	MuteLocalError string
	LampOff        bool
	LampCover      bool
	LampPreset     string
	TrayAnimation  bool
	LatencyMS      int
	// ReceiverLatencyMS is the receiver's own arrival-to-render latency from
	// its stream SETUP response. 0 means the receiver did not report one.
	ReceiverLatencyMS   int
	CaptureEndpoint     string
	CaptureName         string
	CaptureRate         int
	CaptureRunning      bool
	CaptureError        string
	CaptureProcessPID   uint32
	CaptureProcessName  string
	CaptureAppIDs       []string
	CaptureAppNames     []string
	CaptureActiveAppIDs []string
	CaptureWaitingApp   bool
	DroppedSamples      uint64
	Diagnostics         Diagnostics
	StrictAuth          bool
	PTPActive           bool
	PTPReady            bool
	PTPOffsetMS         float64
	LogLines            []string
}

// Active reports whether the key belongs to the live or opening cohort, or to
// the wanted set while a retry waits.
func (s Snapshot) Active(key string) bool {
	for _, k := range s.ActiveTargets {
		if k == key {
			return true
		}
	}
	return false
}

type captureInfo struct {
	gen  int
	rate int
	name string
	err  error
}

type captureSource interface {
	Stop()
	Stats() capture.Stats
}

type captureStart struct {
	src captureSource
	err error
}

// settingsSaveDebounce coalesces settings writes on the run loop: a volume
// drag or a burst of toggles costs one disk write, not one per change.
const settingsSaveDebounce = 500 * time.Millisecond

// lastSeenWriteEvery bounds how often a discovery sighting rewrites
// receivers.json. The stale-group grace is a day, so an hour of resolution is
// plenty and avoids a file write on every three-second discovery cycle.
const lastSeenWriteEvery = time.Hour

var reconnectBackoff = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second,
	15 * time.Second, 30 * time.Second,
}

// Engine ties the engine components together. All mutable state is touched on
// the run goroutine; Snapshot is copied under a mutex for the UI.
type Engine struct {
	sets     *settings.Store
	creds    *creds.Store
	log      *diagnostics.Log
	onChange func(Snapshot)

	ring          *ring.Buffer
	captureSink   *ring.Fanout
	level         *analyze.Meter
	levelFeed     *ring.Buffer
	levelStop     chan struct{}
	levelDone     chan struct{}
	levelRunning  atomic.Bool
	silence       *silenceMeter
	memberRings   []*ring.Buffer
	memberDropped uint64
	// memberDropBase is each member ring's counter at the last fold. Its delta
	// is summed for the diagnostics group overflow; DroppedSamples stays the
	// max, which is what stability uses.
	memberDropBase map[*ring.Buffer]uint64
	discovery      *discovery.Service
	tr             transport
	// plan is the live or opening cohort; zero when idle. It carries the
	// session's credentials and is cleared on close.
	plan          sessionPlan
	life          lifecycle
	capture       captureSource
	captureInfo   chan captureInfo
	captureResult chan captureStart
	mute          localMute
	// The mute worker owns endpoint calls; the loop applies checked results.
	muteReq      chan muteRequest
	muteStatus   chan muteStatus
	muteStop     chan struct{}
	muteDone     chan struct{}
	muteOnce     sync.Once
	muteStopOnce sync.Once
	muteRunning  atomic.Bool
	muteGen      int

	commands       chan command
	quit           chan struct{}
	done           chan struct{}
	startOnce      sync.Once
	stopOnce       sync.Once
	started        atomic.Bool
	stopping       bool
	now            func() time.Time
	newCohort      func(sessionPlan) (transport, error)
	captureStartFn func(capture.Config) (captureSource, error)

	// The endpoint mute is injected so tests never touch the machine's audio.
	setEndpointMuted func(string, bool) error
	defaultEndpoint  func() (capture.Endpoint, error)
	endpoints        func() ([]capture.Endpoint, error)
	// Process mute is the per-app tap's own mute on macOS. Tests inject both
	// the capability and the call so Windows's fallback stays covered.
	processMute     bool
	setProcessMuted func(bool) error

	mu   sync.Mutex
	snap Snapshot

	devices          []device.Device
	selectedID       string
	activeHost       string
	activePort       uint16
	activeMembers    []device.Device
	connectedName    string
	notice           SessionNotice
	lastError        string
	lastErrorKey     UserErrorKey
	lastErrorArgs    []string
	credentialReset  bool
	pinRequired      bool
	pinDevice        string
	passwordRequired bool
	passwordDevice   string
	passwordDeviceID string
	passwordTargetID string
	// receiverLatency is the largest arrival-to-render latency any member of
	// the active session reported. It is diagnostic only.
	receiverLatency int
	// sessionDropped accumulates drops since the current session reached
	// streaming. lastRawDropped is the sink counter at the last fold, and
	// dropSessionActive is true between that transition and close. The
	// accumulation survives a capture sink Reset, which zeroes the raw count.
	sessionDropped    uint64
	lastRawDropped    uint64
	dropSessionActive bool
	// sessionStartedAt is when the current session reached streaming, for
	// diagnostics uptime. Zero when no session streams.
	sessionStartedAt time.Time
	// stability keeps the trailing-window trouble the connection card reads,
	// and statsAt is the next diagnostics publish while a session streams.
	stability            sessionStability
	statsAt              time.Time
	setsValue            settings.Settings
	captureEndpoint      string
	captureName          string
	captureProcess       uint32
	captureApps          []string
	captureAppNames      []string
	captureActiveApps    []string
	captureWaitingApp    bool
	captureRate          int
	captureOpenSeq       uint64
	captureRunning       bool
	captureError         string
	captureErrorLogged   string
	captureErrorLoggedAt time.Time
	captureStarting      bool
	captureQueued        bool
	captureGen           int
	captureRetryAt       time.Time
	captureRetryNum      int
	lastMissingLogAt     time.Time
	settingsDirtyAt      time.Time
	settingsErrorLogged  string
	idle                 idlePhase
	standbyHoldAt        time.Time
	darkBaseline         time.Time
	lastLoudSeen         time.Time
	longIdlePending      bool
	longIdlePendingLog   bool
	autoTried            bool
	autoPendingAt        time.Time

	// track is the last Now Playing payload, replayed to a rebuilt cohort.
	track trackInfo

	// lampJPEG renders a preset; tests replace it. lampImage is the encoded
	// image for lampImagePreset, kept off the sender's tick path.
	lampJPEG          func(preset string) ([]byte, error)
	lampImage         []byte
	lampImagePreset   string
	lampSkipLoggedFor string

	// receivers owns per-receiver configuration. reg is the authoritative
	// in-memory copy; only this goroutine mutates it or saves the store.
	recvStore      *receivers.Store
	reg            receivers.Registry
	discovered     []device.Device
	receiversViews []receivers.View
	passwords      map[string]bool
}

// NewWithReceivers creates the engine over explicit settings, credentials,
// and receiver stores. The startup migration hands in the same registry
// store it initialized, so the engine reads what the migration wrote.
func NewWithReceivers(sets *settings.Store, credsStore *creds.Store, recvStore *receivers.Store, log *diagnostics.Log, onChange func(Snapshot)) *Engine {
	cfg := sets.Load()
	e := &Engine{
		sets:      sets,
		creds:     credsStore,
		recvStore: recvStore,
		log:       log,
		onChange:  onChange,
		// Eight seconds at 48 kHz stereo: the ring outlasts capture jitter
		// and a stalled consumer. A silent source does not fill it — the
		// pacer keeps draining it at 44.1 kHz through the silence keepalive
		// — so the drop counter keeps meaning real capture overflow.
		ring:            ring.New(48000 * 2 * 8),
		levelFeed:       ring.New(levelFeedSamples),
		levelStop:       make(chan struct{}),
		levelDone:       make(chan struct{}),
		captureInfo:     make(chan captureInfo, 8),
		captureResult:   make(chan captureStart, 4),
		commands:        make(chan command, 32),
		quit:            make(chan struct{}),
		done:            make(chan struct{}),
		setsValue:       cfg,
		selectedID:      lastSessionFirst(cfg),
		captureEndpoint: cfg.CaptureEndpointID,
		captureName:     cfg.CaptureEndpointName,
		captureProcess:  cfg.CaptureProcessPID,
		captureApps:     append([]string(nil), cfg.CaptureAppIDs...),
		captureAppNames: append([]string(nil), cfg.CaptureAppNames...),
		now:             time.Now,
		stopping:        false,
		passwords:       map[string]bool{},
		lampJPEG:        lamp.JPEG,
		muteReq:         make(chan muteRequest, 1),
		muteStatus:      make(chan muteStatus, 1),
		muteStop:        make(chan struct{}),
		muteDone:        make(chan struct{}),
	}
	if len(e.captureApps) > 0 {
		e.captureName = strings.Join(e.captureAppNames, " + ")
	}
	e.dropUnsupportedAppSelection()
	// A registry that cannot be read leaves the engine with no remembered
	// receivers but does not stop it; manual and discovered receivers still
	// work. The corrupt-file guard keeps a later save from destroying the
	// file while it cannot be parsed.
	if reg, err := recvStore.Load(); err != nil {
		e.logf("receivers registry: %v", err)
	} else {
		e.reg = reg
	}
	for _, entry := range e.reg.Entries {
		if e.creds.HasPassword(entry.Key) {
			e.passwords[entry.Key] = true
		}
	}
	e.pinExistingCredentials()
	e.noteCredentialQuarantine()
	e.newCohort = e.openCohort
	e.captureStartFn = startCaptureDefault
	e.captureSink = ring.NewFanout(e.ring)
	e.level = analyze.New(0)
	e.silence = newSilenceMeter(e.captureSink, e.levelFeed)
	e.setEndpointMuted = capture.SetEndpointMuted
	e.defaultEndpoint = capture.DefaultEndpoint
	e.endpoints = capture.Endpoints
	e.processMute = capture.ProcessMuteSupported()
	e.setProcessMuted = capture.SetProcessMuted
	e.mute.state = e.idleMuteState()
	e.prepareLampImage()
	e.rebuildTargets()
	e.snap = e.buildSnapshot()
	return e
}

func startCaptureDefault(cfg capture.Config) (captureSource, error) {
	src, err := capture.Start(cfg)
	if err != nil {
		return nil, err
	}
	return src, nil
}

// Start launches discovery, capture, and the event loop.
func (e *Engine) Start() {
	e.startOnce.Do(func() {
		e.stopping = false
		e.discovery = discovery.New(3*time.Second, 1500*time.Millisecond)
		e.discovery.SetSeedHosts(receiverHosts(e.reg))
		e.discovery.Start()
		e.startMuteWorker()
		e.startLevelPump()
		e.startCapture()
		e.started.Store(true)
		go e.run()
	})
}

// Stop ends the session and all discovery and capture work.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		close(e.quit)
		if e.started.Load() {
			<-e.done
		}
	})
}

// Snapshot returns the latest UI state.
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snap
}

// LevelFrame returns the capture level and bands as they were delay ago. The
// Now Playing card passes the receiver latency so the meter follows the audio
// being heard, not the audio just captured.
func (e *Engine) LevelFrame(delay time.Duration) analyze.Frame {
	if e.level == nil {
		return analyze.Frame{}
	}
	frame, _ := e.level.At(time.Now(), delay)
	return frame
}

// Refresh asks discovery to browse now.
func (e *Engine) Refresh() { e.send(command{kind: cmdRefresh}) }

// Connect starts a session with the device ID, or adds that target to the
// live cohort. An active key is a no-op.
func (e *Engine) Connect(deviceID string) {
	e.send(command{kind: cmdConnect, deviceID: deviceID})
}

// Disconnect stops the current session.
func (e *Engine) Disconnect() { e.send(command{kind: cmdDisconnect}) }

// SubmitPIN supplies the receiver's on-screen code.
func (e *Engine) SubmitPIN(code string) {
	e.send(command{kind: cmdPin, text: code})
}

// DismissPasswordPrompt closes the first-connect password prompt without
// changing the session.
func (e *Engine) DismissPasswordPrompt() {
	e.send(command{kind: cmdDismissPassword})
}

// DismissCredentialReset clears the credential-recovery notice for this run.
func (e *Engine) DismissCredentialReset() {
	e.send(command{kind: cmdDismissCredentialReset})
}

// SetCaptureEndpoint selects an output endpoint; empty follows the default.
func (e *Engine) SetCaptureEndpoint(id, name string) {
	e.send(command{kind: cmdCaptureEndpoint, deviceID: id, name: name})
}

// SetStrictAuth toggles receiver proof enforcement.
func (e *Engine) SetStrictAuth(strict bool) {
	v := 0.0
	if strict {
		v = 1
	}
	e.send(command{kind: cmdStrictAuth, value: v})
}

// SetName changes the sender name receivers display.
func (e *Engine) SetName(name string) {
	e.send(command{kind: cmdName, text: name})
}

// dropUnsupportedAppSelection clears a saved app or process selection on a
// platform that does not offer per-app capture, so the stale setting cannot
// strand system capture. It reports whether it changed anything.
func (e *Engine) dropUnsupportedAppSelection() bool {
	if capture.AppCaptureSupported() {
		return false
	}
	if e.captureProcess == 0 && len(e.captureApps) == 0 {
		return false
	}
	e.captureProcess = 0
	e.captureApps = nil
	e.captureAppNames = nil
	e.captureName = e.setsValue.CaptureEndpointName
	e.setsValue.CaptureProcessPID = 0
	e.setsValue.CaptureProcessName = ""
	e.setsValue.CaptureAppIDs = nil
	e.setsValue.CaptureAppNames = nil
	e.markSettingsDirty()
	e.logf("capture: dropped the app selection; per-app capture is not offered on this platform")
	return true
}

// SetCaptureProcess selects application capture; pid 0 returns to endpoint
// capture.
func (e *Engine) SetCaptureProcess(pid uint32, name string) {
	e.send(command{kind: cmdCaptureProcess, value: float64(pid), name: name})
}

// SetCaptureApps selects apps by stable ID (bundle IDs on macOS; empty
// returns to endpoint capture). names is parallel to ids, and label is the
// status label the app built for the selection.
func (e *Engine) SetCaptureApps(ids, names []string, label string) {
	e.send(command{kind: cmdCaptureApps, ids: ids, names: names, label: label})
}

// SetLatency sets the presentation latency in milliseconds; it applies to the
// next session.
func (e *Engine) SetLatency(ms int) {
	e.send(command{kind: cmdLatency, value: float64(ms)})
}

// ApplyLatency sets the presentation latency and restarts an active session
// so the receiver picks it up now. An idle engine applies it on the next
// connection like SetLatency.
func (e *Engine) ApplyLatency(ms int) {
	e.send(command{kind: cmdLatency, value: float64(ms), reconnect: true})
}

// SetLatencySettings keeps the engine's settings copy in step with the
// app-only latency fields, so the next settings save cannot overwrite them.
func (e *Engine) SetLatencySettings(mode settings.LatencyMode, customMS int, showSwitch bool) {
	e.send(command{kind: cmdLatencySettings, text: string(mode), value: float64(customMS), show: showSwitch})
}

// SetReceiverPassword stores the RTSP password for a device; the engine
// moves it into the DPAPI store. See receivers.go for the management
// commands.
func (e *Engine) SetReceiverPassword(deviceID, password string) {
	e.send(command{kind: cmdPassword, deviceID: deviceID, name: password})
}

// SetReconnect toggles automatic reconnection after a dropped session.
func (e *Engine) SetReconnect(on bool) {
	v := 0.0
	if on {
		v = 1
	}
	e.send(command{kind: cmdReconnect, value: v})
}

// SetTrayAnimation toggles the animated tray icon preference.
func (e *Engine) SetTrayAnimation(on bool) {
	v := 0.0
	if on {
		v = 1
	}
	e.send(command{kind: cmdTrayAnimation, value: v})
}

// SetMuteLocal turns the local-output mute on or off for this and later
// sessions. Turning it on while streaming mutes at once; turning it off
// restores the endpoint.
func (e *Engine) SetMuteLocal(on bool) {
	e.send(command{kind: cmdMuteLocal, show: on})
}

// SetNowPlaying pushes track metadata and cover art to the receiver.
func (e *Engine) SetNowPlaying(title, artist, album string, cover []byte, coverMIME string) {
	e.send(command{kind: cmdNowPlaying, text: title, name: artist, album: album, cover: cover, coverMIME: coverMIME})
}

// SetLamp turns the light off manually or on with a cover or template. Off
// is the "关灯" state; the ambient feature itself is always present. The
// setting applies to the live session at once and persists for the next one.
func (e *Engine) SetLamp(off, cover bool, preset string) {
	v := 0.0
	if off {
		v = 1
	}
	e.send(command{kind: cmdLamp, value: v, text: preset, lampCover: cover})
}

// SetStandby turns the ambient light's idle standby on or off and picks its
// idle threshold in minutes. Unknown values fall back to 30 minutes.
func (e *Engine) SetStandby(enabled bool, minutes int) {
	e.send(command{kind: cmdStandby, show: enabled, minutes: minutes})
}

func (e *Engine) send(cmd command) {
	select {
	case e.commands <- cmd:
	case <-e.quit:
	}
}

func (e *Engine) run() {
	defer close(e.done)
	// The launch auto-connect happens before the first select, so a manual
	// receiver starts without waiting for an mDNS cycle. A discovered target
	// waits for setDevices to report it online.
	e.maybeAutoConnect()
	for {
		var evCh <-chan raopio.Event
		var deadCh <-chan struct{}
		if e.tr != nil {
			evCh = e.tr.Events()
			deadCh = e.tr.Dead()
		}
		var updates <-chan []device.Device
		if e.discovery != nil {
			updates = e.discovery.Updates()
		}
		wake := e.nextWake()
		var timerC <-chan time.Time
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		if !wake.IsZero() {
			d := wake.Sub(e.now())
			if d < 0 {
				d = 0
			}
			timer.Reset(d)
			timerC = timer.C
		}
		select {
		case <-e.quit:
			timer.Stop()
			e.shutdown()
			return
		case cmd := <-e.commands:
			timer.Stop()
			e.handleCommand(cmd)
		case ev := <-evCh:
			timer.Stop()
			if e.tr != nil {
				e.tr.HandleEvent(ev)
			}
		case <-deadCh:
			timer.Stop()
			// The transport is beyond saving; teardown runs here, outside
			// HandleEvent, so a stop cannot re-enter a sender callback. A lost
			// member leaves the cohort and the rest keep their session.
			lost := ""
			auth := false
			if e.tr != nil {
				lost = e.tr.LostTarget()
				auth = e.tr.Failure().Auth
			}
			e.sessionLost(lost, auth)
		case devs := <-updates:
			timer.Stop()
			e.setDevices(devs)
		case <-timerC:
			e.onWake()
		case info := <-e.captureInfo:
			timer.Stop()
			e.applyCaptureInfo(info)
		case res := <-e.captureResult:
			timer.Stop()
			e.applyCaptureStart(res)
		case st := <-e.muteStatus:
			timer.Stop()
			e.applyMuteStatus(st)
		}
	}
}

func (e *Engine) nextWake() time.Time {
	wake := time.Time{}
	if e.tr != nil {
		if dl := e.tr.NextDeadline(); !dl.IsZero() {
			wake = dl
		}
	}
	// While the session is idle the pacer is disarmed, so poll for the first
	// audible capture block instead of waiting for the sender's timers. The
	// same poll drives the dark threshold.
	if e.idle != idleActive && e.tr != nil && e.tr.State() == sender.StateStreaming {
		poll := e.now().Add(audioPausePoll)
		if wake.IsZero() || poll.Before(wake) {
			wake = poll
		}
	}
	if e.life.reconnectWanted && !e.life.reconnectAt.IsZero() && (wake.IsZero() || e.life.reconnectAt.Before(wake)) {
		wake = e.life.reconnectAt
	}
	if !e.captureRetryAt.IsZero() && (wake.IsZero() || e.captureRetryAt.Before(wake)) {
		wake = e.captureRetryAt
	}
	if stats := e.statsWake(); !stats.IsZero() && (wake.IsZero() || stats.Before(wake)) {
		wake = stats
	}
	if !e.settingsDirtyAt.IsZero() && (wake.IsZero() || e.settingsDirtyAt.Before(wake)) {
		wake = e.settingsDirtyAt
	}
	if w := e.autoPendingWake(); !w.IsZero() && (wake.IsZero() || w.Before(wake)) {
		wake = w
	}
	return wake
}

func (e *Engine) onWake() {
	now := e.now()
	e.syncCaptureOpen()
	if e.autoPendingDue(now) {
		e.maybeAutoConnect()
		if e.tr != nil {
			return // the launch opened a session; its own deadlines take over
		}
	}
	if e.life.reconnectWanted && !e.life.reconnectAt.IsZero() && !now.Before(e.life.reconnectAt) {
		e.attemptReconnect()
		return
	}
	if !e.captureRetryAt.IsZero() && !now.Before(e.captureRetryAt) {
		e.retryCapture()
	}
	if !e.settingsDirtyAt.IsZero() && !now.Before(e.settingsDirtyAt) {
		e.saveSettingsNow()
	}
	if e.rebuildAfterLongIdle() {
		return // the new session's deadlines take over
	}
	e.updateIdlePhase(now)
	if e.tr != nil {
		e.tr.Tick()
	}
	if e.statsDue(now) {
		e.publish()
	}
}

func (e *Engine) shutdown() {
	e.stopping = true
	e.closeSession(false)
	e.life.clearWanted()
	// Force the final unmute through the worker and wait for it, so quitting
	// never leaves the endpoint muted. closeSession already asked when it
	// believed a mute was active; this covers a dropped status.
	e.requestMute(muteRequest{on: false})
	e.stopMuteWorker()
	e.saveSettingsNow()
	if e.capture != nil {
		e.capture.Stop()
		e.capture = nil
	}
	e.stopLevelPump()
	if e.discovery != nil {
		e.discovery.Stop()
	}
	if e.log != nil {
		e.log.Close()
	}
}

// attachMemberRings switches capture from the single-session ring to the
// group's per-member rings. The single ring is not consumed while a group
// session owns the speakers, so it must stop receiving samples.
func (e *Engine) attachMemberRings(rings []*ring.Buffer) {
	if len(rings) == 0 {
		return
	}
	e.captureSink.Remove(e.ring)
	e.memberDropBase = make(map[*ring.Buffer]uint64, len(rings))
	e.memberDropped = 0
	for _, r := range rings {
		r.Reset()
		e.memberDropBase[r] = 0
		e.captureSink.Add(r)
	}
	e.memberRings = rings
}

func (e *Engine) detachMemberRings() {
	if len(e.memberRings) == 0 {
		return
	}
	for _, r := range e.memberRings {
		e.captureSink.Remove(r)
	}
	e.memberRings = nil
	e.memberDropped = 0
	e.memberDropBase = nil
	e.ring.Reset()
	e.captureSink.Add(e.ring)
}

func (e *Engine) handleCommand(cmd command) {
	switch cmd.kind {
	case cmdConnect:
		e.connectTargets(cmd.deviceID)
	case cmdDisconnect:
		e.disconnectAll()
	case cmdDisconnectTarget:
		e.leaveTarget(cmd.deviceID)
	case cmdTargetVolume:
		e.applyTargetVolume(cmd.deviceID, cmd.value)
	case cmdPin:
		if e.tr != nil {
			e.tr.SubmitPin(cmd.text)
		}
		e.pinRequired = false
		e.publish()
	case cmdDismissPassword:
		e.clearPasswordPrompt()
		e.publish()
	case cmdDismissCredentialReset:
		e.credentialReset = false
		e.publish()
	case cmdCaptureEndpoint:
		e.captureEndpoint = cmd.deviceID
		e.captureName = cmd.name
		e.captureProcess = 0
		e.captureApps = nil
		e.captureAppNames = nil
		e.setsValue.CaptureEndpointID = cmd.deviceID
		e.setsValue.CaptureEndpointName = cmd.name
		e.setsValue.CaptureProcessPID = 0
		e.setsValue.CaptureProcessName = ""
		e.setsValue.CaptureAppIDs = nil
		e.setsValue.CaptureAppNames = nil
		e.markSettingsDirty()
		if e.setsValue.MuteLocalOnConnect || e.mute.state == MuteLocalActive {
			e.refreshLocalMute()
		}
		e.restartCapture()
		e.publish()
	case cmdCaptureProcess:
		e.captureProcess = uint32(cmd.value)
		e.captureName = cmd.name
		e.captureApps = nil
		e.captureAppNames = nil
		e.setsValue.CaptureProcessPID = e.captureProcess
		e.setsValue.CaptureProcessName = cmd.name
		e.setsValue.CaptureAppIDs = nil
		e.setsValue.CaptureAppNames = nil
		e.markSettingsDirty()
		if e.setsValue.MuteLocalOnConnect || e.mute.state == MuteLocalActive {
			e.refreshLocalMute()
		}
		e.restartCapture()
		e.publish()
	case cmdCaptureApps:
		e.captureApps = append([]string(nil), cmd.ids...)
		e.captureAppNames = append([]string(nil), cmd.names...)
		e.captureProcess = 0
		label := cmd.label
		if label == "" && len(e.captureAppNames) > 0 {
			label = strings.Join(e.captureAppNames, " + ")
		}
		e.captureName = label
		e.setsValue.CaptureAppIDs = append([]string(nil), e.captureApps...)
		e.setsValue.CaptureAppNames = append([]string(nil), e.captureAppNames...)
		e.setsValue.CaptureProcessPID = 0
		e.setsValue.CaptureProcessName = ""
		e.markSettingsDirty()
		if e.setsValue.MuteLocalOnConnect || e.mute.state == MuteLocalActive {
			e.refreshLocalMute()
		}
		e.restartCapture()
		e.publish()
	case cmdLatency:
		e.setsValue.LatencyMS = int(cmd.value)
		e.markSettingsDirty()
		if cmd.reconnect && e.tr != nil && e.tr.Active() {
			// A PIN wait is pairing progress, not a live stream: restarting
			// over it would discard the code the user is typing.
			if e.tr.WaitingForPin() {
				e.logf("latency apply: %s is waiting for a PIN; session left alone", e.connectedName)
			} else if missing, offline, ok := e.sessionAddressable(); !ok {
				if missing != "" {
					e.fail(ErrLatencyDeviceMissing)
					e.logf("latency apply: device %s not found; session left running", missing)
				} else {
					e.fail(ErrLatencyOffline)
					e.logf("latency apply: %s is offline; session left running", offline)
				}
			} else {
				e.life.userRebuild()
				// A sender applies latency only at stream SETUP, so the whole
				// cohort rebuilds with the new per-member values and epoch.
				e.rebuildSession()
			}
		}
		e.publish()
	case cmdLatencySettings:
		e.setsValue.LatencyMode = settings.LatencyMode(cmd.text)
		e.setsValue.LatencyCustomMS = int(cmd.value)
		e.setsValue.ShowLatencySwitch = cmd.show
		e.markSettingsDirty()
		e.publish()
	case cmdPassword:
		if err := e.savePassword(cmd.deviceID, cmd.name); err != nil {
			e.fail(ErrSavePassword, err.Error())
		}
		e.publish()
	case cmdRefresh:
		if e.discovery != nil {
			e.setDevices(e.discovery.Snapshot())
		}
	case cmdStrictAuth:
		e.setsValue.StrictReceiverAuth = cmd.value != 0
		if e.tr != nil {
			e.tr.SetStrictReceiverAuth(e.setsValue.StrictReceiverAuth)
		}
		e.markSettingsDirty()
		e.publish()
	case cmdName:
		name := strings.TrimSpace(cmd.text)
		if name == "" {
			name = "声渡"
		}
		e.setsValue.Name = name
		if e.tr != nil {
			e.tr.SetIdentity(sender.Identity{Name: name, DeviceID: e.setsValue.DeviceID, Model: "iPhone14,3"})
		}
		e.markSettingsDirty()
		e.publish()
	case cmdReconnect:
		e.setsValue.Reconnect = cmd.value != 0
		if !e.setsValue.Reconnect {
			e.reconnectPendingOff()
		}
		e.markSettingsDirty()
		e.publish()
	case cmdLamp:
		preset := lamp.Normalize(cmd.text)
		if preset != cmd.text {
			e.logf("ambient light: preset %q is unknown, using %q", cmd.text, preset)
		}
		e.setsValue.Lamp = settings.LampSettings{Off: cmd.value != 0, Cover: cmd.lampCover, Preset: preset}
		now := e.now()
		// A manual lamp action always wins over an existing dark phase; the
		// next audible block ends the hold.
		e.standbyHoldAt = now
		e.prepareLampImage()
		e.idle = e.idlePhase(now)
		e.applyLamp()
		e.markSettingsDirty()
		e.publish()
	case cmdStandby:
		e.setsValue.Standby = settings.StandbySettings{
			Enabled:     cmd.show,
			IdleMinutes: settings.NormalizeStandbyMinutes(cmd.minutes),
		}
		e.idle = e.idlePhase(e.now())
		e.applyLamp()
		e.markSettingsDirty()
		e.publish()
	case cmdTrayAnimation:
		e.setsValue.TrayAnimation = cmd.value != 0
		e.markSettingsDirty()
		e.publish()
	case cmdMuteLocal:
		e.setsValue.MuteLocalOnConnect = cmd.show
		e.markSettingsDirty()
		if cmd.show {
			e.applyLocalMute()
		} else {
			e.restoreLocalMute()
		}
		e.publish()
	case cmdNowPlaying:
		e.track.remember(cmd.text, cmd.name, cmd.album, cmd.cover, cmd.coverMIME)
		if e.tr != nil {
			e.tr.SetNowPlaying(cmd.text, cmd.name, cmd.album, cmd.cover, cmd.coverMIME)
			e.publish()
		}
	case cmdAddReceiver:
		e.handleAddReceiver(cmd.spec)
	case cmdUpdateReceiver:
		e.handleUpdateReceiver(cmd.deviceID, cmd.patch)
	case cmdRemoveReceiver:
		e.handleRemoveReceiver(cmd.deviceID)
	case cmdLinkReceivers, cmdUpdateCombo, cmdUnlinkCombo, cmdSwapComboSides:
		e.dispatchCombo(cmd)
	}
}

func (e *Engine) planReconnect() {
	delay, scheduled := e.life.planRetry(e.now())
	if scheduled {
		e.logf("reconnect scheduled in %s (attempt %d)", delay, e.life.reconnectNum)
	} else {
		e.fail(ErrReconnectStopped, fmt.Sprint(e.life.reconnectNum))
		e.logf("giving up after %d reconnect attempts", e.life.reconnectNum)
	}
	e.publish()
}

func (e *Engine) reconnectPendingOff() {
	e.life.cancelRetry()
	e.publish()
}

// armReconnect schedules the next automatic retry when the gate allows one
// and none is pending. The zero retry time is the once guard: a retry that is
// already armed is consumed by its own due tick.
func (e *Engine) armReconnect() {
	if e.reconnectShouldRetry() && e.life.reconnectAt.IsZero() {
		e.planReconnect()
	}
}

func (e *Engine) attemptReconnect() {
	if e.life.retryDue(e.tr != nil && e.tr.Active(), e.now()) == retryPushOut {
		return
	}
	keys := e.pruneWanted(e.life.wantedKeys)
	if len(keys) == 0 {
		// The wait is indefinite by design; the log line is not. Throttle it
		// so a device that never returns logs once a minute, not every 5 s.
		e.life.reconnectAt = e.now().Add(5 * time.Second)
		if e.lastMissingLogAt.IsZero() || e.now().Sub(e.lastMissingLogAt) >= time.Minute {
			e.lastMissingLogAt = e.now()
			e.logf("reconnect: device %s not discovered yet", e.selectedID)
		}
		return
	}
	e.lastMissingLogAt = time.Time{}
	// A target that no longer passes a command guard (local, FairPlay, a
	// stereo member gone, a moved endpoint) is a synchronous refusal: surface
	// it, keep the wanted set, and let the backoff try again.
	if key, err := e.validateWanted(keys); err != nil {
		if errors.Is(err, errAwaitInput) {
			// The prompt is already set. Waiting for the user is not a
			// failure; the retry resumes when the user acts.
			e.logf("reconnect: %s needs the user's input; waiting", key)
			e.publish()
			return
		}
		e.failPlan(err)
		e.logf("reconnect: %s refused: %v", key, err)
		e.armReconnect()
		e.publish()
		return
	}
	e.clearPasswordPrompt()
	e.life.setWanted(keys)
	e.life.attemptStarted()
	plan, err := e.buildPlan(keys)
	if err != nil {
		// A guard or clock conflict is deterministic; rebuilding the same
		// plan cannot succeed. Surface it and re-arm the backoff rather than
		// stopping the retry loop.
		e.failPlan(err)
		e.logf("reconnect: plan refused: %v", err)
		e.armReconnect()
		e.publish()
		return
	}
	e.startPlan(plan, false)
}

func (e *Engine) reconnectShouldRetry() bool {
	return e.life.mayAutoRetry(e.stopping, e.setsValue.Reconnect, e.selectedID != "")
}

// prepareLampImage renders the selected template on the engine goroutine, so
// the sender's callback path only copies bytes. A render failure keeps the
// setting and retries on the next change or connect.
func (e *Engine) prepareLampImage() {
	preset := lamp.Normalize(e.setsValue.Lamp.Preset)
	if e.lampImagePreset == preset && len(e.lampImage) > 0 {
		return
	}
	data, err := e.lampJPEG(preset)
	if err != nil {
		e.logf("ambient light: render %s: %v", preset, err)
		e.lampImage, e.lampImagePreset = nil, ""
		return
	}
	e.lampImage, e.lampImagePreset = data, preset
}

// applyLamp applies the manual off state and the two lit modes. Off sends
// the paused playback state, which stops the Now Playing renderer and turns
// the top display dark without touching the audio. Cover hands the artwork
// slot to the media cover; a template owns it. While the idle phase is dark,
// the artwork goes first and the playback state stays paused, so the receiver
// keeps the right image for the next state 1. A receiver that cannot show
// top-display artwork is skipped with one diagnostic line per session.
func (e *Engine) applyLamp() {
	if e.tr == nil {
		return
	}
	if !e.lampTargetCapable() {
		if e.lampSkipLoggedFor != e.selectedID {
			e.lampSkipLoggedFor = e.selectedID
			e.logf("ambient light: %s does not support top-display artwork; not sent", e.nameFor(e.selectedID))
		}
		return
	}
	if e.setsValue.Lamp.Off {
		e.logf("ambient light: off (manual) on %s", e.connectedName)
		e.tr.SetPlaybackState(false)
		return
	}
	playing := e.idle != idleDark
	if e.setsValue.Lamp.Cover {
		e.tr.SetLampArtwork(nil, "", false)
		e.logf("ambient light: cover on %s", e.connectedName)
		e.tr.SetPlaybackState(playing)
		return
	}
	preset := lamp.Normalize(e.setsValue.Lamp.Preset)
	if e.lampImagePreset != preset || len(e.lampImage) == 0 {
		return
	}
	e.logf("ambient light: template %s (%d bytes) on %s", e.lampImagePreset, len(e.lampImage), e.connectedName)
	e.tr.SetLampArtwork(e.lampImage, "image/jpeg", true)
	e.tr.SetPlaybackState(playing)
}

// lampTargetCapable fails closed: an unknown model or unknown software version
// is not capable. A cohort is capable when any of its members is; the cohort
// transport filters the rest per member.
func (e *Engine) lampTargetCapable() bool {
	if e.plan.live() {
		for _, m := range e.plan.members {
			if device.TopDisplayCapable(m.device) {
				return true
			}
		}
		return false
	}
	d, ok := e.find(e.selectedID)
	if !ok {
		return false
	}
	if d.IsStereoGroup() {
		for _, m := range d.Members {
			if device.TopDisplayCapable(m) {
				return true
			}
		}
		return false
	}
	return device.TopDisplayCapable(d)
}

func (e *Engine) clearPasswordPrompt() {
	e.passwordRequired = false
	e.passwordDevice = ""
	e.passwordDeviceID = ""
	e.passwordTargetID = ""
}

func needsReceiverPassword(a device.Auth) bool {
	return a == device.AuthHAPPassword || a == device.AuthPassword
}

func (e *Engine) onPinRequired(name string) {
	e.pinRequired = true
	e.pinDevice = name
	e.logf("PIN required for %s", name)
	e.publish()
}

func (e *Engine) onCredentials(deviceID, credsJSON string) {
	if err := e.creds.Save(deviceID, credsJSON); err != nil {
		e.logf("could not save credentials: %v", err)
	} else {
		e.logf("credentials saved for %s", deviceID)
		e.pinPairedEndpoint(deviceID)
		// A fresh pairing proves the rebuilt store is usable, so the recovery
		// notice has done its job.
		e.credentialReset = false
	}
	e.publish()
}

// noteCredentialQuarantine surfaces an unreadable credentials file that the
// store moved aside. The next pairing writes a fresh file; the notice tells
// the user why a receiver asks to pair again.
func (e *Engine) noteCredentialQuarantine() {
	q := e.creds.TakeQuarantine()
	if q == "" {
		return
	}
	e.credentialReset = true
	e.logf("credentials file was unreadable; moved to %s; a receiver may need to be paired again", q)
}

// pinPairedEndpoint records the address a fresh pairing used, so a later
// advertisement cannot move that device id to another host.
func (e *Engine) pinPairedEndpoint(deviceID string) {
	host, port := e.activeHost, e.activePort
	if deviceID != e.selectedID {
		found := false
		for _, m := range e.activeMembers {
			if m.ID == deviceID {
				host, port, found = m.Host, m.Port, true
				break
			}
		}
		if !found {
			return
		}
	}
	if host == "" || port == 0 {
		return
	}
	if entry, ok := e.reg.Get(deviceID); ok {
		if entry.PairedHost == host && entry.PairedPort == port {
			return
		}
		entry.PairedHost, entry.PairedPort = host, port
		e.reg.Put(entry)
		e.saveRegistry()
		return
	}
	for i := range e.reg.Entries {
		entry := &e.reg.Entries[i]
		for j := range entry.Members {
			m := &entry.Members[j]
			if m.ID != deviceID {
				continue
			}
			if m.PairedHost == host && m.PairedPort == port {
				return
			}
			m.PairedHost, m.PairedPort = host, port
			e.saveRegistry()
			return
		}
	}
}

// pinExistingCredentials pins a remembered receiver to the address it has on
// disk. Credentials written before the pin existed have no better record of
// the paired endpoint; pinning the last known address keeps a later
// advertisement from moving them.
func (e *Engine) pinExistingCredentials() {
	changed := false
	pin := func(key, host string, port uint16, set func(string, uint16)) {
		if host == "" || port == 0 {
			return
		}
		if e.creds.Load(key) == "" && !e.creds.HasPassword(key) {
			return
		}
		set(host, port)
		changed = true
	}
	for i := range e.reg.Entries {
		entry := &e.reg.Entries[i]
		if !entry.IsManual() && entry.PairedHost == "" {
			pin(entry.Key, entry.Host, entry.Port, func(host string, port uint16) {
				entry.PairedHost, entry.PairedPort = host, port
			})
		}
		for j := range entry.Members {
			m := &entry.Members[j]
			if m.PairedHost != "" {
				continue
			}
			pin(m.ID, m.Host, m.Port, func(host string, port uint16) {
				m.PairedHost, m.PairedPort = host, port
			})
		}
	}
	if changed {
		e.saveRegistry()
	}
}

func (e *Engine) pinGroupMembers(d device.Device) device.Device {
	if !d.IsStereoGroup() {
		return d
	}
	entry, ok := e.reg.Get(d.ID)
	if !ok || len(entry.Members) == 0 {
		return d
	}
	pinned := make(map[string]receivers.Member, len(entry.Members))
	for _, m := range entry.Members {
		pinned[m.ID] = m
	}
	members := append([]device.Device(nil), d.Members...)
	for i := range members {
		m, ok := pinned[members[i].ID]
		if !ok || m.PairedHost == "" || m.PairedPort == 0 {
			continue
		}
		if members[i].Host != m.PairedHost || members[i].Port != m.PairedPort {
			e.logf("member %s advertised %s:%d, paired %s:%d; using the paired endpoint",
				members[i].Name, members[i].Host, members[i].Port, m.PairedHost, m.PairedPort)
		}
		members[i].Host, members[i].Port = m.PairedHost, m.PairedPort
	}
	d.Members = members
	return d
}

func (e *Engine) onStateChanged(state sender.State) {
	if state == sender.StateStreaming && !e.dropSessionActive {
		e.beginDropSession()
	}
	e.logf("state: %s", state)
	e.publish()
}

// beginDropSession starts the session's drop accounting. The raw capture
// counter may already hold idle overflow; only the delta from here counts.
func (e *Engine) beginDropSession() {
	e.sessionDropped = 0
	e.lastRawDropped = 0
	e.memberDropped = 0
	if e.memberDropBase == nil {
		e.memberDropBase = make(map[*ring.Buffer]uint64, len(e.memberRings))
	}
	for _, r := range e.memberRings {
		e.memberDropBase[r] = r.Dropped()
	}
	if e.captureSink != nil {
		e.lastRawDropped = e.captureSink.Dropped()
	}
	e.dropSessionActive = true
}

func (e *Engine) endDropSession() {
	e.dropSessionActive = false
	e.sessionDropped = 0
	e.lastRawDropped = 0
	e.memberDropped = 0
	e.memberDropBase = nil
}

// foldCaptureDrops adds the sink's new drops to the session total. A raw
// counter below the last fold means the sink was reset; the accumulated total
// survives and folding resumes from zero.
func (e *Engine) foldCaptureDrops() {
	if e.captureSink == nil {
		return
	}
	raw := e.captureSink.Dropped()
	if raw >= e.lastRawDropped {
		e.sessionDropped += raw - e.lastRawDropped
	}
	e.lastRawDropped = raw
	for _, r := range e.memberRings {
		if e.memberDropBase == nil {
			e.memberDropBase = make(map[*ring.Buffer]uint64, len(e.memberRings))
		}
		base := e.memberDropBase[r]
		memberRaw := r.Dropped()
		if memberRaw >= base {
			e.memberDropped += memberRaw - base
		}
		e.memberDropBase[r] = memberRaw
	}
}

// resetCaptureSink folds the drops accrued before a capture reset into the
// session total, then clears the sink. Without the fold, a mid-session
// endpoint or format change would zero the counter and hide the earlier drops.
func (e *Engine) resetCaptureSink() {
	if e.captureSink == nil {
		return
	}
	e.foldCaptureDrops()
	e.captureSink.Reset()
	e.lastRawDropped = 0
	for _, r := range e.memberRings {
		e.memberDropBase[r] = 0
	}
}

// onReceiverLatency records the receiver's own arrival-to-render latency. It
// is diagnostic: the configured presentation latency stays the scheduler's
// budget, and the receiver's output latency is not added to it.
func (e *Engine) onReceiverLatency(ms int) {
	if ms <= 0 || ms <= e.receiverLatency {
		return
	}
	e.receiverLatency = ms
	e.logf("receiver reports %d ms arrival-to-render latency", ms)
	e.publish()
}

// senderLogFor tags every sender line with the receiver it belongs to. A
// stereo pair runs one sender per member, and without the tag their cover,
// playback-state, and reply lines are indistinguishable in the log.
func (e *Engine) senderLogFor(name string) sender.Logger {
	return func(level sender.Level, format string, args ...any) {
		prefix := "info"
		if level == sender.LevelWarn {
			prefix = "warn"
		}
		line := fmt.Sprintf(format, args...)
		if name == "" {
			e.logf("%s: %s", prefix, line)
			return
		}
		e.logf("%s [%s]: %s", prefix, name, line)
	}
}

func (e *Engine) logf(format string, args ...any) {
	if e.log != nil {
		e.log.Printf(format, args...)
	}
}

func (e *Engine) setDevices(devs []device.Device) {
	e.discovered = devs
	e.rememberDiscovered()
	e.rebuildTargets()
	if e.maybeAutoConnect() {
		return
	}
	e.publish()
}

// rememberDiscovered persists first sightings, keeps the last address and
// sighting time, and refreshes the name and model of a remembered receiver, so
// the receiver list keeps offline rows after a receiver leaves the network.
// Stereo pairs are remembered under their group key so their hidden, delay,
// password, and auto-connect settings survive the pair going offline; a group
// unseen for receivers.GroupGrace is pruned with its auto-connect flag. Only
// an actual change writes the file.
func (e *Engine) rememberDiscovered() {
	changed := false
	now := e.now()
	live := make(map[string]bool, len(e.discovered))
	for _, d := range e.discovered {
		live[d.ID] = true
		entry, ok := e.reg.Get(d.ID)
		if !ok {
			// A password moved by the startup migration belongs to a device
			// that had no registry entry yet; seed the projection on the
			// first sighting.
			if e.creds.HasPassword(d.ID) {
				e.passwords[d.ID] = true
			}
			featureBits, featureUnknown := d.Features.Persisted()
			e.reg.Put(receivers.Entry{
				Key: d.ID, Kind: receivers.KindDiscovered,
				Name: d.Name, Model: d.Model, OSVersion: d.OSVersion, AirPlay2: d.AirPlay2,
				Features: featureBits, FeaturesUnknown: featureUnknown,
				SF: d.SF, Auth: d.Auth,
				Members: memberEntries(d.Members),
				Host:    d.Host, Port: d.Port, LastSeen: now,
			})
			changed = true
			continue
		}
		if entry.IsManual() {
			// A typed-in receiver keeps its own name, endpoint, and type.
			continue
		}
		entryChanged := false
		if d.Name != "" && entry.Name != d.Name {
			entry.Name = d.Name
			entryChanged = true
		}
		if d.Model != "" && entry.Model != d.Model {
			entry.Model = d.Model
			entryChanged = true
		}
		if d.OSVersion != "" && entry.OSVersion != d.OSVersion {
			entry.OSVersion = d.OSVersion
			entryChanged = true
		}
		if d.AirPlay2 && !entry.AirPlay2 {
			entry.AirPlay2 = true
			entryChanged = true
		}
		// Keep the last address so a receiver that is offline now can be
		// probed directly on the next launch. A stereo group stores the
		// leader's endpoint, which rediscovers the pair.
		if d.Host != "" && d.Port != 0 && (entry.Host != d.Host || entry.Port != d.Port) {
			entry.Host, entry.Port = d.Host, d.Port
			entryChanged = true
		}
		featureBits, featureUnknown := d.Features.Persisted()
		// A known value replaces the cache, zero included: an advertisement
		// that now reports no feature bits is the receiver's current truth.
		// An unknown one leaves the cache alone.
		if !featureUnknown && entry.Features != featureBits {
			entry.Features = featureBits
			entryChanged = true
		}
		if entry.FeaturesUnknown != featureUnknown {
			entry.FeaturesUnknown = featureUnknown
			entryChanged = true
		}
		if d.SF != 0 && entry.SF != d.SF {
			entry.SF = d.SF
			entryChanged = true
		}
		if d.Auth != device.AuthNone && entry.Auth != d.Auth {
			entry.Auth = d.Auth
			entryChanged = true
		}
		if members := memberEntries(d.Members); len(members) > 0 && !membersEqual(entry.Members, members) {
			entry.Members = members
			entryChanged = true
		}
		if entry.LastSeen.IsZero() || now.Sub(entry.LastSeen) >= lastSeenWriteEvery {
			entry.LastSeen = now
			entryChanged = true
		}
		if entryChanged {
			e.reg.Put(entry)
			changed = true
		}
	}
	// A group remembered before LastSeen existed starts its grace now. A zero
	// timestamp means unknown, not gone; pruning on upgrade could delete a
	// pair that is merely powered off.
	for i := range e.reg.Entries {
		entry := &e.reg.Entries[i]
		if entry.IsGroup() && entry.LastSeen.IsZero() {
			entry.LastSeen = now
			changed = true
		}
	}
	// An active session keeps its row even when discovery has already dropped
	// it; the reconnect policy owns those receivers until the user disconnects.
	for _, key := range e.activeKeys() {
		live[key] = true
	}
	for _, key := range receivers.PruneStaleGroups(&e.reg, live, now, receivers.GroupGrace) {
		e.logf("pruned stale stereo group %s", key)
		changed = true
	}
	if changed {
		e.saveRegistry()
	}
}

// saveRegistry persists the in-memory registry. A failure keeps the
// in-memory copy working; the corrupt-file guard refuses to write over an
// unreadable file.
func (e *Engine) saveRegistry() {
	if err := e.recvStore.Save(e.reg); err != nil {
		e.logf("receivers save: %v", err)
	}
}

// memberEntries caches a stereo pair's speaker addresses and capability
// fields in the registry so the next launch can connect without discovery.
func memberEntries(members []device.Device) []receivers.Member {
	if len(members) == 0 {
		return nil
	}
	out := make([]receivers.Member, 0, len(members))
	for _, m := range members {
		featureBits, featureUnknown := m.Features.Persisted()
		out = append(out, receivers.Member{
			ID:              m.ID,
			Name:            m.Name,
			Host:            m.Host,
			Port:            m.Port,
			Model:           m.Model,
			OSVersion:       m.OSVersion,
			AirPlay2:        m.AirPlay2,
			Features:        featureBits,
			FeaturesUnknown: featureUnknown,
			SF:              m.SF,
			Auth:            m.Auth,
		})
	}
	return out
}

func membersEqual(a, b []receivers.Member) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// rebuildTargets re-projects the registry and discovery onto the row lists.
// Presentation rules live in receivers; the engine supplies inputs. The
// linked-member rows a combo replaces drop from the display lists while the
// full merge stays available to lookups by member key.
func (e *Engine) rebuildTargets() {
	targets := receivers.Merge(e.reg, e.discovered, func(k string) bool { return e.passwords[k] })
	active := make(map[string]bool)
	for _, key := range e.activeKeys() {
		active[key] = true
	}
	targets = receivers.Manageable(targets, active)
	targets = receivers.DropLinked(targets)
	e.devices = receivers.Visible(targets, active)
	e.receiversViews = make([]receivers.View, len(targets))
	for i, t := range targets {
		v := e.receiverView(t)
		if v.Stereo {
			v.Channel = receivers.ChannelStereo
		}
		e.receiversViews[i] = v
	}
	e.refreshDiscoverySeeds()
}

// receiverHosts is the seed list for discovery's directed pass: manual and
// last-known addresses, which survive an offline receiver and a restart.
func receiverHosts(reg receivers.Registry) []string {
	hosts := make([]string, 0, len(reg.Entries))
	for _, entry := range reg.Entries {
		if host, _ := entry.Endpoint(); strings.TrimSpace(host) != "" {
			hosts = append(hosts, host)
		}
		for _, m := range entry.Members {
			host := m.Host
			if m.PairedHost != "" && m.PairedPort != 0 {
				host = m.PairedHost
			}
			if host = strings.TrimSpace(host); host != "" {
				hosts = append(hosts, host)
			}
		}
	}
	return hosts
}

// refreshDiscoverySeeds hands discovery the addresses the registry remembers.
// rebuildTargets is the single projection point, so a manual add or a first
// sighting reaches the next directed pass.
func (e *Engine) refreshDiscoverySeeds() {
	if e.discovery == nil {
		return
	}
	e.discovery.SetSeedHosts(receiverHosts(e.reg))
}

func (e *Engine) find(id string) (device.Device, bool) {
	for _, d := range e.devices {
		if d.ID == id {
			return d, true
		}
	}
	return device.Device{}, false
}

func (e *Engine) target(id string) (receivers.Target, bool) {
	for _, t := range receivers.Merge(e.reg, e.discovered, func(k string) bool { return e.passwords[k] }) {
		if t.Entry.Key == id {
			return t, true
		}
	}
	return receivers.Target{}, false
}

func (e *Engine) hasPassword(key string) bool { return e.passwords[key] }

func (e *Engine) passwordFor(key string) string {
	if pw := e.creds.LoadPassword(key); pw != "" {
		return pw
	}
	return ""
}

// memberPassword resolves a stereo member's password: the member's own,
// else the group's, so one saved password can serve both members.
func (e *Engine) memberPassword(m device.Device) string {
	if pw := e.creds.LoadPassword(m.ID); pw != "" {
		return pw
	}
	if m.StereoID != "" {
		return e.creds.LoadPassword(device.StereoGroupID(m.StereoID))
	}
	return ""
}

// latencyFor is the presentation latency one receiver gets: the global base
// plus its DSP/DAC delay, clamped to the sender's window. This is the value
// the stream SETUP carries and the flyout shows.
func (e *Engine) latencyFor(key string) int {
	delay := 0
	if entry, ok := e.reg.Get(key); ok {
		delay = entry.DelayMS
	}
	return receivers.EffectiveLatencyMS(e.setsValue.LatencyMS, delay)
}

func (e *Engine) nameFor(id string) string {
	if d, ok := e.find(id); ok {
		return d.Name
	}
	return ""
}

func (e *Engine) startCapture() {
	e.restartCapture()
}

// restartCapture closes the old source and starts a new one on a goroutine so
// the event loop never blocks on WASAPI initialization or teardown. The old
// source stops off the loop too; until its pump exits, both sources feed the
// sink for a bounded moment, which the ring's overwrite-oldest policy and the
// start trim absorb. Its late callbacks carry a stale generation and are
// dropped.
func (e *Engine) restartCapture() {
	// A manual or settings-driven restart starts at once; a pending backoff
	// belongs to the failed open it replaced.
	e.captureRetryAt = time.Time{}
	e.captureRetryNum = 0
	if e.capture != nil {
		old := e.capture
		e.capture = nil
		go old.Stop()
	}
	e.captureRunning = false
	e.captureActiveApps = nil
	e.captureWaitingApp = false
	if e.captureStarting {
		e.captureQueued = true
		return
	}
	e.beginCaptureStart()
}

// scheduleCaptureRetry arms the next automatic open after a failed start. A
// source that opened retries itself; this covers the launch case where the
// first open finds no render endpoint, so the engine would otherwise stay
// silent until the user changes the capture selection or restarts the app.
// It shares the reconnect schedule and keeps retrying at the longest delay,
// because a dock or a default device can return at any time.
func (e *Engine) scheduleCaptureRetry() {
	if e.stopping {
		return
	}
	delay := reconnectBackoff[min(e.captureRetryNum, len(reconnectBackoff)-1)]
	e.captureRetryNum++
	e.captureRetryAt = e.now().Add(delay)
	e.logf("capture retry in %s (attempt %d)", delay, e.captureRetryNum)
}

func (e *Engine) retryCapture() {
	e.captureRetryAt = time.Time{}
	if e.stopping || e.capture != nil || e.captureRunning || e.captureStarting {
		return
	}
	e.logf("retrying capture")
	e.beginCaptureStart()
}

func (e *Engine) beginCaptureStart() {
	e.captureStarting = true
	e.captureGen++
	gen := e.captureGen
	if e.levelFeed != nil {
		e.levelFeed.Reset()
	}
	cfg := capture.Config{
		Ring:        e.silence,
		EndpointID:  e.captureEndpoint,
		ProcessID:   e.captureProcess,
		AppIDs:      append([]string(nil), e.captureApps...),
		MuteProcess: e.processMuteWanted(),
		OnFormat: func(rate int) {
			select {
			case e.captureInfo <- captureInfo{gen: gen, rate: rate}:
			default:
			}
		},
		OnError: func(err error) {
			select {
			case e.captureInfo <- captureInfo{gen: gen, err: err}:
			default:
			}
		},
		OnDevice: func(name string) {
			select {
			case e.captureInfo <- captureInfo{gen: gen, name: name}:
			default:
			}
		},
	}
	start := e.captureStartFn
	go func() {
		src, err := start(cfg)
		select {
		case e.captureResult <- captureStart{src: src, err: err}:
		case <-e.quit:
			if src != nil {
				src.Stop()
			}
		}
	}()
}

func (e *Engine) applyCaptureStart(res captureStart) {
	e.captureStarting = false
	if e.captureQueued {
		e.captureQueued = false
		if res.src != nil {
			res.src.Stop()
		}
		e.beginCaptureStart()
		return
	}
	if e.stopping {
		if res.src != nil {
			res.src.Stop()
		}
		return
	}
	if res.err != nil {
		e.captureRunning = false
		e.captureActiveApps = nil
		e.captureWaitingApp = errors.Is(res.err, capture.ErrNoAppProcess)
		e.logCaptureError(res.err)
		e.scheduleCaptureRetry()
		e.publish()
		return
	}
	e.capture = res.src
	e.captureRunning = true
	e.captureWaitingApp = false
	e.captureError = ""
	e.captureRetryAt = time.Time{}
	e.captureRetryNum = 0
	e.captureErrorLogged = ""
	e.captureErrorLoggedAt = time.Time{}
	stats := res.src.Stats()
	e.captureActiveApps = append([]string(nil), stats.Apps...)
	e.captureRate = stats.Rate
	e.captureOpenSeq = stats.OpenSeq
	if e.level != nil {
		e.level.SetRate(stats.Rate)
	}
	if stats.Device != "" {
		e.captureName = stats.Device
	}
	if e.tr != nil {
		e.tr.SetInputFormat(stats.Rate)
	}
	e.resetCaptureSink()
	if e.setsValue.MuteLocalOnConnect && e.tr != nil && e.tr.State() == sender.StateStreaming {
		// A tap opened or reopened during a session must take the mute the
		// session asked for, including a per-app tap's own mute.
		e.applyLocalMute()
	}
	e.logf("capture open: %s (%d Hz)", e.captureName, stats.Rate)
	e.publish()
}

func (e *Engine) applyCaptureInfo(info captureInfo) {
	if info.gen != e.captureGen {
		// A source that is being torn down reported late; the live source's
		// word is the only one that counts.
		return
	}
	if info.rate > 0 && info.rate != e.captureRate {
		e.captureRate = info.rate
		if e.level != nil {
			e.level.SetRate(info.rate)
		}
		if e.tr != nil {
			e.tr.SetInputFormat(info.rate)
		}
		e.resetCaptureSink()
		e.logf("capture rate changed to %d Hz", info.rate)
	}
	if info.name != "" {
		e.captureName = info.name
	}
	if info.err != nil {
		e.captureWaitingApp = errors.Is(info.err, capture.ErrNoAppProcess)
		e.logCaptureError(info.err)
	} else {
		e.captureWaitingApp = false
		e.captureError = ""
		e.captureErrorLogged = ""
		e.captureErrorLoggedAt = time.Time{}
	}
	if e.capture != nil {
		e.captureActiveApps = append([]string(nil), e.capture.Stats().Apps...)
	}
	e.publish()
}

// syncCaptureOpen applies a stream reopen that the lossy OnFormat and OnDevice
// callbacks may not have delivered. Every successful open is a generation
// boundary: a same-rate device switch would otherwise keep the old source's
// samples in the member rings, and a dropped rate notification would resample
// at the wrong input rate until the next open.
func (e *Engine) syncCaptureOpen() {
	if e.capture == nil {
		return
	}
	stats := e.capture.Stats()
	if stats.OpenSeq == 0 || stats.OpenSeq == e.captureOpenSeq {
		return
	}
	e.captureOpenSeq = stats.OpenSeq
	if stats.Rate > 0 {
		e.captureRate = stats.Rate
		if e.level != nil {
			e.level.SetRate(stats.Rate)
		}
		if e.tr != nil {
			e.tr.SetInputFormat(stats.Rate)
		}
	}
	if stats.Device != "" {
		e.captureName = stats.Device
	}
	e.resetCaptureSink()
	e.logf("capture reopened: %s (%d Hz)", e.captureName, stats.Rate)
	e.publish()
}

// logCaptureError records the current error and logs it on the first
// occurrence and then at most once a minute. A source that keeps retrying a
// missing device must not write a line per attempt.
func (e *Engine) logCaptureError(err error) {
	e.captureError = err.Error()
	now := e.now()
	if e.captureErrorLogged != e.captureError || now.Sub(e.captureErrorLoggedAt) >= time.Minute {
		e.logf("capture error: %v", err)
		e.captureErrorLogged = e.captureError
		e.captureErrorLoggedAt = now
	}
}

func (e *Engine) markSettingsDirty() {
	if e.settingsDirtyAt.IsZero() {
		e.settingsDirtyAt = e.now().Add(settingsSaveDebounce)
	}
}

func (e *Engine) saveSettingsNow() {
	e.settingsDirtyAt = time.Time{}
	if e.sets == nil {
		return
	}
	err := e.sets.Update(func(stored *settings.Settings) {
		settings.MergeEngine(stored, &e.setsValue)
	})
	if err == nil {
		e.settingsErrorLogged = ""
		return
	}
	// A full disk or a read-only config directory must not lose the change
	// silently. Keep the write pending so the next debounce retries, and log
	// the failure once instead of on every preference edit.
	if e.settingsErrorLogged != err.Error() {
		e.logf("settings save failed: %v", err)
		e.settingsErrorLogged = err.Error()
	}
	e.settingsDirtyAt = e.now().Add(settingsSaveDebounce)
}

func (e *Engine) buildSnapshot() Snapshot {
	state := sender.StateIdle
	if e.tr != nil {
		state = e.tr.State()
	}
	// Drops before a stream are expected: no sink drains the ring. Report
	// the accumulation since this session reached streaming; it survives a
	// capture sink reset.
	dropped := uint64(0)
	if e.captureSink != nil && state == sender.StateStreaming && e.dropSessionActive {
		e.foldCaptureDrops()
		dropped = e.sessionDropped
	}
	var lines []string
	if e.log != nil {
		lines = e.log.Lines()
	}
	ptpActive, ptpReady, ptpOffset := false, false, 0.0
	if e.tr != nil {
		ptpActive, ptpReady, ptpOffset = e.tr.PTP()
	}
	devs := make([]device.Device, len(e.devices))
	copy(devs, e.devices)
	views := make([]receivers.View, len(e.receiversViews))
	copy(views, e.receiversViews)
	now := e.now()
	diagnostics := e.buildDiagnostics(now, dropped)
	return Snapshot{
		Devices:             devs,
		Receivers:           views,
		SelectedID:          e.selectedID,
		State:               state,
		ConnectedName:       e.connectedName,
		ActiveTargets:       e.activeKeys(),
		Targets:             e.targetControls(),
		Notice:              e.notice,
		Error:               e.lastError,
		ErrorKey:            e.lastErrorKey,
		ErrorArgs:           e.lastErrorArgs,
		CredentialReset:     e.credentialReset,
		TakenOver:           e.life.takenOver,
		PinRequired:         e.pinRequired,
		PinDevice:           e.pinDevice,
		PasswordRequired:    e.passwordRequired,
		PasswordDevice:      e.passwordDevice,
		PasswordDeviceID:    e.passwordDeviceID,
		PasswordTargetID:    e.passwordTargetID,
		Name:                e.setsValue.Name,
		Reconnect:           e.setsValue.Reconnect,
		Reconnecting:        e.life.reconnectWanted && !e.life.reconnectAt.IsZero(),
		AudioIdle:           e.idle != idleActive,
		Standby:             e.idle == idleDark,
		StandbyEnabled:      e.setsValue.Standby.Enabled,
		StandbyMinutes:      settings.NormalizeStandbyMinutes(e.setsValue.Standby.IdleMinutes),
		MuteLocal:           e.mute.state,
		MuteLocalError:      e.mute.err,
		LatencyMS:           e.setsValue.LatencyMS,
		ReceiverLatencyMS:   e.receiverLatency,
		CaptureProcessPID:   e.captureProcess,
		CaptureProcessName:  e.captureName,
		CaptureAppIDs:       append([]string(nil), e.captureApps...),
		CaptureAppNames:     append([]string(nil), e.captureAppNames...),
		CaptureActiveAppIDs: append([]string(nil), e.captureActiveApps...),
		CaptureWaitingApp:   e.captureWaitingApp,
		LampOff:             e.setsValue.Lamp.Off,
		LampCover:           e.setsValue.Lamp.Cover,
		LampPreset:          lamp.Normalize(e.setsValue.Lamp.Preset),
		TrayAnimation:       e.setsValue.TrayAnimation,
		CaptureEndpoint:     e.captureEndpoint,
		CaptureName:         e.captureName,
		CaptureRate:         e.captureRate,
		CaptureRunning:      e.captureRunning,
		CaptureError:        e.captureError,
		DroppedSamples:      dropped,
		Diagnostics:         diagnostics,
		StrictAuth:          e.setsValue.StrictReceiverAuth,
		PTPActive:           ptpActive,
		PTPReady:            ptpReady,
		PTPOffsetMS:         ptpOffset,
		LogLines:            lines,
	}
}

func (e *Engine) publish() {
	// publish is the single projection point: every mutation lands through
	// it, so the row lists always reflect the registry, discovery, and the
	// active key together.
	e.rebuildTargets()
	snap := e.buildSnapshot()
	e.mu.Lock()
	e.snap = snap
	e.mu.Unlock()
	if e.onChange != nil {
		e.onChange(snap)
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
