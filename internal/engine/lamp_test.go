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

func lastPlayback(f *fakeTransport) (bool, bool) {
	if len(f.playback) == 0 {
		return false, false
	}
	return f.playback[len(f.playback)-1], true
}

func TestLampOffOnAppliesAndPersists(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.connect()
	h.launch()
	h.e.lampJPEG = func(preset string) ([]byte, error) {
		return []byte("jpeg:" + preset), nil
	}

	h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "tide"})
	snap := h.e.Snapshot()
	if snap.LampOff || snap.LampPreset != "tide" {
		t.Fatalf("snapshot lamp = off:%v/%q, want on/tide", snap.LampOff, snap.LampPreset)
	}
	data, mime := lastArtwork(h.fake)
	if !bytes.Equal(data, []byte("jpeg:tide")) || mime != "image/jpeg" || !lastArtworkOn(h.fake) {
		t.Fatalf("transport got %q %q on=%v, want jpeg:tide image/jpeg on", data, mime, lastArtworkOn(h.fake))
	}
	if playing, ok := lastPlayback(h.fake); !ok || !playing {
		t.Fatalf("template playback = %v/%v, want playing", playing, ok)
	}

	h.e.handleCommand(command{kind: cmdLamp, value: 1, text: "tide"})
	snap = h.e.Snapshot()
	if !snap.LampOff || snap.LampPreset != "tide" {
		t.Fatalf("snapshot lamp = off:%v/%q, want off/tide", snap.LampOff, snap.LampPreset)
	}
	if playing, ok := lastPlayback(h.fake); !ok || playing {
		t.Fatalf("off playback = %v/%v, want paused", playing, ok)
	}

	h.flushSettings()
	got := h.e.sets.Load()
	if !got.Lamp.Off || got.Lamp.Preset != "tide" {
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
	h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "ember"})
	h.fake.artwork = nil
	h.fake.artworkMIME = nil
	h.fake.artworkOn = nil
	h.fake.playback = nil

	h.launch()
	data, mime := lastArtwork(h.fake)
	if !bytes.Equal(data, []byte("jpeg:ember")) || mime != "image/jpeg" || !lastArtworkOn(h.fake) {
		t.Fatalf("launch artwork = %q %q on=%v, want jpeg:ember image/jpeg on", data, mime, lastArtworkOn(h.fake))
	}
	if playing, ok := lastPlayback(h.fake); !ok || !playing {
		t.Fatalf("launch playback = %v/%v, want playing", playing, ok)
	}
}

func TestLampUnknownPresetFallsBackToDefault(t *testing.T) {
	h := newHarness(t)
	h.e.lampJPEG = func(preset string) ([]byte, error) {
		return []byte("jpeg:" + preset), nil
	}
	// The harness may already have cached the real default image; force the
	// stub to render the normalized default preset.
	h.e.lampImage = nil
	h.e.lampImagePreset = ""
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.connect()
	h.launch()
	h.fake.artwork = nil
	h.fake.artworkMIME = nil
	h.fake.artworkOn = nil
	h.fake.playback = nil

	h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "bogus"})

	snap := h.e.Snapshot()
	if snap.LampCover || snap.LampPreset != lamp.DefaultID {
		t.Fatalf("snapshot lamp = cover:%v/%q, want template %q", snap.LampCover, snap.LampPreset, lamp.DefaultID)
	}
	data, mime := lastArtwork(h.fake)
	if !bytes.Equal(data, []byte("jpeg:"+lamp.DefaultID)) || mime != "image/jpeg" || !lastArtworkOn(h.fake) {
		t.Fatalf("template = %q %q on=%v, want the default template", data, mime, lastArtworkOn(h.fake))
	}
	if playing, ok := lastPlayback(h.fake); !ok || !playing {
		t.Fatalf("template with no media = %v/%v, want playing", playing, ok)
	}
	if n := countLogLines(h, "unknown"); n != 1 {
		t.Fatalf("unknown-preset log lines = %d, want 1", n)
	}
}

// TestLampCoverKeepsPlayingOnPause pins the rule change: a media pause no
// longer touches the playback state; only the manual 关灯 does.
func TestLampCoverKeepsPlayingOnPause(t *testing.T) {
	h := newHarness(t)
	h.e.lampJPEG = func(preset string) ([]byte, error) {
		return []byte("jpeg:" + preset), nil
	}
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "dune", lampCover: true})
	snap := h.e.Snapshot()
	if !snap.LampCover || snap.LampPreset != "dune" {
		t.Fatalf("snapshot lamp = cover:%v/%q, want cover + dune", snap.LampCover, snap.LampPreset)
	}
	if playing, ok := lastPlayback(h.fake); !ok || !playing {
		t.Fatalf("cover = %v/%v, want playing", playing, ok)
	}
	if data, _ := lastArtwork(h.fake); data != nil || lastArtworkOn(h.fake) {
		t.Fatalf("cover sent a lamp image: %q on=%v", data, lastArtworkOn(h.fake))
	}

	h.fake.playback = nil
	h.e.handleCommand(command{kind: cmdNowPlaying, text: "夜曲", name: "周杰伦",
		cover: []byte{0xff, 0xd8, 0xff, 0xd9}, coverMIME: "image/jpeg"})
	if len(h.fake.playback) != 0 {
		t.Fatalf("a media update touched the playback state: %v", h.fake.playback)
	}
	if !bytes.Equal(h.fake.nowCover, []byte{0xff, 0xd8, 0xff, 0xd9}) {
		t.Fatalf("media cover not forwarded: %q", h.fake.nowCover)
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
			h.fake.playback = nil
			h.e.lampJPEG = func(preset string) ([]byte, error) {
				return []byte("jpeg:" + preset), nil
			}
			h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "tide"})

			snap := h.e.Snapshot()
			if snap.LampOff || snap.LampPreset != "tide" {
				t.Fatalf("setting did not survive the skip: off:%v/%q", snap.LampOff, snap.LampPreset)
			}
			if len(h.fake.artwork) != 0 {
				t.Fatalf("sent artwork to an incapable receiver: %q", h.fake.artwork)
			}
			if len(h.fake.playback) != 0 {
				t.Fatalf("sent playback state to an incapable receiver: %v", h.fake.playback)
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
	h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "tide"})
	h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "ember"})
	if n := countLogLines(h, "does not support"); n != 1 {
		t.Fatalf("skip log lines = %d, want 1", n)
	}
	if !strings.Contains(h.e.Snapshot().LampPreset, "ember") {
		t.Fatalf("preset = %q, want ember", h.e.Snapshot().LampPreset)
	}
}
