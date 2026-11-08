//go:build !darwin

package app

import "github.com/egoist/mygo"

// focusAppOnShow brings the app forward after the flyout shows, as Windows
// and Linux expect from a tray flyout.
func focusAppOnShow() { mygo.App.Focus() }
