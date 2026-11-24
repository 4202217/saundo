package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/engine"
)

// TestConnectionLatencyFloorWarns pins the setting guard: a HomePod session
// below the measured 200 ms floor carries the banner, the amber note, and the
// value, while the counters stay untouched.
func TestConnectionLatencyFloorWarns(t *testing.T) {
	a := previewApp()
	a.page = pageConnection
	a.snap.LatencyMS = 95
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		locales.SettingsConnectionLatencyWarn(95, 200),
		" · " + locales.SettingsConnectionBelowFloor(200),
		"95 ms",
	} {
		if !hasExactText(tt, want) {
			t.Fatalf("low-latency session is missing %q; texts: %q", want, tt.Texts())
		}
	}

	a.snap.LatencyMS = 200
	tt = ui.NewTester(a.settingsView, 720, 500)
	if hasExactText(tt, locales.SettingsConnectionLatencyWarn(200, 200)) {
		t.Fatalf("a safe latency still warns; texts: %q", tt.Texts())
	}
	if !hasExactText(tt, " · "+locales.SettingsConnectionFloor(200)) {
		t.Fatalf("the safe card lost its floor note; texts: %q", tt.Texts())
	}
}

// TestConnectionLatencyFloorUnknownFamily pins fail-open for the guard: a
// receiver with no measured family gets no invented floor and no banner.
func TestConnectionLatencyFloorUnknownFamily(t *testing.T) {
	a := previewApp()
	a.page = pageConnection
	a.snap.LatencyMS = 95
	d := device.Device{ID: "manual:living", Name: "Living Room", Online: true}
	a.snap.Devices = []device.Device{d}
	a.snap.ActiveTargets = []string{d.ID}
	a.snap.Targets = []engine.TargetControl{{
		ID: d.ID, Name: d.Name, Channel: "stereo", State: engine.TargetMember,
	}}
	a.snap.SelectedID = d.ID
	a.selID = d.ID

	tt := ui.NewTester(a.settingsView, 720, 500)
	if hasExactText(tt, locales.SettingsConnectionLatencyWarn(95, 0)) {
		t.Fatalf("an unknown family got a floor warning; texts: %q", tt.Texts())
	}
	if hasExactText(tt, " · "+locales.SettingsConnectionBelowFloor(0)) {
		t.Fatalf("an unknown family got a floor note; texts: %q", tt.Texts())
	}
}

func TestConnectionTrafficText(t *testing.T) {
	cases := []struct {
		bps  float64
		want string
	}{
		{182000, "182 KB/s"},
		{364000, "364 KB/s"},
		{7500, "7.5 KB/s"},
		{0, "0.0 KB/s"},
	}
	for _, tc := range cases {
		if got := connectionTrafficText(tc.bps); got != tc.want {
			t.Errorf("traffic %v = %q, want %q", tc.bps, got, tc.want)
		}
	}
}

func TestReceiverLatencyFloor(t *testing.T) {
	cases := []struct {
		dev  device.Device
		want int
	}{
		{device.Device{Model: "AudioAccessory5,1"}, 200},
		{device.Device{Model: "AudioAccessory6,1"}, 200},
		{device.Device{Model: "Mac14,2"}, 250},
		{device.Device{Model: "AppleTV6,2"}, 0},
		{device.Device{}, 0},
	}
	for _, tc := range cases {
		if got := receiverLatencyFloorMS(tc.dev); got != tc.want {
			t.Errorf("floor for %q = %d, want %d", tc.dev.Model, got, tc.want)
		}
	}
}

func TestConnectionEventSummary(t *testing.T) {
	now := time.Unix(1000, 0)
	buckets := []engine.StreamBucket{
		{At: now.Add(-3 * time.Second), Underruns: 2},
		{At: now.Add(-2 * time.Second), Retransmits: 5},
		{At: now.Add(-time.Second), Valid: true},
	}
	if got, want := connectionEventSummary(buckets, now), locales.SettingsConnectionRecentRetr(2, 5); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	buckets[2] = engine.StreamBucket{At: now.Add(-time.Second), Underruns: 3}
	if got, want := connectionEventSummary(buckets, now), locales.SettingsConnectionRecentUnder(1, 3); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if got := connectionEventSummary(nil, now); got != "" {
		t.Fatalf("an empty window produced %q", got)
	}
}

func TestConnectionStripSummaryAndIdle(t *testing.T) {
	old := previewStream
	defer func() { previewStream = old }()
	now := time.Now()
	previewStream = func(dst []engine.StreamBucket) []engine.StreamBucket {
		if len(dst) < engine.StreamBucketCount {
			return nil
		}
		for i := 0; i < engine.StreamBucketCount; i++ {
			dst[i] = engine.StreamBucket{At: now.Add(-time.Duration(engine.StreamBucketCount-1-i) * time.Second), Valid: true, HeadMS: 34}
		}
		dst[50] = engine.StreamBucket{At: now.Add(-9 * time.Second), Valid: true, HeadMS: 0, Underruns: 4}
		dst[55] = engine.StreamBucket{At: now.Add(-4 * time.Second), Valid: true, HeadMS: 0, Idle: true}
		return dst[:engine.StreamBucketCount]
	}
	a := previewApp()
	a.page = pageConnection
	tt := ui.NewTester(a.settingsView, 720, 500)
	found := false
	for _, text := range tt.Texts() {
		if strings.Contains(text, "欠载 4 次") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the strip summary is missing; texts: %q", tt.Texts())
	}
	if !hasExactText(tt, locales.SettingsConnectionHeadroomIdle()) {
		t.Fatalf("the idle legend is missing; texts: %q", tt.Texts())
	}
}
