//go:build arm64 && (linux || windows || darwin)

package main

import "testing"

func emitARM64MemoryAccessSize(t *testing.T, opcode byte, rs byte, imm32 uint32) int {
	t.Helper()
	cb := NewCodeBuffer(4096)
	ji := &JITInstr{
		opcode:   opcode,
		rd:       1,
		rs:       rs,
		size:     IE64_SIZE_L,
		imm32:    imm32,
		pcOffset: 0,
	}
	br := &blockRegs{used: 1 << 1, written: 1 << 1}
	switch opcode {
	case OP_LOAD:
		emitLOAD(cb, ji, PROG_START, br, 0)
	case OP_STORE:
		emitSTORE(cb, ji, PROG_START, br, 0)
	default:
		t.Fatalf("unsupported opcode %#x", opcode)
	}
	return cb.Len()
}

func TestARM64_ConstLowRAMAccessElidesChecks(t *testing.T) {
	for _, opcode := range []byte{OP_LOAD, OP_STORE} {
		constantSize := emitARM64MemoryAccessSize(t, opcode, 0, 0x3100)
		dynamicSize := emitARM64MemoryAccessSize(t, opcode, 2, 0x3100)
		if constantSize >= dynamicSize {
			t.Errorf("opcode %#x constant low-RAM emitter size = %d, dynamic = %d; checks were not elided", opcode, constantSize, dynamicSize)
		}
	}
}
