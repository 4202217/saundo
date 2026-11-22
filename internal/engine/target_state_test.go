package engine

import (
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
)

// targetStateOf reads one active target's published role, failing when the
// snapshot has no entry for it.
func targetStateOf(t *testing.T, snap Snapshot, id string) TargetState {
	t.Helper()
	tc, ok := snap.Target(id)
	if !ok {
		t.Fatalf("no target control for %s; targets: %+v", id, snap.Targets)
	}
	return tc.State
}

// TestConnectPublishesJoiningThenMember pins the first connect: the row the
// click started is the joiner until the cohort streams.
func TestConnectPublishesJoiningThenMember(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	if got := targetStateOf(t, h.e.Snapshot(), testDeviceID); got != TargetJoining {
		t.Fatalf("opening state = %v, want joining", got)
	}
	h.launch()
	if got := targetStateOf(t, h.e.Snapshot(), testDeviceID); got != TargetMember {
		t.Fatalf("streaming state = %v, want member", got)
	}
}

// TestAddPublishesMemberAndJoining pins the add: the cohort's established
// target stays a member while the added target joins.
func TestAddPublishesMemberAndJoining(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})

	snap := h.e.Snapshot()
	if got := targetStateOf(t, snap, testDeviceID); got != TargetMember {
		t.Fatalf("established state = %v, want member", got)
	}
	if got := targetStateOf(t, snap, b.ID); got != TargetJoining {
		t.Fatalf("added state = %v, want joining", got)
	}
	h.launch()
	snap = h.e.Snapshot()
	if targetStateOf(t, snap, testDeviceID) != TargetMember || targetStateOf(t, snap, b.ID) != TargetMember {
		t.Fatalf("after launch states = %+v, want two members", snap.Targets)
	}
}

// TestRejectedAddKeepsMembers pins the pre-open rejection: the live cohort is
// untouched, its rows stay members, and the refused device has no row state.
func TestRejectedAddKeepsMembers(t *testing.T) {
	h := newHarness(t)
	b := secondDevice()
	b.Local = true
	h.e.setDevices([]device.Device{onlineDevice(), b})
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})

	snap := h.e.Snapshot()
	if got := targetStateOf(t, snap, testDeviceID); got != TargetMember {
		t.Fatalf("live state = %v, want member after a refused add", got)
	}
	if _, ok := snap.Target(b.ID); ok {
		t.Fatalf("the refused device has a row state: %+v", snap.Targets)
	}
	if snap.State != sender.StateStreaming {
		t.Fatalf("state = %v, want the cohort still streaming", snap.State)
	}
}

// TestCancelJoiningLeavesMember pins the cancel: the joining target leaves and
// the remaining row is a member, never a joiner.
func TestCancelJoiningLeavesMember(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.e.handleCommand(command{kind: cmdDisconnectTarget, deviceID: b.ID})

	snap := h.e.Snapshot()
	if got := targetStateOf(t, snap, testDeviceID); got != TargetMember {
		t.Fatalf("remaining state = %v, want member", got)
	}
	if _, ok := snap.Target(b.ID); ok {
		t.Fatalf("the canceled target kept a state: %+v", snap.Targets)
	}
}

// TestLeaveAndLatencyRebuildsKeepMembers pins the rebuilds: a per-target leave
// and a latency change re-open the cohort, and every remaining row is a member
// so no row offers cancel.
func TestLeaveAndLatencyRebuildsKeepMembers(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()

	h.e.handleCommand(command{kind: cmdDisconnectTarget, deviceID: testDeviceID})
	if got := targetStateOf(t, h.e.Snapshot(), b.ID); got != TargetMember {
		t.Fatalf("after leave state = %v, want member", got)
	}

	h.e.handleCommand(command{kind: cmdLatency, value: 700, reconnect: true})
	snap := h.e.Snapshot()
	if got := targetStateOf(t, snap, b.ID); got != TargetMember {
		t.Fatalf("after latency state = %v, want member", got)
	}
}

// TestFailedAttemptPublishesRetryOrReopening pins the states with no live
// cohort: a retry scheduled by the engine publishes reopening, and with
// reconnect off the same failure publishes failed so every wanted row offers
// retry. The close stands in for the dead-transport teardown the harness
// transport never signals.
func TestFailedAttemptPublishesRetryOrReopening(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.fake.launch(false, "no answer")
	h.e.closeSession(false)
	h.e.publish()
	if got := targetStateOf(t, h.e.Snapshot(), testDeviceID); got != TargetReopening {
		t.Fatalf("scheduled retry state = %v, want reopening", got)
	}

	h.e.setsValue.Reconnect = false
	h.e.life.reconnectAt = time.Time{}
	h.connect()
	h.fake.launch(false, "no answer")
	h.e.closeSession(false)
	h.e.publish()
	if got := targetStateOf(t, h.e.Snapshot(), testDeviceID); got != TargetFailed {
		t.Fatalf("stopped retry state = %v, want failed", got)
	}
}

// TestTakeoverPublishesFailed pins the takeover: the stale plan's rows are
// failed, so they offer reconnect rather than cancel.
func TestTakeoverPublishesFailed(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()
	h.e.onPreempted("another sender")
	if got := targetStateOf(t, h.e.Snapshot(), testDeviceID); got != TargetFailed {
		t.Fatalf("takeover state = %v, want failed", got)
	}
}

// TestJoinAndRebuildPublishTheSameAudioFields pins the premise the old design
// missed: an add and a latency rebuild publish identical membership and audio
// fields, so the per-target role is the only thing that can tell the two apart.
func TestJoinAndRebuildPublishTheSameAudioFields(t *testing.T) {
	add := newHarness(t)
	b := advertiseBoth(add)
	add.connect()
	add.launch()
	add.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	addSnap := add.e.Snapshot()

	reb := newHarness(t)
	b2 := advertiseBoth(reb)
	reb.connect()
	reb.launch()
	reb.e.handleCommand(command{kind: cmdConnect, deviceID: b2.ID})
	reb.launch()
	reb.e.handleCommand(command{kind: cmdLatency, value: 700, reconnect: true})
	rebSnap := reb.e.Snapshot()

	if len(addSnap.ActiveTargets) != len(rebSnap.ActiveTargets) || len(addSnap.Targets) != len(rebSnap.Targets) {
		t.Fatalf("shapes differ: add %v/%d, rebuild %v/%d",
			addSnap.ActiveTargets, len(addSnap.Targets), rebSnap.ActiveTargets, len(rebSnap.Targets))
	}
	for i := range addSnap.ActiveTargets {
		if addSnap.ActiveTargets[i] != rebSnap.ActiveTargets[i] {
			t.Fatalf("membership differs: %v vs %v", addSnap.ActiveTargets, rebSnap.ActiveTargets)
		}
	}
	ab, _ := addSnap.Target(b.ID)
	rb, _ := rebSnap.Target(b2.ID)
	if ab.Volume != rb.Volume || ab.Channel != rb.Channel {
		t.Fatalf("audio fields differ: %+v vs %+v", addSnap.Targets, rebSnap.Targets)
	}
	if ab.State != TargetJoining {
		t.Fatalf("add state = %v, want joining", ab.State)
	}
	if rb.State != TargetMember {
		t.Fatalf("rebuild state = %v, want member", rb.State)
	}
}
