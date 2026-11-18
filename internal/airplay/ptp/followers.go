// Copyright 2026 daikaiyu.
// SPDX-License-Identifier: GPL-3.0-only
//
// A session may hold several standalone PTP receivers, each keeping its own
// grandmaster. They cannot each bind UDP 319/320, so one hub owns the socket
// pair and demultiplexes the replies by source IPv4 address. The single-peer
// Follower stays for the sender's live probes; the engine uses Followers so a
// one-receiver and a many-receiver session take the same path.
package ptp

import (
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

// Followers tracks several standalone PTP receivers over one socket pair.
// Every receiver keeps its own grandmaster; packets are demultiplexed by the
// source IPv4 address, and each peer keeps its own offset and readiness.
type Followers struct {
	clockID  uint64
	event    *net.UDPConn
	general  *net.UDPConn
	peers    []*Follower
	byIP     map[string]*Follower
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	started  bool
	stopped  bool
}

// NewFollowers binds UDP 319 and 320 once and prepares one borrowed Follower
// per distinct IPv4 host. Start begins the shared exchange loop.
func NewFollowers(hosts []string) (*Followers, error) {
	return newFollowers(hosts, eventPort, generalPort)
}

// newFollowers takes the ports so tests can bind ephemeral ones while a
// running app holds 319/320.
func newFollowers(hosts []string, eventP, generalP int) (*Followers, error) {
	if len(hosts) == 0 {
		return nil, fmt.Errorf("ptp: follower set wants at least one peer, got 0")
	}
	ips := make([]net.IP, 0, len(hosts))
	seen := make(map[string]bool, len(hosts))
	for _, raw := range hosts {
		ip := net.ParseIP(raw)
		if ip == nil {
			return nil, fmt.Errorf("ptp: bad peer address %q", raw)
		}
		v4 := ip.To4()
		if v4 == nil {
			return nil, fmt.Errorf("ptp: peer %q is not IPv4", raw)
		}
		key := v4.String()
		if seen[key] {
			return nil, fmt.Errorf("ptp: duplicate peer %q", raw)
		}
		seen[key] = true
		ips = append(ips, v4)
	}
	event, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: eventP})
	if err != nil {
		return nil, fmt.Errorf("ptp: bind event port %d: %w", eventP, err)
	}
	general, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: generalP})
	if err != nil {
		_ = event.Close()
		return nil, fmt.Errorf("ptp: bind general port %d: %w", generalP, err)
	}
	g := &Followers{
		clockID: randomClockID(),
		event:   event,
		general: general,
		byIP:    make(map[string]*Follower, len(ips)),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	for _, ip := range ips {
		peer := &Follower{peer: ip, clockID: g.clockID, event: event, general: general, hub: g}
		g.peers = append(g.peers, peer)
		g.byIP[ip.String()] = peer
	}
	return g, nil
}

// ClockFor returns the peer tracking one receiver, or nil when the host is not
// part of this follower set.
func (g *Followers) ClockFor(host string) *Follower {
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	v4 := ip.To4()
	if v4 == nil {
		return nil
	}
	return g.byIP[v4.String()]
}

// ClockID is the one sender clock identity every peer advertises in its
// subscriptions and delay requests.
func (g *Followers) ClockID() uint64 { return g.clockID }

// Start begins the shared exchange loop. Repeated calls, and a call after
// Stop, are ignored.
func (g *Followers) Start() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started || g.stopped {
		return
	}
	g.started = true
	go g.run()
}

// Stop ends the loop and closes both shared sockets. Repeated calls are
// ignored and the call waits for the loop when Start ran.
func (g *Followers) Stop() {
	g.mu.Lock()
	g.stopped = true
	started := g.started
	g.mu.Unlock()
	g.stopOnce.Do(func() {
		close(g.stop)
		_ = g.event.Close()
		_ = g.general.Close()
		if started {
			<-g.done
		}
	})
}

// Ready is true only when every peer has established its own offset.
func (g *Followers) Ready() bool {
	for _, peer := range g.peers {
		if !peer.Ready() {
			return false
		}
	}
	return len(g.peers) > 0
}

// Stats aggregates the peers for diagnostics: counters are summed and the
// offset is the largest absolute value across the set.
func (g *Followers) Stats() Stats {
	out := Stats{ClockID: g.clockID, Ready: len(g.peers) > 0}
	var largest float64
	for _, peer := range g.peers {
		st := peer.Stats()
		if !st.Ready {
			out.Ready = false
		}
		out.Received += st.Received
		out.Rejected += st.Rejected
		out.Exchanges += st.Exchanges
		if abs := math.Abs(st.OffsetNS); abs > largest {
			largest = abs
		}
	}
	out.OffsetNS = largest
	return out
}

func (g *Followers) run() {
	defer close(g.done)
	seq := uint16(time.Now().UnixNano())
	buf := make([]byte, maxPacket)
	next := time.Now()
	for {
		select {
		case <-g.stop:
			return
		default:
		}
		now := time.Now()
		if !now.Before(next) {
			g.sendRound(now, seq)
			seq++
			next = now.Add(syncInterval)
		}
		g.readOne(g.event, buf)
		g.readOne(g.general, buf)
		time.Sleep(2 * time.Millisecond)
	}
}

// sendRound gives every live peer one round with the shared sequence. Each
// peer records its own subscription and delay request under that sequence and
// matches its own reply.
func (g *Followers) sendRound(now time.Time, seq uint16) {
	for _, peer := range g.peers {
		if peer.isStopped() {
			continue
		}
		peer.sendRound(now, seq)
	}
}

func (g *Followers) readOne(conn *net.UDPConn, buf []byte) {
	_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))
	n, addr, err := conn.ReadFromUDP(buf)
	if err != nil {
		return
	}
	g.dispatch(addr, buf[:n], uint64(time.Now().UnixNano()))
}

// dispatch routes one datagram to the peer whose address it came from. A
// packet from an unknown source is ignored: it belongs to no receiver in this
// session.
func (g *Followers) dispatch(from *net.UDPAddr, data []byte, localNs uint64) {
	if from == nil || (from.Port != eventPort && from.Port != generalPort) {
		return
	}
	v4 := from.IP.To4()
	if v4 == nil {
		return
	}
	peer := g.byIP[v4.String()]
	if peer == nil {
		return
	}
	peer.handlePacket(data, from, localNs)
}

// isStopped reports whether this peer was stopped on its own. The hub keeps
// running for the remaining peers.
func (f *Follower) isStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}
