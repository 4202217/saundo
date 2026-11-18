package engine

import (
	"errors"
	"slices"

	"saundo/internal/receivers"
)

// maybeAutoConnect connects the launch set once, when its anchor is
// addressable. The registry's single auto-connect entry anchors the attempt;
// when that row belongs to the last session's combination, the whole
// combination launches as one cohort, and otherwise the anchor launches
// alone. A manual row connects immediately, a discovered one on the first
// snapshot that reports it online. A user connect or disconnect ends the
// launch phase, and a failure falls through to the ordinary reconnect policy.
func (e *Engine) maybeAutoConnect() bool {
	if e.autoTried {
		return false
	}
	anchor, ok := receivers.AutoTarget(e.reg)
	if !ok {
		e.autoTried = true
		return false
	}
	if e.tr != nil || e.reconnectWanted {
		e.autoTried = true
		return false
	}
	keys, views, pending := e.autoConnectSet(anchor)
	if pending {
		return false // the anchor is not addressable yet; wait for discovery
	}
	// The anchor resolved into an attempt or a skip; either ends the phase.
	e.autoTried = true
	if len(keys) == 0 {
		return false
	}
	e.connectLaunch(keys, views, anchor)
	return true
}

// autoConnectSet resolves the launch attempt: the anchor plus, when the
// anchor belongs to the last session's combination, the rest of that
// combination in its recorded order. Every kept key resolves to a wire view;
// an offline remembered row reads from its cached endpoint, the same view a
// user click on the offline row would validate. A member that no longer
// resolves or has no endpoint yet drops with a line and waits for a later
// launch. The anchor's own absence or missing endpoint reports pending, so
// the caller waits for discovery, and a stale unpaired group or the local
// receiver ends the attempt with nothing to open.
func (e *Engine) autoConnectSet(anchor string) (keys []string, views map[string]receivers.Target, pending bool) {
	set := []string{anchor}
	if slices.Contains(e.setsValue.LastTargets, anchor) {
		set = append([]string(nil), e.setsValue.LastTargets...)
	}
	views = make(map[string]receivers.Target, len(set))
	keys = make([]string, 0, len(set))
	for _, key := range set {
		t, found := e.target(key)
		if !found {
			if key == anchor {
				return nil, nil, true
			}
			e.logf("auto-connect: dropping %s; it is no longer remembered", key)
			continue
		}
		d := t.Device
		stalePair := t.Entry.IsGroup() && !d.Online && e.pairIsStandalone(t.Entry)
		if key == anchor {
			// The anchor's own quirks end the whole attempt, as before.
			if stalePair {
				e.logf("auto-connect target %s is a stale stereo pair; skipping", t.Entry.DisplayName())
				return nil, nil, false
			}
			if d.Local {
				e.logf("auto-connect target %s is the local receiver; skipping", d.Name)
				return nil, nil, false
			}
		} else {
			if stalePair {
				e.logf("auto-connect: dropping %s; it is a stale stereo pair", t.Entry.DisplayName())
				continue
			}
			if d.Local {
				// A stored flag can outlive the receiver becoming local; a
				// member that became one cannot join a cohort.
				e.logf("auto-connect: dropping %s; it is the local receiver", d.Name)
				continue
			}
		}
		if !d.Online {
			// The registry remembers the address, so a launch does not have
			// to wait for mDNS. Discovery keeps running and replaces this
			// view. The cached view is addressable by definition, which a
			// merged offline row is not: that is what distinguishes launch
			// from a user click.
			cached, ok := receivers.CachedDevice(t.Entry, e.hasPassword(t.Entry.Key))
			if !ok {
				if key == anchor {
					return nil, nil, true
				}
				e.logf("auto-connect: dropping %s; it has no remembered endpoint yet", d.Name)
				continue
			}
			t.Device = cached
			d = cached
		}
		if d.Host == "" || d.Port == 0 {
			if key == anchor {
				return nil, nil, true
			}
			e.logf("auto-connect: dropping %s; it has no address yet", d.Name)
			continue
		}
		keys = append(keys, key)
		views[key] = t
	}
	return keys, views, false
}

// connectLaunch opens the auto-connect cohort. It mirrors the user-click
// path's guards and reset without the single-key intent policy: every key
// validates against its resolved view, one plan opens, and the anchor is the
// joiner whose row offers the cancel.
func (e *Engine) connectLaunch(keys []string, views map[string]receivers.Target, anchor string) {
	// A launch supersedes the previous attempt's state: a stale PIN dialog,
	// a takeover notice, and the old failure all end here.
	e.userDisconnect = false
	e.reconnectCount, e.reconnectNum = 0, 0
	e.connectAuto = false
	e.pinRequired, e.pinDevice = false, ""
	e.clearError()
	e.takenOver = false
	e.notice = SessionNotice{}
	for _, key := range keys {
		if err := e.validateTarget(views[key]); err != nil {
			if errors.Is(err, errAwaitInput) { // password prompt is now set
				e.publish()
				return
			}
			e.failPlan(err)
			e.logf("auto-connect: %s refused: %v", views[key].Device.Name, err)
			e.publish()
			return
		}
	}
	e.clearPasswordPrompt()
	plan, err := e.buildPlan(keys)
	if err != nil {
		e.failPlan(err) // clock mix, duplicate member, structural target error
		e.publish()
		return
	}
	plan.joinKey = anchor
	e.startPlan(plan, false)
}

// pairIsStandalone reports whether discovery currently advertises every
// remembered member of a stereo pair as its own standalone receiver. The
// unpaired pair's key can linger in the registry, but its members no longer
// play as one.
func (e *Engine) pairIsStandalone(entry receivers.Entry) bool {
	if len(entry.Members) == 0 {
		return false
	}
	singles := make(map[string]bool, len(e.discovered))
	for _, d := range e.discovered {
		if !d.IsStereoGroup() {
			singles[d.ID] = true
		}
	}
	for _, m := range entry.Members {
		if m.ID == "" || !singles[m.ID] {
			return false
		}
	}
	return true
}
