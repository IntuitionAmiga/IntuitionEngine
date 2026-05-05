// jit_x86_segment_test.go - Slice-3 segment-override prefix correctness.
// Verifies that JIT-compiled blocks bail prefixed memory ops to the
// interpreter so segment base addition matches.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"testing"
	"time"
)

// TestX86JIT_SegmentOverride_DSExplicit verifies that an explicit DS
// segment override (which is the default segment for most operands)
// produces identical results between interp and force-native JIT.
//
// With prefixSeg = DS and DS = 0 (the IE default at boot), the segment
// base is 0 and the effective address equals the flat offset — the
// JIT's bail-to-interp path must produce the same result as a plain
// MOV without the prefix.
func TestX86JIT_SegmentOverride_DSExplicit(t *testing.T) {
	// 0x10000: MOV ECX, 0x12345678            (B9 78 56 34 12)
	// 0x10005: DS: MOV [ESI+0x10], ECX        (3E 89 4E 10)
	// 0x10009: DS: MOV EAX, [ESI+0x10]        (3E 8B 46 10)
	// 0x1000D: HLT                            (F4)
	code := []byte{
		0xB9, 0x78, 0x56, 0x34, 0x12, // MOV ECX, 0x12345678
		0x3E, 0x89, 0x4E, 0x10, // DS: MOV [ESI+0x10], ECX
		0x3E, 0x8B, 0x46, 0x10, // DS: MOV EAX, [ESI+0x10]
		0xF4, // HLT
	}
	interp, jit := x86CallRetHarness(t, code)
	if interp.EAX != 0x12345678 {
		t.Errorf("interp EAX = 0x%X, want 0x12345678", interp.EAX)
	}
	if interp.EAX != jit.EAX {
		t.Errorf("EAX mismatch: interp=%08X jit=%08X", interp.EAX, jit.EAX)
	}
	if interp.EIP != jit.EIP {
		t.Errorf("EIP mismatch: interp=%08X jit=%08X", interp.EIP, jit.EIP)
	}
}

// TestX86JIT_SegmentOverride_FSAccess verifies FS-prefixed access with
// FS=0 (default). Both interp and JIT should agree (interp resolves
// via prefixSeg → readSegBase(FS), which is 0; JIT bails to interp).
func TestX86JIT_SegmentOverride_FSAccess(t *testing.T) {
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	code := []byte{
		0xB9, 0xCD, 0xAB, 0x00, 0x00, // MOV ECX, 0xABCD
		0x64, 0x89, 0x4E, 0x20, // FS: MOV [ESI+0x20], ECX
		0x64, 0x8B, 0x46, 0x20, // FS: MOV EAX, [ESI+0x20]
		0xF4, // HLT
	}
	build := func(forceNative bool) *CPU_X86 {
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		cpu.memory = adapter.GetMemory()
		cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
		cpu.EIP = 0x10000
		cpu.ESI = 0x20000
		cpu.EDI = 0x20000
		cpu.ESP = 0x20C00
		for i, b := range code {
			cpu.memory[0x10000+uint32(i)] = b
		}
		if forceNative {
			cpu.x86JitEnabled = true
		}
		cpu.running.Store(true)
		cpu.Halted = false
		done := make(chan struct{})
		go func() {
			if forceNative {
				cpu.X86ExecuteJIT()
			} else {
				for cpu.Running() && !cpu.Halted {
					cpu.Step()
				}
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cpu.running.Store(false)
			waitDoneWithGuard(t, done)
			t.Fatal("timed out")
		}
		return cpu
	}
	interp := build(false)
	jit := build(true)
	if interp.EAX != 0xABCD {
		t.Errorf("interp EAX = 0x%X, want 0xABCD", interp.EAX)
	}
	if interp.EAX != jit.EAX {
		t.Errorf("EAX: interp=%08X jit=%08X", interp.EAX, jit.EAX)
	}
}

// TestX86JIT_SegmentOverride_GSAccess covers GS-prefixed access. Same
// expectations as FS — segment base zero by default.
func TestX86JIT_SegmentOverride_GSAccess(t *testing.T) {
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	code := []byte{
		0xBB, 0xEF, 0xBE, 0xAD, 0xDE, // MOV EBX, 0xDEADBEEF
		0x65, 0x89, 0x5E, 0x30, // GS: MOV [ESI+0x30], EBX
		0xF4, // HLT
	}
	build := func(forceNative bool) *CPU_X86 {
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		cpu.memory = adapter.GetMemory()
		cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
		cpu.EIP = 0x10000
		cpu.ESI = 0x20000
		cpu.EDI = 0x20000
		cpu.ESP = 0x20C00
		for i, b := range code {
			cpu.memory[0x10000+uint32(i)] = b
		}
		if forceNative {
			cpu.x86JitEnabled = true
		}
		cpu.running.Store(true)
		cpu.Halted = false
		done := make(chan struct{})
		go func() {
			if forceNative {
				cpu.X86ExecuteJIT()
			} else {
				for cpu.Running() && !cpu.Halted {
					cpu.Step()
				}
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cpu.running.Store(false)
			waitDoneWithGuard(t, done)
			t.Fatal("timed out")
		}
		return cpu
	}
	interp := build(false)
	jit := build(true)
	addr := uint32(0x20000 + 0x30)
	got := uint32(interp.memory[addr]) |
		uint32(interp.memory[addr+1])<<8 |
		uint32(interp.memory[addr+2])<<16 |
		uint32(interp.memory[addr+3])<<24
	if got != 0xDEADBEEF {
		t.Errorf("interp memory = 0x%X, want 0xDEADBEEF", got)
	}
	if !memEqual(interp.memory[addr:addr+4], jit.memory[addr:addr+4]) {
		t.Errorf("memory diff: interp=%X jit=%X",
			interp.memory[addr:addr+4], jit.memory[addr:addr+4])
	}
}

func memEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Stub to keep fmt imported and silence vet.
var _ = fmt.Sprintf
