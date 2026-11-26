package sender

import "testing"

// TestStaleDialRejectsAnUnknownCompletion pins the fail-closed rule for dial
// identity: the host numbers every dial from one, so a completion that does not
// carry the current token is stale even when one side is zero. An unknown token
// is never permission to accept the completion.
func TestStaleDialRejectsAnUnknownCompletion(t *testing.T) {
	cases := []struct {
		name string
		gen  uint64
		dial uint64
		want bool
	}{
		{"the live dial", 5, 5, false},
		{"a replaced dial", 4, 5, true},
		{"an untokened completion for a live dial", 0, 5, true},
		{"a completion for no dial", 5, 0, true},
		{"no token on either side", 0, 0, false},
	}
	for _, tc := range cases {
		if got := staleDial(tc.gen, tc.dial); got != tc.want {
			t.Errorf("%s: staleDial(%d, %d) = %v, want %v", tc.name, tc.gen, tc.dial, got, tc.want)
		}
	}
}
