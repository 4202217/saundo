package engine

import (
	"time"

	"saundo/internal/airplay/sender"
)

// longIdleRebuildAfter is how long the captured source may stay silent before
// the first audible block rebuilds the session instead of resuming the live
// stream. A HomePod 27.3 stereo pair stops rendering the realtime stream after
// a long silent stretch even though RTP keeps flowing with the compact silence
// frame: the display state, a volume change, and a fresh cover do not revive
// it, and only a new session does (measured 2026-11-24). Thirty minutes is the
// maintainer's middle ground after the 2026-11-25 failure: a 59-minute silence
// resumed with PLC static, so the limit sits below an hour, while the 5-minute
// resume of 2026-11-10 was clean. The rebuild costs one connect and the first
// moment of the resume; a receiver that gives up sooner than the threshold
// still needs a manual reconnect.
const longIdleRebuildAfter = 30 * time.Minute

// rebuildAfterLongIdle rebuilds the session when the source becomes audible
// again after a long silent stretch. A HomePod 27.3 pair stops rendering the
// realtime stream after such a stretch and only a new session revives it, so
// the first audible block pays a connect instead of streaming into a silent
// receiver. A gap shorter than the threshold keeps the live stream and its
// seamless resume. It reports whether a rebuild started.
func (e *Engine) rebuildAfterLongIdle() bool {
	if e.silence == nil {
		return false
	}
	loud := e.silence.LastLoud()
	if !loud.After(e.lastLoudSeen) {
		return false
	}
	gap := loud.Sub(e.lastLoudSeen)
	e.lastLoudSeen = loud
	// Record the measured gap on every long resume, so a missed rebuild shows
	// whether the silence was under the threshold or a guard refused it.
	if gap >= 5*time.Minute {
		e.logf("source audible after %s of silence (rebuild threshold %s)", gap.Round(time.Second), longIdleRebuildAfter)
	}
	if gap < longIdleRebuildAfter || e.stopping || e.tr == nil || e.tr.State() != sender.StateStreaming {
		return false
	}
	if e.tr.WaitingForPin() || !e.plan.live() {
		return false
	}
	if missing, offline, ok := e.sessionAddressable(); !ok {
		name := missing
		if name == "" {
			name = offline
		}
		e.logf("long idle %s: %s is unavailable; keeping the live stream", gap.Round(time.Second), name)
		return false
	}
	old := e.tr
	e.logf("long idle %s: rebuilding the session on resume", gap.Round(time.Second))
	e.life.userRebuild()
	e.rebuildSession()
	return e.tr != old
}

// resetIdle returns the session's idle tracking to the active phase and
// re-anchors the long-idle clock to the new session, so a resume from before
// the session cannot rebuild it.
func (e *Engine) resetIdle() {
	e.idle = idleActive
	e.standbyHoldAt = time.Time{}
	e.darkBaseline = time.Time{}
	e.lastLoudSeen = e.now()
	if e.silence != nil {
		e.silence.SetPaused(false)
	}
}
