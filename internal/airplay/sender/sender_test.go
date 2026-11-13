package sender

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/audio/ring"
)

func fillRing(size int) (*ring.Buffer, []int16) {
	rb := ring.New(size * 2 * 2)
	src := make([]int16, size*2)
	for i := 0; i < size; i++ {
		v := int16((i*7)%30000 - 15000)
		src[i*2] = v
		src[i*2+1] = v / 2
	}
	rb.Push(src)
	return rb, src
}

func requireLaunched(t *testing.T, r *rig) {
	t.Helper()
	if len(r.launched) != 1 {
		t.Fatalf("launched events = %d, want 1", len(r.launched))
	}
	if !r.launched[0].ok {
		t.Fatalf("launch failed: %s", r.launched[0].err)
	}
	if r.sender.State() != StateStreaming {
		t.Fatalf("state = %s, want streaming", r.sender.State())
	}
}

func requireAudioMatches(t *testing.T, r *rig, src []int16) {
	t.Helper()
	if len(r.rx.audioFrames) == 0 {
		t.Fatal("no audio received")
	}
	n := len(r.rx.audioFrames)
	if n > len(src) {
		n = len(src)
	}
	for i := 0; i < n; i++ {
		if r.rx.audioFrames[i] != src[i] {
			t.Fatalf("sample %d = %d, want %d", i, r.rx.audioFrames[i], src[i])
		}
	}
	if len(r.rx.audioFrames) >= 352*2 {
		if r.rx.audioMarker[0] != 0xE0 {
			t.Fatalf("first marker = %#x, want 0xE0", r.rx.audioMarker[0])
		}
		for i := 1; i < len(r.rx.audioMarker); i++ {
			if r.rx.audioMarker[i] != 0x60 {
				t.Fatalf("marker %d = %#x, want 0x60", i, r.rx.audioMarker[i])
			}
		}
		first := r.rx.audioSeqs[0]
		for i := range r.rx.audioSeqs {
			if r.rx.audioSeqs[i] != first+uint16(i) {
				t.Fatalf("seq %d = %d, want %d", i, r.rx.audioSeqs[i], first+uint16(i))
			}
		}
		for i := range r.rx.audioTS {
			want := r.sender.latency + uint32(i*framesPerPacket)
			if r.rx.audioTS[i] != want {
				t.Fatalf("timestamp %d = %d, want %d", i, r.rx.audioTS[i], want)
			}
		}
	}
}

func TestAP2TransientStreamsAndKeepsAlive(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	if len(r.rx.sharedSecret) != 64 {
		t.Fatalf("transient shared secret = %d bytes, want 64", len(r.rx.sharedSecret))
	}
	if len(r.rx.audioKey) != 32 {
		t.Fatalf("shk = %d bytes, want 32", len(r.rx.audioKey))
	}

	// The start trim already ran; refill the ring so the comparison starts at
	// the newest edge.
	rb.Reset()
	rb.Push(src)
	r.advance(500 * time.Millisecond)
	requireAudioMatches(t, r, src)

	// The event channel must answer with exactly Server and CSeq.
	r.rx.pushEvent("POST /command RTSP/1.0\r\nCSeq: 42\r\nContent-Length: 0\r\n\r\n")
	r.step()
	if len(r.rx.eventResp) != 1 {
		t.Fatalf("event responses = %d, want 1", len(r.rx.eventResp))
	}
	want := "RTSP/1.0 200 OK\r\nServer: AirTunes/550.10\r\nCSeq: 42\r\n\r\n"
	if string(r.rx.eventResp[0]) != want {
		t.Fatalf("event response = %q, want %q", r.rx.eventResp[0], want)
	}

	// AP2 feedback goes out every two seconds.
	before := r.rx.feedbacks
	r.advance(2100 * time.Millisecond)
	if r.rx.feedbacks <= before {
		t.Fatalf("feedback requests = %d, want more than %d", r.rx.feedbacks, before)
	}

	// Timing requests are answered with the request's send time echoed.
	req := make([]byte, 32)
	req[0] = 0x80
	copy(req[24:32], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	r.emit(raopio.Event{Kind: raopio.EventUDPDatagram, UDP: raopio.UDPTiming, Data: req,
		Peer: raopio.Endpoint{IP: "192.168.1.20", Port: 6002}})
	r.step()
	if len(r.rx.timingResponses) != 1 {
		t.Fatalf("timing responses = %d", len(r.rx.timingResponses))
	}
	resp := r.rx.timingResponses[0]
	if resp[1] != 0xD3 || !bytes.Equal(resp[8:16], req[24:32]) || len(resp) != 32 {
		t.Fatalf("bad timing response %x", resp)
	}
	if !bytes.Equal(resp[16:24], resp[24:32]) {
		t.Fatalf("receive and send time differ: %x", resp)
	}

	// A retransmit request returns the original packet.
	var lost uint16
	for i, seq := range r.rx.audioSeqs {
		if i == 3 {
			lost = seq
			break
		}
	}
	r.emit(raopio.Event{Kind: raopio.EventUDPDatagram, UDP: raopio.UDPControl,
		Data: []byte{0x80, 0x55, 0, 1, byte(lost >> 8), byte(lost), 0, 1},
		Peer: raopio.Endpoint{IP: "192.168.1.20", Port: 6001}})
	r.step()
	found := false
	for _, pkt := range r.rx.syncPackets {
		if len(pkt) >= 4 && pkt[0] == 0x80 && pkt[1] == 0xD6 {
			got := uint16(pkt[2])<<8 | uint16(pkt[3])
			if got != lost {
				t.Fatalf("retransmit seq = %d, want %d", got, lost)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no retransmit response")
	}
}

func TestStereoGroupSessionCarriesGroupUUIDAndPTPTimeline(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	const tsid = "00000000-0000-4000-8000-000000000000"
	r.sender.SetGroupUUID(tsid)
	r.sender.SetPTPClock(readyClock{})
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	r.advance(500 * time.Millisecond)
	requireLaunched(t, r)

	if got := r.rx.sessionSetup["groupUUID"]; got != tsid {
		t.Fatalf("groupUUID = %v, want %q", got, tsid)
	}
	// SETPEERS is a no-op on a PTP session; the receiver's grandmaster
	// supplies the timeline, so the sender must not spend a round trip on it.
	if len(r.rx.setPeersBody) != 0 {
		t.Fatalf("unexpected SETPEERS on a PTP session: % x", r.rx.setPeersBody)
	}
}

func TestDuplicateCopiesAndPrefill(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.sender.SetDuplicates(1)
	r.sender.SetPrefill(2)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	// Two prefilled blocks, two copies each, before the pacer's first tick.
	if got := len(r.rx.audioSeqs); got != 4 {
		t.Fatalf("wire packets after prefill = %d, want 4", got)
	}
	if got := len(r.rx.audioPkts); got != 2 {
		t.Fatalf("decoded blocks after prefill = %d, want 2", got)
	}
	if r.rx.audioSeqs[0] != r.rx.audioSeqs[1] || r.rx.audioSeqs[2] != r.rx.audioSeqs[3] {
		t.Fatalf("duplicate sequence numbers = %v", r.rx.audioSeqs)
	}
	if r.rx.audioSeqs[0] == r.rx.audioSeqs[2] {
		t.Fatal("prefilled blocks shared a sequence number")
	}
}

func TestAP2PinPairsAndReconnectsWithCredentials(t *testing.T) {
	r := newRig(t, modePIN, device.AuthHAPPIN, true, nil)
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	if len(r.pins) != 1 || !r.sender.WaitingForPin() {
		t.Fatalf("expected a PIN prompt, got pins=%v waiting=%v", r.pins, r.sender.WaitingForPin())
	}
	if len(r.launched) != 0 {
		t.Fatal("session launched before the PIN was entered")
	}

	r.sender.SubmitPin("1234")
	r.step()
	r.sender.Tick()
	requireLaunched(t, r)
	rb.Reset()
	rb.Push(src)
	r.advance(300 * time.Millisecond)
	requireAudioMatches(t, r, src)

	if len(r.creds) != 1 {
		t.Fatalf("credentials obtained = %d", len(r.creds))
	}
	parsed, ok := creds.Parse(r.creds[0])
	if !ok || len(parsed.LTSK) != 32 || len(parsed.LTPK) != 32 || parsed.ClientID == "" {
		t.Fatalf("bad credentials %q", r.creds[0])
	}

	stored := &storedPairing{
		clientID:      r.rx.clientID,
		clientLTPK:    r.rx.clientLTPK,
		accessoryID:   r.rx.accessoryID,
		accessoryLTPK: r.rx.accessoryLTPK,
		accessorySeed: r.rx.accessorySeed,
	}
	r2 := newRig(t, modeVerify, device.AuthHAPPIN, true, stored)
	rb2, src2 := fillRing(44100)
	r2.sender.AttachRing(rb2)
	r2.sender.SetAuth(device.AuthHAPPIN, true, "AA:BB:CC:DD:EE:FF", r.creds[0], "")
	r2.start()
	requireLaunched(t, r2)
	if r2.rx.requests["POST /pair-setup"] != 0 {
		t.Fatalf("reconnect ran pair-setup %d times", r2.rx.requests["POST /pair-setup"])
	}
	if r2.rx.requests["POST /pair-verify"] < 2 {
		t.Fatalf("reconnect pair-verify requests = %d", r2.rx.requests["POST /pair-verify"])
	}
	rb2.Reset()
	rb2.Push(src2)
	r2.advance(300 * time.Millisecond)
	requireAudioMatches(t, r2, src2)
}

// TestAP2VerifyRejectionRepairsWithFreshPairing pins the dead-end fix: a
// receiver that forgot the pairing rejects the resumed pair-verify, and the
// sender must pair afresh on the same connection instead of failing the
// session and retrying identical credentials forever.
func TestAP2VerifyRejectionRepairsWithFreshPairing(t *testing.T) {
	r := newRig(t, modePIN, device.AuthHAPPIN, true, nil)
	r.start()
	r.sender.SubmitPin("1234")
	r.step()
	r.sender.Tick()
	requireLaunched(t, r)
	if len(r.creds) != 1 {
		t.Fatalf("credentials obtained = %d", len(r.creds))
	}
	stored := &storedPairing{
		clientID:      r.rx.clientID,
		clientLTPK:    r.rx.clientLTPK,
		accessoryID:   r.rx.accessoryID,
		accessoryLTPK: r.rx.accessoryLTPK,
		accessorySeed: r.rx.accessorySeed,
	}

	// The receiver forgot the pairing: the resumed pair-verify is rejected.
	r2 := newRig(t, modeVerify, device.AuthHAPPIN, true, stored)
	r2.sender.SetAuth(device.AuthHAPPIN, true, "AA:BB:CC:DD:EE:FF", r.creds[0], "")
	r2.rx.rejectVerifyWithTLV = 6
	r2.start()
	if len(r2.pins) != 1 {
		t.Fatalf("repair did not re-enter PIN pairing, pins = %v", r2.pins)
	}
	r2.sender.SubmitPin("1234")
	r2.step()
	r2.sender.Tick()
	requireLaunched(t, r2)
	if r2.rx.requests["POST /pair-setup"] != 3 {
		// M1 + M3 + M5: one complete fresh pairing on the same connection.
		t.Fatalf("repair pair-setup requests = %d, want 3", r2.rx.requests["POST /pair-setup"])
	}
	if r2.rx.requests["POST /pair-verify"] < 2 {
		t.Fatalf("pair-verify requests = %d, want the rejected one plus the repair", r2.rx.requests["POST /pair-verify"])
	}
	if len(r2.creds) != 1 {
		t.Fatalf("credentials after repair = %d, want fresh ones", len(r2.creds))
	}
}

// TestLatencyAboveTwoSecondsReachesStreamSetup pins the shared window: the
// value the interface reports must be the value the receiver sees on the wire.
func TestLatencyAboveTwoSecondsReachesStreamSetup(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.sender.SetLatency(2300)
	r.start()
	// 2300 ms at 44.1 kHz is 101430 samples.
	if r.rx.setupLatencyMin != 101430 || r.rx.setupLatencyMax != 101430 {
		t.Fatalf("stream SETUP latency = %d..%d, want 101430",
			r.rx.setupLatencyMin, r.rx.setupLatencyMax)
	}
}

// TestUnpairedReceiverAuthWarnsOrFails pins both sides of the first-pairing
// policy: strict fails on a bad receiver signature, permissive continues but
// names the receiver in the warning.
func TestUnpairedReceiverAuthWarnsOrFails(t *testing.T) {
	start := func(t *testing.T, strict bool) *rig {
		t.Helper()
		r := newRig(t, modePIN, device.AuthHAPPIN, true, nil)
		r.sender.SetStrictReceiverAuth(strict)
		r.rx.verifySignSeed = crypto.RandomBytes(32)
		r.start()
		r.sender.SubmitPin("1234")
		r.step()
		r.sender.Tick()
		return r
	}

	t.Run("strict fails", func(t *testing.T) {
		r := start(t, true)
		if r.rx.pairVerifyM3Seen {
			t.Fatal("strict mode offered the client signature to an unproven receiver")
		}
		if len(r.launched) == 0 || r.launched[0].ok {
			t.Fatalf("launch results = %+v, want a failure", r.launched)
		}
		if !strings.Contains(r.launched[0].err, "receiver not authenticated") {
			t.Fatalf("error = %q, want the receiver-not-authenticated message", r.launched[0].err)
		}
	})

	t.Run("permissive warns with the receiver name", func(t *testing.T) {
		r := start(t, false)
		requireLaunched(t, r)
		found := false
		for _, line := range r.logs {
			if !strings.Contains(line, "pair-verify signature did not verify, continuing") {
				continue
			}
			found = true
			if !strings.Contains(line, "FakeBox") {
				t.Fatalf("permissive warning does not name the receiver: %q", line)
			}
		}
		if !found {
			t.Fatal("permissive mode did not emit the verification warning")
		}
	})
}

// TestStoredPairingRequiresTheReceiverProof pins the redirect defense at the
// sender boundary: with stored credentials in hand, a receiver that cannot
// prove it holds the stored long-term key must never receive the client's
// signature, and the session must fail.
func TestStoredPairingRequiresTheReceiverProof(t *testing.T) {
	r := newRig(t, modePIN, device.AuthHAPPIN, true, nil)
	r.start()
	r.sender.SubmitPin("1234")
	r.step()
	r.sender.Tick()
	requireLaunched(t, r)
	if len(r.creds) != 1 {
		t.Fatalf("credentials obtained = %d", len(r.creds))
	}
	stored := &storedPairing{
		clientID:      r.rx.clientID,
		clientLTPK:    r.rx.clientLTPK,
		accessoryID:   r.rx.accessoryID,
		accessoryLTPK: r.rx.accessoryLTPK,
		accessorySeed: r.rx.accessorySeed,
	}

	r2 := newRig(t, modeVerify, device.AuthHAPPIN, true, stored)
	r2.sender.SetAuth(device.AuthHAPPIN, true, "AA:BB:CC:DD:EE:FF", r.creds[0], "")
	r2.rx.verifySignSeed = crypto.RandomBytes(32)
	r2.start()
	r2.step()
	if r2.rx.pairVerifyM3Seen {
		t.Fatal("the sender offered its stored-key signature to an unproven receiver")
	}
	if len(r2.launched) == 0 || r2.launched[0].ok {
		t.Fatalf("launch results = %+v, want a failed session", r2.launched)
	}
}

func TestAP1SessionSendsRawPCM(t *testing.T) {
	r := newRig(t, modeAP1, device.AuthNone, false, nil)
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	rb.Reset()
	rb.Push(src)
	r.advance(300 * time.Millisecond)
	requireAudioMatches(t, r, src)
	if len(r.rx.audioSeqs) == 0 {
		t.Fatal("no AP1 audio")
	}
}

func Test48kInputIsResampledAndPaddedWithSilence(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb := ring.New(48000 * 4)
	src := make([]int16, 48000)
	for i := range src {
		src[i] = 1000
	}
	rb.Push(src)
	r.sender.AttachRing(rb)
	r.sender.SetInputFormat(48000)
	r.start()
	requireLaunched(t, r)
	rb.Reset()
	rb.Push(src)
	r.advance(500 * time.Millisecond)
	// audioFrames holds interleaved samples: about 44100 for half a second.
	if len(r.rx.audioFrames) < 40000 || len(r.rx.audioFrames) > 46000 {
		t.Fatalf("produced %d samples from 0.5 s of 48 kHz input (elapsed %v, packets %d)",
			len(r.rx.audioFrames), r.now.Sub(r.sender.clockStart), len(r.rx.audioSeqs))
	}
	for i := 0; i < 36000; i++ {
		if r.rx.audioFrames[i] != 1000 {
			t.Fatalf("sample %d = %d, want 1000", i, r.rx.audioFrames[i])
		}
	}
	r.advance(1500 * time.Millisecond)
	tail := r.rx.audioFrames[len(r.rx.audioFrames)-1000:]
	for i, v := range tail {
		if v != 0 {
			t.Fatalf("tail sample %d = %d, want silence", i, v)
		}
	}
}

type neverReadyClock struct{}

func (neverReadyClock) ClockID() uint64                        { return 42 }
func (neverReadyClock) Ready() bool                            { return false }
func (neverReadyClock) Timeline(uint64) (uint64, uint64, bool) { return 0, 0, false }
func (neverReadyClock) Activate()                              {}

type readyClock struct{}

func (readyClock) ClockID() uint64 { return 42 }
func (readyClock) Ready() bool     { return true }
func (readyClock) Timeline(ns uint64) (uint64, uint64, bool) {
	return 7, ns, true
}
func (readyClock) Activate() {}

// TestPTPWaitToleratesSlowClock keeps the session alive through the extended
// PTP window and fails with the firewall hint only after it expires.
// TestRefusesASessionToThisHost pins the socket-level guard: when the
// connected socket's local and peer addresses match, the target is this
// host, so the sender stops before sending a pairing request.
func TestRefusesASessionToThisHost(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.localIP = "192.168.1.10"
	r.peerIP = "192.168.1.10"
	r.start()
	if len(r.launched) != 1 || r.launched[0].ok {
		t.Fatalf("launched = %+v, want one failure", r.launched)
	}
	if !strings.Contains(r.launched[0].err, "local receiver") {
		t.Fatalf("error = %q, want the local receiver refusal", r.launched[0].err)
	}
	if r.sender.State() != StateIdle {
		t.Fatalf("state = %s, want idle", r.sender.State())
	}
	if len(r.rx.requests) != 0 {
		t.Fatalf("pairing requests reached the receiver: %v", r.rx.requests)
	}
}

func TestPTPWaitToleratesSlowClock(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.sender.SetPTPClock(neverReadyClock{})
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	if r.sender.pairStage != pairAP2WaitPTP {
		t.Fatalf("stage = %d, want WaitPTP", r.sender.pairStage)
	}
	for i := 0; i < 5; i++ {
		r.advance(5 * time.Second)
		if len(r.launched) != 0 {
			t.Fatal("gave up inside the 30s PTP window")
		}
	}
	r.advance(6 * time.Second)
	if len(r.launched) != 1 || r.launched[0].ok {
		t.Fatalf("launched = %+v, want one PTP-timeout failure", r.launched)
	}
	if !strings.Contains(r.launched[0].err, "PTP clock") {
		t.Fatalf("error = %q", r.launched[0].err)
	}
}

// TestReceiverVolumePollAdoptsHardwareChanges proves the streaming-time
// /info poll reports the receiver's own volume when hardware moves it.
func TestReceiverVolumePollAdoptsHardwareChanges(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	r.advance(1100 * time.Millisecond) // first poll: initialVolume -27 dB → 10%
	if len(r.volumeEvents) != 1 || r.volumeEvents[0] != 10 {
		t.Fatalf("first poll events = %v, want [10]", r.volumeEvents)
	}
	r.rx.receiverVolume = -15 // 50 %, as if the hardware knob moved
	r.advance(1100 * time.Millisecond)
	if len(r.volumeEvents) != 2 || r.volumeEvents[1] != 50 {
		t.Fatalf("second poll events = %v, want [10 50]", r.volumeEvents)
	}
	r.rx.receiverVolume = -15 // unchanged: no duplicate event
	r.advance(1100 * time.Millisecond)
	if len(r.volumeEvents) != 2 {
		t.Fatalf("unchanged volume re-reported: %v", r.volumeEvents)
	}
}

// TestLatencyAppliesToStreamSetup proves the configurable latency lands in
// the stream SETUP plist and the sync-packet timeline.
func TestLatencyAppliesToStreamSetup(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.sender.SetLatency(500) // 500 ms → 22050 samples at 44.1 kHz
	r.start()
	requireLaunched(t, r)
	streams, ok := r.rx.streamSetup["streams"].([]any)
	if !ok || len(streams) == 0 {
		t.Fatalf("stream SETUP missing: %+v", r.rx.streamSetup)
	}
	s0 := streams[0].(map[string]any)
	if got := plistInt(s0, "latencyMin"); got != 22050 {
		t.Fatalf("latencyMin = %d, want 22050", got)
	}
	if got := plistInt(s0, "latencyMax"); got != 22050 {
		t.Fatalf("latencyMax = %d, want 22050", got)
	}
	if r.sender.latency != 22050 {
		t.Fatalf("sync timeline latency = %d, want 22050", r.sender.latency)
	}
}

func TestInitialVolumeAdoptedWhenUserDidNotSetOne(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	got := volumeRequests(r)
	if len(got) != 1 || got[0] != "volume: -27.000000" {
		t.Fatalf("volume requests = %q, want [volume: -27.000000]", got)
	}
}

func TestNowPlayingRidesAsDMAP(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.sender.SetNowPlaying("夜曲", "周杰伦", "十一月的萧邦", nil, "")
	r.start()
	requireLaunched(t, r)
	var dmap string
	for _, p := range r.rx.setParams {
		if strings.HasPrefix(p, "application/x-dmap-tagged\x00") {
			dmap = p
		}
	}
	if dmap == "" {
		t.Fatal("no DMAP SET_PARAMETER was sent")
	}
	for _, want := range []string{"minm", "夜曲", "asar", "周杰伦", "asal", "十一月的萧邦"} {
		if !strings.Contains(dmap, want) {
			t.Fatalf("DMAP body missing %q", want)
		}
	}
}

func TestNowPlayingCoverRidesAsImage(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	cover := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	r.sender.SetNowPlaying("夜曲", "周杰伦", "十一月的萧邦", cover, "image/png")
	r.start()
	requireLaunched(t, r)
	var image string
	for _, p := range r.rx.setParams {
		if strings.HasPrefix(p, "image/png\x00") {
			image = p
		}
	}
	if image != "image/png\x00"+string(cover) {
		t.Fatalf("image SET_PARAMETER = %q, want the cover bytes", image)
	}
}

func TestHandshakeTimeoutFails(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.rx.failAll = true
	r.start()
	r.advance(11 * time.Second)
	if len(r.launched) != 1 || r.launched[0].ok {
		t.Fatalf("launched = %+v, want one failure", r.launched)
	}
	if !strings.Contains(r.launched[0].err, "Timed out") {
		t.Fatalf("error = %q", r.launched[0].err)
	}
	if r.closed != 1 {
		t.Fatalf("closed = %d, want 1", r.closed)
	}
}

// TestPacerLagWarnsAfterConsecutiveLateTicks pins the keep-up telemetry: a
// host that cannot meet the wall-clock target warns once per episode after
// five consecutive late pacer ticks, and a caught-up tick clears the lag.
func TestPacerLagWarnsAfterConsecutiveLateTicks(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	// One Tick 200 ms behind the target sends its 16-packet burst and still
	// ends behind, because the target moved ~8820 frames in a single step.
	for i := 0; i < 4; i++ {
		r.now = r.now.Add(200 * time.Millisecond)
		r.sender.Tick()
	}
	if _, lagging := r.sender.PacerLag(); lagging {
		t.Fatal("warned before five consecutive late ticks")
	}
	r.now = r.now.Add(200 * time.Millisecond)
	r.sender.Tick()
	frames, lagging := r.sender.PacerLag()
	if !lagging || frames == 0 {
		t.Fatalf("fifth late tick: frames=%d lagging=%v", frames, lagging)
	}
	warns := 0
	for _, l := range r.logs {
		if strings.Contains(l, "cannot keep up") {
			warns++
		}
	}
	if warns != 1 {
		t.Fatalf("keep-up warnings = %d, want 1", warns)
	}

	// Recovery: normal-paced ticks catch the burst cap up to the target and
	// clear both the lag and the streak.
	for i := 0; i < 20; i++ {
		r.advance(50 * time.Millisecond)
		if frames, lagging := r.sender.PacerLag(); !lagging && frames == 0 {
			return
		}
	}
	if frames, lagging := r.sender.PacerLag(); lagging || frames != 0 {
		t.Fatalf("recovery left lag frames=%d lagging=%v", frames, lagging)
	}
}

func TestStopSendsTeardown(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(4410)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	r.sender.Stop()
	r.step()
	if r.rx.requests["TEARDOWN "+r.sender.rtspURI()] != 1 {
		t.Fatalf("teardown requests = %d", r.rx.requests["TEARDOWN "+r.sender.rtspURI()])
	}
	if r.sender.State() != StateIdle {
		t.Fatalf("state = %s", r.sender.State())
	}
}
