package settings

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

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
	def.ShowNowPlaying = true
	def.VolumeByDevice = map[string]float64{"AA:BB": 33}
	if err := s.Save(def); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if !got.Reconnect || !got.ShowNowPlaying {
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
	cases := map[int]string{20: "20 ms", 200: "200 ms", 700: "700 ms", 1000: "1 s", 1500: "1.5 s", 2000: "2 s"}
	for ms, want := range cases {
		if got := LatencyText(ms); got != want {
			t.Errorf("LatencyText(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestLampDefaultsOffAndOrigin(t *testing.T) {
	s := NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	got := s.Load()
	if got.Lamp.Enabled {
		t.Fatal("lamp should default to off")
	}
	if got.Lamp.Preset != "origin" {
		t.Fatalf("lamp preset = %q, want origin", got.Lamp.Preset)
	}
}

func TestLampOldFileLoadsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","volume":42}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewStoreAt(path).Load()
	if got.Lamp.Enabled || got.Lamp.Preset != "origin" {
		t.Fatalf("legacy file lamp = %+v, want off + origin", got.Lamp)
	}
}

func TestLampUnknownPresetNormalizesToOrigin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"lamp":{"enabled":true,"preset":"bogus"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewStoreAt(path).Load()
	if !got.Lamp.Enabled || got.Lamp.Preset != "origin" {
		t.Fatalf("unknown preset = %+v, want on + origin", got.Lamp)
	}
}

func TestLampRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := NewStoreAt(path)
	cfg := s.Load()
	cfg.Lamp = LampSettings{Enabled: true, Preset: "ember"}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if !got.Lamp.Enabled || got.Lamp.Preset != "ember" {
		t.Fatalf("lamp round trip = %+v", got.Lamp)
	}
}
