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
		t.Fatalf("pauses = %v, want one suspend", h.fake.pauses)
	}
	if h.fake.pushing {
		t.Fatal("the silent stream kept pushing")
	}
	if n := countLogLines(h, "standby: lamp off after"); n != 1 {
		t.Fatalf("lamp-off log lines = %d, want 1", n)
	}

	h.e.updateIdlePhase(h.now)
	h.e.updateIdlePhase(h.now)
	if len(h.fake.playback) != 2 {
		t.Fatalf("duplicate commands: playback=%v", h.fake.playback)
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
		t.Fatalf("pauses = %v, want suspend then resume", h.fake.pauses)
	}
	if !h.fake.pushing {
		t.Fatal("the resume did not restart the pacer")
	}
	snap = h.e.Snapshot()
	if snap.Standby || snap.AudioIdle {
		t.Fatalf("snapshot standby/idle = %v/%v, want false/false", snap.Standby, snap.AudioIdle)
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
	h.e.life.connectAuto = true
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

	old, sessions := h.fake, h.sessions
	h.e.silence.lastLoud.Store(h.now.UnixNano())
	h.e.updateIdlePhase(h.now)
	if h.e.idle != idleActive {
		t.Fatalf("idle after sound = %v, want active", h.e.idle)
	}
	// The session kept its transport; only the pacer suspended and resumed.
	if h.sessions != sessions || h.fake != old {
		t.Fatalf("the resume rebuilt the session (sessions=%d)", h.sessions-sessions)
	}
	if got := old.pauses; len(got) != 2 || got[1] {
		t.Fatalf("pauses = %v, want suspend then resume", got)
	}
	if !h.e.standbyHoldAt.IsZero() {
		t.Fatal("the hold survived the next sound")
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

// TestLaunchIntoSilenceSuspendsTheStream pins the launch path: a session
// opened while the source is silent suspends the pacer immediately.
func TestLaunchIntoSilenceSuspendsTheStream(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices([]device.Device{capableLampDevice()})
	h.e.lampJPEG = func(preset string) ([]byte, error) { return []byte("jpeg:" + preset), nil }
	h.e.silence.lastLoud.Store(h.now.Add(-audioPauseAfter).UnixNano())
	h.connect()
	h.launch()

	if h.e.idle != idleSuspended {
		t.Fatalf("idle after the silent launch = %v, want suspended", h.e.idle)
	}
	if h.fake.pushing {
		t.Fatal("the launch kept pushing")
	}
	if got := h.fake.pauses; len(got) != 1 || !got[0] {
		t.Fatalf("pauses = %v, want one suspend", got)
	}
}

// TestDarkAnchorTracksSoundNotTheStream pins the standby anchor: the clock
// starts at the last audible block, and a silent source keeps the compact
// stream running without moving that anchor.
func TestDarkAnchorTracksSoundNotTheStream(t *testing.T) {
	h := standbyHarness(t)
	lastSound := h.now
	h.e.silence.lastLoud.Store(lastSound.UnixNano())
	h.e.updateIdlePhase(h.now)

	h.advance(6 * time.Second)
	h.e.updateIdlePhase(h.now)
	if h.fake.pushing {
		t.Fatal("a silent source kept the pacer running")
	}
	if h.e.idle != idleSuspended {
		t.Fatalf("idle = %v, want suspended", h.e.idle)
	}
	if !h.e.darkBase().Equal(lastSound) {
		t.Fatalf("dark base = %v, want the last sound %v", h.e.darkBase(), lastSound)
	}

	h.advance(10 * time.Minute)
	h.e.updateIdlePhase(h.now)
	if !h.e.darkBase().Equal(lastSound) {
		t.Fatalf("dark base moved without sound: %v", h.e.darkBase())
	}
	if h.e.idle != idleSuspended {
		t.Fatalf("idle = %v, want suspended before the dark threshold", h.e.idle)
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
		t.Fatalf("a silent source did not suspend the stream: %v", h.fake.pauses)
	}
	if len(h.fake.playback) != 0 {
		t.Fatalf("idling sent playback commands: %v", h.fake.playback)
	}

	h.advance(31 * time.Minute)
	h.e.updateIdlePhase(h.now)
	for range 4 {
		h.e.updateIdlePhase(h.now)
	}
	if len(h.fake.playback) != 1 || h.fake.playback[0] {
		t.Fatalf("playback = %v, want exactly one state 2", h.fake.playback)
	}
	if len(h.fake.pauses) != 1 {
		t.Fatalf("pauses = %v, want the one suspend", h.fake.pauses)
	}
}
