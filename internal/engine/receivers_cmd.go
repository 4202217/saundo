package engine

import (
	"saundo/internal/receivers"
)

// handleAddReceiver creates a manual receiver. Validation happens here so
// both the UI preview and the engine share one rule set; the password
// moves to the DPAPI store and never reaches receivers.json.
func (e *Engine) handleAddReceiver(spec receivers.Spec) {
	if err := spec.Validate(); err != nil {
		e.lastError = err.Error()
		e.publish()
		return
	}
	spec = spec.Resolved()
	key := receivers.NewManualKey()
	e.reg.Put(spec.Entry(key))
	e.lastError = ""
	if spec.Password != "" {
		if err := e.savePassword(key, spec.Password); err != nil {
			e.lastError = "无法保存接收器密码：" + err.Error()
		}
	}
	if err := e.recvStore.Save(e.reg); err != nil {
		e.logf("receivers save: %v", err)
		e.lastError = "无法保存接收器配置：" + err.Error()
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
		e.lastError = "接收器不存在，请刷新后重试"
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
				e.lastError = err.Error()
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

	delayBefore := entry.DelayMS
	if patch.AutoConnect != nil {
		// The flag chooses the next launch's target; it never starts a
		// connection mid-session, and toggling it ends a pending launch wait.
		e.autoTried = true
		e.reg.SingleAutoConnect(key, *patch.AutoConnect)
	}
	e.reg.Put(patch.Apply(entry))
	e.lastError = ""
	if patch.Password != nil {
		if err := e.savePassword(key, *patch.Password); err != nil {
			e.lastError = "无法保存接收器密码：" + err.Error()
		}
	}
	if err := e.recvStore.Save(e.reg); err != nil {
		e.logf("receivers save: %v", err)
		e.lastError = "无法保存接收器配置：" + err.Error()
	}
	e.rebuildTargets()
	e.publish()

	delayAfter := entry.DelayMS
	if applied, ok := e.reg.Get(key); ok {
		delayAfter = applied.DelayMS
	}
	if patch.DelayMS != nil && delayBefore != delayAfter && e.isLive(key) {
		if target, ok := e.target(key); ok {
			e.userDisconnect = false
			e.connect(target.Device)
		}
	}
}

// handleRemoveReceiver deletes a manual receiver: its row, password, HAP
// credentials, and remembered volume. Discovered rows are managed by hiding,
// so removal there would only be undone by the next advertisement.
func (e *Engine) handleRemoveReceiver(key string) {
	entry, ok := e.reg.Get(key)
	if !ok || !entry.IsManual() {
		return
	}
	e.reg.Delete(key)
	if err := e.creds.DeletePassword(key); err != nil {
		e.logf("password delete: %v", err)
		e.lastError = "无法删除接收器密码：" + err.Error()
	}
	if err := e.creds.Delete(key); err != nil {
		e.logf("credentials delete: %v", err)
	}
	delete(e.passwords, key)
	if e.setsValue.VolumeByDevice != nil {
		delete(e.setsValue.VolumeByDevice, key)
		e.saveSettings()
	}
	if e.setsValue.LastDeviceID == key {
		e.setsValue.LastDeviceID = ""
		e.saveSettings()
	}
	if e.selectedID == key {
		e.selectedID = ""
		e.userDisconnect = true
		e.reconnectPendingOff()
		if e.tr != nil {
			e.tr.Stop()
		}
		e.closeSession()
		e.connectedName = ""
	}
	if err := e.recvStore.Save(e.reg); err != nil {
		e.logf("receivers save: %v", err)
		e.lastError = "无法保存接收器配置：" + err.Error()
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
// now: a session, or a reconnect waiting to bring it back.
func (e *Engine) isLive(key string) bool {
	if e.tr != nil && e.selectedID == key {
		return true
	}
	return e.reconnectWanted && e.selectedID == key
}
