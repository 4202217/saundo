//go:build windows

package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/engine"
)

// TestConnectionMuteCopyNamesWindowsAppCapture pins the Windows caveat: the
// per-app selection still mutes the whole output device there.
func TestConnectionMuteCopyNamesWindowsAppCapture(t *testing.T) {
	a := previewApp()
	a.page = pageConnection
	a.cfg.MuteLocalOnConnect = true
	a.snap.CaptureProcessPID = 1234
	a.snap.CaptureProcessName = "Music"
	a.snap.MuteLocal = engine.MuteLocalActive
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		locales.SettingsMuteLocal(),
		locales.SettingsMuteLocalWindowsAppNote(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("connection page is missing %q; texts: %q", want, tt.Texts())
		}
	}
}
