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

// TestIdleCaptureReopenIsAdoptedBeforeTheNextCohort pins the other half of the
// generation boundary: an idle engine has no wake on its schedule, so a reopen
// that happened between sessions is only adopted when the next cohort opens.
// It must be adopted before the cohort takes the input format.
func TestIdleCaptureReopenIsAdoptedBeforeTheNextCohort(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	src := &fakeCaptureSource{openSeq: 1, rate: 48000, device: "A"}
	h.e.capture = src
	h.e.captureOpenSeq = 1
	h.e.captureRate = 48000
	h.e.captureName = "A"

	// The device switched while no session was live and no wake was scheduled.
	src.openSeq = 2
	src.rate = 44100
	src.device = "B"

	h.connect()

	rates := h.fake.inputRates
	if len(rates) == 0 || rates[len(rates)-1] != 44100 {
		t.Fatalf("input rates = %v, want the reopened 44100 Hz", rates)
	}
	if h.e.captureOpenSeq != 2 || h.e.captureName != "B" {
		t.Fatalf("open sequence = %d, name = %q, want 2 and B", h.e.captureOpenSeq, h.e.captureName)
	}
}
