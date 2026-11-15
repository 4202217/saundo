package sender

import "fmt"

// String keeps key material out of logs and error messages. fmt does not
// call a field's Format method for an unexported field, so the state itself
// has to answer for its secrets.
func (a ap2State) String() string {
	return fmt.Sprintf("ap2State{session:%s nonce:%d encrypt:%t}",
		a.sessionUUID, a.audioNonce, a.encryptAudio)
}
