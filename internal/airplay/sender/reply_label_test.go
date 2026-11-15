package sender

import (
	"testing"

	"saundo/internal/airplay/device"
)

// TestPendingRequestLabelNamesTheArtwork pins the reply-log label: a cover
// reply names its content type and size, and a command reply names its path,
// so the two SET_PARAMETER families are distinguishable in the log.
func TestPendingRequestLabelNamesTheArtwork(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.sender.sendRequest("SET_PARAMETER", "rtsp://192.0.2.10/123", "image/jpeg", "abcdef", nil)
	if got := r.sender.pending[0].describe(); got != "SET_PARAMETER image/jpeg (6 B)" {
		t.Fatalf("artwork label = %q", got)
	}
	r.sender.sendRequest("POST", "/command", "application/x-apple-binary-plist", "xy", nil)
	if got := r.sender.pending[1].describe(); got != "POST /command application/x-apple-binary-plist (2 B)" {
		t.Fatalf("command label = %q", got)
	}
}
