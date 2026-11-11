//go:build windows

package capture

import (
	"sync"
	"syscall"
	"testing"
)

// TestWindowScansShareOneCallback pins the callback budget: every
// AudioApplications call used to register two new callbacks, which Go never
// releases. The runtime table holds only 2000, and both scans now share one
// registration.
func TestWindowScansShareOneCallback(t *testing.T) {
	prevNew := newWindowsCallback
	prevCB := enumWindowsCallback
	t.Cleanup(func() {
		newWindowsCallback = prevNew
		enumWindowsCallback = prevCB
	})
	registrations := 0
	newWindowsCallback = func(fn any) uintptr {
		registrations++
		return syscall.NewCallback(fn)
	}
	enumWindowsCallback = sync.OnceValue(func() uintptr {
		return newWindowsCallback(enumWindowsProc)
	})

	collectWindowPIDs(map[uint32]struct{}{})
	_ = windowTitles()
	if registrations != 1 {
		t.Fatalf("callback registrations = %d, want 1 for both scans", registrations)
	}
}
