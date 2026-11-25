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
