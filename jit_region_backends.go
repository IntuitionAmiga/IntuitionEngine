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

// ScanRegionM68K is the M68K region walker. Today returns an empty result
// (no region formation); Phase 4 sub-phase 4a replaces with a real walk.
func ScanRegionM68K(memory []byte, startPC uint32) RegionScanResult {
	return RegionScanResult{Profile: M68KRegionProfile}
}

// ScanRegionIE64 is the IE64 region walker. Sub-phase 4b.
func ScanRegionIE64(memory []byte, startPC uint32) RegionScanResult {
	return RegionScanResult{Profile: IE64RegionProfile}
}

// ScanRegionZ80 is the Z80 region walker. Sub-phase 4c.
func ScanRegionZ80(memory []byte, startPC uint32) RegionScanResult {
	return RegionScanResult{Profile: Z80RegionProfile}
}

// ScanRegionP65 is the 6502 region walker. Sub-phase 4d. 6502 caps at 4
// blocks because of 256-byte page semantics (the existing JIT chain
// patcher cannot cross a page boundary cheaply).
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
