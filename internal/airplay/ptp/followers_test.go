package ptp

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// newTestFollowers binds ephemeral ports; a running app holds 319/320.
func newTestFollowers(t *testing.T, hosts ...string) *Followers {
	t.Helper()
	g, err := newFollowers(hosts, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Stop)
	return g
}

func announceFrom(source, clock uint64) []byte {
	packet := header(11, 76, source, 1, 0x0408, 0)
	binary.BigEndian.PutUint64(packet[53:61], clock)
	return packet
}

func syncFrom(source uint64, seq uint16, local int64, offset int64) (syncPacket, followUp []byte) {
	syncPacket = header(0, 44, source, seq, 0x0608, -3)
	followUp = header(8, 96, source, seq, 0x0408, -3)
	writeTimestamp(followUp[34:44], uint64(local+offset))
	return syncPacket, followUp
}

func dispatchFrom(g *Followers, host string, port int, data []byte, local uint64) {
	g.dispatch(&net.UDPAddr{IP: net.ParseIP(host).To4(), Port: port}, data, local)
}

// exchangePeer drives one Sync/Follow_Up pair through the hub, as the
// receiver's event and general traffic would arrive.
func exchangePeer(g *Followers, host string, source uint64, seq uint16, local, offset int64) {
	syncPacket, followUp := syncFrom(source, seq, local, offset)
	dispatchFrom(g, host, eventPort, syncPacket, uint64(local))
	dispatchFrom(g, host, generalPort, followUp, uint64(local))
}

// settleStartup pretends the readiness dwell already elapsed. The peer still
// needs its three exchanges; this only removes a 250 ms sleep from the test.
func settleStartup(p *Follower) {
	p.mu.Lock()
	p.st.first = time.Now().Add(-time.Second)
	p.mu.Unlock()
}

// TestFollowersDemuxBySourceIP pins the hub's whole contract: every receiver
// keeps its own grandmaster, offset, and readiness; ClockFor returns the right
// peer; the hub is ready only when every peer is; Stats sums the counters and
// reports the largest absolute offset; an unknown source is ignored.
func TestFollowersDemuxBySourceIP(t *testing.T) {
	const (
		peerA   = "192.0.2.50"
		peerB   = "192.0.2.51"
		sourceA = 0x3333
		sourceB = 0x4444
		clockA  = 0x2222
		clockB  = 0x5555
	)
	g := newTestFollowers(t, peerA, peerB)
	a, b := g.ClockFor(peerA), g.ClockFor(peerB)
	if a == nil || b == nil {
		t.Fatalf("ClockFor missed a peer: %v %v", a, b)
	}
	if g.ClockFor("192.0.2.99") != nil {
		t.Fatal("ClockFor returned a peer outside the set")
	}
	if g.ClockFor("2001:db8::1") != nil {
		t.Fatal("ClockFor returned a peer for an IPv6 host")
	}
	if a.ClockID() != g.ClockID() || b.ClockID() != g.ClockID() {
		t.Fatalf("peers do not share the sender clock: %x %x %x", a.ClockID(), b.ClockID(), g.ClockID())
	}

	a.Activate()
	b.Activate()

	base := int64(100_000_000_000)
	dispatchFrom(g, peerA, generalPort, announceFrom(sourceA, clockA), uint64(base))
	dispatchFrom(g, peerB, generalPort, announceFrom(sourceB, clockB), uint64(base))
	// A new grandmaster resets the peer state, so the dwell is faked after the
	// announce, not before. The peers still need their three exchanges.
	settleStartup(a)
	settleStartup(b)

	// A runs a full set one second ahead; B runs two exchanges two seconds
	// behind. B's negative offset is what Stats must report as an absolute.
	for i := 0; i < 3; i++ {
		exchangePeer(g, peerA, sourceA, uint16(i+1), base+int64(i)*100_000_000, 1_000_000_000)
	}
	for i := 0; i < 2; i++ {
		exchangePeer(g, peerB, sourceB, uint16(i+1), base+int64(i)*100_000_000, -2_000_000_000)
	}

	if !a.Ready() {
		t.Fatalf("A is not ready: %+v", a.Stats())
	}
	if b.Ready() {
		t.Fatalf("B is ready after two exchanges: %+v", b.Stats())
	}
	if st := a.Stats(); st.SourceID != sourceA || st.ClockID != clockA {
		t.Fatalf("A kept the wrong grandmaster: %+v", st)
	}
	if st := b.Stats(); st.SourceID != sourceB || st.ClockID != clockB {
		t.Fatalf("B kept the wrong grandmaster: %+v", st)
	}
	if g.Ready() {
		t.Fatal("the hub reported ready while one peer was not")
	}
	if off := a.Stats().OffsetNS; off < 1_000_000_000-1000 || off > 1_000_000_000+1000 {
		t.Fatalf("A offset = %f, want +1s", off)
	}
	if off := b.Stats().OffsetNS; off > -2_000_000_000+1000 || off < -2_000_000_000-1000 {
		t.Fatalf("B offset = %f, want -2s", off)
	}
	stats := g.Stats()
	if stats.Ready {
		t.Fatal("Stats.Ready is true while one peer is not ready")
	}
	if stats.OffsetNS < 2_000_000_000-1000 || stats.OffsetNS > 2_000_000_000+1000 {
		t.Fatalf("aggregate offset = %f, want the largest absolute offset", stats.OffsetNS)
	}
	// Two announces and five validated Sync/Follow pairs.
	if stats.Received != 12 || stats.Exchanges != 5 {
		t.Fatalf("aggregate counters = %+v, want 12 received and 5 exchanges", stats)
	}

	// Another source is not in the set; its packets must not touch any peer.
	before := g.Stats()
	dispatchFrom(g, "192.0.2.99", generalPort, announceFrom(sourceA, clockA), uint64(base))
	dispatchFrom(g, peerB, 12345, announceFrom(sourceB, clockB), uint64(base))
	if after := g.Stats(); after.Received != before.Received || after.Rejected != before.Rejected {
		t.Fatalf("an unknown source changed the counters: %+v -> %+v", before, after)
	}

	// B's third exchange makes both peers ready.
	exchangePeer(g, peerB, sourceB, 3, base+2*100_000_000, -2_000_000_000)
	if !g.Ready() || !g.Stats().Ready {
		t.Fatalf("the hub is not ready after both peers finished: %+v", g.Stats())
	}
}

// TestFollowersValidation pins the constructor boundary: a set needs at least
// one distinct IPv4 peer.
func TestFollowersValidation(t *testing.T) {
	cases := []struct {
		name  string
		hosts []string
		want  string
	}{
		{"empty", nil, "at least one peer"},
		{"bad address", []string{"living-room.local"}, "bad peer address"},
		{"ipv6", []string{"2001:db8::1"}, "not IPv4"},
		{"duplicate", []string{"192.0.2.50", "192.0.2.50"}, "duplicate peer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newFollowers(tc.hosts, 0, 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestFollowersStopWithoutStart pins the lifecycle: Stop on a hub that never
// started returns instead of waiting for a loop that will never close done,
// and a borrowed peer's Stop does not close the shared sockets.
func TestFollowersStopWithoutStart(t *testing.T) {
	g := newTestFollowers(t, "192.0.2.50", "192.0.2.51")
	peer := g.ClockFor("192.0.2.50")
	peer.Stop()

	stopped := make(chan struct{})
	go func() {
		g.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Followers.Stop blocked after New-without-Start")
	}
}
