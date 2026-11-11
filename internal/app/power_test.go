package app

import (
	"testing"

	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
)

func TestSessionActive(t *testing.T) {
	cases := []struct {
		name string
		snap engine.Snapshot
		want bool
	}{
		{"idle", engine.Snapshot{State: sender.StateIdle}, false},
		{"connecting", engine.Snapshot{State: sender.StateConnecting}, true},
		{"pairing", engine.Snapshot{State: sender.StatePairing}, true},
		{"handshake", engine.Snapshot{State: sender.StateHandshake}, true},
		{"streaming", engine.Snapshot{State: sender.StateStreaming}, true},
		{"standby", engine.Snapshot{State: sender.StateStreaming, Standby: true}, true},
		{"reconnecting", engine.Snapshot{State: sender.StateIdle, Reconnecting: true}, true},
		{"taken over", engine.Snapshot{State: sender.StateIdle, TakenOver: true}, false},
		{"taken over while streaming", engine.Snapshot{State: sender.StateStreaming, TakenOver: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionActive(tc.snap); got != tc.want {
				t.Fatalf("sessionActive(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestPowerHoldTransitions(t *testing.T) {
	var acquires, releases int
	var reason string
	h := powerHold{acquire: func(r string) func() {
		acquires++
		reason = r
		return func() { releases++ }
	}}

	h.observe(false)
	if acquires != 0 || releases != 0 {
		t.Fatalf("idle acquired: %d/%d", acquires, releases)
	}
	h.observe(true)
	if acquires != 1 || reason == "" {
		t.Fatalf("active acquire = %d, reason %q", acquires, reason)
	}
	if !h.held {
		t.Fatal("active hold not marked held")
	}
	h.observe(true)
	if acquires != 1 {
		t.Fatalf("repeated active acquired again: %d", acquires)
	}
	h.observe(false)
	if releases != 1 || h.held {
		t.Fatalf("idle releases = %d, held = %v", releases, h.held)
	}
	h.observe(true)
	if acquires != 2 {
		t.Fatalf("reacquire = %d", acquires)
	}
	h.stop()
	if releases != 2 || h.held {
		t.Fatalf("stop releases = %d, held = %v", releases, h.held)
	}
	h.observe(true)
	if acquires != 2 {
		t.Fatalf("activation after stop = %d", acquires)
	}
	h.stop()
	if releases != 2 {
		t.Fatalf("second stop releases = %d", releases)
	}
}

func TestPowerHoldNilAcquireIsNoOp(t *testing.T) {
	h := powerHold{}
	h.observe(true)
	if h.held {
		t.Fatal("a zero powerHold held without an acquire function")
	}
	h.observe(false)
	h.stop()
	if h.held {
		t.Fatal("a zero powerHold held after stop")
	}
}

func TestApplyFollowsSessionWithPower(t *testing.T) {
	a := previewApp()
	var acquires, releases int
	a.power.acquire = func(string) func() {
		acquires++
		return func() { releases++ }
	}

	a.apply(engine.Snapshot{State: sender.StateStreaming})
	if !a.power.held || acquires != 1 {
		t.Fatalf("streaming hold = %v, acquires = %d", a.power.held, acquires)
	}
	a.apply(engine.Snapshot{State: sender.StateIdle})
	if a.power.held || releases != 1 {
		t.Fatalf("idle hold = %v, releases = %d", a.power.held, releases)
	}
	a.apply(engine.Snapshot{State: sender.StateIdle, Reconnecting: true})
	if !a.power.held || acquires != 2 {
		t.Fatalf("reconnect hold = %v, acquires = %d", a.power.held, acquires)
	}
}
