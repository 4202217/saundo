//go:build darwin

package app

const (
	// updatesSupported stays false until a signed, notarized mac feed exists.
	updatesSupported = false
)
