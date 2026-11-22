package engine

import (
	"slices"
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

	h.e.sessionLost(b.ID, false)
	if h.sessions != before+1 {
		t.Fatalf("sessions = %d, want one rebuild without %s", h.sessions, b.ID)
	}
	if got := h.e.Snapshot().ActiveTargets; len(got) != 1 || got[0] != testDeviceID {
		t.Fatalf("active targets = %v, want only the surviving target", got)
	}
	if !h.e.plan.live() || len(h.e.plan.targets) != 1 || h.e.plan.keys()[0] != testDeviceID {
		t.Fatalf("plan = %v, want only the surviving target", h.e.plan.keys())
	}
	if h.e.Snapshot().Reconnecting {
		t.Fatal("the survivor rebuild armed a retry; the session continues")
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

	h.e.sessionLost(testDeviceID, false)
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

	h.e.sessionLost(testDeviceID, false)
	if h.e.tr != nil || h.e.plan.live() {
		t.Fatalf("session survived its only target: tr=%v plan=%v", h.e.tr, h.e.plan.live())
	}
	if got := h.e.Snapshot().ActiveTargets; len(got) != 1 || got[0] != testDeviceID {
		t.Fatalf("active targets = %v, want the target kept for the retry", got)
	}
}

// TestMemberDeathPublishesANotice pins the flyout's explanation: a member that
// leaves while the rest continue is named in the snapshot, and the next user
// connect clears it.
func TestMemberDeathPublishesANotice(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()

	h.e.sessionLost(b.ID, false)
	snap := h.e.Snapshot()
	n := snap.Notice
	if n.Kind != NoticeMemberLost || n.DeviceID != b.ID || n.Remaining != 1 {
		t.Fatalf("notice = %+v, want the lost target and one survivor", n)
	}
	if n.Name == "" {
		t.Fatal("notice has no device name")
	}

	// A user action takes responsibility for the session, so the explanation
	// goes away.
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	if got := h.e.Snapshot().Notice; got.Kind != NoticeNone {
		t.Fatalf("notice survived a user connect: %+v", got)
	}
}

// TestMemberLostWithUnbuildableSurvivorsDropsLostKey pins the rebuild-failure
// branch: when the survivors cannot form a plan, the lost key must still leave
// the wanted set so no retry fights the receiver that took it over.
func TestMemberLostWithUnbuildableSurvivorsDropsLostKey(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()

	// Add a phantom key so buildPlan(remaining) fails on it.
	h.e.life.wantedKeys = append(h.e.life.wantedKeys, "phantom-key")
	h.e.sessionLost(b.ID, false)

	if slices.Contains(h.e.life.wantedKeys, b.ID) {
		t.Fatalf("lost key %s stayed in wanted = %v", b.ID, h.e.life.wantedKeys)
	}
}
