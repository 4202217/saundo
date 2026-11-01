package ptp

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestMasterSyncPairWireShape(t *testing.T) {
	const clock = 0x1234567812345678
	syncPacket, followUp := syncPair(clock, 65535, 1_234_567_890)

	if len(syncPacket) != 44 || len(followUp) != 96 {
		t.Fatalf("lengths sync=%d follow=%d", len(syncPacket), len(followUp))
	}
	if syncPacket[0] != 0x10 || followUp[0] != 0x18 {
		t.Fatalf("message types %#x %#x", syncPacket[0], followUp[0])
	}
	if binary.BigEndian.Uint16(syncPacket[6:8]) != 0x0608 || binary.BigEndian.Uint16(followUp[6:8]) != 0x0408 {
		t.Fatalf("flags sync=%#x follow=%#x",
			binary.BigEndian.Uint16(syncPacket[6:8]), binary.BigEndian.Uint16(followUp[6:8]))
	}
	if int8(syncPacket[33]) != -3 || int8(followUp[33]) != -3 {
		t.Fatalf("intervals sync=%d follow=%d", int8(syncPacket[33]), int8(followUp[33]))
	}
	if binary.BigEndian.Uint64(syncPacket[20:28]) != clock || binary.BigEndian.Uint64(followUp[20:28]) != clock {
		t.Fatalf("clock identity sync=%#x follow=%#x",
			binary.BigEndian.Uint64(syncPacket[20:28]), binary.BigEndian.Uint64(followUp[20:28]))
	}
	if binary.BigEndian.Uint16(syncPacket[30:32]) != 65535 || binary.BigEndian.Uint16(followUp[30:32]) != 65535 {
		t.Fatalf("sequence sync=%d follow=%d",
			binary.BigEndian.Uint16(syncPacket[30:32]), binary.BigEndian.Uint16(followUp[30:32]))
	}
	// 1.234567890 s: 48-bit seconds then a 32-bit nanosecond field.
	wantTime := []byte{0, 0, 0, 0, 0, 1, 13, 251, 56, 210}
	if !bytes.Equal(syncPacket[34:44], wantTime) || !bytes.Equal(followUp[34:44], wantTime) {
		t.Fatalf("timestamps sync=% x follow=% x", syncPacket[34:44], followUp[34:44])
	}
	if !bytes.Equal(followUp[44:48], []byte{0, 3, 0, 28}) {
		t.Fatalf("rate TLV header % x", followUp[44:48])
	}
	if !bytes.Equal(followUp[48:54], []byte{0, 0x80, 0xc2, 0, 0, 1}) {
		t.Fatalf("rate TLV body % x", followUp[48:54])
	}
	if !bytes.Equal(followUp[54:76], make([]byte, 22)) {
		t.Fatalf("rate TLV tail % x", followUp[54:76])
	}
	if !bytes.Equal(followUp[76:80], []byte{0, 3, 0, 16}) {
		t.Fatalf("clock identity TLV header % x", followUp[76:80])
	}
	if !bytes.Equal(followUp[80:86], []byte{0, 0x0d, 0x93, 0, 0, 4}) {
		t.Fatalf("clock identity TLV body % x", followUp[80:86])
	}
	if binary.BigEndian.Uint64(followUp[86:94]) != clock {
		t.Fatalf("TLV clock identity %#x", binary.BigEndian.Uint64(followUp[86:94]))
	}
	if !bytes.Equal(followUp[94:96], []byte{0, 0}) {
		t.Fatalf("clock identity TLV tail % x", followUp[94:96])
	}
}

func TestMasterAnnounceWireShape(t *testing.T) {
	const clock = 0x0badc0ffee0ddf00
	packet := masterAnnounce(clock, 9, 2_000_000_001)

	if len(packet) != 76 {
		t.Fatalf("length %d", len(packet))
	}
	if packet[0] != 0x1b || binary.BigEndian.Uint16(packet[2:4]) != 76 {
		t.Fatalf("header % x", packet[:4])
	}
	if binary.BigEndian.Uint16(packet[6:8]) != 0x0408 {
		t.Fatalf("flags %#x", binary.BigEndian.Uint16(packet[6:8]))
	}
	if binary.BigEndian.Uint64(packet[20:28]) != clock || binary.BigEndian.Uint16(packet[30:32]) != 9 {
		t.Fatalf("identity %#x sequence %d",
			binary.BigEndian.Uint64(packet[20:28]), binary.BigEndian.Uint16(packet[30:32]))
	}
	if !bytes.Equal(packet[34:44], []byte{0, 0, 0, 0, 0, 2, 0, 0, 0, 1}) {
		t.Fatalf("timestamp % x", packet[34:44])
	}
	if packet[47] != 96 || packet[48] != 248 || packet[49] != 0xfe || packet[52] != 128 || packet[63] != 0xa0 {
		t.Fatalf("quality fields % x", packet[47:64])
	}
	if binary.BigEndian.Uint16(packet[50:52]) != 0xffff {
		t.Fatalf("variance %#x", binary.BigEndian.Uint16(packet[50:52]))
	}
	if binary.BigEndian.Uint64(packet[53:61]) != clock {
		t.Fatalf("grandmaster identity %#x", binary.BigEndian.Uint64(packet[53:61]))
	}
	if !bytes.Equal(packet[64:68], []byte{0, 8, 0, 8}) {
		t.Fatalf("time source TLV % x", packet[64:68])
	}
	if binary.BigEndian.Uint64(packet[68:76]) != clock {
		t.Fatalf("repeated identity %#x", binary.BigEndian.Uint64(packet[68:76]))
	}
}

func TestUnicastGrantsValidateRequest(t *testing.T) {
	const master, requester = uint64(0x0123456789abcdef), uint64(0x1111222233334444)
	newRequest := func(seq uint16, kind uint8) []byte {
		return subscriptionPacket(requester, master, 0x8005, seq, kind)
	}

	for _, kind := range []uint8{0, 9, 11} {
		request := newRequest(3, kind)
		grants := unicastGrants(request, master, 3)
		if len(grants) != 1 {
			t.Fatalf("kind %d produced %d grants", kind, len(grants))
		}
		grant := grants[0]
		if len(grant) != 56 || grant[0] != 0x1c || binary.BigEndian.Uint16(grant[2:4]) != 56 {
			t.Fatalf("kind %d grant header % x", kind, grant[:4])
		}
		if binary.BigEndian.Uint16(grant[6:8]) != 0x0408 {
			t.Fatalf("kind %d flags %#x", kind, binary.BigEndian.Uint16(grant[6:8]))
		}
		if binary.BigEndian.Uint64(grant[20:28]) != master ||
			binary.BigEndian.Uint16(grant[28:30]) != 0x8005 ||
			binary.BigEndian.Uint16(grant[30:32]) != 3 {
			t.Fatalf("kind %d grant identity % x", kind, grant[20:32])
		}
		if !bytes.Equal(grant[34:42], request[20:28]) || !bytes.Equal(grant[42:44], request[28:30]) {
			t.Fatalf("kind %d request identity not copied: % x", kind, grant[34:44])
		}
		if !bytes.Equal(grant[44:48], []byte{0, 5, 0, 8}) {
			t.Fatalf("kind %d grant TLV % x", kind, grant[44:48])
		}
		if grant[48] != kind<<4 {
			t.Fatalf("kind %d message type %#x", kind, grant[48])
		}
		if binary.BigEndian.Uint32(grant[50:54]) != 30 {
			t.Fatalf("kind %d duration %d", kind, binary.BigEndian.Uint32(grant[50:54]))
		}
	}

	allFF := newRequest(4, 0)
	for i := 34; i < 44; i++ {
		allFF[i] = 0xff
	}
	if grants := unicastGrants(allFF, master, 4); len(grants) != 1 {
		t.Fatalf("all-ff target produced %d grants", len(grants))
	}
	if grants := unicastGrants(newRequest(5, 0), master+1, 5); len(grants) != 0 {
		t.Fatalf("wrong target produced %d grants", len(grants))
	}

	unsupported := newRequest(6, 0)
	unsupported[48] = 7 << 4
	if grants := unicastGrants(unsupported, master, 6); len(grants) != 0 {
		t.Fatalf("unsupported message type produced %d grants", len(grants))
	}

	zero := newRequest(7, 0)
	binary.BigEndian.PutUint32(zero[50:54], 0)
	if grants := unicastGrants(zero, master, 7); len(grants) != 0 {
		t.Fatalf("zero duration produced %d grants", len(grants))
	}

	long := newRequest(8, 0)
	binary.BigEndian.PutUint32(long[50:54], 60)
	grants := unicastGrants(long, master, 8)
	if len(grants) != 1 || binary.BigEndian.Uint32(grants[0][50:54]) != 30 {
		t.Fatalf("duration clamp produced % x", grants)
	}

	if grants := unicastGrants(newRequest(9, 0)[:50], master, 9); len(grants) != 0 {
		t.Fatalf("truncated TLV produced %d grants", len(grants))
	}
	trailing := append(newRequest(10, 0), 0, 4, 0, 6)
	if grants := unicastGrants(trailing, master, 10); len(grants) != 1 {
		t.Fatalf("grant before a truncated TLV was dropped: %d grants", len(grants))
	}
	if grants := unicastGrants(newRequest(11, 0)[:44], master, 11); len(grants) != 0 {
		t.Fatalf("header-only request produced %d grants", len(grants))
	}
}

func TestMasterReadinessPerPeer(t *testing.T) {
	first := net.ParseIP("192.168.1.10").To4()
	second := net.ParseIP("192.168.1.11").To4()
	m := &Master{clockID: 0x77, peers: map[string]*masterPeer{
		first.String():  {ip: first},
		second.String(): {ip: second},
	}}
	peer := m.peers[first.String()]

	if m.Ready(first.String()) {
		t.Fatal("ready with no exchanges")
	}
	now := time.Now()
	peer.exchanged(now.Add(-time.Second))
	peer.exchanged(now.Add(-875 * time.Millisecond))
	if m.Ready(first.String()) {
		t.Fatal("ready after two exchanges")
	}
	peer.exchanged(now.Add(-750 * time.Millisecond))
	if !m.Ready(first.String()) {
		t.Fatal("not ready after three exchanges spanning 250ms")
	}
	if m.Ready(second.String()) {
		t.Fatal("one member's exchanges made its pair ready")
	}
	if m.Ready("192.168.1.12") || m.Ready("not an ip") {
		t.Fatal("unknown peer reported ready")
	}

	if peer.ready(now.Add(3 * time.Second)) {
		t.Fatal("ready after the exchange timeout")
	}
	// An exchange after the timeout restarts the streak, as AirFlash does.
	peer.exchanged(now.Add(3 * time.Second))
	peer.exchanged(now.Add(3*time.Second + 125*time.Millisecond))
	if peer.ready(now.Add(3*time.Second + 125*time.Millisecond)) {
		t.Fatal("stale exchanges counted toward the new streak")
	}
	peer.exchanged(now.Add(3*time.Second + 250*time.Millisecond))
	if !peer.ready(now.Add(3*time.Second + 250*time.Millisecond)) {
		t.Fatal("not ready after three fresh exchanges")
	}
}

func TestPeerClockTimeline(t *testing.T) {
	const clock = uint64(0xabc)
	first := net.ParseIP("192.168.1.10").To4()
	m := &Master{clockID: clock, peers: map[string]*masterPeer{
		first.String(): {ip: first},
	}}
	view := m.ClockFor(first.String())
	if view == nil {
		t.Fatal("no clock view for a configured peer")
	}
	if view.ClockID() != clock {
		t.Fatalf("clock id %#x", view.ClockID())
	}
	id, network, ok := view.Timeline(987_654_321)
	if id != clock || network != 987_654_321 || !ok {
		t.Fatalf("timeline id=%#x network=%d ok=%v", id, network, ok)
	}
	if view.Ready() {
		t.Fatal("ready before any exchange")
	}
	view.Activate()
	if view.Ready() {
		t.Fatal("Activate changed readiness in master mode")
	}
	if m.ClockFor("192.168.1.11") != nil || m.ClockFor("not an ip") != nil {
		t.Fatal("clock view returned for an unknown peer")
	}
}

func TestNewMasterRejectsBadPeers(t *testing.T) {
	cases := [][]string{
		nil,
		{},
		{"192.168.1.10", "192.168.1.11", "192.168.1.12"},
		{"192.168.1.10", "192.168.1.10"},
		{"192.168.1.10", "::ffff:192.168.1.10"},
		{""},
		{"not-an-ip"},
		{"2001:db8::1"},
	}
	for _, peers := range cases {
		m, err := NewMaster(peers)
		if err == nil {
			m.Stop()
			t.Errorf("NewMaster(%q) accepted invalid peers", peers)
		}
	}
}

func TestNewMasterSecondBindFails(t *testing.T) {
	first, err := NewMaster([]string{"127.0.0.1"})
	if err != nil {
		t.Skipf("PTP ports 319/320 already in use: %v", err)
	}
	defer first.Stop()
	second, err := NewMaster([]string{"127.0.0.1"})
	if err == nil {
		second.Stop()
		t.Fatal("second master bound ports 319/320")
	}
}

func TestMasterStartStopAreIdempotent(t *testing.T) {
	event, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	general, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	m := &Master{
		clockID: randomClockID(),
		peers:   map[string]*masterPeer{},
		event:   event,
		general: general,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if m.ClockID()>>63 != 0 {
		t.Fatalf("clock id %#x is not 63-bit", m.ClockID())
	}
	m.Start()
	m.Start()
	m.Stop()
	m.Stop()
}

func TestMasterAnswersDelayRequest(t *testing.T) {
	capture, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: generalPort})
	if err != nil {
		t.Skipf("general port %d unavailable: %v", generalPort, err)
	}
	defer capture.Close()
	event, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer event.Close()
	general, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer general.Close()

	peerIP := net.ParseIP("127.0.0.1").To4()
	peer := &masterPeer{ip: peerIP}
	const clock = uint64(0x0123456789abcdef)
	m := &Master{clockID: clock, event: event, general: general,
		peers: map[string]*masterPeer{peerIP.String(): peer}}

	request := header(1, 44, 0x9999, 7, 0x0408, -3)
	writeTimestamp(request[34:44], 42)
	const received = uint64(1_234_567_890)
	m.mu.Lock()
	m.respondToDelay(peer, 1, 7, request, received, time.Now())
	m.mu.Unlock()

	buf := make([]byte, 128)
	_ = capture.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := capture.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("delay response not received: %v", err)
	}
	response := buf[:n]
	if len(response) != 54 || response[0] != 0x19 || binary.BigEndian.Uint16(response[2:4]) != 54 {
		t.Fatalf("response header % x", response[:4])
	}
	if binary.BigEndian.Uint16(response[6:8]) != 0x0408 || int8(response[33]) != -3 {
		t.Fatalf("response flags %#x interval %d",
			binary.BigEndian.Uint16(response[6:8]), int8(response[33]))
	}
	if binary.BigEndian.Uint64(response[20:28]) != clock ||
		binary.BigEndian.Uint16(response[28:30]) != 0x8005 ||
		binary.BigEndian.Uint16(response[30:32]) != 7 {
		t.Fatalf("response identity % x", response[20:32])
	}
	if !bytes.Equal(response[34:44], []byte{0, 0, 0, 0, 0, 1, 13, 251, 56, 210}) {
		t.Fatalf("receive timestamp % x", response[34:44])
	}
	if !bytes.Equal(response[44:54], request[20:30]) {
		t.Fatalf("request identity % x not copied into % x", response[44:54], request[20:30])
	}

	m.mu.Lock()
	m.respondToDelay(peer, 1, 7, request, received, time.Now())
	m.mu.Unlock()
	_ = capture.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err := capture.ReadFromUDP(buf); err == nil {
		t.Fatal("master answered a duplicate delay request")
	}
}

func TestMasterAnswersPdelayRequest(t *testing.T) {
	events, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: eventPort})
	if err != nil {
		t.Skipf("event port %d unavailable: %v", eventPort, err)
	}
	defer events.Close()
	generalCapture, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: generalPort})
	if err != nil {
		t.Skipf("general port %d unavailable: %v", generalPort, err)
	}
	defer generalCapture.Close()
	event, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer event.Close()
	general, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer general.Close()

	peerIP := net.ParseIP("127.0.0.1").To4()
	peer := &masterPeer{ip: peerIP}
	const clock = uint64(0x0123456789abcdef)
	m := &Master{clockID: clock, event: event, general: general,
		peers: map[string]*masterPeer{peerIP.String(): peer}}

	request := header(2, 54, 0x9999, 11, 0x0608, -3)
	writeTimestamp(request[34:44], 42)
	const received = uint64(1_234_567_890)
	before := uint64(time.Now().UnixNano())
	m.mu.Lock()
	m.respondToDelay(peer, 2, 11, request, received, time.Now())
	m.mu.Unlock()
	after := uint64(time.Now().UnixNano())

	buf := make([]byte, 128)
	_ = events.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := events.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("pdelay response not received: %v", err)
	}
	response := buf[:n]
	if len(response) != 54 || response[0] != 0x13 || binary.BigEndian.Uint16(response[6:8]) != 0x0608 {
		t.Fatalf("pdelay response % x", response[:8])
	}
	if binary.BigEndian.Uint64(response[20:28]) != clock || binary.BigEndian.Uint16(response[30:32]) != 11 {
		t.Fatalf("pdelay response identity % x", response[20:32])
	}
	if !bytes.Equal(response[34:44], []byte{0, 0, 0, 0, 0, 1, 13, 251, 56, 210}) {
		t.Fatalf("pdelay receive timestamp % x", response[34:44])
	}
	if !bytes.Equal(response[44:54], request[20:30]) {
		t.Fatalf("pdelay request identity % x not copied", response[44:54])
	}

	_ = generalCapture.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err = generalCapture.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("pdelay follow-up not received: %v", err)
	}
	followUp := buf[:n]
	if len(followUp) != 54 || followUp[0] != 0x1a || binary.BigEndian.Uint16(followUp[6:8]) != 0x0408 {
		t.Fatalf("pdelay follow-up % x", followUp[:8])
	}
	if !bytes.Equal(followUp[44:54], request[20:30]) {
		t.Fatalf("pdelay follow-up identity % x", followUp[44:54])
	}
	followTimestamp, ok := readTimestamp(followUp[34:44])
	if !ok || followTimestamp < before || followTimestamp > after {
		t.Fatalf("pdelay follow-up timestamp %d outside [%d, %d]", followTimestamp, before, after)
	}
	if peer.exchanges != 1 {
		t.Fatalf("pdelay exchange count %d", peer.exchanges)
	}
}
