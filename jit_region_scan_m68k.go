// jit_region_scan_m68k.go - backend-neutral M68020 region scanner.
//
// Split out of jit_region_backends.go (M68K JIT parity plan, milestone 2):
// ScanRegionM68K is pure frontend analysis (memory walker over
// m68kScanBlock) that every M68020 backend (amd64, arm64, wasm) needs, so
// it must not live behind the amd64 build tag. RegionScanResult moves with
// it because it is the shared return shape for all backend scanners.
//
// This file must stay free of build tags and emitter symbols.

package main

// RegionScanResult is the shared return shape for backend ScanRegion
// implementations. Empty BlockPCs means "fall back to single-block
// scanning"; that is the scaffold's default.
type RegionScanResult struct {
	BlockPCs   []uint32      // entry PCs of blocks bundled into this region
	Profile    RegionProfile // backend's profile that drove the scan
	Terminator RegionTerminatorClass
}

// ScanRegionM68K walks forward from startPC following statically-known
// BRA/JMP targets and returns the list of block start PCs that would
// form a region under M68KRegionProfile. Mirrors ScanRegionX86: pure
// memory-driven walker that does not consult the JIT cache.
//
// Stops on:
//   - cycle (back-edge that revisits any already-scanned block)
//   - non-region-shaped block (m68kNeedsFallback / empty scan)
//   - non-resolvable terminator (RTS/RTE/JSR/BSR/JMP-indirect/TRAP)
//   - max blocks / max instructions reached (M68KRegionProfile bounds)
//
// Returns an empty BlockPCs when fewer than 2 blocks are formed
// (single-block start has no region payoff).
func ScanRegionM68K(memory []byte, startPC uint32) RegionScanResult {
	res := RegionScanResult{Profile: M68KRegionProfile, Terminator: M68KRegionProfile.Terminator}
	pc := startPC
	totalInstrs := 0
	visited := map[uint32]struct{}{}
	for len(res.BlockPCs) < M68KRegionProfile.MaxBlocks && totalInstrs < M68KRegionProfile.MaxInstructions {
		if _, seen := visited[pc]; seen {
			break
		}
		if pc >= uint32(len(memory)) {
			break
		}
		instrs := m68kScanBlock(memory, pc)
		if len(instrs) == 0 || m68kNeedsFallback(instrs) {
			break
		}
		// Cap check BEFORE append: if this block would push the region
		// past MaxInstructions, stop without including it. Otherwise a
		// large terminal block could overshoot the advertised cap and
		// downstream region compilation would receive more instructions
		// than the profile permits. The first block is always admitted
		// (an empty region is the unhelpful alternative).
		if len(res.BlockPCs) > 0 && totalInstrs+len(instrs) > M68KRegionProfile.MaxInstructions {
			break
		}
		visited[pc] = struct{}{}
		res.BlockPCs = append(res.BlockPCs, pc)
		totalInstrs += len(instrs)

		last := &instrs[len(instrs)-1]
		if !m68kIsBlockTerminator(last.opcode) {
			// Block ran to size cap without a terminator — region cannot
			// confidently extend across the implicit fallthrough since the
			// scan stopped mid-stream.
			break
		}
		// Skip synthetic fused RTS markers — not real terminators.
		if last.fusedFlag&m68kFusedRTSLeafReturn != 0 {
			break
		}
		instrPC := pc + last.pcOffset
		target, ok := m68kResolveTerminatorTarget(last.opcode, instrPC, memory)
		if !ok {
			break
		}
		pc = target
	}
	if len(res.BlockPCs) < 2 {
		res.BlockPCs = nil
	}
	return res
}
