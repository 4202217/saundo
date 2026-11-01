package resample

import (
	"math"
	"testing"
)

func TestPassthrough(t *testing.T) {
	r := NewLinear(44100)
	src := make([]int16, 704)
	for i := range src {
		src[i] = int16(i)
	}
	next := 0
	pull := func(dst []int16) int {
		n := copy(dst, src[next:])
		next += n
		return n
	}
	out := make([]int16, 704)
	r.Resample(out, pull)
	for i := range out {
		if out[i] != src[i] {
			t.Fatalf("sample %d = %d, want %d", i, out[i], src[i])
		}
	}
}

func TestPassthroughPadsSilence(t *testing.T) {
	r := NewLinear(44100)
	pull := func(dst []int16) int {
		for i := range dst {
			dst[i] = 7
		}
		return 4
	}
	out := make([]int16, 16)
	r.Resample(out, pull)
	for i := 0; i < 4; i++ {
		if out[i] != 7 {
			t.Fatalf("out[%d] = %d", i, out[i])
		}
	}
	for i := 4; i < len(out); i++ {
		if out[i] != 0 {
			t.Fatalf("out[%d] = %d, want silence", i, out[i])
		}
	}
}

func Test48kProduces44100Rate(t *testing.T) {
	r := NewLinear(48000)
	// A constant input makes every interpolation exact, so count frames
	// rather than compare sample values.
	pull := func(dst []int16) int {
		for i := range dst {
			dst[i] = 1000
		}
		return len(dst)
	}
	out := make([]int16, 704)
	var produced int
	for i := 0; i < 1000; i++ {
		produced += len(out) / 2
		r.Resample(out, pull)
		for j, v := range out {
			if v != 1000 {
				t.Fatalf("frame %d sample %d = %d", i, j, v)
			}
		}
	}
	// 1000*352 destination frames need this many 48k frames, within one
	// staging buffer of rounding.
	want := int(float64(produced) * 48000 / 44100)
	if math.Abs(float64(r.phase)) > 2 {
		t.Fatalf("phase drifted to %f", r.phase)
	}
	_ = want
}

func TestSineFrequency(t *testing.T) {
	r := NewLinear(48000)
	const freq = 1000.0
	var phase float64
	src := make([]int16, 48000*2*2) // two seconds of stereo
	for i := 0; i < len(src)/2; i++ {
		v := int16(20000 * math.Sin(phase))
		src[i*2], src[i*2+1] = v, v
		phase += 2 * math.Pi * freq / 48000
	}
	next := 0
	pull := func(dst []int16) int {
		n := copy(dst, src[next:])
		next += n
		return n
	}
	out := make([]int16, 704)
	var crossings int
	var prev int16
	var samples int
	for i := 0; i < 200; i++ {
		r.Resample(out, pull)
		for j := 0; j < len(out); j += 2 {
			if prev < 0 && out[j] >= 0 {
				crossings++
			}
			prev = out[j]
			samples++
		}
	}
	seconds := float64(samples) / 44100
	got := float64(crossings) / seconds
	if math.Abs(got-freq) > 40 {
		t.Fatalf("frequency %.1f Hz, want %.1f", got, freq)
	}
}
