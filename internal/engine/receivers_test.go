package engine

import (
	"path/filepath"
	"testing"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/ring"
	"saundo/internal/capture"
	"saundo/internal/diagnostics"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// newHarnessWithStores wires the fake transport over explicit stores.
func newHarnessWithStores(t *testing.T, sets *settings.Store, credsStore *creds.Store, reg *receivers.Store) *harness {
	t.Helper()
	h := &harness{e: NewWithReceivers(sets, credsStore, reg, &diagnostics.Log{}, nil), now: time.Now()}
	h.e.now = func() time.Time { return h.now }
	// The mute worker owns the endpoint calls in production too; tests drive
	// it in place of the event loop's select.
	h.e.startMuteWorker()
	t.Cleanup(h.e.stopMuteWorker)
	// Unit tests must never touch the machine's real render endpoint. The
	// mute default is on, so a connect-and-stream test would otherwise call
	// WASAPI; mute_test.go overrides this with its recording fake.
	h.e.setEndpointMuted = func(string, bool) error { return nil }
	h.e.defaultEndpoint = func() (capture.Endpoint, error) {
		return capture.Endpoint{ID: "default-ep", Name: "Speakers"}, nil
	}
	h.e.endpoints = func() ([]capture.Endpoint, error) {
		return []capture.Endpoint{{ID: "default-ep", Name: "Speakers"}}, nil
	}
	h.e.newSession = func(d device.Device) (transport, func(), error) {
		h.sessions++
		h.fake = newFakeTransport(sender.Events{
			Launched:               h.e.onLaunched,
			Closed:                 h.e.onClosed,
			PinRequired:            h.e.onPinRequired,
			CredentialsObtained:    h.e.onCredentials,
			StateChanged:           h.e.onStateChanged,
			ReceiverLatencyChanged: h.e.onReceiverLatency,
		})
		h.fake.rings = h.rings
		return h.fake, func() { h.fake.stopped = true }, nil
	}
	h.e.captureStartFn = func(capture.Config) (captureSource, error) {
		return &fakeCaptureSource{}, nil
	}
	return h
}

// newHarnessWithRegistry is newHarness with a real receivers store, so the
// tests observe what persists.
func newHarnessWithRegistry(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := sets.Load()
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	reg := receivers.NewStoreAt(filepath.Join(dir, "receivers.json"))
	return newHarnessWithStores(t, sets, creds.NewStoreAt(filepath.Join(dir, "creds")), reg)
}

// manualKey returns the single manual row's key, for tests that added one.
func manualKey(h *harness) string {
	for _, e := range h.e.reg.Entries {
		if e.IsManual() {
			return e.Key
		}
	}
	return ""
}

// onlineDevice is the harness's discovered receiver, marked online: the
// engine only auto-connects or lists a receiver discovery currently sees.
func onlineDevice() device.Device {
	return device.Device{
		ID: testDeviceID, Name: "客厅 HomePod", Host: "192.0.2.10", Port: 7000,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true,
	}
}

// addDeviceOnline feeds the engine one online device.
func (h *harness) addDeviceOnline() {
	h.e.setDevices([]device.Device{onlineDevice()})
}

func TestSetDevicesRemembersFirstSightings(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.addDeviceOnline()
	stored, err := h.e.recvStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := stored.Get(testDeviceID)
	if !ok || e.Kind != receivers.KindDiscovered || e.Name != "客厅 HomePod" {
		t.Fatalf("first sighting not persisted: %+v ok=%v", e, ok)
	}
	if len(h.e.Snapshot().Receivers) != 1 {
		t.Fatalf("snapshot receivers = %+v", h.e.Snapshot().Receivers)
	}
	_ = ring.New(1) // keep the ring import anchored to the harness fields
}

// TestRememberedDeviceLeavesTheListsWhenOffline pins the display rule: a
// remembered receiver without a live advertisement is hidden from both the
// flyout and the settings page, but stays in the registry for when it
// returns.
func TestRememberedDeviceLeavesTheListsWhenOffline(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.addDevice()
	h.e.setDevices(nil) // the receiver left the network
	snap := h.e.Snapshot()
	if len(snap.Devices) != 0 {
		t.Fatalf("offline remembered row still in the flyout: %+v", snap.Devices)
	}
	if len(snap.Receivers) != 0 {
		t.Fatalf("offline remembered row still in the settings list: %+v", snap.Receivers)
	}
	if len(h.e.reg.Entries) != 1 {
		t.Fatalf("registry dropped the remembered row: %+v", h.e.reg.Entries)
	}
	// Coming back restores the live advertisement.
	h.addDeviceOnline()
	snap = h.e.Snapshot()
	if len(snap.Devices) != 1 || !snap.Devices[0].Online || snap.Devices[0].Host != "192.0.2.10" {
		t.Fatalf("receiver did not come back: %+v", snap.Devices)
	}
	if len(snap.Receivers) != 1 || !snap.Receivers[0].Online {
		t.Fatalf("settings list did not restore the receiver: %+v", snap.Receivers)
	}
}

// TestOpenSessionRefusesAddresslessReceiver pins the dial guard: an offline
// remembered group has no host or port, and opening its session must fail
// closed instead of dialing :0.
func TestOpenSessionRefusesAddresslessReceiver(t *testing.T) {
	h := newHarnessWithRegistry(t)
	if _, _, err := h.e.openSession(device.Device{ID: "stereo:gone", Name: "HomePods"}); err == nil {
		t.Fatal("opened a session for an addressless receiver")
	}
}

func TestAutoConnectConnectsRememberedDeviceOnceOnline(t *testing.T) {
	h := newHarnessWithRegistry(t)
	// The registry flag alone drives the launch connection; the legacy
	// settings.AutoConnect flag is unread after migration.
	h.e.reg.Put(receivers.Entry{Key: testDeviceID, Kind: receivers.KindDiscovered, AutoConnect: true})
	h.e.setDevices(nil)
	if h.sessions != 0 {
		t.Fatal("auto-connect ran with nothing addressable")
	}
	h.addDeviceOnline()
	if h.sessions != 1 {
		t.Fatalf("sessions = %d, want 1 once online", h.sessions)
	}
	// A later discovery cycle does not connect again.
	h.addDeviceOnline()
	if h.sessions != 1 {
		t.Fatalf("auto-connect ran twice: %d", h.sessions)
	}
}

func TestAutoConnectManualTargetIsImmediate(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "书房", Type: receivers.TypeAirPlay2, Host: "192.0.2.33", Port: 7000,
	}})
	key := manualKey(h)
	if key == "" {
		t.Fatal("no manual row")
	}
	h.e.reg.SingleAutoConnect(key, true)
	// The run loop's first act; no discovery cycle has reported yet.
	h.e.maybeAutoConnect()
	if h.sessions != 1 {
		t.Fatalf("manual target did not connect without mDNS: sessions=%d", h.sessions)
	}
	if h.fake.authDev != key {
		t.Fatalf("connected to %q, want the manual row", h.fake.authDev)
	}
}

func TestAutoConnectFailureFallsIntoReconnect(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.reg.Put(receivers.Entry{Key: testDeviceID, Kind: receivers.KindDiscovered, AutoConnect: true})
	h.addDeviceOnline()
	if h.sessions != 1 {
		t.Fatalf("auto-connect did not start: sessions=%d", h.sessions)
	}
	h.fake.launch(false, "receiver refused")
	if !h.e.reconnectWanted || h.e.reconnectAt.IsZero() {
		t.Fatalf("failure did not schedule a retry: wanted=%v at=%v", h.e.reconnectWanted, h.e.reconnectAt)
	}
}

func TestAutoConnectStopsAfterUserDisconnect(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.reg.Put(receivers.Entry{Key: testDeviceID, Kind: receivers.KindDiscovered, AutoConnect: true})
	h.e.setDevices(nil)
	h.e.handleCommand(command{kind: cmdDisconnect})
	h.addDeviceOnline()
	if h.sessions != 0 {
		t.Fatalf("auto-connect overrode a user disconnect: sessions=%d", h.sessions)
	}
}

func TestTogglingAutoConnectWaitsForNextLaunch(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.reg.Put(receivers.Entry{Key: testDeviceID, Kind: receivers.KindDiscovered, AutoConnect: true})
	h.e.setDevices(nil) // the launch target is offline and still pending
	second := device.Device{ID: "AA:BB:CC:DD:EE:02", Name: "第二台", Host: "192.0.2.2", Port: 7000,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true}
	h.e.setDevices([]device.Device{second})
	if h.sessions != 0 {
		t.Fatal("an unflagged receiver was connected")
	}
	on := true
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: second.ID,
		patch: receivers.Patch{AutoConnect: &on}})
	h.e.setDevices([]device.Device{second})
	if h.sessions != 0 {
		t.Fatalf("toggling auto-connect started a session: sessions=%d", h.sessions)
	}
}

func TestMigrationSeedsAutoConnectForTheEngine(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := settings.Default()
	cfg.AutoConnect = true
	cfg.LastDeviceID = testDeviceID
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	credsStore := creds.NewStoreAt(filepath.Join(dir, "creds"))
	reg := receivers.NewStoreAt(filepath.Join(dir, "receivers.json"))
	if _, err := receivers.MigrateSettings(sets, credsStore, reg); err != nil {
		t.Fatal(err)
	}
	h := newHarnessWithStores(t, sets, credsStore, reg)
	h.e.setDevices([]device.Device{onlineDevice()})
	if h.sessions != 1 {
		t.Fatalf("migrated auto-connect did not fire: sessions=%d", h.sessions)
	}
}

func TestManualReceiverAppearsWithoutMDNS(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "书房 AirPort Express", Type: receivers.TypeAirPortExpress, Host: "192.0.2.32",
	}})
	snap := h.e.Snapshot()
	if len(snap.Devices) != 1 || !snap.Devices[0].Online {
		t.Fatalf("manual receiver not attemptable: %+v", snap.Devices)
	}
	if got := snap.Devices[0].Auth; got != device.AuthSetup {
		t.Fatalf("auth = %s, want auth-setup", got)
	}
	if snap.Receivers[0].Address != "192.0.2.32:5000" {
		t.Fatalf("address = %q, want default 5000", snap.Receivers[0].Address)
	}
}

func TestManualReceiverPasswordGatesAndServes(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "Mac", Type: receivers.TypeAirPlay2, Host: "192.0.2.40", Port: 7000,
	}})
	key := manualKey(h)
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	if h.fake.authPW != "" {
		t.Fatalf("no-password manual got %q", h.fake.authPW)
	}
	// Storing one flips the row to HAP password pairing and the session to
	// the password.
	h.e.handleCommand(command{kind: cmdPassword, deviceID: key, name: "hunter2"})
	if !h.e.Snapshot().Receivers[0].PasswordSet {
		t.Fatal("the row did not report a saved password")
	}
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	if h.fake.authPW != "hunter2" {
		t.Fatalf("password = %q, want hunter2", h.fake.authPW)
	}
}

func TestManualReceiverAddValidates(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Type: receivers.TypeAirPlay2, Host: "http://example.com/",
	}})
	if len(h.e.reg.Entries) != 0 {
		t.Fatalf("invalid spec was stored: %+v", h.e.reg.Entries)
	}
	if h.e.Snapshot().Error == "" {
		t.Fatal("no error surfaced for the invalid spec")
	}
}

func TestRemoveReceiverDeletesManualRowOnly(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "书房", Type: receivers.TypeAirPlay1, Host: "192.0.2.7", Password: "pw",
	}})
	key := manualKey(h)
	h.addDeviceOnline() // a discovered receiver
	h.e.handleCommand(command{kind: cmdRemoveReceiver, deviceID: testDeviceID})
	if _, ok := h.e.reg.Get(testDeviceID); !ok {
		t.Fatal("a discovered row was removed")
	}
	h.e.handleCommand(command{kind: cmdRemoveReceiver, deviceID: key})
	if _, ok := h.e.reg.Get(key); ok {
		t.Fatal("the manual row survived removal")
	}
	if h.e.creds.HasPassword(key) {
		t.Fatal("the manual row's password survived removal")
	}
	if got := h.e.Snapshot().Receivers; len(got) != 1 {
		t.Fatalf("snapshot rows = %+v", got)
	}
}

func TestSetReceiverHiddenFiltersFlyoutKeepsActive(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.addDeviceOnline()
	h.connect()
	h.launch()
	// The active session's receiver can be hidden and stays reachable.
	hidden := true
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: testDeviceID,
		patch: receivers.Patch{Hidden: &hidden}})
	if got := h.e.Snapshot().Devices; len(got) != 1 {
		t.Fatalf("active hidden row dropped from the flyout: %+v", got)
	}
	if !h.e.Snapshot().Receivers[0].Hidden {
		t.Fatal("settings row did not report hidden")
	}
	// Disconnecting applies the filter: the hidden row leaves the flyout.
	h.e.handleCommand(command{kind: cmdDisconnect})
	if got := h.e.Snapshot().Devices; len(got) != 0 {
		t.Fatalf("hidden row stayed visible after disconnect: %+v", got)
	}
	if got := h.e.Snapshot().Receivers; len(got) != 1 {
		t.Fatalf("hidden row left the settings list: %+v", got)
	}
}

func TestTogglingAutoConnectClearsOthers(t *testing.T) {
	h := newHarnessWithRegistry(t)
	first := device.Device{ID: testDeviceID, Name: "客厅 HomePod", Host: "192.0.2.10", Port: 7000,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true}
	second := device.Device{ID: "AA:BB:CC:DD:EE:02", Name: "第二台", Host: "192.0.2.2", Port: 7000,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true}
	h.e.setDevices([]device.Device{first, second})
	on := true
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: testDeviceID,
		patch: receivers.Patch{AutoConnect: &on}})
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: second.ID,
		patch: receivers.Patch{AutoConnect: &on}})
	if e, _ := h.e.reg.Get(testDeviceID); e.AutoConnect {
		t.Fatal("the previous auto-connect flag survived")
	}
	if e, _ := h.e.reg.Get(second.ID); !e.AutoConnect {
		t.Fatal("the new flag did not land")
	}
}

func TestDelayJoinsLatencyAtConnectAndRestarts(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.addDevice()
	h.e.handleCommand(command{kind: cmdLatency, value: 200})
	delay := 120
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: testDeviceID,
		patch: receivers.Patch{DelayMS: &delay}})
	h.connect()
	if got := h.fake.latencies[len(h.fake.latencies)-1]; got != 320 {
		t.Fatalf("latency at connect = %d, want 320", got)
	}
	if h.e.Snapshot().LatencyMS != 200 {
		t.Fatalf("snapshot base latency = %d, want 200", h.e.Snapshot().LatencyMS)
	}
	// Changing the global base while streaming keeps the receiver's delay.
	h.e.handleCommand(command{kind: cmdLatency, value: 300})
	if got := h.fake.latencies[len(h.fake.latencies)-1]; got != 420 {
		t.Fatalf("latency after base change = %d, want 420", got)
	}
	// Changing the active receiver's delay restarts the session with the
	// new effective value.
	before := h.sessions
	delay = 2000
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: testDeviceID,
		patch: receivers.Patch{DelayMS: &delay}})
	if h.sessions != before+1 {
		t.Fatalf("sessions = %d, want a restart", h.sessions)
	}
	if got := h.fake.latencies[len(h.fake.latencies)-1]; got != 2300 {
		t.Fatalf("restarted latency = %d, want 2300", got)
	}
	// A delay change to an idle receiver does not start a session.
	h.e.handleCommand(command{kind: cmdDisconnect})
	before = h.sessions
	delay = 60
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: testDeviceID,
		patch: receivers.Patch{DelayMS: &delay}})
	if h.sessions != before {
		t.Fatalf("idle delay change restarted: sessions=%d", h.sessions)
	}
}

func TestDelayClampsIntoSenderWindow(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.addDevice()
	delay := 99999
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: testDeviceID,
		patch: receivers.Patch{DelayMS: &delay}})
	if e, _ := h.e.reg.Get(testDeviceID); e.DelayMS != receivers.MaxDelayMS {
		t.Fatalf("stored delay = %d, want %d", e.DelayMS, receivers.MaxDelayMS)
	}
}

func TestGroupPasswordFallsBackToGroupKey(t *testing.T) {
	h := newHarnessWithRegistry(t)
	tsid := "pair-1"
	left := device.Device{ID: "AA:00", Name: "L", Host: "192.0.2.10", Port: 7000, StereoID: tsid, Online: true}
	right := device.Device{ID: "AA:01", Name: "R", Host: "192.0.2.11", Port: 7000, StereoID: tsid, Online: true}
	group := device.Device{ID: device.StereoGroupID(tsid), Name: "Pair", StereoID: tsid,
		Members: []device.Device{left, right}, Online: true}
	h.e.setDevices([]device.Device{group})
	h.e.creds.SavePassword(device.StereoGroupID(tsid), "group-pw")
	h.e.passwords[device.StereoGroupID(tsid)] = true
	if got := h.e.memberPassword(left); got != "group-pw" {
		t.Fatalf("member password = %q, want the group's", got)
	}
	h.e.creds.SavePassword(left.ID, "own-pw")
	if got := h.e.memberPassword(left); got != "own-pw" {
		t.Fatalf("member password = %q, want the member's own", got)
	}
}

func TestEditManualRowKeepsItsKey(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "书房", Type: receivers.TypeAirPlay1, Host: "192.0.2.7",
	}})
	key := manualKey(h)
	name := "书房改名"
	port := uint16(5353)
	host := "192.0.2.8"
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: key,
		patch: receivers.Patch{Name: &name, Host: &host, Port: &port}})
	e, ok := h.e.reg.Get(key)
	if !ok {
		t.Fatal("the edit re-keyed the manual row")
	}
	if e.Name != "书房改名" || e.Host != "192.0.2.8" || e.Port != 5353 {
		t.Fatalf("edited entry = %+v", e)
	}
	if got := h.e.Snapshot().Receivers[0].Address; got != "192.0.2.8:5353" {
		t.Fatalf("address = %q", got)
	}
}

func TestUpdateManualRowRejectsInvalidAddress(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "书房", Type: receivers.TypeAirPlay2, Host: "192.0.2.7",
	}})
	key := manualKey(h)
	host := "http://example.com/"
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: key,
		patch: receivers.Patch{Host: &host}})
	e, _ := h.e.reg.Get(key)
	if e.Host != "192.0.2.7" {
		t.Fatalf("invalid edit landed: %+v", e)
	}
	if h.e.Snapshot().Error == "" {
		t.Fatal("invalid edit surfaced no error")
	}
}

func TestDiscoveredRowIgnoresAddressAndTypeEdits(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.addDevice()
	host := "192.0.2.99"
	typ := receivers.TypeAirPlay2
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: testDeviceID,
		patch: receivers.Patch{Host: &host, Type: &typ}})
	e, _ := h.e.reg.Get(testDeviceID)
	if e.Host != "192.0.2.10" || e.Port != 7000 || e.Type != "" {
		t.Fatalf("discovered entry took a manual edit: %+v", e)
	}
	if h.e.Snapshot().Error != "" {
		t.Fatalf("address edit on a discovered row errored: %q", h.e.Snapshot().Error)
	}
}

func TestClearingPasswordUpdatesTheRow(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "Mac", Type: receivers.TypeAirPlay2, Host: "192.0.2.40", Password: "hunter2",
	}})
	key := manualKey(h)
	if !h.e.Snapshot().Receivers[0].PasswordSet {
		t.Fatal("password was not stored")
	}
	clear := ""
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: key,
		patch: receivers.Patch{Password: &clear}})
	if h.e.creds.HasPassword(key) {
		t.Fatal("cleared password survived in DPAPI")
	}
	if h.e.Snapshot().Receivers[0].PasswordSet {
		t.Fatal("row still reports a password")
	}
}

func TestMigratedPasswordServesFirstSighting(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := settings.Default()
	cfg.Passwords = map[string]string{testDeviceID: "pw-a"}
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	credsStore := creds.NewStoreAt(filepath.Join(dir, "creds"))
	reg := receivers.NewStoreAt(filepath.Join(dir, "receivers.json"))
	if _, err := receivers.MigrateSettings(sets, credsStore, reg); err != nil {
		t.Fatal(err)
	}
	h := newHarnessWithStores(t, sets, credsStore, reg)
	dev := onlineDevice()
	dev.Auth = device.AuthHAPPassword
	h.e.setDevices([]device.Device{dev})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: testDeviceID})
	if h.e.Snapshot().PasswordRequired {
		t.Fatal("migrated password was treated as missing")
	}
	if h.fake.authPW != "pw-a" {
		t.Fatalf("session password = %q, want the migrated one", h.fake.authPW)
	}
}

func TestGroupSettingsPersistOffline(t *testing.T) {
	h := newHarnessWithRegistry(t)
	tsid := "pair-2"
	left := device.Device{ID: "AA:00", Name: "L", Host: "192.0.2.10", Port: 7000, StereoID: tsid, Online: true}
	right := device.Device{ID: "AA:01", Name: "R", Host: "192.0.2.11", Port: 7000, StereoID: tsid, Online: true}
	group := device.Device{ID: device.StereoGroupID(tsid), Name: "Pair", Model: "AudioAccessory5,1",
		StereoID: tsid, Members: []device.Device{left, right}, Online: true, AirPlay2: true, Auth: device.AuthHAPTransient}
	h.e.setDevices([]device.Device{group})
	key := device.StereoGroupID(tsid)
	if _, ok := h.e.reg.Get(key); !ok {
		t.Fatal("stereo group was not remembered")
	}
	delay := 120
	hidden := true
	on := true
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: key,
		patch: receivers.Patch{DelayMS: &delay, Hidden: &hidden, AutoConnect: &on}})
	h.e.setDevices(nil) // the pair goes offline
	if got := h.e.Snapshot().Receivers; len(got) != 0 {
		t.Fatalf("offline group stayed in the settings list: %+v", got)
	}
	entry, ok := h.e.reg.Get(key)
	if !ok || !entry.Hidden || entry.DelayMS != 120 || !entry.AutoConnect {
		t.Fatalf("group settings lost offline: %+v ok=%v", entry, ok)
	}
	// The settings return with the pair.
	h.e.setDevices([]device.Device{group})
	var view receivers.View
	found := false
	for _, v := range h.e.Snapshot().Receivers {
		if v.Key == key {
			view, found = v, true
		}
	}
	if !found || !view.Stereo || !view.Hidden || view.DelayMS != 120 || !view.AutoConnect {
		t.Fatalf("group settings lost on return: %+v found=%v", view, found)
	}
	if len(h.e.Snapshot().Devices) != 0 {
		t.Fatalf("hidden group stayed in the flyout: %+v", h.e.Snapshot().Devices)
	}
}

func TestGroupDelayReachesTheTransport(t *testing.T) {
	h := newHarnessWithRegistry(t)
	tsid := "pair-3"
	left := device.Device{ID: "AA:10", Name: "L", Host: "192.0.2.10", Port: 7000, StereoID: tsid, Online: true}
	right := device.Device{ID: "AA:11", Name: "R", Host: "192.0.2.11", Port: 7000, StereoID: tsid, Online: true}
	group := device.Device{ID: device.StereoGroupID(tsid), Name: "Pair", Model: "AudioAccessory5,1",
		StereoID: tsid, Members: []device.Device{left, right}, Online: true, AirPlay2: true, Auth: device.AuthHAPTransient}
	h.e.setDevices([]device.Device{group})
	h.e.handleCommand(command{kind: cmdLatency, value: 200})
	delay := 120
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: group.ID,
		patch: receivers.Patch{DelayMS: &delay}})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: group.ID})
	if h.sessions != 1 {
		t.Fatalf("group session did not start: sessions=%d", h.sessions)
	}
	if got := h.fake.latencies[len(h.fake.latencies)-1]; got != 320 {
		t.Fatalf("group latency = %d, want 320", got)
	}
}

func TestRemoveActiveReceiverClearsSelection(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "书房", Type: receivers.TypeAirPlay2, Host: "192.0.2.7", Port: 7000,
	}})
	key := manualKey(h)
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	h.launch()
	if h.e.Snapshot().SelectedID != key {
		t.Fatalf("selected = %q, want %q", h.e.Snapshot().SelectedID, key)
	}
	h.e.handleCommand(command{kind: cmdRemoveReceiver, deviceID: key})
	snap := h.e.Snapshot()
	if snap.SelectedID != "" || h.e.setsValue.LastDeviceID != "" {
		t.Fatalf("selection survived removal: selected=%q last=%q", snap.SelectedID, h.e.setsValue.LastDeviceID)
	}
	for _, d := range snap.Devices {
		if d.ID == key {
			t.Fatalf("removed receiver still offered: %+v", d)
		}
	}
}
