package engine

import "time"

// lifecycle owns the session's retry-and-wanted-set decisions. One machine
// holds the state that session events and retries read and write, so the
// transitions that history got wrong — a stale retry killing a live session,
// a lost member dragged back into every retry, a rejected password burning
// the ladder — have exactly one implementation to fix and one model to
// exhaust (see .claude/skills/session-lifecycle).
//
// The machine is sans-i/o: every method takes its time and context as
// arguments and mutates only its own fields. The engine keeps the transport,
// the plan, prompts, and errors; fields here are written through the methods,
// and the few mechanical timer re-arms stay next to the code that arms them.

// reconnectStaleDelay is how long a retry that comes due while a session is
// live is pushed out.
const reconnectStaleDelay = 5 * time.Second

// maxReconnectAttempts bounds automatic retries before giving up.
const maxReconnectAttempts = 5

// retryDecision is what retryDue decided about a due retry.
type retryDecision int

const (
	// retryPushOut defers a retry that arrived while a session is live: the
	// user connected again after it was scheduled, so it must not tear that
	// session's handshake or PIN wait down.
	retryPushOut retryDecision = iota
	// retryRun releases the retry to the engine.
	retryRun
)

// lifecycle is the session's presence-and-retry state. The engine embeds it
// as one field; nothing outside the engine package touches it.
type lifecycle struct {
	// wantedKeys is the user's requested target set in order; the first is
	// primary. It outlives the plan across a failed launch, so Retry on any
	// active row retries the whole set, and only Disconnect, a takeover, and
	// shutdown clear it. It holds registry keys, never credentials.
	wantedKeys []string
	// lost holds the registry keys a member loss dropped. A lost receiver is
	// gone or taken over, so no automatic retry includes it; a setWanted that
	// names the key again is user intent and un-loses it.
	lost map[string]bool

	// reconnectWanted is true while the user's session is wanted and an
	// automatic retry may run.
	reconnectWanted bool
	// reconnectAt is when the next automatic retry fires; zero while none is
	// pending.
	reconnectAt time.Time
	// reconnectNum is how many ladder steps the current cycle consumed.
	reconnectNum int
	// reconnectCount is how many automatic retries the current target has
	// needed. A user connect or disconnect starts a new count.
	reconnectCount int

	// takenOver marks a receiver another sender owns. A takeover is terminal:
	// it must not schedule the reconnect that would fight the new sender for
	// the device.
	takenOver bool
	// userDisconnect records that the user ended the session on purpose, so
	// automatic retries stay off until the next user connect.
	userDisconnect bool
	// connectAuto marks the current attempt as automatic. A user connect
	// starts the standby dark clock; an automatic reconnect does not.
	connectAuto bool
}

// freshAttempt starts a user-driven connect: a stale PIN dialog, a takeover
// notice, the old failure, and the previous attempt's ladder all end here.
// The named keys are the user's explicit choice, so a key a member loss
// dropped is un-lost the moment the user names it again.
func (l *lifecycle) freshAttempt(keys ...string) {
	l.userDisconnect = false
	l.reconnectCount, l.reconnectNum = 0, 0
	l.connectAuto = false
	l.takenOver = false
	for _, k := range keys {
		if l.lost == nil {
			l.lost = make(map[string]bool)
		}
		delete(l.lost, k)
	}
}

// userRebuild clears the user-cancel state for a user-driven rebuild that
// keeps the session's ladder counts, as a latency or delay edit does.
func (l *lifecycle) userRebuild() {
	l.userDisconnect = false
	l.connectAuto = false
}

// userCancel records that the user ended the session on purpose and stops
// every automatic retry.
func (l *lifecycle) userCancel() {
	l.userDisconnect = true
	l.takenOver = false
	l.reconnectCount = 0
	l.cancelRetry()
}

// clearWanted drops the requested set. Only Disconnect, a takeover, and
// shutdown reach it.
func (l *lifecycle) clearWanted() {
	l.wantedKeys = nil
}

// setWanted replaces the requested set. It never un-loses: the automatic
// rebuild paths feed it subsets of the wanted set, and a plan that named a
// lost key again would be exactly the retry that must not happen. Only
// freshAttempt un-loses, because only the user's own connect may.
func (l *lifecycle) setWanted(keys []string) {
	l.wantedKeys = keys
}

// dropLost removes one key from the wanted set and records it lost, so no
// automatic retry drags it back in. It returns the survivors in order.
func (l *lifecycle) dropLost(key string) []string {
	if l.lost == nil {
		l.lost = make(map[string]bool)
	}
	l.lost[key] = true
	remaining := make([]string, 0, len(l.wantedKeys))
	for _, k := range l.wantedKeys {
		if k != key {
			remaining = append(remaining, k)
		}
	}
	l.wantedKeys = remaining
	return remaining
}

// launchSucceeded records a streaming session: the user's set is wanted
// again, the ladder restarts empty, and the attempt is no longer automatic.
func (l *lifecycle) launchSucceeded() {
	l.reconnectWanted = true
	l.reconnectAt = time.Time{}
	l.reconnectNum = 0
	l.takenOver = false
	l.connectAuto = false
}

// passwordRejected records a launch a receiver refused for credentials. A
// rejected password cannot fix itself by retrying, so the retry time clears
// and no retry is scheduled; the engine asks for a new password instead.
func (l *lifecycle) passwordRejected() {
	l.reconnectAt = time.Time{}
}

// attemptStarted counts one automatic retry against the target and marks the
// attempt automatic.
func (l *lifecycle) attemptStarted() {
	l.reconnectCount++
	l.connectAuto = true
}

// memberLostRebuilt records a member loss whose survivors rebuilt: the retry
// onClosed scheduled must not fire into the new cohort, and the rebuild is
// automatic, not a user connect.
func (l *lifecycle) memberLostRebuilt() {
	l.reconnectAt = time.Time{}
	l.reconnectNum = 0
	l.connectAuto = true
}

// preempted records that another sender took the receiver over. Terminal: the
// wanted set survives so the row keeps its Reconnect action, but no automatic
// retry may run.
func (l *lifecycle) preempted() {
	l.takenOver = true
	l.reconnectWanted = false
	l.reconnectNum = 0
	l.reconnectAt = time.Time{}
}

// cancelRetry disarms the retry timer without touching the ladder counts.
func (l *lifecycle) cancelRetry() {
	l.reconnectWanted = false
	l.reconnectAt = time.Time{}
	l.reconnectNum = 0
}

// mayAutoRetry is the automatic-retry gate. The engine contributes the
// context it owns: whether the app is stopping, whether reconnects are
// enabled, and whether a target is selected.
func (l *lifecycle) mayAutoRetry(stopping, enabled, hasSelection bool) bool {
	return !l.userDisconnect && !stopping && !l.takenOver && enabled && hasSelection
}

// scheduleRetry arms the next ladder step and returns its delay. The engine
// logs and publishes.
func (l *lifecycle) scheduleRetry(now time.Time) time.Duration {
	delay := reconnectBackoff[min(l.reconnectNum, len(reconnectBackoff)-1)]
	l.reconnectNum++
	l.reconnectWanted = true
	l.reconnectAt = now.Add(delay)
	return delay
}

// planRetry is the ladder step: the next automatic retry when the budget
// holds, or give-up once it has run out. It returns the armed delay and
// whether a retry was scheduled.
func (l *lifecycle) planRetry(now time.Time) (time.Duration, bool) {
	if l.reconnectNum >= maxReconnectAttempts {
		l.giveUpRetry()
		return 0, false
	}
	return l.scheduleRetry(now), true
}

// giveUpRetry stops the ladder after the budget ran out.
func (l *lifecycle) giveUpRetry() {
	l.reconnectWanted = false
}

// retryDue decides a retry that came due. A retry while a session is live is
// stale — the user connected again after it was scheduled — so it is pushed
// out instead of tearing that session's handshake or PIN wait down.
// Otherwise the pending time is consumed and the retry runs.
func (l *lifecycle) retryDue(active bool, now time.Time) retryDecision {
	if active {
		l.reconnectAt = now.Add(reconnectStaleDelay)
		return retryPushOut
	}
	l.reconnectAt = time.Time{}
	return retryRun
}
