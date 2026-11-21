package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/ring"
)

// fakeIO is a raopio.IO that touches no sockets, so tests can run real
// senders through Start and Stop.
type fakeIO struct{}

func (fakeIO) TCPConnect(raopio.TCPChannel, string, uint16) error { return nil }
func (fakeIO) TCPSend(raopio.TCPChannel, []byte) error            { return nil }
func (fakeIO) TCPClose(raopio.TCPChannel, bool)                   {}
func (fakeIO) UDPBind(raopio.UDPChannel) (uint16, error)          { return 53561, nil }
func (fakeIO) UDPSend(raopio.UDPChannel, raopio.Endpoint, []byte) error {
	return nil
}
func (fakeIO) UDPClose(raopio.UDPChannel) {}

// buildTestCohort wires a cohort of n members over a fake io and real hosts,
// with the same callbacks and shared gate the engine installs.
func buildTestCohort(n int) *cohortTransport {
	c := &cohortTransport{
		events:   make(chan raopio.Event, 8),
		stopCh:   make(chan struct{}),
		dead:     make(chan struct{}),
		byPeer:   map[string]int{},
		logf:     func(string, ...any) {},
		lampSkip: map[string]bool{},
	}
	c.gate = newCohortGate(n, nil)
	nopLog := func(sender.Level, string, ...any) {}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("AA:BB:CC:DD:EE:%02d", i+1)
		m := device.Device{ID: id, Name: id, Host: fmt.Sprintf("10.0.0.%d", i+1), Port: 7000, Auth: device.AuthHAPTransient}
		s := sender.New(fakeIO{}, c.memberEvents(i), nopLog)
		member := &cohortMember{
			index: i, device: m, host: raopio.NewHost(), sender: s, ring: ring.New(48000),
			setLampArtwork: func([]byte, string, bool) {},
		}
		c.members = append(c.members, member)
		c.byPeer[m.Host] = i
	}
	return c
}

// startMembers starts every member sender so Stop and state folds have live
// state to work with.
func startMembers(c *cohortTransport) {
	for _, m := range c.members {
		m.sender.Start(m.device.Host, m.device.Port, m.device.Name)
	}
}

// TestCohortAttributesAUDPReplyToTheMemberThatOwnsTheSocket pins the
// multi-homed routing fix: a receiver that answers UDP from an interface the
// plan did not dial still reaches its own sender in a multi-member cohort.
// Before the socket-ownership stamp, the source IP missed byPeer and the
// datagram dropped once a second receiver joined; the same receiver worked
// alone, because a one-member cohort forwards everything.
func TestCohortAttributesAUDPReplyToTheMemberThatOwnsTheSocket(t *testing.T) {
	c := &cohortTransport{
		events: make(chan raopio.Event, 8), stopCh: make(chan struct{}),
		dead: make(chan struct{}), byPeer: map[string]int{},
		logf: func(string, ...any) {}, lampSkip: map[string]bool{},
	}
	c.gate = newCohortGate(2, nil)
	nopLog := func(sender.Level, string, ...any) {}
	rec := &recordingIO{}
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("AA:BB:CC:DD:EE:%02d", i+1)
		m := device.Device{ID: id, Name: id, Host: fmt.Sprintf("10.0.0.%d", i+1), Port: 7000, Auth: device.AuthHAPTransient}
		var io raopio.IO = rec
		if i == 1 {
			io = fakeIO{}
		}
		s := sender.New(io, c.memberEvents(i), nopLog)
		c.members = append(c.members, &cohortMember{
			index: i, device: m, host: raopio.NewHost(), sender: s, ring: ring.New(48000),
			setLampArtwork: func([]byte, string, bool) {},
		})
		c.byPeer[m.Host] = i
	}
	startMembers(c)

	// A timing request that member 0's receiver asked from a foreign
	// interface. The socket-ownership stamp routes it home; the sender
	// answers over the dialed interface to the requesting socket's port.
	ev := raopio.Event{Kind: raopio.EventUDPDatagram, UDP: raopio.UDPTiming,
		Peer: raopio.Endpoint{IP: "192.168.9.9", Port: 6002}, Data: make([]byte, 32)}
	c.HandleEvent(attributeEvent(c.members[0], ev))

	if len(rec.sends) != 1 {
		t.Fatalf("timing replies = %d, want the datagram routed to member 0's sender", len(rec.sends))
	}
	if rec.sends[0].IP != "10.0.0.1" || rec.sends[0].Port != 6002 {
		t.Fatalf("reply went to %s:%d, want 10.0.0.1:6002 (dialed host, source port)",
			rec.sends[0].IP, rec.sends[0].Port)
	}
}

// TestCohortForwardsReceiverLatency pins the member event wiring: every
// member's sender reports the receiver's latency to the same engine callback.
func TestCohortForwardsReceiverLatency(t *testing.T) {
	c := buildTestCohort(2)
	var got []int
	c.onReceiverLatency = func(ms int) { got = append(got, ms) }

	c.memberEvents(0).ReceiverLatencyChanged(70)
	c.memberEvents(1).ReceiverLatencyChanged(72)
	if len(got) != 2 || got[0] != 70 || got[1] != 72 {
		t.Fatalf("cohort forwarded %v, want [70 72]", got)
	}
}

// TestCohortForwardsPreemption pins the member event wiring, tagged with the
// member that lost the receiver.
func TestCohortForwardsPreemption(t *testing.T) {
	c := buildTestCohort(2)
	var got []string
	c.onPreempted = func(reason string) { got = append(got, reason) }

	ev := c.memberEvents(1)
	if ev.Preempted == nil {
		t.Fatal("member events do not forward Preempted")
	}
	ev.Preempted("receiver closed the session")
	if len(got) != 1 || !strings.Contains(got[0], "receiver closed the session") || !strings.Contains(got[0], "AA:BB:CC:DD:EE:02") {
		t.Fatalf("cohort forwarded %v, want the member and the reason", got)
	}
}

// recordingIO counts the datagrams a sender sends and records their
// destinations, so a routing test can see that its event reached a member.
type recordingIO struct {
	fakeIO
	udpSends int
	sends    []raopio.Endpoint
}

func (r *recordingIO) UDPSend(_ raopio.UDPChannel, to raopio.Endpoint, _ []byte) error {
	r.udpSends++
	r.sends = append(r.sends, to)
	return nil
}

// TestCohortPauseRequiresEveryMember pins the all-or-nothing pause: a mixed
// AP1/AP2 cohort refuses the request before touching the AP2 member, so no
// member is left paused with nothing to resume it.
func TestCohortPauseRequiresEveryMember(t *testing.T) {
	c := buildTestCohort(2)
	c.members[1].sender.SetAuth(device.AuthHAPTransient, true, "AA:BB:CC:DD:EE:02", "", "")
	if c.members[0].sender.PauseSupported() {
		t.Fatal("precondition: the first member must be AirPlay 1")
	}
	if c.SetPaused(true) {
		t.Fatal("a mixed AP1/AP2 cohort applied the pause")
	}
	if c.members[1].sender.PauseRequested() {
		t.Fatal("the AP2 member recorded a pause the cohort refused")
	}
}

// TestCohortInputRateReachesTheGate pins the call site: a capture-rate update
// between two arrivals reaches the shared gate instead of failing the second
// member's arrival.
func TestCohortInputRateReachesTheGate(t *testing.T) {
	c := buildTestCohort(2)
	now := time.Now()
	if err := c.gate.Arrive(now, 48000); err != nil {
		t.Fatalf("first arrive: %v", err)
	}
	c.SetInputFormat(44100)
	if err := c.gate.Arrive(now.Add(time.Millisecond), 44100); err != nil {
		t.Fatalf("rate update between arrivals failed the next member: %v", err)
	}
	if !c.gate.Sealed() {
		t.Fatal("the cohort gate did not seal after the rate update")
	}
}

// TestCohortSingleMemberRoutesUnknownPeer pins the permissive single-member
// fallback: one receiver's replies can carry a local address that differs from
// the dialed one, and there is no other member the event could belong to.
func TestCohortSingleMemberRoutesUnknownPeer(t *testing.T) {
	io := &recordingIO{}
	c := buildTestCohort(1)
	c.members[0].sender = sender.New(io, c.memberEvents(0), func(sender.Level, string, ...any) {})
	startMembers(c)

	c.HandleEvent(raopio.Event{
		Kind: raopio.EventUDPDatagram,
		UDP:  raopio.UDPTiming,
		Peer: raopio.Endpoint{IP: "203.0.113.9", Port: 6000},
		Data: make([]byte, 32),
	})
	if io.udpSends == 0 {
		t.Fatal("the only member did not receive an event from an unknown peer")
	}
}

// TestCohortStopStaysOnEngineGoroutine pins the teardown ownership: a member
// death signals the engine, and the sender teardown runs on the same
// goroutine that drives the senders — never on one spawned inside a member
// callback. Run with -race: an in-callback stop races this exact loop.
func TestCohortStopStaysOnEngineGoroutine(t *testing.T) {
	c := buildTestCohort(2)
	startMembers(c)
	c.launchReported = true

	done := make(chan struct{})
	go func() { // the engine loop stand-in: the only caller of sender methods
		defer close(done)
		for {
			select {
			case <-c.dead:
				c.Stop() // closeSession runs here, outside HandleEvent
				return
			default:
			}
			c.Tick()
			c.NextDeadline()
			_ = c.State()
			time.Sleep(time.Millisecond)
		}
	}()
	time.Sleep(20 * time.Millisecond) // let the loop spin against the upcoming stop
	c.memberClosed(0)                 // the sender's Closed callback, engine goroutine

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("engine loop never saw the dead signal")
	}
	if st := c.State(); st != sender.StateIdle {
		t.Fatalf("state = %v after stop, want idle", st)
	}
}

// TestCohortMemberDeathSignalsEngineOnce pins the one-shot semantics: a
// failed launch reports once and signals once; a later member close adds
// nothing.
func TestCohortMemberDeathSignalsEngineOnce(t *testing.T) {
	c := buildTestCohort(2)
	startMembers(c)
	launched := 0
	closed := 0
	c.onLaunched = func(ok bool, err string) { launched++ }
	c.onClosed = func() { closed++ }

	c.memberLaunched(0, false, "boom")
	c.memberLaunched(0, false, "boom again") // suppressed: already reported
	c.memberClosed(1)                        // suppressed: the failed flag is set

	select {
	case <-c.dead:
	default:
		t.Fatal("member death did not signal the engine")
	}
	if launched != 1 || closed != 0 {
		t.Fatalf("launched = %d, closed = %d; want one launch report, no close", launched, closed)
	}
	c.signalDead() // idempotent
}

// TestCohortMemberCloseBeforeLaunchSignalsEngine pins the early-death path: a
// member that closes before the cohort has reported a launch must still hand
// the teardown to the engine, or the half-dead cohort never reconnects.
func TestCohortMemberCloseBeforeLaunchSignalsEngine(t *testing.T) {
	c := buildTestCohort(2)
	c.members[0].targetKey = "AA:1"
	closed := 0
	c.onClosed = func() { closed++ }

	c.memberClosed(0)
	if got := c.LostTarget(); got != "AA:1" {
		t.Fatalf("lost target = %q, want the closed member's target", got)
	}

	select {
	case <-c.dead:
	default:
		t.Fatal("member close before launch did not signal the engine")
	}
	if closed != 1 {
		t.Fatalf("closed reports = %d, want 1", closed)
	}
}

// TestCohortArtworkFansOutToCapableMembers pins the per-member capability
// filter: every capable member gets the light, the rest are skipped.
func TestCohortArtworkFansOutToCapableMembers(t *testing.T) {
	c := buildTestCohort(2)
	for _, m := range c.members {
		m.device.Model = "AudioAccessory6,1"
		m.device.OSVersion = "27.2"
	}
	var got []string
	for _, m := range c.members {
		member := m
		m.setLampArtwork = func(data []byte, mime string, on bool) {
			got = append(got, string(data)+"\x00"+mime+"\x00"+member.device.Name)
		}
	}

	c.SetLampArtwork([]byte("light"), "image/jpeg", true)

	if len(got) != 2 {
		t.Fatalf("members sent = %d, want 2: %q", len(got), got)
	}
}

func TestCohortArtworkSkipsIncapableMember(t *testing.T) {
	c := buildTestCohort(2)
	c.members[0].device.Model = "AudioAccessory6,1"
	c.members[0].device.OSVersion = "27.2"
	c.members[1].device.Model = "AudioAccessory6,1"
	c.members[1].device.OSVersion = "27.1"
	var got []string
	for _, m := range c.members {
		member := m
		m.setLampArtwork = func(data []byte, mime string, on bool) {
			got = append(got, member.device.Name)
		}
	}

	c.SetLampArtwork([]byte("light"), "image/jpeg", true)

	if len(got) != 1 || got[0] != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("members sent = %q, want only the capable member", got)
	}
	// A second call logs nothing new but still sends to the capable member.
	before := len(c.lampSkip)
	c.SetLampArtwork([]byte("light"), "image/jpeg", true)
	if len(c.lampSkip) != before {
		t.Fatalf("skip log state changed on a repeat call: %v", c.lampSkip)
	}
}

// cohortPlan builds an NTP-only plan without touching the registry or PTP
// sockets, for the transport construction tests.
func cohortPlan(n int) sessionPlan {
	p := sessionPlan{}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("AA:BB:CC:DD:EE:%02d", i+1)
		d := device.Device{
			ID: id, Name: id, Host: fmt.Sprintf("10.0.0.%d", i+1), Port: 7000,
			Model: "AppleTV6,2", Auth: device.AuthHAPPIN, AirPlay2: true,
		}
		p.targets = append(p.targets, plannedTarget{key: id, name: id, first: i, count: 1})
		p.members = append(p.members, plannedMember{key: id, device: d, auth: memberAuth{method: d.Auth, deviceID: id, family: d.Family}})
	}
	return p
}

// TestCohortEveryMemberRingGetsTheSameSamples pins the capture fanout: one
// push reaches every member ring with the same samples, and Rings() reports
// one buffer per member.
func TestCohortEveryMemberRingGetsTheSameSamples(t *testing.T) {
	h := newHarness(t)
	c, err := h.e.openCohort(cohortPlan(3))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	rings := c.Rings()
	if len(rings) != 3 {
		t.Fatalf("rings = %d, want one per member", len(rings))
	}
	for _, r := range rings {
		h.e.captureSink.Add(r)
	}
	samples := []int16{1, 2, 3, 4, 5, 6}
	h.e.captureSink.Push(samples)
	for i, r := range rings {
		got := make([]int16, r.Available())
		n := r.Pop(got)
		if n != len(samples) {
			t.Fatalf("ring %d read %d samples, want %d", i, n, len(samples))
		}
		for j := range samples {
			if got[j] != samples[j] {
				t.Fatalf("ring %d sample %d = %d, want %d", i, j, got[j], samples[j])
			}
		}
	}
}

// TestCohortSingleReceiverIsCohortOfOne pins the one-path rule: a single
// target builds one member ring and a gate that seals on its one arrival.
func TestCohortSingleReceiverIsCohortOfOne(t *testing.T) {
	h := newHarness(t)
	tr, err := h.e.openCohort(cohortPlan(1))
	if err != nil {
		t.Fatal(err)
	}
	c := tr.(*cohortTransport)
	defer c.Stop()
	if got := len(c.Rings()); got != 1 {
		t.Fatalf("rings = %d, want 1", got)
	}
	if err := c.gate.Arrive(time.Now(), 48000); err != nil {
		t.Fatal(err)
	}
	if !c.gate.Sealed() {
		t.Fatal("a one-member gate did not seal on its first arrival")
	}
}

// TestCohortGateWaitsForEveryMember pins N-1 arrivals: the cohort's shared
// gate does not seal until the last member arrives, then one trim covers the
// whole fanout.
func TestCohortGateWaitsForEveryMember(t *testing.T) {
	h := newHarness(t)
	tr, err := h.e.openCohort(cohortPlan(3))
	if err != nil {
		t.Fatal(err)
	}
	c := tr.(*cohortTransport)
	defer c.Stop()
	for _, r := range c.Rings() {
		h.e.captureSink.Add(r)
	}
	now := time.Now()
	for i := 0; i < 2; i++ {
		if err := c.gate.Arrive(now, 48000); err != nil {
			t.Fatal(err)
		}
	}
	if c.gate.Sealed() {
		t.Fatal("the gate sealed before every member arrived")
	}
	if epoch, ok := c.gate.Begin(now); ok {
		t.Fatalf("playback began at %v with a member still handshaking", epoch)
	}
	if err := c.gate.Arrive(now, 48000); err != nil {
		t.Fatal(err)
	}
	if !c.gate.Sealed() {
		t.Fatal("the last arrival did not seal the gate")
	}
	if _, ok := c.gate.Begin(now); !ok {
		t.Fatal("the cohort did not begin after sealing")
	}
}
