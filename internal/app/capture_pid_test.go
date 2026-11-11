package app

import "testing"

// TestCaptureAppPID pins the fail-closed parse: anything that is not a full
// unsigned 32-bit decimal id is unknown, not pid 0.
func TestCaptureAppPID(t *testing.T) {
	cases := []struct {
		id  string
		pid uint32
		ok  bool
	}{
		{"1234", 1234, true},
		{"0", 0, true},
		{"", 0, false},
		{"abc", 0, false},
		{"12x", 0, false},
		{"-1", 0, false},
		{"4294967296", 0, false},
	}
	for _, c := range cases {
		pid, ok := captureAppPID(c.id)
		if pid != c.pid || ok != c.ok {
			t.Errorf("captureAppPID(%q) = (%d,%v), want (%d,%v)", c.id, pid, ok, c.pid, c.ok)
		}
	}
}
