package creds

import (
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
