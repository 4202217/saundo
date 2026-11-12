//go:build !windows && !darwin

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

// AppInfo describes one capturable application. It mirrors the Windows
// definition so callers compile everywhere.
type AppInfo struct {
	PID   uint32
	Name  string
	Path  string
	Title string
	// ID is the stable selection key where the platform has one.
	ID string
	// Icon is the app's PNG icon, or nil when the platform has none.
	Icon []byte
	// Active reports whether the app (its process tree) currently renders
	// audio; process-loopback capture only initializes for active apps.
	Active bool
}

// MultiAppSupported reports whether one capture stream can mix several
// selected apps. The stub platforms cannot.
func MultiAppSupported() bool { return false }

// ProcessMuteSupported reports whether a per-app stream can mute just the
// captured app. The stub platforms cannot.
func ProcessMuteSupported() bool { return false }

// SetProcessMuted is unsupported on this platform.
func SetProcessMuted(bool) error {
	return errors.New("capture: process mute is unsupported on this platform")
}

// AppCaptureSupported reports whether per-application capture enumeration is
// available on this platform. It is false off Windows, so callers skip the
// per-app picker instead of surfacing an empty list.
func AppCaptureSupported() bool { return false }

// AudioApplications is unsupported off Windows.
func AudioApplications() ([]AppInfo, error) {
	return nil, errors.New("capture: unsupported on this platform")
}
