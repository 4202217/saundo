// Copyright 2026 daikaiyu.
// SPDX-License-Identifier: GPL-3.0-only
//
// The AirPlay PTP wire profile and the follower state machine are ported from
// AirFlash (GPLv3 or later), native/airflash-engine/src/clock.rs. See NOTICE.
//
// A standalone HomePod on HomePod software 27 keeps its own PTP grandmaster and
// ignores an NTP-timed session. The sender therefore subscribes to the
// receiver's Announce, Sync/Follow_Up, and Delay_Resp messages, estimates the
// offset between the receiver's clock and the local monotonic wall clock, and
// maps RTP timestamps onto the receiver's timeline.
package ptp

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

const (
	eventPort   = 319
	generalPort = 320

	maxPacket            = 2048
	sampleLifetime       = 2 * time.Second
	subscriptionRetry    = 500 * time.Millisecond
	delayRequestInterval = time.Second
	syncInterval         = 125 * time.Millisecond
	exchangeTimeout      = 3 * time.Second
	startupDwell         = 250 * time.Millisecond
)

// Stats describes the follower's view of the receiver clock.
type Stats struct {
	Ready       bool
	ClockID     uint64
	SourceID    uint64
	OffsetNS    float64
	Exchanges   uint64
	Received    uint64
	Rejected    uint64
	RoundTripNS float64
	HasDelay    bool
}

type pendingSync struct {
	sequence uint16
	at       time.Time
	sync     *sampleAt
	follow   *sampleAt
	complete bool
}

type sampleAt struct {
	value      uint64
	correction float64
}

type syncSample struct {
	sequence   uint16
	origin     uint64
	received   uint64
	correction float64
	at         time.Time
}

type delayRequest struct {
	sequence uint16
	sent     uint64
	at       time.Time
}

type subscription struct {
	sequence uint16
	interval int8
	sent     time.Time
}

type state struct {
	active bool

	clockID    uint64
	sourceID   uint64
	sourcePort uint16

	pending       []pendingSync
	syncPair      *syncSample
	syncOffset    float64
	syncOffsetSet bool

	delay       *delayRequest
	delayOffset float64
	delayAt     time.Time
	delaySet    bool
	roundTrip   float64

	exchanges uint64
	first     time.Time
	last      time.Time

	subs         [3][]subscription
	grantedUntil [3]time.Time
	nextSub      time.Time
	lastDelay    time.Time

	received uint64
	rejected uint64
}

// Follower tracks one receiver's PTP clock.
type Follower struct {
	peer    net.IP
	clockID uint64

	event   *net.UDPConn
	general *net.UDPConn

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	// OnPacket, when set before Start, receives validated packets for
	// diagnostics. It runs on the follower goroutine.
	OnPacket func(kind uint8, seq uint16, data []byte)

	mu      sync.Mutex
	started bool
	stopped bool
	st      state
}

// New binds the PTP event and general ports and prepares a follower for the
// receiver at peerIP. Start begins the exchanges.
func New(peerIP string) (*Follower, error) {
	ip := net.ParseIP(peerIP)
	if ip == nil {
		return nil, fmt.Errorf("ptp: bad peer address %q", peerIP)
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
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
	return &Follower{
		peer:    ip,
		clockID: randomClockID(),
		event:   event,
		general: general,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}, nil
}

// ClockID is this sender's PTP clock identity, advertised in session SETUP.
func (f *Follower) ClockID() uint64 { return f.clockID }

// Start begins the background exchange loop. Repeated calls, and a call after
// Stop, are ignored.
func (f *Follower) Start() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.started || f.stopped {
		return
	}
	f.started = true
	go f.run()
}

// Activate enables exchanges after SETPEERS. Pre-session traffic is ignored.
func (f *Follower) Activate() {
	f.mu.Lock()
	f.st.active = true
	f.st.nextSub = time.Time{}
	f.st.lastDelay = time.Time{}
	f.mu.Unlock()
}

// Stop ends the loop and closes the sockets. It waits for the loop only when
// Start ran, so a Follower that was never started does not hang.
func (f *Follower) Stop() {
	f.mu.Lock()
	f.stopped = true
	started := f.started
	f.mu.Unlock()
	f.stopOnce.Do(func() {
		close(f.stop)
		_ = f.event.Close()
		_ = f.general.Close()
		if started {
			<-f.done
		}
	})
}

// Ready reports whether the offset is established and fresh.
func (f *Follower) Ready() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st.ready(time.Now())
}

// Timeline maps a local Unix-nanosecond time onto the receiver's PTP clock.
func (f *Follower) Timeline(localUnixNs uint64) (id uint64, networkNs uint64, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	if !f.st.ready(now) {
		return 0, 0, false
	}
	offset, ok := f.st.effectiveOffset(now)
	if !ok {
		return 0, 0, false
	}
	mapped := float64(localUnixNs) + offset
	if mapped < 0 || mapped > math.MaxUint64 {
		return 0, 0, false
	}
	return f.st.clockID, uint64(math.Round(mapped)), true
}

// Stats returns a snapshot for diagnostics.
func (f *Follower) Stats() Stats {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	clock := f.st.clockID
	if clock == 0 {
		clock = f.clockID
	}
	out := Stats{
		Ready:     f.st.ready(now),
		ClockID:   clock,
		SourceID:  f.st.sourceID,
		Exchanges: f.st.exchanges,
		Received:  f.st.received,
		Rejected:  f.st.rejected,
	}
	if off, ok := f.st.effectiveOffset(now); ok {
		out.OffsetNS = off
	}
	if f.st.delayAt.Add(sampleLifetime).After(now) {
		out.HasDelay = f.st.delaySet
	}
	out.RoundTripNS = f.st.roundTrip
	return out
}

func (f *Follower) run() {
	defer close(f.done)
	seq := uint16(time.Now().UnixNano())
	buf := make([]byte, maxPacket)
	next := time.Now()
	for {
		select {
		case <-f.stop:
			return
		default:
		}
		now := time.Now()
		if !now.Before(next) {
			f.sendRound(now, seq)
			seq++
			next = now.Add(syncInterval)
		}
		f.readOne(f.event, buf)
		f.readOne(f.general, buf)
		time.Sleep(2 * time.Millisecond)
	}
}

func (f *Follower) readOne(conn *net.UDPConn, buf []byte) {
	_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))
	n, addr, err := conn.ReadFromUDP(buf)
	if err != nil {
		return
	}
	if !addr.IP.Equal(f.peer) || (addr.Port != eventPort && addr.Port != generalPort) {
		return
	}
	local := uint64(time.Now().UnixNano())
	f.mu.Lock()
	f.handle(buf[:n], local, time.Now(), addr)
	f.mu.Unlock()
}

func (f *Follower) sendRound(now time.Time, seq uint16) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.st.active {
		return
	}
	if f.st.nextSub.IsZero() || !now.Before(f.st.nextSub) {
		remote := uint64(0)
		port := uint16(0)
		if f.st.sourceID != 0 {
			remote, port = f.st.sourceID, f.st.sourcePort
		}
		sentAny := false
		for i, kind := range []uint8{0, 9, 11} {
			if f.st.grantedUntil[i].After(now.Add(5 * time.Second)) {
				continue
			}
			pkt := subscriptionPacket(f.clockID, remote, port, seq, kind)
			if _, err := f.general.WriteToUDP(pkt, &net.UDPAddr{IP: f.peer, Port: generalPort}); err == nil {
				f.st.subs[i] = append(f.st.subs[i], subscription{sequence: seq, interval: int8(pkt[49]), sent: now})
				if len(f.st.subs[i]) > 4 {
					f.st.subs[i] = f.st.subs[i][1:]
				}
				sentAny = true
			}
		}
		if sentAny {
			f.st.nextSub = now.Add(subscriptionRetry)
		}
	}
	if f.st.lastDelay.IsZero() || now.Sub(f.st.lastDelay) >= delayRequestInterval {
		pkt := header(1, 44, f.clockID, seq, 0x0408, 0)
		sent := uint64(now.UnixNano())
		writeTimestamp(pkt[34:44], sent)
		if _, err := f.event.WriteToUDP(pkt, &net.UDPAddr{IP: f.peer, Port: eventPort}); err == nil {
			f.st.delay = &delayRequest{sequence: seq, sent: sent, at: now}
			f.st.lastDelay = now
		}
	}
}

func (f *Follower) handle(data []byte, localNs uint64, now time.Time, from *net.UDPAddr) {
	if rejection := packetRejection(data); rejection != "" {
		f.st.rejected++
		return
	}
	kind := data[0] & 0x0f
	seq := binary.BigEndian.Uint16(data[30:32])
	source := binary.BigEndian.Uint64(data[20:28])
	sourcePort := binary.BigEndian.Uint16(data[28:30])

	if kind == 11 && len(data) >= 64 {
		id := binary.BigEndian.Uint64(data[53:61])
		if f.st.clockID != id || f.st.sourceID != source || f.st.sourcePort != sourcePort {
			f.st = state{
				active:     f.st.active,
				clockID:    id,
				sourceID:   source,
				sourcePort: sourcePort,
				received:   f.st.received,
				rejected:   f.st.rejected,
				subs:       f.st.subs,
			}
		}
	}
	if f.st.sourceID == 0 || f.st.sourceID != source || f.st.sourcePort != sourcePort {
		f.st.rejected++
		return
	}
	f.st.received++
	if f.OnPacket != nil {
		f.OnPacket(kind, seq, append([]byte(nil), data...))
	}

	switch kind {
	case 0, 8:
		if len(data) >= 44 {
			f.rememberSync(data, localNs, now)
		}
	case 9:
		if len(data) >= 54 {
			f.handleDelayResponse(data, seq, now)
		}
	case 11:
		// Announce only identifies the source above.
	case 12:
		f.handleGrants(data, now)
	case 2:
		f.respondPdelay(data, localNs, now, from)
	}
}

func (f *Follower) rememberSync(data []byte, localNs uint64, now time.Time) {
	seq := binary.BigEndian.Uint16(data[30:32])
	kind := data[0] & 0x0f
	var origin *uint64
	if kind == 8 || data[6]&2 == 0 {
		if ts, ok := readTimestamp(data[34:44]); ok {
			origin = &ts
		} else {
			f.st.rejected++
			return
		}
	}
	f.st.pending = retainFresh(f.st.pending, now)
	idx := -1
	for i := range f.st.pending {
		if f.st.pending[i].sequence == seq {
			idx = i
			break
		}
	}
	if idx < 0 {
		if len(f.st.pending) == 32 {
			f.st.pending = f.st.pending[1:]
		}
		f.st.pending = append(f.st.pending, pendingSync{sequence: seq, at: now})
		idx = len(f.st.pending) - 1
	}
	entry := &f.st.pending[idx]
	if entry.complete {
		return
	}
	if kind == 0 && entry.sync == nil {
		entry.sync = &sampleAt{value: localNs, correction: correction(data)}
		if origin != nil {
			entry.follow = &sampleAt{value: *origin}
		}
	} else if kind == 8 && entry.follow == nil {
		if origin == nil {
			return
		}
		entry.follow = &sampleAt{value: *origin, correction: correction(data)}
	}
	if entry.sync == nil || entry.follow == nil {
		return
	}
	entry.complete = true
	sample := syncSample{
		sequence:   seq,
		origin:     entry.follow.value,
		received:   entry.sync.value,
		correction: entry.sync.correction + entry.follow.correction,
		at:         entry.at,
	}
	if f.st.syncPair != nil {
		prev := *f.st.syncPair
		advance := seq - prev.sequence
		newer := advance > 0 && advance < 0x8000
		if sample.received <= prev.received || sample.at.Before(prev.at) ||
			(now.Sub(prev.at) < exchangeTimeout && !newer) {
			return
		}
	}
	f.st.syncPair = &sample
	offset := float64(int64(sample.origin)-int64(sample.received)) + sample.correction
	if f.st.syncOffsetSet {
		f.st.syncOffset = f.st.syncOffset*0.875 + offset*0.125
	} else {
		f.st.syncOffset = offset
		f.st.syncOffsetSet = true
	}
	f.st.exchange(now)
}

func (f *Follower) handleDelayResponse(data []byte, seq uint16, now time.Time) {
	if f.st.delay == nil || f.st.delay.sequence != seq {
		return
	}
	if binary.BigEndian.Uint64(data[44:52]) != f.clockID ||
		binary.BigEndian.Uint16(data[52:54]) != 0x8005 {
		return
	}
	if f.st.syncPair == nil || now.Sub(f.st.syncPair.at) >= sampleLifetime ||
		now.Sub(f.st.delay.at) >= sampleLifetime {
		return
	}
	remoteReceived, ok := readTimestamp(data[34:44])
	if !ok {
		return
	}
	sample := *f.st.syncPair
	outward := float64(int64(sample.origin)-int64(sample.received)) + sample.correction
	inward := float64(int64(remoteReceived)-int64(f.st.delay.sent)) - correction(data)
	rtt := inward - outward
	if rtt < 0 || rtt > 250_000_000 {
		f.st.rejected++
		f.st.delay = nil
		return
	}
	offset := (outward + inward) / 2
	if f.st.delaySet && math.Abs(f.st.delayOffset-offset) > 5_000_000 {
		// A jump this large means a new clock or a broken estimate; restart
		// the readiness streak instead of following it.
		f.st.exchanges = 0
		f.st.first = time.Time{}
		f.st.last = time.Time{}
		f.st.delaySet = false
	}
	if f.st.delaySet {
		f.st.delayOffset = f.st.delayOffset*0.875 + offset*0.125
	} else {
		f.st.delayOffset = offset
		f.st.delaySet = true
	}
	f.st.delayAt = now
	f.st.roundTrip = rtt
	f.st.delay = nil
	f.st.exchange(now)
}

func (f *Follower) handleGrants(data []byte, now time.Time) {
	if len(data) < 44 || binary.BigEndian.Uint64(data[34:42]) != f.clockID ||
		binary.BigEndian.Uint16(data[42:44]) != 0x8005 {
		return
	}
	seq := binary.BigEndian.Uint16(data[30:32])
	for at := 44; at+4 <= len(data); {
		kind := binary.BigEndian.Uint16(data[at : at+2])
		length := int(binary.BigEndian.Uint16(data[at+2 : at+4]))
		at += 4
		if length < 0 || at+length > len(data) {
			return
		}
		if kind == 5 && length == 8 {
			body := data[at : at+8]
			msg := body[0] >> 4
			seconds := binary.BigEndian.Uint32(body[2:6])
			for i, want := range []uint8{0, 9, 11} {
				if msg != want || seconds == 0 || seconds > 30 {
					continue
				}
				for _, pending := range f.st.subs[i] {
					if pending.sequence == seq {
						f.st.grantedUntil[i] = now.Add(time.Duration(seconds) * time.Second)
						f.st.subs[i] = nil
						break
					}
				}
			}
		}
		at += length
	}
}

func (f *Follower) respondPdelay(data []byte, localNs uint64, now time.Time, from *net.UDPAddr) {
	if len(data) < 54 {
		return
	}
	seq := binary.BigEndian.Uint16(data[30:32])
	resp := header(3, 54, f.clockID, seq, 0x0608, -3)
	writeTimestamp(resp[34:44], localNs)
	copy(resp[44:54], data[20:30])
	_, _ = f.event.WriteToUDP(resp, &net.UDPAddr{IP: from.IP, Port: eventPort})

	follow := header(10, 54, f.clockID, seq, 0x0408, -3)
	writeTimestamp(follow[34:44], uint64(now.UnixNano()))
	copy(follow[44:54], data[20:30])
	_, _ = f.general.WriteToUDP(follow, &net.UDPAddr{IP: from.IP, Port: generalPort})
}

func (st *state) exchange(now time.Time) {
	if !st.last.IsZero() && now.Sub(st.last) >= exchangeTimeout {
		st.exchanges = 0
		st.first = time.Time{}
	}
	if st.first.IsZero() {
		st.first = now
	}
	st.last = now
	st.exchanges++
}

func (st *state) delayOffsetFresh(now time.Time) (float64, bool) {
	if !st.delaySet || now.Sub(st.delayAt) >= sampleLifetime {
		return 0, false
	}
	return st.delayOffset, true
}

func (st *state) effectiveOffset(now time.Time) (float64, bool) {
	if off, ok := st.delayOffsetFresh(now); ok {
		return off, true
	}
	if st.syncOffsetSet {
		return st.syncOffset, true
	}
	return 0, false
}

func (st *state) ready(now time.Time) bool {
	if !st.active || st.exchanges < 3 || st.first.IsZero() {
		return false
	}
	if now.Sub(st.first) < startupDwell {
		return false
	}
	if st.last.IsZero() || now.Sub(st.last) >= exchangeTimeout {
		return false
	}
	_, ok := st.effectiveOffset(now)
	return ok
}

func retainFresh(pending []pendingSync, now time.Time) []pendingSync {
	out := pending[:0]
	for _, p := range pending {
		if now.Sub(p.at) < sampleLifetime {
			out = append(out, p)
		}
	}
	return out
}

func randomClockID() uint64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uint64(time.Now().UnixNano()) & 0x7fff_ffff_ffff_ffff
	}
	return binary.BigEndian.Uint64(b[:]) & 0x7fff_ffff_ffff_ffff
}

func header(kind uint8, length int, clock uint64, seq uint16, flags uint16, interval int8) []byte {
	p := make([]byte, length)
	p[0] = 0x10 | kind
	p[1] = 2
	binary.BigEndian.PutUint16(p[2:4], uint16(length))
	binary.BigEndian.PutUint16(p[6:8], flags)
	binary.BigEndian.PutUint64(p[20:28], clock)
	binary.BigEndian.PutUint16(p[28:30], 0x8005)
	binary.BigEndian.PutUint16(p[30:32], seq)
	p[33] = byte(interval)
	return p
}

func subscriptionPacket(clock, remote uint64, remotePort uint16, seq uint16, kind uint8) []byte {
	p := header(12, 54, clock, seq, 0x0408, 0)
	if remote == 0 {
		for i := 34; i < 44; i++ {
			p[i] = 0xff
		}
	} else {
		binary.BigEndian.PutUint64(p[34:42], remote)
		binary.BigEndian.PutUint16(p[42:44], remotePort)
	}
	copy(p[44:48], []byte{0, 4, 0, 6})
	p[48] = kind << 4
	if kind == 0 {
		p[49] = 0xFD // interval -3, 8 messages per second
	}
	binary.BigEndian.PutUint32(p[50:54], 30)
	return p
}

// writeTimestamp writes the 10-byte PTP timestamp: 48-bit seconds followed by
// a 32-bit nanosecond field.
func writeTimestamp(dst []byte, ns uint64) {
	secs := ns / 1_000_000_000
	for i := 0; i < 6; i++ {
		dst[i] = byte(secs >> (8 * (5 - i)))
	}
	binary.BigEndian.PutUint32(dst[6:10], uint32(ns%1_000_000_000))
}

func readTimestamp(data []byte) (uint64, bool) {
	if len(data) < 10 {
		return 0, false
	}
	var secs [8]byte
	copy(secs[2:], data[:6])
	ns := binary.BigEndian.Uint32(data[6:10])
	if ns >= 1_000_000_000 {
		return 0, false
	}
	return binary.BigEndian.Uint64(secs[:])*1_000_000_000 + uint64(ns), true
}

func correction(data []byte) float64 {
	if len(data) < 16 {
		return 0
	}
	return float64(int64(binary.BigEndian.Uint64(data[8:16]))) / 65536
}

func packetRejection(data []byte) string {
	if len(data) < 34 {
		return "short_header"
	}
	if data[0]>>4 != 1 || data[1]&0x0f != 2 {
		return "profile_or_version"
	}
	if data[4] != 0 {
		return "domain"
	}
	minimum := 34
	switch data[0] & 0x0f {
	case 0, 1, 8, 12:
		minimum = 44
	case 2, 3, 9, 10:
		minimum = 54
	case 11:
		minimum = 64
	}
	if len(data) < minimum || int(binary.BigEndian.Uint16(data[2:4])) != len(data) {
		return "length"
	}
	return ""
}
