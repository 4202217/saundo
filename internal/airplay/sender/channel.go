package sender

// ChannelMode selects how the incoming stereo pair maps onto the two output
// channels. It is a playback preference, not a receiver capability: a value
// the receiver does not advertise still plays as one of the three modes.
type ChannelMode uint8

const (
	ChannelStereo ChannelMode = iota
	ChannelLeft
	ChannelRight
)

// ParseChannelMode reads a stored channel preference. Empty and unknown
// values read as stereo, because an unset or corrupt preference must never
// mute a side.
func ParseChannelMode(value string) ChannelMode {
	switch value {
	case "left":
		return ChannelLeft
	case "right":
		return ChannelRight
	default:
		return ChannelStereo
	}
}

func (m ChannelMode) String() string {
	switch m {
	case ChannelLeft:
		return "left"
	case ChannelRight:
		return "right"
	default:
		return "stereo"
	}
}

// SetChannel selects the mapping for the next packet. The mix runs after
// resampling and before ALAC, so a live change needs no session rebuild.
func (s *Sender) SetChannel(mode ChannelMode) { s.channel = mode }

// Channel reports the current mapping.
func (s *Sender) Channel() ChannelMode { return s.channel }

// applyChannel copies one input channel into both output slots. Stereo is a
// passthrough; a left or right preference makes the packet mono on that side.
func applyChannel(dst []int16, frames int, mode ChannelMode) {
	if mode == ChannelStereo {
		return
	}
	if max := len(dst) / 2; frames > max {
		frames = max
	}
	src := 0
	if mode == ChannelRight {
		src = 1
	}
	for i := 0; i < frames; i++ {
		sample := dst[i*2+src]
		dst[i*2] = sample
		dst[i*2+1] = sample
	}
}
