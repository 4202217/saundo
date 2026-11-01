package settings

import (
	"path/filepath"
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
