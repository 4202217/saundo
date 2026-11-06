package nowplaying

// Action is one transport command the card sends to a media session.
type Action int

const (
	ActionPlay Action = iota
	ActionPause
	ActionNext
	ActionPrevious
	ActionSeek
)
