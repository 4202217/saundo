//go:build e2e

// The e2e build links the mygo automation plugin so another process can drive
// the app over MYGO_AUTOMATION. Release builds strip the plugin with the
// inspector, so this import never reaches a shipped binary.
package main

import _ "github.com/egoist/mygo/plugins/automation"
