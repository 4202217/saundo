package sender

import (
	"strings"
	"testing"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
)

// TestEventConnectFailureFailsTheHandshake pins the admission gate: RECORD
// waits for the event channel, so a refused event port is a handshake failure
// rather than a stream that reports success and dies about 30 s later.
func TestEventConnectFailureFailsTheHandshake(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.failEventConnect = true
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	if len(r.launched) != 1 || r.launched[0].ok {
		t.Fatalf("launched = %+v, want one handshake failure", r.launched)
	}
	if !strings.Contains(r.launched[0].err, "event channel") {
		t.Fatalf("error = %q, want the event channel named", r.launched[0].err)
	}
	if r.sender.State() != StateIdle {
		t.Fatalf("state = %s, want idle after the failed handshake", r.sender.State())
	}
}

// TestEventChannelCloseMarksSessionRetryable pins the health flag's lifecycle:
// an event-channel close clears it, so the receiver's later clean control
// close reads as the retryable no-event-channel failure, not a takeover.
func TestEventChannelCloseMarksSessionRetryable(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	if !r.sender.eventChannelUp {
		t.Fatal("the event channel did not mark itself up after launch")
	}

	r.sender.HandleEvent(raopio.Event{
		Kind: raopio.EventTCPClosed, TCP: raopio.TCPEvent, Gen: r.sender.eventGen,
	})
	if r.sender.eventChannelUp {
		t.Fatal("an event-channel close left the health flag set")
	}

	r.sender.HandleEvent(raopio.Event{
		Kind: raopio.EventTCPClosed, TCP: raopio.TCPControl, Gen: r.sender.controlGen,
	})
	if len(r.preempted) != 0 {
		t.Fatalf("preempted = %v, want a retryable failure", r.preempted)
	}
	if r.closed == 0 {
		t.Fatal("the clean control close did not report a closed session")
	}
}
