package crypto

import (
	"bytes"
	"crypto/sha512"
	"math/big"
	"testing"
)

// testSRPServer is a minimal SRP-6a server on the same RFC 5054 3072-bit
// group, written from the protocol definition rather than ported from the
// client: it derives the verifier from the password and computes
// S = (A * v^u)^b mod N, the mirror image of the client's
// S = (B - k*g^x)^(a + u*x) mod N. The two ends agree only if the client's
// padding and hashing conventions are right.
type testSRPServer struct {
	salt []byte
	v    *big.Int
	b    *big.Int
	B    *big.Int
	K    []byte
}

func newTestSRPServer(t *testing.T, password string, salt, bSeed []byte) *testSRPServer {
	t.Helper()
	inner := sha512.Sum512([]byte("Pair-Setup:" + password))
	saltAndHash := append(append([]byte(nil), salt...), inner[:]...)
	outer := sha512.Sum512(saltAndHash)
	x := new(big.Int).SetBytes(outer[:])

	v := new(big.Int).Exp(srpG, x, srpN)
	b := new(big.Int).SetBytes(bSeed)
	k := new(big.Int).SetBytes(srpHashPaddedPair(srpN, srpG))
	// B = (k*v + g^b) mod N
	gb := new(big.Int).Exp(srpG, b, srpN)
	kv := new(big.Int).Mod(new(big.Int).Mul(k, v), srpN)
	B := new(big.Int).Mod(new(big.Int).Add(kv, gb), srpN)
	return &testSRPServer{salt: salt, v: v, b: b, B: B}
}

// verify consumes the client's A and M1. It returns false when the proof
// does not match, and otherwise the server proof M2.
func (s *testSRPServer) verify(t *testing.T, clientA, m1 []byte) ([]byte, bool) {
	t.Helper()
	A := new(big.Int).SetBytes(clientA)

	u := new(big.Int).SetBytes(srpHashPaddedPair(A, s.B))
	// S = (A * v^u)^b mod N
	vu := new(big.Int).Exp(s.v, u, srpN)
	base := new(big.Int).Mod(new(big.Int).Mul(A, vu), srpN)
	S := new(big.Int).Exp(base, s.b, srpN)
	K := sum512(srpNaturalBytes(S))

	hN := sum512(srpNaturalBytes(srpN))
	hG := sum512(srpNaturalBytes(srpG))
	hXor := make([]byte, sha512.Size)
	for i := range hXor {
		hXor[i] = hN[i] ^ hG[i]
	}
	hI := sum512([]byte("Pair-Setup"))
	m1in := make([]byte, 0, 64+len(hI)+len(s.salt)+len(clientA)+len(s.B.Bytes())+64)
	m1in = append(m1in, hXor...)
	m1in = append(m1in, hI...)
	m1in = append(m1in, s.salt...)
	m1in = append(m1in, clientA...)
	m1in = append(m1in, s.B.Bytes()...)
	m1in = append(m1in, K...)
	expectM1 := sum512(m1in)
	if !bytes.Equal(expectM1, m1) {
		return nil, false
	}

	m2in := make([]byte, 0, len(clientA)+len(m1)+len(K))
	m2in = append(m2in, clientA...)
	m2in = append(m2in, m1...)
	m2in = append(m2in, K...)
	return sum512(m2in), true
}

// TestSRPRoundTrip drives a full pair-setup key exchange, with a fixed salt
// and server exponent for reproducibility. pyatv carries no fixed HAP SRP
// vector (it runs srptools on both ends), so the check is this two-sided
// round trip plus the wrong-password and tamper negatives below.
func TestSRPRoundTrip(t *testing.T) {
	salt := bytes.Repeat([]byte{0xC5}, 16)
	server := newTestSRPServer(t, "3939", salt, bytes.Repeat([]byte{0x7E}, 64))

	client := NewSRPClient()
	client.Start("3939")

	if !client.Process(salt, server.B.Bytes()) {
		t.Fatal("Process rejected a valid server B")
	}

	m2, ok := server.verify(t, client.PublicA(), client.ProofM1())
	if !ok {
		t.Fatal("server rejected the client proof M1")
	}
	if !client.VerifyServerProof(m2) {
		t.Fatal("client rejected the server proof M2")
	}
	if len(client.SessionKey()) != 64 {
		t.Fatalf("session key %d bytes, want 64", len(client.SessionKey()))
	}
	if len(client.ProofM1()) != 64 {
		t.Fatalf("M1 %d bytes, want 64", len(client.ProofM1()))
	}
}

func TestSRPWrongPasswordFails(t *testing.T) {
	salt := bytes.Repeat([]byte{0x11}, 16)
	server := newTestSRPServer(t, "3939", salt, bytes.Repeat([]byte{0x22}, 64))

	client := NewSRPClient()
	client.Start("1234")
	if !client.Process(salt, server.B.Bytes()) {
		t.Fatal("Process rejected a valid server B")
	}
	if _, ok := server.verify(t, client.PublicA(), client.ProofM1()); ok {
		t.Fatal("server accepted a proof computed with the wrong password")
	}
}

func TestSRPTamperedServerProofRejected(t *testing.T) {
	salt := bytes.Repeat([]byte{0x33}, 16)
	server := newTestSRPServer(t, "3939", salt, bytes.Repeat([]byte{0x44}, 64))

	client := NewSRPClient()
	client.Start("3939")
	if !client.Process(salt, server.B.Bytes()) {
		t.Fatal("Process rejected a valid server B")
	}
	m2, ok := server.verify(t, client.PublicA(), client.ProofM1())
	if !ok {
		t.Fatal("server rejected the client proof M1")
	}
	m2[0] ^= 1
	if client.VerifyServerProof(m2) {
		t.Fatal("tampered M2 accepted")
	}
	if client.VerifyServerProof(nil) {
		t.Fatal("empty M2 accepted")
	}
}

func TestSRPRejectsZeroB(t *testing.T) {
	salt := bytes.Repeat([]byte{0x55}, 16)
	client := NewSRPClient()
	client.Start("3939")

	for _, bad := range [][]byte{nil, srpN.Bytes(), make([]byte, 384)} {
		if client.Process(salt, bad) {
			t.Fatalf("Process accepted B = %x", bad)
		}
	}
	// A B longer than the group length is malformed; the reference's
	// fixed-size padding cannot represent it either.
	oversize := append(bytes.Repeat([]byte{0x01}, 384), 0x02)
	if client.Process(salt, oversize) {
		t.Fatal("Process accepted an oversized B")
	}
}

func TestSRPNotStarted(t *testing.T) {
	client := NewSRPClient()
	if client.PublicA() != nil {
		t.Fatal("PublicA before Start is not nil")
	}
	if client.ProofM1() != nil {
		t.Fatal("ProofM1 before Process is not nil")
	}
	if client.SessionKey() != nil {
		t.Fatal("SessionKey before Process is not nil")
	}
	if client.VerifyServerProof([]byte("x")) {
		t.Fatal("VerifyServerProof before Process succeeded")
	}
	if client.Process([]byte("salt"), big.NewInt(12345).Bytes()) {
		t.Fatal("Process before Start succeeded")
	}
}
