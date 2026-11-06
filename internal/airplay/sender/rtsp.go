package sender

import (
	"bytes"
	"encoding/binary"
	"strings"
	"time"

	"saundo/internal/airplay/crypto"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/audio/resample"
	"saundo/internal/audio/ring"
)

func (s *Sender) rtspURI() string {
	return "rtsp://" + s.localIP + "/" + utoa(uint64(s.sessionID))
}

// sendRequest builds one RTSP request and queues the method so its reply can
// be matched. Replies arrive in request order.
func (s *Sender) sendRequest(method, uri, contentType, body string, extra []header) {
	var b strings.Builder
	b.WriteString(method)
	b.WriteByte(' ')
	b.WriteString(uri)
	b.WriteString(" RTSP/1.0\r\n")
	b.WriteString("CSeq: ")
	b.WriteString(itoa(s.cseq))
	b.WriteString("\r\n")
	s.cseq++
	b.WriteString("User-Agent: " + clientUserAgent + "\r\n")
	b.WriteString("DACP-ID: " + s.dacpID + "\r\n")
	b.WriteString("Active-Remote: " + utoa(uint64(s.activeRemote)) + "\r\n")
	b.WriteString("Client-Instance: " + s.dacpID + "\r\n")
	b.WriteString("X-Apple-Client-Name: " + s.identity.Name + "\r\n")
	if s.digestNonce != "" && s.digestPassword != "" {
		b.WriteString("Authorization: " + crypto.DigestAuthResponse(
			method, uri, s.digestUsername(), s.digestRealm, s.digestPassword, s.digestNonce) + "\r\n")
	}
	for _, h := range extra {
		b.WriteString(h.key + ": " + h.value + "\r\n")
	}
	if contentType != "" {
		b.WriteString("Content-Type: " + contentType + "\r\n")
	}
	if body != "" {
		b.WriteString("Content-Length: " + itoa(len(body)) + "\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString(body)

	s.pending = append(s.pending, pendingReq{method: method})
	s.pendingDigestVerb = method
	s.pendingDigestURI = uri
	s.writeRTSP([]byte(b.String()))
	if s.state == StateHandshake || s.state == StatePairing {
		s.timeout.arm(s.now())
	}
}

// writeRTSP encrypts the control channel after pair-verify. Each frame is
// [2-byte little-endian length][ciphertext][16-byte tag], chunked at 1024
// bytes, with the 2-byte length as AAD and an 8-byte little-endian counter.
func (s *Sender) writeRTSP(data []byte) {
	if !s.controlEncrypted || s.ap2 == nil {
		_ = s.io.TCPSend(raopio.TCPControl, data)
		return
	}
	var out bytes.Buffer
	for off := 0; off < len(data); {
		n := len(data) - off
		if n > 1024 {
			n = 1024
		}
		lp := [2]byte{byte(n), byte(n >> 8)}
		nonce := crypto.CounterNonce8(s.ctrlSendCtr)
		s.ctrlSendCtr++
		ct, err := crypto.ChaCha20Poly1305Encrypt(s.ap2.controlOut, nonce, data[off:off+n], lp[:])
		if err != nil {
			s.warn("control encrypt failed: %v", err)
			s.fail("Encrypted control channel failed")
			return
		}
		out.Write(lp[:])
		out.Write(ct)
		off += n
	}
	_ = s.io.TCPSend(raopio.TCPControl, out.Bytes())
}

func (s *Sender) onRTSPData(data []byte) {
	if !s.controlEncrypted || s.ap2 == nil {
		s.rxBuf = append(s.rxBuf, data...)
	} else {
		s.rtspEncBuf = append(s.rtspEncBuf, data...)
		for len(s.rtspEncBuf) >= 2 {
			n := int(binary.LittleEndian.Uint16(s.rtspEncBuf[:2]))
			need := 2 + n + 16
			if len(s.rtspEncBuf) < need {
				break
			}
			lp := s.rtspEncBuf[:2]
			nonce := crypto.CounterNonce8(s.ctrlRecvCtr)
			s.ctrlRecvCtr++
			plain, err := crypto.ChaCha20Poly1305Decrypt(s.ap2.controlIn, nonce, s.rtspEncBuf[2:need], lp)
			if err != nil {
				s.fail("Encrypted control channel authentication failed")
				return
			}
			s.rxBuf = append(s.rxBuf, plain...)
			s.rtspEncBuf = s.rtspEncBuf[need:]
		}
	}
	if len(s.rxBuf) > 4*1024*1024 {
		s.fail("Oversized RTSP response from the receiver")
		return
	}
	for {
		headEnd := bytes.Index(s.rxBuf, []byte("\r\n\r\n"))
		if headEnd < 0 {
			return
		}
		head := string(s.rxBuf[:headEnd])
		lines := strings.Split(head, "\n")
		headers := make(map[string]string, len(lines))
		for _, line := range lines[1:] {
			line = trimASCII(line)
			colon := strings.IndexByte(line, ':')
			if colon <= 0 {
				continue
			}
			headers[trimASCII(lowerASCII(line[:colon]))] = trimASCII(line[colon+1:])
		}
		contentLen := toInt(headerValue(headers, "content-length"))
		if contentLen < 0 {
			contentLen = 0
		}
		total := headEnd + 4 + contentLen
		if len(s.rxBuf) < total {
			return
		}
		statusLine := trimASCII("")
		if len(lines) > 0 {
			statusLine = trimASCII(lines[0])
		}
		body := string(s.rxBuf[headEnd+4 : total])
		s.rxBuf = s.rxBuf[total:]

		isRTSP := strings.HasPrefix(statusLine, "RTSP/")
		isHTTP := strings.HasPrefix(statusLine, "HTTP/")
		if !isRTSP && !isHTTP {
			s.info("ignoring server request %q", statusLine)
			continue
		}
		parts := strings.Split(statusLine, " ")
		code := 0
		if len(parts) > 1 {
			code = toInt(parts[1])
		}
		if len(s.pending) == 0 {
			s.warn("unexpected response (code %d)", code)
			continue
		}
		p := s.pending[0]
		s.pending = s.pending[1:]
		s.info("reply %q for %s", statusLine, p.method)
		switch {
		case p.http && code == 401 && (p.replay != nil || p.authRetry):
			s.handleAP2DigestChallenge(p, headers)
		case p.http:
			s.onPairingResponse(code, headers, body)
		default:
			s.handleResponse(p.method, code, headers)
		}
		if s.state == StateIdle {
			return
		}
	}
}

// digestUsername picks the user the receiver expects in the digest response:
// the AirPlay realm uses "AirPlay", RAOP receivers use "iTunes".
func (s *Sender) digestUsername() string {
	if strings.EqualFold(s.digestRealm, "raop") {
		return "iTunes"
	}
	return "AirPlay"
}

// parseDigestChallenge reads realm and nonce from a WWW-Authenticate value.
func parseDigestChallenge(wa string) (realm, nonce string) {
	extract := func(key string) string {
		k := strings.Index(lowerASCII(wa), key)
		if k < 0 {
			return ""
		}
		q1 := strings.IndexByte(wa[k:], '"')
		if q1 < 0 {
			return ""
		}
		q1 += k
		q2 := strings.IndexByte(wa[q1+1:], '"')
		if q2 < 0 {
			return ""
		}
		return wa[q1+1 : q1+1+q2]
	}
	return extract("realm="), extract("nonce=")
}

// handleAP2DigestChallenge answers a 401 on an AP2 request. The password
// challenge is HTTP Digest, so the realm and nonce are cached and the same
// request is replayed with an Authorization header. A 401 on the replay
// itself fails rather than looping.
func (s *Sender) handleAP2DigestChallenge(p pendingReq, headers map[string]string) {
	if s.digestPassword == "" {
		s.fail(fmtError("%s requires the receiver's AirPlay password. Save it in the app's settings for this receiver, then connect again.", s.name))
		return
	}
	if p.authRetry || p.replay == nil {
		s.fail("The password was not accepted by the device")
		return
	}
	realm, nonce := parseDigestChallenge(headerValue(headers, "www-authenticate"))
	if realm == "" || nonce == "" {
		s.fail("The device password challenge could not be parsed")
		return
	}
	s.digestRealm, s.digestNonce = realm, nonce
	s.info("digest challenge, re-sending %s with auth", p.method)
	p.replay()
}

func (s *Sender) handleResponse(method string, code int, headers map[string]string) {
	switch method {
	case "SET_PARAMETER":
		if code != 200 {
			s.warn("SET_PARAMETER rejected (%d)", code)
		}
		return
	case "POST":
		if code == 200 {
			if !s.feedback.active {
				s.feedback.interval = feedbackInterval
				s.feedback.arm(s.now())
			}
		} else {
			s.feedback.disarm()
			s.info("/feedback not supported (%d), keep-alive disabled", code)
		}
		return
	case "FEEDBACK", "TEARDOWN", "GET":
		return
	}

	if code == 401 {
		if s.digestPassword == "" {
			s.fail("The device requires a password")
			return
		}
		if s.digestRetried {
			s.fail("The password was not accepted by the device")
			return
		}
		wa := headerValue(headers, "www-authenticate")
		s.digestRealm, s.digestNonce = parseDigestChallenge(wa)
		if s.digestNonce == "" {
			s.fail("The device password challenge could not be parsed")
			return
		}
		s.digestRetried = true
		s.info("digest challenge, re-sending %s with auth", method)
		switch method {
		case "OPTIONS":
			s.sendOptions()
		case "ANNOUNCE":
			s.sendAnnounce()
		case "SETUP":
			s.sendSetup()
		case "RECORD":
			s.sendRecord()
		default:
			s.fail("The device password was required at an unexpected step")
		}
		return
	}

	if code < 200 || code >= 300 {
		if s.state == StateStreaming && (method == "RECORD" || method == "FLUSH") {
			s.info("%s not accepted (%d), non-fatal on AP2 realtime", method, code)
			return
		}
		s.fail(fmtError("Device refused %s (%d)", method, code))
		return
	}

	switch method {
	case "OPTIONS":
		s.sendAnnounce()
	case "ANNOUNCE":
		s.sendSetup()
	case "SETUP":
		transport := headerValue(headers, "transport")
		s.serverPort, s.controlPort, s.timingPort = 0, 0, 0
		for _, opt := range strings.Split(transport, ";") {
			eq := strings.IndexByte(opt, '=')
			if eq <= 0 {
				continue
			}
			key := trimASCII(opt[:eq])
			val := toUInt16(opt[eq+1:])
			switch key {
			case "server_port":
				s.serverPort = val
			case "control_port":
				s.controlPort = val
			case "timing_port":
				s.timingPort = val
			}
		}
		s.rtspSession = headerValue(headers, "session")
		if s.rtspSession == "" {
			s.rtspSession = "1"
		}
		if s.serverPort == 0 {
			s.fail("SETUP reply carried no server_port")
			return
		}
		s.info("remote ports: server=%d control=%d timing=%d", s.serverPort, s.controlPort, s.timingPort)
		s.sendRecord()
	case "RECORD":
		if s.state != StateStreaming {
			s.readyToStream()
		}
	case "FLUSH":
	}
}

func (s *Sender) sendOptions() {
	s.sendRequest("OPTIONS", "*", "", "", nil)
}

func (s *Sender) sendAnnounce() {
	sdp := "v=0\r\n" +
		"o=iTunes " + utoa(uint64(s.sessionID)) + " 0 IN IP4 " + s.localIP + "\r\n" +
		"s=iTunes\r\n" +
		"c=IN IP4 " + s.hostIP + "\r\n" +
		"t=0 0\r\n" +
		"m=audio 0 RTP/AVP 96\r\n" +
		"a=rtpmap:96 L16/44100/2\r\n" +
		"a=fmtp:96 352 0 16 40 10 14 2 255 0 0 44100\r\n"
	s.sendRequest("ANNOUNCE", s.rtspURI(), "application/sdp", sdp, nil)
}

func (s *Sender) sendSetup() {
	transport := "RTP/AVP/UDP;unicast;interleaved=0-1;mode=record;control_port=" +
		utoa(uint64(s.localControl)) + ";timing_port=" + utoa(uint64(s.localTiming))
	s.sendRequest("SETUP", s.rtspURI(), "", "", []header{{"Transport", transport}})
}

func (s *Sender) sendRecord() {
	rtpInfo := "seq=" + utoa(uint64(s.seq)) + ";rtptime=" + utoa(uint64(s.rtpTime32()))
	extra := []header{{"Range", "npt=0-"}, {"RTP-Info", rtpInfo}}
	if s.rtspSession != "" {
		extra = append(extra, header{"Session", s.rtspSession})
	}
	s.sendRequest("RECORD", s.rtspURI(), "", "", extra)
}

// onEventData decrypts and answers the reverse event channel. The response
// must carry only Server and CSeq: any other header corrupts the receiver's
// realtime timeline.
func (s *Sender) onEventData(data []byte) {
	if s.state == StateIdle {
		return
	}
	if s.ap2 == nil || len(s.ap2.eventIn) != 32 || len(s.ap2.eventOut) != 32 {
		return
	}
	s.eventEncBuf = append(s.eventEncBuf, data...)
	for len(s.eventEncBuf) >= 2 {
		n := int(binary.LittleEndian.Uint16(s.eventEncBuf[:2]))
		need := 2 + n + 16
		if len(s.eventEncBuf) < need {
			break
		}
		lp := s.eventEncBuf[:2]
		nonce := crypto.CounterNonce8(s.eventRecvCtr)
		s.eventRecvCtr++
		plain, err := crypto.ChaCha20Poly1305Decrypt(s.ap2.eventIn, nonce, s.eventEncBuf[2:need], lp)
		if err != nil {
			s.warn("event-channel decrypt failed, dropping")
			s.eventEncBuf = s.eventEncBuf[:0]
			return
		}
		s.eventPlain = append(s.eventPlain, plain...)
		s.eventEncBuf = s.eventEncBuf[need:]
	}
	if len(s.eventPlain) > 4*1024*1024 {
		s.eventPlain = s.eventPlain[:0]
		return
	}
	for {
		headEnd := bytes.Index(s.eventPlain, []byte("\r\n\r\n"))
		if headEnd < 0 {
			return
		}
		contentLen := 0
		cseq := ""
		for _, line := range strings.Split(string(s.eventPlain[:headEnd]), "\n") {
			line = trimASCII(line)
			colon := strings.IndexByte(line, ':')
			if colon <= 0 {
				continue
			}
			switch trimASCII(lowerASCII(line[:colon])) {
			case "content-length":
				contentLen = toInt(line[colon+1:])
			case "cseq":
				cseq = trimASCII(line[colon+1:])
			}
		}
		total := headEnd + 4 + contentLen
		if len(s.eventPlain) < total {
			return
		}
		if firstLine := strings.SplitN(string(s.eventPlain[:headEnd]), "\r\n", 2)[0]; firstLine != "" {
			s.info("event request %q", firstLine)
		}
		s.eventPlain = s.eventPlain[total:]

		resp := "RTSP/1.0 200 OK\r\nServer: " + serverUserAgent + "\r\n"
		if cseq != "" {
			resp += "CSeq: " + cseq + "\r\n"
		}
		resp += "\r\n"
		lp := [2]byte{byte(len(resp)), byte(len(resp) >> 8)}
		nonce := crypto.CounterNonce8(s.eventSendCtr)
		s.eventSendCtr++
		ct, err := crypto.ChaCha20Poly1305Encrypt(s.ap2.eventOut, nonce, []byte(resp), lp[:])
		if err != nil {
			s.warn("event-channel encrypt failed: %v", err)
			return
		}
		out := append([]byte{lp[0], lp[1]}, ct...)
		_ = s.io.TCPSend(raopio.TCPEvent, out)
	}
}

// onVolumePoll refreshes the receiver's own volume: AirPlay 2 receivers
// update `initialVolume` in GET /info as their hardware volume moves.
func (s *Sender) onVolumePoll() {
	if s.state != StateStreaming || !s.airplay2 || s.volQuery {
		return
	}
	s.volQuery = true
	s.sendAP2RTSP("GET", "/info", "", nil)
}

// handleVolumeReply parses a streaming-time /info reply for initialVolume and
// reports the receiver's own volume when it moved.
func (s *Sender) handleVolumeReply(body []byte) {
	s.volQuery = false
	if s.state != StateStreaming {
		return
	}
	root, err := crypto.Decode(body)
	if err != nil {
		return
	}
	m, ok := root.(map[string]any)
	if !ok {
		return
	}
	db := float64(plistInt(m, "initialVolume"))
	pct := float64(volPercentFromDB(db))
	if s.receiverVol >= 0 && pct == float64(s.receiverVol) {
		return
	}
	s.receiverVol = int(pct)
	if s.events.VolumeChanged != nil {
		s.events.VolumeChanged(pct)
	}
}

// volPercentFromDB converts AirPlay dBFS to a 0..100 percent like pyatv's
// pct_to_dbfs inverse: anything at or below -30 dB is mute.
func volPercentFromDB(db float64) int {
	if db >= 0 || db <= -30 {
		return 0
	}
	pct := int((db + 30) / 0.3)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct
}

// readyToStream runs when the receiver accepts the stream: RECORD on AP1, the
// stream SETUP reply on AP2. It fixes the RTP latency, disarms the handshake
// timer, and marks this member ready at the start gate. It does not start
// playback: the start happens from Tick, so every member of a pair is released
// in the same engine Tick.
func (s *Sender) readyToStream() {
	if s.streamAccepted {
		return
	}
	s.streamAccepted = true
	s.latency = uint32(s.nextLatencySamples)
	s.framesSent = 0
	s.firstAudio = true
	s.timeout.disarm()
	s.ptpWaitUntil = time.Time{}

	if s.startGate != nil {
		s.activeGate = s.startGate
	} else {
		var trim ring.Trimmer
		if s.ring != nil {
			trim = s.ring
		}
		s.activeGate = NewStartGate(StartGateConfig{
			Members: 1,
			Cushion: DefaultStartCushion,
			Trim:    trim,
		})
	}
	now := s.now()
	s.startWaitUntil = now.Add(startWaitTimeout)
	if err := s.activeGate.Arrive(now, s.inputRate); err != nil {
		s.fail(err.Error())
		return
	}
	s.awaitingStart = true
}

// maybeBegin starts playback exactly once, at the shared epoch, and fails the
// session if the cohort does not complete in time.
func (s *Sender) maybeBegin() {
	if !s.awaitingStart {
		return
	}
	now := s.now()
	if !s.startWaitUntil.IsZero() && now.After(s.startWaitUntil) {
		s.awaitingStart = false
		s.fail("Timed out waiting for both speakers to start together")
		return
	}
	epoch, ok := s.activeGate.Begin(now)
	if !ok {
		return
	}
	s.awaitingStart = false
	s.beginPlayback(epoch)
}

// beginPlayback arms the pacer and sync at the shared epoch. It recreates the
// resampler so the first popped sample is the ring's oldest surviving sample,
// not a staged leftover from a previous session.
func (s *Sender) beginPlayback(epoch time.Time) {
	s.clockStart = epoch
	s.startTs = ntp2ts(ntpAt(epoch), raopRate)
	s.framesSent = 0
	s.firstAudio = true
	s.resampler = resample.NewLinear(s.inputRate)
	s.setState(StateStreaming)
	s.timeout.disarm()
	s.ptpWaitUntil = time.Time{}
	if s.ptpClock != nil {
		s.sync.interval = 100 * time.Millisecond
	} else {
		s.sync.interval = time.Second
	}
	s.sendSyncPacket(true)
	now := s.now()
	s.sync.arm(now)

	if s.airplay2 && s.pendingVolume <= noVolume+1 {
		if s.ap2 != nil && s.ap2.hasInitial && s.ap2.initialVol > -144 {
			// Start at the receiver's own volume instead of forcing 0 dB, so
			// connecting never jumps a quiet HomePod to full volume.
			s.pendingVolume = s.ap2.initialVol
		} else {
			s.pendingVolume = 0
		}
	}
	if s.pendingVolume > noVolume+1 {
		body := "volume: " + fixed6(s.pendingVolume)
		var vh []header
		if s.rtspSession != "" {
			vh = append(vh, header{"Session", s.rtspSession})
		}
		s.sendRequest("SET_PARAMETER", s.rtspURI(), "text/parameters", body, vh)
	}

	s.pacer.arm(now)
	if s.pauseWanted {
		s.applyPause(true)
	}
	if s.airplay2 {
		s.feedback.interval = ap2Feedback
		s.volumePoll.arm(now)
	} else {
		s.feedback.interval = feedbackInterval
		s.sendRequest("POST", "/feedback", "", "", nil)
	}
	s.feedback.arm(now)
	s.sendMetadata()
	s.sendCover()
	s.info("streaming to %q, RTP %d frames/packet at %d Hz%s", s.name, framesPerPacket, raopRate,
		map[bool]string{true: " (encrypted)", false: ""}[s.ap2 != nil && s.ap2.encryptAudio])
	if s.events.Launched != nil {
		s.events.Launched(true, "")
	}
}

// InitialVolumePercent returns the receiver's volume from /info as a percent,
// for restoring it after a temporary change.
func (s *Sender) InitialVolumePercent() (float64, bool) {
	if s.ap2 == nil || !s.ap2.hasInitial {
		return 0, false
	}
	db := s.ap2.initialVol
	if db <= -144 {
		return 0, true
	}
	pct := (db*10 + 300) / 3
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct, true
}

// SetVolume sets the receiver volume in percent; 0 is AirPlay's mute sentinel.
func (s *Sender) SetVolume(pct float64) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	dbfs := (pct*3 - 300) / 10
	if pct < 0.01 {
		dbfs = -144
	}
	s.pendingVolume = dbfs
	if s.state == StateStreaming {
		body := "volume: " + fixed6(dbfs)
		var extra []header
		if s.rtspSession != "" {
			extra = append(extra, header{"Session", s.rtspSession})
		}
		s.sendRequest("SET_PARAMETER", s.rtspURI(), "text/parameters", body, extra)
	}
}

// SetNowPlaying pushes track metadata and the optional receiver cover over
// SET_PARAMETER. Empty fields are omitted.
func (s *Sender) SetNowPlaying(title, artist, album string, cover []byte, coverMIME string) {
	changed := title != s.npTitle || artist != s.npArtist || album != s.npAlbum ||
		!bytes.Equal(cover, s.mediaCover) || coverMIME != s.mediaCoverMIME
	s.npTitle, s.npArtist, s.npAlbum = title, artist, album
	s.mediaCover, s.mediaCoverMIME = cover, coverMIME
	if s.state == StateStreaming && changed {
		s.sendMetadata()
		s.sendCover()
	}
}

// SetLampArtwork stores the ambient-light image and whether it owns the cover
// slot. While on, the light wins over the receiver track-info cover; while
// off, the media cover wins when present and this image (the origin reset) is
// the fallback. An unchanged choice is a no-op; a reconnect resends the
// effective cover.
func (s *Sender) SetLampArtwork(data []byte, mime string, on bool) {
	if bytes.Equal(data, s.lampCover) && mime == s.lampCoverMIME && on == s.lampOn {
		s.info("ambient light: unchanged (%d bytes, on=%v)", len(data), on)
		return
	}
	s.lampCover = append(s.lampCover[:0], data...)
	s.lampCoverMIME = mime
	s.lampOn = on
	s.info("ambient light: %d bytes, on=%v, state=%s", len(data), on, s.state)
	if s.state == StateStreaming {
		s.sendCover()
	}
}

// coverImage is the image that owns the Now Playing cover slot. The ambient
// light wins while it is on; otherwise the receiver track-info cover wins,
// with the origin image as the fallback.
func (s *Sender) coverImage() ([]byte, string) {
	if s.lampOn && len(s.lampCover) > 0 {
		return s.lampCover, s.lampCoverMIME
	}
	if len(s.mediaCover) > 0 {
		return s.mediaCover, s.mediaCoverMIME
	}
	return s.lampCover, s.lampCoverMIME
}

// sendCover pushes the effective cover as a Now Playing image. The receiver
// renders that cover on its top display; there is no separate artwork POST.
func (s *Sender) sendCover() {
	if s.state != StateStreaming {
		return
	}
	img, mime := s.coverImage()
	if len(img) == 0 || len(img) > 8*1024*1024 {
		return
	}
	if mime == "" {
		mime = "image/jpeg"
	}
	if bytes.Equal(img, s.sentCover) && mime == s.sentCoverMIME {
		return
	}
	// The top display needs a Now Playing item to attach the artwork to.
	// When the app has no track metadata, send a minimal one.
	if s.npTitle == "" && s.npArtist == "" && s.npAlbum == "" {
		s.sendRequest("SET_PARAMETER", s.rtspURI(), "application/x-dmap-tagged",
			dmapTag("mlit", dmapTag("minm", "saundo")), s.metadataHeaders())
	}
	s.sendRequest("SET_PARAMETER", s.rtspURI(), mime, string(img), s.metadataHeaders())
	s.sentCover = append(s.sentCover[:0], img...)
	s.sentCoverMIME = mime
	s.info("cover: %d bytes, lamp=%v", len(img), s.lampOn)
}

// metadataHeaders is the Session and RTP-Info pair that track metadata,
// volume, and artwork requests carry.
func (s *Sender) metadataHeaders() []header {
	rtpInfo := "seq=" + utoa(uint64(s.seq)) + ";rtptime=" + utoa(uint64(s.rtpTime32()))
	var hdr []header
	if s.rtspSession != "" {
		hdr = append(hdr, header{"Session", s.rtspSession})
	}
	hdr = append(hdr, header{"RTP-Info", rtpInfo})
	return hdr
}

func (s *Sender) sendMetadata() {
	if s.state != StateStreaming {
		return
	}
	if s.npTitle == "" && s.npArtist == "" && s.npAlbum == "" {
		return
	}
	hdr := s.metadataHeaders()
	var inner strings.Builder
	if s.npTitle != "" {
		inner.WriteString(dmapTag("minm", s.npTitle))
	}
	if s.npAlbum != "" {
		inner.WriteString(dmapTag("asal", s.npAlbum))
	}
	if s.npArtist != "" {
		inner.WriteString(dmapTag("asar", s.npArtist))
	}
	if inner.Len() > 0 {
		s.sendRequest("SET_PARAMETER", s.rtspURI(), "application/x-dmap-tagged",
			dmapTag("mlit", inner.String()), hdr)
	}
}
