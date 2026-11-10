package sender

import (
	"strings"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
)

// sendAP2RTSP sends a method whose reply carries a binary plist. RECORD and
// SETUP are RTSP methods on rtsp://host/sessionId, not POST paths.
func (s *Sender) sendAP2RTSP(method, uri, contentType string, body []byte) {
	s.sendAP2RTSPAttempt(method, uri, contentType, body, false)
}

// sendAP2RTSPAttempt builds one AP2 request. afterAuth marks the replay of a
// request that already met a digest challenge, so a second 401 fails instead
// of looping; the replay carries the cached Authorization header.
func (s *Sender) sendAP2RTSPAttempt(method, uri, contentType string, body []byte, afterAuth bool) {
	var b strings.Builder
	b.WriteString(method + " " + uri + " RTSP/1.0\r\n")
	b.WriteString("CSeq: " + itoa(s.cseq) + "\r\n")
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
	if s.family == device.FamilyMac {
		b.WriteString("X-Apple-ProtocolVersion: 1\r\n")
		if s.ap2 != nil && s.ap2.sessionUUID != "" {
			b.WriteString("X-Apple-Session-ID: " + strings.ToLower(s.ap2.sessionUUID) + "\r\n")
		}
	}
	if method == "SETUP" {
		if s.family == device.FamilyMac {
			b.WriteString("X-Apple-Stream-ID: 1\r\n")
		} else {
			b.WriteString("X-Apple-StreamID: 1\r\n")
		}
	}
	if contentType != "" {
		b.WriteString("Content-Type: " + contentType + "\r\n")
	}
	if len(body) > 0 {
		b.WriteString("Content-Length: " + itoa(len(body)) + "\r\n")
	}
	b.WriteString("\r\n")
	req := pendingReq{method: method, http: true, authRetry: afterAuth}
	if !afterAuth {
		req.replay = func() { s.sendAP2RTSPAttempt(method, uri, contentType, body, true) }
	}
	s.pending = append(s.pending, req)
	s.writeRTSP(append([]byte(b.String()), body...))
	s.timeout.arm(s.now())
}

func (s *Sender) sendAP2Info() {
	s.pairStage = pairAP2Info
	s.sendAP2RTSP("GET", "/info", "", nil)
}

func (s *Sender) sendAP2SetupSession() {
	setup := map[string]any{
		"deviceID":                 s.identity.DeviceID,
		"sessionUUID":              s.ap2.sessionUUID,
		"isMultiSelectAirPlay":     true,
		"groupContainsGroupLeader": false,
		"macAddress":               s.identity.DeviceID,
		"model":                    s.identity.Model,
		"name":                     s.identity.Name,
		"osBuildVersion":           "20F66",
		"osName":                   "iPhone OS",
		"osVersion":                "16.5",
		"senderSupportsRelay":      false,
		"sourceVersion":            "690.7.1",
		"statsCollectionEnabled":   false,
	}
	if s.groupUUID != "" {
		setup["groupUUID"] = s.groupUUID
	}
	if s.ptpClock != nil {
		peer := map[string]any{
			"ID":                                s.ap2.sessionUUID,
			"DeviceType":                        int64(0),
			"ClockID":                           int64(s.ptpClock.ClockID()),
			"SupportsClockPortMatchingOverride": false,
			"Addresses":                         []any{s.localIP},
		}
		setup["timingProtocol"] = "PTP"
		setup["timingPeerInfo"] = peer
		setup["timingPeerList"] = []any{peer}
	} else {
		setup["timingPort"] = int64(s.localTiming)
		setup["timingProtocol"] = "NTP"
	}
	body, err := crypto.Encode(setup)
	if err != nil {
		s.fail("Could not build the session SETUP plist")
		return
	}
	s.pairStage = pairAP2Session
	s.sendAP2RTSP("SETUP", s.rtspURI(), "application/x-apple-binary-plist", body)
	s.info("session SETUP sent (%d plist bytes)", len(body))
}

func (s *Sender) handleAP2SetupSession(body []byte) {
	eventPort := uint16(0)
	if root, err := crypto.Decode(body); err == nil {
		if m, ok := root.(map[string]any); ok {
			eventPort = uint16(plistInt(m, "eventPort"))
		}
	}
	s.ap2.eventPort = eventPort
	s.info("session SETUP ok, eventPort=%d", eventPort)
	if eventPort != 0 {
		s.io.TCPClose(raopio.TCPEvent, false)
		_ = s.io.TCPConnect(raopio.TCPEvent, s.hostIP, eventPort)
		s.info("opening event channel to %s:%d", s.host, eventPort)
	}
	s.sendAP2Record()
}

func (s *Sender) sendAP2Record() {
	s.pairStage = pairAP2Record
	s.sendAP2RTSP("RECORD", s.rtspURI(), "", nil)
}

// sendSetPeers announces the PTP peers before the stream SETUP. The receiver
// includes itself in the list; the sender's address identifies its clock.
func (s *Sender) sendSetPeers() {
	hosts := s.timingPeers
	if len(hosts) == 0 {
		hosts = []string{s.hostIP}
	}
	peers := make([]any, 0, len(hosts)+1)
	seen := make(map[string]bool, len(hosts)+1)
	for _, host := range append(append([]string(nil), hosts...), s.localIP) {
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		peers = append(peers, host)
	}
	body, err := crypto.Encode(peers)
	if err != nil {
		s.fail("Could not build the SETPEERS plist")
		return
	}
	s.pairStage = pairAP2SetPeers
	if s.ptpClock != nil {
		// SETPEERS is a no-op on a PTP session, and the receiver's clock is
		// already running. Activate the follower with the request instead of
		// waiting for the reply, so its three exchanges overlap the round trip.
		s.ptpClock.Activate()
	}
	s.sendAP2RTSP("SETPEERS", s.rtspURI(), "application/x-apple-binary-plist", body)
}

func (s *Sender) sendAP2SetupStream() {
	// HomePod clamps the shared secret to 32 bytes for the ChaCha key. A
	// password-paired Mac uses the Events-Write key (pyatv's macOS profile).
	// The family, not the pairing method, decides: a password-protected
	// HomePod keeps the HomePod key.
	var key []byte
	if s.family == device.FamilyMac && len(s.ap2.eventIn) == 32 {
		key = s.ap2.eventIn
	} else {
		key = s.ap2.sharedSecret
		if len(key) > 32 {
			key = key[:32]
		}
	}
	s.ap2.audioKey = append([]byte(nil), key...)
	s.ap2.audioNonce = 0
	s.ap2.encryptAudio = true

	// HomePod uses the configured latency at both ends of the range. macOS
	// advertises a wider range and rejects a stream whose latencyMin is below
	// 11025 samples, so it gets pyatv's verified range and a 1.5 s scheduling
	// latency.
	samples := int64(float64(s.latencyMS)*raopRate/1000 + 0.5)
	latencyMin, latencyMax := samples, samples
	if s.family == device.FamilyMac {
		latencyMin, latencyMax = 11025, 88200
		samples = 66150
	}
	s.nextLatencySamples = samples
	stream := map[string]any{
		"audioFormat":             int64(0x40000),
		"audioMode":               "default",
		"controlPort":             int64(s.localControl),
		"ct":                      int64(2),
		"isMedia":                 true,
		"latencyMax":              latencyMax,
		"latencyMin":              latencyMin,
		"shk":                     s.ap2.audioKey,
		"spf":                     int64(framesPerPacket),
		"sr":                      int64(raopRate),
		"type":                    int64(0x60),
		"supportsDynamicStreamID": false,
		"streamConnectionID":      int64(s.sessionID),
	}
	body, err := crypto.Encode(map[string]any{"streams": []any{stream}})
	if err != nil {
		s.fail("Could not build the stream SETUP plist")
		return
	}
	s.pairStage = pairAP2Stream
	s.sendAP2RTSP("SETUP", s.rtspURI(), "application/x-apple-binary-plist", body)
}

func (s *Sender) handleAP2SetupStream(body []byte) {
	s.serverPort, s.controlPort = 0, 0
	setup, ok := parseStreamSetup(body)
	if ok {
		s.serverPort = setup.DataPort
		s.controlPort = setup.ControlPort
		// The receiver reports its own output latency here. It is not the
		// sender's presentation latency and must not be added to the
		// configured latency.
		if setup.ReceiverLatencyMS > 0 {
			s.receiverLatencyMS = setup.ReceiverLatencyMS
			if s.events.ReceiverLatencyChanged != nil {
				s.events.ReceiverLatencyChanged(setup.ReceiverLatencyMS)
			}
		}
		if setup.WiFi.Valid {
			s.wifi = setup.WiFi
		}
	}
	if s.serverPort == 0 {
		s.fail("AirPlay 2 stream SETUP returned no data port")
		return
	}
	if s.controlPort == 0 {
		s.controlPort = s.serverPort
	}
	s.timingPort = s.serverPort
	s.info("stream SETUP ok, dataPort=%d controlPort=%d", s.serverPort, s.controlPort)
	s.readyToStream()
}

func plistInt(m map[string]any, key string) int64 {
	v, _ := plistIntOK(m, key)
	return v
}

// plistIntOK is plistInt with the presence of a usable value made explicit.
// A missing key or an unexpected type is unknown, not zero.
func plistIntOK(m map[string]any, key string) (int64, bool) {
	switch v := m[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case uint64:
		return int64(v), true
	case float64:
		return int64(v), true
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}
