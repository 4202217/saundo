package sender

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	raopio "saundo/internal/airplay/io"
)

// sendFailIO fails the next datagrams a test asks it to and records the rest.
type sendFailIO struct {
	recordingIO
	failNext int
	calls    int
}

func (io *sendFailIO) UDPSend(ch raopio.UDPChannel, to raopio.Endpoint, data []byte) error {
	io.calls++
	if io.failNext > 0 {
		io.failNext--
		return errors.New("network unreachable")
	}
	return io.recordingIO.UDPSend(ch, to, data)
}

// TestAudioSendErrorsAreCounted pins the diagnostics gap: a failing audio
// socket must not keep incrementing sentBlocks as if the packet went out, and
// the first failure must reach the log.
func TestAudioSendErrorsAreCounted(t *testing.T) {
	io := &sendFailIO{failNext: 1}
	var logs []string
	s := New(io, Events{}, func(_ Level, format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	s.hostIP = "192.0.2.10"
	s.serverPort = 7000

	s.sendAudioPacket()
	if got := s.Stats().SentBlocks; got != 0 {
		t.Fatalf("a failed audio send counted %d sent blocks", got)
	}
	if got := s.Stats().SendErrors; got != 1 {
		t.Fatalf("send errors = %d, want 1", got)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "audio send failed") {
		t.Fatalf("first send failure logs = %q", logs)
	}

	// One duplicate: the primary fails, the copy goes out, so the packet is
	// delivered but the failed datagram still counts.
	s.duplicates = 1
	io.failNext = 1
	s.sendAudioPacket()
	if got := s.Stats().SentBlocks; got != 1 {
		t.Fatalf("sent blocks = %d, want 1 after the duplicate reached the host", got)
	}
	if got := s.Stats().SendErrors; got != 2 {
		t.Fatalf("send errors = %d, want 2", got)
	}
	if len(logs) != 1 {
		t.Fatalf("every send failure logged: %q", logs)
	}
}

// TestPersistentSendFailureEndsTheSession pins the dead-socket policy: a
// connected session whose every audio datagram fails must not keep streaming
// into nothing.
func TestPersistentSendFailureEndsTheSession(t *testing.T) {
	io := &sendFailIO{failNext: maxConsecutiveSendFailures}
	closed := 0
	s := New(io, Events{Closed: func() { closed++ }}, nil)
	s.state = StateStreaming
	s.hostIP = "192.0.2.10"
	s.serverPort = 7000

	for i := 0; i < maxConsecutiveSendFailures; i++ {
		s.sendAudioPacket()
	}
	if s.State() != StateIdle {
		t.Fatalf("state = %s, want idle after %d failed sends", s.State(), maxConsecutiveSendFailures)
	}
	if closed != 1 {
		t.Fatalf("closed events = %d, want 1", closed)
	}
}
