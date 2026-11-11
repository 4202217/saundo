package sender

import (
	"testing"

	"saundo/internal/airplay/crypto"
)

func eventFrame(t *testing.T, key, plaintext []byte, counter uint64, corrupt bool) []byte {
	t.Helper()
	lp := [2]byte{byte(len(plaintext)), byte(len(plaintext) >> 8)}
	ct, err := crypto.ChaCha20Poly1305Encrypt(key, crypto.CounterNonce8(counter), plaintext, lp[:])
	if err != nil {
		t.Fatal(err)
	}
	if corrupt {
		ct[0] ^= 0xff
	}
	return append([]byte{lp[0], lp[1]}, ct...)
}

// TestEventChannelKeepsFramesAfterABadFrame pins the trim: a frame that fails
// to decrypt must not discard the later frames coalesced in the same read.
func TestEventChannelKeepsFramesAfterABadFrame(t *testing.T) {
	io := &audioRecordIO{}
	s := New(io, Events{}, nil)
	s.state = StateStreaming
	key := make([]byte, 32)
	s.ap2 = &ap2State{eventIn: key, eventOut: key}

	bad := []byte("GET /bad HTTP/1.1\r\nCSeq: 1\r\n\r\n")
	good := []byte("GET /good HTTP/1.1\r\nCSeq: 2\r\n\r\n")
	data := append(eventFrame(t, key, bad, 0, true), eventFrame(t, key, good, 1, false)...)

	s.onEventData(data)

	if len(io.events) != 1 {
		t.Fatalf("event responses = %d, want 1 for the frame after the bad one", len(io.events))
	}
	if s.eventRecvCtr != 2 {
		t.Fatalf("event receive counter = %d, want 2", s.eventRecvCtr)
	}
}
