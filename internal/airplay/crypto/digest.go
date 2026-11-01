// SPDX-License-Identifier: Apache-2.0
//
// Ported from airplay2-sender-cpp (Apache-2.0), src/airplay_crypto.cpp,
// section "RFC 2617 MD5 digest auth".

package crypto

import (
	"crypto/md5"
	"encoding/hex"
)

// DigestAuthResponse builds the Authorization header value for receivers
// that use HTTP digest auth (pw=true). It is the RFC 2069 form without qop:
// response = MD5(HA1 : nonce : HA2).
func DigestAuthResponse(method, uri, username, realm, password, nonce string) string {
	ha1 := md5Hex(username + ":" + realm + ":" + password)
	ha2 := md5Hex(method + ":" + uri)
	response := md5Hex(ha1 + ":" + nonce + ":" + ha2)
	return "Digest username=\"" + username + "\", realm=\"" + realm +
		"\", nonce=\"" + nonce + "\", uri=\"" + uri +
		"\", response=\"" + response + "\""
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
