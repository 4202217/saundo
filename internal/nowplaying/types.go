package nowplaying

import "time"

// PlaybackStatus mirrors GlobalSystemMediaTransportControlsSessionPlaybackStatus.
type PlaybackStatus int32

const (
	StatusClosed PlaybackStatus = iota
	StatusOpened
	StatusChanging
	StatusStopped
	StatusPlaying
	StatusPaused
)

func (s PlaybackStatus) String() string {
	switch s {
	case StatusClosed:
		return "closed"
	case StatusOpened:
		return "opened"
	case StatusChanging:
		return "changing"
	case StatusStopped:
		return "stopped"
	case StatusPlaying:
		return "playing"
	case StatusPaused:
		return "paused"
	default:
		return "unknown"
	}
}

// Session is one SMTC media session at the time of a read.
type Session struct {
	AUMID       string
	AppName     string
	AppIcon     []byte
	Title       string
	Subtitle    string
	Artist      string
	AlbumArtist string
	Album       string
	Status      PlaybackStatus
	Playing     bool
	// Advancing reports whether the timeline is moving. Apple Music briefly
	// publishes playing=true with playbackRate=0 and a cached stamp, so the
	// playing flag alone is not enough to justify extrapolating the position.
	Advancing bool
	CanPlay   bool
	CanPause  bool
	CanNext   bool
	CanPrev   bool
	CanSeek   bool
	Position  time.Duration
	Duration  time.Duration
	Updated   time.Time
	// PositionAt is the wall-clock moment Position was sampled. MediaRemote
	// stamps its elapsed value at timestampEpochMicros; SMTC does not expose a
	// sampling instant, so the Windows leaf leaves this zero and the card does
	// not extrapolate a position it cannot anchor.
	PositionAt time.Time
	// Artwork is the session's cover image. It is nil when the session has no
	// cover or publishes only a decorative placeholder, such as Apple Music's
	// idle frame and note.
	Artwork     []byte
	ArtworkMIME string
}

// Snapshot is the session list and the index of the manager's current session.
type Snapshot struct {
	Sessions []Session
	Current  int
	ReadAt   time.Time
}
