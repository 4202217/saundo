package discovery

import (
	"encoding/binary"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/miekg/dns"

	"saundo/internal/airplay/device"
)

const (
	testPairTSID = "7A6CE980-7265-5ABD-B3E9-B90563D5932F"
	testLeaderID = "D2:91:ED:4D:0B:C9"
	testMemberID = "76:3D:7E:CA:67:71"
	testNameL    = `HomePod\ L._airplay._tcp.local.`
	testNameR    = `HomePod\ R._airplay._tcp.local.`
	testRAOPL    = `763D7ECA6771\@HomePod\ L._raop._tcp.local.`
	testRAOPR    = `D291ED4D0BC9\@HomePod\ R._raop._tcp.local.`
	testHostL    = "HomePod-L.local."
	testHostR    = "HomePod-R.local."
)

type dnsZone struct {
	answer map[string]map[uint16][]dns.RR
	extra  map[string][]dns.RR
}

func (z dnsZone) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	for _, q := range r.Question {
		key := dns.CanonicalName(q.Name)
		if byType, ok := z.answer[key]; ok {
			m.Answer = append(m.Answer, byType[q.Qtype]...)
		}
		m.Extra = append(m.Extra, z.extra[key]...)
	}
	_ = w.WriteMsg(m)
}

// startDNSZone runs a fake unicast responder on loopback for one test.
func startDNSZone(t *testing.T, zone dnsZone) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: zone}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func rrHeader(name string, qtype uint16) dns.RR_Header {
	return dns.RR_Header{Name: dns.CanonicalName(name), Rrtype: qtype, Class: dns.ClassINET, Ttl: 10}
}

func ptrRR(name, target string) dns.RR {
	return &dns.PTR{Hdr: rrHeader(name, dns.TypePTR), Ptr: target}
}

func srvRR(name, target string, port uint16) dns.RR {
	return &dns.SRV{Hdr: rrHeader(name, dns.TypeSRV), Port: port, Target: target}
}

func txtRR(name string, fields ...string) dns.RR {
	return &dns.TXT{Hdr: rrHeader(name, dns.TypeTXT), Txt: fields}
}

func aRR(name, ip string) dns.RR {
	return &dns.A{Hdr: rrHeader(name, dns.TypeA), A: net.ParseIP(ip).To4()}
}

func airplayTXT(id string, groupFields ...string) []string {
	fields := []string{
		"deviceid=" + id,
		"model=AudioAccessory5,1",
		"features=0x4A7FCA00,0x3C354BD0",
		`gpn=\229\141\167\229\174\164`,
		"tsid=" + testPairTSID,
	}
	return append(fields, groupFields...)
}

func raopTXT(sf string) []string {
	return []string{
		"am=AudioAccessory5,1",
		"ft=0x4A7FCA00,0x3C354BD0",
		"sf=" + sf,
	}
}

// homePodZone answers as the two HomePods do: _airplay carries SRV, TXT, and A
// in the additional section of the PTR reply, while _raop answers only the PTR
// and leaves the records to follow-up queries.
func homePodZone() dnsZone {
	z := dnsZone{
		answer: map[string]map[uint16][]dns.RR{
			dns.CanonicalName("_airplay._tcp.local."): {
				dns.TypePTR: {ptrRR("_airplay._tcp.local.", testNameL), ptrRR("_airplay._tcp.local.", testNameR)},
			},
			dns.CanonicalName("_raop._tcp.local."): {
				dns.TypePTR: {ptrRR("_raop._tcp.local.", testRAOPL), ptrRR("_raop._tcp.local.", testRAOPR)},
			},
			dns.CanonicalName(testRAOPL): {
				dns.TypeSRV: {srvRR(testRAOPL, testHostL, 7000)},
				dns.TypeTXT: {txtRR(testRAOPL, raopTXT("0x3ba404")...)},
			},
			dns.CanonicalName(testRAOPR): {
				dns.TypeSRV: {srvRR(testRAOPR, testHostR, 7000)},
				dns.TypeTXT: {txtRR(testRAOPR, raopTXT("0x1b8404")...)},
			},
			dns.CanonicalName(testHostL): {
				dns.TypeA: {aRR(testHostL, "192.168.31.46")},
			},
			dns.CanonicalName(testHostR): {
				dns.TypeA: {aRR(testHostR, "192.168.31.164")},
			},
		},
		extra: map[string][]dns.RR{
			dns.CanonicalName("_airplay._tcp.local."): {
				srvRR(testNameL, testHostL, 7000),
				txtRR(testNameL, airplayTXT(testMemberID, "gcgl=1")...),
				aRR(testHostL, "192.168.31.46"),
				srvRR(testNameR, testHostR, 7000),
				txtRR(testNameR, airplayTXT(testLeaderID, "igl=1", "gcgl=1")...),
				aRR(testHostR, "192.168.31.164"),
			},
		},
	}
	return z
}

// TestUnicastFallbackDiscoversStereoPair drives the directed resolver against
// a loopback responder. The pair arrives through the _airplay additional
// records and through _raop follow-up queries; both merge by deviceid, and the
// snapshot builds the stereo group named from the decoded gpn.
func TestUnicastFallbackDiscoversStereoPair(t *testing.T) {
	port := startDNSZone(t, homePodZone())
	s := New(0, 0)
	base := time.Now()
	s.now = func() time.Time { return base }
	s.unicastPort = port
	s.unicastTimeout = time.Second
	s.neighbors = func() ([]string, error) { return []string{"127.0.0.1"}, nil }
	s.unicastOnce()

	got := s.Snapshot()
	if len(got) != 1 {
		t.Fatalf("snapshot = %+v, want one stereo group", got)
	}
	g := got[0]
	if !g.IsStereoGroup() || g.ID != device.StereoGroupID(testPairTSID) {
		t.Fatalf("group = %+v", g)
	}
	if g.Name != "卧室" {
		t.Fatalf("group name = %q, want the decoded gpn", g.Name)
	}
	if !g.Online || len(g.Members) != 2 {
		t.Fatalf("group online=%v members=%d, want both online", g.Online, len(g.Members))
	}
	if !g.Members[0].GroupLeader || g.Members[0].ID != testLeaderID {
		t.Fatalf("leader must be first: %+v", g.Members)
	}
	if g.Host != "192.168.31.164" || g.Port != 7000 {
		t.Fatalf("group endpoint = %s:%d, want the leader's", g.Host, g.Port)
	}
	// SF exists only in the _raop TXT, so these values prove the follow-up
	// queries merged into the same two devices as the _airplay records.
	if g.Members[0].SF != 0x1b8404 || g.Members[1].SF != 0x3ba404 {
		t.Fatalf("raop records did not merge: %+v", g.Members)
	}
}

// TestUnicastSkipsInstanceWithoutAddress pins the fail-closed rule: a PTR and
// SRV/TXT with no A record is not a device with an empty address.
func TestUnicastSkipsInstanceWithoutAddress(t *testing.T) {
	zone := dnsZone{
		answer: map[string]map[uint16][]dns.RR{
			dns.CanonicalName("_airplay._tcp.local."): {
				dns.TypePTR: {ptrRR("_airplay._tcp.local.", testNameL)},
			},
			dns.CanonicalName(testNameL): {
				dns.TypeSRV: {srvRR(testNameL, testHostL, 7000)},
				dns.TypeTXT: {txtRR(testNameL, airplayTXT(testMemberID, "gcgl=1")...)},
			},
		},
	}
	port := startDNSZone(t, zone)
	s := New(0, 0)
	s.now = func() time.Time { return time.Now() }
	s.unicastPort = port
	s.unicastTimeout = time.Second
	s.neighbors = func() ([]string, error) { return []string{"127.0.0.1"}, nil }
	s.unicastOnce()

	if got := s.Snapshot(); len(got) != 0 {
		t.Fatalf("addressless instance became a device: %+v", got)
	}
}

// TestUnicastRunsEveryFifthCycle pins the throttle: the first cycle and then
// every fifth one, not every three-second browse.
func TestUnicastRunsEveryFifthCycle(t *testing.T) {
	prev := mdnsQuery
	mdnsQuery = func(*mdns.QueryParam) error { return nil }
	defer func() { mdnsQuery = prev }()

	s := New(0, 0)
	s.neighbors = func() ([]string, error) { return []string{"192.0.2.1"}, nil }
	var lookups atomic.Int32
	s.unicastLookup = func(string, int, string, time.Duration) ([]*mdns.ServiceEntry, error) {
		lookups.Add(1)
		return nil, nil
	}
	for i := 0; i < 6; i++ {
		s.queryOnce()
	}
	// Two directed passes (cycles 0 and 5), each asking for both services.
	if got := lookups.Load(); got != 4 {
		t.Fatalf("lookups = %d, want 4 (two passes of two services)", got)
	}
}

// TestUnicastCandidatesMergeNeighborsAndSeeds pins the candidate list: the
// neighbor table and remembered hosts, deduplicated, queried on the injected
// port for both service types.
func TestUnicastCandidatesMergeNeighborsAndSeeds(t *testing.T) {
	s := New(0, 0)
	s.unicastPort = 15353
	s.neighbors = func() ([]string, error) {
		return []string{"192.0.2.1", "192.0.2.2"}, nil
	}
	s.SetSeedHosts([]string{"192.0.2.2", "192.0.2.3", " "})

	var mu sync.Mutex
	seen := make(map[string]map[string]int)
	s.unicastLookup = func(host string, port int, service string, _ time.Duration) ([]*mdns.ServiceEntry, error) {
		if port != 15353 {
			t.Errorf("port = %d, want the injected 15353", port)
		}
		mu.Lock()
		defer mu.Unlock()
		if seen[host] == nil {
			seen[host] = make(map[string]int)
		}
		seen[host][service]++
		return nil, nil
	}
	s.unicastOnce()

	for _, host := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"} {
		if seen[host][raopService] != 1 || seen[host][airplayService] != 1 {
			t.Fatalf("host %s queried as %+v, want one query per service", host, seen[host])
		}
	}
	if len(seen) != 3 {
		t.Fatalf("candidates = %+v, want three deduplicated hosts", seen)
	}
}

// TestLiveUnicastQuery queries real addresses. Set
// SAUNDO_UNICAST_TEST=192.168.31.46,192.168.31.164 to run it; it is skipped
// in the default suite.
func TestLiveUnicastQuery(t *testing.T) {
	raw := os.Getenv("SAUNDO_UNICAST_TEST")
	if raw == "" {
		t.Skip("set SAUNDO_UNICAST_TEST=host[,host] to query the LAN")
	}
	for _, host := range strings.Split(raw, ",") {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		resolved := 0
		for _, service := range []string{raopService, airplayService} {
			entries, err := queryUnicast(host, defaultUnicastPort, service, time.Second)
			if err != nil {
				t.Logf("%s %s: %v", host, service, err)
				continue
			}
			for _, e := range entries {
				d, ok := parseEntry(e, service)
				if !ok {
					t.Errorf("%s %s: entry %+v did not parse", host, service, e)
					continue
				}
				resolved++
				t.Logf("%s %s: id=%s name=%q host=%s port=%d tsid=%q gpn=%q",
					host, service, d.ID, d.Name, d.Host, d.Port, d.StereoID, d.GroupName)
			}
		}
		if resolved == 0 {
			t.Errorf("%s answered no parsable AirPlay instance", host)
		}
	}
}

// TestLiveUnicastBrowse runs the service's directed pass over the real ARP
// neighbor table with multicast disabled. Set SAUNDO_UNICAST_BROWSE=1 to run
// it; it is skipped in the default suite.
func TestLiveUnicastBrowse(t *testing.T) {
	if os.Getenv("SAUNDO_UNICAST_BROWSE") == "" {
		t.Skip("set SAUNDO_UNICAST_BROWSE=1 to browse the LAN by directed queries")
	}
	prev := mdnsQuery
	mdnsQuery = func(*mdns.QueryParam) error { return nil }
	defer func() { mdnsQuery = prev }()

	s := New(0, 0)
	s.unicastEvery = 1
	s.unicastTimeout = time.Second
	s.queryOnce()

	devs := s.Snapshot()
	if len(devs) == 0 {
		t.Fatal("no receiver answered the directed pass")
	}
	groups := 0
	for _, d := range devs {
		t.Logf("directed: id=%q name=%q at %s:%d tsid=%q members=%d online=%v",
			d.ID, d.Name, d.Host, d.Port, d.StereoID, len(d.Members), d.Online)
		if d.IsStereoGroup() {
			groups++
		}
	}
	if groups == 0 {
		t.Log("no stereo group on this network")
	}
}

// TestParseIpNetTableFiltersUnusableNeighbors decodes a synthetic table: live
// dynamic and static rows survive, duplicate and invalid-type rows do not, and
// multicast, broadcast, and short buffers are rejected.
func TestParseIpNetTableFiltersUnusableNeighbors(t *testing.T) {
	const rows = 6
	buf := make([]byte, 4+rows*ipNetRowSize)
	binary.LittleEndian.PutUint32(buf[:4], rows)
	put := func(i int, ip string, typ uint32) {
		off := 4 + i*ipNetRowSize
		a := net.ParseIP(ip).To4()
		binary.LittleEndian.PutUint32(buf[off+ipNetAddrOffset:], binary.LittleEndian.Uint32(a))
		binary.LittleEndian.PutUint32(buf[off+ipNetTypeOffset:], typ)
	}
	put(0, "192.168.31.46", ipNetTypeDynamic)
	put(1, "192.168.31.164", ipNetTypeStatic)
	put(2, "224.0.0.251", ipNetTypeStatic)
	put(3, "255.255.255.255", ipNetTypeStatic)
	put(4, "192.168.31.46", ipNetTypeDynamic)
	put(5, "192.168.31.1", 2)

	got, err := parseIpNetTable(buf)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.168.31.164", "192.168.31.46"}
	if !slices.Equal(got, want) {
		t.Fatalf("neighbors = %v, want %v", got, want)
	}
	if _, err := parseIpNetTable([]byte{0, 1}); err == nil {
		t.Fatal("short table accepted")
	}
	overflow := make([]byte, 4)
	binary.LittleEndian.PutUint32(overflow, 2)
	if _, err := parseIpNetTable(overflow); err == nil {
		t.Fatal("row count beyond the buffer accepted")
	}
}
