package engine

import (
	"sync/atomic"
	"time"

	"saundo/internal/audio/ring"
)

// silencePeak is the absolute sample value above which a block counts as
// audible. Digital silence is exactly zero; ±1 is dither and is ignored.
const silencePeak = 1

// levelFeedSamples is 100 ms at 48 kHz stereo: enough for the meter pump to
// drain between ticks, small enough that a stalled UI drops old audio instead
// of delaying the meter.
const levelFeedSamples = 48000 * 2 / 10

// silenceMeter wraps the capture sink. It records when a block last carried
// audible samples and, while the stream is suspended, drops silent blocks so
// the rings neither fill nor count capture overflow. An audible block is
// forwarded even while suspended, so a resume starts from live audio.
//
// The audio thread only does the loudness scan and two lock-free pushes. The
// visual meter's analyzer runs on levelLoop, fed through meter.
type silenceMeter struct {
	sink     ring.Sink
	meter    ring.Sink
	now      func() time.Time
	lastLoud atomic.Int64
	paused   atomic.Bool
}

func newSilenceMeter(sink, meter ring.Sink) *silenceMeter {
	m := &silenceMeter{sink: sink, meter: meter, now: time.Now}
	m.lastLoud.Store(m.now().UnixNano())
	return m
}

func (m *silenceMeter) Push(samples []int16) int {
	at := m.now()
	if m.meter != nil {
		m.meter.Push(samples)
	}
	loud := false
	for _, v := range samples {
		if v > silencePeak || v < -silencePeak {
			loud = true
			break
		}
	}
	if loud {
		m.lastLoud.Store(at.UnixNano())
	}
	if m.paused.Load() && !loud {
		return 0
	}
	return m.sink.Push(samples)
}

func (m *silenceMeter) Dropped() uint64 { return m.sink.Dropped() }

func (m *silenceMeter) Reset() { m.sink.Reset() }

func (m *silenceMeter) KeepNewest(newest int) int { return m.sink.KeepNewest(newest) }

// LastLoud reports when a block last carried audible samples.
func (m *silenceMeter) LastLoud() time.Time { return time.Unix(0, m.lastLoud.Load()) }

// SetPaused makes Push drop silent blocks while the audio stream is suspended.
func (m *silenceMeter) SetPaused(paused bool) { m.paused.Store(paused) }

// startLevelPump feeds the visual level meter off the audio thread. The
// capture thread only pushes into levelFeed; this worker pops and runs the
// analyzer, whose lock and history append must not run in the IOProc.
func (e *Engine) startLevelPump() {
	if e.levelFeed == nil || e.level == nil {
		return
	}
	e.levelRunning.Store(true)
	go e.levelLoop()
}

func (e *Engine) levelLoop() {
	defer close(e.levelDone)
	buf := make([]int16, levelFeedSamples)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for {
				n := e.levelFeed.Pop(buf)
				if n == 0 {
					break
				}
				e.level.Push(buf[:n], time.Now())
				if n < len(buf) {
					break
				}
			}
		case <-e.levelStop:
			return
		}
	}
}

func (e *Engine) stopLevelPump() {
	if !e.levelRunning.Load() {
		return
	}
	close(e.levelStop)
	select {
	case <-e.levelDone:
	case <-time.After(2 * time.Second):
		e.logf("level pump did not stop within 2s")
	}
}
