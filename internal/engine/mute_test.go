package engine

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"saundo/internal/capture"
)

type muteCall struct {
	endpointID string
	muted      bool
}

type fakeMuter struct {
	muted        map[string]bool
	setCalls     []muteCall
	setErr       error
	defaultID    string
	defaultErr   error
	endpoints    []capture.Endpoint
	endpointsErr error
}

func newFakeMuter() *fakeMuter {
	return &fakeMuter{
		muted:     map[string]bool{},
		defaultID: "default-ep",
		endpoints: []capture.Endpoint{{ID: "default-ep", Name: "Speakers"}},
	}
}

func (f *fakeMuter) setEndpointMuted(id string, muted bool) error {
	f.setCalls = append(f.setCalls, muteCall{endpointID: id, muted: muted})
	if f.setErr != nil {
		return f.setErr
	}
	f.muted[id] = muted
	return nil
}

func (f *fakeMuter) defaultEndpoint() (capture.Endpoint, error) {
	if f.defaultErr != nil {
		return capture.Endpoint{}, f.defaultErr
	}
	return capture.Endpoint{ID: f.defaultID, Name: "Speakers"}, nil
}

func (f *fakeMuter) listEndpoints() ([]capture.Endpoint, error) {
	return f.endpoints, f.endpointsErr
}

func useMuter(h *harness, m *fakeMuter) {
	h.e.setEndpointMuted = m.setEndpointMuted
	h.e.defaultEndpoint = m.defaultEndpoint
	h.e.endpoints = m.listEndpoints
}

func TestMuteLocalFollowsTheSession(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true

	h.addDevice()
	h.connect()
	h.launch()

	if got := h.e.Snapshot().MuteLocal; got != MuteLocalActive {
		t.Fatalf("state after launch = %v, want active", got)
	}
	if !m.muted["default-ep"] {
		t.Fatal("the default endpoint was not muted")
	}
	if _, err := os.Stat(h.e.muteGuardPath()); err != nil {
		t.Fatalf("mute guard missing while muted: %v", err)
	}

	h.e.handleCommand(command{kind: cmdDisconnect})

	if m.muted["default-ep"] {
		t.Fatal("disconnect left the endpoint muted")
	}
	if got := h.e.Snapshot().MuteLocal; got != MuteLocalWaiting {
		t.Fatalf("state after disconnect = %v, want waiting", got)
	}
	if _, err := os.Stat(h.e.muteGuardPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mute guard survived a clean restore: %v", err)
	}
	if len(m.setCalls) != 2 || !m.setCalls[0].muted || m.setCalls[1].muted {
		t.Fatalf("set calls = %+v, want mute then restore", m.setCalls)
	}
}

func TestMuteLocalOffRestoresAtOnce(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	h.addDevice()
	h.connect()
	h.launch()

	h.e.handleCommand(command{kind: cmdMuteLocal, show: false})

	if m.muted["default-ep"] {
		t.Fatal("turning the setting off left the endpoint muted")
	}
	if got := h.e.Snapshot().MuteLocal; got != MuteLocalOff {
		t.Fatalf("state = %v, want off", got)
	}
}

func TestMuteLocalOnWhileStreaming(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.addDevice()
	h.connect()
	h.launch()

	h.e.handleCommand(command{kind: cmdMuteLocal, show: true})

	if !m.muted["default-ep"] {
		t.Fatal("turning the setting on while streaming did not mute")
	}
	if got := h.e.Snapshot().MuteLocal; got != MuteLocalActive {
		t.Fatalf("state = %v, want active", got)
	}
}

func TestMuteLocalFailureIsVisible(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	m.setErr = errors.New("no audio endpoint")
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	h.addDevice()
	h.connect()
	h.launch()

	snap := h.e.Snapshot()
	if snap.MuteLocal != MuteLocalFailed {
		t.Fatalf("state = %v, want failed", snap.MuteLocal)
	}
	if snap.MuteLocalError == "" {
		t.Fatal("failed mute did not carry its error")
	}
	if _, err := os.Stat(h.e.muteGuardPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a failed mute left a guard behind: %v", err)
	}
}

func TestMuteLocalTargetsSelectedEndpoint(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	m.endpoints = append(m.endpoints, capture.Endpoint{ID: "selected-ep", Name: "Headphones"})
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	h.e.captureEndpoint = "selected-ep"
	h.addDevice()
	h.connect()
	h.launch()

	if !m.muted["selected-ep"] {
		t.Fatal("the selected capture endpoint was not muted")
	}
	if m.muted["default-ep"] {
		t.Fatal("the default endpoint was muted instead of the selected one")
	}
	if len(m.setCalls) == 0 || m.setCalls[0].endpointID != "selected-ep" {
		t.Fatalf("set calls = %+v, want selected-ep first", m.setCalls)
	}
}

func TestMuteLocalProcessCaptureUsesDefaultEndpoint(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	h.e.captureEndpoint = "selected-ep"
	h.e.captureProcess = 1234
	h.addDevice()
	h.connect()
	h.launch()

	if !m.muted["default-ep"] {
		t.Fatal("process capture did not mute the default endpoint")
	}
	if m.muted["selected-ep"] {
		t.Fatal("process capture muted the selected capture endpoint")
	}
}

func TestMuteLocalReleasesAPreMutedEndpoint(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	m.muted["default-ep"] = true
	h.e.setsValue.MuteLocalOnConnect = true
	h.addDevice()
	h.connect()
	h.launch()

	h.e.handleCommand(command{kind: cmdDisconnect})

	if m.muted["default-ep"] {
		t.Fatal("disconnect left the pre-session mute in place")
	}
	if got := h.e.Snapshot().MuteLocal; got != MuteLocalWaiting {
		t.Fatalf("state after disconnect = %v, want waiting", got)
	}
	last := m.setCalls[len(m.setCalls)-1]
	if last.muted || last.endpointID != "default-ep" {
		t.Fatalf("last set call = %+v, want unmute of default-ep", last)
	}
}

func TestMuteGuardRecoversAfterCrash(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	m.muted["default-ep"] = true
	// The previousMute field is what the first guard format wrote; recovery
	// releases the endpoint regardless of it.
	guard := []byte(`{"endpointId":"default-ep","previousMute":true,"pid":1234}`)
	if err := os.WriteFile(h.e.muteGuardPath(), guard, 0o600); err != nil {
		t.Fatal(err)
	}

	h.e.recoverMuteGuard()

	if m.muted["default-ep"] {
		t.Fatal("recovery left the endpoint muted")
	}
	if _, err := os.Stat(h.e.muteGuardPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("recovery kept the guard: %v", err)
	}
}

func TestMuteGuardCorruptIsLeftAlone(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	if err := os.WriteFile(h.e.muteGuardPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	h.e.recoverMuteGuard()

	if len(m.setCalls) != 0 {
		t.Fatalf("a corrupt guard touched the endpoint: %+v", m.setCalls)
	}
	if _, err := os.Stat(h.e.muteGuardPath()); err != nil {
		t.Fatalf("a corrupt guard was removed: %v", err)
	}
}

func TestMuteLocalSettingPersists(t *testing.T) {
	h := newHarness(t)
	h.e.handleCommand(command{kind: cmdMuteLocal, show: true})
	h.flushSettings()
	if cfg := h.e.sets.Load(); !cfg.MuteLocalOnConnect {
		t.Fatal("MuteLocalOnConnect did not persist")
	}
}
