package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	return b
}

// TestX25519RFC7748Vector is the RFC 7748 section 6.1 Diffie-Hellman
// known-answer test.
func TestX25519RFC7748Vector(t *testing.T) {
	alicePriv := mustHex(t, "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a")
	alicePub := mustHex(t, "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a")
	bobPriv := mustHex(t, "5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb")
	bobPub := mustHex(t, "de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f")
	wantShared := mustHex(t, "4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742")

	gotAlicePub, err := X25519SharedSecret(alicePriv, curve25519Basepoint())
	if err != nil {
		t.Fatalf("alice public: %v", err)
	}
	if !bytes.Equal(gotAlicePub, alicePub) {
		t.Fatalf("alice public = %x, want %x", gotAlicePub, alicePub)
	}
	gotBobPub, err := X25519SharedSecret(bobPriv, curve25519Basepoint())
	if err != nil {
		t.Fatalf("bob public: %v", err)
	}
	if !bytes.Equal(gotBobPub, bobPub) {
		t.Fatalf("bob public = %x, want %x", gotBobPub, bobPub)
	}

	shared1, err := X25519SharedSecret(alicePriv, bobPub)
	if err != nil {
		t.Fatalf("alice shared: %v", err)
	}
	shared2, err := X25519SharedSecret(bobPriv, alicePub)
	if err != nil {
		t.Fatalf("bob shared: %v", err)
	}
	if !bytes.Equal(shared1, wantShared) {
		t.Fatalf("shared = %x, want %x", shared1, wantShared)
	}
	if !bytes.Equal(shared1, shared2) {
		t.Fatal("shared secrets disagree")
	}
}

// curve25519Basepoint returns the RFC 7748 basepoint, so a public key for a
// fixed private key can be reproduced through the package's own shared-secret
// path (scalarmult by the basepoint is the ECDH with the basepoint).
func curve25519Basepoint() []byte {
	bp := make([]byte, 32)
	bp[0] = 9
	return bp
}

func TestX25519GenerateClampedAndUsable(t *testing.T) {
	pub1, priv1, err := X25519Generate()
	if err != nil {
		t.Fatalf("X25519Generate: %v", err)
	}
	if len(pub1) != 32 || len(priv1) != 32 {
		t.Fatalf("key sizes pub=%d priv=%d, want 32/32", len(pub1), len(priv1))
	}
	if priv1[0]&0x07 != 0 || priv1[31]&0x80 != 0 || priv1[31]&0x40 == 0 {
		t.Fatalf("private key not clamped: %x", priv1)
	}

	pub2, priv2, err := X25519Generate()
	if err != nil {
		t.Fatalf("X25519Generate: %v", err)
	}
	s1, err := X25519SharedSecret(priv1, pub2)
	if err != nil {
		t.Fatalf("shared 1: %v", err)
	}
	s2, err := X25519SharedSecret(priv2, pub1)
	if err != nil {
		t.Fatalf("shared 2: %v", err)
	}
	if !bytes.Equal(s1, s2) {
		t.Fatal("generated pairs do not agree on the shared secret")
	}
}

func TestX25519SharedSecretRejectsLowOrderAndBadSizes(t *testing.T) {
	_, priv, err := X25519Generate()
	if err != nil {
		t.Fatalf("X25519Generate: %v", err)
	}
	if _, err := X25519SharedSecret(priv, make([]byte, 32)); err == nil {
		t.Fatal("all-zero peer public accepted")
	}
	if _, err := X25519SharedSecret(priv, make([]byte, 31)); err == nil {
		t.Fatal("31-byte peer public accepted")
	}
	if _, err := X25519SharedSecret(make([]byte, 31), make([]byte, 32)); err == nil {
		t.Fatal("31-byte private accepted")
	}
}

// TestEd25519RFC8032Vector is RFC 8032 section 7.1 TEST 1 (empty message).
func TestEd25519RFC8032Vector(t *testing.T) {
	seed := mustHex(t, "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	wantPub := mustHex(t, "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a")
	wantSig := mustHex(t, "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b")

	gotPub := Ed25519PublicFromSeed(seed)
	if !bytes.Equal(gotPub, wantPub) {
		t.Fatalf("public = %x, want %x", gotPub, wantPub)
	}
	gotSig := Ed25519Sign(seed, nil)
	if !bytes.Equal(gotSig, wantSig) {
		t.Fatalf("signature = %x, want %x", gotSig, wantSig)
	}
	if !Ed25519Verify(wantPub, nil, gotSig) {
		t.Fatal("RFC 8032 signature rejected")
	}
}

func TestEd25519RoundTripAndTamper(t *testing.T) {
	seed := RandomBytes(32)
	msg := []byte("device info: ltpk || identifier || session pub")
	sig := Ed25519Sign(seed, msg)
	if len(sig) != 64 {
		t.Fatalf("signature %d bytes, want 64", len(sig))
	}
	pub := Ed25519PublicFromSeed(seed)
	if !Ed25519Verify(pub, msg, sig) {
		t.Fatal("valid signature rejected")
	}

	tamperedMsg := append([]byte(nil), msg...)
	tamperedMsg[0] ^= 1
	if Ed25519Verify(pub, tamperedMsg, sig) {
		t.Fatal("tampered message accepted")
	}

	tamperedSig := append([]byte(nil), sig...)
	tamperedSig[10] ^= 1
	if Ed25519Verify(pub, msg, tamperedSig) {
		t.Fatal("tampered signature accepted")
	}

	otherPub := Ed25519PublicFromSeed(RandomBytes(32))
	if Ed25519Verify(otherPub, msg, sig) {
		t.Fatal("signature valid under the wrong key")
	}
	if Ed25519Verify(pub[:31], msg, sig) {
		t.Fatal("short public key accepted")
	}
	if Ed25519Verify(pub, msg, sig[:63]) {
		t.Fatal("short signature accepted")
	}
}
