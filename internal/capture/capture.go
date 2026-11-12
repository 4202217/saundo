// Package capture captures Windows system audio into a ring buffer for the
// AirPlay sender. It opens the render endpoint's shared-mode loopback stream
// and converts the device mix format to interleaved stereo int16 at the
// device rate; resampling to the stream rate is the sender's concern.
package capture

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"saundo/internal/audio/ring"
)

// Timing and error shapes shared by the per-platform capture loops.
const (
	openCheckEvery  = 500 * time.Millisecond
	retryDelayFirst = 200 * time.Millisecond
	// A missing output device is usually a dock, a remote-audio session, or a
	// driver reload. Retry slowly so a long absence does not flood the log or
	// spin on WASAPI activation; a device that returns is picked up on the
	// next attempt.
	retryDelayMax = 30 * time.Second
)

var (
	errStopped              = errors.New("capture: stopped")
	errDefaultDeviceChanged = errors.New("capture: the default render device changed")
	// ErrNoAppProcess reports that none of the selected apps has a Core
	// Audio process to tap yet. The engine shows a waiting state and keeps
	// retrying, not a capture failure.
	ErrNoAppProcess = errors.New("capture: no Core Audio process for the selection yet")
)

// Endpoint identifies one active render device.
type Endpoint struct {
	ID   string
	Name string
}

// Config selects the destination ring and the stream's event callbacks.
// Callbacks run on the capture goroutine and must return quickly; they must
// not call Stop, which waits for that goroutine.
type Config struct {
	Ring       ring.Sink // required; a fanout feeds every stereo-pair member
	EndpointID string    // empty follows the default render device
	// ProcessID selects application-level capture (process-loopback) instead
	// of an endpoint. Nonzero ignores EndpointID.
	ProcessID uint32
	// AppIDs selects apps by stable identifier: bundle IDs on macOS, and
	// pid:<n> for processes that have none. Non-empty takes precedence over
	// ProcessID on a platform that supports it.
	AppIDs []string
	// MuteProcess starts a per-app tap muted: the captured app's audio goes
	// only to the tap while other apps keep playing locally. Platforms
	// without process mute ignore it; the engine falls back to the endpoint
	// mute there.
	MuteProcess bool
	OnFormat    func(rate int)
	OnError     func(error)
	OnDevice    func(name string)
}

// Stats is a snapshot of a capture source.
type Stats struct {
	Running bool
	Rate    int
	Device  string
	// Apps lists the selected stable IDs the current tap actually includes.
	// It is empty for endpoint capture and for platforms without app IDs.
	Apps    []string
	Frames  uint64
	Dropped uint64
	Errors  uint64
}

// Source is one running capture. Stop ends it.
type Source struct {
	cfg Config

	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
	stopped bool

	running bool
	rate    int
	device  string
	apps    []string
	frames  atomic.Uint64
	dropped atomic.Uint64
	errors  uint64
	// muteReq is the per-app tap's desired local-output state. Only the
	// darwin leaf reads it; other platforms leave it false.
	muteReq atomic.Bool
}

// Stop ends capture. It is idempotent and waits for the capture goroutine.
func (s *Source) Stop() {
	s.mu.Lock()
	stop, done := s.stop, s.done
	first := !s.stopped
	s.stopped = true
	s.mu.Unlock()
	if first && stop != nil {
		close(stop)
	}
	if done != nil {
		<-done
	}
}

// Stats returns a snapshot of the capture state.
func (s *Source) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{
		Running: s.running,
		Rate:    s.rate,
		Device:  s.device,
		Apps:    append([]string(nil), s.apps...),
		Frames:  s.frames.Load(),
		Dropped: s.cfg.Ring.Dropped() + s.dropped.Load(),
		Errors:  s.errors,
	}
}

// streamOpened records a successful open and fires the open callbacks.
func (s *Source) streamOpened(rate int, device string, apps []string) {
	s.mu.Lock()
	s.running = true
	s.rate = rate
	s.device = device
	s.apps = append([]string(nil), apps...)
	s.mu.Unlock()
	if s.cfg.OnFormat != nil {
		s.cfg.OnFormat(rate)
	}
	if s.cfg.OnDevice != nil {
		s.cfg.OnDevice(device)
	}
}

func (s *Source) streamClosed() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// reportError counts an error and delivers it to OnError.
func (s *Source) reportError(err error) {
	s.mu.Lock()
	s.errors++
	onError := s.cfg.OnError
	s.mu.Unlock()
	if onError != nil {
		onError(err)
	}
}

func (s *Source) addFrames(n uint32) {
	s.frames.Add(uint64(n))
}

// addDropped records samples captured but dropped before the ring, such as a
// buffer larger than the stream's realtime scratch.
func (s *Source) addDropped(n uint64) {
	s.dropped.Add(n)
}

// wait sleeps d or until stopped, reporting whether capture should go on.
func (s *Source) wait(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-s.stop:
		return false
	case <-t.C:
		return true
	}
}
