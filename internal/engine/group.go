package engine

import (
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

// groupTransport runs one AirPlay 2 session per member of a stereo pair. A
// HomePod pair does not relay one member's stream to the other, and each
// member needs the same groupUUID and a PTP timeline supplied by the sender.
type groupTransport struct {
	group   device.Device
	members []*groupMember
	rings   []*ring.Buffer
	master  *ptp.Master

	events   chan raopio.Event
	stopCh   chan struct{}
	once     sync.Once
	dead     chan struct{}
	deadOnce sync.Once

	onLaunched        func(ok bool, err string)
	onClosed          func()
	onPreempted       func(reason string)
	onPinRequired     func(deviceName string)
	onCredentials     func(deviceID, credsJSON string)
	onState           func(sender.State)
	onVolume          func(percent float64)
	onReceiverLatency func(ms int)
	logf              func(format string, args ...any)

	stopping       bool
	failed         bool
	launchReported bool
	closedReported bool
	lastState      sender.State
}

type groupMember struct {
	device device.Device
	host   *raopio.Host
	sender *sender.Sender
	ring   *ring.Buffer
	// setLampArtwork is the member's ambient-light entry point. The open path
	// points it at the sender; tests substitute a recorder.
	setLampArtwork func(data []byte, mime string, on bool)
}

// openGroupSession builds both member sessions and the shared PTP master. The
// engine supplies per-member credentials through SetMemberAuth before Start.
func (e *Engine) openGroupSession(d device.Device) (transport, func(), error) {
	if len(d.Members) != 2 {
		return nil, nil, fmt.Errorf("stereo group %q has %d members, want 2", d.Name, len(d.Members))
	}
	hosts := make([]string, 0, len(d.Members))
	for _, m := range d.Members {
		hosts = append(hosts, m.Host)
	}
	// One predicate for both routes: a group routes PTP exactly when a single
	// device would (HAP + feature bit 41 + AudioAccessory/Mac model).
	usePTP := usesPTP(d)
	var master *ptp.Master
	if usePTP {
		m, err := ptp.NewMaster(hosts)
		if err != nil {
			return nil, nil, fmt.Errorf("PTP: %w", err)
		}
		master = m
	}
	g := &groupTransport{
		group:             d,
		master:            master,
		events:            make(chan raopio.Event, 512),
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
	}
	// One gate for the pair: every member holds until all streams are accepted,
	// then the fanout is trimmed once under its push lock and both members begin
	// on the same epoch. attachMemberRings runs before Start, so both member
	// rings are on the fanout by trim time.
	gate := sender.NewStartGate(sender.StartGateConfig{
		Members: len(d.Members),
		Cushion: sender.DefaultStartCushion,
		Trim:    e.captureSink,
	})
	for i, m := range d.Members {
		host := raopio.NewHost()
		memberRing := ring.New(e.ring.Capacity())
		member := &groupMember{device: m, host: host, ring: memberRing}
		idx := i
		s := sender.New(host, g.memberEvents(idx), e.senderLog)
		s.SetStrictReceiverAuth(e.setsValue.StrictReceiverAuth)
		entry, _ := e.reg.Get(d.ID)
		s.SetDuplicates(receivers.DuplicatesFor(entry))
		s.SetPrefill(2)
		s.AttachRing(memberRing)
		s.SetStartGate(gate)
		if d.StereoID != "" {
			s.SetGroupUUID(d.StereoID)
		}
		s.SetTimingPeers(hosts)
		if master != nil {
			s.SetPTPClock(master.ClockFor(m.Host))
		}
		member.sender = s
		member.setLampArtwork = s.SetLampArtwork
		g.members = append(g.members, member)
		g.rings = append(g.rings, memberRing)
		go g.forward(member)
	}
	return g, func() { g.Stop() }, nil
}

// memberEvents is one member sender's event set. The group forwards each
// callback to the engine's single set, tagging the member index where the
// callback needs it. Receiver latency is a per-member report; the engine
// keeps the largest value for the session.
func (g *groupTransport) memberEvents(idx int) sender.Events {
	return sender.Events{
		Launched: func(ok bool, err string) { g.memberLaunched(idx, ok, err) },
		Closed:   func() { g.memberClosed(idx) },
		Preempted: func(reason string) {
			if g.onPreempted != nil {
				g.onPreempted(fmt.Sprintf("%s：%s", g.members[idx].device.Name, reason))
			}
		},
		PinRequired: func(name string) {
			if g.onPinRequired != nil {
				g.onPinRequired(name)
			}
		},
		CredentialsObtained: func(deviceID, credsJSON string) {
			if g.onCredentials != nil {
				g.onCredentials(deviceID, credsJSON)
			}
		},
		StateChanged: func(state sender.State) { g.memberStateChanged(idx, state) },
		VolumeChanged: func(percent float64) {
			if g.onVolume != nil {
				g.onVolume(percent)
			}
		},
		ReceiverLatencyChanged: func(ms int) {
			if g.onReceiverLatency != nil {
				g.onReceiverLatency(ms)
			}
		},
	}
}

// forward merges each member host's events into one stream. The sender is
// routed by the peer address; host-level failures carry no peer, so they are
// tagged with the member's address.
func (g *groupTransport) forward(m *groupMember) {
	for {
		select {
		case <-g.stopCh:
			return
		case ev := <-m.host.Events():
			if ev.Peer.IP == "" {
				ev.Peer.IP = m.device.Host
			}
			select {
			case g.events <- ev:
			case <-g.stopCh:
				return
			}
		}
	}
}

func (g *groupTransport) Events() <-chan raopio.Event { return g.events }
func (g *groupTransport) Rings() []*ring.Buffer       { return g.rings }

// Dead closes when a member died or a launch failed. The engine selects on
// it and runs closeSession on the engine goroutine: the stop must not happen
// inside a sender callback (HandleEvent re-entrancy) nor on another
// goroutine (the sender methods are single-goroutine).
func (g *groupTransport) Dead() <-chan struct{} { return g.dead }

func (g *groupTransport) signalDead() {
	g.deadOnce.Do(func() { close(g.dead) })
}

func (g *groupTransport) Start(string, uint16, string) {
	if g.master != nil {
		g.master.Start()
	}
	for _, m := range g.members {
		m.sender.Start(m.device.Host, m.device.Port, m.device.Name)
	}
}

func (g *groupTransport) Stop() {
	g.once.Do(func() {
		g.stopping = true
		close(g.stopCh)
		for _, m := range g.members {
			m.sender.Stop()
		}
		for _, m := range g.members {
			m.host.Close()
		}
		if g.master != nil {
			g.master.Stop()
		}
	})
}

func (g *groupTransport) Active() bool { return g.State() != sender.StateIdle }

func (g *groupTransport) State() sender.State {
	if g.stopping {
		return sender.StateIdle
	}
	allStreaming := true
	anyActive := false
	earliest := sender.StateStreaming
	for _, m := range g.members {
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

func (g *groupTransport) HandleEvent(ev raopio.Event) {
	for _, m := range g.members {
		if m.device.Host == ev.Peer.IP {
			m.sender.HandleEvent(ev)
			return
		}
	}
	g.logf("group: dropping event for unknown peer %s", ev.Peer.IP)
}

func (g *groupTransport) Tick() {
	for _, m := range g.members {
		m.sender.Tick()
	}
}

func (g *groupTransport) NextDeadline() time.Time {
	var best time.Time
	for _, m := range g.members {
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

func (g *groupTransport) SetIdentity(id sender.Identity) {
	for _, m := range g.members {
		m.sender.SetIdentity(id)
	}
}

// SetAuth is unused for a group; the engine calls SetMemberAuth per member.
func (g *groupTransport) SetAuth(device.Auth, bool, string, string, string) {}

// SetFamily is unused for a group; SetMemberAuth carries each member's family.
func (g *groupTransport) SetFamily(device.Family) {}

func (g *groupTransport) SetMemberAuth(m device.Device, credsJSON, password string) {
	for _, member := range g.members {
		if member.device.ID != m.ID {
			continue
		}
		member.sender.SetAuth(m.Auth, m.Auth.IsHAP(), m.ID, credsJSON, password)
		member.sender.SetFamily(m.Family)
	}
}

func (g *groupTransport) SetPTPClock(sender.PTPClock) {}

func (g *groupTransport) AttachRing(*ring.Buffer) {}

func (g *groupTransport) SetInputFormat(rate int) {
	for _, m := range g.members {
		m.sender.SetInputFormat(rate)
	}
}

func (g *groupTransport) SetLatency(ms int) {
	for _, m := range g.members {
		m.sender.SetLatency(ms)
	}
}

func (g *groupTransport) SetVolume(pct float64) {
	for _, m := range g.members {
		m.sender.SetVolume(pct)
	}
}

func (g *groupTransport) SetPaused(paused bool) bool {
	applied := true
	for _, m := range g.members {
		if !m.sender.SetPaused(paused) {
			applied = false
		}
	}
	return applied
}

func (g *groupTransport) SetStrictReceiverAuth(strict bool) {
	for _, m := range g.members {
		m.sender.SetStrictReceiverAuth(strict)
	}
}

func (g *groupTransport) SubmitPin(code string) {
	for _, m := range g.members {
		m.sender.SubmitPin(code)
	}
}

func (g *groupTransport) WaitingForPin() bool {
	for _, m := range g.members {
		if m.sender.WaitingForPin() {
			return true
		}
	}
	return false
}

func (g *groupTransport) InitialVolumePercent() (float64, bool) {
	for _, m := range g.members {
		if pct, ok := m.sender.InitialVolumePercent(); ok {
			return pct, true
		}
	}
	return 0, false
}

func (g *groupTransport) SetNowPlaying(title, artist, album string, cover []byte, coverMIME string) {
	for _, m := range g.members {
		m.sender.SetNowPlaying(title, artist, album, cover, coverMIME)
	}
}

// SetLampArtwork sends the light to each member that can show top-display
// artwork. A member whose model or software version is unknown is skipped,
// never guessed.
func (g *groupTransport) SetLampArtwork(data []byte, mime string, on bool) {
	for _, m := range g.members {
		if !device.TopDisplayCapable(m.device) {
			g.logf("ambient light: %s in %q does not support top-display artwork; skipped", m.device.Name, g.group.Name)
			continue
		}
		m.setLampArtwork(data, mime, on)
	}
}

// SetPlaybackState sends the media playback state to every member; the top
// displays all go dark or light together.
func (g *groupTransport) SetPlaybackState(playing bool) {
	for _, m := range g.members {
		m.sender.SetPlaybackState(playing)
	}
}

func (g *groupTransport) PTP() (active, ready bool, offsetMS float64) {
	if g.master == nil {
		return false, false, 0
	}
	ready = true
	for _, m := range g.members {
		if !g.master.Ready(m.device.Host) {
			ready = false
		}
	}
	return true, ready, 0
}

// Stats returns every member's counters in member order. The engine folds
// them into the diagnostics group rules.
func (g *groupTransport) Stats() []sender.Stats {
	stats := make([]sender.Stats, 0, len(g.members))
	for _, m := range g.members {
		stats = append(stats, m.sender.Stats())
	}
	return stats
}

func (g *groupTransport) memberLaunched(idx int, ok bool, err string) {
	if g.stopping {
		return
	}
	if !ok {
		if g.launchReported {
			return
		}
		g.launchReported = true
		g.failed = true
		if g.onLaunched != nil {
			g.onLaunched(false, fmt.Sprintf("%s：%s", g.members[idx].device.Name, err))
		}
		g.signalDead()
		return
	}
	launched := true
	for _, m := range g.members {
		if m.sender.State() != sender.StateStreaming {
			launched = false
		}
	}
	if launched && !g.launchReported {
		g.launchReported = true
		if g.onLaunched != nil {
			g.onLaunched(true, "")
		}
	}
	g.stateChanged()
}

func (g *groupTransport) memberClosed(idx int) {
	// A close before the cohort reports its launch still means teardown: the
	// member may have streamed while its partner had not launched yet.
	if g.stopping || g.failed || g.closedReported {
		return
	}
	g.closedReported = true
	if g.onClosed != nil {
		g.onClosed()
	}
	g.signalDead()
}

func (g *groupTransport) memberStateChanged(int, sender.State) { g.stateChanged() }

func (g *groupTransport) stateChanged() {
	state := g.State()
	if state == g.lastState {
		return
	}
	g.lastState = state
	if g.onState != nil {
		g.onState(state)
	}
}
