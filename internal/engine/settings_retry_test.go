package engine

import (
	"os"
	"path/filepath"
	"testing"

	"saundo/internal/airplay/creds"
	"saundo/internal/diagnostics"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// TestSettingsSaveFailureRetries pins the lost-preference path: a store that
// cannot write must log the failure once, keep the write pending, and succeed
// on the next debounce once the filesystem recovers.
func TestSettingsSaveFailureRetries(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	sets := settings.NewStoreAt(filepath.Join(blocked, "settings.json"))
	h := newHarnessWithLog(t, sets, creds.NewStoreAt(filepath.Join(dir, "creds")), receivers.NewStoreAt(filepath.Join(dir, "receivers.json")), diagnostics.New(filepath.Join(dir, "saundo.log")))
	defer h.e.log.Close()

	h.e.setsValue.Volume = 42
	h.e.saveSettingsNow()
	if h.e.settingsDirtyAt.IsZero() {
		t.Fatal("a failed settings save left no retry pending")
	}
	if got := countLogLines(h, "settings save failed"); got != 1 {
		t.Fatalf("settings save failure logged %d times, want 1: %q", got, h.e.log.Lines())
	}
	h.e.saveSettingsNow()
	if got := countLogLines(h, "settings save failed"); got != 1 {
		t.Fatalf("a repeated failure logged %d times, want 1: %q", got, h.e.log.Lines())
	}

	// The filesystem recovers; the next debounce flushes the pending write.
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	h.advance(settingsSaveDebounce)
	h.e.onWake()
	if !h.e.settingsDirtyAt.IsZero() {
		t.Fatal("the successful retry left the write pending")
	}
	if got := sets.Load().Volume; got != 42 {
		t.Fatalf("stored volume = %v, want 42", got)
	}
}
