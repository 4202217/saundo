package sender

import (
	"strings"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
)

// ntpNow returns 64-bit NTP wall time: seconds since 1900 in the high word and
// a 2^32-scaled fraction below.
func (s *Sender) ntpNow() uint64 { return ntpAt(s.now()) }

func (s *Sender) beginAuthChain() {
	s.setState(StatePairing)
	switch s.authMethod {
	case device.AuthNone, device.AuthPassword:
		// Password auth is reactive: the first 401 arms the Authorization
		// header.
		s.setState(StateHandshake)
		s.sendOptions()
	case device.AuthSetup:
		s.sendAuthSetup()
	case device.AuthLegacyPin:
		s.fail(fmtError("%s uses Apple FairPlay audio, which this app does not support.", s.name))
	case device.AuthHAPTransient:
		s.ap2 = &ap2State{sessionUUID: upperASCII(uuidV4Lower())}
		s.sendPairSetupM1()
	case device.AuthHAPPassword:
		if s.digestPassword == "" {
			s.fail(fmtError("%s requires the receiver's AirPlay password. Save it in the app's settings for this receiver, then connect again.", s.name))
			return
		}
		s.ap2 = &ap2State{sessionUUID: upperASCII(uuidV4Lower())}
		if s.resumeHAPCredentials() {
			return
		}
		s.sendPairSetupM1()
	case device.AuthHAPPIN:
		s.ap2 = &ap2State{sessionUUID: upperASCII(uuidV4Lower())}
		if s.resumeHAPCredentials() {
			return
		}
		s.sendPairPinStart()
	}
}

// resumeHAPCredentials loads stored HAP credentials and jumps straight to
// pair-verify when they are usable. It reports whether the resume path ran.
func (s *Sender) resumeHAPCredentials() bool {
	if s.credsJSON == "" {
		return false
	}
	if c, ok := creds.Parse(s.credsJSON); ok {
		s.ap2.ltSeed = c.LTSK
		s.ap2.pairingID = []byte(c.ClientID)
		s.ap2.accessoryID = c.ATVID
		s.ap2.accessoryLTPK = c.LTPK
		if len(s.ap2.ltSeed) == 32 && len(s.ap2.accessoryLTPK) > 0 {
			s.ap2.ltPub = crypto.Ed25519PublicFromSeed(s.ap2.ltSeed)
			s.sendPairVerifyM1()
			return true
		}
	}
	s.warn("stored credentials unusable, pairing afresh")
	s.ap2.ltSeed, s.ap2.pairingID = nil, nil
	s.ap2.accessoryID, s.ap2.accessoryLTPK = nil, nil
	return false
}

// sendAuthSetup performs the one-shot MFiSAP exchange. Its reply is ignored.
func (s *Sender) sendAuthSetup() {
	s.pairStage = pairAuthSetup
	pub := []byte{
		0x01,
		0x59, 0x02, 0xed, 0xe9, 0x0d, 0x4e, 0xf2, 0xbd,
		0x4c, 0xb6, 0x8a, 0x63, 0x30, 0x03, 0x82, 0x07,
		0xa9, 0x4d, 0xbd, 0x50, 0xd8, 0xaa, 0x46, 0x5b,
		0x5d, 0x8c, 0x01, 0x2a, 0x0c, 0x7e, 0x1d, 0x4e,
	}
	s.httpPost("/auth-setup", "application/octet-stream", pub)
}

// httpPost sends one HTTP request on the RTSP socket. Receivers parse both
// request lines on the same connection. The reply is routed to
// onPairingResponse because it carries a TLV or plist body.
func (s *Sender) httpPost(uri, contentType string, body []byte) {
	var b strings.Builder
	b.WriteString("POST " + uri + " HTTP/1.1\r\n")
	b.WriteString("CSeq: " + itoa(s.cseq) + "\r\n")
	s.cseq++
	b.WriteString("User-Agent: " + clientUserAgent + "\r\n")
	b.WriteString("Connection: keep-alive\r\n")
	hkp := "3"
	if s.family == device.FamilyHomePod || s.authMethod == device.AuthHAPTransient {
		hkp = "4"
	}
	b.WriteString("X-Apple-HKP: " + hkp + "\r\n")
	b.WriteString("DACP-ID: " + s.dacpID + "\r\n")
	b.WriteString("Active-Remote: " + utoa(uint64(s.activeRemote)) + "\r\n")
	b.WriteString("Client-Instance: " + s.dacpID + "\r\n")
	b.WriteString("X-Apple-Client-Name: " + s.identity.Name + "\r\n")
	if contentType != "" {
		b.WriteString("Content-Type: " + contentType + "\r\n")
	}
	b.WriteString("Content-Length: " + itoa(len(body)) + "\r\n\r\n")
	s.pending = append(s.pending, pendingReq{method: "POST", http: true})
	s.writeRTSP(append([]byte(b.String()), body...))
	if !s.waitingForPin {
		s.timeout.arm(s.now())
	}
}

func (s *Sender) sendPairPinStart() {
	s.pairStage = pairPinStart
	s.httpPost("/pair-pin-start", "application/octet-stream", nil)
}

func (s *Sender) sendPairSetupM1() {
	items := []crypto.TLV{
		{Type: crypto.TLVMethod, Value: []byte{0x00}},
		{Type: crypto.TLVState, Value: []byte{0x01}},
	}
	if s.authMethod == device.AuthHAPTransient {
		items = append(items, crypto.TLV{Type: crypto.TLVFlags, Value: []byte{0x10}})
	}
	s.pairStage = pairSetupM2
	s.httpPost("/pair-setup", "application/octet-stream", crypto.EncodeTLV(items))
}

func (s *Sender) sendPairSetupM3(pin string) {
	s.ap2.srp = crypto.NewSRPClient()
	s.ap2.srp.Start(pin)
	if !s.ap2.srp.Process(s.srpSalt, s.srpServerB) {
		s.fail("Pairing rejected the device's parameters")
		return
	}
	items := []crypto.TLV{
		{Type: crypto.TLVState, Value: []byte{0x03}},
		{Type: crypto.TLVPublicKey, Value: s.ap2.srp.PublicA()},
		{Type: crypto.TLVProof, Value: s.ap2.srp.ProofM1()},
	}
	s.pairStage = pairSetupM4
	s.httpPost("/pair-setup", "application/octet-stream", crypto.EncodeTLV(items))
}

func (s *Sender) onPairingResponse(code int, headers map[string]string, body string) {
	if s.state == StateIdle {
		return
	}
	if s.volQuery && s.state == StateStreaming {
		// A streaming-time volume poll reply, not a pairing-stage reply.
		s.handleVolumeReply([]byte(body))
		return
	}
	if s.state == StateStreaming {
		s.info("ignoring late pairing reply (%d) while streaming", code)
		return
	}
	if s.authMethod == device.AuthSetup && s.ap2 == nil && s.pairStage == pairAuthSetup {
		s.pairStage = pairDone
		s.afterAuthOK()
		return
	}
	if s.pairStage == pairAP2Record {
		s.info("AP2 RECORD reply (%d), proceeding to stream SETUP", code)
		if s.ptpClock != nil {
			// SETPEERS is a no-op on a PTP session, and the receiver can take
			// hundreds of milliseconds to answer it. Activate the follower now
			// and let the PTP readiness gate the stream SETUP directly.
			s.ptpClock.Activate()
			s.pairStage = pairAP2WaitPTP
			s.ptpWaitUntil = s.now().Add(30 * time.Second)
			s.timeout.arm(s.now())
			return
		}
		s.sendAP2SetupStream()
		return
	}

	if code < 200 || code >= 300 {
		if code == 470 && s.authMethod == device.AuthHAPTransient && s.pairStage == pairSetupM2 {
			s.info("%q refused transient pairing (470), falling back to on-screen PIN", s.name)
			s.authMethod = device.AuthHAPPIN
			s.ap2 = &ap2State{sessionUUID: upperASCII(uuidV4Lower())}
			s.sendPairPinStart()
			return
		}
		if code == 403 && s.pairStage == pairPinStart && !s.triedTransientOnPin403 {
			s.info("%q refused /pair-pin-start (403), trying transient pairing", s.name)
			s.triedTransientOnPin403 = true
			s.authMethod = device.AuthHAPTransient
			s.ap2 = &ap2State{sessionUUID: upperASCII(uuidV4Lower())}
			s.sendPairSetupM1()
			return
		}
		if code == 403 {
			if s.family == device.FamilyHomePod {
				s.fail(fmtError("%s refused pairing. In the Home app, allow AirPlay for this network and check the saved AirPlay password.", s.name))
			} else {
				s.fail(fmtError("%s refused pairing. On a Mac, set AirDrop & Handoff > AirPlay Receiver > Allow AirPlay for Everyone and turn off Require Password. On an Apple TV, set AirPlay & HomeKit > Allow Access to Anyone on the Same Network. A HomePod needs to be set up in the Home app first.", s.name))
			}
		} else if code == 470 {
			s.fail(fmtError("%s needs to be paired with a PIN, but it didn't show one. Set AirPlay access to Anyone on the Same Network and try again.", s.name))
		} else {
			s.fail(fmtError("Pairing with %s failed (HTTP %d)", s.name, code))
		}
		return
	}

	switch s.pairStage {
	case pairPinStart:
		s.info("/pair-pin-start OK, receiver should show its code")
		s.sendPairSetupM1()
	case pairSetupM2:
		s.handlePairSetupM2([]byte(body))
	case pairSetupM4:
		s.handlePairSetupM4([]byte(body))
	case pairSetupM6:
		s.handlePairSetupM6([]byte(body))
	case pairVerifyM2:
		s.handlePairVerifyM2([]byte(body))
	case pairVerifyDone:
		s.info("pair-verify complete")
		s.pairStage = pairDone
		s.afterAuthOK()
	case pairAP2Info:
		if root, err := crypto.Decode([]byte(body)); err == nil {
			if m, ok := root.(map[string]any); ok {
				if _, present := m["initialVolume"]; present {
					s.ap2.initialVol = float64(plistInt(m, "initialVolume"))
					s.ap2.hasInitial = true
				}
			}
		}
		s.info("GET /info ok, starting session SETUP")
		s.sendAP2SetupSession()
	case pairAP2Session:
		s.handleAP2SetupSession([]byte(body))
	case pairAP2SetPeers:
		if s.ptpClock != nil {
			s.ptpClock.Activate()
			s.pairStage = pairAP2WaitPTP
			s.ptpWaitUntil = s.now().Add(30 * time.Second)
			s.timeout.arm(s.now())
			s.info("SETPEERS reply (%d), waiting for the PTP clock", code)
			return
		}
		s.sendAP2SetupStream()
	case pairAP2Stream:
		s.handleAP2SetupStream([]byte(body))
	default:
		s.warn("pairing reply at unexpected stage %d", s.pairStage)
	}
}

func tlvError(items []crypto.TLV) (byte, bool) {
	v, ok := crypto.GetTLV(items, crypto.TLVError)
	if !ok {
		return 0, false
	}
	if len(v) == 0 {
		return 0, true
	}
	return v[0], true
}

func (s *Sender) handlePairSetupM2(body []byte) {
	items, err := crypto.DecodeTLV(body)
	if err != nil {
		s.fail("Pairing setup response was malformed")
		return
	}
	if code, ok := tlvError(items); ok {
		s.fail(fmtError("The device rejected pairing (error %d)", code))
		return
	}
	salt, ok1 := crypto.GetTLV(items, crypto.TLVSalt)
	pub, ok2 := crypto.GetTLV(items, crypto.TLVPublicKey)
	if !ok1 || !ok2 {
		s.fail("Pairing setup response was incomplete")
		return
	}
	s.srpSalt = append([]byte(nil), salt...)
	s.srpServerB = append([]byte(nil), pub...)
	switch s.authMethod {
	case device.AuthHAPTransient:
		s.sendPairSetupM3("3939")
	case device.AuthHAPPassword:
		// The stored receiver password is the SRP secret; no PIN round-trip.
		s.sendPairSetupM3(s.digestPassword)
	default:
		s.waitingForPin = true
		s.timeout.disarm()
		s.pinTimer.arm(s.now())
		s.info("HAP PIN pairing, waiting for code")
		if s.events.PinRequired != nil {
			s.events.PinRequired(s.name)
		}
	}
}

func (s *Sender) handlePairSetupM4(body []byte) {
	items, err := crypto.DecodeTLV(body)
	if err != nil {
		s.fail("Pairing finalisation response was malformed")
		return
	}
	if code, ok := tlvError(items); ok {
		if s.authMethod == device.AuthHAPPassword {
			s.fail(fmtError("%s rejected the AirPlay password (error %d). Check the receiver's password in the app's settings.", s.name, code))
		} else {
			s.fail(fmtError("Pairing PIN was not accepted (error %d)", code))
		}
		return
	}
	proof, ok := crypto.GetTLV(items, crypto.TLVProof)
	if !ok || !s.ap2.srp.VerifyServerProof(proof) {
		if s.strictAuth {
			if !ok {
				s.fail("The device sent no pairing proof (receiver not authenticated)")
			} else {
				s.fail("The device's pairing proof did not verify (receiver not authenticated)")
			}
			return
		}
		s.warn("server pairing proof did not verify, continuing")
	}
	if s.authMethod == device.AuthHAPTransient {
		s.ap2.sharedSecret = s.ap2.srp.SessionKey()
		s.deriveControlKeys()
		s.deriveEventKeys()
		s.pairStage = pairDone
		s.afterAuthOK()
		return
	}
	s.sendPairSetupM5()
}

func (s *Sender) sendPairSetupM5() {
	if len(s.ap2.ltSeed) == 0 {
		s.ap2.ltSeed = crypto.RandomBytes(32)
	}
	s.ap2.ltPub = crypto.Ed25519PublicFromSeed(s.ap2.ltSeed)
	if len(s.ap2.pairingID) == 0 {
		s.ap2.pairingID = []byte(uuidV4Lower())
	}
	K := s.ap2.srp.SessionKey()
	sessionKey := crypto.HKDF("Pair-Setup-Encrypt-Salt", "Pair-Setup-Encrypt-Info", K, 32)
	controllerX := crypto.HKDF("Pair-Setup-Controller-Sign-Salt", "Pair-Setup-Controller-Sign-Info", K, 32)
	info := append(append(append([]byte{}, controllerX...), s.ap2.pairingID...), s.ap2.ltPub...)
	sig := crypto.Ed25519Sign(s.ap2.ltSeed, info)

	inner := crypto.EncodeTLV([]crypto.TLV{
		{Type: crypto.TLVIdentifier, Value: s.ap2.pairingID},
		{Type: crypto.TLVPublicKey, Value: s.ap2.ltPub},
		{Type: crypto.TLVSignature, Value: sig},
	})
	enc, err := crypto.ChaCha20Poly1305Encrypt(sessionKey, []byte("PS-Msg05"), inner, nil)
	if err != nil {
		s.fail("Pairing encryption failed")
		return
	}
	s.pairSetupSessionKey = sessionKey
	s.pairStage = pairSetupM6
	s.httpPost("/pair-setup", "application/octet-stream", crypto.EncodeTLV([]crypto.TLV{
		{Type: crypto.TLVState, Value: []byte{0x05}},
		{Type: crypto.TLVEncryptedData, Value: enc},
	}))
}

func (s *Sender) handlePairSetupM6(body []byte) {
	items, err := crypto.DecodeTLV(body)
	if err != nil {
		s.fail("Pairing finalisation response was malformed")
		return
	}
	if code, ok := tlvError(items); ok {
		s.fail(fmtError("Pairing finalisation failed (error %d)", code))
		return
	}
	enc, ok := crypto.GetTLV(items, crypto.TLVEncryptedData)
	if !ok {
		s.fail("Pairing M6 response was incomplete")
		return
	}
	plain, err := crypto.ChaCha20Poly1305Decrypt(s.pairSetupSessionKey, []byte("PS-Msg06"), enc, nil)
	if err != nil {
		s.fail("Pairing M6 could not be decrypted")
		return
	}
	sub, err := crypto.DecodeTLV(plain)
	if err != nil {
		s.fail("Pairing M6 was malformed")
		return
	}
	atvID, ok1 := crypto.GetTLV(sub, crypto.TLVIdentifier)
	atvLTPK, ok2 := crypto.GetTLV(sub, crypto.TLVPublicKey)
	if !ok1 || !ok2 {
		s.fail("Pairing M6 was missing device keys")
		return
	}
	s.ap2.accessoryID = append([]byte(nil), atvID...)
	s.ap2.accessoryLTPK = append([]byte(nil), atvLTPK...)

	text := creds.Marshal(creds.Credentials{
		ATVID:    atvID,
		ClientID: string(s.ap2.pairingID),
		LTPK:     atvLTPK,
		LTSK:     s.ap2.ltSeed,
	})
	if s.events.CredentialsObtained != nil {
		s.events.CredentialsObtained(s.deviceID, text)
	}
	s.info("HAP pairing complete, credentials stored")
	s.pairSetupSessionKey = nil
	s.sendPairVerifyM1()
}

func (s *Sender) sendPairVerifyM1() {
	pub, priv, err := crypto.X25519Generate()
	if err != nil {
		s.fail("Pair-verify key generation failed")
		return
	}
	s.ap2.verifyPub, s.ap2.verifyPriv = pub, priv
	s.pairStage = pairVerifyM2
	s.httpPost("/pair-verify", "application/octet-stream", crypto.EncodeTLV([]crypto.TLV{
		{Type: crypto.TLVState, Value: []byte{0x01}},
		{Type: crypto.TLVPublicKey, Value: pub},
	}))
}

func (s *Sender) handlePairVerifyM2(body []byte) {
	items, err := crypto.DecodeTLV(body)
	if err != nil {
		s.fail("Pair-verify response was malformed")
		return
	}
	if code, ok := tlvError(items); ok {
		if !s.triedRepairOnVerifyAuthError && s.credsJSON != "" {
			// The receiver no longer knows this pairing — it was reset or
			// unpaired on its side. Drop the stored credentials and pair
			// afresh on the same connection, mirroring the 470/403
			// cross-fallbacks. Retrying the stored pairing would fail
			// identically until the user hand-deleted credentials.dat.
			s.info("%q rejected the stored pairing (pair-verify error %d), pairing afresh", s.name, code)
			s.triedRepairOnVerifyAuthError = true
			s.credsJSON = ""
			s.ap2 = &ap2State{sessionUUID: upperASCII(uuidV4Lower())}
			s.pairSetupSessionKey = nil
			if s.authMethod == device.AuthHAPPIN {
				s.sendPairPinStart()
			} else {
				s.sendPairSetupM1()
			}
			return
		}
		s.fail(fmtError("Pair-verify rejected (error %d)", code))
		return
	}
	sessionPub, ok1 := crypto.GetTLV(items, crypto.TLVPublicKey)
	encrypted, ok2 := crypto.GetTLV(items, crypto.TLVEncryptedData)
	if !ok1 || !ok2 {
		s.fail("Pair-verify response was incomplete")
		return
	}
	shared, err := crypto.X25519SharedSecret(s.ap2.verifyPriv, sessionPub)
	if err != nil {
		s.fail("Pair-verify shared-secret derivation failed")
		return
	}
	s.ap2.sharedSecret = shared
	verifyKey := crypto.HKDF("Pair-Verify-Encrypt-Salt", "Pair-Verify-Encrypt-Info", shared, 32)
	plain, err := crypto.ChaCha20Poly1305Decrypt(verifyKey, []byte("PV-Msg02"), encrypted, nil)
	if err != nil {
		s.fail("Pair-verify could not be decrypted")
		return
	}
	sub, err := crypto.DecodeTLV(plain)
	if err != nil {
		s.fail("Pair-verify payload was malformed")
		return
	}
	atvID, ok1 := crypto.GetTLV(sub, crypto.TLVIdentifier)
	atvSig, ok2 := crypto.GetTLV(sub, crypto.TLVSignature)
	if !ok1 || !ok2 {
		s.fail("Pair-verify was missing the device signature")
		return
	}
	verified := false
	if len(s.ap2.accessoryLTPK) > 0 {
		info := append(append(append([]byte{}, sessionPub...), atvID...), s.ap2.verifyPub...)
		verified = crypto.Ed25519Verify(s.ap2.accessoryLTPK, info, atvSig)
	}
	if !verified {
		if s.strictAuth {
			s.fail("The device's pair-verify signature did not verify (receiver not authenticated)")
			return
		}
		s.warn("pair-verify signature did not verify, continuing")
	}

	info := append(append(append([]byte{}, s.ap2.verifyPub...), s.ap2.pairingID...), sessionPub...)
	sig := crypto.Ed25519Sign(s.ap2.ltSeed, info)
	inner := crypto.EncodeTLV([]crypto.TLV{
		{Type: crypto.TLVIdentifier, Value: s.ap2.pairingID},
		{Type: crypto.TLVSignature, Value: sig},
	})
	enc, err := crypto.ChaCha20Poly1305Encrypt(verifyKey, []byte("PV-Msg03"), inner, nil)
	if err != nil {
		s.fail("Pair-verify encryption failed")
		return
	}
	s.deriveControlKeys()
	s.deriveEventKeys()
	s.pairStage = pairVerifyDone
	s.httpPost("/pair-verify", "application/octet-stream", crypto.EncodeTLV([]crypto.TLV{
		{Type: crypto.TLVState, Value: []byte{0x03}},
		{Type: crypto.TLVEncryptedData, Value: enc},
	}))
}

func (s *Sender) deriveControlKeys() {
	s.ap2.controlOut = crypto.HKDF("Control-Salt", "Control-Write-Encryption-Key", s.ap2.sharedSecret, 32)
	s.ap2.controlIn = crypto.HKDF("Control-Salt", "Control-Read-Encryption-Key", s.ap2.sharedSecret, 32)
}

// deriveEventKeys keys the reverse event channel. The receiver writes with
// Events-Write, so that key decrypts, and our replies use Events-Read.
func (s *Sender) deriveEventKeys() {
	s.ap2.eventIn = crypto.HKDF("Events-Salt", "Events-Write-Encryption-Key", s.ap2.sharedSecret, 32)
	s.ap2.eventOut = crypto.HKDF("Events-Salt", "Events-Read-Encryption-Key", s.ap2.sharedSecret, 32)
}

func (s *Sender) afterAuthOK() {
	s.digestRetried = false
	if s.airplay2 {
		if s.ap2 != nil && len(s.ap2.controlOut) == 32 && len(s.ap2.controlIn) == 32 {
			s.controlEncrypted = true
			s.ctrlSendCtr, s.ctrlRecvCtr = 0, 0
			s.rtspEncBuf = s.rtspEncBuf[:0]
			s.info("control channel now encrypted")
		}
		s.setState(StateHandshake)
		s.sendAP2Info()
		return
	}
	s.setState(StateHandshake)
	s.sendOptions()
}
