//go:build !(amd64 || arm64 || 386 || arm || riscv64 || loong64 || mipsle || mips64le || ppc64le || wasm)

package main

// IntuitionEngine uses unsafe.Pointer uint32 stores for framebuffer writes
// and memory bus access, which assume little-endian byte order.
var _ = "IntuitionEngine requires a little-endian architecture" + 1
