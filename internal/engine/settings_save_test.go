package engine

import (
	"path/filepath"
	"testing"

	"saundo/internal/airplay/creds"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// TestEngineShutdownKeepsTheAppsChannelChoice pins the single-writer rule:
// the engine's shutdown flush must not write the update channel it read at
// startup back over the value the app saved since.
func TestEngineShutdownKeepsTheAppsChannelChoice(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	cfg := sets.Load()
	cfg.Beta = true
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	h := newHarnessWithStores(t, sets,
		creds.NewStoreAt(filepath.Join(dir, "creds")),
		receivers.NewStoreAt(filepath.Join(dir, "receivers.json")))
	// The engine's startup snapshot holds Beta=true. The app switches to
	// Stable while the engine runs, then the engine shuts down.
	cfg = sets.Load()
	cfg.Beta = false
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	h.e.shutdown()
	if sets.Load().Beta {
		t.Fatal("engine shutdown restored the app's old channel")
	}
}
