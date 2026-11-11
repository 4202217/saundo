package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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

// localMute is the mute the engine currently owns. endpointID is meaningful
// only in MuteLocalActive.
type localMute struct {
	state      MuteLocalState
	endpointID string
	err        string
}

// muteGuard is written before the endpoint is muted and removed after it is
// released. A crash between those points leaves it for the next launch, which
// releases the endpoint. The PID is diagnostic; the app runs one instance.
type muteGuard struct {
	EndpointID string `json:"endpointId"`
	PID        int    `json:"pid"`
}

func (e *Engine) idleMuteState() MuteLocalState {
	if e.setsValue.MuteLocalOnConnect {
		return MuteLocalWaiting
	}
	return MuteLocalOff
}

// muteRequest is one desired endpoint state for the worker. on is the setting
// combined with the session state; endpoint is the configured capture
// endpoint, or "" to follow the system default.
type muteRequest struct {
	gen      int
	on       bool
	endpoint string
}

// muteStatus reports the worker's result. applied reports whether the
// endpoint is muted; endpoint is the active one when applied, or the restored
// one on a restore.
type muteStatus struct {
	gen      int
	applied  bool
	endpoint string
	err      error
}

// muteWorkerState is owned by the worker goroutine.
type muteWorkerState struct {
	active   bool
	endpoint string
}

func (e *Engine) startMuteWorker() {
	e.muteOnce.Do(func() {
		e.muteRunning.Store(true)
		go e.muteLoop()
	})
}

func (e *Engine) stopMuteWorker() {
	if !e.muteRunning.Load() {
		return
	}
	e.muteStopOnce.Do(func() { close(e.muteStop) })
	select {
	case <-e.muteDone:
	case <-time.After(2 * time.Second):
		e.logf("mute worker did not stop within 2s")
	}
}

// muteLoop owns every blocking endpoint and guard-file call. Recovery runs
// before any desired state, so a session that starts immediately still gets
// the previous run's endpoint released first.
func (e *Engine) muteLoop() {
	defer close(e.muteDone)
	e.recoverMuteGuard()
	var st muteWorkerState
	for {
		select {
		case req := <-e.muteReq:
			e.runMute(req, &st)
		case <-e.muteStop:
			select {
			case req := <-e.muteReq:
				e.runMute(req, &st)
			default:
			}
			return
		}
	}
}

func (e *Engine) runMute(req muteRequest, st *muteWorkerState) {
	if !req.on {
		if !st.active {
			e.postMuteStatus(muteStatus{gen: req.gen})
			return
		}
		endpoint := st.endpoint
		if err := e.setEndpointMuted(endpoint, false); err != nil {
			e.postMuteStatus(muteStatus{gen: req.gen, applied: true, endpoint: endpoint, err: err})
			return
		}
		e.clearMuteGuard()
		st.active, st.endpoint = false, ""
		e.postMuteStatus(muteStatus{gen: req.gen, endpoint: endpoint})
		return
	}
	endpoint := req.endpoint
	if endpoint == "" {
		ep, err := e.defaultEndpoint()
		if err != nil {
			e.postMuteStatus(muteStatus{gen: req.gen, err: err})
			return
		}
		endpoint = ep.ID
	}
	if st.active && st.endpoint == endpoint {
		e.postMuteStatus(muteStatus{gen: req.gen, applied: true, endpoint: endpoint})
		return
	}
	if st.active {
		if err := e.setEndpointMuted(st.endpoint, false); err != nil {
			e.postMuteStatus(muteStatus{gen: req.gen, applied: true, endpoint: st.endpoint, err: err})
			return
		}
		e.clearMuteGuard()
		st.active, st.endpoint = false, ""
	}
	if err := e.writeMuteGuard(endpoint); err != nil {
		e.postMuteStatus(muteStatus{gen: req.gen, err: fmt.Errorf("write mute guard: %w", err)})
		return
	}
	if err := e.setEndpointMuted(endpoint, true); err != nil {
		e.clearMuteGuard()
		e.postMuteStatus(muteStatus{gen: req.gen, err: err})
		return
	}
	st.active, st.endpoint = true, endpoint
	e.postMuteStatus(muteStatus{gen: req.gen, applied: true, endpoint: endpoint})
}

func (e *Engine) postMuteStatus(st muteStatus) {
	select {
	case e.muteStatus <- st:
	default:
		select {
		case <-e.muteStatus:
		default:
		}
		select {
		case e.muteStatus <- st:
		default:
		}
	}
}

// requestMute posts the latest desired state. The loop never blocks: a busy
// worker keeps only the newest request.
func (e *Engine) requestMute(req muteRequest) {
	e.muteGen++
	req.gen = e.muteGen
	select {
	case e.muteReq <- req:
	default:
		select {
		case <-e.muteReq:
		default:
		}
		select {
		case e.muteReq <- req:
		default:
		}
	}
}

// applyMuteStatus adopts a worker result on the engine goroutine. A result
// older than the latest request is stale and ignored.
func (e *Engine) applyMuteStatus(st muteStatus) {
	if st.gen != e.muteGen {
		return
	}
	var next localMute
	switch {
	case st.err != nil && st.applied:
		next = localMute{state: MuteLocalActive, endpointID: st.endpoint, err: st.err.Error()}
	case st.err != nil:
		next = localMute{state: MuteLocalFailed, err: st.err.Error()}
	case st.applied:
		next = localMute{state: MuteLocalActive, endpointID: st.endpoint}
	default:
		next = localMute{state: e.idleMuteState()}
	}
	if next == e.mute {
		return
	}
	prev := e.mute
	e.mute = next
	switch {
	case next.err != "" && next.err != prev.err:
		if next.state == MuteLocalActive {
			e.logf("local output restore failed for %s: %s", next.endpointID, next.err)
		} else {
			e.logf("local output mute failed: %s", next.err)
		}
	case next.state == MuteLocalActive:
		e.logf("local output muted (%s)", next.endpointID)
	case prev.state == MuteLocalActive:
		e.logf("local output restored (%s)", prev.endpointID)
	}
	e.publish()
}

// applyLocalMute asks the worker to mute the local render endpoint when the
// setting is on and a session is streaming. The mute is temporary:
// restoreLocalMute always ends it, even when the endpoint was muted before the
// session began. It is idempotent: an already active mute is a no-op in the
// worker.
func (e *Engine) applyLocalMute() {
	if !e.setsValue.MuteLocalOnConnect {
		e.setMuteState(MuteLocalOff, "")
		return
	}
	if e.tr == nil || e.tr.State() != sender.StateStreaming {
		e.setMuteState(MuteLocalWaiting, "")
		return
	}
	endpoint := e.captureEndpoint
	if e.captureProcess != 0 {
		// Process loopback has no endpoint of its own; the local output is
		// whatever Windows is playing to by default.
		endpoint = ""
	}
	e.requestMute(muteRequest{on: true, endpoint: endpoint})
}

// restoreLocalMute asks the worker to unmute the endpoint and end the
// session's mute. The request is unconditional: an apply may still be in
// flight while the loop state reads Waiting, and the worker no-ops when no
// mute is active. The connection mute never outlives the session. A failed
// restore keeps the state active and the guard in place so a later close or
// the next launch can retry.
func (e *Engine) restoreLocalMute() {
	e.requestMute(muteRequest{on: false})
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
	}
	e.publish()
}

// recoverMuteGuard releases an endpoint left muted by a previous run. It runs
// as the mute worker's first job, before any session can own the mute.
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
	if err := e.setEndpointMuted(guard.EndpointID, false); err != nil {
		if e.endpointExists(guard.EndpointID) {
			e.logf("mute guard release failed for %s: %v", guard.EndpointID, err)
			return
		}
		e.clearMuteGuard()
		e.logf("mute guard endpoint %s is gone; dropped the guard", guard.EndpointID)
		return
	}
	e.clearMuteGuard()
	e.logf("released local output from the previous run (%s)", guard.EndpointID)
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

func (e *Engine) writeMuteGuard(endpointID string) error {
	path := e.muteGuardPath()
	if path == "" {
		return errors.New("no settings directory")
	}
	data, err := json.Marshal(muteGuard{EndpointID: endpointID, PID: os.Getpid()})
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
