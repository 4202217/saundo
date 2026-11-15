package sender

import (
	"fmt"
	"strings"
	"testing"

	"saundo/internal/airplay/creds"
)

// TestAP2StateRedactsLongTermSeed pins the copy: the credential field is
// redacted, and the sender's working copy must not print either.
func TestAP2StateRedactsLongTermSeed(t *testing.T) {
	st := &ap2State{ltSeed: creds.Secret{1, 2, 3, 4}}
	if got := fmt.Sprintf("%v", st.ltSeed); got != "[redacted]" {
		t.Fatalf("ltSeed = %q, want [redacted]", got)
	}
	got := fmt.Sprintf("%+v", st)
	if strings.Contains(got, "[1 2 3 4]") || strings.Contains(got, "01020304") {
		t.Fatalf("ap2State = %q, want the long-term seed redacted", got)
	}
}
