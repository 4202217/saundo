package sender

import (
	"testing"
	"time"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/audio/ring"
)

func TestStatsCountRetransmitsServed(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	r.advance(500 * time.Millisecond)
	if len(r.rx.audioSeqs) == 0 {
		t.Fatal("no audio packets")
	}
	if got := r.sender.Stats().Retransmits; got != 0 {
		t.Fatalf("retransmits before a request = %d, want 0", got)
	}
	lost := r.rx.audioSeqs[3]
	r.emit(raopio.Event{Kind: raopio.EventUDPDatagram, UDP: raopio.UDPControl,
		Data: []byte{0x80, 0x55, 0, 1, byte(lost >> 8), byte(lost), 0, 1},
		Peer: raopio.Endpoint{IP: "192.168.1.20", Port: 6001}})
	r.step()
	if got := r.sender.Stats().Retransmits; got != 1 {
		t.Fatalf("retransmits served = %d, want 1", got)
	}
	if got := r.sender.Stats().SentBlocks; got == 0 {
		t.Fatal("sent blocks did not count")
	}
}

func TestStatsLastSyncTracksTimingPackets(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	first := r.sender.Stats().LastSync
	if first.IsZero() {
		t.Fatal("first sync packet did not record a time")
	}
	r.advance(1100 * time.Millisecond)
	later := r.sender.Stats().LastSync
	if !later.After(first) {
		t.Fatalf("last sync = %v, want after %v", later, first)
	}
}

func TestStatsCountUnderrunsAndBuffer(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb := ring.New(44100 * 2 * 2)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	r.advance(100 * time.Millisecond)
	stats := r.sender.Stats()
	if stats.SentBlocks == 0 {
		t.Fatal("no blocks sent")
	}
	if stats.Underruns == 0 {
		t.Fatal("an empty ring produced no underrun")
	}
	if !stats.BufferValid {
		t.Fatal("buffer read as unknown while streaming")
	}
	if stats.BufferMS != 0 {
		t.Fatalf("empty buffer = %f ms, want 0", stats.BufferMS)
	}

	rb.Push(make([]int16, 4410*2))
	stats = r.sender.Stats()
	if !stats.BufferValid {
		t.Fatal("buffer read as unknown after a push")
	}
	if stats.BufferMS < 99 || stats.BufferMS > 101 {
		t.Fatalf("buffer = %f ms, want about 100", stats.BufferMS)
	}
}

func TestStatsCountLatePacerTicks(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	rb, _ := fillRing(44100 * 4)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)

	// Move the clock two seconds without ticking: the capped burst can repay
	// only 16 packets, so the tick ends behind the wall clock.
	r.now = r.now.Add(2 * time.Second)
	r.sender.Tick()
	if got := r.sender.Stats().LateTicks; got == 0 {
		t.Fatal("a stalled tick did not count as late")
	}
}

func TestStats48kFullRingIsNotAnUnderrun(t *testing.T) {
	r := newRig(t, modeTransient, device.AuthHAPTransient, true, nil)
	r.sender.SetInputFormat(48000)
	rb, _ := fillRing(44100 * 4)
	r.sender.AttachRing(rb)
	r.start()
	requireLaunched(t, r)
	// The start gate trimmed the ring to the 50 ms cushion. Refill it with two
	// seconds at the capture rate so the sender has input for the run.
	rb.Reset()
	rb.Push(make([]int16, 48000*2*2))
	r.advance(500 * time.Millisecond)
	if got := r.sender.Stats().Underruns; got != 0 {
		t.Fatalf("a full 48 kHz ring counted %d underruns", got)
	}
}
