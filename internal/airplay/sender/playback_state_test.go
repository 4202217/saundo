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

// TestPairSessionSendsPlayingStateBeforeTheFirstCover pins the follower's
// renderer start. A pair member sends an explicit state 1 at stream start,
// before the first cover, instead of assuming playing.
func TestPairSessionSendsPlayingStateBeforeTheFirstCover(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.sender.SetGroupUUID("1602DA4F-8C8F-58E4-A360-F9F2D0480FC5")
	image := []byte{0xff, 0xd8, 0xff, 0xe0, 0x11, 0xff, 0xd9}
	r.sender.SetLampArtwork(image, "image/jpeg", true)
	r.start()

	command, cover := -1, -1
	for i, req := range r.rx.seen {
		if command < 0 && req.method == "POST" && req.uri == "/command" {
			command = i
		}
		if cover < 0 && req.method == "SET_PARAMETER" && req.headers["content-type"] == "image/jpeg" {
			cover = i
		}
	}
	if command < 0 {
		t.Fatal("pair member sent no state 1 at stream start")
	}
	if cover < 0 {
		t.Fatal("pair member sent no cover")
	}
	if command > cover {
		t.Fatalf("state command at %d came after the cover at %d", command, cover)
	}
	got := commandRequests(r)
	if len(got) != 1 || !bytes.Equal([]byte(got[0]), playbackBody(t, 1)) {
		t.Fatalf("state commands = %d % x, want one playing state", len(got), got)
	}
}

// TestPairSessionKeepsAPreStartPause pins that an explicit paused state set
// before the stream starts is not replaced by the pair's default state 1.
func TestPairSessionKeepsAPreStartPause(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.sender.SetGroupUUID("1602DA4F-8C8F-58E4-A360-F9F2D0480FC5")
	r.sender.SetPlaybackState(false)
	r.step()
	r.start()

	got := commandRequests(r)
	if len(got) != 1 || !bytes.Equal([]byte(got[0]), playbackBody(t, 2)) {
		t.Fatalf("state commands = %d % x, want one paused state", len(got), got)
	}
}

// TestStandaloneSessionStillAssumesPlaying pins the scope: a single receiver
// keeps the image-only first light.
func TestStandaloneSessionStillAssumesPlaying(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	image := []byte{0xff, 0xd8, 0xff, 0xe0, 0x12, 0xff, 0xd9}
	r.sender.SetLampArtwork(image, "image/jpeg", true)
	r.start()
	if got := commandRequests(r); len(got) != 0 {
		t.Fatalf("standalone sent state at stream start: %v", got)
	}
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("standalone covers = %d, want 1", len(got))
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
