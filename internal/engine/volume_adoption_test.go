package engine

import "testing"

func TestFirstLaunchAdoptsMuteSentinel(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.fake.initialPct, h.fake.hasInitial = 0, true
	h.launch()

	if h.e.volume != 0 || h.e.setsValue.Volume != 0 {
		t.Fatalf("volume = %f, stored = %f, want 0", h.e.volume, h.e.setsValue.Volume)
	}
	if _, ok := h.e.setsValue.VolumeByDevice[testDeviceID]; ok {
		t.Fatal("launch adoption wrote a per-device volume")
	}
}

func TestFirstLaunchKeepsConfiguredVolumeWhenUnknown(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.e.volume = 65
	h.e.setsValue.Volume = 65
	h.launch()

	if h.e.volume != 65 || h.e.setsValue.Volume != 65 {
		t.Fatalf("volume = %f, stored = %f, want the configured 65", h.e.volume, h.e.setsValue.Volume)
	}
	if _, ok := h.e.setsValue.VolumeByDevice[testDeviceID]; ok {
		t.Fatal("an unknown receiver volume wrote a per-device entry")
	}
}

func TestFirstLaunchKeepsRememberedVolume(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.e.volume = 42
	h.e.setsValue.Volume = 42
	h.e.setsValue.VolumeByDevice = map[string]float64{testDeviceID: 42}
	h.connect()
	h.fake.initialPct, h.fake.hasInitial = 80, true
	h.launch()

	if h.e.volume != 42 {
		t.Fatalf("volume = %f, want the remembered 42", h.e.volume)
	}
	if got := h.e.setsValue.VolumeByDevice[testDeviceID]; got != 42 {
		t.Fatalf("per-device volume = %f, want 42", got)
	}
	if len(h.fake.volumes) != 1 || h.fake.volumes[0] != 42 {
		t.Fatalf("transport volume calls = %v, want [42]", h.fake.volumes)
	}
}

func TestReceiverVolumeEventUpdatesPerDevice(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.fake.ev.VolumeChanged(37)

	if h.e.volume != 37 || h.e.setsValue.Volume != 37 {
		t.Fatalf("volume = %f, stored = %f, want 37", h.e.volume, h.e.setsValue.Volume)
	}
	if got := h.e.setsValue.VolumeByDevice[testDeviceID]; got != 37 {
		t.Fatalf("per-device volume = %f, want 37", got)
	}
}
