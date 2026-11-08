//go:build darwin

package app

const (
	// flyoutStyleSetting offers the macOS-only choice between the rounded
	// flyout and the system popover.
	flyoutStyleSetting = true
	// updatesSupported stays false until a signed, notarized mac feed exists.
	updatesSupported = false
)
