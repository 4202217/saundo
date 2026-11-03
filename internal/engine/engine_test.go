package engine

import (
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/ring"
	"saundo/internal/capture"
	"saundo/internal/diagnostics"
	"saundo/internal/settings"
)

const testDeviceID = "AA:BB:CC:DD:EE:01"

type fakeTransport struct {
	evs        chan raopio.Event
	ev         sender.Events
	started    bool
	stopped    bool
	state      sender.State
	identity   sender.Identity
	authDev    string
	authAP2    bool
	strict     bool
	ptpSet     bool
	inputRates []int
	latencies  []int
	volumes    []float64
	authPW     string
	nowPlaying []string
	pins       []string
	pauses     []bool
	pauseOK    bool
	initialPct float64
	hasInitial bool
	rings      []*ring.Buffer
}

func newFakeTransport(ev sender.Events) *fakeTransport {
	return &fakeTransport{evs: make(chan raopio.Event, 16), ev: ev, pauseOK: true}
}

func (f *fakeTransport) Events() <-chan raopio.Event { return f.evs }
func (f *fakeTransport) Start(host string, port uint16, name string) {
	f.started = true
}
func (f *fakeTransport) Stop() {
	f.stopped = true
	f.state = sender.StateIdle
}
func (f *fakeTransport) Active() bool             { return f.state != sender.StateIdle }
func (f *fakeTransport) State() sender.State      { return f.state }
func (f *fakeTransport) HandleEvent(raopio.Event) {}
func (f *fakeTransport) Tick()                    {}
func (f *fakeTransport) NextDeadline() time.Time  { return time.Time{} }
func (f *fakeTransport) SetPaused(paused bool) bool {
	f.pauses = append(f.pauses, paused)
	return f.pauseOK
}
func (f *fakeTransport) SetIdentity(id sender.Identity) { f.identity = id }
func (f *fakeTransport) SetAuth(a device.Auth, ap2 bool, id, creds, pw string) {
	f.authDev = id
	f.authAP2 = ap2
	f.authPW = pw
}
func (f *fakeTransport) SetMemberAuth(m device.Device, creds, pw string) {
	f.authDev = m.ID
	f.authPW = pw
}
func (f *fakeTransport) Rings() []*ring.Buffer         { return f.rings }
func (f *fakeTransport) SetPTPClock(c sender.PTPClock) { f.ptpSet = c != nil }
func (f *fakeTransport) AttachRing(*ring.Buffer)       {}
func (f *fakeTransport) SetInputFormat(rate int)       { f.inputRates = append(f.inputRates, rate) }
func (f *fakeTransport) SetLatency(ms int)             { f.latencies = append(f.latencies, ms) }
func (f *fakeTransport) SetVolume(p float64)           { f.volumes = append(f.volumes, p) }
func (f *fakeTransport) SetStrictReceiverAuth(v bool)  { f.strict = v }
func (f *fakeTransport) SubmitPin(code string)         { f.pins = append(f.pins, code) }
func (f *fakeTransport) InitialVolumePercent() (float64, bool) {
	return f.initialPct, f.hasInitial
}
func (f *fakeTransport) SetNowPlaying(title, artist, album string) {
	f.nowPlaying = append(f.nowPlaying, title+"\x00"+artist+"\x00"+album)
}
func (f *fakeTransport) PTP() (bool, bool, float64) { return false, false, 0 }

// launch simulates the sender reporting a launch result.
func (f *fakeTransport) launch(ok bool, err string) {
	if ok {
		f.state = sender.StateStreaming
	} else {
		f.state = sender.StateIdle
	}
	f.ev.Launched(ok, err)
}

type fakeCaptureSource struct{ stopped bool }

func (f *fakeCaptureSource) Stop() { f.stopped = true }
func (f *fakeCaptureSource) Stats() capture.Stats {
	return capture.Stats{Running: true, Rate: 48000, Device: "TestOut"}
}

type harness struct {
	e        *Engine
	fake     *fakeTransport
	now      time.Time
	sessions int
	rings    []*ring.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := sets.Load()
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	h := &harness{e: New(sets, creds.NewStoreAt(filepath.Join(dir, "creds")), &diagnostics.Log{}, nil), now: time.Now()}
	h.e.now = func() time.Time { return h.now }
	h.e.newSession = func(d device.Device) (transport, func(), error) {
		h.sessions++
		h.fake = newFakeTransport(sender.Events{
			Launched:            h.e.onLaunched,
			Closed:              h.e.onClosed,
			PinRequired:         h.e.onPinRequired,
			CredentialsObtained: h.e.onCredentials,
			StateChanged:        h.e.onStateChanged,
		})
		h.fake.rings = h.rings
		return h.fake, func() { h.fake.stopped = true }, nil
	}
	h.e.captureStartFn = func(capture.Config) (captureSource, error) {
		return &fakeCaptureSource{}, nil
	}
	return h
}

func (h *harness) addDevice() {
	h.e.setDevices([]device.Device{{
		ID: testDeviceID, Name: "客厅 HomePod", Host: "192.0.2.10", Port: 7000,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true,
	}})
}

func (h *harness) connect() {
	h.e.handleCommand(command{kind: cmdConnect, deviceID: testDeviceID})
}

func (h *harness) launch() {
	h.fake.launch(true, "")
}

func (h *harness) advance(d time.Duration) {
	h.now = h.now.Add(d)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUsesPTPSelectsHomePodAndMac(t *testing.T) {
	cases := []struct {
		name string
		dev  device.Device
		want bool
	}{
		{"homepod", device.Device{Model: "AudioAccessory5,1", Features: ptpFeatureBit, Auth: device.AuthHAPTransient}, true},
		{"mac", device.Device{Model: "Mac14,2", Features: ptpFeatureBit, Auth: device.AuthHAPPassword}, true},
		{"mac without ptp bit", device.Device{Model: "Mac14,2", Auth: device.AuthHAPPassword}, false},
		{"non-ap2 mac", device.Device{Model: "Mac14,2", Features: ptpFeatureBit, Auth: device.AuthNone}, false},
		{"fairplay tv", device.Device{Model: "AppleTV3,2", Auth: device.AuthLegacyPin}, false},
	}
	for _, c := range cases {
		if got := usesPTP(c.dev); got != c.want {
			t.Errorf("%s: usesPTP = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestConnectAppliesRememberedVolume(t *testing.T) {
	h := newHarness(t)
	h.e.setsValue.VolumeByDevice = map[string]float64{testDeviceID: 42}
	h.addDevice()
	h.connect()
	if !h.fake.started {
		t.Fatal("session did not start")
	}
	if h.fake.authDev != testDeviceID || !h.fake.authAP2 {
		t.Fatalf("auth not applied: dev=%q ap2=%v", h.fake.authDev, h.fake.authAP2)
	}
	if len(h.fake.volumes) == 0 || h.fake.volumes[len(h.fake.volumes)-1] != 42 {
		t.Fatalf("remembered volume not applied: %v", h.fake.volumes)
	}
}

func TestFirstLaunchAdoptsReceiverVolume(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.fake.initialPct, h.fake.hasInitial = 37, true
	if len(h.fake.volumes) != 0 {
		t.Fatalf("volume set before launch: %v", h.fake.volumes)
	}
	h.launch()
	if h.e.volume != 37 || h.e.setsValue.Volume != 37 {
		t.Fatalf("volume = %f, want 37", h.e.volume)
	}
}

func TestVolumeCommandStoresPerDevice(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdVolume, value: 55})
	if got := h.e.setsValue.VolumeByDevice[testDeviceID]; got != 55 {
		t.Fatalf("per-device volume = %f, want 55", got)
	}
	if h.fake.volumes[len(h.fake.volumes)-1] != 55 {
		t.Fatalf("volume not forwarded: %v", h.fake.volumes)
	}
}

func TestReconnectBackoffThenSuccess(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()

	h.e.onClosed()
	if !h.e.reconnectWanted || h.e.reconnectAt.IsZero() {
		t.Fatal("no reconnect scheduled after close")
	}
	if got := h.e.reconnectAt.Sub(h.now); got != time.Second {
		t.Fatalf("first backoff = %s, want 1s", got)
	}
	if h.e.reconnectNum != 1 {
		t.Fatalf("attempt = %d, want 1", h.e.reconnectNum)
	}

	h.advance(time.Second)
	h.e.attemptReconnect()
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want 2", h.sessions)
	}
	h.launch()
	if h.e.reconnectNum != 0 || h.e.reconnectAt != (time.Time{}) {
		t.Fatal("successful launch did not reset the reconnect state")
	}

	h.e.onClosed()
	if got := h.e.reconnectAt.Sub(h.now); got != time.Second {
		t.Fatalf("backoff after reset = %s, want 1s", got)
	}
}

func TestReconnectBackoffEscalatesAndGivesUp(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()

	// First failure: 1 s. Each failed launch schedules the next attempt.
	h.e.onClosed()
	if got := h.e.reconnectAt.Sub(h.now); got != time.Second {
		t.Fatalf("first backoff = %s, want 1s", got)
	}
	h.advance(time.Second)
	h.e.attemptReconnect()
	h.fake.launch(false, "Device refused")
	for _, d := range []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second} {
		if got := h.e.reconnectAt.Sub(h.now); got != d {
			t.Fatalf("backoff = %s, want %s", got, d)
		}
		h.advance(d)
		h.e.attemptReconnect()
		h.fake.launch(false, "Device refused")
	}
	// The sixth schedule exceeds maxReconnectAttempts.
	if h.e.reconnectWanted {
		t.Fatal("reconnect did not give up after five attempts")
	}
	if !strings.Contains(h.e.lastError, "自动重连") {
		t.Fatalf("error = %q", h.e.lastError)
	}
	if h.sessions != 6 {
		t.Fatalf("sessions = %d, want 6", h.sessions)
	}
}

func TestUserDisconnectDoesNotReconnect(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdDisconnect})
	h.e.onClosed()
	if h.e.reconnectWanted || !h.e.reconnectAt.IsZero() {
		t.Fatal("user disconnect scheduled a reconnect")
	}
	if !h.fake.stopped {
		t.Fatal("session was not stopped")
	}
}

func TestReconnectToggleOffCancelsPending(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()
	h.e.onClosed()
	h.e.handleCommand(command{kind: cmdReconnect, value: 0})
	if h.e.reconnectWanted || !h.e.reconnectAt.IsZero() {
		t.Fatal("toggling reconnect off did not cancel the pending retry")
	}
}

func TestLatencyAndPasswordReachSession(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.e.handleCommand(command{kind: cmdLatency, value: 500})
	h.e.handleCommand(command{kind: cmdPassword, deviceID: testDeviceID, name: "secret"})
	if h.e.setsValue.LatencyMS != 500 || h.e.setsValue.Passwords[testDeviceID] != "secret" {
		t.Fatalf("settings not stored: %+v", h.e.setsValue)
	}
	h.connect()
	if h.fake.authPW != "secret" {
		t.Fatalf("password = %q", h.fake.authPW)
	}
	if len(h.fake.latencies) == 0 || h.fake.latencies[len(h.fake.latencies)-1] != 500 {
		t.Fatalf("latency at connect = %v", h.fake.latencies)
	}
}

func TestFairPlayReceiverFailsFast(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{{
		ID: testDeviceID, Name: "REDMI 显示器", Model: "AppleTV3,2", Auth: device.AuthLegacyPin,
	}})
	h.connect()
	if h.sessions != 0 {
		t.Fatal("a FairPlay receiver started a session")
	}
	if snap := h.e.Snapshot(); !strings.Contains(snap.Error, "FairPlay") {
		t.Fatalf("error = %q, want a FairPlay message", snap.Error)
	}
}

func TestMissingPasswordPromptsAndUsesIt(t *testing.T) {
	for _, auth := range []device.Auth{device.AuthHAPPassword, device.AuthPassword} {
		t.Run(auth.String(), func(t *testing.T) {
			h := newHarness(t)
			h.e.setDevices([]device.Device{{
				ID: testDeviceID, Name: "MacBook Air", Host: "192.168.1.20", Port: 7000,
				Model: "Mac14,2", Auth: auth, AirPlay2: true,
			}})
			h.connect()
			if h.sessions != 0 {
				t.Fatal("session started without a password")
			}
			snap := h.e.Snapshot()
			if !snap.PasswordRequired || snap.PasswordDevice != "MacBook Air" || snap.PasswordDeviceID != testDeviceID {
				t.Fatalf("password prompt = %+v", snap)
			}
			h.e.handleCommand(command{kind: cmdDismissPassword})
			if h.e.Snapshot().PasswordRequired {
				t.Fatal("dismiss left the password prompt open")
			}
			h.e.handleCommand(command{kind: cmdPassword, deviceID: testDeviceID, name: "secret"})
			h.connect()
			if h.sessions != 1 || h.fake.authPW != "secret" {
				t.Fatalf("sessions=%d password=%q, want one session with the entered password", h.sessions, h.fake.authPW)
			}
			if h.e.Snapshot().PasswordRequired {
				t.Fatal("password prompt stayed open after connecting")
			}
		})
	}
}

func TestCaptureRateChangeMidSession(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()
	h.e.ring.Push([]int16{1, 2, 3, 4})
	h.e.applyCaptureInfo(captureInfo{rate: 44100})
	if len(h.fake.inputRates) == 0 || h.fake.inputRates[len(h.fake.inputRates)-1] != 44100 {
		t.Fatalf("input format not updated: %v", h.fake.inputRates)
	}
	if h.e.ring.Available() != 0 {
		t.Fatalf("ring not reset: %d samples", h.e.ring.Available())
	}
	if h.e.tr.State() != sender.StateStreaming {
		t.Fatalf("state regressed to %s", h.e.tr.State())
	}
}

func TestAsyncCaptureStartDoesNotBlock(t *testing.T) {
	h := newHarness(t)
	gate := make(chan *fakeCaptureSource)
	var calls atomic.Int32
	h.e.captureStartFn = func(capture.Config) (captureSource, error) {
		calls.Add(1)
		src := <-gate
		if src == nil {
			return nil, errors.New("closed")
		}
		return src, nil
	}

	start := time.Now()
	h.e.restartCapture()
	waitFor(t, func() bool { return calls.Load() == 1 })
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("restartCapture blocked for %s", elapsed)
	}
	if !h.e.captureStarting {
		t.Fatal("restartCapture ran synchronously")
	}
	h.e.restartCapture()
	if calls.Load() != 1 {
		t.Fatalf("queued restart started a second capture early (calls=%d)", calls.Load())
	}

	src1 := &fakeCaptureSource{}
	gate <- src1
	var res captureStart
	select {
	case res = <-h.e.captureResult:
	case <-time.After(2 * time.Second):
		t.Fatal("no capture result")
	}
	h.e.applyCaptureStart(res)
	if !src1.stopped {
		t.Fatal("superseded capture was not stopped")
	}
	waitFor(t, func() bool { return calls.Load() == 2 })

	src2 := &fakeCaptureSource{}
	gate <- src2
	select {
	case res = <-h.e.captureResult:
	case <-time.After(2 * time.Second):
		t.Fatal("no second capture result")
	}
	h.e.applyCaptureStart(res)
	if src2.stopped {
		t.Fatal("the adopted capture was stopped")
	}
	if !h.e.captureRunning || h.e.captureRate != 48000 {
		t.Fatalf("capture not running: %+v", h.e.Snapshot())
	}
}

func TestConnectSelectsTheDeviceBeingConnected(t *testing.T) {
	h := newHarness(t)
	const staleID = "AA:BB:CC:DD:EE:99"
	// The previous session's device is stale: the user connected a different
	// device from the list.
	h.e.selectedID = staleID
	h.e.setsValue.LastDeviceID = staleID
	h.e.setDevices([]device.Device{
		{ID: staleID, Name: "REDMI", Model: "AppleTV3,2", Auth: device.AuthLegacyPin},
		{ID: testDeviceID, Name: "客厅 HomePod", Host: "192.0.2.10", Port: 7000,
			Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true},
	})
	h.connect()
	if h.fake == nil || !h.fake.started {
		t.Fatal("session did not start")
	}
	h.launch()

	snap := h.e.Snapshot()
	if snap.SelectedID != testDeviceID {
		t.Fatalf("selected device = %q, want %q", snap.SelectedID, testDeviceID)
	}
	if snap.ConnectedName != "客厅 HomePod" {
		t.Fatalf("connected name = %q, want 客厅 HomePod", snap.ConnectedName)
	}
	if got := h.e.sets.Load().LastDeviceID; got != testDeviceID {
		t.Fatalf("persisted last device = %q, want %q", got, testDeviceID)
	}
}

func TestFailedLaunchDoesNotReplaceLastDevice(t *testing.T) {
	h := newHarness(t)
	const lastID = "AA:BB:CC:DD:EE:99"
	h.e.setsValue.LastDeviceID = lastID
	h.addDevice()
	h.connect()
	h.fake.launch(false, "Device refused")

	if got := h.e.setsValue.LastDeviceID; got != lastID {
		t.Fatalf("last device after a failed launch = %q, want %q", got, lastID)
	}
	if got := h.e.sets.Load().LastDeviceID; got != "" {
		t.Fatalf("failed launch persisted last device %q", got)
	}
}

func TestCaptureErrorClearsWhenCaptureRecovers(t *testing.T) {
	h := newHarness(t)
	h.e.applyCaptureInfo(captureInfo{err: errors.New("capture: audio discontinuity on the capture stream")})
	if got := h.e.Snapshot().CaptureError; got == "" {
		t.Fatal("capture error was not reported")
	}
	// The stream reopened after the transient failure; the UI must not keep
	// showing the old error.
	h.e.applyCaptureInfo(captureInfo{name: "扬声器", rate: 48000})
	if got := h.e.Snapshot().CaptureError; got != "" {
		t.Fatalf("stale capture error after recovery: %q", got)
	}
}

func TestOfflineStereoGroupDoesNotStart(t *testing.T) {
	h := newHarness(t)
	group := device.Device{
		ID: device.StereoGroupID("tsid"), Name: "HomePods", StereoID: "tsid",
		Members: []device.Device{
			{ID: "AA:BB:CC:DD:EE:01", Name: "HomePod L", Host: "192.0.2.10", Online: true},
			{ID: "AA:BB:CC:DD:EE:02", Name: "HomePod R", Host: "192.0.2.11", Online: false},
		},
		Online: false,
	}
	h.e.setDevices([]device.Device{group})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: group.ID})
	if h.sessions != 0 {
		t.Fatalf("offline stereo group started %d sessions", h.sessions)
	}
	if !strings.Contains(h.e.lastError, "两台") {
		t.Fatalf("error = %q, want an incomplete-group message", h.e.lastError)
	}
}

func TestGroupMemberRingsTakeOverCapture(t *testing.T) {
	h := newHarness(t)
	rings := []*ring.Buffer{ring.New(16), ring.New(16)}
	h.rings = rings
	h.addDevice()
	h.connect()
	if h.fake == nil || !h.fake.started {
		t.Fatal("session did not start")
	}
	if len(h.e.memberRings) != 2 {
		t.Fatalf("member rings = %d, want 2", len(h.e.memberRings))
	}
	h.e.captureSink.Push([]int16{1, 2, 3, 4})
	for i, r := range rings {
		if r.Available() != 4 {
			t.Fatalf("member ring %d available = %d, want 4", i, r.Available())
		}
	}
	if h.e.ring.Available() != 0 {
		t.Fatalf("single ring received group audio: %d", h.e.ring.Available())
	}

	h.e.closeSession()
	if len(h.e.memberRings) != 0 {
		t.Fatal("member rings not detached")
	}
	h.e.captureSink.Push([]int16{5, 6})
	if h.e.ring.Available() != 2 {
		t.Fatalf("single ring after detach = %d, want 2", h.e.ring.Available())
	}
}

func TestTrayAnimationCommandPersists(t *testing.T) {
	h := newHarness(t)
	h.e.handleCommand(command{kind: cmdTrayAnimation, value: 0})
	if h.e.buildSnapshot().TrayAnimation {
		t.Fatal("snapshot kept tray animation on")
	}
	if h.e.sets.Load().TrayAnimation {
		t.Fatal("tray animation off was not saved")
	}
	h.e.setsValue.Volume = 42
	h.e.saveSettings()
	if h.e.sets.Load().TrayAnimation {
		t.Fatal("a later settings save reverted tray animation")
	}
	h.e.handleCommand(command{kind: cmdTrayAnimation, value: 1})
	if !h.e.buildSnapshot().TrayAnimation {
		t.Fatal("snapshot did not turn tray animation on")
	}
	if !h.e.sets.Load().TrayAnimation {
		t.Fatal("tray animation on was not saved")
	}
}

func TestNowPlayingCommandForwardsWhenEnabled(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()

	h.e.handleCommand(command{kind: cmdNowPlaying, text: "歌", name: "手", album: "片"})
	if len(h.fake.nowPlaying) != 0 {
		t.Fatal("metadata forwarded while the switch is off")
	}

	h.e.handleCommand(command{kind: cmdShowNowPlaying, value: 1})
	h.e.handleCommand(command{kind: cmdNowPlaying, text: "歌", name: "手", album: "片"})
	if len(h.fake.nowPlaying) != 1 || h.fake.nowPlaying[0] != "歌\x00手\x00片" {
		t.Fatalf("now playing = %q", h.fake.nowPlaying)
	}
}

func TestApplyLatencyRestartsActiveSession(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.e.handleCommand(command{kind: cmdLatency, value: 200})
	h.connect()
	h.launch()
	before := h.sessions
	h.e.handleCommand(command{kind: cmdLatency, value: 1500, reconnect: true})
	if h.sessions != before+1 {
		t.Fatalf("sessions = %d, want %d after applying latency", h.sessions, before+1)
	}
	if len(h.fake.latencies) == 0 || h.fake.latencies[len(h.fake.latencies)-1] != 1500 {
		t.Fatalf("new session latency = %v, want 1500", h.fake.latencies)
	}
}

func TestLatencySettingsReachStore(t *testing.T) {
	h := newHarness(t)
	h.e.handleCommand(command{kind: cmdLatencySettings, text: "standard", value: 1500, show: true})
	got := h.e.sets.Load()
	if got.LatencyMode != settings.LatencyStandard || got.LatencyCustomMS != 1500 || !got.ShowLatencySwitch {
		t.Fatalf("latency settings = %+v", got)
	}
}
