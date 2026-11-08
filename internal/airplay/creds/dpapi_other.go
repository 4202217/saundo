//go:build !windows && !darwin

package creds

import "errors"

func protect([]byte) ([]byte, error) {
	return nil, errors.New("creds: DPAPI is only available on Windows")
}

func unprotect([]byte) ([]byte, error) {
	return nil, errors.New("creds: DPAPI is only available on Windows")
}
