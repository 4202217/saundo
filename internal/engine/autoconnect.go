package engine

import (
	"errors"

	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
)

// maybeAutoConnect connects the flagged receiver set once, when every row in
// it is addressable: the registry's auto-connect flags are the launch set,
// the same rows a user click would use, and they open as one cohort in
// registry order. A manual row is addressable immediately, a discovered one
// on the first snapshot that reports it online, and a remembered one through
// its cached endpoint. A user connect or disconnect ends the launch phase,
// and a failure falls through to the ordinary reconnect policy.
func (e *Engine) maybeAutoConnect() bool {
	if e.autoTried {
		return false
	}
	flagged := receivers.AutoTargets(e.reg)
	if len(flagged) == 0 {
		e.autoTried = true
		return false
	}
	if e.tr != nil || e.reconnectWanted {
		e.autoTried = true
		return false
	}
	keys, views, pending := e.autoConnectSet(flagged)
	if pending {
		return false // a flagged row has no endpoint yet; wait for discovery
	}
	// Every flagged row resolved into an attempt, a drop, or a skip; either
	// way the launch phase ends here.
	e.autoTried = true
	if len(keys) == 0 {
		return false
	}
	e.connectLaunch(keys, views, keys[0])
	return true
}

// autoConnectSet resolves the flagged launch set. Every kept key resolves to
// a wire view; an offline remembered row reads from its cached endpoint, the
// same view a user click on the offline row would validate. A row that no
// longer resolves or is structurally unusable (a stale unpaired group, the
// local receiver) drops with a line; a row that merely has no endpoint yet
// reports pending, so the caller waits for discovery rather than launching
// a partial set.
func (e *Engine) autoConnectSet(flagged []string) (keys []string, views map[string]receivers.Target, pending bool) {
	views = make(map[string]receivers.Target, len(flagged))
	keys = make([]string, 0, len(flagged))
	for _, key := range flagged {
		t, found := e.target(key)
		if !found {
			e.logf("auto-connect: dropping %s; it is no longer remembered", key)
			continue
		}
		if t.Device.StereoID != "" && len(t.Device.Members) == 0 {
			// A receiver advertised as one half of a pair is not a standalone
			// target; flagged beside its pair it would fail the shared plan.
			e.logf("auto-connect: %s is a stereo-pair member; skipping", t.Entry.DisplayName())
			continue
		}
		if t.Entry.IsCombo() {
			// A combo launches from its members' own rows: an online member
			// takes its live view, an offline one reads its cached endpoint,
			// and a member with no address yet holds the launch for discovery.
			members := make([]device.Device, 0, len(t.Entry.Members))
			broken := false
			for _, m := range t.Entry.Members {
				mt, mfound := e.target(m.ID)
				if !mfound {
					e.logf("auto-connect: %s lost member %s; skipping", t.Entry.DisplayName(), m.Name)
					broken = true
					break
				}
				md := mt.Device
				if !md.Online {
					cached, ok := receivers.CachedDevice(mt.Entry, e.hasPassword(mt.Entry.Key))
					if !ok {
						return nil, nil, true
					}
					md = cached
				}
				if md.Host == "" || md.Port == 0 {
					return nil, nil, true
				}
				members = append(members, md)
			}
			if broken {
				continue
			}
			t.Device = receivers.ComboDevice(t.Entry, members)
			keys = append(keys, key)
			views[key] = t
			continue
		}
		d := t.Device
		if t.Entry.IsGroup() && !d.Online && e.pairIsStandalone(t.Entry) {
			e.logf("auto-connect: %s is a stale stereo pair; skipping", t.Entry.DisplayName())
			continue
		}
		if d.Local {
			e.logf("auto-connect: %s is the local receiver; skipping", d.Name)
			continue
		}
		if !d.Online {
			// The registry remembers the address, so a launch does not have
			// to wait for mDNS. Discovery keeps running and replaces this
			// view. The cached view is addressable by definition, which a
			// merged offline row is not: that is what distinguishes launch
			// from a user click.
			cached, ok := receivers.CachedDevice(t.Entry, e.hasPassword(t.Entry.Key))
			if !ok {
				return nil, nil, true
			}
			t.Device = cached
			d = cached
		}
		if d.Host == "" || d.Port == 0 {
			return nil, nil, true
		}
		keys = append(keys, key)
		views[key] = t
	}
	return keys, views, false
}

// connectLaunch opens the auto-connect cohort. It mirrors the user-click
// path's guards and reset without the single-key intent policy: every key
// validates against its resolved view, one plan opens, and the first flagged
// key is the joiner whose row offers the cancel.
func (e *Engine) connectLaunch(keys []string, views map[string]receivers.Target, joiner string) {
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
	plan.joinKey = joiner
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
