//go:build preview

package app

import (
	"testing"
	"time"

	"saundo/internal/airplay/sender"
)

func TestPreviewConnectReachesStreaming(t *testing.T) {
	a := PreviewApp()
	oldAfter, oldOnMain := previewAfter, previewOnMain
	defer func() { previewAfter, previewOnMain = oldAfter, oldOnMain }()

	var transition func()
	previewAfter = func(_ time.Duration, fn func()) *time.Timer {
		transition = fn
		return nil
	}
	previewOnMain = func(fn func()) { fn() }

	if !previewReduce(a, "connect", "AA:BB:CC:DD:EE:02") {
		t.Fatal("connect was not handled")
	}
	if a.snap.State != sender.StateHandshake {
		t.Fatalf("state = %v, want handshake", a.snap.State)
	}
	if transition == nil {
		t.Fatal("no transition was scheduled")
	}
	transition()
	if a.snap.State != sender.StateStreaming || a.snap.ConnectedName != "Apple TV" {
		t.Fatalf("state = %v, name = %q", a.snap.State, a.snap.ConnectedName)
	}

	previewReduce(a, "disconnect", nil)
	transition()
	if a.snap.State != sender.StateIdle {
		t.Fatalf("stale transition revived %v", a.snap.State)
	}
}
