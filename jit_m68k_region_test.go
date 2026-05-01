// jit_m68k_region_test.go - tests for the M68K region scanner
// (Phase 4 sub-phase 4a: pure memory-driven walker, no compile/exec).
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// putBE16 writes a big-endian 16-bit value at offset.
func putBE16(mem []byte, off uint32, v uint16) {
	mem[off] = byte(v >> 8)
	mem[off+1] = byte(v)
}

// putBranchDisp16 writes a 16-bit signed branch displacement at offset.
// disp may be negative; the conversion goes via int32 to avoid Go const
// "negative→unsigned" prohibition when the displacement is fixed.
func putBranchDisp16(mem []byte, off uint32, disp int32) {
	v := uint16(int16(disp))
	putBE16(mem, off, v)
}

// putBE32 writes a big-endian 32-bit value at offset.
func putBE32(mem []byte, off uint32, v uint32) {
	mem[off] = byte(v >> 24)
	mem[off+1] = byte(v >> 16)
	mem[off+2] = byte(v >> 8)
	mem[off+3] = byte(v)
}

// emitNopMoveq writes a MOVEQ #0,D0 (0x7000) at offset; harmless register
// op so the scanner has at least one non-terminator instruction in the
// block before reaching the terminator.
func emitMoveq(mem []byte, off uint32) {
	putBE16(mem, off, 0x7000)
}

// TestScanRegionM68K_FollowsBRAChain builds three blocks each ending
// in a BRA to the next, and asserts the scanner stitches them.
func TestScanRegionM68K_FollowsBRAChain(t *testing.T) {
	mem := make([]byte, 0x1000)

	// Block A @ 0x100: MOVEQ; BRA.W to 0x200.
	emitMoveq(mem, 0x100)
	putBE16(mem, 0x102, 0x6000) // BRA with 16-bit displacement (low byte 0)
	// disp16 from end of opcode word: target = (0x102 + 2) + disp = 0x200
	putBranchDisp16(mem, 0x104, int32(0x200)-int32(0x102+2))

	// Block B @ 0x200: MOVEQ; BRA.W to 0x300.
	emitMoveq(mem, 0x200)
	putBE16(mem, 0x202, 0x6000)
	putBE16(mem, 0x204, uint16(int16(0x300-(0x202+2))))

	// Block C @ 0x300: MOVEQ; RTS (terminator without static target).
	emitMoveq(mem, 0x300)
	putBE16(mem, 0x302, 0x4E75)

	res := ScanRegionM68K(mem, 0x100)
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

// TestScanRegionM68K_StopsOnRTS verifies the scanner terminates at an
// RTS-only block and does not silently extend past indirect transfers.
func TestScanRegionM68K_StopsOnRTS(t *testing.T) {
	mem := make([]byte, 0x1000)

	// Block A @ 0x100: BRA.W to 0x200.
	putBE16(mem, 0x100, 0x6000)
	putBE16(mem, 0x102, uint16(int16(0x200-(0x100+2))))

	// Block B @ 0x200: RTS.
	putBE16(mem, 0x200, 0x4E75)

	res := ScanRegionM68K(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 {
		t.Fatalf("BlockPCs len = %d want 2 (%v)", got, res.BlockPCs)
	}
}

// TestScanRegionM68K_StopsOnBSR ensures the scanner does not follow
// calls into the callee's region (BSR is a statically-known target but
// is treated as a call boundary).
func TestScanRegionM68K_StopsOnBSR(t *testing.T) {
	mem := make([]byte, 0x1000)

	// Block @ 0x100: BSR.W to 0x300.
	putBE16(mem, 0x100, 0x6100) // BSR (cond=1) with 16-bit disp
	putBE16(mem, 0x102, uint16(int16(0x300-(0x100+2))))

	// Block @ 0x300: RTS.
	putBE16(mem, 0x300, 0x4E75)

	res := ScanRegionM68K(mem, 0x100)
	if len(res.BlockPCs) != 0 {
		t.Errorf("expected single BSR-only block to yield no region, got %v", res.BlockPCs)
	}
}

// TestScanRegionM68K_FollowsJMPAbsLong builds a chain via JMP <abs.L>.
func TestScanRegionM68K_FollowsJMPAbsLong(t *testing.T) {
	mem := make([]byte, 0x1000)

	// Block A @ 0x100: MOVEQ; JMP <abs.L> 0x200.
	emitMoveq(mem, 0x100)
	putBE16(mem, 0x102, 0x4EF9) // JMP abs.L
	putBE32(mem, 0x104, 0x200)

	// Block B @ 0x200: RTS.
	putBE16(mem, 0x200, 0x4E75)

	res := ScanRegionM68K(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 || res.BlockPCs[0] != 0x100 || res.BlockPCs[1] != 0x200 {
		t.Errorf("expected [100, 200], got %v", res.BlockPCs)
	}
}

// TestScanRegionM68K_DetectsBackEdge stops cleanly on a back-edge
// instead of looping forever.
func TestScanRegionM68K_DetectsBackEdge(t *testing.T) {
	mem := make([]byte, 0x1000)

	// Block A @ 0x100: MOVEQ; BRA.W to 0x200.
	emitMoveq(mem, 0x100)
	putBE16(mem, 0x102, 0x6000)
	putBranchDisp16(mem, 0x104, int32(0x200)-int32(0x102+2))

	// Block B @ 0x200: MOVEQ; BRA.W back to 0x100.
	emitMoveq(mem, 0x200)
	putBE16(mem, 0x202, 0x6000)
	putBranchDisp16(mem, 0x204, int32(0x100)-int32(0x202+2))

	res := ScanRegionM68K(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 {
		t.Errorf("expected exactly 2 blocks (back-edge stop), got %d (%v)", got, res.BlockPCs)
	}
}

// TestScanRegionM68K_RejectsOutOfBounds matches the X86 contract.
func TestScanRegionM68K_RejectsOutOfBounds(t *testing.T) {
	res := ScanRegionM68K(make([]byte, 0x100), 0xFFFFFFFF)
	if len(res.BlockPCs) != 0 {
		t.Errorf("out-of-bounds startPC must yield empty region, got %v", res.BlockPCs)
	}
}

// TestScanRegionM68K_RespectsMaxInstructions guards against the
// append-then-update bug where a long final block could push the
// returned region past MaxInstructions. Builds blocks that nearly fill
// the maxInstr, then a final block of MOVEQs sized to overshoot — the
// scanner must drop the overshoot block, not include it.
func TestScanRegionM68K_RespectsMaxInstructions(t *testing.T) {
	mem := make([]byte, 0x20000)
	maxInstr := M68KRegionProfile.MaxInstructions
	if maxInstr == 0 {
		t.Skip("MaxInstructions is 0; maxInstr not enforceable")
	}

	// Block A @ 0x100: one MOVEQ + BRA.W to 0x200. (instr count ≈ 2)
	emitMoveq(mem, 0x100)
	putBE16(mem, 0x102, 0x6000)
	putBranchDisp16(mem, 0x104, int32(0x200)-int32(0x102+2))

	// Block B @ 0x200: maxInstr-1 MOVEQs followed by RTS. Block alone has
	// ~maxInstr instrs. After admitting block A (~2 instrs), admitting block B
	// (~maxInstr instrs) would push total over maxInstr. The scanner must stop
	// before appending block B.
	pc := uint32(0x200)
	for i := 0; i < maxInstr+10; i++ {
		emitMoveq(mem, pc)
		pc += 2
	}
	putBE16(mem, pc, 0x4E75) // RTS

	res := ScanRegionM68K(mem, 0x100)
	// Sum the instruction counts of admitted blocks; must not exceed maxInstr.
	totalAdmitted := 0
	for _, bpc := range res.BlockPCs {
		totalAdmitted += len(m68kScanBlock(mem, bpc))
	}
	if totalAdmitted > maxInstr {
		t.Errorf("region instruction sum %d exceeded MaxInstructions maxInstr %d (BlockPCs=%v)",
			totalAdmitted, maxInstr, res.BlockPCs)
	}
}

// TestM68KFormRegion_RejectsSingleBlock matches m68kFormRegion's
// per-backend single-block rejection contract — fewer than 2 blocks
// returns nil so the caller falls back to per-block compile.
func TestM68KFormRegion_RejectsSingleBlock(t *testing.T) {
	mem := make([]byte, 0x1000)
	// Single RTS block at 0x100.
	putBE16(mem, 0x100, 0x4E75)
	if region := m68kFormRegion(0x100, mem); region != nil {
		t.Errorf("expected nil region for single-block scan, got %+v", region)
	}
}

// TestM68KFormRegion_BuildsTwoBlockChain checks the cache-aware region
// builder admits a clean BRA-chain region.
func TestM68KFormRegion_BuildsTwoBlockChain(t *testing.T) {
	mem := make([]byte, 0x1000)
	// Block A: MOVEQ; BRA.W to 0x200.
	emitMoveq(mem, 0x100)
	putBE16(mem, 0x102, 0x6000)
	putBranchDisp16(mem, 0x104, int32(0x200)-int32(0x102+2))
	// Block B: MOVEQ; RTS.
	emitMoveq(mem, 0x200)
	putBE16(mem, 0x202, 0x4E75)

	region := m68kFormRegion(0x100, mem)
	if region == nil {
		t.Fatal("expected non-nil region for valid 2-block chain")
	}
	if len(region.blocks) != 2 || region.blockPCs[0] != 0x100 || region.blockPCs[1] != 0x200 {
		t.Errorf("region shape mismatch: blockPCs=%v len(blocks)=%d", region.blockPCs, len(region.blocks))
	}
	if region.entryPC != 0x100 {
		t.Errorf("entryPC=%X want 0x100", region.entryPC)
	}
}

// TestM68KCompileRegion_BuildsSingleNativeBlock exercises
// m68kCompileRegion end-to-end: scan + form + compile, then verify the
// returned JITBlock spans the region's PC range and was admitted into
// ExecMem.
func TestM68KCompileRegion_BuildsSingleNativeBlock(t *testing.T) {
	mem := make([]byte, 0x1000)
	emitMoveq(mem, 0x100)
	putBE16(mem, 0x102, 0x6000)
	putBranchDisp16(mem, 0x104, int32(0x200)-int32(0x102+2))
	emitMoveq(mem, 0x200)
	putBE16(mem, 0x202, 0x4E75)

	region := m68kFormRegion(0x100, mem)
	if region == nil {
		t.Fatal("expected non-nil region")
	}

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	block, err := m68kCompileRegion(region, execMem, mem)
	if err != nil {
		t.Fatalf("m68kCompileRegion: %v", err)
	}
	if block.startPC != 0x100 {
		t.Errorf("startPC=%X want 0x100", block.startPC)
	}
	if block.endPC < 0x202 {
		t.Errorf("endPC=%X must cover both blocks (>= 0x202)", block.endPC)
	}
	if block.execAddr == 0 {
		t.Error("execAddr is 0; native code not written")
	}
	if block.instrCount < 2 {
		t.Errorf("instrCount=%d want >= 2 (one MOVEQ per block)", block.instrCount)
	}
}

// TestM68KCompileRegion_PopulatesCoveredRanges_NonMonotonic guards
// against the SMC-invalidation gap where a region following
// non-monotonic static targets would derive endPC only from the last
// block's address. Builds 0x100→0x5000→0x200 and asserts the compiled
// region's coveredRanges enumerates all three block PC ranges so SMC
// invalidation can catch a guest write to any of them.
func TestM68KCompileRegion_PopulatesCoveredRanges_NonMonotonic(t *testing.T) {
	mem := make([]byte, 0x10000)
	// Block A @ 0x100: MOVEQ; BRA.W to 0x5000.
	emitMoveq(mem, 0x100)
	putBE16(mem, 0x102, 0x6000)
	putBranchDisp16(mem, 0x104, int32(0x5000)-int32(0x102+2))
	// Block B @ 0x5000: MOVEQ; BRA.W to 0x200.
	emitMoveq(mem, 0x5000)
	putBE16(mem, 0x5002, 0x6000)
	putBranchDisp16(mem, 0x5004, int32(0x200)-int32(0x5002+2))
	// Block C @ 0x200: MOVEQ; RTS.
	emitMoveq(mem, 0x200)
	putBE16(mem, 0x202, 0x4E75)

	region := m68kFormRegion(0x100, mem)
	if region == nil || len(region.blocks) != 3 {
		t.Fatalf("expected 3-block region, got %v", region)
	}

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	block, err := m68kCompileRegion(region, execMem, mem)
	if err != nil {
		t.Fatalf("m68kCompileRegion: %v", err)
	}
	if block.coveredRanges == nil {
		t.Fatal("region block missing coveredRanges (would silently miss SMC for middle block)")
	}
	covered := JITBlockCoveredRanges(block)
	if len(covered) != 3 {
		t.Fatalf("coveredRanges len = %d want 3 (one per block) — got %v", len(covered), covered)
	}
	// Verify a guest-write probe at 0x5000 would invalidate via the
	// CodeCache.InvalidateRange path.
	cc := NewCodeCache()
	cc.Put(block)
	cc.InvalidateRange(0x5000, 0x5004)
	if cc.Get(0x100) != nil {
		t.Error("region survived InvalidateRange(0x5000,0x5004) — SMC invalidation gap not closed")
	}
}

// TestScanRegionM68K_RespectsMaxBlocks bounds the walk per profile.
func TestScanRegionM68K_RespectsMaxBlocks(t *testing.T) {
	mem := make([]byte, 0x10000)
	// Lay down a long chain of MOVEQ; BRA.W to next.
	stride := uint32(0x100)
	first := uint32(0x100)
	for i := 0; i < M68KRegionProfile.MaxBlocks+4; i++ {
		pc := first + uint32(i)*stride
		emitMoveq(mem, pc)
		putBE16(mem, pc+2, 0x6000)
		next := pc + stride
		putBranchDisp16(mem, pc+4, int32(next)-int32(pc+2)-2)
	}
	res := ScanRegionM68K(mem, first)
	if len(res.BlockPCs) > M68KRegionProfile.MaxBlocks {
		t.Errorf("region exceeded MaxBlocks: got %d, maxInstr %d", len(res.BlockPCs), M68KRegionProfile.MaxBlocks)
	}
}
