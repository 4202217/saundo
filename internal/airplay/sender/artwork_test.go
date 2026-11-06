package sender

import (
	"bytes"
	"strings"
	"testing"

	"saundo/internal/airplay/device"
)

// coverRequests returns the SET_PARAMETER image bodies the fake receiver saw.
func coverRequests(r *rig) []string {
	var out []string
	for _, p := range r.rx.setParams {
		ct, body, ok := strings.Cut(p, "\x00")
		if ok && ct == "image/jpeg" {
			out = append(out, body)
		}
	}
	return out
}

// TestSetLampArtworkSendsCover pins the top-display path. The image becomes
// the Now Playing cover and the receiver repaints from it; the separate
// artwork POST was removed after a cover-only HomePod test.
func TestSetLampArtworkSendsCover(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	image := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0xff, 0xd9}

	r.sender.SetLampArtwork(image, "image/jpeg", true)
	r.step()

	covers := coverRequests(r)
	if len(covers) != 1 {
		t.Fatalf("cover requests = %d, want 1", len(covers))
	}
	if !bytes.Equal([]byte(covers[0]), image) {
		t.Fatalf("cover = % x, want % x", covers[0], image)
	}
}

func TestSetLampArtworkWaitsForStreaming(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	image := []byte{0xff, 0xd8, 0xff, 0xd9}
	r.sender.SetLampArtwork(image, "image/jpeg", true)
	r.step()
	if got := coverRequests(r); len(got) != 0 {
		t.Fatalf("cover sent before streaming: %v", got)
	}

	r.start()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("cover on streaming start = %d, want 1", len(got))
	}
}

func TestSetLampArtworkDuplicateIsNoOp(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	image := []byte{0xff, 0xd8, 0xff, 0xd9}

	r.sender.SetLampArtwork(image, "image/jpeg", true)
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("first cover = %d, want 1", len(got))
	}

	r.sender.SetLampArtwork(append([]byte(nil), image...), "image/jpeg", true)
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("duplicate cover = %d, want 1", len(got))
	}
}

func TestSetLampArtworkDoesNotTouchDMAPText(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.sender.SetNowPlaying("Title", "Artist", "Album", nil, "")
	r.step()
	dmapBefore := 0
	for _, p := range r.rx.setParams {
		if strings.HasPrefix(p, "application/x-dmap-tagged") {
			dmapBefore++
		}
	}
	if dmapBefore == 0 {
		t.Fatal("precondition: no DMAP request was sent")
	}

	r.sender.SetLampArtwork([]byte{0xff, 0xd8, 0xff, 0xd9}, "image/jpeg", true)
	r.step()
	dmapAfter := 0
	for _, p := range r.rx.setParams {
		if strings.HasPrefix(p, "application/x-dmap-tagged") {
			dmapAfter++
		}
	}
	if dmapAfter != dmapBefore {
		t.Fatalf("artwork resent DMAP text: %d -> %d", dmapBefore, dmapAfter)
	}
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("cover requests = %d, want 1", len(got))
	}
}

func TestSetLampArtworkClearSendsNothing(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	r.sender.SetLampArtwork([]byte{0xff, 0xd8, 0xff, 0xd9}, "image/jpeg", true)
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("first cover = %d, want 1", len(got))
	}

	r.sender.SetLampArtwork(nil, "", false)
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("clear sent a cover: %d requests", len(got))
	}
}

// TestMediaCoverWinsWhenLampIsOff pins the precedence: the ambient light owns
// the cover slot while on; while off, the receiver track-info cover wins and
// the origin image is only the fallback.
func TestMediaCoverWinsWhenLampIsOff(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	media := []byte{0xff, 0xd8, 0xff, 0xe0, 0x01, 0xff, 0xd9}
	origin := []byte{0xff, 0xd8, 0xff, 0xe0, 0x02, 0xff, 0xd9}

	r.sender.SetNowPlaying("Title", "", "", media, "image/jpeg")
	r.step()
	got := coverRequests(r)
	if len(got) != 1 || !bytes.Equal([]byte(got[0]), media) {
		t.Fatalf("media cover = %d % x, want the image", len(got), got)
	}

	// Turning the lamp off with the origin image leaves the media cover in
	// the slot.
	r.sender.SetLampArtwork(origin, "image/jpeg", false)
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("origin replaced the media cover: %d requests", len(got))
	}

	// Clearing the media cover falls back to the origin image.
	r.sender.SetNowPlaying("", "", "", nil, "")
	r.step()
	got = coverRequests(r)
	if len(got) != 2 || !bytes.Equal([]byte(got[1]), origin) {
		t.Fatalf("fallback = %d % x, want origin", len(got), got)
	}
}

// TestLampCoverWinsWhileOn pins the other half: with the light on, a new
// receiver track-info cover must not replace the light.
func TestLampCoverWinsWhileOn(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	media := []byte{0xff, 0xd8, 0xff, 0xe0, 0x01, 0xff, 0xd9}
	lampImage := []byte{0xff, 0xd8, 0xff, 0xe0, 0x02, 0xff, 0xd9}
	media2 := []byte{0xff, 0xd8, 0xff, 0xe0, 0x03, 0xff, 0xd9}

	r.sender.SetNowPlaying("One", "", "", media, "image/jpeg")
	r.step()
	r.sender.SetLampArtwork(lampImage, "image/jpeg", true)
	r.step()
	if got := coverRequests(r); len(got) != 2 || !bytes.Equal([]byte(got[1]), lampImage) {
		t.Fatalf("light did not take the slot: %d % x", len(got), got)
	}

	r.sender.SetNowPlaying("Two", "", "", media2, "image/jpeg")
	r.step()
	got := coverRequests(r)
	if len(got) != 2 || !bytes.Equal([]byte(got[1]), lampImage) {
		t.Fatalf("media cover replaced the light: %d % x", len(got), got)
	}
}
