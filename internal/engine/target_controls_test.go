package engine

import (
	"testing"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// setChannel stores a receiver's channel preference the way the settings
// command would.
func setChannel(t *testing.T, h *harness, key, channel string) {
	t.Helper()
	entry, ok := h.e.reg.Get(key)
	if !ok {
		entry = receivers.Entry{Key: key, Kind: receivers.KindDiscovered}
	}
	entry.Channel = channel
	h.e.reg.Put(entry)
}

// connectTwo starts one cohort with the harness receiver and the second
// target, and leaves both streaming.
func connectTwo(t *testing.T) (*harness, device.Device) {
	t.Helper()
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()
	return h, b
}

// TestBuildPlanTargetVolumeAndChannel pins the plan data: a target's own
// volume and channel ride the plan, an unset volume stays unset so launch
// adoption can run, and a stereo pair is forced to stereo.
func TestBuildPlanTargetVolumeAndChannel(t *testing.T) {
	h := newHarness(t)
	pair := testPair("P", "AudioAccessory5,1")
	single := testSingle("AA:02", "192.0.2.12", "AppleTV6,2")
	h.e.setDevices([]device.Device{pair, single})
	setChannel(t, h, pair.ID, receivers.ChannelLeft)
	setChannel(t, h, single.ID, receivers.ChannelRight)
	h.e.setsValue.VolumeByDevice = map[string]float64{pair.ID: 64}

	p, err := h.e.buildPlan([]string{pair.ID, single.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(p.targets))
	}
	pt, st := p.targets[0], p.targets[1]
	if !pt.volumeSet || pt.volume != 64 || pt.channel != sender.ChannelStereo {
		t.Fatalf("pair target = volume %v set %v channel %v, want 64 set and stereo", pt.volume, pt.volumeSet, pt.channel)
	}
	if st.volumeSet || st.channel != sender.ChannelRight {
		t.Fatalf("single target = set %v channel %v, want unset and right", st.volumeSet, st.channel)
	}
	for i, m := range p.members[:2] {
		if m.targetKey != pair.ID || !m.volumeSet || m.volume != 64 || m.channel != sender.ChannelStereo {
			t.Fatalf("pair member %d = %+v", i, m)
		}
	}
	last := p.members[2]
	if last.targetKey != single.ID || last.volumeSet || last.channel != sender.ChannelRight {
		t.Fatalf("single member = %+v", last)
	}
}

// TestSnapshotTargetControls pins the live projection: one entry per target in
// plan order, each from its own settings and registry entry.
func TestSnapshotTargetControls(t *testing.T) {
	h, b := connectTwo(t)
	h.e.setsValue.VolumeByDevice = map[string]float64{testDeviceID: 30}
	setChannel(t, h, b.ID, receivers.ChannelLeft)
	h.e.publish()

	snap := h.e.Snapshot()
	if len(snap.Targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(snap.Targets))
	}
	first, ok := snap.Target(testDeviceID)
	if !ok || first.Name != "客厅 HomePod" || first.Volume != 30 || first.Channel != "stereo" || first.Stereo {
		t.Fatalf("primary controls = %+v ok=%v", first, ok)
	}
	second, ok := snap.Target(b.ID)
	if !ok || second.Name != "卧室 HomePod" || second.Volume != h.e.setsValue.Volume || second.Channel != "left" {
		t.Fatalf("second controls = %+v ok=%v", second, ok)
	}
	if _, ok := snap.Target("missing"); ok {
		t.Fatal("Target returned an unknown key")
	}
}

// TestSetTargetVolumeAppliesOnlyItsTarget pins the per-target command: the
// map and the transport both carry only the named target, and the primary's
// legacy value does not move.
func TestSetTargetVolumeAppliesOnlyItsTarget(t *testing.T) {
	h, b := connectTwo(t)
	primaryVolume := h.e.setsValue.Volume
	h.e.handleCommand(command{kind: cmdTargetVolume, deviceID: b.ID, value: 30})

	if got := h.e.setsValue.VolumeByDevice[b.ID]; got != 30 {
		t.Fatalf("target volume = %f, want 30", got)
	}
	if _, ok := h.e.setsValue.VolumeByDevice[testDeviceID]; ok {
		t.Fatal("the other target's volume moved")
	}
	if got := h.fake.targetVolumes[b.ID]; got != 30 {
		t.Fatalf("transport target volume = %f, want 30", got)
	}
	if len(h.fake.targetVolumes) != 1 {
		t.Fatalf("transport applied to %v, want only the target", h.fake.targetVolumes)
	}
	if h.e.setsValue.Volume != primaryVolume {
		t.Fatalf("primary volume = %f, want %f", h.e.setsValue.Volume, primaryVolume)
	}

	h.e.handleCommand(command{kind: cmdTargetVolume, deviceID: b.ID, value: 140})
	if got := h.e.setsValue.VolumeByDevice[b.ID]; got != 100 {
		t.Fatalf("clamped volume = %f, want 100", got)
	}
}

// TestReceiverVolumeEventMapsToItsTarget pins the hardware knob report: the
// member callback tags its target, and the other target keeps its value.
func TestReceiverVolumeEventMapsToItsTarget(t *testing.T) {
	h, b := connectTwo(t)
	h.e.onReceiverVolume(b.ID, 44)

	if got := h.e.setsValue.VolumeByDevice[b.ID]; got != 44 {
		t.Fatalf("target volume = %f, want 44", got)
	}
	if _, ok := h.e.setsValue.VolumeByDevice[testDeviceID]; ok {
		t.Fatal("the report moved the other target's volume")
	}
	if h.e.setsValue.Volume == 44 {
		t.Fatal("a secondary report moved the primary volume")
	}
}

// TestInitialVolumeAdoptedPerTarget pins launch adoption for every target
// without a remembered value, including a target added later.
func TestInitialVolumeAdoptedPerTarget(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.fake.initialByKey = map[string]float64{testDeviceID: 37}
	h.launch()
	if got := h.e.setsValue.VolumeByDevice[testDeviceID]; got != 37 {
		t.Fatalf("primary adopted volume = %f, want 37", got)
	}

	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.fake.initialByKey = map[string]float64{b.ID: 61}
	h.launch()
	if got := h.e.setsValue.VolumeByDevice[b.ID]; got != 61 {
		t.Fatalf("added target adopted volume = %f, want 61", got)
	}
	if got := h.e.setsValue.VolumeByDevice[testDeviceID]; got != 37 {
		t.Fatalf("primary volume changed on add: %f, want 37", got)
	}
	if got := h.e.setsValue.Volume; got != 37 {
		t.Fatalf("primary stored volume = %f, want 37", got)
	}
}

// TestReceiverChannelPatchSavesAndAppliesLive pins the channel command: the
// registry keeps the value, the target's members take it at once, and no
// session rebuild happens.
func TestReceiverChannelPatchSavesAndAppliesLive(t *testing.T) {
	h, b := connectTwo(t)
	sessions := h.sessions
	left := receivers.ChannelLeft
	h.e.handleUpdateReceiver(b.ID, receivers.Patch{Channel: &left})

	entry, ok := h.e.reg.Get(b.ID)
	if !ok || entry.Channel != receivers.ChannelLeft {
		t.Fatalf("saved channel = %q ok=%v, want left", entry.Channel, ok)
	}
	if got := h.fake.targetChannels[b.ID]; got != sender.ChannelLeft {
		t.Fatalf("live channel = %v, want left", got)
	}
	if _, ok := h.fake.targetChannels[testDeviceID]; ok {
		t.Fatal("the channel patch reached the other target")
	}
	if h.sessions != sessions {
		t.Fatalf("sessions = %d, want no rebuild (%d)", h.sessions, sessions)
	}
	if got, _ := h.e.Snapshot().Target(b.ID); got.Channel != "left" {
		t.Fatalf("snapshot channel = %q, want left", got.Channel)
	}
}

// TestReceiverChannelPatchOnPairStaysStereo pins the pair rule: a non-stereo
// side is ignored, and the live pair keeps its forced stereo map.
func TestReceiverChannelPatchOnPairStaysStereo(t *testing.T) {
	h := newHarness(t)
	pair := testPair("P", "AudioAccessory5,1")
	h.e.setDevices([]device.Device{pair})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: pair.ID})
	if h.fake == nil {
		t.Fatalf("pair session did not open: %+v", h.e.Snapshot())
	}
	h.launch()

	left := receivers.ChannelLeft
	h.e.handleUpdateReceiver(pair.ID, receivers.Patch{Channel: &left})
	entry, ok := h.e.reg.Get(pair.ID)
	if !ok || entry.Channel == receivers.ChannelLeft {
		t.Fatalf("pair saved channel = %q ok=%v, want not left", entry.Channel, ok)
	}
	if got, _ := h.e.Snapshot().Target(pair.ID); got.Channel != "stereo" {
		t.Fatalf("pair snapshot channel = %q, want stereo", got.Channel)
	}
}

// TestCohortVolumeEventCarriesItsTarget pins the member callback: a knob
// report is tagged with the reporting member's target, so a pair reports its
// shared key and a single its own.
func TestCohortVolumeEventCarriesItsTarget(t *testing.T) {
	c := buildTestCohort(2)
	c.members[0].targetKey = "AA:01"
	c.members[1].targetKey = "stereo:P"
	var got []string
	c.onVolume = func(key string, pct float64) { got = append(got, key) }

	c.memberEvents(0).VolumeChanged(30)
	c.memberEvents(1).VolumeChanged(40)
	if len(got) != 2 || got[0] != "AA:01" || got[1] != "stereo:P" {
		t.Fatalf("volume callbacks = %v, want the members' own target keys", got)
	}
}

// TestCohortTargetChannelReachesEveryMember pins that a pair's channel change
// reaches both senders while another target stays untouched.
func TestCohortTargetChannelReachesEveryMember(t *testing.T) {
	c := buildTestCohort(3)
	c.members[0].targetKey = "stereo:P"
	c.members[1].targetKey = "stereo:P"
	c.members[2].targetKey = "AA:03"

	c.SetTargetChannel("stereo:P", sender.ChannelLeft)
	for i, m := range c.members[:2] {
		if got := m.sender.Channel(); got != sender.ChannelLeft {
			t.Fatalf("pair member %d channel = %v, want left", i, got)
		}
	}
	if got := c.members[2].sender.Channel(); got != sender.ChannelStereo {
		t.Fatalf("other target channel = %v, want stereo", got)
	}
}

// TestTargetControlsCarryMeasuredHealth pins the per-target fold: each target
// reports its own members' retransmits, sent blocks, and underruns, and a
// mismatched stats slice publishes no health instead of guessing.
func TestTargetControlsCarryMeasuredHealth(t *testing.T) {
	h := newHarness(t)
	b := advertiseBoth(h)
	h.connect()
	h.launch()
	h.e.handleCommand(command{kind: cmdConnect, deviceID: b.ID})
	h.launch()
	if len(h.e.plan.members) != 2 {
		t.Fatalf("members = %d, want two", len(h.e.plan.members))
	}
	h.fake.stats = []sender.Stats{
		{Retransmits: 3, SentBlocks: 100, Underruns: 1},
		{Retransmits: 5, SentBlocks: 200, Underruns: 0},
	}
	h.e.publish()
	snap := h.e.Snapshot()
	first, _ := snap.Target(testDeviceID)
	if got, want := first.Health, (TargetHealth{Retransmits: 3, SentBlocks: 100, RecentUnderruns: 1}); got != want {
		t.Fatalf("first target health = %+v, want %+v", got, want)
	}
	second, _ := snap.Target(b.ID)
	if got, want := second.Health, (TargetHealth{Retransmits: 5, SentBlocks: 200}); got != want {
		t.Fatalf("second target health = %+v, want %+v", got, want)
	}

	// One stat for two members cannot be attributed; fail closed.
	h.fake.stats = h.fake.stats[:1]
	h.e.publish()
	snap = h.e.Snapshot()
	first, _ = snap.Target(testDeviceID)
	if got := first.Health; got != (TargetHealth{}) {
		t.Fatalf("mismatched stats leaked health: %+v", got)
	}
}
