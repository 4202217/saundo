package sender

import (
	"fmt"
	"time"

	"saundo/internal/audio/ring"
)

// DefaultStartCushion is the newest-edge capture retained when a session
// begins. At 44.1 kHz it is 2205 frames, about 46 ms, or roughly five pacer
// packets. It covers a WASAPI shared-mode period (about 10 ms), one pacer tick
// (8 ms), and capture scheduling jitter while adding only 50 ms to true
// end-to-end latency.
const DefaultStartCushion = 50 * time.Millisecond

// StartGateConfig describes one cohort of senders that must begin consuming
// the same capture stream at one wall-clock instant.
type StartGateConfig struct {
	// Members is the number of senders that must arrive. Values below 1 mean 1.
	Members int
	// Cushion is the newest-edge audio kept at the epoch. Zero trims everything.
	Cushion time.Duration
	// Trim trims every buffer sharing the capture stream atomically. Nil skips
	// trimming; the gate still supplies the shared epoch.
	Trim ring.Trimmer
}

// StartGate is the single owner of a session's start epoch and of the one trim
// that leaves the startup cushion. Senders sharing a gate call Arrive once when
// their receiver accepts the stream, then call Begin from Tick until it returns
// true.
//
// All methods run on the engine goroutine. The gate owns no lock, timer, or
// goroutine; the only lock in the path is the capture fanout's, taken by Begin
// through Trim.
type StartGate struct {
	members int
	cushion time.Duration
	trim    ring.Trimmer

	arrivals int
	rate     int
	epoch    time.Time
	sealed   bool
	trimmed  bool
}

// NewStartGate returns a single-use gate. Create one per session; reusing a
// gate would return the previous session's epoch.
func NewStartGate(cfg StartGateConfig) *StartGate {
	if cfg.Members < 1 {
		cfg.Members = 1
	}
	if cfg.Cushion < 0 {
		cfg.Cushion = 0
	}
	return &StartGate{
		members: cfg.Members,
		cushion: cfg.Cushion,
		trim:    cfg.Trim,
	}
}

// Arrive marks one sender stream-ready at now. The last arrival seals the
// shared epoch at that instant. Arriving after the seal is a no-op. A later
// arrival with a different input rate is an error: the two cushions would
// cover different durations of audio.
func (g *StartGate) Arrive(now time.Time, inputRate int) error {
	if g.sealed {
		return nil
	}
	if g.arrivals == 0 {
		g.rate = inputRate
	} else if inputRate != g.rate {
		return fmt.Errorf("start gate: members disagree on input rate (%d Hz vs %d Hz)", g.rate, inputRate)
	}
	g.arrivals++
	if g.arrivals >= g.members {
		g.epoch = now
		g.sealed = true
	}
	return nil
}

// Begin reports the shared start epoch once every member has arrived and now
// has reached it. The first successful call performs the one trim that keeps
// the newest cushion in every registered buffer; later calls return the same
// epoch without trimming again.
func (g *StartGate) Begin(now time.Time) (time.Time, bool) {
	if !g.sealed || now.Before(g.epoch) {
		return time.Time{}, false
	}
	if !g.trimmed {
		if g.trim != nil {
			g.trim.KeepNewest(g.cushionSamples())
		}
		g.trimmed = true
	}
	return g.epoch, true
}

// Sealed reports whether every member has arrived.
func (g *StartGate) Sealed() bool { return g.sealed }

// Epoch reports the sealed start instant, or the zero time before sealing.
func (g *StartGate) Epoch() time.Time { return g.epoch }

// cushionSamples converts the time cushion into interleaved stereo samples at
// the capture rate.
func (g *StartGate) cushionSamples() int {
	if g.cushion <= 0 || g.rate <= 0 {
		return 0
	}
	frames := int(float64(g.rate) * g.cushion.Seconds())
	return frames * channels
}
