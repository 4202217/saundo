package receivers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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
