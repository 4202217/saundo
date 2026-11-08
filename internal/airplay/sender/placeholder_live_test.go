package sender

import (
	"os"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/discovery"
	"saundo/internal/lamp"
)

// TestLivePlaceholderReturn probes whether the top display returns to the
// receiver's default placeholder after a template when the sender clears the
// artwork slot and has no media cover. It is silent: the receiver volume is 0
// and the ring is empty.
//
// SAUNDO_PLACEHOLDER_TEST=1 enables it. Stop the saundo app first so PTP UDP
// 319/320 are free.
func TestLivePlaceholderReturn(t *testing.T) {
	if os.Getenv("SAUNDO_PLACEHOLDER_TEST") == "" {
		t.Skip("set SAUNDO_PLACEHOLDER_TEST=1 to run against a real receiver")
	}
	if testing.Short() {
		t.Skip("live device test")
	}

	disc := discovery.New(2*time.Second, 2*time.Second)
	disc.Start()
	defer disc.Stop()
	var target device.Device
	deadline := time.After(12 * time.Second)
wait:
	for {
		select {
		case devs := <-disc.Updates():
			for _, d := range devs {
				if d.Auth == device.AuthHAPTransient && d.AirPlay2 {
					target = d
					break wait
				}
			}
		case <-deadline:
			t.Skip("no transient AirPlay 2 receiver found")
		}
	}
	t.Logf("target: %s (%s) at %s:%d model=%s os=%s",
		target.Name, target.ID, target.Host, target.Port, target.Model, target.OSVersion)

	ls := startLiveSession(t, target)
	defer ls.teardown()
	if !ls.pumpUntilLaunched(30 * time.Second) {
		t.Fatalf("session did not launch: %s", ls.lastErr)
	}
	t.Log("streaming; volume 0 and the ring is empty (silent)")

	image, err := lamp.JPEG("tide")
	if err != nil {
		t.Fatal(err)
	}
	ls.s.SetLampArtwork(image, "image/jpeg", true)
	ls.pumpFor(8 * time.Second)
	t.Log("STEP 1 template tide; the top display should be blue")

	// The sender has no artwork-clear command. Clearing both slots only stops
	// future sends, so this step shows whether the receiver keeps the last
	// image or falls back to its own placeholder.
	ls.s.SetLampArtwork(nil, "", false)
	ls.s.SetNowPlaying("", "", "", nil, "")
	ls.pumpFor(10 * time.Second)
	t.Log("STEP 2 cleared artwork and media; watch whether the top stays blue or returns to the white center")

	ls.s.SetPlaybackState(false)
	ls.pumpFor(5 * time.Second)
	ls.s.SetPlaybackState(true)
	ls.pumpFor(10 * time.Second)
	t.Log("STEP 3 paused then playing; watch whether the top returns to blue or the placeholder")

	ls.s.SetNowPlaying("saundo", "", "", nil, "")
	ls.pumpFor(10 * time.Second)
	t.Log("STEP 4 new item without artwork; watch whether the top returns to the placeholder")
	t.Log("probe done; no sound was played")
}
