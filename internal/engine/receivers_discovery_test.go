package engine

import (
	"slices"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
)

// TestDiscoveredReceiverPersistsAddressAndLastSeen pins the restart seed: the
// last advertised endpoint and sighting time survive in receivers.json so a
// carried-over row can be probed directly.
func TestDiscoveredReceiverPersistsAddressAndLastSeen(t *testing.T) {
	h := newHarnessWithRegistry(t)
	base := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	h.now = base
	h.addDeviceOnline()

	entry, ok := h.e.reg.Get(testDeviceID)
	if !ok || entry.Host != "192.0.2.10" || entry.Port != 7000 || !entry.LastSeen.Equal(base) {
		t.Fatalf("entry = %+v ok=%v, want the advertised address and sighting time", entry, ok)
	}
	stored, err := h.e.recvStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok = stored.Get(testDeviceID)
	if !ok || entry.Host != "192.0.2.10" || !entry.LastSeen.Equal(base) {
		t.Fatalf("stored entry = %+v ok=%v, want the last address persisted", entry, ok)
	}
}

// TestDiscoveredGroupPersistsLeaderAddress pins the pair rule: the group row
// stores the leader's host and port, which is the address a directed scan
// probes to rediscover the pair and its tsid.
func TestDiscoveredGroupPersistsLeaderAddress(t *testing.T) {
	h := newHarnessWithRegistry(t)
	base := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	h.now = base
	tsid := "pair-new"
	left := device.Device{ID: "AA:00", Name: "HomePod L", Host: "192.0.2.10", Port: 7000, StereoID: tsid, Online: true}
	right := device.Device{ID: "AA:01", Name: "HomePod R", Host: "192.0.2.11", Port: 7000, StereoID: tsid, GroupLeader: true, Online: true}
	group := device.Device{
		ID: device.StereoGroupID(tsid), Name: "卧室", Host: "192.0.2.11", Port: 7000,
		StereoID: tsid, Members: []device.Device{right, left}, Online: true,
		AirPlay2: true, Auth: device.AuthHAPTransient,
	}
	h.e.setDevices([]device.Device{group})

	entry, ok := h.e.reg.Get(group.ID)
	if !ok || entry.Host != "192.0.2.11" || entry.Port != 7000 || !entry.LastSeen.Equal(base) {
		t.Fatalf("group entry = %+v ok=%v, want the leader endpoint", entry, ok)
	}
}

// TestLegacyGroupGetsGraceInsteadOfImmediatePrune pins the upgrade path: a
// group stored before LastSeen existed is stamped now, not treated as
// infinitely old, so a powered-off pair is not deleted on the first launch.
func TestLegacyGroupGetsGraceInsteadOfImmediatePrune(t *testing.T) {
	h := newHarnessWithRegistry(t)
	base := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	h.now = base
	key := device.StereoGroupID("old-pair")
	h.e.reg.Put(receivers.Entry{Key: key, Kind: receivers.KindDiscovered, Name: "HomePods", AutoConnect: true})

	h.e.setDevices(nil)

	entry, ok := h.e.reg.Get(key)
	if !ok || !entry.LastSeen.Equal(base) {
		t.Fatalf("legacy group = %+v ok=%v, want a fresh grace timestamp", entry, ok)
	}
	stored, err := h.e.recvStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored.Get(key); !ok {
		t.Fatal("stamped group was not persisted")
	}
}

// TestStaleGroupPrunedAfterGraceAndLosesAutoConnect pins the user's case: the
// group that was split and re-merged under a new tsid is removed after the
// grace, along with its auto-connect flag, and the new tsid appears as a new
// row.
func TestStaleGroupPrunedAfterGraceAndLosesAutoConnect(t *testing.T) {
	h := newHarnessWithRegistry(t)
	base := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	h.now = base
	oldKey := device.StereoGroupID("old-pair")
	h.e.reg.Put(receivers.Entry{
		Key: oldKey, Kind: receivers.KindDiscovered, Name: "HomePods",
		AutoConnect: true, LastSeen: base.Add(-25 * time.Hour),
	})

	h.e.setDevices(nil)

	if _, ok := h.e.reg.Get(oldKey); ok {
		t.Fatal("stale group survived the grace period")
	}
	if key, ok := receivers.AutoTarget(h.e.reg); ok {
		t.Fatalf("auto-connect survived on %q", key)
	}
	stored, err := h.e.recvStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored.Get(oldKey); ok {
		t.Fatal("stale group was not removed from disk")
	}

	// The re-merged pair advertises a new tsid and becomes its own row.
	newKey := device.StereoGroupID("new-pair")
	group := device.Device{
		ID: newKey, Name: "卧室", Host: "192.0.2.11", Port: 7000, StereoID: "new-pair",
		Members: []device.Device{
			{ID: "AA:01", Name: "HomePod R", Host: "192.0.2.11", Port: 7000, GroupLeader: true, Online: true},
			{ID: "AA:00", Name: "HomePod L", Host: "192.0.2.10", Port: 7000, Online: true},
		},
		Online: true, AirPlay2: true,
	}
	h.e.setDevices([]device.Device{group})
	if _, ok := h.e.reg.Get(newKey); !ok {
		t.Fatal("new group did not become a new row")
	}
}

// TestGroupOfflineWithinGraceIsKept pins the other half of the rule: a pair
// that is merely powered off for a short time keeps its row and settings.
func TestGroupOfflineWithinGraceIsKept(t *testing.T) {
	h := newHarnessWithRegistry(t)
	base := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	h.now = base
	key := device.StereoGroupID("pair-recent")
	h.e.reg.Put(receivers.Entry{
		Key: key, Kind: receivers.KindDiscovered, Name: "卧室",
		AutoConnect: true, LastSeen: base.Add(-23 * time.Hour),
	})

	h.e.setDevices(nil)

	entry, ok := h.e.reg.Get(key)
	if !ok || !entry.AutoConnect {
		t.Fatalf("recently seen group pruned: %+v ok=%v", entry, ok)
	}
}

// TestReceiverHostsCoverManualAndRememberedAddresses pins the directed-pass
// seed list: a manual host, a discovered receiver's last address, and a
// group's leader address, with empty rows omitted.
func TestReceiverHostsCoverManualAndRememberedAddresses(t *testing.T) {
	reg := receivers.Registry{Entries: []receivers.Entry{
		{Key: "m:01", Kind: receivers.KindManual, Host: "192.0.2.9"},
		{Key: "AA:00", Kind: receivers.KindDiscovered, Host: "192.0.2.10"},
		{Key: device.StereoGroupID("tsid"), Kind: receivers.KindDiscovered, Host: "192.0.2.11"},
		{Key: "AA:01", Kind: receivers.KindDiscovered},
	}}
	got := receiverHosts(reg)
	want := []string{"192.0.2.9", "192.0.2.10", "192.0.2.11"}
	if !slices.Equal(got, want) {
		t.Fatalf("hosts = %v, want %v", got, want)
	}
}
