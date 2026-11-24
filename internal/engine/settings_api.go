package engine

import (
	"saundo/internal/capture"
)

// The connection and capture switches: UI commands that only post to the
// engine loop. They own no state here; the loop applies them.

// SetCaptureEndpoint selects an output endpoint; empty follows the default.
func (e *Engine) SetCaptureEndpoint(id, name string) {
	e.send(command{kind: cmdCaptureEndpoint, deviceID: id, name: name})
}

// SetStrictAuth toggles receiver proof enforcement.
func (e *Engine) SetStrictAuth(strict bool) {
	v := 0.0
	if strict {
		v = 1
	}
	e.send(command{kind: cmdStrictAuth, value: v})
}

// SetName changes the sender name receivers display.
func (e *Engine) SetName(name string) {
	e.send(command{kind: cmdName, text: name})
}

// dropUnsupportedAppSelection clears a saved app or process selection on a
// platform that does not offer per-app capture, so the stale setting cannot
// strand system capture. It reports whether it changed anything.
func (e *Engine) dropUnsupportedAppSelection() bool {
	if capture.AppCaptureSupported() {
		return false
	}
	if e.captureProcess == 0 && len(e.captureApps) == 0 {
		return false
	}
	e.captureProcess = 0
	e.captureApps = nil
	e.captureAppNames = nil
	e.captureName = e.setsValue.CaptureEndpointName
	e.setsValue.CaptureProcessPID = 0
	e.setsValue.CaptureProcessName = ""
	e.setsValue.CaptureAppIDs = nil
	e.setsValue.CaptureAppNames = nil
	e.markSettingsDirty()
	e.logf("capture: dropped the app selection; per-app capture is not offered on this platform")
	return true
}

// SetCaptureProcess selects application capture; pid 0 returns to endpoint
// capture.
func (e *Engine) SetCaptureProcess(pid uint32, name string) {
	e.send(command{kind: cmdCaptureProcess, value: float64(pid), name: name})
}

// SetCaptureApps selects apps by stable ID (bundle IDs on macOS; empty
// returns to endpoint capture). names is parallel to ids, and label is the
// status label the app built for the selection.
func (e *Engine) SetCaptureApps(ids, names []string, label string) {
	e.send(command{kind: cmdCaptureApps, ids: ids, names: names, label: label})
}
