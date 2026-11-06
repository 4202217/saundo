package analyze

import (
	"math"
	"testing"
	"time"
)

const testRate = 48000

func tone(amp, freq float64, frames int) []int16 {
	out := make([]int16, frames*2)
	for i := 0; i < frames; i++ {
		v := int16(amp * 32767 * math.Sin(2*math.Pi*freq*float64(i)/testRate))
		out[2*i], out[2*i+1] = v, v
	}
	return out
}

func TestMeterLevelBandsAndDelay(t *testing.T) {
	m := New(testRate)
	start := time.Unix(1000, 0)
	loud := tone(0.5, 1000, 480)
	for k := 0; k < 100; k++ {
		m.Push(loud, start.Add(time.Duration(k)*10*time.Millisecond))
	}
	frame, ok := m.At(start.Add(time.Second), 0)
	if !ok {
		t.Fatal("no fresh frame")
	}
	if frame.Level < 0.5 {
		t.Fatalf("level = %v, want a loud frame", frame.Level)
	}
	var peak float32
	for _, b := range frame.Bands {
		if b > peak {
			peak = b
		}
	}
	if peak < 0.2 {
		t.Fatalf("bands = %v, want a rising band", frame.Bands)
	}

	// Fill the ring with silence, then read back the loud epoch.
	silent := make([]int16, 960)
	for k := 0; k < 100; k++ {
		m.Push(silent, start.Add(time.Second+time.Duration(k)*10*time.Millisecond))
	}
	delayed, ok := m.At(start.Add(1500*time.Millisecond), 400*time.Millisecond)
	if !ok || delayed.Level < 0.3 {
		t.Fatalf("delayed = %+v ok=%v, want the loud epoch", delayed, ok)
	}

	for k := 0; k < 200; k++ {
		m.Push(silent, start.Add(1500*time.Millisecond+time.Duration(k)*10*time.Millisecond))
	}
	quiet, ok := m.At(start.Add(3600*time.Millisecond), 0)
	if !ok || quiet.Level > 0.1 {
		t.Fatalf("quiet = %+v ok=%v, want a decayed level", quiet, ok)
	}
}

func noise(amp int32, frames int, seed uint32) []int16 {
	out := make([]int16, frames*2)
	x := seed
	for i := 0; i < frames; i++ {
		x = x*1664525 + 1013904223
		v := int16(int32(x>>16)%(2*amp+1) - amp)
		out[2*i], out[2*i+1] = v, v
	}
	return out
}

func TestMeterGatesNoiseBelowTheFloor(t *testing.T) {
	m := New(testRate)
	start := time.Unix(2000, 0)
	quiet := noise(20, 480, 1) // about -69 dBFS
	for k := 0; k < 300; k++ {
		m.Push(quiet, start.Add(time.Duration(k)*10*time.Millisecond))
	}
	frame, ok := m.At(start.Add(3*time.Second), 0)
	if !ok {
		t.Fatal("no fresh frame")
	}
	for i, b := range frame.Bands {
		if b > 0.05 {
			t.Fatalf("band %d = %v for noise below the floor, want it gated", i, b)
		}
	}
}

func TestMeterQuietToneKeepsBarsLow(t *testing.T) {
	m := New(testRate)
	start := time.Unix(3000, 0)
	quiet := tone(0.01, 600, 480) // about -40 dBFS
	for k := 0; k < 300; k++ {
		m.Push(quiet, start.Add(time.Duration(k)*10*time.Millisecond))
	}
	frame, ok := m.At(start.Add(3*time.Second), 0)
	if !ok {
		t.Fatal("no fresh frame")
	}
	if b := frame.Bands[2]; b < 0.1 || b > 0.6 {
		t.Fatalf("600 Hz band = %v for a -40 dBFS tone, want the compressed low range", b)
	}
}

func TestFollowIgnoresSubBarChanges(t *testing.T) {
	if got := follow(0.5, 0.51, 0.45, 0.08); got != 0.5 {
		t.Fatalf("follow = %v for a change below the deadband, want it still", got)
	}
	if got := follow(0.5, 0.6, 0.45, 0.08); got <= 0.5 {
		t.Fatalf("follow = %v, want a rise for a larger change", got)
	}
	if got := follow(0.02, 0, 0.45, 0.08); got != 0 {
		t.Fatalf("follow = %v, want silence to settle at zero", got)
	}
}

func TestMeterWithoutRateOnlyMeasuresLevel(t *testing.T) {
	m := New(0)
	now := time.Now()
	for k := 0; k < 10; k++ {
		m.Push(tone(0.5, 1000, 480), now.Add(time.Duration(k)*10*time.Millisecond))
	}
	frame, ok := m.At(now.Add(100*time.Millisecond), 0)
	if !ok {
		t.Fatal("no frame")
	}
	if frame.Level < 0.5 {
		t.Fatalf("level = %v", frame.Level)
	}
	for i, b := range frame.Bands {
		if b != 0 {
			t.Fatalf("band %d = %v without a sample rate", i, b)
		}
	}
}
