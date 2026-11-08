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
	CanPlay     bool
	CanPause    bool
	CanNext     bool
	CanPrev     bool
	CanSeek     bool
	Position    time.Duration
	Duration    time.Duration
	Updated     time.Time
	Artwork     []byte
	ArtworkMIME string
}

// Snapshot is the session list and the index of the manager's current session.
type Snapshot struct {
	Sessions []Session
	Current  int
	ReadAt   time.Time
}
