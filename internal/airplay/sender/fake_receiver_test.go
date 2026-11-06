package sender

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
)

type rxMode int

const (
	modeAP1 rxMode = iota
	modeTransient
	modePIN
	modeVerify
	modePassword
)

// testReceiverPassword is the SRP secret the fake receiver accepts in
// password mode.
const testReceiverPassword = "sesame"

type storedPairing struct {
	clientID      []byte
	clientLTPK    []byte
	accessoryID   []byte
	accessoryLTPK []byte
	accessorySeed []byte
}

type launchResult struct {
	ok  bool
	err string
}

type rig struct {
	t            *testing.T
	io           *fakeIO
	sender       *Sender
	rx           *fakeReceiver
	now          time.Time
	events       []raopio.Event
	launched     []launchResult
	closed       int
	pins         []string
	creds        []string
	states       []State
	volumeEvents []float64
	logs         []string
}

func newRig(t *testing.T, mode rxMode, auth device.Auth, airplay2 bool, stored *storedPairing) *rig {
	t.Helper()
	r := &rig{t: t, now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	r.io = &fakeIO{r: r}
	pin := "1234"
	switch mode {
	case modeTransient:
		pin = "3939"
	case modePassword:
		pin = testReceiverPassword
	}
	r.rx = &fakeReceiver{r: r, mode: mode, pin: pin, requests: map[string]int{}, receiverVolume: -27}
	if stored != nil {
		r.rx.clientID = stored.clientID
		r.rx.clientLTPK = stored.clientLTPK
		r.rx.accessoryID = stored.accessoryID
		r.rx.accessoryLTPK = stored.accessoryLTPK
		r.rx.accessorySeed = stored.accessorySeed
	}
	r.sender = New(r.io, Events{
		Launched:            func(ok bool, err string) { r.launched = append(r.launched, launchResult{ok, err}) },
		Closed:              func() { r.closed++ },
		PinRequired:         func(name string) { r.pins = append(r.pins, name) },
		CredentialsObtained: func(deviceID, credsJSON string) { r.creds = append(r.creds, credsJSON) },
		StateChanged:        func(s State) { r.states = append(r.states, s) },
		VolumeChanged:       func(p float64) { r.volumeEvents = append(r.volumeEvents, p) },
	}, func(level Level, format string, args ...any) {
		r.logs = append(r.logs, fmt.Sprintf(format, args...))
		t.Logf("[%d] "+format, append([]any{level}, args...)...)
	})
	r.sender.SetClock(func() time.Time { return r.now })
	r.sender.SetInputFormat(44100)
	r.sender.SetAuth(auth, airplay2, "AA:BB:CC:DD:EE:FF", "", "")
	return r
}

func (r *rig) emit(ev raopio.Event) {
	r.events = append(r.events, ev)
}

func (r *rig) step() {
	for len(r.events) > 0 {
		ev := r.events[0]
		r.events = r.events[1:]
		r.sender.HandleEvent(ev)
	}
}

// advance moves the fake clock through every due timer up to d from now.
func (r *rig) advance(d time.Duration) {
	end := r.now.Add(d)
	for {
		r.step()
		dl := r.sender.NextDeadline()
		if dl.IsZero() || dl.After(end) {
			break
		}
		if !dl.After(r.now) {
			r.now = r.now.Add(time.Millisecond)
		} else {
			r.now = dl
		}
		r.sender.Tick()
	}
	r.now = end
	r.step()
}

func (r *rig) start() {
	r.sender.Start("192.168.1.20", 7000, "FakeBox")
	r.step()
	// A plain Tick at the current fake time seals the start gate and fires
	// Launched without sending audio, so tests control the first packet.
	r.sender.Tick()
	r.step()
}

type fakeIO struct{ r *rig }

func (f *fakeIO) TCPConnect(ch raopio.TCPChannel, host string, port uint16) error {
	f.r.emit(raopio.Event{
		Kind: raopio.EventTCPConnected, TCP: ch,
		Local: raopio.Endpoint{IP: "192.168.1.10", Port: 40000},
		Peer:  raopio.Endpoint{IP: "192.168.1.20", Port: port},
	})
	return nil
}

func (f *fakeIO) TCPSend(ch raopio.TCPChannel, data []byte) error {
	switch ch {
	case raopio.TCPControl:
		f.r.rx.handleControl(data)
	default:
		f.r.rx.handleEventResponse(data)
	}
	return nil
}

func (f *fakeIO) TCPClose(raopio.TCPChannel, bool) {}

func (f *fakeIO) UDPBind(ch raopio.UDPChannel) (uint16, error) {
	return 5000 + uint16(ch), nil
}

func (f *fakeIO) UDPSend(ch raopio.UDPChannel, to raopio.Endpoint, data []byte) error {
	switch ch {
	case raopio.UDPAudio:
		f.r.rx.handleAudio(data)
	case raopio.UDPControl:
		f.r.rx.syncPackets = append(f.r.rx.syncPackets, append([]byte(nil), data...))
	case raopio.UDPTiming:
		f.r.rx.timingResponses = append(f.r.rx.timingResponses, append([]byte(nil), data...))
	}
	return nil
}

func (f *fakeIO) UDPClose(raopio.UDPChannel) {}

type request struct {
	method, uri, proto string
	headers            map[string]string
	body               []byte
}

func parseRequests(buf *[]byte) []request {
	var out []request
	for {
		idx := bytes.Index(*buf, []byte("\r\n\r\n"))
		if idx < 0 {
			return out
		}
		lines := strings.Split(string((*buf)[:idx]), "\r\n")
		headers := map[string]string{}
		for _, line := range lines[1:] {
			colon := strings.IndexByte(line, ':')
			if colon <= 0 {
				continue
			}
			headers[strings.ToLower(strings.TrimSpace(line[:colon]))] = strings.TrimSpace(line[colon+1:])
		}
		n := 0
		if v, ok := headers["content-length"]; ok {
			n, _ = strconv.Atoi(v)
		}
		total := idx + 4 + n
		if len(*buf) < total {
			return out
		}
		body := append([]byte(nil), (*buf)[idx+4:total]...)
		parts := strings.Split(lines[0], " ")
		req := request{headers: headers, body: body}
		if len(parts) > 0 {
			req.method = parts[0]
		}
		if len(parts) > 1 {
			req.uri = parts[1]
		}
		if len(parts) > 2 {
			req.proto = parts[2]
		}
		out = append(out, req)
		*buf = (*buf)[total:]
	}
}

type fakeReceiver struct {
	r    *rig
	mode rxMode
	pin  string

	rawBuf    []byte
	encrypted bool
	readKey   []byte
	writeKey  []byte
	readCtr   uint64
	writeCtr  uint64
	encBuf    []byte
	reqBuf    []byte

	srp          *srpServer
	sharedSecret []byte
	pairSetupKey []byte
	clientID     []byte
	clientLTPK   []byte

	accessorySeed []byte
	accessoryLTPK []byte
	accessoryID   []byte

	verifyPriv    []byte
	verifyPub     []byte
	verifyKey     []byte
	lastClientPub []byte

	// rejectVerifyWithTLV makes the next pair-verify M2 fail with this TLV
	// error code — a receiver that forgot the pairing. One-shot: after the
	// sender re-pairs, the receiver accepts it.
	rejectVerifyWithTLV byte

	eventReadKey  []byte
	eventWriteKey []byte
	eventReadCtr  uint64
	eventWriteCtr uint64
	eventEncBuf   []byte
	eventPlain    []byte
	eventResp     [][]byte
	eventOpen     bool

	audioKey    []byte
	audioNonce  uint64
	audioFrames []int16
	audioSeqs   []uint16
	audioTS     []uint32
	audioMarker []byte
	audioPkts   [][]byte
	ap2Audio    bool

	syncPackets       [][]byte
	timingResponses   [][]byte
	retransmitResps   [][]byte
	feedbacks         int
	receiverVolume    int64
	requests          map[string]int
	setParams         []string
	sessionSetup      map[string]any
	streamSetup       map[string]any
	setPeersBody      []byte
	encryptAfterReply bool
	failAll           bool

	digestChallenge  bool
	digestNonce      string
	digestAuth       string
	digestChallenges int
	order            []string
}

func (f *fakeReceiver) decryptFrames(key []byte, ctr *uint64, buf *[]byte, dst *[]byte, note string) {
	for len(*buf) >= 2 {
		n := int(binary.LittleEndian.Uint16((*buf)[:2]))
		need := 2 + n + 16
		if len(*buf) < need {
			return
		}
		nonce := crypto.CounterNonce8(*ctr)
		*ctr++
		plain, err := crypto.ChaCha20Poly1305Decrypt(key, nonce, (*buf)[2:need], (*buf)[:2])
		if err != nil {
			f.r.t.Fatalf("%s: decrypt frame failed: %v", note, err)
		}
		*dst = append(*dst, plain...)
		*buf = (*buf)[need:]
	}
}

func (f *fakeReceiver) handleControl(data []byte) {
	if f.encrypted {
		f.encBuf = append(f.encBuf, data...)
		f.decryptFrames(f.readKey, &f.readCtr, &f.encBuf, &f.reqBuf, "control")
	} else {
		f.reqBuf = append(f.reqBuf, data...)
	}
	for _, req := range parseRequests(&f.reqBuf) {
		key := req.method + " " + req.uri
		f.requests[key]++
		f.order = append(f.order, key)
		f.dispatch(req)
	}
}

func (f *fakeReceiver) reply(req request, code int, contentType string, body []byte, extra ...header) {
	if f.failAll {
		return
	}
	proto := "RTSP/1.0"
	if req.proto == "HTTP/1.1" {
		proto = "HTTP/1.1"
	}
	var b strings.Builder
	b.WriteString(proto + " " + itoa(code) + " OK\r\n")
	if cseq := req.headers["cseq"]; cseq != "" {
		b.WriteString("CSeq: " + cseq + "\r\n")
	}
	for _, h := range extra {
		b.WriteString(h.key + ": " + h.value + "\r\n")
	}
	if contentType != "" {
		b.WriteString("Content-Type: " + contentType + "\r\n")
	}
	b.WriteString("Content-Length: " + itoa(len(body)) + "\r\n\r\n")
	out := append([]byte(b.String()), body...)
	if f.encrypted {
		var framed []byte
		for off := 0; off < len(out); {
			n := len(out) - off
			if n > 1024 {
				n = 1024
			}
			lp := []byte{byte(n), byte(n >> 8)}
			nonce := crypto.CounterNonce8(f.writeCtr)
			f.writeCtr++
			ct, err := crypto.ChaCha20Poly1305Encrypt(f.writeKey, nonce, out[off:off+n], lp)
			if err != nil {
				f.r.t.Fatalf("encrypt reply: %v", err)
			}
			framed = append(append(framed, lp...), ct...)
			off += n
		}
		out = framed
	}
	f.r.emit(raopio.Event{Kind: raopio.EventTCPData, TCP: raopio.TCPControl, Data: out})
	if f.encryptAfterReply {
		f.encrypted = true
		f.encryptAfterReply = false
	}
}

func (f *fakeReceiver) dispatch(req request) {
	key := req.method + " " + req.uri
	switch {
	case req.method == "POST" && req.uri == "/pair-pin-start":
		f.reply(req, 200, "application/octet-stream", nil)
	case req.method == "POST" && req.uri == "/pair-setup":
		f.pairSetup(req)
	case req.method == "POST" && req.uri == "/pair-verify":
		f.pairVerify(req)
	case req.method == "GET" && req.uri == "/info":
		body, _ := crypto.Encode(map[string]any{"initialVolume": f.receiverVolume})
		f.reply(req, 200, "application/x-apple-binary-plist", body)
	case req.method == "SETUP":
		if len(req.body) == 0 {
			f.reply(req, 200, "", nil,
				header{"Transport", "RTP/AVP/UDP;unicast;mode=record;server_port=6000;control_port=6001;timing_port=6002"},
				header{"Session", "1"})
			return
		}
		root, err := crypto.Decode(req.body)
		if err != nil {
			f.r.t.Fatalf("SETUP body is not a plist: %v", err)
		}
		m, _ := root.(map[string]any)
		if _, ok := m["streams"]; ok {
			f.streamSetup = m
			streams := m["streams"].([]any)
			stream := streams[0].(map[string]any)
			f.audioKey = append([]byte(nil), stream["shk"].([]byte)...)
			f.ap2Audio = true
			body, _ := crypto.Encode(map[string]any{"streams": []any{map[string]any{
				"dataPort": int64(6000), "controlPort": int64(6001), "type": int64(96),
			}}})
			f.reply(req, 200, "application/x-apple-binary-plist", body)
			return
		}
		if f.digestChallenge && f.sharedSecret != nil && !f.digestAuthOK(req) {
			f.digestChallenges++
			f.reply(req, 401, "", nil,
				header{"WWW-Authenticate", `Digest realm="airplay", nonce="` + f.digestNonce + `"`})
			return
		}
		f.sessionSetup = m
		body, _ := crypto.Encode(map[string]any{"eventPort": int64(7000)})
		f.reply(req, 200, "application/x-apple-binary-plist", body)
	case req.method == "RECORD":
		f.reply(req, 200, "", nil)
	case req.method == "SETPEERS":
		f.setPeersBody = append([]byte(nil), req.body...)
		f.reply(req, 200, "", nil)
	case req.method == "SET_PARAMETER":
		f.setParams = append(f.setParams, req.headers["content-type"]+"\x00"+string(req.body))
		f.reply(req, 200, "", nil)
	case req.method == "POST" && req.uri == "/feedback":
		f.feedbacks++
		f.reply(req, 200, "", nil)
	case req.method == "OPTIONS":
		f.reply(req, 200, "", nil)
	case req.method == "ANNOUNCE":
		f.reply(req, 200, "", nil)
	case req.method == "TEARDOWN":
		f.reply(req, 200, "", nil)
	case req.method == "FLUSH":
		f.reply(req, 200, "", nil)
	default:
		f.r.t.Fatalf("fake receiver: unhandled request %s", key)
	}
}

// digestAuthOK checks the RFC 2069 response against the fake receiver's
// password and records the header for assertions.
func (f *fakeReceiver) digestAuthOK(req request) bool {
	auth := req.headers["authorization"]
	if auth == "" {
		return false
	}
	f.digestAuth = auth
	return auth == crypto.DigestAuthResponse(req.method, req.uri, "AirPlay", "airplay", f.pin, f.digestNonce)
}

func bodyHasStreams(body []byte) bool {
	root, err := crypto.Decode(body)
	if err != nil {
		return false
	}
	m, ok := root.(map[string]any)
	if !ok {
		return false
	}
	_, ok = m["streams"]
	return ok
}

func (f *fakeReceiver) pairSetup(req request) {
	items, err := crypto.DecodeTLV(req.body)
	if err != nil {
		f.r.t.Fatalf("pair-setup TLV: %v", err)
	}
	state, _ := crypto.GetTLV(items, crypto.TLVState)
	switch {
	case len(state) == 1 && state[0] == 1:
		if f.mode == modePassword {
			if m, ok := crypto.GetTLV(items, crypto.TLVMethod); !ok || len(m) != 1 || m[0] != 0 {
				f.r.t.Fatalf("password M1 method = %v, want 0", m)
			}
			if fl, ok := crypto.GetTLV(items, crypto.TLVFlags); ok {
				f.r.t.Fatalf("password M1 carried Flags %x; that is the transient path", fl)
			}
		}
		f.srp = newSRPServer(f.pin)
		f.reply(req, 200, "application/octet-stream", crypto.EncodeTLV([]crypto.TLV{
			{Type: crypto.TLVState, Value: []byte{2}},
			{Type: crypto.TLVSalt, Value: f.srp.salt},
			{Type: crypto.TLVPublicKey, Value: f.srp.B.Bytes()},
		}))
	case len(state) == 1 && state[0] == 3:
		a, _ := crypto.GetTLV(items, crypto.TLVPublicKey)
		proof, _ := crypto.GetTLV(items, crypto.TLVProof)
		if !f.srp.process(a, proof) {
			f.reply(req, 200, "application/octet-stream", crypto.EncodeTLV([]crypto.TLV{
				{Type: crypto.TLVState, Value: []byte{4}},
				{Type: crypto.TLVError, Value: []byte{2}},
			}))
			return
		}
		resp := crypto.EncodeTLV([]crypto.TLV{
			{Type: crypto.TLVState, Value: []byte{4}},
			{Type: crypto.TLVProof, Value: f.srp.serverM2},
		})
		transient := f.mode == modeTransient
		if transient {
			f.sharedSecret = f.srp.K
			f.deriveKeys()
		}
		f.reply(req, 200, "application/octet-stream", resp)
		if transient {
			// M4's reply is plaintext; the client encrypts everything after it.
			f.encrypted = true
		}
	case len(state) == 1 && state[0] == 5:
		f.pairSetupM5(req, items)
	default:
		f.r.t.Fatalf("pair-setup unexpected state %v", state)
	}
}

func (f *fakeReceiver) pairSetupM5(req request, items []crypto.TLV) {
	enc, _ := crypto.GetTLV(items, crypto.TLVEncryptedData)
	f.pairSetupKey = crypto.HKDF("Pair-Setup-Encrypt-Salt", "Pair-Setup-Encrypt-Info", f.srp.K, 32)
	plain, err := crypto.ChaCha20Poly1305Decrypt(f.pairSetupKey, []byte("PS-Msg05"), enc, nil)
	if err != nil {
		f.r.t.Fatalf("M5 decrypt: %v", err)
	}
	sub, err := crypto.DecodeTLV(plain)
	if err != nil {
		f.r.t.Fatal(err)
	}
	f.clientID, _ = crypto.GetTLV(sub, crypto.TLVIdentifier)
	f.clientLTPK, _ = crypto.GetTLV(sub, crypto.TLVPublicKey)
	sig, _ := crypto.GetTLV(sub, crypto.TLVSignature)
	x := crypto.HKDF("Pair-Setup-Controller-Sign-Salt", "Pair-Setup-Controller-Sign-Info", f.srp.K, 32)
	info := append(append(append([]byte{}, x...), f.clientID...), f.clientLTPK...)
	if !crypto.Ed25519Verify(f.clientLTPK, info, sig) {
		f.r.t.Fatalf("client pair-setup signature did not verify")
	}

	f.accessorySeed = crypto.RandomBytes(32)
	f.accessoryLTPK = crypto.Ed25519PublicFromSeed(f.accessorySeed)
	f.accessoryID = crypto.RandomBytes(16)
	xA := crypto.HKDF("Pair-Setup-Accessory-Sign-Salt", "Pair-Setup-Accessory-Sign-Info", f.srp.K, 32)
	accInfo := append(append(append([]byte{}, xA...), f.accessoryID...), f.accessoryLTPK...)
	accSig := crypto.Ed25519Sign(f.accessorySeed, accInfo)
	inner := crypto.EncodeTLV([]crypto.TLV{
		{Type: crypto.TLVIdentifier, Value: f.accessoryID},
		{Type: crypto.TLVPublicKey, Value: f.accessoryLTPK},
		{Type: crypto.TLVSignature, Value: accSig},
	})
	body, err := crypto.ChaCha20Poly1305Encrypt(f.pairSetupKey, []byte("PS-Msg06"), inner, nil)
	if err != nil {
		f.r.t.Fatal(err)
	}
	f.reply(req, 200, "application/octet-stream", crypto.EncodeTLV([]crypto.TLV{
		{Type: crypto.TLVState, Value: []byte{6}},
		{Type: crypto.TLVEncryptedData, Value: body},
	}))
}

func (f *fakeReceiver) pairVerify(req request) {
	items, err := crypto.DecodeTLV(req.body)
	if err != nil {
		f.r.t.Fatalf("pair-verify TLV: %v", err)
	}
	state, _ := crypto.GetTLV(items, crypto.TLVState)
	switch {
	case len(state) == 1 && state[0] == 1:
		if f.rejectVerifyWithTLV != 0 {
			code := f.rejectVerifyWithTLV
			f.rejectVerifyWithTLV = 0
			f.reply(req, 200, "application/octet-stream", crypto.EncodeTLV([]crypto.TLV{
				{Type: crypto.TLVState, Value: []byte{2}},
				{Type: crypto.TLVError, Value: []byte{code}},
			}))
			return
		}
		clientPub, _ := crypto.GetTLV(items, crypto.TLVPublicKey)
		f.lastClientPub = append([]byte(nil), clientPub...)
		f.verifyPub, f.verifyPriv, _ = crypto.X25519Generate()
		shared, err := crypto.X25519SharedSecret(f.verifyPriv, clientPub)
		if err != nil {
			f.r.t.Fatal(err)
		}
		f.sharedSecret = shared
		f.deriveKeys()
		f.verifyKey = crypto.HKDF("Pair-Verify-Encrypt-Salt", "Pair-Verify-Encrypt-Info", shared, 32)
		sigInfo := append(append(append([]byte{}, f.verifyPub...), f.accessoryID...), clientPub...)
		sig := crypto.Ed25519Sign(f.accessorySeed, sigInfo)
		enc, err := crypto.ChaCha20Poly1305Encrypt(f.verifyKey, []byte("PV-Msg02"), crypto.EncodeTLV([]crypto.TLV{
			{Type: crypto.TLVIdentifier, Value: f.accessoryID},
			{Type: crypto.TLVSignature, Value: sig},
		}), nil)
		if err != nil {
			f.r.t.Fatal(err)
		}
		f.reply(req, 200, "application/octet-stream", crypto.EncodeTLV([]crypto.TLV{
			{Type: crypto.TLVState, Value: []byte{2}},
			{Type: crypto.TLVPublicKey, Value: f.verifyPub},
			{Type: crypto.TLVEncryptedData, Value: enc},
		}))
	case len(state) == 1 && state[0] == 3:
		enc, _ := crypto.GetTLV(items, crypto.TLVEncryptedData)
		plain, err := crypto.ChaCha20Poly1305Decrypt(f.verifyKey, []byte("PV-Msg03"), enc, nil)
		if err != nil {
			f.r.t.Fatalf("PV-Msg03 decrypt: %v", err)
		}
		sub, _ := crypto.DecodeTLV(plain)
		clientID, _ := crypto.GetTLV(sub, crypto.TLVIdentifier)
		sig, _ := crypto.GetTLV(sub, crypto.TLVSignature)
		if !crypto.Ed25519Verify(f.clientLTPK, f.lastClientVerifyInfo(clientID), sig) {
			f.r.t.Fatalf("client pair-verify signature did not verify")
		}
		f.encryptAfterReply = true
		f.reply(req, 200, "application/octet-stream", nil)
	default:
		f.r.t.Fatalf("pair-verify unexpected state %v", state)
	}
}

// lastClientVerifyInfo rebuilds clientPub || clientID || serverPub from the
// last verification exchange.
func (f *fakeReceiver) lastClientVerifyInfo(clientID []byte) []byte {
	return append(append(append([]byte{}, f.lastClientPub...), clientID...), f.verifyPub...)
}

func (f *fakeReceiver) deriveKeys() {
	f.readKey = crypto.HKDF("Control-Salt", "Control-Write-Encryption-Key", f.sharedSecret, 32)
	f.writeKey = crypto.HKDF("Control-Salt", "Control-Read-Encryption-Key", f.sharedSecret, 32)
	f.eventWriteKey = crypto.HKDF("Events-Salt", "Events-Write-Encryption-Key", f.sharedSecret, 32)
	f.eventReadKey = crypto.HKDF("Events-Salt", "Events-Read-Encryption-Key", f.sharedSecret, 32)
}

func (f *fakeReceiver) handleEventResponse(data []byte) {
	f.eventEncBuf = append(f.eventEncBuf, data...)
	f.decryptFrames(f.eventReadKey, &f.eventReadCtr, &f.eventEncBuf, &f.eventPlain, "event response")
	for {
		idx := bytes.Index(f.eventPlain, []byte("\r\n\r\n"))
		if idx < 0 {
			return
		}
		lines := strings.Split(string(f.eventPlain[:idx]), "\r\n")
		n := 0
		for _, line := range lines[1:] {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "content-length") {
				n, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
		total := idx + 4 + n
		if len(f.eventPlain) < total {
			return
		}
		f.eventResp = append(f.eventResp, append([]byte(nil), f.eventPlain[:total]...))
		f.eventPlain = f.eventPlain[total:]
	}
}

// pushEvent sends one encrypted receiver request on the event channel.
func (f *fakeReceiver) pushEvent(req string) {
	var framed []byte
	for off := 0; off < len(req); {
		n := len(req) - off
		if n > 1024 {
			n = 1024
		}
		lp := []byte{byte(n), byte(n >> 8)}
		nonce := crypto.CounterNonce8(f.eventWriteCtr)
		f.eventWriteCtr++
		ct, err := crypto.ChaCha20Poly1305Encrypt(f.eventWriteKey, nonce, []byte(req[off:off+n]), lp)
		if err != nil {
			f.r.t.Fatal(err)
		}
		framed = append(append(framed, lp...), ct...)
		off += n
	}
	f.eventOpen = true
	f.r.emit(raopio.Event{Kind: raopio.EventTCPData, TCP: raopio.TCPEvent, Data: framed})
}

func (f *fakeReceiver) handleAudio(data []byte) {
	if len(data) < 12 {
		f.r.t.Fatalf("short audio packet %d", len(data))
	}
	f.audioSeqs = append(f.audioSeqs, binary.BigEndian.Uint16(data[2:4]))
	f.audioTS = append(f.audioTS, binary.BigEndian.Uint32(data[4:8]))
	f.audioMarker = append(f.audioMarker, data[1])
	payload := data[12:]
	if f.ap2Audio {
		if len(payload) < 24 {
			f.r.t.Fatalf("short encrypted payload %d", len(payload))
		}
		ct := payload[:len(payload)-8]
		nonce := payload[len(payload)-8:]
		want := crypto.CounterNonce8(f.audioNonce)
		if !bytes.Equal(nonce, want) {
			f.r.t.Fatalf("audio nonce %x, want %x", nonce, want)
		}
		plain, err := crypto.ChaCha20Poly1305Decrypt(f.audioKey, nonce, ct, data[4:12])
		if err != nil {
			f.r.t.Fatalf("audio decrypt: %v", err)
		}
		payload = plain
		f.audioNonce++
	}
	f.audioPkts = append(f.audioPkts, append([]byte(nil), payload...))
	if f.ap2Audio {
		samples, err := decodeALAC(payload)
		if err != nil {
			f.r.t.Fatalf("ALAC decode: %v", err)
		}
		f.audioFrames = append(f.audioFrames, samples...)
	} else {
		for i := 0; i+1 < len(payload); i += 2 {
			f.audioFrames = append(f.audioFrames, int16(binary.BigEndian.Uint16(payload[i:i+2])))
		}
	}
}

// decodeALAC reads the uncompressed ALAC frame layout, MSB-first.
func decodeALAC(frame []byte) ([]int16, error) {
	br := bitReader{data: frame}
	tag := br.read(3)
	if tag != 1 {
		return nil, errString("unexpected channel tag")
	}
	br.read(4)
	br.read(12)
	br.read(1)
	br.read(2)
	if br.read(1) != 1 {
		return nil, errString("not uncompressed")
	}
	out := make([]int16, framesPerPacket*channels)
	for i := range out {
		out[i] = int16(br.read(16))
	}
	if br.read(3) != 7 {
		return nil, errString("missing END tag")
	}
	return out, nil
}

type bitReader struct {
	data  []byte
	pos   int
	shift uint
}

func (b *bitReader) read(n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		byteIdx := b.pos / 8
		if byteIdx >= len(b.data) {
			return v
		}
		bit := (b.data[byteIdx] >> (7 - (b.pos % 8))) & 1
		v = v<<1 | uint32(bit)
		b.pos++
	}
	return v
}

type errString string

func (e errString) Error() string { return string(e) }
