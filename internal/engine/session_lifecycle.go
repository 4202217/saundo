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

// rememberSessionStats stores the retransmit ratio in each target's registry
// entry and, for a pair or combo, in each member's own entry too. It reads
// the live transport instead of the last publish so a close that lands
// between two publishes, or a close the user started, still records the
// session. A synthesized pair or combo key may have no history of its own;
// the member entries are what the next plan reads for its duplicate policy.
func (e *Engine) rememberSessionStats() {
	health := e.targetHealth()
	if health == nil {
		return
	}
	saved := false
	for _, t := range e.plan.targets {
		h, ok := health[t.key]
		if !ok {
			continue
		}
		entry, ok := e.reg.Get(t.key)
		if !ok {
			continue
		}
		if entry.LastRetransmits == h.Retransmits && entry.LastSentBlocks == h.SentBlocks {
			continue
		}
		entry.LastRetransmits = h.Retransmits
		entry.LastSentBlocks = h.SentBlocks
		e.reg.Put(entry)
		saved = true
	}
	// A pair or combo's folded health also lands on each member, so its next
	// plan can size the copies from the receiver that actually had loss.
	stats := e.tr.Stats()
	if len(stats) == len(e.plan.members) {
		for i, pm := range e.plan.members {
			entry, ok := e.reg.Get(pm.device.ID)
			if !ok {
				continue
			}
			if entry.LastRetransmits == stats[i].Retransmits && entry.LastSentBlocks == stats[i].SentBlocks {
				continue
			}
			entry.LastRetransmits = stats[i].Retransmits
			entry.LastSentBlocks = stats[i].SentBlocks
			e.reg.Put(entry)
			saved = true
		}
	}
	if saved {
		e.saveRegistry()
	}
}

// closeSession tears the session down. holdLocalMute keeps the connection
// mute in place for a rebuild, where the user's session continues over a new
// cohort; every other close ends the mute with the session. wantedKeys is not
// cleared here: a close during a rebuild must keep the requested set.
func (e *Engine) closeSession(holdLocalMute bool) {
	// Record the session's loss ratio before the transport goes away. User
	// disconnect, quit, and every rebuild end here; without this write the
	// duplicate policy would read a zero entry and keep the first-session
	// extra copy forever.
	e.rememberSessionStats()
	if !holdLocalMute {
		e.restoreLocalMute()
		// The session is over, not rebuilt: end the drop accounting so the
		// next connection opens a fresh baseline instead of inheriting this
		// session's drops and pre-stream overflow.
		e.endDropSession()
	}
	e.detachMemberRings()
	e.resetIdle()
	e.sessionStartedAt = time.Time{}
	if e.tr != nil {
		tr := e.tr
		e.tr = nil
		e.plan = sessionPlan{}
		tr.Stop()
	}
}
