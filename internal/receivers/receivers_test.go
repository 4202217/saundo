package receivers

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/settings"
)

func discovered(id, name, host string, port uint16) device.Device {
	return device.Device{
		ID: id, Name: name, Host: host, Port: port,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true,
	}
}

func TestMergeJoinsRememberedManualAndFirstSightings(t *testing.T) {
	reg := Registry{Entries: []Entry{
		{Key: "AA:00", Kind: KindDiscovered, Name: "记住的"},
		{Key: "m:01", Kind: KindManual, Type: TypeAirPlay2, Name: "手动", Host: "192.0.2.9", Port: 7000},
	}}
	devs := []device.Device{
		discovered("AA:01", "新设备", "192.0.2.1", 7000),
		discovered("AA:00", "记住的", "192.0.2.2", 7000),
		discovered("m:01", "同一台", "192.0.2.9", 7000),
	}
	targets := Merge(reg, devs, nil)
	if len(targets) != 3 {
		t.Fatalf("targets = %d rows, want 3: %+v", len(targets), targets)
	}
	byKey := map[string]Target{}
	for _, tg := range targets {
		byKey[tg.Entry.Key] = tg
	}
	live, ok := byKey["AA:00"]
	if !ok || !live.Device.Online || live.Device.Host != "192.0.2.2" {
		t.Fatalf("remembered entry did not take the live advertisement: %+v", live)
	}
	manual, ok := byKey["m:01"]
	if !ok || !manual.Device.Online || manual.Device.ID != "m:01" || manual.Device.Name != "手动" {
		t.Fatalf("manual row lost its identity: %+v", manual)
	}
	if manual.Device.Host != "192.0.2.9" || manual.Device.Model != "AudioAccessory5,1" {
		t.Fatalf("manual row did not claim the live endpoint and model: %+v", manual.Device)
	}
	fresh, ok := byKey["AA:01"]
	if !ok || fresh.Entry.Kind != KindDiscovered || fresh.Entry.Name != "" {
		t.Fatalf("first sighting: %+v", fresh)
	}
}

// TestMergeHidesTheLocalReceiver pins the local rule: a receiver on this
// host is forced into the hidden group, loses any auto-connect flag, and
// never reaches the flyout list.
func TestMergeHidesTheLocalReceiver(t *testing.T) {
	reg := Registry{Entries: []Entry{{
		Key: "AA:00", Kind: KindDiscovered, Name: "本机",
		Hidden: false, AutoConnect: true,
	}}}
	d := discovered("AA:00", "本机", "192.168.1.10", 7000)
	d.Local = true
	targets := Merge(reg, []device.Device{d}, nil)
	if len(targets) != 1 {
		t.Fatalf("targets = %+v, want one row", targets)
	}
	tg := targets[0]
	if !tg.Entry.Hidden || tg.Entry.AutoConnect {
		t.Fatalf("local entry = %+v, want hidden and no auto-connect", tg.Entry)
	}
	v := tg.View(false)
	if !v.Local || !v.Hidden || v.AutoConnect {
		t.Fatalf("local view = %+v, want local, hidden, no auto-connect", v)
	}
	if got := Visible(targets, ""); len(got) != 0 {
		t.Fatalf("local receiver leaked into the flyout: %+v", got)
	}
}

// TestMergeHidesAFreshLocalSighting covers the first sighting, before the
// engine has remembered the receiver.
func TestMergeHidesAFreshLocalSighting(t *testing.T) {
	d := discovered("AA:01", "本机", "192.168.1.10", 7000)
	d.Local = true
	targets := Merge(Registry{}, []device.Device{d}, nil)
	if len(targets) != 1 || !targets[0].Entry.Hidden {
		t.Fatalf("fresh local sighting = %+v, want a hidden row", targets)
	}
}

func TestMergeKeepsRememberedRowOffline(t *testing.T) {
	reg := Registry{Entries: []Entry{
		{Key: "AA:00", Kind: KindDiscovered, Name: "客厅", Model: "AudioAccessory5,1", AirPlay2: true},
	}}
	targets := Merge(reg, nil, nil)
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}
	d := targets[0].Device
	if d.Online {
		t.Fatal("a remembered device with no advertisement read as online")
	}
	if d.Name != "客厅" || d.Model != "AudioAccessory5,1" || !d.AirPlay2 {
		t.Fatalf("offline row lost its remembered fields: %+v", d)
	}
}

// TestMergePinsAPairedEndpoint pins the redirect defense: discovery cannot
// authenticate an advertisement, so a paired device id never moves to the
// address a later reply claims, while the rest of the advertisement updates.
func TestMergePinsAPairedEndpoint(t *testing.T) {
	reg := Registry{Entries: []Entry{{
		Key: "AA:00", Kind: KindDiscovered, Name: "HomePod",
		Host: "192.0.2.66", Port: 7000, PairedHost: "192.0.2.10", PairedPort: 7000,
		Model: "AudioAccessory5,1", AirPlay2: true, Features: 1 << 41, Auth: device.AuthHAPTransient,
	}}}
	targets := Merge(reg, []device.Device{discovered("AA:00", "Renamed", "192.0.2.66", 7000)}, nil)
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}
	d := targets[0].Device
	if d.Host != "192.0.2.10" || d.Port != 7000 {
		t.Fatalf("paired endpoint moved to %s:%d", d.Host, d.Port)
	}
	if d.Name != "Renamed" {
		t.Fatalf("name = %q, want the live advertisement", d.Name)
	}
}

func TestCachedDeviceUsesPairedEndpoint(t *testing.T) {
	e := Entry{
		Key: "AA:00", Host: "192.0.2.66", Port: 7000,
		PairedHost: "192.0.2.10", PairedPort: 7000, AirPlay2: true,
	}
	d, ok := CachedDevice(e, false)
	if !ok {
		t.Fatal("CachedDevice rejected a complete entry")
	}
	if d.Host != "192.0.2.10" || d.Port != 7000 {
		t.Fatalf("cached endpoint = %s:%d, want the paired one", d.Host, d.Port)
	}
}

// TestMergeKeepsOfflineCapabilities pins the remembered row's wire data: an
// offline receiver must keep its cached endpoint, auth path, and features so
// a connect or a restart cannot fall back to AP1 or NTP.
func TestMergeKeepsOfflineCapabilities(t *testing.T) {
	reg := Registry{Entries: []Entry{{
		Key: "AA:00", Kind: KindDiscovered, Name: "HomePod",
		Host: "192.0.2.10", Port: 7000, Model: "AudioAccessory5,1",
		AirPlay2: true, Features: 1 << 41, Auth: device.AuthHAPTransient,
	}}}
	targets := Merge(reg, nil, nil)
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}
	d := targets[0].Device
	if d.Online {
		t.Fatal("offline row read as online")
	}
	if d.Host != "192.0.2.10" || d.Port != 7000 {
		t.Fatalf("offline row lost its endpoint: %s:%d", d.Host, d.Port)
	}
	if d.Auth != device.AuthHAPTransient {
		t.Fatalf("offline row auth = %v, want HAP transient", d.Auth)
	}
	if d.Features != 1<<41 {
		t.Fatalf("offline row features = %#x, want the cached bits", d.Features)
	}
}

// TestManageableDropsOfflineRememberedRows pins the display rule: a
// remembered receiver with no live advertisement is not shown, while a
// manual receiver and an online receiver are, and the active session's row
// stays reachable even when it just went offline.
func TestManageableDropsOfflineRememberedRows(t *testing.T) {
	reg := Registry{Entries: []Entry{
		{Key: "AA:00", Kind: KindDiscovered, Name: "在线"},
		{Key: "AA:01", Kind: KindDiscovered, Name: "离线"},
		{Key: "m:01", Kind: KindManual, Type: TypeAirPlay2, Name: "手动", Host: "192.0.2.9", Port: 7000},
	}}
	devs := []device.Device{discovered("AA:00", "在线", "192.0.2.1", 7000)}
	targets := Manageable(Merge(reg, devs, nil), "")
	if len(targets) != 2 {
		t.Fatalf("targets = %+v, want the online and the manual row", targets)
	}
	for _, target := range targets {
		if target.Entry.Key == "AA:01" {
			t.Fatalf("offline remembered row survived: %+v", target)
		}
	}
	if got := Manageable(Merge(reg, devs, nil), "AA:01"); len(got) != 3 {
		t.Fatalf("active row dropped: %+v", got)
	}
}

func TestMergeManualAuthFollowsTypeAndPassword(t *testing.T) {
	reg := Registry{Entries: []Entry{
		{Key: "m:ap2", Kind: KindManual, Type: TypeAirPlay2, Host: "h", Port: 7000},
		{Key: "m:ap1", Kind: KindManual, Type: TypeAirPlay1, Host: "h", Port: 5000},
		{Key: "m:ax", Kind: KindManual, Type: TypeAirPortExpress, Host: "h", Port: 5000},
		{Key: "m:ap2pw", Kind: KindManual, Type: TypeAirPlay2, Host: "h", Port: 7000},
	}}
	pw := map[string]bool{"m:ap2pw": true, "m:ap1": true}
	targets := Merge(reg, nil, func(k string) bool { return pw[k] })
	auths := map[string]device.Auth{}
	for _, tg := range targets {
		auths[tg.Entry.Key] = tg.Device.Auth
	}
	want := map[string]device.Auth{
		"m:ap2":   device.AuthHAPTransient,
		"m:ap1":   device.AuthPassword,
		"m:ax":    device.AuthSetup,
		"m:ap2pw": device.AuthHAPPassword,
	}
	for key, auth := range want {
		if auths[key] != auth {
			t.Errorf("%s auth = %s, want %s", key, auths[key], auth)
		}
	}
}

func TestVisibleFiltersHiddenAndKeepsActive(t *testing.T) {
	targets := []Target{
		{Device: discovered("AA:00", "可见", "h", 7000), Entry: Entry{Key: "AA:00", Kind: KindDiscovered}},
		{Device: discovered("AA:01", "隐藏", "h", 7000), Entry: Entry{Key: "AA:01", Kind: KindDiscovered, Hidden: true}},
	}
	got := Visible(targets, "")
	if len(got) != 1 || got[0].ID != "AA:00" {
		t.Fatalf("hidden row leaked into the flyout: %+v", got)
	}
	got = Visible(targets, "AA:01")
	if len(got) != 2 {
		t.Fatalf("the active session's hidden row was dropped: %+v", got)
	}
}

func TestAutoTargetPicksTheSingleFlag(t *testing.T) {
	reg := Registry{Entries: []Entry{
		{Key: "AA:00", Kind: KindDiscovered},
		{Key: "AA:01", Kind: KindDiscovered, AutoConnect: true},
	}}
	key, ok := AutoTarget(reg)
	if !ok || key != "AA:01" {
		t.Fatalf("auto target = %q %v, want AA:01", key, ok)
	}
	if _, ok := AutoTarget(Registry{}); ok {
		t.Fatal("empty registry chose a target")
	}
}

func TestSingleAutoConnectKeepsOneFlag(t *testing.T) {
	reg := Registry{Entries: []Entry{
		{Key: "AA:00", Kind: KindDiscovered, AutoConnect: true},
		{Key: "AA:01", Kind: KindDiscovered},
	}}
	reg.SingleAutoConnect("AA:01", true)
	if reg.Entries[0].AutoConnect || !reg.Entries[1].AutoConnect {
		t.Fatalf("two auto-connect flags: %+v", reg.Entries)
	}
	reg.SingleAutoConnect("AA:01", false)
	if reg.Entries[1].AutoConnect {
		t.Fatal("flag did not clear")
	}
}

func TestEffectiveLatencyAddsAndClamps(t *testing.T) {
	cases := []struct{ global, delay, want int }{
		{200, 0, 200},
		{200, 120, 320},
		{2000, 2000, 4000},
		{2000, 5000, 4000},
		{20, 0, 20},
		{200, -50, 200}, // a negative delay clamps to 0, the base stands
	}
	for _, c := range cases {
		if got := EffectiveLatencyMS(c.global, c.delay); got != c.want {
			t.Errorf("EffectiveLatencyMS(%d, %d) = %d, want %d", c.global, c.delay, got, c.want)
		}
	}
}

func TestClampedDelay(t *testing.T) {
	for _, c := range []struct{ in, want int }{{-1, 0}, {0, 0}, {120, 120}, {5000, 2000}} {
		if got := ClampedDelay(c.in); got != c.want {
			t.Errorf("ClampedDelay(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestStoreRoundTripAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receivers.json")
	s := NewStoreAt(path)
	reg, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Entries) != 0 {
		t.Fatalf("missing file is not empty: %+v", reg)
	}
	reg.Put(Entry{Key: "AA:00", Kind: KindDiscovered, Name: "客厅", DelayMS: 120, Hidden: true})
	if err := s.Save(reg); err != nil {
		t.Fatal(err)
	}
	reg, err = NewStoreAt(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Entries) != 1 || reg.Entries[0].Name != "客厅" || reg.Entries[0].DelayMS != 120 || !reg.Entries[0].Hidden {
		t.Fatalf("round trip = %+v", reg)
	}
	if reg.Version != SchemaVersion {
		t.Fatalf("version = %d, want %d", reg.Version, SchemaVersion)
	}
}

func TestSaveRefusesToClobberCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receivers.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStoreAt(path)
	_, err := s.Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("load error = %v, want ErrCorrupt", err)
	}
	reg := Registry{Entries: []Entry{{Key: "AA:00", Kind: KindDiscovered}}}
	if err := s.Save(reg); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("save error = %v, want ErrCorrupt", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{not json" {
		t.Fatal("the corrupt file was overwritten")
	}
}

func TestSaveWorksAfterLoadRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receivers.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStoreAt(path)
	if _, err := s.Load(); !errors.Is(err, ErrCorrupt) {
		t.Fatal("expected corrupt")
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	reg := Registry{Entries: []Entry{{Key: "AA:00", Kind: KindDiscovered}}}
	if err := s.Save(reg); err != nil {
		t.Fatalf("save after recovery: %v", err)
	}
	got, err := NewStoreAt(path).Load()
	if err != nil || len(got.Entries) != 1 {
		t.Fatalf("recovered save = %+v %v", got, err)
	}
}

func TestStoreRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receivers.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStoreAt(path)
	if _, err := s.Load(); err == nil {
		t.Fatal("newer schema loaded as current")
	}
	reg := Registry{Entries: []Entry{{Key: "AA:00", Kind: KindDiscovered}}}
	if err := s.Save(reg); err == nil {
		t.Fatal("save overwrote a newer schema")
	}
}

func TestViewMarksArememberedGroup(t *testing.T) {
	// An offline group entry keeps its group identity from the key alone.
	target := Target{
		Device: device.Device{ID: "stereo:pair", Name: "Pair", Online: false},
		Entry:  Entry{Key: "stereo:pair", Kind: KindDiscovered, Name: "Pair"},
	}
	if v := target.View(false); !v.Stereo {
		t.Fatalf("offline group lost its identity: %+v", v)
	}
}

func TestSpecValidationNamesTheField(t *testing.T) {
	cases := []struct {
		spec  Spec
		field Field
	}{
		{Spec{Type: "bogus", Host: "h"}, FieldType},
		{Spec{Type: TypeAirPlay2}, FieldHost},
		{Spec{Type: TypeAirPlay2, Host: "http://example.com/"}, FieldHost},
		{Spec{Type: TypeAirPlay2, Host: "192.0.2.1:7000"}, FieldHost},
		{Spec{Type: TypeAirPlay2, Host: "bad..host"}, FieldHost},
	}
	for _, c := range cases {
		err := c.spec.Validate()
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != c.field {
			t.Errorf("%+v: err = %v, want field %d", c.spec, err, c.field)
		}
	}
	if err := (Spec{Type: TypeAirPlay1, Host: "receiver.local", Port: 1}).Validate(); err != nil {
		t.Fatalf("port 1 rejected: %v", err)
	}
}

func TestStoreClampsStoredDelay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receivers.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"entries":[{"key":"AA:00","kind":"discovered","delayMs":99999}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := NewStoreAt(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if reg.Entries[0].DelayMS != MaxDelayMS {
		t.Fatalf("stored delay = %d, want %d", reg.Entries[0].DelayMS, MaxDelayMS)
	}
}

// TestMigrateSettingsRefusesCorruptSettings pins the startup path that used to
// replace a corrupt settings.json with freshly minted defaults.
// TestMigrateSettingsRefusesNewerSettingsSchema pins the startup path against
// rewriting a document written by a newer build.
func TestMigrateSettingsRefusesNewerSettingsSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	seed := []byte(`{"version":999,"name":"x","futureKey":"keep-me"}`)
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	sets := settings.NewStoreAt(path)
	secret := &fakePasswords{values: map[string]string{}}
	reg := NewStoreAt(filepath.Join(dir, "receivers.json"))
	if _, err := MigrateSettings(sets, secret, reg); err == nil {
		t.Fatal("migration accepted a settings file from a newer schema")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(seed) {
		t.Fatalf("newer settings file was rewritten: %q", got)
	}
}

func TestMigrateSettingsRefusesCorruptSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	corrupt := []byte(`{"passwords":`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	sets := settings.NewStoreAt(path)
	secret := &fakePasswords{values: map[string]string{}}
	regPath := filepath.Join(dir, "receivers.json")
	reg := NewStoreAt(regPath)
	if _, err := MigrateSettings(sets, secret, reg); !errors.Is(err, settings.ErrCorrupt) {
		t.Fatalf("migration error = %v, want ErrCorrupt", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("settings.json was rewritten: %q", got)
	}
	if _, err := os.Stat(regPath); !os.IsNotExist(err) {
		t.Fatalf("migration wrote the registry despite the corrupt settings: %v", err)
	}
}

func TestMigrateSettingsMovesPasswordsAndSeedsAutoConnect(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := settings.Default()
	cfg.AutoConnect = true
	cfg.LastDeviceID = "AA:00"
	cfg.Passwords = map[string]string{"AA:00": "pw-a", "AA:01": "pw-b"}
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	secret := &fakePasswords{values: map[string]string{}}
	reg := NewStoreAt(filepath.Join(dir, "receivers.json"))

	res, err := MigrateSettings(sets, secret, reg)
	if err != nil {
		t.Fatal(err)
	}
	if res.MovedPasswords != 2 || res.KeptPasswords != 0 || !res.SeededAuto {
		t.Fatalf("migration = %+v", res)
	}
	if got := secret.LoadPassword("AA:00"); got != "pw-a" {
		t.Fatalf("DPAPI password AA:00 = %q", got)
	}
	if got := secret.LoadPassword("AA:01"); got != "pw-b" {
		t.Fatalf("DPAPI password AA:01 = %q", got)
	}
	saved := sets.Load()
	if len(saved.Passwords) != 0 {
		t.Fatalf("plaintext passwords survived: %v", saved.Passwords)
	}
	if saved.AutoConnect {
		t.Fatal("the legacy flag survived; the migration would run twice")
	}
	stored, err := reg.Load()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := stored.Get("AA:00")
	if !ok || !e.AutoConnect || e.Kind != KindDiscovered {
		t.Fatalf("seeded entry = %+v ok=%v", e, ok)
	}
}

func TestMigrateSettingsIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := settings.Default()
	cfg.AutoConnect = true
	cfg.LastDeviceID = "AA:00"
	cfg.Passwords = map[string]string{"AA:00": "pw-a"}
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	secret := &fakePasswords{values: map[string]string{}}
	reg := NewStoreAt(filepath.Join(dir, "receivers.json"))

	if _, err := MigrateSettings(sets, secret, reg); err != nil {
		t.Fatal(err)
	}
	// A second run moves nothing and does not touch the seed.
	res, err := MigrateSettings(sets, secret, reg)
	if err != nil {
		t.Fatal(err)
	}
	if res.MovedPasswords != 0 || res.SeededAuto {
		t.Fatalf("second migration = %+v", res)
	}
	stored, _ := reg.Load()
	if len(stored.Entries) != 1 {
		t.Fatalf("entries after two runs = %+v", stored.Entries)
	}
}

func TestMigrateSettingsKeepsPasswordDpapiCannotSave(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := settings.Default()
	cfg.Passwords = map[string]string{"AA:00": "pw-a"}
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	secret := &fakePasswords{values: map[string]string{}, fail: true}
	reg := NewStoreAt(filepath.Join(dir, "receivers.json"))

	res, err := MigrateSettings(sets, secret, reg)
	if err != nil {
		t.Fatal(err)
	}
	if res.MovedPasswords != 0 || res.KeptPasswords != 1 {
		t.Fatalf("migration = %+v", res)
	}
	if got := sets.Load().Passwords["AA:00"]; got != "pw-a" {
		t.Fatalf("the password was lost: %q", got)
	}
}

func TestMigrateSettingsClearsAnUnseedableAutoConnect(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := settings.Default()
	cfg.AutoConnect = true
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	secret := &fakePasswords{values: map[string]string{}}
	reg := NewStoreAt(filepath.Join(dir, "receivers.json"))
	res, err := MigrateSettings(sets, secret, reg)
	if err != nil {
		t.Fatal(err)
	}
	if res.SeededAuto {
		t.Fatalf("migration seeded without a device: %+v", res)
	}
	if sets.Load().AutoConnect {
		t.Fatal("the unseedable legacy flag survived")
	}
}

func TestMigrateSettingsKeepsAnExistingDpapiPassword(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := settings.Default()
	cfg.Passwords = map[string]string{"AA:00": "stale-plaintext"}
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	// A password changed through the receiver page after an earlier partial
	// migration must not be overwritten by the older plaintext copy.
	secret := &fakePasswords{values: map[string]string{"AA:00": "newer"}}
	reg := NewStoreAt(filepath.Join(dir, "receivers.json"))
	res, err := MigrateSettings(sets, secret, reg)
	if err != nil {
		t.Fatal(err)
	}
	if got := secret.LoadPassword("AA:00"); got != "newer" {
		t.Fatalf("DPAPI password = %q, want the newer one", got)
	}
	if len(sets.Load().Passwords) != 0 {
		t.Fatal("stale plaintext password survived")
	}
	if res.MovedPasswords != 1 || res.KeptPasswords != 0 {
		t.Fatalf("migration = %+v", res)
	}
}

type fakePasswords struct {
	values map[string]string
	fail   bool
}

// TestPruneStaleGroupsRemovesOnlyExpiredUndiscoveredGroups pins the prune
// rule: a stereo group unseen for longer than the grace goes, carrying its
// auto-connect flag; a recent group, an unstamped group, a live group, a
// single receiver, and a manual entry stay.
func TestPruneStaleGroupsRemovesOnlyExpiredUndiscoveredGroups(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	old := now.Add(-25 * time.Hour)
	reg := Registry{Entries: []Entry{
		{Key: "stereo:old", Kind: KindDiscovered, AutoConnect: true, LastSeen: old},
		{Key: "stereo:recent", Kind: KindDiscovered, LastSeen: now.Add(-time.Hour)},
		{Key: "stereo:unstamped", Kind: KindDiscovered},
		{Key: "stereo:live", Kind: KindDiscovered, LastSeen: old},
		{Key: "AA:00", Kind: KindDiscovered, LastSeen: old},
		{Key: "m:01", Kind: KindManual, Host: "192.0.2.9", LastSeen: old},
	}}
	removed := PruneStaleGroups(&reg, map[string]bool{"stereo:live": true}, now, GroupGrace)
	if !slices.Equal(removed, []string{"stereo:old"}) {
		t.Fatalf("removed = %v, want the expired group", removed)
	}
	if _, ok := reg.Get("stereo:old"); ok {
		t.Fatal("expired group survived")
	}
	for _, key := range []string{"stereo:recent", "stereo:unstamped", "stereo:live", "AA:00", "m:01"} {
		if _, ok := reg.Get(key); !ok {
			t.Fatalf("row %q was pruned", key)
		}
	}
}

func (f *fakePasswords) SavePassword(key, password string) error {
	if f.fail {
		return errors.New("dpapi down")
	}
	f.values[key] = password
	return nil
}
func (f *fakePasswords) LoadPassword(key string) string  { return f.values[key] }
func (f *fakePasswords) DeletePassword(key string) error { delete(f.values, key); return nil }
func (f *fakePasswords) HasPassword(key string) bool     { _, ok := f.values[key]; return ok }

func TestCachedDeviceGroupUsesMembers(t *testing.T) {
	e := Entry{
		Key: "stereo:tsid", Name: "Pair", Host: "192.0.2.10", Port: 7000,
		Model: "AudioAccessory5,1", AirPlay2: true, Features: 1 << 41,
		Auth: device.AuthHAPTransient,
		Members: []Member{
			{ID: "L", Name: "L", Host: "192.0.2.10", Port: 7000, Model: "AudioAccessory5,1", AirPlay2: true, Features: 1 << 41, Auth: device.AuthHAPTransient},
			{ID: "R", Name: "R", Host: "192.0.2.11", Port: 7000, Model: "AudioAccessory5,1", AirPlay2: true, Features: 1 << 41, Auth: device.AuthHAPTransient},
		},
	}
	d, ok := CachedDevice(e, false)
	if !ok {
		t.Fatal("CachedDevice rejected a complete group")
	}
	if d.StereoID != "tsid" || len(d.Members) != 2 || !d.Online {
		t.Fatalf("cached group = %+v", d)
	}
	if d.Members[1].Host != "192.0.2.11" || d.Members[1].Port != 7000 {
		t.Fatalf("member R = %+v", d.Members[1])
	}
	if d.Features != 1<<41 || d.Auth != device.AuthHAPTransient {
		t.Fatalf("cached group capabilities = features %#x auth %v", d.Features, d.Auth)
	}
}

func TestCachedDeviceGroupWithoutMembersWaits(t *testing.T) {
	e := Entry{Key: "stereo:tsid", Host: "192.0.2.10", Port: 7000, AirPlay2: true}
	if d, ok := CachedDevice(e, false); ok {
		t.Fatalf("group without members produced %+v", d)
	}
}

func TestCachedDeviceUsesPasswordAuth(t *testing.T) {
	e := Entry{Key: "AA:00", Host: "192.0.2.10", Port: 7000, AirPlay2: true}
	d, ok := CachedDevice(e, true)
	if !ok || d.Auth != device.AuthHAPPassword {
		t.Fatalf("cached device = %+v, ok=%v", d, ok)
	}
}

func TestDuplicatesFor(t *testing.T) {
	cases := []struct {
		sent, retransmits uint64
		want              int
	}{
		{0, 0, 1},
		{1000, 0, 0},
		{1000, 1, 1},
		{1000, 40, 2},
	}
	for _, c := range cases {
		got := DuplicatesFor(Entry{LastSentBlocks: c.sent, LastRetransmits: c.retransmits})
		if got != c.want {
			t.Fatalf("DuplicatesFor(sent=%d, retransmits=%d) = %d, want %d", c.sent, c.retransmits, got, c.want)
		}
	}
}
