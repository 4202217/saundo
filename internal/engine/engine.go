// Package engine is the UI-agnostic session controller. It owns discovery,
// capture, the audio ring, and the sender session, and funnels every input
// through one event-loop goroutine.
package engine

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/ptp"
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
	Devices          []device.Device
	Receivers        []receivers.View
	SelectedID       string
	State            sender.State
	ConnectedName    string
	Error            string
	PinRequired      bool
	PinDevice        string
	PasswordRequired bool
	PasswordDevice   string
	PasswordDeviceID string
	// PasswordTargetID is the target the UI reconnects after the password is
	// saved. It differs from PasswordDeviceID for a stereo member, whose
	// password is stored under the member key but whose pair is the target.
	PasswordTargetID string
	Volume           float64
	Name             string
	Reconnect        bool
	Reconnecting     bool
	// AudioIdle is true while the engine has suspended RTP after sustained
	// digital silence. The session stays connected.
	AudioIdle      bool
	MuteLocal      MuteLocalState
	MuteLocalError string
	LampOff        bool
	LampCover      bool
	LampPreset     string
	TrayAnimation  bool
	FlyoutPopover  bool
	LatencyMS      int
	// ReceiverLatencyMS is the receiver's own arrival-to-render latency from
	// its stream SETUP response. 0 means the receiver did not report one.
	ReceiverLatencyMS  int
	CaptureEndpoint    string
	CaptureName        string
	CaptureRate        int
	CaptureRunning     bool
	CaptureError       string
	CaptureProcessPID  uint32
	CaptureProcessName string
	DroppedSamples     uint64
	StrictAuth         bool
	PTPActive          bool
	PTPReady           bool
	PTPOffsetMS        float64
	LogLines           []string
}

type commandKind uint8

const (
	cmdConnect commandKind = iota
	cmdDisconnect
	cmdVolume
	cmdPin
	cmdDismissPassword
	cmdCaptureEndpoint
	cmdRefresh
	cmdStrictAuth
	cmdName
	cmdReconnect
	cmdLamp
	cmdTrayAnimation
	cmdFlyoutPopover
	cmdMuteLocal
	cmdNowPlaying
	cmdLatency
	cmdLatencySettings
	cmdCaptureProcess
	cmdPassword
	cmdQuit
	cmdAddReceiver
	cmdUpdateReceiver
	cmdRemoveReceiver
)

type command struct {
	kind      commandKind
	deviceID  string
	value     float64
	text      string
	name      string
	album     string
	cover     []byte
	coverMIME string
	reconnect bool
	show      bool
	lampCover bool
	spec      receivers.Spec
	patch     receivers.Patch
}

type captureInfo struct {
	gen  int
	rate int
	name string
	err  error
}

// captureSource is the part of a running capture the engine touches.
type captureSource interface {
	Stop()
	Stats() capture.Stats
}

type captureStart struct {
	src captureSource
	err error
}

// transport is one live sender session. The production implementation wraps
// sender.Sender with its net host and PTP clock; tests replace it.
type transport interface {
	Events() <-chan raopio.Event
	// Dead closes when the transport is beyond saving (a stereo member
	// died, a launch failed) and closeSession must run. Nil for transports
	// that never signal this.
	Dead() <-chan struct{}
	Start(host string, port uint16, name string)
	Stop()
	Active() bool
	State() sender.State
	HandleEvent(ev raopio.Event)
	Tick()
	NextDeadline() time.Time
	// SetPaused suspends or resumes the audio pacer. It reports whether the
	// transport applied the request.
	SetPaused(paused bool) bool
	SetIdentity(id sender.Identity)
	SetAuth(auth device.Auth, airplay2 bool, deviceID, credsJSON, password string)
	SetMemberAuth(m device.Device, credsJSON, password string)
	Rings() []*ring.Buffer
	SetPTPClock(clock sender.PTPClock)
	AttachRing(rb *ring.Buffer)
	SetInputFormat(rate int)
	SetLatency(ms int)
	SetVolume(pct float64)
	SetStrictReceiverAuth(strict bool)
	SubmitPin(code string)
	WaitingForPin() bool
	InitialVolumePercent() (float64, bool)
	SetNowPlaying(title, artist, album string, cover []byte, coverMIME string)
	SetLampArtwork(data []byte, mime string, on bool)
	SetPlaybackState(playing bool)
	PTP() (active, ready bool, offsetMS float64)
}

// session is the production transport.
type session struct {
	host   *raopio.Host
	sender *sender.Sender
	ptp    *ptp.Follower
}

func (s *session) Events() <-chan raopio.Event { return s.host.Events() }
func (s *session) Dead() <-chan struct{}       { return nil }
func (s *session) Start(host string, port uint16, name string) {
	s.sender.Start(host, port, name)
}
func (s *session) Stop()                          { s.sender.Stop() }
func (s *session) Active() bool                   { return s.sender.Active() }
func (s *session) State() sender.State            { return s.sender.State() }
func (s *session) HandleEvent(ev raopio.Event)    { s.sender.HandleEvent(ev) }
func (s *session) Tick()                          { s.sender.Tick() }
func (s *session) NextDeadline() time.Time        { return s.sender.NextDeadline() }
func (s *session) SetPaused(paused bool) bool     { return s.sender.SetPaused(paused) }
func (s *session) SetIdentity(id sender.Identity) { s.sender.SetIdentity(id) }
func (s *session) SetAuth(a device.Auth, ap2 bool, id, creds, pw string) {
	s.sender.SetAuth(a, ap2, id, creds, pw)
}
func (s *session) SetMemberAuth(m device.Device, credsJSON, password string) {
	s.sender.SetAuth(m.Auth, m.Auth.IsHAP(), m.ID, credsJSON, password)
}
func (s *session) Rings() []*ring.Buffer         { return nil }
func (s *session) SetPTPClock(c sender.PTPClock) { s.sender.SetPTPClock(c) }
func (s *session) AttachRing(rb *ring.Buffer)    { s.sender.AttachRing(rb) }
func (s *session) SetInputFormat(rate int)       { s.sender.SetInputFormat(rate) }
func (s *session) SetLatency(ms int)             { s.sender.SetLatency(ms) }
func (s *session) SetVolume(pct float64)         { s.sender.SetVolume(pct) }
func (s *session) SetStrictReceiverAuth(v bool)  { s.sender.SetStrictReceiverAuth(v) }
func (s *session) SubmitPin(code string)         { s.sender.SubmitPin(code) }
func (s *session) WaitingForPin() bool           { return s.sender.WaitingForPin() }
func (s *session) InitialVolumePercent() (float64, bool) {
	return s.sender.InitialVolumePercent()
}
func (s *session) SetNowPlaying(title, artist, album string, cover []byte, coverMIME string) {
	s.sender.SetNowPlaying(title, artist, album, cover, coverMIME)
}
func (s *session) SetLampArtwork(data []byte, mime string, on bool) {
	s.sender.SetLampArtwork(data, mime, on)
}
func (s *session) SetPlaybackState(playing bool) { s.sender.SetPlaybackState(playing) }
func (s *session) PTP() (active, ready bool, offsetMS float64) {
	if s.ptp == nil {
		return false, false, 0
	}
	st := s.ptp.Stats()
	return true, st.Ready, st.OffsetNS / 1e6
}

// ptpFeatureBit is the receiver feature that advertises PTP support.
const ptpFeatureBit = 1 << 41

// usesPTP reports whether the receiver needs the sender as a PTP peer. HomePod
// software 27 and macOS AirPlay receivers advertise the PTP bit and silently
// ignore an NTP-timed stream; both use the same follower.
func usesPTP(d device.Device) bool {
	return d.Auth.IsHAP() && d.Features&ptpFeatureBit != 0 &&
		(strings.HasPrefix(d.Model, "AudioAccessory") || strings.HasPrefix(d.Model, "Mac"))
}

// maxReconnectAttempts bounds automatic retries before giving up.
const maxReconnectAttempts = 5

// settingsSaveDebounce coalesces settings writes on the run loop: a volume
// drag or a burst of toggles costs one disk write, not one per change.
const settingsSaveDebounce = 500 * time.Millisecond

// audioPauseAfter is how long the capture stream must be digitally silent
// before the engine suspends the audio pacer. Two seconds keeps a gap between
// tracks streaming while stopping a real pause quickly.
const (
	audioPauseAfter = 2 * time.Second
	audioPausePoll  = 25 * time.Millisecond
)

// lastSeenWriteEvery bounds how often a discovery sighting rewrites
// receivers.json. The stale-group grace is a day, so an hour of resolution is
// plenty and avoids a file write on every three-second discovery cycle.
const lastSeenWriteEvery = time.Hour

// reconnectBackoff is the delay before the nth retry.
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
	silence       *silenceMeter
	memberRings   []*ring.Buffer
	discovery     *discovery.Service
	tr            transport
	stopSession   func()
	capture       captureSource
	captureInfo   chan captureInfo
	captureResult chan captureStart
	mute          localMute

	commands       chan command
	quit           chan struct{}
	done           chan struct{}
	startOnce      sync.Once
	stopOnce       sync.Once
	started        atomic.Bool
	stopping       bool
	now            func() time.Time
	newSession     func(d device.Device) (transport, func(), error)
	captureStartFn func(capture.Config) (captureSource, error)

	// The endpoint mute is injected so tests never touch the machine's audio.
	setEndpointMuted func(string, bool) error
	defaultEndpoint  func() (capture.Endpoint, error)
	endpoints        func() ([]capture.Endpoint, error)

	mu   sync.Mutex
	snap Snapshot

	devices          []device.Device
	selectedID       string
	connectedName    string
	lastError        string
	pinRequired      bool
	pinDevice        string
	passwordRequired bool
	passwordDevice   string
	passwordDeviceID string
	passwordTargetID string
	volume           float64
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
	setsValue         settings.Settings
	captureEndpoint   string
	captureName       string
	captureProcess    uint32
	captureRate       int
	captureRunning    bool
	captureError      string
	captureStarting   bool
	captureQueued     bool
	captureGen        int
	userDisconnect    bool
	reconnectWanted   bool
	reconnectAt       time.Time
	reconnectNum      int
	lastMissingLogAt  time.Time
	settingsDirtyAt   time.Time
	audioPaused       bool
	pauseUnsupported  bool
	autoTried         bool

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
		sets:            sets,
		creds:           credsStore,
		recvStore:       recvStore,
		log:             log,
		onChange:        onChange,
		ring:            ring.New(48000 * 2 * 4),
		captureInfo:     make(chan captureInfo, 8),
		captureResult:   make(chan captureStart, 4),
		commands:        make(chan command, 32),
		quit:            make(chan struct{}),
		done:            make(chan struct{}),
		setsValue:       cfg,
		volume:          cfg.Volume,
		selectedID:      cfg.LastDeviceID,
		captureEndpoint: cfg.CaptureEndpointID,
		captureName:     cfg.CaptureEndpointName,
		captureProcess:  cfg.CaptureProcessPID,
		now:             time.Now,
		stopping:        false,
		passwords:       map[string]bool{},
		lampJPEG:        lamp.JPEG,
	}
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
	e.newSession = e.openSession
	e.captureStartFn = startCaptureDefault
	e.captureSink = ring.NewFanout(e.ring)
	e.level = analyze.New(0)
	e.silence = newSilenceMeter(e.captureSink, e.level)
	e.setEndpointMuted = capture.SetEndpointMuted
	e.defaultEndpoint = capture.DefaultEndpoint
	e.endpoints = capture.Endpoints
	e.mute.state = e.idleMuteState()
	e.recoverMuteGuard()
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

// openSession builds the production transport for one receiver.
func (e *Engine) openSession(d device.Device) (transport, func(), error) {
	e.receiverLatency = 0
	if d.Host == "" || d.Port == 0 {
		return nil, nil, fmt.Errorf("receiver %q has no address", d.Name)
	}
	if d.IsStereoGroup() {
		return e.openGroupSession(d)
	}
	usePTP := usesPTP(d)
	host := raopio.NewHost()
	var clock *ptp.Follower
	if usePTP {
		c, err := ptp.New(d.Host)
		if err != nil {
			host.Close()
			return nil, nil, fmt.Errorf("PTP: %w", err)
		}
		c.Start()
		clock = c
	}
	s := sender.New(host, sender.Events{
		Launched:               e.onLaunched,
		Closed:                 e.onClosed,
		PinRequired:            e.onPinRequired,
		CredentialsObtained:    e.onCredentials,
		StateChanged:           e.onStateChanged,
		VolumeChanged:          e.onReceiverVolume,
		ReceiverLatencyChanged: e.onReceiverLatency,
	}, e.senderLog)
	s.SetStrictReceiverAuth(e.setsValue.StrictReceiverAuth)
	s.AttachRing(e.ring)
	// One gate for the session: it owns the single start trim and the start
	// epoch. A lone receiver keeps the newest capture cushion and starts as
	// soon as its stream is accepted.
	s.SetStartGate(sender.NewStartGate(sender.StartGateConfig{
		Members: 1,
		Cushion: sender.DefaultStartCushion,
		Trim:    e.ring,
	}))
	if clock != nil {
		s.SetPTPClock(clock)
	}
	sess := &session{host: host, sender: s, ptp: clock}
	return sess, func() {
		s.Stop()
		if clock != nil {
			clock.Stop()
		}
		host.Close()
	}, nil
}

// Start launches discovery, capture, and the event loop.
func (e *Engine) Start() {
	e.startOnce.Do(func() {
		e.stopping = false
		e.discovery = discovery.New(3*time.Second, 1500*time.Millisecond)
		e.discovery.SetSeedHosts(receiverHosts(e.reg))
		e.discovery.Start()
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

// Connect starts a session with the device ID.
func (e *Engine) Connect(deviceID string) {
	e.send(command{kind: cmdConnect, deviceID: deviceID})
}

// Disconnect stops the current session.
func (e *Engine) Disconnect() { e.send(command{kind: cmdDisconnect}) }

// SetVolume sets receiver volume in percent.
func (e *Engine) SetVolume(pct float64) {
	e.send(command{kind: cmdVolume, value: pct})
}

// SubmitPIN supplies the receiver's on-screen code.
func (e *Engine) SubmitPIN(code string) {
	e.send(command{kind: cmdPin, text: code})
}

// DismissPasswordPrompt closes the first-connect password prompt without
// changing the session.
func (e *Engine) DismissPasswordPrompt() {
	e.send(command{kind: cmdDismissPassword})
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

// SetCaptureProcess selects application capture; pid 0 returns to endpoint
// capture.
func (e *Engine) SetCaptureProcess(pid uint32, name string) {
	e.send(command{kind: cmdCaptureProcess, value: float64(pid), name: name})
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

// SetFlyoutPopover toggles the popover-style flyout preference (macOS).
func (e *Engine) SetFlyoutPopover(on bool) {
	v := 0.0
	if on {
		v = 1
	}
	e.send(command{kind: cmdFlyoutPopover, value: v})
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
			// The transport signaled it is beyond saving. closeSession does
			// the teardown here, on the engine goroutine, outside
			// HandleEvent — the sender's single-goroutine contract holds and
			// the stop cannot re-enter a sender callback.
			e.closeSession()
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
		}
	}
}

// nextWake is the earliest timer: a sender deadline or a reconnect retry.
func (e *Engine) nextWake() time.Time {
	wake := time.Time{}
	if e.tr != nil {
		if dl := e.tr.NextDeadline(); !dl.IsZero() {
			wake = dl
		}
	}
	// While the audio is suspended the pacer is disarmed, so poll for the
	// first audible capture block instead of waiting for the sender's timers.
	if e.audioPaused && e.tr != nil && e.tr.State() == sender.StateStreaming {
		poll := e.now().Add(audioPausePoll)
		if wake.IsZero() || poll.Before(wake) {
			wake = poll
		}
	}
	if e.reconnectWanted && !e.reconnectAt.IsZero() && (wake.IsZero() || e.reconnectAt.Before(wake)) {
		wake = e.reconnectAt
	}
	if !e.settingsDirtyAt.IsZero() && (wake.IsZero() || e.settingsDirtyAt.Before(wake)) {
		wake = e.settingsDirtyAt
	}
	return wake
}

func (e *Engine) onWake() {
	now := e.now()
	if e.reconnectWanted && !e.reconnectAt.IsZero() && !now.Before(e.reconnectAt) {
		e.attemptReconnect()
		return
	}
	if !e.settingsDirtyAt.IsZero() && !now.Before(e.settingsDirtyAt) {
		e.saveSettingsNow()
	}
	e.updateAudioPause(now)
	if e.tr != nil {
		e.tr.Tick()
	}
}

// updateAudioPause suspends the audio pacer after a sustained digital silence
// and resumes it on the first audible block. The sender keeps its sync,
// feedback, volume, and event timers running while the stream is suspended,
// so the receiver's timeline and session stay alive without any RTP.
func (e *Engine) updateAudioPause(now time.Time) {
	if e.tr == nil || e.tr.State() != sender.StateStreaming || e.pauseUnsupported {
		return
	}
	loud := now.Sub(e.silence.LastLoud()) < audioPauseAfter
	wanted := !loud
	if wanted == e.audioPaused {
		return
	}
	if !e.tr.SetPaused(wanted) {
		// A legacy AirPlay 1 session keeps its continuous silence stream.
		e.pauseUnsupported = true
		return
	}
	e.audioPaused = wanted
	e.silence.SetPaused(wanted)
	if e.audioPaused {
		e.logf("audio idle for %s, stream suspended", audioPauseAfter)
	} else {
		e.logf("audio resumed")
	}
	e.publish()
}

func (e *Engine) shutdown() {
	e.stopping = true
	e.closeSession()
	e.saveSettingsNow()
	if e.capture != nil {
		e.capture.Stop()
		e.capture = nil
	}
	if e.discovery != nil {
		e.discovery.Stop()
	}
	if e.log != nil {
		e.log.Close()
	}
}

func (e *Engine) closeSession() {
	e.restoreLocalMute()
	e.detachMemberRings()
	e.audioPaused = false
	e.pauseUnsupported = false
	if e.silence != nil {
		e.silence.SetPaused(false)
	}
	if e.stopSession != nil {
		stop := e.stopSession
		e.tr = nil
		e.stopSession = nil
		stop()
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
	for _, r := range rings {
		r.Reset()
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
	e.ring.Reset()
	e.captureSink.Add(e.ring)
}

func (e *Engine) handleCommand(cmd command) {
	switch cmd.kind {
	case cmdConnect:
		e.autoTried = true
		if t, ok := e.target(cmd.deviceID); ok {
			e.userDisconnect = false
			e.connect(t.Device)
		} else {
			e.lastError = "未找到设备，请刷新后重试"
			e.publish()
		}
	case cmdDisconnect:
		e.autoTried = true
		e.userDisconnect = true
		// A cancel or a disconnect ends the attempt, so its failure is stale.
		// Clear it before reconnectPendingOff publishes the idle snapshot.
		e.lastError = ""
		e.reconnectWanted = false
		e.reconnectPendingOff()
		if e.tr != nil && e.tr.Active() {
			e.tr.Stop()
		}
		e.closeSession()
		e.connectedName = ""
		e.pinRequired = false
		e.clearPasswordPrompt()
		e.publish()
	case cmdVolume:
		e.volume = clamp(cmd.value, 0, 100)
		e.setsValue.Volume = e.volume
		if e.selectedID != "" {
			if e.setsValue.VolumeByDevice == nil {
				e.setsValue.VolumeByDevice = make(map[string]float64)
			}
			e.setsValue.VolumeByDevice[e.selectedID] = e.volume
		}
		e.markSettingsDirty()
		if e.tr != nil {
			e.tr.SetVolume(e.volume)
		}
		e.publish()
	case cmdPin:
		if e.tr != nil {
			e.tr.SubmitPin(cmd.text)
		}
		e.pinRequired = false
		e.publish()
	case cmdDismissPassword:
		e.clearPasswordPrompt()
		e.publish()
	case cmdCaptureEndpoint:
		e.captureEndpoint = cmd.deviceID
		e.captureName = cmd.name
		e.captureProcess = 0
		e.setsValue.CaptureEndpointID = cmd.deviceID
		e.setsValue.CaptureEndpointName = cmd.name
		e.setsValue.CaptureProcessPID = 0
		e.setsValue.CaptureProcessName = ""
		e.markSettingsDirty()
		if e.setsValue.MuteLocalOnConnect || e.mute.state == MuteLocalActive {
			e.refreshLocalMute()
		}
		e.restartCapture()
		e.publish()
	case cmdCaptureProcess:
		e.captureProcess = uint32(cmd.value)
		e.captureName = cmd.name
		e.setsValue.CaptureProcessPID = e.captureProcess
		e.setsValue.CaptureProcessName = cmd.name
		e.markSettingsDirty()
		if e.setsValue.MuteLocalOnConnect || e.mute.state == MuteLocalActive {
			e.refreshLocalMute()
		}
		e.restartCapture()
		e.publish()
	case cmdLatency:
		e.setsValue.LatencyMS = int(cmd.value)
		if e.tr != nil {
			// The active receiver keeps its own DSP/DAC delay: the base is
			// only half of the value the SETUP carries.
			e.tr.SetLatency(e.latencyFor(e.selectedID))
		}
		e.markSettingsDirty()
		if cmd.reconnect && e.tr != nil && e.tr.Active() {
			// A PIN wait is pairing progress, not a live stream: restarting
			// over it would discard the code the user is typing.
			if e.tr.WaitingForPin() {
				e.logf("latency apply: %s is waiting for a PIN; session left alone", e.selectedID)
			} else if d, ok := e.find(e.selectedID); !ok {
				e.lastError = "延迟已保存，但未找到设备，会话未重启"
				e.logf("latency apply: device %s not found; session left running", e.selectedID)
			} else if !d.Online {
				e.lastError = "延迟已保存，但设备当前不在线，会话未重启"
				e.logf("latency apply: %s is offline; session left running", e.selectedID)
			} else {
				e.userDisconnect = false
				e.connect(d)
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
			e.lastError = "无法保存接收器密码：" + err.Error()
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
		e.prepareLampImage()
		e.applyLamp()
		e.markSettingsDirty()
		e.publish()
	case cmdTrayAnimation:
		e.setsValue.TrayAnimation = cmd.value != 0
		e.markSettingsDirty()
		e.publish()
	case cmdFlyoutPopover:
		e.setsValue.FlyoutPopover = cmd.value != 0
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
	}
}

// connect starts a session with d. It keeps reconnectPending: only a
// successful launch or a user disconnect clears it.
func (e *Engine) connect(d device.Device) {
	// A new connect supersedes the previous session's PIN dialog; a pairing
	// that needs a code raises PinRequired again on its own.
	e.pinRequired = false
	e.pinDevice = ""
	// A new attempt starts clean. The previous receiver's failure must not
	// keep the flyout in its error phase while this attempt runs.
	e.lastError = ""
	if d.IsStereoGroup() && (!d.Online || len(d.Members) != 2) {
		e.lastError = "立体声组需要两台成员都在线"
		e.publish()
		return
	}
	if d.Auth == device.AuthLegacyPin {
		e.lastError = d.Name + " 使用 Apple FairPlay 音频，当前应用不支持。"
		e.logf("FairPlay receiver %s is not supported", d.Name)
		e.publish()
		return
	}
	password := ""
	if !d.IsStereoGroup() && e.hasPassword(d.ID) {
		password = e.passwordFor(d.ID)
	}
	if !d.IsStereoGroup() && needsReceiverPassword(d.Auth) {
		if !e.hasPassword(d.ID) || password == "" {
			// No stored password, or one the DPAPI file could not decrypt:
			// prompt instead of attempting the permissive path.
			if e.hasPassword(d.ID) {
				e.logf("stored receiver password for %s is unreadable; prompting again", d.ID)
			}
			e.passwordRequired = true
			e.passwordDevice = d.Name
			e.passwordDeviceID = d.ID
			e.passwordTargetID = d.ID
			e.lastError = ""
			e.logf("AirPlay password required for %s", d.Name)
			e.publish()
			return
		}
	}
	if d.IsStereoGroup() {
		// A member that needs a receiver password and has none would burn
		// the reconnect budget on 401 digest challenges; prompt for it the
		// same way a single receiver would.
		for _, m := range d.Members {
			// memberPassword falls back to the pair's own key, so a password
			// stored once for both members already satisfies this member.
			if !needsReceiverPassword(m.Auth) || e.memberPassword(m) != "" {
				continue
			}
			e.passwordRequired = true
			e.passwordDevice = m.Name
			e.passwordDeviceID = m.ID
			e.passwordTargetID = d.ID
			e.lastError = ""
			e.logf("AirPlay password required for %s (stereo member)", m.Name)
			e.publish()
			return
		}
	}
	e.clearPasswordPrompt()
	e.closeSession()
	e.lampSkipLoggedFor = ""
	e.prepareLampImage()
	// The device being connected becomes the selection, so snapshots,
	// per-device volume, and reconnect all follow the live session.
	e.selectedID = d.ID
	tr, stop, err := e.newSession(d)
	if err != nil {
		e.lastError = "无法建立会话：" + err.Error()
		e.logf("session open failed: %v", err)
		e.publish()
		return
	}
	e.tr = tr
	e.stopSession = stop
	// AirPlay 2 realtime runs over HAP pairing or an open _airplay receiver
	// with no pairing. A _raop-only receiver keeps the plaintext AP1 path.
	airplay2 := d.Auth.IsHAP()
	tr.SetIdentity(sender.Identity{Name: e.setsValue.Name, DeviceID: e.setsValue.DeviceID, Model: "iPhone14,3"})
	if d.IsStereoGroup() {
		for _, m := range d.Members {
			tr.SetMemberAuth(m, e.creds.Load(m.ID), e.memberPassword(m))
		}
	} else {
		tr.SetAuth(d.Auth, airplay2, d.ID, e.creds.Load(d.ID), password)
	}
	if rings := tr.Rings(); len(rings) > 0 {
		e.attachMemberRings(rings)
	}
	tr.SetLatency(e.latencyFor(d.ID))
	tr.SetStrictReceiverAuth(e.setsValue.StrictReceiverAuth)
	if e.captureRate > 0 {
		tr.SetInputFormat(e.captureRate)
	}
	if d.IsStereoGroup() {
		e.logf("stereo group %q: streaming to %d members", d.Name, len(d.Members))
	} else if d.GroupName != "" {
		e.logf("note: %s belongs to AirPlay group %q; audio plays on this device only", d.Name, d.GroupName)
	}
	e.logf("connecting to %s (%s) auth=%s ap2=%v", d.Name, d.Host, d.Auth, airplay2)
	tr.Start(d.Host, d.Port, d.Name)
	if v, ok := e.setsValue.VolumeByDevice[d.ID]; ok {
		tr.SetVolume(v)
	}
	e.publish()
}

// planReconnect schedules the next retry with the backoff schedule. After
// maxReconnectAttempts retries it gives up and surfaces an error.
func (e *Engine) planReconnect() {
	if e.reconnectNum >= maxReconnectAttempts {
		e.reconnectWanted = false
		e.lastError = fmt.Sprintf("自动重连 %d 次后停止", e.reconnectNum)
		e.logf("giving up after %d reconnect attempts", e.reconnectNum)
		e.publish()
		return
	}
	delay := reconnectBackoff[min(e.reconnectNum, len(reconnectBackoff)-1)]
	e.reconnectNum++
	e.reconnectWanted = true
	e.reconnectAt = e.now().Add(delay)
	e.logf("reconnect scheduled in %s (attempt %d)", delay, e.reconnectNum)
	e.publish()
}

func (e *Engine) reconnectPendingOff() {
	e.reconnectWanted = false
	e.reconnectAt = time.Time{}
	e.reconnectNum = 0
	e.publish()
}

// attemptReconnect retries the selected device; if it is gone, it waits for
// discovery to bring it back. It consumes the pending retry time. A retry
// that comes due while a live session exists is stale — the user connected
// again after it was scheduled — so it is pushed out instead of tearing down
// that session's handshake or PIN wait.
func (e *Engine) attemptReconnect() {
	if e.tr != nil && e.tr.Active() {
		e.reconnectAt = e.now().Add(5 * time.Second)
		return
	}
	e.reconnectAt = time.Time{}
	d, ok := e.find(e.selectedID)
	if !ok {
		// The wait is indefinite by design; the log line is not. Throttle it
		// so a device that never returns logs once a minute, not every 5 s.
		e.reconnectAt = e.now().Add(5 * time.Second)
		if e.lastMissingLogAt.IsZero() || e.now().Sub(e.lastMissingLogAt) >= time.Minute {
			e.lastMissingLogAt = e.now()
			e.logf("reconnect: device %s not discovered yet", e.selectedID)
		}
		return
	}
	e.lastMissingLogAt = time.Time{}
	e.connect(d)
}

func (e *Engine) reconnectShouldRetry() bool {
	return !e.userDisconnect && !e.stopping && e.setsValue.Reconnect && e.selectedID != ""
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
// slot to the media cover; a template owns it. A media pause keeps the
// current light. A receiver that cannot show top-display artwork is skipped
// with one diagnostic line per session.
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
	if e.setsValue.Lamp.Cover {
		e.tr.SetLampArtwork(nil, "", false)
		e.logf("ambient light: cover on %s", e.connectedName)
		e.tr.SetPlaybackState(true)
		return
	}
	preset := lamp.Normalize(e.setsValue.Lamp.Preset)
	if e.lampImagePreset != preset || len(e.lampImage) == 0 {
		return
	}
	e.logf("ambient light: template %s (%d bytes) on %s", e.lampImagePreset, len(e.lampImage), e.connectedName)
	e.tr.SetLampArtwork(e.lampImage, "image/jpeg", true)
	e.tr.SetPlaybackState(true)
}

// lampTargetCapable fails closed: a missing target, an unknown model, or an
// unknown software version is not capable. A stereo pair is capable when any
// member is; the group transport filters the rest per member.
func (e *Engine) lampTargetCapable() bool {
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

func (e *Engine) onLaunched(ok bool, err string) {
	if ok {
		e.connectedName = e.nameFor(e.selectedID)
		if e.setsValue.LastDeviceID != e.selectedID {
			e.setsValue.LastDeviceID = e.selectedID
			e.markSettingsDirty()
		}
		e.lastError = ""
		e.reconnectWanted = true
		e.reconnectNum = 0
		e.reconnectAt = time.Time{}
		if _, remembered := e.setsValue.VolumeByDevice[e.selectedID]; !remembered && e.tr != nil {
			// Adopt the receiver's own volume the first time so connecting
			// never jumps the device to the sender's default.
			if p, ok := e.tr.InitialVolumePercent(); ok {
				e.volume = clamp(p, 0, 100)
				e.setsValue.Volume = e.volume
				e.markSettingsDirty()
			}
		}
		e.applyLocalMute()
		e.logf("streaming to %s", e.connectedName)
		e.applyLamp()
		if !e.dropSessionActive {
			e.beginDropSession()
		}
	} else {
		e.restoreLocalMute()
		e.connectedName = ""
		e.receiverLatency = 0
		e.endDropSession()
		e.lastError = err
		e.logf("session failed: %s", err)
		if e.reconnectShouldRetry() && e.reconnectAt.IsZero() {
			e.planReconnect()
		}
	}
	e.publish()
}

func (e *Engine) onClosed() {
	e.connectedName = ""
	e.receiverLatency = 0
	e.endDropSession()
	e.pinRequired = false
	e.clearPasswordPrompt()
	if e.reconnectShouldRetry() && e.reconnectAt.IsZero() {
		e.planReconnect()
	}
	e.publish()
}

// clearPasswordPrompt drops the first-connect password prompt.
func (e *Engine) clearPasswordPrompt() {
	e.passwordRequired = false
	e.passwordDevice = ""
	e.passwordDeviceID = ""
	e.passwordTargetID = ""
}

// needsReceiverPassword reports whether the auth path needs the receiver's
// AirPlay password. HAP PIN pairing asks for a code instead.
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
	}
	e.publish()
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
	if e.captureSink != nil {
		e.lastRawDropped = e.captureSink.Dropped()
	}
	e.dropSessionActive = true
}

func (e *Engine) endDropSession() {
	e.dropSessionActive = false
	e.sessionDropped = 0
	e.lastRawDropped = 0
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
}

// onReceiverVolume adopts the receiver's own volume when its hardware knob
// moves. It mirrors the value into the per-device memory and the UI.
func (e *Engine) onReceiverVolume(pct float64) {
	e.volume = clamp(pct, 0, 100)
	e.setsValue.Volume = e.volume
	if e.selectedID != "" {
		if e.setsValue.VolumeByDevice == nil {
			e.setsValue.VolumeByDevice = make(map[string]float64)
		}
		e.setsValue.VolumeByDevice[e.selectedID] = e.volume
	}
	e.markSettingsDirty()
	e.publish()
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

func (e *Engine) senderLog(level sender.Level, format string, args ...any) {
	prefix := "info"
	if level == sender.LevelWarn {
		prefix = "warn"
	}
	e.logf("%s: %s", prefix, fmt.Sprintf(format, args...))
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

// maybeAutoConnect connects the launch target once, when it is addressable:
// a manual receiver immediately, a discovered one on the first snapshot that
// reports it online. A user connect or disconnect ends the launch phase, and
// a failure falls through to the ordinary reconnect policy.
func (e *Engine) maybeAutoConnect() bool {
	if e.autoTried {
		return false
	}
	key, ok := receivers.AutoTarget(e.reg)
	if !ok {
		e.autoTried = true
		return false
	}
	if e.tr != nil || e.reconnectWanted {
		e.autoTried = true
		return false
	}
	target, found := e.target(key)
	if !found || !target.Device.Online {
		return false
	}
	e.autoTried = true
	e.userDisconnect = false
	e.connect(target.Device)
	return true
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
			e.reg.Put(receivers.Entry{
				Key: d.ID, Kind: receivers.KindDiscovered,
				Name: d.Name, Model: d.Model, AirPlay2: d.AirPlay2,
				Host: d.Host, Port: d.Port, LastSeen: now,
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
	// it; the reconnect policy owns that receiver until the user disconnects.
	if (e.tr != nil || e.reconnectWanted) && e.selectedID != "" {
		live[e.selectedID] = true
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

// rebuildTargets re-projects the registry and discovery onto the row lists.
// Presentation rules live in receivers; the engine supplies inputs.
func (e *Engine) rebuildTargets() {
	targets := receivers.Merge(e.reg, e.discovered, func(k string) bool { return e.passwords[k] })
	active := ""
	if e.tr != nil || e.reconnectWanted {
		active = e.selectedID
	}
	targets = receivers.Manageable(targets, active)
	e.devices = receivers.Visible(targets, active)
	e.receiversViews = make([]receivers.View, len(targets))
	for i, t := range targets {
		e.receiversViews[i] = t.View(e.passwords[t.Entry.Key])
	}
	e.refreshDiscoverySeeds()
}

// receiverHosts is the seed list for discovery's directed pass: manual and
// last-known addresses, which survive an offline receiver and a restart.
func receiverHosts(reg receivers.Registry) []string {
	hosts := make([]string, 0, len(reg.Entries))
	for _, entry := range reg.Entries {
		if host := strings.TrimSpace(entry.Host); host != "" {
			hosts = append(hosts, host)
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

// target is the merged row for a key: the wire device plus its entry.
func (e *Engine) target(id string) (receivers.Target, bool) {
	for _, t := range receivers.Merge(e.reg, e.discovered, func(k string) bool { return e.passwords[k] }) {
		if t.Entry.Key == id {
			return t, true
		}
	}
	return receivers.Target{}, false
}

// hasPassword reports whether a stored receiver password exists.
func (e *Engine) hasPassword(key string) bool { return e.passwords[key] }

// passwordFor is the stored password for a receiver, "" when absent.
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
	if e.capture != nil {
		old := e.capture
		e.capture = nil
		go old.Stop()
	}
	e.captureRunning = false
	if e.captureStarting {
		e.captureQueued = true
		return
	}
	e.beginCaptureStart()
}

func (e *Engine) beginCaptureStart() {
	e.captureStarting = true
	e.captureGen++
	gen := e.captureGen
	cfg := capture.Config{
		Ring:       e.silence,
		EndpointID: e.captureEndpoint,
		ProcessID:  e.captureProcess,
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

// applyCaptureStart adopts a finished capture start.
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
		e.captureError = res.err.Error()
		e.captureRunning = false
		e.logf("capture failed: %v", res.err)
		e.publish()
		return
	}
	e.capture = res.src
	e.captureRunning = true
	e.captureError = ""
	stats := res.src.Stats()
	e.captureRate = stats.Rate
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
	if e.mute.state == MuteLocalActive && e.captureProcess == 0 && e.captureEndpoint == "" {
		// Capture follows the default device. If Windows moved the default,
		// move the mute with it.
		if ep, err := e.defaultEndpoint(); err == nil && ep.ID != e.mute.endpointID {
			e.refreshLocalMute()
		}
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
		e.captureError = info.err.Error()
		e.logf("capture error: %v", info.err)
	} else {
		e.captureError = ""
	}
	e.publish()
}

// markSettingsDirty schedules a settings write. The run loop coalesces
// writes at settingsSaveDebounce so interaction bursts cost one disk write;
// shutdown flushes whatever is pending.
func (e *Engine) markSettingsDirty() {
	if e.settingsDirtyAt.IsZero() {
		e.settingsDirtyAt = e.now().Add(settingsSaveDebounce)
	}
}

func (e *Engine) saveSettingsNow() {
	e.settingsDirtyAt = time.Time{}
	if e.sets != nil {
		_ = e.sets.Save(e.setsValue)
	}
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
	return Snapshot{
		Devices:            devs,
		Receivers:          views,
		SelectedID:         e.selectedID,
		State:              state,
		ConnectedName:      e.connectedName,
		Error:              e.lastError,
		PinRequired:        e.pinRequired,
		PinDevice:          e.pinDevice,
		PasswordRequired:   e.passwordRequired,
		PasswordDevice:     e.passwordDevice,
		PasswordDeviceID:   e.passwordDeviceID,
		PasswordTargetID:   e.passwordTargetID,
		Volume:             e.volume,
		Name:               e.setsValue.Name,
		Reconnect:          e.setsValue.Reconnect,
		Reconnecting:       e.reconnectWanted && !e.reconnectAt.IsZero(),
		AudioIdle:          e.audioPaused,
		MuteLocal:          e.mute.state,
		MuteLocalError:     e.mute.err,
		LatencyMS:          e.setsValue.LatencyMS,
		ReceiverLatencyMS:  e.receiverLatency,
		CaptureProcessPID:  e.captureProcess,
		CaptureProcessName: e.captureName,
		LampOff:            e.setsValue.Lamp.Off,
		LampCover:          e.setsValue.Lamp.Cover,
		LampPreset:         lamp.Normalize(e.setsValue.Lamp.Preset),
		TrayAnimation:      e.setsValue.TrayAnimation,
		CaptureEndpoint:    e.captureEndpoint,
		CaptureName:        e.captureName,
		CaptureRate:        e.captureRate,
		CaptureRunning:     e.captureRunning,
		CaptureError:       e.captureError,
		DroppedSamples:     dropped,
		StrictAuth:         e.setsValue.StrictReceiverAuth,
		PTPActive:          ptpActive,
		PTPReady:           ptpReady,
		PTPOffsetMS:        ptpOffset,
		LogLines:           lines,
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
