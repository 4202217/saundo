package engine

import (
	"time"

	"saundo/internal/airplay/sender"
)

// The stability window is the trailing span the connection page judges
// stutter over. Joining a receiver or rebuilding a cohort can pad a packet or
// two with silence; that is a blip, not a session that is stuttering now, so
// the verdict and the counters beside it forget trouble older than this span.
const (
	stabilityWindow      = 60 * time.Second
	stabilitySampleEvery = time.Second
	// stabilityPublishEvery is how often a streaming session refreshes its
	// diagnostics. Without it the window could never age out on a quiet
	// session, because the engine publishes only on events.
	stabilityPublishEvery = 5 * time.Second
)

// stabilityCounts is the trailing-window evidence the projections read.
// Members follows the transport's sender order, so a target's count is the
// sum over its own members.
type stabilityCounts struct {
	Underruns  uint64
	Dropped    uint64
	SendErrors uint64
	Members    []uint64
}

// counterWindow tracks how far a cumulative counter has grown since the
// trailing window began. Samples are throttled: a reading taken less than
// stabilitySampleEvery after the previous one replaces it, so a volume drag
// that publishes repeatedly cannot pile up samples. A counter that moves
// backwards means its sender session restarted, and the window starts over.
type counterWindow struct {
	samples []counterSample
}

type counterSample struct {
	at time.Time
	v  uint64
}

func (w *counterWindow) add(at time.Time, v uint64) {
	n := len(w.samples)
	if n == 0 {
		w.samples = append(w.samples, counterSample{at: at, v: v})
		return
	}
	last := &w.samples[n-1]
	if v < last.v {
		w.samples = append(w.samples[:0], counterSample{at: at})
		return
	}
	if at.Sub(last.at) < stabilitySampleEvery {
		last.v = v
		return
	}
	w.samples = append(w.samples, counterSample{at: at, v: v})
}

// recent is the counter's growth over the trailing window. A session younger
// than the window is measured from its transport's zero start, so its first
// minute reports everything it did.
func (w *counterWindow) recent(now time.Time) uint64 {
	n := len(w.samples)
	if n == 0 {
		return 0
	}
	cutoff := now.Add(-stabilityWindow)
	base, keep := uint64(0), 0
	for i, s := range w.samples {
		if s.at.After(cutoff) {
			break
		}
		base, keep = s.v, i
	}
	w.samples = w.samples[keep:]
	last := w.samples[len(w.samples)-1].v
	if last < base {
		return 0
	}
	return last - base
}

// sessionStability is the engine's trailing-window view: one underrun window
// per sender member for the device rows, and session windows for the counters
// the stability verdict reads. observe runs once per publish, before either
// projection reads recent.
type sessionStability struct {
	members    []counterWindow
	dropped    counterWindow
	sendErrors counterWindow
	recent     stabilityCounts
}

// observe folds the transport's current counters into the windows and
// recomputes the trailing-window values. Empty member stats keep the dropped
// window alive; only reset ends the session's view.
func (s *sessionStability) observe(now time.Time, members []sender.Stats, dropped uint64) {
	if len(s.members) != len(members) {
		s.members = make([]counterWindow, len(members))
		s.recent.Members = make([]uint64, len(members))
	}
	var send uint64
	s.recent.Underruns = 0
	for i, m := range members {
		s.members[i].add(now, m.Underruns)
		s.recent.Members[i] = s.members[i].recent(now)
		s.recent.Underruns += s.recent.Members[i]
		send += m.SendErrors
	}
	s.dropped.add(now, dropped)
	s.recent.Dropped = s.dropped.recent(now)
	s.sendErrors.add(now, send)
	s.recent.SendErrors = s.sendErrors.recent(now)
}

// reset forgets the view when the session's transport closes: its counters
// belonged to the transport that just ended.
func (s *sessionStability) reset() { *s = sessionStability{} }

// statsDue reports whether the streaming session should publish its
// diagnostics now. The window needs a publish after trouble ages out, so this
// timer is what turns 有卡顿 back into 良好 without another engine event.
func (e *Engine) statsDue(now time.Time) bool {
	if e.tr == nil || e.tr.State() != sender.StateStreaming {
		e.statsAt = time.Time{}
		return false
	}
	if !e.statsAt.IsZero() && now.Before(e.statsAt) {
		return false
	}
	e.statsAt = now.Add(stabilityPublishEvery)
	return true
}

// statsWake is the diagnostics publish deadline, or zero while nothing
// streams.
func (e *Engine) statsWake() time.Time {
	if e.tr == nil || e.tr.State() != sender.StateStreaming {
		return time.Time{}
	}
	if e.statsAt.IsZero() {
		return e.now()
	}
	return e.statsAt
}
