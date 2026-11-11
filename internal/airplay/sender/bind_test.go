package sender

import (
	"errors"
	"testing"

	raopio "saundo/internal/airplay/io"
)

type bindErrorIO struct {
	errOn raopio.UDPChannel
}

func (b *bindErrorIO) TCPConnect(raopio.TCPChannel, string, uint16) error { return nil }
func (b *bindErrorIO) TCPSend(raopio.TCPChannel, []byte) error            { return nil }
func (b *bindErrorIO) TCPClose(raopio.TCPChannel, bool)                   {}
func (b *bindErrorIO) UDPSend(raopio.UDPChannel, raopio.Endpoint, []byte) error {
	return nil
}
func (b *bindErrorIO) UDPClose(raopio.UDPChannel) {}

func (b *bindErrorIO) UDPBind(ch raopio.UDPChannel) (uint16, error) {
	if ch == b.errOn {
		return 6000 + uint16(ch), errors.New("bind refused")
	}
	return 6000 + uint16(ch), nil
}

// TestStartFailsWhenUDPBindReportsError pins the fail-closed bind: a host that
// returns a usable port alongside an error must not be treated as bound.
func TestStartFailsWhenUDPBindReportsError(t *testing.T) {
	var launched []launchResult
	s := New(&bindErrorIO{errOn: raopio.UDPControl}, Events{
		Launched: func(ok bool, why string) { launched = append(launched, launchResult{ok, why}) },
	}, nil)

	s.Start("192.168.1.20", 7000, "FakeBox")

	if s.state != StateIdle {
		t.Fatalf("state = %v after a failed bind, want idle", s.state)
	}
	if s.udpBound {
		t.Fatal("udpBound stayed true after a failed bind")
	}
	if len(launched) != 1 {
		t.Fatalf("Launched calls = %d, want 1", len(launched))
	}
	if launched[0].ok || launched[0].err != "Could not bind UDP sockets" {
		t.Fatalf("Launched = %+v, want the bind failure", launched[0])
	}
}
