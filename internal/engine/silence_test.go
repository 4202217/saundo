package engine

import (
	"testing"
	"time"

	"saundo/internal/airplay/sender"
	"saundo/internal/audio/analyze"
	"saundo/internal/audio/ring"
)

type recordingSink struct {
	pushed int
}

func (s *recordingSink) Push(samples []int16) int { s.pushed += len(samples); return 0 }
func (s *recordingSink) Dropped() uint64          { return 0 }
func (s *recordingSink) Reset()                   {}
func (s *recordingSink) KeepNewest(int) int       { return 0 }

func TestSilenceMeterFeedsTheLevelMeter(t *testing.T) {
	level := analyze.New(48000)
	feed := ring.New(4096)
	m := newSilenceMeter(&recordingSink{}, feed)
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	block := make([]int16, 960)
	for i := range block {
		if i%2 == 0 {
			block[i] = 12000
		} else {
			block[i] = -12000
		}
	}
	m.Push(block)
	out := make([]int16, len(block))
	if n := feed.Pop(out); n != len(block) {
		t.Fatalf("level feed pop = %d, want %d", n, len(block))
	}
	level.Push(out, now)
	frame, ok := level.At(now, 0)
	if !ok || frame.Level == 0 {
		t.Fatalf("level frame = %+v ok=%v, want the capture block's level", frame, ok)
	}
}

func TestLevelPumpFeedsTheMeter(t *testing.T) {
	e := &Engine{
		level:     analyze.New(48000),
		levelFeed: ring.New(4096),
		levelStop: make(chan struct{}),
		levelDone: make(chan struct{}),
	}
	e.startLevelPump()
	t.Cleanup(e.stopLevelPump)
	block := make([]int16, 960)
	for i := range block {
		block[i] = 12000
	}
	e.levelFeed.Push(block)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if frame, ok := e.level.At(time.Now(), 0); ok && frame.Level > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("level pump did not feed the meter")
}

func TestSilenceMeterPushDoesNotAllocate(t *testing.T) {
	sink := ring.NewFanout(ring.New(4096))
	m := newSilenceMeter(sink, ring.New(4096))
	block := make([]int16, 960)
	m.Push(block)
	if got := testing.AllocsPerRun(1000, func() { m.Push(block) }); got != 0 {
		t.Fatalf("silence meter push allocates %.1f times per run", got)
	}
}

func TestSilenceMeterDropsSilentBlocksOnlyWhilePaused(t *testing.T) {
	sink := &recordingSink{}
	m := newSilenceMeter(sink, nil)
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	m.lastLoud.Store(now.UnixNano())
	zeros := make([]int16, 960)
	m.Push(zeros)
	if sink.pushed != len(zeros) {
		t.Fatalf("silent block not forwarded while playing: %d", sink.pushed)
	}

	before := m.LastLoud()
	m.SetPaused(true)
	m.Push(zeros)
	if sink.pushed != len(zeros) {
		t.Fatalf("silent block forwarded while paused: %d", sink.pushed)
	}

	loud := make([]int16, 960)
	loud[100] = 200
	now = now.Add(time.Second)
	m.Push(loud)
	if sink.pushed != len(zeros)+len(loud) {
		t.Fatalf("loud block not forwarded while paused: %d", sink.pushed)
	}
	if !m.LastLoud().After(before) {
		t.Fatal("LastLoud did not advance on an audible block")
	}

	m.SetPaused(false)
	m.Push(zeros)
	if sink.pushed != len(zeros)*2+len(loud) {
		t.Fatalf("silent block not forwarded after resume: %d", sink.pushed)
	}
}

func TestEngineSuspendsAfterSilenceAndResumes(t *testing.T) {
	h := newHarness(t)
	f := newFakeTransport(sender.Events{})
	f.state = sender.StateStreaming
	h.e.tr = f

	h.e.silence.lastLoud.Store(h.now.Add(-6 * time.Second).UnixNano())
	h.e.updateIdlePhase(h.now)
	if len(f.pauses) != 1 || !f.pauses[0] {
		t.Fatalf("pauses = %v, want [true]", f.pauses)
	}
	if h.e.idle != idleSuspended {
		t.Fatal("engine did not enter the suspended phase")
	}
	if !h.e.buildSnapshot().AudioIdle {
		t.Fatal("snapshot did not report audio idle while suspended")
	}
	if !h.e.silence.paused.Load() {
		t.Fatal("silence meter did not stop forwarding")
	}

	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.updateIdlePhase(h.now)
	if len(f.pauses) != 2 || f.pauses[1] {
		t.Fatalf("pauses = %v, want [true false]", f.pauses)
	}
	if h.e.buildSnapshot().AudioIdle {
		t.Fatal("snapshot still reports audio idle after resume")
	}
	h.e.updateIdlePhase(h.now)
	if len(f.pauses) != 2 {
		t.Fatalf("duplicate pause transition: %v", f.pauses)
	}

	h.e.idle = idleSuspended
	wake := h.e.nextWake()
	if wake.IsZero() || wake.After(h.now.Add(2*audioPausePoll)) {
		t.Fatalf("nextWake while paused = %v, want a %s poll", wake, audioPausePoll)
	}
}

// TestPauseThresholdFitsTheCaptureRing keeps the silence policy inside the
// ring, so a pause never overflows it with silence and the UI's dropped
// figure keeps meaning capture overflow.
func TestPauseThresholdFitsTheCaptureRing(t *testing.T) {
	h := newHarness(t)
	block := make([]int16, 960) // 10 ms at 48 kHz stereo
	blocks := int(audioPauseAfter.Seconds() * 100)
	for range blocks {
		h.e.silence.Push(block)
	}
	if h.e.ring.Capacity() < blocks*len(block) {
		t.Fatalf("ring capacity %d < %d samples of silence", h.e.ring.Capacity(), blocks*len(block))
	}
	if got := h.e.silence.Dropped(); got != 0 {
		t.Fatalf("silence policy overflowed the capture ring: dropped %d samples", got)
	}
}

func TestEngineKeepsLegacySessionStreaming(t *testing.T) {
	h := newHarness(t)
	f := newFakeTransport(sender.Events{})
	f.state = sender.StateStreaming
	f.pauseOK = false
	h.e.tr = f

	h.e.silence.lastLoud.Store(h.now.Add(-6 * time.Second).UnixNano())
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleActive {
		t.Fatal("engine paused a legacy AirPlay 1 session")
	}
	if !h.e.pauseUnsupported {
		t.Fatal("engine did not remember the unsupported pause")
	}
	if h.e.silence.paused.Load() {
		t.Fatal("silence meter stopped forwarding for a legacy session")
	}
	h.e.updateIdlePhase(h.now)
	if len(f.pauses) != 1 {
		t.Fatalf("pauses = %v, want one attempt", f.pauses)
	}
}
