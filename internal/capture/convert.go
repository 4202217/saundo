package capture

import (
	"fmt"
	"math"
)

// Sample tags as they appear in a WAVEFORMATEX format tag or in the first
// DWORD of a WAVEFORMATEXTENSIBLE SubFormat GUID.
const (
	tagPCM   uint16 = 1
	tagFloat uint16 = 3
)

// Sample kinds a converter can decode.
const (
	kindFloat32 uint8 = iota
	kindInt16
	kindInt24
	kindInt32
)

// converter reads interleaved device frames from a raw capture packet and
// writes interleaved stereo int16 samples. It is a pure value: all format
// validation happens once, in newConverter, at the parse boundary.
type converter struct {
	kind       uint8
	channels   int
	blockAlign int
}

// newConverter validates a parsed mix format and returns its converter.
func newConverter(tag uint16, bits, channels, blockAlign int) (converter, error) {
	var kind uint8
	switch {
	case tag == tagFloat && bits == 32:
		kind = kindFloat32
	case tag == tagPCM && bits == 16:
		kind = kindInt16
	case tag == tagPCM && bits == 24:
		kind = kindInt24
	case tag == tagPCM && bits == 32:
		kind = kindInt32
	default:
		return converter{}, fmt.Errorf("capture: unsupported sample format (tag %d, %d bits)", tag, bits)
	}
	if channels < 1 || channels > 8 {
		return converter{}, fmt.Errorf("capture: unsupported channel count %d", channels)
	}
	if blockAlign < channels*(bits/8) {
		return converter{}, fmt.Errorf("capture: inconsistent block align %d for %d channels of %d bits", blockAlign, channels, bits)
	}
	return converter{kind: kind, channels: channels, blockAlign: blockAlign}, nil
}

// convert writes one int16 stereo sample pair per input frame and returns the
// number of int16 samples written. The first two channels are kept, a mono
// channel is duplicated, and samples are clipped to int16. dst must be at
// least two samples per frame; src is read in blockAlign strides.
func (c converter) convert(dst []int16, src []byte) int {
	if c.channels == 0 || c.blockAlign == 0 {
		return 0 // the zero converter decodes nothing instead of dividing by zero
	}
	frames := len(src) / c.blockAlign
	if len(dst)/2 < frames {
		frames = len(dst) / 2
	}
	out := 0
	for i := 0; i < frames; i++ {
		base := i * c.blockAlign
		left := c.sample(src[base:])
		right := left
		if c.channels > 1 {
			right = c.sample(src[base+c.width():])
		}
		dst[out], dst[out+1] = left, right
		out += 2
	}
	return out
}

func (c converter) width() int {
	switch c.kind {
	case kindInt24:
		return 3
	case kindInt16:
		return 2
	default:
		return 4
	}
}

// sample decodes the little-endian sample at the start of b as int16.
func (c converter) sample(b []byte) int16 {
	switch c.kind {
	case kindFloat32:
		return floatTo16(leF32(b))
	case kindInt16:
		return int16(uint16(b[0]) | uint16(b[1])<<8)
	case kindInt24:
		v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		if b[2]&0x80 != 0 {
			v |= ^int32(0xffffff)
		}
		return int16(v >> 8)
	default: // kindInt32
		v := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		return int16(v >> 16)
	}
}

// floatTo16 scales an IEEE float sample to int16 by 32768 with round to
// nearest, clips out-of-range values, and maps NaN to zero.
func floatTo16(v float32) int16 {
	if math.IsNaN(float64(v)) {
		return 0
	}
	s := float64(v) * 32768
	if s >= 32767 {
		return 32767
	}
	if s <= -32768 {
		return -32768
	}
	return int16(math.Round(s))
}

func leF32(b []byte) float32 {
	return math.Float32frombits(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
}
