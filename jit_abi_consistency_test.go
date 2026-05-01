// jit_abi_consistency_test.go - cross-check the per-backend ABI scaffold
// (Phase 7g of the six-CPU JIT unification plan).
//
// Asserts that every (backend, slot, host-reg) entry in BackendCanonicalABI
// (jit_abi_common.go) matches the per-backend ABI scaffold constants
// declared in jit_<cpu>_abi.go. If a future edit touches one without the
// other, this test fails and points the maintainer at the divergence.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func TestBackendCanonicalABI_MatchesScaffold(t *testing.T) {
	cases := []struct {
		backend string
		slot    string
		want    string
	}{
		// 6502
		{"6502", string(ABISlotAccumulator), P65ABIRegA},
		{"6502", string(ABISlotIndexX), P65ABIRegX},
		{"6502", string(ABISlotIndexY), P65ABIRegY},
		{"6502", string(ABISlotStack), P65ABIRegSP},
		{"6502", string(ABISlotPC), P65ABIRegPC},
		{"6502", string(ABISlotStatus), P65ABIRegSR},
		// M68K
		{"m68k", "D0", M68KABIRegD0},
		{"m68k", "D1", M68KABIRegD1},
		{"m68k", "A0", M68KABIRegA0},
		{"m68k", string(ABISlotStack), M68KABIRegA7},
		{"m68k", string(ABISlotStatus), M68KABIRegCCR},
		{"m68k", "DataBase", M68KABIRegDataBase},
		{"m68k", string(ABISlotMemoryBase), M68KABIRegMemBase},
		{"m68k", "AddrBase", M68KABIRegAddrBase},
		// Z80
		{"z80", string(ABISlotAccumulator), Z80ABIRegA},
		{"z80", "F", Z80ABIRegF},
		{"z80", "BC", Z80ABIRegBC},
		{"z80", "DE", Z80ABIRegDE},
		{"z80", "HL", Z80ABIRegHL},
		{"z80", string(ABISlotMemoryBase), Z80ABIRegMem},
		{"z80", "DPB", Z80ABIRegDPB},
		{"z80", "CPB", Z80ABIRegCPB},
		// x86
		{"x86", "EAX", X86ABIRegGuestEAX},
		{"x86", "ECX", X86ABIRegGuestECX},
		{"x86", "EDX", X86ABIRegGuestEDX},
		{"x86", "EBX", X86ABIRegGuestEBX},
		{"x86", "ESP", X86ABIRegGuestESP},
		{"x86", string(ABISlotMemoryBase), X86ABIRegMemBase},
		{"x86", "IOBM", X86ABIRegIOBM},
		// IE64
		{"ie64", "R1", IE64ABIRegR1},
		{"ie64", "R2", IE64ABIRegR2},
		{"ie64", "R3", IE64ABIRegR3},
		{"ie64", "R4", IE64ABIRegR4},
		{"ie64", string(ABISlotStack), IE64ABIRegR31},
		{"ie64", string(ABISlotPC), IE64ABIRegPC},
	}
	for _, c := range cases {
		got, ok := BackendCanonicalABI[c.backend][CanonicalABISlot(c.slot)]
		if !ok {
			t.Errorf("BackendCanonicalABI[%q][%q]: missing", c.backend, c.slot)
			continue
		}
		if got != c.want {
			t.Errorf("BackendCanonicalABI[%q][%q]: got %q, scaffold says %q",
				c.backend, c.slot, got, c.want)
		}
	}
}
