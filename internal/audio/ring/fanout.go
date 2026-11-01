package ring

import "sync"

// Sink is the producer-facing side of an audio buffer. A stereo-pair session
// feeds several member rings from one capture stream.
type Sink interface {
	Push(samples []int16) int
	Dropped() uint64
	Reset()
	KeepNewest(newest int) int
}

// Trimmer is the buffer-side capability a start gate needs. *Buffer trims one
// stream; *Fanout trims every stream sharing one capture source atomically.
type Trimmer interface {
	KeepNewest(newest int) int
}

// Fanout writes every push to all registered sinks. Sinks can be added and
// removed while capture is running; a push is never split across a change.
type Fanout struct {
	mu    sync.Mutex
	sinks []Sink
}

// NewFanout returns a fanout over the given sinks.
func NewFanout(sinks ...Sink) *Fanout {
	f := &Fanout{}
	for _, s := range sinks {
		if s != nil {
			f.sinks = append(f.sinks, s)
		}
	}
	return f
}

// Add registers a sink. It reports false when the sink was already present.
func (f *Fanout) Add(s Sink) bool {
	if s == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.sinks {
		if existing == s {
			return false
		}
	}
	f.sinks = append(f.sinks, s)
	return true
}

// Remove unregisters a sink.
func (f *Fanout) Remove(s Sink) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.sinks[:0]
	for _, existing := range f.sinks {
		if existing != s {
			out = append(out, existing)
		}
	}
	f.sinks = out
}

// Push writes samples to every registered sink and returns the largest drop
// count any sink reported for this push.
func (f *Fanout) Push(samples []int16) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	dropped := 0
	for _, s := range f.sinks {
		if d := s.Push(samples); d > dropped {
			dropped = d
		}
	}
	return dropped
}

// KeepNewest trims every registered sink under the same lock Push uses, so a
// capture push can never land between two sinks' trims. It returns the largest
// count discarded by any sink. The counts are not added to Dropped.
func (f *Fanout) KeepNewest(newest int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	dropped := 0
	for _, s := range f.sinks {
		if d := s.KeepNewest(newest); d > dropped {
			dropped = d
		}
	}
	return dropped
}

// Dropped reports the largest drop count across the registered sinks.
func (f *Fanout) Dropped() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var dropped uint64
	for _, s := range f.sinks {
		if d := s.Dropped(); d > dropped {
			dropped = d
		}
	}
	return dropped
}

// Reset clears every registered sink.
func (f *Fanout) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sinks {
		s.Reset()
	}
}
