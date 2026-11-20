//go:build js && preview

package main

import (
	"log"

	"saundo/internal/app"
)

func main() {
	if err := app.RunPreview(); err != nil {
		log.Fatal(err)
	}
}
