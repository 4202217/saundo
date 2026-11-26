package sender

import (
	"fmt"
	"strings"
	"testing"
)

// TestPTPTimelineSkipsAreCounted pins the silent-gap signal: a PTP follower
// without a timeline drops sync packets, and the session records how many
// instead of leaving the gap invisible.
func TestPTPTimelineSkipsAreCounted(t *testing.T) {
	var logs []string
	s := New(&audioRecordIO{}, Events{}, func(_ Level, format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	s.state = StateStreaming
	s.serverPort = 6000
	s.controlPort = 6001
	s.SetPTPClock(neverReadyClock{})

	s.sendSyncPacket(false)
	s.sendSyncPacket(false)

	if got := s.Stats().SyncSkips; got != 2 {
		t.Fatalf("sync skips = %d, want 2", got)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "PTP timeline") {
		t.Fatalf("sync skip logs = %q, want one PTP timeline line", logs)
	}
}
