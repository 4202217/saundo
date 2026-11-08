package sender

import (
	"os"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/discovery"
	"saundo/internal/lamp"
)

// pumpFor runs the sender event loop for d, the same way pumpUntilLaunched
// does. The probe uses it to hold a live session between commands.
func (ls *liveSession) pumpFor(d time.Duration) {
	deadline := time.Now().Add(d)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for time.Now().Before(deadline) {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var timerC <-chan time.Time
		if dl := ls.s.NextDeadline(); !dl.IsZero() {
			wait := time.Until(dl)
			if wait < 0 {
				wait = 0
			}
			timer.Reset(wait)
			timerC = timer.C
		}
		select {
		case ev := <-ls.host.Events():
			ls.s.HandleEvent(ev)
		case <-timerC:
			ls.s.Tick()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// TestLivePlaybackState probes whether the receiver's MR playback state can
// turn the top display off while the RTP stream keeps running. The receiver
// is muted (volume 0) and the ring is empty, so the probe makes no sound.
//
// SAUNDO_PLAYBACK_STATE_TEST=1 enables it. It needs a LAN HomePod on 27.2,
// Windows Firewall permission for inbound UDP timing, and the saundo app
// stopped so PTP UDP 319/320 are free.
func TestLivePlaybackState(t *testing.T) {
	if os.Getenv("SAUNDO_PLAYBACK_STATE_TEST") == "" {
		t.Skip("set SAUNDO_PLAYBACK_STATE_TEST=1 to run against a real receiver")
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
	t.Log("streaming; receiver volume 0 and the ring is empty (silent)")

	image, err := lamp.JPEG("tide")
	if err != nil {
		t.Fatal(err)
	}
	ls.s.SetLampArtwork(image, "image/jpeg", true)
	ls.pumpFor(4 * time.Second)
	t.Logf("%s palette sent; the top display should be lit", time.Now().Format("15:04:05"))

	steps := []struct {
		state int64
		label string
		hold  time.Duration
	}{
		{2, "PAUSED: the top display should go dark; audio must keep streaming", 15 * time.Second},
		{1, "PLAYING: the top display should light again", 15 * time.Second},
		{2, "PAUSED again: dark", 10 * time.Second},
		{1, "PLAYING again: lit", 10 * time.Second},
	}
	for _, step := range steps {
		ls.s.SetPlaybackState(step.state == 1)
		t.Logf("%s sent playback state playing=%v; %s", time.Now().Format("15:04:05"), step.state == 1, step.label)
		ls.pumpFor(step.hold)
		t.Logf("%s state=%s framesSent=%d closed=%v", time.Now().Format("15:04:05"),
			ls.s.State(), ls.s.framesSent, ls.closed)
	}
	t.Log("probe done; the session stayed up and no sound was played")
}
