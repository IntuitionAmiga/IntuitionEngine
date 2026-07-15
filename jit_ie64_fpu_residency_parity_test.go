//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// buildFP32ResidencyLoopProgram builds a hot single-block FP32 loop that keeps
// several FP32 registers live across iterations: an accumulator (F3 += F2, an
// rd==rs alias), a pure writer (F4 = F2*F2), and an in-place unary (F5 = |F5|).
// Values are exact powers of two so JIT and interpreter cannot diverge on
// rounding, isolating residency (prologue load, in-XMM loop, exit spill) as the
// only variable. Loops R10 times then HALTs.
func buildFP32ResidencyLoopProgram(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, b []byte) { copy(mem[base+off:], b) }
	put(0x00, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0x40000000)) // R2 = 2.0 bits
	put(0x08, ie64Instr(OP_FMOVI, 2, 0, 0, 2, 0, 0))                   // F2 = 2.0
	put(0x10, ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 0x00000000)) // R3 = +0.0 bits
	put(0x18, ie64Instr(OP_FMOVI, 3, 0, 0, 3, 0, 0))                   // F3 = 0.0 (accumulator)
	put(0x20, ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 1, 0, 0, 0xC0000000)) // R5 = -2.0 bits
	put(0x28, ie64Instr(OP_FMOVI, 5, 0, 0, 5, 0, 0))                   // F5 = -2.0
	put(0x30, ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 5))         // R10 = 5 (trip count)
	// loop head @ 0x38
	put(0x38, ie64Instr(OP_FADD, 3, 0, 0, 3, 2, 0))            // F3 += F2 (alias rd==rs)
	put(0x40, ie64Instr(OP_FMUL, 4, 0, 0, 2, 2, 0))            // F4 = F2*F2
	put(0x48, ie64Instr(OP_FABS, 5, 0, 0, 5, 0, 0))            // F5 = |F5|
	put(0x50, ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1)) // R10--
	negBack := int32(-0x20)
	put(0x58, ie64Instr(OP_BNE, 0, 0, 0, 10, 0, uint32(negBack))) // BNE -> 0x38
	put(0x60, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

// buildFP32ResidencyStraightProgram is a barrier-free straight-line FP32 block
// (no loop) exercising the prologue-load and single-exit spill without a
// backward branch.
func buildFP32ResidencyStraightProgram(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, b []byte) { copy(mem[base+off:], b) }
	put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x40400000)) // R1 = 3.0 bits
	put(0x08, ie64Instr(OP_FMOVI, 1, 0, 0, 1, 0, 0))                   // F1 = 3.0
	put(0x10, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0x40800000)) // R2 = 4.0 bits
	put(0x18, ie64Instr(OP_FMOVI, 2, 0, 0, 2, 0, 0))                   // F2 = 4.0
	put(0x20, ie64Instr(OP_FADD, 3, 0, 0, 1, 2, 0))                    // F3 = 7.0
	put(0x28, ie64Instr(OP_FMUL, 4, 0, 0, 3, 2, 0))                    // F4 = 28.0
	put(0x30, ie64Instr(OP_FNEG, 1, 0, 0, 1, 0, 0))                    // F1 = -3.0
	put(0x38, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

// buildFP64ResidencyLoopProgram is the FP64 analogue: a hot loop keeping D0..D6
// pairs resident across iterations. D4 accumulates (+= D2), D6 = D2*D2, D0 =
// |D0|. Exact integers-as-doubles avoid rounding divergence and keep every value
// finite so the non-finite bail never fires (pure residency path).
func buildFP64ResidencyLoopProgram(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, b []byte) { copy(mem[base+off:], b) }
	put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 2))  // R1 = 2
	put(0x08, ie64Instr(OP_DCVTIF, 2, 0, 0, 1, 0, 0))          // D2 = 2.0
	put(0x10, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0))  // R1 = 0
	put(0x18, ie64Instr(OP_DCVTIF, 4, 0, 0, 1, 0, 0))          // D4 = 0.0 (accumulator)
	put(0x20, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 3))  // R1 = 3
	put(0x28, ie64Instr(OP_DCVTIF, 0, 0, 0, 1, 0, 0))          // D0 = 3.0
	put(0x30, ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 5)) // R10 = 5
	// loop head @ 0x38
	put(0x38, ie64Instr(OP_DADD, 4, IE64_SIZE_L, 0, 4, 2, 0))  // D4 += D2 (alias)
	put(0x40, ie64Instr(OP_DMUL, 6, IE64_SIZE_L, 0, 2, 2, 0))  // D6 = D2*D2
	put(0x48, ie64Instr(OP_DABS, 0, 0, 0, 0, 0, 0))            // D0 = |D0|
	put(0x50, ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1)) // R10--
	negBack := int32(-0x20)
	put(0x58, ie64Instr(OP_BNE, 0, 0, 0, 10, 0, uint32(negBack))) // BNE -> 0x38
	put(0x60, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

// buildFP64ResidencyBailProgram forces a non-finite resident operand so DADD
// bails to the interpreter mid-block, with NO residency barrier so the block
// stays eligible. A finite/zero DDIV manufactures +Inf into a resident pair
// (finite inputs -> no bail, DZ flag set), then a DADD reading that resident
// +Inf hits the non-finite check and bails. The resident pairs must be spilled
// to canonical memory before the bail so the interpreter reads correct inputs
// and matches the pure interpreter run.
func buildFP64ResidencyBailProgram(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, b []byte) { copy(mem[base+off:], b) }
	put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 3)) // R1 = 3
	put(0x08, ie64Instr(OP_DCVTIF, 2, 0, 0, 1, 0, 0))         // D2 = 3.0
	put(0x10, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0)) // R1 = 0
	put(0x18, ie64Instr(OP_DCVTIF, 0, 0, 0, 1, 0, 0))         // D0 = 0.0
	put(0x20, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 5)) // R1 = 5
	put(0x28, ie64Instr(OP_DCVTIF, 4, 0, 0, 1, 0, 0))         // D4 = 5.0
	put(0x30, ie64Instr(OP_DDIV, 6, IE64_SIZE_L, 0, 2, 0, 0)) // D6 = 3.0/0.0 = +Inf (no bail, DZ)
	put(0x38, ie64Instr(OP_DADD, 4, IE64_SIZE_L, 0, 4, 6, 0)) // D4 += D6 (non-finite) -> bail
	put(0x40, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

// buildFP32ResidencyRegionLoop is a hot three-block FP loop (A->B->C joined by
// in-region BRAs, C's BNE back-edge to A) mixing FP32 singles (F2/F3/F5) and an
// FP64 pair (D4/D6). It is barrier-free so the region qualifies for FP
// residency; the trip count exceeds the promotion threshold so the region tier
// actually runs and FP residents live in XMM8..15 across the internal edges.
func buildFP32ResidencyRegionLoop(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, b []byte) { copy(mem[base+off:], b) }
	// Seeds (tier-1, before the loop head).
	put(0x000, ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 20000)) // R10 = trip count
	put(0x008, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 2))      // R1 = 2
	put(0x010, ie64Instr(OP_FCVTIF, 2, 0, 0, 1, 0, 0))              // F2 = 2.0
	put(0x018, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0))      // R1 = 0
	put(0x020, ie64Instr(OP_FCVTIF, 3, 0, 0, 1, 0, 0))              // F3 = 0.0 (FP32 accum)
	put(0x028, ie64Instr(OP_DCVTIF, 4, 0, 0, 1, 0, 0))              // D4 = 0.0 (FP64 accum)
	put(0x030, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 3))      // R1 = 3
	put(0x038, ie64Instr(OP_DCVTIF, 6, 0, 0, 1, 0, 0))              // D6 = 3.0
	// Block A (loop head) @ 0x040: F3 += F2 ; BRA B
	put(0x040, ie64Instr(OP_FADD, 3, 0, 0, 3, 2, 0))            // F3 += 2.0
	put(0x048, ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(0x0B8))) // -> 0x100
	// Block B @ 0x100: D4 += D6 ; F5 = F2*F2 ; BRA C
	put(0x100, ie64Instr(OP_DADD, 4, IE64_SIZE_L, 0, 4, 6, 0))  // D4 += 3.0
	put(0x108, ie64Instr(OP_FMUL, 5, 0, 0, 2, 2, 0))            // F5 = 4.0
	put(0x110, ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(0x0F0))) // -> 0x200
	// Block C @ 0x200: R10-- ; BNE -> A ; fall through HALT
	put(0x200, ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1)) // R10--
	negBack := int32(-0x1C8)                                    // 0x208 -> 0x040
	put(0x208, ie64Instr(OP_BNE, 0, 0, 0, 10, 0, uint32(negBack)))
	put(0x210, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

// assertFPParity now lives in jit_ie64_fp_parity_common_test.go so that every
// backend, not just amd64, can use it.

// TestJIT_vs_Interpreter_FP32Residency is the parity gate for FP32 register
// residency (Technique 3, B1). With residency enabled the JIT keeps F2..F5 in
// XMM8..15 across the loop; the interpreter uses the memory register file. Both
// must reach identical FP state, proving the prologue load, in-XMM arithmetic
// and exit spill are correct.
func TestJIT_vs_Interpreter_FP32Residency(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	t.Setenv("IE64_JIT_FP_RESIDENCY", "1")
	if !ie64FPResidencyEnabled() {
		t.Skip("FP residency unavailable on this host ABI")
	}
	assertFPParity(t, "loop", buildFP32ResidencyLoopProgram)
	assertFPParity(t, "straight", buildFP32ResidencyStraightProgram)
}

// TestJIT_vs_Interpreter_FP64Residency is the parity gate for FP64 pair
// residency (Technique 3, B2): a pure finite loop keeps D-pairs in XMM8..15
// across iterations, and a non-finite bail case proves residents are spilled to
// canonical memory before the mid-block bail to the interpreter.
func TestJIT_vs_Interpreter_FP64Residency(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	t.Setenv("IE64_JIT_FP_RESIDENCY", "1")
	if !ie64FPResidencyEnabled() {
		t.Skip("FP residency unavailable on this host ABI")
	}
	assertFPParity(t, "fp64-loop", buildFP64ResidencyLoopProgram)
	assertFPParity(t, "fp64-bail", buildFP64ResidencyBailProgram)
}

// TestJIT_vs_Interpreter_FPResidencyRegion is the parity gate for FP residency
// on the promoted region tier (Technique 3, B3). The hot three-block FP loop
// promotes to a region; with residency enabled the region keeps F2/F3/F5 and the
// D4/D6 pair in XMM8..15 across the internal block edges, spilling only at the
// external back-edge chain exit and the fall-through epilogue. It must reach the
// same FP state as the interpreter, and the region must actually have promoted.
func TestJIT_vs_Interpreter_FPResidencyRegion(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	t.Setenv("IE64_JIT_REGIONS", "1")
	t.Setenv("IE64_JIT_FP_RESIDENCY", "1")
	if !ie64FPResidencyEnabled() {
		t.Skip("FP residency unavailable on this host ABI")
	}
	before := ie64JITStatsLoad().regions
	jitCPU := runToHaltAt(t, true, buildFP32ResidencyRegionLoop)
	delta := ie64JITStatsLoad().regions - before
	interpCPU := runToHaltAt(t, false, buildFP32ResidencyRegionLoop)

	if delta == 0 {
		t.Fatalf("no region promotion occurred; region-tier FP residency codegen was not exercised")
	}
	if jitCPU.PC != interpCPU.PC {
		t.Fatalf("PC mismatch: JIT 0x%X, interp 0x%X", jitCPU.PC, interpCPU.PC)
	}
	for i := range jitCPU.regs {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Fatalf("R%d mismatch: JIT 0x%X, interp 0x%X", i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
	if jitCPU.FPU.FPSR != interpCPU.FPU.FPSR {
		t.Fatalf("FPSR mismatch: JIT 0x%08X, interp 0x%08X", jitCPU.FPU.FPSR, interpCPU.FPU.FPSR)
	}
	for i := range jitCPU.FPU.FPRegs {
		if jitCPU.FPU.FPRegs[i] != interpCPU.FPU.FPRegs[i] {
			t.Fatalf("F%d mismatch: JIT 0x%08X, interp 0x%08X", i, jitCPU.FPU.FPRegs[i], interpCPU.FPU.FPRegs[i])
		}
	}
	// Sanity: the loop must have accumulated non-trivial FP state. F3 is the
	// FP32 accumulator; slot 5 is D4's high word (D4's low word is legitimately
	// zero for an exact integer double).
	if jitCPU.FPU.FPRegs[3] == 0 || jitCPU.FPU.FPRegs[5] == 0 {
		t.Fatalf("region loop produced trivial FP state F3=0x%X D4hi=0x%X", jitCPU.FPU.FPRegs[3], jitCPU.FPU.FPRegs[5])
	}
}

// TestFP32Residency_MatchesMemoryPath cross-checks that enabling residency does
// not change the result versus the default memory-backed FP path: JIT-with and
// JIT-without residency must agree (and both agree with the interpreter via the
// test above).
func TestFP32Residency_MatchesMemoryPath(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	if !ie64FPResidencySysV() {
		t.Skip("FP residency unavailable on this host ABI")
	}
	t.Setenv("IE64_JIT_FP_RESIDENCY", "0")
	off := runToHaltAt(t, true, buildFP32ResidencyLoopProgram)
	t.Setenv("IE64_JIT_FP_RESIDENCY", "1")
	on := runToHaltAt(t, true, buildFP32ResidencyLoopProgram)
	for i := range off.FPU.FPRegs {
		if off.FPU.FPRegs[i] != on.FPU.FPRegs[i] {
			t.Fatalf("F%d differs with residency: off 0x%08X, on 0x%08X", i, off.FPU.FPRegs[i], on.FPU.FPRegs[i])
		}
	}
	if off.FPU.FPSR != on.FPU.FPSR {
		t.Fatalf("FPSR differs with residency: off 0x%08X, on 0x%08X", off.FPU.FPSR, on.FPU.FPSR)
	}
}
