package engine

import (
	"testing"
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/ring"
)

func TestDiagnosticsSingleAggregates(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	lastSync := h.now.Add(-250 * time.Millisecond)
	h.fake.stats = []sender.Stats{{
		Retransmits: 4,
		LateTicks:   2,
		Underruns:   1,
		SentBlocks:  120,
		LastSync:    lastSync,
		BufferMS:    42,
		BufferValid: true,
		WiFi: sender.WiFiMetrics{
			Valid: true, RSSI: -52, SSID: "HomeNet", Band: sender.WiFiBand5,
		},
	}}
	h.launch()
	h.advance(2 * time.Minute)
	h.e.publish()

	d := h.e.Snapshot().Diagnostics
	if d.Group != GroupSingle || d.GroupSize != 1 {
		t.Fatalf("group = %v/%d, want single/1", d.Group, d.GroupSize)
	}
	if !d.HasStats {
		t.Fatal("sender stats read as absent")
	}
	if d.Reconnects != 0 {
		t.Fatalf("reconnects = %d, want 0", d.Reconnects)
	}
	if d.SessionUptime != 2*time.Minute {
		t.Fatalf("uptime = %s, want 2m", d.SessionUptime)
	}
	if !d.SyncValid || d.SyncAge != 2*time.Minute+250*time.Millisecond {
		t.Fatalf("sync = %v/%s, want 2m250ms", d.SyncValid, d.SyncAge)
	}
	if d.Retransmits != 4 || d.LateTicks != 2 || d.Underruns != 1 {
		t.Fatalf("counters = %d/%d/%d, want 4/2/1", d.Retransmits, d.LateTicks, d.Underruns)
	}
	if !d.BufferValid || d.SendBufferMS != 42 {
		t.Fatalf("buffer = %v/%f, want 42", d.BufferValid, d.SendBufferMS)
	}
	if !d.WiFi.Valid || d.WiFi.RSSI != -52 || d.WiFi.Band != sender.WiFiBand5 {
		t.Fatalf("WiFi = %+v", d.WiFi)
	}
	if !d.Degraded {
		t.Fatal("an underrun did not mark the session degraded")
	}
}

func TestDiagnosticsStereoTakesWorstAndSums(t *testing.T) {
	h := newHarness(t)
	group := device.Device{
		ID: device.StereoGroupID("tsid"), Name: "卧室 HomePods", GroupName: "卧室",
		StereoID: "tsid", Online: true,
		Members: []device.Device{
			{ID: "AA:BB:CC:DD:EE:01", Name: "L", Host: "192.0.2.10", Port: 7000,
				Auth: device.AuthHAPTransient, StereoID: "tsid", Online: true},
			{ID: "AA:BB:CC:DD:EE:02", Name: "R", Host: "192.0.2.11", Port: 7000,
				Auth: device.AuthHAPTransient, StereoID: "tsid", Online: true},
		},
	}
	h.e.setDevices([]device.Device{group})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: group.ID})
	h.fake.stats = []sender.Stats{
		{
			Retransmits: 2, Underruns: 1, BufferMS: 42, BufferValid: true,
			LastSync: h.now.Add(-100 * time.Millisecond),
			WiFi:     sender.WiFiMetrics{Valid: true, RSSI: -62, Band: sender.WiFiBand5},
		},
		{
			Retransmits: 3, LateTicks: 3, Underruns: 2, BufferMS: 8, BufferValid: true,
			LastSync: h.now.Add(-300 * time.Millisecond),
			WiFi:     sender.WiFiMetrics{Valid: true, RSSI: -78, Band: sender.WiFiBand24},
		},
	}
	h.launch()
	h.advance(time.Minute)
	h.e.publish()

	d := h.e.Snapshot().Diagnostics
	if d.Group != GroupStereo || d.GroupSize != 2 || d.GroupName != "卧室" {
		t.Fatalf("group = %v/%d/%q, want stereo/2/卧室", d.Group, d.GroupSize, d.GroupName)
	}
	if !d.WiFi.Valid || d.WiFi.RSSI != -78 || d.WiFi.Band != sender.WiFiBand24 {
		t.Fatalf("WiFi = %+v, want the worse member", d.WiFi)
	}
	if d.Retransmits != 5 {
		t.Fatalf("retransmits = %d, want the sum 5", d.Retransmits)
	}
	if d.LateTicks != 3 {
		t.Fatalf("late ticks = %d, want the worst 3", d.LateTicks)
	}
	if d.Underruns != 3 {
		t.Fatalf("underruns = %d, want the sum 3", d.Underruns)
	}
	if !d.BufferValid || d.SendBufferMS != 8 {
		t.Fatalf("buffer = %v/%f, want the worst 8", d.BufferValid, d.SendBufferMS)
	}
	if !d.SyncValid || d.SyncAge != time.Minute+300*time.Millisecond {
		t.Fatalf("sync = %v/%s, want the oldest member's age", d.SyncValid, d.SyncAge)
	}
	if !d.Degraded {
		t.Fatal("the pair did not degrade")
	}
}

func TestDiagnosticsStereoOverflowSumsMembers(t *testing.T) {
	h := newHarness(t)
	h.rings = []*ring.Buffer{ring.New(4), ring.New(4)}
	group := device.Device{
		ID: device.StereoGroupID("tsid"), Name: "HomePods", StereoID: "tsid", Online: true,
		Members: []device.Device{
			{ID: "AA:BB:CC:DD:EE:01", Name: "L", Host: "192.0.2.10", Port: 7000,
				Auth: device.AuthHAPTransient, StereoID: "tsid", Online: true},
			{ID: "AA:BB:CC:DD:EE:02", Name: "R", Host: "192.0.2.11", Port: 7000,
				Auth: device.AuthHAPTransient, StereoID: "tsid", Online: true},
		},
	}
	h.e.setDevices([]device.Device{group})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: group.ID})
	h.launch()
	h.e.captureSink.Push(make([]int16, 16))
	h.e.publish()

	snap := h.e.Snapshot()
	if snap.DroppedSamples != 12 {
		t.Fatalf("stability drops = %d, want the max 12", snap.DroppedSamples)
	}
	if snap.Diagnostics.Overflow != 24 {
		t.Fatalf("overflow = %d, want the members' sum 24", snap.Diagnostics.Overflow)
	}
}

func TestDiagnosticsUnknownWhenMemberDidNotSync(t *testing.T) {
	members := []sender.Stats{
		{LastSync: time.Unix(100, 0), BufferValid: true, BufferMS: 20},
		{BufferValid: false},
	}
	d := aggregateDiagnostics(members, GroupStereo, 2, "Pair", 0, 0, 0, time.Unix(101, 0))
	if d.SyncValid {
		t.Fatal("a member without a sync packet still read as synced")
	}
	if d.SyncAge != 0 {
		t.Fatalf("sync age = %s, want zero for unknown", d.SyncAge)
	}
	if d.BufferValid {
		t.Fatal("a member without a buffer still read as valid")
	}
}

func TestDiagnosticsOverflowComesFromSessionDrops(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()
	h.e.sessionDropped = 3
	h.e.dropSessionActive = true
	h.fake.state = sender.StateStreaming
	h.e.publish()

	d := h.e.Snapshot().Diagnostics
	if d.Overflow != 3 {
		t.Fatalf("overflow = %d, want 3", d.Overflow)
	}
	if !d.Degraded {
		t.Fatal("dropped samples did not mark the session degraded")
	}
}

func TestDiagnosticsReconnectCountAndUptimeReset(t *testing.T) {
	h := newHarness(t)
	h.addDevice()
	h.connect()
	h.launch()
	h.advance(5 * time.Minute)
	h.e.publish()
	if got := h.e.Snapshot().Diagnostics.SessionUptime; got != 5*time.Minute {
		t.Fatalf("uptime = %s, want 5m", got)
	}

	h.e.setsValue.Reconnect = true
	h.fake.state = sender.StateIdle
	h.e.onClosed()
	h.e.reconnectAt = h.now
	h.e.attemptReconnect()
	if got := h.e.Snapshot().Diagnostics.Reconnects; got != 1 {
		t.Fatalf("reconnects = %d, want 1", got)
	}
	if got := h.e.Snapshot().Diagnostics.SessionUptime; got != 0 {
		t.Fatalf("uptime after close = %s, want 0", got)
	}
}

func TestDiagnosticsUnknownWhenMemberDidNotReportWiFi(t *testing.T) {
	members := []sender.Stats{
		{WiFi: sender.WiFiMetrics{Valid: true, RSSI: -52, Band: sender.WiFiBand5}},
		{},
	}
	d := aggregateDiagnostics(members, GroupStereo, 2, "Pair", 0, 0, 0, time.Unix(101, 0))
	if d.WiFi.Valid {
		t.Fatal("a member without Wi-Fi data still read as a valid group value")
	}
}
