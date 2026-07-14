//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import "testing"

// TestJIT_vs_Interpreter_ConstAddrLoadStore exercises the constant-address
// LOAD/STORE elision (Technique 1): STORE and LOAD with base R0 and a
// non-negative displacement below IO_REGION_START. The elided native path must
// produce byte-identical register and memory results to the interpreter.
func TestJIT_vs_Interpreter_ConstAddrLoadStore(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	base := uint64(PROG_START)
	const dataAddr = 0x2000 // low RAM, below IO_REGION_START (0xA0000)
	build := func(mem []byte) {
		// MOVE.Q R1, #0xDEADBEEF
		// STORE.Q R1, dataAddr(R0)      -> constant-address store
		// LOAD.Q  R2, dataAddr(R0)      -> constant-address load
		// STORE.L R1, (dataAddr+16)(R0)
		// LOAD.L  R3, (dataAddr+16)(R0)
		// HALT
		put := func(off uint64, b []byte) { copy(mem[base+off:], b) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEADBEEF))
		put(0x08, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 0, 0, uint32(dataAddr)))
		put(0x10, ie64Instr(OP_LOAD, 2, IE64_SIZE_Q, 0, 0, 0, uint32(dataAddr)))
		put(0x18, ie64Instr(OP_STORE, 1, IE64_SIZE_L, 0, 0, 0, uint32(dataAddr+16)))
		put(0x20, ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 0, 0, uint32(dataAddr+16)))
		put(0x28, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}

	jitCPU := runToHaltAt(t, true, build)
	interpCPU := runToHaltAt(t, false, build)

	for i := range jitCPU.regs {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Fatalf("R%d mismatch: JIT 0x%X, interp 0x%X", i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
	if jitCPU.regs[2] != 0xDEADBEEF {
		t.Fatalf("R2 (LOAD.Q) = 0x%X, want 0xDEADBEEF", jitCPU.regs[2])
	}
	if jitCPU.regs[3] != 0xDEADBEEF {
		t.Fatalf("R3 (LOAD.L) = 0x%X, want 0xDEADBEEF", jitCPU.regs[3])
	}
	// Memory image must match too.
	for _, off := range []uint64{dataAddr, dataAddr + 16} {
		for k := uint64(0); k < 8; k++ {
			if jitCPU.memory[off+k] != interpCPU.memory[off+k] {
				t.Fatalf("mem[0x%X] mismatch: JIT 0x%X, interp 0x%X",
					off+k, jitCPU.memory[off+k], interpCPU.memory[off+k])
			}
		}
	}
}
