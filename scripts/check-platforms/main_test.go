package main

import "testing"

// TestGoBinaryHonorsGO pins the documented GO= override on the gate that runs
// the cross-platform builds.
func TestGoBinaryHonorsGO(t *testing.T) {
	t.Setenv("GO", "/opt/go/bin/go")
	if got := goBinary(); got != "/opt/go/bin/go" {
		t.Fatalf("goBinary = %q, want the GO override", got)
	}
	t.Setenv("GO", "")
	if got := goBinary(); got != "go" {
		t.Fatalf("goBinary without GO = %q, want go", got)
	}
}
