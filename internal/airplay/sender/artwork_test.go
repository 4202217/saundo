package sender

import (
	"bytes"
	"strings"
	"testing"

	"saundo/internal/airplay/device"
)

func imageRequests(r *rig) []string {
	return r.rx.artworks
}

// SetArtwork must push the image as one RTSP POST on the encrypted control
// channel, with a raw JPEG body, the way the reference sender does.
func TestSetArtworkSendsArtworkCommand(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.artworks = nil
	image := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0xff, 0xd9}

	r.sender.SetArtwork(image, "image/jpeg")
	r.step()

	got := imageRequests(r)
	if len(got) != 1 {
		t.Fatalf("artwork requests = %d, want 1", len(got))
	}
	ct, body, ok := strings.Cut(got[0], "\x00")
	if !ok {
		t.Fatalf("artwork capture missing content-type separator: %q", got[0][:min(40, len(got[0]))])
	}
	if ct != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", ct)
	}
	if !bytes.Equal([]byte(body), image) {
		t.Fatalf("image = % x, want % x", body, image)
	}
}

func TestSetArtworkWaitsForStreaming(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	image := []byte{0xff, 0xd8, 0xff, 0xd9}
	r.sender.SetArtwork(image, "image/jpeg")
	r.step()
	if got := imageRequests(r); len(got) != 0 {
		t.Fatalf("artwork sent before streaming: %v", got)
	}

	r.start()
	if got := imageRequests(r); len(got) != 1 {
		t.Fatalf("artwork sent on streaming start = %d, want 1", len(got))
	}
}

func TestSetArtworkDuplicateIsNoOp(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	image := []byte{0xff, 0xd8, 0xff, 0xd9}

	r.sender.SetArtwork(image, "image/jpeg")
	r.step()
	if got := imageRequests(r); len(got) != 1 {
		t.Fatalf("first send = %d, want 1", len(got))
	}
	r.sender.SetArtwork(append([]byte(nil), image...), "image/jpeg")
	r.step()
	if got := imageRequests(r); len(got) != 1 {
		t.Fatalf("duplicate send = %d, want 1", len(got))
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
	if got := imageRequests(r); len(got) != 1 {
		t.Fatalf("image requests = %d, want 1", len(got))
	}
}

func TestSetArtworkClearSendsNothing(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.setParams = nil
	r.sender.SetArtwork([]byte{0xff, 0xd8, 0xff, 0xd9}, "image/jpeg")
	r.step()
	if got := imageRequests(r); len(got) != 1 {
		t.Fatalf("first send = %d, want 1", len(got))
	}

	r.sender.SetArtwork(nil, "")
	r.step()
	if got := imageRequests(r); len(got) != 1 {
		t.Fatalf("clear sent an image: %d requests", len(got))
	}
}
