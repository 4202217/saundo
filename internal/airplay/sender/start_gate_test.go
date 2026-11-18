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

// TestStartGateThreeMembersOneEpoch pins the cohort case: N > 2 members seal
// one epoch on the last arrival, and the one trim covers the whole fanout.
func TestStartGateThreeMembersOneEpoch(t *testing.T) {
	trim := &recordingTrimmer{}
	gate := NewStartGate(StartGateConfig{Members: 3, Cushion: 50 * time.Millisecond, Trim: trim})
	for i := 0; i < 2; i++ {
		if err := gate.Arrive(gateEpochBase.Add(time.Duration(i)*time.Millisecond), 48000); err != nil {
			t.Fatal(err)
		}
		if gate.Sealed() {
			t.Fatalf("sealed after %d of 3 arrivals", i+1)
		}
	}
	if _, ok := gate.Begin(gateEpochBase); ok {
		t.Fatal("began before the third member arrived")
	}
	seal := gateEpochBase.Add(10 * time.Millisecond)
	if err := gate.Arrive(seal, 48000); err != nil {
		t.Fatal(err)
	}
	if !gate.Sealed() || !gate.Epoch().Equal(seal) {
		t.Fatalf("sealed=%v epoch=%v, want %v", gate.Sealed(), gate.Epoch(), seal)
	}
	if epoch, ok := gate.Begin(seal); !ok || !epoch.Equal(seal) {
		t.Fatalf("begin = %v %v, want %v true", epoch, ok, seal)
	}
	want := int(48000*0.05) * 2
	if len(trim.calls) != 1 || trim.calls[0] != want {
		t.Fatalf("trims = %v, want one trim of %d samples", trim.calls, want)
	}
}

// TestStartGateFollowsARateChangeBetweenArrivals pins the capture-update case:
// a rate arriving after the first member must reach the open gate, so the next
// member on the new rate seals instead of failing the mismatch check. Once
// sealed, the rate the cushion was computed from is fixed.
func TestStartGateFollowsARateChangeBetweenArrivals(t *testing.T) {
	trim := &recordingTrimmer{}
	gate := NewStartGate(StartGateConfig{Members: 2, Cushion: 50 * time.Millisecond, Trim: trim})
	if err := gate.Arrive(gateEpochBase, 48000); err != nil {
		t.Fatalf("first arrive: %v", err)
	}
	gate.SetInputRate(44100)
	seal := gateEpochBase.Add(time.Millisecond)
	if err := gate.Arrive(seal, 44100); err != nil {
		t.Fatalf("a capture-rate update between arrivals failed the second member: %v", err)
	}
	if !gate.Sealed() {
		t.Fatal("the second arrival did not seal the gate")
	}
	gate.SetInputRate(32000) // sealed: the cushion stays on the sealed rate
	if _, ok := gate.Begin(seal); !ok {
		t.Fatal("gate did not begin after sealing")
	}
	want := int(44100*0.05) * 2
	if len(trim.calls) != 1 || trim.calls[0] != want {
		t.Fatalf("trims = %v, want one trim of %d samples at the updated rate", trim.calls, want)
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
