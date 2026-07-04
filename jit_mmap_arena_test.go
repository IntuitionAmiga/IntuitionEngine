// jit_mmap_arena_test.go - ExecMem arena reclamation tests.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build (amd64 || arm64) && linux

package main

import (
	"encoding/binary"
	"testing"
)

func TestArena_ReclaimAfterLastBlockEvicted(t *testing.T) {
	execMem, err := AllocExecMem(2 * execMemArenaMinSize)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	t.Cleanup(execMem.Free)

	code := make([]byte, execMemArenaMinSize-128)
	code[0] = 0xC3 // RET on amd64; harmless data for this test on arm64.
	addr1, err := execMem.Write(code)
	if err != nil {
		t.Fatalf("Write block1: %v", err)
	}
	addr2, err := execMem.Write(code)
	if err != nil {
		t.Fatalf("Write block2: %v", err)
	}
	if _, err := execMem.Write(code); err == nil {
		t.Fatal("third arena allocation succeeded before any arena was reclaimed")
	}

	cache := NewCodeCache()
	block1 := &JITBlock{startPC: 0x1000, endPC: 0x1010, execAddr: addr1, execSize: len(code)}
	block2 := &JITBlock{startPC: 0x2000, endPC: 0x2010, execAddr: addr2, execSize: len(code)}
	cache.Put(block1)
	cache.Put(block2)
	if !cache.RemoveBlock(block1) {
		t.Fatal("RemoveBlock(block1) returned false")
	}

	addr3, err := execMem.Write(code)
	if err != nil {
		t.Fatalf("Write block3 after evicting block1: %v", err)
	}
	if addr3 != addr1 {
		t.Fatalf("reclaimed arena addr = %#x, want reused first arena addr %#x", addr3, addr1)
	}
}

func TestArena_PatchRel32AcrossArenas(t *testing.T) {
	execMem, err := AllocExecMem(2 * execMemArenaMinSize)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	t.Cleanup(execMem.Free)

	sourceCode := make([]byte, execMemArenaMinSize-128)
	sourceCode[0] = 0xE9 // JMP rel32
	targetCode := make([]byte, execMemArenaMinSize-128)
	targetCode[0] = 0xC3 // RET

	sourceAddr, err := execMem.Write(sourceCode)
	if err != nil {
		t.Fatalf("Write source: %v", err)
	}
	targetAddr, err := execMem.Write(targetCode)
	if err != nil {
		t.Fatalf("Write target: %v", err)
	}
	if targetAddr-sourceAddr < uintptr(execMemArenaMinSize/2) {
		t.Fatalf("target addr %#x was not placed in a later arena than source %#x", targetAddr, sourceAddr)
	}

	PatchRel32At(sourceAddr+1, targetAddr)
	gotBytes, ok := lookupExecBytes(sourceAddr+1, 4)
	if !ok {
		t.Fatal("patched source displacement not visible through exec view")
	}
	got := int32(binary.LittleEndian.Uint32(gotBytes))
	want := int32(targetAddr - (sourceAddr + 5))
	if got != want {
		t.Fatalf("patched rel32 = %d, want %d", got, want)
	}
}
