package main

import (
	"os"
	"path/filepath"
	"testing"
)

// seedRoot builds the minimal tree loadIndex needs, with the given SKILL.md
// content. It returns the root.
func seedRoot(t *testing.T, skill string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module test\n")
	write("cmd/saundo/locales/en.json", "{}\n")
	write("internal/keep.go", "package internal\n")
	write("scripts/keep.go", "package scripts\n")
	write("docs/protocol.md", "")
	write("docs/devices.md", "")
	write(".claude/skills/verify/SKILL.md", skill)
	return root
}

// TestVerifyMapMissingMapDirFails pins the fail-closed contract: a moved or
// uncommitted map must not make the gate silently green.
func TestVerifyMapMissingMapDirFails(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := run(root); code == 0 {
		t.Fatal("missing map directory exited 0")
	}
}

func TestVerifyMapNotADirectoryFails(t *testing.T) {
	root := seedRoot(t, "")
	path := filepath.Join(root, mapDir)
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := run(root); code == 0 {
		t.Fatal("a map path that is not a directory exited 0")
	}
}

func TestVerifyMapDanglingTestReferenceFails(t *testing.T) {
	root := seedRoot(t, "The map cites `TestNoSuchTest`.\n")
	if code := run(root); code != 1 {
		t.Fatalf("dangling test reference exit = %d, want 1", code)
	}
}

func TestVerifyMapValidMapPasses(t *testing.T) {
	root := seedRoot(t, "This page states no map assertions.\n")
	if code := run(root); code != 0 {
		t.Fatalf("valid map exit = %d, want 0", code)
	}
}
