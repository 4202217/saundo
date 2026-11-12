//go:build darwin

package capture

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"saundo/internal/audio/ring"
	"saundo/internal/darwin"
)

// writeTestTone renders a stereo 440 Hz WAV to a temp file.
func writeTestTone(t *testing.T, seconds int) string {
	t.Helper()
	const rate = 48000
	frames := rate * seconds
	pcm := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		sample := int16(12000 * math.Sin(2*math.Pi*440*float64(i)/rate))
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(sample))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(sample))
	}
	header := make([]byte, 44)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(pcm)))
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], 2)
	binary.LittleEndian.PutUint32(header[24:], rate)
	binary.LittleEndian.PutUint32(header[28:], rate*4)
	binary.LittleEndian.PutUint16(header[32:], 4)
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(pcm)))
	path := filepath.Join(t.TempDir(), "tone.wav")
	if err := os.WriteFile(path, append(header, pcm...), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPerAppCaptureLive plays a tone through afplay and proves a tap on that
// process alone delivers real audio. It needs audio output and capture
// consent, so it runs only when SAUNDO_LIVE_APP_TAP=1.
func TestPerAppCaptureLive(t *testing.T) {
	if os.Getenv("SAUNDO_LIVE_APP_TAP") == "" {
		t.Skip("set SAUNDO_LIVE_APP_TAP=1 to capture a live tone")
	}
	cmd := exec.Command("afplay", "-v", "0.2", writeTestTone(t, 8))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	pid := uint32(cmd.Process.Pid)

	// afplay connects to the HAL when it starts rendering; wait for its
	// process object before tapping it.
	var obj darwin.ProcessObject
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		procs, err := darwin.ProcessObjects()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range procs {
			if p.PID == pid {
				obj = p
				break
			}
		}
		if obj.ID != 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if obj.ID == 0 {
		t.Fatalf("afplay pid %d never appeared as a Core Audio process", pid)
	}

	const rate = 48000
	rb := ring.New(rate * 2 * 2)
	src, err := Start(Config{Ring: rb, AppIDs: []string{fmt.Sprintf("pid:%d", pid)}})
	if err != nil {
		t.Fatalf("per-app capture: %v", err)
	}
	defer src.Stop()
	if st := activeIO.Load(); st == nil || !st.perApp {
		t.Fatal("no live per-app tap")
	}
	if got := src.Stats().Apps; len(got) != 1 || got[0] != fmt.Sprintf("pid:%d", pid) {
		t.Fatalf("captured apps = %v, want the selected pid", got)
	}
	if err := SetProcessMuted(true); err != nil {
		t.Fatalf("mute the captured app: %v", err)
	}
	st := waitStream(t, true)
	t.Logf("muted tap %d", st.tapID)
	time.Sleep(700 * time.Millisecond)
	saw := false
	buf := make([]int16, 2048)
	for i := 0; i < 100 && !saw; i++ {
		n := rb.Pop(buf)
		for _, v := range buf[:n] {
			if v != 0 {
				saw = true
				break
			}
		}
		if !saw {
			time.Sleep(30 * time.Millisecond)
		}
	}
	stats := src.Stats()
	if !saw {
		t.Fatalf("per-app tap opened but stayed silent: %+v", stats)
	}
	if err := SetProcessMuted(false); err != nil {
		t.Fatalf("restore the captured app: %v", err)
	}
	st = waitStream(t, false)
	t.Logf("unmuted tap %d", st.tapID)
	t.Logf("per-app capture delivered audio: frames=%d rate=%d name=%q", stats.Frames, stats.Rate, stats.Device)
}

// waitStream waits for the capture loop to rebuild the tap with the wanted
// local-output behavior.
func waitStream(t *testing.T, muted bool) *stream {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := activeIO.Load(); st != nil && st.muted == muted {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no stream with muted=%v", muted)
	return nil
}
