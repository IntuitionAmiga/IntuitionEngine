// jit_x86_tier_amd64.go - x86 Tier-2 controller binding (Phase 3a).
//
// Holds x86TierController, the package-level singleton bound to x86's
// reference RegPressureProfile. Build tag matches jit_x86_exec.go (the
// sole consumer) so the symbol is defined on every platform that
// compiles the exec loop — amd64 (linux/windows/darwin) plus arm64
// linux.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

// x86TierController is the shared Phase 3 promotion controller bound
// to x86's reference RegPressureProfile. The inline gate in
// jit_x86_exec.go delegates to ShouldPromote so any future threshold
// tweak applies uniformly across backends. Threshold defaults
// (DefaultTierThresholds: 64 execs, <25% I/O bail rate) match the
// preexisting x86Tier2Threshold + 1/4 ratio bit-for-bit.
//
// Promotion mechanism: region-only. x86CompileRegion (3+ block hot
// regions) consumes ShouldPromote and recompiles with x86Tier2RegAlloc.
// Single-block Tier-2 was retired (see x86CompileBlock comment);
// reviving it would require execMem reclamation +
// chain-slot-rewrite-on-promotion infrastructure that no measured
// workload currently justifies.
var x86TierController = NewTierController(X86RegProfile)
