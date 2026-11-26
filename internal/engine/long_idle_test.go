package engine

import (
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
)

// TestLongIdleRebuildsTheSessionOnResume pins the 2026-11-24 device finding:
// a HomePod 27.3 pair leaves the realtime stream silent after a long silent
// stretch, and only a new session revives it. The first audible block after
// the threshold rebuilds instead of streaming into the silent receiver.
func TestLongIdleRebuildsTheSessionOnResume(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{onlineDevice()})
	h.connect()
	h.launch()
	h.fake.pushing = false // the suspended idle is what the rebuild revives
	if h.sessions != 1 {
		t.Fatalf("sessions = %d, want 1", h.sessions)
	}

	h.advance(longIdleRebuildAfter + time.Second)
	h.e.silence.lastLoud.Store(h.now.UnixNano())
	if !h.e.rebuildAfterLongIdle() {
		t.Fatal("a long idle did not rebuild the session on resume")
	}
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want the rebuild", h.sessions)
	}
}

// TestShortIdleKeepsTheLiveStream pins the seamless side: a resume inside the
// threshold streams into the live session, with no connect.
func TestShortIdleKeepsTheLiveStream(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{onlineDevice()})
	h.connect()
	h.launch()
	h.fake.pushing = false

	h.advance(longIdleRebuildAfter / 2)
	h.e.silence.lastLoud.Store(h.now.UnixNano())
	if h.e.rebuildAfterLongIdle() {
		t.Fatal("a short idle rebuilt the session")
	}
	if h.sessions != 1 {
		t.Fatalf("sessions = %d, want the live session kept", h.sessions)
	}
}

// TestLongIdleRebuildSkipsAnUnaddressablePlan pins the guard: a member that
// is offline must not take the healthy cohort down for a rebuild.
func TestLongIdleRebuildSkipsAnUnaddressablePlan(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{onlineDevice()})
	h.connect()
	h.launch()
	h.fake.pushing = false

	offline := onlineDevice()
	offline.Online = false
	h.e.setDevices([]device.Device{offline})
	h.advance(longIdleRebuildAfter + time.Second)
	h.e.silence.lastLoud.Store(h.now.UnixNano())
	if h.e.rebuildAfterLongIdle() {
		t.Fatal("a plan with an offline member rebuilt")
	}
	if h.sessions != 1 {
		t.Fatalf("sessions = %d, want the live session kept", h.sessions)
	}

	// The target returns while the same audible block is still current. The
	// pending intent must fire; otherwise the consumed lastLoudSeen hides the
	// long gap forever and the receiver stays silent.
	h.e.setDevices([]device.Device{onlineDevice()})
	if !h.e.rebuildAfterLongIdle() {
		t.Fatal("the pending long-idle rebuild did not fire after the target returned")
	}
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want the pending rebuild", h.sessions)
	}
}

// TestLongIdleRebuildSkipsANonStreamingTransport pins the guard around a
// handshake: only a live stream has a receiver renderer to revive.
func TestLongIdleRebuildSkipsANonStreamingTransport(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{onlineDevice()})
	h.connect()
	h.launch()
	h.fake.state = sender.StateConnecting

	h.advance(longIdleRebuildAfter + time.Second)
	h.e.silence.lastLoud.Store(h.now.UnixNano())
	if h.e.rebuildAfterLongIdle() {
		t.Fatal("a connecting session rebuilt")
	}
	if h.sessions != 1 {
		t.Fatalf("sessions = %d, want no rebuild", h.sessions)
	}
}

// TestLongIdlePendingSurvivesTheEngineResume pins the delivery order that the
// direct rebuildAfterLongIdle tests miss: the engine has already suspended the
// stream, the first audible block arms a pending rebuild while the target is
// offline, and the same wake then sees the sound. Resuming the pacer there
// makes the next wake's Pushing guard drop the intent, and the receiver stays
// silent for the rest of the session until a manual reconnect.
func TestLongIdlePendingSurvivesTheEngineResume(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{onlineDevice()})
	h.connect()
	h.launch()

	// The source goes silent; the engine suspends the pacer.
	h.advance(audioPauseAfter + time.Second)
	h.e.onWake()
	if h.fake.pushing {
		t.Fatal("the stream did not suspend")
	}

	// Most of the long gap passes, then the target drops off the network.
	h.advance(longIdleRebuildAfter)
	offline := onlineDevice()
	offline.Online = false
	h.e.setDevices([]device.Device{offline})

	// The first sound arrives while the target is unreachable.
	h.advance(time.Second)
	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.onWake()
	if !h.e.longIdlePending {
		t.Fatal("the long-idle rebuild was not left pending")
	}
	if h.sessions != 1 {
		t.Fatalf("sessions = %d, want the live session kept", h.sessions)
	}
	if h.fake.pushing {
		t.Fatal("the engine resumed the pacer while the rebuild was pending")
	}

	// The target returns; the pending rebuild fires on the next wake.
	h.e.setDevices([]device.Device{onlineDevice()})
	h.e.onWake()
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want the pending rebuild to fire", h.sessions)
	}
}
