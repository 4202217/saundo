package sender

import (
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
