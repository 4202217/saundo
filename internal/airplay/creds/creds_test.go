package creds

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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

func TestStoreRefusesCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	const blob = "not a DPAPI blob"
	if err := os.WriteFile(path, []byte(blob), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStoreAt(path)
	if got := s.LoadPassword("AA:BB"); got != "" {
		t.Fatalf("corrupt store read as %q", got)
	}
	if s.HasPassword("AA:BB") {
		t.Fatal("corrupt store reports a password")
	}
	if err := s.SavePassword("AA:BB", "secret"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("save error = %v, want ErrCorrupt", err)
	}
	if err := s.Save("AA:BB", `{"atvId":"00","clientId":"c","ltpk":"00","ltsk":"00"}`); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("HAP save error = %v, want ErrCorrupt", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != blob {
		t.Fatal("the corrupt credentials file was overwritten")
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
