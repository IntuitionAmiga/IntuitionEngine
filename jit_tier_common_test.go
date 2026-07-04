// jit_tier_common_test.go - Phase 3a parity gate for the shared
// TierController.ShouldPromote arithmetic.
//
// jit_x86_exec.go formerly inlined the promotion gate as:
//
//   if block.tier == 0 && block.execCount >= 64 {
//       if block.lastPromoteAt == 0 {
//           if block.ioBails*4 < block.execCount {
//               shouldPromote = true
//           }
//       }
//   }
//
// Phase 3a replaced that with x86TierController.ShouldPromote(...). The
// table below pins the bit-for-bit equivalence so any future threshold
// tweak fails this gate before reaching production.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func legacyX86ShouldPromote(tier int, execCount, ioBails, lastPromoteAt uint32) bool {
	if !(tier == 0 && execCount >= 64) {
		return false
	}
	if lastPromoteAt != 0 {
		return false
	}
	return ioBails*4 < execCount
}

func TestTierController_X86Parity(t *testing.T) {
	cases := []struct {
		name                              string
		tier                              int
		execCount, ioBails, lastPromoteAt uint32
	}{
		{"cold-not-promotable", 0, 10, 0, 0},
		{"hot-clean", 0, 64, 0, 0},
		{"hot-clean-high", 0, 4096, 100, 0},
		{"hot-borderline-iobail", 0, 64, 16, 0}, // 16*4 == 64, not < 64 → false
		{"hot-just-under-iobail", 0, 64, 15, 0}, // 15*4 == 60 < 64 → true
		{"already-tier2", 1, 1024, 0, 0},
		{"already-promoted-once", 0, 1024, 0, 64},
		{"hot-iobound", 0, 100, 30, 0},     // 120 >= 100 → false
		{"hot-not-iobound", 0, 100, 24, 0}, // 96 < 100 → true
		{"zero-exec", 0, 0, 0, 0},
		{"u32-near-max", 0, 1 << 30, 1 << 28, 0}, // 4*(1<<28) == 1<<30; not < → false
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := legacyX86ShouldPromote(tc.tier, tc.execCount, tc.ioBails, tc.lastPromoteAt)
			got := x86TierController.ShouldPromote(tc.tier, tc.execCount, tc.ioBails, tc.lastPromoteAt)
			if got != want {
				t.Errorf("ShouldPromote(tier=%d exec=%d io=%d last=%d) = %v, want %v",
					tc.tier, tc.execCount, tc.ioBails, tc.lastPromoteAt, got, want)
			}
		})
	}
}

func TestTierController_X86_DefaultThresholds(t *testing.T) {
	if x86TierController.Thresholds.PromoteAtExecCount != x86Tier2Threshold {
		t.Errorf("x86TierController threshold (%d) drifted from x86Tier2Threshold (%d)",
			x86TierController.Thresholds.PromoteAtExecCount, x86Tier2Threshold)
	}
	if x86TierController.Thresholds.IOBailMaxNumerator != 1 ||
		x86TierController.Thresholds.IOBailMaxDenominator != 4 {
		t.Errorf("x86TierController bail ratio drifted from 1/4 (got %d/%d)",
			x86TierController.Thresholds.IOBailMaxNumerator,
			x86TierController.Thresholds.IOBailMaxDenominator)
	}
	if x86TierController.Thresholds.RegionMinBlocks != 3 {
		t.Errorf("x86 region floor = %d, want legacy 3", x86TierController.Thresholds.RegionMinBlocks)
	}
}

func TestTierController_RegionFloorPerBackend(t *testing.T) {
	cases := []struct {
		name       string
		controller *TierController
		wantFloor  uint32
	}{
		{"x86", x86TierController, 3},
		{"ie64", ie64TierController, 2},
		{"m68k", m68kTierController, 2},
		{"6502", p65TierController, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.controller.Thresholds.RegionMinBlocks; got != tc.wantFloor {
				t.Fatalf("RegionMinBlocks = %d, want %d", got, tc.wantFloor)
			}
			if tc.controller.ShouldPromoteRegion(int(tc.wantFloor - 1)) {
				t.Fatalf("admitted %d-block region below floor %d", tc.wantFloor-1, tc.wantFloor)
			}
			if !tc.controller.ShouldPromoteRegion(int(tc.wantFloor)) {
				t.Fatalf("rejected %d-block region at floor", tc.wantFloor)
			}
		})
	}
}

func TestTierController_LegacyParityAllBackends(t *testing.T) {
	cases := []struct {
		name       string
		controller *TierController
		threshold  uint32
	}{
		{"x86", x86TierController, 64},
		{"ie64", ie64TierController, 64},
		{"m68k", m68kTierController, 64},
		{"6502", p65TierController, 32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.controller.ShouldPromote(0, tc.threshold-1, 0, 0) {
				t.Fatal("promoted below exec threshold")
			}
			if !tc.controller.ShouldPromote(0, tc.threshold, 0, 0) {
				t.Fatal("rejected clean block at exec threshold")
			}
			if tc.controller.ShouldPromote(0, tc.threshold, tc.threshold/4, 0) {
				t.Fatal("promoted block at legacy 25 percent deopt boundary")
			}
			if !tc.controller.ShouldPromote(0, tc.threshold, tc.threshold/4-1, 0) {
				t.Fatal("rejected block below legacy deopt boundary")
			}
		})
	}
}

func TestTierController_DeoptParity(t *testing.T) {
	cases := []struct {
		name                              string
		tier                              int
		execCount, ioBails, lastPromoteAt uint32
	}{
		{"cold", 0, 10, 0, 0},
		{"hot-clean", 0, 64, 0, 0},
		{"borderline", 0, 64, 16, 0},
		{"under-borderline", 0, 64, 15, 0},
		{"already-tier2", 1, 1024, 0, 0},
		{"already-promoted", 0, 1024, 0, 64},
		{"hot-iobound", 0, 100, 30, 0},
		{"hot-not-iobound", 0, 100, 24, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var deopts DeoptStatsSnapshot
			deopts.Counts[DeoptMMIO] = uint64(tc.ioBails)
			deopts.Total = uint64(tc.ioBails)
			got := x86TierController.ShouldPromoteDeopt(tc.tier, tc.execCount, tc.lastPromoteAt, deopts)
			want := legacyX86ShouldPromote(tc.tier, tc.execCount, tc.ioBails, tc.lastPromoteAt)
			if got != want {
				t.Fatalf("ShouldPromoteDeopt = %v, want %v", got, want)
			}
		})
	}
}

func TestTierController_AlwaysDeoptNeverPromoted(t *testing.T) {
	var deopts DeoptStatsSnapshot
	deopts.Counts[DeoptUnsupported] = uint64(x86Tier2Threshold)
	deopts.Total = uint64(x86Tier2Threshold)
	if x86TierController.ShouldPromoteDeopt(0, x86Tier2Threshold, 0, deopts) {
		t.Fatal("always-deopting block promoted")
	}
}
