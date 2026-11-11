// Package discovery browses mDNS for AirPlay receivers and aggregates the
// _raop and _airplay advertisements of each device. A multicast browse is the
// primary path; a directed DNS-SD query to known addresses covers receivers
// whose multicast traffic does not reach this PC.
package discovery

import (
	"io"
	"log"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"

	"saundo/internal/airplay/device"
)

// quietMDNS drops the IPv6 bind failures the mDNS client retries on every
// query when the host has no usable IPv6 multicast. Other mDNS messages
// still reach the app log.
type quietMDNS struct{ out io.Writer }

func (w quietMDNS) Write(p []byte) (int, error) {
	s := string(p)
	if strings.Contains(s, "Failed to bind to udp6 port") ||
		strings.Contains(s, "Failed to listen to both unicast and multicast on IPv6") {
		return len(p), nil
	}
	return w.out.Write(p)
}

var mdnsLog = log.New(quietMDNS{out: log.Default().Writer()}, "", log.LstdFlags)

const (
	raopService    = "_raop._tcp"
	airplayService = "_airplay._tcp"
)

// mdnsQuery is the query entry point, swappable in tests.
var mdnsQuery = mdns.Query

// Service continuously browses both AirPlay service types.
type Service struct {
	mu      sync.Mutex
	devices map[string]*seen
	updates chan []device.Device

	queryInterval time.Duration
	queryTimeout  time.Duration
	// staleAfter marks a device offline when it has not been seen this long.
	staleAfter time.Duration
	// dropAfter removes a stale device from the list entirely.
	dropAfter time.Duration
	now       func() time.Time
	stop      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once

	// Unicast fallback. The neighbors lookup and the query are injected so
	// tests can supply a loopback DNS server and a fixed address list.
	neighbors      func() ([]string, error)
	unicastLookup  unicastLookupFunc
	unicastPort    int
	unicastTimeout time.Duration
	unicastEvery   int
	cycles         int
	seedsMu        sync.Mutex
	seeds          []string

	// interfaceAddrs lists this host's own addresses, injected so tests can
	// mark a device local. localSet keeps the last successful read: a failed
	// read is unknown, not empty, so the previous set stands.
	interfaceAddrs func() ([]net.Addr, error)
	localMu        sync.Mutex
	localSet       map[netip.Addr]struct{}
}

type serviceEndpoint struct {
	host string
	port uint16
	when time.Time
}

type seen struct {
	dev     device.Device
	when    time.Time
	raop    serviceEndpoint
	airplay serviceEndpoint
}

// recordEndpoint keeps each service's own SRV endpoint. The merge cannot pick
// one at reply time: the _raop and _airplay browses race, and a later lost
// reply must not leave the other service's stale address in place.
func (e *seen) recordEndpoint(d device.Device, now time.Time) {
	if d.Host == "" || d.Port == 0 {
		return
	}
	ep := serviceEndpoint{host: d.Host, port: d.Port, when: now}
	if d.AirPlay2 {
		e.airplay = ep
		return
	}
	e.raop = ep
}

// endpointFor chooses the control endpoint for the receiver's auth path. HAP
// and open AirPlay 2 receivers use the _airplay SRV port; an AirPort or AP1
// receiver uses _raop. A preferred endpoint older than the staleness window
// falls back to the other service, so a lost _airplay reply cannot pin a
// stale address.
func (e *seen) endpointFor(dev device.Device, now time.Time, stale time.Duration) (string, uint16) {
	preferAirPlay := dev.Auth.IsHAP() || (dev.AirPlay2 && dev.Auth == device.AuthNone)
	preferred, fallback := e.raop, e.airplay
	if preferAirPlay {
		preferred, fallback = e.airplay, e.raop
	}
	fresh := func(ep serviceEndpoint) bool {
		return ep.host != "" && ep.port != 0 && now.Sub(ep.when) < stale
	}
	if fresh(preferred) {
		return preferred.host, preferred.port
	}
	if fresh(fallback) {
		return fallback.host, fallback.port
	}
	if e.airplay.when.After(e.raop.when) {
		return e.airplay.host, e.airplay.port
	}
	return e.raop.host, e.raop.port
}

// New returns a discovery service. Query timing is tunable for tests.
func New(queryInterval, queryTimeout time.Duration) *Service {
	if queryInterval <= 0 {
		queryInterval = 3 * time.Second
	}
	if queryTimeout <= 0 {
		queryTimeout = 2 * time.Second
	}
	return &Service{
		devices:       make(map[string]*seen),
		updates:       make(chan []device.Device, 1),
		queryInterval: queryInterval,
		queryTimeout:  queryTimeout,
		// The offline window must outlast a full directed-pass period plus its
		// timeout. A receiver reachable only through that pass would otherwise
		// flap offline between passes.
		staleAfter: max(3*queryInterval+queryTimeout,
			time.Duration(unicastEveryPasses)*queryInterval+queryTimeout),
		dropAfter:      90 * time.Second,
		now:            time.Now,
		stop:           make(chan struct{}),
		neighbors:      neighborHosts,
		unicastLookup:  queryUnicast,
		unicastPort:    defaultUnicastPort,
		unicastTimeout: 500 * time.Millisecond,
		unicastEvery:   unicastEveryPasses,
		interfaceAddrs: net.InterfaceAddrs,
	}
}

// Start begins browsing. It returns immediately.
func (s *Service) Start() {
	s.startOnce.Do(func() {
		go s.run()
	})
}

// Stop ends browsing. It does not join an in-flight query: mdns queries run
// up to queryTimeout with no way to interrupt them, and joining one stalled
// Engine.Stop — and with it the UI quit — for seconds. The run loop exits on
// its own as soon as the in-flight query returns.
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
}

// Snapshot returns the current devices sorted by name, each marked Online
// when discovery saw it within staleAfter.
func (s *Service) Snapshot() []device.Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	out := make([]device.Device, 0, len(s.devices))
	pairs := make(map[string][]device.Device)
	for _, d := range s.devices {
		dev := d.dev
		dev.Online = now.Sub(d.when) < s.staleAfter
		dev.Host, dev.Port = d.endpointFor(dev, now, s.staleAfter)
		if dev.StereoID != "" {
			pairs[dev.StereoID] = append(pairs[dev.StereoID], dev)
			continue
		}
		out = append(out, dev)
	}
	for tsid, members := range pairs {
		out = append(out, stereoGroup(tsid, members))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// stereoGroup collapses a stereo pair into one logical target. The physical
// members leave the list: a pair is addressed as a whole, never one half.
func stereoGroup(tsid string, members []device.Device) device.Device {
	sort.Slice(members, func(i, j int) bool {
		if members[i].GroupLeader != members[j].GroupLeader {
			return members[i].GroupLeader
		}
		if members[i].Name == members[j].Name {
			return members[i].ID < members[j].ID
		}
		return members[i].Name < members[j].Name
	})
	leader := members[0]
	for _, m := range members {
		if m.GroupLeader {
			leader = m
			break
		}
	}
	g := leader
	g.ID = device.StereoGroupID(tsid)
	g.StereoID = tsid
	g.Members = append([]device.Device(nil), members...)
	g.GroupLeader = false
	if leader.GroupName != "" {
		g.Name = leader.GroupName
	}
	g.Online = len(members) == 2
	for _, m := range members {
		if !m.Online {
			g.Online = false
		}
	}
	return g
}

// Updates delivers a snapshot after each discovery cycle. The newest snapshot
// replaces an unread one.
func (s *Service) Updates() <-chan []device.Device { return s.updates }

func (s *Service) run() {
	ticker := time.NewTicker(s.queryInterval)
	defer ticker.Stop()
	s.queryOnce()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.queryOnce()
		}
	}
}

func (s *Service) queryOnce() {
	local := s.localAddresses()
	var wg sync.WaitGroup
	// The directed pass is parallel to the browse so a slow neighbor does not
	// extend the cycle by its full timeout.
	if s.unicastDue() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.unicastOnce()
		}()
	}
	for _, service := range []string{raopService, airplayService} {
		wg.Add(1)
		go func(service string) {
			defer wg.Done()
			entries := make(chan *mdns.ServiceEntry, 32)
			var read sync.WaitGroup
			read.Add(1)
			go func() {
				defer read.Done()
				for e := range entries {
					if d, ok := parseEntry(e, service); ok {
						d.Local = localHost(local, d.Host)
						s.merge(d)
					}
				}
			}()
			_ = mdnsQuery(&mdns.QueryParam{
				Service: service,
				Domain:  "local.",
				Timeout: s.queryTimeout,
				Entries: entries,
				Logger:  mdnsLog,
			})
			close(entries)
			read.Wait()
		}(service)
	}
	wg.Wait()
	s.prune()
	s.publish()
}

func (s *Service) merge(d device.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	prev, ok := s.devices[d.ID]
	if !ok {
		e := &seen{dev: d, when: now}
		e.recordEndpoint(d, now)
		s.devices[d.ID] = e
		return
	}
	merged := prev.dev
	if d.Name != "" {
		merged.Name = d.Name
	}
	// The local flag is a safety rule, not a fact about the last reply: one
	// record from this host keeps the receiver out of the target list.
	merged.Local = merged.Local || d.Local
	if d.Model != "" {
		merged.Model = d.Model
	}
	if d.OSVersion != "" {
		merged.OSVersion = d.OSVersion
	}
	if !d.FeaturesUnknown && d.Features != 0 {
		merged.Features = d.Features
	}
	// One advertisement with a parsed features value makes the device known,
	// even when the other service's record carried none.
	merged.FeaturesUnknown = merged.FeaturesUnknown && d.FeaturesUnknown
	if d.SF != 0 {
		merged.SF = d.SF
	}
	if d.AM != "" {
		merged.AM = d.AM
	}
	if d.GroupName != "" {
		merged.GroupName = d.GroupName
	}
	if d.StereoID != "" {
		merged.StereoID = d.StereoID
	}
	merged.GroupLeader = merged.GroupLeader || d.GroupLeader
	merged.PasswordRequired = merged.PasswordRequired || d.PasswordRequired
	merged.AirPlay2 = merged.AirPlay2 || d.AirPlay2
	merged.Family = device.FamilyFor(merged.Model, merged.AM)
	merged.Auth = chooseAuth(merged)
	prev.dev = merged
	prev.when = now
	prev.recordEndpoint(d, now)
}

func (s *Service) prune() {
	cutoff := s.now().Add(-s.dropAfter)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, d := range s.devices {
		if d.when.Before(cutoff) {
			delete(s.devices, id)
		}
	}
}

func (s *Service) publish() {
	snap := s.Snapshot()
	select {
	case s.updates <- snap:
	default:
		select {
		case <-s.updates:
		default:
		}
		select {
		case s.updates <- snap:
		default:
		}
	}
}

// localAddresses returns the host's own addresses as a set. A failed read
// keeps the previous set; before the first successful read the set is nil and
// localHost marks nothing, so an unknown host stays a candidate.
func (s *Service) localAddresses() map[netip.Addr]struct{} {
	addrs, err := s.interfaceAddrs()
	if err != nil {
		s.localMu.Lock()
		defer s.localMu.Unlock()
		return s.localSet
	}
	set := make(map[netip.Addr]struct{}, len(addrs))
	for _, a := range addrs {
		if ip, ok := addrIP(a); ok {
			set[ip.Unmap()] = struct{}{}
		}
	}
	s.localMu.Lock()
	s.localSet = set
	s.localMu.Unlock()
	return set
}

// addrIP extracts the address from the net.Addr forms InterfaceAddrs returns.
func addrIP(a net.Addr) (netip.Addr, bool) {
	var raw net.IP
	switch v := a.(type) {
	case *net.IPNet:
		raw = v.IP
	case *net.IPAddr:
		raw = v.IP
	}
	if raw == nil {
		return netip.Addr{}, false
	}
	ip, ok := netip.AddrFromSlice(raw)
	if !ok {
		return netip.Addr{}, false
	}
	return ip, true
}

// localHost reports whether host is one of this host's own addresses. A
// hostname is unknown, not local; the advertisement carries an IP.
func localHost(set map[netip.Addr]struct{}, host string) bool {
	if len(set) == 0 {
		return false
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	if ip.Zone() != "" {
		ip = ip.WithZone("")
	}
	_, ok := set[ip]
	return ok
}

func parseEntry(e *mdns.ServiceEntry, service string) (device.Device, bool) {
	if e == nil {
		return device.Device{}, false
	}
	txt := parseTXT(e.InfoFields)
	id := txt["deviceid"]
	// hashicorp/mdns reports records from every answer and extra section to
	// whichever browse is listening, so an `_airplay` entry can arrive while
	// parsing `_raop` (and the reverse). The instance name says which service
	// the entry belongs to; the query that delivered it does not.
	rawName := unescapeDNS(e.Name)
	service = serviceForName(rawName, service)
	name := trimServiceSuffix(rawName, service)
	if service == raopService {
		// The RAOP instance is "<MAC>@<friendly name>". The MAC is the same
		// deviceid the AirPlay service advertises, so it joins the two
		// advertisements into one device.
		if at := strings.IndexByte(name, '@'); at >= 0 {
			id = idOrInstance(id, name[:at])
			name = name[at+1:]
		}
	}
	if id == "" {
		return device.Device{}, false
	}
	id = normalizeDeviceID(id)
	host := ""
	switch {
	case e.AddrV4 != nil:
		host = e.AddrV4.String()
	case e.Addr != nil && e.Addr.To4() != nil:
		host = e.Addr.String()
	case e.Addr != nil || e.AddrV6IPAddr != nil:
		// The sender's sockets are IPv4-only. An AAAA-only advertisement
		// would hand over a zone-less IPv6 literal and fail at connect, so
		// fail closed here instead.
		return device.Device{}, false
	default:
		host = e.Host
	}
	if host == "" || e.Port <= 0 {
		return device.Device{}, false
	}
	featuresTXT := txt["features"]
	if featuresTXT == "" {
		featuresTXT = txt["ft"]
	}
	featureBits, featuresOK := parseFeatures(featuresTXT)
	sf, _ := parseHex(txt["sf"])
	// A RAOP-only advertisement has no model field; `am` names the model.
	model := txt["model"]
	if model == "" {
		model = txt["am"]
	}
	leader := txt["igl"] == "1" || strings.EqualFold(txt["igl"], "true")
	d := device.Device{
		ID:               id,
		Name:             name,
		Host:             host,
		Port:             uint16(e.Port),
		Model:            model,
		OSVersion:        strings.TrimSpace(txt["osvers"]),
		Features:         featureBits,
		FeaturesUnknown:  !featuresOK,
		SF:               sf,
		AM:               txt["am"],
		PasswordRequired: strings.EqualFold(txt["pw"], "true") || strings.EqualFold(txt["pw"], "1") || sf&0x80 != 0,
		AirPlay2:         service == airplayService,
		Family:           device.FamilyFor(model, txt["am"]),
		GroupName:        unescapeDNS(txt["gpn"]),
		GroupLeader:      leader,
		StereoID:         strings.TrimSpace(txt["tsid"]),
	}
	d.Auth = chooseAuth(d)
	return d, true
}

// unescapeDNS decodes the RFC 1035 escapes hashicorp/mdns reports in instance
// names and TXT strings. `\DDD` is one decimal byte and `\X` is the literal
// character X, so a UTF-8 name arrives as a run of three-digit escapes
// (HomePod\ R becomes HomePod R, and a `gpn` of 卧室 stays escaped until it
// passes through here).
func unescapeDNS(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		if i+3 < len(s) && isASCIIDigit(s[i+1]) && isASCIIDigit(s[i+2]) && isASCIIDigit(s[i+3]) {
			v := int(s[i+1]-'0')*100 + int(s[i+2]-'0')*10 + int(s[i+3]-'0')
			if v <= 255 {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		i++
		b.WriteByte(s[i])
	}
	return b.String()
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

// serviceForName returns the AirPlay service an advertisement belongs to,
// falling back to the queried service when the instance name carries no
// known suffix.
func serviceForName(name, fallback string) string {
	for _, service := range []string{raopService, airplayService} {
		for _, suffix := range serviceSuffixes(service) {
			if _, ok := trimSuffixFold(name, suffix); ok {
				return service
			}
		}
	}
	return fallback
}

func serviceSuffixes(service string) []string {
	return []string{"." + service + ".local.", "." + service + ".local", "." + service}
}

func trimServiceSuffix(name, service string) string {
	for _, suffix := range serviceSuffixes(service) {
		if trimmed, ok := trimSuffixFold(name, suffix); ok {
			name = trimmed
			break
		}
	}
	for _, suffix := range []string{".local.", ".local"} {
		if trimmed, ok := trimSuffixFold(name, suffix); ok {
			name = trimmed
			break
		}
	}
	return strings.TrimSpace(name)
}

// trimSuffixFold removes suffix from s when the tail matches without regard
// to DNS name case.
func trimSuffixFold(s, suffix string) (string, bool) {
	if len(s) < len(suffix) || !strings.EqualFold(s[len(s)-len(suffix):], suffix) {
		return s, false
	}
	return s[:len(s)-len(suffix)], true
}

func idOrInstance(id, instance string) string {
	if id != "" {
		return id
	}
	return instance
}

// normalizeDeviceID turns a bare 12-digit MAC into colon-separated form.
func normalizeDeviceID(id string) string {
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) == 12 && !strings.Contains(clean, ":") {
		var b strings.Builder
		for i := 0; i < 12; i += 2 {
			if i > 0 {
				b.WriteByte(':')
			}
			b.WriteString(clean[i : i+2])
		}
		return strings.ToUpper(b.String())
	}
	return strings.ToUpper(id)
}

func parseTXT(fields []string) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}

// parseFeatures decodes "0xLOW,0xHIGH" or a single 0x value. The bool is
// false when a word does not parse, so the caller keeps unknown distinct from
// a legitimate zero.
func parseFeatures(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	parts := strings.Split(s, ",")
	if len(parts) == 2 {
		low, ok1 := parseHexWord(parts[0])
		high, ok2 := parseHexWord(parts[1])
		if !ok1 || !ok2 {
			return 0, false
		}
		return high<<32 | low, true
	}
	return parseHex(s)
}

// parseHexWord reads one 32-bit word of the two-part feature value. Both the
// lowercase and uppercase prefix are accepted, and the word is always hex.
func parseHexWord(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseHex(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	base := 10
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
		base = 16
	} else if strings.ContainsAny(s, "abcdefABCDEF") {
		base = 16
	}
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

const (
	featureHKPairing = 1 << 46
	featureCoreUtils = 1 << 48
)

// chooseAuth maps the advertisement onto the pairing path the sender runs.
func chooseAuth(d device.Device) device.Auth {
	if strings.HasPrefix(d.AM, "AirPort") {
		return device.AuthSetup
	}
	if d.FeaturesUnknown && (strings.HasPrefix(d.Model, "AudioAccessory") || strings.HasPrefix(d.Model, "Mac")) {
		// A failed features parse must not take the permissive AP1 path for a
		// family that pairs over HAP.
		if d.PasswordRequired {
			return device.AuthHAPPassword
		}
		return device.AuthHAPTransient
	}
	hap := d.Features&featureHKPairing != 0 || d.Features&featureCoreUtils != 0
	switch {
	case hap && d.PasswordRequired:
		// A HAP receiver with Require Password gates HAP pair-setup, not the
		// AP1 digest. The family is decided separately; a HomePod and a Mac
		// both land here.
		return device.AuthHAPPassword
	case hap && strings.HasPrefix(d.Model, "AppleTV"):
		return device.AuthHAPPIN
	case hap:
		return device.AuthHAPTransient
	case d.PasswordRequired:
		return device.AuthPassword
	case strings.HasPrefix(d.Model, "AppleTV3,"):
		// Apple TV 3-style receivers use FairPlay for audio, which this sender
		// does not implement. Fail fast instead of offering a session that
		// connects but never produces sound.
		return device.AuthLegacyPin
	default:
		return device.AuthNone
	}
}
