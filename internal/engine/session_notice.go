package engine

// NoticeKind names a transient session event the flyout explains.
type NoticeKind uint8

const (
	NoticeNone NoticeKind = iota
	// NoticeMemberLost means a member closed while the rest of the cohort
	// kept their session.
	NoticeMemberLost
)

// SessionNotice is the last session event the interface has not dismissed.
// The engine keeps it across publishes and clears it on the next user connect
// or disconnect, so a snapshot never loses the explanation.
type SessionNotice struct {
	Kind      NoticeKind
	DeviceID  string
	Name      string
	Remaining int
}
