//go:build !(linux && (amd64 || arm64))

package main

func ie32JITEnterGenerated(cpu *CPU) {}
