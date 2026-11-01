package sender

import (
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
)

// TestAP2DigestChallengesSessionSetup drives the Mac's password challenge on
// the encrypted AP2 session SETUP. The first attempt is refused with 401; the
// replay carries a valid RFC 2069 Authorization for the AirPlay realm, and the
// session reaches streaming. Without the replay the 401 fails the session.
func TestAP2DigestChallengesSessionSetup(t *testing.T) {
	r := newRig(t, modePassword, device.AuthHAPPassword, true, nil)
	r.sender.SetAuth(device.AuthHAPPassword, true, "AA:BB:CC:DD:EE:FF", "", testReceiverPassword)
	r.rx.digestChallenge = true
	r.rx.digestNonce = "0123456789abcdef"
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	if r.rx.digestChallenges != 1 {
		t.Fatalf("401 challenges = %d, want 1", r.rx.digestChallenges)
	}
	uri := r.sender.rtspURI()
	want := crypto.DigestAuthResponse("SETUP", uri, "AirPlay", "airplay", testReceiverPassword, r.rx.digestNonce)
	if r.rx.digestAuth != want {
		t.Fatalf("Authorization = %q, want %q", r.rx.digestAuth, want)
	}
	if !strings.Contains(r.rx.digestAuth, `username="AirPlay"`) {
		t.Fatalf("Authorization username is not AirPlay: %q", r.rx.digestAuth)
	}
	if !strings.Contains(r.rx.digestAuth, `uri="`+uri+`"`) {
		t.Fatalf("Authorization uri = %q, want %q", r.rx.digestAuth, uri)
	}
	// The refused session SETUP, its replay, then the stream SETUP.
	if got := r.rx.requests["SETUP "+uri]; got != 3 {
		t.Fatalf("SETUP requests = %d, want 3", got)
	}

	rb.Reset()
	rb.Push(src)
	r.advance(300 * time.Millisecond)
	requireAudioMatches(t, r, src)
}
