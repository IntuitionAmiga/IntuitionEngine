//go:build darwin && arm64

package main

import "testing"

func TestExecMemDarwinARM64_AllocAndCall(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	addr, err := em.Write([]byte{
		0x40, 0x05, 0x80, 0xD2,
		0xC0, 0x03, 0x5F, 0xD6,
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if got := callNativeRet(addr); got != 42 {
		t.Fatalf("callNativeRet = %d, want 42", got)
	}
}

func TestExecMemDarwinARM64_ResetAndRewrite(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	addr, err := em.Write([]byte{
		0x20, 0x00, 0x80, 0xD2,
		0xC0, 0x03, 0x5F, 0xD6,
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if got := callNativeRet(addr); got != 1 {
		t.Fatalf("callNativeRet = %d, want 1", got)
	}

	em.Reset()

	addr, err = em.Write([]byte{
		0x40, 0x05, 0x80, 0xD2,
		0xC0, 0x03, 0x5F, 0xD6,
	})
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}
	if got := callNativeRet(addr); got != 42 {
		t.Fatalf("rewritten callNativeRet = %d, want 42", got)
	}
}
