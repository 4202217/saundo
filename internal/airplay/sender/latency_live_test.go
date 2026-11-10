package sender

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/ptp"
	"saundo/internal/audio/ring"
	"saundo/internal/discovery"
)

// TestLiveLatencySweep runs a real session at each requested presentation
// latency and records the receiver's own arrival-to-render latency plus the
// sender's pacer, ring, and retransmit counters. It is a probe, not a gate:
// the receiver is muted and the ring is fed continuously, so the numbers are
// about scheduling and network behavior, not audible quality.
//
// SAUNDO_LATENCY_TEST=1 enables it. SAUNDO_LATENCY_TARGET selects a receiver
// by name. SAUNDO_LATENCY_MS is a comma-separated list (default
// 200,150,120,100,90,80). SAUNDO_LATENCY_HOLD is seconds per value (default
// 15). SAUNDO_LATENCY_TONE=1 feeds a quiet tone instead of silence; the
// receiver stays muted either way.
func TestLiveLatencySweep(t *testing.T) {
	if os.Getenv("SAUNDO_LATENCY_TEST") == "" {
		t.Skip("set SAUNDO_LATENCY_TEST=1 to run the real-receiver latency sweep")
	}
	if testing.Short() {
		t.Skip("live latency sweep")
	}
	target := latencyTarget(t, os.Getenv("SAUNDO_LATENCY_TARGET"))
	t.Logf("target: %s (%s) at %s:%d model=%s", target.Name, target.ID, target.Host, target.Port, target.Model)

	values := []int{200, 150, 120, 100, 90, 80}
	if raw := os.Getenv("SAUNDO_LATENCY_MS"); raw != "" {
		values = values[:0]
		for _, part := range strings.Split(raw, ",") {
			ms, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || ms <= 0 {
				t.Fatalf("bad SAUNDO_LATENCY_MS entry %q", part)
			}
			values = append(values, ms)
		}
	}
	hold := 15 * time.Second
	if raw := os.Getenv("SAUNDO_LATENCY_HOLD"); raw != "" {
		secs, err := strconv.Atoi(raw)
		if err != nil || secs <= 0 {
			t.Fatalf("bad SAUNDO_LATENCY_HOLD %q", raw)
		}
		hold = time.Duration(secs) * time.Second
	}
	tone := os.Getenv("SAUNDO_LATENCY_TONE") != ""

	for _, ms := range values {
		ms := ms
		t.Run(fmt.Sprintf("%dms", ms), func(t *testing.T) {
			runLatencySession(t, target, ms, hold, tone)
		})
		// Let the receiver release the session and its PTP peer state.
		time.Sleep(6 * time.Second)
	}
}

func latencyTarget(t *testing.T, want string) device.Device {
	t.Helper()
	disc := discovery.New(2*time.Second, 2*time.Second)
	disc.Start()
	defer disc.Stop()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case devs := <-disc.Updates():
			for _, d := range devs {
				// Discovery collapses a stereo pair into one synthetic
				// group; the individual members live in Members.
				if d.IsStereoGroup() {
					t.Logf("group %q at %s:%d has %d members", d.Name, d.Host, d.Port, len(d.Members))
					for _, m := range d.Members {
						t.Logf("  member %q (%s) at %s:%d auth=%s ap2=%v", m.Name, m.ID, m.Host, m.Port, m.Auth, m.AirPlay2)
						if m.Auth != device.AuthHAPTransient || !m.AirPlay2 {
							continue
						}
						if want != "" && !strings.EqualFold(m.Name, want) && !strings.Contains(m.Name, want) {
							continue
						}
						return m
					}
					continue
				}
				if d.Auth != device.AuthHAPTransient || !d.AirPlay2 {
					continue
				}
				if want != "" && !strings.EqualFold(d.Name, want) && !strings.Contains(d.Name, want) {
					continue
				}
				return d
			}
		case <-deadline:
			t.Skip("no transient AirPlay 2 receiver found")
		}
	}
}

func runLatencySession(t *testing.T, target device.Device, latencyMS int, hold time.Duration, tone bool) {
	t.Helper()
	host := raopio.NewHost()
	defer host.Close()
	clock, err := ptp.New(target.Host)
	if err != nil {
		t.Fatal(err)
	}
	clock.Start()
	defer clock.Stop()

	var launched bool
	var launchedAt time.Time
	var closed bool
	var lastErr string
	receiverMS := 0
	s := New(host, Events{
		Launched: func(ok bool, err string) {
			launched = ok
			lastErr = err
			if ok {
				launchedAt = time.Now()
			}
		},
		Closed:      func() { closed = true },
		PinRequired: func(string) {},
		StateChanged: func(state State) {
			t.Logf("state: %s", state)
		},
		ReceiverLatencyChanged: func(ms int) {
			receiverMS = ms
			t.Logf("receiver reports %d ms arrival-to-render", ms)
		},
		FeedbackBody: func(b []byte) {
			t.Logf("feedback body %d bytes: %x", len(b), b)
		},
	}, func(level Level, format string, args ...any) {
		t.Logf("[%d] "+format, append([]any{level}, args...)...)
	})
	s.SetIdentity(Identity{Name: "saundo-latency-probe", DeviceID: "02:00:00:00:00:04", Model: "iPhone14,3"})
	s.SetAuth(target.Auth, true, target.ID, "", "")
	s.SetPTPClock(clock)
	s.SetInputFormat(44100)
	rb := ring.New(44100 * 2 * 2)
	// One second of audio before the pacer arms, so a late feeder tick does
	// not look like a sender underrun. The live engine trims to 50 ms at the
	// start gate; this probe keeps a deeper cushion on purpose.
	prefill := make([]int16, 44100*2)
	rb.Push(prefill)
	s.AttachRing(rb)
	s.SetLatency(latencyMS)
	s.Start(target.Host, target.Port, target.Name)
	s.SetVolume(0)

	stopFeed := make(chan struct{})
	go feedLatencyRing(rb, stopFeed, tone)

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	start := time.Now()
	lastStats := time.Now()
	var final Stats
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
			close(stopFeed)
			t.Fatalf("receiver closed the session: %s", lastErr)
		}
		if !launched && time.Since(start) > 30*time.Second {
			close(stopFeed)
			t.Fatalf("session did not launch; state=%s err=%s", s.State(), lastErr)
		}
		if time.Since(lastStats) >= time.Second {
			lastStats = time.Now()
			final = s.Stats()
			t.Logf("latency=%dms state=%s frames=%d receiver=%dms stats=%+v",
				latencyMS, s.State(), s.framesSent, receiverMS, final)
		}
		if launched && time.Since(launchedAt) >= hold {
			break
		}
	}

	final = s.Stats()
	elapsed := time.Since(launchedAt).Seconds()
	rate := 0.0
	if elapsed > 0 {
		rate = float64(s.framesSent) / elapsed
	}
	t.Logf("RESULT latency=%dms receiver=%dms frames=%d rate=%.1f/s retransmits=%d underruns=%d lateTicks=%d sentBlocks=%d bufferMs=%.1f wifi=%+v",
		latencyMS, receiverMS, s.framesSent, rate, final.Retransmits, final.Underruns,
		final.LateTicks, final.SentBlocks, final.BufferMS, final.WiFi)

	// Restore the receiver's own volume before leaving.
	if pct, ok := s.InitialVolumePercent(); ok {
		s.SetVolume(pct)
		t.Logf("restored receiver volume to %.0f%%", pct)
		time.Sleep(500 * time.Millisecond)
	}
	s.Stop()
	close(stopFeed)
}

func feedLatencyRing(rb *ring.Buffer, stop <-chan struct{}, tone bool) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	block := make([]int16, 2205*2) // 50 ms at 44.1 kHz, stereo
	var phase float64
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if tone {
				for i := 0; i < 2205; i++ {
					v := int16(1200 * math.Sin(2*math.Pi*440*phase/44100))
					phase++
					block[i*2], block[i*2+1] = v, v
				}
			} else {
				for i := range block {
					block[i] = 0
				}
			}
			rb.Push(block)
		}
	}
}
