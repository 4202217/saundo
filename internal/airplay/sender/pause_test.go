package sender

import (
	"fmt"
	"testing"
	"time"

	"saundo/internal/airplay/device"
)

func TestPauseStopsAudioKeepsKeepAliveAndResumesAnchored(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	rb.Reset()
	rb.Push(src)
	r.advance(500 * time.Millisecond)
	if len(r.rx.audioSeqs) == 0 {
		t.Fatal("no audio before pause")
	}

	before := len(r.rx.audioSeqs)
	syncs := len(r.rx.syncPackets)
	r.sender.SetPaused(true)
	r.advance(2 * time.Second)
	if got := len(r.rx.audioSeqs); got != before {
		t.Fatalf("audio packets during pause = %d, want 0", got-before)
	}
	if r.sender.State() != StateStreaming {
		t.Fatalf("state during pause = %s, want streaming", r.sender.State())
	}
	if len(r.rx.syncPackets) <= syncs {
		t.Fatal("sync packets stopped during pause")
	}

	feedbacks := r.rx.feedbacks
	r.advance(2100 * time.Millisecond)
	if r.rx.feedbacks <= feedbacks {
		t.Fatal("feedback stopped during pause")
	}

	rb.Reset()
	rb.Push(src)
	r.sender.SetPaused(false)
	wantFirst := r.sender.latency + uint32(r.sender.framesSent)
	first := len(r.rx.audioSeqs)
	r.advance(time.Second)
	if len(r.rx.audioSeqs) <= first {
		t.Fatal("audio did not resume")
	}
	if got := r.rx.audioTS[first]; got != wantFirst {
		t.Fatalf("first resumed RTP = %d, want %d", got, wantFirst)
	}
	if len(r.rx.flushRTPInfo) != 1 {
		t.Fatalf("flush requests = %d, want one on resume", len(r.rx.flushRTPInfo))
	}
	wantFlush := fmt.Sprintf("seq=%d;rtptime=%d", r.rx.audioSeqs[first], wantFirst)
	if got := r.rx.flushRTPInfo[0]; got != wantFlush {
		t.Fatalf("flush RTP-Info = %q, want %q", got, wantFlush)
	}
}

func TestPauseIsIgnoredOnAirPlay1(t *testing.T) {
	r := newRig(t, modeAP1, device.AuthNone, false, nil)
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	rb.Reset()
	rb.Push(src)
	r.advance(500 * time.Millisecond)

	before := len(r.rx.audioSeqs)
	if r.sender.SetPaused(true) {
		t.Fatal("AirPlay 1 sender accepted a pause")
	}
	r.advance(500 * time.Millisecond)
	if len(r.rx.audioSeqs) <= before {
		t.Fatal("AirPlay 1 sender stopped streaming after a pause request")
	}
}

// TestPauseSupportAndRequestExposeTheCohortContract pins the getters the
// cohort reads before it touches any member: support decides, and a request is
// recorded only when the sender accepted it.
func TestPauseSupportAndRequestExposeTheCohortContract(t *testing.T) {
	ap2 := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	if !ap2.sender.PauseSupported() {
		t.Fatal("an AP2 sender reported pause unsupported")
	}
	ap2.sender.SetPaused(true)
	if !ap2.sender.PauseRequested() {
		t.Fatal("an AP2 sender did not record the pause request")
	}
	ap2.sender.SetPaused(false)
	if ap2.sender.PauseRequested() {
		t.Fatal("the pause request survived the resume")
	}

	ap1 := newRig(t, modeAP1, device.AuthNone, false, nil)
	if ap1.sender.PauseSupported() {
		t.Fatal("an AirPlay 1 sender reported pause support")
	}
	if ap1.sender.SetPaused(true) {
		t.Fatal("an AirPlay 1 sender accepted a pause")
	}
	if ap1.sender.PauseRequested() {
		t.Fatal("a refused pause was recorded as a request")
	}
}

func TestPauseIsIdempotent(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, src := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	rb.Reset()
	rb.Push(src)
	r.advance(500 * time.Millisecond)

	r.sender.SetPaused(true)
	r.sender.SetPaused(true)
	before := len(r.rx.audioSeqs)
	r.advance(time.Second)
	if got := len(r.rx.audioSeqs); got != before {
		t.Fatalf("audio packets while paused twice = %d, want 0", got-before)
	}

	r.sender.SetPaused(false)
	r.sender.SetPaused(false)
	first := len(r.rx.audioSeqs)
	r.advance(500 * time.Millisecond)
	if len(r.rx.audioSeqs) <= first {
		t.Fatal("audio did not resume after repeated resume")
	}
}
