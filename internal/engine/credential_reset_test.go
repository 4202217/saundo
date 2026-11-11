package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"saundo/internal/airplay/creds"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// TestCredentialResetNoticeAndRecovery pins the issue 35 recovery surface:
// an unreadable file raises the notice, a fresh pairing writes into the
// rebuilt store and clears it, and the dialog's dismiss also clears it.
func TestCredentialResetNoticeAndRecovery(t *testing.T) {
	dir := t.TempDir()
	sets := settings.NewStoreAt(filepath.Join(dir, "settings.json"))
	credsPath := filepath.Join(dir, "credentials.dat")
	if err := os.WriteFile(credsPath, []byte("not a protected blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	regStore := receivers.NewStoreAt(filepath.Join(dir, "receivers.json"))
	if err := regStore.Save(receivers.Registry{Entries: []receivers.Entry{
		{Key: testDeviceID, Name: "客厅 HomePod"},
	}}); err != nil {
		t.Fatal(err)
	}
	h := newHarnessWithStores(t, sets, creds.NewStoreAt(credsPath), regStore)

	snap := h.e.Snapshot()
	if !snap.CredentialReset {
		t.Fatal("a quarantined credentials file did not surface the recovery notice")
	}
	if !strings.Contains(strings.Join(snap.LogLines, "\n"), "credentials file was unreadable") {
		t.Fatalf("the quarantine was not logged; lines: %q", snap.LogLines)
	}

	h.e.onCredentials(testDeviceID, "stored-json")
	if h.e.Snapshot().CredentialReset {
		t.Fatal("a saved pairing did not clear the recovery notice")
	}
	if got := h.e.creds.Load(testDeviceID); got != "stored-json" {
		t.Fatalf("the rebuilt store did not accept the new credential: %q", got)
	}

	h.e.credentialReset = true
	h.e.handleCommand(command{kind: cmdDismissCredentialReset})
	if h.e.Snapshot().CredentialReset {
		t.Fatal("dismiss did not clear the recovery notice")
	}
}
