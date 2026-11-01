package crypto

import (
	"bytes"
	"encoding/binary"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestChaCha20Poly1305RoundTrip(t *testing.T) {
	key := RandomBytes(32)
	nonce := CounterNonce8(42)
	plaintext := []byte("pair-verify payload bytes")
	aad := []byte{0x12, 0x00}

	ct, err := ChaCha20Poly1305Encrypt(key, nonce, plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(ct) != len(plaintext)+16 {
		t.Fatalf("ciphertext %d bytes, want %d (plaintext+tag)", len(ct), len(plaintext)+16)
	}
	got, err := ChaCha20Poly1305Decrypt(key, nonce, ct, aad)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch: %q", got)
	}
}

func TestChaCha20Poly1305EmptyPlaintext(t *testing.T) {
	key := RandomBytes(32)
	nonce := CounterNonce8(0)
	ct, err := ChaCha20Poly1305Encrypt(key, nonce, nil, nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(ct) != 16 {
		t.Fatalf("ciphertext %d bytes, want 16 (tag only)", len(ct))
	}
	if _, err := ChaCha20Poly1305Decrypt(key, nonce, ct, nil); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
}

func TestChaCha20Poly1305TamperRejection(t *testing.T) {
	key := RandomBytes(32)
	nonce := CounterNonce8(1)
	aad := []byte("AAD")
	ct, err := ChaCha20Poly1305Encrypt(key, nonce, []byte("secret audio"), aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip a ciphertext byte.
	badCt := append([]byte(nil), ct...)
	badCt[0] ^= 1
	if _, err := ChaCha20Poly1305Decrypt(key, nonce, badCt, aad); err == nil {
		t.Fatal("flipped ciphertext byte accepted")
	}

	// Flip a tag byte.
	badTag := append([]byte(nil), ct...)
	badTag[len(badTag)-1] ^= 1
	if _, err := ChaCha20Poly1305Decrypt(key, nonce, badTag, aad); err == nil {
		t.Fatal("flipped tag byte accepted")
	}

	// Flip an AAD byte.
	badAad := append([]byte(nil), aad...)
	badAad[0] ^= 1
	if _, err := ChaCha20Poly1305Decrypt(key, nonce, ct, badAad); err == nil {
		t.Fatal("flipped AAD byte accepted")
	}

	// Wrong nonce.
	if _, err := ChaCha20Poly1305Decrypt(key, CounterNonce8(2), ct, aad); err == nil {
		t.Fatal("wrong counter accepted")
	}
}

func TestChaCha20Poly1305RejectsBadSizes(t *testing.T) {
	nonce := CounterNonce8(0)
	if _, err := ChaCha20Poly1305Encrypt(make([]byte, 31), nonce, nil, nil); err == nil {
		t.Fatal("31-byte key accepted")
	}
	key := RandomBytes(32)
	if _, err := ChaCha20Poly1305Encrypt(key, make([]byte, 7), nil, nil); err == nil {
		t.Fatal("7-byte nonce accepted")
	}
	if _, err := ChaCha20Poly1305Decrypt(key, nonce, make([]byte, 15), nil); err == nil {
		t.Fatal("15-byte ciphertext accepted")
	}
}

// TestChaCha20Poly1305HAPNonceVector pins the HAP nonce layout byte for byte.
// The expected bytes are derived independently of the package wrapper: the
// AEAD is taken directly from x/crypto and sealed with the 12-byte nonce
// [0,0,0,0, counter little-endian] built in this test, mirroring pyatv's
// Struct("<LQ").pack(0, counter).
func TestChaCha20Poly1305HAPNonceVector(t *testing.T) {
	key := bytes.Repeat([]byte{0xAB}, 32)
	counter := uint64(0x0102030405060708)
	nonce8 := CounterNonce8(counter)
	plaintext := []byte("HAP nonce layout check")
	aad := []byte{0x01, 0x02}

	want, err := func() ([]byte, error) {
		nonce12 := make([]byte, 12)
		binary.LittleEndian.PutUint64(nonce12[4:], counter)
		aead, err := chacha20poly1305.New(key)
		if err != nil {
			return nil, err
		}
		return aead.Seal(nil, nonce12, plaintext, aad), nil
	}()
	if err != nil {
		t.Fatalf("independent seal: %v", err)
	}

	got, err := ChaCha20Poly1305Encrypt(key, nonce8, plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("HAP nonce vector mismatch:\n got %x\nwant %x", got, want)
	}

	dec, err := ChaCha20Poly1305Decrypt(key, nonce8, want, aad)
	if err != nil {
		t.Fatalf("Decrypt of independent ciphertext: %v", err)
	}
	if !bytes.Equal(dec, plaintext) {
		t.Fatalf("plaintext mismatch: %q", dec)
	}
}

func TestCounterNonce8Layout(t *testing.T) {
	got := CounterNonce8(0x0102030405060708)
	want := []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}
	if !bytes.Equal(got, want) {
		t.Fatalf("CounterNonce8 = %x, want little-endian %x", got, want)
	}
	if got := CounterNonce8(1); got[0] != 1 || got[7] != 0 {
		t.Fatalf("CounterNonce8(1) = %x", got)
	}
}
