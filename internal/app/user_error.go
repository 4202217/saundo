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
	case engine.ErrFairPlay:
		return locales.ErrorFairPlay(arg(0))
	case engine.ErrEndpointMoved:
		return locales.ErrorEndpointMoved(arg(0))
	case engine.ErrSessionOpen:
		return locales.ErrorSessionOpen(arg(0))
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
