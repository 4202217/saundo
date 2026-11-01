package crypto

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"testing"

	"golang.org/x/crypto/hkdf"
)

// refHKDF is an independent HKDF-SHA-512 (RFC 5869 extract-then-expand),
// written from the RFC rather than via x/crypto, to check the package
// wrapper against.
func refHKDF(salt, info, ikm []byte, length int) []byte {
	extractor := hmac.New(sha512.New, salt)
	extractor.Write(ikm)
	prk := extractor.Sum(nil)

	var out, t []byte
	for i := byte(1); len(out) < length; i++ {
		h := hmac.New(sha512.New, prk)
		h.Write(t)
		h.Write(info)
		h.Write([]byte{i})
		t = h.Sum(nil)
		out = append(out, t...)
	}
	return out[:length]
}

// TestHKDFAgainstRFC5869Shape checks the wrapper against the independent
// implementation for the RFC 5869 test-case-1 inputs (the RFC publishes
// SHA-256/SHA-1 vectors only, so the SHA-512 expectation is computed here,
// not copied).
func TestHKDFAgainstRFC5869Shape(t *testing.T) {
	ikm := []byte{0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b,
		0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b,
		0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b}
	salt := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c}
	info := []byte{0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7,
		0xf8, 0xf9}

	got := HKDF(string(salt), string(info), ikm, 42)
	want := refHKDF(salt, info, ikm, 42)
	if !bytes.Equal(got, want) {
		t.Fatalf("HKDF mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestHKDFSeveralSaltsAndInfos(t *testing.T) {
	ikm := []byte("pairing shared secret bytes")
	cases := []struct {
		salt, info string
		length     int
	}{
		{"Pair-Setup-Encrypt-Salt", "Pair-Setup-Encrypt-Info", 32},
		{"Control-Salt", "Control-Write-Encryption-Key", 32},
		{"Control-Salt", "Control-Read-Encryption-Key", 32},
		{"", "", 32},
		{"", "info only", 64},
		{"salt only", "", 80},
		{"longer salt for the road", "multi-block output check", 16320},
	}
	for _, tc := range cases {
		got := HKDF(tc.salt, tc.info, ikm, tc.length)
		want := refHKDF([]byte(tc.salt), []byte(tc.info), ikm, tc.length)
		if !bytes.Equal(got, want) {
			t.Fatalf("HKDF(salt=%q, info=%q, len=%d) mismatch", tc.salt, tc.info, tc.length)
		}
		viaPkg := hkdf.New(sha512.New, ikm, []byte(tc.salt), []byte(tc.info))
		pkgOut := make([]byte, tc.length)
		if _, err := viaPkg.Read(pkgOut); err != nil {
			t.Fatalf("x/crypto/hkdf read: %v", err)
		}
		if !bytes.Equal(got, pkgOut) {
			t.Fatalf("HKDF(salt=%q, info=%q, len=%d) differs from x/crypto/hkdf", tc.salt, tc.info, tc.length)
		}
	}
}

// TestHKDFEmptySaltMatchesZeroSalt pins the convention the C++ reference
// gets from mbedtls: an empty salt and a hash-length zero salt derive the
// same key.
func TestHKDFEmptySaltMatchesZeroSalt(t *testing.T) {
	ikm := []byte("ikm")
	empty := HKDF("", "info", ikm, 32)
	zeros := HKDF(string(make([]byte, sha512.Size)), "info", ikm, 32)
	if !bytes.Equal(empty, zeros) {
		t.Fatal("empty salt and zero salt diverge")
	}
}
