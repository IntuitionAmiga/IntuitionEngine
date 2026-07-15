//go:build arm64 && (linux || windows || darwin)

// jit_ie64_arm64_regmap_test.go - constraints on the IE64 -> ARM64 GPR mapping.
//
// These are structural tests: they pin properties the emitter relies on but
// which nothing else checks, most importantly that the mapping never lands on
// X18. X18 is the AAPCS64 platform register. Darwin reserves it for the kernel
// and Windows/ARM64 keeps the TEB pointer in it, and this file's build tag
// covers both, so a mapped IE64 register living in X18 means JIT-compiled code
// clobbers platform state with no save or restore. Linux tolerates it only
// because the Go toolchain independently declines to allocate R18.

package main

import "testing"

// arm64ReservedGPRs lists host registers the emitter has already claimed for a
// fixed purpose. A mapped IE64 register aliasing any of these would silently
// corrupt the block's own bookkeeping.
func arm64ReservedGPRs() map[byte]string {
	return map[byte]string{
		arm64RegCtx:       "JITContext pointer",
		1:                 "helper scratch",
		2:                 "helper scratch",
		3:                 "helper scratch",
		4:                 "helper scratch",
		arm64RegIOBitmap:  "ioPageBitmap base",
		arm64RegFPUBase:   "FPU register file base",
		arm64RegLoopCount: "backward branch counter",
		arm64RegBase:      "register file base",
		arm64RegMemBase:   "memory base",
		arm64RegIOStart:   "IO_REGION_START",
		arm64RegScratch:   "address scratch",
		18:                "AAPCS64 platform register (Darwin kernel / Windows TEB)",
		arm64RegIE64PC:    "IE64 PC",
		arm64RegFP:        "Go frame pointer",
		arm64RegLR:        "Go link register",
	}
}

// TestARM64RegMap_NeverUsesX18 is the direct guard. IE64 R7 mapped to X18 under
// the original contiguous R1-R15 -> X12-X26 scheme.
func TestARM64RegMap_NeverUsesX18(t *testing.T) {
	for ie64Reg := byte(0); ie64Reg <= 31; ie64Reg++ {
		host, mapped := ie64ToARM64Reg(ie64Reg)
		if mapped && host == 18 {
			t.Errorf("IE64 R%d maps to X18, the platform register; "+
				"JIT code would clobber the Darwin kernel register / Windows TEB pointer",
				ie64Reg)
		}
	}
}

// TestARM64RegMap_AvoidsReservedRegisters checks the mapping against every host
// register the emitter has otherwise spoken for.
func TestARM64RegMap_AvoidsReservedRegisters(t *testing.T) {
	reserved := arm64ReservedGPRs()
	for ie64Reg := byte(1); ie64Reg <= 31; ie64Reg++ {
		host, mapped := ie64ToARM64Reg(ie64Reg)
		if !mapped {
			continue
		}
		// R31 deliberately lives in the dedicated SP register.
		if ie64Reg == 31 && host == arm64RegIE64SP {
			continue
		}
		if why, bad := reserved[host]; bad {
			t.Errorf("IE64 R%d maps to X%d, already reserved as %s", ie64Reg, host, why)
		}
	}
}

// TestARM64RegMap_IsInjective ensures two IE64 registers never share a host
// register, which would make one silently alias the other.
func TestARM64RegMap_IsInjective(t *testing.T) {
	owner := map[byte]byte{}
	for ie64Reg := byte(1); ie64Reg <= 31; ie64Reg++ {
		host, mapped := ie64ToARM64Reg(ie64Reg)
		if !mapped {
			continue
		}
		if prev, seen := owner[host]; seen {
			t.Errorf("X%d is mapped by both IE64 R%d and R%d", host, prev, ie64Reg)
			continue
		}
		owner[host] = ie64Reg
	}
}

// TestARM64RegMap_CalleeSavedPairsMatchMapping ties arm64CalleeSavedPairs to
// ie64ToARM64Reg. The prologue saves a callee-saved pair only when the block
// uses one of the IE64 registers the table associates with it, so a table that
// disagrees with the mapping means a block either clobbers a callee-saved
// register without saving it or reloads a stale value over a live one.
func TestARM64RegMap_CalleeSavedPairsMatchMapping(t *testing.T) {
	for i, p := range arm64CalleeSavedPairs {
		gotLo, mappedLo := ie64ToARM64Reg(p.loIE64)
		if !mappedLo || gotLo != p.loHost {
			t.Errorf("pair %d: table says IE64 R%d is in X%d, mapping says (X%d, mapped=%v)",
				i, p.loIE64, p.loHost, gotLo, mappedLo)
		}
		gotHi, mappedHi := ie64ToARM64Reg(p.hiIE64)
		if !mappedHi || gotHi != p.hiHost {
			t.Errorf("pair %d: table says IE64 R%d is in X%d, mapping says (X%d, mapped=%v)",
				i, p.hiIE64, p.hiHost, gotHi, mappedHi)
		}
		if p.hiHost != p.loHost+1 {
			t.Errorf("pair %d: X%d/X%d are not adjacent, STP/LDP requires consecutive registers",
				i, p.loHost, p.hiHost)
		}
	}
}

// TestARM64RegMap_CalleeSavedPairsCoverResidents checks that every mapped IE64
// register living in the callee-saved range X19-X28 is covered by some pair.
// An uncovered resident would be clobbered with no save, which is the same
// class of defect as the X18 bug.
func TestARM64RegMap_CalleeSavedPairsCoverResidents(t *testing.T) {
	covered := map[byte]bool{}
	for _, p := range arm64CalleeSavedPairs {
		covered[p.loIE64] = true
		covered[p.hiIE64] = true
	}
	for ie64Reg := byte(1); ie64Reg <= 30; ie64Reg++ {
		host, mapped := ie64ToARM64Reg(ie64Reg)
		if !mapped {
			continue
		}
		// X19-X26 is the callee-saved span this backend allocates from; X27/X28
		// are saved unconditionally by the prologue.
		if host < 19 || host > 26 {
			continue
		}
		if !covered[ie64Reg] {
			t.Errorf("IE64 R%d is resident in callee-saved X%d but no arm64CalleeSavedPairs entry saves it",
				ie64Reg, host)
		}
	}
}

// TestARM64RegMap_R0IsZeroRegister pins the XZR convention.
func TestARM64RegMap_R0IsZeroRegister(t *testing.T) {
	host, mapped := ie64ToARM64Reg(0)
	if !mapped || host != 31 {
		t.Errorf("IE64 R0 = (X%d, mapped=%v), want XZR (31, true)", host, mapped)
	}
}

// TestARM64RegMap_ResidentSetIsExpected pins which IE64 registers are resident.
// Dropping X18 costs one slot, so R15 spills; R1-R14 must stay resident or the
// backend has lost more than the platform-register fix requires.
func TestARM64RegMap_ResidentSetIsExpected(t *testing.T) {
	for ie64Reg := byte(1); ie64Reg <= 14; ie64Reg++ {
		if _, mapped := ie64ToARM64Reg(ie64Reg); !mapped {
			t.Errorf("IE64 R%d should be resident in a host register", ie64Reg)
		}
	}
	if _, mapped := ie64ToARM64Reg(15); mapped {
		t.Error("IE64 R15 should be spilled once X18 is excluded from the mapping")
	}
	for ie64Reg := byte(16); ie64Reg <= 30; ie64Reg++ {
		if _, mapped := ie64ToARM64Reg(ie64Reg); mapped {
			t.Errorf("IE64 R%d should be spilled", ie64Reg)
		}
	}
}
