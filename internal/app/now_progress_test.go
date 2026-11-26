package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/internal/nowplaying"
)

// TestProgressPositionAnchorsToPayloadTimestamp pins the macOS timeline rule:
// MediaRemote's elapsedTimeMicros is measured at timestampEpochMicros, and
// Apple Music only refreshes both every few seconds. Advancing from the stamp
// instead of the poll time keeps the clock moving between updates.
func TestProgressPositionAnchorsToPayloadTimestamp(t *testing.T) {
	stamp := time.Unix(1_795_623_224, 993_643_000)
	now := stamp.Add(2 * time.Second)
	s := nowplaying.Session{
		Title: "track", Playing: true, Advancing: true,
		Position: 65 * time.Second, Duration: 297 * time.Second,
		Updated: stamp, PositionAt: stamp,
	}
	if got := progressPosition(s, now); got != 67*time.Second {
		t.Fatalf("position after 2 s = %v, want 67 s", got)
	}
}

// TestProgressPositionSurvivesStalePolls reproduces the 0:00 / -0:01 jump.
// Each poll carries the same frozen elapsed time and stamp while real time
// moves forward; the displayed position must rise, not reset.
func TestProgressPositionSurvivesStalePolls(t *testing.T) {
	stamp := time.Unix(1_795_623_224, 993_643_000)
	s := nowplaying.Session{
		Title: "track", Playing: true, Advancing: true,
		Position: 0, Duration: 3 * time.Second,
		Updated: stamp, PositionAt: stamp,
	}
	for i := 0; i < 6; i++ {
		// The 2-second app poll hands over an unchanged payload with no newer
		// stamp; the wall clock has advanced two seconds each time.
		now := stamp.Add(time.Duration(i) * 2 * time.Second)
		want := time.Duration(i) * 2 * time.Second
		if want > s.Duration {
			want = s.Duration
		}
		if got := progressPosition(s, now); got != want {
			t.Fatalf("poll %d: position = %v, want %v", i, got, want)
		}
	}
}

// TestProgressPositionBoundsStaleStamp pins the start-of-playback case: a
// playing payload briefly carries a cached stamp from tens of seconds earlier.
// Extrapolating all the way from that stamp would throw the clock forward.
func TestProgressPositionBoundsStaleStamp(t *testing.T) {
	stamp := time.Unix(1_795_623_224, 993_643_000)
	now := stamp.Add(55 * time.Second)
	s := nowplaying.Session{
		Title: "track", Playing: true, Advancing: true,
		Position: 33 * time.Second, Duration: 243 * time.Second,
		Updated: stamp, PositionAt: stamp,
	}
	want := 33*time.Second + maxProgressExtrapolation
	if got := progressPosition(s, now); got != want {
		t.Fatalf("position from a 55 s stale stamp = %v, want %v", got, want)
	}
}

// TestProgressPositionHoldsWhenRateIsZero pins the start-of-playback payload
// captured from Apple Music: playing=true with playbackRate=0 and a cached
// stamp must not extrapolate a position that is not moving yet.
func TestProgressPositionHoldsWhenRateIsZero(t *testing.T) {
	stamp := time.Unix(1_795_623_224, 993_643_000)
	s := nowplaying.Session{
		Title: "track", Playing: true, Advancing: false,
		Position: 33 * time.Second, Duration: 243 * time.Second,
		Updated: stamp, PositionAt: stamp,
	}
	if got := progressPosition(s, stamp.Add(55*time.Second)); got != 33*time.Second {
		t.Fatalf("position with rate zero = %v, want 33 s", got)
	}
}

// TestProgressPositionAcceptsSeekAndNewerStamp checks that a genuine
// MediaRemote update and a user seek still move the clock, even backward.
func TestProgressPositionAcceptsSeekAndNewerStamp(t *testing.T) {
	stamp := time.Unix(1_795_623_224, 993_643_000)
	s := nowplaying.Session{
		Title: "track", Playing: true, Advancing: true,
		Position: 60 * time.Second, Duration: 297 * time.Second,
		Updated: stamp, PositionAt: stamp,
	}
	if got := progressPosition(s, stamp); got != 60*time.Second {
		t.Fatalf("first position = %v, want 60 s", got)
	}
	// A newer stamp with a larger elapsed value (the normal refresh) wins.
	s2 := s
	s2.Position = 70 * time.Second
	s2.Updated = stamp.Add(10 * time.Second)
	s2.PositionAt = s2.Updated
	if got := progressPosition(s2, s2.Updated); got != 70*time.Second {
		t.Fatalf("refreshed position = %v, want 70 s", got)
	}
	// A seek within the same stamp is a legitimate backward step.
	s3 := s2
	s3.Position = 5 * time.Second
	if got := progressPosition(s3, s2.Updated); got != 5*time.Second {
		t.Fatalf("seek position = %v, want 5 s", got)
	}
	// A new track starts from its own position.
	s4 := s3
	s4.Title = "next track"
	s4.Position = 0
	if got := progressPosition(s4, s2.Updated); got != 0 {
		t.Fatalf("new track position = %v, want 0", got)
	}
}

// TestProgressRowDoesNotFlipRemaining pins the visible symptom: with a frozen
// two-second-old Apple Music payload, the card keeps a single steady remaining
// value instead of bouncing between 0:00 and -0:01.
func TestProgressRowDoesNotFlipRemaining(t *testing.T) {
	a := previewApp()
	stamp := time.Unix(1_795_623_224, 993_643_000)
	now := stamp.Add(2 * time.Second)
	a.nowTime = func() time.Time { return now }
	a.nowSnap = nowplaying.Snapshot{
		Sessions: []nowplaying.Session{{
			AUMID: "com.apple.Music", AppName: "Apple Music", Title: "track", Artist: "artist",
			Status: nowplaying.StatusPlaying, Playing: true, Advancing: true,
			CanPlay: true, CanPause: true, CanNext: true, CanPrev: true,
			Position: 0, Duration: 3 * time.Second, Updated: stamp, PositionAt: stamp,
		}},
		Current: 0, ReadAt: stamp,
	}
	a.nowMediaAUMID = "com.apple.Music"
	a.flyoutEntered = true
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if !tt.HasText("-0:01") {
		t.Fatalf("remaining label missing; texts: %q", tt.Texts())
	}
	if tt.HasText("0:00") {
		t.Fatalf("elapsed label still reset to 0:00; texts: %q", tt.Texts())
	}
}
