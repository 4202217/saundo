package engine

import "time"

// The transport's lifecycle callbacks. They run on the engine goroutine, own
// the session's start and failure transitions, and publish once at the end.

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
		if e.setsValue.LastDeviceID != e.selectedID {
			e.setsValue.LastDeviceID = e.selectedID
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

// rememberSessionStats stores the session's retransmit ratio for the next
// launch's duplicate policy.
func (e *Engine) rememberSessionStats() {
	key := e.selectedID
	if key == "" {
		return
	}
	entry, ok := e.reg.Get(key)
	if !ok {
		return
	}
	diag := e.snap.Diagnostics
	entry.LastRetransmits = diag.Retransmits
	entry.LastSentBlocks = diag.SentBlocks
	e.reg.Put(entry)
	e.saveRegistry()
}
