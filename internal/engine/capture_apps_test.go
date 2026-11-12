package engine

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"saundo/internal/capture"
)

// TestCaptureAppsCommandPersistsAndStartsTheSelection pins the multi-app
// path through the engine: stable IDs are stored, a legacy PID is cleared,
// and the capture start sees the selection.
func TestCaptureAppsCommandPersistsAndStartsTheSelection(t *testing.T) {
	h := newHarness(t)
	started := make(chan capture.Config, 2)
	h.e.captureStartFn = func(cfg capture.Config) (captureSource, error) {
		started <- cfg
		return &fakeCaptureSource{}, nil
	}
	h.e.handleCommand(command{
		kind:  cmdCaptureApps,
		ids:   []string{"com.apple.Music", "com.google.Chrome"},
		names: []string{"Music", "Google Chrome"},
		label: "2 apps",
	})
	if got := h.e.captureApps; len(got) != 2 || got[0] != "com.apple.Music" || got[1] != "com.google.Chrome" {
		t.Fatalf("captureApps = %v", got)
	}
	if h.e.captureProcess != 0 || h.e.captureName != "2 apps" {
		t.Fatalf("process=%d name=%q, want none and the label", h.e.captureProcess, h.e.captureName)
	}
	if !h.e.perAppCapture() {
		t.Fatal("an app selection did not count as per-app capture")
	}
	if got := h.e.setsValue; len(got.CaptureAppIDs) != 2 || got.CaptureProcessPID != 0 {
		t.Fatalf("persisted selection = %+v", got)
	}
	select {
	case cfg := <-started:
		if cfg.ProcessID != 0 || len(cfg.AppIDs) != 2 || cfg.AppIDs[1] != "com.google.Chrome" {
			t.Fatalf("capture config = %+v", cfg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capture did not start with the app selection")
	}

	select {
	case res := <-h.e.captureResult:
		h.e.applyCaptureStart(res)
	case <-time.After(2 * time.Second):
		t.Fatal("the app capture start never finished")
	}

	h.e.handleCommand(command{kind: cmdCaptureEndpoint, name: "Speakers"})
	if len(h.e.captureApps) != 0 || len(h.e.setsValue.CaptureAppIDs) != 0 {
		t.Fatalf("endpoint selection kept the apps: %+v", h.e.setsValue)
	}
	select {
	case cfg := <-started:
		if len(cfg.AppIDs) != 0 {
			t.Fatalf("endpoint capture kept app IDs: %+v", cfg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capture did not restart for the endpoint")
	}
}

// TestCaptureNoAppProcessWaits pins the waiting state: a selection with no
// Core Audio process is not a red capture failure, but other errors still
// are.
func TestCaptureNoAppProcessWaits(t *testing.T) {
	h := newHarness(t)
	h.e.captureStartFn = func(capture.Config) (captureSource, error) {
		return nil, fmt.Errorf("%w: com.apple.Music", capture.ErrNoAppProcess)
	}
	h.e.restartCapture()
	var res captureStart
	select {
	case res = <-h.e.captureResult:
	case <-time.After(2 * time.Second):
		t.Fatal("the capture start never finished")
	}
	h.e.applyCaptureStart(res)
	snap := h.e.Snapshot()
	if !snap.CaptureWaitingApp {
		t.Fatalf("snapshot = %+v, want a waiting app state", snap)
	}
	if snap.CaptureError == "" || snap.CaptureRunning {
		t.Fatalf("snapshot = %+v, want a reason and no running capture", snap)
	}

	h.e.captureStartFn = func(capture.Config) (captureSource, error) {
		return nil, errors.New("no render device")
	}
	h.e.restartCapture()
	select {
	case res = <-h.e.captureResult:
	case <-time.After(2 * time.Second):
		t.Fatal("the second capture start never finished")
	}
	h.e.applyCaptureStart(res)
	if h.e.Snapshot().CaptureWaitingApp {
		t.Fatal("a device error was classified as waiting for an app")
	}
}
