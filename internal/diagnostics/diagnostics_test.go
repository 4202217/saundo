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
