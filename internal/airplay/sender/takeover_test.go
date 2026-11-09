package sender

import (
	"testing"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
)

// TestCleanControlCloseDuringStreamingPreempts pins the takeover signal seen
// on a HomePod: a second sender causes the receiver to close the displaced
// control channel without an error. That close is terminal, not a retryable
// failure.
func TestCleanControlCloseDuringStreamingPreempts(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	requireLaunched(t, r)

	r.emit(raopio.Event{
		Kind: raopio.EventTCPClosed,
		TCP:  raopio.TCPControl,
		Gen:  r.sender.controlGen,
	})
	r.step()

	if len(r.preempted) != 1 {
		t.Fatalf("preempted = %d, want 1", len(r.preempted))
	}
	if r.closed != 1 {
		t.Fatalf("closed = %d, want 1", r.closed)
	}
	if len(r.launched) != 1 || !r.launched[0].ok {
		t.Fatalf("launch events = %+v, want only the successful launch", r.launched)
	}
	if got := r.sender.State(); got != StateIdle {
		t.Fatalf("state = %s, want idle", got)
	}
}

// TestControlCloseWithErrorIsNotPreemption keeps a network failure on the
// retryable path.
func TestControlCloseWithErrorIsNotPreemption(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	requireLaunched(t, r)

	r.emit(raopio.Event{
		Kind: raopio.EventTCPClosed,
		TCP:  raopio.TCPControl,
		Gen:  r.sender.controlGen,
		Err:  "connection reset",
	})
	r.step()

	if len(r.preempted) != 0 {
		t.Fatalf("preempted = %d, want 0 for a network error", len(r.preempted))
	}
	if r.closed != 1 {
		t.Fatalf("closed = %d, want 1", r.closed)
	}
}
