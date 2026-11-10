package settings

import "reflect"

// Ownership rule: each setting has exactly one writer, so two snapshots of the
// file can be merged without one overwriting the other's fields. The app owns
// the preferences it edits directly. The engine owns the session state it
// persists. A field in neither list belongs to the store or the startup
// migrations, and no merge touches it.
var (
	appOwnedFields = []string{
		"ShowLatencySwitch",
		"ShowNowPlayingPanel",
		"PlaybackControls",
		"CoverColors",
		"CoverMode",
		"AudioLevel",
		"MuteLocalOnConnect",
		"TrayAnimation",
		"LaunchAtStart",
		"Language",
		"Theme",
		"Beta",
		"AutoUpdate",
		"AutoDownload",
		"SkippedVersion",
		"LastCheck",
	}

	engineOwnedFields = []string{
		"Name",
		"Volume",
		"VolumeByDevice",
		"LatencyMS",
		"LatencyMode",
		"LatencyCustomMS",
		"CaptureEndpointID",
		"CaptureEndpointName",
		"CaptureProcessPID",
		"CaptureProcessName",
		"LastDeviceID",
		"StrictReceiverAuth",
		"Reconnect",
		"Lamp",
	}

	storeOwnedFields = []string{
		"NameMigrated",
		"DeviceID",
		"Passwords",
		"AutoConnect",
	}
)

// MergeApp copies the app-owned fields of src into dst.
func MergeApp(dst, src *Settings) { merge(dst, src, appOwnedFields) }

// MergeEngine copies the engine-owned fields of src into dst.
func MergeEngine(dst, src *Settings) { merge(dst, src, engineOwnedFields) }

func merge(dst, src *Settings, fields []string) {
	dv, sv := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()
	for _, name := range fields {
		dv.FieldByName(name).Set(sv.FieldByName(name))
	}
}
