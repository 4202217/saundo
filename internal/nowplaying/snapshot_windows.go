//go:build windows

package nowplaying

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// SMTC interface slots, each after the six IInspectable slots.
const (
	managerGetSessions           = 7
	vectorGetAt                  = 6
	vectorSize                   = 7
	sessionSourceAUMID           = 6
	sessionGetTimelineProperties = 8
	sessionGetPlaybackInfo       = 9

	mediaPropsSubtitle    = 7
	mediaPropsAlbumArtist = 8
	mediaPropsThumbnail   = 15

	playbackControls = 6
	playbackStatus   = 7

	controlsPlay   = 6
	controlsPause  = 7
	controlsNext   = 12
	controlsPrev   = 13
	controlsSeek   = 20
	timelineStart  = 6
	timelineEnd    = 7
	timelinePos    = 10
	timelineUpdate = 11

	thumbnailOpenReadAsync = 6
	rasGetSize             = 6
	rasGetInputStreamAt    = 8
	inputStreamReadAsync   = 6
	bufferFactoryCreate    = 6
	bufferGetLength        = 7
	bufferByteAccessBuffer = 3
	asyncProgressResults   = 10
)

const maxArtworkBytes = 16 << 20

var (
	iidBufferFactory       = guid{0x71af914d, 0xc10f, 0x484b, [8]byte{0xbc, 0x50, 0x14, 0xbc, 0x62, 0x3b, 0x3a, 0x27}}
	iidIRandomAccessStream = guid{0x905a0fe1, 0xbc53, 0x11df, [8]byte{0x8c, 0x49, 0x00, 0x1e, 0x4f, 0xc6, 0x86, 0xda}}
	iidBufferByteAccess    = guid{0x905a0fef, 0xbc53, 0x11df, [8]byte{0x8c, 0x49, 0x00, 0x1e, 0x4f, 0xc6, 0x86, 0xda}}
	bufferFactoryOnce      sync.Once
	bufferFactory          uintptr
	bufferFactoryErr       error
)

// awaitSlot polls a WinRT async operation and reads its result from resultsSlot.
// IAsyncOperation<T> uses slot 8; IAsyncOperationWithProgress<T,P> uses slot 10.
func awaitSlot(op uintptr, resultsSlot int, timeout time.Duration) (uintptr, error) {
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
		if status != 0 {
			return 0, fmt.Errorf("nowplaying: async operation status %d", status)
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("nowplaying: async operation timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return callObjectProperty(op, resultsSlot), nil
}

func callBool(obj uintptr, slot int) bool {
	var v int32
	if hr, _, _ := syscall.SyscallN(vtblFn(obj, slot), obj, uintptr(unsafe.Pointer(&v))); hr != 0 {
		return false
	}
	return v != 0
}

func callInt32(obj uintptr, slot int) (int32, bool) {
	var v int32
	if hr, _, _ := syscall.SyscallN(vtblFn(obj, slot), obj, uintptr(unsafe.Pointer(&v))); hr != 0 {
		return 0, false
	}
	return v, true
}

func callInt64(obj uintptr, slot int) (int64, bool) {
	var v int64
	if hr, _, _ := syscall.SyscallN(vtblFn(obj, slot), obj, uintptr(unsafe.Pointer(&v))); hr != 0 {
		return 0, false
	}
	return v, true
}

func callUint32(obj uintptr, slot int) (uint32, bool) {
	var v uint32
	if hr, _, _ := syscall.SyscallN(vtblFn(obj, slot), obj, uintptr(unsafe.Pointer(&v))); hr != 0 {
		return 0, false
	}
	return v, true
}

func callUint64(obj uintptr, slot int) (uint64, bool) {
	var v uint64
	if hr, _, _ := syscall.SyscallN(vtblFn(obj, slot), obj, uintptr(unsafe.Pointer(&v))); hr != 0 {
		return 0, false
	}
	return v, true
}

// Read returns every SMTC session and the manager's current session index.
func Read() (Snapshot, error) {
	if err := ensureManager(); err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{ReadAt: time.Now()}
	var vec uintptr
	if hr, _, _ := syscall.SyscallN(vtblFn(initState.mgr, managerGetSessions), initState.mgr, uintptr(unsafe.Pointer(&vec))); hr != 0 {
		return snap, fmt.Errorf("nowplaying: GetSessions failed (HRESULT %#08x)", uint32(hr))
	}
	if vec == 0 {
		return snap, nil
	}
	defer release(vec)
	size, ok := callUint32(vec, vectorSize)
	if !ok {
		return snap, fmt.Errorf("nowplaying: session vector size failed")
	}
	for i := uint32(0); i < size; i++ {
		var session uintptr
		if hr, _, _ := syscall.SyscallN(vtblFn(vec, vectorGetAt), vec, uintptr(i), uintptr(unsafe.Pointer(&session))); hr != 0 || session == 0 {
			continue
		}
		snap.Sessions = append(snap.Sessions, readSession(session))
		release(session)
	}
	var current uintptr
	if hr, _, _ := syscall.SyscallN(vtblFn(initState.mgr, managerGetCurrentSession), initState.mgr, uintptr(unsafe.Pointer(&current))); hr == 0 && current != 0 {
		aumid := callStringProperty(current, sessionSourceAUMID)
		release(current)
		for i := range snap.Sessions {
			if snap.Sessions[i].AUMID == aumid {
				snap.Current = i
				break
			}
		}
	}
	return snap, nil
}

func readSession(session uintptr) Session {
	out := Session{AUMID: callStringProperty(session, sessionSourceAUMID)}
	out.AppName, out.AppIcon = resolveAppIdentity(out.AUMID)
	if op := callObjectProperty(session, sessionTryGetMediaPropertiesAsync); op != 0 {
		if props, err := await(op, 3*time.Second); err == nil && props != 0 {
			out.Title = callStringProperty(props, mediaPropsTitle)
			out.Subtitle = callStringProperty(props, mediaPropsSubtitle)
			out.AlbumArtist = callStringProperty(props, mediaPropsAlbumArtist)
			out.Artist = callStringProperty(props, mediaPropsArtist)
			out.Album = callStringProperty(props, mediaPropsAlbumTitle)
			out.Artwork, out.ArtworkMIME = readArtwork(props)
			release(props)
		}
		release(op)
	}
	if pi := callObjectProperty(session, sessionGetPlaybackInfo); pi != 0 {
		if status, ok := callInt32(pi, playbackStatus); ok {
			out.Status = PlaybackStatus(status)
			out.Playing = out.Status == StatusPlaying
		}
		if controls := callObjectProperty(pi, playbackControls); controls != 0 {
			out.CanPlay = callBool(controls, controlsPlay)
			out.CanPause = callBool(controls, controlsPause)
			out.CanNext = callBool(controls, controlsNext)
			out.CanPrev = callBool(controls, controlsPrev)
			out.CanSeek = callBool(controls, controlsSeek)
			release(controls)
		}
		release(pi)
	}
	if tl := callObjectProperty(session, sessionGetTimelineProperties); tl != 0 {
		start, _ := callInt64(tl, timelineStart)
		end, _ := callInt64(tl, timelineEnd)
		pos, _ := callInt64(tl, timelinePos)
		updated, _ := callInt64(tl, timelineUpdate)
		out.Position = ticksToDuration(pos - start)
		out.Duration = ticksToDuration(end - start)
		out.Updated = filetimeToTime(updated)
		release(tl)
	}
	return out
}

var artworkDebug func(format string, args ...any)

func artDebugf(format string, args ...any) {
	if artworkDebug != nil {
		artworkDebug(format, args...)
	}
}

func readArtwork(props uintptr) ([]byte, string) {
	ref := callObjectProperty(props, mediaPropsThumbnail)
	if ref == 0 {
		artDebugf("thumbnail ref nil")
		return nil, ""
	}
	defer release(ref)
	op := callObjectProperty(ref, thumbnailOpenReadAsync)
	if op == 0 {
		artDebugf("openRead op nil")
		return nil, ""
	}
	stream, err := await(op, 3*time.Second)
	release(op)
	if err != nil || stream == 0 {
		artDebugf("openRead: stream=%#x err=%v", stream, err)
		return nil, ""
	}
	defer release(stream)
	var ras uintptr
	if hr := comCallQuery(stream, &iidIRandomAccessStream, &ras); hr != 0 || ras == 0 {
		artDebugf("IRandomAccessStream QI: hr=%#x ras=%#x", hr, ras)
		return nil, ""
	}
	defer release(ras)
	size, ok := callUint64(ras, rasGetSize)
	if !ok || size == 0 || size > maxArtworkBytes {
		artDebugf("size: ok=%v size=%d", ok, size)
		return nil, ""
	}
	var in uintptr
	if hr, _, _ := syscall.SyscallN(vtblFn(ras, rasGetInputStreamAt), ras, 0, uintptr(unsafe.Pointer(&in))); hr != 0 || in == 0 {
		artDebugf("GetInputStreamAt: hr=%#x in=%#x", hr, in)
		return nil, ""
	}
	defer release(in)
	factory, err := ensureBufferFactory()
	if err != nil {
		artDebugf("buffer factory: %v", err)
		return nil, ""
	}
	var buf uintptr
	if hr, _, _ := syscall.SyscallN(vtblFn(factory, bufferFactoryCreate), factory, uintptr(uint32(size)), uintptr(unsafe.Pointer(&buf))); hr != 0 || buf == 0 {
		artDebugf("buffer create: hr=%#x buf=%#x", hr, buf)
		return nil, ""
	}
	defer release(buf)
	var readOp uintptr
	if hr, _, _ := syscall.SyscallN(vtblFn(in, inputStreamReadAsync), in, buf, uintptr(uint32(size)), 0, uintptr(unsafe.Pointer(&readOp))); hr != 0 || readOp == 0 {
		artDebugf("ReadAsync: hr=%#x op=%#x", hr, readOp)
		return nil, ""
	}
	result, err := awaitSlot(readOp, asyncProgressResults, 3*time.Second)
	release(readOp)
	if err != nil || result == 0 {
		artDebugf("ReadAsync result: result=%#x err=%v", result, err)
		return nil, ""
	}
	defer release(result)
	length, ok := callUint32(result, bufferGetLength)
	if !ok || length == 0 {
		artDebugf("buffer length: ok=%v length=%d", ok, length)
		return nil, ""
	}
	if uint64(length) > size {
		length = uint32(size)
	}
	var mem uintptr
	if hr := comCallQuery(result, &iidBufferByteAccess, &mem); hr != 0 || mem == 0 {
		artDebugf("IBufferByteAccess QI: hr=%#x mem=%#x", hr, mem)
		return nil, ""
	}
	defer release(mem)
	var data *byte
	if hr, _, _ := syscall.SyscallN(vtblFn(mem, bufferByteAccessBuffer), mem, uintptr(unsafe.Pointer(&data))); hr != 0 || data == nil {
		artDebugf("Buffer: hr=%#x data=%p", hr, data)
		return nil, ""
	}
	n := int(length)
	out := make([]byte, n)
	copy(out, unsafe.Slice(data, n))
	artDebugf("artwork ok: %d bytes", len(out))
	return out, sniffImage(out)
}

func ensureBufferFactory() (uintptr, error) {
	bufferFactoryOnce.Do(func() {
		class, err := hstringFrom("Windows.Storage.Streams.Buffer")
		if err != nil {
			bufferFactoryErr = err
			return
		}
		defer hstringFree(class)
		var factory uintptr
		hr, _, _ := procRoGetActivationFactory.Call(class, uintptr(unsafe.Pointer(&iidBufferFactory)), uintptr(unsafe.Pointer(&factory)))
		if hr != 0 {
			bufferFactoryErr = fmt.Errorf("nowplaying: buffer factory (HRESULT %#08x)", uint32(hr))
			return
		}
		if factory == 0 {
			bufferFactoryErr = fmt.Errorf("nowplaying: buffer factory is nil")
			return
		}
		bufferFactory = factory
	})
	return bufferFactory, bufferFactoryErr
}

func ticksToDuration(ticks int64) time.Duration {
	if ticks <= 0 {
		return 0
	}
	return time.Duration(ticks) * 100
}

func filetimeToTime(ticks int64) time.Time {
	if ticks <= 0 {
		return time.Time{}
	}
	const ticksPerSecond = 10_000_000
	const epochDelta = 11644473600
	sec := ticks/ticksPerSecond - epochDelta
	nsec := (ticks % ticksPerSecond) * 100
	return time.Unix(sec, nsec)
}

func appName(aumid string) string {
	switch {
	case strings.HasPrefix(aumid, "AppleInc.AppleMusicWin"):
		return "Apple Music"
	case strings.HasPrefix(aumid, "Spotify"):
		return "Spotify"
	case strings.HasPrefix(aumid, "Chrome"):
		return "Chrome"
	case strings.HasPrefix(aumid, "Microsoft.ZuneMusic"), strings.HasPrefix(aumid, "Microsoft.Media.Player"):
		return "Media Player"
	}
	// An unpackaged player registers its executable as the AUMID, e.g.
	// "MyGo Music.exe"; its stem is the name the app gives itself.
	if name := exeStem(aumid); name != "" {
		return name
	}
	// A packaged app's AUMID is "<package family>!<app id>"; an unknown one
	// is left empty so the card can name it in the user's language instead
	// of showing a package identifier fragment.
	return ""
}

// exeFile returns the executable file name of a .exe AUMID, or "" when the
// AUMID is not a path to an executable.
func exeFile(aumid string) string {
	name := aumid
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	if len(name) <= 4 || !strings.EqualFold(name[len(name)-4:], ".exe") {
		return ""
	}
	return name
}

// exeStem returns the executable name without its directory or .exe suffix.
func exeStem(aumid string) string {
	name := exeFile(aumid)
	if name == "" {
		return ""
	}
	return strings.TrimSpace(name[:len(name)-4])
}

func sniffImage(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff:
		return "image/jpeg"
	case len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a"):
		return "image/gif"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	case len(b) >= 2 && b[0] == 'B' && b[1] == 'M':
		return "image/bmp"
	default:
		return ""
	}
}
