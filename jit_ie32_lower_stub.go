//go:build !(linux && (amd64 || arm64)) && !(js && wasm)

package main

func ie32JITTryRunDirect(cpu *CPU) uint64 { return 0 }
