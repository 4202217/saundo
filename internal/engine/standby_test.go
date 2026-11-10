package engine

import (
	"testing"
	"time"

	"saundo/internal/airplay/device"
)

// standbyHarness connects and streams to a capable HomePod with the default
// lamp and standby settings.
func standbyHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.e.lampJPEG = func(preset string) ([]byte, error) {
		return []byte("jpeg:" + preset), nil
	}
	h.connect()
	h.launch()
	return h
}

func TestStandbyDarkensAtThresholdAndResumesOnSound(t *testing.T) {
	h := standbyHarness(t)
	h.e.silence.lastLoud.Store(h.now.Add(-time.Hour).UnixNano())
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleSuspended {
		t.Fatalf("idle after the pause = %v, want suspended", h.e.idle)
	}
	h.advance(31 * time.Minute)
	h.e.updateIdlePhase(h.now)

	if h.e.idle != idleDark {
		t.Fatalf("idle = %v, want dark", h.e.idle)
	}
	snap := h.e.Snapshot()
	if !snap.Standby || !snap.AudioIdle {
		t.Fatalf("snapshot standby/idle = %v/%v, want true/true", snap.Standby, snap.AudioIdle)
	}
	if len(h.fake.playback) != 2 || h.fake.playback[1] {
		t.Fatalf("playback = %v, want the launch state 1 then one state 2", h.fake.playback)
	}
	if len(h.fake.pauses) != 1 || !h.fake.pauses[0] {
		t.Fatalf("pauses = %v, want one pause", h.fake.pauses)
	}
	if n := countLogLines(h, "standby: lamp off after"); n != 1 {
		t.Fatalf("lamp-off log lines = %d, want 1", n)
	}

	h.e.updateIdlePhase(h.now)
	h.e.updateIdlePhase(h.now)
	if len(h.fake.playback) != 2 || len(h.fake.pauses) != 1 {
		t.Fatalf("duplicate commands: playback=%v pauses=%v", h.fake.playback, h.fake.pauses)
	}

	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleActive {
		t.Fatalf("idle after sound = %v, want active", h.e.idle)
	}
	if len(h.fake.playback) != 3 || !h.fake.playback[2] {
		t.Fatalf("playback = %v, want state 2 then state 1", h.fake.playback)
	}
	if len(h.fake.pauses) != 2 || h.fake.pauses[1] {
		t.Fatalf("pauses = %v, want pause then resume", h.fake.pauses)
	}
	snap = h.e.Snapshot()
	if snap.Standby || snap.AudioIdle {
		t.Fatalf("snapshot standby/idle = %v/%v, want false/false", snap.Standby, snap.AudioIdle)
	}
	if n := countLogLines(h, "standby: lamp on"); n != 1 {
		t.Fatalf("lamp-on log lines = %d, want 1", n)
	}
}

func TestUserConnectStartsTheStandbyClockAtSessionStart(t *testing.T) {
	h := standbyHarness(t)
	h.e.silence.lastLoud.Store(h.now.Add(-2 * time.Hour).UnixNano())
	h.advance(20 * time.Minute)
	h.e.updateIdlePhase(h.now)

	if h.e.idle != idleSuspended {
		t.Fatalf("idle after 20 minutes = %v, want suspended", h.e.idle)
	}
	if h.e.Snapshot().Standby {
		t.Fatal("a fresh user connect started dark")
	}

	h.advance(11 * time.Minute)
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleDark {
		t.Fatalf("idle after 31 minutes = %v, want dark", h.e.idle)
	}
}

func TestAutomaticReconnectStartsDarkWithoutStateOne(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.connect()
	h.e.connectAuto = true
	h.e.lastPushAt = h.now.Add(-2 * time.Hour)
	h.e.silence.lastLoud.Store(h.now.Add(-2 * time.Hour).UnixNano())
	h.launch()

	if h.e.idle != idleDark {
		t.Fatalf("idle = %v, want dark", h.e.idle)
	}
	if len(h.fake.playback) != 1 || h.fake.playback[0] {
		t.Fatalf("playback = %v, want exactly one state 2 and no state 1", h.fake.playback)
	}
	if !h.e.Snapshot().Standby {
		t.Fatal("snapshot did not report standby")
	}
}

func TestManualLampActionHoldsTheLightUntilTheNextSound(t *testing.T) {
	h := standbyHarness(t)
	h.e.silence.lastLoud.Store(h.now.Add(-time.Hour).UnixNano())
	h.advance(31 * time.Minute)
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleDark {
		t.Fatalf("idle = %v, want dark", h.e.idle)
	}

	h.e.handleCommand(command{kind: cmdLamp, value: 0, text: "ember"})
	if h.e.idle != idleSuspended {
		t.Fatalf("idle after the manual lamp action = %v, want suspended", h.e.idle)
	}
	commands := len(h.fake.playback)
	if commands != 3 || !h.fake.playback[2] {
		t.Fatalf("playback = %v, want state 2 then the manual state 1", h.fake.playback)
	}

	h.advance(2 * time.Hour)
	h.e.updateIdlePhase(h.now)
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleSuspended {
		t.Fatalf("idle after another threshold = %v, want the hold to keep it lit", h.e.idle)
	}
	if len(h.fake.playback) != commands {
		t.Fatalf("the held light sent another playback command: %v", h.fake.playback)
	}

	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleActive {
		t.Fatalf("idle after sound = %v, want active", h.e.idle)
	}
	if !h.e.standbyHoldAt.IsZero() {
		t.Fatal("the hold survived the next sound")
	}
	if len(h.fake.pauses) != 2 || h.fake.pauses[1] {
		t.Fatalf("pauses = %v, want pause then resume", h.fake.pauses)
	}
}

func TestStandbySkipsIncapableReceivers(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()
	h.e.silence.lastLoud.Store(h.now.Add(-time.Hour).UnixNano())
	h.advance(31 * time.Minute)
	h.e.updateIdlePhase(h.now)

	if h.e.idle != idleSuspended {
		t.Fatalf("idle = %v, want suspended", h.e.idle)
	}
	if h.e.Snapshot().Standby {
		t.Fatal("an incapable receiver reported standby")
	}
	if len(h.fake.playback) != 0 {
		t.Fatalf("sent playback commands to an incapable receiver: %v", h.fake.playback)
	}
}

func TestDisablingStandbyWhileDarkRestoresTheLight(t *testing.T) {
	h := standbyHarness(t)
	h.e.silence.lastLoud.Store(h.now.Add(-time.Hour).UnixNano())
	h.advance(31 * time.Minute)
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleDark {
		t.Fatalf("idle = %v, want dark", h.e.idle)
	}

	h.e.handleCommand(command{kind: cmdStandby, show: false, minutes: 30})
	if h.e.idle != idleSuspended {
		t.Fatalf("idle = %v, want suspended", h.e.idle)
	}
	if h.e.Snapshot().Standby {
		t.Fatal("snapshot still reports standby")
	}
	if len(h.fake.playback) != 3 || !h.fake.playback[2] {
		t.Fatalf("playback = %v, want state 2 then state 1", h.fake.playback)
	}
	h.flushSettings()
	if got := h.e.sets.Load().Standby; got.Enabled {
		t.Fatalf("persisted standby = %+v, want off", got)
	}
}

func TestTakeoverClearsStandby(t *testing.T) {
	h := standbyHarness(t)
	h.e.silence.lastLoud.Store(h.now.Add(-time.Hour).UnixNano())
	h.advance(31 * time.Minute)
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleDark {
		t.Fatalf("idle = %v, want dark", h.e.idle)
	}

	h.e.onPreempted("receiver closed the session")
	snap := h.e.Snapshot()
	if snap.Standby || snap.AudioIdle {
		t.Fatalf("takeover left standby/idle = %v/%v, want false/false", snap.Standby, snap.AudioIdle)
	}
}

// TestLastPushTracksTheStreamNotSilence pins the standby anchor: the clock
// advances while the sender pushes and freezes when the pacer stops, not when
// the captured audio happens to be quiet.
func TestLastPushTracksTheStreamNotSilence(t *testing.T) {
	h := standbyHarness(t)
	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.updateIdlePhase(h.now)
	if !h.e.lastPushAt.Equal(h.now) {
		t.Fatalf("last push = %v, want %v while pushing", h.e.lastPushAt, h.now)
	}

	h.advance(time.Minute)
	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.updateIdlePhase(h.now)
	if !h.e.lastPushAt.Equal(h.now) {
		t.Fatalf("last push did not advance while pushing: %v", h.e.lastPushAt)
	}

	h.advance(6 * time.Second)
	h.e.updateIdlePhase(h.now)
	if h.fake.pushing {
		t.Fatal("three seconds of silence did not stop the push")
	}
	paused := h.e.lastPushAt
	h.advance(10 * time.Minute)
	h.e.updateIdlePhase(h.now)
	if !h.e.lastPushAt.Equal(paused) {
		t.Fatalf("last push moved while the pacer was stopped: %v -> %v", paused, h.e.lastPushAt)
	}
}

func TestIdlePhaseChangesSendOneCommandEach(t *testing.T) {
	h := standbyHarness(t)
	h.fake.playback = nil
	h.e.silence.lastLoud.Store(h.now.Add(-6 * time.Second).UnixNano())
	h.e.updateIdlePhase(h.now)
	for range 4 {
		h.e.updateIdlePhase(h.now)
	}
	if len(h.fake.pauses) != 1 || !h.fake.pauses[0] {
		t.Fatalf("pauses = %v, want one pause", h.fake.pauses)
	}
	if len(h.fake.playback) != 0 {
		t.Fatalf("suspending sent playback commands: %v", h.fake.playback)
	}

	h.advance(31 * time.Minute)
	h.e.updateIdlePhase(h.now)
	for range 4 {
		h.e.updateIdlePhase(h.now)
	}
	if len(h.fake.playback) != 1 || h.fake.playback[0] {
		t.Fatalf("playback = %v, want exactly one state 2", h.fake.playback)
	}
}
