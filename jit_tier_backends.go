// jit_tier_backends.go - per-backend Tier-2 allocator dispositions
// (Phase 3 of the six-CPU JIT unification plan).
//
// The closure pass retired per-block register-map promotion for M68K, Z80,
// and 6502. IE64 now takes its high-tier payoff through the turbo region
// path in the exec loop; Z80/6502 keep their existing chain-patching and
// pinned-register designs. x86 single-block promotion was also retired; x86
// region promotion remains implemented in the x86-specific path.
//
// These no-op allocators are retained only to keep the shared TierAllocator
// registry total over the backend set. A false return is the permanent
// disposition, not an unfinished implementation hook.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

// IE64TierAllocator implements TierAllocator for the IE64 backend.
// IE64's aggressive tier is region-level turbo compilation rather than
// single-block replacement. The exec loop drives promotion directly through
// ie64FormRegion/ie64CompileRegion so it can reject MMU, I/O-heavy, and
// unsupported region shapes before compiling. PromoteBlock remains a no-op
// because there is no standalone per-block IE64 tier-2 entry point.
type IE64TierAllocator struct{}

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
// Retired (closure-plan B.3.c): pinning IX/IY into additional host
// scratch slots collides with the heavy R10/R11/RAX/RCX/RDX
// consumption across z80EmitMemRead / z80EmitMemWrite /
// z80EmitALUByteOp / z80EmitFlags_* — the same scratch-conflict
// pattern that retired M68K and IE64 Tier-2. Z80 already pins BC/DE/HL
// packed; promotion would force a costly per-instruction spill/reload
// dance with no measurable headroom on demos that touch IX/IY.
// PromoteBlock stays a permanent no-op for API uniformity. Z80 region
// formation (closure-plan B.3.b retire — see Z80TierAllocator header)
// stays at the chain-patching layer rather than single-JITBlock fusion.
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

// X86TierAllocator is a permanent no-op. x86 region promotion is driven by
// x86FormRegion/x86CompileRegion in the x86 exec path; the abandoned
// single-block promotion path was removed during closure.
type X86TierAllocator struct{}

func (X86TierAllocator) PromoteBlock(pc uint32) bool { return false }

// BackendTierAllocators is audit metadata keyed by backend tag string
// (matching BackendCanonicalABI). Exec loops that do region promotion call
// TierController.ShouldPromote directly and intentionally do not delegate to
// these retired allocators.
var BackendTierAllocators = map[string]TierAllocator{
	"ie64": IE64TierAllocator{},
	"m68k": M68KTierAllocator{},
	"z80":  Z80TierAllocator{},
	"6502": P65TierAllocator{},
	"x86":  X86TierAllocator{},
}
