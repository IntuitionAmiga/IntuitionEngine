//go:build linux && arm64 && cgo

package main

import "testing"

func TestJITCallARM64ArgumentReturn(t *testing.T) {
	execMem, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer execMem.Free()

	addr, err := execMem.Write([]byte{
		0xC0, 0x03, 0x5F, 0xD6, // RET
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	const want = uintptr(0x123456789ABCDEF0)
	if got := callNativeArgRet(addr, want); got != want {
		t.Fatalf("callNativeArgRet = %#x, want %#x", got, want)
	}
}
