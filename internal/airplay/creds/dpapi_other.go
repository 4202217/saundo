//go:build !windows && !darwin

package creds

import (
	"errors"
	"fmt"
)

func protect([]byte) ([]byte, error) {
	return nil, errors.New("creds: DPAPI is only available on Windows")
}

func unprotect([]byte) ([]byte, error) {
	// No at-rest key exists on this platform, so the failure is on the key
	// side: fail closed without quarantining whatever file is present.
	return nil, fmt.Errorf("%w: DPAPI is only available on Windows", errKeyUnavailable)
}
