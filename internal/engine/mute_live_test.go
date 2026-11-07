//go:build windows

package engine

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/capture"
	"saundo/internal/diagnostics"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// TestLiveMuteLocal runs a real HomePod session and proves the connection
// mute: the local endpoint is muted once the stream starts and restored on
// disconnect. It reuses the app's sender identity and remembered volume,
// writes its own settings, credentials, and receiver registry under
// t.TempDir, and mutes the receiver before audio can flow. SAUNDO_MUTE_TEST=1
// enables it; it drives the engine directly, not the app's settings UI.
func TestLiveMuteLocal(t *testing.T) {
	if os.Getenv("SAUNDO_MUTE_TEST") == "" {
		t.Skip("set SAUNDO_MUTE_TEST=1 to run against a real receiver")
	}
	if testing.Short() {
		t.Skip("live device test")
	}

	realCfg := settings.NewStore().Load()

	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := settings.Default()
	cfg.DeviceID = realCfg.DeviceID
	cfg.Name = realCfg.Name
	cfg.MuteLocalOnConnect = true
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	credsStore := creds.NewStoreAt(filepath.Join(dir, "credentials.dat"))
	reg := receivers.NewStoreAt(filepath.Join(dir, "receivers.json"))

	previous, err := capture.EndpointMuted("")
	if err != nil {
		t.Fatalf("read the local endpoint: %v", err)
	}
	if err := capture.SetEndpointMuted("", false); err != nil {
		t.Fatalf("unmute the local endpoint: %v", err)
	}
	defer capture.SetEndpointMuted("", previous)

	e := NewWithReceivers(sets, credsStore, reg, &diagnostics.Log{}, nil)
	e.Start()
	defer e.Stop()

	var target device.Device
	waitLive(t, 20*time.Second, "a transient receiver", e, func() bool {
		for _, d := range e.Snapshot().Devices {
			if !d.Online || !d.AirPlay2 {
				continue
			}
			if d.IsStereoGroup() {
				if len(d.Members) == 2 &&
					d.Members[0].Auth == device.AuthHAPTransient &&
					d.Members[1].Auth == device.AuthHAPTransient {
					target = d
					return true
				}
				continue
			}
			if d.Auth == device.AuthHAPTransient {
				target = d
				return true
			}
		}
		return false
	})
	t.Logf("target: %s (%s) at %s:%d", target.Name, target.ID, target.Host, target.Port)

	e.Connect(target.ID)
	e.SetVolume(0) // mute the receiver before audio can flow
	waitLive(t, 40*time.Second, "streaming", e, func() bool {
		return e.Snapshot().State == sender.StateStreaming
	})
	waitLive(t, 5*time.Second, "the local mute", e, func() bool {
		return e.Snapshot().MuteLocal == MuteLocalActive
	})

	muted, err := capture.EndpointMuted("")
	if err != nil {
		t.Fatalf("read the local endpoint: %v", err)
	}
	if !muted {
		t.Fatal("the local endpoint is not muted while streaming")
	}
	if _, err := os.Stat(e.muteGuardPath()); err != nil {
		t.Fatalf("mute guard missing while muted: %v", err)
	}
	t.Logf("streaming; local endpoint muted")

	restoreVolume := realCfg.VolumeByDevice[target.ID]
	if restoreVolume <= 0 {
		restoreVolume = realCfg.Volume
	}
	e.SetVolume(restoreVolume)
	time.Sleep(500 * time.Millisecond)

	e.Disconnect()
	waitLive(t, 15*time.Second, "idle after disconnect", e, func() bool {
		return e.Snapshot().State == sender.StateIdle && e.Snapshot().MuteLocal == MuteLocalWaiting
	})
	muted, err = capture.EndpointMuted("")
	if err != nil {
		t.Fatalf("read the local endpoint: %v", err)
	}
	if muted {
		t.Fatal("the local endpoint stayed muted after disconnect")
	}
	if _, err := os.Stat(e.muteGuardPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mute guard survived the disconnect: %v", err)
	}
	t.Logf("disconnected; local endpoint restored")

	// A second session starts with the endpoint muted, the state left by a
	// user who muted local output before connecting. The connection mute must
	// still release it on disconnect.
	if err := capture.SetEndpointMuted("", true); err != nil {
		t.Fatalf("pre-mute the local endpoint: %v", err)
	}
	e.Connect(target.ID)
	e.SetVolume(0)
	waitLive(t, 40*time.Second, "streaming again", e, func() bool {
		return e.Snapshot().State == sender.StateStreaming
	})
	waitLive(t, 5*time.Second, "the local mute again", e, func() bool {
		return e.Snapshot().MuteLocal == MuteLocalActive
	})
	muted, err = capture.EndpointMuted("")
	if err != nil {
		t.Fatalf("read the local endpoint: %v", err)
	}
	if !muted {
		t.Fatal("the pre-muted local endpoint is not muted while streaming")
	}
	t.Logf("streaming; pre-muted local endpoint held muted")

	e.SetVolume(restoreVolume)
	time.Sleep(500 * time.Millisecond)
	e.Disconnect()
	waitLive(t, 15*time.Second, "idle after the second disconnect", e, func() bool {
		return e.Snapshot().State == sender.StateIdle && e.Snapshot().MuteLocal == MuteLocalWaiting
	})
	muted, err = capture.EndpointMuted("")
	if err != nil {
		t.Fatalf("read the local endpoint: %v", err)
	}
	if muted {
		t.Fatal("the local endpoint stayed muted after disconnect")
	}
	if _, err := os.Stat(e.muteGuardPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mute guard survived the second disconnect: %v", err)
	}
	t.Logf("disconnected; pre-muted local endpoint released")
}

func waitLive(t *testing.T, timeout time.Duration, what string, e *Engine, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; state=%s error=%q",
				what, e.Snapshot().State, e.Snapshot().Error)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
