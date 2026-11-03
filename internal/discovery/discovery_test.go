package discovery

import (
	"net"
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

// TestBrowseIntegration queries the real LAN. Set SAUNDO_MDNS_TEST=1 to
// run it; it needs the app allowed through Windows Firewall for inbound mDNS.
func TestBrowseIntegration(t *testing.T) {
	if os.Getenv("SAUNDO_MDNS_TEST") == "" {
		t.Skip("set SAUNDO_MDNS_TEST=1 to browse the LAN")
	}
	s := New(2*time.Second, 2*time.Second)
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
	base = base.Add(11 * time.Second)
	if got := s.Snapshot(); len(got) != 1 || got[0].Online {
		t.Fatalf("stale device should be offline but kept: %+v", got)
	}
	base = base.Add(95 * time.Second)
	s.prune()
	if got := s.Snapshot(); len(got) != 0 {
		t.Fatalf("long-stale device should be pruned: %+v", got)
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

func TestMergePrefersRAOPEndpointAndAirPlayName(t *testing.T) {
	s := New(0, 0)
	raop, _ := parseEntry(&mdns.ServiceEntry{
		Name: "AABBCCDDEEFF@Old Name", AddrV4: net.IPv4(10, 0, 0, 2), Port: 5000,
		InfoFields: []string{"deviceid=AA:BB:CC:DD:EE:FF", "model=AudioAccessory5,1", "features=0x4a7fca00,0x3c354b00"},
	}, raopService)
	air, _ := parseEntry(&mdns.ServiceEntry{
		Name: "New Name", AddrV4: net.IPv4(10, 0, 0, 2), Port: 7000,
		InfoFields: []string{"deviceid=AA:BB:CC:DD:EE:FF", "model=AudioAccessory5,1", "features=0x4a7fca00,0x3c354b00"},
	}, airplayService)
	s.merge(raop)
	s.merge(air)
	got := s.Snapshot()
	if len(got) != 1 {
		t.Fatalf("devices = %d", len(got))
	}
	if got[0].Name != "New Name" || got[0].Port != 7000 {
		t.Fatalf("merged = %+v", got[0])
	}
}

// TestStopReturnsDuringInFlightQuery pins the stop semantics: Stop must not
// join a blocking mdns query, or quitting the app stalls on it.
func TestStopReturnsDuringInFlightQuery(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(block) })
	prev := mdnsQuery
	mdnsQuery = func(*mdns.QueryParam) error {
		close(started)
		<-block
		return nil
	}
	defer func() { mdnsQuery = prev; releaseOnce() }()

	s := New(0, 0)
	s.Start()
	<-started

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
