package engine

import (
	"errors"
	"testing"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// TestFreshAttemptCancelsThePreviousRetry pins the attempt boundary: a user
// connect replaces the pending retry, so when the new target fails its own
// guards the old target is not reconnected behind the user's back.
func TestFreshAttemptCancelsThePreviousRetry(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()

	h.fake.state = sender.StateIdle
	h.e.onClosed()
	if h.e.life.reconnectAt.IsZero() {
		t.Fatal("the failed session did not arm a retry")
	}

	tv := device.Device{
		ID: "AA:BB:CC:DD:EE:02", Name: "客厅 Apple TV", Host: "192.0.2.11", Port: 7000,
		Model: "AppleTV3,2", Auth: device.AuthLegacyPin, Online: true,
	}
	h.e.setDevices([]device.Device{onlineDevice(), tv})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: tv.ID})

	if !h.e.life.reconnectAt.IsZero() || h.e.life.reconnectWanted {
		t.Fatal("the replaced attempt kept its retry armed")
	}
}

// TestAutoConnectStartPlanFailureArmsTheRetry pins the shared open-failure
// policy: the auto-connect path opens its cohort through startPlan, so a
// synchronous session-open failure must schedule the backoff like the
// reconnect path does.
func TestAutoConnectStartPlanFailureArmsTheRetry(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.newCohort = func(sessionPlan) (transport, error) {
		return nil, errors.New("ptp port busy")
	}
	h.e.reg.Put(receivers.Entry{Key: testDeviceID, Kind: receivers.KindDiscovered, AutoConnect: true})
	h.addDeviceOnline()

	if h.e.lastErrorKey != ErrSessionOpen {
		t.Fatalf("error key = %q, want %q", h.e.lastErrorKey, ErrSessionOpen)
	}
	if !h.e.Snapshot().Reconnecting {
		t.Fatal("the auto-connect open failure did not schedule a retry")
	}
}
