// jit_region_common.go - shared region/superblock formation profile (Phase 4
// of the six-CPU JIT unification plan).
//
// x86 has region superblocks (jit_x86_common.go's x86FormRegion) that absorb
// up to 8 contiguous Tier-1 blocks (≤512 instructions, fall-through-only,
// terminator at the end) into a single Tier-2 compilation unit. The plan
// extends this to M68K, IE64, Z80, and 6502 in expected-payoff order:
// M68K → IE64 → Z80 → 6502 (capped at 4 blocks for 6502 because of 256-byte
// page semantics).
//
// This file collects the per-backend tunables — block count cap, instruction
// count cap, terminator rules, branch-direction restrictions — into a single
// RegionProfile struct, plus reference profiles per backend.
//
// The actual region-formation routine is backend-specific (each ISA's
// instruction shape determines fall-through detection); the shared piece is
// the budget arithmetic and the terminator-class enum.

//go:build amd64 && (linux || windows || darwin)

package main

// RegionTerminatorClass classifies the last-instruction-shape rules a
// backend uses when deciding whether a block can absorb its successor into
// the current region.
type RegionTerminatorClass int

const (
	// RegionTermAny — region may include any block whose terminator falls
	// through to a known-static successor PC. Default for x86, IE64, M68K.
	RegionTermAny RegionTerminatorClass = iota

	// RegionTermPageRespecting — region may only absorb successors whose
	// start PC sits on the same memory page as the current block (6502's
	// 256-byte page rule, where branch crossing pages costs an extra cycle
	// the JIT must conservatively model).
	RegionTermPageRespecting

	// RegionTermPrefixCapped — region must not absorb a successor that
	// starts with a prefix instruction that would re-bind a previous
	// per-block invariant (Z80 prefix-table re-entry, x86 segment override
	// chains). Reserved; not yet used.
	RegionTermPrefixCapped
)

// RegionProfile is a backend's region-formation budget.
type RegionProfile struct {
	// MaxBlocks caps how many Tier-1 blocks a region may absorb. x86's
	// reference is 8; 6502 caps at 4 because of 256-byte page semantics.
	MaxBlocks int

	// MaxInstructions caps the total guest-instruction count across all
	// absorbed blocks. x86's reference is 512; backends with larger
	// per-instruction encoded code may set lower.
	MaxInstructions int

	// Terminator is the shape rule the backend applies when deciding
	// whether a block's last instruction can flow into the next region
	// member.
	Terminator RegionTerminatorClass

	// AllowBackwardChain reports whether the region may include a block
	// whose terminator chains backward into an earlier region member (a
	// loop). x86 sets true so the rotozoomer's tight loop body becomes a
	// single region; 6502 sets true; Z80 sets true with R-register-tracking
	// safety; M68K sets true; IE64 sets true.
	AllowBackwardChain bool
}

// Reference profiles per backend. Phase 4's per-backend rollout consults
// these when deciding region absorption — the actual terminator-shape
// detection is in each backend's *FormRegion routine.
var (
	X86RegionProfile  = RegionProfile{MaxBlocks: 8, MaxInstructions: 512, Terminator: RegionTermAny, AllowBackwardChain: true}
	IE64RegionProfile = RegionProfile{MaxBlocks: 8, MaxInstructions: 512, Terminator: RegionTermAny, AllowBackwardChain: true}
	M68KRegionProfile = RegionProfile{MaxBlocks: 8, MaxInstructions: 512, Terminator: RegionTermAny, AllowBackwardChain: true}
	Z80RegionProfile  = RegionProfile{MaxBlocks: 6, MaxInstructions: 384, Terminator: RegionTermAny, AllowBackwardChain: true}
	P65RegionProfile  = RegionProfile{MaxBlocks: 4, MaxInstructions: 256, Terminator: RegionTermPageRespecting, AllowBackwardChain: true}
)
