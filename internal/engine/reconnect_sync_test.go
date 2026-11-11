package engine

import (
	"errors"
	"testing"

	"saundo/internal/airplay/device"
)

// TestSynchronousReconnectFailureKeepsTheBackoff pins the retry loop against
// every synchronous rejection: the pending time is consumed before connect
// runs, so only an explicit re-arm keeps the documented backoff alive.
func TestSynchronousReconnectFailureKeepsTheBackoff(t *testing.T) {
	t.Run("session open failure", func(t *testing.T) {
		h := newHarness(t)
		h.addDeviceOnline()
		h.e.newSession = func(device.Device) (transport, func(), error) {
			return nil, nil, errors.New("ptp port busy")
		}
		h.e.selectedID = testDeviceID
		h.e.reconnectWanted = true
		h.e.attemptReconnect()
		if h.e.reconnectAt.IsZero() {
			t.Fatal("the reconnect loop died after a failed session open")
		}
	})

	t.Run("synchronous connect guard", func(t *testing.T) {
		h := newHarness(t)
		h.addDeviceOnline()
		h.e.discovered[0].Auth = device.AuthLegacyPin
		h.e.rebuildTargets()
		h.e.selectedID = testDeviceID
		h.e.reconnectWanted = true
		h.e.attemptReconnect()
		if h.e.reconnectAt.IsZero() {
			t.Fatal("the reconnect loop died after a synchronous guard")
		}
	})
}
