package engine

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"saundo/internal/capture"
)

// TestCaptureStartFailureIsRetried pins the launch case where the first
// capture open fails because no render endpoint exists yet. The engine must
// try again on its own instead of waiting for the user to change the capture
// selection or restart the app.
func TestCaptureStartFailureIsRetried(t *testing.T) {
	h := newHarness(t)
	var calls atomic.Int32
	h.e.captureStartFn = func(capture.Config) (captureSource, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("no active render endpoint")
		}
		return &fakeCaptureSource{}, nil
	}

	h.e.restartCapture()
	var res captureStart
	select {
	case res = <-h.e.captureResult:
	case <-time.After(2 * time.Second):
		t.Fatal("the first capture open did not finish")
	}
	h.e.applyCaptureStart(res)
	if h.e.captureRunning {
		t.Fatal("capture reported running after a failed open")
	}
	if h.e.captureError == "" {
		t.Fatal("the failed open left no capture error")
	}

	// A retry must not run at once; the first backoff is a second.
	h.e.onWake()
	select {
	case <-h.e.captureResult:
		t.Fatal("capture retried before the backoff elapsed")
	case <-time.After(50 * time.Millisecond):
	}

	h.advance(31 * time.Second)
	h.e.onWake()
	select {
	case res = <-h.e.captureResult:
	case <-time.After(2 * time.Second):
		t.Fatal("the engine did not retry the failed capture open")
	}
	h.e.applyCaptureStart(res)
	if !h.e.captureRunning {
		t.Fatal("the retried capture did not start")
	}
	if h.e.captureError != "" {
		t.Fatalf("capture error survived a successful retry: %q", h.e.captureError)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("capture start calls = %d, want 2", got)
	}
}

// TestCaptureRestartDoesNotWaitForRetry pins the manual path: after a failed
// open scheduled a backoff, a user changing the capture selection starts at
// once instead of waiting for the timer.
func TestCaptureRestartDoesNotWaitForRetry(t *testing.T) {
	h := newHarness(t)
	var calls atomic.Int32
	h.e.captureStartFn = func(capture.Config) (captureSource, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("no active render endpoint")
		}
		return &fakeCaptureSource{}, nil
	}
	h.e.restartCapture()
	var res captureStart
	select {
	case res = <-h.e.captureResult:
	case <-time.After(2 * time.Second):
		t.Fatal("the first capture open did not finish")
	}
	h.e.applyCaptureStart(res)

	h.e.restartCapture()
	select {
	case res = <-h.e.captureResult:
		h.e.applyCaptureStart(res)
	case <-time.After(2 * time.Second):
		t.Fatal("a manual restart waited for the pending retry")
	}
	if !h.e.captureRunning {
		t.Fatal("the manual restart did not start capture")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("capture start calls = %d, want 2", got)
	}
}
