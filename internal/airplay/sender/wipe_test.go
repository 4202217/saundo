package sender

import (
	"bytes"
	"testing"

	"saundo/internal/airplay/crypto"
)

func zeroed(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// TestStopWipesSessionKeys pins the teardown wipe: the AP2 keys, the SRP
// session key, and the pair-setup key must not survive a stopped session.
func TestStopWipesSessionKeys(t *testing.T) {
	secret := func(n int, v byte) []byte { return bytes.Repeat([]byte{v}, n) }
	srpK := secret(64, 9)
	srp := crypto.NewSRPClient()
	srp.K = srpK

	ltSeed := secret(32, 1)
	verifyPriv := secret(32, 2)
	shared := secret(32, 3)
	controlOut := secret(32, 4)
	controlIn := secret(32, 5)
	eventOut := secret(32, 6)
	eventIn := secret(32, 7)
	audioKey := secret(32, 8)
	pairKey := secret(32, 10)

	s := New(&audioRecordIO{}, Events{}, nil)
	s.state = StateStreaming
	s.ap2 = &ap2State{
		ltSeed: ltSeed, verifyPriv: verifyPriv, sharedSecret: shared,
		controlOut: controlOut, controlIn: controlIn,
		eventOut: eventOut, eventIn: eventIn, audioKey: audioKey, srp: srp,
	}
	s.pairSetupSessionKey = pairKey

	s.Stop()

	for name, b := range map[string][]byte{
		"ltSeed":              ltSeed,
		"verifyPriv":          verifyPriv,
		"sharedSecret":        shared,
		"controlOut":          controlOut,
		"controlIn":           controlIn,
		"eventOut":            eventOut,
		"eventIn":             eventIn,
		"audioKey":            audioKey,
		"pairSetupSessionKey": pairKey,
		"srp.K":               srpK,
	} {
		if !zeroed(b) {
			t.Errorf("%s was not zeroed", name)
		}
	}
}
