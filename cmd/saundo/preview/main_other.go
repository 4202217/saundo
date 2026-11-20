//go:build !js

package main

// The preview runs in a browser; the host build is a stub so go build ./...
// stays green without the js/wasm toolchain.
func main() {}
