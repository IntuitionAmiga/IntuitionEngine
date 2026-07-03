//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// m68kAddrRegFileByteDelta is the compile-time distance from DataRegs[0] to
// AddrRegs[0] that the folded reg-file base relies on. If a struct-layout
// change (e.g. altering _padding0) moved AddrRegs, every JIT address-register
// spill would silently read/write the wrong memory. Pin it to the live struct.
func TestM68KAddrRegFileByteDelta(t *testing.T) {
	var cpu M68KCPU
	want := int32(uintptr(unsafe.Pointer(&cpu.AddrRegs[0])) - uintptr(unsafe.Pointer(&cpu.DataRegs[0])))
	if m68kAddrRegFileByteDelta != want {
		t.Fatalf("m68kAddrRegFileByteDelta=%d, struct layout says %d", m68kAddrRegFileByteDelta, want)
	}
	// The largest displacement the JIT emits (A7 slot) must stay in signed-byte
	// range so the emitter's compact [base+disp8] encoding is valid.
	if d := m68kAddrRegFileDisp(7); d < -128 || d > 127 {
		t.Fatalf("addr-reg file disp for A7 = %d escapes signed-byte encoding", d)
	}
}

// A5 and A6 are pinned in host registers R9/R8 (freed AddrBase / IOThreshold).
// These parity tests drive the interpreter (oracle) and the forced-native JIT
// from identical state and require bit-identical core state plus a data window,
// so a bug in the reg-file fold, the mapped-register sync, or the retired
// R8-scratch spill sites (NEG/NEGX/TAS mem, MOVE mem-to-mem, MOVE ea-to-mem)
// surfaces as a mismatch. Every case also asserts a native block ran, so a
// silent full fallback can't mask a codegen bug.

const m68kAregDataLo, m68kAregDataHi = uint32(0x3000), uint32(0x3140)

func runM68KAregParity(t *testing.T, name string, preset func(*M68KCPU), prog ...uint16) {
	t.Helper()
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const startPC = uint32(0x1000)

	interp := newM68KTestProgramCPU(t, startPC)
	preset(interp)
	writeM68KStopProgram(interp, startPC, prog...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.m68kJitForceNative = true
	preset(jit)
	writeM68KStopProgram(jit, startPC, prog...)
	runM68KJITUntilStopped(t, jit)

	assertM68KCoreStateEqual(t, jit, interp)
	for addr := m68kAregDataLo; addr < m68kAregDataHi; addr++ {
		if jit.memory[addr] != interp.memory[addr] {
			t.Fatalf("%s: memory[0x%04X] mismatch: jit=0x%02X interp=0x%02X",
				name, addr, jit.memory[addr], interp.memory[addr])
		}
	}
	if jit.m68kJitNativeBlocksExecuted.Load() == 0 {
		t.Fatalf("%s: no native blocks executed (change did not exercise the JIT)", name)
	}
}

// seedData writes a recognizable byte ramp into the shared data window.
func seedData(cpu *M68KCPU) {
	for i := uint32(0); m68kAregDataLo+i < m68kAregDataHi; i++ {
		cpu.memory[m68kAregDataLo+i] = byte(0x10 + i)
	}
}

// A5/A6 as EA base across the common addressing modes plus writeback.
func TestM68KJIT_A5A6_AddressingModes(t *testing.T) {
	runM68KAregParity(t, "a5a6-addressing", func(cpu *M68KCPU) {
		seedData(cpu)
		cpu.AddrRegs[5] = 0x3000
		cpu.AddrRegs[6] = 0x3100
	},
		0x2ABC, 0x1122, 0x3344, // MOVE.L #$11223344,(A5)
		0x2415,         // MOVE.L (A5),D2
		0x2C82,         // MOVE.L D2,(A6)
		0x2616,         // MOVE.L (A6),D3
		0x281D,         // MOVE.L (A5)+,D4      (A5: 3000->3004)
		0x2A26,         // MOVE.L -(A6),D5      (A6: 3104->3100)
		0x2C2D, 0x0004, // MOVE.L (4,A5),D6
		0xDBFC, 0x0000, 0x0008, // ADDA.L #8,A5
		0xDDFC, 0x0000, 0x0004, // ADDA.L #4,A6
	)
}

// TAS on a memory EA held in A5 — retired-scratch cluster 3.
func TestM68KJIT_A5A6_TAS(t *testing.T) {
	runM68KAregParity(t, "a5a6-tas", func(cpu *M68KCPU) {
		seedData(cpu)
		cpu.AddrRegs[5] = 0x3010
		cpu.AddrRegs[6] = 0x3011
	},
		0x4AD5, // TAS (A5)
		0x4AD6, // TAS (A6)
	)
}

// NEG.B / NEGX.B on memory EAs in A5/A6 — retired-scratch clusters 1 and 2.
func TestM68KJIT_A5A6_NegMem(t *testing.T) {
	runM68KAregParity(t, "a5a6-negmem", func(cpu *M68KCPU) {
		seedData(cpu)
		cpu.AddrRegs[5] = 0x3020
		cpu.AddrRegs[6] = 0x3024
	},
		0x4415, // NEG.B (A5)
		0x4016, // NEGX.B (A6)
		0x4455, // NEG.W (A5)
		0x4095, // NEGX.L (A5)
	)
}

// MOVE (A5)+,(A6)+ — retired-scratch cluster 4 (mem-to-mem postincrement).
func TestM68KJIT_A5A6_MoveMemToMemPostInc(t *testing.T) {
	runM68KAregParity(t, "a5a6-mem2mem", func(cpu *M68KCPU) {
		seedData(cpu)
		cpu.AddrRegs[5] = 0x3000
		cpu.AddrRegs[6] = 0x3100
	},
		0x2CDD, // MOVE.L (A5)+,(A6)+
		0x3CDD, // MOVE.W (A5)+,(A6)+
		0x1CDD, // MOVE.B (A5)+,(A6)+
	)
}

// MOVE Dn -> (A5)/(A6) memory dest — retired-scratch cluster 5 (ea-to-mem setCC).
func TestM68KJIT_A5A6_MoveRegToMem(t *testing.T) {
	runM68KAregParity(t, "a5a6-reg2mem", func(cpu *M68KCPU) {
		seedData(cpu)
		cpu.AddrRegs[5] = 0x3040
		cpu.AddrRegs[6] = 0x3050
		cpu.DataRegs[3] = 0x00000000 // exercises Z-flag setCC path
		cpu.DataRegs[4] = 0xFFFF8000
	},
		0x2A83, // MOVE.L D3,(A5)
		0x2C84, // MOVE.L D4,(A6)
		0x1A84, // MOVE.B D4,(A5)
		0x3C83, // MOVE.W D3,(A6)
	)
}

// MOVEM save/restore of the full register set (A5/A6 in the mask, both
// directions) round-tripped through the folded reg-file base.
func TestM68KJIT_A5A6_MovemRoundTrip(t *testing.T) {
	runM68KAregParity(t, "a5a6-movem", func(cpu *M68KCPU) {
		seedData(cpu)
		cpu.AddrRegs[5] = 0xA5A5A5A5
		cpu.AddrRegs[6] = 0xC6C6C6C6
		cpu.DataRegs[7] = 0xD7D7D7D7
		cpu.AddrRegs[3] = 0x3120 // scratch buffer for the store
	},
		0x48D3, 0x7FFE, // MOVEM.L D1-D7/A0-A6,(A3)
		0x4A85,         // TST.L D5 (filler, native)
		0x4CD3, 0x7FFE, // MOVEM.L (A3),D1-D7/A0-A6
	)
}

// A5/A6 postincrement pointers driven across a backward branch, so the loop's
// second+ iterations run from a chained block entry that must reload A5/A6 from
// the reg file (exercises the mapped-register sync on the chain edge).
func TestM68KJIT_A5A6_LoopChain(t *testing.T) {
	runM68KAregParity(t, "a5a6-loopchain", func(cpu *M68KCPU) {
		seedData(cpu)
		cpu.AddrRegs[5] = 0x3000
		cpu.AddrRegs[6] = 0x3100
	},
		0x700F, // MOVEQ #15,D0
		0x1CDD, // loop: MOVE.B (A5)+,(A6)+
		0x5380, // SUBQ.L #1,D0
		0x66FA, // BNE.B loop  (disp -6)
	)
}
