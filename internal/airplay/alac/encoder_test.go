package alac

import "testing"

type bitReader struct {
	data []byte
	bit  int
}

func (r *bitReader) read(n int) uint32 {
	var value uint32
	for i := 0; i < n; i++ {
		byteIndex := r.bit / 8
		bitIndex := 7 - r.bit%8
		value = value<<1 | uint32(r.data[byteIndex]>>uint(bitIndex)&1)
		r.bit++
	}
	return value
}

func (r *bitReader) remaining() int {
	return len(r.data)*8 - r.bit
}

func TestEncodeUncompressedLayout(t *testing.T) {
	samples := make([]int16, SamplesPerPacket)
	for i := 0; i < FramesPerPacket; i++ {
		samples[2*i] = int16(i - 176)
		samples[2*i+1] = int16(176 - i)
	}
	samples[0] = -32768
	samples[1] = 32767

	frame, err := EncodeUncompressed(samples)
	if err != nil {
		t.Fatalf("EncodeUncompressed: %v", err)
	}
	if len(frame) != FrameBytes {
		t.Fatalf("frame length = %d, want %d", len(frame), FrameBytes)
	}

	r := &bitReader{data: frame}
	fields := []struct {
		name string
		bits int
		want uint32
	}{
		{"channel tag", 3, 1},
		{"unused", 4, 0},
		{"unknown", 12, 0},
		{"hasSize", 1, 0},
		{"wastedBytes", 2, 0},
		{"isNotCompressed", 1, 1},
	}
	for _, field := range fields {
		if got := r.read(field.bits); got != field.want {
			t.Fatalf("%s = %d, want %d", field.name, got, field.want)
		}
	}
	for i, sample := range samples {
		if got := r.read(16); got != uint32(uint16(sample)) {
			t.Fatalf("sample %d = %d, want %d", i, got, uint16(sample))
		}
	}
	if got := r.read(3); got != 7 {
		t.Fatalf("END tag = %d, want 7", got)
	}
	for r.remaining() > 0 {
		if got := r.read(1); got != 0 {
			t.Fatalf("padding bit = %d, want 0", got)
		}
	}
}

func TestEncodeUncompressedRejectsWrongSampleCount(t *testing.T) {
	for _, length := range []int{0, 1, SamplesPerPacket - 1, SamplesPerPacket + 1} {
		if _, err := EncodeUncompressed(make([]int16, length)); err == nil {
			t.Fatalf("length %d: expected error", length)
		}
	}
}

func TestEncodeSilence(t *testing.T) {
	frame := EncodeSilence()
	if len(frame) != FrameBytes {
		t.Fatalf("frame length = %d, want %d", len(frame), FrameBytes)
	}
}
