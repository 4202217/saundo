package sender

import (
	"encoding/binary"
	"testing"
	"time"

	raopio "saundo/internal/airplay/io"
)

func retransmitRequest(seq, count uint16) []byte {
	dg := make([]byte, 8)
	dg[1] = 0x55
	binary.BigEndian.PutUint16(dg[4:6], seq)
	binary.BigEndian.PutUint16(dg[6:8], count)
	return dg
}

// TestRetransmitRejectsForeignSourceAndBoundsBurst pins the reflection bound:
// only the receiver's address may request a retransmit, one request is capped,
// and one window has a fixed total budget.
func TestRetransmitRejectsForeignSourceAndBoundsBurst(t *testing.T) {
	io := &audioRecordIO{}
	s := New(io, Events{}, nil)
	s.state = StateStreaming
	s.hostIP = "192.168.1.20"
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	s.clock = func() time.Time { return now }
	for i := 0; i < backlogSize; i++ {
		s.backlog[i] = []byte{byte(i)}
		s.backlogSeq[i] = int32(i)
	}

	receiver := raopio.Endpoint{IP: "192.168.1.20", Port: 6000}
	s.onControlDatagram(retransmitRequest(0, backlogSize), raopio.Endpoint{IP: "10.0.0.9", Port: 6000})
	if len(io.control) != 0 {
		t.Fatalf("a foreign source produced %d retransmits, want 0", len(io.control))
	}

	s.onControlDatagram(retransmitRequest(0, backlogSize), receiver)
	if len(io.control) != maxRetransmitBurst {
		t.Fatalf("one request produced %d retransmits, want %d", len(io.control), maxRetransmitBurst)
	}

	for i := 0; i < 10; i++ {
		s.onControlDatagram(retransmitRequest(0, maxRetransmitBurst), receiver)
	}
	if len(io.control) != retransmitWindowBudget {
		t.Fatalf("one window produced %d retransmits, want %d", len(io.control), retransmitWindowBudget)
	}
	if !s.allowRetransmit(now.Add(retransmitWindowPeriod)) {
		t.Fatal("the budget did not reset in the next window")
	}
}
