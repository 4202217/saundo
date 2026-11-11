package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"saundo/internal/diagnostics"
	"saundo/internal/settings"
)

// TestSaveSettingsLogsAStoreFailureOnce pins the app-owned half: a preference
// toggle that cannot be written is logged once, not on every edit, and a later
// success clears the latch.
func TestSaveSettingsLogsAStoreFailureOnce(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	sets := settings.NewStoreAt(filepath.Join(blocked, "settings.json"))
	log := diagnostics.New(filepath.Join(dir, "saundo.log"))
	defer log.Close()
	a := &App{sets: sets, cfg: settings.Default(), log: log}

	a.saveSettings()
	a.saveSettings()
	if got := countAppLogLines(log.Lines(), "settings save failed"); got != 1 {
		t.Fatalf("settings save failure logged %d times, want 1: %q", got, log.Lines())
	}
	if a.settingsErrorLogged == "" {
		t.Fatal("the failure did not latch")
	}

	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	a.saveSettings()
	if a.settingsErrorLogged != "" {
		t.Fatalf("a successful save kept the failure latch: %q", a.settingsErrorLogged)
	}
	if _, err := os.Stat(filepath.Join(blocked, "settings.json")); err != nil {
		t.Fatalf("settings file not written after recovery: %v", err)
	}
}

func countAppLogLines(lines []string, want string) int {
	n := 0
	for _, line := range lines {
		if strings.Contains(line, want) {
			n++
		}
	}
	return n
}
