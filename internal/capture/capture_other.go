//go:build !windows

package capture

import "errors"

// Endpoints is unsupported off Windows.
func Endpoints() ([]Endpoint, error) {
	return nil, errors.New("capture: unsupported on this platform")
}

// DefaultEndpoint is unsupported off Windows.
func DefaultEndpoint() (Endpoint, error) {
	return Endpoint{}, errors.New("capture: unsupported on this platform")
}

// Start is unsupported off Windows.
func Start(cfg Config) (*Source, error) {
	return nil, errors.New("capture: unsupported on this platform")
}
