package engine

import (
	"time"

	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/ring"
)

// transport is one live sender session. The production implementation is a
// cohort of one or more senders sharing one start gate and at most one PTP
// role; tests replace it.
type transport interface {
	Events() <-chan raopio.Event
	// Dead closes when the transport is beyond saving (a member died, a
	// launch failed) and closeSession must run. Nil for transports that never
	// signal this.
	Dead() <-chan struct{}
	// LostTarget names the target whose member closed first. The engine keeps
	// the other members playing instead of retrying the whole cohort. Empty
	// when the failure has no single owner (a launch failure).
	LostTarget() string
	// Start opens every member's handshake. Endpoints and auth ride the plan.
	Start()
	Stop()
	Active() bool
	State() sender.State
	HandleEvent(ev raopio.Event)
	Tick()
	NextDeadline() time.Time
	// SetPaused suspends or resumes the audio pacer. It reports whether the
	// transport applied the request.
	SetPaused(paused bool) bool
	// Pushing reports whether the realtime stream is currently sending RTP.
	Pushing() bool
	SetIdentity(id sender.Identity)
	Rings() []*ring.Buffer
	SetInputFormat(rate int)
	SetVolume(pct float64)
	// SetTargetVolume applies a volume to one target's members; a pair shares
	// it and a single receiver gets its own.
	SetTargetVolume(key string, pct float64)
	// SetTargetChannel applies a stereo map to one target's members without a
	// session rebuild.
	SetTargetChannel(key string, mode sender.ChannelMode)
	SetStrictReceiverAuth(strict bool)
	SubmitPin(code string)
	WaitingForPin() bool
	InitialVolumePercent() (float64, bool)
	// InitialVolumePercentFor is the first report among one target's members.
	InitialVolumePercentFor(key string) (float64, bool)
	SetNowPlaying(title, artist, album string, cover []byte, coverMIME string)
	SetLampArtwork(data []byte, mime string, on bool)
	SetPlaybackState(playing bool)
	PTP() (active, ready bool, offsetMS float64)
	// Stats returns one sender.Stats per member session, in plan order.
	Stats() []sender.Stats
	// DroppedEvents is how many host notifications the event queue discarded
	// for this session.
	DroppedEvents() uint64
}
