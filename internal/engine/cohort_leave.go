package engine

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
	e.autoTried = true
	e.userDisconnect = true
	e.reconnectCount = 0
	// A cancel or a disconnect ends the attempt, so its failure is stale.
	// Clear it before reconnectPendingOff publishes the idle snapshot.
	e.clearError()
	e.takenOver = false
	e.reconnectWanted = false
	e.reconnectPendingOff()
	if e.tr != nil && e.tr.Active() {
		e.tr.Stop()
	}
	e.wantedKeys = nil
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
	// A cohort that exists counts as live even before its first state: the
	// sender is between commands, and the connect that created it must be
	// undone by a rebuild, not by shrinking the wanted set.
	live := e.tr != nil && e.plan.live()
	keys := e.plan.keys()
	if !live {
		keys = e.wantedKeys
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
		e.wantedKeys = remaining
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
