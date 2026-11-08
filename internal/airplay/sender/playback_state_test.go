package sender

import (
	"bytes"
	"strings"
	"testing"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
)

func commandRequests(r *rig) []string {
	var out []string
	for _, c := range r.rx.commands {
		ct, body, ok := strings.Cut(c, "\x00")
		if ok && ct == "application/x-apple-binary-plist" {
			out = append(out, body)
		}
	}
	return out
}

func playbackBody(t *testing.T, state int) []byte {
	t.Helper()
	body, err := crypto.Encode(map[string]any{
		"type": "updateMRPlaybackState",
		"params": map[string]any{
			"mrPlaybackState": state,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestSetPlaybackStateSendsCommand(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.commands = nil

	r.sender.SetPlaybackState(false)
	r.step()
	got := commandRequests(r)
	if len(got) != 1 {
		t.Fatalf("command requests = %d, want 1", len(got))
	}
	if !bytes.Equal([]byte(got[0]), playbackBody(t, 2)) {
		t.Fatalf("paused body = % x, want % x", got[0], playbackBody(t, 2))
	}

	// The same state is a no-op; playing sends the other value.
	r.sender.SetPlaybackState(false)
	r.step()
	if got := commandRequests(r); len(got) != 1 {
		t.Fatalf("duplicate state sent: %d", len(got))
	}
	r.sender.SetPlaybackState(true)
	r.step()
	got = commandRequests(r)
	if len(got) != 2 || !bytes.Equal([]byte(got[1]), playbackBody(t, 1)) {
		t.Fatalf("playing body = %d % x", len(got), got)
	}
}

func TestSetPlaybackStateAssumesPlayingOnSessionStart(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.commands = nil

	r.sender.SetPlaybackState(true)
	r.step()
	if got := commandRequests(r); len(got) != 0 {
		t.Fatalf("redundant playing state sent on a fresh session: %v", got)
	}
}

func TestSetPlaybackStateResumesWithTheCover(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	image := []byte{0xff, 0xd8, 0xff, 0xe0, 0x10, 0xff, 0xd9}
	r.sender.SetLampArtwork(image, "image/jpeg", true)
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("initial cover = %d, want 1", len(got))
	}

	r.sender.SetPlaybackState(false)
	r.step()
	r.sender.SetPlaybackState(true)
	r.step()

	if got := commandRequests(r); len(got) != 2 {
		t.Fatalf("state commands = %d, want paused then playing", len(got))
	}
	if got := coverRequests(r); len(got) != 2 {
		t.Fatalf("resume covers = %d, want the artwork after the state", len(got))
	}
	if !bytes.Equal([]byte(coverRequests(r)[1]), image) {
		t.Fatalf("resume cover = % x, want the template", coverRequests(r)[1])
	}
}

func TestSetPlaybackStateWaitsForStreaming(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.sender.SetPlaybackState(false)
	r.step()
	if got := commandRequests(r); len(got) != 0 {
		t.Fatalf("playback state sent before streaming: %v", got)
	}

	r.start()
	if got := commandRequests(r); len(got) != 1 {
		t.Fatalf("playback state on streaming start = %d, want 1", len(got))
	}
}
