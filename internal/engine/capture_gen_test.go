package engine

import "testing"

// TestCaptureReopenResetsTheSinkAtTheSameRate pins the generation boundary: a
// stream reopen with the same rate is still a new source, so the old device's
// samples must not survive in the rings that feed the senders.
func TestCaptureReopenResetsTheSinkAtTheSameRate(t *testing.T) {
	h := newHarness(t)
	src := &fakeCaptureSource{openSeq: 1, rate: 48000, device: "A"}
	h.e.capture = src
	h.e.captureOpenSeq = 1
	h.e.captureRate = 48000
	h.e.captureName = "A"
	h.e.ring.Push([]int16{1, 2, 3, 4})
	if h.e.ring.Available() == 0 {
		t.Fatal("the test did not buffer old-device samples")
	}

	src.openSeq = 2
	src.device = "B"
	h.e.syncCaptureOpen()

	if got := h.e.ring.Available(); got != 0 {
		t.Fatalf("ring kept %d old-device samples", got)
	}
	if h.e.captureOpenSeq != 2 {
		t.Fatalf("open sequence = %d, want 2", h.e.captureOpenSeq)
	}
	if h.e.captureName != "B" {
		t.Fatalf("capture name = %q, want the reopened device", h.e.captureName)
	}
}
