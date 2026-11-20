package engine

import (
	"testing"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// comboSecondDevice is the second online receiver a combo links: two HomePods
// in different rooms.
func comboSecondDevice() device.Device {
	return device.Device{
		ID: "AA:BB:CC:DD:EE:02", Name: "书房的 Apple TV", Host: "192.0.2.11", Port: 7000,
		Model: "AudioAccessory6,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true,
	}
}

func comboDevices() []device.Device {
	return []device.Device{onlineDevice(), comboSecondDevice()}
}

// comboKeyOf returns the registry's single combo key, "" when none.
func comboKeyOf(h *harness) string {
	for _, e := range h.e.reg.Entries {
		if e.IsCombo() {
			return e.Key
		}
	}
	return ""
}

// linkCombo links the harness's two receivers and returns the combo key.
func linkCombo(t *testing.T) (*harness, string) {
	t.Helper()
	h := newHarness(t)
	h.e.setDevices(comboDevices())
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, comboSecondDevice().ID}})
	key := comboKeyOf(h)
	if key == "" {
		t.Fatal("the combo was not created")
	}
	return h, key
}

func TestLinkComboSynthesizesOneRow(t *testing.T) {
	h, key := linkCombo(t)
	entry, ok := h.e.reg.Get(key)
	if !ok || entry.Kind != receivers.KindCombo {
		t.Fatalf("combo entry = %+v ok=%v", entry, ok)
	}
	if len(entry.Members) != 2 || entry.Members[0].ID != testDeviceID || entry.Members[1].ID != "AA:BB:CC:DD:EE:02" {
		t.Fatalf("members = %+v, want left then right", entry.Members)
	}
	if entry.Name != "客厅 HomePod + 书房的 Apple TV" {
		t.Fatalf("name = %q", entry.Name)
	}
	snap := h.e.Snapshot()
	var comboView, memberViews int
	for _, v := range snap.Receivers {
		if v.Key == key {
			comboView++
			if !v.Stereo || !v.Device.IsStereoGroup() || len(v.Device.Members) != 2 {
				t.Fatalf("combo view is not a stereo group: %+v", v)
			}
			continue
		}
		if v.Key == testDeviceID || v.Key == "AA:BB:CC:DD:EE:02" {
			memberViews++
		}
	}
	if comboView != 1 || memberViews != 0 {
		t.Fatalf("snapshot rows: combo=%d members=%d, want the combo alone", comboView, memberViews)
	}
}

func TestLinkComboEndsAMembersSession(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices(comboDevices())
	h.e.handleCommand(command{kind: cmdConnect, deviceID: testDeviceID})
	h.launch()
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, comboSecondDevice().ID}})
	if h.e.plan.live() || h.e.Snapshot().State != sender.StateIdle {
		t.Fatal("linking a playing receiver must end its session")
	}
	if comboKeyOf(h) == "" {
		t.Fatal("the combo was not created")
	}
}

func TestLinkComboRejectsInvalidSides(t *testing.T) {
	h, key := linkCombo(t)
	third := testSingle("AA:BB:CC:DD:EE:03", "192.0.2.12", "AudioAccessory5,1")
	h.e.setDevices(append(comboDevices(), third))
	// A receiver already inside a combo refuses a second link.
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, third.ID}})
	if snap := h.e.Snapshot(); snap.ErrorKey != ErrComboInvalid {
		t.Fatalf("linked an already-comboed receiver: error=%q", snap.ErrorKey)
	}
	if n := comboKeyOf(h); n != key {
		t.Fatal("a second combo was created")
	}
	// One key cannot play both sides.
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{third.ID, third.ID}})
	if snap := h.e.Snapshot(); snap.ErrorKey != ErrComboInvalid {
		t.Fatalf("linked one receiver to itself: error=%q", snap.ErrorKey)
	}
}

func TestLinkComboRejectsAirPlay1(t *testing.T) {
	h := newHarness(t)
	ap1 := device.Device{
		ID: "AA:BB:CC:DD:EE:04", Name: "Express", Host: "192.0.2.14", Port: 5000,
		Model: "AirPortExpress", Auth: device.AuthNone, AirPlay2: false, Online: true,
	}
	h.e.setDevices([]device.Device{onlineDevice(), ap1})
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, ap1.ID}})
	if snap := h.e.Snapshot(); snap.ErrorKey != ErrComboInvalid {
		t.Fatalf("linked an AirPlay 1 receiver: error=%q", snap.ErrorKey)
	}
	if comboKeyOf(h) != "" {
		t.Fatal("a combo entry was created")
	}
}

func TestComboPlanPlaysLeftAndRight(t *testing.T) {
	h, key := linkCombo(t)
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	h.launch()
	if snap := h.e.Snapshot(); snap.ErrorKey != "" {
		t.Fatalf("the combo connect failed: %q", snap.ErrorKey)
	}
	if len(h.plan.members) != 2 {
		t.Fatalf("plan members = %d, want the combo's two receivers", len(h.plan.members))
	}
	if h.plan.members[0].device.ID != testDeviceID || h.plan.members[1].device.ID != "AA:BB:CC:DD:EE:02" {
		t.Fatalf("member order = %q %q, want left then right", h.plan.members[0].device.ID, h.plan.members[1].device.ID)
	}
	if h.plan.members[0].channel != sender.ChannelLeft || h.plan.members[1].channel != sender.ChannelRight {
		t.Fatalf("channels = %v %v, want left then right", h.plan.members[0].channel, h.plan.members[1].channel)
	}
	if h.plan.members[0].groupUUID != "" || h.plan.members[0].timingPeers != nil {
		t.Fatal("a combo must not send the pair's group UUID or SETPEERS")
	}
	if h.plan.clock.role != ptpFollowers {
		t.Fatalf("clock role = %v, want a follower set over both members", h.plan.clock.role)
	}
	if got := h.plan.clock.followerHosts; len(got) != 2 || got[0] != "192.0.2.10" || got[1] != "192.0.2.11" {
		t.Fatalf("follower hosts = %v, want both combo members", got)
	}
	if snap := h.e.Snapshot(); !snap.Targets[0].Split || !snap.Targets[0].Stereo {
		t.Fatalf("combo target control = %+v, want a locked split", snap.Targets[0])
	}
}

func TestSwapComboSidesFlipsMembers(t *testing.T) {
	h, key := linkCombo(t)
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	h.launch()
	h.e.handleCommand(command{kind: cmdSwapComboSides, deviceID: key})
	entry, ok := h.e.reg.Get(key)
	if !ok || entry.Members[0].ID != "AA:BB:CC:DD:EE:02" || entry.Members[1].ID != testDeviceID {
		t.Fatalf("members after swap = %+v", entry.Members)
	}
	// The live window keeps its device order, so each side takes the
	// opposite channel in place: member 0 was left and now plays right.
	if got := h.fake.memberChannels[key+"\x000"]; got != sender.ChannelRight {
		t.Fatalf("member 0 channel = %v, want right", got)
	}
	if got := h.fake.memberChannels[key+"\x001"]; got != sender.ChannelLeft {
		t.Fatalf("member 1 channel = %v, want left", got)
	}
	if h.sessions != 1 {
		t.Fatalf("sessions = %d, want the swap to hot-apply without a rebuild", h.sessions)
	}
}

func TestUnlinkComboRestoresMembers(t *testing.T) {
	h, key := linkCombo(t)
	h.e.setsValue.VolumeByDevice = map[string]float64{key: 42}
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	h.launch()
	h.e.handleCommand(command{kind: cmdUnlinkCombo, deviceID: key})
	if comboKeyOf(h) != "" {
		t.Fatal("the combo entry survived the unlink")
	}
	if _, ok := h.e.setsValue.VolumeByDevice[key]; ok {
		t.Fatal("the combo's remembered volume survived the unlink")
	}
	if h.e.plan.live() || !h.fake.stopped {
		t.Fatal("unlinking a live combo must end its session")
	}
	snap := h.e.Snapshot()
	var members int
	for _, v := range snap.Receivers {
		if v.Key == testDeviceID || v.Key == "AA:BB:CC:DD:EE:02" {
			members++
		}
	}
	if members != 2 {
		t.Fatalf("member rows = %d, want both restored", members)
	}
}

func TestRemoveMemberUnlinksCombo(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.setDevices(comboDevices())
	h.e.handleCommand(command{kind: cmdAddReceiver, spec: receivers.Spec{
		Name: "书房", Type: receivers.TypeAirPlay2, Host: "192.0.2.33", Port: 7000,
	}})
	manual := manualKey(h)
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, manual}})
	key := comboKeyOf(h)
	if key == "" {
		t.Fatal("the combo was not created")
	}
	h.e.handleCommand(command{kind: cmdRemoveReceiver, deviceID: manual})
	if comboKeyOf(h) != "" {
		t.Fatal("removing a member must unlink its combo")
	}
	if _, ok := h.e.reg.Get(testDeviceID); !ok {
		t.Fatal("the surviving member's entry was lost")
	}
}

func TestComboAutoConnectLaunchesBothMembers(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices(comboDevices())
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, comboSecondDevice().ID}})
	key := comboKeyOf(h)
	// The persisted flag the run loop reads at startup, not a toggle command:
	// toggling auto-connect ends the launch phase by design. The loop already
	// consumed this test's discovery event, so reset the phase it ended.
	entry, _ := h.e.reg.Get(key)
	entry.AutoConnect = true
	h.e.reg.Put(entry)
	h.e.autoTried = false
	if !h.e.maybeAutoConnect() {
		t.Fatal("the flagged combo did not launch")
	}
	if h.e.plan.targets[0].key != key || len(h.plan.members) != 2 {
		t.Fatalf("plan = %d members for %q, want the combo's two", len(h.plan.members), key)
	}
	if h.plan.members[0].channel != sender.ChannelLeft || h.plan.members[1].channel != sender.ChannelRight {
		t.Fatalf("launched channels = %v %v", h.plan.members[0].channel, h.plan.members[1].channel)
	}
}

func TestComboWithOfflineMemberRefusesConnect(t *testing.T) {
	h, key := linkCombo(t)
	offline := comboSecondDevice()
	offline.Online = false
	h.e.setDevices([]device.Device{onlineDevice(), offline})
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	if snap := h.e.Snapshot(); snap.ErrorKey != ErrStereoMembersOffline {
		t.Fatalf("error = %q, want stereoMembersOffline", snap.ErrorKey)
	}
	if h.e.plan.live() {
		t.Fatal("an offline member must refuse the combo session")
	}
}

func TestComboVolumeReportConvergesTheMembers(t *testing.T) {
	h, key := linkCombo(t)
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	h.launch()
	h.e.onReceiverVolume(key, 40)
	if got := h.fake.targetVolumes[key]; got != 40 {
		t.Fatalf("the report was not pushed to the target's members: %v", got)
	}
}

// TestLinkComboClearsAPendingPin pins the teardown's prompt hygiene: linking
// over a session whose PIN dialog is up ends it without leaving the dialog on
// screen for a session that no longer exists.
func TestLinkComboClearsAPendingPin(t *testing.T) {
	h := newHarness(t)
	h.e.setDevices(comboDevices())
	h.e.handleCommand(command{kind: cmdConnect, deviceID: testDeviceID})
	h.e.pinRequired, h.e.pinDevice = true, "客厅 HomePod"
	h.e.notice = SessionNotice{Kind: NoticeMemberLost, DeviceID: testDeviceID, Name: "客厅 HomePod", Remaining: 3}
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, comboSecondDevice().ID}})
	snap := h.e.Snapshot()
	if snap.PinRequired || snap.PinDevice != "" {
		t.Fatalf("the PIN dialog survived the link: %v %q", snap.PinRequired, snap.PinDevice)
	}
	if snap.Notice.Kind != NoticeNone {
		t.Fatal("the session notice survived the link")
	}
}

// TestDelayRebuildRefusesAnOfflineMember pins the per-row delay guard: the
// rebuild opens a new cohort for the whole set, so an offline member refuses
// the apply and the running cohort keeps playing, the way the global latency
// apply already does.
func TestDelayRebuildRefusesAnOfflineMember(t *testing.T) {
	h, key := linkCombo(t)
	delay := 150
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	h.launch()
	sessions := h.sessions
	offline := comboSecondDevice()
	offline.Online = false
	h.e.setDevices([]device.Device{onlineDevice(), offline})
	h.e.handleCommand(command{kind: cmdUpdateReceiver, deviceID: key, patch: receivers.Patch{DelayMS: &delay}})
	if h.sessions != sessions {
		t.Fatal("the delay rebuild tore down a cohort with an offline member")
	}
	if snap := h.e.Snapshot(); snap.ErrorKey != ErrLatencyOffline {
		t.Fatalf("error = %q, want latencyOffline", snap.ErrorKey)
	}
}

// TestLinkComboMovesTheLaunchFlag pins the launch intent's home: a flagged
// member hands its auto-connect flag to the combo, and the member rows keep
// none, so a launch acts on the combo rather than the hidden members.
func TestLinkComboMovesTheLaunchFlag(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.setDevices(comboDevices())
	for _, key := range []string{testDeviceID, comboSecondDevice().ID} {
		if e, ok := h.e.reg.Get(key); ok {
			e.AutoConnect = true
			h.e.reg.Put(e)
		}
	}
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, comboSecondDevice().ID}})
	key := comboKeyOf(h)
	if key == "" {
		t.Fatal("the combo was not created")
	}
	entry, _ := h.e.reg.Get(key)
	if !entry.AutoConnect {
		t.Fatal("the combo did not inherit the members' launch flag")
	}
	for _, memberKey := range []string{testDeviceID, comboSecondDevice().ID} {
		if m, _ := h.e.reg.Get(memberKey); m.AutoConnect {
			t.Fatalf("member %s kept its launch flag", memberKey)
		}
	}
	if got := receivers.AutoTargets(h.e.reg); len(got) != 1 || got[0] != key {
		t.Fatalf("launch set = %v, want the combo alone", got)
	}
}

// TestUnlinkComboReturnsTheLaunchFlag pins the split: a combo that started the
// app hands its flag back to both members, which become standalone rows.
func TestUnlinkComboReturnsTheLaunchFlag(t *testing.T) {
	h, key := linkCombo(t)
	entry, _ := h.e.reg.Get(key)
	entry.AutoConnect = true
	h.e.reg.Put(entry)
	h.e.handleCommand(command{kind: cmdUnlinkCombo, deviceID: key})
	for _, memberKey := range []string{testDeviceID, comboSecondDevice().ID} {
		m, ok := h.e.reg.Get(memberKey)
		if !ok || !m.AutoConnect {
			t.Fatalf("member %s did not get the flag back", memberKey)
		}
	}
}

// TestComboLinkEndsAMembersAutoConnect covers the reported bug: two flagged
// receivers linked into a combo used to launch themselves while the combo
// stayed idle. After the link the launch set is the combo alone.
func TestComboLinkEndsAMembersAutoConnect(t *testing.T) {
	h := newHarnessWithRegistry(t)
	h.e.setDevices(comboDevices())
	for _, key := range []string{testDeviceID, comboSecondDevice().ID} {
		if e, ok := h.e.reg.Get(key); ok {
			e.AutoConnect = true
			h.e.reg.Put(e)
		}
	}
	h.e.handleCommand(command{kind: cmdLinkReceivers, ids: []string{testDeviceID, comboSecondDevice().ID}})
	key := comboKeyOf(h)
	h.e.autoTried = false
	if !h.e.maybeAutoConnect() {
		t.Fatal("the launch did not start")
	}
	if len(h.e.plan.targets) != 1 || h.e.plan.targets[0].key != key {
		t.Fatalf("launched %v, want the combo alone", h.e.plan.keys())
	}
}
