package alac

import (
	"math/bits"
	"testing"
)

// unread steps the reader back, mirroring the reference decoder's unreadbits.
func (r *bitReader) unread(n int) { r.bit -= n }

// entropyValue mirrors the reference ALAC decoder's entropy_decode_value: a
// unary prefix, then a k-bit remainder, with the escape form (a prefix longer
// than riceThreshold followed by a raw value).
func (r *bitReader) entropyValue(readSampleSize, k int, kmodifierMask uint32) int {
	x := 0
	for x <= riceThreshold && r.read(1) == 1 {
		x++
	}
	if x > riceThreshold {
		return int(r.read(readSampleSize) & (0xFFFFFFFF >> uint(32-readSampleSize)))
	}
	if k != 1 {
		extra := int(r.read(k))
		x *= int(uint32((1<<uint(k))-1) & kmodifierMask)
		if extra > 1 {
			x += extra - 1
		} else {
			r.unread(1)
		}
	}
	return x
}

// rice mirrors the reference decoder's entropy_rice_decode, including the
// zero-run case that makes a silent block small.
func (r *bitReader) rice(out []int, readSampleSize, initialHistory, kmodifier, historyMult, kmodifierMask int) {
	history := initialHistory
	signModifier := 0
	for i := 0; i < len(out); i++ {
		k := 31 - kmodifier - bits.LeadingZeros32(uint32((history>>9)+3))
		if k < 0 {
			k += kmodifier
		} else {
			k = kmodifier
		}
		decoded := r.entropyValue(readSampleSize, k, 0xFFFFFFFF) + signModifier
		final := (decoded + 1) / 2
		if decoded&1 != 0 {
			final = -final
		}
		out[i] = final
		signModifier = 0
		history += decoded*historyMult - ((history * historyMult) >> 9)
		if decoded > 0xFFFF {
			history = 0xFFFF
		}
		if history < 128 && i+1 < len(out) {
			signModifier = 1
			k = bits.LeadingZeros32(uint32(history)) + (history+16)/64 - 24
			block := r.entropyValue(16, k, uint32(kmodifierMask))
			for j := 1; j <= block && i+j < len(out); j++ {
				out[i+j] = 0
			}
			i += block
			if block > 0xFFFF {
				signModifier = 0
			}
			history = 0
		}
	}
}

// decodeCompact runs the reference decoder over one compressed CPE frame and
// returns its two channels plus how many bits the frame used. The frame is
// required to use the layout EncodeCompactSilence writes: interlace 0/0,
// prediction order 0.
func decodeCompact(t *testing.T, frame []byte) (left, right []int, used int) {
	t.Helper()
	r := bitReader{data: frame}
	if tag := r.read(3); tag != 1 {
		t.Fatalf("channel tag = %d, want 1", tag)
	}
	r.read(4)
	r.read(12)
	if r.read(1) != 0 {
		t.Fatal("frame carries its own sample size")
	}
	if r.read(2) != 0 {
		t.Fatal("frame carries wasted bytes")
	}
	if r.read(1) != 0 {
		t.Fatal("frame is not compressed")
	}
	if shift := r.read(8); shift != 0 {
		t.Fatalf("interlace shift = %d, want 0", shift)
	}
	if weight := r.read(8); weight != 0 {
		t.Fatalf("interlace leftweight = %d, want 0", weight)
	}

	const (
		sampleSize     = 16
		initialHistory = 10
		kmodifier      = 14
		historyMult    = 40
	)
	riceMods := make([]int, Channels)
	for i := range riceMods {
		if got := r.read(4); got != 0 {
			t.Fatalf("channel %d prediction type = %d, want 0", i, got)
		}
		r.read(4) // quantization
		riceMods[i] = int(r.read(3))
		order := r.read(5)
		if order != 0 {
			t.Fatalf("channel %d predictor order = %d, want 0", i, order)
		}
	}

	// CPE readsamplesize: the sample size plus the sign bit, with no
	// uncompressed bytes.
	readSampleSize := sampleSize + 1
	mask := (1 << kmodifier) - 1
	left = make([]int, FramesPerPacket)
	right = make([]int, FramesPerPacket)
	r.rice(left, readSampleSize, initialHistory, kmodifier, riceMods[0]*historyMult/4, mask)
	r.rice(right, readSampleSize, initialHistory, kmodifier, riceMods[1]*historyMult/4, mask)
	if tag := r.read(3); tag != 7 {
		t.Fatalf("END tag = %d, want 7", tag)
	}
	return left, right, r.bit
}

// TestCompactSilenceDecodesToZeroFrames pins the frame the sender uses for a
// silent fill: the reference ALAC decoder must expand it to 352 zero frames
// per channel, and it must stay small enough to make a silent stream cheap.
func TestCompactSilenceDecodesToZeroFrames(t *testing.T) {
	frame := EncodeCompactSilence()
	if len(frame) != 16 {
		t.Fatalf("compact frame = %d bytes, want 16", len(frame))
	}
	left, right, used := decodeCompact(t, frame)
	if used != 126 {
		t.Fatalf("frame used %d bits, want 126 (the rest is byte padding)", used)
	}
	nonZero := 0
	for i := range left {
		if left[i] != 0 || right[i] != 0 {
			nonZero++
		}
	}
	if nonZero != 0 {
		t.Fatalf("%d of %d frames decoded non-zero", nonZero, FramesPerPacket)
	}
}
