//go:build !windows && !darwin

package capture

import "errors"

var errMuteUnsupported = errors.New("capture: endpoint mute is unsupported on this platform")

// EndpointMuted is unsupported off Windows.
func EndpointMuted(string) (bool, error) { return false, errMuteUnsupported }

// SetEndpointMuted is unsupported off Windows.
func SetEndpointMuted(string, bool) error { return errMuteUnsupported }
