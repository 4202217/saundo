package sender

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/ptp"
	"saundo/internal/audio/ring"
)

// lossyIO wraps the real host and drops or delays audio datagrams. Control
// and timing channels pass untouched, so retransmit requests and PTP behave
// as they do on the wire.
type lossyIO struct {
	inner  raopio.IO
	loss   float64
	jitter time.Duration
	rng    *rand.Rand
	mu     sync.Mutex
}

func (l *lossyIO) TCPConnect(ch raopio.TCPChannel, host string, port uint16) error {
	return l.inner.TCPConnect(ch, host, port)
}

func (l *lossyIO) TCPSend(ch raopio.TCPChannel, data []byte) error {
	return l.inner.TCPSend(ch, data)
}

func (l *lossyIO) TCPClose(ch raopio.TCPChannel, flush bool) {
	l.inner.TCPClose(ch, flush)
}

func (l *lossyIO) UDPBind(ch raopio.UDPChannel) (uint16, error) {
	return l.inner.UDPBind(ch)
}

func (l *lossyIO) UDPClose(ch raopio.UDPChannel) {
	l.inner.UDPClose(ch)
}

func (l *lossyIO) UDPSend(ch raopio.UDPChannel, to raopio.Endpoint, data []byte) error {
	if ch != raopio.UDPAudio {
		return l.inner.UDPSend(ch, to, data)
	}
	l.mu.Lock()
	drop := l.loss > 0 && l.rng.Float64() < l.loss
	var delay time.Duration
	if l.jitter > 0 {
		delay = time.Duration(l.rng.Int63n(int64(l.jitter)))
	}
	l.mu.Unlock()
	if drop {
		return nil
	}
	if delay > 0 {
		// The sender owns a single event loop; a delayed datagram must not
		// block the pacer. The UDP socket is safe for concurrent writes.
		cp := append([]byte(nil), data...)
		go func() {
			time.Sleep(delay)
			_ = l.inner.UDPSend(ch, to, cp)
		}()
		return nil
	}
	return l.inner.UDPSend(ch, to, data)
}

type lossyCase struct {
	latencyMS  int
	loss       float64
	jitter     time.Duration
	duplicates int
	prefill    int
}

// TestLiveLossySweep drives a real HomePod through the lossy wrapper. It is a
// probe, not a gate: the receiver is muted, the ring is fed continuously, and
// the counters show how loss and jitter interact with the pacer and the
// receiver's retransmit path.
//
// SAUNDO_LOSSY_TEST=1 enables it. SAUNDO_LOSSY_TARGET selects a receiver by
// name. SAUNDO_LOSSY_HOLD is seconds per case (default 15). Stop the saundo app
// first so PTP can bind UDP 319/320.
func TestLiveLossySweep(t *testing.T) {
	if os.Getenv("SAUNDO_LOSSY_TEST") == "" {
		t.Skip("set SAUNDO_LOSSY_TEST=1 to run the lossy real-receiver sweep")
	}
	if testing.Short() {
		t.Skip("live lossy sweep")
	}
	target := latencyTarget(t, os.Getenv("SAUNDO_LOSSY_TARGET"))
	t.Logf("target: %s (%s) at %s:%d model=%s", target.Name, target.ID, target.Host, target.Port, target.Model)

	hold := 15 * time.Second
	if raw := os.Getenv("SAUNDO_LOSSY_HOLD"); raw != "" {
		secs, err := strconv.Atoi(raw)
		if err != nil || secs <= 0 {
			t.Fatalf("bad SAUNDO_LOSSY_HOLD %q", raw)
		}
		hold = time.Duration(secs) * time.Second
	}

	cases := []lossyCase{
		{100, 0, 0, 0, 0},
		{100, 0.01, 0, 0, 0},
		{100, 0.03, 0, 0, 0},
		{100, 0.05, 0, 0, 0},
		{100, 0.03, 0, 1, 0},
		{100, 0.05, 0, 1, 0},
		{100, 0.05, 0, 2, 0},
		{100, 0.03, 20 * time.Millisecond, 0, 0},
		{100, 0.03, 20 * time.Millisecond, 1, 0},
		{90, 0.01, 0, 0, 0},
		{90, 0.01, 0, 1, 0},
		{150, 0.05, 0, 0, 0},
		{150, 0.05, 0, 1, 0},
		{200, 0.05, 20 * time.Millisecond, 0, 0},
	}
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%dms-l%d-j%d-d%d", c.latencyMS, int(c.loss*100), c.jitter.Milliseconds(), c.duplicates), func(t *testing.T) {
			runLossySession(t, target, c, hold, int64(i+1))
		})
		time.Sleep(6 * time.Second)
	}
}

// TestLiveLossyJitter focuses on the jitter side. The first sweep showed that
// duplicates fix loss but not late packets, so this matrix compares larger
// presentation latencies and 2x/3x copies under the same 20 ms jitter.
func TestLiveLossyJitter(t *testing.T) {
	if os.Getenv("SAUNDO_LOSSY_TEST") == "" {
		t.Skip("set SAUNDO_LOSSY_TEST=1 to run the lossy real-receiver sweep")
	}
	if testing.Short() {
		t.Skip("live lossy jitter sweep")
	}
	target := latencyTarget(t, os.Getenv("SAUNDO_LOSSY_TARGET"))
	hold := 15 * time.Second
	if raw := os.Getenv("SAUNDO_LOSSY_HOLD"); raw != "" {
		secs, err := strconv.Atoi(raw)
		if err != nil || secs <= 0 {
			t.Fatalf("bad SAUNDO_LOSSY_HOLD %q", raw)
		}
		hold = time.Duration(secs) * time.Second
	}
	cases := []lossyCase{
		{100, 0.03, 20 * time.Millisecond, 0, 0},
		{100, 0.03, 20 * time.Millisecond, 2, 0},
		{150, 0.03, 20 * time.Millisecond, 1, 0},
		{200, 0.03, 20 * time.Millisecond, 1, 0},
		{150, 0.05, 20 * time.Millisecond, 1, 0},
		{200, 0.05, 20 * time.Millisecond, 1, 0},
		{200, 0.05, 20 * time.Millisecond, 0, 0},
	}
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%dms-l%d-j%d-d%d", c.latencyMS, int(c.loss*100), c.jitter.Milliseconds(), c.duplicates), func(t *testing.T) {
			runLossySession(t, target, c, hold, int64(100+i))
		})
		time.Sleep(6 * time.Second)
	}
}

// TestLivePacerPrefill sends two extra audio packets as soon as the session
// starts streaming, then measures whether the receiver's buffer absorbs the
// 20 ms jitter at 80/90 ms better than the normal one-packet-per-tick pacer.
func TestLivePacerPrefill(t *testing.T) {
	if os.Getenv("SAUNDO_LOSSY_TEST") == "" {
		t.Skip("set SAUNDO_LOSSY_TEST=1 to run the lossy real-receiver sweep")
	}
	if testing.Short() {
		t.Skip("live pacer prefill")
	}
	target := latencyTarget(t, os.Getenv("SAUNDO_LOSSY_TARGET"))
	hold := 15 * time.Second
	if raw := os.Getenv("SAUNDO_LOSSY_HOLD"); raw != "" {
		secs, err := strconv.Atoi(raw)
		if err != nil || secs <= 0 {
			t.Fatalf("bad SAUNDO_LOSSY_HOLD %q", raw)
		}
		hold = time.Duration(secs) * time.Second
	}
	cases := []lossyCase{
		{80, 0.03, 20 * time.Millisecond, 1, 0},
		{80, 0.03, 20 * time.Millisecond, 1, 2},
		{80, 0.03, 20 * time.Millisecond, 2, 2},
		{90, 0.03, 20 * time.Millisecond, 1, 2},
		{90, 0.03, 20 * time.Millisecond, 2, 2},
		{80, 0, 0, 1, 2},
		{90, 0, 0, 1, 2},
	}
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%dms-l%d-j%d-d%d-p%d", c.latencyMS, int(c.loss*100), c.jitter.Milliseconds(), c.duplicates, c.prefill), func(t *testing.T) {
			runLossySession(t, target, c, hold, int64(200+i))
		})
		time.Sleep(6 * time.Second)
	}
}

func runLossySession(t *testing.T, target device.Device, c lossyCase, hold time.Duration, seed int64) {
	t.Helper()
	host := raopio.NewHost()
	defer host.Close()
	lossy := &lossyIO{
		inner:  host,
		loss:   c.loss,
		jitter: c.jitter,
		rng:    rand.New(rand.NewSource(seed)),
	}
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
	s := New(lossy, Events{
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
		},
		FeedbackBody: func(b []byte) {
			t.Logf("feedback body %d bytes: %x", len(b), b)
		},
	}, func(level Level, format string, args ...any) {
		t.Logf("[%d] "+format, append([]any{level}, args...)...)
	})
	s.SetIdentity(Identity{Name: "saundo-lossy-probe", DeviceID: "02:00:00:00:00:05", Model: "iPhone14,3"})
	s.SetAuth(target.Auth, true, target.ID, "", "")
	s.SetPTPClock(clock)
	s.SetInputFormat(44100)
	rb := ring.New(44100 * 2 * 2)
	prefill := make([]int16, 44100*2)
	rb.Push(prefill)
	s.AttachRing(rb)
	s.SetLatency(c.latencyMS)
	s.setDuplicates(c.duplicates)
	s.Start(target.Host, target.Port, target.Name)
	s.SetVolume(0)

	stopFeed := make(chan struct{})
	defer close(stopFeed)
	go feedLatencyRing(rb, stopFeed, false)

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	start := time.Now()
	lastStats := time.Now()
	prefilled := false
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
			final = s.Stats()
			t.Logf("RESULT-FAIL latency=%dms loss=%.0f%% jitter=%dms dup=%d state=%s retransmits=%d underruns=%d lateTicks=%d frames=%d",
				c.latencyMS, c.loss*100, c.jitter.Milliseconds(), c.duplicates, s.State(),
				final.Retransmits, final.Underruns, final.LateTicks, s.framesSent)
			t.Fatalf("receiver closed the session: %s", lastErr)
		}
		if !launched && time.Since(start) > 30*time.Second {
			t.Fatalf("session did not launch; state=%s err=%s", s.State(), lastErr)
		}
		if launched && !prefilled {
			for i := 0; i < c.prefill; i++ {
				s.sendAudioPacket()
			}
			prefilled = true
			if c.prefill > 0 {
				t.Logf("prefilled %d packets", c.prefill)
			}
		}
		if time.Since(lastStats) >= time.Second {
			lastStats = time.Now()
			final = s.Stats()
			t.Logf("latency=%dms loss=%.0f%% jitter=%dms dup=%d state=%s frames=%d receiver=%dms stats=%+v",
				c.latencyMS, c.loss*100, c.jitter.Milliseconds(), c.duplicates,
				s.State(), s.framesSent, receiverMS, final)
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
	t.Logf("RESULT latency=%dms loss=%.0f%% jitter=%dms dup=%d prefill=%d receiver=%dms frames=%d rate=%.1f/s retransmits=%d underruns=%d lateTicks=%d sentBlocks=%d bufferMs=%.1f",
		c.latencyMS, c.loss*100, c.jitter.Milliseconds(), c.duplicates, c.prefill, receiverMS,
		s.framesSent, rate, final.Retransmits, final.Underruns, final.LateTicks,
		final.SentBlocks, final.BufferMS)

	if pct, ok := s.InitialVolumePercent(); ok {
		s.SetVolume(pct)
		time.Sleep(500 * time.Millisecond)
	}
	s.Stop()
}
