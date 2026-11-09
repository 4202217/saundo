package sender

import (
	"bytes"
	"testing"
	"time"

	"saundo/internal/airplay/device"
)

// TestHomePodPasswordKeepsHomePodProfile pins the family split. A HomePod that
// gates AirPlay with a password still uses the HomePod key and latency profile;
// only the pairing method changes. A Mac uses the Events-Write key, the wide
// latency range, and the Mac request headers.
func TestHomePodPasswordKeepsHomePodProfile(t *testing.T) {
	r := newRig(t, modePassword, device.AuthHAPPassword, true, nil)
	r.sender.SetFamily(device.FamilyHomePod)
	r.sender.SetAuth(device.AuthHAPPassword, true, "AA:BB:CC:DD:EE:FF", "", testReceiverPassword)
	r.sender.SetLatency(200)
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	if len(r.sender.ap2.sharedSecret) < 32 {
		t.Fatalf("shared secret = %d bytes, want at least 32", len(r.sender.ap2.sharedSecret))
	}
	wantKey := r.sender.ap2.sharedSecret[:32]
	if !bytes.Equal(r.rx.audioKey, wantKey) {
		t.Fatal("HomePod shk must be the first 32 bytes of the shared secret")
	}
	if r.sender.nextLatencySamples != 8820 {
		t.Fatalf("HomePod latency = %d samples, want 8820", r.sender.nextLatencySamples)
	}

	streams, ok := r.rx.streamSetup["streams"].([]any)
	if !ok || len(streams) == 0 {
		t.Fatalf("stream SETUP carried no streams: %#v", r.rx.streamSetup)
	}
	stream, ok := streams[0].(map[string]any)
	if !ok {
		t.Fatalf("stream SETUP entry is %T", streams[0])
	}
	if got := plistInt(stream, "latencyMin"); got != 8820 {
		t.Fatalf("latencyMin = %d, want 8820", got)
	}
	if got := plistInt(stream, "latencyMax"); got != 8820 {
		t.Fatalf("latencyMax = %d, want 8820", got)
	}

	pair := lastRequest(t, r, "POST", "/pair-setup")
	if got := pair.headers["x-apple-hkp"]; got != "4" {
		t.Fatalf("pair-setup X-Apple-HKP = %q, want 4", got)
	}
	setup := lastRequest(t, r, "SETUP", "")
	if got := setup.headers["x-apple-streamid"]; got != "1" {
		t.Fatalf("X-Apple-StreamID = %q, want 1", got)
	}
	if got := setup.headers["x-apple-stream-id"]; got != "" {
		t.Fatalf("HomePod must not send the Mac header X-Apple-Stream-ID: %q", got)
	}
	if got := setup.headers["x-apple-protocolversion"]; got != "" {
		t.Fatalf("HomePod must not send X-Apple-ProtocolVersion: %q", got)
	}
	if got := setup.headers["x-apple-session-id"]; got != "" {
		t.Fatalf("HomePod must not send X-Apple-Session-ID: %q", got)
	}

	rb.Reset()
	rb.Push(src)
	r.advance(300 * time.Millisecond)
	requireAudioMatches(t, r, src)
}

func lastRequest(t *testing.T, r *rig, method, uri string) request {
	t.Helper()
	for i := len(r.rx.seen) - 1; i >= 0; i-- {
		req := r.rx.seen[i]
		if req.method == method && (uri == "" || req.uri == uri) {
			return req
		}
	}
	t.Fatalf("no %s %s request", method, uri)
	return request{}
}
