package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saundo.log")
	l := New(path)
	l.maxBytes = 64
	for i := 0; i < 6; i++ {
		l.Printf("line-%d %s", i, strings.Repeat("x", 40))
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
	if len(l.Lines()) != 6 {
		t.Fatalf("lines = %d, want 6", len(l.Lines()))
	}
	l.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) > 200 {
		t.Fatalf("current log is %d bytes; rotation did not truncate", len(data))
	}
	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rotated) == 0 {
		t.Fatal("rotated file is empty")
	}
}

// TestUnopenableLogIsReported pins the fallback's visibility: when the file
// cannot be created, the in-memory log says why instead of staying empty.
func TestUnopenableLogIsReported(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := New(filepath.Join(blocker, "saundo.log"))
	lines := l.Lines()
	if len(lines) == 0 {
		t.Fatal("the in-memory log did not report the open failure")
	}
	found := false
	for _, line := range lines {
		if strings.Contains(line, "diagnostics: cannot create") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("lines = %v, want a diagnostics: cannot create line", lines)
	}
	l.Close()
}

// TestPrintfAfterCloseIsNoOp pins the shutdown guard: a late logger must not
// write a closed file or panic.
func TestPrintfAfterCloseIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saundo.log")
	l := New(path)
	l.Printf("before-close")
	l.Close()
	l.Printf("after-close") // must not panic or write
	if n := len(l.Lines()); n != 1 {
		t.Fatalf("lines = %d, want only the pre-close line", n)
	}
}
