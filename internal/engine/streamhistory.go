package engine

import (
	"sync"
	"time"

	"saundo/internal/airplay/sender"
)

// The connection page's strip: 100 ms samples over the trailing minute,
// pulled by the UI. The engine owns the ring; the UI copies one-second
// buckets out under the lock, so it never reads memory the engine mutates.

const (
	streamSampleEvery = 100 * time.Millisecond
	streamWindow      = 60 * time.Second
	streamSamples     = int(streamWindow / streamSampleEvery) // 600
	streamBucketCount = int(streamWindow / time.Second)       // 60

	// StreamBucketCount is the number of one-second buckets the connection
	// strip draws. The UI sizes its reusable buffer with it.
	StreamBucketCount = streamBucketCount
)

// StreamBucket is one second of the send side for the strip: the lowest ring
// headroom seen that second and the events committed inside it. Underruns and
// Retransmits are counts in this bucket, not cumulative totals.
type StreamBucket struct {
	At     time.Time
	HeadMS float64
	Valid  bool
	// Idle marks a second whose source was silent: the ring is dry by
	// design, so a zero there is not starvation.
	Idle        bool
	Underruns   uint64
	Retransmits uint64
}

type streamPoint struct {
	at          time.Time
	headMS      float64
	valid       bool
	idle        bool
	underruns   uint64
	retransmits uint64
}

// dashboard carries the engine's live send-side view: the 100 ms ring the
// connection page draws, and the byte-rate samples the flow numbers read.
type dashboard struct {
	stream streamHistory
	net    rateSample
	links  map[string]rateSample
}

type rateSample struct {
	at    time.Time
	bytes uint64
	bps   float64
	valid bool
}

type streamHistory struct {
	mu     sync.Mutex
	points [streamSamples]streamPoint
	head   int
	count  int
	last   time.Time
}

func (h *streamHistory) reset() {
	h.mu.Lock()
	h.head, h.count, h.last = 0, 0, time.Time{}
	h.mu.Unlock()
}

// sample records one 100 ms sample from the member stats. A counter that
// moved backwards means a new session, so the ring starts over instead of
// drawing a negative delta.
func (h *streamHistory) sample(now time.Time, members []sender.Stats, idle bool) {
	if len(members) == 0 {
		return
	}
	headSet, head := false, 0.0
	var under, retrans uint64
	for _, m := range members {
		if m.BufferValid {
			if !headSet || m.BufferMS < head {
				head = m.BufferMS
			}
			headSet = true
		}
		under += m.Underruns
		retrans += m.Retransmits
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.last.IsZero() && now.Sub(h.last) < streamSampleEvery {
		return
	}
	if h.count > 0 {
		prev := h.points[(h.head-1+streamSamples)%streamSamples]
		if under < prev.underruns || retrans < prev.retransmits {
			h.head, h.count = 0, 0
		}
	}
	h.points[h.head] = streamPoint{at: now, headMS: head, valid: headSet, idle: idle, underruns: under, retransmits: retrans}
	h.head = (h.head + 1) % streamSamples
	if h.count < streamSamples {
		h.count++
	}
	h.last = now
}

// minHead is the lowest valid headroom in the trailing window, for the
// measured card.
func (h *streamHistory) minHead(now time.Time) (float64, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cutoff := now.Add(-streamWindow)
	best, ok := 0.0, false
	for i := 0; i < h.count; i++ {
		p := h.points[(h.head-h.count+i+streamSamples)%streamSamples]
		if p.at.Before(cutoff) || !p.valid || p.idle {
			continue
		}
		if !ok || p.headMS < best {
			best, ok = p.headMS, true
		}
	}
	return best, ok
}

// buckets copies the trailing minute into one-second buckets, oldest first.
// dst must hold streamBucketCount entries; the caller reuses it every paint.
func (h *streamHistory) buckets(now time.Time, dst []StreamBucket) []StreamBucket {
	if len(dst) < streamBucketCount {
		return nil
	}
	dst = dst[:streamBucketCount]
	for i := range dst {
		dst[i] = StreamBucket{}
	}
	start := now.Add(-streamWindow)
	h.mu.Lock()
	defer h.mu.Unlock()
	var prev streamPoint
	havePrev := false
	for i := 0; i < h.count; i++ {
		p := h.points[(h.head-h.count+i+streamSamples)%streamSamples]
		if p.at.Before(start) {
			prev, havePrev = p, true
			continue
		}
		bucket := streamBucketCount - 1 - int(now.Sub(p.at)/time.Second)
		if bucket < 0 {
			continue
		}
		if bucket >= streamBucketCount {
			bucket = streamBucketCount - 1
		}
		b := &dst[bucket]
		if b.At.IsZero() {
			b.At = start.Add(time.Duration(bucket) * time.Second)
		}
		if p.valid && (!b.Valid || p.headMS < b.HeadMS) {
			b.HeadMS, b.Valid = p.headMS, true
		}
		b.Idle = p.idle
		if havePrev {
			if p.underruns > prev.underruns {
				b.Underruns += p.underruns - prev.underruns
			}
			if p.retransmits > prev.retransmits {
				b.Retransmits += p.retransmits - prev.retransmits
			}
		}
		prev, havePrev = p, true
	}
	return dst
}

// StreamBuckets copies the trailing minute's strip data into dst and returns
// the filled slice. The caller owns dst; the engine keeps no reference. Call
// it from the UI paint path.
func (e *Engine) StreamBuckets(dst []StreamBucket) []StreamBucket {
	return e.dash.stream.buckets(e.now(), dst)
}

// sampleStream feeds one strip sample from the live transport. It runs on
// the engine goroutine, where Stats is safe to read.
func (e *Engine) sampleStream(now time.Time) {
	if e.tr == nil || e.tr.State() != sender.StateStreaming {
		return
	}
	e.dash.stream.sample(now, e.tr.Stats(), e.idle != idleActive)
}

// rate returns bytes per second over at least a second, keeping the previous
// value through bursts of event-driven publishes.
func (r *rateSample) rate(now time.Time, bytes uint64) (float64, bool) {
	if r.at.IsZero() || bytes < r.bytes {
		*r = rateSample{at: now, bytes: bytes}
		return 0, false
	}
	if dt := now.Sub(r.at); dt >= time.Second {
		r.bps = float64(bytes-r.bytes) / dt.Seconds()
		r.valid = true
		r.at, r.bytes = now, bytes
	}
	return r.bps, r.valid
}

// netRate is the session's total send rate.
func (e *Engine) netRate(now time.Time, bytes uint64) (float64, bool) {
	return e.dash.net.rate(now, bytes)
}

// linkRate is one target's send rate, keyed by the plan's target key.
func (e *Engine) linkRate(now time.Time, key string, bytes uint64) (float64, bool) {
	if e.dash.links == nil {
		e.dash.links = make(map[string]rateSample)
	}
	r := e.dash.links[key]
	bps, ok := r.rate(now, bytes)
	e.dash.links[key] = r
	return bps, ok
}

// resetDashboard forgets the previous session's samples and rates.
func (e *Engine) resetDashboard() {
	e.dash.stream.reset()
	e.dash.net = rateSample{}
	e.dash.links = nil
}
