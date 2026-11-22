package engine

import (
	"errors"
	"strings"
	"testing"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// testPair builds a complete stereo pair with the given key prefix.
func testPair(prefix string, model string) device.Device {
	tsid := "tsid-" + prefix
	return device.Device{
		ID: device.StereoGroupID(tsid), Name: prefix + " Pair", StereoID: tsid, Online: true,
		Model: model, Auth: device.AuthHAPTransient, AirPlay2: true,
		Members: []device.Device{
			{ID: prefix + "-L", Name: prefix + " L", Host: "192.0.2.10", Port: 7000,
				Model: model, Auth: device.AuthHAPTransient, StereoID: tsid, Online: true},
			{ID: prefix + "-R", Name: prefix + " R", Host: "192.0.2.11", Port: 7000,
				Model: model, Auth: device.AuthHAPTransient, StereoID: tsid, Online: true},
		},
	}
}

// testSingle builds a standalone receiver; model "AppleTV6,2" is NTP timed.
func testSingle(id, host, model string) device.Device {
	auth := device.AuthHAPTransient
	if strings.HasPrefix(model, "AppleTV") {
		auth = device.AuthHAPPIN
	}
	return device.Device{ID: id, Name: id, Host: host, Port: 7000, Model: model, Auth: auth, AirPlay2: true, Online: true}
}

// setDelay stores a receiver's DSP/DAC delay the way the settings command
// would.
func setDelay(h *harness, key string, delay int) {
	entry, ok := h.e.reg.Get(key)
	if !ok {
		entry = receivers.Entry{Key: key, Kind: receivers.KindDiscovered}
	}
	entry.DelayMS = delay
	h.e.reg.Put(entry)
}

func memberLatencies(p sessionPlan) []int {
	out := make([]int, len(p.members))
	for i, m := range p.members {
		out[i] = m.latencyMS
	}
	return out
}

// TestBuildPlanMemberLatency pins per-member latency: every member resolves
// through its target's registry key, and a pair's two members share the pair's
// delay because that is where the registry stores it.
func TestBuildPlanMemberLatency(t *testing.T) {
	t.Run("two singles", func(t *testing.T) {
		h := newHarness(t)
		a := testSingle("AA:01", "192.0.2.10", "AppleTV6,2")
		b := testSingle("AA:02", "192.0.2.11", "AppleTV6,2")
		h.e.setDevices([]device.Device{a, b})
		h.e.setsValue.LatencyMS = 200
		setDelay(h, a.ID, 120)
		setDelay(h, b.ID, 60)
		p, err := h.e.buildPlan([]string{a.ID, b.ID})
		if err != nil {
			t.Fatal(err)
		}
		if got := memberLatencies(p); len(got) != 2 || got[0] != 320 || got[1] != 260 {
			t.Fatalf("latencies = %v, want [320 260]", got)
		}
	})
	t.Run("pair and single", func(t *testing.T) {
		h := newHarness(t)
		pair := testPair("P", "AudioAccessory5,1")
		b := testSingle("AA:02", "192.0.2.12", "AppleTV6,2")
		h.e.setDevices([]device.Device{pair, b})
		h.e.setsValue.LatencyMS = 200
		setDelay(h, pair.ID, 100)
		setDelay(h, b.ID, 60)
		p, err := h.e.buildPlan([]string{pair.ID, b.ID})
		if err != nil {
			t.Fatal(err)
		}
		if got := memberLatencies(p); len(got) != 3 || got[0] != 300 || got[1] != 300 || got[2] != 260 {
			t.Fatalf("latencies = %v, want [300 300 260]", got)
		}
		for i, m := range p.members[:2] {
			if m.key != pair.ID {
				t.Fatalf("pair member %d key = %q, want the pair's key", i, m.key)
			}
		}
	})
}

// TestBuildPlanPairIdentity pins pair identity: both members carry the pair's
// group UUID and both member hosts as timing peers; a standalone target
// carries neither.
func TestBuildPlanPairIdentity(t *testing.T) {
	h := newHarness(t)
	pair := testPair("P", "AudioAccessory5,1")
	single := testSingle("AA:02", "192.0.2.12", "AppleTV6,2")
	h.e.setDevices([]device.Device{pair, single})
	p, err := h.e.buildPlan([]string{pair.ID, single.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.members) != 3 {
		t.Fatalf("members = %d, want 3", len(p.members))
	}
	for i, m := range p.members[:2] {
		if m.groupUUID != pair.StereoID {
			t.Fatalf("pair member %d groupUUID = %q, want %q", i, m.groupUUID, pair.StereoID)
		}
		if len(m.timingPeers) != 2 || m.timingPeers[0] != "192.0.2.10" || m.timingPeers[1] != "192.0.2.11" {
			t.Fatalf("pair member %d timing peers = %v", i, m.timingPeers)
		}
	}
	last := p.members[2]
	if last.groupUUID != "" || len(last.timingPeers) != 0 {
		t.Fatalf("standalone member carries pair identity: %+v", last)
	}
}

// TestClockPlanRoles pins the role rule for every supported mix. Any number of
// standalone PTP receivers share one follower set; a pair needs the sender as
// master; NTP receivers join either.
func TestClockPlanRoles(t *testing.T) {
	cases := []struct {
		name        string
		devices     []device.Device
		wantRole    ptpRole
		wantFollows []string
		wantPeers   int
	}{
		{"ntp only", []device.Device{testSingle("AA:01", "192.0.2.10", "AppleTV6,2")}, ptpNone, nil, 0},
		{"one ptp single", []device.Device{testSingle("AA:01", "192.0.2.10", "AudioAccessory5,1")}, ptpFollowers, []string{"192.0.2.10"}, 0},
		{"one ptp pair", []device.Device{testPair("P", "AudioAccessory5,1")}, ptpMaster, nil, 2},
		{"follower and ntp", []device.Device{
			testSingle("AA:01", "192.0.2.10", "AudioAccessory5,1"),
			testSingle("AA:02", "192.0.2.11", "AppleTV6,2"),
		}, ptpFollowers, []string{"192.0.2.10"}, 0},
		{"two ptp singles", []device.Device{
			testSingle("AA:01", "192.0.2.10", "AudioAccessory5,1"),
			testSingle("AA:02", "192.0.2.11", "AudioAccessory6,1"),
		}, ptpFollowers, []string{"192.0.2.10", "192.0.2.11"}, 0},
		{"master and ntp", []device.Device{
			testPair("P", "AudioAccessory5,1"),
			testSingle("AA:02", "192.0.2.12", "AppleTV6,2"),
		}, ptpMaster, nil, 2},
		{"two pairs", []device.Device{
			testPair("P", "AudioAccessory5,1"),
			func() device.Device {
				p := testPair("Q", "AudioAccessory5,1")
				p.Members[0].Host, p.Members[1].Host = "192.0.2.20", "192.0.2.21"
				return p
			}(),
		}, ptpMaster, nil, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.e.setDevices(tc.devices)
			keys := make([]string, len(tc.devices))
			for i, d := range tc.devices {
				keys[i] = d.ID
			}
			p, err := h.e.buildPlan(keys)
			if err != nil {
				t.Fatal(err)
			}
			if p.clock.role != tc.wantRole {
				t.Fatalf("role = %v, want %v", p.clock.role, tc.wantRole)
			}
			if len(p.clock.followerHosts) != len(tc.wantFollows) {
				t.Fatalf("follower hosts = %v, want %v", p.clock.followerHosts, tc.wantFollows)
			}
			for i, host := range tc.wantFollows {
				if p.clock.followerHosts[i] != host {
					t.Fatalf("follower hosts = %v, want %v", p.clock.followerHosts, tc.wantFollows)
				}
			}
			if len(p.clock.masterPeers) != tc.wantPeers {
				t.Fatalf("master peers = %v, want %d", p.clock.masterPeers, tc.wantPeers)
			}
		})
	}
}

// TestClockPlanRejectsConflicts pins the one remaining clock conflict: a
// follower set beside a master would bind UDP 319/320 twice.
func TestClockPlanRejectsConflicts(t *testing.T) {
	cases := []struct {
		name    string
		devices []device.Device
		want    []string
	}{
		{"follower beside master", []device.Device{
			testSingle("AA:01", "192.0.2.30", "AudioAccessory5,1"),
			testPair("P", "AudioAccessory5,1"),
		}, []string{"AA:01", "P Pair"}},
		{"two followers beside master", []device.Device{
			testSingle("AA:01", "192.0.2.30", "AudioAccessory5,1"),
			testSingle("AA:02", "192.0.2.31", "AudioAccessory6,1"),
			testPair("P", "AudioAccessory5,1"),
		}, []string{"AA:01", "P Pair"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.e.setDevices(tc.devices)
			keys := make([]string, len(tc.devices))
			for i, d := range tc.devices {
				keys[i] = d.ID
			}
			_, err := h.e.buildPlan(keys)
			var pe *planError
			if !errors.As(err, &pe) || pe.key != ErrClockConflict {
				t.Fatalf("error = %v, want a clock conflict", err)
			}
			if len(pe.args) != 2 || pe.args[0] != tc.want[0] || pe.args[1] != tc.want[1] {
				t.Fatalf("conflict args = %q, want %q", pe.args, tc.want)
			}
		})
	}
}

// TestBuildPlanRejectsDuplicateMember pins the duplicate guard: a receiver
// cannot enter a plan twice, once as a single row and once inside a pair.
func TestBuildPlanRejectsDuplicateMember(t *testing.T) {
	h := newHarness(t)
	single := testSingle("AA:01", "192.0.2.10", "AppleTV6,2")
	pair := testPair("P", "AudioAccessory5,1")
	pair.Members[0].ID = single.ID
	pair.Members[0].Host = single.Host
	h.e.setDevices([]device.Device{single, pair})
	_, err := h.e.buildPlan([]string{single.ID, pair.ID})
	var pe *planError
	if !errors.As(err, &pe) || pe.key != ErrSessionOpen {
		t.Fatalf("error = %v, want a duplicate-member rejection", err)
	}
}

// TestConnectIntent pins the whole Connect policy.
func TestConnectIntent(t *testing.T) {
	t.Run("idle new key starts", func(t *testing.T) {
		h := newHarness(t)
		action, keys := h.e.connectIntent(testDeviceID)
		if action != connectStart || len(keys) != 1 || keys[0] != testDeviceID {
			t.Fatalf("intent = %v %v, want start [%s]", action, keys, testDeviceID)
		}
	})
	t.Run("failed plan key retries every wanted target", func(t *testing.T) {
		h := newHarness(t)
		h.e.life.wantedKeys = []string{testDeviceID, "AA:02"}
		action, keys := h.e.connectIntent("AA:02")
		if action != connectRetry || len(keys) != 2 || keys[0] != testDeviceID {
			t.Fatalf("intent = %v %v, want retry of the whole set", action, keys)
		}
	})
	t.Run("streaming active key is a no-op", func(t *testing.T) {
		h := newHarness(t)
		advertiseBoth(h)
		h.connect()
		h.launch()
		action, keys := h.e.connectIntent(testDeviceID)
		if action != connectNoop || keys != nil {
			t.Fatalf("intent = %v %v, want no-op", action, keys)
		}
	})
	t.Run("streaming new key adds", func(t *testing.T) {
		h := newHarness(t)
		b := advertiseBoth(h)
		h.connect()
		h.launch()
		action, keys := h.e.connectIntent(b.ID)
		if action != connectAdd || len(keys) != 2 || keys[0] != testDeviceID || keys[1] != b.ID {
			t.Fatalf("intent = %v %v, want add [%s %s]", action, keys, testDeviceID, b.ID)
		}
	})
	t.Run("handshaking new key replaces", func(t *testing.T) {
		h := newHarness(t)
		b := advertiseBoth(h)
		h.connect()
		h.fake.state = sender.StateConnecting
		action, keys := h.e.connectIntent(b.ID)
		if action != connectReplace || len(keys) != 1 || keys[0] != b.ID {
			t.Fatalf("intent = %v %v, want replace [%s]", action, keys, b.ID)
		}
	})
}
