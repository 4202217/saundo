package sender

import (
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
)

// TestCommandRejectionDoesNotDisarmFeedback pins the URI-scoped feedback
// rule: a receiver rejecting a /command reply must not disable the AP2
// feedback keep-alive, which only /feedback answers may arm or disarm.
func TestCommandRejectionDoesNotDisarmFeedback(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	if !r.sender.feedback.active {
		t.Fatal("feedback not armed after launch")
	}
	r.rx.commandStatus = 404
	// Launch leaves the playback state unset; force a fresh /command.
	r.sender.playbackState = 1
	r.sender.playbackStateSent = 0
	r.sender.sendPlaybackState()
	r.step()
	if !r.sender.feedback.active {
		t.Fatal("a rejected /command reply disarmed the feedback keep-alive")
	}
	for _, line := range r.logs {
		if strings.Contains(line, "keep-alive disabled") {
			t.Fatalf("a /command reply was handled as /feedback: %q", line)
		}
	}
}

// TestMismatchedCSeqKeepsQueue pins the CSeq matching rule: a response whose
// CSeq matches no queued request is dropped with a warning, and the queue
// keeps its order so later replies still land on the right handler. A
// response without a CSeq header falls back to FIFO for pairing replies.
func TestMismatchedCSeqKeepsQueue(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	s := r.sender
	s.sendRequest("GET", "/info", "", "", nil)
	if len(s.pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(s.pending))
	}
	want := s.pending[0].cseq
	// A response with an unknown CSeq must not consume the queued request.
	s.onRTSPData([]byte("RTSP/1.0 200 OK\r\nCSeq: " + itoa(want+7) + "\r\n\r\n"))
	if len(s.pending) != 1 {
		t.Fatalf("a mismatched CSeq consumed the queued request, pending = %d", len(s.pending))
	}
	// The matching reply still lands, and a reply without CSeq stays FIFO.
	s.onRTSPData([]byte("RTSP/1.0 200 OK\r\nCSeq: " + itoa(want) + "\r\n\r\n"))
	if len(s.pending) != 0 {
		t.Fatalf("the matching reply did not consume the request, pending = %d", len(s.pending))
	}
	s.sendRequest("GET", "/info", "", "", nil)
	s.onRTSPData([]byte("RTSP/1.0 200 OK\r\n\r\n"))
	if len(s.pending) != 0 {
		t.Fatalf("a CSeq-less reply did not fall back to FIFO, pending = %d", len(s.pending))
	}
}

// TestEventDecryptSyncLossFails pins the event-channel failure limit: frames
// that fail to decrypt are dropped one by one, but a stream that never
// decrypts again must fail the session instead of running mute forever.
func TestEventDecryptSyncLossFails(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	badKey := make([]byte, 32)
	for i := 0; i < eventDecryptFailLimit; i++ {
		plain := []byte(strings.Repeat("x", 24))
		nonce := crypto.CounterNonce8(r.sender.eventRecvCtr + uint64(i))
		lp := [2]byte{byte(len(plain)), byte(len(plain) >> 8)}
		ct, err := crypto.ChaCha20Poly1305Encrypt(badKey, nonce, plain, lp[:])
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		frame := append([]byte{lp[0], lp[1]}, ct...)
		r.sender.onEventData(frame)
	}
	// A fail during streaming reports through Closed, not Launched(false).
	if r.sender.state != StateIdle {
		t.Fatalf("state = %v, want the event channel failure to end the session", r.sender.state)
	}
	found := false
	for _, line := range r.logs {
		if strings.Contains(line, "lost its decryption sync") {
			found = true
		}
	}
	if !found {
		t.Fatal("the event channel failure was not reported")
	}
}

// TestUnreadableContentLengthFailsClosed pins the unknown-not-empty rule for
// the RTSP frame splitter: a Content-Length header that does not parse is a
// broken stream, not a zero-length body.
func TestUnreadableContentLengthFailsClosed(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	s := r.sender
	s.sendRequest("GET", "/info", "", "", nil)
	s.onRTSPData([]byte("RTSP/1.0 200 OK\r\nCSeq: 0\r\nContent-Length: abc\r\n\r\n"))
	if len(r.launched) == 0 || r.launched[len(r.launched)-1].ok {
		t.Fatalf("launch results = %+v, want a failure", r.launched)
	}
}

// TestVolumePollTimeoutRecovers pins the in-flight volume query timeout: a
// receiver that never answers one poll must not stall the 1 Hz poll forever.
func TestVolumePollTimeoutRecovers(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	s := r.sender
	s.volQuery = true
	s.volQueryAt = s.now().Add(-volumeQueryTimeout - time.Second)
	s.onVolumePoll()
	// The query stays in flight: the retry replaced the lost one.
	if !s.volQuery {
		t.Fatal("the retry did not keep the poll in flight")
	}
	if s.now().Sub(s.volQueryAt) > time.Second {
		t.Fatal("the retry did not restart the in-flight window")
	}
	if len(s.pending) == 0 {
		t.Fatal("no new volume poll was sent after the timeout")
	}
}

// TestPendingCapDropsOldest pins the queue cap: a receiver that never answers
// must not grow the pending queue without bound.
func TestPendingCapDropsOldest(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	s := r.sender
	for i := 0; i < maxPendingRequests+5; i++ {
		s.sendRequest("GET", "/info", "", "", nil)
	}
	if len(s.pending) != maxPendingRequests {
		t.Fatalf("pending = %d, want the cap %d", len(s.pending), maxPendingRequests)
	}
}

// TestCleanCloseWithoutEventChannelIsRetryable pins the takeover
// classification: a clean control-channel close can only be a takeover when
// the event channel worked this session. A receiver whose event port never
// accepted the connection tears the control channel down after roughly 30 s
// — that is a connect failure, not a takeover, and must stay retryable.
func TestCleanCloseWithoutEventChannelIsRetryable(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	// Simulate the session whose event port refused the connect: the flag
	// says the channel never came up.
	r.sender.eventChannelUp = false
	r.sender.HandleEvent(raopio.Event{TCP: raopio.TCPControl, Kind: raopio.EventTCPClosed, Gen: r.sender.controlGen})
	if len(r.preempted) != 0 {
		t.Fatalf("preempted = %v, want a retryable failure", r.preempted)
	}
	found := false
	for _, line := range r.logs {
		if strings.Contains(line, "RAOP error") {
			found = true
		}
	}
	if !found {
		t.Fatal("the clean close was not reported as a failure")
	}
}

// TestCleanCloseAfterEventChannelIsTakeover pins the other side: with a
// working event channel the same close is the takeover signal.
func TestCleanCloseAfterEventChannelIsTakeover(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	if !r.sender.eventChannelUp {
		t.Fatal("the event channel did not mark itself up after launch")
	}
	r.sender.HandleEvent(raopio.Event{TCP: raopio.TCPControl, Kind: raopio.EventTCPClosed, Gen: r.sender.controlGen})
	if len(r.preempted) != 1 {
		t.Fatalf("preempted = %v, want the takeover", r.preempted)
	}
}
