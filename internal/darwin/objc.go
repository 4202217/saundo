//go:build darwin

// Package darwin drives macOS platform APIs from pure Go, no cgo: the
// Objective-C runtime through purego/objc, the Keychain through Security,
// Core Audio through its C API. It backs the darwin implementations of the
// capture and creds packages; the UI keeps to mygo.
package darwin

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type ID = objc.ID

var (
	libObjC, libCF, libFoundation, libSecurity, libCoreAudio, libAppKit uintptr

	msgSendAddr uintptr
	poolPushFn  uintptr
	poolPopFn   uintptr

	loadOnce sync.Once
	loadErr  error
)

func mustDlopen(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
}

// load opens the frameworks the package binds. It is lazy: callers run on
// machines where some frameworks may be absent, and the error surfaces to
// them instead of panicking.
func load() error {
	loadOnce.Do(func() {
		var err error
		if libObjC, err = mustDlopen("/usr/lib/libobjc.A.dylib"); err != nil {
			loadErr = fmt.Errorf("darwin: %w", err)
			return
		}
		if libFoundation, err = mustDlopen("/System/Library/Frameworks/Foundation.framework/Foundation"); err != nil {
			loadErr = fmt.Errorf("darwin: %w", err)
			return
		}
		if libCF, err = mustDlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"); err != nil {
			loadErr = fmt.Errorf("darwin: %w", err)
			return
		}
		if libAppKit, err = mustDlopen("/System/Library/Frameworks/AppKit.framework/AppKit"); err != nil {
			loadErr = fmt.Errorf("darwin: %w", err)
			return
		}
		if msgSendAddr, err = purego.Dlsym(libObjC, "objc_msgSend"); err != nil {
			loadErr = fmt.Errorf("darwin: %w", err)
			return
		}
		if poolPushFn, err = purego.Dlsym(libObjC, "objc_autoreleasePoolPush"); err != nil {
			loadErr = fmt.Errorf("darwin: %w", err)
			return
		}
		if poolPopFn, err = purego.Dlsym(libObjC, "objc_autoreleasePoolPop"); err != nil {
			loadErr = fmt.Errorf("darwin: %w", err)
			return
		}
		purego.RegisterLibFunc(&cfReleaseFn, libCF, "CFRelease")
	})
	return loadErr
}

// Dlsym resolves name from an already loaded library.
func Dlsym(lib uintptr, name string) (uintptr, error) { return purego.Dlsym(lib, name) }

var (
	selMu    sync.RWMutex
	selCache = map[string]objc.SEL{}
	clsCache = map[string]ID{}
)

func sel(name string) objc.SEL {
	selMu.RLock()
	s, ok := selCache[name]
	selMu.RUnlock()
	if ok {
		return s
	}
	s = objc.RegisterName(name)
	selMu.Lock()
	selCache[name] = s
	selMu.Unlock()
	return s
}

// class returns the class object for name, cached. It reports false when the
// class does not exist, which callers treat as a missing platform API.
func class(name string) (ID, bool) {
	selMu.RLock()
	c, ok := clsCache[name]
	selMu.RUnlock()
	if ok {
		return c, true
	}
	if c = ID(objc.GetClass(name)); c == 0 {
		return 0, false
	}
	selMu.Lock()
	clsCache[name] = c
	selMu.Unlock()
	return c, true
}

// mustClass is class for classes the package requires.
func mustClass(name string) ID {
	c, ok := class(name)
	if !ok {
		panic("darwin: Objective-C class " + name + " not found")
	}
	return c
}

// Class returns the class object for name, cached, reporting false when the
// class does not exist. Frameworks must be loaded first for their classes to
// register; EnsureCoreAudio loads the one that owns CATapDescription.
func Class(name string) (ID, bool) {
	if err := load(); err != nil {
		return 0, false
	}
	return class(name)
}

// MustClass panics for a missing class: callers use it for Foundation
// classes every macOS has.
func MustClass(name string) ID {
	if err := load(); err != nil {
		panic(err)
	}
	return mustClass(name)
}

// NumberBool boxes a Go bool as an NSNumber.
func NumberBool(b bool) ID {
	v := uintptr(0)
	if b {
		v = 1
	}
	return Send(MustClass("NSNumber"), "numberWithBool:", v)
}

// NumberUint boxes a Go uint32 as an NSNumber.
func NumberUint(v uint32) ID {
	return Send(MustClass("NSNumber"), "numberWithUnsignedInt:", uintptr(v))
}

// Send sends a message whose arguments are integers or pointers. Go pointers
// converted in the call are moved to the heap and kept alive until it
// returns (go:uintptrescapes).
//
//go:uintptrescapes
func Send(obj ID, selector string, args ...uintptr) ID {
	if err := load(); err != nil {
		return 0
	}
	var a [12]uintptr
	a[0], a[1] = uintptr(obj), uintptr(sel(selector))
	n := copy(a[2:], args)
	r, _, _ := purego.SyscallN(msgSendAddr, a[:n+2]...)
	return ID(r)
}

// SendInt sends a message returning NSInteger.
func SendInt(obj ID, selector string, args ...uintptr) int {
	return int(Send(obj, selector, args...))
}

// Alloc creates an owned (+1) instance of a class with init.
func Alloc(className string) ID {
	return Send(Send(mustClass(className), "alloc"), "init")
}

// WithPool runs fn inside an autorelease pool. A pool belongs to the thread
// that pushed it, so the goroutine stays on its thread meanwhile.
func WithPool(fn func()) {
	if err := load(); err != nil {
		fn()
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool, _, _ := purego.SyscallN(poolPushFn)
	defer purego.SyscallN(poolPopFn, pool)
	fn()
}

const nsUTF8StringEncoding = 4

// NSString returns an autoreleased NSString.
func NSString(s string) ID {
	b := unsafe.StringData(s)
	str := Send(Send(mustClass("NSString"), "alloc"), "initWithBytes:length:encoding:",
		uintptr(unsafe.Pointer(b)), uintptr(len(s)), nsUTF8StringEncoding)
	runtime.KeepAlive(s)
	return Send(str, "autorelease")
}

// GoString copies an NSString into a Go string.
func GoString(str ID) string {
	if str == 0 {
		return ""
	}
	p := Send(str, "UTF8String")
	if p == 0 {
		return ""
	}
	n := SendInt(str, "lengthOfBytesUsingEncoding:", nsUTF8StringEncoding)
	return string(unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), n))
}

// NSData returns an autoreleased NSData with a copy of b.
func NSData(b []byte) ID {
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	d := Send(mustClass("NSData"), "dataWithBytes:length:", uintptr(p), uintptr(len(b)))
	runtime.KeepAlive(b)
	return d
}

// GoBytes copies the contents of an NSData.
func GoBytes(data ID) []byte {
	if data == 0 {
		return nil
	}
	n := SendInt(data, "length")
	if n == 0 {
		return []byte{}
	}
	p := Send(data, "bytes")
	return append([]byte(nil), unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), n)...)
}

// NSArray returns an autoreleased NSArray of objs.
func NSArray(objs ...ID) ID {
	arr := Send(mustClass("NSMutableArray"), "arrayWithCapacity:", uintptr(len(objs)))
	for _, o := range objs {
		Send(arr, "addObject:", uintptr(o))
	}
	return arr
}

// CFRelease balances a +1 reference from a Copy or Create function.
func CFRelease(obj uintptr) {
	if obj != 0 && cfReleaseFn != nil {
		cfReleaseFn(obj)
	}
}

var cfReleaseFn func(obj uintptr)

// ConstString reads an exported CFStringRef or NSString* constant.
func ConstString(lib uintptr, name string) ID {
	p, err := purego.Dlsym(lib, name)
	if err != nil || p == 0 {
		return 0
	}
	return **(**ID)(unsafe.Pointer(&p))
}

// BundleResourcePath returns the resource directory of the running app
// bundle, or "" when the process does not run from a bundle.
func BundleResourcePath() string {
	if err := load(); err != nil {
		return ""
	}
	bundle := Send(mustClass("NSBundle"), "mainBundle")
	if bundle == 0 {
		return ""
	}
	p := Send(bundle, "resourcePath")
	if p == 0 {
		return ""
	}
	return GoString(p)
}
