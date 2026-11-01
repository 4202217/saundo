// Package ring provides the bounded, overwrite-oldest audio buffer between
// the capture goroutine and the sender.
package ring

import "sync"

// Buffer holds interleaved int16 stereo samples. One producer pushes and one
// consumer pops. When the buffer is full, Push discards the oldest samples so
// capture never blocks on a stalled network.
type Buffer struct {
	mu      sync.Mutex
	buf     []int16
	r, w    int
	count   int
	dropped uint64
}

// New returns a buffer that holds capacity samples. A capacity below two is
// raised to two.
func New(capacity int) *Buffer {
	if capacity < 2 {
		capacity = 2
	}
	return &Buffer{buf: make([]int16, capacity)}
}

// Capacity is the number of interleaved samples the buffer holds.
func (b *Buffer) Capacity() int { return len(b.buf) }

// Available is the number of samples ready to pop.
func (b *Buffer) Available() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

// Dropped reports how many samples have been discarded since the last Reset.
func (b *Buffer) Dropped() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

// Push appends samples and returns how many older samples it discarded to make
// room. Samples longer than the buffer keep only the newest capacity samples.
func (b *Buffer) Push(samples []int16) int {
	if len(samples) == 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	droppedNow := 0
	if len(samples) > len(b.buf) {
		cut := len(samples) - len(b.buf)
		b.dropped += uint64(b.count + cut)
		b.count = 0
		b.r, b.w = 0, 0
		samples = samples[cut:]
		droppedNow += cut
	}
	free := len(b.buf) - b.count
	drop := len(samples) - free
	if drop > 0 {
		b.r = (b.r + drop) % len(b.buf)
		b.count -= drop
		b.dropped += uint64(drop)
		droppedNow += drop
	}
	n := copy(b.buf[b.w:], samples)
	if n < len(samples) {
		copy(b.buf, samples[n:])
	}
	b.w = (b.w + len(samples)) % len(b.buf)
	b.count += len(samples)
	return droppedNow
}

// Pop copies up to len(dst) samples into dst and returns the count.
func (b *Buffer) Pop(dst []int16) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(dst)
	if n > b.count {
		n = b.count
	}
	if n == 0 {
		return 0
	}
	copied := copy(dst, b.buf[b.r:])
	if copied < n {
		copy(dst[copied:], b.buf[:])
	}
	b.r = (b.r + n) % len(b.buf)
	b.count -= n
	return n
}

// KeepNewest discards the oldest samples so at most newest remain, keeping the
// newest edge. It returns the number discarded. A negative or oversized newest
// is clamped to zero or the current count.
//
// The discarded samples are deliberately not added to Dropped: a start trim is
// a policy decision, not capture overflow, so the UI's dropped figure keeps
// meaning "capture overflow".
func (b *Buffer) KeepNewest(newest int) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if newest < 0 {
		newest = 0
	}
	if newest >= b.count {
		return 0
	}
	drop := b.count - newest
	b.r = (b.r + drop) % len(b.buf)
	b.count = newest
	return drop
}

// Reset empties the buffer and clears the drop counter.
func (b *Buffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.r, b.w, b.count, b.dropped = 0, 0, 0, 0
}
