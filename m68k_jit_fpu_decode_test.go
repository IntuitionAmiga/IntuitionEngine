package main

import "testing"

// Slice 1 of native M68K FPU JIT: the pure register-to-register decode/eligibility
// predicate. It must mirror execFPURegToReg + m68kFPUDecodePrecisionOpmode exactly
// (src/dst/precision) so the native path and the interpreter never disagree.
// Only the SSE2-cleanly-mappable arithmetic ops are eligible; transcendentals,
// EA-operand forms, control/FMOVEM and non-general Line-F stay on the helper.

// fpuRegToRegCmd builds a general FPU register-to-register command word.
func fpuRegToRegCmd(src, dst int, op uint16) uint16 {
	return uint16(src&7)<<10 | uint16(dst&7)<<7 | (op & 0x7F)
}

const fpuGeneralOpcode = 0xF200 // Line-F, cp-id 1, type field 0 (general)

func TestM68KDecodeNativeFPU_ArithmeticRegToReg(t *testing.T) {
	cases := []struct {
		name    string
		op      uint16
		want    m68kFPUNativeOp
		wantPre int
	}{
		{"FMOVE", FPU_OP_FMOVE, m68kFPUNativeFMOVE, m68kFPURoundExtended},
		{"FADD", FPU_OP_FADD, m68kFPUNativeFADD, m68kFPURoundExtended},
		{"FSUB", FPU_OP_FSUB, m68kFPUNativeFSUB, m68kFPURoundExtended},
		{"FMUL", FPU_OP_FMUL, m68kFPUNativeFMUL, m68kFPURoundExtended},
		{"FDIV", FPU_OP_FDIV, m68kFPUNativeFDIV, m68kFPURoundExtended},
		{"FABS", FPU_OP_FABS, m68kFPUNativeFABS, m68kFPURoundExtended},
		{"FNEG", FPU_OP_FNEG, m68kFPUNativeFNEG, m68kFPURoundExtended},
		{"FSQRT", FPU_OP_FSQRT, m68kFPUNativeFSQRT, m68kFPURoundExtended},
	}
	for _, c := range cases {
		cmd := fpuRegToRegCmd(2, 3, c.op)
		op, src, dst, pre, ok := m68kDecodeNativeFPURegToReg(fpuGeneralOpcode, cmd)
		if !ok || op != c.want {
			t.Errorf("%s: op=%v ok=%v, want %v true", c.name, op, ok, c.want)
		}
		if src != 2 || dst != 3 {
			t.Errorf("%s: src=%d dst=%d, want 2,3", c.name, src, dst)
		}
		if pre != c.wantPre {
			t.Errorf("%s: precision=%d, want %d", c.name, pre, c.wantPre)
		}
	}
}

func TestM68KDecodeNativeFPU_PrecisionVariants(t *testing.T) {
	// FSADD (single) sets bit 6; FDADD (double) sets bits 6+2. Base op must
	// still decode to FADD, matching m68kFPUDecodePrecisionOpmode.
	single := fpuRegToRegCmd(1, 4, FPU_OP_FADD|0x40)
	op, _, _, pre, ok := m68kDecodeNativeFPURegToReg(fpuGeneralOpcode, single)
	if !ok || op != m68kFPUNativeFADD || pre != m68kFPURoundSingle {
		t.Fatalf("FSADD: op=%v pre=%d ok=%v, want FADD single", op, pre, ok)
	}
	double := fpuRegToRegCmd(1, 4, FPU_OP_FADD|0x44)
	op, _, _, pre, ok = m68kDecodeNativeFPURegToReg(fpuGeneralOpcode, double)
	if !ok || op != m68kFPUNativeFADD || pre != m68kFPURoundDouble {
		t.Fatalf("FDADD: op=%v pre=%d ok=%v, want FADD double", op, pre, ok)
	}
}

func TestM68KDecodeNativeFPU_RejectsNonNative(t *testing.T) {
	rej := []struct {
		name   string
		opcode uint16
		cmd    uint16
	}{
		{"FSIN (transcendental)", fpuGeneralOpcode, fpuRegToRegCmd(2, 3, 0x0E)},
		{"FSINCOS", fpuGeneralOpcode, fpuRegToRegCmd(2, 3, 0x30)},
		{"R/M=1 EA source", fpuGeneralOpcode, fpuRegToRegCmd(2, 3, FPU_OP_FADD) | 0x4000},
		{"control/FMOVEM", fpuGeneralOpcode, fpuRegToRegCmd(2, 3, FPU_OP_FADD) | 0x8000},
		{"FBcc type field", 0xF280, fpuRegToRegCmd(2, 3, FPU_OP_FADD)},
		{"FDBcc type field", 0xF240, fpuRegToRegCmd(2, 3, FPU_OP_FADD)},
		// Precision-qualified FCMP/FTST: the interpreter applies
		// applyFPUResultPrecision to fp[dst]; the native handlers don't, so
		// these must stay on the helper.
		{"FCMP single", fpuGeneralOpcode, fpuRegToRegCmd(2, 3, FPU_OP_FCMP|0x40)},
		{"FCMP double", fpuGeneralOpcode, fpuRegToRegCmd(2, 3, FPU_OP_FCMP|0x44)},
		{"FTST single", fpuGeneralOpcode, fpuRegToRegCmd(2, 3, FPU_OP_FTST|0x40)},
		{"FTST double", fpuGeneralOpcode, fpuRegToRegCmd(2, 3, FPU_OP_FTST|0x44)},
	}
	for _, c := range rej {
		if op, _, _, _, ok := m68kDecodeNativeFPURegToReg(c.opcode, c.cmd); ok {
			t.Errorf("%s: ok=true op=%v, want not native-eligible", c.name, op)
		}
	}
}
