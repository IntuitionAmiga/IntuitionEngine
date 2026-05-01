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
// Closure-plan B.1.c disposition (RETIRED with architectural blocker):
// Pinning A1-A6 into a dedicated host register requires a free scratch
// slot. Both R10 and R11 are heavily used by the existing emitter for
// CCR-build sequences (SETcc result staging) and 64-bit MULL split
// paths — pinning either would require a 30-site emit-path refactor
// to route those scratch uses through RAX/RCX/RDX. The refactor
// exceeds the B.1.c slice budget; B.1.b's region compile (chain-exit
// internalisation) already captures the BranchDense win without
// register pinning. PromoteBlock stays a permanent no-op for API
// uniformity. M68K Tier-2 lives at region granularity only — see the
// plan's note "M68K Tier-2 lives at region granularity only, no
// single-block promotion."
type M68KTierAllocator struct{}

func (M68KTierAllocator) PromoteBlock(pc uint32) bool { return false }

// Z80TierAllocator implements TierAllocator for the Z80 backend.
// Z80 already has BC/DE/HL pinned packed; promotion targets are IX/IY
// shadow regs (currently re-loaded each access).
type Z80TierAllocator struct{}

func (Z80TierAllocator) PromoteBlock(pc uint32) bool { return false }

// P65TierAllocator implements TierAllocator for the 6502 backend.
// Retired (closure-plan B.4): 6502 region scope is deliberately
// vacant — the existing JIT already pins A/X/Y/P aggressively, and the
// 4-block region cap dictated by 256-byte page semantics leaves no
// regalloc surface that justifies the chain-rewrite cost. PromoteBlock
// stays a permanent no-op for API uniformity. Revisit only if the
// Phase 9 gate shows 6502 lagging.
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
