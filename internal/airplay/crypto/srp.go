// SPDX-License-Identifier: Apache-2.0
//
// Ported from airplay2-sender-cpp (Apache-2.0), src/airplay_crypto.cpp,
// section "SRP-6a 3072 SHA-512, client side". The padding conventions
// (k and u hashed over N-length zero-padded big-endian operands; salt, N, g,
// A, B and S hashed at natural length) follow pyatv's srptools usage and
// pair_ap's H_nn_pad, and are what real Apple receivers expect.

package crypto

import (
	"crypto/sha512"
	"math/big"
)

// srpNHex is the RFC 5054 3072-bit group, g = 5. Big-endian hex.
const srpNHex = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74" +
	"020BBEA63B139B22514A08798E3404DDEF9519B3CD3A431B302B0A6DF25F1437" +
	"4FE1356D6D51C245E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
	"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3DC2007CB8A163BF05" +
	"98DA48361C55D39A69163FA8FD24CF5F83655D23DCA3AD961C62F356208552BB" +
	"9ED529077096966D670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
	"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9DE2BCBF6955817183" +
	"995497CEA956AE515D2261898FA051015728E5A8AAAC42DAD33170D04507A33A" +
	"85521ABDF1CBA64ECFB850458DBEF0A8AEA71575D060C7DB3970F85A6E1E4C7A" +
	"BF5AE8CDB0933D71E8C94E04A25619DCEE3D2261AD2EE6BF12FFA06D98A0864D" +
	"87602733EC86A64521F2B18177B200CBBE117577A615D6C770988C0BAD946E20" +
	"8E24FA074E5AB3143DB5BFCE0FD108E4B82D120A93AD2CAFFFFFFFFFFFFFFFF"

const srpNBytes = 384 // 3072 bits

var (
	srpN = mustSrpInt(srpNHex)
	srpG = big.NewInt(5)
)

func mustSrpInt(hex string) *big.Int {
	n, ok := new(big.Int).SetString(hex, 16)
	if !ok {
		panic("crypto: bad SRP group constant")
	}
	return n
}

// SRPClient is the client side of HomeKit pair-setup. The username is always
// "Pair-Setup"; the password is the PIN ("3939" for transient HomePod and
// macOS pairing, the on-screen code for Apple TV).
type SRPClient struct {
	password  string
	a         *big.Int
	A         *big.Int
	B         *big.Int
	salt      []byte
	k         *big.Int
	u         *big.Int
	x         *big.Int
	S         *big.Int
	K         []byte
	M1        []byte
	processed bool
}

// NewSRPClient creates a client with the RFC 5054 3072-bit group loaded.
func NewSRPClient() *SRPClient {
	return &SRPClient{}
}

// Start records the password (PIN), generates the 256-bit ephemeral secret a
// and the public A = g^a mod N.
func (c *SRPClient) Start(password string) {
	c.password = password
	c.a = new(big.Int).SetBytes(RandomBytes(32))
	c.A = new(big.Int).Exp(srpG, c.a, srpN)
}

// PublicA returns A, big-endian and trimmed of leading zero bytes, the wire
// form the HAP TLV8 PublicKey carries.
func (c *SRPClient) PublicA() []byte {
	if c.A == nil {
		return nil
	}
	return srpNaturalBytes(c.A)
}

// Process consumes the server's salt and public B, computes the premaster S,
// the session key K = SHA-512(S) and the client proof M1. It returns false
// when B is malformed or B ≡ 0 (mod N), the RFC 5054 safety check against a
// malicious or garbled server value.
func (c *SRPClient) Process(salt, serverB []byte) bool {
	if c.a == nil || len(serverB) > srpNBytes {
		return false
	}
	c.salt = append([]byte(nil), salt...)
	B := new(big.Int).SetBytes(serverB)
	c.B = B

	zero := new(big.Int).Mod(B, srpN)
	if zero.Sign() == 0 {
		return false
	}

	c.k = new(big.Int).SetBytes(srpHashPaddedPair(srpN, srpG))
	c.u = new(big.Int).SetBytes(srpHashPaddedPair(c.A, B))

	// x = H(salt | H("Pair-Setup:" password)).
	inner := sha512.Sum512([]byte("Pair-Setup:" + c.password))
	saltAndHash := append(append([]byte(nil), salt...), inner[:]...)
	outer := sha512.Sum512(saltAndHash)
	c.x = new(big.Int).SetBytes(outer[:])

	// S = (B - k*g^x) ^ (a + u*x) mod N, with the base kept positive mod N.
	gx := new(big.Int).Exp(srpG, c.x, srpN)
	kgx := new(big.Int).Mod(new(big.Int).Mul(c.k, gx), srpN)
	base := new(big.Int).Mod(new(big.Int).Sub(B, kgx), srpN)
	exp := new(big.Int).Add(c.a, new(big.Int).Mul(c.u, c.x))
	c.S = new(big.Int).Exp(base, exp, srpN)

	// K = SHA-512(S) at natural byte length.
	c.K = sum512(srpNaturalBytes(c.S))

	// M1 = H( H(N) xor H(g) | H("Pair-Setup") | salt | A | B | K ).
	hN := sum512(srpNaturalBytes(srpN))
	hG := sum512(srpNaturalBytes(srpG))
	hXor := make([]byte, sha512.Size)
	for i := range hXor {
		hXor[i] = hN[i] ^ hG[i]
	}
	hI := sum512([]byte("Pair-Setup"))
	m1in := make([]byte, 0, len(hXor)+len(hI)+len(c.salt)+384+384+len(c.K))
	m1in = append(m1in, hXor...)
	m1in = append(m1in, hI...)
	m1in = append(m1in, c.salt...)
	m1in = append(m1in, srpNaturalBytes(c.A)...)
	m1in = append(m1in, srpNaturalBytes(B)...)
	m1in = append(m1in, c.K...)
	c.M1 = sum512(m1in)

	c.processed = true
	return true
}

// ProofM1 returns the 64-byte client proof.
func (c *SRPClient) ProofM1() []byte {
	return append([]byte(nil), c.M1...)
}

// SessionKey returns K, the 64-byte HKDF input keying material.
func (c *SRPClient) SessionKey() []byte {
	return append([]byte(nil), c.K...)
}

// VerifyServerProof checks the server's proof M2 = H(A | M1 | K).
func (c *SRPClient) VerifyServerProof(serverM2 []byte) bool {
	if !c.processed {
		return false
	}
	in := make([]byte, 0, len(c.M1)+len(c.K)+384)
	in = append(in, srpNaturalBytes(c.A)...)
	in = append(in, c.M1...)
	in = append(in, c.K...)
	m2 := sum512(in)
	return constantTimeEqual(m2, serverM2)
}

// srpNaturalBytes renders a big.Int big-endian at minimal length, with zero
// as one 0x00 byte, matching the reference's mpiToBytes.
func srpNaturalBytes(v *big.Int) []byte {
	b := v.Bytes()
	if len(b) == 0 {
		return []byte{0}
	}
	return b
}

// srpHashPaddedPair hashes two big integers, each zero-extended on the left
// to the 384-byte group length. This is the HAP k = H(N|g) and u = H(A|B)
// convention (pair_ap H_nn_pad).
func srpHashPaddedPair(a, b *big.Int) []byte {
	buf := make([]byte, 0, 2*srpNBytes)
	for _, v := range []*big.Int{a, b} {
		vb := v.Bytes()
		if len(vb) > srpNBytes {
			// Operand exceeds the group length; caller rejects B this large.
			return nil
		}
		padded := make([]byte, srpNBytes)
		copy(padded[srpNBytes-len(vb):], vb)
		buf = append(buf, padded...)
	}
	return sum512(buf)
}

func sum512(data []byte) []byte {
	s := sha512.Sum512(data)
	return s[:]
}
