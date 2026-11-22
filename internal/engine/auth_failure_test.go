package engine

import (
	"testing"

	"saundo/internal/airplay/sender"
)

// TestRejectedPasswordPromptsInsteadOfRetrying pins the auth failure path: a
// receiver that refuses its saved password is dropped from the session, the
// survivors keep playing, and the UI gets a password prompt instead of a retry
// ladder.
func TestRejectedPasswordPromptsInsteadOfRetrying(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()
	before := h.sessions

	h.fake.failure = LaunchFailure{TargetKey: b.ID, DeviceID: b.ID, Name: b.Name, Auth: true}
	h.fake.launch(false, "The password was not accepted by the device")
	snap := h.e.Snapshot()
	if snap.ErrorKey != ErrPasswordRejected {
		t.Fatalf("error key = %q, want %q (error %q)", snap.ErrorKey, ErrPasswordRejected, snap.Error)
	}
	if !snap.PasswordRequired || snap.PasswordDeviceID != b.ID || snap.PasswordTargetID != b.ID {
		t.Fatalf("password prompt = %+v", snap)
	}
	if !h.e.life.reconnectAt.IsZero() {
		t.Fatalf("a retry was scheduled for %v; a rejected password cannot fix itself", h.e.life.reconnectAt)
	}

	// The dead arm drops the refused member and keeps the survivor.
	h.e.sessionLost(b.ID, true)
	if h.sessions != before+1 {
		t.Fatalf("sessions = %d, want one rebuild without %s", h.sessions, b.ID)
	}
	if len(h.e.life.wantedKeys) != 1 || h.e.life.wantedKeys[0] != testDeviceID {
		t.Fatalf("wanted = %v, want only the survivor", h.e.life.wantedKeys)
	}
	if got := h.e.Snapshot().Notice; got.Kind != NoticeNone {
		t.Fatalf("an auth failure published a lost notice: %+v", got)
	}
	h.launch()
	if got := h.e.Snapshot().State; got != sender.StateStreaming {
		t.Fatalf("survivor state = %v, want streaming", got)
	}
}

// TestRejectedPasswordOnTheOnlyTargetDoesNotRetry pins the single-target path:
// the prompt replaces the retry ladder and the wanted target stays for a later
// manual connect.
func TestRejectedPasswordOnTheOnlyTargetDoesNotRetry(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.fake.failure = LaunchFailure{TargetKey: testDeviceID, DeviceID: testDeviceID, Name: "客厅 HomePod", Auth: true}
	h.fake.launch(false, "The password was not accepted by the device")

	if !h.e.life.reconnectAt.IsZero() {
		t.Fatalf("a retry was scheduled for %v", h.e.life.reconnectAt)
	}
	if !h.e.Snapshot().PasswordRequired {
		t.Fatal("the rejected password did not prompt")
	}
	h.e.sessionLost(testDeviceID, true)
	if len(h.e.life.wantedKeys) != 1 || h.e.life.wantedKeys[0] != testDeviceID {
		t.Fatalf("wanted = %v, want the target kept for a manual connect", h.e.life.wantedKeys)
	}
	if len(h.e.Snapshot().ActiveTargets) != 1 {
		t.Fatalf("active targets = %v", h.e.Snapshot().ActiveTargets)
	}
}
