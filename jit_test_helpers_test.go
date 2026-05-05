//go:build amd64 || arm64

package main

import "testing"

func mustExecBytes(t *testing.T, addr uintptr, size int) []byte {
	t.Helper()
	b, ok := lookupExecBytes(addr, size)
	if !ok {
		t.Fatalf("execution address 0x%X is not backed by ExecMem", addr)
	}
	return b
}

func mustExecRel32(t *testing.T, addr uintptr) int32 {
	t.Helper()
	b := mustExecBytes(t, addr, 4)
	return int32(uint32(b[0]) |
		uint32(b[1])<<8 |
		uint32(b[2])<<16 |
		uint32(b[3])<<24)
}
