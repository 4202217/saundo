//go:build windows && muteprobe

package capture

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"saundo/internal/audio/ring"
)

// This probe answers one question before the mute-on-connect feature ships:
// does muting the render endpoint also silence the loopback capture on this
// machine? The answer is driver-dependent, so it is a manual hardware test,
// not a CI test. Run it with:
//
//	go test -tags muteprobe ./internal/capture -run TestLoopbackSurvivesEndpointMute -v -count=1

var (
	winmmProbe     = syscall.NewLazyDLL("winmm.dll")
	procPlaySoundW = winmmProbe.NewProc("PlaySoundW")
)

const (
	sndAsync     = 0x0001
	sndNoDefault = 0x0002
	sndLoop      = 0x0008
	sndFilename  = 0x00020000
)

func TestLoopbackSurvivesEndpointMute(t *testing.T) {
	if testing.Short() {
		t.Skip("hardware probe")
	}
	wav := filepath.Join(t.TempDir(), "tone.wav")
	if err := writeProbeTone(wav, 48000, 2, 3*time.Second); err != nil {
		t.Fatal(err)
	}

	r := ring.New(48000 * 2 * 8)
	src, err := Start(Config{Ring: r})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	defer src.Stop()
	time.Sleep(300 * time.Millisecond)
	r.Reset()

	// Force the endpoint audible so the probe measures the tone, then restore
	// whatever the user had at the end.
	previous, err := EndpointMuted("")
	if err != nil {
		t.Fatalf("read mute state: %v", err)
	}
	if err := SetEndpointMuted("", false); err != nil {
		t.Fatalf("unmute: %v", err)
	}
	defer SetEndpointMuted("", previous)

	p, _ := syscall.UTF16PtrFromString(wav)
	if ok, _, err := procPlaySoundW.Call(uintptr(unsafe.Pointer(p)), 0,
		sndFilename|sndAsync|sndLoop|sndNoDefault); ok == 0 {
		t.Fatalf("PlaySound: %v", err)
	}
	defer procPlaySoundW.Call(0, 0, 0)

	time.Sleep(700 * time.Millisecond)
	unmuted := drainProbeRMS(r)

	if err := SetEndpointMuted("", true); err != nil {
		t.Fatalf("mute: %v", err)
	}
	time.Sleep(700 * time.Millisecond)
	muted := drainProbeRMS(r)

	if err := SetEndpointMuted("", previous); err != nil {
		t.Fatalf("restore: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	restored := drainProbeRMS(r)

	t.Logf("RMS unmuted=%.4f muted=%.4f restored=%.4f previousMute=%v",
		unmuted, muted, restored, previous)

	if unmuted < 0.01 {
		t.Skipf("no audible source captured (RMS %.4f); cannot judge on this device", unmuted)
	}
	if muted < unmuted*0.1 {
		t.Logf("RESULT: endpoint mute silences loopback capture on this device")
		return
	}
	t.Logf("RESULT: loopback capture survives endpoint mute on this device")
}

func writeProbeTone(path string, rate, channels int, d time.Duration) error {
	frames := int(float64(rate) * d.Seconds())
	dataSize := frames * channels * 2
	b := make([]byte, 44+dataSize)
	copy(b[0:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], uint32(36+dataSize))
	copy(b[8:12], "WAVE")
	copy(b[12:16], "fmt ")
	binary.LittleEndian.PutUint32(b[16:20], 16)
	binary.LittleEndian.PutUint16(b[20:22], 1)
	binary.LittleEndian.PutUint16(b[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(b[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:32], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(b[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(b[34:36], 16)
	copy(b[36:40], "data")
	binary.LittleEndian.PutUint32(b[40:44], uint32(dataSize))
	for i := 0; i < frames; i++ {
		v := int16(0.25 * 32767 * math.Sin(2*math.Pi*440*float64(i)/float64(rate)))
		for ch := 0; ch < channels; ch++ {
			binary.LittleEndian.PutUint16(b[44+(i*channels+ch)*2:], uint16(v))
		}
	}
	return os.WriteFile(path, b, 0o600)
}

func drainProbeRMS(r *ring.Buffer) float64 {
	n := r.Available()
	if n <= 0 {
		return 0
	}
	buf := make([]int16, n)
	got := r.Pop(buf)
	if got <= 0 {
		return 0
	}
	var sum float64
	for _, s := range buf[:got] {
		v := float64(s) / 32768
		sum += v * v
	}
	return math.Sqrt(sum / float64(got))
}
