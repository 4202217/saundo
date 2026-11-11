package app

import (
	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
)

// powerHoldReason is what macOS lists as the reason the app stays at full
// speed.
const powerHoldReason = "Streaming to AirPlay"

// powerHold keeps macOS from putting the hidden tray app into App Nap while
// a session runs. It is main-thread only: mygo.Power.KeepRunning calls
// onMain, which blocks a caller off the main thread, and Engine.Stop waits
// there for the engine goroutine, so acquiring from onEngineChange would
// deadlock quit.
type powerHold struct {
	acquire func(reason string) func()
	release func()
	held    bool
	stopped bool
}

// observe turns the hold on when a session becomes active and off when it
// returns to idle. A nil acquire is a no-op, so the zero App in tests needs
// no running mygo application.
func (h *powerHold) observe(active bool) {
	if h.stopped || active == h.held {
		return
	}
	if !active {
		h.held = false
		h.release()
		h.release = nil
		return
	}
	if h.acquire == nil {
		return
	}
	h.release = h.acquire(powerHoldReason)
	h.held = h.release != nil
}

// stop releases the hold before Engine.Stop and latches, so a session
// snapshot still queued on the engine goroutine cannot reacquire it mid-quit.
func (h *powerHold) stop() {
	if h.stopped {
		return
	}
	h.stopped = true
	if h.held {
		h.held = false
		h.release()
		h.release = nil
	}
}

// sessionActive reports whether a connection or stream is in progress.
// A takeover is terminal and the receiver belongs to another sender, so it
// releases the hold even if the transport has not reported idle yet.
func sessionActive(snap engine.Snapshot) bool {
	return !snap.TakenOver && (snap.State != sender.StateIdle || snap.Reconnecting)
}
