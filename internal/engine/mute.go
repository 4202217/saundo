package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"saundo/internal/airplay/sender"
)

// MuteLocalState is the effective state of the connection mute. The setting
// itself lives in Settings.MuteLocalOnConnect; this state says what the engine
// has actually done with the render endpoint.
type MuteLocalState uint8

const (
	MuteLocalOff MuteLocalState = iota
	MuteLocalWaiting
	MuteLocalActive
	MuteLocalFailed
)

// localMute is the mute the engine currently owns. endpointID and previous are
// meaningful only in MuteLocalActive.
type localMute struct {
	state      MuteLocalState
	endpointID string
	previous   bool
	err        string
}

// muteGuard is written before the endpoint is muted and removed after it is
// restored. A crash between those points leaves it for the next launch. The
// PID is diagnostic; the app runs one instance.
type muteGuard struct {
	EndpointID   string `json:"endpointId"`
	PreviousMute bool   `json:"previousMute"`
	PID          int    `json:"pid"`
}

func (e *Engine) idleMuteState() MuteLocalState {
	if e.setsValue.MuteLocalOnConnect {
		return MuteLocalWaiting
	}
	return MuteLocalOff
}

// applyLocalMute mutes the local render endpoint when the setting is on and a
// session is streaming. It is idempotent: an already active mute returns.
func (e *Engine) applyLocalMute() {
	if !e.setsValue.MuteLocalOnConnect {
		e.setMuteState(MuteLocalOff, "")
		return
	}
	if e.tr == nil || e.tr.State() != sender.StateStreaming {
		e.setMuteState(MuteLocalWaiting, "")
		return
	}
	if e.mute.state == MuteLocalActive {
		return
	}
	endpointID := e.captureEndpoint
	if e.captureProcess != 0 {
		// Process loopback has no endpoint of its own; the local output is
		// whatever Windows is playing to by default.
		endpointID = ""
	}
	if endpointID == "" {
		ep, err := e.defaultEndpoint()
		if err != nil {
			e.failLocalMute(err)
			return
		}
		endpointID = ep.ID
	}
	previous, err := e.endpointMuted(endpointID)
	if err != nil {
		e.failLocalMute(err)
		return
	}
	if err := e.writeMuteGuard(endpointID, previous); err != nil {
		e.failLocalMute(fmt.Errorf("write mute guard: %w", err))
		return
	}
	if err := e.setEndpointMuted(endpointID, true); err != nil {
		e.clearMuteGuard()
		e.failLocalMute(err)
		return
	}
	e.mute = localMute{state: MuteLocalActive, endpointID: endpointID, previous: previous}
	e.logf("local output muted (%s)", endpointID)
	e.publish()
}

// restoreLocalMute puts the endpoint back to the state it had before the
// session. A failed restore keeps the state active and the guard in place so a
// later close or the next launch can retry.
func (e *Engine) restoreLocalMute() {
	if e.mute.state != MuteLocalActive {
		return
	}
	if err := e.setEndpointMuted(e.mute.endpointID, e.mute.previous); err != nil {
		e.mute.err = err.Error()
		e.logf("local output restore failed for %s: %v", e.mute.endpointID, err)
		e.publish()
		return
	}
	e.logf("local output restored (%s)", e.mute.endpointID)
	e.clearMuteGuard()
	e.mute = localMute{state: e.idleMuteState()}
	e.publish()
}

// refreshLocalMute moves the mute when the capture endpoint changes.
func (e *Engine) refreshLocalMute() {
	e.restoreLocalMute()
	e.applyLocalMute()
}

func (e *Engine) setMuteState(state MuteLocalState, errText string) {
	if e.mute.state == state && e.mute.err == errText {
		return
	}
	e.mute.state = state
	e.mute.err = errText
	if state != MuteLocalActive {
		e.mute.endpointID = ""
		e.mute.previous = false
	}
	e.publish()
}

func (e *Engine) failLocalMute(err error) {
	e.logf("local output mute failed: %v", err)
	e.setMuteState(MuteLocalFailed, err.Error())
}

// recoverMuteGuard restores an endpoint left muted by a previous run. It runs
// once at construction, before any session can own the mute.
func (e *Engine) recoverMuteGuard() {
	path := e.muteGuardPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		e.logf("mute guard: %v", err)
		return
	}
	var guard muteGuard
	if err := json.Unmarshal(data, &guard); err != nil {
		// A failed parse means unknown, not "no mute": keep the file.
		e.logf("mute guard is unreadable; leaving it in place: %v", err)
		return
	}
	if guard.EndpointID == "" {
		e.logf("mute guard has no endpoint; leaving it in place")
		return
	}
	if err := e.setEndpointMuted(guard.EndpointID, guard.PreviousMute); err != nil {
		if e.endpointExists(guard.EndpointID) {
			e.logf("mute guard restore failed for %s: %v", guard.EndpointID, err)
			return
		}
		e.clearMuteGuard()
		e.logf("mute guard endpoint %s is gone; dropped the guard", guard.EndpointID)
		return
	}
	e.clearMuteGuard()
	e.logf("restored local output from the previous run (%s)", guard.EndpointID)
}

func (e *Engine) endpointExists(id string) bool {
	if e.endpoints == nil {
		return true // unknown: keep the guard for the next launch
	}
	endpoints, err := e.endpoints()
	if err != nil {
		return true // unknown: keep the guard for the next launch
	}
	for _, ep := range endpoints {
		if ep.ID == id {
			return true
		}
	}
	return false
}

func (e *Engine) muteGuardPath() string {
	if e.sets == nil {
		return ""
	}
	return filepath.Join(e.sets.Dir(), "mute-guard.json")
}

func (e *Engine) writeMuteGuard(endpointID string, previous bool) error {
	path := e.muteGuardPath()
	if path == "" {
		return errors.New("no settings directory")
	}
	data, err := json.Marshal(muteGuard{EndpointID: endpointID, PreviousMute: previous, PID: os.Getpid()})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (e *Engine) clearMuteGuard() {
	path := e.muteGuardPath()
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.logf("mute guard: could not remove %s: %v", path, err)
	}
}
