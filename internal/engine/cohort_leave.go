package engine

import (
	"slices"
)

// Per-target leave: one row's disconnect drops that target from the plan, and
// the remaining targets rebuild as one cohort.

// DisconnectTarget removes one target from the session. The remaining
// targets rebuild as one cohort and keep playing; removing the last target
// ends the session like a full disconnect.
func (e *Engine) DisconnectTarget(id string) {
	e.send(command{kind: cmdDisconnectTarget, deviceID: id})
}

// disconnectAll ends the session and clears the wanted set. It is the cancel
// and full-disconnect path; a per-target leave only reaches it with the last
// target.
func (e *Engine) disconnectAll() {
	e.notice = SessionNotice{}
	e.autoTried = true
	// A cancel or a disconnect ends the attempt, so its failure is stale.
	// Clear it before the cancel publishes the idle snapshot.
	e.clearError()
	e.life.userCancel()
	if e.tr != nil && e.tr.Active() {
		e.tr.Stop()
	}
	e.life.clearWanted()
	e.closeSession(false)
	e.connectedName = ""
	e.pinRequired = false
	e.clearPasswordPrompt()
	e.publish()
}

// leaveTarget drops one key from the live plan or the wanted set. A live
// cohort rebuilds with the rest and holds the connection mute, as an add
// does; removing the last key ends the session.
func (e *Engine) leaveTarget(key string) {
	if key == "" {
		return
	}
	e.notice = SessionNotice{}
	// A cohort that exists counts as live even before its first state: the
	// sender is between commands, and the connect that created it must be
	// undone by a rebuild, not by shrinking the wanted set.
	live := e.tr != nil && e.plan.live()
	keys := e.plan.keys()
	if !live {
		keys = e.life.wantedKeys
	}
	remaining := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != key {
			remaining = append(remaining, k)
		}
	}
	if len(remaining) == len(keys) {
		e.logf("cohort: %s is not in the session", e.nameFor(key))
		e.publish()
		return
	}
	if !live {
		if len(remaining) == 0 {
			e.disconnectAll()
			return
		}
		e.life.wantedKeys = remaining
		e.publish()
		return
	}
	if len(remaining) == 0 {
		e.disconnectAll()
		return
	}
	plan, err := e.buildPlan(remaining)
	if err != nil {
		e.failPlan(err)
		e.logf("disconnect: %s: %v", e.nameFor(key), err)
		e.publish()
		return
	}
	e.logf("cohort: leaving %s", e.nameFor(key))
	e.startPlan(plan, true)
}

// sessionLost handles a dead transport. When one member closed while others
// were in the session, the rest continue without it and the lost target leaves
// the wanted set, so no retry drags the survivors down. A session whose only
// target died keeps the existing close-and-retry path.
func (e *Engine) sessionLost(key string, auth bool) {
	if key != "" && len(e.life.wantedKeys) > 1 && slices.Contains(e.life.wantedKeys, key) {
		// The lost receiver is gone (or taken over) whether or not the
		// survivors can rebuild. dropLost removes its key either way, so no
		// later retry fights the receiver that owns it now.
		remaining := e.life.dropLost(key)
		if plan, err := e.buildPlan(remaining); err == nil {
			name := e.nameFor(key)
			if name == "" {
				name = key
			}
			if !auth {
				// A rejected password explains itself with the prompt; a
				// member that merely vanished needs the lost notice.
				e.notice = SessionNotice{Kind: NoticeMemberLost, DeviceID: key, Name: name, Remaining: len(remaining)}
			}
			e.logf("cohort: %s left the session; continuing with %d target(s)", name, len(remaining))
			// The session continues, so the retry onClosed scheduled must not
			// fire into the new cohort. The survivor rebuild is automatic: it
			// must not count as a user connect and light a standby display.
			e.life.memberLostRebuilt()
			e.startPlan(plan, true)
			return
		} else {
			e.logf("cohort: continuing without %s failed: %v", e.nameFor(key), err)
		}
	}
	e.closeSession(false)
	e.publish()
}
