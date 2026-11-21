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
	// GroupCohort is more than one target. GroupSize counts senders, not
	// targets, because the stats are per sender.
	GroupCohort
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
	SentBlocks  uint64
	SendErrors  uint64
	// DroppedEvents is how many host notifications the session's event queue
	// discarded, summed across stereo members.
	DroppedEvents uint64
	// RecentUnderruns and RecentDropped are the trailing stability window's
	// counts: audio packets padded with silence, and samples the capture
	// dropped, summed across members. The verdict reads only this span, so a
	// join blip stops reading as stutter once it ages out.
	RecentUnderruns uint64
	RecentDropped   uint64
	SendBufferMS    float64
	BufferValid     bool
	WiFi            sender.WiFiMetrics
	// Degraded is true when the stability window saw a dropped sample, a
	// packet padded with silence, or a refused audio datagram.
	Degraded bool
}

// buildDiagnostics folds the active transport's per-member counters into the
// page model. The group shape comes from the plan, not from the stats' length,
// so a pair whose member is missing still reads as a pair. GroupSize counts
// senders, because the stats are per sender.
func (e *Engine) buildDiagnostics(now time.Time, dropped uint64) Diagnostics {
	kind, size, name := GroupNone, 0, ""
	switch {
	case len(e.plan.targets) > 1:
		kind = GroupCohort
		size = len(e.plan.members)
		name = e.plan.connectedName()
	case len(e.plan.targets) == 1:
		t := e.plan.targets[0]
		if t.stereo {
			kind = GroupStereo
			size = t.count
			name = t.groupName
			if name == "" {
				name = t.name
			}
		} else {
			kind = GroupSingle
			size = 1
		}
	default:
		// Idle keeps showing the last target's shape from the merged rows.
		if dev, ok := e.find(e.selectedID); ok {
			if dev.IsStereoGroup() {
				kind = GroupStereo
				size = len(dev.Members)
				name = dev.GroupName
				if name == "" {
					name = dev.Name
				}
			} else {
				kind = GroupSingle
				size = 1
			}
		}
	}
	var uptime time.Duration
	if !e.sessionStartedAt.IsZero() {
		uptime = now.Sub(e.sessionStartedAt)
		if uptime < 0 {
			uptime = 0
		}
	}
	var members []sender.Stats
	if e.tr != nil {
		members = e.tr.Stats()
	}
	overflow := dropped
	if len(e.memberRings) > 0 {
		// Member rings exist for every cohort, including one sender. The
		// group's 欠载 / 溢出 row sums the members; DroppedSamples stays the
		// max because stability is a session state, not a total.
		overflow = e.memberDropped
	}
	if e.tr != nil {
		e.stability.observe(now, members, overflow)
	} else {
		e.stability.reset()
	}
	d := aggregateDiagnostics(members, kind, size, name, e.reconnectCount, uptime, e.stability.recent, now)
	if e.tr != nil {
		d.DroppedEvents = e.tr.DroppedEvents()
	}
	return d
}

// aggregateDiagnostics folds the members' sender stats into one model.
// recent is the trailing-window evidence from sessionStability; lifetime
// counters that it does not window stay in their own fields.
func aggregateDiagnostics(members []sender.Stats, group GroupKind, groupSize int, groupName string,
	reconnects int, sessionUptime time.Duration, recent stabilityCounts, now time.Time) Diagnostics {
	d := Diagnostics{
		Group:           group,
		GroupSize:       groupSize,
		GroupName:       groupName,
		Reconnects:      reconnects,
		SessionUptime:   sessionUptime,
		RecentUnderruns: recent.Underruns,
		RecentDropped:   recent.Dropped,
		Degraded:        recent.Underruns > 0 || recent.Dropped > 0 || recent.SendErrors > 0,
		HasStats:        len(members) > 0,
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
	return d
}
