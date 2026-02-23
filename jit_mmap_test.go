// jit_mmap_test.go - Tests for executable memory allocation

//go:build (amd64 || arm64) && linux

package main

import (
	"runtime"
	"testing"
)

func TestExecMem_AllocAndFree(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	if em.buf == nil {
		t.Fatal("buf should not be nil")
	}
	if em.Used() != 0 {
		t.Fatalf("Used = %d, want 0", em.Used())
	}
}

func TestExecMem_AllocAndCall(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	var code []byte
	switch runtime.GOARCH {
	case "amd64":
		// x86-64: MOV EAX, 42; RET
		code = []byte{
			0xB8, 0x2A, 0x00, 0x00, 0x00, // MOV EAX, 42
			0xC3, // RET
		}
	case "arm64":
		// ARM64: MOV X0, #42; RET
		code = []byte{
			0x40, 0x05, 0x80, 0xD2, // MOV X0, #42
			0xC0, 0x03, 0x5F, 0xD6, // RET
		}
	default:
		t.Skip("unsupported architecture")
	}

	addr, err := em.Write(code)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if addr == 0 {
		t.Fatal("addr should not be 0")
	}

	// Call the function and verify it returns 42.
	// Go function values are pointers to a runtime.funcval struct whose
	// first field (fn) is the code pointer. We need two levels of
	// indirection: the function value must be a pointer to a memory
	// location that contains the code address.
	result := callNativeRet(addr)
	if result != 42 {
		t.Fatalf("result = %d, want 42", result)
	}
	runtime.KeepAlive(em)
}

func TestExecMem_Alignment(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	// Write a 3-byte sequence, then a 5-byte sequence.
	// Second write should be 16-byte aligned.
	addr1, err := em.Write([]byte{0x90, 0x90, 0x90}) // 3 bytes
	if err != nil {
		t.Fatalf("first Write failed: %v", err)
	}

	addr2, err := em.Write([]byte{0x90, 0x90, 0x90, 0x90, 0x90}) // 5 bytes
	if err != nil {
		t.Fatalf("second Write failed: %v", err)
	}

	if addr2%16 != 0 {
		t.Fatalf("addr2 = 0x%X, not 16-byte aligned", addr2)
	}
	if addr2 <= addr1 {
		t.Fatalf("addr2 (0x%X) should be after addr1 (0x%X)", addr2, addr1)
	}
}

func TestExecMem_Reset(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	em.Write([]byte{0x90, 0x90, 0x90})
	if em.Used() == 0 {
		t.Fatal("Used should be > 0 after Write")
	}

	em.Reset()
	if em.Used() != 0 {
		t.Fatalf("Used = %d after Reset, want 0", em.Used())
	}
}

func TestExecMem_Exhausted(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	// Try to write more than the buffer size
	bigCode := make([]byte, 5000)
	_, err = em.Write(bigCode)
	if err == nil {
		t.Fatal("expected error when writing beyond capacity")
	}
}

func TestExecMem_MultipleWrites(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	var retCode []byte
	switch runtime.GOARCH {
	case "amd64":
		retCode = []byte{0xC3} // RET
	case "arm64":
		retCode = []byte{0xC0, 0x03, 0x5F, 0xD6} // RET
	default:
		t.Skip("unsupported architecture")
	}

	// Write multiple blocks
	addrs := make([]uintptr, 10)
	for i := range 10 {
		addr, err := em.Write(retCode)
		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
		addrs[i] = addr
	}

	// All addresses should be unique and aligned
	seen := make(map[uintptr]bool)
	for i, addr := range addrs {
		if seen[addr] {
			t.Fatalf("duplicate address at index %d: 0x%X", i, addr)
		}
		seen[addr] = true
		if addr%16 != 0 {
			t.Fatalf("addr[%d] = 0x%X, not 16-byte aligned", i, addr)
		}
	}
}
