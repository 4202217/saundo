//go:build windows

// Package nowplaying reads the system "now playing" session through Windows'
// SMTC (GlobalSystemMediaTransportControlsSessionManager, WinRT). It polls
// rather than subscribing, which avoids implementing WinRT event delegates.
package nowplaying

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Track is one media session's track metadata.
type Track struct {
	Title  string
	Artist string
	Album  string
}

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

var (
	iidStatics    = guid{0x2050c4ee, 0x11a0, 0x57de, [8]byte{0xae, 0xd7, 0xc9, 0x7c, 0x70, 0x33, 0x82, 0x45}}
	iidIAsyncInfo = guid{0x00000036, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}

	smtcClassName = "Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager"

	combase                       = syscall.NewLazyDLL("combase.dll")
	procRoInitialize              = combase.NewProc("RoInitialize")
	procRoGetActivationFactory    = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString       = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString       = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuffer = combase.NewProc("WindowsGetStringRawBuffer")
)

// vtable slot indices, each after the six IInspectable slots.
const (
	slotRelease                       = 2
	staticsRequestAsync               = 6
	managerGetCurrentSession          = 6
	sessionTryGetMediaPropertiesAsync = 7
	asyncGetResults                   = 8
	asyncInfoStatus                   = 7
	mediaPropsTitle                   = 6
	mediaPropsArtist                  = 9
	mediaPropsAlbumTitle              = 10
)

const asyncStatusCompleted = 1

var initState struct {
	once sync.Once
	err  error
	mgr  uintptr
}

func native(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

func hstringFrom(s string) (uintptr, error) {
	var h uintptr
	u16, err := syscall.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	hr, _, _ := procWindowsCreateString.Call(
		uintptr(unsafe.Pointer(&u16[0])),
		uintptr(len(s)),
		uintptr(unsafe.Pointer(&h)),
	)
	if hr != 0 {
		return 0, fmt.Errorf("WindowsCreateString failed (HRESULT %#08x)", uint32(hr))
	}
	return h, nil
}

func hstringRead(h uintptr) string {
	if h == 0 {
		return ""
	}
	var length uint32
	ptr, _, _ := procWindowsGetStringRawBuffer.Call(h, uintptr(unsafe.Pointer(&length)))
	if ptr == 0 || length == 0 {
		return ""
	}
	// The syscall returned the PCWSTR in a uintptr; reinterpret its storage
	// instead of converting, so vet's uintptr check stays quiet.
	p := *(*(*uint16))(unsafe.Pointer(&ptr))
	return syscall.UTF16ToString(unsafe.Slice(p, length))
}

func hstringFree(h uintptr) {
	if h != 0 {
		procWindowsDeleteString.Call(h)
	}
}

func release(obj uintptr) {
	if obj == 0 {
		return
	}
	vtbl := *(*uintptr)(native(obj))
	releaseFn := *(*uintptr)(unsafe.Add(native(vtbl), slotRelease*int(unsafe.Sizeof(uintptr(0)))))
	syscall.SyscallN(releaseFn, obj)
}

func vtblFn(obj uintptr, slot int) uintptr {
	vtbl := *(*uintptr)(native(obj))
	return *(*uintptr)(unsafe.Add(native(vtbl), slot*int(unsafe.Sizeof(uintptr(0)))))
}

// callStringProperty invokes a vtbl slot that returns an HSTRING.
func callStringProperty(obj uintptr, slot int) string {
	var h uintptr
	if hr, _, _ := syscall.SyscallN(vtblFn(obj, slot), obj, uintptr(unsafe.Pointer(&h))); hr != 0 {
		return ""
	}
	defer hstringFree(h)
	return hstringRead(h)
}

// callObjectProperty invokes a vtbl slot that returns an interface pointer.
func callObjectProperty(obj uintptr, slot int) uintptr {
	var out uintptr
	if hr, _, _ := syscall.SyscallN(vtblFn(obj, slot), obj, uintptr(unsafe.Pointer(&out))); hr != 0 {
		return 0
	}
	return out
}

func comCallQuery(obj uintptr, iid *guid, out *uintptr) uint32 {
	hr, _, _ := syscall.SyscallN(vtblFn(obj, slotRelease-2), obj, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(out)))
	return uint32(hr)
}

// await polls a WinRT IAsyncOperation<T> until it completes and returns the
// result object.
func await(op uintptr, timeout time.Duration) (uintptr, error) {
	if op == 0 {
		return 0, fmt.Errorf("nowplaying: nil async operation")
	}
	var info uintptr
	if hr := comCallQuery(op, &iidIAsyncInfo, &info); hr != 0 {
		return 0, fmt.Errorf("nowplaying: async operation is not IAsyncInfo (HRESULT %#08x)", hr)
	}
	defer release(info)
	statusFn := vtblFn(info, asyncInfoStatus)
	deadline := time.Now().Add(timeout)
	for {
		var status int32
		if hr, _, _ := syscall.SyscallN(statusFn, info, uintptr(unsafe.Pointer(&status))); hr != 0 {
			return 0, fmt.Errorf("nowplaying: get_Status failed (HRESULT %#08x)", uint32(hr))
		}
		if status == asyncStatusCompleted {
			break
		}
		if status != 0 { // not Started
			return 0, fmt.Errorf("nowplaying: async operation status %d", status)
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("nowplaying: async operation timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return callObjectProperty(op, asyncGetResults), nil
}

// ensureManager activates the SMTC session manager once per process.
func ensureManager() error {
	initState.once.Do(func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		// RO_INIT_MULTITHREADED; a thread already in another apartment is fine.
		procRoInitialize.Call(1)
		class, err := hstringFrom(smtcClassName)
		if err != nil {
			initState.err = err
			return
		}
		defer hstringFree(class)
		var statics uintptr
		hr, _, _ := procRoGetActivationFactory.Call(
			class,
			uintptr(unsafe.Pointer(&iidStatics)),
			uintptr(unsafe.Pointer(&statics)),
		)
		if hr != 0 {
			initState.err = fmt.Errorf("RoGetActivationFactory failed (HRESULT %#08x)", uint32(hr))
			return
		}
		defer release(statics)
		op := callObjectProperty(statics, staticsRequestAsync)
		if op == 0 {
			initState.err = fmt.Errorf("nowplaying: RequestAsync returned nothing")
			return
		}
		mgr, aerr := await(op, 3*time.Second)
		release(op)
		if aerr != nil {
			initState.err = aerr
			return
		}
		if mgr == 0 {
			initState.err = fmt.Errorf("nowplaying: session manager is nil")
			return
		}
		initState.mgr = mgr
	})
	return initState.err
}

// Current returns the current media session's track metadata. ok is false
// when nothing is playing.
func Current() (Track, bool, error) {
	if err := ensureManager(); err != nil {
		return Track{}, false, err
	}
	var session uintptr
	slot := managerGetCurrentSession
	hrRaw, _, _ := syscall.SyscallN(vtblFn(initState.mgr, slot), initState.mgr, uintptr(unsafe.Pointer(&session)))
	hr := uint32(hrRaw)
	if hr != 0 {
		return Track{}, false, fmt.Errorf("nowplaying: GetCurrentSession failed (HRESULT %#08x)", hr)
	}
	if session == 0 {
		// GetCurrentSession returns null with S_OK when nothing plays.
		return Track{}, false, nil
	}
	defer release(session)
	op := callObjectProperty(session, sessionTryGetMediaPropertiesAsync)
	if op == 0 {
		return Track{}, false, fmt.Errorf("nowplaying: GetMediaPropertiesAsync returned nothing")
	}
	props, err := await(op, 3*time.Second)
	release(op)
	if err != nil {
		return Track{}, false, err
	}
	if props == 0 {
		return Track{}, false, nil
	}
	defer release(props)
	track := Track{
		Title:  callStringProperty(props, mediaPropsTitle),
		Artist: callStringProperty(props, mediaPropsArtist),
		Album:  callStringProperty(props, mediaPropsAlbumTitle),
	}
	if track.Title == "" {
		return Track{}, false, nil
	}
	return track, true, nil
}
