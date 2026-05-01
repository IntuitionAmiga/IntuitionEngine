// jit_tier_common.go - shared Tier-2 promotion controller (Phase 3 of the
// six-CPU JIT unification plan).
//
// x86 has a Tier-2 dynamic register-allocation path (jit_x86_exec.go:225-280)
// that lifts a hot block from the Tier-1 fixed-regmap encoding to a per-
// block reg-allocated encoding once the block has been re-entered enough
// times to amortize the recompile cost. The promoter has three knobs:
//
//   - execCount threshold (x86Tier2Threshold = 64 today)
//   - I/O bail-rate gate (ioBails*4 < execCount, i.e. <25%)
//   - never-promote-twice via lastPromoteAt
//
// Plan calls for lifting these into a backend-neutral controller so IE64,
// M68K, Z80, and 6502 can adopt them. This file defines the controller and
// the per-backend interfaces (TierAllocator + RegPressureProfile). Phase 3a
// migrates x86 onto the controller; sub-phases 3b-3e roll out to other
// backends in expected-payoff order: IE64 (5 mapped regs, large headroom)
// → M68K (A1-A6 spilled today, biggest single win) → Z80 → 6502.
//
// The controller itself is policy. The decision-making body of x86's
// promotion (region attempt, fall back to single-block) is backend-specific
// and stays in jit_x86_exec.go. The shared piece is the threshold/gate
// arithmetic so a behavioral change applies uniformly.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

// TierThresholds groups the policy knobs the promotion controller uses to
// decide whether a hot block should be recompiled at a higher tier.
//
// Defaults match x86's Tier-2 reference values (execCount ≥ 64, ioBail rate
// < 25%). Backends may override per-RegPressureProfile.
type TierThresholds struct {
	// PromoteAtExecCount is the minimum number of re-entries before the
	// block is considered hot enough to recompile. x86's reference value is
	// 64; tighter (e.g. 32) backends with smaller blocks may set lower.
	PromoteAtExecCount uint32

	// IOBailMaxNumerator/IOBailMaxDenominator encode the I/O bail-rate
	// ceiling above which the block is considered I/O-bound and not worth
	// promoting (the promoted Tier-2 encoding helps register pressure, not
	// MMIO latency). The check is:
	//
	//   block.ioBails * IOBailMaxDenominator < block.execCount * IOBailMaxNumerator
	//
	// x86 reference: 1/4 (i.e. 25% bail rate). Stored as an explicit ratio
	// so non-power-of-two thresholds remain expressible.
	IOBailMaxNumerator   uint32
	IOBailMaxDenominator uint32
}

// DefaultTierThresholds matches x86's reference Tier-2 promoter (the policy
// values currently coded inline at jit_x86_exec.go:235-242).
var DefaultTierThresholds = TierThresholds{
	PromoteAtExecCount:   64,
	IOBailMaxNumerator:   1,
	IOBailMaxDenominator: 4,
}

// RegPressureProfile describes a backend's host-register budget for the
// purpose of Tier-2 register allocation. Used by the controller to decide
// whether promotion is even possible (a backend with no spare host regs
// gains nothing from Tier 2).
type RegPressureProfile struct {
	// HostScratchCount is the number of host (amd64) GP registers the
	// backend can clobber freely without saving.
	HostScratchCount int

	// PinnedGuestRegs is the number of host registers the backend already
	// pins for guest state in Tier 1 (the fixed regmap). Lower → more
	// promotion headroom.
	PinnedGuestRegs int

	// HasFloatRegs reports whether the backend uses the host xmm bank in
	// addition to GP registers. IE64 (FPU) and x86 (SSE for FP/string ops)
	// set true.
	HasFloatRegs bool
}

// Reference profiles for the five amd64 backends. Numbers are derived from
// each backend's Tier-1 regmap; future Tier-2 work consults these to size
// per-block allocation budgets.
var (
	IE64RegProfile = RegPressureProfile{HostScratchCount: 8, PinnedGuestRegs: 5, HasFloatRegs: true}
	X86RegProfile  = RegPressureProfile{HostScratchCount: 6, PinnedGuestRegs: 7, HasFloatRegs: true}
	M68KRegProfile = RegPressureProfile{HostScratchCount: 7, PinnedGuestRegs: 8, HasFloatRegs: false}
	Z80RegProfile  = RegPressureProfile{HostScratchCount: 9, PinnedGuestRegs: 5, HasFloatRegs: false}
	P65RegProfile  = RegPressureProfile{HostScratchCount: 9, PinnedGuestRegs: 6, HasFloatRegs: false}
)

// TierController encodes the shared promotion policy. Backends call
// ShouldPromote on every hot-block hit and act on the result.
type TierController struct {
	Thresholds TierThresholds
	Profile    RegPressureProfile
}

// NewTierController constructs a controller with default thresholds for the
// given backend profile. Callers that want non-default thresholds set them
// directly on the returned struct.
func NewTierController(profile RegPressureProfile) *TierController {
	return &TierController{
		Thresholds: DefaultTierThresholds,
		Profile:    profile,
	}
}

// ShouldPromote reports whether a block at Tier 1 with the given hot-block
// counters should be promoted to Tier 2.
//
// Inputs:
//
//   - currentTier:    block.tier (0 = Tier 1, ≥1 = already promoted)
//   - execCount:      block.execCount (re-entries observed)
//   - ioBails:        block.ioBails (count of MMIO/MMU bails since compile)
//   - lastPromoteAt:  block.lastPromoteAt (0 if never; non-zero if we've
//     already promoted this block once and don't want to
//     re-promote)
//
// Returns true iff:
//
//   - currentTier == 0
//   - lastPromoteAt == 0
//   - execCount ≥ Thresholds.PromoteAtExecCount
//   - ioBails * IOBailMaxDenominator < execCount * IOBailMaxNumerator
//
// The arithmetic mirrors x86's existing inline check so x86's behavior is
// preserved bit-for-bit when sub-phase 3a migrates onto the controller.
func (c *TierController) ShouldPromote(currentTier int, execCount, ioBails, lastPromoteAt uint32) bool {
	if currentTier != 0 {
		return false
	}
	if lastPromoteAt != 0 {
		return false
	}
	if execCount < c.Thresholds.PromoteAtExecCount {
		return false
	}
	if uint64(ioBails)*uint64(c.Thresholds.IOBailMaxDenominator) >=
		uint64(execCount)*uint64(c.Thresholds.IOBailMaxNumerator) {
		return false
	}
	return true
}

// TierAllocator is the per-backend hook the controller calls when it
// promotes a block. The backend's implementation is responsible for the
// recompile, cache swap, and chain re-patching; the controller does not
// touch the code cache. Phase 3a wires x86Tier2RegAlloc as the first
// implementation; sub-phases 3b-3e add IE64/M68K/Z80/6502.
//
// The interface is intentionally narrow: the controller passes the block
// identity and the backend returns success/failure. The data shape
// (`*JITBlock` vs `*X86JITBlock` etc.) is backend-private, hidden behind
// the type-erasing parameter.
type TierAllocator interface {
	// PromoteBlock recompiles the block at the given guest PC at a higher
	// tier and installs it in the backend's code cache. Returns true on
	// success. On failure the block stays at its current tier; the
	// controller will not re-attempt until lastPromoteAt is reset.
	PromoteBlock(pc uint32) bool
}
