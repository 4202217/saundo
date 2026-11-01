//go:build windows

package capture

import (
	"runtime"
	"testing"
	"time"
	"unsafe"
)

// findRenderingPID returns the process currently rendering audio.
func findRenderingPID(t *testing.T) uint32 {
	t.Helper()
	en, err := newEnumerator()
	if err != nil {
		t.Fatal(err)
	}
	defer release(en)
	var coll uintptr
	if hr := comCall(en, 3, eRender, deviceStateActive, uintptr(unsafe.Pointer(&coll))); failed(hr) {
		t.Fatal("enum endpoints")
	}
	defer release(coll)
	var n uint32
	comCall(coll, 3, uintptr(unsafe.Pointer(&n)))
	for i := uint32(0); i < n; i++ {
		var dev uintptr
		if hr := comCall(coll, 4, uintptr(i), uintptr(unsafe.Pointer(&dev))); failed(hr) {
			continue
		}
		var manager uintptr
		if hr := comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioSessionManager2)), clsctxAll, 0,
			uintptr(unsafe.Pointer(&manager))); failed(hr) {
			release(dev)
			continue
		}
		var sessions uintptr
		if hr := comCall(manager, 5, uintptr(unsafe.Pointer(&sessions))); failed(hr) {
			release(manager)
			release(dev)
			continue
		}
		var count uint32
		comCall(sessions, 3, uintptr(unsafe.Pointer(&count)))
		for j := uint32(0); j < count; j++ {
			var ctrl uintptr
			if hr := comCall(sessions, 4, uintptr(j), uintptr(unsafe.Pointer(&ctrl))); failed(hr) {
				continue
			}
			var state uint32
			comCall(ctrl, 3, uintptr(unsafe.Pointer(&state)))
			var ctrl2 uintptr
			if hr := comCall(ctrl, 0, uintptr(unsafe.Pointer(&iidIAudioSessionControl2)), uintptr(unsafe.Pointer(&ctrl2))); failed(hr) {
				release(ctrl)
				continue
			}
			var pid uint32
			comCall(ctrl2, 14, uintptr(unsafe.Pointer(&pid)))
			release(ctrl2)
			release(ctrl)
			if state == 1 && pid != 0 {
				return pid
			}
		}
		release(sessions)
		release(manager)
		release(dev)
	}
	return 0
}

// TestProbeAppInitMatrix tries Initialize parameter combinations against a
// process that is actively rendering audio and prints which succeed.
func TestProbeAppInitMatrix(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	balance, err := comInit()
	if err != nil {
		t.Fatal(err)
	}
	defer balance()
	apps, err := AudioApplications()
	if err != nil {
		t.Fatal(err)
	}
	pid := uint32(0)
	for _, a := range apps {
		t.Logf("app pid=%d active=%v %s", a.PID, a.Active, a.Name)
		if a.Active {
			pid = a.PID
		}
	}
	if pid == 0 {
		t.Skip("nothing is rendering")
	}
	t.Logf("probing pid=%d", pid)
	client, err := activateProcessLoopback(pid)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	defer release(client)

	type combo struct {
		name   string
		tag    uint16
		bits   uint16
		align  uint32
		avg    uint32
		flags  uintptr
		buffer uintptr
	}
	combos := []combo{
		{"float32 loopback event buf0", 3, 32, 8, 48000 * 8, audclntStreamFlagsLoopback | audclntStreamFlagsEventCB, 0},
		{"float32 loopback event buf200000", 3, 32, 8, 48000 * 8, audclntStreamFlagsLoopback | audclntStreamFlagsEventCB, 200000},
		{"pcm16 loopback event auto buf200000", 1, 16, 4, 48000 * 4, audclntStreamFlagsLoopback | audclntStreamFlagsEventCB | appStreamAutoConvert, 200000},
		{"pcm16 event auto buf200000 (no loopback)", 1, 16, 4, 48000 * 4, audclntStreamFlagsEventCB | appStreamAutoConvert, 200000},
		{"float32 event noflag buf0", 3, 32, 8, 48000 * 8, audclntStreamFlagsEventCB, 0},
	}
	for i, c := range combos {
		format := struct {
			tag            uint16
			channels       uint16
			samplesPerSec  uint32
			avgBytesPerSec uint32
			blockAlign     uint16
			bitsPerSample  uint16
		}{tag: c.tag, channels: 2, samplesPerSec: 48000, avgBytesPerSec: c.avg, blockAlign: uint16(c.align), bitsPerSample: c.bits}
		hr := comCall(client, 3, audclntShareModeShared, c.flags, c.buffer, 0, uintptr(unsafe.Pointer(&format)), 0)
		t.Logf("combo %d %s -> HRESULT %#08x", i, c.name, uint32(hr))
		time.Sleep(20 * time.Millisecond)
	}
}
