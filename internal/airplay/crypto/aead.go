// SPDX-License-Identifier: Apache-2.0
//
// Ported from airplay2-sender-cpp (Apache-2.0), src/airplay_crypto.cpp,
// section "ChaCha20-Poly1305 AEAD".

package crypto

import (
	"crypto/subtle"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// CounterNonce8 builds the 8-byte little-endian counter nonce HAP uses for
// the audio and control channels.
func CounterNonce8(counter uint64) []byte {
	n := make([]byte, 8)
	for i := 0; i < 8; i++ {
		n[i] = byte(counter >> (8 * i))
	}
	return n
}

// hapNonce12 pads an 8-byte HAP nonce to the 12-byte IETF nonce: 4 zero bytes
// followed by the 8-byte value. This matches pyatv's
// Struct("<LQ").pack(0, counter) and the reference's pad12.
func hapNonce12(nonce8 []byte) []byte {
	n := make([]byte, 12)
	copy(n[4:], nonce8)
	return n
}

// ChaCha20Poly1305Encrypt encrypts plaintext with the HAP 8-byte nonce and
// returns ciphertext followed by the 16-byte tag. This is the AEAD of the
// encrypted RTSP control channel, the event channel, and the realtime audio
// payload. The key is a 32-byte HKDF output.
func ChaCha20Poly1305Encrypt(key, nonce8, plaintext, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: chacha key size %d, want 32", len(key))
	}
	if len(nonce8) != 8 {
		return nil, fmt.Errorf("crypto: chacha nonce size %d, want 8", len(nonce8))
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, hapNonce12(nonce8), plaintext, aad), nil
}

// ChaCha20Poly1305Decrypt decrypts ciphertext with the appended 16-byte tag.
// A bad tag (wrong key, wrong AAD, corruption) returns an error, which the
// caller treats as an authentication failure.
func ChaCha20Poly1305Decrypt(key, nonce8, ciphertext, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: chacha key size %d, want 32", len(key))
	}
	if len(nonce8) != 8 {
		return nil, fmt.Errorf("crypto: chacha nonce size %d, want 8", len(nonce8))
	}
	if len(ciphertext) < 16 {
		return nil, fmt.Errorf("crypto: chacha ciphertext %d bytes, want at least 16", len(ciphertext))
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, hapNonce12(nonce8), ciphertext, aad)
}

// constantTimeEqual compares two byte slices in constant time. Proofs and
// tags are compared against network-supplied values.
func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
