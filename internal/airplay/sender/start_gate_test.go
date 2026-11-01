package sender

import (
	"testing"
	"time"
)

type recordingTrimmer struct{ calls []int }

func (r *recordingTrimmer) KeepNewest(newest int) int {
	r.calls = append(r.calls, newest)
	return 0
}

var gateEpochBase = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func TestStartGateSealsOnLastArrivalAndTrimsOnce(t *testing.T) {
	trim := &recordingTrimmer{}
	gate := NewStartGate(StartGateConfig{Members: 2, Cushion: 50 * time.Millisecond, Trim: trim})

	if err := gate.Arrive(gateEpochBase, 48000); err != nil {
		t.Fatalf("first arrive: %v", err)
	}
	if gate.Sealed() {
		t.Fatal("sealed after one of two arrivals")
	}
	if _, ok := gate.Begin(gateEpochBase); ok {
		t.Fatal("began before every member arrived")
	}
	if len(trim.calls) != 0 {
		t.Fatalf("trimmed before the seal: %v", trim.calls)
	}

	epoch := gateEpochBase.Add(150 * time.Millisecond)
	if err := gate.Arrive(epoch, 48000); err != nil {
		t.Fatalf("second arrive: %v", err)
	}
	if !gate.Sealed() || !gate.Epoch().Equal(epoch) {
		t.Fatalf("sealed=%v epoch=%v, want %v", gate.Sealed(), gate.Epoch(), epoch)
	}
	if _, ok := gate.Begin(epoch.Add(-time.Millisecond)); ok {
		t.Fatal("began before the epoch")
	}
	got, ok := gate.Begin(epoch)
	if !ok || !got.Equal(epoch) {
		t.Fatalf("begin = %v %v, want %v true", got, ok, epoch)
	}
	want := int(48000*0.05) * 2
	if len(trim.calls) != 1 || trim.calls[0] != want {
		t.Fatalf("trims = %v, want one call with %d samples", trim.calls, want)
	}
	if again, ok := gate.Begin(epoch.Add(time.Second)); !ok || !again.Equal(epoch) {
		t.Fatalf("second begin = %v %v, want the same epoch", again, ok)
	}
	if len(trim.calls) != 1 {
		t.Fatalf("begin trimmed twice: %v", trim.calls)
	}
}

func TestStartGateRateMismatchFailsClosed(t *testing.T) {
	gate := NewStartGate(StartGateConfig{Members: 2, Cushion: DefaultStartCushion})
	if err := gate.Arrive(gateEpochBase, 48000); err != nil {
		t.Fatalf("first arrive: %v", err)
	}
	if err := gate.Arrive(gateEpochBase, 44100); err == nil {
		t.Fatal("mismatched input rate accepted")
	}
	if gate.Sealed() {
		t.Fatal("a mismatched arrival sealed the gate")
	}
	// The rejected arrival did not count, so a matching one still seals.
	if err := gate.Arrive(gateEpochBase, 48000); err != nil {
		t.Fatalf("matching arrive after a mismatch: %v", err)
	}
	if !gate.Sealed() {
		t.Fatal("gate did not seal after the member count was reached")
	}
}

func TestStartGateZeroMemberClamp(t *testing.T) {
	gate := NewStartGate(StartGateConfig{Members: 0, Cushion: DefaultStartCushion})
	if err := gate.Arrive(gateEpochBase, 44100); err != nil {
		t.Fatalf("arrive: %v", err)
	}
	if !gate.Sealed() {
		t.Fatal("a zero-member gate did not seal on the first arrival")
	}
	if epoch, ok := gate.Begin(gateEpochBase); !ok || !epoch.Equal(gateEpochBase) {
		t.Fatalf("begin = %v %v, want %v true", epoch, ok, gateEpochBase)
	}
}

func TestStartGateZeroCushionTrimsEverything(t *testing.T) {
	trim := &recordingTrimmer{}
	gate := NewStartGate(StartGateConfig{Members: 1, Cushion: -time.Second, Trim: trim})
	if err := gate.Arrive(gateEpochBase, 44100); err != nil {
		t.Fatalf("arrive: %v", err)
	}
	if _, ok := gate.Begin(gateEpochBase); !ok {
		t.Fatal("gate did not begin")
	}
	if len(trim.calls) != 1 || trim.calls[0] != 0 {
		t.Fatalf("trims = %v, want one zero-sample trim", trim.calls)
	}
}

func TestStartGateNilTrimStillSuppliesTheEpoch(t *testing.T) {
	gate := NewStartGate(StartGateConfig{Members: 1, Cushion: DefaultStartCushion})
	if err := gate.Arrive(gateEpochBase, 44100); err != nil {
		t.Fatalf("arrive: %v", err)
	}
	if epoch, ok := gate.Begin(gateEpochBase); !ok || !epoch.Equal(gateEpochBase) {
		t.Fatalf("begin = %v %v, want %v true", epoch, ok, gateEpochBase)
	}
}

func TestStartGateArriveAfterSealIsNoOp(t *testing.T) {
	gate := NewStartGate(StartGateConfig{Members: 1, Cushion: DefaultStartCushion})
	if err := gate.Arrive(gateEpochBase, 44100); err != nil {
		t.Fatalf("arrive: %v", err)
	}
	if err := gate.Arrive(gateEpochBase.Add(time.Second), 48000); err != nil {
		t.Fatalf("late arrival errored: %v", err)
	}
	if !gate.Epoch().Equal(gateEpochBase) {
		t.Fatalf("late arrival moved the epoch to %v", gate.Epoch())
	}
}
