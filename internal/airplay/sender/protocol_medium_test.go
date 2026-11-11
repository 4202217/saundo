package sender

import (
	"strings"
	"testing"

	"saundo/internal/airplay/device"
)

// TestAP2RecordFailureFailsTheSession pins the status check on RECORD: a
// refusal must stop the handshake instead of sending SETPEERS and the stream
// SETUP into a generic PTP timeout.
func TestAP2RecordFailureFailsTheSession(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.rx.recordStatus = 500
	r.start()
	if len(r.launched) == 0 || r.launched[0].ok {
		t.Fatalf("launch results = %+v, want a failed session", r.launched)
	}
	if !strings.Contains(r.launched[0].err, "RECORD") || !strings.Contains(r.launched[0].err, "500") {
		t.Fatalf("error = %q, want the RECORD status", r.launched[0].err)
	}
	if r.rx.ap2Audio {
		t.Fatal("stream SETUP ran after a refused RECORD")
	}
}

// TestAP2SessionSetupRequiresEventPort pins the fail-closed SETUP decode: a
// body that cannot be read, or one without eventPort, must fail the session
// rather than stream into the receiver's 25-30 s teardown.
func TestAP2SessionSetupRequiresEventPort(t *testing.T) {
	cases := []struct {
		name string
		set  func(*fakeReceiver)
	}{
		{"missing key", func(f *fakeReceiver) { f.omitEventPort = true }},
		{"garbage body", func(f *fakeReceiver) { f.garbageSessionSetup = true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
			c.set(r.rx)
			r.start()
			if len(r.launched) == 0 || r.launched[0].ok {
				t.Fatalf("launch results = %+v, want a failed session", r.launched)
			}
			if r.rx.ap2Audio {
				t.Fatal("stream SETUP ran without an event channel")
			}
		})
	}
}

// TestVolumePercentTopOfScale pins the AirPlay dBFS mapping at both ends and
// keeps the poll's conversion in agreement with the handshake helper.
func TestVolumePercentTopOfScale(t *testing.T) {
	cases := []struct {
		db  float64
		pct int
	}{
		{0, 100},
		{1, 100},
		{-0.0001, 99},
		{-15, 50},
		{-30, 0},
		{-144, 0},
	}
	for _, c := range cases {
		if got := volPercentFromDB(c.db); got != c.pct {
			t.Errorf("volPercentFromDB(%v) = %d, want %d", c.db, got, c.pct)
		}
	}
	s := &Sender{}
	for _, db := range []float64{0, -3, -15, -30, -144} {
		s.ap2 = &ap2State{hasInitial: true, initialVol: db}
		got, ok := s.InitialVolumePercent()
		if !ok || got != float64(volPercentFromDB(db)) {
			t.Errorf("InitialVolumePercent(%v) = %v, volPercentFromDB = %d", db, got, volPercentFromDB(db))
		}
	}
}

// TestVolumePollKeepsTheStoredVolumeOnUnknownField pins the absence rule: a
// reply without a usable initialVolume must leave both the sender's and the
// engine's stored volume alone.
func TestVolumePollKeepsTheStoredVolumeOnUnknownField(t *testing.T) {
	cases := []struct {
		name string
		set  func(*fakeReceiver)
	}{
		{"missing key", func(f *fakeReceiver) { f.omitInitialVolume = true }},
		{"wrong type", func(f *fakeReceiver) { f.initialVolumeString = true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
			r.start()
			c.set(r.rx)
			r.sender.receiverVol = 42
			r.sender.volQuery = false
			r.volumeEvents = nil
			r.sender.onVolumePoll()
			r.step()
			if len(r.volumeEvents) != 0 {
				t.Fatalf("volume changed from an unknown field: %v", r.volumeEvents)
			}
			if r.sender.receiverVol != 42 {
				t.Fatalf("receiverVol = %d, want the previous 42", r.sender.receiverVol)
			}
		})
	}
}

// TestVolumePollAdoptsAPresentValue is the control case: the same poll must
// still report a value the receiver actually sent.
func TestVolumePollAdoptsAPresentValue(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.receiverVolume = -15
	r.sender.receiverVol = 42
	r.sender.volQuery = false
	r.volumeEvents = nil
	r.sender.onVolumePoll()
	r.step()
	if len(r.volumeEvents) != 1 || r.volumeEvents[0] != 50 {
		t.Fatalf("volume events = %v, want one 50 percent update", r.volumeEvents)
	}
}
