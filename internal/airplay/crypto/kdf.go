// SPDX-License-Identifier: Apache-2.0
//
// Ported from airplay2-sender-cpp (Apache-2.0), src/airplay_crypto.cpp,
// sections "hashing / KDF" and the CTR-DRBG wrapper.

package crypto

import (
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// HKDF derives `length` bytes with HKDF-SHA-512 (RFC 5869). Every session key
// (control write/read, event write/read) is HKDF over the pairing shared
// secret; the audio key is not (it is the raw first 32 bytes of the shared
// secret). An empty salt behaves like the reference's empty-string salt:
// HMAC with an all-zero key padded to the block size, which is what RFC 5869
// specifies when no salt is provided.
func HKDF(salt, info string, ikm []byte, length int) []byte {
	out := make([]byte, length)
	r := hkdf.New(sha512.New, ikm, []byte(salt), []byte(info))
	if _, err := io.ReadFull(r, out); err != nil {
		// Only reachable above 255*64 bytes, far past any real derivation.
		panic(fmt.Sprintf("crypto: HKDF length %d: %v", length, err))
	}
	return out
}

// RandomBytes returns n cryptographically secure random bytes, used for
// X25519 ephemerals, the SRP secret a, and Ed25519 seeds.
func RandomBytes(n int) []byte {
	if n < 0 {
		panic(fmt.Sprintf("crypto: RandomBytes negative length %d", n))
	}
	out := make([]byte, n)
	if _, err := rand.Read(out); err != nil {
		panic(fmt.Sprintf("crypto: rand: %v", err))
	}
	return out
}
