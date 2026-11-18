package engine

import (
	"slices"
	"time"

	"saundo/internal/settings"
)

// The transport's lifecycle callbacks. They run on the engine goroutine, own
// the session's start and failure transitions, and publish once at the end.

// lastSessionFirst is the previous session's first target, for the
// pre-session selection. Empty before the first session.
func lastSessionFirst(cfg settings.Settings) string {
	if len(cfg.LastTargets) > 0 {
		return cfg.LastTargets[0]
	}
	return ""
}

func (e *Engine) onLaunched(ok bool, err string) {
	e.plan.joinKey = "" // the attempt ended: no row is the user's joiner now
	if ok {
		now := e.now()
		e.sessionStartedAt = now
		// The dark threshold counts from the last RTP push: a user connect
		// starts it, an automatic reconnect does not.
		if e.connectAuto {
			e.darkBaseline = time.Time{}
		} else {
			e.darkBaseline = now
		}
		e.connectAuto = false
		if e.lastPushAt.IsZero() {
			e.lastPushAt = now
		}
		e.connectedName = e.plan.connectedName()
		if e.connectedName == "" {
			e.connectedName = e.nameFor(e.selectedID)
		}
		// The session's target set is what a launch restores and what the
		// UI's pre-session selection seeds from.
		keys := e.plan.keys()
		if !slices.Equal(e.setsValue.LastTargets, keys) {
			e.setsValue.LastTargets = keys
			e.markSettingsDirty()
		}
		e.clearError()
		e.takenOver = false
		e.reconnectWanted = true
		e.reconnectNum = 0
		e.reconnectAt = time.Time{}
		e.adoptInitialVolumes()
		e.applyLocalMute()
		e.logf("streaming to %s", e.connectedName)
		// The sender arms its pacer before reporting the launch. If the
		// capture is already silent, stop it here so the stream never pushes a
		// stale cushion, then derive the phase from the actual push state. The
		// phase is computed before the lamp, so a reconnect past the threshold
		// goes straight to dark with no state 1 flash.
		if e.tr != nil && e.tr.Pushing() && now.Sub(e.silence.LastLoud()) >= audioPauseAfter {
			if e.tr.SetPaused(true) {
				e.silence.SetPaused(true)
			} else {
				e.pauseUnsupported = true
			}
		}
		e.idle = e.idlePhase(now)
		e.applyLamp()
		if !e.dropSessionActive {
			e.beginDropSession()
		}
	} else {
		e.connectAuto = false
		e.restoreLocalMute()
		e.connectedName = ""
		e.receiverLatency = 0
		e.sessionStartedAt = time.Time{}
		e.endDropSession()
		failure := LaunchFailure{}
		if e.tr != nil {
			failure = e.tr.Failure()
		}
		if failure.Auth {
			// A rejected password cannot fix itself by retrying. Keep the rest
			// of the session and ask for a new password on that receiver.
			e.passwordRequired = true
			e.passwordDevice = failure.Name
			e.passwordDeviceID = failure.DeviceID
			e.passwordTargetID = failure.TargetKey
			if e.passwordTargetID == "" {
				e.passwordTargetID = failure.DeviceID
			}
			e.fail(ErrPasswordRejected, failure.Name)
			e.reconnectAt = time.Time{}
			e.logf("session failed: %s did not accept the password", failure.Name)
		} else {
			e.failText(err)
			e.logf("session failed: %s", err)
			if e.reconnectShouldRetry() && e.reconnectAt.IsZero() {
				e.planReconnect()
			}
		}
	}
	e.publish()
}

func (e *Engine) onClosed() {
	e.rememberSessionStats()
	e.connectedName = ""
	e.receiverLatency = 0
	e.sessionStartedAt = time.Time{}
	e.endDropSession()
	e.pinRequired = false
	e.clearPasswordPrompt()
	if e.reconnectShouldRetry() && e.reconnectAt.IsZero() {
		e.planReconnect()
	}
	e.publish()
}

// onPreempted records that another sender took over the receiver. A takeover
// is terminal: it must not schedule the reconnect that would fight the new
// sender for the device.
func (e *Engine) onPreempted(reason string) {
	e.rememberSessionStats()
	e.takenOver = true
	e.reconnectWanted = false
	e.reconnectNum = 0
	e.reconnectAt = time.Time{}
	e.connectedName = ""
	e.receiverLatency = 0
	e.sessionStartedAt = time.Time{}
	e.endDropSession()
	// The receiver belongs to another sender now, and the closed control
	// channel cannot light the display. The wanted set stays so the active
	// rows keep the Reconnect action; only a user click retries.
	e.resetIdle()
	e.clearError()
	e.restoreLocalMute()
	e.logf("session taken over: %s", reason)
	e.publish()
}

// rememberSessionStats stores each target's retransmit ratio in its own
// registry entry, for that target's next launch's duplicate policy. The
// snapshot's targets carry the health folded per target; a close reads the
// last publish, the same source the connection card shows.
func (e *Engine) rememberSessionStats() {
	saved := false
	for _, t := range e.snap.Targets {
		entry, ok := e.reg.Get(t.ID)
		if !ok {
			continue
		}
		entry.LastRetransmits = t.Health.Retransmits
		entry.LastSentBlocks = t.Health.SentBlocks
		e.reg.Put(entry)
		saved = true
	}
	if saved {
		e.saveRegistry()
	}
}
