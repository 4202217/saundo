package discovery

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/mdns"

	"saundo/internal/airplay/device"
)

func TestParseRAOPEntry(t *testing.T) {
	e := &mdns.ServiceEntry{
		Name:   "AABBCCDDEEFF@Kitchen",
		AddrV4: net.IPv4(192, 168, 1, 30),
		Port:   5000,
		InfoFields: []string{
			"deviceid=AA:BB:CC:DD:EE:FF",
			"model=AudioAccessory5,1",
			"features=0x4A7FCA00,0x3C354B00",
			"sf=0x4",
			"am=AudioAccessory5,1",
			"pw=false",
		},
	}
	d, ok := parseEntry(e, raopService)
	if !ok {
		t.Fatal("entry rejected")
	}
	if d.ID != "AA:BB:CC:DD:EE:FF" || d.Name != "Kitchen" || d.Host != "192.168.1.30" || d.Port != 5000 {
		t.Fatalf("bad device %+v", d)
	}
	if d.Auth != device.AuthHAPTransient {
		t.Fatalf("auth = %v, want transient", d.Auth)
	}
	if d.Features != 0x3C354B004A7FCA00 {
		t.Fatalf("features = %#x", d.Features)
	}
	if d.FeaturesUnknown {
		t.Fatal("a parsed features value was marked unknown")
	}
}

// TestParseEntryUppercaseFeaturePrefixKeepsCapabilities pins the two-part
// feature decoder against the uppercase prefix a receiver can advertise. A
// failed parse reads as zero today, which routes a HomePod to AP1 and NTP.
func TestParseEntryUppercaseFeaturePrefixKeepsCapabilities(t *testing.T) {
	d, ok := parseEntry(&mdns.ServiceEntry{
		Name:   "HomePod._airplay._tcp.local.",
		AddrV4: net.IPv4(192, 168, 1, 20),
		Port:   7000,
		InfoFields: []string{
			"deviceid=AA:BB:CC:DD:EE:FF",
			"model=AudioAccessory5,1",
			"features=0X4A7FCA00,0X3C354B00",
		},
	}, airplayService)
	if !ok {
		t.Fatal("entry rejected")
	}
	if d.Features != 0x3C354B004A7FCA00 {
		t.Fatalf("features = %#x, want 0x3C354B004A7FCA00", d.Features)
	}
	if d.Auth != device.AuthHAPTransient {
		t.Fatalf("auth = %v, want HAP transient", d.Auth)
	}
}

// TestParseEntryMarksAnUnparseableFeatureWord pins the other trigger: an empty
// high word must stay unknown rather than read as a zero value with the low
// word's bits only.
func TestParseEntryMarksAnUnparseableFeatureWord(t *testing.T) {
	d, ok := parseEntry(&mdns.ServiceEntry{
		Name:   "HomePod._airplay._tcp.local.",
		AddrV4: net.IPv4(192, 168, 1, 20),
		Port:   7000,
		InfoFields: []string{
			"deviceid=AA:BB:CC:DD:EE:FF",
			"model=AudioAccessory5,1",
			"features=0x4A7FCA00,",
		},
	}, airplayService)
	if !ok {
		t.Fatal("entry rejected")
	}
	if !d.FeaturesUnknown {
		t.Fatalf("features = %#x, want unknown", d.Features)
	}
	if d.Auth != device.AuthHAPTransient {
		t.Fatalf("auth = %v, want HAP transient", d.Auth)
	}
}

func TestParseAppleTVEntryChoosesPin(t *testing.T) {
	e := &mdns.ServiceEntry{
		Name:   "Living Room",
		AddrV4: net.IPv4(192, 168, 1, 40),
		Port:   7000,
		InfoFields: []string{
			"deviceid=11:22:33:44:55:66",
			"model=AppleTV6,2",
			"features=0x4A7FCA00,0x3C354B00",
		},
	}
	d, ok := parseEntry(e, airplayService)
	if !ok || d.Auth != device.AuthHAPPIN || !d.AirPlay2 {
		t.Fatalf("bad Apple TV: %+v ok=%v", d, ok)
	}
}

func TestParseOSVersionTXT(t *testing.T) {
	d, ok := parseEntry(&mdns.ServiceEntry{
		Name:   "Living Room._airplay._tcp.local.",
		AddrV4: net.IPv4(192, 168, 1, 40),
		Port:   7000,
		InfoFields: []string{
			"deviceid=11:22:33:44:55:66",
			"model=AudioAccessory6,1",
			"osvers=27.2",
			"features=0x4A7FCA00,0x3C354B00",
		},
	}, airplayService)
	if !ok {
		t.Fatal("entry rejected")
	}
	if d.OSVersion != "27.2" {
		t.Fatalf("OSVersion = %q, want 27.2", d.OSVersion)
	}
}

// TestParseEntryUsesTheAdvertisedService pins the name leak: hashicorp/mdns
// delivers records from both AirPlay services to either browse, so the
// instance name must be trimmed by the service it belongs to, not by the
// query that happened to receive it.
func TestParseEntryUsesTheAdvertisedService(t *testing.T) {
	t.Run("airplay name in the raop browse", func(t *testing.T) {
		d, ok := parseEntry(&mdns.ServiceEntry{
			Name:   "MacBook Air._airplay._tcp.local.",
			AddrV4: net.IPv4(192, 168, 1, 40),
			Port:   7000,
			InfoFields: []string{
				"deviceid=11:22:33:44:55:66",
				"model=Mac14,2",
				"features=0x4A7FCA00,0x3C354B00",
			},
		}, raopService)
		if !ok {
			t.Fatal("entry rejected")
		}
		if d.Name != "MacBook Air" {
			t.Fatalf("name = %q, want MacBook Air", d.Name)
		}
		if !d.AirPlay2 {
			t.Fatal("airplay advertisement parsed as a raop device")
		}
	})
	t.Run("raop name in the airplay browse", func(t *testing.T) {
		d, ok := parseEntry(&mdns.ServiceEntry{
			Name:   "AABBCCDDEEFF@MacBook Air._raop._tcp.local.",
			AddrV4: net.IPv4(192, 168, 1, 40),
			Port:   5000,
			InfoFields: []string{
				"deviceid=AA:BB:CC:DD:EE:FF",
				"am=Mac14,2",
			},
		}, airplayService)
		if !ok {
			t.Fatal("entry rejected")
		}
		if d.Name != "MacBook Air" {
			t.Fatalf("name = %q, want MacBook Air", d.Name)
		}
		if d.AirPlay2 {
			t.Fatal("raop advertisement parsed as an airplay device")
		}
	})
}

// TestMergeKeepsOSVersion covers the two advertisements: the AirPlay record
// carries osvers while the RAOP record does not, and the merged device keeps
// it.
func TestMergeKeepsOSVersion(t *testing.T) {
	s := New(0, 0)
	raop, _ := parseEntry(&mdns.ServiceEntry{
		Name: "AABBCCDDEEFF@Living Room", AddrV4: net.IPv4(10, 0, 0, 2), Port: 5000,
		InfoFields: []string{"deviceid=AA:BB:CC:DD:EE:FF", "model=AudioAccessory6,1"},
	}, raopService)
	air, _ := parseEntry(&mdns.ServiceEntry{
		Name: "Living Room", AddrV4: net.IPv4(10, 0, 0, 2), Port: 7000,
		InfoFields: []string{"deviceid=AA:BB:CC:DD:EE:FF", "model=AudioAccessory6,1", "osvers=27.2"},
	}, airplayService)
	if raop.OSVersion != "" {
		t.Fatalf("RAOP record carried osvers %q", raop.OSVersion)
	}
	s.merge(raop)
	s.merge(air)
	got := s.Snapshot()
	if len(got) != 1 || got[0].OSVersion != "27.2" {
		t.Fatalf("merged OSVersion = %+v, want 27.2", got)
	}
}

func TestPasswordAndAirportAuth(t *testing.T) {
	ap := device.Device{AM: "AirPort10,115", Features: featureHKPairing}
	if chooseAuth(ap) != device.AuthSetup {
		t.Fatal("AirPort device should use auth-setup")
	}
	pw := device.Device{PasswordRequired: true}
	if chooseAuth(pw) != device.AuthPassword {
		t.Fatal("a password gate without HAP bits is the legacy AP1 digest")
	}
	hapPw := device.Device{PasswordRequired: true, Features: featureHKPairing}
	if chooseAuth(hapPw) != device.AuthHAPPassword {
		t.Fatal("a password gate with HAP bits must use HAP password pairing")
	}
}

func TestParseEscapedRAOPName(t *testing.T) {
	e := &mdns.ServiceEntry{
		Name:   `02000000000A\@HomePod\ R._raop._tcp.local.`,
		AddrV4: net.IPv4(192, 0, 2, 12),
		Port:   7000,
		InfoFields: []string{
			"am=AudioAccessory5,1",
			"ft=0x4A7FCA00,0x3C354BD0",
			"sf=0x298404",
		},
	}
	d, ok := parseEntry(e, raopService)
	if !ok {
		t.Fatal("escaped RAOP entry rejected")
	}
	if d.ID != "02:00:00:00:00:0A" || d.Name != "HomePod R" || d.Model != "AudioAccessory5,1" {
		t.Fatalf("parsed %+v", d)
	}
	if d.Auth != device.AuthHAPTransient {
		t.Fatalf("auth = %v, want transient from the ft feature bits", d.Auth)
	}
}

// TestUnescapeDNSDecodesDecimalEscapes covers the escaped instance name the
// Redmi advertises: each UTF-8 byte arrives as an RFC 1035 \DDD escape.
func TestUnescapeDNSDecodesDecimalEscapes(t *testing.T) {
	tests := []struct{ in, want string }{
		{`REDMI\ \230\152\190\231\164\186\229\153\168\ \226\128\166`, "REDMI 显示器 …"},
		{`HomePod\ R`, "HomePod R"},
		{`Kitchen`, "Kitchen"},
		{`trailing\`, `trailing\`},
		{`\999`, "999"},
		{`a\065b`, "aAb"},
	}
	for _, tt := range tests {
		if got := unescapeDNS(tt.in); got != tt.want {
			t.Errorf("unescapeDNS(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRedmiFairPlayReceiverAndNameDecoding mirrors the Redmi TV
// advertisement: no HAP bits, pw=false, model AppleTV3,2, sf=0x44. It is an
// Apple TV 3-style FairPlay receiver, so discovery fails fast instead of
// offering a session that connects but never produces sound. The escaped mDNS
// name is decoded to UTF-8.
func TestRedmiFairPlayReceiverAndNameDecoding(t *testing.T) {
	fields := []string{
		"deviceid=02:00:00:00:00:0C",
		"model=AppleTV3,2",
		"features=0x527FFFF7,0x1E",
		"sf=0x44",
		"pw=false",
	}
	air, ok := parseEntry(&mdns.ServiceEntry{
		Name:       `REDMI\ \230\152\190\231\164\186\229\153\168\ \226\128\166`,
		AddrV4:     net.IPv4(192, 168, 1, 60),
		Port:       47000,
		InfoFields: fields,
	}, airplayService)
	if !ok {
		t.Fatal("airplay entry rejected")
	}
	if air.Auth != device.AuthLegacyPin {
		t.Fatalf("auth = %v, want legacy-pin (FairPlay unsupported)", air.Auth)
	}
	if air.Name != "REDMI 显示器 …" {
		t.Fatalf("name = %q, want the decoded UTF-8 name", air.Name)
	}
}

func TestNormalizeDeviceID(t *testing.T) {
	if got := normalizeDeviceID("aabbccddeeff"); got != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeDeviceID("aa:bb:cc:dd:ee:ff"); got != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("got %q", got)
	}
}

// TestQueryMarksTheLocalReceiver pins the advertisement rule: a receiver
// advertising one of this host's own addresses is marked Local, so the rest
// of the app can keep it out of the target list.
func TestQueryMarksTheLocalReceiver(t *testing.T) {
	prev := mdnsQuery
	mdnsQuery = func(p *mdns.QueryParam) error {
		if p.Service != airplayService {
			return nil
		}
		p.Entries <- &mdns.ServiceEntry{
			Name:   "MacBook Air._airplay._tcp.local.",
			AddrV4: net.IPv4(192, 168, 1, 10),
			Port:   7000,
			InfoFields: []string{
				"deviceid=02:8F:B8:25:3A:B2",
				"model=Mac14,2",
				"features=0x4A7FCA00,0x3C354B00",
				"sf=0x284",
			},
		}
		return nil
	}
	defer func() { mdnsQuery = prev }()

	s := New(0, 0)
	s.unicastEvery = 0
	s.interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.IPv4(192, 168, 1, 10), Mask: net.CIDRMask(24, 32)}}, nil
	}
	s.queryOnce()

	got := s.Snapshot()
	if len(got) != 1 {
		t.Fatalf("snapshot = %+v, want one receiver", got)
	}
	if !got[0].Local {
		t.Fatalf("the receiver at this host's address is not marked local: %+v", got[0])
	}
}

// TestLocalAddressesKeepTheLastSetOnReadError pins the unknown-not-empty
// rule: a failed enumeration must not turn the local receiver back into a
// candidate.
func TestLocalAddressesKeepTheLastSetOnReadError(t *testing.T) {
	s := New(0, 0)
	s.interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.IPv4(10, 0, 0, 5), Mask: net.CIDRMask(24, 32)}}, nil
	}
	if !localHost(s.localAddresses(), "10.0.0.5") {
		t.Fatal("a successful read did not mark the host address")
	}
	s.interfaceAddrs = func() ([]net.Addr, error) { return nil, errors.New("no interfaces") }
	if !localHost(s.localAddresses(), "10.0.0.5") {
		t.Fatal("a failed read dropped the last known local addresses")
	}
}

// TestLocalHostMatchesOnlyIPs pins the comparison: exact IPs match, a
// hostname or another address does not, and IPv4-mapped forms unify.
func TestLocalHostMatchesOnlyIPs(t *testing.T) {
	set := map[netip.Addr]struct{}{netip.MustParseAddr("192.168.1.10"): {}}
	cases := []struct {
		host string
		want bool
	}{
		{"192.168.1.10", true},
		{"::ffff:192.168.1.10", true},
		{"192.168.1.11", false},
		{"MacBook.local", false},
		{"", false},
	}
	for _, c := range cases {
		if got := localHost(set, c.host); got != c.want {
			t.Errorf("localHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
	if localHost(nil, "192.168.1.10") {
		t.Error("an unknown local set marked an address")
	}
}

// TestMergeKeepsLocalFlag pins that a later sighting carries the flag into
// the merged device.
func TestMergeKeepsLocalFlag(t *testing.T) {
	s := New(0, 0)
	d, ok := parseEntry(&mdns.ServiceEntry{
		Name:   "MacBook Air._airplay._tcp.local.",
		AddrV4: net.IPv4(192, 168, 1, 10),
		Port:   7000,
		InfoFields: []string{
			"deviceid=02:8F:B8:25:3A:B2",
			"model=Mac14,2",
			"features=0x4A7FCA00,0x3C354B00",
		},
	}, airplayService)
	if !ok {
		t.Fatal("entry rejected")
	}
	d.Local = true
	s.merge(d)
	got := s.Snapshot()
	if len(got) != 1 || !got[0].Local {
		t.Fatalf("merge dropped the local flag: %+v", got)
	}
}

// TestBrowseIntegration queries the real LAN. Set SAUNDO_MDNS_TEST=1 to
// run it; it needs the app allowed through Windows Firewall for inbound mDNS.
func TestBrowseIntegration(t *testing.T) {
	if os.Getenv("SAUNDO_MDNS_TEST") == "" {
		t.Skip("set SAUNDO_MDNS_TEST=1 to browse the LAN")
	}
	s := New(2*time.Second, 2*time.Second)
	// This test measures the multicast browse alone; the directed fallback
	// has its own live test.
	s.unicastEvery = 0
	s.Start()
	defer s.Stop()
	deadline := time.After(8 * time.Second)
	for {
		select {
		case devs := <-s.Updates():
			if len(devs) > 0 {
				for _, d := range devs {
					t.Logf("found id=%q name=%q at %s:%d model=%s auth=%s ap2=%v",
						d.ID, d.Name, d.Host, d.Port, d.Model, d.Auth, d.AirPlay2)
				}
				return
			}
		case <-deadline:
			t.Skip("no AirPlay devices answered mDNS")
		}
	}
}

func TestOnlineStatusAndPrune(t *testing.T) {
	s := New(0, 0)
	base := time.Now()
	s.now = func() time.Time { return base }
	e, ok := parseEntry(&mdns.ServiceEntry{
		Name:   "AABBCCDDEEFF@Kitchen",
		AddrV4: net.IPv4(10, 0, 0, 2),
		Port:   5000,
		InfoFields: []string{
			"deviceid=AA:BB:CC:DD:EE:FF",
			"model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00",
		},
	}, raopService)
	if !ok {
		t.Fatal("entry rejected")
	}
	s.merge(e)
	if got := s.Snapshot(); len(got) != 1 || !got[0].Online {
		t.Fatalf("fresh device should be online: %+v", got)
	}
	base = base.Add(s.staleAfter + time.Second)
	if got := s.Snapshot(); len(got) != 1 || got[0].Online {
		t.Fatalf("stale device should be offline but kept: %+v", got)
	}
	base = base.Add(95 * time.Second)
	s.prune()
	if got := s.Snapshot(); len(got) != 0 {
		t.Fatalf("long-stale device should be pruned: %+v", got)
	}
}

// TestStaleWindowOutlastsDirectedPass pins the offline window against both
// discovery cadences: the multicast browse and the slower directed pass.
func TestStaleWindowOutlastsDirectedPass(t *testing.T) {
	s := New(3*time.Second, 1500*time.Millisecond)
	want := time.Duration(unicastEveryPasses)*s.queryInterval + s.queryTimeout
	if s.staleAfter < want {
		t.Fatalf("staleAfter = %s, want at least one directed-pass period plus timeout = %s", s.staleAfter, want)
	}
}

// TestDirectedSightingStaysOnlineUntilNextPass reproduces the flap: a record
// seen at t0 must still be online 12 s later, because the next directed pass
// cannot answer before 15 s.
func TestDirectedSightingStaysOnlineUntilNextPass(t *testing.T) {
	s := New(3*time.Second, 1500*time.Millisecond)
	base := time.Now()
	s.now = func() time.Time { return base }
	e, ok := parseEntry(&mdns.ServiceEntry{
		Name:   "AABBCCDDEEFF@Kitchen",
		AddrV4: net.IPv4(10, 0, 0, 2),
		Port:   5000,
		InfoFields: []string{
			"deviceid=AA:BB:CC:DD:EE:FF",
			"model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00",
		},
	}, raopService)
	if !ok {
		t.Fatal("entry rejected")
	}
	s.merge(e)
	base = base.Add(12 * time.Second)
	if got := s.Snapshot(); len(got) != 1 || !got[0].Online {
		t.Fatalf("a directed-pass sighting flapped offline before the next pass: %+v", got)
	}
}

func TestGroupFieldsParsedAndMerged(t *testing.T) {
	raop, ok := parseEntry(&mdns.ServiceEntry{
		Name:   `AABBCCDDEEFF@Kitchen._raop._tcp.local.`,
		AddrV4: net.IPv4(10, 0, 0, 2),
		Port:   5000,
		InfoFields: []string{
			"deviceid=AA:BB:CC:DD:EE:FF",
			"model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00",
			"gpn=HomePods",
			"igl=1",
		},
	}, raopService)
	if !ok {
		t.Fatal("raop entry rejected")
	}
	if raop.GroupName != "HomePods" || !raop.GroupLeader {
		t.Fatalf("group fields = %q %v", raop.GroupName, raop.GroupLeader)
	}
	air, ok := parseEntry(&mdns.ServiceEntry{
		Name:   "Kitchen._airplay._tcp.local.",
		AddrV4: net.IPv4(10, 0, 0, 2),
		Port:   7000,
		InfoFields: []string{
			"deviceid=AA:BB:CC:DD:EE:FF",
			"model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00",
		},
	}, airplayService)
	if !ok {
		t.Fatal("airplay entry rejected")
	}
	s := New(0, 0)
	s.merge(raop)
	s.merge(air)
	got := s.Snapshot()
	if len(got) != 1 {
		t.Fatalf("devices = %d, want 1", len(got))
	}
	if got[0].GroupName != "HomePods" || !got[0].GroupLeader {
		t.Fatalf("merged group fields = %q %v", got[0].GroupName, got[0].GroupLeader)
	}
	if got[0].Auth != device.AuthHAPTransient {
		t.Fatalf("auth = %v", got[0].Auth)
	}
}

func TestGCGLDoesNotMarkTheMemberAsLeader(t *testing.T) {
	d, ok := parseEntry(&mdns.ServiceEntry{
		Name: "HomePod L._airplay._tcp.local.", AddrV4: net.IPv4(10, 0, 0, 2), Port: 7000,
		InfoFields: []string{
			"deviceid=AA:BB:CC:DD:EE:FF",
			"model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00",
			"gpn=HomePods",
			"gcgl=1",
			"tsid=00000000-0000-4000-8000-000000000000",
		},
	}, airplayService)
	if !ok {
		t.Fatal("entry rejected")
	}
	if d.GroupLeader {
		t.Fatal("gcgl must not make a member the leader; only igl does")
	}
	if d.StereoID != "00000000-0000-4000-8000-000000000000" {
		t.Fatalf("stereo ID = %q", d.StereoID)
	}
}

func TestSnapshotCollapsesStereoPair(t *testing.T) {
	s := New(0, 0)
	base := time.Now()
	s.now = func() time.Time { return base }
	follower, _ := parseEntry(&mdns.ServiceEntry{
		Name: "HomePod L._airplay._tcp.local.", AddrV4: net.IPv4(10, 0, 0, 2), Port: 7000,
		InfoFields: []string{
			"deviceid=02:00:00:00:00:0B", "model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00", "gpn=HomePods", "icl=0",
			"gcgl=1", "tsid=00000000-0000-4000-8000-000000000000",
		},
	}, airplayService)
	leader, _ := parseEntry(&mdns.ServiceEntry{
		Name: "HomePod R._airplay._tcp.local.", AddrV4: net.IPv4(10, 0, 0, 3), Port: 7000,
		InfoFields: []string{
			"deviceid=02:00:00:00:00:0A", "model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00", "gpn=HomePods", "igl=1",
			"gcgl=1", "tsid=00000000-0000-4000-8000-000000000000",
		},
	}, airplayService)
	s.merge(follower)
	s.merge(leader)
	got := s.Snapshot()
	if len(got) != 1 {
		t.Fatalf("snapshot = %+v, want one stereo group", got)
	}
	g := got[0]
	if !g.IsStereoGroup() || g.ID != device.StereoGroupID("00000000-0000-4000-8000-000000000000") {
		t.Fatalf("group = %+v", g)
	}
	if g.Name != "HomePods" || !g.Online || len(g.Members) != 2 {
		t.Fatalf("group = %+v", g)
	}
	if !g.Members[0].GroupLeader || g.Members[0].ID != "02:00:00:00:00:0A" {
		t.Fatalf("leader must be first: %+v", g.Members)
	}
}

// TestStereoGroupNameDecodesDNSEscapes mirrors a HomePod pair whose Home app
// group name is 卧室: the TXT `gpn` carries each UTF-8 byte as an RFC 1035
// \DDD escape, the same presentation form hashicorp/mdns reports for instance
// names. The collapsed group must show the decoded name.
func TestStereoGroupNameDecodesDNSEscapes(t *testing.T) {
	fields := func(id, leader string) []string {
		return []string{
			"deviceid=" + id, "model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00",
			`gpn=\229\141\167\229\174\164`,
			leader, "tsid=00000000-0000-4000-8000-000000000000",
		}
	}
	right, ok := parseEntry(&mdns.ServiceEntry{
		Name: "HomePod R._airplay._tcp.local.", AddrV4: net.IPv4(10, 0, 0, 3), Port: 7000,
		InfoFields: fields("02:00:00:00:00:0A", "igl=1"),
	}, airplayService)
	if !ok {
		t.Fatal("leader rejected")
	}
	if right.GroupName != "卧室" {
		t.Fatalf("group name = %q, want the decoded UTF-8 name", right.GroupName)
	}
	left, ok := parseEntry(&mdns.ServiceEntry{
		Name: "HomePod L._airplay._tcp.local.", AddrV4: net.IPv4(10, 0, 0, 2), Port: 7000,
		InfoFields: fields("02:00:00:00:00:0B", "gcgl=1"),
	}, airplayService)
	if !ok {
		t.Fatal("member rejected")
	}
	s := New(0, 0)
	s.now = func() time.Time { return time.Now() }
	s.merge(right)
	s.merge(left)
	got := s.Snapshot()
	if len(got) != 1 || got[0].Name != "卧室" {
		t.Fatalf("snapshot = %+v, want one group named 卧室", got)
	}
}

func TestIncompleteStereoGroupIsOffline(t *testing.T) {
	s := New(0, 0)
	base := time.Now()
	s.now = func() time.Time { return base }
	member, _ := parseEntry(&mdns.ServiceEntry{
		Name: "HomePod L._airplay._tcp.local.", AddrV4: net.IPv4(10, 0, 0, 2), Port: 7000,
		InfoFields: []string{
			"deviceid=02:00:00:00:00:0B", "model=AudioAccessory5,1",
			"features=0x4a7fca00,0x3c354b00", "gpn=HomePods",
			"gcgl=1", "tsid=00000000-0000-4000-8000-000000000000",
		},
	}, airplayService)
	s.merge(member)
	got := s.Snapshot()
	if len(got) != 1 || !got[0].IsStereoGroup() {
		t.Fatalf("snapshot = %+v", got)
	}
	if got[0].Online || len(got[0].Members) != 1 {
		t.Fatalf("incomplete group must be offline: %+v", got[0])
	}
}

// TestLegacyAppleTVFailsFastForFairPlay pins the FairPlay fail-fast. An
// Apple TV 3-style receiver uses Apple's FairPlay audio, which this sender does
// not implement, so discovery refuses it instead of starting a silent session.
// A modern HAP Apple TV still takes the PIN path.
func TestLegacyAppleTVFailsFastForFairPlay(t *testing.T) {
	legacy := device.Device{Model: "AppleTV3,2"}
	if chooseAuth(legacy) != device.AuthLegacyPin {
		t.Fatalf("auth = %v, want legacy-pin for a HAP-less AppleTV3,2", chooseAuth(legacy))
	}
	modern := device.Device{Model: "AppleTV6,2", Features: featureHKPairing}
	if chooseAuth(modern) != device.AuthHAPPIN {
		t.Fatalf("auth = %v, want hap-pin", chooseAuth(modern))
	}
}

// TestPasswordProtectedMacMapsToHAPPassword mirrors the real MacBook Air
// advertisement: _raop port 7000, model Mac14,2, HAP feature bits, and
// sf=0x284 whose bit 7 sets PasswordRequired with no `pw` TXT.
func TestPasswordProtectedMacMapsToHAPPassword(t *testing.T) {
	e := &mdns.ServiceEntry{
		Name:   "MacBook Air._raop._tcp.local.",
		AddrV4: net.IPv4(192, 168, 1, 50),
		Port:   7000,
		InfoFields: []string{
			"deviceid=22:33:44:55:66:77",
			"model=Mac14,2",
			"features=0x4A7FCFD5,0x38174FDE",
			"sf=0x284",
		},
	}
	d, ok := parseEntry(e, raopService)
	if !ok {
		t.Fatal("entry rejected")
	}
	if !d.PasswordRequired {
		t.Fatal("sf=0x284 must mark the receiver password-protected")
	}
	if d.Auth != device.AuthHAPPassword {
		t.Fatalf("auth = %v, want hap-password", d.Auth)
	}
}

// TestMergePrefersTheAirPlayEndpointForHAP pins the endpoint rule and its
// order independence: a HAP receiver uses the _airplay SRV port whichever
// browse replies last. Real records carry the same name on both services, so
// the test asserts the endpoint only.
func TestMergePrefersTheAirPlayEndpointForHAP(t *testing.T) {
	parse := func(t *testing.T, name string, port int, service string) device.Device {
		t.Helper()
		d, ok := parseEntry(&mdns.ServiceEntry{
			Name: name, AddrV4: net.IPv4(10, 0, 0, 2), Port: port,
			InfoFields: []string{"deviceid=AA:BB:CC:DD:EE:FF", "model=AudioAccessory5,1", "features=0x4a7fca00,0x3c354b00"},
		}, service)
		if !ok {
			t.Fatal("entry rejected")
		}
		return d
	}
	cases := []struct {
		name  string
		first func(*testing.T) device.Device
		last  func(*testing.T) device.Device
	}{
		{"raop first", func(t *testing.T) device.Device { return parse(t, "AABBCCDDEEFF@Old Name", 5000, raopService) }, func(t *testing.T) device.Device { return parse(t, "New Name", 7000, airplayService) }},
		{"airplay first", func(t *testing.T) device.Device { return parse(t, "New Name", 7000, airplayService) }, func(t *testing.T) device.Device { return parse(t, "AABBCCDDEEFF@Old Name", 5000, raopService) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New(0, 0)
			s.merge(c.first(t))
			s.merge(c.last(t))
			got := s.Snapshot()
			if len(got) != 1 {
				t.Fatalf("devices = %d", len(got))
			}
			if got[0].Host != "10.0.0.2" || got[0].Port != 7000 {
				t.Fatalf("endpoint = %s:%d, want the _airplay endpoint 10.0.0.2:7000", got[0].Host, got[0].Port)
			}
		})
	}
}

// TestMergeLocalFlagSurvivesRemoteSighting pins the monotonic local rule: a
// later remote-looking record must not re-enable a receiver on this host.
func TestMergeLocalFlagSurvivesRemoteSighting(t *testing.T) {
	s := New(0, 0)
	local := device.Device{
		ID: "AA:BB:CC:DD:EE:FF", Host: "10.0.0.2", Port: 7000,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Local: true,
	}
	remote := local
	remote.Host = "10.0.0.3"
	remote.Local = false
	s.merge(local)
	s.merge(remote)
	got := s.Snapshot()
	if len(got) != 1 {
		t.Fatalf("devices = %d", len(got))
	}
	if !got[0].Local {
		t.Fatalf("local flag = false after a remote sighting: %+v", got[0])
	}
}

// TestMergeFallsBackWhenThePreferredServiceGoesStale pins the freshness rule:
// a lost _airplay reply must not keep its old address when the _raop record is
// still refreshing the receiver.
func TestMergeFallsBackWhenThePreferredServiceGoesStale(t *testing.T) {
	s := New(0, 0)
	base := time.Now()
	s.now = func() time.Time { return base }
	air := device.Device{
		ID: "AA:BB:CC:DD:EE:FF", Host: "10.0.0.2", Port: 7000,
		Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true,
	}
	raop := air
	raop.Port = 5000
	raop.AirPlay2 = false
	s.merge(air)
	base = base.Add(s.staleAfter + time.Second)
	s.merge(raop)
	got := s.Snapshot()
	if len(got) != 1 {
		t.Fatalf("devices = %d", len(got))
	}
	if got[0].Host != "10.0.0.2" || got[0].Port != 5000 {
		t.Fatalf("endpoint = %s:%d, want the fresh _raop fallback 10.0.0.2:5000", got[0].Host, got[0].Port)
	}
}

// TestStopReturnsDuringInFlightQuery pins the stop semantics: Stop must not
// join a blocking mdns query, or quitting the app stalls on it.
func TestStopReturnsDuringInFlightQuery(t *testing.T) {
	block := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(block) })
	var entered, finished sync.WaitGroup
	entered.Add(2)
	finished.Add(2)
	prev := mdnsQuery
	mdnsQuery = func(*mdns.QueryParam) error {
		entered.Done()
		<-block
		finished.Done()
		return nil
	}
	// Restore the global only after both service queries have left the mock,
	// so the race detector sees no unsynchronized read of mdnsQuery.
	defer func() { releaseOnce(); finished.Wait(); mdnsQuery = prev }()

	s := New(0, 0)
	// This test replaces only the multicast query; keep the directed pass off
	// the LAN so no neighbor is probed for real.
	s.neighbors = func() ([]string, error) { return nil, nil }
	s.Start()
	entered.Wait()

	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop waited for the in-flight query")
	}
}
