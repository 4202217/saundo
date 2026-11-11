package engine

import (
	"path/filepath"
	"testing"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

func pinnedEntry(host string, port uint16) receivers.Entry {
	return receivers.Entry{
		Key: testDeviceID, Kind: receivers.KindDiscovered, Name: "客厅 HomePod",
		Host: host, Port: port, PairedHost: "192.0.2.10", PairedPort: 7000,
		Model: "AudioAccessory5,1", AirPlay2: true, Auth: device.AuthHAPTransient,
	}
}

func movedDevice(host string) device.Device {
	return device.Device{
		ID: testDeviceID, Name: "客厅 HomePod", Host: host, Port: 7000,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true,
	}
}

// TestConnectRefusesAMovedPairedEndpoint pins the engine boundary: even a
// device that bypassed the merge cannot hand stored credentials to a host
// other than the one it was paired with.
func TestConnectRefusesAMovedPairedEndpoint(t *testing.T) {
	h := newHarness(t)
	h.e.reg.Put(pinnedEntry("192.0.2.66", 7000))
	h.e.connect(movedDevice("192.0.2.66"))
	if h.sessions != 0 {
		t.Fatalf("sessions = %d, want no session to the moved endpoint", h.sessions)
	}
	if h.e.lastError == "" {
		t.Fatal("the refusal was not surfaced")
	}
}

// TestDiscoveryKeepsAPairedEndpoint pins the engine integration: a live
// advertisement at a new address updates the row's name but not its endpoint.
func TestDiscoveryKeepsAPairedEndpoint(t *testing.T) {
	h := newHarness(t)
	h.e.reg.Put(pinnedEntry("192.0.2.66", 7000))
	live := movedDevice("192.0.2.66")
	live.Name = "Renamed"
	h.e.setDevices([]device.Device{live})
	target, ok := h.e.target(testDeviceID)
	if !ok {
		t.Fatal("paired receiver missing from the rows")
	}
	if target.Device.Host != "192.0.2.10" || target.Device.Port != 7000 {
		t.Fatalf("target endpoint = %s:%d, want the paired one", target.Device.Host, target.Device.Port)
	}
	if target.Device.Name != "Renamed" {
		t.Fatalf("target name = %q, want the live advertisement", target.Device.Name)
	}
}

// TestCredentialsPinThePairedEndpoint pins the write half: the address a
// fresh pairing used becomes the pin, and it persists.
func TestCredentialsPinThePairedEndpoint(t *testing.T) {
	h := newHarness(t)
	h.addDeviceOnline()
	h.connect()
	if h.sessions != 1 {
		t.Fatalf("sessions = %d, want the connect to open one", h.sessions)
	}
	h.e.onCredentials(testDeviceID, "stored-json")
	entry, ok := h.e.reg.Get(testDeviceID)
	if !ok || entry.PairedHost != "192.0.2.10" || entry.PairedPort != 7000 {
		t.Fatalf("entry = %+v ok=%v, want the pairing endpoint pinned", entry, ok)
	}
	stored, err := h.e.recvStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	persisted, ok := stored.Get(testDeviceID)
	if !ok || persisted.PairedHost != "192.0.2.10" || persisted.PairedPort != 7000 {
		t.Fatalf("persisted entry = %+v ok=%v", persisted, ok)
	}
}

// TestEditingAManualAddressClearsThePin pins the deliberate user path: a
// typed address edit is a re-pair, so the old pin must not block it.
func TestEditingAManualAddressClearsThePin(t *testing.T) {
	h := newHarness(t)
	h.e.handleAddReceiver(receivers.Spec{
		Name: "书房", Type: receivers.TypeAirPlay2, Host: "192.0.2.10", Port: 7000,
	})
	key := manualKey(h)
	entry, ok := h.e.reg.Get(key)
	if !ok {
		t.Fatal("manual receiver was not added")
	}
	entry.PairedHost, entry.PairedPort = "192.0.2.10", 7000
	h.e.reg.Put(entry)
	host := "192.0.2.11"
	h.e.handleUpdateReceiver(key, receivers.Patch{Host: &host})
	got, _ := h.e.reg.Get(key)
	if got.Host != "192.0.2.11" {
		t.Fatalf("host = %q, want the edited address", got.Host)
	}
	if got.PairedHost != "" || got.PairedPort != 0 {
		t.Fatalf("pin = %s:%d, want it cleared by the edit", got.PairedHost, got.PairedPort)
	}
}

// TestStartupPinsExistingCredentials covers the upgrade path: credentials
// written before the pin existed are pinned to the last known address before
// discovery can move them.
func TestStartupPinsExistingCredentials(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	if err := sets.Save(sets.Load()); err != nil {
		t.Fatal(err)
	}
	regStore := receivers.NewStoreAt(filepath.Join(dir, "receivers.json"))
	reg := receivers.Registry{Entries: []receivers.Entry{{
		Key: testDeviceID, Kind: receivers.KindDiscovered, Name: "客厅 HomePod",
		Host: "192.0.2.10", Port: 7000, Model: "AudioAccessory5,1",
		AirPlay2: true, Auth: device.AuthHAPTransient,
	}}}
	if err := regStore.Save(reg); err != nil {
		t.Fatal(err)
	}
	credsStore := creds.NewStoreAt(filepath.Join(dir, "creds"))
	if err := credsStore.Save(testDeviceID, "stored-json"); err != nil {
		t.Fatal(err)
	}
	newHarnessWithStores(t, sets, credsStore, regStore)
	stored, err := regStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := stored.Get(testDeviceID)
	if !ok || entry.PairedHost != "192.0.2.10" || entry.PairedPort != 7000 {
		t.Fatalf("entry = %+v ok=%v, want a startup pin", entry, ok)
	}
}
