//go:build darwin

package creds

import (
	"bytes"
	"testing"
)

// The env override must decide the key entirely: the same blob decrypts
// under its own key, fails under another, and a malformed value fails
// closed instead of falling back to the keychain.
func TestMasterKeyEnvOverride(t *testing.T) {
	const keyA = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	const keyB = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201000"
	plain := []byte("round trip through the override")

	t.Setenv(envMasterKey, keyA)
	blob, err := protect(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unprotect(blob)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip = %q, %v", got, err)
	}

	t.Setenv(envMasterKey, keyB)
	if _, err := unprotect(blob); err == nil {
		t.Fatal("unprotect succeeded under a different override key")
	}

	t.Setenv(envMasterKey, "not-hex")
	if _, err := protect(plain); err == nil {
		t.Fatal("protect succeeded with a malformed override")
	}

	t.Setenv(envMasterKey, keyA)
	if got, err = unprotect(blob); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip after resets = %q, %v", got, err)
	}
}
