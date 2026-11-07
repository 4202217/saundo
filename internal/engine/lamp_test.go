package engine

import (
	"bytes"
	"strings"
	"testing"

	"saundo/internal/airplay/device"
	"saundo/internal/lamp"
)

func capableLampDevice() device.Device {
	return device.Device{
		ID: testDeviceID, Name: "客厅 HomePod", Host: "192.0.2.10", Port: 7000,
		Model: "AudioAccessory6,1", OSVersion: "27.2",
		Auth: device.AuthHAPTransient, AirPlay2: true, Online: true,
	}
}

func lastArtwork(f *fakeTransport) ([]byte, string) {
	if len(f.artwork) == 0 {
		return nil, ""
	}
	return f.artwork[len(f.artwork)-1], f.artworkMIME[len(f.artworkMIME)-1]
}

func lastArtworkOn(f *fakeTransport) bool {
	if len(f.artworkOn) == 0 {
		return false
	}
	return f.artworkOn[len(f.artworkOn)-1]
}

func TestLampToggleAppliesAndPersists(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.connect()
	h.launch()
	h.e.lampJPEG = func(preset string) ([]byte, error) {
		return []byte("jpeg:" + preset), nil
	}

	h.e.handleCommand(command{kind: cmdLamp, value: 1, text: "tide"})
	snap := h.e.Snapshot()
	if !snap.LampEnabled || snap.LampPreset != "tide" {
		t.Fatalf("snapshot lamp = %v/%q, want on/tide", snap.LampEnabled, snap.LampPreset)
	}
	data, mime := lastArtwork(h.fake)
	if !bytes.Equal(data, []byte("jpeg:tide")) || mime != "image/jpeg" || !lastArtworkOn(h.fake) {
		t.Fatalf("transport got %q %q on=%v, want jpeg:tide image/jpeg on", data, mime, lastArtworkOn(h.fake))
	}

	h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "tide"})
	if h.e.Snapshot().LampEnabled {
		t.Fatal("snapshot stayed on after the switch turned off")
	}
	data, mime = lastArtwork(h.fake)
	if !bytes.Equal(data, lamp.Off()) || mime != "image/jpeg" || lastArtworkOn(h.fake) {
		t.Fatalf("off artwork = %d bytes %q on=%v, want the black off image/jpeg", len(data), mime, lastArtworkOn(h.fake))
	}

	h.flushSettings()
	got := h.e.sets.Load()
	if got.Lamp.Enabled || got.Lamp.Preset != "tide" {
		t.Fatalf("persisted lamp = %+v, want off + tide", got.Lamp)
	}
}

func TestLampAppliedWhenStreamingStarts(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.connect()
	h.e.lampJPEG = func(preset string) ([]byte, error) {
		return []byte("jpeg:" + preset), nil
	}
	h.e.handleCommand(command{kind: cmdLamp, value: 1, text: "ember"})
	h.fake.artwork = nil
	h.fake.artworkMIME = nil
	h.fake.artworkOn = nil

	h.launch()
	data, mime := lastArtwork(h.fake)
	if !bytes.Equal(data, []byte("jpeg:ember")) || mime != "image/jpeg" || !lastArtworkOn(h.fake) {
		t.Fatalf("launch artwork = %q %q on=%v, want jpeg:ember image/jpeg on", data, mime, lastArtworkOn(h.fake))
	}
}

func TestLampUnknownPresetFallsBackToOrigin(t *testing.T) {
	h := newHarness(t)
	h.e.setsValue.Lamp.Enabled = false
	h.e.lampJPEG = func(preset string) ([]byte, error) {
		return []byte("jpeg:" + preset), nil
	}
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdLamp, value: 1, text: "bogus"})

	if got := h.e.Snapshot().LampPreset; got != "origin" {
		t.Fatalf("preset = %q, want origin", got)
	}
	if data, _ := lastArtwork(h.fake); !bytes.Equal(data, []byte("jpeg:origin")) {
		t.Fatalf("artwork = %q, want jpeg:origin", data)
	}
	if n := countLogLines(h, "unknown"); n != 1 {
		t.Fatalf("unknown-preset log lines = %d, want 1", n)
	}
}

func TestLampSkipsIncapableReceivers(t *testing.T) {
	cases := []struct {
		name string
		dev  device.Device
	}{
		{"mini on old software", device.Device{Model: "AudioAccessory5,1", OSVersion: "27.1"}},
		{"old software", device.Device{Model: "AudioAccessory6,1", OSVersion: "27.1"}},
		{"unknown software", device.Device{Model: "AudioAccessory6,1"}},
		{"Mac", device.Device{Model: "Mac14,2", OSVersion: "27.2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			d := tc.dev
			d.ID, d.Name, d.Host, d.Port = testDeviceID, "Receiver", "192.0.2.10", 7000
			d.Auth, d.AirPlay2 = device.AuthHAPTransient, true
			h.e.setDevices([]device.Device{d})
			h.connect()
			h.launch()
			h.fake.artwork = nil
			h.fake.artworkMIME = nil
			h.e.lampJPEG = func(preset string) ([]byte, error) {
				return []byte("jpeg:" + preset), nil
			}
			h.e.handleCommand(command{kind: cmdLamp, value: 1, text: "tide"})

			snap := h.e.Snapshot()
			if !snap.LampEnabled || snap.LampPreset != "tide" {
				t.Fatalf("setting did not survive the skip: %v/%q", snap.LampEnabled, snap.LampPreset)
			}
			if len(h.fake.artwork) != 0 {
				t.Fatalf("sent artwork to an incapable receiver: %q", h.fake.artwork)
			}
			if n := countLogLines(h, "does not support"); n != 1 {
				t.Fatalf("skip log lines = %d, want 1", n)
			}
		})
	}
}

// TestLampTogglingTwiceLogsTheSkipOnce pins the per-session throttle: a
// second preset change on the same receiver does not repeat the diagnostic.
func TestLampTogglingTwiceLogsTheSkipOnce(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{{
		ID: testDeviceID, Name: "HomePod mini", Host: "192.0.2.10", Port: 7000,
		Model: "AudioAccessory5,1", OSVersion: "27.1", Auth: device.AuthHAPTransient, AirPlay2: true,
	}})
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdLamp, value: 1, text: "tide"})
	h.e.handleCommand(command{kind: cmdLamp, value: 1, text: "ember"})
	if n := countLogLines(h, "does not support"); n != 1 {
		t.Fatalf("skip log lines = %d, want 1", n)
	}
	if !strings.Contains(h.e.Snapshot().LampPreset, "ember") {
		t.Fatalf("preset = %q, want ember", h.e.Snapshot().LampPreset)
	}
}
