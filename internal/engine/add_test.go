package engine

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"saundo/internal/airplay/device"
)

// secondDevice is the other discovered receiver an add test connects to. It
// is NTP timed, so the combined cohort stays inside the one-PTP-role rule.
func secondDevice() device.Device {
	d := onlineDevice()
	d.ID = "AA:BB:CC:DD:EE:02"
	d.Name = "卧室 HomePod"
	d.Host = "192.0.2.11"
	d.Model = "AppleTV6,2"
	d.Auth = device.AuthHAPPIN
	return d
}

// advertiseBoth feeds the harness's receiver and the added target together.
func advertiseBoth(h *harness) device.Device {
	b := secondDevice()
	h.e.setDevices([]device.Device{onlineDevice(), b})
	return b
}

// TestAddTargetKeepsLocalMute pins the add policy: connecting another
// receiver while the cohort streams rebuilds the transport, but the user
// never disconnected, so the connection mute survives the gap and the new
// cohort inherits it. A later disconnect still restores the endpoint.
func TestAddTargetKeepsLocalMute(t *testing.T) {
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
		t.Fatalf("sessions = %d, want 2 after the add", h.sessions)
	}
	for _, c := range m.setCalls[before:] {
		if !c.muted {
			t.Fatalf("the add unmuted the endpoint: %+v", m.setCalls)
		}
	}
	if !m.muted["default-ep"] {
		t.Fatal("the endpoint is not muted during the add gap")
	}

	h.launch()
	h.settleMute(t, MuteLocalActive)
	if !m.muted["default-ep"] {
		t.Fatal("the new session lost the mute")
	}
	if _, err := os.Stat(h.e.muteGuardPath()); err != nil {
		t.Fatalf("mute guard missing after the add: %v", err)
	}

	h.e.handleCommand(command{kind: cmdDisconnect})
	h.settleMute(t, MuteLocalWaiting)
	if m.muted["default-ep"] {
		t.Fatal("disconnect after the add left the endpoint muted")
	}
}

// TestAddTargetKeepsProcessMute covers the per-app tap flavor: adding a
// receiver must not rebuild the tap unmuted.
func TestAddTargetKeepsProcessMute(t *testing.T) {
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
			t.Fatalf("the add unmuted the tap: %v", pm.calls)
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

// TestAddTargetRestoresMuteWhenTheNewSessionFails pins the limit of the
// hold: an add that cannot open its new cohort must not leave the endpoint
// muted with no stream to belong to.
func TestAddTargetRestoresMuteWhenTheNewSessionFails(t *testing.T) {
	h := newHarness(t)
	m := newFakeMuter()
	useMuter(h, m)
	h.e.setsValue.MuteLocalOnConnect = true
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.settleMute(t, MuteLocalActive)

	h.e.newCohort = func(sessionPlan) (transport, error) {
		return nil, errors.New("no route")
	}
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.settleMute(t, MuteLocalWaiting)
	if m.muted["default-ep"] {
		t.Fatal("a failed add left the endpoint muted")
	}
	if _, err := os.Stat(h.e.muteGuardPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a failed add kept the guard: %v", err)
	}
}

// TestAddTargetRestoresMuteWhenTheLaunchFails covers the asynchronous
// failure: the transport opened but the receiver refused the stream.
func TestAddTargetRestoresMuteWhenTheLaunchFails(t *testing.T) {
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
