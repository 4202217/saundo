package capture

import (
	"math"
	"testing"
)

func mustConverter(t *testing.T, tag uint16, bits, channels, blockAlign int) converter {
	t.Helper()
	c, err := newConverter(tag, bits, channels, blockAlign)
	if err != nil {
		t.Fatalf("newConverter(tag %d, %d bits, %d ch, align %d): %v", tag, bits, channels, blockAlign, err)
	}
	return c
}

func leBytes(v uint32, n int) []byte {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte(v >> (8 * i))
	}
	return b
}

func TestConvertFloat32(t *testing.T) {
	c := mustConverter(t, tagFloat, 32, 2, 8)
	var src []byte
	var want []int16
	for _, tc := range []struct {
		in   float32
		want int16
	}{
		{0, 0},
		{0.5, 16384},
		{-0.5, -16384},
		{0.25, 8192},
		{1, 32767},     // 32768 clips
		{-1, -32768},   // exact low end
		{1.5, 32767},   // clip high
		{-1.5, -32768}, // clip low
		{float32(math.NaN()), 0},
		{float32(math.Inf(1)), 32767},
		{float32(math.Inf(-1)), -32768},
	} {
		src = append(src, leBytes(math.Float32bits(tc.in), 4)...)
		src = append(src, leBytes(math.Float32bits(tc.in), 4)...)
		want = append(want, tc.want, tc.want)
	}
	dst := make([]int16, len(want))
	if n := c.convert(dst, src); n != len(want) {
		t.Fatalf("convert wrote %d samples, want %d", n, len(want))
	}
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("sample %d = %d, want %d", i, dst[i], want[i])
		}
	}
}

func TestConvertInt16(t *testing.T) {
	c := mustConverter(t, tagPCM, 16, 2, 4)
	src := leBytes(uint32(uint16(0x8000)), 2) // -32768
	src = append(src, leBytes(0x7FFF, 2)...)
	src = append(src, leBytes(0x1234, 2)...)
	src = append(src, leBytes(0, 2)...)
	dst := make([]int16, 4)
	if n := c.convert(dst, src); n != 4 {
		t.Fatalf("convert wrote %d samples, want 4", n)
	}
	want := []int16{-32768, 32767, 0x1234, 0}
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("sample %d = %d, want %d", i, dst[i], want[i])
		}
	}
}

func TestConvertInt24(t *testing.T) {
	c := mustConverter(t, tagPCM, 24, 2, 6)
	// Each frame: left, right. -8388608, 8388607, 65536 (= 256 in int16).
	frames := [][]byte{
		{0x00, 0x00, 0x80, 0x00, 0x00, 0x80},
		{0xFF, 0xFF, 0x7F, 0xFF, 0xFF, 0x7F},
		{0x00, 0x00, 0x01, 0x00, 0x00, 0x01},
	}
	var src []byte
	for _, f := range frames {
		src = append(src, f...)
	}
	dst := make([]int16, 6)
	if n := c.convert(dst, src); n != 6 {
		t.Fatalf("convert wrote %d samples, want 6", n)
	}
	want := []int16{-32768, -32768, 32767, 32767, 256, 256}
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("sample %d = %d, want %d", i, dst[i], want[i])
		}
	}
}

func TestConvertInt32(t *testing.T) {
	c := mustConverter(t, tagPCM, 32, 2, 8)
	var src []byte
	for _, v := range []uint32{0x80000000, 0x7FFFFFFF, 0x00010000, 0} {
		src = append(src, leBytes(v, 4)...)
		src = append(src, leBytes(v, 4)...)
	}
	dst := make([]int16, 8)
	if n := c.convert(dst, src); n != 8 {
		t.Fatalf("convert wrote %d samples, want 8", n)
	}
	want := []int16{-32768, -32768, 32767, 32767, 1, 1, 0, 0}
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("sample %d = %d, want %d", i, dst[i], want[i])
		}
	}
}

func TestConvertMonoDuplicate(t *testing.T) {
	c := mustConverter(t, tagFloat, 32, 1, 4)
	src := leBytes(math.Float32bits(0.25), 4)
	dst := make([]int16, 2)
	if n := c.convert(dst, src); n != 2 {
		t.Fatalf("convert wrote %d samples, want 2", n)
	}
	if dst[0] != 8192 || dst[1] != 8192 {
		t.Errorf("mono frame converted to [%d, %d], want [8192, 8192]", dst[0], dst[1])
	}
}

func TestConvertExtraChannels(t *testing.T) {
	c := mustConverter(t, tagPCM, 16, 4, 8)
	src := leBytes(100, 2)
	src = append(src, leBytes(uint32(uint16(0xFF38)), 2)...) // -200
	src = append(src, leBytes(300, 2)...)
	src = append(src, leBytes(400, 2)...)
	dst := make([]int16, 2)
	if n := c.convert(dst, src); n != 2 {
		t.Fatalf("convert wrote %d samples, want 2", n)
	}
	if dst[0] != 100 || dst[1] != -200 {
		t.Errorf("4-channel frame converted to [%d, %d], want [100, -200]", dst[0], dst[1])
	}
}

func TestConvertBounds(t *testing.T) {
	c := mustConverter(t, tagFloat, 32, 2, 8)

	// A partial frame decodes nothing.
	dst := make([]int16, 2)
	if n := c.convert(dst, make([]byte, 7)); n != 0 {
		t.Errorf("partial frame wrote %d samples, want 0", n)
	}
	if n := c.convert(dst, nil); n != 0 {
		t.Errorf("empty src wrote %d samples, want 0", n)
	}

	// dst limits the frame count.
	src := make([]byte, 16) // two frames
	if n := c.convert(make([]int16, 2), src); n != 2 {
		t.Errorf("small dst wrote %d samples, want 2", n)
	}

	// A padded stride keeps frames apart.
	padded := mustConverter(t, tagPCM, 16, 2, 6)
	src = leBytes(1, 2)
	src = append(src, leBytes(2, 2)...)
	src = append(src, 0xFF, 0xFF) // padding bytes inside the first stride
	src = append(src, leBytes(3, 2)...)
	src = append(src, leBytes(4, 2)...)
	src = append(src, 0xFF, 0xFF)
	dst = make([]int16, 4)
	if n := padded.convert(dst, src); n != 4 {
		t.Fatalf("padded stride wrote %d samples, want 4", n)
	}
	want := []int16{1, 2, 3, 4}
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("padded stride sample %d = %d, want %d", i, dst[i], want[i])
		}
	}
}

func TestNewConverterRejects(t *testing.T) {
	for _, tc := range []struct {
		tag                   uint16
		bits, channels, align int
	}{
		{2, 16, 2, 4},        // unknown tag
		{tagPCM, 8, 2, 2},    // unsupported width
		{tagFloat, 16, 2, 4}, // float only comes in 32 bits
		{tagPCM, 16, 0, 2},   // no channels
		{tagPCM, 16, 9, 18},  // too many channels
		{tagPCM, 16, 2, 3},   // align below two samples
	} {
		if c, err := newConverter(tc.tag, tc.bits, tc.channels, tc.align); err == nil {
			t.Errorf("newConverter(tag %d, %d bits, %d ch, align %d) = %+v, want error", tc.tag, tc.bits, tc.channels, tc.align, c)
		}
	}
}
