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
}
