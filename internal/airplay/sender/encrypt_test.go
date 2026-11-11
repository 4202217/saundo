package sender

import (
	"testing"

	raopio "saundo/internal/airplay/io"
)

type audioRecordIO struct {
	audio  [][]byte
	events [][]byte
}

func (a *audioRecordIO) TCPConnect(raopio.TCPChannel, string, uint16) error { return nil }
func (a *audioRecordIO) TCPSend(ch raopio.TCPChannel, data []byte) error {
	if ch == raopio.TCPEvent {
		a.events = append(a.events, append([]byte(nil), data...))
	}
	return nil
}
func (a *audioRecordIO) TCPClose(raopio.TCPChannel, bool)             {}
func (a *audioRecordIO) UDPBind(ch raopio.UDPChannel) (uint16, error) { return 5000 + uint16(ch), nil }
func (a *audioRecordIO) UDPClose(raopio.UDPChannel)                   {}

func (a *audioRecordIO) UDPSend(ch raopio.UDPChannel, _ raopio.Endpoint, data []byte) error {
	if ch == raopio.UDPAudio {
		a.audio = append(a.audio, append([]byte(nil), data...))
	}
	return nil
}

// TestEncryptFailureDropsThePacket pins the fail-closed audio path: a cipher
// error must not fall back to the plaintext payload.
func TestEncryptFailureDropsThePacket(t *testing.T) {
	io := &audioRecordIO{}
	s := New(io, Events{}, nil)
	s.state = StateStreaming
	s.hostIP = "192.168.1.20"
	s.serverPort = 7000
	s.ap2 = &ap2State{encryptAudio: true, audioKey: []byte{1, 2, 3}}

	s.sendAudioPacket()

	if len(io.audio) != 0 {
		t.Fatalf("audio datagrams = %d, want 0 after a cipher error", len(io.audio))
	}
	if s.encryptErrors != 1 || s.sendErrors != 0 || s.sentBlocks != 0 {
		t.Fatalf("counters = encrypt %d, send %d, sent %d", s.encryptErrors, s.sendErrors, s.sentBlocks)
	}
	if s.seq != 1 || s.framesSent != framesPerPacket {
		t.Fatalf("timeline = seq %d, frames %d, want 1 and %d", s.seq, s.framesSent, framesPerPacket)
	}
	if s.ap2.audioNonce != 0 {
		t.Fatalf("audio nonce = %d, want 0 after a dropped packet", s.ap2.audioNonce)
	}

	s.ap2.audioKey = make([]byte, 32)
	s.sendAudioPacket()

	if len(io.audio) != 1 {
		t.Fatalf("audio datagrams = %d after a valid key, want 1", len(io.audio))
	}
	if s.encryptErrors != 1 || s.sentBlocks != 1 {
		t.Fatalf("counters after recovery = encrypt %d, sent %d", s.encryptErrors, s.sentBlocks)
	}
}
