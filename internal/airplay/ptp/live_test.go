package ptp

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"
	"time"
)

// TestLivePTP captures PTP packets from a receiver on the LAN. Set
// SAUNDO_PTP_PEER to its IPv4 address; the receiver must be in an active
// AirPlay session (SETPEERS) for it to exchange timing with this process.
func TestLivePTP(t *testing.T) {
	peer := os.Getenv("SAUNDO_PTP_PEER")
	if peer == "" {
		t.Skip("set SAUNDO_PTP_PEER to a receiver address")
	}
	f, err := New(peer)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	f.OnPacket = func(kind uint8, seq uint16, data []byte) {
		if seen >= 12 {
			return
		}
		seen++
		ts, _ := readTimestamp(data[34:44])
		t.Logf("kind=%d seq=%d len=%d flags=%04x ts=%d bytes34_44=%s",
			kind, seq, len(data), binary.BigEndian.Uint16(data[6:8]), ts,
			hex.EncodeToString(data[34:44]))
	}
	f.Start()
	defer f.Stop()
	f.Activate()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		t.Logf("stats: %+v", f.Stats())
	}
}
