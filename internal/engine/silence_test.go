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

// TestSilentSourceSuspendsTheStreamAndThePhaseIdles pins the policy: after
// five seconds of digital silence the engine suspends the pacer, so the
// receiver's buffer drains; the first audible block resumes it with a
// re-anchored timeline.
func TestSilentSourceSuspendsTheStreamAndThePhaseIdles(t *testing.T) {
	h := newHarness(t)
	f := newFakeTransport(sender.Events{})
	f.state = sender.StateStreaming
	h.e.tr = f

	h.e.silence.lastLoud.Store(h.now.Add(-audioPauseAfter).UnixNano())
	h.e.updateIdlePhase(h.now)
	if len(f.pauses) != 1 || !f.pauses[0] {
		t.Fatalf("pauses = %v, want one suspend", f.pauses)
	}
	if f.pushing {
		t.Fatal("a silent source kept the pacer running")
	}
	if h.e.idle != idleSuspended {
		t.Fatal("engine did not enter the suspended phase")
	}
	if !h.e.buildSnapshot().AudioIdle {
		t.Fatal("snapshot did not report audio idle while the source is silent")
	}
	if !h.e.silence.paused.Load() {
		t.Fatal("silence meter kept forwarding silent blocks")
	}

	h.advance(2 * time.Hour)
	h.e.updateIdlePhase(h.now)
	if len(f.pauses) != 1 || f.pushing {
		t.Fatalf("an hours-long silence changed the suspension: pauses=%v pushing=%v", f.pauses, f.pushing)
	}

	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.updateIdlePhase(h.now)
	if len(f.pauses) != 2 || f.pauses[1] {
		t.Fatalf("pauses = %v, want suspend then resume", f.pauses)
	}
	if !f.pushing {
		t.Fatal("the first audible block did not resume the pacer")
	}
	if h.e.silence.paused.Load() {
		t.Fatal("silence meter stayed paused after the resume")
	}
	if h.e.buildSnapshot().AudioIdle {
		t.Fatal("snapshot still reports audio idle after resume")
	}
	h.e.updateIdlePhase(h.now)
	if len(f.pauses) != 2 {
		t.Fatalf("resuming paused the transport again: %v", f.pauses)
	}

	h.e.idle = idleSuspended
	wake := h.e.nextWake()
	if wake.IsZero() || wake.After(h.now.Add(2*audioPausePoll)) {
		t.Fatalf("nextWake while idle = %v, want a %s poll", wake, audioPausePoll)
	}
}
