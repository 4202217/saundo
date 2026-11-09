// Package resample converts interleaved stereo int16 frames from the capture
// device rate to the RAOP rate with linear interpolation.
package resample

// RaopRate is the fixed AirPlay realtime sample rate.
const RaopRate = 44100

// maxStagingFrames bounds the working buffer; the ring is the deep buffer.
const maxStagingFrames = 8192

// Linear resamples one stream. It is not safe for concurrent use.
type Linear struct {
	inputRate int
	phase     float64
	in        []int16
}

// NewLinear returns a resampler for the given input rate. A non-positive rate
// is treated as 48000, the Windows shared-mode default.
func NewLinear(inputRate int) *Linear {
	r := &Linear{}
	r.SetInputRate(inputRate)
	return r
}

// SetInputRate changes the source rate. The staged samples keep their own rate,
// so callers change this only while the stream is restarted.
func (r *Linear) SetInputRate(rate int) {
	if rate <= 0 {
		rate = 48000
	}
	if rate != r.inputRate {
		r.phase = 0
		r.in = r.in[:0]
	}
	r.inputRate = rate
}

// InputRate reports the configured source rate.
func (r *Linear) InputRate() int { return r.inputRate }

// Resample fills dst with exactly len(dst)/2 stereo frames at 44100 Hz. pull
// supplies interleaved source samples and returns how many it wrote; when it
// has nothing, the remaining destination frames are silence so the receiver
// timeline keeps moving. It returns the number of destination frames produced
// from real input; a short return means the rest of dst was padded with
// silence.
//
// dst length must be even. A zero-length dst is a no-op.
func (r *Linear) Resample(dst []int16, pull func([]int16) int) int {
	want := len(dst) / 2
	if want == 0 {
		return 0
	}
	if r.inputRate == RaopRate {
		n := pull(dst)
		if frames := n / 2; frames < want {
			clear(dst[frames*2:])
		}
		return n / 2
	}

	step := float64(r.inputRate) / float64(RaopRate)

	// Top up the staging buffer from the source, up to the working-set cap.
	stagedFrames := len(r.in) / 2
	room := maxStagingFrames - stagedFrames
	if room > 0 {
		old := len(r.in)
		r.in = append(r.in, make([]int16, room*2)...)
		n := pull(r.in[old : old+room*2])
		r.in = r.in[:old+n]
		stagedFrames = len(r.in) / 2
	}

	produced := 0
	for produced < want {
		i0 := int(r.phase)
		if i0+1 >= stagedFrames {
			break
		}
		frac := r.phase - float64(i0)
		a := r.in[i0*2:]
		b := a[2:]
		dst[produced*2] = int16(float64(a[0]) + float64(b[0]-a[0])*frac)
		dst[produced*2+1] = int16(float64(a[1]) + float64(b[1]-a[1])*frac)
		r.phase += step
		produced++
	}
	if produced < want {
		clear(dst[produced*2:])
	}

	// Drop consumed frames, keeping floor(phase), the interpolation's left
	// neighbour, and rebase the phase.
	keepFrom := int(r.phase)
	if keepFrom > 0 {
		if keepFrom > stagedFrames {
			keepFrom = stagedFrames
		}
		copy(r.in, r.in[keepFrom*2:])
		r.in = r.in[:(stagedFrames-keepFrom)*2]
		r.phase -= float64(keepFrom)
	}
	return produced
}
