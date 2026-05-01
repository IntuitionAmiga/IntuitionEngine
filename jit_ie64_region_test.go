// jit_ie64_region_test.go - tests for the IE64 region scanner +
// compile path (Phase 4 sub-phase B.2.b: pure memory-driven walker
// + multi-block region compilation with in-region BRA/JMP direct
// JMP rel32 internalisation).
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
)

// putIE64Instr writes a packed IE64 instruction at offset.
//
// Layout: byte0=opcode, byte1=(rd<<3 | size<<1 | xbit),
// byte2=(rs<<3), byte3=(rt<<3), bytes4-7=imm32 (little-endian).
func putIE64Instr(mem []byte, off uint32, opcode byte, rd, rs, rt byte, imm32 uint32) {
	mem[off] = opcode
	mem[off+1] = rd << 3
	mem[off+2] = rs << 3
	mem[off+3] = rt << 3
	binary.LittleEndian.PutUint32(mem[off+4:], imm32)
}

// putIE64BRA writes BRA imm32 (PC-relative). target = instrPC + imm32.
func putIE64BRA(mem []byte, off uint32, target uint32) {
	disp := uint32(int32(target) - int32(off))
	putIE64Instr(mem, off, OP_BRA, 0, 0, 0, disp)
}

// putIE64JMP writes JMP rs=0, imm32 (absolute target).
func putIE64JMP(mem []byte, off uint32, target uint32) {
	putIE64Instr(mem, off, OP_JMP, 0, 0, 0, target)
}

// putIE64NOP writes a harmless register-only ADD R0,R0,R0 — emits
// native code without memory access, no terminator.
func putIE64NOP(mem []byte, off uint32) {
	putIE64Instr(mem, off, OP_ADD, 0, 0, 0, 0)
}

// putIE64RTS writes a plain RTS terminator (no static target).
func putIE64RTS(mem []byte, off uint32) {
	putIE64Instr(mem, off, OP_RTS64, 0, 0, 0, 0)
}

// TestScanRegionIE64_FollowsBRAChain builds three blocks each ending
// in a BRA to the next, asserts the scanner stitches all three.
func TestScanRegionIE64_FollowsBRAChain(t *testing.T) {
	mem := make([]byte, 0x1000)

	// Block A @ 0x100: NOP; BRA to 0x200.
	putIE64NOP(mem, 0x100)
	putIE64BRA(mem, 0x108, 0x200)
	// Block B @ 0x200: NOP; BRA to 0x300.
	putIE64NOP(mem, 0x200)
	putIE64BRA(mem, 0x208, 0x300)
	// Block C @ 0x300: NOP; RTS.
	putIE64NOP(mem, 0x300)
	putIE64RTS(mem, 0x308)

	res := ScanRegionIE64(mem, 0x100)
	if got := len(res.BlockPCs); got != 3 {
		t.Fatalf("BlockPCs len = %d want 3 (%v)", got, res.BlockPCs)
	}
	want := []uint32{0x100, 0x200, 0x300}
	for i, pc := range want {
		if res.BlockPCs[i] != pc {
			t.Errorf("BlockPCs[%d] = %X want %X", i, res.BlockPCs[i], pc)
		}
	}
}

// TestScanRegionIE64_StopsOnRTS terminates at an RTS-only block; does
// not silently extend past indirect transfers.
func TestScanRegionIE64_StopsOnRTS(t *testing.T) {
	mem := make([]byte, 0x1000)
	putIE64BRA(mem, 0x100, 0x200)
	putIE64RTS(mem, 0x200)

	res := ScanRegionIE64(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 {
		t.Fatalf("BlockPCs len = %d want 2 (%v)", got, res.BlockPCs)
	}
}

// TestScanRegionIE64_FollowsJMPAbs builds a chain via JMP rs=0
// (statically-resolvable absolute target).
func TestScanRegionIE64_FollowsJMPAbs(t *testing.T) {
	mem := make([]byte, 0x1000)
	putIE64NOP(mem, 0x100)
	putIE64JMP(mem, 0x108, 0x200)
	putIE64RTS(mem, 0x200)

	res := ScanRegionIE64(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 || res.BlockPCs[0] != 0x100 || res.BlockPCs[1] != 0x200 {
		t.Errorf("expected [100, 200], got %v", res.BlockPCs)
	}
}

// TestScanRegionIE64_StopsOnJMPIndirect refuses JMP rs!=0
// (register-indirect, not statically resolvable).
func TestScanRegionIE64_StopsOnJMPIndirect(t *testing.T) {
	mem := make([]byte, 0x1000)
	// JMP rs=1, imm32=0 — target is R1 + 0, not statically known.
	putIE64Instr(mem, 0x100, OP_JMP, 0, 1, 0, 0)
	putIE64RTS(mem, 0x108)

	res := ScanRegionIE64(mem, 0x100)
	if len(res.BlockPCs) != 0 {
		t.Errorf("expected nil region for JMP rs!=0, got %v", res.BlockPCs)
	}
}

// TestScanRegionIE64_DetectsBackEdge stops cleanly on a back-edge.
func TestScanRegionIE64_DetectsBackEdge(t *testing.T) {
	mem := make([]byte, 0x1000)
	putIE64NOP(mem, 0x100)
	putIE64BRA(mem, 0x108, 0x200)
	putIE64NOP(mem, 0x200)
	putIE64BRA(mem, 0x208, 0x100) // back to 0x100

	res := ScanRegionIE64(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 {
		t.Errorf("expected 2 blocks (back-edge stop), got %d (%v)", got, res.BlockPCs)
	}
}

// TestIE64FormRegion_RejectsSingleBlock matches form-region's
// single-block rejection contract.
func TestIE64FormRegion_RejectsSingleBlock(t *testing.T) {
	mem := make([]byte, 0x1000)
	putIE64RTS(mem, 0x100)
	if region := ie64FormRegion(0x100, mem); region != nil {
		t.Errorf("expected nil region for single-block scan, got %+v", region)
	}
}

// TestIE64FormRegion_BuildsTwoBlockChain admits a clean BRA-chain.
func TestIE64FormRegion_BuildsTwoBlockChain(t *testing.T) {
	mem := make([]byte, 0x1000)
	putIE64NOP(mem, 0x100)
	putIE64BRA(mem, 0x108, 0x200)
	putIE64NOP(mem, 0x200)
	putIE64RTS(mem, 0x208)

	region := ie64FormRegion(0x100, mem)
	if region == nil {
		t.Fatal("expected non-nil region for valid 2-block chain")
	}
	if len(region.blocks) != 2 || region.blockPCs[0] != 0x100 || region.blockPCs[1] != 0x200 {
		t.Errorf("region shape mismatch: blockPCs=%v len(blocks)=%d", region.blockPCs, len(region.blocks))
	}
}

// TestIE64CompileRegion_BuildsSingleNativeBlock exercises
// ie64CompileRegion end-to-end: scan + form + compile, then verify
// the returned JITBlock spans the region's PC range and was admitted
// into ExecMem.
func TestIE64CompileRegion_BuildsSingleNativeBlock(t *testing.T) {
	mem := make([]byte, 0x1000)
	putIE64NOP(mem, 0x100)
	putIE64BRA(mem, 0x108, 0x200)
	putIE64NOP(mem, 0x200)
	putIE64RTS(mem, 0x208)

	region := ie64FormRegion(0x100, mem)
	if region == nil {
		t.Fatal("expected non-nil region")
	}

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	block, err := ie64CompileRegion(region, execMem, mem)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	if block.startPC != 0x100 {
		t.Errorf("startPC=%X want 0x100", block.startPC)
	}
	if block.endPC < 0x210 {
		t.Errorf("endPC=%X must cover both blocks (>= 0x210)", block.endPC)
	}
	if block.execAddr == 0 {
		t.Error("execAddr is 0; native code not written")
	}
	if block.coveredRanges == nil || len(block.coveredRanges) != 2 {
		t.Errorf("coveredRanges = %v want 2 entries", block.coveredRanges)
	}
}

// TestIE64CompileRegion_BackEdgePreservesBudget guards against the
// raw-JMP-rel32 back-edge bug where a hot in-region loop could spin
// forever in native code without returning to the dispatcher.
// Compiles a 2-block region whose second block back-edges to the
// first; asserts the compile succeeds and the resulting code includes
// the loop-counter / jitBudget machinery that yields after jitBudget
// iterations.
//
// Functional verification of the actual yield is left to the JIT
// integration tests; this guards against regressing the emit-side
// budget-check sequence.
func TestIE64CompileRegion_BackEdgePreservesBudget(t *testing.T) {
	mem := make([]byte, 0x1000)
	// Block A @ 0x100: NOP; BRA to 0x200.
	putIE64NOP(mem, 0x100)
	putIE64BRA(mem, 0x108, 0x200)
	// Block B @ 0x200: NOP; BRA back to 0x100. Back-edge.
	putIE64NOP(mem, 0x200)
	putIE64BRA(mem, 0x208, 0x100)

	region := ie64FormRegion(0x100, mem)
	if region == nil || len(region.blocks) != 2 {
		t.Fatalf("expected 2-block region with back-edge, got %v", region)
	}

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	block, err := ie64CompileRegion(region, execMem, mem)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	if block.execAddr == 0 {
		t.Error("execAddr is 0; native code not written")
	}
	// Sanity: execSize must exceed a trivial 2-NOP-with-direct-JMP
	// region (~30 bytes). Back-edge budget machinery adds a CMP/JCC
	// budget check + counter add/sub sequence; total must be larger.
	if block.execSize < 50 {
		t.Errorf("execSize=%d too small — back-edge budget machinery may have been skipped", block.execSize)
	}
}

// TestIE64CompileRegion_PopulatesCoveredRanges_NonMonotonic verifies
// non-monotonic regions get every constituent block recorded in
// coveredRanges, so SMC invalidation catches a guest write to the
// middle block (mirrors M68K's regression test).
func TestIE64CompileRegion_PopulatesCoveredRanges_NonMonotonic(t *testing.T) {
	mem := make([]byte, 0x10000)
	putIE64NOP(mem, 0x100)
	putIE64BRA(mem, 0x108, 0x5000)
	putIE64NOP(mem, 0x5000)
	putIE64BRA(mem, 0x5008, 0x200)
	putIE64NOP(mem, 0x200)
	putIE64RTS(mem, 0x208)

	region := ie64FormRegion(0x100, mem)
	if region == nil || len(region.blocks) != 3 {
		t.Fatalf("expected 3-block region, got %v", region)
	}

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	block, err := ie64CompileRegion(region, execMem, mem)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	covered := JITBlockCoveredRanges(block)
	if len(covered) != 3 {
		t.Fatalf("coveredRanges len = %d want 3 — got %v", len(covered), covered)
	}
	cc := NewCodeCache()
	cc.Put(block)
	cc.InvalidateRange(0x5000, 0x5008)
	if cc.Get(0x100) != nil {
		t.Error("region survived InvalidateRange(0x5000,0x5008) — SMC gap not closed")
	}
}
