//go:build !(linux && (amd64 || arm64))

package main

// ie32JITExecutableArenaSize is zero where IE32 has no native code arena.
func ie32JITExecutableArenaSize(*CPU) int { return 0 }
