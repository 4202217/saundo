//go:build darwin

package capture

import (
	"runtime"
	"sync"
	"testing"
	"unsafe"

	"saundo/internal/audio/ring"
	"saundo/internal/darwin"
)

func testIOBuffer(frames int) (*darwin.AudioBufferList, []float32) {
	raw := make([]float32, frames*2)
	for i := range raw {
		raw[i] = 0.25
	}
	abl := &darwin.AudioBufferList{
		NumberBuffers: 1,
		Buffers: [1]darwin.AudioBuffer{
			{
				NumberChannels: 2,
				DataByteSize:   uint32(len(raw) * 4),
				Data:           unsafe.Pointer(&raw[0]),
			},
		},
	}
	return abl, raw
}

func testStream(t *testing.T, scratch int) (*stream, *Source, *ring.Fanout) {
	t.Helper()
	conv, err := newConverter(tagFloat, 32, 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	sink := ring.NewFanout(ring.New(4096))
	src := &Source{cfg: Config{Ring: sink}}
	return &stream{src: src, conv: conv, scratch: make([]int16, scratch)}, src, sink
}

// TestIOProcPushDoesNotAllocate pins the realtime contract: the Core Audio
// IOProc path converts, rings, and counts without taking a lock or allocating.
func TestIOProcPushDoesNotAllocate(t *testing.T) {
	abl, raw := testIOBuffer(480)
	st, _, _ := testStream(t, len(makeScratch(48000)))
	st.push(abl)
	if got := testing.AllocsPerRun(1000, func() { st.push(abl) }); got != 0 {
		t.Fatalf("IOProc push allocates %.1f times per run", got)
	}
	runtime.KeepAlive(raw)
}

// TestIOProcOversizedBufferIsDropped keeps an implausibly large IO buffer from
// growing the scratch on the audio thread.
func TestIOProcOversizedBufferIsDropped(t *testing.T) {
	abl, raw := testIOBuffer(480)
	st, src, sink := testStream(t, 16)
	st.push(abl)
	if got := src.Stats().Dropped; got != 960 {
		t.Fatalf("dropped = %d, want 960 samples", got)
	}
	if got := sink.Dropped(); got != 0 {
		t.Fatalf("ring reported %d drops for a buffer that never reached it", got)
	}
	runtime.KeepAlive(raw)
}

// TestIOProcPushAndStatsRace runs the audio-thread push against the control
// plane's Stats; with -race it fails if frames or the ring still share state.
func TestIOProcPushAndStatsRace(t *testing.T) {
	abl, raw := testIOBuffer(480)
	st, src, _ := testStream(t, len(makeScratch(48000)))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 5000; i++ {
			st.push(abl)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 5000; i++ {
			_ = src.Stats()
		}
	}()
	wg.Wait()
	if src.Stats().Frames == 0 {
		t.Fatal("no frames were counted")
	}
	runtime.KeepAlive(raw)
}
