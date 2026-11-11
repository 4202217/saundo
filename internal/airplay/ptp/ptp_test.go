package ptp

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// newTestFollower avoids the fixed PTP ports, which the running app holds.
func newTestFollower(t *testing.T) *Follower {
	t.Helper()
	event, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	general, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		_ = event.Close()
		t.Fatal(err)
	}
	return &Follower{
		event: event, general: general,
		stop: make(chan struct{}), done: make(chan struct{}),
	}
}

// TestFollowerStopWithoutStart pins the lifecycle contract: Stop on a follower
// that never started must return instead of waiting for a loop that will never
// close done.
func TestFollowerStopWithoutStart(t *testing.T) {
	f := newTestFollower(t)
	stopped := make(chan struct{})
	go func() {
		f.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Follower.Stop blocked after New-without-Start")
	}
}

// TestFollowerSecondStartIsIgnored pins the other half: a repeated Start must
// not launch two loops that both close done.
func TestFollowerSecondStartIsIgnored(t *testing.T) {
	f := newTestFollower(t)
	f.Start()
	f.Start()
	stopped := make(chan struct{})
	go func() {
		f.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Follower.Stop blocked after a repeated Start")
	}
	// Let a second run's deferred close surface as a panic if one was spawned.
	time.Sleep(100 * time.Millisecond)
}

func TestTimestampRoundTrip(t *testing.T) {
	buf := make([]byte, 10)
	for _, ns := range []uint64{0, 1, 999_999_999, 1_573_985_916_250, 1_793_383_335_000_000_000} {
		writeTimestamp(buf, ns)
		got, ok := readTimestamp(buf)
		if !ok || got != ns {
			t.Fatalf("round trip %d -> %d (ok=%v)", ns, got, ok)
		}
	}
	// The wire layout is 48-bit seconds then 32-bit nanoseconds.
	writeTimestamp(buf, 1_573_985_916_250)
	if buf[0] != 0 || buf[1] != 0 || buf[2] != 0 || buf[3] != 0 || buf[4] != 0x06 || buf[5] != 0x25 {
		t.Fatalf("seconds bytes = %x", buf[:6])
	}
	if binary.BigEndian.Uint32(buf[6:10]) != 985_916_250 {
		t.Fatalf("nanoseconds = %d", binary.BigEndian.Uint32(buf[6:10]))
	}
}

func TestSubscriptionLayout(t *testing.T) {
	p := subscriptionPacket(0x1122334455667788, 0x99aabbccddeeff00, 0x8005, 7, 0)
	if len(p) != 54 || p[0] != 0x1c || p[1] != 2 {
		t.Fatalf("header %x", p[:2])
	}
	if binary.BigEndian.Uint16(p[2:4]) != 54 || binary.BigEndian.Uint64(p[20:28]) != 0x1122334455667788 {
		t.Fatalf("header fields %x", p[:34])
	}
	if binary.BigEndian.Uint64(p[34:42]) != 0x99aabbccddeeff00 || binary.BigEndian.Uint16(p[42:44]) != 0x8005 {
		t.Fatalf("target %x", p[34:44])
	}
	if p[48] != 0 || p[49] != 0xfd || binary.BigEndian.Uint32(p[50:54]) != 30 {
		t.Fatalf("subscription body %x", p[44:])
	}
}

func TestPacketRejection(t *testing.T) {
	good := header(0, 44, 1, 0, 0x0608, -3)
	if r := packetRejection(good); r != "" {
		t.Fatalf("valid packet rejected: %s", r)
	}
	short := good[:33]
	if r := packetRejection(short); r != "short_header" {
		t.Fatalf("short = %q", r)
	}
	badLen := append([]byte(nil), good...)
	badLen[3] = 99
	if r := packetRejection(badLen); r != "length" {
		t.Fatalf("length = %q", r)
	}
}

func TestFollowerEstimatesOffsetAndTimeline(t *testing.T) {
	peer := net.ParseIP("192.168.1.50").To4()
	f := &Follower{peer: peer, clockID: 0x1111}
	f.st.active = true
	addr := &net.UDPAddr{IP: peer, Port: generalPort}

	// The source identity in the header is 0x3333; the grandmaster clock the
	// timeline uses is 0x2222.
	announce := header(11, 76, 0x3333, 1, 0x0408, 0)
	binary.BigEndian.PutUint64(announce[53:61], 0x2222)
	// Keep the synthetic exchanges in the recent past so freshness checks
	// against the real clock behave as they would in a live session.
	now := time.Now().Add(-2 * time.Second)
	f.handle(announce, 0, now, addr)

	const offset = uint64(1_000_000_000) // receiver is one second ahead
	base := uint64(100_000_000_000)
	for i := 0; i < 3; i++ {
		seq := uint16(i + 1)
		local := base + uint64(i)*100_000_000
		sync := header(0, 44, 0x3333, seq, 0x0608, -3)
		follow := header(8, 96, 0x3333, seq, 0x0408, -3)
		writeTimestamp(follow[34:44], local+offset)
		now = now.Add(200 * time.Millisecond)
		f.handle(sync, local, now, addr)
		f.handle(follow, local, now, addr)
	}
	if !f.st.syncOffsetSet || f.st.syncOffset < float64(offset)-1000 || f.st.syncOffset > float64(offset)+1000 {
		t.Fatalf("sync offset = %f (set=%v)", f.st.syncOffset, f.st.syncOffsetSet)
	}
	if !f.Ready() {
		t.Fatalf("not ready: %+v", f.Stats())
	}

	// A delay response with no path delay confirms the same offset.
	local := base + 300_000_000
	f.st.delay = &delayRequest{sequence: 42, sent: local, at: now}
	delayResp := header(9, 54, 0x3333, 42, 0x0608, 0)
	writeTimestamp(delayResp[34:44], local+offset)
	binary.BigEndian.PutUint64(delayResp[44:52], f.clockID)
	binary.BigEndian.PutUint16(delayResp[52:54], 0x8005)
	f.handle(delayResp, local+offset, now, addr)
	if !f.st.delaySet {
		t.Fatalf("delay offset not set: %+v", f.Stats())
	}

	id, network, ok := f.Timeline(local + 5_000_000_000)
	if !ok || id != 0x2222 {
		t.Fatalf("timeline id=%x ok=%v", id, ok)
	}
	if network < local+offset+5_000_000_000-1000 || network > local+offset+5_000_000_000+1000 {
		t.Fatalf("timeline network = %d", network)
	}
}
