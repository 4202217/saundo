package settings

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestNewerSettingsSchemaIsNotRewritten pins the downgrade guard: a document
// from a newer build loads its known fields read-only and is never replaced.
func TestNewerSettingsSchemaIsNotRewritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := []byte(`{"version":999,"name":"x","futureKey":"keep-me"}`)
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStoreAt(path)
	if _, err := s.LoadChecked(); err == nil {
		t.Fatal("a settings file stamped version 999 loaded without error")
	}
	if got := s.Load(); got.Name != "x" {
		t.Fatalf("read-only load name = %q, want the stored name", got.Name)
	}
	if err := s.Save(Default()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Save after a newer-schema load = %v, want ErrCorrupt", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, seed) {
		t.Fatalf("newer settings file was rewritten: %q", got)
	}
	if !bytes.Contains(got, []byte("futureKey")) {
		t.Fatal("the newer build's field was stripped")
	}
}

// TestMissingVersionReadsAsCurrent keeps the upgrade path open: a file written
// before the version field existed loads clean and is stamped on save.
func TestMissingVersionReadsAsCurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"name":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStoreAt(path)
	cfg, err := s.LoadChecked()
	if err != nil {
		t.Fatalf("versionless settings file rejected: %v", err)
	}
	if cfg.Version != SchemaVersion {
		t.Fatalf("version = %d, want %d", cfg.Version, SchemaVersion)
	}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"version": 1`)) {
		t.Fatalf("saved file lacks the schema version: %s", got)
	}
}

// TestNewProfilesDefaultToStrictReceiverAuth pins the shipped default: a new
// profile verifies the receiver's proof on the first pairing.
func TestNewProfilesDefaultToStrictReceiverAuth(t *testing.T) {
	if !Default().StrictReceiverAuth {
		t.Fatal("Default().StrictReceiverAuth = false, want true")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if !NewStoreAt(path).Load().StrictReceiverAuth {
		t.Fatal("a missing settings file did not inherit strict receiver auth")
	}
}

// TestExistingPermissiveProfileStaysPermissive pins the new-only rollout:
// files written by an older build carry an explicit false and are not
// migrated.
func TestExistingPermissiveProfileStaysPermissive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"strictReceiverAuth":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if NewStoreAt(path).Load().StrictReceiverAuth {
		t.Fatal("an explicit strictReceiverAuth=false was overridden")
	}
}

// TestCorruptSettingsFileIsNotOverwritten pins the data-loss guard: a parse
// failure must surface, and no later save may replace the user's file.
func TestCorruptSettingsFileIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	corrupt := []byte(`{"name":"x",`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStoreAt(path)
	if _, err := s.LoadChecked(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("LoadChecked error = %v, want ErrCorrupt", err)
	}
	if err := s.Save(Default()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Save after a corrupt load = %v, want ErrCorrupt", err)
	}
	if err := s.Update(func(*Settings) {}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Update after a corrupt load = %v, want ErrCorrupt", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, corrupt) {
		t.Fatalf("settings.json was rewritten: %q", got)
	}
}

func TestVolumeByDeviceAndFlagsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := NewStoreAt(path)
	def := s.Load()
	if def.VolumeByDevice == nil {
		t.Fatal("VolumeByDevice not initialized")
	}
	if !def.Reconnect {
		t.Fatal("reconnect should default to on")
	}
	def.Reconnect = true
	def.VolumeByDevice = map[string]float64{"AA:BB": 33}
	if err := s.Save(def); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if !got.Reconnect {
		t.Fatalf("flags lost: %+v", got)
	}
	if got.VolumeByDevice["AA:BB"] != 33 {
		t.Fatalf("per-device volume lost: %+v", got.VolumeByDevice)
	}
}

func TestTrayAnimationDefaultsOnAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := NewStoreAt(path)
	if !s.Load().TrayAnimation {
		t.Fatal("tray animation should default to on")
	}
	if err := os.WriteFile(path, []byte(`{"name":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !s.Load().TrayAnimation {
		t.Fatal("a settings file without trayAnimation should keep the default")
	}
	cfg := s.Load()
	cfg.TrayAnimation = false
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if s.Load().TrayAnimation {
		t.Fatal("tray animation off did not persist")
	}
}

func TestThemeDefaultsToSystemAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := NewStoreAt(path)
	if got := s.Load().Theme; got != ThemeSystem {
		t.Fatalf("default theme = %q, want system", got)
	}
	if err := os.WriteFile(path, []byte(`{"name":"x","theme":"midnight"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.Load().Theme; got != ThemeSystem {
		t.Fatalf("unknown theme = %q, want system", got)
	}
	cfg := s.Load()
	cfg.Theme = ThemeDark
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if got := s.Load().Theme; got != ThemeDark {
		t.Fatalf("theme after save = %q, want dark", got)
	}
}

func TestLoadDefaultsAndSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := NewStoreAt(path)
	def := s.Load()
	if def.Name == "" || def.DeviceID == "" || def.Volume != 80 {
		t.Fatalf("bad defaults: %+v", def)
	}
	def.Name = "客厅"
	def.Volume = 55
	def.LastDeviceID = "AA:BB:CC:DD:EE:FF"
	if err := s.Save(def); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if got.Name != "客厅" || got.Volume != 55 || got.LastDeviceID != "AA:BB:CC:DD:EE:FF" || got.DeviceID != def.DeviceID {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestLatencyModeDefaults(t *testing.T) {
	s := NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	got := s.Load()
	if got.LatencyMode != LatencyLow || got.LatencyMS != LatencyLowMS {
		t.Fatalf("default latency = %q/%d, want low/%d", got.LatencyMode, got.LatencyMS, LatencyLowMS)
	}
	if got.LatencyCustomMS != DefaultLatencyCustomMS {
		t.Fatalf("default custom = %d, want %d", got.LatencyCustomMS, DefaultLatencyCustomMS)
	}
	if !got.ShowLatencySwitch {
		t.Fatal("the panel switch should default to on")
	}
}

func TestLegacyLatencyMSChoosesMode(t *testing.T) {
	cases := []struct {
		ms   int
		mode LatencyMode
	}{
		{200, LatencyLow},
		{1500, LatencyStandard},
		{2000, LatencyStable},
		{700, LatencyCustom},
	}
	for _, tc := range cases {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(`{"latencyMs":`+strconv.Itoa(tc.ms)+`}`), 0o600); err != nil {
			t.Fatal(err)
		}
		got := NewStoreAt(path).Load()
		if got.LatencyMode != tc.mode {
			t.Errorf("%d ms mode = %q, want %q", tc.ms, got.LatencyMode, tc.mode)
		}
		if got.LatencyMS != tc.ms {
			t.Errorf("%d ms stored as %d", tc.ms, got.LatencyMS)
		}
		if tc.mode == LatencyCustom && got.LatencyCustomMS != tc.ms {
			t.Errorf("%d ms custom = %d", tc.ms, got.LatencyCustomMS)
		}
	}
}

func TestLatencyPresetNormalizesAndCustomRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := NewStoreAt(path)
	cfg := s.Load()
	cfg.LatencyMode = LatencyStable
	cfg.LatencyMS = 300
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if got.LatencyMS != LatencyStableMS || got.LatencyMode != LatencyStable {
		t.Fatalf("stable normalized to %q/%d", got.LatencyMode, got.LatencyMS)
	}
	got.LatencyMode = LatencyCustom
	got.LatencyMS = 640
	got.LatencyCustomMS = 640
	if err := s.Save(got); err != nil {
		t.Fatal(err)
	}
	back := s.Load()
	if back.LatencyMode != LatencyCustom || back.LatencyMS != 640 || back.LatencyCustomMS != 640 {
		t.Fatalf("custom round trip = %+v", back)
	}
	back.ShowLatencySwitch = false
	if err := s.Save(back); err != nil {
		t.Fatal(err)
	}
	if s.Load().ShowLatencySwitch {
		t.Fatal("panel switch off did not persist")
	}
}

func TestLatencyText(t *testing.T) {
	cases := map[int]string{20: "20 ms", 200: "200 ms", 700: "700 ms", 999: "999 ms", 1000: "1 s", 1500: "1.5 s", 2000: "2 s"}
	for ms, want := range cases {
		if got := LatencyText(ms); got != want {
			t.Errorf("LatencyText(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestLampDefaultsOnAndCover(t *testing.T) {
	s := NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	got := s.Load()
	if got.Lamp.Off {
		t.Fatal("light should default to on")
	}
	if !got.Lamp.Cover || got.Lamp.Preset != "dune" {
		t.Fatalf("lamp = %+v, want cover on + the dune template", got.Lamp)
	}
}

func TestDefaultsEnableLocalMute(t *testing.T) {
	if !Default().MuteLocalOnConnect {
		t.Fatal("local output mute should default to on")
	}
}

func TestLampOldFileLoadsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","volume":42}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewStoreAt(path).Load()
	if got.Lamp.Off || !got.Lamp.Cover || got.Lamp.Preset != "dune" {
		t.Fatalf("legacy file lamp = %+v, want light on + cover + dune", got.Lamp)
	}
}

func TestLampUnknownPresetNormalizesToCover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"lamp":{"enabled":true,"preset":"bogus"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewStoreAt(path).Load()
	if got.Lamp.Off || !got.Lamp.Cover || got.Lamp.Preset != "dune" {
		t.Fatalf("unknown preset = %+v, want light on + cover + dune", got.Lamp)
	}
}

func TestLampLegacyEnabledInvertsToOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"lamp":{"enabled":false,"preset":"ember"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewStoreAt(path).Load()
	if !got.Lamp.Off || got.Lamp.Cover || got.Lamp.Preset != "ember" {
		t.Fatalf("legacy enabled=false = %+v, want off + template ember", got.Lamp)
	}
}

func TestLampRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := NewStoreAt(path)
	cfg := s.Load()
	cfg.Lamp = LampSettings{Off: true, Cover: false, Preset: "ember"}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if !got.Lamp.Off || got.Lamp.Cover || got.Lamp.Preset != "ember" {
		t.Fatalf("lamp round trip = %+v, want off + template ember", got.Lamp)
	}
	cfg = s.Load()
	cfg.Lamp = LampSettings{Off: false, Cover: true, Preset: "moss"}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got = s.Load()
	if got.Lamp.Off || !got.Lamp.Cover || got.Lamp.Preset != "moss" {
		t.Fatalf("cover round trip = %+v, want on + cover + moss", got.Lamp)
	}
}

func TestLampOldMediaPresetTurnsCoverOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"lamp":{"preset":"media"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewStoreAt(path).Load()
	if !got.Lamp.Cover || got.Lamp.Preset != "dune" {
		t.Fatalf("old media preset = %+v, want cover + dune", got.Lamp)
	}
}

func TestLampOldTemplatePresetTurnsCoverOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"lamp":{"preset":"ember"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewStoreAt(path).Load()
	if got.Lamp.Cover || got.Lamp.Preset != "ember" {
		t.Fatalf("old template preset = %+v, want template ember", got.Lamp)
	}
}

func TestCoverModeDefaultsToMedia(t *testing.T) {
	if got := Default().CoverMode; got != CoverMedia {
		t.Fatalf("default cover mode = %q, want media", got)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"coverMode":"nonsense"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := NewStoreAt(path).Load().CoverMode; got != CoverMedia {
		t.Fatalf("invalid cover mode = %q, want media", got)
	}
	if err := os.WriteFile(path, []byte(`{"coverMode":"placeholder"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := NewStoreAt(path).Load().CoverMode; got != CoverPlaceholder {
		t.Fatalf("stored cover mode = %q, want placeholder", got)
	}
}

func TestStandbyDefaultsOnWithThirtyMinutes(t *testing.T) {
	got := NewStoreAt(filepath.Join(t.TempDir(), "settings.json")).Load()
	if !got.Standby.Enabled || got.Standby.IdleMinutes != 30 {
		t.Fatalf("standby defaults = %+v, want enabled + 30 minutes", got.Standby)
	}
}

func TestStandbyIdleMinutesNormalize(t *testing.T) {
	cases := map[int]int{-5: 30, 0: 30, 1: 1, 5: 5, 7: 30, 10: 10, 30: 30, 60: 30}
	for stored, want := range cases {
		path := filepath.Join(t.TempDir(), "settings.json")
		body := `{"standby":{"idleMinutes":` + strconv.Itoa(stored) + `}}`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := NewStoreAt(path).Load().Standby.IdleMinutes; got != want {
			t.Errorf("stored %d minutes = %d, want %d", stored, got, want)
		}
	}
}

func TestStandbyOffSurvivesLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"standby":{"enabled":false,"idleMinutes":5}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewStoreAt(path).Load()
	if got.Standby.Enabled {
		t.Fatal("stored standby off was reset on load")
	}
	if got.Standby.IdleMinutes != 5 {
		t.Fatalf("idle minutes = %d, want 5", got.Standby.IdleMinutes)
	}
}

func TestStoreExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := NewStoreAt(path)
	if s.Exists() {
		t.Fatal("a missing settings file reports as existing")
	}
	if err := os.WriteFile(path, []byte(`{"name":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !s.Exists() {
		t.Fatal("an existing settings file reports as missing")
	}
}
