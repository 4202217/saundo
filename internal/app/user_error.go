package app

import (
	"saundo/cmd/saundo/locales"
	"saundo/internal/engine"
)

// userErrorText resolves an engine key against the catalog. A technical
// error with no key keeps its own text.
func userErrorText(key engine.UserErrorKey, args []string, fallback string) string {
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	switch key {
	case engine.ErrDeviceMissing:
		return locales.ErrorDeviceMissing()
	case engine.ErrLatencyDeviceMissing:
		return locales.ErrorLatencyDeviceMissing()
	case engine.ErrLatencyOffline:
		return locales.ErrorLatencyOffline()
	case engine.ErrSavePassword:
		return locales.ErrorSavePassword(arg(0))
	case engine.ErrSaveReceiver:
		return locales.ErrorSaveReceiver(arg(0))
	case engine.ErrLocalReceiver:
		return locales.ErrorLocalReceiver()
	case engine.ErrStereoMembersOffline:
		return locales.ErrorStereoMembersOffline()
	case engine.ErrComboInvalid:
		return locales.ErrorComboInvalid()
	case engine.ErrFairPlay:
		return locales.ErrorFairPlay(arg(0))
	case engine.ErrPasswordRejected:
		return locales.ErrorPasswordRejected(arg(0))
	case engine.ErrEndpointMoved:
		return locales.ErrorEndpointMoved(arg(0))
	case engine.ErrSessionOpen:
		return locales.ErrorSessionOpen(arg(0))
	case engine.ErrClockConflict:
		return locales.ErrorClockConflict()
	case engine.ErrReconnectStopped:
		return locales.ErrorReconnectStopped(arg(0))
	case engine.ErrReceiverMissing:
		return locales.ErrorReceiverMissing()
	case engine.ErrDeletePassword:
		return locales.ErrorDeletePassword(arg(0))
	}
	return fallback
}

func (a *App) errorText() string {
	return userErrorText(a.snap.ErrorKey, a.snap.ErrorArgs, a.snap.Error)
}

// errorBarNeeded keeps the old failed-phase bar for a real message, and adds
// the connect and refused-add keys over a healthy stream. A takeover is the
// error phase with no message, so it must not draw an empty bar.
func errorBarNeeded(s engine.Snapshot) bool {
	return (phaseOf(s) == phaseError && (s.Error != "" || s.ErrorKey != "")) ||
		connectionErrorKey(s.ErrorKey)
}

// connectionErrorKey reports whether the key is a connect or refused-add
// failure. Those surface over a healthy stream; save and latency errors do
// not, because the session they describe is still running.
func connectionErrorKey(key engine.UserErrorKey) bool {
	switch key {
	case engine.ErrClockConflict, engine.ErrDeviceMissing, engine.ErrLocalReceiver,
		engine.ErrStereoMembersOffline, engine.ErrComboInvalid, engine.ErrFairPlay,
		engine.ErrEndpointMoved, engine.ErrSessionOpen, engine.ErrPasswordRejected:
		return true
	}
	return false
}
