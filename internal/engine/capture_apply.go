package engine

import (
	"errors"
	"time"

	"saundo/internal/airplay/sender"
	"saundo/internal/capture"
)

func (e *Engine) applyCaptureStart(res captureStart) {
	e.captureStarting = false
	if e.captureQueued {
		e.captureQueued = false
		if res.src != nil {
			res.src.Stop()
		}
		e.beginCaptureStart()
		return
	}
	if e.stopping {
		if res.src != nil {
			res.src.Stop()
		}
		return
	}
	if res.err != nil {
		e.captureRunning = false
		e.captureActiveApps = nil
		e.captureWaitingApp = errors.Is(res.err, capture.ErrNoAppProcess)
		e.logCaptureError(res.err)
		e.scheduleCaptureRetry()
		e.publish()
		return
	}
	e.capture = res.src
	e.captureRunning = true
	e.captureWaitingApp = false
	e.captureError = ""
	e.captureRetryAt = time.Time{}
	e.captureRetryNum = 0
	e.captureErrorLogged = ""
	e.captureErrorLoggedAt = time.Time{}
	stats := res.src.Stats()
	e.captureActiveApps = append([]string(nil), stats.Apps...)
	e.captureRate = stats.Rate
	e.captureOpenSeq = stats.OpenSeq
	if e.level != nil {
		e.level.SetRate(stats.Rate)
	}
	if stats.Device != "" {
		e.captureName = stats.Device
	}
	if e.tr != nil {
		e.tr.SetInputFormat(stats.Rate)
	}
	e.resetCaptureSink()
	if e.setsValue.MuteLocalOnConnect && e.tr != nil && e.tr.State() == sender.StateStreaming {
		// A tap opened or reopened during a session must take the mute the
		// session asked for, including a per-app tap's own mute.
		e.applyLocalMute()
	}
	e.logf("capture open: %s (%d Hz)", e.captureName, stats.Rate)
	e.publish()
}

func (e *Engine) applyCaptureInfo(info captureInfo) {
	if info.gen != e.captureGen {
		// A source that is being torn down reported late; the live source's
		// word is the only one that counts.
		return
	}
	if info.rate > 0 && info.rate != e.captureRate {
		e.captureRate = info.rate
		if e.level != nil {
			e.level.SetRate(info.rate)
		}
		if e.tr != nil {
			e.tr.SetInputFormat(info.rate)
		}
		e.resetCaptureSink()
		e.logf("capture rate changed to %d Hz", info.rate)
	}
	if info.name != "" {
		e.captureName = info.name
	}
	if info.err != nil {
		e.captureWaitingApp = errors.Is(info.err, capture.ErrNoAppProcess)
		e.logCaptureError(info.err)
	} else {
		e.captureWaitingApp = false
		e.captureError = ""
		e.captureErrorLogged = ""
		e.captureErrorLoggedAt = time.Time{}
	}
	if e.capture != nil {
		e.captureActiveApps = append([]string(nil), e.capture.Stats().Apps...)
	}
	e.publish()
}

// syncCaptureOpen applies a stream reopen that the lossy OnFormat and OnDevice
// callbacks may not have delivered. Every successful open is a generation
// boundary: a same-rate device switch would otherwise keep the old source's
// samples in the member rings, and a dropped rate notification would resample
// at the wrong input rate until the next open.
func (e *Engine) syncCaptureOpen() {
	if e.capture == nil {
		return
	}
	stats := e.capture.Stats()
	if stats.OpenSeq == 0 || stats.OpenSeq == e.captureOpenSeq {
		return
	}
	e.captureOpenSeq = stats.OpenSeq
	if stats.Rate > 0 {
		e.captureRate = stats.Rate
		if e.level != nil {
			e.level.SetRate(stats.Rate)
		}
		if e.tr != nil {
			e.tr.SetInputFormat(stats.Rate)
		}
	}
	if stats.Device != "" {
		e.captureName = stats.Device
	}
	e.resetCaptureSink()
	e.logf("capture reopened: %s (%d Hz)", e.captureName, stats.Rate)
	e.publish()
}

// logCaptureError records the current error and logs it on the first
// occurrence and then at most once a minute. A source that keeps retrying a
// missing device must not write a line per attempt.
func (e *Engine) logCaptureError(err error) {
	e.captureError = err.Error()
	now := e.now()
	if e.captureErrorLogged != e.captureError || now.Sub(e.captureErrorLoggedAt) >= time.Minute {
		e.logf("capture error: %v", err)
		e.captureErrorLogged = e.captureError
		e.captureErrorLoggedAt = now
	}
}
