package engine

import (
	"testing"
	"time"
)

// TestSingleSessionSignalsDeadOnTerminalEvent pins the single transport's
// teardown signal. Preemption and a terminal close must both make Dead
// readable, so the engine goroutine can run closeSession and stop the PTP
// follower and sockets the sender does not own.
func TestSingleSessionSignalsDeadOnTerminalEvent(t *testing.T) {
	h := newHarness(t)
	sess := &session{dead: make(chan struct{})}
	events := h.e.sessionEvents(sess)
	events.Preempted("takeover")
	events.Closed()
	select {
	case <-sess.Dead():
	case <-time.After(time.Second):
		t.Fatal("single-session transport never signaled dead")
	}
	// A second terminal callback must not close the channel twice.
	events.Closed()
}
