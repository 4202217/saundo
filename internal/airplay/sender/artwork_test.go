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

// TestSetArtworkSendsCover pins the top-display path. The image becomes the
// Now Playing cover and the receiver repaints from it; the separate artwork
// POST was removed after a cover-only HomePod test.
func TestSetArtworkSendsCover(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	image := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0xff, 0xd9}

	r.sender.SetArtwork(image, "image/jpeg")
	r.step()

	covers := coverRequests(r)
	if len(covers) != 1 {
		t.Fatalf("cover requests = %d, want 1", len(covers))
	}
	if !bytes.Equal([]byte(covers[0]), image) {
		t.Fatalf("cover = % x, want % x", covers[0], image)
	}
}

func TestSetArtworkWaitsForStreaming(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	image := []byte{0xff, 0xd8, 0xff, 0xd9}
	r.sender.SetArtwork(image, "image/jpeg")
	r.step()
	if got := coverRequests(r); len(got) != 0 {
		t.Fatalf("cover sent before streaming: %v", got)
	}

	r.start()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("cover on streaming start = %d, want 1", len(got))
	}
}

func TestSetArtworkDuplicateIsNoOp(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	image := []byte{0xff, 0xd8, 0xff, 0xd9}

	r.sender.SetArtwork(image, "image/jpeg")
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("first cover = %d, want 1", len(got))
	}

	r.sender.SetArtwork(append([]byte(nil), image...), "image/jpeg")
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("duplicate cover = %d, want 1", len(got))
	}
}

func TestSetArtworkDoesNotTouchDMAPText(t *testing.T) {
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

	r.sender.SetArtwork([]byte{0xff, 0xd8, 0xff, 0xd9}, "image/jpeg")
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

func TestSetArtworkClearSendsNothing(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	r.sender.SetArtwork([]byte{0xff, 0xd8, 0xff, 0xd9}, "image/jpeg")
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("first cover = %d, want 1", len(got))
	}

	r.sender.SetArtwork(nil, "")
	r.step()
	if got := coverRequests(r); len(got) != 1 {
		t.Fatalf("clear sent a cover: %d requests", len(got))
	}
}
