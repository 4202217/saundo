package sender

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/device"
)

// TestHAPPasswordPairsAndStreams drives the password-protected macOS
// receiver: HAP pair-setup keyed by the stored receiver password on one
// connection, no transient Flags, no /pair-pin-start, no PIN wait, then AP2
// streaming and a credentials-only reconnect.
func TestHAPPasswordPairsAndStreams(t *testing.T) {
	r := newRig(t, modePassword, device.AuthHAPPassword, true, nil)
	r.sender.SetAuth(device.AuthHAPPassword, true, "AA:BB:CC:DD:EE:FF", "", testReceiverPassword)
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	if r.rx.requests["POST /pair-pin-start"] != 0 {
		t.Fatal("password pairing must not call /pair-pin-start")
	}
	if len(r.pins) != 0 || r.sender.WaitingForPin() {
		t.Fatal("password pairing must not wait for a PIN")
	}
	if len(r.creds) != 1 {
		t.Fatalf("credentials obtained = %d, want 1", len(r.creds))
	}
	if len(r.rx.audioKey) != 32 {
		t.Fatalf("shk = %d bytes, want 32", len(r.rx.audioKey))
	}
	if len(r.sender.ap2.eventIn) != 32 || !bytes.Equal(r.rx.audioKey, r.sender.ap2.eventIn) {
		t.Fatal("macOS shk must be the Events-Write key")
	}
	if r.sender.nextLatencySamples != 66150 {
		t.Fatalf("macOS latency = %d samples, want 66150", r.sender.nextLatencySamples)
	}
	streams, ok := r.rx.streamSetup["streams"].([]any)
	if !ok || len(streams) == 0 {
		t.Fatalf("stream SETUP carried no streams: %#v", r.rx.streamSetup)
	}
	stream, ok := streams[0].(map[string]any)
	if !ok {
		t.Fatalf("stream SETUP entry is %T", streams[0])
	}
	if got := plistInt(stream, "latencyMin"); got != 11025 {
		t.Fatalf("latencyMin = %d, want 11025", got)
	}
	if got := plistInt(stream, "latencyMax"); got != 88200 {
		t.Fatalf("latencyMax = %d, want 88200", got)
	}
	if got := plistInt(stream, "audioFormat"); got != 0x40000 {
		t.Fatalf("audioFormat = %#x, want ALAC 0x40000", got)
	}
	if got := plistInt(stream, "ct"); got != 2 {
		t.Fatalf("ct = %d, want ALAC 2", got)
	}
	rb.Reset()
	rb.Push(src)
	r.advance(300 * time.Millisecond)
	requireAudioMatches(t, r, src)

	// A reconnect carries the stored pairing and skips pair-setup entirely.
	stored := &storedPairing{
		clientID:      r.rx.clientID,
		clientLTPK:    r.rx.clientLTPK,
		accessoryID:   r.rx.accessoryID,
		accessoryLTPK: r.rx.accessoryLTPK,
		accessorySeed: r.rx.accessorySeed,
	}
	r2 := newRig(t, modePassword, device.AuthHAPPassword, true, stored)
	r2.sender.SetAuth(device.AuthHAPPassword, true, "AA:BB:CC:DD:EE:FF", r.creds[0], testReceiverPassword)
	rb2, src2 := fillRing(44100)
	r2.sender.AttachRing(rb2)
	r2.start()
	requireLaunched(t, r2)
	if r2.rx.requests["POST /pair-setup"] != 0 {
		t.Fatalf("reconnect ran pair-setup %d times", r2.rx.requests["POST /pair-setup"])
	}
	if r2.rx.requests["POST /pair-verify"] < 2 {
		t.Fatalf("reconnect pair-verify requests = %d", r2.rx.requests["POST /pair-verify"])
	}
	rb2.Reset()
	rb2.Push(src2)
	r2.advance(300 * time.Millisecond)
	requireAudioMatches(t, r2, src2)
}

// TestHAPPasswordWrongPasswordFailsWithoutPrompt proves a rejected password
// surfaces as a launch failure naming the password, never a PIN prompt.
func TestHAPPasswordWrongPasswordFailsWithoutPrompt(t *testing.T) {
	r := newRig(t, modePassword, device.AuthHAPPassword, true, nil)
	r.sender.SetAuth(device.AuthHAPPassword, true, "AA:BB:CC:DD:EE:FF", "", "not-the-password")
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	if len(r.launched) != 1 || r.launched[0].ok {
		t.Fatalf("launched = %+v, want one failure", r.launched)
	}
	if !strings.Contains(r.launched[0].err, "password") {
		t.Fatalf("error = %q, want a password rejection", r.launched[0].err)
	}
	if len(r.pins) != 0 || r.sender.WaitingForPin() {
		t.Fatal("a wrong password must not prompt for a PIN")
	}
}

// TestHAPPasswordRequiredFailsBeforePairing proves the sender refuses to
// start an HAP password session with no password stored.
func TestHAPPasswordRequiredFailsBeforePairing(t *testing.T) {
	r := newRig(t, modePassword, device.AuthHAPPassword, true, nil)
	r.sender.SetAuth(device.AuthHAPPassword, true, "AA:BB:CC:DD:EE:FF", "", "")
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	if len(r.launched) != 1 || r.launched[0].ok {
		t.Fatalf("launched = %+v, want one failure", r.launched)
	}
	if !strings.Contains(r.launched[0].err, "password") {
		t.Fatalf("error = %q, want a password-required hint", r.launched[0].err)
	}
	if r.rx.requests["POST /pair-setup"] != 0 {
		t.Fatal("pairing started without a password")
	}
}
