// jit_m68k_abi_arm64_test.go - pins the arm64 M68K register mapping
// invariants (M68K JIT parity plan, milestone 3).
//
// The IE64 arm64 backend once mapped a guest register onto X18 because the
// mapping was re-derived inline and nothing pinned the reserved-register
// rule. This test makes the same mistake impossible for the M68K backend.

//go:build arm64 && (linux || windows || darwin)

package main

import "testing"

func TestM68KARM64ABI_ReservedRegistersNeverPinned(t *testing.T) {
	for _, r := range m68kARM64PinnedRegs {
		switch r {
		case m68kARM64PlatformReg:
			t.Fatalf("X18 (platform register) pinned by the M68K arm64 ABI")
		case 16, 17:
			t.Fatalf("X%d (IP0/IP1 emitter scratch) pinned by the M68K arm64 ABI", r)
		case 29, 30:
			t.Fatalf("X%d (Go FP/LR) pinned by the M68K arm64 ABI", r)
		}
	}
}

func TestM68KARM64ABI_PinnedRegistersUnique(t *testing.T) {
	seen := map[byte]bool{}
	for _, r := range m68kARM64PinnedRegs {
		if seen[r] {
			t.Fatalf("host register X%d pinned twice", r)
		}
		seen[r] = true
	}
}

func TestM68KARM64ABI_PinnedRegistersCalleeSaved(t *testing.T) {
	for _, r := range m68kARM64PinnedRegs {
		if r < 19 || r > 28 {
			t.Fatalf("pinned register X%d outside callee-saved X19-X28; helper calls would clobber guest state", r)
		}
	}
}

func TestM68KARM64ABI_ScratchPoolAvoidsPins(t *testing.T) {
	if m68kARM64ScratchLo > m68kARM64ScratchHi {
		t.Fatalf("empty scratch pool")
	}
	pinned := map[byte]bool{}
	for _, r := range m68kARM64PinnedRegs {
		pinned[r] = true
	}
	for r := byte(m68kARM64ScratchLo); r <= m68kARM64ScratchHi; r++ {
		if pinned[r] {
			t.Fatalf("scratch register X%d is also pinned", r)
		}
		if r == m68kARM64PlatformReg {
			t.Fatalf("scratch pool includes X18")
		}
	}
}
