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
	// Underruns counts audio packets padded with silence because the ring
	// ran dry while it was still being fed real audio. Padding silence over
	// an idle source is the stream's correct output, not a defect, so it is
	// not counted.
	Underruns uint64
	// SentBlocks counts audio packets at least one copy of which reached the
	// host. SendErrors counts the datagrams the host refused, including a
	// primary send whose duplicate still went out.
	SentBlocks uint64
	SendErrors uint64
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
	s.ringFed, s.lastFedFrames = false, 0
	s.encryptErrors = 0
	s.retransmitWindowStart, s.retransmitBudget = time.Time{}, 0
	s.lastSync = time.Time{}
	s.wifi = WiFiMetrics{}
}

// Stats returns the session's diagnostic counters.
func (s *Sender) Stats() Stats {
	stats := Stats{
		Retransmits:   s.retransmits,
		LateTicks:     s.lateTicks,
		Underruns:     s.underruns,
		SentBlocks:    s.sentBlocks,
		SendErrors:    s.sendErrors,
		EncryptErrors: s.encryptErrors,
		LastSync:      s.lastSync,
		WiFi:          s.wifi,
	}
	if s.ring != nil && s.inputRate > 0 {
		frames := float64(s.ring.Available()) / channels
		stats.BufferMS = frames / float64(s.inputRate) * 1000
		stats.BufferValid = s.state == StateStreaming
	}
	return stats
}
