package engine

import (
	"testing"

	"saundo/internal/airplay/sender"
)

// TestMemberDeathKeepsOthersPlaying pins the isolation: when one target's
// member closes, the rest rebuild without it and keep streaming, and the lost
// target leaves the wanted set so no retry drags the survivors down.
func TestMemberDeathKeepsOthersPlaying(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want the add's rebuild", h.sessions)
	}
	h.launch()
	before := h.sessions

	h.e.sessionLost(b.ID)
	if h.sessions != before+1 {
		t.Fatalf("sessions = %d, want one rebuild without %s", h.sessions, b.ID)
	}
	if len(h.e.wantedKeys) != 1 || h.e.wantedKeys[0] != testDeviceID {
		t.Fatalf("wanted = %v, want only the surviving target", h.e.wantedKeys)
	}
	if !h.e.plan.live() || len(h.e.plan.targets) != 1 || h.e.plan.keys()[0] != testDeviceID {
		t.Fatalf("plan = %v, want only the surviving target", h.e.plan.keys())
	}
	if !h.e.reconnectAt.IsZero() {
		t.Fatalf("a reconnect is still scheduled for %v; the session continues", h.e.reconnectAt)
	}
	h.launch()
	snap := h.e.Snapshot()
	if snap.State != sender.StateStreaming || len(snap.ActiveTargets) != 1 || snap.ActiveTargets[0] != testDeviceID {
		t.Fatalf("survivor state = %v targets = %v", snap.State, snap.ActiveTargets)
	}
}

// TestPrimaryDeathPromotesTheRemainingTarget pins the identity handover: the
// survivor becomes the primary when the old primary's member closes.
func TestPrimaryDeathPromotesTheRemainingTarget(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()

	h.e.sessionLost(testDeviceID)
	snap := h.e.Snapshot()
	if snap.SelectedID != b.ID {
		t.Fatalf("primary = %q, want the surviving target %q", snap.SelectedID, b.ID)
	}
	if len(snap.ActiveTargets) != 1 || snap.ActiveTargets[0] != b.ID {
		t.Fatalf("targets = %v, want only %s", snap.ActiveTargets, b.ID)
	}
}

// TestMemberDeathOfTheOnlyTargetKeepsTheRetry pins the single-target path: the
// session closes and the wanted set stays for the automatic retry.
func TestMemberDeathOfTheOnlyTargetKeepsTheRetry(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()

	h.e.sessionLost(testDeviceID)
	if h.e.tr != nil || h.e.plan.live() {
		t.Fatalf("session survived its only target: tr=%v plan=%v", h.e.tr, h.e.plan.live())
	}
	if len(h.e.wantedKeys) != 1 || h.e.wantedKeys[0] != testDeviceID {
		t.Fatalf("wanted = %v, want the target kept for the retry", h.e.wantedKeys)
	}
}
