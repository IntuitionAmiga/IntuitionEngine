//go:build amd64 || arm64 || 386 || arm || riscv64 || loong64 || mipsle || mips64le || ppc64le || wasm

// le_check.go - IntuitionEngine requires a little-endian architecture.
//
// This file compiles on known LE targets. The sibling file be_unsupported.go
// contains a deliberate compile error for any architecture not listed here.

package main
