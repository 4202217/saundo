package sender

import (
	"testing"

	"saundo/internal/airplay/device"
)

// TestStreamSetupReportsReceiverLatency pins the receiver's own
// arrival-to-render latency from the stream SETUP response. It is diagnostic
// and never changes the configured presentation latency.
func TestStreamSetupReportsReceiverLatency(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.rx.arrivalRenderMS = 70
	r.start()

	if got := r.sender.ReceiverLatencyMS(); got != 70 {
		t.Fatalf("receiver latency = %d, want 70", got)
	}
	if len(r.receiverMS) != 1 || r.receiverMS[0] != 70 {
		t.Fatalf("receiver latency events = %v, want [70]", r.receiverMS)
	}
}

// TestStreamSetupWithoutReceiverLatency treats an absent field as unknown,
// not as zero milliseconds.
func TestStreamSetupWithoutReceiverLatency(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.rx.arrivalRenderMS = 0
	r.start()

	if got := r.sender.ReceiverLatencyMS(); got != 0 {
		t.Fatalf("receiver latency = %d, want 0 for an absent field", got)
	}
	if len(r.receiverMS) != 0 {
		t.Fatalf("receiver latency events = %v, want none", r.receiverMS)
	}
}
