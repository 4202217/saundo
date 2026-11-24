package sender

import "time"

// Stats is one sender session's diagnostic counters. The engine reads it on
// the sender's event loop and aggregates it into the UI snapshot; it is not
// safe to call from another goroutine while a session is live.
type Stats struct {
	// Retransmits counts backlog packets served in answer to 0x55 requests.
	Retransmits uint64
	// LateTicks counts pacer ticks that ended at least one packet behind the
	// wall-clock target after the capped burst.
	LateTicks uint64
	// Underruns counts audio packets padded with silence because the ring ran
	// dry while it was still being fed real audio, with the source back
	// within the window. A source that stays away is a pause: its silence run
	// is the stream's correct output and its short fills are never counted.
	Underruns uint64
	// SentBlocks counts audio packets at least one copy of which reached the
	// host. SendErrors counts the datagrams the host refused, including a
	// primary send whose duplicate still went out.
	SentBlocks uint64
	SendErrors uint64
	// SentBytes counts the audio datagrams the host accepted at L4, the UDP
	// header included: every duplicate and every retransmit reply.
	SentBytes uint64
	// LastPacketBytes is the L4 size of the last audio datagram built (RTP
	// header, payload, UDP header). Zero before the first packet.
	LastPacketBytes int
	// LastUnderrun is when the last counted underrun committed: the source
	// returned inside the window after a short fill. Zero means none yet.
	LastUnderrun time.Time
	// EncryptErrors counts audio packets dropped because the AP2 cipher
	// refused the payload. A packet that cannot be encrypted is never sent.
	EncryptErrors uint64
	// LastSync is when the last timing packet was sent. Zero means the
	// session has not sent one.
	LastSync time.Time
	// BufferMS is the ring's buffered audio in milliseconds; BufferValid is
	// false when no ring is attached or the session is not streaming.
	BufferMS    float64
	BufferValid bool
	// WiFi is the receiver's snapshot from the stream SETUP response.
	WiFi WiFiMetrics
}

func (s *Sender) resetSessionCounters() {
	s.retransmits, s.lateTicks, s.underruns, s.sentBlocks, s.sendErrors = 0, 0, 0, 0, 0
	s.sentBytes, s.lastPacketBytes, s.lastUnderrun = 0, 0, time.Time{}
	s.pendingUnderruns = 0
	s.ringFed, s.lastFedFrames = false, 0
	s.encryptErrors = 0
	s.retransmitWindowStart, s.retransmitBudget = time.Time{}, 0
	s.lastSync = time.Time{}
	s.wifi = WiFiMetrics{}
}

// Stats returns the session's diagnostic counters.
func (s *Sender) Stats() Stats {
	stats := Stats{
		Retransmits:     s.retransmits,
		LateTicks:       s.lateTicks,
		Underruns:       s.underruns,
		SentBlocks:      s.sentBlocks,
		SendErrors:      s.sendErrors,
		SentBytes:       s.sentBytes,
		LastPacketBytes: s.lastPacketBytes,
		LastUnderrun:    s.lastUnderrun,
		EncryptErrors:   s.encryptErrors,
		LastSync:        s.lastSync,
		WiFi:            s.wifi,
	}
	if s.ring != nil && s.inputRate > 0 {
		frames := float64(s.ring.Available()) / channels
		stats.BufferMS = frames / float64(s.inputRate) * 1000
		stats.BufferValid = s.state == StateStreaming
	}
	return stats
}
