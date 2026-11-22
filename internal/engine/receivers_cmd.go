package engine

import (
	"saundo/internal/receivers"
)

// handleAddReceiver creates a manual receiver. Validation happens here so
// both the UI preview and the engine share one rule set; the password
// moves to the DPAPI store and never reaches receivers.json.
func (e *Engine) handleAddReceiver(spec receivers.Spec) {
	if err := spec.Validate(); err != nil {
		e.failText(err.Error())
		e.publish()
		return
	}
	spec = spec.Resolved()
	key := receivers.NewManualKey()
	e.reg.Put(spec.Entry(key))
	e.clearError()
	if spec.Password != "" {
		if err := e.savePassword(key, spec.Password); err != nil {
			e.fail(ErrSavePassword, err.Error())
		}
	}
	if err := e.recvStore.Save(e.reg); err != nil {
		e.logf("receivers save: %v", err)
		e.fail(ErrSaveReceiver, err.Error())
	}
	e.rebuildTargets()
	e.publish()
}

// handleUpdateReceiver applies one patch. Address and type edits only reach
// a manual row through the edit dialog; discovered and group rows accept the
// per-row settings. A delay change to the receiver the engine is working
// with restarts the session, because the sender applies latency only at
// stream SETUP; the UI debounces like the global latency control.
func (e *Engine) handleUpdateReceiver(key string, patch receivers.Patch) {
	entry, ok := e.reg.Get(key)
	if !ok {
		e.fail(ErrReceiverMissing)
		e.publish()
		return
	}
	if entry.IsManual() {
		manual := receivers.Spec{
			Name: entry.Name, Type: entry.Type, Host: entry.Host, Port: entry.Port,
		}
		if patch.Name != nil {
			manual.Name = *patch.Name
		}
		if patch.Type != nil {
			manual.Type = *patch.Type
		}
		if patch.Host != nil {
			manual.Host = *patch.Host
		}
		if patch.Port != nil {
			manual.Port = *patch.Port
		}
		if patch.Name != nil || patch.Type != nil || patch.Host != nil || patch.Port != nil {
			manual = manual.Resolved()
			if err := manual.Validate(); err != nil {
				e.failText(err.Error())
				e.publish()
				return
			}
			patch.Name = &manual.Name
			patch.Type = &manual.Type
			patch.Host = &manual.Host
			patch.Port = &manual.Port
		}
	} else {
		// A discovered receiver's identity and pairing path come from its
		// advertisement; only the per-row settings are editable.
		patch.Name, patch.Type, patch.Host, patch.Port = nil, nil, nil, nil
	}

	if entry.IsManual() &&
		((patch.Host != nil && *patch.Host != entry.Host) || (patch.Port != nil && *patch.Port != entry.Port)) {
		// A user-typed address change is a deliberate re-pair, not an
		// advertisement: accept the new endpoint.
		entry.PairedHost, entry.PairedPort = "", 0
	}
	if patch.Channel != nil && (entry.IsGroup() || entry.IsCombo()) && receivers.ParseChannel(*patch.Channel) != receivers.ChannelStereo {
		// A stereo pair and a combo always play stereo; a combo's sides live
		// in its member order. Ignore a non-stereo side.
		patch.Channel = nil
	}
	delayBefore := entry.DelayMS
	if patch.AutoConnect != nil {
		// The flagged set is the next launch's target list; it never starts
		// a connection mid-session, and toggling it ends a pending launch
		// wait.
		e.autoTried = true
	}
	e.reg.Put(patch.Apply(entry))
	e.clearError()
	if patch.Password != nil {
		if err := e.savePassword(key, *patch.Password); err != nil {
			e.fail(ErrSavePassword, err.Error())
		}
	}
	if err := e.recvStore.Save(e.reg); err != nil {
		e.logf("receivers save: %v", err)
		e.fail(ErrSaveReceiver, err.Error())
	}
	e.rebuildTargets()
	if patch.Channel != nil {
		// A channel edit is a per-packet mix, not a SETUP value, so the live
		// target's members take it without a rebuild.
		e.applyReceiverChannel(key)
	}
	e.publish()

	delayAfter := entry.DelayMS
	if applied, ok := e.reg.Get(key); ok {
		delayAfter = applied.DelayMS
	}
	if patch.DelayMS != nil && delayBefore != delayAfter && e.isLive(key) {
		// A delay while a reconnect waits only updates the registry: the next
		// retry's buildPlan recomputes every member's latency. With a live
		// cohort the sender applies latency only at stream SETUP, so the whole
		// cohort rebuilds with the new per-member values — under the same
		// guard the global latency apply runs, because one missing or offline
		// member would fail the launch and take the running session down.
		if e.tr != nil {
			if e.tr.WaitingForPin() {
				e.logf("delay apply: %s is waiting for a PIN; session left alone", e.connectedName)
				e.publish()
			} else if missing, offline, ok := e.sessionAddressable(); !ok {
				if missing != "" {
					e.fail(ErrLatencyDeviceMissing)
					e.logf("delay apply: device %s not found; session left running", missing)
				} else {
					e.fail(ErrLatencyOffline)
					e.logf("delay apply: %s is offline; session left running", offline)
				}
				e.publish()
			} else {
				// A delay edit is a user action even when it lands while an
				// automatic reconnect is still connecting. Start the standby
				// clock at the new session so the light shows the restart.
				e.life.userRebuild()
				e.rebuildSession()
			}
		}
	}
}

// handleRemoveReceiver deletes a manual receiver: its row, password, HAP
// credentials, and remembered volume. Discovered rows are managed by hiding,
// so removal there would only be undone by the next advertisement. A combo
// that linked this receiver loses the member and is unlinked with it: a
// one-sided combo is meaningless.
func (e *Engine) handleRemoveReceiver(key string) {
	entry, ok := e.reg.Get(key)
	if !ok || !entry.IsManual() {
		return
	}
	e.reg.Delete(key)
	if err := e.creds.DeletePassword(key); err != nil {
		e.logf("password delete: %v", err)
		e.fail(ErrDeletePassword, err.Error())
	}
	if err := e.creds.Delete(key); err != nil {
		e.logf("credentials delete: %v", err)
	}
	delete(e.passwords, key)
	e.forgetVolume(key)
	e.forgetLastTarget(key)
	// A combo that linked this receiver loses the member and is unlinked with
	// it: a one-sided combo is meaningless. The keys collect first — deleting
	// while ranging shifts the slice under the loop.
	var unlinked []string
	for _, other := range e.reg.Entries {
		if !other.IsCombo() {
			continue
		}
		for _, m := range other.Members {
			if m.ID == key {
				unlinked = append(unlinked, other.Key)
				break
			}
		}
	}
	for _, comboKey := range unlinked {
		combo, hadEntry := e.reg.Get(comboKey)
		e.reg.Delete(comboKey)
		if hadEntry && combo.AutoConnect {
			// The combo's launch intent returns to the members that survive, the
			// way an explicit unlink hands it back.
			for _, m := range combo.Members {
				if m.ID == key {
					continue
				}
				if survivor, ok := e.reg.Get(m.ID); ok {
					survivor.AutoConnect = true
					e.reg.Put(survivor)
				}
			}
		}
		e.forgetVolume(comboKey)
		e.forgetLastTarget(comboKey)
	}
	e.saveReceivers()
	e.dropFromSession(key)
	for _, comboKey := range unlinked {
		// The combo may be the session's target even when the removed member
		// itself is not.
		e.dropFromSession(comboKey)
	}
	e.rebuildTargets()
	e.publish()
}

// savePassword moves a receiver password into the DPAPI store and refreshes
// the engine's password-presence projection. An empty password clears it.
func (e *Engine) savePassword(key, password string) error {
	if err := e.creds.SavePassword(key, password); err != nil {
		e.logf("password save: %v", err)
		return err
	}
	if password == "" {
		delete(e.passwords, key)
	} else {
		e.passwords[key] = true
	}
	e.rebuildTargets()
	return nil
}

// isLive reports whether the engine is working with this receiver right
// now: a member of the live cohort, or a wanted key while a reconnect waits.
func (e *Engine) isLive(key string) bool {
	if e.tr != nil {
		return e.plan.has(key)
	}
	if !e.life.reconnectWanted {
		return false
	}
	for _, k := range e.life.wantedKeys {
		if k == key {
			return true
		}
	}
	return false
}
