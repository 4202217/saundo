package capture

import (
	"testing"

	"saundo/internal/audio/ring"
)

// TestStreamOpenedIncrementsOpenSeq pins the generation counter the engine
// polls: every successful open, including a same-rate reopen, is a new
// boundary.
func TestStreamOpenedIncrementsOpenSeq(t *testing.T) {
	s := &Source{cfg: Config{Ring: ring.New(8)}}
	s.streamOpened(48000, "A", nil)
	if got := s.Stats().OpenSeq; got != 1 {
		t.Fatalf("open sequence = %d, want 1", got)
	}
	s.streamOpened(48000, "B", nil)
	if got := s.Stats().OpenSeq; got != 2 {
		t.Fatalf("open sequence = %d, want 2", got)
	}
}
