package engine

import (
	"slices"
	"time"

	"saundo/internal/settings"
)

// The transport's lifecycle callbacks. They run on the engine goroutine, own
// the session's start and failure transitions, and publish once at the end.

// lastSessionFirst is the previous session's first target, for the
// pre-session selection. A settings file written before LastTargets keeps its
// old single selection through LastDeviceID. Empty before the first session.
func lastSessionFirst(cfg settings.Settings) string {
	if len(cfg.LastTargets) > 0 {
		return cfg.LastTargets[0]
	}
	return cfg.LastDeviceID
}

func (e *Engine) onLaunched(ok bool, err string) {
	e.plan.joinKey = "" // the attempt ended: no row is the user's joiner now
	if ok {
		now := e.now()
		e.sessionStartedAt = now
		e.lastLoudSeen = now
		if e.life.connectAuto {
			e.darkBaseline = time.Time{}
		} else {
			e.darkBaseline = now
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
		e.life.launchSucceeded()
		e.adoptInitialVolumes()
		e.applyLocalMute()
		e.logf("streaming to %s", e.connectedName)
		// The phase is computed before the lamp, so a session launched into
		// silence goes straight to the idle face with no state 1 flash. The
		// pacer keeps streaming compact silence either way.
		e.idle = e.idlePhase(now)
		e.applyLamp()
		// A launch, including a rebuild that adds a member, opens fresh
		// senders with zeroed counters; the stability window starts over
		// with them instead of attributing the old members' trouble to them.
		e.stability.reset()
		if !e.dropSessionActive {
			e.beginDropSession()
		}
	} else {
		e.life.connectAuto = false
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
			e.passwordRequired = true
			e.passwordDevice = failure.Name
			e.passwordDeviceID = failure.DeviceID
			e.passwordTargetID = failure.TargetKey
			if e.passwordTargetID == "" {
				e.passwordTargetID = failure.DeviceID
			}
			e.fail(ErrPasswordRejected, failure.Name)
			e.life.passwordRejected()
			e.logf("session failed: %s did not accept the password", failure.Name)
		} else {
			e.failText(err)
			e.logf("session failed: %s", err)
			e.armReconnect()
		}
	}
	e.publish()
}

func (e *Engine) onClosed() {
	e.rememberSessionStats()
	e.connectedName = ""
	e.receiverLatency = 0
	e.sessionStartedAt = time.Time{}
	e.stability.reset()
	e.endDropSession()
	e.pinRequired = false
	e.clearPasswordPrompt()
	e.armReconnect()
	e.publish()
}

func (e *Engine) onPreempted(reason string) {
	e.rememberSessionStats()
	if len(e.plan.targets) > 1 {
		// A takeover of one receiver is a member loss, not the session's end:
		// the sender's close rebuilds the other targets and their local mute
		// stays held. The lost notice comes from sessionLost.
		e.logf("session taken over: %s; the other targets continue", reason)
		return
	}
	e.life.preempted()
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
