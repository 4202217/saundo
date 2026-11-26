package app

import (
	"time"

	"saundo/internal/nowplaying"
)

// maxProgressExtrapolation bounds how far the displayed clock may run ahead of
// its last metadata sample. MediaRemote can briefly publish playing=true with
// a cached elapsed/timestamp pair from a paused session; without a bound that
// stale stamp would throw the clock tens of seconds forward.
const maxProgressExtrapolation = 30 * time.Second

// progressPosition returns the position to draw for s at the current wall
// clock time. MediaRemote's elapsedTimeMicros is measured at
// timestampEpochMicros, and Apple Music refreshes both only every few seconds
// (the values stay frozen between notifications). Advancing from that stamp —
// not from the poll's read time — keeps the clock moving forward across polls
// that carry the same frozen payload, which is what made the card's numbers
// jump back and forth at the start of a track.
func progressPosition(s nowplaying.Session, now time.Time) time.Duration {
	if s.Duration <= 0 {
		return 0
	}
	pos := s.Position
	// Without a stamp there is no reliable base to extrapolate from; show the
	// reported position rather than guessing from the poll's arrival time.
	if s.Advancing && !s.PositionAt.IsZero() && !s.PositionAt.After(now) {
		age := now.Sub(s.PositionAt)
		if age > maxProgressExtrapolation {
			age = maxProgressExtrapolation
		}
		pos += age
	}
	if pos < 0 {
		pos = 0
	}
	if pos > s.Duration {
		pos = s.Duration
	}
	return pos
}
