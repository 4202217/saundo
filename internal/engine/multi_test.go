package engine

import (
	"bytes"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
)

// TestAddSecondStandalonePTPReceiver pins the unpaired-pair scenario: two
// standalone HomePods each keep their own grandmaster, so they share one
// follower set and the second joins the streaming cohort instead of failing on
// the PTP ports.
func TestAddSecondStandalonePTPReceiver(t *testing.T) {
	h := newHarness(t)
	r := testSingle("AA:BB:CC:DD:EE:02", "192.0.2.11", "AudioAccessory6,1")
	h.e.setDevices([]device.Device{onlineDevice(), r})
	h.connect()
	h.launch()

	h.e.handleCommand(command{kind: cmdConnect, deviceID: r.ID})
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want one rebuilt cohort with both receivers", h.sessions)
	}
	if snap := h.e.Snapshot(); snap.ErrorKey != "" || snap.Error != "" {
		t.Fatalf("the add failed: %q %q", snap.ErrorKey, snap.Error)
	}
	if h.plan.clock.role != ptpFollowers {
		t.Fatalf("clock role = %v, want a follower set", h.plan.clock.role)
	}
	if got := h.plan.clock.followerHosts; len(got) != 2 || got[0] != "192.0.2.10" || got[1] != "192.0.2.11" {
		t.Fatalf("follower hosts = %v, want both receivers", got)
	}
	if snap := h.e.Snapshot(); len(snap.ActiveTargets) != 2 {
		t.Fatalf("active targets = %v, want both", snap.ActiveTargets)
	}
}

// TestConnectWhileStreamingAddsTarget pins the live add: one Connect rebuilds
// the whole cohort with both keys, the old transport stops, and the snapshot
// reports both targets without moving the primary.
func TestConnectWhileStreamingAddsTarget(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	first := h.fake

	if snap := h.e.Snapshot(); snap.ConnectedName != "客厅 HomePod" || len(snap.ActiveTargets) != 1 {
		t.Fatalf("initial snapshot = %q %v", snap.ConnectedName, snap.ActiveTargets)
	}

	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want one rebuilt cohort", h.sessions)
	}
	if !first.stopped {
		t.Fatal("the old cohort was not stopped")
	}
	h.launch()

	snap := h.e.Snapshot()
	if snap.SelectedID != testDeviceID {
		t.Fatalf("primary = %q, want the first target", snap.SelectedID)
	}
	if len(snap.ActiveTargets) != 2 || snap.ActiveTargets[0] != testDeviceID || snap.ActiveTargets[1] != b.ID {
		t.Fatalf("active targets = %v, want both in target order", snap.ActiveTargets)
	}
	if snap.ConnectedName != "客厅 HomePod + 卧室 HomePod" {
		t.Fatalf("connected name = %q, want the joined names", snap.ConnectedName)
	}
	if !snap.Active(testDeviceID) || !snap.Active(b.ID) || snap.Active("AA:00") {
		t.Fatalf("Active() does not follow the cohort: %v", snap.ActiveTargets)
	}
}

// TestDisconnectTargetLeavesOthers pins the per-device disconnect: one
// target leaves the live cohort, the rest rebuilds and keeps playing, and the
// last target takes the session down like a full disconnect.
func TestDisconnectTargetLeavesOthers(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want the add's rebuild", h.sessions)
	}
	h.launch()

	// Removing the primary leaves the other target as the new primary.
	h.e.handleCommand(command{kind: cmdDisconnectTarget, deviceID: testDeviceID})
	if h.sessions != 3 {
		t.Fatalf("sessions = %d, want one rebuild with the remaining target", h.sessions)
	}
	h.launch()
	snap := h.e.Snapshot()
	if snap.SelectedID != b.ID {
		t.Fatalf("primary = %q, want the remaining target", snap.SelectedID)
	}
	if len(snap.ActiveTargets) != 1 || snap.ActiveTargets[0] != b.ID {
		t.Fatalf("active targets = %v, want only the remaining target", snap.ActiveTargets)
	}
	if snap.Active(testDeviceID) {
		t.Fatal("the removed target is still active")
	}
	if snap.ConnectedName != b.Name {
		t.Fatalf("connected name = %q, want %q", snap.ConnectedName, b.Name)
	}

	// The last target takes the session down.
	h.e.handleCommand(command{kind: cmdDisconnectTarget, deviceID: b.ID})
	snap = h.e.Snapshot()
	if len(snap.ActiveTargets) != 0 || snap.ConnectedName != "" {
		t.Fatalf("session survived its last target: %v %q", snap.ActiveTargets, snap.ConnectedName)
	}
	if snap.State != sender.StateIdle {
		t.Fatalf("state = %v after the last target", snap.State)
	}
}

// TestDisconnectJoiningTargetBeforeStreaming pins the cancel path: an add
// that has not reached streaming yet still rebuilds when its target leaves,
// rather than leaving the connecting cohort in place with both members.
func TestDisconnectJoiningTargetBeforeStreaming(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.e.handleCommand(command{kind: cmdDisconnectTarget, deviceID: b.ID})
	if h.sessions != 3 {
		t.Fatalf("sessions = %d, want a rebuild with the first target", h.sessions)
	}
	h.launch()
	snap := h.e.Snapshot()
	if len(snap.ActiveTargets) != 1 || snap.ActiveTargets[0] != testDeviceID {
		t.Fatalf("active targets = %v, want only the first target", snap.ActiveTargets)
	}
	if snap.Active(b.ID) {
		t.Fatal("the canceled target is still active")
	}
}

// TestRejectedAddLeavesCohortLive pins the fail-before-stop rule: a rejected
// add publishes its key and never touches the streaming cohort.
func TestRejectedAddLeavesCohortLive(t *testing.T) {
	ptpPair := testPair("P", "AudioAccessory5,1")
	ptpPair.Members[0].Host, ptpPair.Members[1].Host = "192.0.2.30", "192.0.2.31"
	local := secondDevice()
	local.Local = true
	legacy := secondDevice()
	legacy.Auth = device.AuthLegacyPin
	offlinePair := testPair("P", "AudioAccessory5,1")
	offlinePair.Members[0].Host, offlinePair.Members[1].Host = "192.0.2.30", "192.0.2.31"
	offlinePair.Online = false
	cases := []struct {
		name    string
		extra   device.Device
		wantKey UserErrorKey
	}{
		{"clock conflict", ptpPair, ErrClockConflict},
		{"local receiver", local, ErrLocalReceiver},
		{"legacy pin", legacy, ErrFairPlay},
		{"stereo offline", offlinePair, ErrStereoMembersOffline},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.e.setDevices([]device.Device{onlineDevice(), tc.extra})
			h.connect()
			h.launch()
			first := h.fake
			before := h.sessions

			h.e.handleCommand(command{kind: cmdConnect, deviceID: tc.extra.ID})
			if h.sessions != before {
				t.Fatalf("a rejected add rebuilt the cohort: sessions %d -> %d", before, h.sessions)
			}
			if first.stopped {
				t.Fatal("a rejected add stopped the live cohort")
			}
			snap := h.e.Snapshot()
			if snap.ErrorKey != tc.wantKey {
				t.Fatalf("error key = %q, want %q (error %q)", snap.ErrorKey, tc.wantKey, snap.Error)
			}
			if len(snap.ActiveTargets) != 1 || snap.ActiveTargets[0] != testDeviceID {
				t.Fatalf("active targets = %v, want the live target only", snap.ActiveTargets)
			}
			if snap.State != sender.StateStreaming {
				t.Fatalf("state = %v, want the live cohort still streaming", snap.State)
			}
		})
	}
}

// TestDisconnectEndsCohort pins the total Disconnect: plan, wanted set, rings,
// mute, and rows all clear.
func TestDisconnectEndsCohort(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()
	h.settleMute(t, MuteLocalActive)

	h.e.handleCommand(command{kind: cmdDisconnect})
	h.settleMute(t, MuteLocalWaiting)

	snap := h.e.Snapshot()
	if len(snap.ActiveTargets) != 0 || snap.ConnectedName != "" {
		t.Fatalf("snapshot kept the cohort: %+v", snap)
	}
	if snap.Active(testDeviceID) || snap.Active(b.ID) {
		t.Fatalf("rows stayed active after Disconnect: %v", snap.ActiveTargets)
	}
	if h.e.plan.live() || len(h.e.wantedKeys) != 0 || h.e.tr != nil {
		t.Fatalf("engine kept the cohort: plan=%v wanted=%v tr=%v", h.e.plan.live(), h.e.wantedKeys, h.e.tr)
	}
	if len(h.e.memberRings) != 0 {
		t.Fatalf("member rings survived: %d", len(h.e.memberRings))
	}
	h.e.captureSink.Push([]int16{1, 2})
	if h.e.ring.Available() != 2 {
		t.Fatalf("the single ring is not back on the fanout: %d", h.e.ring.Available())
	}
}

// TestSameHostTargetsAreRejected pins the routing constraint: the cohort
// routes events by peer host, so a plan with two members on one host cannot
// tell them apart and is refused before it opens.
func TestSameHostTargetsAreRejected(t *testing.T) {
	h := newHarness(t)
	a := testSingle("AA:BB:CC:DD:EE:01", "192.0.2.10", "AppleTV6,2")
	b := testSingle("AA:BB:CC:DD:EE:02", "192.0.2.10", "AppleTV6,2")
	b.Port = 7005
	h.e.setDevices([]device.Device{a, b})
	h.connect()
	h.launch()
	before := h.sessions

	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	if h.sessions != before {
		t.Fatalf("a same-host add rebuilt the cohort: sessions %d -> %d", before, h.sessions)
	}
	if snap := h.e.Snapshot(); snap.ErrorKey != ErrSessionOpen {
		t.Fatalf("error key = %q, want %q (error %q)", snap.ErrorKey, ErrSessionOpen, snap.Error)
	}
	if snap := h.e.Snapshot(); snap.State != sender.StateStreaming {
		t.Fatalf("state = %v, want the live cohort still streaming", snap.State)
	}
}

// TestRetryPrunesMissingWantedKeys pins the user Retry mirror: a wanted key
// that disappeared is dropped with the reconnect log line, and the rest of the
// set still reconnects.
func TestRetryPrunesMissingWantedKeys(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.fake.launch(false, "authentication failed")
	before := h.sessions

	h.e.reg.Delete(b.ID)
	h.e.setDevices([]device.Device{onlineDevice()})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: testDeviceID})

	if h.sessions != before+1 {
		t.Fatalf("sessions = %d, want one retry of the surviving target", h.sessions-before)
	}
	if got := h.e.plan.keys(); len(got) != 1 || got[0] != testDeviceID {
		t.Fatalf("retry plan = %v, want the surviving target only", got)
	}
	if got := h.e.wantedKeys; len(got) != 1 || got[0] != testDeviceID {
		t.Fatalf("wanted keys = %v, want the surviving target only", got)
	}
	if n := countLogLines(h, "dropping "+b.ID); n != 1 {
		t.Fatalf("dropped-target log lines = %d, want 1", n)
	}
}

// TestRetryRefusesAGuardViolationAndKeepsTheWantedSet pins the other half of
// the mirror: a wanted key that now fails a guard surfaces its error, opens no
// session, and leaves the requested set intact.
func TestRetryRefusesAGuardViolationAndKeepsTheWantedSet(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.fake.launch(false, "refused")
	before := h.sessions

	legacy := b
	legacy.Auth = device.AuthLegacyPin
	h.e.setDevices([]device.Device{onlineDevice(), legacy})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: testDeviceID})

	if h.sessions != before {
		t.Fatalf("a guard-refused retry opened a session: sessions %d -> %d", before, h.sessions)
	}
	if snap := h.e.Snapshot(); snap.ErrorKey != ErrFairPlay {
		t.Fatalf("error key = %q, want %q (error %q)", snap.ErrorKey, ErrFairPlay, snap.Error)
	}
	if got := h.e.wantedKeys; len(got) != 2 || got[0] != testDeviceID || got[1] != b.ID {
		t.Fatalf("wanted keys = %v, want the requested set intact", got)
	}
	if n := countLogLines(h, "retry: "+b.ID+" refused"); n != 1 {
		t.Fatalf("refused-key log lines = %d, want 1", n)
	}
}

// TestAddReplaysNowPlaying pins the rebuilt cohort's metadata: a track that
// reached the first cohort is handed to the new one, cover included, before it
// starts.
func TestAddReplaysNowPlaying(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	cover := []byte{0xFF, 0xD8, 0x01, 0x02}
	h.e.handleCommand(command{
		kind: cmdNowPlaying, text: "夜曲", name: "周杰伦", album: "十一月的萧邦",
		cover: cover, coverMIME: "image/jpeg",
	})
	if len(h.fake.nowPlaying) == 0 {
		t.Fatal("the live cohort did not receive the track")
	}
	cover[0] = 0x00 // the caller reuses its buffer after the command

	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()

	now := h.fake.nowPlaying
	if len(now) == 0 || now[len(now)-1] != "夜曲\x00周杰伦\x00十一月的萧邦" {
		t.Fatalf("rebuilt cohort tracks = %q, want the cached track", now)
	}
	if !bytes.Equal(h.fake.nowCover, []byte{0xFF, 0xD8, 0x01, 0x02}) {
		t.Fatalf("rebuilt cohort cover = %v, want the cached bytes", h.fake.nowCover)
	}
	if h.fake.nowCoverMIME != "image/jpeg" {
		t.Fatalf("rebuilt cohort cover mime = %q, want image/jpeg", h.fake.nowCoverMIME)
	}
}

// TestLatencyRebuildRebuildsWholeCohort pins the latency edit: one rebuild for
// the whole cohort, every member recomputed from its own governing key.
func TestLatencyRebuildRebuildsWholeCohort(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	setDelay(h, testDeviceID, 120)
	setDelay(h, b.ID, 60)
	h.e.handleCommand(command{kind: cmdLatency, value: 200})
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()
	if got := memberLatencies(h.plan); len(got) != 2 || got[0] != 320 || got[1] != 260 {
		t.Fatalf("initial latencies = %v, want [320 260]", got)
	}

	before := h.sessions
	h.e.handleCommand(command{kind: cmdLatency, value: 300, reconnect: true})
	if h.sessions != before+1 {
		t.Fatalf("sessions = %d, want one rebuild", h.sessions-before)
	}
	if got := memberLatencies(h.plan); len(got) != 2 || got[0] != 420 || got[1] != 360 {
		t.Fatalf("rebuilt latencies = %v, want [420 360]", got)
	}
}

// TestMemberFailureReconnectsLastCohort pins the session-level reconnect: any
// member death is terminal, and the retry rebuilds the wanted set minus keys
// that no longer resolve.
func TestMemberFailureReconnectsLastCohort(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.e.setsValue.Reconnect = true
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()
	if got := h.e.wantedKeys; len(got) != 2 {
		t.Fatalf("wanted keys = %v, want both targets", got)
	}

	h.fake.state = sender.StateIdle
	h.e.onClosed()
	if !h.e.reconnectWanted || h.e.reconnectAt.IsZero() {
		t.Fatal("a member close did not arm the session reconnect")
	}

	// The second target disappears entirely: no advertisement, no registry
	// entry, so the retry drops it with a log line.
	h.e.reg.Delete(b.ID)
	h.e.setDevices([]device.Device{onlineDevice()})
	h.advance(time.Second)
	h.e.attemptReconnect()

	if h.sessions != 3 {
		t.Fatalf("sessions = %d, want the retry to open a new cohort", h.sessions)
	}
	if got := h.e.plan.keys(); len(got) != 1 || got[0] != testDeviceID {
		t.Fatalf("retry plan = %v, want the surviving target only", got)
	}
	if n := countLogLines(h, "dropping "+b.ID); n != 1 {
		t.Fatalf("dropped-target log lines = %d, want 1", n)
	}
}

// TestRetryFromEitherActiveRowRetriesEveryTarget pins the wanted-set retry: a
// Retry click on any active row after a failed multi-target launch rebuilds
// every wanted target, not just the clicked one.
func TestRetryFromEitherActiveRowRetriesEveryTarget(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.fake.launch(false, "authentication failed")

	snap := h.e.Snapshot()
	if len(snap.ActiveTargets) != 2 {
		t.Fatalf("active targets after a failed launch = %v, want the whole wanted set", snap.ActiveTargets)
	}
	if snap.ErrorKey != "" || snap.Error == "" {
		t.Fatalf("failed launch error = %q/%q, want the technical text", snap.ErrorKey, snap.Error)
	}
	before := h.sessions

	// The user clicks Retry on the second row.
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	if h.sessions != before+1 {
		t.Fatalf("sessions = %d, want one retry attempt", h.sessions-before)
	}
	if got := h.e.plan.keys(); len(got) != 2 || got[0] != testDeviceID || got[1] != b.ID {
		t.Fatalf("retry plan = %v, want every wanted target", got)
	}
	if h.e.Snapshot().Error != "" {
		t.Fatal("a retry kept the previous failure")
	}
}

// TestWantedSetSurvivesAFailedLaunch pins the intent contract: a failed
// multi-target launch keeps the wanted set for the next Retry.
func TestWantedSetSurvivesAFailedLaunch(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.fake.launch(false, "refused")

	if got := h.e.wantedKeys; len(got) != 2 || got[0] != testDeviceID || got[1] != b.ID {
		t.Fatalf("wanted keys after a failed launch = %v", got)
	}
	h.e.handleCommand(command{kind: cmdDisconnect})
	if len(h.e.wantedKeys) != 0 {
		t.Fatalf("disconnect kept the wanted set: %v", h.e.wantedKeys)
	}
}

// TestPreemptedKeepsTheWantedSetWithoutRetry pins the takeover row action: the
// wanted set stays so every active row keeps 重新连接, but no automatic retry
// is armed against the sender that took the receiver.
func TestPreemptedKeepsTheWantedSetWithoutRetry(t *testing.T) {
	h := newHarness(t)
	h.addDeviceOnline()
	h.connect()
	h.launch()
	h.e.onPreempted("receiver closed the session")

	if got := h.e.wantedKeys; len(got) != 1 || got[0] != testDeviceID {
		t.Fatalf("wanted keys after a takeover = %v, want the requested target", got)
	}
	if h.e.reconnectWanted || !h.e.reconnectAt.IsZero() {
		t.Fatalf("a takeover armed a retry: wanted=%v at=%v", h.e.reconnectWanted, h.e.reconnectAt)
	}
	// The run loop tears the dead transport down next; the row must stay active
	// so the flyout still offers 重新连接, not 连接.
	h.e.closeSession(false)
	snap := h.e.Snapshot()
	if !snap.Active(testDeviceID) {
		t.Fatalf("active rows after a takeover = %v, want the wanted target", snap.ActiveTargets)
	}
	if snap.Reconnecting {
		t.Fatal("the takeover snapshot shows an automatic retry")
	}
}
