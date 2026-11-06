package engine

import (
	"testing"
	"time"

	"saundo/internal/airplay/sender"
	"saundo/internal/audio/analyze"
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
	m := newSilenceMeter(&recordingSink{}, level)
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
	frame, ok := level.At(now, 0)
	if !ok || frame.Level == 0 {
		t.Fatalf("level frame = %+v ok=%v, want the capture block's level", frame, ok)
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

	h.e.silence.lastLoud.Store(h.now.Add(-3 * time.Second).UnixNano())
	h.e.updateAudioPause(h.now)
	if len(f.pauses) != 1 || !f.pauses[0] {
		t.Fatalf("pauses = %v, want [true]", f.pauses)
	}
	if !h.e.audioPaused {
		t.Fatal("engine did not enter the paused state")
	}
	if !h.e.buildSnapshot().AudioIdle {
		t.Fatal("snapshot did not report audio idle while suspended")
	}
	if !h.e.silence.paused.Load() {
		t.Fatal("silence meter did not stop forwarding")
	}

	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.updateAudioPause(h.now)
	if len(f.pauses) != 2 || f.pauses[1] {
		t.Fatalf("pauses = %v, want [true false]", f.pauses)
	}
	if h.e.buildSnapshot().AudioIdle {
		t.Fatal("snapshot still reports audio idle after resume")
	}
	h.e.updateAudioPause(h.now)
	if len(f.pauses) != 2 {
		t.Fatalf("duplicate pause transition: %v", f.pauses)
	}

	h.e.audioPaused = true
	wake := h.e.nextWake()
	if wake.IsZero() || wake.After(h.now.Add(2*audioPausePoll)) {
		t.Fatalf("nextWake while paused = %v, want a %s poll", wake, audioPausePoll)
	}
}

func TestEngineKeepsLegacySessionStreaming(t *testing.T) {
	h := newHarness(t)
	f := newFakeTransport(sender.Events{})
	f.state = sender.StateStreaming
	f.pauseOK = false
	h.e.tr = f

	h.e.silence.lastLoud.Store(h.now.Add(-3 * time.Second).UnixNano())
	h.e.updateAudioPause(h.now)
	if h.e.audioPaused {
		t.Fatal("engine paused a legacy AirPlay 1 session")
	}
	if !h.e.pauseUnsupported {
		t.Fatal("engine did not remember the unsupported pause")
	}
	if h.e.silence.paused.Load() {
		t.Fatal("silence meter stopped forwarding for a legacy session")
	}
	h.e.updateAudioPause(h.now)
	if len(f.pauses) != 1 {
		t.Fatalf("pauses = %v, want one attempt", f.pauses)
	}
}
