// Copyright 2026 daikaiyu.
// SPDX-License-Identifier: GPL-3.0-only
//
// The sender-side PTP master is ported from AirFlash (GPLv3 or later),
// native/airflash-engine/src/clock.rs: header, timestamp, sync_pair, announce,
// unicast_grants, respond_to_delay, and the ptp_loop sender path
// (PeerState.follow == false). See NOTICE.
//
// A standalone HomePod on HomePod software 27 keeps its own PTP grandmaster and
// is tracked by Follower. A stereo pair instead expects the sender to supply
// the timeline (AirFlash docs/PTP-TROUBLESHOOTING.md: clock_role sender is
// correct for a stereo pair), so Master answers the pair's Delay_Req and
// subscription traffic as the grandmaster.
package ptp

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

const (
	masterAnnounceInterval = time.Second
	maxExchangeTimeout     = 12 * time.Second
)

// Master is a PTP grandmaster for one AirPlay 2 stereo pair. It owns one pair
// of UDP sockets for all peers and sends Sync/Follow_Up, Announce, and delay
// responses without touching the system clock.
type Master struct {
	clockID uint64
	peers   map[string]*masterPeer

	event   *net.UDPConn
	general *net.UDPConn

	mu      sync.Mutex
	started bool
	stopped bool

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

type masterPeer struct {
	ip        net.IP
	exchanges uint64
	first     time.Time
	last      time.Time

	// interval is the request's logMessageInterval converted to a duration.
	// Zero means no valid interval has been seen yet.
	interval    time.Duration
	requestKind uint8
	requestSeq  uint16
	requestAt   time.Time
}

// NewMaster binds UDP 319 and 320 for a grandmaster serving one or two
// distinct IPv4 peers.
func NewMaster(peerIPs []string) (*Master, error) {
	if len(peerIPs) == 0 || len(peerIPs) > 2 {
		return nil, fmt.Errorf("ptp: master wants 1 or 2 peers, got %d", len(peerIPs))
	}
	peers := make(map[string]*masterPeer, len(peerIPs))
	for _, raw := range peerIPs {
		ip := net.ParseIP(raw)
		if ip == nil {
			return nil, fmt.Errorf("ptp: bad peer address %q", raw)
		}
		v4 := ip.To4()
		if v4 == nil {
			return nil, fmt.Errorf("ptp: peer %q is not IPv4", raw)
		}
		key := v4.String()
		if _, exists := peers[key]; exists {
			return nil, fmt.Errorf("ptp: duplicate peer %q", raw)
		}
		peers[key] = &masterPeer{ip: v4}
	}
	event, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: eventPort})
	if err != nil {
		return nil, fmt.Errorf("ptp: bind event port %d: %w", eventPort, err)
	}
	general, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: generalPort})
	if err != nil {
		_ = event.Close()
		return nil, fmt.Errorf("ptp: bind general port %d: %w", generalPort, err)
	}
	return &Master{
		clockID: randomClockID(),
		peers:   peers,
		event:   event,
		general: general,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}, nil
}

// ClockID is this sender's PTP clock identity, which the master advertises in
// Sync, Announce, and delay responses.
func (m *Master) ClockID() uint64 { return m.clockID }

// Start begins the exchange loop. Repeated calls are ignored.
func (m *Master) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started || m.stopped {
		return
	}
	m.started = true
	go m.run()
}

// Stop ends the loop and closes both sockets. Repeated calls are ignored and
// the call waits for the loop when Start ran.
func (m *Master) Stop() {
	m.mu.Lock()
	m.stopped = true
	started := m.started
	m.mu.Unlock()
	m.stopOnce.Do(func() {
		close(m.stop)
		_ = m.event.Close()
		_ = m.general.Close()
		if started {
			<-m.done
		}
	})
}

// Ready reports whether the peer's delay exchanges are established and fresh.
func (m *Master) Ready(peerIP string) bool {
	peer := m.peer(peerIP)
	if peer == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return peer.ready(time.Now())
}

// ClockFor returns a per-peer view of the master clock for one member of the
// pair, or nil when peerIP is not a configured peer.
func (m *Master) ClockFor(peerIP string) *PeerClock {
	peer := m.peer(peerIP)
	if peer == nil {
		return nil
	}
	return &PeerClock{master: m, peer: peer}
}

// PeerClock is one receiver's view of the master clock. Its method set matches
// the sender's PTPClock interface without importing the sender package.
type PeerClock struct {
	master *Master
	peer   *masterPeer
}

var _ interface {
	ClockID() uint64
	Ready() bool
	Timeline(localUnixNs uint64) (id uint64, networkNs uint64, ok bool)
	Activate()
} = (*PeerClock)(nil)

// ClockID is the grandmaster clock identity both receivers must use.
func (c *PeerClock) ClockID() uint64 { return c.master.clockID }

// Ready reports whether this peer's delay exchanges are established and fresh.
func (c *PeerClock) Ready() bool {
	c.master.mu.Lock()
	defer c.master.mu.Unlock()
	return c.peer.ready(time.Now())
}

// Timeline maps local time onto the master clock, which is the local clock.
func (c *PeerClock) Timeline(localUnixNs uint64) (uint64, uint64, bool) {
	return c.master.clockID, localUnixNs, true
}

// Activate is a no-op: master mode starts sending when Start runs, so there is
// no receiver-side subscription to enable after SETPEERS.
func (c *PeerClock) Activate() {}

func (m *Master) peer(host string) *masterPeer {
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	v4 := ip.To4()
	if v4 == nil {
		return nil
	}
	return m.peers[v4.String()]
}

func (m *Master) run() {
	defer close(m.done)
	seq := uint16(time.Now().UnixNano())
	buf := make([]byte, maxPacket)
	next := time.Now()
	lastAnnounce := time.Now().Add(-masterAnnounceInterval)
	for {
		select {
		case <-m.stop:
			return
		default:
		}
		now := time.Now()
		if !now.Before(next) {
			m.sendRound(now, seq, &lastAnnounce)
			seq++
			next = now.Add(syncInterval)
		}
		m.readOne(m.event, buf)
		m.readOne(m.general, buf)
		time.Sleep(2 * time.Millisecond)
	}
}

func (m *Master) sendRound(now time.Time, seq uint16, lastAnnounce *time.Time) {
	for _, peer := range m.peers {
		syncPacket, followUp := syncPair(m.clockID, seq, uint64(time.Now().UnixNano()))
		m.send(m.event, syncPacket, peer.ip, eventPort)
		m.send(m.general, followUp, peer.ip, generalPort)
	}
	if now.Sub(*lastAnnounce) < masterAnnounceInterval {
		return
	}
	for _, peer := range m.peers {
		packet := masterAnnounce(m.clockID, seq, uint64(time.Now().UnixNano()))
		m.send(m.general, packet, peer.ip, generalPort)
	}
	*lastAnnounce = now
}

func (m *Master) readOne(conn *net.UDPConn, buf []byte) {
	_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))
	n, addr, err := conn.ReadFromUDP(buf)
	if err != nil {
		return
	}
	peer := m.peerFor(addr)
	if peer == nil {
		return
	}
	received := uint64(time.Now().UnixNano())
	m.mu.Lock()
	m.handle(peer, buf[:n], received, time.Now())
	m.mu.Unlock()
}

func (m *Master) peerFor(addr *net.UDPAddr) *masterPeer {
	if addr.Port != eventPort && addr.Port != generalPort {
		return nil
	}
	v4 := addr.IP.To4()
	if v4 == nil {
		return nil
	}
	return m.peers[v4.String()]
}

func (m *Master) handle(peer *masterPeer, data []byte, receivedNs uint64, now time.Time) {
	if rejection := packetRejection(data); rejection != "" {
		return
	}
	kind := data[0] & 0x0f
	seq := binary.BigEndian.Uint16(data[30:32])
	if kind == 12 {
		for _, grant := range unicastGrants(data, m.clockID, seq) {
			m.send(m.general, grant, peer.ip, generalPort)
		}
	}
	m.respondToDelay(peer, kind, seq, data, receivedNs, now)
}

func (m *Master) respondToDelay(peer *masterPeer, kind uint8, seq uint16, data []byte, receivedNs uint64, now time.Time) {
	if kind != 1 && kind != 2 {
		return
	}
	if !peer.requestAt.IsZero() && peer.requestKind == kind && peer.requestSeq == seq &&
		now.Sub(peer.requestAt) < peer.timeout() {
		return
	}
	var sent bool
	if kind == 1 {
		response := header(9, 54, m.clockID, seq, 0x0408, -3)
		writeTimestamp(response[34:44], receivedNs)
		copy(response[44:54], data[20:30])
		sent = m.send(m.general, response, peer.ip, generalPort)
	} else {
		response := header(3, 54, m.clockID, seq, 0x0608, -3)
		writeTimestamp(response[34:44], receivedNs)
		copy(response[44:54], data[20:30])
		responded := m.send(m.event, response, peer.ip, eventPort)

		followUp := header(10, 54, m.clockID, seq, 0x0408, -3)
		writeTimestamp(followUp[34:44], uint64(time.Now().UnixNano()))
		copy(followUp[44:54], data[20:30])
		followed := m.send(m.general, followUp, peer.ip, generalPort)
		sent = responded && followed
	}
	if !sent {
		return
	}
	peer.requestKind = kind
	peer.requestSeq = seq
	peer.requestAt = now
	// Only bounded, explicit intervals are usable. 0x7f/0x80 are unspecified.
	if interval := int8(data[33]); interval >= -7 && interval <= 2 {
		peer.interval = time.Duration(math.Pow(2, float64(interval)) * float64(time.Second))
	}
	peer.exchanged(now)
}

func (m *Master) send(conn *net.UDPConn, packet []byte, ip net.IP, port int) bool {
	written, err := conn.WriteToUDP(packet, &net.UDPAddr{IP: ip, Port: port})
	return err == nil && written == len(packet)
}

func (p *masterPeer) timeout() time.Duration {
	if p.interval == 0 {
		return exchangeTimeout
	}
	timeout := p.interval * 3
	if timeout < exchangeTimeout {
		return exchangeTimeout
	}
	if timeout > maxExchangeTimeout {
		return maxExchangeTimeout
	}
	return timeout
}

func (p *masterPeer) exchanged(now time.Time) {
	if !p.last.IsZero() && now.Before(p.last) {
		return
	}
	if p.last.IsZero() || now.Sub(p.last) >= p.timeout() {
		p.exchanges = 0
		p.first = now
	}
	p.last = now
	p.exchanges++
}

func (p *masterPeer) ready(now time.Time) bool {
	return p.exchanges >= 3 && !p.first.IsZero() && now.Sub(p.first) >= startupDwell &&
		!p.last.IsZero() && now.Sub(p.last) < p.timeout()
}

func unicastGrants(request []byte, clockID uint64, seq uint16) [][]byte {
	if len(request) < 44 {
		return nil
	}
	target := binary.BigEndian.Uint64(request[34:42]) == clockID &&
		binary.BigEndian.Uint16(request[42:44]) == 0x8005
	allFF := true
	for _, b := range request[34:44] {
		if b != 0xff {
			allFF = false
			break
		}
	}
	if !target && !allFF {
		return nil
	}
	var grants [][]byte
	for at := 44; at+4 <= len(request); {
		kind := binary.BigEndian.Uint16(request[at : at+2])
		length := int(binary.BigEndian.Uint16(request[at+2 : at+4]))
		if at+4+length > len(request) {
			break
		}
		if kind == 4 && length == 6 {
			body := request[at+4 : at+10]
			message := body[0] >> 4
			seconds := binary.BigEndian.Uint32(body[2:6])
			if (message == 0 || message == 9 || message == 11) && seconds > 0 {
				grant := header(12, 56, clockID, seq, 0x0408, 0)
				copy(grant[34:44], request[20:30])
				copy(grant[44:48], []byte{0, 5, 0, 8})
				copy(grant[48:54], body)
				if seconds > 30 {
					seconds = 30
				}
				binary.BigEndian.PutUint32(grant[50:54], seconds)
				grants = append(grants, grant)
			}
		}
		at += 4 + length
	}
	return grants
}

func masterAnnounce(clock uint64, seq uint16, ns uint64) []byte {
	packet := header(11, 76, clock, seq, 0x0408, 0)
	writeTimestamp(packet[34:44], ns)
	// A local oscillator; the priority is a preference, not a GPS claim.
	packet[47] = 96
	packet[48] = 248
	packet[49] = 0xfe
	binary.BigEndian.PutUint16(packet[50:52], 0xffff)
	packet[52] = 128
	binary.BigEndian.PutUint64(packet[53:61], clock)
	packet[63] = 0xa0
	copy(packet[64:68], []byte{0, 8, 0, 8})
	binary.BigEndian.PutUint64(packet[68:76], clock)
	return packet
}

func syncPair(clock uint64, seq uint16, ns uint64) (syncPacket, followUp []byte) {
	syncPacket = header(0, 44, clock, seq, 0x0608, -3)
	writeTimestamp(syncPacket[34:44], ns)

	followUp = header(8, 96, clock, seq, 0x0408, -3)
	writeTimestamp(followUp[34:44], ns)
	rate := make([]byte, 28)
	copy(rate, []byte{0, 0x80, 0xc2, 0, 0, 1})
	clockIdentity := make([]byte, 16)
	copy(clockIdentity, []byte{0, 0x0d, 0x93, 0, 0, 4})
	binary.BigEndian.PutUint64(clockIdentity[6:14], clock)
	copy(followUp[44:76], ptpTLV(rate))
	copy(followUp[76:96], ptpTLV(clockIdentity))
	return syncPacket, followUp
}

func ptpTLV(body []byte) []byte {
	out := make([]byte, 4+len(body))
	binary.BigEndian.PutUint16(out[0:2], 3)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(body)))
	copy(out[4:], body)
	return out
}
