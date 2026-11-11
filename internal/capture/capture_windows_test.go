//go:build windows

package capture

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"
	"unsafe"

	"saundo/internal/audio/ring"
)

// TestLoopbackOpen exercises the real WASAPI loopback open on this machine.
// A silent desktop may deliver no packets; that alone is not a failure.
func TestLoopbackOpen(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	eps, err := Endpoints()
	if err != nil {
		t.Fatalf("Endpoints: %v", err)
	}
	if len(eps) == 0 {
		t.Fatal("no active render endpoints")
	}
	def, err := DefaultEndpoint()
	if err != nil {
		t.Fatalf("DefaultEndpoint: %v", err)
	}
	t.Logf("%d endpoints, default %q (%s)", len(eps), def.Name, def.ID)

	var mu sync.Mutex
	rate := 0
	device := ""
	var errs []error
	src, err := Start(Config{
		Ring:     ring.New(44100 * 4),
		OnFormat: func(r int) { mu.Lock(); rate = r; mu.Unlock() },
		OnDevice: func(n string) { mu.Lock(); device = n; mu.Unlock() },
		OnError:  func(e error) { mu.Lock(); errs = append(errs, e); mu.Unlock() },
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer src.Stop()

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	if rate < 8000 || rate > 192000 {
		mu.Unlock()
		t.Fatalf("opened at rate %d, want 8000..192000", rate)
	}
	if device == "" {
		mu.Unlock()
		t.Fatal("no device name reported on open")
	}
	st := src.Stats()
	t.Logf("stats: %+v, %d OnError callbacks", st, len(errs))
	mu.Unlock()

	if !st.Running {
		t.Error("Stats.Running false while the stream is open")
	}
	if st.Device != device {
		t.Errorf("Stats.Device = %q, want %q", st.Device, device)
	}
	if st.Rate != rate {
		t.Errorf("Stats.Rate = %d, want %d", st.Rate, rate)
	}
	if st.Dropped > st.Frames {
		t.Errorf("Stats.Dropped %d exceeds captured frames %d", st.Dropped, st.Frames)
	}

	src.Stop()
	src.Stop() // idempotent
	if st := src.Stats(); st.Running {
		t.Error("Stats.Running true after Stop")
	}
}

// TestLoopbackReceivesSamples plays a quiet tone through the default device
// and checks that loopback capture delivers real samples into the ring.
func TestLoopbackReceivesSamples(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	r := ring.New(44100 * 4)
	var mu sync.Mutex
	var errs []error
	src, err := Start(Config{
		Ring:    r,
		OnError: func(e error) { mu.Lock(); errs = append(errs, e); mu.Unlock() },
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer src.Stop()

	if err := playTone(400 * time.Millisecond); err != nil {
		t.Fatalf("playTone: %v", err)
	}

	mu.Lock()
	nErrs := len(errs)
	errTexts := make([]string, len(errs))
	for i, e := range errs {
		errTexts[i] = e.Error()
	}
	mu.Unlock()
	st := src.Stats()
	t.Logf("stats after tone: %+v, %d capture errors %v", st, nErrs, errTexts)
	if st.Frames == 0 {
		t.Fatal("no frames captured while a tone played")
	}
	if nErrs != 0 {
		t.Fatalf("normal playback reported %d capture error(s): %v", nErrs, errTexts)
	}

	peak := int16(0)
	buf := make([]int16, 4096)
	for {
		n := r.Pop(buf)
		if n == 0 {
			break
		}
		for _, s := range buf[:n] {
			if s > peak {
				peak = s
			}
			if -s > peak {
				peak = -s
			}
		}
	}
	t.Logf("peak captured sample: %d", peak)
	if peak < 1000 {
		t.Errorf("peak captured sample %d is too quiet for the rendered tone", peak)
	}
}

type waveFormatExtensible struct {
	formatTag      uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	size           uint16
	validBits      uint16
	channelMask    uint32
	subFormat      guid
}

var ksDataFormatSubtypeFloat = guid{0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
var ksDataFormatSubtypePCM = guid{0x00000001, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}

// TestParseMixFormatExtensible pins the SubFormat offset. A
// WAVEFORMATEXTENSIBLE appends wValidBitsPerSample (2 bytes) and
// dwChannelMask (4 bytes) after cbSize, so the 16-byte GUID starts at byte 24.
// Reading the tag from byte 18 instead would pick up the valid-bits word and
// reject every extensible mix format.
func TestParseMixFormatExtensible(t *testing.T) {
	cases := []struct {
		name       string
		sub        guid
		bits       uint16
		blockAlign uint16
	}{
		{"pcm16", ksDataFormatSubtypePCM, 16, 4},
		{"float32", ksDataFormatSubtypeFloat, 32, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := waveFormatExtensible{
				formatTag:      0xfffe,
				channels:       2,
				samplesPerSec:  48000,
				avgBytesPerSec: 48000 * uint32(c.blockAlign),
				blockAlign:     c.blockAlign,
				bitsPerSample:  c.bits,
				size:           22,
				validBits:      c.bits,
				channelMask:    0x3,
				subFormat:      c.sub,
			}
			_, rate, blockAlign, err := parseMixFormat(uintptr(unsafe.Pointer(&f)))
			if err != nil {
				t.Fatalf("parseMixFormat: %v", err)
			}
			if rate != 48000 || blockAlign != int(c.blockAlign) {
				t.Fatalf("rate=%d blockAlign=%d, want 48000/%d", rate, blockAlign, c.blockAlign)
			}
		})
	}
}

var iidIAudioRenderClient = guid{0xf294acfc, 0x3146, 0x4483, [8]byte{0xa7, 0xbf, 0xad, 0xdc, 0xa7, 0xc2, 0x60, 0xe2}}

// playTone renders a quiet 440 Hz sine on the default device for d, at a
// fixed rate the engine converts into the mix format.
func playTone(d time.Duration) error {
	balance, err := comInit()
	if err != nil {
		return err
	}
	defer balance()
	en, err := newEnumerator()
	if err != nil {
		return err
	}
	defer release(en)
	dev, err := defaultDevice(en)
	if err != nil {
		return err
	}
	defer release(dev)
	var client uintptr
	if hr := comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0,
		uintptr(unsafe.Pointer(&client))); failed(hr) {
		return hresult("IMMDevice.Activate", hr)
	}
	defer release(client)
	f := waveFormatExtensible{
		formatTag:      0xfffe,
		channels:       2,
		samplesPerSec:  44100,
		avgBytesPerSec: 44100 * 2 * 4,
		blockAlign:     2 * 4,
		bitsPerSample:  32,
		size:           22,
		validBits:      32,
		channelMask:    0x3,
		subFormat:      ksDataFormatSubtypeFloat,
	}
	flags := uintptr(audclntStreamFlagsEventCB | audclntStreamFlagsNoPersist | 0x80000000) // + AUTOCONVERT
	if hr := comCall(client, 3, audclntShareModeShared, flags, 0, 0, uintptr(unsafe.Pointer(&f)), 0); failed(hr) {
		return hresult("IAudioClient.Initialize", hr)
	}
	var bufFrames uint32
	if hr := comCall(client, 4, uintptr(unsafe.Pointer(&bufFrames))); failed(hr) {
		return hresult("IAudioClient.GetBufferSize", hr)
	}
	var render uintptr
	if hr := comCall(client, 14, uintptr(unsafe.Pointer(&iidIAudioRenderClient)),
		uintptr(unsafe.Pointer(&render))); failed(hr) {
		return hresult("IAudioClient.GetService(IAudioRenderClient)", hr)
	}
	defer release(render)
	ev, _, callErr := procCreateEventW.Call(0, 0, 0, 0)
	if ev == 0 {
		return fmt.Errorf("CreateEvent failed: %v", callErr)
	}
	defer procCloseHandle.Call(ev)
	if hr := comCall(client, 13, ev); failed(hr) {
		return hresult("IAudioClient.SetEventHandle", hr)
	}
	if hr := comCall(client, 10); failed(hr) { // Start
		return hresult("IAudioClient.Start", hr)
	}
	defer comCall(client, 11) // Stop

	const chunk = 441 // 10 ms at 44100
	phase := 0.0
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		var padding uint32
		if hr := comCall(client, 6, uintptr(unsafe.Pointer(&padding))); failed(hr) { // GetCurrentPadding
			return hresult("IAudioClient.GetCurrentPadding", hr)
		}
		if bufFrames-padding >= chunk {
			var p uintptr
			if hr := comCall(render, 3, chunk, uintptr(unsafe.Pointer(&p))); failed(hr) { // GetBuffer
				return hresult("IAudioRenderClient.GetBuffer", hr)
			}
			out := unsafe.Slice((*float32)(native(p)), chunk*2)
			for i := 0; i < chunk; i++ {
				v := float32(0.15 * math.Sin(phase))
				phase += 2 * math.Pi * 440 / 44100
				out[2*i], out[2*i+1] = v, v
			}
			if hr := comCall(render, 4, chunk, 0); failed(hr) { // ReleaseBuffer
				return hresult("IAudioRenderClient.ReleaseBuffer", hr)
			}
		}
		procWaitForSingleObject.Call(ev, 100)
	}
	return nil
}
