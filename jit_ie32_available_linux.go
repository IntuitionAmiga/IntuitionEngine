//go:build linux && (amd64 || arm64)

package main

const (
	ie32JITAvailable = true
	ie32JITBackend   = "native"
)
