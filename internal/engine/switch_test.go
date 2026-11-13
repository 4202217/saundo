package engine

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"saundo/internal/airplay/device"
)

// secondDevice is the other discovered receiver a switch test connects to.
func secondDevice() device.Device {
	d := onlineDevice()
	d.ID = "AA:BB:CC:DD:EE:02"
	d.Name = "卧室 HomePod"
	d.Host = "192.0.2.11"
	return d
}

// advertiseBoth feeds the harness's receiver and the switch target together.
func advertiseBoth(h *harness) device.Device {
	b := secondDevice()
	h.e.setDevices([]device.Device{onlineDevice(), b})
	return b
}

// TestDeviceSwitchKeepsLocalMute pins the switch policy: connecting another
// receiver while the session streams replaces the transport, but the user
// never disconnected, so the connection mute survives the gap and the new
// session inherits it. A later disconnect still restores the endpoint.
func TestDeviceSwitchKeepsLocalMute(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)
	before := len(m.setCalls)

	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	if h.sessions != 2 {
		t.Fatalf("sessions = %d, want 2 after the switch", h.sessions)
	}
	for _, c := range m.setCalls[before:] {
		if !c.muted {
			t.Fatalf("the switch unmuted the endpoint: %+v", m.setCalls)
		}
	}
	if !m.muted["default-ep"] {
		t.Fatal("the endpoint is not muted during the switch gap")
	}

	h.launch()
	h.settleMute(t, MuteLocalActive)
	if !m.muted["default-ep"] {
		t.Fatal("the new session lost the mute")
	}
	if _, err := os.Stat(h.e.muteGuardPath()); err != nil {
		t.Fatalf("mute guard missing after the switch: %v", err)
	}

	h.e.handleCommand(command{kind: cmdDisconnect})
	h.settleMute(t, MuteLocalWaiting)
	if m.muted["default-ep"] {
		t.Fatal("disconnect after the switch left the endpoint muted")
	}
}

// TestDeviceSwitchKeepsProcessMute covers the per-app tap flavor: switching
// receivers must not rebuild the tap unmuted.
func TestDeviceSwitchKeepsProcessMute(t *testing.T) {
	h := newHarness(t)
	pm := &fakeProcessMuter{}
	useProcessMuter(h, pm)
	h.e.setsValue.MuteLocalOnConnect = true
	h.e.captureProcess = 1234
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)
	before := len(pm.calls)

	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	for _, muted := range pm.calls[before:] {
		if !muted {
			t.Fatalf("the switch unmuted the tap: %v", pm.calls)
		}
	}
	h.launch()
	h.settleMute(t, MuteLocalActive)
	if last := pm.calls[len(pm.calls)-1]; !last {
		t.Fatalf("process mute calls = %v, want the last to be mute", pm.calls)
	}

	h.e.handleCommand(command{kind: cmdDisconnect})
	h.settleMute(t, MuteLocalWaiting)
	if last := pm.calls[len(pm.calls)-1]; last {
		t.Fatalf("process mute calls = %v, want restore", pm.calls)
	}
}

// TestDeviceSwitchRestoresMuteWhenTheNewSessionFails pins the limit of the
// hold: a switch that cannot open its new session must not leave the endpoint
// muted with no stream to belong to.
func TestDeviceSwitchRestoresMuteWhenTheNewSessionFails(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)

	h.e.newSession = func(device.Device) (transport, func(), error) {
		return nil, nil, errors.New("no route")
	}
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.settleMute(t, MuteLocalWaiting)
	if m.muted["default-ep"] {
		t.Fatal("a failed switch left the endpoint muted")
	}
	if _, err := os.Stat(h.e.muteGuardPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a failed switch kept the guard: %v", err)
	}
}

// TestDeviceSwitchRestoresMuteWhenTheLaunchFails covers the asynchronous
// failure: the transport opened but the receiver refused the stream.
func TestDeviceSwitchRestoresMuteWhenTheLaunchFails(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)

	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.fake.launch(false, "authentication failed")
	h.settleMute(t, MuteLocalWaiting)
	if m.muted["default-ep"] {
		t.Fatal("a failed launch left the endpoint muted")
	}
	if _, err := os.Stat(h.e.muteGuardPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a failed launch kept the guard: %v", err)
	}
}
