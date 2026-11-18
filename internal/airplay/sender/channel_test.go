package sender

import (
	"encoding/binary"
	"testing"

	"saundo/internal/audio/resample"
	"saundo/internal/audio/ring"
)

// channelSender returns a sender whose ring holds frames of (left, right)
// pairs at rate. The ring is long enough for one full packet.
func channelSender(rate int, frames int, left, right int16) *Sender {
	s := New(&audioRecordIO{}, Events{}, nil)
	s.SetInputFormat(rate)
	s.resampler = resample.NewLinear(rate)
	s.ring = ring.New(frames * channels)
	samples := make([]int16, frames*channels)
	for i := 0; i < frames; i++ {
		samples[i*2] = left
		samples[i*2+1] = right
	}
	s.ring.Push(samples)
	return s
}

func fillPacket(s *Sender) []int16 {
	dst := make([]int16, framesPerPacket*channels)
	if n := s.fillFrames(dst); n != framesPerPacket {
		panic("short frame fill")
	}
	return dst
}

// TestChannelMix pins the three maps at 48 kHz, which also covers the
// resampler: the mix runs on the 44.1 kHz output frame, not the input.
func TestChannelMix(t *testing.T) {
	cases := []struct {
		name         string
		mode         ChannelMode
		wantL, wantR int16
	}{
		{"stereo unchanged", ChannelStereo, 1000, -1000},
		{"left into both slots", ChannelLeft, 1000, 1000},
		{"right into both slots", ChannelRight, -1000, -1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := channelSender(48000, 4096, 1000, -1000)
			s.SetChannel(tc.mode)
			dst := fillPacket(s)
			for i := 0; i < framesPerPacket; i++ {
				if dst[i*2] != tc.wantL || dst[i*2+1] != tc.wantR {
					t.Fatalf("frame %d = (%d, %d), want (%d, %d)", i, dst[i*2], dst[i*2+1], tc.wantL, tc.wantR)
				}
			}
		})
	}
}

// TestChannelMixAtRaopRate pins the no-resample path.
func TestChannelMixAtRaopRate(t *testing.T) {
	s := channelSender(44100, framesPerPacket, 700, -700)
	s.SetChannel(ChannelLeft)
	dst := fillPacket(s)
	if dst[0] != 700 || dst[1] != 700 {
		t.Fatalf("first slot = (%d, %d), want (700, 700)", dst[0], dst[1])
	}
}

func TestParseChannelModeDefaultsStereo(t *testing.T) {
	cases := map[string]ChannelMode{
		"":         ChannelStereo,
		"stereo":   ChannelStereo,
		"left":     ChannelLeft,
		"right":    ChannelRight,
		"surround": ChannelStereo,
		"LEFT":     ChannelStereo,
	}
	for value, want := range cases {
		if got := ParseChannelMode(value); got != want {
			t.Errorf("ParseChannelMode(%q) = %v, want %v", value, got, want)
		}
	}
	if ChannelStereo.String() != "stereo" || ChannelLeft.String() != "left" || ChannelRight.String() != "right" {
		t.Fatalf("mode strings = %q, %q, %q", ChannelStereo, ChannelLeft, ChannelRight)
	}
}

// TestLiveSetChannelChangesTheNextPacket pins the live path: no session
// rebuild, the very next RTP payload carries the new map.
func TestLiveSetChannelChangesTheNextPacket(t *testing.T) {
	io := &audioRecordIO{}
	s := New(io, Events{}, nil)
	s.state = StateStreaming
	s.hostIP = "192.0.2.10"
	s.serverPort = 7000
	s.SetInputFormat(44100)
	s.ring = ring.New(1408 * channels)
	samples := make([]int16, 1408*channels)
	for i := 0; i < 1408; i++ {
		samples[i*2] = 1000
		samples[i*2+1] = -1000
	}
	s.ring.Push(samples)

	s.sendAudioPacket()
	s.SetChannel(ChannelLeft)
	if got := s.Channel(); got != ChannelLeft {
		t.Fatalf("Channel() = %v, want left", got)
	}
	s.sendAudioPacket()
	if len(io.audio) != 2 {
		t.Fatalf("audio datagrams = %d, want 2", len(io.audio))
	}
	first := io.audio[0][12:]
	second := io.audio[1][12:]
	if got := int16(binary.BigEndian.Uint16(first[0:2])); got != 1000 {
		t.Fatalf("first packet left = %d, want 1000", got)
	}
	if got := int16(binary.BigEndian.Uint16(first[2:4])); got != -1000 {
		t.Fatalf("first packet right = %d, want -1000", got)
	}
	if got := int16(binary.BigEndian.Uint16(second[0:2])); got != 1000 {
		t.Fatalf("second packet left = %d, want 1000", got)
	}
	if got := int16(binary.BigEndian.Uint16(second[2:4])); got != 1000 {
		t.Fatalf("second packet right = %d, want 1000", got)
	}
}
