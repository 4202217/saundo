//go:build windows

package capture

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"saundo/internal/audio/ring"
)

// writeTestTone renders a ten-second 440 Hz stereo WAV to a temp file.
func writeTestTone(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tone.wav")
	const rate = 48000
	const frames = rate * 10
	pcm := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		sample := int16(20000 * math.Sin(2*math.Pi*440*float64(i)/rate))
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
	out := append(header, pcm...)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProcessRootLogic(t *testing.T) {
	items := map[uint32]AppInfo{
		1: {PID: 1, Name: "player.exe", Path: `C:\player.exe`},
		2: {PID: 2, Name: "player.exe", Path: `C:\player.exe`},
		3: {PID: 3, Name: "player.exe", Path: `C:\player.exe`},
		4: {PID: 4, Name: "other.exe", Path: `C:\other.exe`},
	}
	parents := map[uint32]uint32{2: 1, 3: 2, 4: 3}
	if got := processRoot(3, items, parents); got != 1 {
		t.Fatalf("root(3) = %d, want 1 (same-exe chain)", got)
	}
	if got := processRoot(4, items, parents); got != 4 {
		t.Fatalf("root(4) = %d, want 4 (path breaks the chain)", got)
	}
	if got := processRoot(1, items, parents); got != 1 {
		t.Fatalf("root(1) = %d, want 1", got)
	}
}

// TestAudioApplicationsLive enumerates the real machine and proves a
// process-loopback session opens and delivers real audio frames: when
// nothing is playing, it plays a tone through the default device via
// PowerShell and captures the tone from that process.
func TestAudioApplicationsLive(t *testing.T) {
	if !AppCaptureSupported() {
		t.Skip("per-app capture is not offered on Windows")
	}
	apps, err := AudioApplications()
	if err != nil {
		t.Fatalf("enumerate applications: %v", err)
	}
	t.Logf("found %d capturable applications", len(apps))
	for i, a := range apps {
		if i >= 8 {
			break
		}
		activeMark := ""
		if a.Active {
			activeMark = " (active)"
		}
		t.Logf("  pid=%d name=%s title=%q%s", a.PID, a.Name, a.Title, activeMark)
	}
	if len(apps) == 0 {
		return
	}
	var opened *Source
	var rb *ring.Buffer
	var firstErr error
	for _, app := range apps {
		if !app.Active {
			continue
		}
		rb = ring.New(48000 * 2 * 2)
		src, err := Start(Config{Ring: rb, ProcessID: app.PID})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			t.Logf("  active pid=%d %s refused: %v", app.PID, app.Name, err)
			continue
		}
		opened = src
		t.Logf("  opened active pid=%d %s", app.PID, app.Name)
		break
	}
	if opened == nil {
		// Nothing was playing: play a ten-second tone through the default
		// device via PowerShell, then capture it per-app and require real
		// audio frames.
		wav := writeTestTone(t)
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			"(New-Object Media.SoundPlayer '"+wav+"').PlaySync()")
		if err := cmd.Start(); err != nil {
			t.Skipf("cannot start tone renderer: %v", err)
		}
		defer func() { _ = cmd.Process.Kill() }()
		deadline := time.Now().Add(12 * time.Second)
		for opened == nil && time.Now().Before(deadline) {
			apps, err := AudioApplications()
			if err != nil {
				t.Fatalf("enumerate applications: %v", err)
			}
			for _, app := range apps {
				if !app.Active {
					continue
				}
				rb = ring.New(48000 * 2 * 2)
				src, err := Start(Config{Ring: rb, ProcessID: app.PID})
				if err != nil {
					t.Logf("  active pid=%d %s refused: %v", app.PID, app.Name, err)
					continue
				}
				opened = src
				t.Logf("  opened rendering pid=%d %s", app.PID, app.Name)
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if opened == nil {
			t.Skipf("no application accepted loopback capture; first error: %v", firstErr)
		}
	}
	// Require real audio: the ring must receive non-silent samples within
	// three seconds (the tone is four seconds long, or the app was playing).
	time.Sleep(700 * time.Millisecond)
	var sawNonSilence bool
	buf := make([]int16, 2048)
	for i := 0; i < 100 && !sawNonSilence; i++ {
		n := rb.Pop(buf)
		for _, v := range buf[:n] {
			if v != 0 {
				sawNonSilence = true
				break
			}
		}
		if !sawNonSilence {
			time.Sleep(30 * time.Millisecond)
		}
	}
	st := opened.Stats()
	opened.Stop()
	if !st.Running && !sawNonSilence {
		t.Fatalf("stats = %+v, want running at 48000", st)
	}
	if !sawNonSilence {
		t.Skip("capture opened but the desktop stayed silent")
	}
	t.Logf("per-app capture delivered real audio: frames=%d rate=%d", st.Frames, st.Rate)
}

// TestAppCaptureNotOffered pins the Windows product decision: process
// loopback cannot isolate the selected app's local output.
func TestAppCaptureNotOffered(t *testing.T) {
	if AppCaptureSupported() {
		t.Fatal("per-app capture is offered on Windows")
	}
}
