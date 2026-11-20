package engine

// UserErrorKey names a message the UI resolves against its catalog. The
// engine never builds a user-facing sentence, so the same failure reads in
// the interface's language.
type UserErrorKey string

const (
	ErrDeviceMissing        UserErrorKey = "deviceMissing"
	ErrLatencyDeviceMissing UserErrorKey = "latencyDeviceMissing"
	ErrLatencyOffline       UserErrorKey = "latencyOffline"
	ErrSavePassword         UserErrorKey = "savePassword"
	ErrSaveReceiver         UserErrorKey = "saveReceiver"
	ErrLocalReceiver        UserErrorKey = "localReceiver"
	ErrStereoMembersOffline UserErrorKey = "stereoMembersOffline"
	// ErrComboInvalid rejects a link whose two receivers cannot form a combo:
	// one side is missing, already linked, a native pair, the local host, or
	// not an AirPlay 2 receiver.
	ErrComboInvalid  UserErrorKey = "comboInvalid"
	ErrFairPlay      UserErrorKey = "fairPlay"
	ErrEndpointMoved UserErrorKey = "endpointMoved"
	ErrSessionOpen   UserErrorKey = "sessionOpen"
	// ErrClockConflict rejects a clock mix the process cannot host: a set of
	// standalone PTP receivers beside a sender-side PTP master. Args are one
	// target name from each family.
	ErrClockConflict    UserErrorKey = "clockConflict"
	ErrReconnectStopped UserErrorKey = "reconnectStopped"
	ErrReceiverMissing  UserErrorKey = "receiverMissing"
	ErrDeletePassword   UserErrorKey = "deletePassword"
	// ErrPasswordRejected means the receiver refused the saved password.
	// Args are its display name.
	ErrPasswordRejected UserErrorKey = "passwordRejected"
)

// UserErrorKeys lists every key the UI must resolve. The app's resolver test
// walks it, so a new key cannot land without a catalog message.
var UserErrorKeys = []UserErrorKey{
	ErrDeviceMissing,
	ErrLatencyDeviceMissing,
	ErrLatencyOffline,
	ErrSavePassword,
	ErrSaveReceiver,
	ErrLocalReceiver,
	ErrStereoMembersOffline,
	ErrComboInvalid,
	ErrFairPlay,
	ErrEndpointMoved,
	ErrSessionOpen,
	ErrClockConflict,
	ErrReconnectStopped,
	ErrReceiverMissing,
	ErrDeletePassword,
	ErrPasswordRejected,
}

// fail records a keyed user error. The key is also the fallback string, so
// the existing non-empty checks and logs keep working without a sentence.
func (e *Engine) fail(key UserErrorKey, args ...string) {
	e.lastError = string(key)
	e.lastErrorKey = key
	e.lastErrorArgs = args
}

// failText records a technical error with no catalog key. The UI shows it
// only when the key is empty.
func (e *Engine) failText(text string) {
	e.lastError = text
	e.lastErrorKey = ""
	e.lastErrorArgs = nil
}

// clearError clears the keyed and technical error together. Every reset must
// use it, or a later raw error can inherit a stale key.
func (e *Engine) clearError() {
	e.lastError = ""
	e.lastErrorKey = ""
	e.lastErrorArgs = nil
}
