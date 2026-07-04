// jit_ie64_smc_amd64_test.go - amd64 IE64 SMC emitter regression tests.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestIE64SMC_AMD64SecondStoreForcesFullInvalidation(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	r := newJITTestRig(t)
	cpu := r.cpu

	copy(cpu.memory[PROG_START:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0))
	copy(cpu.memory[PROG_START+IE64_INSTR_SIZE:], ie64Instr(OP_STORE, 4, IE64_SIZE_L, 0, 3, 0, 0))
	copy(cpu.memory[PROG_START+2*IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	instrs := scanBlock(cpu.memory, PROG_START)
	if len(instrs) < 2 {
		t.Fatalf("scanBlock returned %d instructions, want at least 2", len(instrs))
	}
	block, err := compileBlock(instrs[:2], PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlock: %v", err)
	}

	const firstAddr = uint64(0x2100)
	const secondAddr = uint64(0x3100)
	cpu.regs[1] = firstAddr
	cpu.regs[2] = 0xAABBCCDD
	cpu.regs[3] = secondAddr
	cpu.regs[4] = 0x11223344

	bitmap := make([]byte, 0x40)
	bitmap[firstAddr>>8] = 1
	bitmap[secondAddr>>8] = 1
	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&cpu.memory[0]))
	r.ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&bitmap[0]))
	r.ctx.CodePageBitmapLen = uint32(len(bitmap))

	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	if got := binary.LittleEndian.Uint32(cpu.memory[firstAddr:]); got != 0xAABBCCDD {
		t.Fatalf("first store = 0x%08X, want 0xAABBCCDD", got)
	}
	if got := binary.LittleEndian.Uint32(cpu.memory[secondAddr:]); got != 0x11223344 {
		t.Fatalf("second store = 0x%08X, want 0x11223344", got)
	}
	if r.ctx.NeedInval != 1 {
		t.Fatal("NeedInval was not set for SMC stores")
	}
	if r.ctx.InvalAddr != firstAddr {
		t.Fatalf("InvalAddr = 0x%X, want first SMC address 0x%X", r.ctx.InvalAddr, uint64(firstAddr))
	}
	if r.ctx.InvalSize != 0 {
		t.Fatalf("InvalSize = %d, want 0 to force full invalidation after second SMC range", r.ctx.InvalSize)
	}
}
