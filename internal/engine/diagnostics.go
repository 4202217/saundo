package engine

import (
	"time"

	"saundo/internal/airplay/sender"
)

// GroupKind is the shape of the active target. A single receiver and a
// stereo pair aggregate their diagnostics differently, so the UI reads the
// kind instead of re-deriving it from the device list.
type GroupKind uint8

const (
	GroupNone GroupKind = iota
	GroupSingle
	GroupStereo
)

// Diagnostics is the diagnostics data, aggregated once per publish on the
// engine goroutine. Counters are the sender's; group values follow the group
// rules: WiFi, late ticks, and buffer take the worst member, while resends
// and underruns sum.
type Diagnostics struct {
	Group         GroupKind
	GroupSize     int
	GroupName     string
	Reconnects    int
	SessionUptime time.Duration
	// SyncAge is the time since the oldest member's last timing packet.
	// SyncValid is false when any member has not sent one.
	SyncAge   time.Duration
	SyncValid bool
	// HasStats is false when no sender session exists, so the page can show
	// no source instead of a healthy zero.
	HasStats    bool
	Retransmits uint64
	LateTicks   uint64
	Underruns   uint64
	SentBlocks  uint64
	SendErrors  uint64
	// DroppedEvents is how many host notifications the session's event queue
	// discarded, summed across stereo members.
	DroppedEvents uint64
	// Overflow is the session's dropped sample count, already the worst
	// member's when the target is a stereo pair.
	Overflow     uint64
	SendBufferMS float64
	BufferValid  bool
	WiFi         sender.WiFiMetrics
	// Degraded is true when the session dropped samples or padded an audio
	// packet with silence.
	Degraded bool
}

// aggregateDiagnostics folds the members' sender stats into one model.
// The dropped count comes from the engine's session accounting, which already
// scopes it to the session and takes the worst member.
func aggregateDiagnostics(members []sender.Stats, group GroupKind, groupSize int, groupName string,
	reconnects int, sessionUptime time.Duration, dropped uint64, now time.Time) Diagnostics {
	d := Diagnostics{
		Group:         group,
		GroupSize:     groupSize,
		GroupName:     groupName,
		Reconnects:    reconnects,
		SessionUptime: sessionUptime,
		Overflow:      dropped,
		Degraded:      dropped > 0,
		HasStats:      len(members) > 0,
	}
	if len(members) == 0 {
		return d
	}
	bufferSet := false
	bufferAll := true
	wifiAll := true
	syncValid := true
	var oldest time.Time
	var wifi sender.WiFiMetrics
	for _, m := range members {
		if m.WiFi.Valid {
			if !wifi.Valid || m.WiFi.RSSI < wifi.RSSI {
				wifi = m.WiFi
			}
		} else {
			wifiAll = false
		}
		d.Retransmits += m.Retransmits
		d.Underruns += m.Underruns
		d.SentBlocks += m.SentBlocks
		d.SendErrors += m.SendErrors
		if m.LateTicks > d.LateTicks {
			d.LateTicks = m.LateTicks
		}
		if m.BufferValid {
			if !bufferSet || m.BufferMS < d.SendBufferMS {
				d.SendBufferMS = m.BufferMS
			}
			bufferSet = true
		} else {
			bufferAll = false
		}
		if m.LastSync.IsZero() {
			syncValid = false
			continue
		}
		if oldest.IsZero() || m.LastSync.Before(oldest) {
			oldest = m.LastSync
		}
	}
	if !wifiAll {
		wifi = sender.WiFiMetrics{}
	}
	d.WiFi = wifi
	d.BufferValid = bufferSet && bufferAll
	d.SyncValid = syncValid && !oldest.IsZero()
	if d.SyncValid {
		d.SyncAge = now.Sub(oldest)
		if d.SyncAge < 0 {
			d.SyncAge = 0
		}
	}
	if d.Underruns > 0 || d.SendErrors > 0 {
		d.Degraded = true
	}
	return d
}
