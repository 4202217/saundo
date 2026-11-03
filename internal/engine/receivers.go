package engine

import (
	"saundo/internal/receivers"
)

// AddReceiver adds a manually configured receiver and returns its key
// through the next snapshot. Manual receivers are attemptable without
// mDNS. A password on the spec moves to the DPAPI store.
func (e *Engine) AddReceiver(spec receivers.Spec) {
	e.send(command{kind: cmdAddReceiver, spec: spec})
}

// UpdateReceiver applies the patch to one receiver's configuration. Manual
// rows edit name, type, host, port, and password; every row toggles hidden,
// auto-connect, and delay.
func (e *Engine) UpdateReceiver(key string, patch receivers.Patch) {
	e.send(command{kind: cmdUpdateReceiver, deviceID: key, patch: patch})
}

// RemoveReceiver deletes a manual receiver's row, password, and remembered
// volume. Discovered rows are managed by hiding, not removal, so this is a
// no-op for them.
func (e *Engine) RemoveReceiver(key string) {
	e.send(command{kind: cmdRemoveReceiver, deviceID: key})
}

// SetReceiverAutoConnect makes key the single auto-connect target; toggling
// one receiver clears the others.
func (e *Engine) SetReceiverAutoConnect(key string, on bool) {
	e.send(command{kind: cmdUpdateReceiver, deviceID: key,
		patch: receivers.Patch{AutoConnect: &on}})
}

// SetReceiverHidden toggles the row between the flyout (shown) and the
// settings page's hidden group.
func (e *Engine) SetReceiverHidden(key string, hidden bool) {
	e.send(command{kind: cmdUpdateReceiver, deviceID: key,
		patch: receivers.Patch{Hidden: &hidden}})
}

// SetReceiverDelay sets the receiver's DSP/DAC delay in milliseconds
// (0..2000). A change to the active receiver restarts its session.
func (e *Engine) SetReceiverDelay(key string, ms int) {
	e.send(command{kind: cmdUpdateReceiver, deviceID: key,
		patch: receivers.Patch{DelayMS: &ms}})
}
