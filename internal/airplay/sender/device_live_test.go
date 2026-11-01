package sender

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/ptp"
	"saundo/internal/audio/ring"
	"saundo/internal/discovery"
)

// TestLiveHomePodHandshake runs the complete AP2 session against a receiver on
// the LAN, with an empty ring and the receiver muted. It proves pairing, the
// encrypted control channel, session and stream SETUP, RECORD, the event
// channel, and the two-second feedback keep-alive on real hardware without
// playing audio.
//
// SAUNDO_DEVICE_TEST=1 enables it. It needs a LAN with an AirPlay 2
// receiver and Windows Firewall permission for inbound UDP timing.
func TestLiveHomePodHandshake(t *testing.T) {
	if os.Getenv("SAUNDO_DEVICE_TEST") == "" {
		t.Skip("set SAUNDO_DEVICE_TEST=1 to run against a real receiver")
	}
	if testing.Short() {
		t.Skip("live device test")
	}

	disc := discovery.New(2*time.Second, 2*time.Second)
	disc.Start()
	defer disc.Stop()
	var target device.Device
	deadline := time.After(12 * time.Second)
wait:
	for {
		select {
		case devs := <-disc.Updates():
			for _, d := range devs {
				if d.Auth == device.AuthHAPTransient && d.AirPlay2 {
					target = d
					break wait
				}
			}
		case <-deadline:
			t.Skip("no transient AirPlay 2 receiver found")
		}
	}
	t.Logf("target: %s (%s) at %s:%d model=%s", target.Name, target.ID, target.Host, target.Port, target.Model)

	host := raopio.NewHost()
	defer host.Close()

	var launched bool
	var launchedAt time.Time
	var lastErr string
	var closed bool
	s := New(host, Events{
		Launched: func(ok bool, err string) {
			launched = ok
			lastErr = err
			if ok {
				launchedAt = time.Now()
			}
		},
		Closed:      func() { closed = true },
		PinRequired: func(name string) { t.Logf("unexpected PIN request from %s", name) },
		StateChanged: func(state State) {
			t.Logf("state: %s", state)
		},
	}, func(level Level, format string, args ...any) {
		t.Logf("[%d] "+format, append([]any{level}, args...)...)
	})
	clock, err := ptp.New(target.Host)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	clock.OnPacket = func(kind uint8, seq uint16, data []byte) {
		if seen >= 12 {
			return
		}
		seen++
		var secs [8]byte
		copy(secs[2:], data[34:40])
		ts := binary.BigEndian.Uint64(secs[:])*1_000_000_000 +
			uint64(binary.BigEndian.Uint32(data[40:44]))
		t.Logf("ptp kind=%d seq=%d len=%d flags=%04x ts=%d bytes34_44=%s",
			kind, seq, len(data), binary.BigEndian.Uint16(data[6:8]), ts,
			hex.EncodeToString(data[34:44]))
	}
	clock.Start()
	defer clock.Stop()
	s.SetIdentity(Identity{Name: "saundo-test", DeviceID: "02:00:00:00:00:01", Model: "iPhone14,3"})
	s.SetAuth(target.Auth, true, target.ID, "", "")
	s.SetPTPClock(clock)

	tone := os.Getenv("SAUNDO_DEVICE_TONE") != ""
	rb := ring.New(44100 * 2 * 2)
	if tone {
		samples := make([]int16, 44100*2*2)
		for i := 0; i < 44100*2; i++ {
			v := int16(12000 * math.Sin(2*math.Pi*440*float64(i)/44100))
			samples[i*2], samples[i*2+1] = v, v
		}
		rb.Push(samples)
	}
	s.AttachRing(rb)
	s.Start(target.Host, target.Port, target.Name)
	if tone {
		s.SetVolume(50)
		t.Log("playing a quiet 440 Hz tone at 50% for four seconds")
	} else {
		s.SetVolume(0) // mute; the ring is empty, so no audio plays
	}

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	lastStats := time.Now()
	hold := 5 * time.Second
	if tone {
		hold = 4 * time.Second
	}
	if os.Getenv("SAUNDO_DEVICE_LONG") != "" {
		hold = 35 * time.Second
	}
	stopAt := time.Now().Add(hold + 15*time.Second)
	var restoreAt time.Time
	for {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var timerC <-chan time.Time
		if dl := s.NextDeadline(); !dl.IsZero() {
			d := time.Until(dl)
			if d < 0 {
				d = 0
			}
			timer.Reset(d)
			timerC = timer.C
		}
		select {
		case ev := <-host.Events():
			s.HandleEvent(ev)
		case <-timerC:
			s.Tick()
		case <-tick.C:
		}
		if closed {
			t.Fatalf("receiver closed the session before verification: %s", lastErr)
		}
		if time.Since(lastStats) > time.Second {
			t.Logf("ptp: %+v", clock.Stats())
			lastStats = time.Now()
		}
		if launched && time.Since(launchedAt) > hold {
			if restoreAt.IsZero() {
				pct, ok := s.InitialVolumePercent()
				if !ok {
					pct = 10
				}
				s.SetVolume(pct)
				t.Logf("restored receiver volume to %.0f%%", pct)
				restoreAt = time.Now()
			} else if time.Since(restoreAt) > 500*time.Millisecond {
				break
			}
		}
		if time.Now().After(stopAt) {
			t.Fatalf("session did not reach streaming; state=%s err=%s ptp=%+v", s.State(), lastErr, clock.Stats())
		}
	}

	if !launched {
		t.Fatalf("session did not launch: %s", lastErr)
	}
	if s.State() != StateStreaming {
		t.Fatalf("state after five seconds = %s", s.State())
	}
	if !s.controlEncrypted {
		t.Fatal("control channel was not encrypted")
	}
	if s.ap2 == nil || s.ap2.eventPort == 0 {
		t.Fatal("no event channel was opened")
	}
	if s.serverPort == 0 || s.framesSent == 0 {
		t.Fatalf("no stream: dataPort=%d framesSent=%d", s.serverPort, s.framesSent)
	}
	t.Logf("live session ok: eventPort=%d dataPort=%d framesSent=%d encrypted=%v ptp=%+v",
		s.ap2.eventPort, s.serverPort, s.framesSent, s.controlEncrypted, clock.Stats())
	s.Stop()
}

// liveSession is one real-device session with its own host and PTP clock.
type liveSession struct {
	s          *Sender
	host       *raopio.Host
	clock      *ptp.Follower
	launched   bool
	launchedAt time.Time
	closed     bool
	lastErr    string
}

func startLiveSession(t *testing.T, target device.Device) *liveSession {
	t.Helper()
	ls := &liveSession{host: raopio.NewHost()}
	clock, err := ptp.New(target.Host)
	if err != nil {
		ls.host.Close()
		t.Fatal(err)
	}
	ls.clock = clock
	ls.s = New(ls.host, Events{
		Launched: func(ok bool, err string) {
			if ok {
				ls.launched, ls.launchedAt = true, time.Now()
			}
			ls.lastErr = err
		},
		Closed:       func() { ls.closed = true },
		PinRequired:  func(string) {},
		StateChanged: func(State) {},
	}, func(Level, string, ...any) {})
	ls.s.SetIdentity(Identity{Name: "saundo-test", DeviceID: "02:00:00:00:00:03", Model: "iPhone14,3"})
	ls.s.SetAuth(target.Auth, true, target.ID, "", "")
	ls.s.SetPTPClock(clock)
	ls.s.AttachRing(ring.New(44100 * 2 * 2)) // silence only
	ls.clock.Start()
	ls.s.SetVolume(0)
	ls.s.Start(target.Host, target.Port, target.Name)
	return ls
}

func (ls *liveSession) teardown() {
	ls.s.Stop()
	ls.clock.Stop()
	ls.host.Close()
}

func (ls *liveSession) pumpUntilLaunched(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for time.Now().Before(deadline) {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var timerC <-chan time.Time
		if dl := ls.s.NextDeadline(); !dl.IsZero() {
			d := time.Until(dl)
			if d < 0 {
				d = 0
			}
			timer.Reset(d)
			timerC = timer.C
		}
		select {
		case ev := <-ls.host.Events():
			ls.s.HandleEvent(ev)
		case <-timerC:
			ls.s.Tick()
		case <-time.After(20 * time.Millisecond):
		}
		if ls.closed {
			return false
		}
		if ls.launched {
			return true
		}
	}
	return false
}

// TestLiveColdStartTiming measures how fast a fresh session reaches streaming
// right after a clean TEARDOWN, which is the reconnect path users feel.
// SAUNDO_DEVICE_TEST=1 enables it; it needs UDP 319/320 free.
func TestLiveColdStartTiming(t *testing.T) {
	if os.Getenv("SAUNDO_DEVICE_TEST") == "" {
		t.Skip("set SAUNDO_DEVICE_TEST=1 to run against a real receiver")
	}
	if testing.Short() {
		t.Skip("live device test")
	}
	disc := discovery.New(2*time.Second, 2*time.Second)
	disc.Start()
	defer disc.Stop()
	var target device.Device
	deadline := time.After(12 * time.Second)
wait:
	for {
		select {
		case devs := <-disc.Updates():
			for _, d := range devs {
				if d.Auth == device.AuthHAPTransient && d.AirPlay2 {
					target = d
					break wait
				}
			}
		case <-deadline:
			t.Skip("no transient AirPlay 2 receiver found")
		}
	}

	first := startLiveSession(t, target)
	if !first.pumpUntilLaunched(30 * time.Second) {
		first.teardown()
		t.Fatalf("first session failed: %s", first.lastErr)
	}
	time.Sleep(5 * time.Second)
	first.teardown()
	time.Sleep(10 * time.Second)

	second := startLiveSession(t, target)
	begin := time.Now()
	ok := second.pumpUntilLaunched(30 * time.Second)
	cold := second.launchedAt.Sub(begin)
	second.teardown()
	if !ok {
		t.Fatalf("second session failed: %s", second.lastErr)
	}
	t.Logf("cold start to streaming: %s", cold.Round(time.Millisecond))
	if cold > 15*time.Second {
		t.Fatalf("cold start took %s, want < 15s", cold)
	}
}

// TestLiveAppleTVPinStart runs pair-pin-start and pair-setup M1/M2 against a
// receiver that uses the on-screen PIN path and asserts the sender reaches the
// waiting-for-PIN state. It stops before asking anyone to type the code.
// SAUNDO_DEVICE_TEST=1 enables it.
func TestLiveAppleTVPinStart(t *testing.T) {
	if os.Getenv("SAUNDO_DEVICE_TEST") == "" {
		t.Skip("set SAUNDO_DEVICE_TEST=1 to run against a real receiver")
	}
	if testing.Short() {
		t.Skip("live device test")
	}
	disc := discovery.New(2*time.Second, 2*time.Second)
	disc.Start()
	defer disc.Stop()
	var target device.Device
	deadline := time.After(12 * time.Second)
wait:
	for {
		select {
		case devs := <-disc.Updates():
			for _, d := range devs {
				if d.Auth == device.AuthHAPPIN && d.AirPlay2 {
					target = d
					break wait
				}
			}
		case <-deadline:
			t.Skip("no PIN-pairing AirPlay 2 receiver found")
		}
	}
	t.Logf("pin target: %s (%s) at %s:%d model=%s", target.Name, target.ID, target.Host, target.Port, target.Model)

	host := raopio.NewHost()
	defer host.Close()
	var pinAsked bool
	var pinName string
	var launched bool
	s := New(host, Events{
		Launched:     func(ok bool, err string) { launched = ok },
		Closed:       func() {},
		PinRequired:  func(name string) { pinAsked, pinName = true, name },
		StateChanged: func(State) {},
	}, func(level Level, format string, args ...any) {
		t.Logf("[%d] "+format, append([]any{level}, args...)...)
	})
	s.SetIdentity(Identity{Name: "saundo-test", DeviceID: "02:00:00:00:00:04", Model: "iPhone14,3"})
	s.SetAuth(target.Auth, true, target.ID, "", "")
	s.Start(target.Host, target.Port, target.Name)

	deadline2 := time.After(45 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case ev := <-host.Events():
			s.HandleEvent(ev)
		case <-deadline2:
			s.Stop()
			t.Fatalf("sender never reached the PIN prompt; launched=%v", launched)
		case <-tick.C:
			if pinAsked {
				s.Stop()
				t.Logf("PIN prompt reached from %s; receiver shows a code", pinName)
				return
			}
			if launched {
				s.Stop()
				t.Fatal("unexpectedly launched without a PIN")
			}
		}
	}
}
