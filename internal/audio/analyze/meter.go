// Package analyze turns captured stereo blocks into the level and five-band
// frames the Now Playing card draws.
package analyze

import (
	"math"
	"sync"
	"time"
)

const (
	// BandCount is the number of spectrum bars in the card meter.
	BandCount = 5

	// ringSize is about 20 seconds of 100 Hz capture blocks, enough for the
	// longest presentation latency plus the visual delay.
	ringSize = 2048

	// window is the Goertzel window; about 42 ms at 48 kHz.
	window = 2048
)

var bandFreqs = [BandCount]float64{100, 250, 600, 1500, 4000}

// Frame is one measured block. Level and Bands are 0..1.
type Frame struct {
	At    time.Time
	Level float32
	Bands [BandCount]float32
}

// Meter measures level and bands from interleaved stereo int16 blocks. One
// capture goroutine calls Push; the UI calls At. Both are safe to run
// concurrently.
type Meter struct {
	mu       sync.Mutex
	rate     int
	mono     []float32
	frames   [ringSize]Frame
	head     int
	count    int
	levelEnv float32
	bandEnv  [BandCount]float32
	bandPeak [BandCount]float64
}

// New returns a meter for the given sample rate. SetRate may change it later.
func New(rate int) *Meter {
	return &Meter{rate: rate}
}

// SetRate changes the capture rate and clears the band history.
func (m *Meter) SetRate(rate int) {
	m.mu.Lock()
	m.rate = rate
	m.mono = m.mono[:0]
	m.mu.Unlock()
}

// Push measures one capture block. at is the block's capture time.
func (m *Meter) Push(samples []int16, at time.Time) {
	if len(samples) < 2 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// The level follows the energy in both channels; the mono mix below is
	// only for the bands. An out-of-phase stereo pair would cancel in the mono
	// mix and read as silence even though it is loud.
	var sum float64
	for i := 0; i+1 < len(samples); i += 2 {
		l := float64(samples[i]) / 32768
		r := float64(samples[i+1]) / 32768
		m.mono = append(m.mono, float32((l+r)/2))
		sum += (l*l + r*r) / 2
	}
	frames := len(samples) / 2
	rms := math.Sqrt(sum / float64(frames))
	level := normalizeDB(20 * math.Log10(math.Max(rms, 1e-6)))
	m.levelEnv = follow(m.levelEnv, level, 0.35, 0.06)

	var bands [BandCount]float32
	if m.rate > 0 && len(m.mono) >= window/2 {
		start := len(m.mono) - window
		if start < 0 {
			start = 0
		}
		seg := m.mono[start:]
		for b, freq := range bandFreqs {
			mag := goertzel(seg, m.rate, freq)
			peak := m.bandPeak[b]
			if mag > peak {
				peak = mag
			} else {
				peak *= 0.995
			}
			m.bandPeak[b] = peak
			v := float32(0)
			if peak > 1e-9 {
				v = float32(mag / peak)
			}
			if v > 1 {
				v = 1
			}
			m.bandEnv[b] = follow(m.bandEnv[b], v, 0.45, 0.08)
			bands[b] = m.bandEnv[b]
		}
	} else {
		for b := range bands {
			m.bandEnv[b] = follow(m.bandEnv[b], 0, 0.45, 0.08)
			bands[b] = m.bandEnv[b]
		}
	}
	if len(m.mono) > window*2 {
		copy(m.mono, m.mono[len(m.mono)-window:])
		m.mono = m.mono[:window]
	}

	m.frames[m.head] = Frame{At: at, Level: m.levelEnv, Bands: bands}
	m.head = (m.head + 1) % ringSize
	if m.count < ringSize {
		m.count++
	}
}

// At returns the frame captured delay before now. ok is false when no fresh
// frame exists, which happens while capture is stopped.
func (m *Meter) At(now time.Time, delay time.Duration) (Frame, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.count == 0 {
		return Frame{}, false
	}
	newest := m.frames[(m.head-1+ringSize)%ringSize]
	if now.Sub(newest.At) > 2*time.Second {
		return Frame{}, false
	}
	target := now.Add(-delay)
	for i := 1; i <= m.count; i++ {
		f := m.frames[(m.head-i+ringSize)%ringSize]
		if !f.At.After(target) {
			return f, true
		}
	}
	return m.frames[(m.head-m.count+ringSize)%ringSize], true
}

func normalizeDB(db float64) float32 {
	v := (db + 60) / 60
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return float32(v)
}

// follow is a one-pole envelope: fast attack, slow release.
func follow(env, x, attack, release float32) float32 {
	if x > env {
		return env + (x-env)*attack
	}
	return env + (x-env)*release
}

func goertzel(samples []float32, rate int, freq float64) float64 {
	n := len(samples)
	if n == 0 || rate <= 0 {
		return 0
	}
	k := int(0.5 + float64(n)*freq/float64(rate))
	w := 2 * math.Pi * float64(k) / float64(n)
	coeff := 2 * math.Cos(w)
	var s0, s1, s2 float64
	for _, x := range samples {
		s0 = float64(x) + coeff*s1 - s2
		s2 = s1
		s1 = s0
	}
	power := s1*s1 + s2*s2 - coeff*s1*s2
	if power < 0 {
		return 0
	}
	return math.Sqrt(power) / float64(n)
}
