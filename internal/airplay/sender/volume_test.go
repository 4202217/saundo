package sender

import (
	"math"
	"strings"
	"testing"

	"saundo/internal/airplay/device"
)

func volumeRequests(r *rig) []string {
	var out []string
	for _, p := range r.rx.setParams {
		if strings.HasPrefix(p, "text/parameters\x00") {
			out = append(out, strings.TrimPrefix(p, "text/parameters\x00"))
		}
	}
	return out
}

// TestFirstConnectUnknownVolumeSendsNoRequest pins the fail-closed rule: an
// absent, malformed, non-numeric, or out-of-range initialVolume sends no
// volume request, so the receiver keeps its own level instead of jumping to
// 0 dB.
func TestFirstConnectUnknownVolumeSendsNoRequest(t *testing.T) {
	cases := []struct {
		name string
		set  func(*fakeReceiver)
	}{
		{"absent", func(f *fakeReceiver) { f.omitInitialVolume = true }},
		{"string", func(f *fakeReceiver) { f.initialVolumeString = true }},
		{"bool", func(f *fakeReceiver) { f.initialVolumeBool = true }},
		{"malformed", func(f *fakeReceiver) { f.garbageInfo = true }},
		{"non-dictionary", func(f *fakeReceiver) { f.infoArray = true }},
		{"positive", func(f *fakeReceiver) { f.receiverVolume = 3 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
			c.set(r.rx)
			rb, _ := fillRing(4410)
			r.sender.AttachRing(rb)
			r.start()
			requireLaunched(t, r)

			if got := volumeRequests(r); len(got) != 0 {
				t.Fatalf("volume requests = %q, want none", got)
			}
			if _, ok := r.sender.InitialVolumePercent(); ok {
				t.Fatal("an unknown receiver volume was adopted")
			}
		})
	}
}

func TestFirstConnectMuteSentinelStaysMuted(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.rx.receiverVolume = -144
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	got := volumeRequests(r)
	if len(got) != 1 || got[0] != "volume: -144.000000" {
		t.Fatalf("volume requests = %q, want [volume: -144.000000]", got)
	}
	if pct, ok := r.sender.InitialVolumePercent(); !ok || pct != 0 {
		t.Fatalf("InitialVolumePercent = (%v,%v), want (0,true)", pct, ok)
	}
}

func TestFirstConnectBelowMuteClampsToMute(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.rx.receiverVolume = -1000
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	got := volumeRequests(r)
	if len(got) != 1 || got[0] != "volume: -144.000000" {
		t.Fatalf("volume requests = %q, want [volume: -144.000000]", got)
	}
	if pct, ok := r.sender.InitialVolumePercent(); !ok || pct != 0 {
		t.Fatalf("InitialVolumePercent = (%v,%v), want (0,true)", pct, ok)
	}
}

func TestFirstConnectRealVolumeKeepsFraction(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.rx.initialVolumeRealSet = true
	r.rx.initialVolumeReal = -27.3
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	got := volumeRequests(r)
	if len(got) != 1 || got[0] != "volume: -27.300000" {
		t.Fatalf("volume requests = %q, want [volume: -27.300000]", got)
	}
	if pct, ok := r.sender.InitialVolumePercent(); !ok || pct != 8 {
		t.Fatalf("InitialVolumePercent = (%v,%v), want (8,true)", pct, ok)
	}
}

// TestFirstConnectKnownZeroSendsFullVolume keeps the fix from over-correcting:
// a receiver that really reports 0 dB is known full scale, unlike an unknown
// value, and must still get the explicit request.
func TestFirstConnectKnownZeroSendsFullVolume(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.rx.receiverVolume = 0
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	got := volumeRequests(r)
	if len(got) != 1 || got[0] != "volume: 0.000000" {
		t.Fatalf("volume requests = %q, want [volume: 0.000000]", got)
	}
	if pct, ok := r.sender.InitialVolumePercent(); !ok || pct != 100 {
		t.Fatalf("InitialVolumePercent = (%v,%v), want (100,true)", pct, ok)
	}
}

func TestVolumePollRealVolumeMatchesTheHandshake(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.start()
	r.rx.initialVolumeRealSet = true
	r.rx.initialVolumeReal = -27.3
	r.sender.receiverVol = 42
	r.sender.volQuery = false
	r.volumeEvents = nil
	before := r.rx.requests["GET /info"]
	r.sender.onVolumePoll()
	r.step()

	if got := r.rx.requests["GET /info"]; got <= before {
		t.Fatal("the poll sent no GET /info")
	}
	if len(r.volumeEvents) != 1 || r.volumeEvents[0] != 8 {
		t.Fatalf("volume events = %v, want one 8 percent update", r.volumeEvents)
	}
}

func TestPlistRealOK(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  float64
		ok    bool
	}{
		{"int64", int64(-27), -27, true},
		{"int", -27, -27, true},
		{"uint64", uint64(3), 3, true},
		{"real", -27.3, -27.3, true},
		{"bool true", true, 0, false},
		{"bool false", false, 0, false},
		{"string", "loud", 0, false},
		{"nil", nil, 0, false},
		{"nan", math.NaN(), 0, false},
		{"inf", math.Inf(1), 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := plistRealOK(map[string]any{"v": c.value}, "v")
			if ok != c.ok || (ok && got != c.want) {
				t.Fatalf("plistRealOK(%v) = (%v,%v), want (%v,%v)", c.value, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestVolumeDBInRange(t *testing.T) {
	cases := []struct {
		db   float64
		want float64
		ok   bool
	}{
		{0, 0, true},
		{-27.3, -27.3, true},
		{-144, -144, true},
		{-1000, -144, true},
		{0.1, 0, false},
		{3, 0, false},
		{math.NaN(), 0, false},
		{math.Inf(1), 0, false},
		{math.Inf(-1), 0, false},
	}
	for _, c := range cases {
		got, ok := volumeDBInRange(c.db)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("volumeDBInRange(%v) = (%v,%v), want (%v,%v)", c.db, got, ok, c.want, c.ok)
		}
	}
}
