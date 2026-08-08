//go:build js && wasm

package main

const (
	ie32JITAvailable = true
	ie32JITBackend   = "wasm"
)
