package engine

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/ptp"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/ring"
	"saundo/internal/receivers"
)

// cohortEventQueue bounds the merged host-event queue. 1024 covers several
// members' control traffic; the per-member forwarders block rather than drop,
// so pressure reaches the host readers instead of vanishing.
const cohortEventQueue = 1024

// cohortPrefill is the extra audio packets sent as playback begins, for every
// member. It matches the single-session and pair values.
const cohortPrefill = 2

// cohortTransport is the one production transport: one sender per planned
// member, one shared StartGate, one capture-fanout trim, and at most one PTP
// role family. A one-target session is the same code path with one or two
// members.
type cohortTransport struct {
	plan      sessionPlan
	members   []*cohortMember
	byPeer    map[string]int // member host -> index, for event routing
	gate      *sender.StartGate
	followers *ptp.Followers
	master    *ptp.Master

	events   chan raopio.Event
	stopCh   chan struct{}
	once     sync.Once
	dead     chan struct{}
	deadOnce sync.Once
	// lostKey is the target whose member closed first, read on the dead arm so
	// the rest of the session can continue without it.
	lostKey string

	onLaunched        func(ok bool, err string)
	onClosed          func()
	onPreempted       func(reason string)
	onPinRequired     func(deviceName string)
	onCredentials     func(deviceID, credsJSON string)
	onState           func(sender.State)
	onVolume          func(key string, percent float64)
	onReceiverLatency func(ms int)
	logf              func(format string, args ...any)

	stopping       bool
	failed         bool
	launchReported bool
	closedReported bool
	lastState      sender.State
	lampSkip       map[string]bool
}

type cohortMember struct {
	index     int
	device    device.Device
	targetKey string
	host      *raopio.Host
	sender    *sender.Sender
	ring      *ring.Buffer
	ptpTimed  bool
	// setLampArtwork is the member's ambient-light entry point; tests
	// substitute a recorder.
	setLampArtwork func(data []byte, mime string, on bool)
}

// openCohort builds the one production transport for a plan. Every value a
// sender needs before Start comes from the plan: endpoint, pairing material,
// family, latency, duplicate policy, group UUID, timing peers, and clock.
// Nothing about identity or timing is a post-construction setter.
func (e *Engine) openCohort(p sessionPlan) (transport, error) {
	if !p.live() || len(p.members) == 0 {
		return nil, fmt.Errorf("engine: empty session plan")
	}
	c := &cohortTransport{
		plan:              p,
		events:            make(chan raopio.Event, cohortEventQueue),
		stopCh:            make(chan struct{}),
		dead:              make(chan struct{}),
		onLaunched:        e.onLaunched,
		onClosed:          e.onClosed,
		onPreempted:       e.onPreempted,
		onPinRequired:     e.onPinRequired,
		onCredentials:     e.onCredentials,
		onState:           e.onStateChanged,
		onVolume:          e.onReceiverVolume,
		onReceiverLatency: e.onReceiverLatency,
		logf:              e.logf,
		lampSkip:          map[string]bool{},
	}
	switch p.clock.role {
	case ptpFollowers:
		f, err := ptp.NewFollowers(p.clock.followerHosts)
		if err != nil {
			return nil, fmt.Errorf("PTP: %w", err)
		}
		f.Start()
		c.followers = f
	case ptpMaster:
		m, err := ptp.NewMaster(p.clock.masterPeers)
		if err != nil {
			return nil, fmt.Errorf("PTP: %w", err)
		}
		c.master = m
	}
	// One gate for the cohort: every member must arrive before the epoch
	// seals, and the one trim covers every member ring on the capture fanout.
	c.gate = newCohortGate(len(p.members), e.captureSink)
	c.byPeer = make(map[string]int, len(p.members))
	for i := range p.members {
		pm := &p.members[i]
		host := raopio.NewHost()
		memberRing := ring.New(e.ring.Capacity())
		s := sender.New(host, c.memberEvents(i), e.senderLogFor(pm.device.Name))
		s.SetStrictReceiverAuth(e.setsValue.StrictReceiverAuth)
		s.SetDuplicates(pm.duplicates)
		s.SetPrefill(cohortPrefill)
		s.SetLatency(pm.latencyMS)
		s.AttachRing(memberRing)
		s.SetStartGate(c.gate)
		if pm.groupUUID != "" {
			s.SetGroupUUID(pm.groupUUID)
		}
		if len(pm.timingPeers) > 0 {
			s.SetTimingPeers(pm.timingPeers)
		}
		s.SetAuth(pm.auth.method, pm.auth.airplay2, pm.auth.deviceID,
			string(pm.auth.credsJSON), string(pm.auth.password))
		s.SetFamily(pm.auth.family)
		if pm.volumeSet {
			s.SetVolume(pm.volume)
		}
		s.SetChannel(pm.channel)
		if pm.ptpTimed {
			if c.followers != nil {
				s.SetPTPClock(c.followers.ClockFor(pm.device.Host))
			} else if c.master != nil {
				s.SetPTPClock(c.master.ClockFor(pm.device.Host))
			}
		}
		member := &cohortMember{
			index: i, device: pm.device, targetKey: pm.targetKey, host: host, sender: s, ring: memberRing,
			ptpTimed: pm.ptpTimed, setLampArtwork: s.SetLampArtwork,
		}
		c.members = append(c.members, member)
		c.byPeer[pm.device.Host] = i
		go c.forward(member)
	}
	return c, nil
}

// newCohortGate is the one gate constructor: every planned member must arrive
// before the epoch seals, and the one trim covers every ring on the capture
// fanout.
func newCohortGate(members int, trim ring.Trimmer) *sender.StartGate {
	return sender.NewStartGate(sender.StartGateConfig{
		Members: members,
		Cushion: sender.DefaultStartCushion,
		Trim:    trim,
	})
}

// memberEvents is one member sender's callback set. Launch, close, and
// preemption are terminal for the whole cohort; the other callbacks fan into
// the engine's single set, tagging the member where the message needs it.
func (c *cohortTransport) memberEvents(idx int) sender.Events {
	name := c.membersName(idx)
	return sender.Events{
		Launched: func(ok bool, err string) { c.memberLaunched(idx, ok, err) },
		Closed:   func() { c.memberClosed(idx) },
		Preempted: func(reason string) {
			if c.onPreempted != nil {
				c.onPreempted(fmt.Sprintf("%s：%s", name(), reason))
			}
		},
		PinRequired: func(deviceName string) {
			if c.onPinRequired != nil {
				c.onPinRequired(deviceName)
			}
		},
		CredentialsObtained: func(deviceID, credsJSON string) {
			if c.onCredentials != nil {
				c.onCredentials(deviceID, credsJSON)
			}
		},
		StateChanged: func(state sender.State) { c.memberStateChanged(idx, state) },
		VolumeChanged: func(percent float64) {
			if c.onVolume != nil {
				c.onVolume(c.memberTargetKey(idx), percent)
			}
		},
		ReceiverLatencyChanged: func(ms int) {
			if c.onReceiverLatency != nil {
				c.onReceiverLatency(ms)
			}
		},
	}
}

// membersName is a late-bound member name: memberEvents may be built before
// the member slice is complete when a test constructs the transport directly.
func (c *cohortTransport) membersName(idx int) func() string {
	return func() string {
		if idx >= 0 && idx < len(c.members) {
			return c.members[idx].device.Name
		}
		return ""
	}
}

// memberTargetKey is the late-bound counterpart for the per-target callbacks.
func (c *cohortTransport) memberTargetKey(idx int) string {
	if idx >= 0 && idx < len(c.members) {
		return c.members[idx].targetKey
	}
	return ""
}

// forward merges one member host's events into the cohort stream. One
// goroutine per member, the only goroutine the cohort adds.
func (c *cohortTransport) forward(m *cohortMember) {
	for {
		select {
		case <-c.stopCh:
			return
		case ev := <-m.host.Events():
			if ev.Peer.IP == "" {
				ev.Peer.IP = m.device.Host
			}
			select {
			case c.events <- ev:
			case <-c.stopCh:
				return
			}
		}
	}
}

func (c *cohortTransport) Events() <-chan raopio.Event { return c.events }
func (c *cohortTransport) Dead() <-chan struct{}       { return c.dead }
func (c *cohortTransport) LostTarget() string          { return c.lostKey }
func (c *cohortTransport) Rings() []*ring.Buffer {
	rings := make([]*ring.Buffer, 0, len(c.members))
	for _, m := range c.members {
		rings = append(rings, m.ring)
	}
	return rings
}

// Start starts the clock role's loop, then every member's handshake. The
// follower was already started at construction, as the single session did.
func (c *cohortTransport) Start() {
	if c.master != nil {
		c.master.Start()
	}
	for _, m := range c.members {
		m.sender.Start(m.device.Host, m.device.Port, m.device.Name)
	}
}

// Stop stops every sender, closes every host, then stops the clock role. It is
// idempotent.
func (c *cohortTransport) Stop() {
	c.once.Do(func() {
		c.stopping = true
		close(c.stopCh)
		for _, m := range c.members {
			m.sender.Stop()
		}
		for _, m := range c.members {
			m.host.Close()
		}
		if c.followers != nil {
			c.followers.Stop()
		}
		if c.master != nil {
			c.master.Stop()
		}
	})
}

func (c *cohortTransport) Active() bool { return c.State() != sender.StateIdle }

// State folds the members: streaming only when every member streams, idle when
// none is active, else the earliest active member's state.
func (c *cohortTransport) State() sender.State {
	if c.stopping {
		return sender.StateIdle
	}
	allStreaming := true
	anyActive := false
	earliest := sender.StateStreaming
	for _, m := range c.members {
		state := m.sender.State()
		if state != sender.StateStreaming {
			allStreaming = false
		}
		if state != sender.StateIdle {
			anyActive = true
			if state < earliest {
				earliest = state
			}
		}
	}
	if allStreaming {
		return sender.StateStreaming
	}
	if !anyActive {
		return sender.StateIdle
	}
	return earliest
}

// HandleEvent routes a host event to its member by peer address. A
// single-member cohort takes any event: one receiver's replies can carry a
// local address that differs from the one the plan dialed, and there is no
// other member the event could belong to. A multi-member cohort drops an
// unknown peer, because one host cannot be told from another.
func (c *cohortTransport) HandleEvent(ev raopio.Event) {
	if i, ok := c.byPeer[ev.Peer.IP]; ok && i < len(c.members) {
		c.members[i].sender.HandleEvent(ev)
		return
	}
	if len(c.members) == 1 {
		c.members[0].sender.HandleEvent(ev)
		return
	}
	c.logf("cohort: dropping event for unknown peer %s", ev.Peer.IP)
}

func (c *cohortTransport) Tick() {
	for _, m := range c.members {
		m.sender.Tick()
	}
}

// NextDeadline is the earliest member deadline.
func (c *cohortTransport) NextDeadline() time.Time {
	var best time.Time
	for _, m := range c.members {
		deadline := m.sender.NextDeadline()
		if deadline.IsZero() {
			continue
		}
		if best.IsZero() || deadline.Before(best) {
			best = deadline
		}
	}
	return best
}

func (c *cohortTransport) SetIdentity(id sender.Identity) {
	for _, m := range c.members {
		m.sender.SetIdentity(id)
	}
}

func (c *cohortTransport) SetInputFormat(rate int) {
	for _, m := range c.members {
		m.sender.SetInputFormat(rate)
	}
	// The shared gate computes its cushion from this rate, and a capture
	// update can land between two member arrivals.
	c.gate.SetInputRate(rate)
}

func (c *cohortTransport) SetVolume(pct float64) {
	for _, m := range c.members {
		m.sender.SetVolume(pct)
	}
}

// SetTargetVolume applies one target's volume to every member, so a stereo
// pair shares the value and a single receiver gets its own.
func (c *cohortTransport) SetTargetVolume(key string, pct float64) {
	for _, m := range c.members {
		if m.targetKey == key {
			m.sender.SetVolume(pct)
		}
	}
}

// SetTargetChannel applies one target's stereo map to every member. The mix
// runs per packet, so no session state changes.
func (c *cohortTransport) SetTargetChannel(key string, mode sender.ChannelMode) {
	for _, m := range c.members {
		if m.targetKey == key {
			m.sender.SetChannel(mode)
		}
	}
}

// SetPaused applies the request only when every member can apply it, then
// applies it to every member. A mixed AP1/AP2 cohort must not pause its AP2
// members alone: the AP1 member refuses and keeps streaming, and nothing would
// ever resume the paused members.
func (c *cohortTransport) SetPaused(paused bool) bool {
	for _, m := range c.members {
		if !m.sender.PauseSupported() {
			return false
		}
	}
	applied := true
	for _, m := range c.members {
		if !m.sender.SetPaused(paused) {
			applied = false
		}
	}
	return applied
}

// Pushing reports whether any member is still sending RTP.
func (c *cohortTransport) Pushing() bool {
	for _, m := range c.members {
		if m.sender.Pushing() {
			return true
		}
	}
	return false
}

func (c *cohortTransport) SetStrictReceiverAuth(strict bool) {
	for _, m := range c.members {
		m.sender.SetStrictReceiverAuth(strict)
	}
}

func (c *cohortTransport) SubmitPin(code string) {
	for _, m := range c.members {
		m.sender.SubmitPin(code)
	}
}

func (c *cohortTransport) WaitingForPin() bool {
	for _, m := range c.members {
		if m.sender.WaitingForPin() {
			return true
		}
	}
	return false
}

// InitialVolumePercent takes the first report in plan order, so the primary
// target's volume wins.
func (c *cohortTransport) InitialVolumePercent() (float64, bool) {
	for _, m := range c.members {
		if pct, ok := m.sender.InitialVolumePercent(); ok {
			return pct, true
		}
	}
	return 0, false
}

// InitialVolumePercentFor takes the first report among one target's members.
func (c *cohortTransport) InitialVolumePercentFor(key string) (float64, bool) {
	for _, m := range c.members {
		if m.targetKey != key {
			continue
		}
		if pct, ok := m.sender.InitialVolumePercent(); ok {
			return pct, true
		}
	}
	return 0, false
}

func (c *cohortTransport) SetNowPlaying(title, artist, album string, cover []byte, coverMIME string) {
	for _, m := range c.members {
		m.sender.SetNowPlaying(title, artist, album, cover, coverMIME)
	}
}

// SetLampArtwork sends the light to every member that can show top-display
// artwork. A member whose model or software version is unknown is skipped,
// never guessed, and the skip is logged once per member.
func (c *cohortTransport) SetLampArtwork(data []byte, mime string, on bool) {
	for _, m := range c.members {
		if !device.TopDisplayCapable(m.device) {
			if !c.lampSkip[m.device.Name] {
				c.lampSkip[m.device.Name] = true
				c.logf("ambient light: %s does not support top-display artwork; skipped", m.device.Name)
			}
			continue
		}
		m.setLampArtwork(data, mime, on)
	}
}

func (c *cohortTransport) SetPlaybackState(playing bool) {
	for _, m := range c.members {
		m.sender.SetPlaybackState(playing)
	}
}

// PTP reports the one role family's state: the follower set's aggregate
// stats, or master readiness over every timed member.
func (c *cohortTransport) PTP() (active, ready bool, offsetMS float64) {
	if c.followers != nil {
		st := c.followers.Stats()
		return true, st.Ready, st.OffsetNS / 1e6
	}
	if c.master == nil {
		return false, false, 0
	}
	ready = true
	for _, m := range c.members {
		if !m.ptpTimed {
			continue
		}
		if !c.master.Ready(m.device.Host) {
			ready = false
		}
	}
	return true, ready, 0
}

// Stats returns one entry per member in plan order.
func (c *cohortTransport) Stats() []sender.Stats {
	stats := make([]sender.Stats, 0, len(c.members))
	for _, m := range c.members {
		stats = append(stats, m.sender.Stats())
	}
	return stats
}

// DroppedEvents is the sum of the members' host queue drops.
func (c *cohortTransport) DroppedEvents() uint64 {
	var dropped uint64
	for _, m := range c.members {
		dropped += m.host.Dropped()
	}
	return dropped
}

func (c *cohortTransport) signalDead() {
	c.deadOnce.Do(func() { close(c.dead) })
}

func (c *cohortTransport) memberLaunched(idx int, ok bool, err string) {
	if c.stopping {
		return
	}
	if !ok {
		if c.launchReported {
			return
		}
		c.launchReported = true
		c.failed = true
		if c.onLaunched != nil {
			c.onLaunched(false, fmt.Sprintf("%s：%s", c.members[idx].device.Name, err))
		}
		c.signalDead()
		return
	}
	launched := true
	for _, m := range c.members {
		if m.sender.State() != sender.StateStreaming {
			launched = false
		}
	}
	if launched && !c.launchReported {
		c.launchReported = true
		if c.onLaunched != nil {
			c.onLaunched(true, "")
		}
	}
	c.stateChanged()
}

func (c *cohortTransport) memberClosed(idx int) {
	// A close before the cohort reports its launch still means teardown: the
	// member may have streamed while its partner had not launched yet.
	if c.stopping || c.failed || c.closedReported {
		return
	}
	c.closedReported = true
	c.lostKey = c.members[idx].targetKey
	if c.onClosed != nil {
		c.onClosed()
	}
	c.signalDead()
}

func (c *cohortTransport) memberStateChanged(int, sender.State) { c.stateChanged() }

func (c *cohortTransport) stateChanged() {
	state := c.State()
	if state == c.lastState {
		return
	}
	c.lastState = state
	if c.onState != nil {
		c.onState(state)
	}
}

// startPlan closes the current session and opens the cohort for plan. hold
// keeps the local output muted across the rebuild: the mute belongs to the
// user's session, not to a transport. When the cohort cannot open, the plan is
// cleared, the mute is restored, and the error surfaces as ErrSessionOpen.
// wantedKeys keeps the new set either way, so Retry on any active row retries
// the whole request.
func (e *Engine) startPlan(plan sessionPlan, hold bool) {
	// A rebuild carries the user's join onto the new plan while that target is
	// still planned, so a leave or a latency change mid-join keeps its row the
	// joining one. A user connect sets its own key before calling here.
	if plan.joinKey == "" && plan.has(e.plan.joinKey) {
		plan.joinKey = e.plan.joinKey
	}
	// buildPlan loads the member credentials, so an unreadable file may have
	// been moved aside a moment ago. Surface that before the cohort opens, as
	// the single-session connect did.
	e.noteCredentialQuarantine()
	e.closeSession(hold)
	e.receiverLatency = 0
	e.plan = plan
	e.wantedKeys = plan.keys()
	e.selectedID = plan.targets[0].key // the primary target; stable across adds
	primary := plan.targets[0]
	e.activeHost = plan.members[primary.first].device.Host
	e.activePort = plan.members[primary.first].device.Port
	e.activeMembers = plan.devices()

	tr, err := e.newCohort(plan)
	if err != nil {
		e.plan = sessionPlan{}
		e.activeMembers = nil
		if hold {
			e.restoreLocalMute()
		}
		e.fail(ErrSessionOpen, err.Error())
		e.logf("session open failed: %v", err)
		e.publish()
		return
	}
	e.tr = tr
	tr.SetIdentity(sender.Identity{Name: e.setsValue.Name, DeviceID: e.setsValue.DeviceID, Model: "iPhone14,3"})
	tr.SetStrictReceiverAuth(e.setsValue.StrictReceiverAuth)
	if e.captureRate > 0 {
		tr.SetInputFormat(e.captureRate)
	}
	// The rings go on the fanout before Start so the gate's one trim covers
	// every member. This runs for one target too: there is no single-ring path.
	e.attachMemberRings(tr.Rings())
	// The rebuilt cohort has seen no metadata yet. Hand it the last payload
	// before Start, so every receiver shows the current track immediately.
	e.track.replay(tr)
	tr.Start()
	for _, m := range plan.members {
		e.logf("cohort: %s latency=%d ms", m.device.Name, m.latencyMS)
	}
	e.publish()
}

// connectTargets is the one Connect entry point.
func (e *Engine) connectTargets(key string) {
	e.connectResolved(key, nil)
}

// connectResolved runs Connect with an optional pre-resolved target. The
// launch path hands in the registry's cached view, which may describe an
// offline pair as addressable; every other caller re-resolves from the merged
// rows. An add that cannot run never touches the live session: nothing is
// stopped until validation and the plan both succeed.
func (e *Engine) connectResolved(key string, cached *receivers.Target) {
	e.autoTried = true
	// A user action supersedes the previous attempt's state: a stale PIN
	// dialog, a takeover notice, and the old failure all end here.
	e.userDisconnect = false
	e.reconnectCount, e.reconnectNum = 0, 0
	e.connectAuto = false
	e.pinRequired, e.pinDevice = false, ""
	e.clearError()
	e.takenOver = false
	e.notice = SessionNotice{}

	action, keys := e.connectIntent(key)
	if action == connectNoop {
		e.logf("cohort: %s is already in the session", e.nameFor(key))
		e.publish()
		return
	}
	t, ok := e.target(key)
	if cached != nil {
		t, ok = *cached, true
	}
	if !ok {
		e.fail(ErrDeviceMissing)
		e.publish()
		return
	}
	if (action == connectAdd || action == connectReplace) && e.plan.containsAnyMember(t) {
		// A row that resolves to a receiver already in the cohort (for
		// example a pair member seen under its own key) is a no-op click.
		e.logf("cohort: %s is already in the session", t.Device.Name)
		e.publish()
		return
	}
	if err := e.validateTarget(t); err != nil {
		if errors.Is(err, errAwaitInput) { // password prompt is now set
			e.publish()
			return
		}
		e.failPlan(err)
		e.publish()
		return // the live cohort keeps playing
	}
	if action == connectRetry {
		// Retry rebuilds every wanted target, so it runs the same prune and
		// guards as the automatic retry; validating only the clicked key would
		// let the rest of the set bypass them.
		keys = e.pruneWanted(keys)
		if len(keys) == 0 {
			e.fail(ErrDeviceMissing)
			e.publish()
			return
		}
		if key, err := e.validateWanted(keys); err != nil {
			if errors.Is(err, errAwaitInput) { // password prompt is now set
				e.publish()
				return
			}
			e.failPlan(err)
			e.logf("retry: %s refused: %v", key, err)
			e.publish()
			return // the wanted set stays for the next attempt
		}
	}
	e.clearPasswordPrompt()

	plan, err := e.buildPlan(keys)
	if err != nil {
		e.failPlan(err) // clock mix, duplicate member, structural target error
		e.publish()
		return
	}
	// The clicked target is this attempt's joiner; a retry names it too, so the
	// row the user acted on is the one that can cancel.
	plan.joinKey = key
	e.startPlan(plan, action == connectAdd || action == connectReplace)
}

// rebuildSession rebuilds the live cohort from its own keys. The new base
// latency is already in settings, so buildPlan recomputes every member's value.
func (e *Engine) rebuildSession() {
	if !e.plan.live() {
		return
	}
	plan, err := e.buildPlan(e.plan.keys())
	if err != nil {
		e.failPlan(err)
		e.logf("session rebuild refused; the running cohort keeps its latency")
		e.publish()
		return
	}
	e.startPlan(plan, true)
}
