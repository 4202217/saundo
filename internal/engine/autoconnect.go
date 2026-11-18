package engine

import "saundo/internal/receivers"

// maybeAutoConnect connects the launch target once, when it is addressable:
// a manual receiver immediately, a discovered one on the first snapshot that
// reports it online. A user connect or disconnect ends the launch phase, and
// a failure falls through to the ordinary reconnect policy.
func (e *Engine) maybeAutoConnect() bool {
	if e.autoTried {
		return false
	}
	key, ok := receivers.AutoTarget(e.reg)
	if !ok {
		e.autoTried = true
		return false
	}
	if e.tr != nil || e.reconnectWanted {
		e.autoTried = true
		return false
	}
	target, found := e.target(key)
	if !found {
		return false
	}
	if target.Entry.IsGroup() && !target.Device.Online && e.pairIsStandalone(target.Entry) {
		// The registry kept the pair's auto-connect flag after the user
		// unpaired it, so the group no longer advertises while both speakers
		// do. Starting the pair would fail; start idle and let the user
		// connect one and add the other.
		e.autoTried = true
		e.logf("auto-connect target %s is a stale stereo pair; skipping", target.Entry.DisplayName())
		return false
	}
	d := target.Device
	if !d.Online {
		// The registry remembers the address, so a launch does not have to
		// wait for mDNS. Discovery keeps running and replaces this view. The
		// cached view is addressable by definition, which a merged offline row
		// is not: that is what distinguishes launch from a user click.
		cached, ok := receivers.CachedDevice(target.Entry, e.hasPassword(target.Entry.Key))
		if !ok {
			return false
		}
		d = cached
		target.Device = cached
	}
	if d.Host == "" || d.Port == 0 {
		return false
	}
	if d.Local {
		// A stored flag can outlive the receiver becoming local. Skip it
		// instead of publishing a launch failure every start.
		e.autoTried = true
		e.logf("auto-connect target %s is the local receiver; skipping", d.Name)
		return false
	}
	e.autoTried = true
	e.connectResolved(key, &target)
	return true
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
