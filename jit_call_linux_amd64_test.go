//go:build linux && amd64 && cgo

package main

import "testing"

func TestJITCallAMD64ArgumentReturn(t *testing.T) {
	execMem, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer execMem.Free()

	addr, err := execMem.Write([]byte{
		0x48, 0x89, 0xF8, // MOV RDI,RAX
		0xC3, // RET
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	const want = uintptr(0x123456789ABCDEF0)
	if got := callNativeArgRet(addr, want); got != want {
		t.Fatalf("callNativeArgRet = %#x, want %#x", got, want)
	}
}
