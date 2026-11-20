package engine

import (
	"slices"
	"strings"

	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// The stereo combo lifecycle: create over two receiver keys, split back into
// standalone rows, and swap the two sides. A combo is registry configuration
// — one volume, one auto-connect flag, one hidden flag on the combo key —
// whose members keep their own credentials and endpoints. The plan treats a
// combo like a remembered pair, minus the pair's own protocol: no group UUID,
// no SETPEERS, and a follower clock per member instead of a sender master.

// LinkReceivers creates a stereo combo: the first key plays the left side,
// the second the right. Name is the combo's display name; empty derives
// "A + B" from the two receivers' names.
func (e *Engine) LinkReceivers(left, right, name string) {
	e.send(command{kind: cmdLinkReceivers, ids: []string{left, right}, name: name})
}

// UnlinkCombo deletes a stereo combo. Its member receivers return to
// standalone rows with their own settings.
func (e *Engine) UnlinkCombo(key string) {
	e.send(command{kind: cmdUnlinkCombo, deviceID: key})
}

// SwapComboSides flips a stereo combo's left and right members. A live combo
// takes the new sides without a rebuild; the mix runs per packet.
func (e *Engine) SwapComboSides(key string) {
	e.send(command{kind: cmdSwapComboSides, deviceID: key})
}

// linkable rejects a receiver that cannot be a combo member. Two sides of one
// input stream need two AirPlay 2 receivers: the per-packet channel mix runs
// on the ALAC stream, and an AirPlay 1 member would play full stereo over the
// plaintext path. A receiver already inside a pair or combo, the local host,
// and a missing row all refuse.
func (e *Engine) linkable(key string) (receivers.Target, bool) {
	t, ok := e.target(key)
	if !ok {
		return t, false
	}
	if t.Device.Local || t.Device.IsStereoGroup() || t.Entry.IsGroup() || t.Entry.IsCombo() {
		return t, false
	}
	if !t.Device.AirPlay2 {
		return t, false
	}
	return t, true
}

// comboInUse reports whether any combo entry already links key.
func (e *Engine) comboInUse(key string) bool {
	for _, entry := range e.reg.Entries {
		if !entry.IsCombo() {
			continue
		}
		for _, m := range entry.Members {
			if m.ID == key {
				return true
			}
		}
	}
	return false
}

// handleLinkReceivers creates the combo. Linking a receiver that plays, or
// waits in a wanted set, ends that session first: the plan cannot keep two
// standalone keys that the registry now folds into one row.
func (e *Engine) handleLinkReceivers(ids []string, name string) {
	defer e.publish()
	if len(ids) != 2 || ids[0] == ids[1] {
		e.fail(ErrComboInvalid)
		return
	}
	left, ok := e.linkable(ids[0])
	right, ok2 := e.linkable(ids[1])
	if !ok || !ok2 || e.comboInUse(ids[0]) || e.comboInUse(ids[1]) {
		e.fail(ErrComboInvalid)
		return
	}
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		name = trimmed
	} else {
		name = strings.Join([]string{left.Device.Name, right.Device.Name}, " + ")
	}
	entry := receivers.Entry{
		Key:  receivers.NewComboKey(),
		Kind: receivers.KindCombo,
		Name: name,
		Members: []receivers.Member{
			{ID: ids[0], Name: left.Device.Name},
			{ID: ids[1], Name: right.Device.Name},
		},
	}
	e.reg.Put(entry)
	e.clearError()
	e.saveReceivers()
	e.forgetLastTarget(ids[0])
	e.forgetLastTarget(ids[1])
	e.dropFromSession(ids[0])
	e.dropFromSession(ids[1])
	e.rebuildTargets()
}

// handleUnlinkCombo deletes the combo. A live combo ends its session; the
// members keep playing nothing until the user connects them again.
func (e *Engine) handleUnlinkCombo(key string) {
	defer e.publish()
	entry, ok := e.reg.Get(key)
	if !ok || !entry.IsCombo() {
		return
	}
	e.reg.Delete(key)
	e.forgetVolume(key)
	e.forgetLastTarget(key)
	e.clearError()
	e.saveReceivers()
	e.dropFromSession(key)
	e.rebuildTargets()
}

// handleSwapComboSides flips the combo's member order. The live plan's member
// window keeps its device order, so each side takes the opposite channel in
// place; the next plan reads the new order from the registry.
func (e *Engine) handleSwapComboSides(key string) {
	defer e.publish()
	entry, ok := e.reg.Get(key)
	if !ok || !entry.IsCombo() || len(entry.Members) != 2 {
		return
	}
	entry.Members = []receivers.Member{entry.Members[1], entry.Members[0]}
	e.reg.Put(entry)
	e.clearError()
	e.saveReceivers()
	if e.tr != nil && e.plan.has(key) {
		e.tr.SetMemberChannel(key, 0, sender.ChannelRight)
		e.tr.SetMemberChannel(key, 1, sender.ChannelLeft)
	}
	e.rebuildTargets()
}

// saveReceivers persists the registry, surfacing a failed write as the save
// error the receiver commands share.
func (e *Engine) saveReceivers() {
	if err := e.recvStore.Save(e.reg); err != nil {
		e.logf("receivers save: %v", err)
		e.fail(ErrSaveReceiver, err.Error())
	}
}

// dropFromSession ends the session when key is part of it: a live or wanted
// target cannot keep playing a plan whose receiver the user just removed or
// folded into a combo.
func (e *Engine) dropFromSession(key string) {
	wanted := false
	for _, k := range e.wantedKeys {
		if k == key {
			wanted = true
			break
		}
	}
	if e.selectedID != key && !e.plan.has(key) && !wanted {
		return
	}
	if e.selectedID == key {
		e.selectedID = ""
	}
	e.userDisconnect = true
	e.reconnectPendingOff()
	e.wantedKeys = nil
	e.pinRequired, e.pinDevice = false, ""
	e.clearPasswordPrompt()
	e.notice = SessionNotice{}
	if e.tr != nil {
		e.tr.Stop()
	}
	e.closeSession(false)
	e.connectedName = ""
}

// forgetVolume drops a key's remembered volume.
func (e *Engine) forgetVolume(key string) {
	if e.setsValue.VolumeByDevice == nil {
		return
	}
	if _, ok := e.setsValue.VolumeByDevice[key]; ok {
		delete(e.setsValue.VolumeByDevice, key)
		e.markSettingsDirty()
	}
}

// forgetLastTarget drops key from the remembered combination.
func (e *Engine) forgetLastTarget(key string) {
	if len(e.setsValue.LastTargets) == 0 {
		return
	}
	kept := slices.DeleteFunc(slices.Clone(e.setsValue.LastTargets), func(k string) bool {
		return k == key
	})
	if len(kept) != len(e.setsValue.LastTargets) {
		e.setsValue.LastTargets = kept
		e.markSettingsDirty()
	}
}
