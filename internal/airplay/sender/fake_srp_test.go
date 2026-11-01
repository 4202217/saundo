package sender

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"math/big"
)

const srpN3072Hex = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74" +
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

func sha512Sum(parts ...[]byte) []byte {
	h := sha512.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

func padTo(v *big.Int, n int) []byte {
	out := make([]byte, n)
	b := v.Bytes()
	copy(out[n-len(b):], b)
	return out
}

type srpServer struct {
	N, g     *big.Int
	salt     []byte
	b, B     *big.Int
	A, K, M1 []byte
	serverM2 []byte
	verified bool
	pin      string
}

func newSRPServer(pin string) *srpServer {
	n, _ := new(big.Int).SetString(srpN3072Hex, 16)
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	bBytes := make([]byte, 32)
	_, _ = rand.Read(bBytes)
	b := new(big.Int).SetBytes(bBytes)
	g := big.NewInt(5)

	x := new(big.Int).SetBytes(sha512Sum(salt, sha512Sum([]byte("Pair-Setup:"+pin))))
	v := new(big.Int).Exp(g, x, n)
	kBytes := sha512Sum(padTo(n, 384), padTo(g, 384))
	k := new(big.Int).SetBytes(kBytes)
	B := new(big.Int).Mul(k, v)
	B.Add(B, new(big.Int).Exp(g, b, n))
	B.Mod(B, n)

	return &srpServer{N: n, g: g, salt: salt, b: b, B: B, pin: pin}
}

func (s *srpServer) process(aBytes, m1 []byte) bool {
	s.A = append([]byte(nil), aBytes...)
	A := new(big.Int).SetBytes(aBytes)
	uBytes := sha512Sum(padTo(A, 384), padTo(s.B, 384))
	u := new(big.Int).SetBytes(uBytes)
	x := new(big.Int).SetBytes(sha512Sum(s.salt, sha512Sum([]byte("Pair-Setup:"+s.pin))))
	v := new(big.Int).Exp(s.g, x, s.N)
	vu := new(big.Int).Exp(v, u, s.N)
	base := new(big.Int).Mul(A, vu)
	base.Mod(base, s.N)
	S := new(big.Int).Exp(base, s.b, s.N)
	s.K = sha512Sum(S.Bytes())

	hN := sha512Sum(s.N.Bytes())
	hg := sha512Sum(s.g.Bytes())
	hxor := make([]byte, 64)
	for i := range hxor {
		hxor[i] = hN[i] ^ hg[i]
	}
	hI := sha512Sum([]byte("Pair-Setup"))
	s.M1 = sha512Sum(hxor, hI, s.salt, s.A, s.B.Bytes(), s.K)
	s.serverM2 = sha512Sum(s.A, s.M1, s.K)
	if len(m1) != len(s.M1) {
		return false
	}
	var diff byte
	for i := range m1 {
		diff |= m1[i] ^ s.M1[i]
	}
	s.verified = diff == 0
	return s.verified
}

var _ = hex.EncodeToString
