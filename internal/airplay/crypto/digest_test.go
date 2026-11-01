package crypto

import (
	"crypto/md5"
	"fmt"
	"testing"
)

// TestDigestAuthResponse checks the full Authorization header against the
// RFC 2069 response computed independently here with crypto/md5:
// response = MD5(MD5(user:realm:pass) : nonce : MD5(method:uri)).
func TestDigestAuthResponse(t *testing.T) {
	ha1 := fmt.Sprintf("%x", md5.Sum([]byte("Pair-Setup:HomePod:3939")))
	ha2 := fmt.Sprintf("%x", md5.Sum([]byte("POST:rtsp://10.0.0.2/PAIR-SETUP")))
	wantResp := fmt.Sprintf("%x", md5.Sum([]byte(ha1+":nonce-value:"+ha2)))

	want := "Digest username=\"Pair-Setup\", realm=\"HomePod\", nonce=\"nonce-value\", " +
		"uri=\"rtsp://10.0.0.2/PAIR-SETUP\", response=\"" + wantResp + "\""

	got := DigestAuthResponse("POST", "rtsp://10.0.0.2/PAIR-SETUP",
		"Pair-Setup", "HomePod", "3939", "nonce-value")
	if got != want {
		t.Fatalf("DigestAuthResponse:\n got %s\nwant %s", got, want)
	}
}
