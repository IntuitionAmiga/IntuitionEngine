//go:build amd64 && linux

package main

import "testing"

// Index/absolute/PC-relative EA coverage audit for the integer JIT. Each case
// runs a single instruction whose source operand uses a brief-format index
// (d8,An,Xn), an absolute long (xxx).L, or PC-relative (d16,PC) addressing, and
// asserts JIT/interpreter parity AND that the block executed natively with zero
// fallback instructions. A regression that quietly routes one of these common
// addressing modes through the interpreter fails here loudly.
//
// The shared source-EA reader (m68kEmitReadSourceEA) already covers these
// modes; this test pins that coverage so it cannot silently regress.

func TestM68KJIT_IntegerIndexAbsCoverage(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const dataAddr = uint32(0x4000)

	// Brief index (d8,A0,D1.L*1), disp 0: EA = A0 + D1.
	const idxExt = uint16(1)<<12 | uint16(1)<<11 // idxReg=1, long

	// EA→D2 (.L) opcodes for the common ALU/MOVE families, source = (d8,A0,D1.L).
	idxOps := []struct {
		name string
		op   uint16
	}{
		{"MOVE.L", 0x2430},
		{"ADD.L", 0xD4B0},
		{"SUB.L", 0x94B0},
		{"AND.L", 0xC4B0},
		{"OR.L", 0x84B0},
		{"CMP.L", 0xB4B0},
	}
	for _, o := range idxOps {
		op := o.op
		t.Run("index_"+o.name, func(t *testing.T) {
			runM68KJITDifferentialBlock(t, m68kDiffCase{
				name:  "idx_" + o.name,
				words: []uint16{op, idxExt},
				setup: func(cpu *M68KCPU) {
					cpu.AddrRegs[0] = dataAddr
					cpu.DataRegs[1] = 0
					cpu.DataRegs[2] = 0x11111111
					cpu.Write32(dataAddr, 0x0000002A)
				},
				requireProdSafe:  true,
				requireNativeRun: true,
			}, 1)
		})
	}

	// Absolute long source: EA = (xxx).L. Same op families, src mode 7 reg 1.
	absOps := []struct {
		name string
		op   uint16
	}{
		{"MOVE.L", 0x2439},
		{"ADD.L", 0xD4B9},
		{"AND.L", 0xC4B9},
	}
	for _, o := range absOps {
		op := o.op
		t.Run("abs_"+o.name, func(t *testing.T) {
			runM68KJITDifferentialBlock(t, m68kDiffCase{
				name:  "abs_" + o.name,
				words: []uint16{op, uint16(dataAddr >> 16), uint16(dataAddr)},
				setup: func(cpu *M68KCPU) {
					cpu.DataRegs[2] = 0x22222222
					cpu.Write32(dataAddr, 0x00000055)
				},
				requireProdSafe:  true,
				requireNativeRun: true,
			}, 1)
		})
	}

	// PC-relative source (d16,PC): EA = PC-of-ext + disp. Data placed after.
	t.Run("pcrel_MOVE.L", func(t *testing.T) {
		runM68KJITDifferentialBlock(t, m68kDiffCase{
			name:  "pcrel_move",
			words: []uint16{0x243A, 0x0006}, // MOVE.L (d16,PC),D2 ; disp16=6
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[2] = 0x33333333
				// extPC = startPC+2; EA = extPC + 6 = startPC+8.
				cpu.Write32(m68kDiffStartPC+8, 0x0000007F)
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		}, 1)
	})
}
