package sender

import (
	"time"

	"saundo/internal/airplay/device"
	"saundo/internal/audio/ring"
)

// The sender's configuration API: the setters and getters the host uses to
// describe the receiver, timing, and capture before Start. They own no I/O
// and run before or between sessions.

// SetClock replaces the clock used by timers and pacing. Tests use it.
func (s *Sender) SetClock(clock func() time.Time) {
	if clock == nil {
		clock = time.Now
	}
	s.clock = clock
}

// SetIdentity sets the name, device id, and model the receiver sees.
func (s *Sender) SetIdentity(id Identity) {
	defaults := defaultIdentity()
	id.Name = stripCRLF(id.Name)
	id.DeviceID = stripCRLF(id.DeviceID)
	id.Model = stripCRLF(id.Model)
	if id.Name == "" {
		id.Name = defaults.Name
	}
	if id.DeviceID == "" {
		id.DeviceID = defaults.DeviceID
	}
	if id.Model == "" {
		id.Model = defaults.Model
	}
	s.identity = id
}

// SetStrictReceiverAuth fails the session when a receiver proof or signature
// does not verify. The default logs and continues, matching the reference.
func (s *Sender) SetStrictReceiverAuth(strict bool) { s.strictAuth = strict }

// SetPTPClock selects PTP timing. A nil clock keeps NTP timing.
func (s *Sender) SetPTPClock(clock PTPClock) { s.ptpClock = clock }

// SetGroupUUID marks the session as a member of a stereo pair. The value is
// the pair's `tsid`, sent as groupUUID in the session SETUP.
func (s *Sender) SetGroupUUID(id string) { s.groupUUID = id }

// SetTimingPeers lists every receiver address in the pair. SETPEERS announces
// them together with the sender's own address.
func (s *Sender) SetTimingPeers(hosts []string) {
	s.timingPeers = append([]string(nil), hosts...)
}

// SetAuth describes how to reach the receiver before Start.
func (s *Sender) SetAuth(auth device.Auth, airplay2 bool, deviceID, credsJSON, password string) {
	s.authMethod = auth
	s.airplay2 = airplay2
	s.deviceID = deviceID
	s.credsJSON = credsJSON
	s.digestPassword = password
}

// SetFamily selects the receiver family used for key derivation, latency,
// and request headers. It is independent of SetAuth: a HomePod can pair
// transiently or with a password and keeps the HomePod profile.
func (s *Sender) SetFamily(family device.Family) { s.family = family }

// AttachRing supplies the PCM ring the sender pulls from while streaming.
func (s *Sender) AttachRing(r *ring.Buffer) { s.ring = r }

// SetStartGate puts the sender in a shared start cohort. Senders that share a
// gate begin consuming at the gate's epoch. A nil gate gates the sender alone
// over its own ring. Call before Start; create a fresh gate per session.
func (s *Sender) SetStartGate(g *StartGate) { s.startGate = g }

// SetInputFormat sets the sample rate of the PCM in the ring. A non-positive
// rate is treated as 48000.
func (s *Sender) SetInputFormat(rate int) {
	if rate <= 0 {
		rate = 48000
	}
	s.inputRate = rate
	if s.resampler != nil {
		s.resampler.SetInputRate(rate)
	}
}

// SetLatency sets the presentation latency in milliseconds; it takes effect
// on the next session's stream SETUP.
func (s *Sender) SetLatency(ms int) {
	if ms < device.MinLatencyMS {
		ms = device.MinLatencyMS
	}
	if ms > device.MaxLatencyMS {
		ms = device.MaxLatencyMS
	}
	s.latencyMS = ms
}

// setDuplicates sets how many extra copies of each audio packet go on the
// wire. The receiver dedups by RTP sequence, so the copies are idempotent.
// Zero is the production default; the lossy-network probe drives it.
func (s *Sender) setDuplicates(n int) {
	if n < 0 {
		n = 0
	}
	if n > 7 {
		n = 7
	}
	s.duplicates = n
}

// SetDuplicates sets how many extra copies of each audio packet go on the
// wire. One extra copy is the normal weak-network safety net.
func (s *Sender) SetDuplicates(n int) { s.setDuplicates(n) }

// SetPrefill sends n extra audio packets as playback begins, so the receiver
// has a few packets buffered before the first jittered packet arrives.
func (s *Sender) SetPrefill(n int) {
	if n < 0 {
		n = 0
	}
	if n > 8 {
		n = 8
	}
	s.prefill = n
}

// ReceiverLatencyMS returns the receiver's reported arrival-to-render latency
// in milliseconds. 0 means the receiver did not report one.
func (s *Sender) ReceiverLatencyMS() int { return s.receiverLatencyMS }

// Active reports whether a session is in progress.
func (s *Sender) Active() bool { return s.state != StateIdle }

// WaitingForPin reports whether the sender waits for SubmitPin.
func (s *Sender) WaitingForPin() bool { return s.waitingForPin }

// State reports the current protocol phase.
func (s *Sender) State() State { return s.state }
