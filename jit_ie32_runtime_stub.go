//go:build !((linux && (amd64 || arm64)) || (js && wasm))

package main

func ie32JITRuntimeAvailable() bool { return false }
