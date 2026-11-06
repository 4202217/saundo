//go:build windows

package nowplaying

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

// SMTC session transport slots, each after the six IInspectable slots.
const (
	sessionTryPlayAsync                   = 10
	sessionTryPauseAsync                  = 11
	sessionTrySkipNextAsync               = 16
	sessionTrySkipPreviousAsync           = 17
	sessionTryChangePlaybackPositionAsync = 24
)

// Control sends one transport command to the session with aumid. The session
// is looked up again instead of being kept, so a card that is one poll behind
// still targets the right app after the sessions change.
func Control(aumid string, action Action, position time.Duration) error {
	if err := ensureManager(); err != nil {
		return err
	}
	session, err := sessionByAUMID(aumid)
	if err != nil {
		return err
	}
	if session == 0 {
		return fmt.Errorf("nowplaying: no media session for %q", aumid)
	}
	defer release(session)

	slot := 0
	switch action {
	case ActionPlay:
		slot = sessionTryPlayAsync
	case ActionPause:
		slot = sessionTryPauseAsync
	case ActionNext:
		slot = sessionTrySkipNextAsync
	case ActionPrevious:
		slot = sessionTrySkipPreviousAsync
	case ActionSeek:
		slot = sessionTryChangePlaybackPositionAsync
	default:
		return fmt.Errorf("nowplaying: unknown action %d", action)
	}

	var op uintptr
	if action == ActionSeek {
		// TimeSpan is 100 ns ticks.
		hr, _, _ := syscall.SyscallN(vtblFn(session, slot), session,
			uintptr(position.Nanoseconds()/100), uintptr(unsafe.Pointer(&op)))
		if hr != 0 {
			return fmt.Errorf("nowplaying: seek failed (HRESULT %#08x)", uint32(hr))
		}
	} else {
		hr, _, _ := syscall.SyscallN(vtblFn(session, slot), session, uintptr(unsafe.Pointer(&op)))
		if hr != 0 {
			return fmt.Errorf("nowplaying: action %d failed (HRESULT %#08x)", action, uint32(hr))
		}
	}
	if op == 0 {
		return fmt.Errorf("nowplaying: action %d returned no operation", action)
	}
	defer release(op)
	accepted, err := awaitSlot(op, asyncGetResults, 3*time.Second)
	if err != nil {
		return err
	}
	if accepted == 0 {
		return fmt.Errorf("nowplaying: the media app declined action %d", action)
	}
	return nil
}

// sessionByAUMID returns an owned SMTC session pointer, or 0 when no session
// carries that AUMID.
func sessionByAUMID(aumid string) (uintptr, error) {
	if aumid == "" {
		return 0, nil
	}
	var vec uintptr
	if hr, _, _ := syscall.SyscallN(vtblFn(initState.mgr, managerGetSessions), initState.mgr, uintptr(unsafe.Pointer(&vec))); hr != 0 {
		return 0, fmt.Errorf("nowplaying: GetSessions failed (HRESULT %#08x)", uint32(hr))
	}
	if vec == 0 {
		return 0, nil
	}
	defer release(vec)
	size, ok := callUint32(vec, vectorSize)
	if !ok {
		return 0, fmt.Errorf("nowplaying: session vector size failed")
	}
	for i := uint32(0); i < size; i++ {
		var session uintptr
		if hr, _, _ := syscall.SyscallN(vtblFn(vec, vectorGetAt), vec, uintptr(i), uintptr(unsafe.Pointer(&session))); hr != 0 || session == 0 {
			continue
		}
		if callStringProperty(session, sessionSourceAUMID) == aumid {
			return session, nil
		}
		release(session)
	}
	return 0, nil
}
