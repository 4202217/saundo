// Package ring provides the bounded, overwrite-oldest audio buffer between
// the capture thread and the sender.
package ring

import "sync/atomic"

// Buffer holds interleaved int16 stereo samples. One producer pushes and one
// consumer pops; the indices are atomic, and the samples are stored in atomic
// slots, so Push never locks or allocates on a realtime audio thread. When the
// buffer is full, Push discards the oldest samples so capture never blocks.
type Buffer struct {
	slots   []atomic.Int32
	read    atomic.Uint64
	write   atomic.Uint64
	dropped atomic.Uint64
}

// New returns a buffer that holds capacity samples. A capacity below two is
// raised to two.
func New(capacity int) *Buffer {
	if capacity < 2 {
		capacity = 2
	}
	return &Buffer{slots: make([]atomic.Int32, capacity)}
}

// Capacity is the number of interleaved samples the buffer holds.
func (b *Buffer) Capacity() int { return len(b.slots) }

// Available is the number of samples ready to pop.
func (b *Buffer) Available() int {
	w, r := b.write.Load(), b.read.Load()
	if w <= r {
		return 0
	}
	n := w - r
	if n > uint64(len(b.slots)) {
		n = uint64(len(b.slots))
	}
	return int(n)
}

// Dropped reports how many samples have been discarded since the last Reset.
func (b *Buffer) Dropped() uint64 { return b.dropped.Load() }

func (b *Buffer) slot(i uint64) *atomic.Int32 {
	return &b.slots[i%uint64(len(b.slots))]
}

// Push appends samples and returns how many older samples it discarded to make
// room. Samples longer than the buffer keep only the newest capacity samples.
// It is safe on a realtime thread: it takes no lock and allocates nothing.
func (b *Buffer) Push(samples []int16) int {
	if len(samples) == 0 {
		return 0
	}
	capacity := uint64(len(b.slots))
	w := b.write.Load()
	n := uint64(len(samples))

	if n >= capacity {
		// Keep the newest capacity samples. The read jumps to the first kept
		// sample; every older sample and the head of this push is discarded.
		keep := samples[len(samples)-int(capacity):]
		first := w + n - capacity
		var dropped int
		for {
			r := b.read.Load()
			if first <= r {
				break
			}
			if b.read.CompareAndSwap(r, first) {
				b.dropped.Add(first - r)
				dropped = int(first - r)
				break
			}
		}
		for i, s := range keep {
			b.slot(first + uint64(i)).Store(int32(s))
		}
		b.write.Store(w + n)
		return dropped
	}

	var dropped int
	for {
		r := b.read.Load()
		count := w - r
		if count+n <= capacity {
			break
		}
		drop := count + n - capacity
		if b.read.CompareAndSwap(r, r+drop) {
			b.dropped.Add(drop)
			dropped += int(drop)
		}
	}
	for i, s := range samples {
		b.slot(w + uint64(i)).Store(int32(s))
	}
	b.write.Store(w + n)
	return dropped
}

// Pop copies up to len(dst) samples into dst and returns the count. It is
// lock-free; it returns 0 instead of retrying when a concurrent Push had to
// overwrite the window it was copying.
func (b *Buffer) Pop(dst []int16) int {
	if len(dst) == 0 {
		return 0
	}
	r := b.read.Load()
	w := b.write.Load()
	if w <= r {
		return 0
	}
	n := w - r
	if n > uint64(len(dst)) {
		n = uint64(len(dst))
	}
	for i := uint64(0); i < n; i++ {
		dst[i] = int16(b.slot(r + i).Load())
	}
	if !b.read.CompareAndSwap(r, r+n) {
		return 0
	}
	return int(n)
}

// KeepNewest discards the oldest samples so at most newest remain, keeping the
// newest edge. It returns the number discarded. A negative or oversized newest
// is clamped to zero or the current count.
//
// The discarded samples are deliberately not added to Dropped: a start trim is
// a policy decision, not capture overflow, so the UI's dropped figure keeps
// meaning "capture overflow".
func (b *Buffer) KeepNewest(newest int) int {
	if newest < 0 {
		newest = 0
	}
	for {
		r, w := b.read.Load(), b.write.Load()
		if w <= r {
			return 0
		}
		count := w - r
		if count > uint64(len(b.slots)) {
			count = uint64(len(b.slots))
		}
		if uint64(newest) >= count {
			return 0
		}
		drop := count - uint64(newest)
		if b.read.CompareAndSwap(r, r+drop) {
			return int(drop)
		}
	}
}

// Reset empties the buffer and clears the drop counter.
func (b *Buffer) Reset() {
	b.read.Store(b.write.Load())
	b.dropped.Store(0)
}
