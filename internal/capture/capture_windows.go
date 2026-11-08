// Copyright 2026 daikaiyu.
// SPDX-License-Identifier: GPL-3.0-only
//
// The WASAPI shared-mode loopback capture approach is ported from AirFlash
// (GPLv3 or later), native/airflash-engine/src/live.rs. See NOTICE.

//go:build windows

package capture

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"saundo/internal/audio/ring"
)

// Capture runs on one pinned goroutine in COM's multithreaded apartment. It
// opens the render endpoint's shared-mode loopback stream, converts packets
// to stereo int16, and drops them into the ring. When the default device
// changes or the stream breaks, it reopens with a short backoff.

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

type propertyKey struct {
	fmtid guid
	pid   uint32
}

// IIDs and the friendly-name property key, from mmdeviceapi.idl, audioclient.idl,
// propsys.idl, and functiondiscoverykeys_devpkey.h of the Windows SDK.
var (
	clsidMMDeviceEnumerator = guid{0xbcde0395, 0xe52f, 0x467c, [8]byte{0x8e, 0x3d, 0xc4, 0x57, 0x92, 0x91, 0x69, 0x2e}}
	iidIMMDeviceEnumerator  = guid{0xa95664d2, 0x9614, 0x4f35, [8]byte{0xa7, 0x46, 0xde, 0x8d, 0xb6, 0x36, 0x17, 0xe6}}
	iidIMMDeviceCollection  = guid{0x0bd7a1be, 0x7a1a, 0x44db, [8]byte{0x83, 0x97, 0xcc, 0x53, 0x92, 0x38, 0x7b, 0x5e}}
	iidIAudioClient         = guid{0x1cb9ad4c, 0xdbfa, 0x4c32, [8]byte{0xb1, 0x78, 0xc2, 0xf5, 0x68, 0xa7, 0x03, 0xb2}}
	iidIAudioCaptureClient  = guid{0xc8adbd64, 0xe71e, 0x48a0, [8]byte{0xa4, 0xde, 0x18, 0x5c, 0x39, 0x5c, 0xd3, 0x17}}
	pkeyDeviceFriendlyName  = propertyKey{guid{0xa45c254e, 0xdf1c, 0x4efd, [8]byte{0x80, 0x20, 0x67, 0xd1, 0x46, 0xa8, 0x50, 0xe0}}, 14}

	ole32                   = syscall.NewLazyDLL("ole32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procCoInitializeEx      = ole32.NewProc("CoInitializeEx")
	procCoUninitialize      = ole32.NewProc("CoUninitialize")
	procCoCreateInstance    = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree       = ole32.NewProc("CoTaskMemFree")
	procPropVariantClear    = ole32.NewProc("PropVariantClear")
	procCreateEventW        = kernel32.NewProc("CreateEventW")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
)

const (
	clsctxAll           = 0x17
	coinitMultithreaded = 0
	eRender             = 0
	eConsole            = 0
	deviceStateActive   = 1
	stgmRead            = 0
	vtLPWSTR            = 31

	waveFormatExtensibleTag = 0xfffe

	audclntShareModeShared = 0
	// Loopback capture is event-driven: the engine signals when the render
	// side crosses a period, so Stop rechecks at least every waitTimeoutMs.
	audclntStreamFlagsLoopback  = 0x00020000
	audclntStreamFlagsEventCB   = 0x00040000
	audclntStreamFlagsNoPersist = 0x00080000

	audclntBufferFlagsSilent = 0x2

	waitObject0 = 0
	waitFailed  = 0xFFFFFFFF

	initBuffer100ns = 200000 // 20 ms, AirFlash's shared-mode loopback buffer
)

const (
	waitTimeoutMs   = 100
	maxEndpoints    = 256
	maxNameChars    = 4096
	maxPacketFrames = 1 << 24
)

// comCall calls method i of the COM interface obj. Pointers converted to
// uintptr in the call stay valid until it returns (go:uintptrescapes).
//
//go:uintptrescapes
func comCall(obj uintptr, i int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(native(obj))
	fn := *(*uintptr)(unsafe.Add(native(vtbl), i*int(unsafe.Sizeof(uintptr(0)))))
	var a [16]uintptr
	a[0] = obj
	n := copy(a[1:], args)
	r, _, _ := syscall.SyscallN(fn, a[:n+1]...)
	return r
}

// native converts a native address to a pointer.
func native(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func hresult(what string, hr uintptr) error {
	return fmt.Errorf("capture: %s failed (HRESULT %#08x)", what, uint32(hr))
}

func release(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2) // IUnknown.Release
	}
}

// comInit initializes COM multithreaded on the calling thread. The returned
// function balances a successful CoInitializeEx. A thread already in another
// apartment proceeds unbalanced; the in-proc MMDevice objects work from either.
func comInit() (func(), error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	if !failed(hr) { // S_OK or S_FALSE
		return func() { procCoUninitialize.Call() }, nil
	}
	if uint32(hr) == 0x80010106 { // RPC_E_CHANGED_MODE
		return func() {}, nil
	}
	return func() {}, hresult("CoInitializeEx", hr)
}

// Start opens the loopback capture and returns once the first stream is
// running. If the first open fails, so does Start.
func Start(cfg Config) (*Source, error) {
	if cfg.Ring == nil {
		return nil, errors.New("capture: Ring is required")
	}
	s := &Source{cfg: cfg, stop: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go s.run(ready)
	if err := <-ready; err != nil {
		<-s.done
		return nil, err
	}
	return s, nil
}

// run drives capture on a thread pinned for COM. It sends exactly one value
// on ready: nil once the first stream is open, or the reason it could not.
func (s *Source) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.done)

	balance, err := comInit()
	if err != nil {
		ready <- err
		return
	}
	defer balance()

	en, err := newEnumerator()
	if err != nil {
		ready <- err
		return
	}
	defer release(en)

	backoff := retryDelayFirst
	first := true
	for {
		st, err := openStream(en, s.cfg)
		if err != nil {
			if first {
				ready <- err
				return
			}
			s.reportError(err)
			if !s.wait(backoff) {
				return
			}
			backoff = min(backoff*2, retryDelayMax)
			continue
		}
		wasFirst := first
		first = false
		backoff = retryDelayFirst
		s.streamOpened(st.rate, st.name)
		if wasFirst {
			ready <- nil
		}

		err = st.pump(s, en, s.cfg.ProcessID == 0 && s.cfg.EndpointID == "")
		st.close()
		s.streamClosed()
		if err == nil || errors.Is(err, errStopped) {
			return
		}
		if errors.Is(err, errDefaultDeviceChanged) {
			backoff = retryDelayFirst
			continue
		}
		s.reportError(err)
		if !s.wait(backoff) {
			return
		}
		backoff = min(backoff*2, retryDelayMax)
	}
}

// stream is one open WASAPI loopback capture.
type stream struct {
	device     uintptr // IMMDevice
	client     uintptr // IAudioClient
	capture    uintptr // IAudioCaptureClient
	event      uintptr // handle
	id, name   string
	rate       int
	blockAlign int
	conv       converter
	scratch    []int16
}

// openStream opens and starts the loopback capture for the chosen endpoint
// (or the default render endpoint when id is empty), or for a process tree
// when the config selects application capture.
func openStream(en uintptr, cfg Config) (st *stream, err error) {
	if cfg.ProcessID != 0 {
		return openAppStream(en, cfg, cfg.ProcessID)
	}
	var dev uintptr
	if id := cfg.EndpointID; id == "" {
		dev, err = defaultDevice(en)
	} else {
		dev, err = deviceByID(en, id)
	}
	if err != nil {
		return nil, err
	}
	strm := &stream{device: dev}
	defer func() {
		if err != nil {
			strm.close()
		}
	}()
	if strm.id = deviceID(strm.device); strm.id == "" {
		err = errors.New("capture: endpoint has no id")
		return nil, err
	}
	strm.name = deviceName(strm.device)
	if strm.name == "" {
		strm.name = strm.id
	}
	if hr := comCall(strm.device, 3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0,
		uintptr(unsafe.Pointer(&strm.client))); failed(hr) { // IMMDevice.Activate
		err = hresult("IMMDevice.Activate", hr)
		return nil, err
	}
	var fmtPtr uintptr
	if hr := comCall(strm.client, 8, uintptr(unsafe.Pointer(&fmtPtr))); failed(hr) { // GetMixFormat
		err = hresult("GetMixFormat", hr)
		return nil, err
	}
	conv, rate, blockAlign, perr := parseMixFormat(fmtPtr)
	if perr == nil {
		flags := uintptr(audclntStreamFlagsLoopback | audclntStreamFlagsEventCB | audclntStreamFlagsNoPersist)
		if hr := comCall(strm.client, 3, audclntShareModeShared, flags, initBuffer100ns, 0, fmtPtr, 0); failed(hr) {
			perr = hresult("IAudioClient.Initialize", hr)
		}
	}
	procCoTaskMemFree.Call(fmtPtr)
	if perr != nil {
		err = perr
		return nil, err
	}
	strm.rate, strm.blockAlign, strm.conv = rate, blockAlign, conv
	if err = startClientStream(strm); err != nil {
		return nil, err
	}
	st = strm
	return st, nil
}

// close stops the stream and releases every pointer and the event handle.
func (st *stream) close() {
	if st.client != 0 {
		comCall(st.client, 11) // IAudioClient.Stop
	}
	release(st.capture)
	release(st.client)
	release(st.device)
	st.capture, st.client, st.device = 0, 0, 0
	if st.event != 0 {
		procCloseHandle.Call(st.event)
		st.event = 0
	}
}

// pump drains the capture client until stopped, the followed default device
// changes, or an error surfaces. It returns the reason it ended.
func (st *stream) pump(s *Source, en uintptr, follow bool) error {
	nextCheck := time.Now().Add(openCheckEvery)
	for {
		select {
		case <-s.stop:
			return errStopped
		default:
		}
		ev, _, _ := procWaitForSingleObject.Call(st.event, waitTimeoutMs)
		select {
		case <-s.stop:
			return errStopped
		default:
		}
		if ev == waitFailed {
			return errors.New("capture: the stream event failed; the device went away")
		}
		if follow && !time.Now().Before(nextCheck) {
			nextCheck = time.Now().Add(openCheckEvery)
			if id, err := defaultDeviceID(en); err == nil && id != st.id {
				return errDefaultDeviceChanged
			}
			// A transient read failure keeps the stream; the next check retries.
		}
		if ev != waitObject0 {
			continue // a silent desktop emits no loopback packets; the sender pads silence
		}
		for {
			var pkt, frames, flags uint32
			if hr := comCall(st.capture, 5, uintptr(unsafe.Pointer(&pkt))); failed(hr) {
				return hresult("IAudioCaptureClient.GetNextPacketSize", hr)
			}
			if pkt == 0 {
				break
			}
			var data uintptr
			if hr := comCall(st.capture, 3, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)),
				uintptr(unsafe.Pointer(&flags)), 0, 0); failed(hr) { // GetBuffer
				return hresult("IAudioCaptureClient.GetBuffer", hr)
			}
			if frames == 0 {
				break
			}
			if frames > maxPacketFrames {
				return fmt.Errorf("capture: implausible packet of %d frames", frames)
			}
			// DATA_DISCONTINUITY is normal on loopback (render paused, device
			// reconfigured) and can arrive spuriously, so it is not an error.
			st.push(data, frames, flags, s.cfg.Ring)
			s.addFrames(frames)
			if hr := comCall(st.capture, 4, uintptr(frames)); failed(hr) { // ReleaseBuffer
				return hresult("IAudioCaptureClient.ReleaseBuffer", hr)
			}
		}
	}
}

// push converts one capture packet to stereo int16 and drops it into the
// ring, which discards its oldest samples when full, so capture never blocks.
func (st *stream) push(data uintptr, frames, flags uint32, r ring.Sink) {
	n := int(frames) * 2
	if cap(st.scratch) < n {
		st.scratch = make([]int16, n+2048) // headroom for packets of varying size
	}
	dst := st.scratch[:n]
	if flags&audclntBufferFlagsSilent != 0 || data == 0 {
		clear(dst)
	} else {
		src := unsafe.Slice((*byte)(native(data)), int(frames)*st.blockAlign)
		st.conv.convert(dst, src)
	}
	r.Push(dst)
}

// parseMixFormat validates the device mix format and builds its converter.
// An extensible format's SubFormat GUID carries the real sample tag.
func parseMixFormat(p uintptr) (conv converter, rate, blockAlign int, err error) {
	raw := unsafe.Slice((*byte)(native(p)), 18)
	tag := binary.LittleEndian.Uint16(raw[0:2])
	channels := int(binary.LittleEndian.Uint16(raw[2:4]))
	rate = int(binary.LittleEndian.Uint32(raw[4:8]))
	blockAlign = int(binary.LittleEndian.Uint16(raw[12:14]))
	bits := int(binary.LittleEndian.Uint16(raw[14:16]))
	if tag == waveFormatExtensibleTag {
		if binary.LittleEndian.Uint16(raw[16:18]) < 22 {
			return converter{}, 0, 0, errors.New("capture: extensible format too small")
		}
		ext := unsafe.Slice((*byte)(native(p)), 40)
		tag = uint16(binary.LittleEndian.Uint32(ext[24:28])) // SubFormat: 1 = PCM, 3 = float
	}
	conv, err = newConverter(tag, bits, channels, blockAlign)
	if err != nil {
		return converter{}, 0, 0, err
	}
	if rate < 8000 || rate > 192000 {
		return converter{}, 0, 0, fmt.Errorf("capture: unsupported sample rate %d", rate)
	}
	return conv, rate, blockAlign, nil
}

func newEnumerator() (uintptr, error) {
	var en uintptr
	if hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&en))); failed(hr) {
		return 0, hresult("CoCreateInstance(MMDeviceEnumerator)", hr)
	}
	return en, nil
}

// Endpoints lists the active render endpoints with their friendly names.
func Endpoints() ([]Endpoint, error) {
	// COM apartments belong to OS threads, and goroutines may migrate.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	balance, err := comInit()
	if err != nil {
		return nil, err
	}
	defer balance()
	en, err := newEnumerator()
	if err != nil {
		return nil, err
	}
	defer release(en)
	var coll uintptr
	if hr := comCall(en, 3, eRender, deviceStateActive, uintptr(unsafe.Pointer(&coll))); failed(hr) {
		return nil, hresult("EnumAudioEndpoints", hr)
	}
	defer release(coll)
	var n uint32
	if hr := comCall(coll, 3, uintptr(unsafe.Pointer(&n))); failed(hr) {
		return nil, hresult("IMMDeviceCollection.GetCount", hr)
	}
	if n > maxEndpoints {
		n = maxEndpoints
	}
	out := make([]Endpoint, 0, n)
	for i := uint32(0); i < n; i++ {
		var dev uintptr
		if hr := comCall(coll, 4, uintptr(i), uintptr(unsafe.Pointer(&dev))); failed(hr) {
			continue
		}
		id, name := deviceID(dev), deviceName(dev)
		release(dev)
		if id == "" {
			continue
		}
		out = append(out, Endpoint{ID: id, Name: name})
	}
	return out, nil
}

// DefaultEndpoint returns the default render endpoint.
func DefaultEndpoint() (Endpoint, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	balance, err := comInit()
	if err != nil {
		return Endpoint{}, err
	}
	defer balance()
	en, err := newEnumerator()
	if err != nil {
		return Endpoint{}, err
	}
	defer release(en)
	dev, err := defaultDevice(en)
	if err != nil {
		return Endpoint{}, err
	}
	defer release(dev)
	id := deviceID(dev)
	if id == "" {
		return Endpoint{}, errors.New("capture: the default endpoint has no id")
	}
	name := deviceName(dev)
	if name == "" {
		name = id
	}
	return Endpoint{ID: id, Name: name}, nil
}

// defaultDevice returns the default render endpoint without releasing it.
func defaultDevice(en uintptr) (uintptr, error) {
	var dev uintptr
	if hr := comCall(en, 4, eRender, eConsole, uintptr(unsafe.Pointer(&dev))); failed(hr) {
		if uint32(hr) == 0x80070490 { // E_NOTFOUND
			return 0, errors.New("capture: no audio output device")
		}
		return 0, hresult("GetDefaultAudioEndpoint", hr)
	}
	return dev, nil
}

// defaultDeviceID returns the default render endpoint's id.
func defaultDeviceID(en uintptr) (string, error) {
	dev, err := defaultDevice(en)
	if err != nil {
		return "", err
	}
	defer release(dev)
	return deviceID(dev), nil
}

// deviceByID returns the endpoint with the given id without releasing it.
func deviceByID(en uintptr, id string) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(id)
	if err != nil {
		return 0, fmt.Errorf("capture: bad endpoint id: %v", err)
	}
	var dev uintptr
	if hr := comCall(en, 5, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&dev))); failed(hr) {
		return 0, hresult("GetDevice", hr)
	}
	return dev, nil
}

// deviceID reads and frees the endpoint's id string.
func deviceID(dev uintptr) string {
	var p *uint16
	if hr := comCall(dev, 5, uintptr(unsafe.Pointer(&p))); failed(hr) { // IMMDevice.GetId
		return ""
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	return pwstrToString(p)
}

// deviceName reads the endpoint's friendly name through its property store.
func deviceName(dev uintptr) string {
	var store uintptr
	if hr := comCall(dev, 4, stgmRead, uintptr(unsafe.Pointer(&store))); failed(hr) { // OpenPropertyStore
		return ""
	}
	defer release(store)
	var pv propVariant
	if hr := comCall(store, 5, uintptr(unsafe.Pointer(&pkeyDeviceFriendlyName)),
		uintptr(unsafe.Pointer(&pv))); failed(hr) { // GetValue
		return ""
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
	if pv.vt != vtLPWSTR {
		return ""
	}
	return pwstrToString((*uint16)(native(pv.val)))
}

// propVariant mirrors PROPVARIANT for the string properties read here.
type propVariant struct {
	vt   uint16
	res1 uint16
	res2 uint16
	res3 uint16
	val  uintptr // union: LPWSTR for VT_LPWSTR
}

// pwstrToString reads a null-terminated UTF-16 string, bounded.
func pwstrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	for n := 0; n < maxNameChars; n++ {
		if *(*uint16)(unsafe.Add(unsafe.Pointer(p), n*2)) == 0 {
			return syscall.UTF16ToString(unsafe.Slice(p, n))
		}
	}
	return syscall.UTF16ToString(unsafe.Slice(p, maxNameChars))
}
