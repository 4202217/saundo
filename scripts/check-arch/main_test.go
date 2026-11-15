package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seed(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module saundo\n")
	for rel, src := range files {
		write(t, root, rel, src)
	}
	return root
}

func write(t *testing.T, root, rel, src string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCleanTreePasses(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/app/app.go":               "package app\nimport \"github.com/egoist/mygo\"\nvar _ = mygo.Window{}\n",
		"internal/engine/engine.go":         "package engine\nimport \"time\"\nfunc Run() { t := time.NewTimer(time.Second); t.Stop(); ch := make(chan struct{}, 1); _ = ch }\n",
		"internal/airplay/sender/sender.go": "package sender\nfunc Start() {}\n",
	})
	if err := run(root); err != nil {
		t.Fatalf("clean tree: %v", err)
	}
}

func TestGoroutineInSansIOPackage(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/airplay/sender/sender.go": "package sender\nfunc Start() { go func() {}() }\n",
	})
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "sans-I/O and owns no threads") {
		t.Fatalf("goroutine leak = %v, want a sans-I/O violation", err)
	}
}

func TestGoroutineOutsideSansIOAllowed(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\nfunc Start() { go func() {}() }\n",
	})
	if err := run(root); err != nil {
		t.Fatalf("engine goroutine = %v, want no violation", err)
	}
}

func TestTimeAfterBanned(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\nimport \"time\"\nfunc Wait() { <-time.After(time.Second) }\n",
	})
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "time.After cannot be stopped") {
		t.Fatalf("time.After = %v, want a timer violation", err)
	}
}

func TestTimeNewTimerAllowed(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\nimport \"time\"\nfunc Wait() { t := time.NewTimer(time.Second); defer t.Stop(); <-t.C }\n",
	})
	if err := run(root); err != nil {
		t.Fatalf("time.NewTimer = %v, want no violation", err)
	}
}

func TestUnbufferedDataChannelBanned(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\nfunc Run() { ch := make(chan int); _ = ch }\n",
	})
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "data channel with no capacity") {
		t.Fatalf("unbuffered channel = %v, want a channel violation", err)
	}
}

func TestChannelTypeAliasBanned(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\ntype events chan int\nfunc Run() { ch := make(events); _ = ch }\n",
	})
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "through the type alias events") {
		t.Fatalf("channel alias = %v, want a channel violation", err)
	}
}

func TestChannelAliasWithCapacityAllowed(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\ntype events chan int\nfunc Run() { ch := make(events, 8); _ = ch }\n",
	})
	if err := run(root); err != nil {
		t.Fatalf("aliased channel with capacity: %v", err)
	}
}

func TestAfterFuncBannedInSansIOPackage(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/airplay/sender/sender.go": "package sender\nimport \"time\"\nfunc Start() { _ = time.AfterFunc(time.Second, func() {}) }\n",
	})
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "starts a goroutine") {
		t.Fatalf("AfterFunc = %v, want a sans-I/O violation", err)
	}
}

func TestAfterFuncOutsideSansIOAllowed(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\nimport \"time\"\nfunc Start() { _ = time.AfterFunc(time.Second, func() {}) }\n",
	})
	if err := run(root); err != nil {
		t.Fatalf("engine AfterFunc = %v, want no violation", err)
	}
}

func TestSignalChannelAllowed(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\nfunc Run() { ch := make(chan struct{}); _ = ch }\n",
	})
	if err := run(root); err != nil {
		t.Fatalf("signal channel = %v, want no violation", err)
	}
}

func TestTestsMayUseTimeAfter(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine_test.go": "package engine\nimport \"time\"\nfunc wait() { <-time.After(time.Second) }\n",
	})
	if err := run(root); err != nil {
		t.Fatalf("test timer = %v, want no violation", err)
	}
}

func TestMygoImportOutsideUI(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\nimport \"github.com/egoist/mygo\"\nvar _ = mygo.Window{}\n",
	})
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "internal/engine/engine.go imports github.com/egoist/mygo") {
		t.Fatalf("mygo leak = %v, want an import violation", err)
	}
}

func TestAppImportOutsideCmd(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine.go": "package engine\nimport \"saundo/internal/app\"\nvar _ = app.Main\n",
	})
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "imports saundo/internal/app") {
		t.Fatalf("app leak = %v, want an import violation", err)
	}
}

func TestEngineImportedFromLowerLayer(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/airplay/sender/sender.go": "package sender\nimport \"saundo/internal/engine\"\nvar _ = engine.New\n",
	})
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "imports saundo/internal/engine") {
		t.Fatalf("engine leak = %v, want an import violation", err)
	}
}

func TestTestFilesKeepTheImportRule(t *testing.T) {
	root := seed(t, map[string]string{
		"internal/engine/engine_test.go": "package engine\nimport \"saundo/internal/app\"\nvar _ = app.Main\n",
	})
	if err := run(root); err == nil {
		t.Fatal("an engine test importing app passed; the seam applies to tests too")
	}
}
