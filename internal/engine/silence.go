package engine

import (
	"sync/atomic"
	"time"

	"saundo/internal/audio/ring"
)

// silencePeak is the absolute sample value above which a block counts as
// audible. Digital silence is exactly zero; ±1 is dither and is ignored.
const silencePeak = 1

// silenceMeter wraps the capture sink. It records when a block last carried
// audible samples and, while the stream is suspended, drops silent blocks so
// the rings neither fill nor count capture overflow. An audible block is
// forwarded even while suspended, so a resume starts from live audio.
type silenceMeter struct {
	sink     ring.Sink
	now      func() time.Time
	lastLoud atomic.Int64
	paused   atomic.Bool
}

func newSilenceMeter(sink ring.Sink) *silenceMeter {
	m := &silenceMeter{sink: sink, now: time.Now}
	m.lastLoud.Store(m.now().UnixNano())
	return m
}

func (m *silenceMeter) Push(samples []int16) int {
	loud := false
	for _, v := range samples {
		if v > silencePeak || v < -silencePeak {
			loud = true
			break
		}
	}
	if loud {
		m.lastLoud.Store(m.now().UnixNano())
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
