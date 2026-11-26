package sender

import (
	"encoding/binary"
	"time"

	"saundo/internal/airplay/alac"
	"saundo/internal/airplay/crypto"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/audio/resample"
)

func (s *Sender) now() time.Time { return s.clock() }

// rtpTime32 is the RTP timestamp of the next packet. The huge NTP-derived
// base cancels, so it starts at the fixed latency and advances one per frame.
func (s *Sender) rtpTime32() uint32 {
	return uint32(uint64(s.latency) + s.framesSent)
}

// onPacerTick keeps frames on the wire tracking wall clock at 44100 per second.
// A late tick is repaid in a capped burst so a scheduler stall does not flood
// the receiver. Ticks that still end behind the target after the burst are
// counted; a streak of them is the only signal that this host cannot encode
// and send at realtime.
func (s *Sender) onPacerTick() {
	elapsed := s.now().Sub(s.clockStart).Nanoseconds()
	if elapsed < 0 {
		return
	}
	target := uint64(elapsed) * raopRate / 1_000_000_000
	sent := 0
	for s.framesSent+framesPerPacket <= target && sent < maxPacketsPerTick {
		s.sendAudioPacket()
		sent++
	}
	// A healthy tick ends less than one packet behind the target; only a
	// residual at least a full packet wide means the burst cap bound the
	// send and this host is genuinely falling behind.
	if gap := target - s.framesSent; gap >= framesPerPacket {
		s.pacerLagFrames = gap
		s.pacerLagStreak++
		s.lateTicks++
		if s.pacerLagStreak == pacerLagWarnStreak {
			s.warn("sender cannot keep up: %d frames behind after %d late pacer ticks", s.pacerLagFrames, s.pacerLagStreak)
		}
		return
	}
	s.pacerLagFrames = 0
	s.pacerLagStreak = 0
}

// PacerLag reports how far the last pacer tick left the stream behind the
// wall-clock target, and whether late ticks have accumulated to a warning.
func (s *Sender) PacerLag() (frames uint64, lagging bool) {
	return s.pacerLagFrames, s.pacerLagStreak >= pacerLagWarnStreak
}

// SetPaused suspends or resumes the audio pacer. While suspended the sender
// sends no RTP: the receiver drains its buffer and stays silent, while the
// sync, feedback, volume, and event timers keep the session and its clock
// alive. Resume re-anchors the RTP time to the wall clock and starts from the
// newest capture cushion, so the receiver schedules the first packet at its
// configured latency instead of at the stale pre-pause timestamp.
//
// Pausing before streaming records the request; beginPlayback applies it.
// AirPlay 1 receivers keep the continuous silence stream: suspend and resume
// are verified only on HAP AP2.
func (s *Sender) SetPaused(paused bool) bool {
	if !s.PauseSupported() {
		return false
	}
	s.pauseWanted = paused
	if s.state == StateStreaming {
		s.applyPause(paused)
	}
	return true
}

// PauseSupported reports whether this sender can suspend and resume the
// realtime stream. Only HAP AP2 receivers verify it; an AirPlay 1 receiver
// keeps streaming silence.
func (s *Sender) PauseSupported() bool { return s.airplay2 }

// PauseRequested reports whether a pause is recorded for this sender, either
// applied now or waiting for the stream to begin. A caller that must not
// pause part of a cohort checks it before any member is touched.
func (s *Sender) PauseRequested() bool { return s.pauseWanted }

// Pushing reports whether the realtime stream is currently sending RTP. A
// suspended pacer is not pushing, which is the state the idle standby counts.
func (s *Sender) Pushing() bool {
	return s.state == StateStreaming && !s.paused
}

func (s *Sender) applyPause(paused bool) {
	if s.paused == paused {
		return
	}
	s.paused = paused
	s.pendingUnderruns = 0
	if paused {
		s.pacer.disarm()
		s.info("audio idle: stream suspended")
		return
	}
	now := s.now()
	s.framesSent = s.elapsedFrames(now)
	s.resampler = resample.NewLinear(s.inputRate)
	if s.ring != nil {
		frames := int(float64(s.inputRate) * DefaultStartCushion.Seconds())
		s.ring.KeepNewest(frames * channels)
	}
	s.ringFed, s.lastFedFrames = false, s.framesSent
	// A resume re-anchors the RTP time. Without a flush the receiver sees a
	// timestamp jump with a continuous sequence and can leave its jitter
	// buffer or PLC in a bad state (2026-11-25); the flush tells it to discard
	// buffered audio before the next packet.
	s.sendFlush()
	s.pacer.arm(now)
	s.info("audio resumed at frame %d", s.framesSent)
}

// sendFlush tells the receiver to discard buffered audio before the next
// packet. A resume after a suspension re-anchors the RTP time; the flush
// marks the same point on the receiver's side so the timestamp jump is not
// read as loss. A non-2xx reply is non-fatal on an AP2 realtime stream.
func (s *Sender) sendFlush() {
	if !s.airplay2 || s.state != StateStreaming {
		return
	}
	rtpInfo := "seq=" + utoa(uint64(s.seq)) + ";rtptime=" + utoa(uint64(s.rtpTime32()))
	extra := []header{{"RTP-Info", rtpInfo}}
	if s.rtspSession != "" {
		extra = append(extra, header{"Session", s.rtspSession})
	}
	s.sendRequest("FLUSH", s.rtspURI(), "", "", extra)
	s.info("flush: seq=%d rtptime=%d", s.seq, s.rtpTime32())
}

func (s *Sender) sendAudioPacket() {
	var frames [framesPerPacket * channels]int16
	available := s.fillFrames(frames[:])
	if available < framesPerPacket {
		// A short fill is padded with silence. Whether that is a defect
		// depends on what follows: a source that comes back within the window
		// lost audio, while one that stays away is a pause and the padding is
		// the correct output. Hold the count until a full packet commits it or
		// the window closes. The window divides the capture rate while
		// framesSent counts output frames, so at 48 kHz capture it runs about
		// a tenth longer than 100 ms — close enough for a heuristic.
		if s.ringFed && s.framesSent-s.lastFedFrames <= uint64(s.inputRate)/10 {
			s.pendingUnderruns++
		} else {
			s.pendingUnderruns = 0
		}
	} else {
		if s.framesSent-s.lastFedFrames <= uint64(s.inputRate)/10 {
			s.underruns += s.pendingUnderruns
		}
		s.pendingUnderruns = 0
		s.ringFed = true
		s.lastFedFrames = s.framesSent
	}

	var header [12]byte
	header[0] = 0x80
	if s.firstAudio {
		header[1] = 0xE0
	} else {
		header[1] = 0x60
	}
	binary.BigEndian.PutUint16(header[2:4], s.seq)
	binary.BigEndian.PutUint32(header[4:8], s.rtpTime32())
	binary.BigEndian.PutUint32(header[8:12], s.sessionID)

	var payload []byte
	if s.ap2 != nil && s.ap2.encryptAudio {
		if available == 0 {
			// A fully starved packet is a silent fill. The compressed silence
			// frame keeps the receiver's realtime timeline fed at a fraction
			// of the uncompressed frame's bandwidth, so an idle source never
			// starves the receiver's renderer.
			payload = alac.EncodeCompactSilence()
		} else if frame, err := alac.EncodeUncompressed(frames[:]); err == nil {
			payload = frame
		} else {
			payload = alac.EncodeSilence()
		}
	} else {
		payload = make([]byte, len(frames)*2)
		for i, sample := range frames {
			binary.BigEndian.PutUint16(payload[i*2:], uint16(sample))
		}
	}
	wire, err := s.encryptAudioPayload(header[:], payload)
	if err != nil {
		s.encryptErrors++
		s.encryptFailStreak++
		if s.encryptFailStreak >= maxConsecutiveEncryptFailures {
			// The key or the cipher state is broken on this session. A
			// retransmit cannot fill the gap because the packet never entered
			// the backlog, so end the session and let the engine rebuild it.
			s.fail("The audio stream could not be encrypted for the receiver.")
			return
		}
		if s.encryptErrors == 1 {
			s.warn("audio encrypt failed, dropping packet: %v", err)
		}
		s.firstAudio = false
		s.seq++
		s.framesSent += framesPerPacket
		return
	}
	s.encryptFailStreak = 0

	pkt := make([]byte, 0, len(header)+len(wire))
	pkt = append(pkt, header[:]...)
	pkt = append(pkt, wire...)
	delivered := false
	for i := 0; i <= s.duplicates; i++ {
		if err := s.io.UDPSend(raopio.UDPAudio, raopio.Endpoint{IP: s.hostIP, Port: s.serverPort}, pkt); err != nil {
			s.sendErrors++
			if s.sendErrors == 1 {
				s.warn("audio send failed: %v", err)
			}
			continue
		}
		delivered = true
	}
	if delivered {
		s.sendFailStreak = 0
		s.sentBlocks++
	} else {
		s.sendFailStreak++
		if s.sendFailStreak >= maxConsecutiveSendFailures {
			// Every copy of every packet for 400 ms failed. The session is
			// connected but the receiver is not getting audio; fail it and
			// let the engine retry instead of streaming into a dead socket.
			s.fail("The audio stream could not be sent to the receiver.")
			return
		}
	}

	slot := int(s.seq) & (backlogSize - 1)
	s.backlog[slot] = append(s.backlog[slot][:0], pkt...)
	s.backlogSeq[slot] = int32(s.seq)

	s.firstAudio = false
	s.seq++
	s.framesSent += framesPerPacket
}

func (s *Sender) fillFrames(dst []int16) int {
	if s.ring == nil {
		clear(dst)
		return 0
	}
	if s.resampler == nil {
		s.resampler = resample.NewLinear(s.inputRate)
	}
	frames := s.resampler.Resample(dst, s.ring.Pop)
	applyChannel(dst, frames, s.channel)
	return frames
}

// encryptAudioPayload applies the AP2 audio cipher. AAD is RTP header bytes 4
// through 11; the 8-byte nonce is appended after ciphertext and tag. A cipher
// error returns no payload, so the caller drops the packet instead of sending
// plaintext. The nonce stays unadvanced because the receiver never saw it.
func (s *Sender) encryptAudioPayload(header, payload []byte) ([]byte, error) {
	if s.ap2 == nil || !s.ap2.encryptAudio {
		return payload, nil
	}
	nonce := crypto.CounterNonce8(s.ap2.audioNonce)
	ct, err := crypto.ChaCha20Poly1305Encrypt(s.ap2.audioKey, nonce, payload, header[4:12])
	if err != nil {
		return nil, err
	}
	s.ap2.audioNonce++
	return append(ct, nonce...), nil
}

func (s *Sender) sendSyncPacket(first bool) {
	if s.serverPort == 0 || s.controlPort == 0 {
		return
	}
	if s.ptpClock != nil {
		now := s.mediaRTP(s.now())
		id, network, ok := s.ptpClock.Timeline(uint64(s.now().UnixNano()))
		if !ok {
			s.ptpSyncSkips++
			if s.ptpSyncSkips == 1 || s.ptpSyncSkips%1000 == 0 {
				s.warn("PTP timeline unavailable, sync packet skipped (%d)", s.ptpSyncSkips)
			}
			return
		}
		pkt := make([]byte, 28)
		if first {
			pkt[0] = 0x90
		} else {
			pkt[0] = 0x80
		}
		pkt[1] = 0xD7 // PTP-timed sync
		binary.BigEndian.PutUint16(pkt[2:4], 6)
		binary.BigEndian.PutUint32(pkt[4:8], now)
		binary.BigEndian.PutUint64(pkt[8:16], network)
		binary.BigEndian.PutUint32(pkt[16:20], now-s.latency)
		binary.BigEndian.PutUint64(pkt[20:28], id)
		if err := s.io.UDPSend(raopio.UDPControl, raopio.Endpoint{IP: s.hostIP, Port: s.controlPort}, pkt); err == nil {
			s.lastSync = s.now()
		}
		return
	}
	elapsed := s.elapsedFrames(s.now())
	now := uint32(uint64(s.latency) + elapsed)
	// curNTP uses the unwrapped frame count: deriving it from the 32-bit RTP
	// value would wrap after about 27 hours.
	curNTP := ts2ntp(s.startTs+elapsed, raopRate)
	pkt := make([]byte, 20)
	if first {
		pkt[0] = 0x90
	} else {
		pkt[0] = 0x80
	}
	pkt[1] = 0xD4
	binary.BigEndian.PutUint16(pkt[2:4], 7)
	binary.BigEndian.PutUint32(pkt[4:8], now-s.latency)
	binary.BigEndian.PutUint32(pkt[8:12], uint32(curNTP>>32))
	binary.BigEndian.PutUint32(pkt[12:16], uint32(curNTP))
	binary.BigEndian.PutUint32(pkt[16:20], now)
	if err := s.io.UDPSend(raopio.UDPControl, raopio.Endpoint{IP: s.hostIP, Port: s.controlPort}, pkt); err == nil {
		s.lastSync = s.now()
	}
}

// onControlDatagram answers type 0x55 retransmit requests from the backlog,
// replying to the source address with the original packet. Only the receiver
// that owns this session may ask: the reply goes to the datagram's UDP source,
// so an unvalidated request turns the sender into a reflector. The port is not
// checked because a receiver's retransmit source port can differ from its
// control port; its address cannot.
func (s *Sender) onControlDatagram(dg []byte, from raopio.Endpoint) {
	if len(dg) < 8 {
		return
	}
	if dg[1]&0x7F != 0x55 {
		return
	}
	if s.hostIP == "" || from.IP != s.hostIP {
		return
	}
	lostSeq := binary.BigEndian.Uint16(dg[4:6])
	lostCount := int(binary.BigEndian.Uint16(dg[6:8]))
	if lostCount > maxRetransmitBurst {
		lostCount = maxRetransmitBurst
	}
	now := s.now()
	for i := 0; i < lostCount; i++ {
		seq := lostSeq + uint16(i)
		slot := int(seq) & (backlogSize - 1)
		if s.backlogSeq[slot] != int32(seq) || len(s.backlog[slot]) == 0 {
			continue
		}
		if !s.allowRetransmit(now) {
			return
		}
		resp := make([]byte, 4, 4+len(s.backlog[slot]))
		resp[0], resp[1] = 0x80, 0xD6
		binary.BigEndian.PutUint16(resp[2:4], seq)
		resp = append(resp, s.backlog[slot]...)
		if err := s.io.UDPSend(raopio.UDPControl, from, resp); err == nil {
			s.retransmits++
		}
	}
}

// allowRetransmit consumes one packet from the session's per-window budget.
// The receiver is the only accepted source, so one budget covers the peer.
func (s *Sender) allowRetransmit(now time.Time) bool {
	if s.retransmitWindowStart.IsZero() || now.Sub(s.retransmitWindowStart) >= retransmitWindowPeriod {
		s.retransmitWindowStart = now
		s.retransmitBudget = retransmitWindowBudget
	}
	if s.retransmitBudget <= 0 {
		return false
	}
	s.retransmitBudget--
	return true
}

// onTimingDatagram answers the receiver's NTP request with its send time
// echoed as the reference time.
func (s *Sender) onTimingDatagram(dg []byte, from raopio.Endpoint) {
	if len(dg) < 32 {
		return
	}
	now := s.ntpNow()
	resp := make([]byte, 32)
	resp[0] = dg[0]
	resp[1] = 0xD3
	binary.BigEndian.PutUint16(resp[2:4], 7)
	// bytes 4..8 are zero padding. The reference time echoes the request's
	// transmit time; receive and send time are the same wall-clock read.
	copy(resp[8:16], dg[24:32])
	binary.BigEndian.PutUint32(resp[16:20], uint32(now>>32))
	binary.BigEndian.PutUint32(resp[20:24], uint32(now))
	binary.BigEndian.PutUint32(resp[24:28], uint32(now>>32))
	binary.BigEndian.PutUint32(resp[28:32], uint32(now))
	_ = s.io.UDPSend(raopio.UDPTiming, from, resp)
}
