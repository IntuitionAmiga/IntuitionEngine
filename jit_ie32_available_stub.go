//go:build !((linux && (amd64 || arm64)) || (js && wasm))

package main

const (
	ie32JITAvailable = false
	ie32JITBackend   = "none"
)
