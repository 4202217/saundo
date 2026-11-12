//go:build windows

package capture

// Application-level capture through the Windows process-loopback virtual
// audio device (Windows 10 2004+). The activation is the COM-heavy part:
// ActivateAudioInterfaceAsync takes a PROPVARIANT describing the target
// process tree and a callback object that receives the IAudioClient.

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// AppInfo describes one capturable application: a visible window or an
// active audio session, rooted at its same-exe process-tree leader.
type AppInfo struct {
	PID   uint32
	Name  string
	Path  string
	Title string
	// ID is unset on Windows: the picker selects by PID there.
	ID string
	// Icon is unset on Windows: the shared picker shows text only there.
	Icon []byte
	// Active reports whether the app (its process tree) currently renders
	// audio; process-loopback capture only initializes for active apps.
	Active bool
}

// MaxAppCaptureBuild is the minimum Windows build with process loopback
// (Windows 10 2004).
const minAppCaptureBuild = 19041

type guidBytes [16]byte

// GUIDs in the native guid layout (capture_windows.go), compared in QI.
var (
	guidIActivationHandler   = guid{0x41d949ab, 0x9862, 0x444a, [8]byte{0x80, 0xf6, 0xc2, 0x61, 0x33, 0x4d, 0xa5, 0xeb}}
	guidIAgileObject         = guid{0x94ea2b94, 0xe9cc, 0x49e0, [8]byte{0xc0, 0xff, 0xee, 0x64, 0xca, 0x8f, 0x5b, 0x90}}
	guidIUnknown             = guid{}
	iidIAudioSessionManager2 = guid{0x77aa99a0, 0x1bd6, 0x484f, [8]byte{0x8b, 0xc7, 0x2c, 0x65, 0x4c, 0x9a, 0x9b, 0x6f}}
	iidIAudioSessionControl2 = guid{0xbfb7ff88, 0x7239, 0x4fc9, [8]byte{0x8f, 0xa2, 0x07, 0xc9, 0x50, 0xbe, 0x9c, 0x6d}}
)

const (
	virtualLoopbackDevice = "VAD\\Process_Loopback"
	vtBlob                = 65

	activationTypeProcessLoopback  = 1
	loopbackModeIncludeProcessTree = 0

	procQueryLimitedInformation = 0x1000

	// 16-bit PCM at a fixed 48 kHz; WASAPI converts the app's mix.
	appCaptureRate       = 48000
	appCaptureAlign      = 4
	appStreamAutoConvert = 0x80000000
)

var (
	mmdevapi                        = syscall.NewLazyDLL("mmdevapi.dll")
	procActivateAudioInterfaceAsync = mmdevapi.NewProc("ActivateAudioInterfaceAsync")
	user32                          = syscall.NewLazyDLL("user32.dll")
	procEnumWindows                 = user32.NewProc("EnumWindows")
	procIsWindowVisible             = user32.NewProc("IsWindowVisible")
	procGetWindowTextW              = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW        = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadPID          = user32.NewProc("GetWindowThreadProcessId")
	kernel32App                     = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess                 = kernel32App.NewProc("OpenProcess")
	procCloseHandleApp              = kernel32App.NewProc("CloseHandle")
	procQueryFullProcessName        = kernel32App.NewProc("QueryFullProcessImageNameW")
	procGetProcessTimes             = kernel32App.NewProc("GetProcessTimes")
	procCreateToolhelpSnap          = kernel32App.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW             = kernel32App.NewProc("Process32FirstW")
	procProcess32NextW              = kernel32App.NewProc("Process32NextW")
	procGetCurrentPID               = kernel32App.NewProc("GetCurrentProcessId")
	procWaitForObjectApp            = kernel32App.NewProc("WaitForSingleObject")
	ntdll                           = syscall.NewLazyDLL("ntdll.dll")
	procRtlGetVersion               = ntdll.NewProc("RtlGetVersion")
)

// MultiAppSupported reports whether one capture stream can mix several
// selected apps. Windows process loopback captures one process tree.
func MultiAppSupported() bool { return false }

// ProcessMuteSupported reports whether a per-app stream can mute just the
// captured app. Windows process loopback cannot.
func ProcessMuteSupported() bool { return false }

// SetProcessMuted is unsupported on Windows.
func SetProcessMuted(bool) error {
	return errors.New("capture: process mute is unsupported on this platform")
}

// AppCaptureSupported reports whether this Windows build can activate
// process-loopback capture.
func AppCaptureSupported() bool {
	return appCaptureSupported()
}

// appCaptureSupported reports whether this Windows build can activate
// process-loopback capture.
func appCaptureSupported() bool {
	type osVersionInfo struct {
		size            uint32
		major, minor    uint32
		build           uint32
		platform        uint32
		csd             [128]uint16
		packageMajor    uint16
		packageMinor    uint16
		packageBuild    uint16
		packageRevision uint16
	}
	var info osVersionInfo
	info.size = uint32(unsafe.Sizeof(info))
	rc, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	return rc == 0 && info.major >= 10 && info.build >= minAppCaptureBuild
}

// ---- activation callback object ----

// activationVTbl is IActivateAudioInterfaceCompletionHandler's vtable:
// QueryInterface, AddRef, Release, ActivateCompleted.
type activationVTbl struct {
	qi, addRef, release, completed uintptr
}

var handlerVTbl = activationVTbl{
	qi:        syscall.NewCallback(handlerQueryInterface),
	addRef:    syscall.NewCallback(handlerAddRef),
	release:   syscall.NewCallback(handlerRelease),
	completed: syscall.NewCallback(handlerActivateCompleted),
}

// The handler object stays reachable past its activation: COM may invoke a
// very late callback carrying this pointer, and completed retains the object
// so the allocator cannot recycle its address into a newer handler.
type handlerObject struct {
	vtbl *activationVTbl
	ch   chan appClient
}

type appClient struct {
	client uintptr
	hr     int32
}

var (
	handlerMu sync.Mutex
	// handlers routes a completion to the handler that owns it, keyed by the
	// exact interface pointer handed to ActivateAudioInterfaceAsync. completed
	// keeps retired handlers reachable against address reuse by a later
	// allocation.
	handlers  = map[uintptr]*handlerObject{}
	completed = map[uintptr]*handlerObject{}
)

// registerHandler publishes h under the pointer COM echoes back as `this`.
func registerHandler(h *handlerObject) uintptr {
	this := uintptr(unsafe.Pointer(&h.vtbl))
	handlerMu.Lock()
	handlers[this] = h
	handlerMu.Unlock()
	return this
}

// retireHandler marks the activation finished and releases any client a late
// callback already delivered into the now-unread channel.
func retireHandler(this uintptr, h *handlerObject) {
	handlerMu.Lock()
	if handlers[this] == h {
		delete(handlers, this)
	}
	completed[this] = h
	handlerMu.Unlock()
	for {
		select {
		case res := <-h.ch:
			releaseDelivered(res)
		default:
			return
		}
	}
}

// deliverActivation routes one completion to its own handler. A missing or
// retired handler, or a full channel, releases the interface instead of
// leaking it.
func deliverActivation(this uintptr, res appClient) {
	handlerMu.Lock()
	if h, ok := handlers[this]; ok {
		select {
		case h.ch <- res:
			handlerMu.Unlock()
			return
		default:
		}
	}
	handlerMu.Unlock()
	releaseDelivered(res)
}

func releaseDelivered(res appClient) {
	if res.client != 0 {
		release(res.client)
	}
}

func handlerAddRef() uintptr { return 1 }

func handlerRelease() uintptr { return 1 }

func handlerQueryInterface(this uintptr, riid *guid, out *uintptr) uintptr {
	if riid == nil || out == nil {
		return 0x80004003 // E_POINTER
	}
	switch {
	case *riid == guidIUnknown, *riid == guidIActivationHandler, *riid == guidIAgileObject:
		handlerAddRef()
		*out = this
		return 0 // S_OK
	}
	return 0x80004004 // E_NOINTERFACE
}

// handlerActivateCompleted reads the operation's activate result and hands
// the IAudioClient (or HRESULT) to the waiting goroutine.
func handlerActivateCompleted(this uintptr, operation uintptr) uintptr {
	if operation == 0 {
		return 0
	}
	vtbl := *(*uintptr)(native(operation))
	getResult := *(*uintptr)(unsafe.Add(native(vtbl), 3*int(unsafe.Sizeof(uintptr(0)))))
	var hr uint32
	var iface uintptr
	hrCall, _, _ := syscall.SyscallN(getResult, operation, uintptr(unsafe.Pointer(&hr)), uintptr(unsafe.Pointer(&iface)))
	res := appClient{hr: int32(uint32(hr))}
	if hrCall == 0 && !failed(uintptr(hr)) && iface != 0 {
		res.client = iface
	}
	deliverActivation(this, res)
	return 0 // S_OK
}

// activateProcessLoopback activates the process-loopback virtual device for
// pid's whole process tree and returns the ready IAudioClient.
func activateProcessLoopback(pid uint32) (uintptr, error) {
	// COM must already be initialized (MTA) on this thread.
	params := struct {
		activationType int32
		processID      uint32
		loopbackMode   int32
	}{activationTypeProcessLoopback, pid, loopbackModeIncludeProcessTree}

	handler := &handlerObject{vtbl: &handlerVTbl, ch: make(chan appClient, 1)}
	this := registerHandler(handler)
	defer retireHandler(this, handler)

	var pv struct {
		vt       uint16
		r1, r2   uint16
		r3       uint16
		cb       uint32
		blobData unsafe.Pointer
	}
	pv.vt = vtBlob
	pv.cb = uint32(unsafe.Sizeof(params))
	pv.blobData = unsafe.Pointer(&params)

	devicePath, err := syscall.UTF16PtrFromString(virtualLoopbackDevice)
	if err != nil {
		return 0, err
	}
	var operation uintptr
	hr, _, _ := procActivateAudioInterfaceAsync.Call(
		uintptr(unsafe.Pointer(devicePath)),
		uintptr(unsafe.Pointer(&iidIAudioClient)),
		uintptr(unsafe.Pointer(&pv)),
		this,
		uintptr(unsafe.Pointer(&operation)),
	)
	if failed(hr) {
		return 0, hresult("ActivateAudioInterfaceAsync", hr)
	}
	// Keep the async operation referenced until the callback fires; it is
	// released implicitly when this activation ends.
	if operation != 0 {
		defer release(operation)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		select {
		case res := <-handler.ch:
			if res.hr != 0 {
				return 0, fmt.Errorf("capture: process-loopback activation failed (HRESULT %#08x)", uint32(res.hr))
			}
			if res.client == 0 {
				return 0, errors.New("capture: process-loopback activation returned no interface")
			}
			return res.client, nil
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return 0, errors.New("capture: process-loopback activation timed out")
		}
	}
}

// openAppStream activates capture for a process tree and starts the stream.
// The virtual device wants an explicit format; 16-bit stereo at 48 kHz keeps
// the ring's contract and lets WASAPI convert anything the app renders.
func openAppStream(en uintptr, cfg Config, pid uint32) (st *stream, err error) {
	if !appCaptureSupported() {
		return nil, errors.New("capture: application capture needs Windows 10 2004 (build 19041) or later")
	}
	client, err := activateProcessLoopback(pid)
	if err != nil {
		return nil, err
	}
	strm := &stream{client: client}
	defer func() {
		if err != nil {
			strm.close()
		}
	}()
	// The virtual device wants float32 stereo at a chosen rate, with zero
	// buffer duration and the LOOPBACK flag, like the Windows
	// ApplicationLoopback sample.
	format := struct {
		tag            uint16
		channels       uint16
		samplesPerSec  uint32
		avgBytesPerSec uint32
		blockAlign     uint16
		bitsPerSample  uint16
	}{
		tag:            3, // IEEE float
		channels:       2,
		samplesPerSec:  appCaptureRate,
		avgBytesPerSec: appCaptureRate * 8,
		blockAlign:     8,
		bitsPerSample:  32,
	}
	flags := uintptr(audclntStreamFlagsLoopback | audclntStreamFlagsEventCB)
	// Initialize rejects E_INVALIDARG until the target's audio graph is up
	// (it may not be rendering yet); retry briefly before giving up.
	var initErr error
	for attempt := 0; attempt < 25; attempt++ {
		hr := comCall(strm.client, 3, audclntShareModeShared, flags, 0, 0,
			uintptr(unsafe.Pointer(&format)), 0)
		if !failed(hr) {
			initErr = nil
			break
		}
		initErr = hresult("IAudioClient.Initialize (process loopback)", hr)
		if uint32(hr) != 0x80070057 { // E_INVALIDARG: only this is retryable
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if initErr != nil {
		err = initErr
		return nil, err
	}
	strm.id = fmt.Sprintf("app://%d", pid)
	strm.name = appStreamName(pid)
	if strm.name == "" {
		strm.name = strm.id
	}
	strm.rate, strm.blockAlign = appCaptureRate, 8
	strm.conv = converter{kind: kindFloat32, channels: 2, blockAlign: 8}
	if err = startClientStream(strm); err != nil {
		return nil, err
	}
	st = strm
	return st, nil
}

// startClientStream wires the event handle, the capture client, and starts
// the stream. Shared by endpoint and application capture.
func startClientStream(st *stream) error {
	ev, _, callErr := procCreateEventW.Call(0, 0, 0, 0)
	if ev == 0 {
		return fmt.Errorf("capture: CreateEvent failed: %v", callErr)
	}
	st.event = ev
	if hr := comCall(st.client, 13, ev); failed(hr) { // SetEventHandle
		return hresult("IAudioClient.SetEventHandle", hr)
	}
	if hr := comCall(st.client, 14, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)),
		uintptr(unsafe.Pointer(&st.capture))); failed(hr) { // GetService
		return hresult("IAudioClient.GetService(IAudioCaptureClient)", hr)
	}
	if hr := comCall(st.client, 10); failed(hr) { // Start
		return hresult("IAudioClient.Start", hr)
	}
	return nil
}

// AudioApplications lists capturable applications: visible windows and
// processes holding audio sessions, deduplicated to same-exe process-tree
// roots so one app shows once even with child processes.
func AudioApplications() ([]AppInfo, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	balance, err := comInit()
	if err != nil {
		return nil, err
	}
	defer balance()

	interesting := make(map[uint32]struct{})
	activePIDs, err := collectAudioSessionPIDs(interesting)
	if err != nil {
		return nil, err
	}
	collectWindowPIDs(interesting)

	items, parents := processesSnapshot()
	roots := make(map[uint32]struct{})
	rootActive := make(map[uint32]struct{})
	for pid := range interesting {
		if _, ok := items[pid]; !ok {
			continue
		}
		root := processRoot(pid, items, parents)
		roots[root] = struct{}{}
		if _, isActive := activePIDs[pid]; isActive {
			rootActive[root] = struct{}{}
		}
	}
	titles := windowTitles()
	out := make([]AppInfo, 0, len(roots))
	for pid := range roots {
		item, ok := items[pid]
		if !ok {
			continue
		}
		out = append(out, AppInfo{PID: pid, Name: item.Name, Path: item.Path, Title: titles[pid], Active: func() bool { _, ok := rootActive[pid]; return ok }()})
	}
	sort.Slice(out, func(i, j int) bool {
		if strings.ToLower(out[i].Name) == strings.ToLower(out[j].Name) {
			return out[i].PID < out[j].PID
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// collectAudioSessionPIDs gathers the process IDs behind every audio session
// of every active render endpoint. A vanishing endpoint must not block the
// enumeration of the others.
func collectAudioSessionPIDs(into map[uint32]struct{}) (map[uint32]struct{}, error) {
	active := make(map[uint32]struct{})
	en, err := newEnumerator()
	if err != nil {
		return active, err
	}
	defer release(en)
	var coll uintptr
	if hr := comCall(en, 3, eRender, deviceStateActive, uintptr(unsafe.Pointer(&coll))); failed(hr) {
		return active, hresult("EnumAudioEndpoints", hr)
	}
	defer release(coll)
	var n uint32
	if hr := comCall(coll, 3, uintptr(unsafe.Pointer(&n))); failed(hr) {
		return active, hresult("IMMDeviceCollection.GetCount", hr)
	}
	for i := uint32(0); i < n && i < maxEndpoints; i++ {
		var dev uintptr
		if hr := comCall(coll, 4, uintptr(i), uintptr(unsafe.Pointer(&dev))); failed(hr) {
			continue
		}
		var manager uintptr
		if hr := comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioSessionManager2)), clsctxAll, 0,
			uintptr(unsafe.Pointer(&manager))); failed(hr) { // IMMDevice.Activate
			release(dev)
			continue
		}
		var sessions uintptr
		if hr := comCall(manager, 5, uintptr(unsafe.Pointer(&sessions))); failed(hr) { // GetSessionEnumerator
			release(manager)
			release(dev)
			continue
		}
		var count uint32
		if hr := comCall(sessions, 3, uintptr(unsafe.Pointer(&count))); failed(hr) {
			release(sessions)
			release(manager)
			release(dev)
			continue
		}
		for j := uint32(0); j < count && j < maxEndpoints; j++ {
			var ctrl uintptr
			if hr := comCall(sessions, 4, uintptr(j), uintptr(unsafe.Pointer(&ctrl))); failed(hr) {
				continue
			}
			var ctrl2 uintptr
			if hr := comCall(ctrl, 0, uintptr(unsafe.Pointer(&iidIAudioSessionControl2)), uintptr(unsafe.Pointer(&ctrl2))); failed(hr) {
				release(ctrl)
				continue
			}
			var pid uint32
			if hr := comCall(ctrl2, 14, uintptr(unsafe.Pointer(&pid))); failed(hr) { // GetProcessId
				release(ctrl2)
				release(ctrl)
				continue
			}
			var state uint32
			comCall(ctrl, 3, uintptr(unsafe.Pointer(&state))) // GetState: 1 = active
			if pid != 0 {
				into[pid] = struct{}{}
				if state == 1 {
					active[pid] = struct{}{}
				}
			}
			release(ctrl2)
			release(ctrl)
		}
		release(sessions)
		release(manager)
		release(dev)
	}
	return active, nil
}

type windowScanMode uint8

const (
	scanPIDs windowScanMode = iota
	scanTitles
)

// windowScan is the state an EnumWindows callback reads. It is kept in a
// registry keyed by a token, so no Go pointer crosses the Windows callback.
type windowScan struct {
	mode   windowScanMode
	pids   map[uint32]struct{}
	titles map[uint32]string
}

var (
	windowScanMu  sync.Mutex
	windowScans   = map[uintptr]*windowScan{}
	windowScanSeq uintptr

	// newWindowsCallback is a test seam. The callback itself is registered
	// once: Go never releases a syscall callback, and the runtime table holds
	// only 2000.
	newWindowsCallback  = syscall.NewCallback
	enumWindowsCallback = sync.OnceValue(func() uintptr {
		return newWindowsCallback(enumWindowsProc)
	})
)

func beginWindowScan(scan *windowScan) uintptr {
	windowScanMu.Lock()
	windowScanSeq++
	id := windowScanSeq
	windowScans[id] = scan
	windowScanMu.Unlock()
	return id
}

func endWindowScan(id uintptr) {
	windowScanMu.Lock()
	delete(windowScans, id)
	windowScanMu.Unlock()
}

func windowScanFor(id uintptr) *windowScan {
	windowScanMu.Lock()
	defer windowScanMu.Unlock()
	return windowScans[id]
}

func enumWindowsProc(hwnd, lparam uintptr) uintptr {
	scan := windowScanFor(lparam)
	if scan == nil {
		return 1
	}
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if visible == 0 {
		return 1
	}
	length, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if length == 0 {
		return 1
	}
	var pid uint32
	procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return 1
	}
	switch scan.mode {
	case scanPIDs:
		scan.pids[pid] = struct{}{}
	case scanTitles:
		if scan.titles[pid] == "" {
			buf := make([]uint16, length+1)
			procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(length+1))
			scan.titles[pid] = syscall.UTF16ToString(buf)
		}
	}
	return 1
}

func collectWindowPIDs(into map[uint32]struct{}) {
	id := beginWindowScan(&windowScan{mode: scanPIDs, pids: into})
	defer endWindowScan(id)
	procEnumWindows.Call(enumWindowsCallback(), id)
}

func windowTitles() map[uint32]string {
	out := make(map[uint32]string)
	id := beginWindowScan(&windowScan{mode: scanTitles, titles: out})
	defer endWindowScan(id)
	procEnumWindows.Call(enumWindowsCallback(), id)
	return out
}

type processEntry struct {
	pid, parent uint32
	exe         string
}

func processesSnapshot() (items map[uint32]AppInfo, parents map[uint32]uint32) {
	items = make(map[uint32]AppInfo)
	parents = make(map[uint32]uint32)
	snap, _, _ := procCreateToolhelpSnap.Call(0x2, 0) // TH32CS_SNAPPROCESS
	if snap == 0 || snap == ^uintptr(0) {
		return items, parents
	}
	defer procCloseHandleApp.Call(snap)
	type processEntry32 struct {
		size     uint32
		cntUsage uint32
		pid      uint32
		heap     uintptr
		moduleID uint32
		threads  uint32
		parent   uint32
		priority int32
		flags    uint32
		exe      [260]uint16
	}
	var entry processEntry32
	entry.size = uint32(unsafe.Sizeof(entry))
	selfPID, _, _ := procGetCurrentPID.Call()
	ok, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		pid := entry.pid
		parents[pid] = entry.parent
		if pid != 0 && pid != uint32(selfPID) {
			if info, err := processIdentity(pid); err == nil {
				items[pid] = info
			}
		}
		ok, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry)))
		entry.size = uint32(unsafe.Sizeof(entry))
	}
	return items, parents
}

func processIdentity(pid uint32) (AppInfo, error) {
	handle, _, _ := procOpenProcess.Call(procQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return AppInfo{}, fmt.Errorf("open process %d", pid)
	}
	defer procCloseHandleApp.Call(handle)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if rc, _, _ := procQueryFullProcessName.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); rc == 0 {
		return AppInfo{}, fmt.Errorf("query image name of process %d", pid)
	}
	path := syscall.UTF16ToString(buf[:size])
	name := path
	if idx := strings.LastIndexAny(path, `\/`); idx >= 0 {
		name = path[idx+1:]
	}
	return AppInfo{PID: pid, Name: name, Path: path}, nil
}

// processRoot walks up through parents whose exe path matches, so one app's
// browser/renderer children report under the same entry.
func processRoot(pid uint32, items map[uint32]AppInfo, parents map[uint32]uint32) uint32 {
	current := pid
	visited := make(map[uint32]struct{})
	for {
		if _, seen := visited[current]; seen {
			return current
		}
		visited[current] = struct{}{}
		parentPID, hasParent := parents[current]
		parent, parentKnown := items[parentPID]
		child, childKnown := items[current]
		if !hasParent || !parentKnown || !childKnown {
			return current
		}
		if !strings.EqualFold(child.Path, parent.Path) || parent.PID == 0 {
			return current
		}
		current = parentPID
	}
}

func appStreamName(pid uint32) string {
	if info, err := processIdentity(pid); err == nil {
		return info.Name
	}
	return ""
}
