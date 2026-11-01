package sender

import (
	"os"
	"strconv"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/ptp"
	"saundo/internal/audio/ring"
	"saundo/internal/discovery"
)

// TestLiveSoak streams silence to a real receiver for 30 minutes and asserts
// the session, the PTP clock, and the pacer survive the whole run.
//
// SAUNDO_SOAK=1 enables it; SAUNDO_SOAK_MINUTES overrides the length.
// It binds UDP 319/320, so no other PTP session may run at the same time.
func TestLiveSoak(t *testing.T) {
	if os.Getenv("SAUNDO_SOAK") == "" {
		t.Skip("set SAUNDO_SOAK=1 to run the 30-minute soak")
	}
	if testing.Short() {
		t.Skip("live soak")
	}
	minutes := 30
	if v := os.Getenv("SAUNDO_SOAK_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minutes = n
		}
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
	t.Logf("soak target: %s (%s) at %s:%d", target.Name, target.ID, target.Host, target.Port)

	host := raopio.NewHost()
	defer host.Close()
	clock, err := ptp.New(target.Host)
	if err != nil {
		t.Fatal(err)
	}
	clock.Start()
	defer clock.Stop()

	var closed bool
	var launchedAt time.Time
	var lastErr string
	s := New(host, Events{
		Launched: func(ok bool, err string) {
			if ok {
				launchedAt = time.Now()
			}
			lastErr = err
		},
		Closed:      func() { closed = true },
		PinRequired: func(string) { t.Log("unexpected PIN request") },
		StateChanged: func(state State) {
			t.Logf("state: %s", state)
		},
	}, func(level Level, format string, args ...any) {
		t.Logf("[%d] "+format, append([]any{level}, args...)...)
	})
	s.SetIdentity(Identity{Name: "saundo-soak", DeviceID: "02:00:00:00:00:02", Model: "iPhone14,3"})
	s.SetAuth(target.Auth, true, target.ID, "", "")
	s.SetPTPClock(clock)
	rb := ring.New(44100 * 2 * 2)
	s.AttachRing(rb) // stays empty: the soak streams silence
	s.Start(target.Host, target.Port, target.Name)
	s.SetVolume(0) // mute: the soak must stay inaudible

	hold := time.Duration(minutes) * time.Minute
	startDeadline := time.Now().Add(30 * time.Second)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	var lastProgress time.Time
	var lastFrames uint64
	stalled := 0
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
			t.Fatalf("receiver closed the session during soak: %s", lastErr)
		}
		if launchedAt.IsZero() && time.Now().After(startDeadline) {
			t.Fatalf("session did not reach streaming; state=%s err=%s ptp=%+v", s.State(), lastErr, clock.Stats())
		}
		if !launchedAt.IsZero() {
			if s.State() != StateStreaming {
				t.Fatalf("state regressed to %s during soak", s.State())
			}
			if s.framesSent == lastFrames {
				stalled++
				if stalled >= 4 {
					t.Fatalf("no frames for 8 s at frame %d", lastFrames)
				}
			} else {
				stalled = 0
				lastFrames = s.framesSent
			}
			if time.Since(launchedAt) >= hold {
				break
			}
		}
		if time.Since(lastProgress) > time.Minute {
			lastProgress = time.Now()
			t.Logf("soak: frames=%d ringDropped=%d ptp=%+v", s.framesSent, rb.Dropped(), clock.Stats())
		}
	}

	if rb.Dropped() != 0 {
		t.Fatalf("ring dropped %d samples during soak", rb.Dropped())
	}
	if !clock.Ready() {
		t.Fatalf("PTP clock not ready at the end: %+v", clock.Stats())
	}
	t.Logf("soak ok: %s elapsed, frames=%d, ptp=%+v",
		time.Since(launchedAt).Round(time.Second), s.framesSent, clock.Stats())
	s.Stop()
}
