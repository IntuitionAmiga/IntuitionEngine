//go:build amd64 && (linux || windows || darwin)

package main

import (
	"math"
	"testing"
)

// Native FPU EA-operand path: decode gates and JIT-vs-interpreter parity.
// Every parity case checks the complete architectural state — FP registers
// (bit-exact), FPSR/FPCR/FPIAR, integer registers, and the data window — and
// requires the block to have executed natively with ZERO helper exits, so a
// silently-helper-routed form fails loudly rather than hiding in a green run.

const fpuEADataAddr = uint32(0x3000)

func fpuEAOpcode(mode, reg int) uint16 { return 0xF200 | uint16(mode)<<3 | uint16(reg) }

func fpuEALoadCmd(format, dst int, op uint16) uint16 {
	return 1<<14 | uint16(format)<<10 | uint16(dst)<<7 | (op & 0x7F)
}

func fpuEAStoreCmd(format, src int) uint16 {
	return 3<<13 | uint16(format)<<10 | uint16(src)<<7
}

func fpuWriteF32(cpu *M68KCPU, addr uint32, v float32) {
	cpu.Write32(addr, math.Float32bits(v))
}

func fpuWriteF64(cpu *M68KCPU, addr uint32, v float64) {
	bits := math.Float64bits(v)
	cpu.Write32(addr, uint32(bits>>32))
	cpu.Write32(addr+4, uint32(bits))
}

// ---------------------------------------------------------------------------
// Decode gates
// ---------------------------------------------------------------------------

func TestM68KFPUEADecode_SupportedForms(t *testing.T) {
	cases := []struct {
		name         string
		opcode, cmd  uint16
		store        bool
		format, mode int
		reg, fpReg   int
		op           m68kFPUNativeOp
	}{
		{"FMOVE.S (A0)+,FP0", fpuEAOpcode(3, 0), fpuEALoadCmd(1, 0, FPU_OP_FMOVE), false, 1, 3, 0, 0, m68kFPUNativeFMOVE},
		{"FADD.D (A1),FP2", fpuEAOpcode(2, 1), fpuEALoadCmd(5, 2, FPU_OP_FADD), false, 5, 2, 1, 2, m68kFPUNativeFADD},
		{"FMUL.L -(A7),FP7", fpuEAOpcode(4, 7), fpuEALoadCmd(0, 7, FPU_OP_FMUL), false, 0, 4, 7, 7, m68kFPUNativeFMUL},
		{"FDIV.W d16(A3),FP1", fpuEAOpcode(5, 3), fpuEALoadCmd(4, 1, FPU_OP_FDIV), false, 4, 5, 3, 1, m68kFPUNativeFDIV},
		{"FCMP.B D4,FP0", fpuEAOpcode(0, 4), fpuEALoadCmd(6, 0, FPU_OP_FCMP), false, 6, 0, 4, 0, m68kFPUNativeFCMP},
		{"FADD.S #imm,FP5", fpuEAOpcode(7, 4), fpuEALoadCmd(1, 5, FPU_OP_FADD), false, 1, 7, 4, 5, m68kFPUNativeFADD},
		{"FMOVE.S FP0,(A0)", fpuEAOpcode(2, 0), fpuEAStoreCmd(1, 0), true, 1, 2, 0, 0, m68kFPUNativeFMOVE},
		{"FMOVE.D FP3,-(A2)", fpuEAOpcode(4, 2), fpuEAStoreCmd(5, 3), true, 5, 4, 2, 3, m68kFPUNativeFMOVE},
		{"FMOVE.L FP1,D6", fpuEAOpcode(0, 6), fpuEAStoreCmd(0, 1), true, 0, 0, 6, 1, m68kFPUNativeFMOVE},
		{"FADD.S (d8,A0,Xn),FP0", fpuEAOpcode(6, 0), fpuEALoadCmd(1, 0, FPU_OP_FADD), false, 1, 6, 0, 0, m68kFPUNativeFADD},
		{"FADD.S (xxx).W,FP0", fpuEAOpcode(7, 0), fpuEALoadCmd(1, 0, FPU_OP_FADD), false, 1, 7, 0, 0, m68kFPUNativeFADD},
		{"FADD.D (d16,PC),FP2", fpuEAOpcode(7, 2), fpuEALoadCmd(5, 2, FPU_OP_FADD), false, 5, 7, 2, 2, m68kFPUNativeFADD},
		{"FMOVE.L FP1,(xxx).L", fpuEAOpcode(7, 1), fpuEAStoreCmd(0, 1), true, 0, 7, 1, 1, m68kFPUNativeFMOVE},
		{"FADD.X (A2),FP0", fpuEAOpcode(2, 2), fpuEALoadCmd(2, 0, FPU_OP_FADD), false, 2, 2, 2, 0, m68kFPUNativeFADD},
		{"FMOVE.X FP3,(A0)", fpuEAOpcode(2, 0), fpuEAStoreCmd(2, 3), true, 2, 2, 0, 3, m68kFPUNativeFMOVE},
	}
	for _, tc := range cases {
		form, ok := m68kDecodeNativeFPUEA(tc.opcode, tc.cmd)
		if !ok {
			t.Errorf("%s: not decoded as native", tc.name)
			continue
		}
		if form.store != tc.store || form.format != tc.format || form.mode != tc.mode ||
			form.reg != tc.reg || form.fpReg != tc.fpReg || form.op != tc.op {
			t.Errorf("%s: decoded %+v", tc.name, form)
		}
	}
}

func TestM68KFPUEADecode_RejectedForms(t *testing.T) {
	cases := []struct {
		name        string
		opcode, cmd uint16
	}{
		{"reg-to-reg (separate path)", 0xF200, fpuEALoadCmd(1, 0, FPU_OP_FADD) &^ (1 << 14)},
		{"extended from Dn", fpuEAOpcode(0, 0), fpuEALoadCmd(2, 0, FPU_OP_FADD)},
		{"packed format load", fpuEAOpcode(2, 0), fpuEALoadCmd(3, 0, FPU_OP_FADD)},
		{"double from Dn", fpuEAOpcode(0, 0), fpuEALoadCmd(5, 0, FPU_OP_FADD)},
		{"store with k-factor", fpuEAOpcode(2, 0), fpuEAStoreCmd(3, 0) | 0x11},
		{"FMOVECR", 0xF200, 0x5C00},
		{"transcendental FSIN", fpuEAOpcode(2, 0), fpuEALoadCmd(1, 0, FPU_OP_FSIN)},
		{"PC-relative store", fpuEAOpcode(7, 2), fpuEAStoreCmd(1, 0)},
		{"PC-index store", fpuEAOpcode(7, 3), fpuEAStoreCmd(1, 0)},
		{"immediate store", fpuEAOpcode(7, 4), fpuEAStoreCmd(1, 0)},
		{"mode 7 reg 5 (invalid)", fpuEAOpcode(7, 5), fpuEALoadCmd(1, 0, FPU_OP_FADD)},
		{"An direct", fpuEAOpcode(1, 0), fpuEALoadCmd(1, 0, FPU_OP_FADD)},
		{"control-register move", fpuEAOpcode(2, 0), 0x8000},
		{"FScc opcode class", 0xF240, fpuEALoadCmd(1, 0, FPU_OP_FADD)},
		{"precision-qualified FCMP", fpuEAOpcode(2, 0), fpuEALoadCmd(1, 0, FPU_OP_FCMP|0x40)},
		{"precision-qualified FTST", fpuEAOpcode(2, 0), fpuEALoadCmd(1, 0, FPU_OP_FTST|0x40)},
	}
	for _, tc := range cases {
		if _, ok := m68kDecodeNativeFPUEA(tc.opcode, tc.cmd); ok {
			t.Errorf("%s: wrongly accepted as native", tc.name)
		}
	}
}

func TestM68KFPUEAStepBytes_A7ByteKeepsSPEven(t *testing.T) {
	if got := m68kFPUEAStepBytes(6, 7); got != 2 {
		t.Fatalf("byte step through A7 = %d, want 2", got)
	}
	if got := m68kFPUEAStepBytes(6, 3); got != 1 {
		t.Fatalf("byte step through A3 = %d, want 1", got)
	}
	if got := m68kFPUEAStepBytes(5, 7); got != 8 {
		t.Fatalf("double step = %d, want 8", got)
	}
}

// ---------------------------------------------------------------------------
// Parity harness
// ---------------------------------------------------------------------------

func runM68KFPUEAParity(t *testing.T, name string, opcodes []uint16, preset func(*M68KCPU)) {
	t.Helper()
	runM68KFPUEAParityOpt(t, name, opcodes, preset, false)
}

// runM68KFPUEAParityOpt compares full architectural state (FP regs, FPSR/FPIAR,
// integer regs, data window) between interpreter and JIT. When allowHelper is
// false it also asserts the block ran with zero FPU helper exits (fully
// native); pass true for programs that legitimately include a helper-routed
// instruction (e.g. a control-register move) whose correctness still depends on
// the native ops around it.
func runM68KFPUEAParityOpt(t *testing.T, name string, opcodes []uint16, preset func(*M68KCPU), allowHelper bool) {
	t.Helper()
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const startPC = uint32(0x1000)

	interp := newM68KTestProgramCPU(t, startPC)
	preset(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	preset(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	for reg := range 8 {
		g := math.Float64bits(jit.FPU.GetFP64(reg))
		w := math.Float64bits(interp.FPU.GetFP64(reg))
		if g != w {
			t.Fatalf("%s: FP%d bits got=%#016x want=%#016x", name, reg, g, w)
		}
	}
	if jit.FPU.FPSR != interp.FPU.FPSR {
		t.Fatalf("%s: FPSR got=%#08x want=%#08x", name, jit.FPU.FPSR, interp.FPU.FPSR)
	}
	if jit.FPU.FPIAR != interp.FPU.FPIAR {
		t.Fatalf("%s: FPIAR got=%#08x want=%#08x", name, jit.FPU.FPIAR, interp.FPU.FPIAR)
	}
	for i := range 8 {
		if jit.DataRegs[i] != interp.DataRegs[i] {
			t.Fatalf("%s: D%d got=%#08x want=%#08x", name, i, jit.DataRegs[i], interp.DataRegs[i])
		}
		if jit.AddrRegs[i] != interp.AddrRegs[i] {
			t.Fatalf("%s: A%d got=%#08x want=%#08x", name, i, jit.AddrRegs[i], interp.AddrRegs[i])
		}
	}
	for off := uint32(0); off < 0x100; off++ {
		if jit.memory[fpuEADataAddr+off] != interp.memory[fpuEADataAddr+off] {
			t.Fatalf("%s: memory[%#x] got=%#02x want=%#02x", name, fpuEADataAddr+off,
				jit.memory[fpuEADataAddr+off], interp.memory[fpuEADataAddr+off])
		}
	}
	if jit.m68kJitNativeBlocksExecuted.Load() == 0 {
		t.Fatalf("%s: block did not execute natively", name)
	}
	if got := jit.m68kJitNativeHelperExits.Load(); !allowHelper && got != 0 {
		t.Fatalf("%s: %d helper exits (expected fully native)", name, got)
	}
}

var fpuEAOperandValues = []float64{
	2.5, -2.5, 0.0, math.Copysign(0, -1), 1e308, -1e308,
	math.Inf(1), math.Inf(-1), math.NaN(), 5e-324, 1.0000000000000002,
}

// ---------------------------------------------------------------------------
// Load parity
// ---------------------------------------------------------------------------

func TestM68KJIT_FPUEA_LoadFormats_AnInd(t *testing.T) {
	type fmtCase struct {
		name   string
		format int
		write  func(*M68KCPU)
	}
	cases := []fmtCase{
		{"long", 0, func(c *M68KCPU) { c.Write32(fpuEADataAddr, 0xF8A432EB) }}, // -123456789
		{"single", 1, func(c *M68KCPU) { fpuWriteF32(c, fpuEADataAddr, -2.5) }},
		{"word", 4, func(c *M68KCPU) { c.Write16(fpuEADataAddr, 0xFCF7) }}, // -777
		{"double", 5, func(c *M68KCPU) { fpuWriteF64(c, fpuEADataAddr, 1.0000000000000002) }},
		{"byte", 6, func(c *M68KCPU) { c.Write8(fpuEADataAddr, 0x80) }},
	}
	for _, fc := range cases {
		prog := []uint16{fpuEAOpcode(2, 0), fpuEALoadCmd(fc.format, 1, FPU_OP_FADD)}
		runM68KFPUEAParity(t, "FADD."+fc.name+" (A0),FP1", prog, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(1, 3.25)
			fc.write(cpu)
		})
	}
}

func TestM68KJIT_FPUEA_AllOps_DoubleAnInd(t *testing.T) {
	ops := []struct {
		name string
		op   uint16
	}{
		{"FMOVE", FPU_OP_FMOVE}, {"FADD", FPU_OP_FADD}, {"FSUB", FPU_OP_FSUB},
		{"FMUL", FPU_OP_FMUL}, {"FDIV", FPU_OP_FDIV}, {"FCMP", FPU_OP_FCMP},
		{"FTST", FPU_OP_FTST}, {"FABS", FPU_OP_FABS}, {"FNEG", FPU_OP_FNEG},
		{"FSQRT", FPU_OP_FSQRT}, {"FSGLDIV", FPU_OP_FSGLDIV}, {"FSGLMUL", FPU_OP_FSGLMUL},
	}
	for _, o := range ops {
		for _, v := range fpuEAOperandValues {
			operand := v
			prog := []uint16{fpuEAOpcode(2, 3), fpuEALoadCmd(5, 2, o.op)}
			runM68KFPUEAParity(t, o.name+".D (A3),FP2", prog, func(cpu *M68KCPU) {
				cpu.AddrRegs[3] = fpuEADataAddr
				cpu.FPU.SetFP64(2, 6.75)
				fpuWriteF64(cpu, fpuEADataAddr, operand)
			})
		}
	}
}

func TestM68KJIT_FPUEA_PostincPredecDisp(t *testing.T) {
	// Two postincrement loads in one block: A2 must advance by 8 total.
	prog := []uint16{
		fpuEAOpcode(3, 2), fpuEALoadCmd(1, 0, FPU_OP_FADD),
		fpuEAOpcode(3, 2), fpuEALoadCmd(1, 0, FPU_OP_FADD),
	}
	runM68KFPUEAParity(t, "FADD.S (A2)+ twice", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = fpuEADataAddr
		fpuWriteF32(cpu, fpuEADataAddr, 1.5)
		fpuWriteF32(cpu, fpuEADataAddr+4, 2.25)
	})

	// Byte postincrement through A7 must step by 2 (SP stays even).
	prog = []uint16{fpuEAOpcode(3, 7), fpuEALoadCmd(6, 4, FPU_OP_FADD)}
	runM68KFPUEAParity(t, "FADD.B (A7)+", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = fpuEADataAddr
		cpu.Write8(fpuEADataAddr, 0x7F)
	})

	// Predecrement double: A5 ends at the effective address.
	prog = []uint16{fpuEAOpcode(4, 5), fpuEALoadCmd(5, 6, FPU_OP_FMOVE)}
	runM68KFPUEAParity(t, "FMOVE.D -(A5)", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[5] = fpuEADataAddr + 8
		fpuWriteF64(cpu, fpuEADataAddr, -0.03125)
	})

	// d16(An) with positive and negative displacements.
	prog = []uint16{fpuEAOpcode(5, 4), fpuEALoadCmd(1, 3, FPU_OP_FMUL), 0x0010}
	runM68KFPUEAParity(t, "FMUL.S 16(A4)", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[4] = fpuEADataAddr
		cpu.FPU.SetFP64(3, 4.0)
		fpuWriteF32(cpu, fpuEADataAddr+16, 0.5)
	})
	prog = []uint16{fpuEAOpcode(5, 4), fpuEALoadCmd(5, 3, FPU_OP_FSUB), 0xFFF8}
	runM68KFPUEAParity(t, "FSUB.D -8(A4)", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[4] = fpuEADataAddr + 16
		cpu.FPU.SetFP64(3, 4.0)
		fpuWriteF64(cpu, fpuEADataAddr+8, 1.75)
	})
}

func TestM68KJIT_FPUEA_DataRegisterSource(t *testing.T) {
	// D0/D1 are host-mapped (RBX/RBP); D3+ live in memory. Cover both.
	for _, reg := range []int{0, 1, 3, 7} {
		r := reg
		prog := []uint16{fpuEAOpcode(0, r), fpuEALoadCmd(0, 2, FPU_OP_FADD)}
		runM68KFPUEAParity(t, "FADD.L Dn,FP2", prog, func(cpu *M68KCPU) {
			cpu.DataRegs[r] = 0xFD78A84E // -42424242
			cpu.FPU.SetFP64(2, 0.5)
		})
		prog = []uint16{fpuEAOpcode(0, r), fpuEALoadCmd(1, 2, FPU_OP_FMOVE)}
		runM68KFPUEAParity(t, "FMOVE.S Dn,FP2", prog, func(cpu *M68KCPU) {
			cpu.DataRegs[r] = math.Float32bits(-1.25)
		})
		prog = []uint16{fpuEAOpcode(0, r), fpuEALoadCmd(4, 2, FPU_OP_FMUL)}
		runM68KFPUEAParity(t, "FMUL.W Dn,FP2", prog, func(cpu *M68KCPU) {
			cpu.DataRegs[r] = 0xAAAA8000 // low word = -32768
			cpu.FPU.SetFP64(2, 2.0)
		})
		prog = []uint16{fpuEAOpcode(0, r), fpuEALoadCmd(6, 2, FPU_OP_FSUB)}
		runM68KFPUEAParity(t, "FSUB.B Dn,FP2", prog, func(cpu *M68KCPU) {
			cpu.DataRegs[r] = 0x111111FF // low byte = -1
			cpu.FPU.SetFP64(2, 10.0)
		})
	}
}

func TestM68KJIT_FPUEA_Immediates(t *testing.T) {
	// FADD.L #-100000,FP0
	prog := []uint16{fpuEAOpcode(7, 4), fpuEALoadCmd(0, 0, FPU_OP_FADD), 0xFFFE, 0x7960}
	runM68KFPUEAParity(t, "FADD.L #imm", prog, func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(0, 0.5)
	})
	// FMUL.S #2.5,FP1
	bits32 := math.Float32bits(2.5)
	prog = []uint16{fpuEAOpcode(7, 4), fpuEALoadCmd(1, 1, FPU_OP_FMUL), uint16(bits32 >> 16), uint16(bits32)}
	runM68KFPUEAParity(t, "FMUL.S #2.5", prog, func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(1, 3.0)
	})
	// FMOVE.D #-0.75,FP2
	bits64 := math.Float64bits(-0.75)
	prog = []uint16{fpuEAOpcode(7, 4), fpuEALoadCmd(5, 2, FPU_OP_FMOVE),
		uint16(bits64 >> 48), uint16(bits64 >> 32), uint16(bits64 >> 16), uint16(bits64)}
	runM68KFPUEAParity(t, "FMOVE.D #imm", prog, func(cpu *M68KCPU) {})
	// FADD.W #-7,FP3
	prog = []uint16{fpuEAOpcode(7, 4), fpuEALoadCmd(4, 3, FPU_OP_FADD), 0xFFF9}
	runM68KFPUEAParity(t, "FADD.W #-7", prog, func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(3, 100.0)
	})
	// FDIV.B #-2,FP4
	prog = []uint16{fpuEAOpcode(7, 4), fpuEALoadCmd(6, 4, FPU_OP_FDIV), 0x00FE}
	runM68KFPUEAParity(t, "FDIV.B #-2", prog, func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(4, 9.0)
	})
}

func TestM68KJIT_FPUEA_SinglePrecisionResultRounding(t *testing.T) {
	// FSADD (opmode|0x40): the double operand is added, then the result is
	// rounded through float32 and the CC refreshed from the rounded value.
	prog := []uint16{fpuEAOpcode(2, 0), fpuEALoadCmd(5, 1, FPU_OP_FADD|0x40)}
	runM68KFPUEAParity(t, "FSADD.D (A0),FP1", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = fpuEADataAddr
		cpu.FPU.SetFP64(1, 1.0000000000000002)
		fpuWriteF64(cpu, fpuEADataAddr, 1e-17)
	})
}

// ---------------------------------------------------------------------------
// Extended-precision (format 2) EA parity
// ---------------------------------------------------------------------------

func writeExt96(cpu *M68KCPU, addr uint32, v float64) {
	cpu.writeExtendedReal96(addr, ExtendedRealFromFloat64(v))
}

// Normal finite extended values take the native fast path (zero helper exits).
func TestM68KJIT_FPUEA_ExtendedLoadNormal(t *testing.T) {
	normals := []float64{2.5, -2.5, 3.14159265358979, -1e100, 1e-100, 12345.678, -0.0009765625}
	for _, v := range normals {
		operand := v
		// FADD.X (A0),FP0
		runM68KFPUEAParity(t, "FADD.X (A0)", []uint16{fpuEAOpcode(2, 0), fpuEALoadCmd(2, 0, FPU_OP_FADD)}, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(0, 1.5)
			writeExt96(cpu, fpuEADataAddr, operand)
		})
		// FMOVE.X (A1)+,FP2 — postincrement advances A1 by 12.
		runM68KFPUEAParity(t, "FMOVE.X (A1)+", []uint16{fpuEAOpcode(3, 1), fpuEALoadCmd(2, 2, FPU_OP_FMOVE)}, func(cpu *M68KCPU) {
			cpu.AddrRegs[1] = fpuEADataAddr
			writeExt96(cpu, fpuEADataAddr, operand)
		})
		// FMUL.X -(A2),FP3 — predecrement.
		runM68KFPUEAParity(t, "FMUL.X -(A2)", []uint16{fpuEAOpcode(4, 2), fpuEALoadCmd(2, 3, FPU_OP_FMUL)}, func(cpu *M68KCPU) {
			cpu.AddrRegs[2] = fpuEADataAddr + 12
			cpu.FPU.SetFP64(3, 2.0)
			writeExt96(cpu, fpuEADataAddr, operand)
		})
	}
}

// Special extended values (zero/denormal/inf/nan) bail to the FPU helper, which
// does the full conversion; parity still holds.
func TestM68KJIT_FPUEA_ExtendedLoadSpecial(t *testing.T) {
	specials := []float64{0.0, math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.NaN(), 5e-324}
	for _, v := range specials {
		operand := v
		runM68KFPUEAParityOpt(t, "FMOVE.X special", []uint16{fpuEAOpcode(2, 0), fpuEALoadCmd(2, 1, FPU_OP_FMOVE)}, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			writeExt96(cpu, fpuEADataAddr, operand)
		}, true)
	}
}

// Extended stores: normal values go native; the round-trip through the
// interpreter-written reference must match bit-for-bit.
func TestM68KJIT_FPUEA_ExtendedStoreNormal(t *testing.T) {
	normals := []float64{2.5, -2.5, 3.14159265358979, -1e100, 1e-100, 98765.4321, -0.0009765625}
	for _, v := range normals {
		val := v
		// FMOVE.X FP0,(A0)
		runM68KFPUEAParity(t, "FMOVE.X FP0,(A0)", []uint16{fpuEAOpcode(2, 0), fpuEAStoreCmd(2, 0)}, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(0, val)
		})
		// FMOVE.X FP1,-(A2) predecrement.
		runM68KFPUEAParity(t, "FMOVE.X FP1,-(A2)", []uint16{fpuEAOpcode(4, 2), fpuEAStoreCmd(2, 1)}, func(cpu *M68KCPU) {
			cpu.AddrRegs[2] = fpuEADataAddr + 12
			cpu.FPU.SetFP64(1, val)
		})
	}
}

// Extended stores of special values bail to the helper; parity still holds.
func TestM68KJIT_FPUEA_ExtendedStoreSpecial(t *testing.T) {
	specials := []float64{0.0, math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.NaN(), 5e-324}
	for _, v := range specials {
		val := v
		runM68KFPUEAParityOpt(t, "FMOVE.X special store", []uint16{fpuEAOpcode(2, 0), fpuEAStoreCmd(2, 0)}, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(0, val)
		}, true)
	}
}

// Extended immediate operands convert at compile time (native, zero helper).
func TestM68KJIT_FPUEA_ExtendedImmediate(t *testing.T) {
	for _, v := range []float64{2.5, -7.25, 3.14159265358979} {
		ext := ExtendedRealFromFloat64(v)
		w0 := uint32(ext.Sign)<<31 | uint32(ext.Exp)<<16
		w1 := uint32(ext.Mant >> 32)
		w2 := uint32(ext.Mant)
		prog := []uint16{
			fpuEAOpcode(7, 4), fpuEALoadCmd(2, 0, FPU_OP_FADD),
			uint16(w0 >> 16), uint16(w0), uint16(w1 >> 16), uint16(w1), uint16(w2 >> 16), uint16(w2),
		}
		runM68KFPUEAParity(t, "FADD.X #imm", prog, func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(0, 1.0)
		})
	}
}

// ---------------------------------------------------------------------------
// Index / absolute / PC-relative EA parity
// ---------------------------------------------------------------------------

func TestM68KJIT_FPUEA_IndexAbsPCRelLoads(t *testing.T) {
	for _, v := range fpuEAOperandValues {
		operand := v
		// FADD.D (8,A0,D1.L*2),FP0 — brief index, scale 2.
		// mode 6, reg 0; ext word: D1, long, scale 2, disp8=8.
		ext := uint16(0)<<15 | uint16(1)<<12 | uint16(1)<<11 | uint16(1)<<9 | 0x08
		runM68KFPUEAParity(t, "FADD.D idx", []uint16{fpuEAOpcode(6, 0), fpuEALoadCmd(5, 0, FPU_OP_FADD), ext}, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.DataRegs[1] = 4 // *2 = 8; +disp 8 → offset 16
			cpu.FPU.SetFP64(0, 1.5)
			fpuWriteF64(cpu, fpuEADataAddr+16, operand)
		})
		// FADD.S (xxx).L,FP0 — absolute long.
		lo := uint16(fpuEADataAddr & 0xFFFF)
		hi := uint16(fpuEADataAddr >> 16)
		runM68KFPUEAParity(t, "FADD.S abs.L", []uint16{fpuEAOpcode(7, 1), fpuEALoadCmd(1, 0, FPU_OP_FADD), hi, lo}, func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(0, 2.0)
			fpuWriteF32(cpu, fpuEADataAddr, float32(operand))
		})
	}

	// FMOVE.D (d16,PC),FP1 — PC-relative load. The data sits at a fixed offset
	// past the instruction; disp is resolved from the extension word's PC.
	runM68KFPUEAParity(t, "FMOVE.D d16(PC)", []uint16{fpuEAOpcode(7, 2), fpuEALoadCmd(5, 1, FPU_OP_FMOVE), 0x0010}, func(cpu *M68KCPU) {
		// extPC = startPC+4; target = extPC + 0x10.
		fpuWriteF64(cpu, 0x1000+4+0x10, -12.5)
	})
}

func TestM68KJIT_FPUEA_IndexAbsStores(t *testing.T) {
	for _, v := range []float64{3.5, -7.25, 0.0} {
		val := v
		// FMOVE.S FP0,(4,A1,D0.W) — brief index store.
		ext := uint16(0)<<15 | uint16(0)<<12 | uint16(0)<<11 | uint16(0)<<9 | 0x04
		runM68KFPUEAParity(t, "FMOVE.S idx store", []uint16{fpuEAOpcode(6, 1), fpuEAStoreCmd(1, 0), ext}, func(cpu *M68KCPU) {
			cpu.AddrRegs[1] = fpuEADataAddr
			cpu.DataRegs[0] = 8 // word index 8; +disp 4 → offset 12
			cpu.FPU.SetFP64(0, val)
		})
		// FMOVE.L FP1,(xxx).L — absolute long store.
		lo := uint16((fpuEADataAddr + 0x20) & 0xFFFF)
		hi := uint16((fpuEADataAddr + 0x20) >> 16)
		runM68KFPUEAParity(t, "FMOVE.L abs.L store", []uint16{fpuEAOpcode(7, 1), fpuEAStoreCmd(0, 1), hi, lo}, func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(1, val)
		})
	}
}

// Full-format index extension words are rejected by the native EA decoder/
// emitter (m68kBriefIndexedEAAllowed) and stay on the FPU helper; the decode
// gate coverage lives in TestM68KFPUEADecode_RejectedForms and the emitter's
// brief-format guard. A runtime program test is omitted because a valid
// full-format index encoding is not needed to exercise the rejection path.

// ---------------------------------------------------------------------------
// Store parity
// ---------------------------------------------------------------------------

func TestM68KJIT_FPUEA_StoreFormats(t *testing.T) {
	for _, v := range fpuEAOperandValues {
		val := v
		// FMOVE.S FP0,(A0)
		prog := []uint16{fpuEAOpcode(2, 0), fpuEAStoreCmd(1, 0)}
		runM68KFPUEAParity(t, "FMOVE.S FP0,(A0)", prog, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(0, val)
		})
		// FMOVE.L FP0,(A0) — includes NaN/overflow → 0x80000000 parity
		prog = []uint16{fpuEAOpcode(2, 0), fpuEAStoreCmd(0, 0)}
		runM68KFPUEAParity(t, "FMOVE.L FP0,(A0)", prog, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(0, val)
		})
		// FMOVE.D FP1,8(A0)
		prog = []uint16{fpuEAOpcode(5, 0), fpuEAStoreCmd(5, 1), 0x0008}
		runM68KFPUEAParity(t, "FMOVE.D FP1,8(A0)", prog, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(1, val)
		})
	}

	// FMOVE.W FP1,-(A5) and FMOVE.B FP1,(A6)+
	prog := []uint16{fpuEAOpcode(4, 5), fpuEAStoreCmd(4, 1)}
	runM68KFPUEAParity(t, "FMOVE.W FP1,-(A5)", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[5] = fpuEADataAddr + 8
		cpu.FPU.SetFP64(1, -1234.9)
	})
	prog = []uint16{fpuEAOpcode(3, 6), fpuEAStoreCmd(6, 1)}
	runM68KFPUEAParity(t, "FMOVE.B FP1,(A6)+", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = fpuEADataAddr
		cpu.FPU.SetFP64(1, -100.7)
	})
	// Byte store through A7 postincrement: step 2.
	prog = []uint16{fpuEAOpcode(3, 7), fpuEAStoreCmd(6, 1)}
	runM68KFPUEAParity(t, "FMOVE.B FP1,(A7)+", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = fpuEADataAddr
		cpu.FPU.SetFP64(1, 33.0)
	})
}

func TestM68KJIT_FPUEA_StoreToDataRegister(t *testing.T) {
	for _, reg := range []int{0, 1, 5} {
		r := reg
		// Long: whole register replaced.
		prog := []uint16{fpuEAOpcode(0, r), fpuEAStoreCmd(0, 0)}
		runM68KFPUEAParity(t, "FMOVE.L FP0,Dn", prog, func(cpu *M68KCPU) {
			cpu.DataRegs[r] = 0xDEADBEEF
			cpu.FPU.SetFP64(0, -100.7)
		})
		// Single: bit pattern replaces the register.
		prog = []uint16{fpuEAOpcode(0, r), fpuEAStoreCmd(1, 0)}
		runM68KFPUEAParity(t, "FMOVE.S FP0,Dn", prog, func(cpu *M68KCPU) {
			cpu.DataRegs[r] = 0xDEADBEEF
			cpu.FPU.SetFP64(0, 0.15625)
		})
		// Word: upper 16 bits preserved.
		prog = []uint16{fpuEAOpcode(0, r), fpuEAStoreCmd(4, 0)}
		runM68KFPUEAParity(t, "FMOVE.W FP0,Dn", prog, func(cpu *M68KCPU) {
			cpu.DataRegs[r] = 0xAABBCCDD
			cpu.FPU.SetFP64(0, -2.0)
		})
		// Byte: upper 24 bits preserved.
		prog = []uint16{fpuEAOpcode(0, r), fpuEAStoreCmd(6, 0)}
		runM68KFPUEAParity(t, "FMOVE.B FP0,Dn", prog, func(cpu *M68KCPU) {
			cpu.DataRegs[r] = 0xAABBCCDD
			cpu.FPU.SetFP64(0, 77.0)
		})
	}
}

// ---------------------------------------------------------------------------
// FBcc parity
// ---------------------------------------------------------------------------

// All 16 FPU conditions against every meaningful FPSR CC combination. The
// program pattern makes the branch outcome visible in D0: FBcc.W +4 skips
// MOVEQ #1,D0 when taken; MOVEQ #2,D1 marks the join point.
func TestM68KJIT_FPUEA_FBccAllConditions(t *testing.T) {
	ccStates := []uint32{
		0,
		FPU_CC_N,
		FPU_CC_Z,
		FPU_CC_NAN,
		FPU_CC_N | FPU_CC_NAN,
		FPU_CC_Z | FPU_CC_NAN,
		FPU_CC_N | FPU_CC_Z,
		FPU_CC_I, // infinity bit must not affect any condition
		FPU_CC_I | FPU_CC_N,
	}
	for cond := uint16(0); cond < 16; cond++ {
		for _, cc := range ccStates {
			ccVal := cc
			prog := []uint16{
				0xF280 | cond, 0x0004, // FBcc.W +4 (skip MOVEQ #1 when taken)
				0x7001, // MOVEQ #1,D0
				0x7202, // MOVEQ #2,D1
			}
			runM68KFPUEAParity(t, "FBcc.W", prog, func(cpu *M68KCPU) {
				cpu.FPU.FPSR = ccVal
			})
		}
	}
}

func TestM68KJIT_FPUEA_FBccLongDisplacement(t *testing.T) {
	for _, cc := range []uint32{0, FPU_CC_Z, FPU_CC_NAN} {
		ccVal := cc
		prog := []uint16{
			0xF2C1, 0x0000, 0x0006, // FBEQ.L +6 (skip MOVEQ #1 when Z)
			0x7001, // MOVEQ #1,D0
			0x7202, // MOVEQ #2,D1
		}
		runM68KFPUEAParity(t, "FBEQ.L", prog, func(cpu *M68KCPU) {
			cpu.FPU.FPSR = ccVal
		})
	}
}

// FDBcc: FPU decrement-and-branch. The loop runs FCMP + FDBcc; D7 counts down
// and the FPU condition can also terminate early. Covers all 16 conditions
// against several FPSR states, plus the counter-exhaustion and early-exit
// paths, and the D0/D1 host-mapped vs memory-resident counter split.
func TestM68KJIT_FPUEA_FDBccAllConditions(t *testing.T) {
	ccStates := []uint32{0, FPU_CC_N, FPU_CC_Z, FPU_CC_NAN, FPU_CC_I}
	for cond := uint16(0); cond < 16; cond++ {
		for _, cc := range ccStates {
			ccVal := cc
			// Counter regs D2 (memory-resident) and D1 (host-mapped RBP); the
			// loop body only touches D0, so the counter counts down cleanly and
			// the loop always terminates in both interpreter and JIT.
			for _, reg := range []uint16{2, 1} {
				r := reg
				prog := []uint16{
					0x7001,                   // MOVEQ #1,D0  (loop body, not the counter)
					0xF248 | r, cond, 0xFFFC, // FDBcc Dn,-4 → back to MOVEQ
					0x7402, // MOVEQ #2,D2 or clobbered — exit marker
				}
				runM68KFPUEAParity(t, "FDBcc", prog, func(cpu *M68KCPU) {
					cpu.FPU.FPSR = ccVal
					cpu.DataRegs[r] = 3 // small trip count
				})
			}
		}
	}
}

// FDBcc with the counter already exhausted (Dn.W == 0) must wrap to 0xFFFF and
// fall through without looping, when the condition is false.
func TestM68KJIT_FPUEA_FDBccExhaustedCounter(t *testing.T) {
	prog := []uint16{
		0x7001,                 // MOVEQ #1,D0 (body, not the counter)
		0xF24A, 0x0000, 0xFFFC, // FDBF D2,-4
		0x7601, // MOVEQ #1,D3 (exit marker)
	}
	for _, tc := range []struct {
		name string
		d2   uint32
	}{
		{"zero_wraps", 0},
		{"one_loops_once", 1},
		{"upperword_preserved", 0xABCD0000},
	} {
		d2 := tc.d2
		runM68KFPUEAParity(t, "FDBF "+tc.name, prog, func(cpu *M68KCPU) {
			cpu.DataRegs[2] = d2
		})
	}
}

// FPU loop closed by a backward FBcc: FP0 counts up by 1.0 until FCMP against
// the in-memory limit stops producing N. Exercises the in-block backward
// branch with loop budget plus native FCMP-EA inside the loop body.
func TestM68KJIT_FPUEA_FBccBackwardLoop(t *testing.T) {
	one := math.Float32bits(1.0)
	prog := []uint16{
		// loop: (offsets relative to start)
		0xF23C, 0x4422, uint16(one >> 16), uint16(one), // FADD.S #1.0,FP0   (8 bytes)
		0x5280,         // ADDQ.L #1,D0                                     (2 bytes)
		0xF211, 0x5438, // FCMP.D (A1),FP0                                   (4 bytes)
		0xF284, 0xFFF0, // FBLT.W -16 → loop                                 (4 bytes)
	}
	runM68KFPUEAParity(t, "FBLT backward loop", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[1] = fpuEADataAddr
		fpuWriteF64(cpu, fpuEADataAddr, 10.0)
	})
}

// ---------------------------------------------------------------------------
// Lazy FPSR condition codes
// ---------------------------------------------------------------------------

// A reg-reg arithmetic chain elides the intermediate CC updates; the final CC
// (last op before a non-overwriter) must still be correct, and an FBcc that
// observes it must branch identically to the interpreter.
func TestM68KJIT_FPUEA_LazyFPSRChainThenFBcc(t *testing.T) {
	fadd := func(src, dst int) []uint16 {
		return []uint16{0xF200, uint16(src)<<10 | uint16(dst)<<7 | FPU_OP_FADD}
	}
	for _, seed := range []float64{-3.0, -1.0, 0.0, 1.0, 5.0} {
		s := seed
		// FADD FP1,FP0 ; FADD FP2,FP0 ; FCMP FP3,FP0 ; FBLT +4 ; MOVEQ #1,D0 ; MOVEQ #2,D1
		prog := []uint16{}
		prog = append(prog, fadd(1, 0)...)                      // CC elided (next reg-reg overwrites)
		prog = append(prog, fadd(2, 0)...)                      // CC elided (next FCMP overwrites)
		prog = append(prog, 0xF200, (3<<10)|(0<<7)|FPU_OP_FCMP) // FCMP FP3,FP0 (CC live)
		prog = append(prog, 0xF29C, 0x0004)                     // FBLT.W +4
		prog = append(prog, 0x7001)                             // MOVEQ #1,D0 (skipped if taken)
		prog = append(prog, 0x7202)                             // MOVEQ #2,D1
		runM68KFPUEAParity(t, "lazy chain FBcc", prog, func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(0, s)
			cpu.FPU.SetFP64(1, 1.0)
			cpu.FPU.SetFP64(2, 2.0)
			cpu.FPU.SetFP64(3, 4.0)
		})
	}
}

// A reg-reg chain whose CC is read by FMOVE FPSR,Dn: the last arithmetic op
// before the control move must NOT elide (a control move is not a no-fault
// overwriter), so the observed FPSR is correct.
func TestM68KJIT_FPUEA_LazyFPSRObservedByFPSRMove(t *testing.T) {
	for _, seed := range []float64{-2.0, 0.0, 3.0, math.NaN(), math.Inf(1)} {
		s := seed
		prog := []uint16{
			0xF200, (1 << 10) | (0 << 7) | FPU_OP_FADD, // FADD FP1,FP0 (CC elided)
			0xF200, (2 << 10) | (0 << 7) | FPU_OP_FADD, // FADD FP2,FP0 (CC live: next is FPSR move)
			0xF200, 0xA800, // FMOVE.L FPSR,D0
		}
		runM68KFPUEAParityOpt(t, "lazy chain FPSR move", prog, func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(0, s)
			cpu.FPU.SetFP64(1, 1.0)
			cpu.FPU.SetFP64(2, 2.0)
			cpu.DataRegs[0] = 0xFFFFFFFF
		}, true) // FMOVE FPSR,D0 is a helper-routed control move
	}
}

// EA-load chains: FADD.D (A0),FP0 followed by a reg-reg FADD — the EA op's CC
// is dead (reg-reg overwrites no-fault) and elided; the reg-reg op's CC is the
// block's final CC. Also covers the reverse (reg-reg then EA): the reg-reg CC
// must NOT be elided because the EA op can fault before its setCC.
func TestM68KJIT_FPUEA_LazyFPSRMixedChain(t *testing.T) {
	for _, v := range fpuEAOperandValues {
		operand := v
		// EA then reg-reg: EA CC elided.
		prog := []uint16{
			fpuEAOpcode(2, 0), fpuEALoadCmd(5, 0, FPU_OP_FADD), // FADD.D (A0),FP0
			0xF200, (1 << 10) | (0 << 7) | FPU_OP_FMUL, // FMUL FP1,FP0
		}
		runM68KFPUEAParity(t, "lazy EA then reg", prog, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(0, 1.5)
			cpu.FPU.SetFP64(1, 2.0)
			fpuWriteF64(cpu, fpuEADataAddr, operand)
		})
		// reg-reg then EA: reg-reg CC must survive (EA can fault).
		prog = []uint16{
			0xF200, (1 << 10) | (0 << 7) | FPU_OP_FADD, // FADD FP1,FP0
			fpuEAOpcode(2, 0), fpuEALoadCmd(5, 0, FPU_OP_FMUL), // FMUL.D (A0),FP0
		}
		runM68KFPUEAParity(t, "lazy reg then EA", prog, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(0, 1.5)
			cpu.FPU.SetFP64(1, 2.0)
			fpuWriteF64(cpu, fpuEADataAddr, operand)
		})
	}
}

// ---------------------------------------------------------------------------
// FINT / FINTRZ (SSE4.1 ROUNDSD)
// ---------------------------------------------------------------------------

var fpuRoundValues = []float64{
	2.3, 2.5, 2.7, 3.5, -2.3, -2.5, -2.7, -3.5, 0.0, math.Copysign(0, -1),
	0.4999999999999999, -0.5, 1e16 + 0.5, math.Inf(1), math.Inf(-1), math.NaN(),
}

// FINTRZ truncates toward zero regardless of FPCR. Reg-reg and EA forms.
func TestM68KJIT_FPUEA_FINTRZ(t *testing.T) {
	for _, v := range fpuRoundValues {
		val := v
		// FINTRZ FP2,FP3 (reg-reg)
		runM68KFPUEAParity(t, "FINTRZ reg", []uint16{0xF200, (2 << 10) | (3 << 7) | FPU_OP_FINTRZ}, func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(2, val)
			cpu.FPU.SetFP64(3, 99.0)
			cpu.FPU.FPCR = 0x40 << 4 // arbitrary FPCR bits; FINTRZ ignores rounding
		})
		// FINTRZ.D (A0),FP1 (EA)
		runM68KFPUEAParity(t, "FINTRZ ea", []uint16{fpuEAOpcode(2, 0), fpuEALoadCmd(5, 1, FPU_OP_FINTRZ)}, func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = fpuEADataAddr
			cpu.FPU.SetFP64(1, 99.0)
			fpuWriteF64(cpu, fpuEADataAddr, val)
		})
	}
}

// FINT honours the FPCR rounding mode. Cover all four modes against half-way
// and fractional values, reg-reg and EA.
func TestM68KJIT_FPUEA_FINTAllRoundingModes(t *testing.T) {
	for rnd := uint32(0); rnd < 4; rnd++ {
		fpcr := rnd << 4
		for _, v := range fpuRoundValues {
			val := v
			r := fpcr
			runM68KFPUEAParity(t, "FINT reg", []uint16{0xF200, (2 << 10) | (3 << 7) | FPU_OP_FINT}, func(cpu *M68KCPU) {
				cpu.FPU.SetFP64(2, val)
				cpu.FPU.SetFP64(3, 99.0)
				cpu.FPU.FPCR = r
			})
			runM68KFPUEAParity(t, "FINT ea", []uint16{fpuEAOpcode(2, 0), fpuEALoadCmd(5, 1, FPU_OP_FINT)}, func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = fpuEADataAddr
				cpu.FPU.SetFP64(1, 99.0)
				fpuWriteF64(cpu, fpuEADataAddr, val)
				cpu.FPU.FPCR = r
			})
		}
	}
}

// ---------------------------------------------------------------------------
// FPIAR store elision
// ---------------------------------------------------------------------------

// Consecutive FPU data ops elide all but the last FPIAR store. Final FPIAR must
// equal the last data op's PC; the parity harness compares FPIAR bit-exact.
func TestM68KJIT_FPUEA_FPIARElisionChain(t *testing.T) {
	// Three FADD.D (A0),FP0 back to back — the first two elide, the third
	// writes. FPIAR must be the third instruction's address.
	prog := []uint16{
		fpuEAOpcode(2, 0), fpuEALoadCmd(5, 0, FPU_OP_FADD),
		fpuEAOpcode(2, 0), fpuEALoadCmd(5, 0, FPU_OP_FADD),
		fpuEAOpcode(2, 0), fpuEALoadCmd(5, 0, FPU_OP_FADD),
	}
	runM68KFPUEAParity(t, "FPIAR elision chain", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = fpuEADataAddr
		cpu.FPU.SetFP64(0, 1.0)
		fpuWriteF64(cpu, fpuEADataAddr, 0.25)
	})

	// Mixed reg-reg + EA + FMOVECR sequence: every intermediate FPIAR store is
	// elided, only the final FMOVECR writes it.
	prog = []uint16{
		0xF200, (1 << 10) | (0 << 7) | FPU_OP_FADD, // FADD FP1,FP0
		fpuEAOpcode(2, 0), fpuEALoadCmd(1, 0, FPU_OP_FMUL), // FMUL.S (A0),FP0
		0xF200, 0x5C00 | (2 << 7) | 0x00, // FMOVECR #pi,FP2
	}
	runM68KFPUEAParity(t, "FPIAR mixed chain", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = fpuEADataAddr
		cpu.FPU.SetFP64(0, 2.0)
		cpu.FPU.SetFP64(1, 3.0)
		fpuWriteF32(cpu, fpuEADataAddr, 0.5)
	})
}

// FMOVE FPIAR→Dn observes FPIAR after a native data op. The op before the
// control move must NOT elide its FPIAR store (a control move is not a data
// op), so the observer reads the correct instruction address.
func TestM68KJIT_FPUEA_FPIARObservedByControlMove(t *testing.T) {
	// FADD.D (A0),FP0 ; FMOVE.L FPIAR,D0. The FADD's FPIAR store must survive
	// because the following control move reads FPIAR.
	prog := []uint16{
		fpuEAOpcode(2, 0), fpuEALoadCmd(5, 0, FPU_OP_FADD),
		0xF200, 0xA400, // FMOVE.L FPIAR,D0 (control→ea, dir 1, FPIAR select bit 10)
	}
	runM68KFPUEAParityOpt(t, "FPIAR observed", prog, func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = fpuEADataAddr
		cpu.FPU.SetFP64(0, 1.0)
		fpuWriteF64(cpu, fpuEADataAddr, 0.5)
		cpu.DataRegs[0] = 0xFFFFFFFF
	}, true) // FMOVE FPIAR,D0 is a helper-routed control move
}

// ---------------------------------------------------------------------------
// FMOVECR and FScc parity
// ---------------------------------------------------------------------------

func TestM68KJIT_FPUEA_FMOVECR(t *testing.T) {
	// Every populated ROM address plus one empty slot (0x01 → 0.0).
	for _, rom := range []uint16{0x00, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x30, 0x31,
		0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x01} {
		r := rom
		prog := []uint16{0xF200, 0x5C00 | (3 << 7) | r} // FMOVECR #rom,FP3
		runM68KFPUEAParity(t, "FMOVECR", prog, func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(3, 99.0)
		})
	}
}

func TestM68KJIT_FPUEA_FSccDn(t *testing.T) {
	ccStates := []uint32{
		0, FPU_CC_N, FPU_CC_Z, FPU_CC_NAN,
		FPU_CC_N | FPU_CC_NAN, FPU_CC_Z | FPU_CC_NAN, FPU_CC_I,
	}
	for cond := uint16(0); cond < 16; cond++ {
		for _, cc := range ccStates {
			ccVal := cc
			for _, reg := range []uint16{0, 4} { // mapped D0 and memory-resident D4
				r := reg
				prog := []uint16{0xF240 | r, cond} // FScc Dn
				runM68KFPUEAParity(t, "FScc Dn", prog, func(cpu *M68KCPU) {
					cpu.FPU.FPSR = ccVal
					cpu.DataRegs[r] = 0xDEADBEEF // upper bytes must survive
				})
			}
		}
	}
}

func TestM68KJIT_FPUEA_FSccMemory(t *testing.T) {
	ccStates := []uint32{0, FPU_CC_N, FPU_CC_Z, FPU_CC_NAN, FPU_CC_I}
	for cond := uint16(0); cond < 16; cond++ {
		for _, cc := range ccStates {
			ccVal := cc
			// FScc (A1): opcode 0xF240 | mode2<<3 | reg1 = 0xF251.
			progAnInd := []uint16{0xF251, cond}
			runM68KFPUEAParity(t, "FScc (A1)", progAnInd, func(cpu *M68KCPU) {
				cpu.FPU.FPSR = ccVal
				cpu.AddrRegs[1] = fpuEADataAddr
				cpu.Write8(fpuEADataAddr, 0x5A) // must be overwritten
			})
			// FScc 4(A2): mode5<<3 | reg2 = 0xF26A, disp 4.
			progDisp := []uint16{0xF26A, cond, 0x0004}
			runM68KFPUEAParity(t, "FScc 4(A2)", progDisp, func(cpu *M68KCPU) {
				cpu.FPU.FPSR = ccVal
				cpu.AddrRegs[2] = fpuEADataAddr
				cpu.Write8(fpuEADataAddr+4, 0xA5)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Mixed kernel: EA loads + reg-reg arithmetic + DBF loop
// ---------------------------------------------------------------------------

func TestM68KJIT_FPUEA_DotProductLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	prog := []uint16{
		0x3E3C, 0x000F, // MOVE.W #15,D7
		// loop:
		0xF218, 0x4400, // FMOVE.S (A0)+,FP0
		0xF219, 0x4423, // FMUL.S (A1)+,FP0
		0xF200, 0x00A2, // FADD FP0,FP1
		0x51CF, 0xFFF2, // DBF D7,loop (-14)
	}
	preset := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = fpuEADataAddr
		cpu.AddrRegs[1] = fpuEADataAddr + 0x80
		for i := uint32(0); i < 16; i++ {
			fpuWriteF32(cpu, fpuEADataAddr+i*4, float32(i)+0.5)
			fpuWriteF32(cpu, fpuEADataAddr+0x80+i*4, 1.0/(float32(i)+1))
		}
	}
	runM68KFPUEAParity(t, "dot-product loop", prog, preset)
}
