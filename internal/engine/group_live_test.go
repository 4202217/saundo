package engine

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/diagnostics"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// TestLiveStereoGroup connects to the real HomePod stereo pair, streams a
// 440 Hz tone (with SAUNDO_STEREO_TONE=1) and proves that both member
// sessions reached streaming through the sender-supplied PTP timeline.
// Set SAUNDO_STEREO_TEST=1 to run it. The tone is audible, so run it only
// when sound in the room is welcome.
func TestLiveStereoGroup(t *testing.T) {
	if os.Getenv("SAUNDO_STEREO_TEST") == "" {
		t.Skip("set SAUNDO_STEREO_TEST=1 to stream to the real stereo pair")
	}
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	if err := sets.Save(sets.Load()); err != nil {
		t.Fatal(err)
	}
	e := NewWithReceivers(sets, creds.NewStoreAt(filepath.Join(dir, "creds")),
		receivers.NewStoreAt(filepath.Join(dir, "receivers.json")), &diagnostics.Log{}, nil)
	e.Start()
	defer e.Stop()

	var group device.Device
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, d := range e.Snapshot().Devices {
			if d.IsStereoGroup() && d.Online {
				group = d
				break
			}
		}
		if group.ID != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if group.ID == "" {
		t.Fatal("no complete stereo pair appeared in discovery")
	}
	t.Logf("stereo group %q id=%s members=%d", group.Name, group.ID, len(group.Members))
	for _, m := range group.Members {
		t.Logf("  member %q %s leader=%v", m.Name, m.Host, m.GroupLeader)
	}

	e.Connect(group.ID)
	deadline = time.Now().Add(45 * time.Second)
	var snap Snapshot
	for time.Now().Before(deadline) {
		snap = e.Snapshot()
		if snap.State == sender.StateStreaming && snap.PTPReady {
			break
		}
		if snap.Error != "" && snap.State == sender.StateIdle {
			t.Fatalf("group session failed: %s", snap.Error)
		}
		time.Sleep(250 * time.Millisecond)
	}
	if snap.State != sender.StateStreaming {
		t.Fatalf("group did not start: state=%s error=%s ptp=%v/%v", snap.State, snap.Error, snap.PTPActive, snap.PTPReady)
	}
	if !snap.PTPActive || !snap.PTPReady {
		t.Fatalf("PTP master not ready for both members: active=%v ready=%v", snap.PTPActive, snap.PTPReady)
	}
	t.Logf("streaming to %q, volume %.0f, ptp=%v/%v, member rings=%d", snap.ConnectedName, snap.Volume, snap.PTPActive, snap.PTPReady, len(e.memberRings))

	initialVolume := snap.Volume
	if initialVolume > 30 {
		e.SetVolume(30)
		snap.Volume = 30
		time.Sleep(300 * time.Millisecond)
	}
	if os.Getenv("SAUNDO_STEREO_TONE") == "1" {
		rate := snap.CaptureRate
		if rate <= 0 {
			rate = 48000
		}
		tone := make([]int16, rate*2*4)
		for i := 0; i < rate*4; i++ {
			v := int16(8000 * math.Sin(2*math.Pi*440*float64(i)/float64(rate)))
			tone[i*2] = v
			tone[i*2+1] = v
		}
		const chunkFrames = 960
		chunk := make([]int16, chunkFrames*2)
		for off := 0; off < len(tone); off += len(chunk) {
			n := copy(chunk, tone[off:])
			e.captureSink.Push(chunk[:n])
			time.Sleep(time.Duration(n/2) * time.Second / time.Duration(rate))
		}
		time.Sleep(time.Second)
		snap = e.Snapshot()
		if snap.State != sender.StateStreaming {
			t.Fatalf("group stopped during the tone: state=%s error=%s", snap.State, snap.Error)
		}
		if snap.DroppedSamples != 0 {
			t.Fatalf("ring dropped %d samples during the tone", snap.DroppedSamples)
		}
		t.Logf("tone finished: state=%s dropped=%d", snap.State, snap.DroppedSamples)
	}

	if initialVolume > 30 && initialVolume <= 100 {
		e.SetVolume(initialVolume)
		time.Sleep(300 * time.Millisecond)
	}
	e.Disconnect()
	time.Sleep(500 * time.Millisecond)
}
