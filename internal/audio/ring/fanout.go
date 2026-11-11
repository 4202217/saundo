package ring

import (
	"sync"
	"sync/atomic"
)

// Sink is the producer-facing side of an audio buffer. A stereo-pair session
// feeds several member rings from one capture stream.
type Sink interface {
	Push(samples []int16) int
	Dropped() uint64
	Reset()
	KeepNewest(newest int) int
}

// Trimmer is the buffer-side capability a start gate needs. *Buffer trims one
// stream; *Fanout trims every stream sharing one capture source.
type Trimmer interface {
	KeepNewest(newest int) int
}

// Fanout writes every push to all registered sinks. Add and Remove publish a
// new immutable sink list; Push reads one snapshot, so it takes no lock and
// allocates nothing, and a push is never split across a change.
type Fanout struct {
	mu    sync.Mutex
	sinks atomic.Pointer[[]Sink]
}

// NewFanout returns a fanout over the given sinks.
func NewFanout(sinks ...Sink) *Fanout {
	f := &Fanout{}
	list := make([]Sink, 0, len(sinks))
	for _, s := range sinks {
		if s != nil {
			list = append(list, s)
		}
	}
	f.sinks.Store(&list)
	return f
}

func (f *Fanout) snapshot() []Sink {
	if p := f.sinks.Load(); p != nil {
		return *p
	}
	return nil
}

// Add registers a sink. It reports false when the sink was already present.
func (f *Fanout) Add(s Sink) bool {
	if s == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cur := f.snapshot()
	for _, existing := range cur {
		if existing == s {
			return false
		}
	}
	next := make([]Sink, len(cur)+1)
	copy(next, cur)
	next[len(cur)] = s
	f.sinks.Store(&next)
	return true
}

// Remove unregisters a sink.
func (f *Fanout) Remove(s Sink) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur := f.snapshot()
	next := make([]Sink, 0, len(cur))
	for _, existing := range cur {
		if existing != s {
			next = append(next, existing)
		}
	}
	f.sinks.Store(&next)
}

// Push writes samples to every registered sink and returns the largest drop
// count any sink reported for this push.
func (f *Fanout) Push(samples []int16) int {
	dropped := 0
	for _, s := range f.snapshot() {
		if d := s.Push(samples); d > dropped {
			dropped = d
		}
	}
	return dropped
}

// KeepNewest trims every sink in the current snapshot and returns the largest
// count discarded by any sink. A push can land between two sinks' trims, so a
// start trim is best-effort; the counts are not added to Dropped.
func (f *Fanout) KeepNewest(newest int) int {
	dropped := 0
	for _, s := range f.snapshot() {
		if d := s.KeepNewest(newest); d > dropped {
			dropped = d
		}
	}
	return dropped
}

// Dropped reports the largest drop count across the registered sinks.
func (f *Fanout) Dropped() uint64 {
	var dropped uint64
	for _, s := range f.snapshot() {
		if d := s.Dropped(); d > dropped {
			dropped = d
		}
	}
	return dropped
}

// Reset clears every registered sink.
func (f *Fanout) Reset() {
	for _, s := range f.snapshot() {
		s.Reset()
	}
}
