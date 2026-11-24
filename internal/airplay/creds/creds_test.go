package creds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretFormattingRedacts(t *testing.T) {
	key := Secret{1, 2, 3, 4}
	for _, got := range []string{
		fmt.Sprintf("%v", key),
		fmt.Sprintf("%s", key),
		fmt.Sprintf("%x", key),
		fmt.Sprintf("%q", key),
		fmt.Sprintf("%+v", key),
	} {
		if got != "[redacted]" {
			t.Fatalf("formatted secret = %q, want [redacted]", got)
		}
	}
	c := Credentials{ClientID: "c", LTSK: key}
	got := fmt.Sprintf("%+v", c)
	if !strings.Contains(got, "[redacted]") || strings.Contains(got, "[1 2 3 4]") || strings.Contains(got, "01020304") {
		t.Fatalf("credentials = %q, want the long-term key redacted", got)
	}
}

func TestMarshalParseRoundTrip(t *testing.T) {
	want := Credentials{
		ATVID:    []byte{0xaa, 0xbb},
		ClientID: "0f0e0d0c-1111-2222-3333-444455556666",
		LTPK:     []byte{1, 2, 3},
		LTSK:     make([]byte, 32),
	}
	text := Marshal(want)
	got, ok := Parse(text)
	if !ok {
		t.Fatalf("Parse(%q) failed", text)
	}
	if got.ClientID != want.ClientID || string(got.ATVID) != string(want.ATVID) ||
		string(got.LTPK) != string(want.LTPK) || len(got.LTSK) != 32 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestParseRejects(t *testing.T) {
	for _, text := range []string{
		"",
		"{}",
		`{"atvId":"zz","clientId":"x","ltpk":"00","ltsk":"00"}`,
		`{"atvId":"00","clientId":"","ltpk":"00","ltsk":"00"}`,
		"not json",
	} {
		if _, ok := Parse(text); ok {
			t.Fatalf("Parse(%q) accepted invalid credentials", text)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	s := NewStoreAt(path)
	if got := s.Load("AA:BB"); got != "" {
		t.Fatalf("empty store load = %q", got)
	}
	if err := s.Save("AA:BB", `{"atvId":"00","clientId":"c","ltpk":"00","ltsk":"00"}`); err != nil {
		t.Fatal(err)
	}
	if got := s.Load("AA:BB"); got == "" {
		t.Fatal("saved credentials not found")
	}
	if err := s.Delete("AA:BB"); err != nil {
		t.Fatal(err)
	}
	if got := s.Load("AA:BB"); got != "" {
		t.Fatalf("deleted credentials still present: %q", got)
	}
}

// TestCorruptStoreIsQuarantinedAndRebuilt pins the recovery: an unreadable
// file is moved aside, reads stay empty, and the next save creates a fresh
// store instead of failing forever.
func TestCorruptStoreIsQuarantinedAndRebuilt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	const blob = "not a DPAPI blob"
	if err := os.WriteFile(path, []byte(blob), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStoreAt(path)
	if got := s.Load("AA:BB"); got != "" {
		t.Fatalf("corrupt store load = %q, want empty", got)
	}
	q := s.TakeQuarantine()
	if !strings.HasPrefix(q, path+".corrupt-") {
		t.Fatalf("quarantine path = %q, want %s.corrupt-*", q, path)
	}
	data, err := os.ReadFile(q)
	if err != nil {
		t.Fatalf("quarantined file: %v", err)
	}
	if string(data) != blob {
		t.Fatal("the quarantined file does not hold the original bytes")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the unreadable file still sits at the live path: %v", err)
	}
	if got := s.LoadPassword("AA:BB"); got != "" {
		t.Fatalf("password after quarantine = %q, want empty", got)
	}
	if s.HasPassword("AA:BB") {
		t.Fatal("quarantined store reports a password")
	}
	if err := s.SavePassword("AA:BB", "secret"); err != nil {
		t.Fatalf("save password after quarantine: %v", err)
	}
	if got := s.LoadPassword("AA:BB"); got != "secret" {
		t.Fatalf("password after rebuild = %q", got)
	}
	if err := s.Save("AA:BB", `{"atvId":"00","clientId":"c","ltpk":"00","ltsk":"00"}`); err != nil {
		t.Fatalf("HAP save after quarantine: %v", err)
	}
	if s.TakeQuarantine() != "" {
		t.Fatal("the store reported a second quarantine")
	}
}

// TestLoadKeepsPreviousOnReadError pins the rule that a failed read is
// unknown, not empty: a transient I/O failure must not read as "unpaired",
// which would silently re-pair the receiver.
func TestLoadKeepsPreviousOnReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	s := NewStoreAt(path)
	if err := s.Save("dev", `{"atvId":"aa"}`); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Swap the file for a directory so ReadFile fails with a non-not-exist
	// error while the store still holds its last good map.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := s.Load("dev"); got != `{"atvId":"aa"}` {
		t.Fatalf("Load after read error = %q, want the last known credentials", got)
	}
}

// TestHasPasswordKeepsPreviousOnReadError pins the projection: a failed read
// keeps the last good map, so the receivers merge does not read "no
// password" and re-pair the receiver.
func TestHasPasswordKeepsPreviousOnReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	s := NewStoreAt(path)
	if err := s.SavePassword("AA:BB", "secret"); err != nil {
		t.Fatalf("save password: %v", err)
	}
	// Swap the file for a directory so ReadFile fails with a non-not-exist
	// error while the store still holds its last good map.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if !s.HasPassword("AA:BB") {
		t.Fatal("HasPassword after read error = false, want the last known password")
	}
}

// TestKeyUnavailableDoesNotQuarantine pins the decrypt-error taxonomy: a key
// side failure keeps the blob and fails closed; only an authentic decrypt
// failure moves the file aside.
func TestKeyUnavailableDoesNotQuarantine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	s := NewStoreAt(path)
	if err := s.Save("dev", `{"atvId":"aa"}`); err != nil {
		t.Fatalf("save: %v", err)
	}
	orig := unprotectFunc
	unprotectFunc = func([]byte) ([]byte, error) {
		return nil, fmt.Errorf("%w: keychain locked", errKeyUnavailable)
	}
	defer func() { unprotectFunc = orig }()
	if got := s.Load("dev"); got != `{"atvId":"aa"}` {
		t.Fatalf("Load on key unavailable = %q, want the last known credentials", got)
	}
	if p := s.TakeQuarantine(); p != "" {
		t.Fatalf("key unavailable quarantined to %q, want no quarantine", p)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("credentials file was removed: %v", err)
	}
	if err := s.Save("dev", `{"atvId":"bb"}`); err == nil {
		t.Fatal("Save on key unavailable succeeded, want fail closed")
	}
}

// TestAuthFailureStillQuarantines pins the other side of the taxonomy.
func TestAuthFailureStillQuarantines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	s := NewStoreAt(path)
	if err := s.Save("dev", `{"atvId":"aa"}`); err != nil {
		t.Fatalf("save: %v", err)
	}
	orig := unprotectFunc
	unprotectFunc = func([]byte) ([]byte, error) {
		return nil, fmt.Errorf("creds: decrypt: cipher: message authentication failed")
	}
	defer func() { unprotectFunc = orig }()
	_ = s.Load("dev")
	if p := s.TakeQuarantine(); p == "" {
		t.Fatal("an authentic decrypt failure did not quarantine the file")
	}
}

// TestNullPlaintextDoesNotPanic pins the nil-map guard: a blob that
// decrypts to the JSON literal null must read as an empty store, and a Save
// on top of it must not panic.
func TestNullPlaintextDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	s := NewStoreAt(path)
	// The store needs an existing file so readAll reaches the decrypt step.
	if err := s.Save("seed", `{"atvId":"00"}`); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	orig := unprotectFunc
	unprotectFunc = func([]byte) ([]byte, error) {
		return []byte("null"), nil
	}
	defer func() { unprotectFunc = orig }()
	if got := s.Load("dev"); got != "" {
		t.Fatalf("Load on null plaintext = %q, want empty", got)
	}
	if err := s.Save("dev", `{"atvId":"aa"}`); err != nil {
		t.Fatalf("save over null plaintext: %v", err)
	}
	unprotectFunc = orig
	if got := s.Load("dev"); got != `{"atvId":"aa"}` {
		t.Fatalf("Load after save = %q, want the saved credentials", got)
	}
}

func TestPasswordNamespaceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	s := NewStoreAt(path)
	if s.HasPassword("AA:BB") {
		t.Fatal("empty store reports a password")
	}
	if got := s.LoadPassword("AA:BB"); got != "" {
		t.Fatalf("empty store load = %q", got)
	}
	if err := s.SavePassword("AA:BB", "secret"); err != nil {
		t.Fatal(err)
	}
	if got := s.LoadPassword("AA:BB"); got != "secret" {
		t.Fatalf("load = %q, want secret", got)
	}
	if !s.HasPassword("AA:BB") {
		t.Fatal("saved password not reported")
	}
	// The HAP key namespace and the password namespace stay separate.
	if err := s.Save("AA:BB", `{"atvId":"00","clientId":"c","ltpk":"00","ltsk":"00"}`); err != nil {
		t.Fatal(err)
	}
	if got := s.LoadPassword("AA:BB"); got != "secret" {
		t.Fatalf("HAP save clobbered the password: %q", got)
	}
	if got := s.Load("AA:BB"); got == "" {
		t.Fatal("password save clobbered the HAP keys")
	}
	if err := s.SavePassword("AA:BB", ""); err != nil {
		t.Fatal(err)
	}
	if s.HasPassword("AA:BB") {
		t.Fatal("empty save did not delete the password")
	}
}
