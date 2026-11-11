package sender

import (
	"bytes"
	"testing"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
)

// TestEventChannelNegativeContentLengthDoesNotPanic pins the crash a hostile
// event frame can cause. The malformed frame must be consumed, answered, and
// the parser must go on to the next frame instead of unwinding the engine's
// event loop.
func TestEventChannelNegativeContentLengthDoesNotPanic(t *testing.T) {
	r := newRig(t, modeAP1, device.AuthNone, false, nil)
	s := r.sender
	s.state = StateStreaming
	s.ap2 = &ap2State{
		eventIn:  bytes.Repeat([]byte{0x11}, 32),
		eventOut: bytes.Repeat([]byte{0x22}, 32),
	}
	// The fake receiver decrypts the sender's event replies with this key.
	r.rx.eventReadKey = append([]byte(nil), s.ap2.eventOut...)
	r.rx.eventReadCtr = 0

	var bodies []string
	eventBodyHook = func(firstLine, _ string, _ []byte) { bodies = append(bodies, firstLine) }
	defer func() { eventBodyHook = nil }()

	frames := [][]byte{
		[]byte("RTSP/1.0 200 OK\r\nContent-Length: -1000\r\n\r\n"),
		[]byte("GET /info RTSP/1.0\r\nContent-Length: 0\r\n\r\n"),
	}
	var stream []byte
	for i, plain := range frames {
		lp := [2]byte{byte(len(plain)), byte(len(plain) >> 8)}
		ct, err := crypto.ChaCha20Poly1305Encrypt(s.ap2.eventIn, crypto.CounterNonce8(uint64(i)), plain, lp[:])
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, lp[:]...)
		stream = append(stream, ct...)
	}
	s.onEventData(stream)

	if len(bodies) != 2 {
		t.Fatalf("event bodies = %v, want the malformed frame and the one after it", bodies)
	}
	if bodies[1] != "GET /info RTSP/1.0" {
		t.Fatalf("second event = %q, want GET /info RTSP/1.0", bodies[1])
	}
}

// TestControlChannelHugeContentLengthDoesNotPanic covers the same unchecked
// arithmetic on the control channel: a positive length near MaxInt overflows
// the frame end and slices the buffer with a negative index.
func TestControlChannelHugeContentLengthDoesNotPanic(t *testing.T) {
	r := newRig(t, modeAP1, device.AuthNone, false, nil)
	s := r.sender
	s.state = StateHandshake
	s.onRTSPData([]byte("RTSP/1.0 200 OK\r\nContent-Length: 9223372036854775807\r\n\r\n"))
	if len(s.rxBuf) == 0 {
		t.Fatal("the incomplete frame was dropped; the parser should wait for its body")
	}
}
