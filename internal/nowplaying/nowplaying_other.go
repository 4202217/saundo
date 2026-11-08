//go:build !windows && !darwin

// Package nowplaying reads the system "now playing" session. Only Windows
// implements it; other platforms report unsupported.
package nowplaying

import (
	"errors"
	"time"
)

// Track is one media session's track metadata.
type Track struct {
	Title  string
	Artist string
	Album  string
}

// Current returns unsupported on non-Windows platforms.
func Current() (Track, bool, error) {
	return Track{}, false, errors.New("nowplaying: only supported on Windows")
}

// Read returns unsupported on non-Windows platforms.
func Read() (Snapshot, error) {
	return Snapshot{}, errors.New("nowplaying: only supported on Windows")
}

// Control returns unsupported on non-Windows platforms.
func Control(aumid string, action Action, position time.Duration) error {
	return errors.New("nowplaying: only supported on Windows")
}
