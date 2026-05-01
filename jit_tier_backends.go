// jit_tier_backends.go - per-backend Tier-2 allocator scaffolds
// (Phase 3 sub-phases 3b/3c/3d/3e of the six-CPU JIT unification plan).
//
// Phase 3a migrates x86's existing Tier-2 promoter (jit_x86_exec.go:233-266
// + x86Tier2RegAlloc) to the shared TierController. Phase 3b-3e roll the
// same controller out to IE64, M68K, Z80, 6502, in order of expected
// payoff.
//
// Each backend's TierAllocator implementation lives below as a scaffold
// — it returns a static decision today (no Tier-2 promotion) so existing
// behavior is preserved. The follow-up patch per backend replaces
// PromoteBlock / PreserveAcrossInvalidation with the real regalloc.
//
// Wiring: each backend's exec loop (jit_<cpu>_exec.go) calls
// TierController.ShouldPromote(block) once execCount crosses its
// per-backend threshold. The controller delegates to the backend's
// allocator below.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

// IE64TierAllocator implements TierAllocator for the IE64 backend.
// IE64 has 5 mapped regs (R1-R4, R31) plus headroom in R5-R30 (spilled),
// so Tier-2 promotion can pin a small set of frequently-used spilled
// regs into RAX/RCX/RDX/R10/R11 scratch slots for the duration of a
// hot block.
type IE64TierAllocator struct{}

// PromoteBlock recompiles the block at the given guest PC at Tier 2.
// Scaffold: always false (no behavior change). Phase 3b real impl
// inspects block instr histogram for spilled-reg use density.
func (IE64TierAllocator) PromoteBlock(pc uint32) bool { return false }

// M68KTierAllocator implements TierAllocator for the M68K backend.
// M68K's biggest single-backend win: A1-A6 (currently spilled into the
// AddrRegs[] array) get pinned during a hot block. Plan Phase 3c
// estimates +25% on Mixed.
type M68KTierAllocator struct{}

func (M68KTierAllocator) PromoteBlock(pc uint32) bool { return false }

// Z80TierAllocator implements TierAllocator for the Z80 backend.
// Z80 already has BC/DE/HL pinned packed; promotion targets are IX/IY
// shadow regs (currently re-loaded each access).
type Z80TierAllocator struct{}

func (Z80TierAllocator) PromoteBlock(pc uint32) bool { return false }

// P65TierAllocator implements TierAllocator for the 6502 backend.
// Plan: 6502 is "already nearly fully pinned, mostly for API uniformity."
// The promotion the controller can grant is bench-kernel-style fusion
// of zero-page accesses.
type P65TierAllocator struct{}

func (P65TierAllocator) PromoteBlock(pc uint32) bool { return false }

// X86TierAllocator wraps the existing x86 Tier-2 promoter
// (x86Tier2RegAlloc) under the shared TierAllocator interface. Phase 3a
// completion replaces this stub with a thin delegation.
type X86TierAllocator struct{}

func (X86TierAllocator) PromoteBlock(pc uint32) bool { return false }

// BackendTierAllocators is the registry consumed by per-backend exec
// loops to look up their allocator. Keyed by backend tag string (matches
// BackendCanonicalABI).
//
// Initialised here, consumed by the per-backend exec loop's wiring patch.
var BackendTierAllocators = map[string]TierAllocator{
	"ie64": IE64TierAllocator{},
	"m68k": M68KTierAllocator{},
	"z80":  Z80TierAllocator{},
	"6502": P65TierAllocator{},
	"x86":  X86TierAllocator{},
}
