package sender

import (
	"testing"
	"time"
)

// TestElapsedFramesSurvivesLongSessions pins the media timeline past the old
// 4.84-day overflow: ns * 44100 exceeded uint64, so a long session wrapped
// its frame count and all timestamps derived from it.
func TestElapsedFramesSurvivesLongSessions(t *testing.T) {
	base := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	s := &Sender{clockStart: base}
	cases := []struct {
		name string
		d    time.Duration
		want uint64
	}{
		{"one second", time.Second, 44100},
		{"fractional", 1500 * time.Millisecond, 66150},
		{"ten days", 10 * 24 * time.Hour, 10 * 24 * 3600 * 44100},
	}
	for _, tc := range cases {
		if got := s.elapsedFrames(base.Add(tc.d)); got != tc.want {
			t.Errorf("%s: elapsedFrames = %d, want %d", tc.name, got, tc.want)
		}
	}
}
