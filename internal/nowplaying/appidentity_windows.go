//go:build windows

package nowplaying

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// SMTC names an app only by its AUMID. Windows keeps the display name and
// icon in the shell's AppsFolder; an unpackaged player registers its
// executable as the AUMID, and its version resource carries the name.

const (
	siigbfBiggerSizeOK = 0x1
	siigbfIconOnly     = 0x4
	siigbfScaleUp      = 0x100

	iconPixels = 64

	processQueryLimitedInformation = 0x1000
	th32csSnapProcess              = 0x2
	processPathChars               = 32768

	sigdnNormalDisplay         = 0
	sigdnParentRelativeParsing = 0x80018001

	appsFolderPrefix = `shell:AppsFolder\`
)

var (
	iidShellItem             = guid{0x43826d1e, 0xe718, 0x42ee, [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
	iidShellItemImageFactory = guid{0xbcc18b79, 0xba16, 0x442f, [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b}}
	iidEnumShellItems        = guid{0x70629033, 0xe363, 0x4a28, [8]byte{0xa5, 0x67, 0x0d, 0xb7, 0x80, 0x06, 0xe6, 0xd7}}
	bhidEnumItems            = guid{0x94f60519, 0x2850, 0x4924, [8]byte{0xaa, 0x5a, 0xd1, 0x5e, 0x84, 0x86, 0x80, 0x39}}

	shell32  = syscall.NewLazyDLL("shell32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	version  = syscall.NewLazyDLL("version.dll")

	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	procGetDC                       = user32.NewProc("GetDC")
	procReleaseDC                   = user32.NewProc("ReleaseDC")
	procGetDIBits                   = gdi32.NewProc("GetDIBits")
	procGetObjectW                  = gdi32.NewProc("GetObjectW")
	procDeleteObject                = gdi32.NewProc("DeleteObject")
	procCoTaskMemFree               = ole32.NewProc("CoTaskMemFree")

	procCreateToolhelp32Snapshot   = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW            = kernel32.NewProc("Process32FirstW")
	procProcess32NextW             = kernel32.NewProc("Process32NextW")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle                = kernel32.NewProc("CloseHandle")

	procGetFileVersionInfoSizeW = version.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = version.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = version.NewProc("VerQueryValueW")
)

// appIdentity is what the shell knows about an app: the name to show and a
// PNG icon when one can be rendered. Failures are cached too, so a missing
// identity is not retried on every two-second poll.
type appIdentity struct {
	name string
	icon []byte
}

var appIdentities sync.Map // AUMID -> appIdentity

// resolveAppIdentity returns the display name and icon for aumid. The name
// falls back to the known-app map and the executable stem; the icon is nil
// when the shell cannot render one.
func resolveAppIdentity(aumid string) (string, []byte) {
	if aumid == "" {
		return "", nil
	}
	if v, ok := appIdentities.Load(aumid); ok {
		id := v.(appIdentity)
		return id.name, id.icon
	}
	id := lookupAppIdentity(aumid)
	appIdentities.Store(aumid, id)
	return id.name, id.icon
}

func lookupAppIdentity(aumid string) appIdentity {
	// The shell COM calls need an apartment on this thread; RoInitialize is
	// idempotent and the poll goroutine may not be the one that initialized
	// Windows.Media.
	procRoInitialize.Call(1)
	if app, ok := matchInstalledApp(aumid, installedApps()); ok {
		return appIdentity{name: app.name, icon: shellItemIcon(app.path)}
	}
	// An item the AppsFolder listing did not match may still parse by its
	// exact id or its profile prefix.
	for _, candidate := range appCandidates(aumid) {
		parsing := appsFolderPrefix + candidate
		name, icon := shellIdentity(parsing)
		if name != "" || len(icon) > 0 {
			if name == "" {
				name = appName(aumid)
			}
			return appIdentity{name: name, icon: icon}
		}
	}
	if file := exeFile(aumid); file != "" {
		if path := runningExecutable(file); path != "" {
			name := fileDescription(path)
			if name == "" {
				name = exeStem(aumid)
			}
			return appIdentity{name: name, icon: shellItemIcon(path)}
		}
	}
	return appIdentity{name: appName(aumid)}
}

// shellApp is one item of the shell's Applications folder.
type shellApp struct {
	name string
	id   string
	path string
}

var (
	appsOnce sync.Once
	appsList []shellApp
)

// installedApps lists the Start menu's Applications folder once per process:
// desktop shortcuts and packaged apps alike.
func installedApps() []shellApp {
	appsOnce.Do(func() {
		procRoInitialize.Call(1)
		folder, err := shellItem("shell:AppsFolder", &iidShellItem)
		if err != nil {
			return
		}
		defer release(folder)
		var enum uintptr
		hr, _, _ := syscall.SyscallN(vtblFn(folder, 3), folder,
			uintptr(unsafe.Pointer(&bhidEnumItems)), uintptr(unsafe.Pointer(&iidEnumShellItems)),
			uintptr(unsafe.Pointer(&enum)))
		if hr != 0 || enum == 0 {
			return
		}
		defer release(enum)
		for {
			var item uintptr
			var fetched uint32
			hr, _, _ := syscall.SyscallN(vtblFn(enum, 3), enum, 1,
				uintptr(unsafe.Pointer(&item)), uintptr(unsafe.Pointer(&fetched)))
			if hr != 0 || fetched == 0 || item == 0 {
				return
			}
			name := shellDisplayName(item, sigdnNormalDisplay)
			id := shellDisplayName(item, sigdnParentRelativeParsing)
			release(item)
			if name == "" || id == "" {
				continue
			}
			appsList = append(appsList, shellApp{name: name, id: id, path: appsFolderPrefix + id})
		}
	})
	return appsList
}

// matchInstalledApp maps an SMTC AUMID to an Applications-folder item: an
// exact id, the exe file name of a desktop app, its display name, or the
// token before a profile suffix.
func matchInstalledApp(aumid string, apps []shellApp) (shellApp, bool) {
	target := strings.ToLower(aumid)
	stem := strings.ToLower(exeStem(aumid))
	base := strings.ToLower(exeFile(aumid))
	prefix := target
	if i := strings.IndexByte(prefix, '.'); i > 0 {
		prefix = prefix[:i]
	}
	for _, app := range apps {
		id := strings.ToLower(app.id)
		switch {
		case id == target:
		case base != "" && (strings.ToLower(exeFile(app.id)) == base || strings.ToLower(exeStem(app.id)) == stem):
		case stem != "" && strings.ToLower(app.name) == stem:
		case prefix != "" && id == prefix:
		default:
			continue
		}
		return app, true
	}
	return shellApp{}, false
}

// appCandidates lists the shell ids to try for an AUMID: the exact id, and
// the token before a profile suffix ("Chrome.UserData.Profile1" -> "Chrome").
func appCandidates(aumid string) []string {
	out := []string{aumid}
	if i := strings.IndexByte(aumid, '.'); i > 0 {
		if prefix := aumid[:i]; prefix != aumid {
			out = append(out, prefix)
		}
	}
	return out
}

func shellIdentity(parsing string) (string, []byte) {
	item, err := shellItem(parsing, &iidShellItem)
	if err != nil {
		return "", nil
	}
	defer release(item)
	return shellDisplayName(item, sigdnNormalDisplay), shellItemIcon(parsing)
}

func shellDisplayName(item uintptr, sigdn uint32) string {
	var display unsafe.Pointer
	if hr, _, _ := syscall.SyscallN(vtblFn(item, 5), item, uintptr(sigdn),
		uintptr(unsafe.Pointer(&display))); hr != 0 || display == nil {
		return ""
	}
	name := strings.TrimSpace(utf16PtrString(display))
	procCoTaskMemFree.Call(uintptr(display))
	return name
}

func shellItem(parsing string, iid *guid) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(parsing)
	if err != nil {
		return 0, err
	}
	var obj uintptr
	hr, _, _ := procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&obj)))
	if hr != 0 || obj == 0 {
		return 0, fmt.Errorf("nowplaying: %s: HRESULT %#08x", parsing, uint32(hr))
	}
	return obj, nil
}

// shellItemIcon renders the item's icon at iconPixels and returns it as PNG.
func shellItemIcon(parsing string) []byte {
	factory, err := shellItem(parsing, &iidShellItemImageFactory)
	if err != nil {
		return nil
	}
	defer release(factory)
	var hbm uintptr
	size := uintptr(uint64(uint32(iconPixels)) | uint64(uint32(iconPixels))<<32)
	flags := uintptr(siigbfIconOnly | siigbfBiggerSizeOK | siigbfScaleUp)
	if hr, _, _ := syscall.SyscallN(vtblFn(factory, 3), factory, size, flags, uintptr(unsafe.Pointer(&hbm))); hr != 0 || hbm == 0 {
		return nil
	}
	defer procDeleteObject.Call(hbm)
	return hbitmapPNG(hbm)
}

type bitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type bitmapInfo struct {
	header bitmapInfoHeader
	colors [3]uint32
}

func hbitmapPNG(hbm uintptr) []byte {
	var bm bitmap
	if r, _, _ := procGetObjectW.Call(hbm, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		return nil
	}
	w, h := int(bm.bmWidth), int(bm.bmHeight)
	if w <= 0 || h <= 0 || w > 256 || h > 256 {
		return nil
	}
	info := bitmapInfo{}
	info.header.biSize = uint32(unsafe.Sizeof(info.header))
	info.header.biWidth = int32(w)
	info.header.biHeight = -int32(h) // top-down
	info.header.biPlanes = 1
	info.header.biBitCount = 32
	pix := make([]byte, w*h*4)
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return nil
	}
	defer procReleaseDC.Call(0, hdc)
	if r, _, _ := procGetDIBits.Call(hdc, hbm, 0, uintptr(h), uintptr(unsafe.Pointer(&pix[0])),
		uintptr(unsafe.Pointer(&info)), 0); r == 0 {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	opaque := false
	for i := 0; i < len(pix); i += 4 {
		a := pix[i+3]
		if a != 0 {
			opaque = true
		}
		img.Pix[i] = pix[i+2]
		img.Pix[i+1] = pix[i+1]
		img.Pix[i+2] = pix[i]
		img.Pix[i+3] = a
	}
	if !opaque {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

type processEntry32 struct {
	size            uint32
	cntUsage        uint32
	processID       uint32
	defaultHeapID   uintptr
	moduleID        uint32
	threads         uint32
	parentProcessID uint32
	priClassBase    int32
	flags           uint32
	exeFile         [260]uint16
}

// runningExecutable finds a running process whose executable is name and
// returns its full path.
func runningExecutable(name string) string {
	snap, _, _ := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snap == 0 || snap == uintptr(syscall.InvalidHandle) {
		return ""
	}
	defer procCloseHandle.Call(snap)
	var entry processEntry32
	entry.size = uint32(unsafe.Sizeof(entry))
	r, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	for r != 0 {
		if strings.EqualFold(syscall.UTF16ToString(entry.exeFile[:]), name) {
			if path := processPath(entry.processID); path != "" {
				return path
			}
		}
		r, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	}
	return ""
}

func processPath(pid uint32) string {
	h, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)
	buf := make([]uint16, processPathChars)
	size := uint32(len(buf))
	if r, _, _ := procQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// fileDescription reads the executable's localized FileDescription, then its
// ProductName, from the version resource.
func fileDescription(path string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	var handle uint32
	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&handle)))
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if r, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(size),
		uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return ""
	}
	var trans unsafe.Pointer
	var transLen uint32
	key, _ := syscall.UTF16PtrFromString(`\VarFileInfo\Translation`)
	if r, _, _ := procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(key)),
		uintptr(unsafe.Pointer(&trans)), uintptr(unsafe.Pointer(&transLen))); r == 0 || transLen < 4 {
		return ""
	}
	langs := unsafe.Slice((*uint16)(trans), transLen/2)
	for _, field := range []string{"FileDescription", "ProductName"} {
		for i := 0; i+1 < len(langs); i += 2 {
			sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, langs[i], langs[i+1], field)
			sp, _ := syscall.UTF16PtrFromString(sub)
			var vp unsafe.Pointer
			var vl uint32
			if r, _, _ := procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(sp)),
				uintptr(unsafe.Pointer(&vp)), uintptr(unsafe.Pointer(&vl))); r != 0 && vl > 0 && vp != nil {
				if s := strings.TrimSpace(unsafeUTF16(vp, int(vl))); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func unsafeUTF16(p unsafe.Pointer, chars int) string {
	if chars <= 0 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(p), chars))
}

func utf16PtrString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	const max = 4096
	s := unsafe.Slice((*uint16)(p), max)
	for i, v := range s {
		if v == 0 {
			return syscall.UTF16ToString(s[:i])
		}
	}
	return ""
}
