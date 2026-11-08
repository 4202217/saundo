//go:build darwin

package app

// focusAppOnShow is a no-op on macOS: the flyout panel is nonactivating and
// takes key on its own, and activating the app would raise every visible app
// window (an open settings page included).
func focusAppOnShow() {}
