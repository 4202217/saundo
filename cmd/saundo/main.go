// Command saundo streams Windows system audio to AirPlay 2 receivers.
package main

import (
	"log"

	"saundo/internal/app"
	"saundo/internal/diagnostics"
)

func main() {
	if err := app.Run(); err != nil {
		// The release build has no console, so keep the failure in the log.
		diagnostics.New("").Printf("fatal: %v", err)
		log.Fatal(err)
	}
}
