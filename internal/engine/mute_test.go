package engine

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"

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
	setHook      func(id string, muted bool) error
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
	if f.setHook != nil {
		if err := f.setHook(id, muted); err != nil {
			return err
		}
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

type fakeProcessMuter struct {
	calls []bool
	err   error
}

func (f *fakeProcessMuter) set(muted bool) error {
	f.calls = append(f.calls, muted)
	return f.err
}

func useProcessMuter(h *harness, m *fakeProcessMuter) {
	h.e.processMute = true
	h.e.setProcessMuted = m.set
}

// settleMute drains worker results until the loop-visible state matches want.
// The worker is real; only the endpoint calls are faked.
func (h *harness) settleMute(t *testing.T, want MuteLocalState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case st := <-h.e.muteStatus:
			h.e.applyMuteStatus(st)
		default:
			if got := h.e.Snapshot().MuteLocal; got == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("mute state = %v, want %v", h.e.Snapshot().MuteLocal, want)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func TestMuteLocalFollowsTheSession(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true

	h.addDevice()
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)

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
	h.settleMute(t, MuteLocalWaiting)

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
	h.settleMute(t, MuteLocalActive)

	h.e.handleCommand(command{kind: cmdMuteLocal, show: false})
	h.settleMute(t, MuteLocalOff)

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
	h.settleMute(t, MuteLocalActive)

	h.e.handleCommand(command{kind: cmdMuteLocal, show: true})
	h.settleMute(t, MuteLocalActive)

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
	h.settleMute(t, MuteLocalFailed)

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
	h.settleMute(t, MuteLocalActive)

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
	// A platform without process mute, as Windows: the endpoint fallback.
	h.e.processMute = false
	h.e.setsValue.MuteLocalOnConnect = true
	h.e.captureEndpoint = "selected-ep"
	h.e.captureProcess = 1234
	h.addDevice()
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)

	if !m.muted["default-ep"] {
		t.Fatal("process capture did not mute the default endpoint")
	}
	if m.muted["selected-ep"] {
		t.Fatal("process capture muted the selected capture endpoint")
	}
}

// TestMuteLocalProcessCaptureMutesTheSelectedApp pins the macOS behavior: a
// per-app session mutes only the captured app and never the output device.
func TestMuteLocalProcessCaptureMutesTheSelectedApp(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	pm := &fakeProcessMuter{}
	useProcessMuter(h, pm)
	h.e.setsValue.MuteLocalOnConnect = true
	h.e.captureProcess = 1234
	h.addDevice()
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)

	if len(pm.calls) == 0 || !pm.calls[len(pm.calls)-1] {
		t.Fatalf("process mute calls = %v, want the last to be mute", pm.calls)
	}
	if len(m.setCalls) != 0 {
		t.Fatalf("endpoint mute calls = %+v, want none", m.setCalls)
	}

	h.e.handleCommand(command{kind: cmdDisconnect})
	h.settleMute(t, MuteLocalWaiting)
	last := pm.calls[len(pm.calls)-1]
	if last {
		t.Fatalf("process mute calls = %v, want restore", pm.calls)
	}
	if len(m.setCalls) != 0 {
		t.Fatalf("endpoint mute calls = %+v, want none", m.setCalls)
	}
}

// TestMuteLocalAppSelectionMutesTheApps covers the multi-app selection with
// the same tap mute as the legacy single process.
func TestMuteLocalAppSelectionMutesTheApps(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	pm := &fakeProcessMuter{}
	useProcessMuter(h, pm)
	h.e.setsValue.MuteLocalOnConnect = true
	h.e.captureApps = []string{"com.apple.Music", "com.google.Chrome"}
	h.addDevice()
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)

	if len(pm.calls) == 0 || !pm.calls[len(pm.calls)-1] {
		t.Fatalf("process mute calls = %v, want the last to be mute", pm.calls)
	}
	if len(m.setCalls) != 0 {
		t.Fatalf("endpoint mute calls = %+v, want none", m.setCalls)
	}
}

func TestMuteLocalProcessFailureIsVisible(t *testing.T) {
	h := newHarness(t)
	pm := &fakeProcessMuter{err: errors.New("no tap")}
	useProcessMuter(h, pm)
	h.e.setsValue.MuteLocalOnConnect = true
	h.e.captureProcess = 1234
	h.addDevice()
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalFailed)

	if snap := h.e.Snapshot(); snap.MuteLocalError == "" {
		t.Fatal("failed process mute did not carry its error")
	}
}

func TestMuteLocalProcessOffRestores(t *testing.T) {
	h := newHarness(t)
	pm := &fakeProcessMuter{}
	useProcessMuter(h, pm)
	h.e.setsValue.MuteLocalOnConnect = true
	h.e.captureProcess = 1234
	h.addDevice()
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)

	h.e.handleCommand(command{kind: cmdMuteLocal, show: false})
	h.settleMute(t, MuteLocalOff)
	if last := pm.calls[len(pm.calls)-1]; last {
		t.Fatalf("process mute calls = %v, want restore", pm.calls)
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
	h.settleMute(t, MuteLocalActive)

	h.e.handleCommand(command{kind: cmdDisconnect})
	h.settleMute(t, MuteLocalWaiting)

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

// TestMuteLocalCommandUpdatesStateOnly pins the ownership split: the app owns
// the persisted preference, so the engine's save leaves the stored value
// alone even after the command updates its runtime state.
// TestMuteLocalDoesNotBlockTheEventLoop pins the offload: while the endpoint
// call is stuck, the loop must still process commands and publish.
func TestMuteLocalDoesNotBlockTheEventLoop(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	entered := make(chan struct{})
	release := make(chan struct{})
	m.setHook = func(id string, muted bool) error {
		if muted {
			close(entered)
			<-release
		}
		return nil
	}
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = false
	h.addDevice()
	h.connect()
	h.launch()

	go h.e.run()
	defer func() {
		close(h.e.quit)
		<-h.e.done
	}()

	h.e.send(command{kind: cmdMuteLocal, show: true})
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the mute worker never reached setEndpointMuted")
	}

	h.e.send(command{kind: cmdTrayAnimation, value: 0})
	deadline := time.Now().Add(500 * time.Millisecond)
	for h.e.Snapshot().TrayAnimation {
		if time.Now().After(deadline) {
			t.Fatal("the event loop stayed blocked inside the mute call")
		}
		time.Sleep(time.Millisecond)
	}

	close(release)
	deadline = time.Now().Add(2 * time.Second)
	for h.e.Snapshot().MuteLocal != MuteLocalActive {
		if time.Now().After(deadline) {
			t.Fatalf("mute state = %v, want active after release", h.e.Snapshot().MuteLocal)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestMuteLocalCommandUpdatesStateOnly(t *testing.T) {
	h := newHarness(t)
	cfg := h.e.sets.Load()
	cfg.MuteLocalOnConnect = false
	if err := h.e.sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	h.e.handleCommand(command{kind: cmdMuteLocal, show: true})
	if !h.e.setsValue.MuteLocalOnConnect {
		t.Fatal("the command did not update the engine state")
	}
	h.flushSettings()
	if cfg := h.e.sets.Load(); cfg.MuteLocalOnConnect {
		t.Fatal("the engine's save wrote the app-owned mute preference")
	}
}
