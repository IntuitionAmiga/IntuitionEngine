//go:build darwin && amd64

package main

import "testing"

func TestExecMemDarwinAMD64_AllocAndCall(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	addr, err := em.Write([]byte{
		0xB8, 0x2A, 0x00, 0x00, 0x00,
		0xC3,
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if got := callNativeRet(addr); got != 42 {
		t.Fatalf("callNativeRet = %d, want 42", got)
	}
}

func TestExecMemDarwinAMD64_PatchRel32At(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	addr, err := em.Write([]byte{
		0xE9, 0x00, 0x00, 0x00, 0x00,
		0xB8, 0x01, 0x00, 0x00, 0x00,
		0xC3,
		0xB8, 0x2A, 0x00, 0x00, 0x00,
		0xC3,
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	PatchRel32At(addr+1, addr+11)

	if got := callNativeRet(addr); got != 42 {
		t.Fatalf("patched callNativeRet = %d, want 42", got)
	}
}
