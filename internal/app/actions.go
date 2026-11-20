package app

// Action names one view action. Views send it through act, so production and
// the preview registry share one identifier, and an unregistered action fails
// closed in preview.
type Action string

const (
	ActionRefresh          Action = "refresh"
	ActionSettings         Action = "settings"
	ActionDisconnect       Action = "disconnect"
	ActionConnect          Action = "connect"
	ActionVolume           Action = "volume"
	ActionTargetVolume     Action = "target-volume"
	ActionTargetMute       Action = "target-mute"
	ActionTargetChannel    Action = "target-channel"
	ActionDisconnectTarget Action = "disconnect-target"
	ActionReceiverAdd      Action = "receiver-add"
	ActionReceiverUpdate   Action = "receiver-update"
	ActionReceiverRemove   Action = "receiver-remove"
	ActionReceiverPassword Action = "receiver-password"
	ActionReceiverHidden   Action = "receiver-hidden"
	ActionReceiverAuto     Action = "receiver-auto"
	ActionReceiverDelay    Action = "receiver-delay"
	ActionName             Action = "name"
	ActionLaunch           Action = "launch"
	ActionReconnect        Action = "reconnect"
	ActionStrict           Action = "strict"
	ActionLanguage         Action = "language"
)

// allActions is every action the views send through act. The preview's
// registry test requires a handler for each entry and no handler outside it.
var allActions = []Action{
	ActionRefresh,
	ActionSettings,
	ActionDisconnect,
	ActionConnect,
	ActionVolume,
	ActionTargetVolume,
	ActionTargetMute,
	ActionTargetChannel,
	ActionDisconnectTarget,
	ActionReceiverAdd,
	ActionReceiverUpdate,
	ActionReceiverRemove,
	ActionReceiverPassword,
	ActionReceiverHidden,
	ActionReceiverAuto,
	ActionReceiverDelay,
	ActionName,
	ActionLaunch,
	ActionReconnect,
	ActionStrict,
	ActionLanguage,
}
