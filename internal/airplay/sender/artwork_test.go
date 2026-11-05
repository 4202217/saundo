package sender

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
)

func imageRequests(r *rig) []string {
	return r.rx.artworks
}

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
	root, err := crypto.Decode([]byte(got[0]))
	if err != nil {
		t.Fatalf("artwork body is not a plist: %v", err)
	}
	m, _ := root.(map[string]any)
	dataURL, _ := m["jpeg"].(string)
	const prefix = "data:image/jpeg;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		t.Fatalf("jpeg = %q, want a %s data URL", dataURL, prefix)
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[len(prefix):])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, image) {
		t.Fatalf("image = % x, want % x", raw, image)
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
