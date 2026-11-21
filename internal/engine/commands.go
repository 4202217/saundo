package engine

import "saundo/internal/receivers"

// commandKind is the vocabulary the UI drives the engine with. Every command
// runs on the engine goroutine through the bounded command channel; the
// dispatch lives in Engine.run.
type commandKind uint8

const (
	cmdConnect commandKind = iota
	cmdDisconnect
	cmdDisconnectTarget
	cmdTargetVolume
	cmdPin
	cmdDismissPassword
	cmdDismissCredentialReset
	cmdCaptureEndpoint
	cmdRefresh
	cmdStrictAuth
	cmdName
	cmdReconnect
	cmdLamp
	cmdStandby
	cmdTrayAnimation
	cmdMuteLocal
	cmdNowPlaying
	cmdLatency
	cmdLatencySettings
	cmdCaptureProcess
	cmdCaptureApps
	cmdPassword
	cmdQuit
	cmdAddReceiver
	cmdUpdateReceiver
	cmdRemoveReceiver
	// cmdLinkReceivers creates a stereo combo over two receiver keys, in
	// command order: the first plays left, the second right.
	cmdLinkReceivers
	// cmdUpdateCombo edits a combo in place: its name and the two receiver
	// keys that play left and right.
	cmdUpdateCombo
	// cmdUnlinkCombo deletes a stereo combo; its member receivers return to
	// standalone rows with their own settings.
	cmdUnlinkCombo
	// cmdSwapComboSides flips a combo's left and right members.
	cmdSwapComboSides
)

type command struct {
	kind      commandKind
	deviceID  string
	value     float64
	text      string
	name      string
	album     string
	cover     []byte
	coverMIME string
	reconnect bool
	show      bool
	lampCover bool
	minutes   int
	ids       []string
	names     []string
	label     string
	spec      receivers.Spec
	patch     receivers.Patch
}
