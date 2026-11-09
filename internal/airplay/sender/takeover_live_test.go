package sender

import (
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/ptp"
	"saundo/internal/audio/ring"
	"saundo/internal/discovery"
)

// TestLiveTakeoverProbe runs two concurrent senders against one receiver and
// logs what the losing session receives. The full app is single-instance, so
// this reproduces the two-machine case at the protocol layer with two sender
// sessions in one process. Both are muted and carry an empty ring.
//
// SAUNDO_TAKEOVER_TEST=1 enables it. Stop the saundo app first so PTP can bind
// UDP 319/320.
func TestLiveTakeoverProbe(t *testing.T) {
	if os.Getenv("SAUNDO_TAKEOVER_TEST") == "" {
		t.Skip("set SAUNDO_TAKEOVER_TEST=1 to run the takeover probe")
	}
	if testing.Short() {
		t.Skip("live device test")
	}

	target := discoverTakeoverTarget(t)
	t.Logf("target: %s (%s) at %s:%d model=%s group=%q", target.Name, target.ID, target.Host, target.Port, target.Model, target.GroupName)

	clock, err := ptp.New(target.Host)
	if err != nil {
		t.Fatal(err)
	}
	clock.Start()
	defer clock.Stop()

	logf := func(tag, format string, args ...any) {
		t.Logf("%s %s", tag, fmt.Sprintf(format, args...))
	}
	eventBodyHook = func(firstLine, contentType string, body []byte) {
		logf("EVENT", "%q ct=%q body=%s", firstLine, contentType, summarizeEventBody(body))
	}
	defer func() { eventBodyHook = nil }()

	a := newTakeoverProbe(target, clock, "A", "02:00:00:00:00:11", logf)
	defer a.teardown()
	b := newTakeoverProbe(target, clock, "B", "02:00:00:00:00:12", logf)
	defer b.teardown()

	a.start(target)
	if !a.waitLaunched(t, 30*time.Second) {
		t.Fatalf("A did not reach streaming: %s", a.lastErr)
	}
	logf("PROBE", "A is streaming, starting B")
	time.Sleep(2 * time.Second)

	b.start(target)
	pumpTakeover(t, a, b, 15*time.Second, logf)

	for round := 1; round <= 2; round++ {
		if a.closed {
			logf("PROBE", "round %d: A closed, restarting after 2s", round)
			time.Sleep(2 * time.Second)
			a.restart(target)
		}
		if b.closed {
			logf("PROBE", "round %d: B closed, restarting after 2s", round)
			time.Sleep(2 * time.Second)
			b.restart(target)
		}
		pumpTakeover(t, a, b, 15*time.Second, logf)
	}
}

type takeoverProbe struct {
	tag      string
	host     *raopio.Host
	sender   *Sender
	launched bool
	closed   bool
	lastErr  string
}

func newTakeoverProbe(target device.Device, clock *ptp.Follower, tag, deviceID string, logf func(string, string, ...any)) *takeoverProbe {
	p := &takeoverProbe{tag: tag, host: raopio.NewHost()}
	p.sender = New(p.host, Events{
		Launched: func(ok bool, err string) {
			if ok {
				p.launched = true
			}
			p.lastErr = err
			logf(tag, "launched=%v err=%q", ok, err)
		},
		Closed:       func() { p.closed = true; logf(tag, "closed") },
		Preempted:    func(reason string) { logf(tag, "preempted=%q", reason) },
		PinRequired:  func(name string) { logf(tag, "pin required for %s", name) },
		StateChanged: func(state State) { logf(tag, "state=%s", state) },
	}, func(level Level, format string, args ...any) {
		logf(tag+"-log", format, args...)
	})
	p.sender.SetIdentity(Identity{Name: "saundo-probe-" + tag, DeviceID: deviceID, Model: "iPhone14,3"})
	p.sender.SetAuth(target.Auth, target.AirPlay2, target.ID, "", "")
	p.sender.SetFamily(target.Family)
	p.sender.SetPTPClock(clock)
	p.sender.AttachRing(ring.New(44100 * 2 * 2))
	p.sender.SetVolume(0)
	return p
}

func (p *takeoverProbe) start(target device.Device) {
	p.launched = false
	p.closed = false
	p.lastErr = ""
	p.sender.Start(target.Host, target.Port, target.Name)
}

func (p *takeoverProbe) restart(target device.Device) {
	p.sender.Stop()
	p.start(target)
}

func (p *takeoverProbe) teardown() {
	p.sender.Stop()
	p.host.Close()
}

func (p *takeoverProbe) waitLaunched(t *testing.T, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for time.Now().Before(deadline) {
		select {
		case ev := <-p.host.Events():
			p.sender.HandleEvent(ev)
		case <-tick.C:
			p.sender.Tick()
		}
		if p.closed {
			return false
		}
		if p.launched {
			return true
		}
	}
	return false
}

func pumpTakeover(t *testing.T, a, b *takeoverProbe, dur time.Duration, logf func(string, string, ...any)) {
	t.Helper()
	deadline := time.Now().Add(dur)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for time.Now().Before(deadline) {
		select {
		case ev := <-a.host.Events():
			a.sender.HandleEvent(ev)
		case ev := <-b.host.Events():
			b.sender.HandleEvent(ev)
		case <-tick.C:
		}
		a.sender.Tick()
		b.sender.Tick()
	}
	logf("PROBE", "pump done: A launched=%v closed=%v state=%s err=%q; B launched=%v closed=%v state=%s err=%q",
		a.launched, a.closed, a.sender.State(), a.lastErr,
		b.launched, b.closed, b.sender.State(), b.lastErr)
}

func discoverTakeoverTarget(t *testing.T) device.Device {
	t.Helper()
	disc := discovery.New(2*time.Second, 2*time.Second)
	disc.Start()
	defer disc.Stop()
	deadline := time.After(12 * time.Second)
	var fallback *device.Device
	for {
		select {
		case devs := <-disc.Updates():
			for i := range devs {
				d := devs[i]
				if d.Auth != device.AuthHAPTransient || !d.AirPlay2 || d.Host == "" {
					continue
				}
				if d.GroupName == "" {
					return d
				}
				if fallback == nil {
					copy := d
					fallback = &copy
				}
			}
			if fallback != nil {
				return *fallback
			}
		case <-deadline:
			t.Skip("no transient AirPlay 2 receiver found")
		}
	}
}

func summarizeEventBody(body []byte) string {
	if len(body) == 0 {
		return "<empty>"
	}
	root, err := crypto.Decode(body)
	if err != nil {
		if len(body) > 64 {
			body = body[:64]
		}
		return "raw:" + hex.EncodeToString(body)
	}
	s := fmt.Sprintf("%v", root)
	if len(s) > 512 {
		s = s[:512] + "..."
	}
	return s
}
