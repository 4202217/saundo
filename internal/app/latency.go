package app

import (
	"time"

	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
	"saundo/internal/settings"
)

// latencyHoldTimeout bounds how long the flyout keeps its last streaming
// snapshot while a latency change reconnects. A slow reconnect then falls
// back to the real connecting state instead of hiding it indefinitely.
const latencyHoldTimeout = 3 * time.Second

// Rapid mode clicks coalesce: the selection changes at once, but the engine
// restart waits for the last click. The custom slider keeps its longer pause.
const (
	presetLatencyDelay = 300 * time.Millisecond
	customLatencyDelay = 500 * time.Millisecond
)

// apply takes an engine snapshot, holding it while a latency change is
// applying so the device row, footer, capture line, and tray keep their
// streaming face. The latency row and ALAC line still update at once.
func (a *App) apply(snap engine.Snapshot) {
	if a.latencyApplying && a.holdLatencyApply(snap) {
		return
	}
	a.applySnapshot(snap)
}

func (a *App) holdLatencyApply(snap engine.Snapshot) bool {
	if snap.Error != "" || snap.PinRequired || snap.PasswordRequired {
		a.finishLatencyApply()
		return false
	}
	if snap.State != sender.StateStreaming {
		a.latencySawChange = true
	}
	if snap.State == sender.StateStreaming && a.latencySawChange {
		a.finishLatencyApply()
		return false
	}
	a.latencyPendingSnap = snap
	a.latencyHasPending = true
	return true
}

func (a *App) beginLatencyApply() {
	if a.eng == nil || phaseOf(a.snap) != phaseStreaming {
		return
	}
	a.latencyApplying = true
	a.latencySawChange = false
	a.latencyHasPending = false
	if a.latencyHoldTimer != nil {
		a.latencyHoldTimer.Stop()
	}
	a.latencyHoldTimer = time.AfterFunc(latencyHoldTimeout, func() {
		a.updateUI(func() {
			if !a.latencyApplying {
				return
			}
			pending, ok := a.latencyPendingSnap, a.latencyHasPending
			a.finishLatencyApply()
			if ok {
				a.applySnapshot(pending)
			}
		})
	})
}

func (a *App) finishLatencyApply() {
	if a.latencyHoldTimer != nil {
		a.latencyHoldTimer.Stop()
		a.latencyHoldTimer = nil
	}
	a.latencyApplying = false
	a.latencySawChange = false
	a.latencyHasPending = false
}

func (a *App) disconnect() {
	a.cancelLatencyApply()
	a.finishLatencyApply()
	if a.eng != nil {
		a.eng.Disconnect()
	}
}

// latencyMS is the value the mode resolves to, with the engine's last value as
// the fallback while settings are being migrated.
func (a *App) latencyMS() int {
	switch a.latencyMode {
	case settings.LatencyLow:
		return settings.LatencyLowMS
	case settings.LatencyStandard:
		return settings.LatencyStandardMS
	case settings.LatencyStable:
		return settings.LatencyStableMS
	case settings.LatencyCustom:
		if a.latencyCustom >= 20 && a.latencyCustom <= 2000 {
			return int(a.latencyCustom + 0.5)
		}
	}
	if a.latency >= 20 {
		return int(a.latency + 0.5)
	}
	return settings.LatencyLowMS
}

// latencyDisplay is the value shown in the playback head's ALAC line.
func (a *App) latencyDisplay() string { return settings.LatencyText(a.latencyMS()) }

func (a *App) latencyModeIndex() int {
	switch a.latencyMode {
	case settings.LatencyStandard:
		return 1
	case settings.LatencyStable:
		return 2
	case settings.LatencyCustom:
		return 3
	}
	return 0
}

func latencyModeAt(i int) settings.LatencyMode {
	switch i {
	case 1:
		return settings.LatencyStandard
	case 2:
		return settings.LatencyStable
	case 3:
		return settings.LatencyCustom
	}
	return settings.LatencyLow
}

// setLatencyMode selects a named preset or the stored custom value. A live
// session restarts so the receiver's stream SETUP carries the new latency.
func (a *App) setLatencyMode(mode settings.LatencyMode) {
	if mode == a.latencyMode {
		return
	}
	if mode == settings.LatencyCustom && a.latencyCustom < 20 {
		a.latencyCustom = a.latency
	}
	ms := settings.LatencyModeValue(mode, int(a.latencyCustom+0.5))
	a.latencyMode = mode
	a.cfg.LatencyMode = mode
	a.cfg.LatencyCustomMS = int(a.latencyCustom + 0.5)
	a.latency = float64(ms)
	a.cfg.LatencyMS = ms
	a.saveSettings()
	if a.eng != nil {
		// Sync the setting and the engine's effective value now; only the
		// session restart waits for the debounce.
		a.eng.SetLatencySettings(mode, int(a.latencyCustom+0.5), a.showLatencySwitch)
		a.eng.SetLatency(ms)
		a.scheduleLatencyApply(presetLatencyDelay)
	}
}

// setLatencyCustom edits the stored custom value. While a session is live the
// change is applied after a short pause, so dragging does not restart the
// stream on every frame.
func (a *App) setLatencyCustom(v float64) {
	ms := int(v + 0.5)
	if ms < 20 {
		ms = 20
	}
	if ms > 2000 {
		ms = 2000
	}
	a.latencyCustom = float64(ms)
	a.cfg.LatencyCustomMS = ms
	if a.latencyMode == settings.LatencyCustom {
		a.latency = float64(ms)
		a.cfg.LatencyMS = ms
	}
	a.saveSettings()
	if a.eng == nil {
		return
	}
	a.eng.SetLatencySettings(a.latencyMode, ms, a.showLatencySwitch)
	if a.latencyMode == settings.LatencyCustom {
		a.eng.SetLatency(ms)
		a.scheduleLatencyApply(customLatencyDelay)
	}
}

// scheduleLatencyApply restarts the stream once, after the user stops
// changing the latency. An in-flight hold is extended across the pause so an
// older session cannot end the quiet state early.
func (a *App) scheduleLatencyApply(delay time.Duration) {
	if a.latencyApplying {
		a.latencySawChange = false
		a.latencyHasPending = false
	}
	a.latencySeq++
	seq := a.latencySeq
	if a.latencyTimer != nil {
		a.latencyTimer.Stop()
	}
	a.latencyTimer = time.AfterFunc(delay, func() {
		a.updateUI(func() {
			if seq != a.latencySeq || a.eng == nil {
				return
			}
			a.beginLatencyApply()
			a.eng.ApplyLatency(a.latencyMS())
		})
	})
}

func (a *App) cancelLatencyApply() {
	a.latencySeq++
	if a.latencyTimer != nil {
		a.latencyTimer.Stop()
		a.latencyTimer = nil
	}
}

func (a *App) setShowLatencySwitch(on bool) {
	a.showLatencySwitch = on
	a.cfg.ShowLatencySwitch = on
	a.saveSettings()
	if a.eng != nil {
		a.eng.SetLatencySettings(a.latencyMode, int(a.latencyCustom+0.5), on)
	}
	if a.win != nil {
		a.fitFlyout()
	}
}
