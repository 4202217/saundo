package engine

import "testing"

func TestFirstLaunchAdoptsMuteSentinel(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.fake.initialPct, h.fake.hasInitial = 0, true
	h.launch()

	if got := h.e.setsValue.Volume; got != 0 {
		t.Fatalf("stored volume = %f, want 0", got)
	}
	if got := h.e.setsValue.VolumeByDevice[testDeviceID]; got != 0 {
		t.Fatalf("per-device volume = %f, want the adopted 0", got)
	}
}

func TestFirstLaunchKeepsConfiguredVolumeWhenUnknown(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.e.setsValue.Volume = 65
	h.launch()

	if got := h.e.setsValue.Volume; got != 65 {
		t.Fatalf("stored volume = %f, want the configured 65", got)
	}
	if _, ok := h.e.setsValue.VolumeByDevice[testDeviceID]; ok {
		t.Fatal("an unknown receiver volume wrote a per-device entry")
	}
}

func TestFirstLaunchKeepsRememberedVolume(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.e.setsValue.Volume = 42
	h.e.setsValue.VolumeByDevice = map[string]float64{testDeviceID: 42}
	h.connect()
	h.fake.initialPct, h.fake.hasInitial = 80, true
	h.launch()

	if got := h.e.setsValue.Volume; got != 42 {
		t.Fatalf("stored volume = %f, want the remembered 42", got)
	}
	if got := h.e.setsValue.VolumeByDevice[testDeviceID]; got != 42 {
		t.Fatalf("per-device volume = %f, want 42", got)
	}
	if !h.plan.targets[0].volumeSet || h.plan.targets[0].volume != 42 {
		t.Fatalf("planned volume = %v set=%v, want 42 set", h.plan.targets[0].volume, h.plan.targets[0].volumeSet)
	}
}

func TestReceiverVolumeEventUpdatesPerDevice(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.fake.ev.VolumeChanged(37)

	if got := h.e.setsValue.Volume; got != 37 {
		t.Fatalf("stored volume = %f, want 37", got)
	}
	if got := h.e.setsValue.VolumeByDevice[testDeviceID]; got != 37 {
		t.Fatalf("per-device volume = %f, want 37", got)
	}
}
