package engine

import (
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

// buildGroup wires a two-member groupTransport the way openGroupSession
// does, over a fake io and real hosts so Stop has something to close.
func buildGroup() *groupTransport {
	g := &groupTransport{
		group: device.Device{
			ID: "stereo:AA:BB:CC:DD:EE:01", Name: "Pair",
			Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient,
		},
		events: make(chan raopio.Event, 8),
		stopCh: make(chan struct{}),
		dead:   make(chan struct{}),
		logf:   func(string, ...any) {},
	}
	nopLog := func(sender.Level, string, ...any) {}
	for i, id := range []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02"} {
		idx := i
		m := device.Device{ID: id, Name: id, Host: "10.0.0.1", Port: 7000, Auth: device.AuthHAPTransient}
		s := sender.New(fakeIO{}, sender.Events{
			Closed:       func() { g.memberClosed(idx) },
			Launched:     func(ok bool, err string) { g.memberLaunched(idx, ok, err) },
			StateChanged: func(sender.State) { g.memberStateChanged(idx, sender.StateIdle) },
		}, nopLog)
		g.members = append(g.members, &groupMember{
			device: m, host: raopio.NewHost(), sender: s, ring: ring.New(48000),
			setArtwork: func([]byte, string) {},
		})
	}
	return g
}

// TestGroupStopStaysOnEngineGoroutine pins the teardown ownership: a member
// death signals the engine, and the sender teardown runs on the same
// goroutine that drives the senders — never on one spawned inside a member
// callback. Run with -race: the old `go g.Stop()` raced this exact loop.
func TestGroupStopStaysOnEngineGoroutine(t *testing.T) {
	g := buildGroup()
	for _, m := range g.members {
		m.sender.Start(m.device.Host, m.device.Port, m.device.Name)
	}
	g.launchReported = true

	done := make(chan struct{})
	go func() { // the engine loop stand-in: the only caller of sender methods
		defer close(done)
		for {
			select {
			case <-g.dead:
				g.Stop() // closeSession runs here, outside HandleEvent
				return
			default:
			}
			g.Tick()
			g.NextDeadline()
			_ = g.State()
			time.Sleep(time.Millisecond)
		}
	}()
	time.Sleep(20 * time.Millisecond) // let the loop spin against the upcoming stop
	g.memberClosed(0)                 // the sender's Closed callback, engine goroutine

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("engine loop never saw the dead signal")
	}
	if st := g.State(); st != sender.StateIdle {
		t.Fatalf("state = %v after stop, want idle", st)
	}
}

// TestGroupMemberDeathSignalsEngineOnce pins the one-shot semantics: a
// failed launch reports once and signals once; a later member close adds
// nothing.
func TestGroupMemberDeathSignalsEngineOnce(t *testing.T) {
	g := buildGroup()
	launched := 0
	closed := 0
	g.onLaunched = func(ok bool, err string) { launched++ }
	g.onClosed = func() { closed++ }

	g.memberLaunched(0, false, "boom")
	g.memberLaunched(0, false, "boom again") // suppressed: already reported
	g.memberClosed(1)                        // suppressed: the failed flag is set

	select {
	case <-g.dead:
	default:
		t.Fatal("member death did not signal the engine")
	}
	if launched != 1 || closed != 0 {
		t.Fatalf("launched = %d, closed = %d; want one launch report, no close", launched, closed)
	}
	g.signalDead() // idempotent
}

// TestGroupMemberCloseBeforeLaunchSignalsEngine pins the early-death path: a
// member that closes before the cohort has reported a launch must still hand
// the teardown to the engine, or the half-dead pair never reconnects.
func TestGroupMemberCloseBeforeLaunchSignalsEngine(t *testing.T) {
	g := buildGroup()
	closed := 0
	g.onClosed = func() { closed++ }

	g.memberClosed(0)

	select {
	case <-g.dead:
	default:
		t.Fatal("member close before launch did not signal the engine")
	}
	if closed != 1 {
		t.Fatalf("closed reports = %d, want 1", closed)
	}
}

func TestGroupArtworkFansOutToBothCapableMembers(t *testing.T) {
	g := buildGroup()
	for _, m := range g.members {
		m.device.Model = "AudioAccessory6,1"
		m.device.OSVersion = "27.2"
	}
	var got []string
	for i, m := range g.members {
		idx := i
		m.setArtwork = func(data []byte, mime string) {
			got = append(got, string(data)+"\x00"+mime+"\x00"+g.members[idx].device.Name)
		}
	}

	g.SetArtwork([]byte("light"), "image/jpeg")

	if len(got) != 2 {
		t.Fatalf("members sent = %d, want 2: %q", len(got), got)
	}
	for _, want := range []string{"light\x00image/jpeg\x00AA:BB:CC:DD:EE:01", "light\x00image/jpeg\x00AA:BB:CC:DD:EE:02"} {
		found := false
		for _, s := range got {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestGroupArtworkSkipsIncapableMember(t *testing.T) {
	g := buildGroup()
	g.members[0].device.Model = "AudioAccessory6,1"
	g.members[0].device.OSVersion = "27.2"
	g.members[1].device.Model = "AudioAccessory6,1"
	g.members[1].device.OSVersion = "27.1"
	var got []string
	for _, m := range g.members {
		member := m
		m.setArtwork = func(data []byte, mime string) {
			got = append(got, member.device.Name)
		}
	}

	g.SetArtwork([]byte("light"), "image/jpeg")

	if len(got) != 1 || got[0] != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("members sent = %q, want only the capable member", got)
	}
}
