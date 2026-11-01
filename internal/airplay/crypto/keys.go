// SPDX-License-Identifier: Apache-2.0
//
// Ported from airplay2-sender-cpp (Apache-2.0), src/airplay_crypto.cpp,
// sections "X25519" and "Ed25519".

package crypto

import (
	"crypto/ed25519"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// X25519Generate returns a fresh X25519 key pair, 32 bytes each. The stored
// private key is clamped so a later X25519SharedSecret with the same private
// key is consistent with the published public key.
func X25519Generate() (pub, priv []byte, err error) {
	priv = RandomBytes(32)
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	pub, err = curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: x25519 base mult: %w", err)
	}
	return pub, priv, nil
}

// X25519SharedSecret computes the 32-byte ECDH shared secret of pair-verify.
// theirPub is attacker-controlled (a receiver's TLV PublicKey), so anything
// but 32 bytes is rejected. An all-zero output means the peer sent a
// low-order point that would force a known, predictable secret; a legitimate
// receiver never yields one.
func X25519SharedSecret(ourPriv, theirPub []byte) ([]byte, error) {
	if len(ourPriv) != 32 {
		return nil, fmt.Errorf("crypto: x25519 private size %d, want 32", len(ourPriv))
	}
	if len(theirPub) != 32 {
		return nil, fmt.Errorf("crypto: x25519 public size %d, want 32", len(theirPub))
	}
	out, err := curve25519.X25519(ourPriv, theirPub)
	if err != nil {
		return nil, fmt.Errorf("crypto: x25519: %w", err)
	}
	acc := byte(0)
	for _, b := range out {
		acc |= b
	}
	if acc == 0 {
		return nil, fmt.Errorf("crypto: x25519 all-zero shared secret (low-order peer key)")
	}
	return out, nil
}

// Ed25519PublicFromSeed derives the 32-byte long-term public key (ltpk) from
// the 32-byte long-term secret seed (ltsk).
func Ed25519PublicFromSeed(seed []byte) []byte {
	key := ed25519.NewKeyFromSeed(seed)
	pub := key.Public().(ed25519.PublicKey)
	return append([]byte(nil), pub...)
}

// Ed25519Sign signs msg with the key derived from seed.
func Ed25519Sign(seed, msg []byte) []byte {
	return ed25519.Sign(ed25519.NewKeyFromSeed(seed), msg)
}

// Ed25519Verify reports whether sig is a valid 64-byte signature of msg by
// the 32-byte public key.
func Ed25519Verify(pub, msg, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}
