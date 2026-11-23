// Package alac encodes the ALAC frames used by the AirPlay 2 realtime stream:
// uncompressed frames for real audio and one compact compressed frame for a
// silent fill.
package alac

import "fmt"

const (
	// FramesPerPacket is the RAOP realtime frame size.
	FramesPerPacket = 352
	// Channels is the interleaving of the realtime stream.
	Channels = 2
	// SamplesPerPacket is the number of interleaved int16 samples in one frame.
	SamplesPerPacket = FramesPerPacket * Channels
	// FrameBytes is the size of one uncompressed ALAC frame at this format.
	FrameBytes = 1412
)

// EncodeUncompressed encodes 352 interleaved stereo int16 samples as the
// uncompressed ALAC frame a modern AirPlay 2 receiver expects on the realtime
// stream. Layout, MSB-first:
//
//	3b  channel tag = 1 (stereo CPE)
//	4b  0
//	12b 0
//	1b  hasSize = 0 (the cookie's default 352)
//	2b  wastedBytes = 0
//	1b  isNotCompressed = 1
//	352 x {L16, R16}
//	3b  END tag = 7
//
// The frame is padded to a byte boundary.
func EncodeUncompressed(samples []int16) ([]byte, error) {
	if len(samples) != SamplesPerPacket {
		return nil, fmt.Errorf("alac: need %d interleaved samples, got %d", SamplesPerPacket, len(samples))
	}
	w := bitWriter{buf: make([]byte, 0, FrameBytes)}
	w.write(1, 3)
	w.write(0, 4)
	w.write(0, 12)
	w.write(0, 1)
	w.write(0, 2)
	w.write(1, 1)
	for _, sample := range samples {
		w.write(uint32(uint16(sample)), 16)
	}
	w.write(7, 3)
	w.flush()
	return w.buf, nil
}

// EncodeSilence returns a frame of silence.
func EncodeSilence() []byte {
	frame, _ := EncodeUncompressed(make([]int16, SamplesPerPacket))
	return frame
}

// riceThreshold is the ALAC rice coder's longest unary prefix. A value that
// would need more ones is sent as the escape form: the prefix plus a raw
// 16-bit count.
const riceThreshold = 8

// compactSilence is the compressed all-zero frame, built once.
var compactSilence = buildCompactSilence()

// EncodeCompactSilence returns the compressed ALAC frame for one packet of
// digital silence: 352 zero frames in 16 bytes instead of the uncompressed
// frame's 1412. The receiver's ALAC decoder expands it through the rice
// coder's zero-run case, so a silent stream keeps the receiver's realtime
// timeline fed at a fraction of the uncompressed bandwidth.
//
// Layout, MSB-first, with the SDP cookie's 352-frame length and 16-bit
// stereo samples:
//
//	3b  channel tag = 1 (stereo CPE)
//	4b  0
//	12b 0
//	1b  hasSize = 0 (the cookie's default 352)
//	2b  wastedBytes = 0
//	1b  isNotCompressed = 0
//	8b  interlace shift = 0
//	8b  interlace leftweight = 0
//	2 x {4b prediction type 0, 4b quantization 0, 3b rice modifier 4, 5b order 0}
//	2 x {1b first residual 0, escape prefix, 16b zero-run count 351}
//	3b  END tag = 7
//
// The returned slice is shared and read-only; a caller that needs to modify
// a frame must copy it.
func EncodeCompactSilence() []byte { return compactSilence }

func buildCompactSilence() []byte {
	w := bitWriter{buf: make([]byte, 0, 16)}
	w.write(1, 3)  // stereo CPE
	w.write(0, 4)  // instance
	w.write(0, 12) // unused
	w.write(0, 1)  // hasSize: the cookie carries 352
	w.write(0, 2)  // wasted bytes
	w.write(0, 1)  // compressed
	w.write(0, 8)  // interlace shift
	w.write(0, 8)  // interlace leftweight
	for ch := 0; ch < Channels; ch++ {
		w.write(0, 4) // prediction type: adaptive FIR
		w.write(0, 4) // quantization
		w.write(4, 3) // rice modifier, scaled against the cookie's 40
		w.write(0, 5) // predictor order 0: the residual is the sample
	}
	for ch := 0; ch < Channels; ch++ {
		// The first residual is a plain zero. Its rice parameter comes out
		// as 1 (history 10, kmodifier 14), which costs one 0 bit.
		w.write(0, 1)
		// With history under 128 the decoder then reads a run of zeros. The
		// count 351 needs the escape form: riceThreshold+1 ones, then the
		// raw 16-bit value.
		for i := 0; i <= riceThreshold; i++ {
			w.write(1, 1)
		}
		w.write(FramesPerPacket-1, 16)
	}
	w.write(7, 3) // END
	w.flush()
	return w.buf
}

type bitWriter struct {
	buf []byte
	cur byte
	n   uint8
}

func (w *bitWriter) write(value uint32, bits uint8) {
	for i := int(bits) - 1; i >= 0; i-- {
		w.cur = w.cur<<1 | byte(value>>uint(i)&1)
		w.n++
		if w.n == 8 {
			w.buf = append(w.buf, w.cur)
			w.cur, w.n = 0, 0
		}
	}
}

func (w *bitWriter) flush() {
	if w.n == 0 {
		return
	}
	w.buf = append(w.buf, w.cur<<(8-w.n))
	w.cur, w.n = 0, 0
}
