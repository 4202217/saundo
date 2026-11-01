// Command saundo streams Windows system audio to AirPlay 2 receivers.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater/native"

	"saundo/internal/app"
	"saundo/internal/diagnostics"
)

func main() {
	mygo.Use(native.Plugin)
	if err := app.Run(); err != nil {
		// The release build has no console, so keep the failure in the log.
		diagnostics.New("").Printf("fatal: %v", err)
		log.Fatal(err)
	}
}
