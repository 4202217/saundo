package engine

import (
	"time"

	"saundo/internal/airplay/sender"
	"saundo/internal/settings"
)

// audioPauseAfter is how long the captured source must be digitally silent
// before the engine suspends the RTP pacer. Five seconds keeps longer track
// gaps and silent passages streaming while a real pause suspends promptly and
// starts the standby clock. An AirPlay 2 receiver resumes from a re-anchored
// timeline; a receiver that cannot suspend keeps the continuous silence
// stream.
const (
	audioPauseAfter = 5 * time.Second
	audioPausePoll  = 25 * time.Millisecond
)

type idlePhase uint8

const (
	idleActive    idlePhase = iota // audible audio, or a session that just started
	idleSuspended                  // RTP suspended, session alive
	idleDark                       // RTP suspended, lamp off, session alive
)

// pauseAudio suspends the pacer for a silent source and stops the meter from
// filling the rings with silence. A receiver that cannot suspend (AirPlay 1)
// keeps the continuous silence stream.
func (e *Engine) pauseAudio() {
	if e.tr == nil {
		return
	}
	if !e.tr.SetPaused(true) {
		e.logf("audio idle: the receiver keeps the continuous silence stream")
		return
	}
	if e.silence != nil {
		e.silence.SetPaused(true)
	}
	e.logf("audio idle for %s, stream suspended", audioPauseAfter)
}

// resumeAudio re-arms the pacer on the first audible block. The sender
// re-anchors the RTP time to the wall clock and starts from the newest
// cushion.
func (e *Engine) resumeAudio() {
	if e.tr == nil {
		return
	}
	if !e.tr.SetPaused(false) {
		return
	}
	if e.silence != nil {
		e.silence.SetPaused(false)
	}
	e.logf("audio resumed")
}

// idlePhase derives the session's idle phase from the captured audio. A
// silent source suspends the pacer; the phase, the standby clock, and the
// tray read the source itself, so a receiver that cannot suspend still flips
// the phase while its continuous silence stream keeps running.
func (e *Engine) idlePhase(now time.Time) idlePhase {
	if e.tr == nil || e.tr.State() != sender.StateStreaming {
		return idleActive
	}
	if now.Sub(e.silence.LastLoud()) < audioPauseAfter {
		return idleActive
	}
	if e.darkEligible() && now.Sub(e.darkBase()) >= e.standbyAfter() {
		return idleDark
	}
	return idleSuspended
}

func (e *Engine) darkEligible() bool {
	return !e.setsValue.Lamp.Off &&
		e.setsValue.Standby.Enabled &&
		e.lampTargetCapable() &&
		e.standbyHoldAt.IsZero()
}

// darkBase is when the current idle stretch started for the dark threshold:
// the last audible block, or the session start a user connect carries. An
// automatic reconnect carries no baseline, so it counts from the last sound
// and an idle PC does not wake the room.
func (e *Engine) darkBase() time.Time {
	base := e.silence.LastLoud()
	if e.darkBaseline.After(base) {
		base = e.darkBaseline
	}
	if base.IsZero() {
		base = e.now()
	}
	return base
}

func (e *Engine) standbyAfter() time.Duration {
	return time.Duration(settings.NormalizeStandbyMinutes(e.setsValue.Standby.IdleMinutes)) * time.Minute
}

func (e *Engine) updateIdlePhase(now time.Time) {
	if !e.standbyHoldAt.IsZero() && e.silence.LastLoud().After(e.standbyHoldAt) {
		e.standbyHoldAt = time.Time{}
	}
	if e.tr == nil || e.tr.State() != sender.StateStreaming {
		if phase := e.idlePhase(now); phase != e.idle {
			e.idle = phase
			e.publish()
		}
		return
	}

	phase := e.idlePhase(now)
	if phase == e.idle {
		return
	}
	prev := e.idle
	switch phase {
	case idleActive:
		if prev != idleActive {
			// The resume re-anchors the RTP timeline; state 1 follows so the
			// re-anchored stream's first packet finds the display ready.
			e.resumeAudio()
		}
		if prev == idleDark {
			e.tr.SetPlaybackState(true)
			e.logf("standby: lamp on")
		}
	case idleSuspended, idleDark:
		if prev == idleActive {
			e.pauseAudio()
		}
		if phase == idleDark {
			e.tr.SetPlaybackState(false)
			e.logf("standby: lamp off after %s idle on %s", now.Sub(e.darkBase()).Round(time.Second), e.connectedName)
		} else if prev == idleDark {
			e.tr.SetPlaybackState(true)
		}
	}
	e.idle = phase
	e.publish()
}
