package sender

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	raopio "saundo/internal/airplay/io"
	"saundo/internal/audio/ring"
)

// recordingIO records raw audio payloads and UDP control datagrams.
type recordingIO struct {
	audio [][]byte
	sync  [][]byte
}

func (io *recordingIO) TCPConnect(raopio.TCPChannel, string, uint16) error { return nil }
func (io *recordingIO) TCPSend(raopio.TCPChannel, []byte) error            { return nil }
func (io *recordingIO) TCPClose(raopio.TCPChannel, bool)                   {}
func (io *recordingIO) UDPBind(ch raopio.UDPChannel) (uint16, error) {
	return 6000 + uint16(ch), nil
}
func (io *recordingIO) UDPSend(ch raopio.UDPChannel, _ raopio.Endpoint, data []byte) error {
	p := append([]byte(nil), data...)
	switch ch {
	case raopio.UDPAudio:
		io.audio = append(io.audio, p[12:])
	case raopio.UDPControl:
		io.sync = append(io.sync, p)
	}
	return nil
}
func (io *recordingIO) UDPClose(raopio.UDPChannel) {}

func TestPairStartsInLockstep(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	fanout := ring.NewFanout()
	rings := [2]*ring.Buffer{ring.New(44100 * 2 * 4), ring.New(44100 * 2 * 4)}
	fanout.Add(rings[0])
	fanout.Add(rings[1])

	// A ramp preloaded while the pair is idle; every sample identifies its
	// capture position.
	ramp := make([]int16, 44100*2*3)
	for i := range ramp {
		ramp[i] = int16(i % 30000)
	}
	fanout.Push(ramp)

	gate := NewStartGate(StartGateConfig{
		Members: 2,
		Cushion: DefaultStartCushion,
		Trim:    fanout,
	})
	mk := func(rb *ring.Buffer) (*Sender, *recordingIO) {
		io := &recordingIO{}
		s := New(io, Events{}, nil)
		s.SetClock(func() time.Time { return now })
		s.SetInputFormat(44100) // pass-through resampler for exact comparison
		s.SetStartGate(gate)
		s.AttachRing(rb)
		s.serverPort, s.controlPort = 6000, 6001
		s.nextLatencySamples = 8820
		s.setState(StateHandshake)
		return s, io
	}
	a, ioA := mk(rings[0])
	b, ioB := mk(rings[1])

	// A finishes its handshake first and must wait for B.
	a.readyToStream()
	if !a.awaitingStart || a.State() == StateStreaming {
		t.Fatal("A started before B arrived")
	}
	if len(ioA.audio) != 0 {
		t.Fatal("A sent audio before the shared epoch")
	}

	// B's handshake is 150 ms slower.
	now = now.Add(150 * time.Millisecond)
	b.readyToStream()

	// Drive both in 8 ms pacer steps so the max-packets-per-tick cap cannot
	// hide a start-instant difference.
	for i := 0; i < 60; i++ {
		now = now.Add(8 * time.Millisecond)
		a.Tick()
		b.Tick()
	}

	if len(ioA.audio) == 0 || len(ioB.audio) == 0 {
		t.Fatal("no audio from one or both members")
	}
	if len(ioA.audio) != len(ioB.audio) {
		t.Fatalf("member frame counts differ: %d vs %d", len(ioA.audio), len(ioB.audio))
	}
	for i := range ioA.audio {
		if !bytes.Equal(ioA.audio[i], ioB.audio[i]) {
			t.Fatalf("member content differs in packet %d", i)
		}
	}

	first := int16(binary.BigEndian.Uint16(ioA.audio[0][:2]))
	if first == ramp[0] {
		t.Fatal("first sample is the oldest pre-connect audio")
	}
	cushion := int(DefaultStartCushion.Seconds()*44100) * 2
	if want := ramp[len(ramp)-cushion]; first != want {
		t.Fatalf("first sample = %d, want the cushion boundary %d", first, want)
	}

	// Both members wrote their first sync at the same fake instant from the
	// same epoch, so the NTP mapping is byte-identical.
	if len(ioA.sync) == 0 || len(ioB.sync) == 0 {
		t.Fatal("missing sync packet")
	}
	if !bytes.Equal(ioA.sync[0], ioB.sync[0]) {
		t.Fatal("NTP sync mappings differ between pair members")
	}
}
