package sender

import (
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/audio/ring"
)

// Capture runs from app launch, so the ring holds seconds of audio that was
// heard on the PC before the user connected. A new session must start at the
// live edge, not replay that backlog.
func TestStreamingStartsFromTheNewestCapture(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb := ring.New(48000 * 2 * 4)
	stale := make([]int16, 48000*2*3)
	for i := range stale {
		stale[i] = 1111
	}
	for i := len(stale) - 48000/10*2; i < len(stale); i++ {
		stale[i] = 2222
	}
	rb.Push(stale)
	r.sender.AttachRing(rb)
	r.sender.SetInputFormat(48000)
	r.start()
	requireLaunched(t, r)

	cushion := int(DefaultStartCushion.Seconds()*48000) * 2
	want := stale[len(stale)-cushion]

	fresh := make([]int16, 48000*2)
	for i := range fresh {
		fresh[i] = -1234
	}
	rb.Push(fresh)

	r.advance(300 * time.Millisecond)
	if len(r.rx.audioFrames) == 0 {
		t.Fatal("no audio sent")
	}
	first := r.rx.audioFrames[0]
	if first == stale[0] {
		t.Fatalf("first sample = %d, replaying the oldest pre-connect backlog", first)
	}
	if first != want {
		t.Fatalf("first sample = %d, want the newest cushion boundary %d", first, want)
	}
	for i, v := range r.rx.audioFrames {
		if v == 1111 {
			t.Fatalf("sample %d = 1111, stale backlog survived the session start", i)
		}
	}
	foundFresh := false
	for i, v := range r.rx.audioFrames {
		if v == -1234 {
			foundFresh = true
			if i > 48000/10*2 {
				t.Fatalf("fresh audio at sample %d, more than the startup cushion behind", i)
			}
			break
		}
	}
	if !foundFresh {
		t.Fatal("fresh capture never reached the receiver")
	}
	// A start trim is policy, not capture overflow.
	if rb.Dropped() != 0 {
		t.Fatalf("start trim counted as a capture drop: %d", rb.Dropped())
	}
}
