package engine

import (
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// TargetState is one target's role in the current attempt. The flyout offers
// a verb per state instead of deriving it from the session phase: a rebuild
// re-opens every member at once, so the phase alone cannot tell the target
// the user just added from a member that is merely re-establishing.
type TargetState uint8

const (
	// TargetInactive is a device with no part in the current attempt.
	TargetInactive TargetState = iota
	// TargetMember is a target in the user's session. It keeps its mixer
	// while the cohort re-establishes.
	TargetMember
	// TargetJoining is the target the user's connect started or added.
	TargetJoining
	// TargetReopening is a wanted target an automatic retry is opening.
	TargetReopening
	// TargetFailed is a wanted target whose attempt ended without a stream.
	TargetFailed
)

// TargetHealth is one target's measured session health, folded over its
// members. Zero when no transport runs or its stats do not line up with the
// plan, so the UI shows no numbers instead of a guessed one.
type TargetHealth struct {
	Retransmits uint64
	SentBlocks  uint64
	Underruns   uint64
}

// TargetControl is one active target's per-device audio settings and its role
// in the current attempt: the volume the flyout slider and the settings page
// share, and the stereo side a single receiver plays. A stereo pair is always
// stereo. A wanted target keeps an entry while a retry waits, so every active
// row has a state.
type TargetControl struct {
	ID, Name string
	Volume   float64
	Channel  string
	Stereo   bool
	// Split marks a stereo combo: the target's two receivers play one side
	// each, so the row's channel control is a locked status distinct from a
	// native pair's.
	Split  bool
	State  TargetState
	Health TargetHealth
}

// Target returns the live target's controls. An idle snapshot has no targets.
func (s Snapshot) Target(id string) (TargetControl, bool) {
	for _, t := range s.Targets {
		if t.ID == id {
			return t, true
		}
	}
	return TargetControl{}, false
}

// receiverView projects one settings row and fills the volume the receiver
// page's slider shows. The registry View has no settings source.
func (e *Engine) receiverView(t receivers.Target) receivers.View {
	v := t.View(e.passwords[t.Entry.Key])
	v.Volume = e.volumeFor(v.Key)
	return v
}

// volumeFor is the target's remembered volume, defaulting to the global value
// the single-receiver build used.
func (e *Engine) volumeFor(key string) float64 {
	if v, ok := e.setsValue.VolumeByDevice[key]; ok {
		return v
	}
	return e.setsValue.Volume
}

// channelFor is the target's normalized channel preference. An empty or
// unknown stored value reads as stereo.
func (e *Engine) channelFor(key string) sender.ChannelMode {
	entry, ok := e.reg.Get(key)
	if !ok {
		return sender.ChannelStereo
	}
	return sender.ParseChannelMode(entry.Channel)
}

// targetState is one active key's role. Only a plan key can be joining; every
// other planned key is a member, so a rebuild keeps the members' mixers and
// offers no cancel. Without a plan, a wanted key is reopening while a retry
// waits and failed once the engine stops retrying.
func (e *Engine) targetState(key string) TargetState {
	if e.plan.live() && !e.takenOver {
		if key == e.plan.joinKey {
			return TargetJoining
		}
		return TargetMember
	}
	if e.reconnectWanted && !e.reconnectAt.IsZero() {
		return TargetReopening
	}
	return TargetFailed
}

// targetHealth folds each member's sender stats onto its target. The stats
// slice follows the plan's member order; a mismatch leaves every target at
// zero instead of attributing one member's counters to another.
func (e *Engine) targetHealth() map[string]TargetHealth {
	if e.tr == nil || !e.plan.live() {
		return nil
	}
	stats := e.tr.Stats()
	if len(stats) != len(e.plan.members) {
		return nil
	}
	out := make(map[string]TargetHealth, len(e.plan.targets))
	i := 0
	for _, t := range e.plan.targets {
		var h TargetHealth
		for j := 0; j < t.count; j++ {
			s := stats[i]
			h.Retransmits += s.Retransmits
			h.SentBlocks += s.SentBlocks
			h.Underruns += s.Underruns
			i++
		}
		out[t.key] = h
	}
	return out
}

// targetControls projects the active keys, the settings map, and the registry
// onto the UI's per-device controls. A wanted key keeps an entry while a retry
// waits, so the flyout never has to synthesize one.
func (e *Engine) targetControls() []TargetControl {
	keys := e.activeKeys()
	if len(keys) == 0 {
		return nil
	}
	health := e.targetHealth()
	out := make([]TargetControl, 0, len(keys))
	for _, key := range keys {
		state := e.targetState(key)
		name, stereo, combo := "", false, false
		if t, ok := e.plan.target(key); ok {
			name, stereo, combo = t.name, t.stereo, t.combo
		} else {
			t, ok := e.target(key)
			if !ok {
				continue
			}
			name, stereo, combo = t.Device.Name, t.Device.IsStereoGroup(), t.Entry.IsCombo()
		}
		channel := e.channelFor(key)
		if stereo {
			channel = sender.ChannelStereo
		}
		out = append(out, TargetControl{
			ID:      key,
			Name:    name,
			Volume:  e.volumeFor(key),
			Channel: channel.String(),
			Stereo:  stereo,
			Split:   combo,
			State:   state,
			Health:  health[key],
		})
	}
	return out
}

// storeTargetVolume clamps and remembers one target's volume. The primary
// target also updates the global value, which becomes the default a target
// with no remembered volume adopts.
func (e *Engine) storeTargetVolume(key string, pct float64) {
	if key == "" {
		return
	}
	pct = clamp(pct, 0, 100)
	if e.setsValue.VolumeByDevice == nil {
		e.setsValue.VolumeByDevice = make(map[string]float64)
	}
	e.setsValue.VolumeByDevice[key] = pct
	if key == e.selectedID {
		e.setsValue.Volume = pct
	}
	e.markSettingsDirty()
}

// applyTargetVolume stores a user's volume and sends it to that target's live
// members.
func (e *Engine) applyTargetVolume(key string, pct float64) {
	if key == "" {
		return
	}
	pct = clamp(pct, 0, 100)
	e.storeTargetVolume(key, pct)
	if e.tr != nil {
		e.tr.SetTargetVolume(key, pct)
	}
	e.publish()
}

// onReceiverVolume adopts a hardware knob change for the member's target. A
// combo's members each poll their own hardware volume, so one member's report
// becomes the target's level and the other member takes it too — the two
// speakers converge instead of trading overwrites.
func (e *Engine) onReceiverVolume(key string, pct float64) {
	if key == "" {
		key = e.selectedID
	}
	e.storeTargetVolume(key, pct)
	if e.tr != nil {
		if t, ok := e.plan.target(key); ok && t.count > 1 {
			e.tr.SetTargetVolume(key, pct)
		}
	}
	e.publish()
}

// adoptInitialVolumes records the receiver's own volume for every plan target
// the user never set, so connecting does not jump that receiver to a default.
func (e *Engine) adoptInitialVolumes() {
	if e.tr == nil {
		return
	}
	for _, t := range e.plan.targets {
		if _, remembered := e.setsValue.VolumeByDevice[t.key]; remembered {
			continue
		}
		if pct, ok := e.tr.InitialVolumePercentFor(t.key); ok {
			e.storeTargetVolume(t.key, pct)
			if t.count > 1 {
				// A combo is one target with one volume: push the adopted level
				// so both speakers start together instead of keeping the last
				// member's hardware level.
				e.tr.SetTargetVolume(t.key, pct)
			}
		}
	}
}

// SetTargetVolume sets one target's volume in percent.
func (e *Engine) SetTargetVolume(id string, pct float64) {
	e.send(command{kind: cmdTargetVolume, deviceID: id, value: pct})
}

// applyReceiverChannel pushes a saved channel to the target's live members. A
// stereo pair stays stereo, and the change never rebuilds the session: the mix
// runs per packet. A combo's sides live in its member order, so no whole-target
// channel write ever reaches it.
func (e *Engine) applyReceiverChannel(key string) {
	if e.tr == nil || !e.isLive(key) {
		return
	}
	if e.isGroupKey(key) || e.isComboKey(key) {
		return
	}
	e.tr.SetTargetChannel(key, e.channelFor(key))
}

// isGroupKey reports whether the registry key names a stereo pair.
func (e *Engine) isGroupKey(key string) bool {
	entry, ok := e.reg.Get(key)
	return ok && entry.IsGroup()
}

// isComboKey reports whether the registry key names a stereo combo.
func (e *Engine) isComboKey(key string) bool {
	entry, ok := e.reg.Get(key)
	return ok && entry.IsCombo()
}
