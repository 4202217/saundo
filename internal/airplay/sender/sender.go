// Package sender implements the AirPlay 2 realtime sender state machine.
//
// It is sans-I/O: it owns no socket, timer, or goroutine. Every outbound byte
// goes through raopio.IO, and the engine feeds it raopio.Event values plus
// Tick calls from one goroutine. The protocol order follows
// airplay2-sender-cpp, which was verified against Apple TV, HomePod, and
// macOS receivers:
//
//  1. Pair (HAP transient or HAP PIN), then encrypt RTSP immediately.
//  2. GET /info, session SETUP over rtsp://host/sessionId.
//  3. Open the event channel, send RECORD, then stream SETUP.
//  4. Stream uncompressed ALAC RTP with the audio key from the shared secret.
//  5. Answer every event-channel request with a minimal 200 OK, and send
//     POST /feedback every 2 seconds.
//
// SPDX-License-Identifier: Apache-2.0
package sender

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/audio/resample"
	"saundo/internal/audio/ring"
)

const (
	framesPerPacket    = 352
	raopRate           = 44100
	channels           = 2
	payloadBytes       = framesPerPacket * channels * 2
	backlogSize        = 1024
	pacerInterval      = 8 * time.Millisecond
	maxPacketsPerTick  = 16
	pacerLagWarnStreak = 5
	handshakeTimeout   = 10 * time.Second
	pinWaitTimeout     = 180 * time.Second
	feedbackInterval   = 25 * time.Second
	ap2Feedback        = 2 * time.Second
	volumePollInterval = time.Second
	// startWaitTimeout bounds how long a ready pair member waits for its peer.
	// It outlasts the 30 s PTP re-synchronization window.
	startWaitTimeout = 40 * time.Second
	inBufMaxFrames   = 8192
	defaultLatency   = 22050 + 44100
	noVolume         = -1000.0
	serverUserAgent  = "AirTunes/550.10"
	clientUserAgent  = "AirPlay/550.10"
)

// State is the sender's protocol phase.
type State uint8

const (
	StateIdle State = iota
	StateConnecting
	StatePairing
	StateHandshake
	StateStreaming
)

func (s State) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StatePairing:
		return "pairing"
	case StateHandshake:
		return "handshake"
	case StateStreaming:
		return "streaming"
	default:
		return "idle"
	}
}

// Level is a log severity.
type Level uint8

const (
	LevelInfo Level = iota
	LevelWarn
)

// Logger receives protocol diagnostics. It must not log PINs or keys.
type Logger func(level Level, format string, args ...any)

// Events are the sender's notifications to its host application. They fire on
// the goroutine that drives the sender.
type Events struct {
	// Launched reports that playback began (ok) or that the session failed
	// to start, with a user-facing reason.
	Launched func(ok bool, err string)
	// Closed reports that the session ended.
	Closed func()
	// Preempted reports that another sender took over the receiver. The
	// session is finished and must not be retried automatically.
	Preempted func(reason string)
	// PinRequired asks the UI for the receiver's 4-digit code.
	PinRequired func(deviceName string)
	// CredentialsObtained carries a first pairing's long-term credentials for
	// the host to persist under deviceID.
	CredentialsObtained func(deviceID, credsJSON string)
	// StateChanged reports every state transition, for the UI.
	StateChanged func(state State)
	// VolumeChanged reports the receiver's own volume in percent, polled
	// while streaming so hardware volume changes reach the UI.
	VolumeChanged func(percent float64)
	// ReceiverLatencyChanged reports the receiver's own arrival-to-render
	// latency in milliseconds from its stream SETUP response. An absent field
	// is not reported.
	ReceiverLatencyChanged func(ms int)
	// FeedbackBody reports the body of a /feedback reply. The receiver may
	// include queue or underrun telemetry; an empty body is not an error.
	FeedbackBody func([]byte)
}

// PTPClock is the receiver-synchronized clock a HomePod on software 27
// requires. Timeline maps a local Unix-nanosecond time onto the receiver's PTP
// clock and reports its clock identity.
type PTPClock interface {
	ClockID() uint64
	Ready() bool
	Timeline(localUnixNs uint64) (id uint64, networkNs uint64, ok bool)
	Activate()
}

// Identity is how the receiver sees this sender.
type Identity struct {
	Name     string
	DeviceID string
	Model    string
}

func defaultIdentity() Identity {
	return Identity{
		Name:     "声渡",
		DeviceID: "AA:BB:CC:DD:EE:FF",
		Model:    "iPhone14,3",
	}
}

type header struct{ key, value string }

type timer struct {
	interval  time.Duration
	repeating bool
	active    bool
	due       time.Time
}

func (t *timer) arm(now time.Time) {
	t.due = now.Add(t.interval)
	t.active = true
}

func (t *timer) disarm() { t.active = false }

type pendingReq struct {
	method string
	http   bool
	// replay resends the identical AP2 request with a fresh CSeq after a 401
	// digest challenge.
	replay func()
	// authRetry marks a request sent in answer to a challenge; a second 401
	// fails instead of looping.
	authRetry bool
}

type pairStage uint8

const (
	pairNone pairStage = iota
	pairAuthSetup
	pairPinStart
	pairSetupM2
	pairSetupM4
	pairSetupM6
	pairVerifyM2
	pairVerifyDone
	pairAP2Info
	pairAP2Session
	pairAP2Record
	pairAP2SetPeers
	pairAP2WaitPTP
	pairAP2Stream
	pairDone
)

type ap2State struct {
	srp *crypto.SRPClient

	ltSeed    []byte
	ltPub     []byte
	pairingID []byte

	accessoryID   []byte
	accessoryLTPK []byte

	verifyPub  []byte
	verifyPriv []byte

	sharedSecret []byte
	controlOut   []byte
	controlIn    []byte
	eventOut     []byte
	eventIn      []byte

	audioKey     []byte
	audioNonce   uint64
	encryptAudio bool

	sessionUUID string
	eventPort   uint16
	initialVol  float64
	hasInitial  bool
}

// Sender owns one AirPlay session's protocol state. All methods must be called
// from the engine's single event-loop goroutine, except the exported setters
// documented as safe before Start.
type Sender struct {
	io     raopio.IO
	events Events
	log    Logger
	clock  func() time.Time

	identity   Identity
	strictAuth bool
	// latencyMS is the presentation latency in milliseconds, applied to the
	// PTP stream SETUP (latencyMin/latencyMax) and the sync packets.
	latencyMS int
	// nextLatencySamples carries the SETUP latency to startStreaming.
	nextLatencySamples int64
	// receiverLatencyMS is the receiver's own arrival-to-render latency from
	// its stream SETUP response. 0 means it did not report one.
	receiverLatencyMS int
	// Diagnostic counters for the current session. They reset on Start.
	retransmits uint64
	lateTicks   uint64
	underruns   uint64
	sentBlocks  uint64
	sendErrors  uint64
	// duplicates is the number of extra copies of each audio packet. Zero is
	// the production default; the lossy-network probe drives it.
	duplicates int
	// prefill sends this many extra audio packets at the start epoch, filling
	// the receiver's buffer before the first network jitter arrives.
	prefill  int
	lastSync time.Time
	wifi     WiFiMetrics

	rtspConnected bool
	udpBound      bool
	localIP       string
	hostIP        string
	localControl  uint16
	localTiming   uint16
	rxBuf         []byte

	controlEncrypted bool
	controlGen       uint64
	ctrlSendCtr      uint64
	ctrlRecvCtr      uint64
	rtspEncBuf       []byte

	eventGen     uint64
	eventSendCtr uint64
	eventRecvCtr uint64
	eventEncBuf  []byte
	eventPlain   []byte

	pending []pendingReq
	cseq    int

	sessionID    uint32
	dacpID       string
	activeRemote uint32
	rtspSession  string
	host         string
	port         uint16
	name         string
	state        State

	serverPort  uint16
	controlPort uint16
	timingPort  uint16

	pacer      timer
	sync       timer
	timeout    timer
	pinTimer   timer
	feedback   timer
	volumePoll timer

	// Pacer health: how far the last tick left the stream behind the
	// wall-clock target, and the consecutive-late-tick streak. A streak is
	// the only signal that this host cannot encode and send at realtime.
	pacerLagFrames uint64
	pacerLagStreak int
	// volQuery marks an in-flight GET /info volume poll, and receiverVol is
	// the last percentage reported to the host (-1 = nothing yet).
	volQuery    bool
	receiverVol int
	// ptpWaitUntil extends the handshake deadline while the sender waits for
	// the PTP clock: after a long session the receiver needs a longer
	// re-synchronization window than the 10 s handshake timeout.
	ptpWaitUntil time.Time

	// startGate is the cohort this sender joins. Set before Start; nil means
	// the sender builds a one-member gate from its own ring.
	startGate *StartGate
	// activeGate is the gate for the current session.
	activeGate *StartGate
	// streamAccepted makes readyToStream idempotent: a duplicate RECORD or
	// SETUP reply must not count as a second arrival.
	streamAccepted bool
	// awaitingStart is true between arrival and the shared epoch.
	awaitingStart bool
	// startWaitUntil bounds how long a ready member waits for its peers.
	startWaitUntil time.Time

	clockStart    time.Time
	startTs       uint64
	framesSent    uint64
	latency       uint32
	seq           uint16
	firstAudio    bool
	paused        bool
	pauseWanted   bool
	pendingVolume float64

	ring      *ring.Buffer
	inputRate int
	resampler *resample.Linear

	backlog    [][]byte
	backlogSeq []int32

	npTitle, npArtist, npAlbum string
	// mediaCover is the cover the receiver track-info push supplies. lampCover
	// is the ambient-light image; lampOn says it owns the cover slot. With the
	// lamp off, a media cover wins and lampCover is the origin fallback.
	mediaCover     []byte
	mediaCoverMIME string
	lampCover      []byte
	lampCoverMIME  string
	lampOn         bool
	// sentCover is the last image delivered to this session, so an unchanged
	// cover is a no-op while a reconnect resends it.
	sentCover     []byte
	sentCoverMIME string
	// playbackState is the MediaRemote state the light follows (1 playing,
	// 2 paused); playbackStateSent is the last state delivered this session.
	playbackState     int
	playbackStateSent int

	ptpClock PTPClock
	// groupUUID tags a session as one member of a stereo pair. timingPeers
	// lists every member's address for SETPEERS; empty means this receiver
	// alone. Both are set by the group transport for a pair session.
	groupUUID   string
	timingPeers []string

	authMethod                   device.Auth
	family                       device.Family
	airplay2                     bool
	deviceID                     string
	credsJSON                    string
	digestPassword               string
	pairStage                    pairStage
	waitingForPin                bool
	triedTransientOnPin403       bool
	triedRepairOnVerifyAuthError bool
	srpSalt                      []byte
	srpServerB                   []byte
	pairSetupSessionKey          []byte
	ap2                          *ap2State

	digestRealm       string
	digestNonce       string
	pendingDigestURI  string
	pendingDigestVerb string
	digestRetried     bool
}

// New returns a sender that sends through io and reports through events.
func New(io raopio.IO, events Events, log Logger) *Sender {
	s := &Sender{
		io:         io,
		events:     events,
		log:        log,
		clock:      time.Now,
		identity:   defaultIdentity(),
		inputRate:  48000,
		backlog:    make([][]byte, backlogSize),
		backlogSeq: make([]int32, backlogSize),
	}
	s.resampler = resample.NewLinear(s.inputRate)
	for i := range s.backlogSeq {
		s.backlogSeq[i] = -1
	}
	s.pacer.interval = pacerInterval
	s.pacer.repeating = true
	s.sync.interval = time.Second
	s.sync.repeating = true
	s.timeout.interval = handshakeTimeout
	s.pinTimer.interval = pinWaitTimeout
	s.feedback.interval = feedbackInterval
	s.feedback.repeating = true
	s.volumePoll.interval = volumePollInterval
	s.volumePoll.repeating = true
	s.latencyMS = 200
	s.receiverVol = -1
	s.latency = defaultLatency
	s.nextLatencySamples = defaultLatency
	s.pendingVolume = noVolume
	return s
}

// SetClock replaces the clock used by timers and pacing. Tests use it.
func (s *Sender) SetClock(clock func() time.Time) {
	if clock == nil {
		clock = time.Now
	}
	s.clock = clock
}

// SetIdentity sets the name, device id, and model the receiver sees.
func (s *Sender) SetIdentity(id Identity) {
	defaults := defaultIdentity()
	id.Name = stripCRLF(id.Name)
	id.DeviceID = stripCRLF(id.DeviceID)
	id.Model = stripCRLF(id.Model)
	if id.Name == "" {
		id.Name = defaults.Name
	}
	if id.DeviceID == "" {
		id.DeviceID = defaults.DeviceID
	}
	if id.Model == "" {
		id.Model = defaults.Model
	}
	s.identity = id
}

// SetStrictReceiverAuth fails the session when a receiver proof or signature
// does not verify. The default logs and continues, matching the reference.
func (s *Sender) SetStrictReceiverAuth(strict bool) { s.strictAuth = strict }

// SetPTPClock selects PTP timing. A nil clock keeps NTP timing.
func (s *Sender) SetPTPClock(clock PTPClock) { s.ptpClock = clock }

// SetGroupUUID marks the session as a member of a stereo pair. The value is
// the pair's `tsid`, sent as groupUUID in the session SETUP.
func (s *Sender) SetGroupUUID(id string) { s.groupUUID = id }

// SetTimingPeers lists every receiver address in the pair. SETPEERS announces
// them together with the sender's own address.
func (s *Sender) SetTimingPeers(hosts []string) {
	s.timingPeers = append([]string(nil), hosts...)
}

// SetAuth describes how to reach the receiver before Start.
func (s *Sender) SetAuth(auth device.Auth, airplay2 bool, deviceID, credsJSON, password string) {
	s.authMethod = auth
	s.airplay2 = airplay2
	s.deviceID = deviceID
	s.credsJSON = credsJSON
	s.digestPassword = password
}

// SetFamily selects the receiver family used for key derivation, latency,
// and request headers. It is independent of SetAuth: a HomePod can pair
// transiently or with a password and keeps the HomePod profile.
func (s *Sender) SetFamily(family device.Family) { s.family = family }

// AttachRing supplies the PCM ring the sender pulls from while streaming.
func (s *Sender) AttachRing(r *ring.Buffer) { s.ring = r }

// SetStartGate puts the sender in a shared start cohort. Senders that share a
// gate begin consuming at the gate's epoch. A nil gate gates the sender alone
// over its own ring. Call before Start; create a fresh gate per session.
func (s *Sender) SetStartGate(g *StartGate) { s.startGate = g }

// SetInputFormat sets the sample rate of the PCM in the ring. A non-positive
// rate is treated as 48000.
func (s *Sender) SetInputFormat(rate int) {
	if rate <= 0 {
		rate = 48000
	}
	s.inputRate = rate
	if s.resampler != nil {
		s.resampler.SetInputRate(rate)
	}
}

// SetLatency sets the presentation latency in milliseconds; it takes effect
// on the next session's stream SETUP.
func (s *Sender) SetLatency(ms int) {
	if ms < device.MinLatencyMS {
		ms = device.MinLatencyMS
	}
	if ms > device.MaxLatencyMS {
		ms = device.MaxLatencyMS
	}
	s.latencyMS = ms
}

// setDuplicates sets how many extra copies of each audio packet go on the
// wire. The receiver dedups by RTP sequence, so the copies are idempotent.
// Zero is the production default; the lossy-network probe drives it.
func (s *Sender) setDuplicates(n int) {
	if n < 0 {
		n = 0
	}
	if n > 7 {
		n = 7
	}
	s.duplicates = n
}

// SetDuplicates sets how many extra copies of each audio packet go on the
// wire. One extra copy is the normal weak-network safety net.
func (s *Sender) SetDuplicates(n int) { s.setDuplicates(n) }

// SetPrefill sends n extra audio packets as playback begins, so the receiver
// has a few packets buffered before the first jittered packet arrives.
func (s *Sender) SetPrefill(n int) {
	if n < 0 {
		n = 0
	}
	if n > 8 {
		n = 8
	}
	s.prefill = n
}

// ReceiverLatencyMS returns the receiver's reported arrival-to-render latency
// in milliseconds. 0 means the receiver did not report one.
func (s *Sender) ReceiverLatencyMS() int { return s.receiverLatencyMS }

// Active reports whether a session is in progress.
func (s *Sender) Active() bool { return s.state != StateIdle }

// WaitingForPin reports whether the sender waits for SubmitPin.
func (s *Sender) WaitingForPin() bool { return s.waitingForPin }

// State reports the current protocol phase.
func (s *Sender) State() State { return s.state }

func (s *Sender) info(format string, args ...any) {
	if s.log != nil {
		s.log(LevelInfo, format, args...)
	}
}

func (s *Sender) warn(format string, args ...any) {
	if s.log != nil {
		s.log(LevelWarn, format, args...)
	}
}

func (s *Sender) setState(state State) {
	if s.state == state {
		return
	}
	s.state = state
	if s.events.StateChanged != nil {
		s.events.StateChanged(state)
	}
}

// Start connects, pairs, and begins a session. It binds the UDP sockets before
// SETUP because the request advertises their ports.
func (s *Sender) Start(host string, port uint16, name string) {
	if s.state != StateIdle {
		s.Stop()
	}
	s.host = host
	s.name = name
	s.hostIP = ""
	s.localIP = ""

	s.sessionID = rand32()
	s.dacpID = strings.ToUpper(fmt.Sprintf("%X", rand64()))
	s.activeRemote = rand32()
	s.seq = rand16()
	s.cseq = 0
	s.rxBuf = s.rxBuf[:0]
	s.controlEncrypted = false
	s.controlGen, s.eventGen = 0, 0
	s.ctrlSendCtr, s.ctrlRecvCtr = 0, 0
	s.rtspEncBuf = s.rtspEncBuf[:0]
	s.eventSendCtr, s.eventRecvCtr = 0, 0
	s.eventEncBuf = s.eventEncBuf[:0]
	s.eventPlain = s.eventPlain[:0]
	s.pending = s.pending[:0]
	s.rtspSession = ""
	s.port = port
	s.sentCover = s.sentCover[:0]
	s.sentCoverMIME = ""
	// A new session starts playing; a paused state is sent explicitly. This
	// keeps the old image-only path for the first light and avoids a state 1
	// command that some receivers answer by dropping the artwork.
	s.playbackStateSent = playbackPlaying
	s.serverPort, s.controlPort, s.timingPort = 0, 0, 0
	s.framesSent = 0
	s.firstAudio = true
	s.retransmits, s.lateTicks, s.underruns, s.sentBlocks, s.sendErrors = 0, 0, 0, 0, 0
	s.lastSync = time.Time{}
	s.wifi = WiFiMetrics{}
	if s.resampler == nil {
		s.resampler = resample.NewLinear(s.inputRate)
	} else {
		s.resampler.SetInputRate(s.inputRate)
	}
	for i := range s.backlog {
		s.backlog[i] = nil
	}
	for i := range s.backlogSeq {
		s.backlogSeq[i] = -1
	}
	s.pendingVolume = noVolume

	s.ap2 = nil
	s.pairStage = pairNone
	s.waitingForPin = false
	s.ptpWaitUntil = time.Time{}
	s.resetStartState()
	s.volQuery = false
	s.receiverVol = -1
	s.triedTransientOnPin403 = false
	s.triedRepairOnVerifyAuthError = false
	s.srpSalt = s.srpSalt[:0]
	s.srpServerB = s.srpServerB[:0]
	s.pairSetupSessionKey = nil
	s.digestRealm, s.digestNonce = "", ""
	s.pendingDigestURI, s.pendingDigestVerb = "", ""
	s.digestRetried = false

	timingPort, timingErr := s.io.UDPBind(raopio.UDPTiming)
	controlPort, controlErr := s.io.UDPBind(raopio.UDPControl)
	audioPort, audioErr := s.io.UDPBind(raopio.UDPAudio)
	s.localTiming, s.localControl = timingPort, controlPort
	s.udpBound = true
	bindErr := errors.Join(timingErr, controlErr, audioErr)
	if bindErr != nil || s.localTiming == 0 || s.localControl == 0 || audioPort == 0 {
		if bindErr != nil {
			s.warn("RAOP UDP bind failed: %v", bindErr)
		} else {
			s.warn("RAOP UDP bind failed")
		}
		s.releaseSockets()
		s.finishStartFailure("Could not bind UDP sockets")
		return
	}

	s.setState(StateConnecting)
	s.rtspConnected = false
	s.timeout.arm(s.now())
	s.info("connecting to %s:%d (%q)", host, port, name)
	if err := s.io.TCPConnect(raopio.TCPControl, host, port); err != nil {
		// A host may report an immediate failure; the normal path reports it
		// through an Event.
		s.fail(err.Error())
	}
}

// Stop tears the session down and closes the sockets.
func (s *Sender) Stop() {
	if s.state == StateIdle {
		return
	}
	s.stopTimers()
	if s.rtspConnected && (s.rtspSession != "" || s.airplay2) {
		var extra []header
		if s.rtspSession != "" {
			extra = append(extra, header{"Session", s.rtspSession})
		}
		s.sendRequest("TEARDOWN", s.rtspURI(), "", "", extra)
	}
	s.setState(StateIdle)
	s.io.TCPClose(raopio.TCPControl, true)
	s.rtspConnected = false
	s.releaseSockets()
	s.resetStartState()
	for i := range s.backlog {
		s.backlog[i] = nil
	}
	for i := range s.backlogSeq {
		s.backlogSeq[i] = -1
	}
	s.info("session stopped")
}

func (s *Sender) releaseSockets() {
	if s.udpBound {
		s.io.UDPClose(raopio.UDPAudio)
		s.io.UDPClose(raopio.UDPControl)
		s.io.UDPClose(raopio.UDPTiming)
		s.udpBound = false
	}
	s.localControl, s.localTiming = 0, 0
	s.io.TCPClose(raopio.TCPEvent, false)
}

func (s *Sender) finishStartFailure(why string) {
	s.setState(StateIdle)
	s.resetStartState()
	if s.events.Launched != nil {
		s.events.Launched(false, why)
	}
	if s.events.Closed != nil {
		s.events.Closed()
	}
}

func (s *Sender) fail(why string) {
	s.warn("RAOP error: %s", why)
	wasStarting := s.state != StateStreaming
	s.setState(StateIdle)
	s.stopTimers()
	s.resetStartState()
	s.io.TCPClose(raopio.TCPControl, false)
	s.rtspConnected = false
	s.releaseSockets()
	if wasStarting && s.events.Launched != nil {
		s.events.Launched(false, why)
	}
	if s.events.Closed != nil {
		s.events.Closed()
	}
}

// preempt ends a streaming session that the receiver handed to another
// sender. The receiver closes the displaced control channel cleanly, so this
// is a terminal outcome, not a retryable failure.
func (s *Sender) preempt(why string) {
	s.warn("session taken over: %s", why)
	s.setState(StateIdle)
	s.stopTimers()
	s.resetStartState()
	s.io.TCPClose(raopio.TCPControl, false)
	s.rtspConnected = false
	s.releaseSockets()
	for i := range s.backlog {
		s.backlog[i] = nil
	}
	for i := range s.backlogSeq {
		s.backlogSeq[i] = -1
	}
	if s.events.Preempted != nil {
		s.events.Preempted(why)
	}
	if s.events.Closed != nil {
		s.events.Closed()
	}
}

func (s *Sender) stopTimers() {
	s.pacer.disarm()
	s.sync.disarm()
	s.timeout.disarm()
	s.pinTimer.disarm()
	s.feedback.disarm()
	s.volumePoll.disarm()
}

// HandleEvent feeds one host notification into the sender.
func (s *Sender) HandleEvent(ev raopio.Event) {
	// Ignore late bytes or a close from a socket the sender already replaced.
	switch ev.TCP {
	case raopio.TCPControl:
		switch ev.Kind {
		case raopio.EventTCPConnected:
			s.controlGen = ev.Gen
		case raopio.EventTCPData, raopio.EventTCPClosed:
			if ev.Gen != s.controlGen {
				return
			}
		}
	case raopio.TCPEvent:
		switch ev.Kind {
		case raopio.EventTCPConnected:
			s.eventGen = ev.Gen
		case raopio.EventTCPData, raopio.EventTCPClosed:
			if ev.Gen != s.eventGen {
				return
			}
		}
	}
	switch ev.Kind {
	case raopio.EventTCPConnected:
		if ev.TCP == raopio.TCPEvent {
			if s.state != StateIdle {
				s.info("event channel open (%s:%d)", ev.Peer.IP, ev.Peer.Port)
			}
			return
		}
		if s.state != StateConnecting {
			return
		}
		s.rtspConnected = true
		s.localIP = ev.Local.IP
		s.hostIP = ev.Peer.IP
		s.info("RTSP connected to %s:%d", s.host, ev.Peer.Port)
		// The socket itself proves the target is this host, even when
		// discovery could not classify it (a manual row or a failed
		// interface read). Refuse before pairing; the receiver rejects the
		// session SETUP anyway, and a stream to its own host would loop.
		if s.localIP != "" && s.localIP == s.hostIP {
			s.fail("本机正在运行声渡，不能作为接收器。")
			return
		}
		s.beginAuthChain()
	case raopio.EventTCPConnectFailed:
		if ev.TCP == raopio.TCPEvent {
			if s.state == StateStreaming || s.state == StateHandshake {
				s.warn("event channel error: %s", ev.Err)
			}
			return
		}
		if s.state != StateIdle {
			why := ev.Err
			if why == "" {
				why = "Connection failed"
			}
			s.fail(why)
		}
	case raopio.EventTCPData:
		switch ev.TCP {
		case raopio.TCPControl:
			s.onRTSPData(ev.Data)
		default:
			s.onEventData(ev.Data)
		}
	case raopio.EventTCPClosed:
		if ev.TCP == raopio.TCPEvent {
			if s.state == StateStreaming || s.state == StateHandshake {
				if ev.Err != "" {
					s.warn("event channel closed: %s", ev.Err)
				} else {
					s.warn("event channel closed")
				}
			}
			return
		}
		s.rtspConnected = false
		if s.state == StateIdle {
			return
		}
		if ev.Err != "" {
			s.fail(ev.Err)
			return
		}
		if s.state == StateStreaming {
			s.preempt("receiver closed the session")
			return
		}
		wasStarting := s.state != StateStreaming
		s.setState(StateIdle)
		s.stopTimers()
		s.resetStartState()
		s.releaseSockets()
		for i := range s.backlog {
			s.backlog[i] = nil
		}
		for i := range s.backlogSeq {
			s.backlogSeq[i] = -1
		}
		s.info("connection closed by the device")
		if wasStarting && s.events.Launched != nil {
			s.events.Launched(false, "Connection closed by the device")
		}
		if s.events.Closed != nil {
			s.events.Closed()
		}
	case raopio.EventUDPDatagram:
		if s.state == StateIdle {
			return
		}
		switch ev.UDP {
		case raopio.UDPControl:
			s.onControlDatagram(ev.Data, ev.Peer)
		case raopio.UDPTiming:
			s.onTimingDatagram(ev.Data, ev.Peer)
		}
	}
}

func (s *Sender) now() time.Time { return s.clock() }

// resetStartState drops per-session wait state so a stopped sender reports no
// pending start deadline. startGate is configuration and is kept.
func (s *Sender) resetStartState() {
	s.streamAccepted = false
	s.awaitingStart = false
	s.activeGate = nil
	s.startWaitUntil = time.Time{}
	s.paused = false
	s.pauseWanted = false
}

// ntpAt returns 64-bit NTP wall time for t: seconds since 1900 in the high word
// and a 2^32-scaled fraction below.
func ntpAt(t time.Time) uint64 {
	us := uint64(t.UnixMicro())
	sec := us / 1_000_000
	frac := us % 1_000_000
	return ((sec + 0x83AA7E80) << 32) | ((frac << 32) / 1_000_000)
}

// elapsedFrames is the number of 44.1 kHz frames since the shared start epoch,
// and zero before it. The split keeps the multiply inside uint64: whole
// seconds times 44100 overflows only after millions of years, where the
// un-split nanosecond product wrapped after 4.84 days. The result matches the
// old floor(ns * 44100 / 1e9) exactly for durations that did not overflow.
func (s *Sender) elapsedFrames(t time.Time) uint64 {
	if s.clockStart.IsZero() || t.Before(s.clockStart) {
		return 0
	}
	d := t.Sub(s.clockStart)
	whole := uint64(d / time.Second)
	frac := uint64(d % time.Second)
	return whole*raopRate + frac*raopRate/1_000_000_000
}

// mediaRTP maps a local instant to the RTP timestamp that plays then, anchored
// at clockStart. Every member of a pair computes the same mapping, so pacer
// tick jitter does not become a published offset.
func (s *Sender) mediaRTP(t time.Time) uint32 {
	return uint32(uint64(s.latency) + s.elapsedFrames(t))
}

// Tick fires every timer whose deadline has passed.
func (s *Sender) Tick() {
	s.maybeBegin()
	if s.state == StateIdle {
		return
	}
	now := s.now()
	due := func(t *timer) bool {
		if !t.active || t.due.After(now) {
			return false
		}
		if t.repeating {
			t.due = now.Add(t.interval)
		} else {
			t.active = false
		}
		return true
	}
	if due(&s.timeout) {
		s.onHandshakeTimeout()
	}
	if due(&s.pinTimer) {
		s.onPinTimeout()
	}
	if due(&s.feedback) {
		s.onFeedbackTimer()
	}
	if due(&s.volumePoll) {
		s.onVolumePoll()
	}
	if due(&s.sync) {
		s.sendSyncPacket(false)
	}
	if due(&s.pacer) {
		s.onPacerTick()
	}
	if s.pairStage == pairAP2WaitPTP && s.state == StateHandshake && s.ptpClock != nil &&
		s.ptpClock.Ready() {
		s.info("PTP clock synchronized, starting stream SETUP")
		s.sendAP2SetupStream()
	}
}

// NextDeadline reports the earliest active timer, or the zero time. While the
// sender waits for the PTP clock, it asks to be polled so readiness is noticed
// without a timer of its own.
func (s *Sender) NextDeadline() time.Time {
	var best time.Time
	for _, t := range []*timer{&s.pacer, &s.sync, &s.timeout, &s.pinTimer, &s.feedback, &s.volumePoll} {
		if !t.active {
			continue
		}
		if best.IsZero() || t.due.Before(best) {
			best = t.due
		}
	}
	if s.pairStage == pairAP2WaitPTP && s.state == StateHandshake {
		poll := s.now().Add(50 * time.Millisecond)
		if best.IsZero() || poll.Before(best) {
			best = poll
		}
	}
	if s.awaitingStart {
		// Poll while a peer is outstanding; wake exactly at the epoch once the
		// gate is sealed. The bound in startWaitUntil is checked on each poll.
		now := s.now()
		poll := now.Add(50 * time.Millisecond)
		if s.activeGate != nil && s.activeGate.Sealed() {
			if epoch := s.activeGate.Epoch(); epoch.After(now) {
				poll = epoch
			} else {
				poll = now
			}
		}
		if best.IsZero() || poll.Before(best) {
			best = poll
		}
	}
	return best
}

func (s *Sender) onHandshakeTimeout() {
	if s.waitingForPin {
		return
	}
	if s.state == StateConnecting || s.state == StatePairing || s.state == StateHandshake {
		s.warn("handshake timeout in state %s, stage %d", s.state, s.pairStage)
		if s.pairStage == pairAP2WaitPTP && s.now().Before(s.ptpWaitUntil) {
			// The receiver gets a longer window to re-establish PTP after a
			// long session; keep waiting.
			s.timeout.arm(s.now())
			return
		}
		if s.pairStage == pairAP2WaitPTP {
			s.fail("PTP clock did not synchronize with the receiver. Allow Saundo through Windows Firewall on the private network, and make sure UDP ports 319 and 320 are free.")
			return
		}
		s.fail("Timed out waiting for the device")
	}
}

func (s *Sender) onPinTimeout() {
	if !s.waitingForPin {
		return
	}
	s.waitingForPin = false
	s.fail(fmt.Sprintf("No PIN was entered. Switch %s on and make sure its screen shows the AirPlay code, then try again.", s.name))
}

func (s *Sender) onFeedbackTimer() {
	if s.state != StateStreaming {
		return
	}
	if !s.airplay2 {
		s.sendRequest("POST", "/feedback", "", "", nil)
		return
	}
	req := "POST /feedback RTSP/1.0\r\n" +
		"CSeq: " + itoa(s.cseq) + "\r\n" +
		"User-Agent: " + clientUserAgent + "\r\n" +
		"DACP-ID: " + s.dacpID + "\r\n" +
		"Active-Remote: " + utoa(uint64(s.activeRemote)) + "\r\n" +
		"Client-Instance: " + s.dacpID + "\r\n" +
		"Content-Length: 0\r\n\r\n"
	s.cseq++
	s.pending = append(s.pending, pendingReq{method: "FEEDBACK"})
	s.writeRTSP([]byte(req))
}

// SubmitPin supplies the on-screen code and continues pairing.
func (s *Sender) SubmitPin(code string) {
	if !s.waitingForPin {
		return
	}
	s.waitingForPin = false
	s.pinTimer.disarm()
	s.timeout.arm(s.now())
	s.info("PIN entered, continuing pairing")
	s.sendPairSetupM3(code)
}

// Credentials is re-exported so callers do not depend on the creds package for
// the common case.
type Credentials = creds.Credentials

func rand32() uint32 {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return binary.LittleEndian.Uint32(b[:])
}

func rand64() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

func rand16() uint16 {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return binary.LittleEndian.Uint16(b[:])
}

func uuidV4Lower() string {
	b := crypto.RandomBytes(16)
	b[6] = (b[6] & 0x0F) | 0x40
	b[8] = (b[8] & 0x3F) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func itoa(v int) string    { return fmt.Sprint(v) }
func utoa(v uint64) string { return fmt.Sprint(v) }
