package discovery

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/miekg/dns"
)

// Directed DNS-SD fallback. Some receivers answer a unicast query on port 5353
// while their multicast advertisements never reach this PC. A directed PTR
// query returns the instance, and its SRV/TXT/A records resolve the same
// fields the multicast path parses.

const (
	defaultUnicastPort = 5353
	// unicastEveryPasses is one directed pass every five query cycles: 15 s at
	// the default 3 s interval, not a probe of every neighbor every cycle.
	unicastEveryPasses = 5
	// unicastConcurrency bounds the number of neighbors queried at once.
	unicastConcurrency = 16
)

// unicastLookupFunc resolves one service type from one address. It returns
// synthesized mDNS entries, which flow through parseEntry exactly like
// multicast results.
type unicastLookupFunc func(host string, port int, service string, timeout time.Duration) ([]*mdns.ServiceEntry, error)

// SetSeedHosts replaces the addresses the directed pass tries in addition to
// the neighbor table. The engine supplies manual and last-known receiver
// hosts; the caller may call it from another goroutine.
func (s *Service) SetSeedHosts(hosts []string) {
	s.seedsMu.Lock()
	s.seeds = append(s.seeds[:0], hosts...)
	s.seedsMu.Unlock()
}

// unicastDue reports whether this cycle runs a directed pass and advances the
// cycle counter. The first cycle counts, so a launch probes immediately.
func (s *Service) unicastDue() bool {
	if s.unicastEvery <= 0 {
		return false
	}
	due := s.cycles%s.unicastEvery == 0
	s.cycles++
	return due
}

// unicastOnce probes every candidate address for both service types. A parse
// error or a missing record skips that instance; it never synthesizes an empty
// device.
func (s *Service) unicastOnce() {
	hosts := s.candidateHosts()
	if len(hosts) == 0 {
		return
	}
	local := s.localAddresses()
	sem := make(chan struct{}, unicastConcurrency)
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var services sync.WaitGroup
			for _, service := range []string{raopService, airplayService} {
				services.Add(1)
				go func(service string) {
					defer services.Done()
					entries, err := s.unicastLookup(host, s.unicastPort, service, s.unicastTimeout)
					if err != nil {
						return
					}
					for _, e := range entries {
						if d, ok := parseEntry(e, service); ok {
							d.Local = localHost(local, d.Host)
							s.merge(d)
						}
					}
				}(service)
			}
			services.Wait()
		}(host)
	}
	wg.Wait()
}

// candidateHosts is the neighbor table plus the engine's remembered addresses,
// deduplicated. A failed neighbor read drops only that source; known hosts
// still count.
func (s *Service) candidateHosts() []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(host string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		key := strings.ToLower(host)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, host)
	}
	if s.neighbors != nil {
		if hosts, err := s.neighbors(); err == nil {
			for _, host := range hosts {
				add(host)
			}
		}
	}
	s.seedsMu.Lock()
	for _, host := range s.seeds {
		add(host)
	}
	s.seedsMu.Unlock()
	return out
}

// queryUnicast asks one address for a service's PTR records, then resolves the
// SRV, TXT, and A records the responder did not include. Answers may arrive in
// the additional section, as Apple's responders send them, or from follow-up
// queries.
func queryUnicast(host string, port int, service string, timeout time.Duration) ([]*mdns.ServiceEntry, error) {
	if host == "" || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("discovery: bad unicast address %q:%d", host, port)
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client := &dns.Client{Net: "udp", Timeout: timeout, UDPSize: 8192}
	serviceName := dns.CanonicalName(service + ".local.")
	resp, err := dnsExchange(client, addr, serviceName, dns.TypePTR)
	if err != nil {
		return nil, err
	}
	index := newRecordIndex()
	index.add(resp)
	var out []*mdns.ServiceEntry
	for _, instance := range index.ptrTargets(serviceName) {
		if entry, ok := index.resolve(client, addr, instance); ok {
			out = append(out, entry)
		}
	}
	return out, nil
}

func dnsExchange(client *dns.Client, addr, name string, qtype uint16) (*dns.Msg, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.CanonicalName(name), qtype)
	msg.RecursionDesired = false
	resp, _, err := client.Exchange(msg, addr)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("discovery: empty DNS response from %s", addr)
	}
	if resp.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("discovery: %s from %s: %s", name, addr, dns.RcodeToString[resp.Rcode])
	}
	return resp, nil
}

type instanceRecords struct {
	srv *dns.SRV
	txt *dns.TXT
	a   *dns.A
}

// recordIndex collects the records of one or more responses by owner name.
type recordIndex struct {
	byName map[string]*instanceRecords
	ptrs   map[string][]string
}

func newRecordIndex() *recordIndex {
	return &recordIndex{
		byName: make(map[string]*instanceRecords),
		ptrs:   make(map[string][]string),
	}
}

func (ix *recordIndex) records(name string) *instanceRecords {
	rec, ok := ix.byName[name]
	if !ok {
		rec = &instanceRecords{}
		ix.byName[name] = rec
	}
	return rec
}

func (ix *recordIndex) add(msg *dns.Msg) {
	if msg == nil {
		return
	}
	for _, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		for _, rr := range section {
			switch r := rr.(type) {
			case *dns.PTR:
				owner := dns.CanonicalName(r.Hdr.Name)
				ix.ptrs[owner] = append(ix.ptrs[owner], r.Ptr)
			case *dns.SRV:
				ix.records(dns.CanonicalName(r.Hdr.Name)).srv = r
			case *dns.TXT:
				ix.records(dns.CanonicalName(r.Hdr.Name)).txt = r
			case *dns.A:
				ix.records(dns.CanonicalName(r.Hdr.Name)).a = r
			}
		}
	}
}

// ptrTargets returns the instances one service advertised, deduplicated.
func (ix *recordIndex) ptrTargets(service string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, target := range ix.ptrs[service] {
		key := dns.CanonicalName(target)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	return out
}

// resolve completes one instance and builds the ServiceEntry parseEntry
// expects. A missing SRV, TXT, or A record skips the instance: an unknown
// address is not a device with an empty address.
func (ix *recordIndex) resolve(client *dns.Client, addr, instance string) (*mdns.ServiceEntry, bool) {
	key := dns.CanonicalName(instance)
	if rec := ix.byName[key]; rec == nil || rec.srv == nil {
		resp, err := dnsExchange(client, addr, key, dns.TypeSRV)
		if err != nil {
			return nil, false
		}
		ix.add(resp)
	}
	rec := ix.byName[key]
	if rec == nil || rec.srv == nil {
		return nil, false
	}
	if rec.txt == nil {
		if resp, err := dnsExchange(client, addr, key, dns.TypeTXT); err == nil {
			ix.add(resp)
		}
	}
	rec = ix.byName[key]
	if rec == nil || rec.txt == nil {
		return nil, false
	}
	target := dns.CanonicalName(rec.srv.Target)
	host := ix.byName[target]
	if host == nil || host.a == nil {
		if resp, err := dnsExchange(client, addr, target, dns.TypeA); err == nil {
			ix.add(resp)
		}
	}
	host = ix.byName[target]
	if host == nil || host.a == nil || host.a.A == nil || host.a.A.IsUnspecified() || host.a.A.IsMulticast() {
		return nil, false
	}
	if rec.srv.Port == 0 {
		return nil, false
	}
	return &mdns.ServiceEntry{
		Name:       instance,
		Host:       rec.srv.Target,
		AddrV4:     host.a.A,
		Port:       int(rec.srv.Port),
		InfoFields: rec.txt.Txt,
	}, true
}
