//go:build windows && (amd64 || arm64)

package main

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestExecMemWindows_AllocAndCall(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	var code []byte
	switch runtime.GOARCH {
	case "amd64":
		code = []byte{
			0xB8, 0x2A, 0x00, 0x00, 0x00,
			0xC3,
		}
	case "arm64":
		code = []byte{
			0x40, 0x05, 0x80, 0xD2,
			0xC0, 0x03, 0x5F, 0xD6,
		}
	default:
		t.Fatalf("unsupported GOARCH %s", runtime.GOARCH)
	}

	addr, err := em.Write(code)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if got := callNativeRet(addr); got != 42 {
		t.Fatalf("callNativeRet = %d, want 42", got)
	}
}

func TestExecMemWindows_DualViewsAndPatchRel32(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("rel32 patching test is specific to amd64 code layout")
	}

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

	writableAddr := uintptr(unsafe.Pointer(&em.writable[0]))
	execAddr := uintptr(unsafe.Pointer(&em.exec[0]))
	if writableAddr == execAddr {
		t.Fatal("writable and exec views must be distinct virtual addresses")
	}

	PatchRel32At(addr+1, addr+11)

	if got := callNativeRet(addr); got != 42 {
		t.Fatalf("patched callNativeRet = %d, want 42", got)
	}

	patched := em.writable[1:5]
	if patched[0] == 0 && patched[1] == 0 && patched[2] == 0 && patched[3] == 0 {
		t.Fatal("PatchRel32At did not write through the writable view")
	}
}
