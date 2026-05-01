// jit_region_backends.go - per-backend region/superblock scan scaffolds
// (Phase 4 of the six-CPU JIT unification plan).
//
// jit_region_common.go defines RegionProfile + per-backend constants
// (X86/IE64/M68K=8 blocks, Z80=6, P65=4 with page-respecting termination).
// This file declares per-backend ScanRegion entry points so the rollout
// (M68K → IE64 → Z80 → 6502 per plan) lands behind a uniform shape.
//
// Each entry point today returns nil — meaning "no region formation for
// this backend yet, fall back to existing scanBlock behavior." The
// follow-up patches per backend replace the stub with a real region
// walker that honors the backend's terminator class and max-block count.

//go:build amd64 && (linux || windows || darwin)

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

// ScanRegionIE64 walks forward from startPC following statically-known
// BRA/JMP targets and returns the list of block start PCs that would
// form a region under IE64RegionProfile. Mirrors ScanRegionX86 /
// ScanRegionM68K: pure memory-driven walker, no cache lookup.
//
// Stops on:
//   - cycle (back-edge that revisits an already-scanned block)
//   - non-region-shaped block (needsFallback / empty scan)
//   - non-resolvable terminator (RTS/JSR/JSR_IND/HALT/RTI/WAIT/SYSCALL/...)
//   - max blocks / max instructions reached (IE64RegionProfile bounds)
//
// Returns an empty BlockPCs when fewer than 2 blocks form (single-block
// start has no region payoff).
func ScanRegionIE64(memory []byte, startPC uint32) RegionScanResult {
	res := RegionScanResult{Profile: IE64RegionProfile, Terminator: IE64RegionProfile.Terminator}
	pc := startPC
	totalInstrs := 0
	visited := map[uint32]struct{}{}
	for len(res.BlockPCs) < IE64RegionProfile.MaxBlocks && totalInstrs < IE64RegionProfile.MaxInstructions {
		if _, seen := visited[pc]; seen {
			break
		}
		if pc >= uint32(len(memory)) {
			break
		}
		instrs := scanBlock(memory, pc)
		if len(instrs) == 0 || needsFallback(instrs) {
			break
		}
		// Cap check before append: a long final block must not push
		// the region past MaxInstructions. First block always admitted.
		if len(res.BlockPCs) > 0 && totalInstrs+len(instrs) > IE64RegionProfile.MaxInstructions {
			break
		}
		visited[pc] = struct{}{}
		res.BlockPCs = append(res.BlockPCs, pc)
		totalInstrs += len(instrs)

		last := &instrs[len(instrs)-1]
		if !isBlockTerminator(last.opcode) {
			break
		}
		// Skip synthetic fused RTS markers — not real terminators.
		if last.fusedFlag&ie64FusedRTSLeafReturn != 0 {
			break
		}
		instrPC := pc + last.pcOffset
		target, ok := ie64ResolveTerminatorTarget(last.opcode, last.rs, last.imm32, instrPC)
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

// ScanRegionZ80 is the Z80 region walker. Sub-phase 4c.
func ScanRegionZ80(memory []byte, startPC uint32) RegionScanResult {
	return RegionScanResult{Profile: Z80RegionProfile}
}

// ScanRegionP65 is intentionally empty. Closure-plan B.4 retired the
// 6502 region scope: page-boundary back-edges make region formation
// risky, and the 4-block cap dictated by 256-byte page semantics
// leaves no expected speedup that justifies the chain-rewrite cost.
// Revisit only if the Phase 9 gate identifies 6502 as the lagging
// backend.
func ScanRegionP65(memory []byte, startPC uint32) RegionScanResult {
	return RegionScanResult{Profile: P65RegionProfile}
}

// ScanRegionX86 walks forward from startPC following statically-known
// terminator targets and returns the list of block start PCs that
// would form a region under x86RegionProfile. This is a pure
// memory-driven walker — it does not consult the JIT cache (callers
// that want cache-aware region formation use the existing
// x86FormRegion in jit_x86_common.go which adds the cache check).
//
// Returns an empty BlockPCs when the start block is non-region-shaped
// (no static terminator, fallback-required instruction, or out of
// memory bounds).
func ScanRegionX86(memory []byte, startPC uint32) RegionScanResult {
	res := RegionScanResult{Profile: X86RegionProfile, Terminator: X86RegionProfile.Terminator}
	pc := startPC
	totalInstrs := 0
	visited := map[uint32]struct{}{}
	for len(res.BlockPCs) < X86RegionProfile.MaxBlocks && totalInstrs < X86RegionProfile.MaxInstructions {
		if _, seen := visited[pc]; seen {
			break
		}
		if pc >= uint32(len(memory)) {
			break
		}
		instrs := x86ScanBlock(memory, pc)
		if len(instrs) == 0 || x86NeedsFallback(instrs) {
			break
		}
		visited[pc] = struct{}{}
		res.BlockPCs = append(res.BlockPCs, pc)
		totalInstrs += len(instrs)
		last := &instrs[len(instrs)-1]
		if !x86IsBlockTerminator(last.opcode) {
			pc = last.opcodePC + uint32(last.length)
			continue
		}
		target, ok := x86ResolveTerminatorTarget(last, memory, pc)
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

// BackendRegionScanners is the registry consumed by per-backend exec
// loops once region formation lands. Keyed by backend tag string.
var BackendRegionScanners = map[string]func(memory []byte, startPC uint32) RegionScanResult{
	"m68k": ScanRegionM68K,
	"ie64": ScanRegionIE64,
	"z80":  ScanRegionZ80,
	"6502": ScanRegionP65,
	"x86":  ScanRegionX86,
}
