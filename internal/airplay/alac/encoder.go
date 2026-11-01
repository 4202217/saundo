// Package alac encodes the uncompressed ALAC frames used by the AirPlay 2
// realtime stream.
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
	w.write(1, 3)  // stereo channel-pair element
	w.write(0, 4)  // unused
	w.write(0, 12) // unknown
	w.write(0, 1)  // hasSize = 0
	w.write(0, 2)  // wastedBytes = 0
	w.write(1, 1)  // isNotCompressed = 1
	for _, sample := range samples {
		w.write(uint32(uint16(sample)), 16)
	}
	w.write(7, 3) // END element
	w.flush()
	return w.buf, nil
}

// EncodeSilence returns a frame of silence.
func EncodeSilence() []byte {
	frame, _ := EncodeUncompressed(make([]int16, SamplesPerPacket))
	return frame
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
