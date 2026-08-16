// jit_x86_fpu_coverage_test.go - JIT coverage tests for the x87 forms
// compilers emit in float-heavy code: memory-operand arithmetic, the
// DE pop forms, and the reversed/remaining register forms. Each test
// exercises the JIT path via the shared rig; a form without a native
// emitter ends the compiled prefix, leaving FPU state untouched, so
// these tests fail until the emitter exists.

// Matches the constraint on the x86 JIT emitter under test
// (jit_x86_emit_amd64.go) and on the shared rig in jit_x86_emit_amd64_test.go.
// A broader constraint such as !js pulls this file into arm64 builds, where the
// rig does not exist.
//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"math"
	"testing"
	"unsafe"
)

func writeF32(r *x86JITTestRig, addr uint32, v float32) {
	bits := math.Float32bits(v)
	r.cpu.memory[addr] = byte(bits)
	r.cpu.memory[addr+1] = byte(bits >> 8)
	r.cpu.memory[addr+2] = byte(bits >> 16)
	r.cpu.memory[addr+3] = byte(bits >> 24)
}

func writeF64(r *x86JITTestRig, addr uint32, v float64) {
	bits := math.Float64bits(v)
	for i := 0; i < 8; i++ {
		r.cpu.memory[addr+uint32(i)] = byte(bits >> (8 * i))
	}
}

// d8Mem32 encodes "D8 /op m32" with disp32 addressing (mod=00 rm=101).
func d8Mem32(op byte, addr uint32) []byte {
	return []byte{0xD8, 0x05 | op<<3, byte(addr), byte(addr >> 8), byte(addr >> 16), byte(addr >> 24)}
}

// dcMem64 encodes "DC /op m64" with disp32 addressing.
func dcMem64(op byte, addr uint32) []byte {
	return []byte{0xDC, 0x05 | op<<3, byte(addr), byte(addr >> 8), byte(addr >> 16), byte(addr >> 24)}
}

// ---------------------------------------------------------------------------
// Regression: the pre-existing FXCH/FST(P) ST(i) emitters encoded R8 as
// a SIB index without REX.X, silently addressing [FPUPtr+FPUPtr]. These
// run them with a non-zero TOP so the mis-encoding cannot cancel out.
// ---------------------------------------------------------------------------

func TestX86JIT_FPU_FXCH_NonZeroTop(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(5)
	r.cpu.FPU.regs[5] = 1.0 // ST(0)
	r.cpu.FPU.regs[6] = 2.0 // ST(1)

	// D9 C9: FXCH ST(1)
	r.compileAndRun(t, 0x1000, 0xD9, 0xC9)

	if r.cpu.FPU.regs[5] != 2.0 || r.cpu.FPU.regs[6] != 1.0 {
		t.Errorf("after FXCH: regs[5]=%f regs[6]=%f, want 2.0 / 1.0",
			r.cpu.FPU.regs[5], r.cpu.FPU.regs[6])
	}
}

func TestX86JIT_FPU_FSTP_STi_NonZeroTop(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(5)
	r.cpu.FPU.regs[5] = 42.0 // ST(0)
	r.cpu.FPU.regs[7] = 0.0  // ST(2)

	// DD DA: FSTP ST(2)
	r.compileAndRun(t, 0x1000, 0xDD, 0xDA)

	if r.cpu.FPU.regs[7] != 42.0 {
		t.Errorf("ST(2) after FSTP = %f, want 42.0", r.cpu.FPU.regs[7])
	}
	if top := r.cpu.FPU.top(); top != 6 {
		t.Errorf("TOP after FSTP = %d, want 6", top)
	}
}

func TestX86JIT_FPU_FSTP_STi_CopiesExactTagClass(t *testing.T) {
	setup := func(cpu *CPU_X86) {
		cpu.FPU.Reset()
		cpu.FPU.setTop(5)
		cpu.FPU.regs[5] = 0.0
		cpu.FPU.setTag(5, x87TagZero) // ST(0)
		cpu.FPU.regs[7] = 1.5
		cpu.FPU.setTag(7, x87TagValid) // ST(2)
	}

	jit := runX86JITProgramWithSetup(t, 0x1000, setup, 0xDD, 0xDA, 0xF4) // FSTP ST(2); HLT
	interp := runX86InterpreterProgramWithSetup(t, 0x1000, setup, 0xDD, 0xDA, 0xF4)

	if got, want := jit.FPU.FTW, interp.FPU.FTW; got != want {
		t.Fatalf("FTW=%04X want %04X", got, want)
	}
	if got, want := jit.FPU.FSW, interp.FPU.FSW; got != want {
		t.Fatalf("FSW=%04X want %04X", got, want)
	}
	if got, want := jit.FPU.regs[7], interp.FPU.regs[7]; got != want {
		t.Fatalf("phys7=%f want %f", got, want)
	}
	if got, want := jit.FPU.getTag(7), interp.FPU.getTag(7); got != want {
		t.Fatalf("tag(7)=%d want %d", got, want)
	}
	if got, want := jit.FPU.getTag(0), interp.FPU.getTag(0); got != want {
		t.Fatalf("tag(0)=%d want %d", got, want)
	}
}

func TestX86JIT_FPU_DDReg2IsNotNativeFST(t *testing.T) {
	// This project's interpreter deliberately leaves DD /2 register forms as
	// no-ops. A native FST ST(i) here would create a JIT-only guest ISA.
	instrs := x86ScanBlock([]byte{0xDD, 0xD1, 0xF4}, 0)
	if len(instrs) == 0 {
		t.Fatal("x86ScanBlock returned no instruction")
	}
	if x86FPUFormSupported(&instrs[0]) {
		t.Fatal("DD D1 must remain interpreter-only: the interpreter has no FST ST(i) form")
	}
}

func TestX86JIT_FPUHelperUsesDecodedBytes(t *testing.T) {
	r := newX86JITTestRig(t)
	const pc = uint32(0x1000)
	// D9 F0 is F2XM1, intentionally owned by the canonical helper rather
	// than SSE lowering.  Replace the guest bytes with FNOP after decoding;
	// a helper that calls Step against mutable guest code would silently no-op.
	r.cpu.memory[pc] = 0xD9
	r.cpu.memory[pc+1] = 0xF0
	instrs := x86ScanBlock(r.cpu.memory, pc)
	if len(instrs) == 0 {
		t.Fatal("x86ScanBlock returned no instruction")
	}
	payload, ok := x86FPUHelperPayloadFor(instrs[0], r.cpu.memory, r.cpu.CS)
	if !ok {
		t.Fatal("x86FPUHelperPayloadFor rejected F2XM1")
	}
	r.cpu.memory[pc+1] = 0xD0 // FNOP in mutable guest backing
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 1
	r.cpu.FPU.setTag(0, x87TagValid)
	r.cpu.EIP = pc

	r.cpu.x86RunFPUHelper(payload)

	if got, want := r.cpu.FPU.regs[0], 1.0; got != want {
		t.Fatalf("F2XM1 result = %v, want %v", got, want)
	}
	if got, want := r.cpu.EIP, pc+2; got != want {
		t.Fatalf("EIP = 0x%X, want 0x%X", got, want)
	}
	if r.cpu.jitDecodedFPU != nil {
		t.Fatal("decoded helper payload was not cleared")
	}
}

func TestX86JIT_FPUHelperPreservesExactTagsForFNSTENV(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(0)
	r.cpu.FPU.regs[0] = 1
	r.cpu.FPU.setTag(0, x87TagZero)
	r.cpu.FPU.regs[1] = 2
	r.cpu.FPU.setTag(1, x87TagSpecial)
	r.cpu.x86FPUEnvLoaded = true

	const pc = uint32(0x1000)
	env := uint32(0x5000)
	code := []byte{0xD9, 0x35, byte(env), byte(env >> 8), byte(env >> 16), byte(env >> 24)}
	copy(r.cpu.memory[pc:], code)
	instrs := x86ScanBlock(r.cpu.memory, pc)
	if len(instrs) == 0 {
		t.Fatal("x86ScanBlock returned no FNSTENV instruction")
	}
	payload, ok := x86FPUHelperPayloadFor(instrs[0], r.cpu.memory, r.cpu.CS)
	if !ok {
		t.Fatal("x86FPUHelperPayloadFor rejected FNSTENV")
	}
	r.cpu.x86RunFPUHelper(payload)
	gotFTW := uint16(r.cpu.memory[env+8]) | uint16(r.cpu.memory[env+9])<<8
	if got, want := gotFTW, r.cpu.FPU.FTW; got != want {
		t.Fatalf("FNSTENV saved FTW=%04X, want exact FTW=%04X", got, want)
	}
	if got, want := r.cpu.FPU.getTag(0), x87TagZero; got != want {
		t.Fatalf("FNSTENV changed zero tag to %d, want %d", got, want)
	}
	if got, want := r.cpu.FPU.getTag(1), x87TagSpecial; got != want {
		t.Fatalf("FNSTENV changed special tag to %d, want %d", got, want)
	}
}

func TestX86JIT_FPUHelperPreservesExactTagsForFNSAVE(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(0)
	r.cpu.FPU.regs[0] = 1
	r.cpu.FPU.setTag(0, x87TagZero)
	r.cpu.FPU.regs[1] = 2
	r.cpu.FPU.setTag(1, x87TagSpecial)
	wantFTW := r.cpu.FPU.FTW
	r.cpu.x86FPUEnvLoaded = true

	const pc = uint32(0x1000)
	env := uint32(0x5000)
	code := []byte{0xDD, 0x35, byte(env), byte(env >> 8), byte(env >> 16), byte(env >> 24)}
	copy(r.cpu.memory[pc:], code)
	instrs := x86ScanBlock(r.cpu.memory, pc)
	if len(instrs) == 0 {
		t.Fatal("x86ScanBlock returned no FNSAVE instruction")
	}
	payload, ok := x86FPUHelperPayloadFor(instrs[0], r.cpu.memory, r.cpu.CS)
	if !ok {
		t.Fatal("x86FPUHelperPayloadFor rejected FNSAVE")
	}
	r.cpu.x86RunFPUHelper(payload)
	gotFTW := uint16(r.cpu.memory[env+8]) | uint16(r.cpu.memory[env+9])<<8
	if gotFTW != wantFTW {
		t.Fatalf("FNSAVE saved FTW=%04X, want exact FTW=%04X", gotFTW, wantFTW)
	}
	if got, want := r.cpu.FPU.FTW, uint16(0xFFFF); got != want {
		t.Fatalf("FNSAVE post-reset FTW=%04X, want %04X", got, want)
	}
}

func TestX86JIT_NativeFPUOperationInvalidatesExactEnvironmentTags(t *testing.T) {
	const pc = uint32(0x1000)
	env := uint32(0x5000)
	setup := func(cpu *CPU_X86) {
		cpu.FPU.Reset()
		cpu.FPU.setTop(0)
		cpu.FPU.regs[0] = 1
		cpu.FPU.setTag(0, x87TagZero)
		cpu.FPU.regs[1] = 2
		cpu.FPU.setTag(1, x87TagSpecial)
		cpu.x86FPUEnvLoaded = true
	}

	jit := runX86JITProgramWithSetup(t, pc, setup,
		0xD8, 0xC1, // FADD ST(0), ST(1), native x87 operation
		0xD9, 0x35, byte(env), byte(env>>8), byte(env>>16), byte(env>>24), // FNSTENV
		0xF4,
	)
	gotFTW := uint16(jit.memory[env+8]) | uint16(jit.memory[env+9])<<8
	if got, want := gotFTW, uint16(0xFFF0); got != want {
		t.Fatalf("FNSTENV saved stale native-boundary FTW=%04X, want valid tags %04X", got, want)
	}
}

func TestX86JIT_FPUHelperFormEmitsCanonicalExit(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.EAX = 7
	r.cpu.FPU.regs[0] = 1
	r.cpu.FPU.setTag(0, x87TagValid)

	const pc = uint32(0x1000)
	// D9 F0 is F2XM1. It is interpreter-owned, but must still be a compiled
	// x87 helper exit rather than a zero-length native prefix.
	r.compileAndRun(t, pc,
		0xD9, 0xF0, // F2XM1
		0xB8, 1, 0, 0, 0, // unreachable MOV EAX,1
	)
	if r.ctx.ExitReason != x86JITExitFPUHelper || r.ctx.NeedIOFallback == 0 {
		t.Fatalf("helper form exit = reason %d io %d, want canonical helper exit", r.ctx.ExitReason, r.ctx.NeedIOFallback)
	}
	payload, ok := x86FPUHelperPayloadFromContext(r.ctx)
	if !ok {
		t.Fatal("compiled F2XM1 did not publish a helper payload")
	}
	if got := r.ctx.RetCount; got != 0 {
		t.Fatalf("helper exit retired %d native instructions, want 0", got)
	}
	if got := r.cpu.EAX; got != 7 {
		t.Fatalf("instruction after helper changed EAX to %d, want 7", got)
	}
	r.cpu.x86RunFPUHelper(payload)
	if got, want := r.cpu.FPU.regs[0], 1.0; got != want {
		t.Fatalf("F2XM1 result = %v, want %v", got, want)
	}
}

func TestX86JIT_FPUHelperPrefixFormsKeepDecodedBytes(t *testing.T) {
	for _, code := range [][]byte{
		{0x64, 0xD9, 0xF0}, // FS:F2XM1
		{0x67, 0xD9, 0xF0}, // address-size F2XM1
	} {
		r := newX86JITTestRig(t)
		r.cpu.FPU.Reset()
		r.cpu.FPU.regs[0] = 1
		r.cpu.FPU.setTag(0, x87TagValid)
		r.compileAndRun(t, 0x1000, code...)
		payload, ok := x86FPUHelperPayloadFromContext(r.ctx)
		if !ok {
			t.Fatalf("% X: no canonical helper payload", code)
		}
		if got, want := payload.Bytes[:len(code)], code; !bytes.Equal(got, want) {
			t.Fatalf("% X: payload bytes = % X, want % X", code, got, want)
		}
		r.cpu.x86RunFPUHelper(payload)
		if got, want := r.cpu.FPU.regs[0], 1.0; got != want {
			t.Fatalf("% X: F2XM1 result = %v, want %v", code, got, want)
		}
	}
}

func TestX86JIT_FPUHelperPrefixFormsAreAdmittedByBlockCompiler(t *testing.T) {
	// These forms are intentionally helper-owned, but must still enter a JIT
	// block so their decoded prefix bytes survive an SMC boundary. This catches
	// a liveness gate that would otherwise route them straight to CPU_X86.Step.
	for _, code := range [][]byte{
		{0x64, 0xD9, 0xF0, 0xF4}, // FS:F2XM1
		{0x67, 0xD9, 0xF0, 0xF4}, // address-size F2XM1
	} {
		memory := make([]byte, 0x2000)
		copy(memory[0x1000:], code)
		length := x86InstrLength(memory, 0x1000)
		instr := x86DecodeInstr(memory, 0x1000, uint16(length))
		if !x86FlagAnalysisCanCompileInstruction(&instr) {
			t.Fatalf("% X is not admitted to the x87 helper exit", code[:len(code)-1])
		}

		setup := func(cpu *CPU_X86) {
			cpu.FPU.Reset()
			cpu.FPU.regs[0] = 1
			cpu.FPU.setTag(0, x87TagValid)
		}
		jit := runX86JITProgramWithSetup(t, 0x1000, setup, code...)
		interp := runX86InterpreterProgramWithSetup(t, 0x1000, setup, code...)
		if got, want := jit.FPU.regs[0], interp.FPU.regs[0]; got != want {
			t.Fatalf("% X: ST(0) = %v, want interpreter %v", code[:len(code)-1], got, want)
		}
		if got, want := jit.FPU.FIP, interp.FPU.FIP; got != want {
			t.Fatalf("% X: FIP = %#x, want interpreter %#x", code[:len(code)-1], got, want)
		}
	}
}

func TestX86JIT_FPUHelperOwnedEscapeFamiliesEmitCanonicalExit(t *testing.T) {
	// One representative from every remaining helper-owned escape family. The
	// test runs the generated code, not merely the scanner, so a future emitter
	// gap cannot silently shrink a compiled block back to a dispatcher Step.
	forms := [][]byte{
		{0xD9, 0xF0},                         // F2XM1
		{0xD9, 0x25, 0x00, 0x50, 0x00, 0x00}, // FLDENV m28
		{0xDB, 0x2D, 0x00, 0x50, 0x00, 0x00}, // FLD m80
		{0xDD, 0x25, 0x00, 0x50, 0x00, 0x00}, // FRSTOR m94
		{0xDE, 0xD9},                         // FCOMPP
		{0xDF, 0x25, 0x00, 0x50, 0x00, 0x00}, // FBLD m80
		{0xDF, 0x15, 0x00, 0x50, 0x00, 0x00}, // FIST m16
		{0xDD, 0x3D, 0x00, 0x50, 0x00, 0x00}, // FNSTSW m16
	}
	for _, code := range forms {
		r := newX86JITTestRig(t)
		r.compileAndRun(t, 0x1000, code...)
		if r.ctx.ExitReason != x86JITExitFPUHelper || r.ctx.NeedIOFallback == 0 {
			t.Fatalf("% X: exit = reason %d io %d, want canonical helper exit", code, r.ctx.ExitReason, r.ctx.NeedIOFallback)
		}
		payload, ok := x86FPUHelperPayloadFromContext(r.ctx)
		if !ok {
			t.Fatalf("% X: no canonical helper payload", code)
		}
		if got, want := payload.Bytes[:len(code)], code; !bytes.Equal(got, want) {
			t.Fatalf("% X: payload bytes = % X, want % X", code, got, want)
		}
	}
}

func TestX86JIT_FPUDynamicExitPublishesDecodedPayload(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.FCW = 0x077F // RC=01: round down, handled by the helper
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 2.9
	r.cpu.FPU.setTag(6, x87TagValid)

	const pc = uint32(0x1000)
	// DB 1D <addr>: FISTP dword [addr]. The native lowering reaches its
	// FCW-dependent exit after publishing the resolved EA and decoded bytes.
	r.compileAndRun(t, pc, 0xDB, 0x1D, 0x00, 0x50, 0x00, 0x00)
	payload, ok := x86FPUHelperPayloadFromContext(r.ctx)
	if !ok {
		t.Fatal("FISTP dynamic exit did not publish an x87 helper payload")
	}
	if got, want := payload.InstrPC, pc; got != want {
		t.Fatalf("payload PC = 0x%X, want 0x%X", got, want)
	}
	if got, want := payload.EA, uint32(0x5000); got != want {
		t.Fatalf("payload EA = 0x%X, want 0x%X", got, want)
	}
	if got, want := payload.Width, uint32(4); got != want {
		t.Fatalf("payload width = %d, want %d", got, want)
	}
	if got, want := payload.Bytes[:6], []byte{0xDB, 0x1D, 0x00, 0x50, 0x00, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("payload bytes = % X, want % X", got, want)
	}

	// A code write after the native exit must not change the helper operation.
	r.cpu.memory[pc] = 0xD9
	r.cpu.memory[pc+1] = 0xD0 // FNOP in mutable guest backing
	r.cpu.x86RunFPUHelper(payload)
	if got := int32(uint32(r.cpu.memory[0x5000]) | uint32(r.cpu.memory[0x5001])<<8 |
		uint32(r.cpu.memory[0x5002])<<16 | uint32(r.cpu.memory[0x5003])<<24); got != 2 {
		t.Fatalf("helper FISTP result = %d, want 2", got)
	}
}

func TestX86JIT_FPUDirectMemoryBoundaryBailsWithDecodedPayload(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 1
	r.cpu.FPU.setTag(0, x87TagValid)

	// FADD dword [visible-RAM-3] straddles the ceiling. The first page is a
	// compile-time-safe RAM page, so this also protects against accidentally
	// eliding the x87 span guard before checking the complete operand.
	addr := uint32(len(r.cpu.memory) - 3)
	r.compileAndRun(t, 0x1000, d8Mem32(0, addr)...)
	if got, want := r.ctx.ExitReason, x86JITExitFPUHelper; got != want {
		t.Fatalf("exit reason = %d, want FPU helper %d", got, want)
	}
	if got := r.cpu.EIP; got != 0x1000 {
		t.Fatalf("EIP = %#x, want 0x1000 for helper replay", got)
	}
	if got := r.cpu.FPU.regs[0]; got != 1 {
		t.Fatalf("ST(0) = %v, want unchanged 1 before helper replay", got)
	}
	payload, ok := x86FPUHelperPayloadFromContext(r.ctx)
	if !ok {
		t.Fatal("boundary exit did not publish an x87 helper payload")
	}
	if got, want := payload.EA, addr; got != want {
		t.Fatalf("payload EA = %#x, want %#x", got, want)
	}
	if got, want := payload.Width, uint32(4); got != want {
		t.Fatalf("payload width = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// D8 memory-operand arithmetic (FADD/FMUL/FSUB/FSUBR/FDIV/FDIVR m32)
// ---------------------------------------------------------------------------

func TestX86JIT_FPU_FADDS_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 1.5
	r.cpu.FPU.setTop(0)
	writeF32(r, 0x5000, 2.25)

	r.compileAndRun(t, 0x1000, d8Mem32(0, 0x5000)...)

	if r.cpu.FPU.regs[0] != 3.75 {
		t.Errorf("ST(0) = %f, want 3.75", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FMULS_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 3.0
	r.cpu.FPU.setTop(0)
	writeF32(r, 0x5000, 7.0)

	r.compileAndRun(t, 0x1000, d8Mem32(1, 0x5000)...)

	if r.cpu.FPU.regs[0] != 21.0 {
		t.Errorf("ST(0) = %f, want 21.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FSUBS_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 10.0
	r.cpu.FPU.setTop(0)
	writeF32(r, 0x5000, 3.0)

	r.compileAndRun(t, 0x1000, d8Mem32(4, 0x5000)...)

	if r.cpu.FPU.regs[0] != 7.0 {
		t.Errorf("ST(0) = %f, want 7.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FSUBRS_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 3.0
	r.cpu.FPU.setTop(0)
	writeF32(r, 0x5000, 10.0)

	r.compileAndRun(t, 0x1000, d8Mem32(5, 0x5000)...)

	if r.cpu.FPU.regs[0] != 7.0 {
		t.Errorf("ST(0) = %f, want 7.0 (m32 - ST0)", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FDIVS_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 21.0
	r.cpu.FPU.setTop(0)
	writeF32(r, 0x5000, 3.0)

	r.compileAndRun(t, 0x1000, d8Mem32(6, 0x5000)...)

	if r.cpu.FPU.regs[0] != 7.0 {
		t.Errorf("ST(0) = %f, want 7.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FDIVRS_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 3.0
	r.cpu.FPU.setTop(0)
	writeF32(r, 0x5000, 21.0)

	r.compileAndRun(t, 0x1000, d8Mem32(7, 0x5000)...)

	if r.cpu.FPU.regs[0] != 7.0 {
		t.Errorf("ST(0) = %f, want 7.0 (m32 / ST0)", r.cpu.FPU.regs[0])
	}
}

// FADDS through a register-indirect address (mod=00 rm=EBX): compilers
// address vertex arrays through registers, not just disp32.
func TestX86JIT_FPU_FADDS_mem32_RegIndirect(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 1.0
	r.cpu.FPU.setTop(0)
	r.cpu.EBX = 0x5000
	writeF32(r, 0x5000, 41.0)

	// D8 03: FADD dword [EBX]
	r.compileAndRun(t, 0x1000, 0xD8, 0x03)

	if r.cpu.FPU.regs[0] != 42.0 {
		t.Errorf("ST(0) = %f, want 42.0", r.cpu.FPU.regs[0])
	}
}

// ---------------------------------------------------------------------------
// DC memory-operand arithmetic (m64) and DC register forms
// ---------------------------------------------------------------------------

func TestX86JIT_FPU_FADDL_mem64(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 1.5
	r.cpu.FPU.setTop(0)
	writeF64(r, 0x5000, 2.25)

	r.compileAndRun(t, 0x1000, dcMem64(0, 0x5000)...)

	if r.cpu.FPU.regs[0] != 3.75 {
		t.Errorf("ST(0) = %f, want 3.75", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FMUL_ToSTi(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 3.0 // ST(0)
	r.cpu.FPU.regs[1] = 7.0 // ST(1)
	r.cpu.FPU.setTop(0)

	// DC C9: FMUL ST(1), ST(0) -> ST(1) = ST(1) * ST(0)
	r.compileAndRun(t, 0x1000, 0xDC, 0xC9)

	if r.cpu.FPU.regs[1] != 21.0 {
		t.Errorf("ST(1) = %f, want 21.0", r.cpu.FPU.regs[1])
	}
	if r.cpu.FPU.regs[0] != 3.0 {
		t.Errorf("ST(0) = %f, want 3.0 unchanged", r.cpu.FPU.regs[0])
	}
}

// ---------------------------------------------------------------------------
// D8 register forms missing from the original set (FSUBR/FDIVR ST0,STi)
// ---------------------------------------------------------------------------

func TestX86JIT_FPU_FSUBR_RegReg(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 3.0  // ST(0)
	r.cpu.FPU.regs[1] = 10.0 // ST(1)
	r.cpu.FPU.setTop(0)

	// D8 E9: FSUBR ST(0), ST(1) -> ST(0) = ST(1) - ST(0)
	r.compileAndRun(t, 0x1000, 0xD8, 0xE9)

	if r.cpu.FPU.regs[0] != 7.0 {
		t.Errorf("ST(0) = %f, want 7.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FDIVR_RegReg(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 3.0  // ST(0)
	r.cpu.FPU.regs[1] = 21.0 // ST(1)
	r.cpu.FPU.setTop(0)

	// D8 F9: FDIVR ST(0), ST(1) -> ST(0) = ST(1) / ST(0)
	r.compileAndRun(t, 0x1000, 0xD8, 0xF9)

	if r.cpu.FPU.regs[0] != 7.0 {
		t.Errorf("ST(0) = %f, want 7.0", r.cpu.FPU.regs[0])
	}
}

// ---------------------------------------------------------------------------
// DE pop forms: op then pop, freeing the old ST(0)
// ---------------------------------------------------------------------------

// popFormRig sets ST(0)=top, ST(1)=below with TOP=6 so the pop's TOP
// increment is visible (6 -> 7).
func popFormRig(t *testing.T, below, top float64) *x86JITTestRig {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = top   // ST(0)
	r.cpu.FPU.regs[7] = below // ST(1)
	return r
}

func assertPopResult(t *testing.T, r *x86JITTestRig, want float64) {
	t.Helper()
	if got := r.cpu.FPU.regs[7]; got != want {
		t.Errorf("ST(1) after pop-op = %f, want %f", got, want)
	}
	if top := r.cpu.FPU.top(); top != 7 {
		t.Errorf("TOP after pop = %d, want 7", top)
	}
}

func TestX86JIT_FPU_FADDP(t *testing.T) {
	r := popFormRig(t, 2.5, 1.5)
	// DE C1: FADDP ST(1), ST(0)
	r.compileAndRun(t, 0x1000, 0xDE, 0xC1)
	assertPopResult(t, r, 4.0)
}

func TestX86JIT_FPU_FMULP(t *testing.T) {
	r := popFormRig(t, 7.0, 3.0)
	// DE C9: FMULP ST(1), ST(0)
	r.compileAndRun(t, 0x1000, 0xDE, 0xC9)
	assertPopResult(t, r, 21.0)
}

func TestX86JIT_FPU_FSUBP(t *testing.T) {
	r := popFormRig(t, 10.0, 3.0)
	// DE E9: FSUBP ST(1), ST(0) -> ST(1) = ST(1) - ST(0)
	r.compileAndRun(t, 0x1000, 0xDE, 0xE9)
	assertPopResult(t, r, 7.0)
}

func TestX86JIT_FPU_FSUBRP(t *testing.T) {
	r := popFormRig(t, 3.0, 10.0)
	// DE E1: FSUBRP ST(1), ST(0) -> ST(1) = ST(0) - ST(1)
	r.compileAndRun(t, 0x1000, 0xDE, 0xE1)
	assertPopResult(t, r, 7.0)
}

func TestX86JIT_FPU_FDIVP(t *testing.T) {
	r := popFormRig(t, 21.0, 3.0)
	// DE F9: FDIVP ST(1), ST(0) -> ST(1) = ST(1) / ST(0)
	r.compileAndRun(t, 0x1000, 0xDE, 0xF9)
	assertPopResult(t, r, 7.0)
}

func TestX86JIT_FPU_FDIVRP(t *testing.T) {
	r := popFormRig(t, 3.0, 21.0)
	// DE F1: FDIVRP ST(1), ST(0) -> ST(1) = ST(0) / ST(1)
	r.compileAndRun(t, 0x1000, 0xDE, 0xF1)
	assertPopResult(t, r, 7.0)
}

// ---------------------------------------------------------------------------
// Single-precision stores (FSTP/FST m32)
// ---------------------------------------------------------------------------

func readF32(r *x86JITTestRig, addr uint32) float32 {
	bits := uint32(r.cpu.memory[addr]) | uint32(r.cpu.memory[addr+1])<<8 |
		uint32(r.cpu.memory[addr+2])<<16 | uint32(r.cpu.memory[addr+3])<<24
	return math.Float32frombits(bits)
}

// d9Mem encodes "D9 /op m32" with disp32 addressing.
func d9Mem(op byte, addr uint32) []byte {
	return []byte{0xD9, 0x05 | op<<3, byte(addr), byte(addr >> 8), byte(addr >> 16), byte(addr >> 24)}
}

func TestX86JIT_FPU_FSTPS_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 2.5

	r.compileAndRun(t, 0x1000, d9Mem(3, 0x5000)...)

	if got := readF32(r, 0x5000); got != 2.5 {
		t.Errorf("m32 after FSTP = %f, want 2.5", got)
	}
	if top := r.cpu.FPU.top(); top != 7 {
		t.Errorf("TOP after FSTP = %d, want 7", top)
	}
}

func TestX86JIT_FPU_FSTS_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 2.5

	r.compileAndRun(t, 0x1000, d9Mem(2, 0x5000)...)

	if got := readF32(r, 0x5000); got != 2.5 {
		t.Errorf("m32 after FST = %f, want 2.5", got)
	}
	if top := r.cpu.FPU.top(); top != 6 {
		t.Errorf("TOP after FST = %d, want 6 (no pop)", top)
	}
}

// ---------------------------------------------------------------------------
// Constant loads
// ---------------------------------------------------------------------------

func TestX86JIT_FPU_FLD1(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(0)

	// D9 E8: FLD1
	r.compileAndRun(t, 0x1000, 0xD9, 0xE8)

	if top := r.cpu.FPU.top(); top != 7 {
		t.Fatalf("TOP after FLD1 = %d, want 7", top)
	}
	if r.cpu.FPU.regs[7] != 1.0 {
		t.Errorf("ST(0) = %f, want 1.0", r.cpu.FPU.regs[7])
	}
}

func TestX86JIT_FPU_FLDZ(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(0)

	// D9 EE: FLDZ
	r.compileAndRun(t, 0x1000, 0xD9, 0xEE)

	if top := r.cpu.FPU.top(); top != 7 {
		t.Fatalf("TOP after FLDZ = %d, want 7", top)
	}
	if r.cpu.FPU.regs[7] != 0.0 {
		t.Errorf("ST(0) = %f, want 0.0", r.cpu.FPU.regs[7])
	}
}

// ---------------------------------------------------------------------------
// Integer loads/stores and integer arithmetic
// ---------------------------------------------------------------------------

func TestX86JIT_FPU_FILDL_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(0)
	r.cpu.memory[0x5000] = 0xFE
	r.cpu.memory[0x5001] = 0xFF
	r.cpu.memory[0x5002] = 0xFF
	r.cpu.memory[0x5003] = 0xFF // -2 as int32

	// DB 05 <addr>: FILD dword [addr]
	r.compileAndRun(t, 0x1000, 0xDB, 0x05, 0x00, 0x50, 0x00, 0x00)

	if r.cpu.FPU.regs[7] != -2.0 {
		t.Errorf("ST(0) = %f, want -2.0", r.cpu.FPU.regs[7])
	}
}

func TestX86JIT_FPU_FILDS_mem16(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(0)
	r.cpu.memory[0x5000] = 0xFE
	r.cpu.memory[0x5001] = 0xFF // -2 as int16

	// DF 05 <addr>: FILD word [addr]
	r.compileAndRun(t, 0x1000, 0xDF, 0x05, 0x00, 0x50, 0x00, 0x00)

	if r.cpu.FPU.regs[7] != -2.0 {
		t.Errorf("ST(0) = %f, want -2.0", r.cpu.FPU.regs[7])
	}
}

func TestX86JIT_FPU_FILDQ_mem64(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	value := int64(-0x102030405)
	for i := range 8 {
		r.cpu.memory[0x5000+uint32(i)] = byte(uint64(value) >> (8 * i))
	}

	// DF 2D <addr>: FILD qword [addr]
	r.compileAndRun(t, 0x1000, 0xDF, 0x2D, 0x00, 0x50, 0x00, 0x00)

	if got, want := r.cpu.FPU.regs[7], float64(value); got != want {
		t.Errorf("ST(0) = %f, want %f", got, want)
	}
}

func TestX86JIT_FPU_FISTPL_Truncate(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.FCW = 0x0F7F // RC=11 truncate
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 2.9

	// DB 1D <addr>: FISTP dword [addr]
	r.compileAndRun(t, 0x1000, 0xDB, 0x1D, 0x00, 0x50, 0x00, 0x00)

	got := int32(uint32(r.cpu.memory[0x5000]) | uint32(r.cpu.memory[0x5001])<<8 |
		uint32(r.cpu.memory[0x5002])<<16 | uint32(r.cpu.memory[0x5003])<<24)
	if got != 2 {
		t.Errorf("m32 after FISTP(truncate) = %d, want 2", got)
	}
	if top := r.cpu.FPU.top(); top != 7 {
		t.Errorf("TOP after FISTP = %d, want 7", top)
	}
}

func TestX86JIT_FPU_FISTPL_Nearest(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset() // FCW default RC=00 nearest-even
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 2.5

	r.compileAndRun(t, 0x1000, 0xDB, 0x1D, 0x00, 0x50, 0x00, 0x00)

	got := int32(uint32(r.cpu.memory[0x5000]) | uint32(r.cpu.memory[0x5001])<<8 |
		uint32(r.cpu.memory[0x5002])<<16 | uint32(r.cpu.memory[0x5003])<<24)
	if got != 2 {
		t.Errorf("m32 after FISTP(nearest) = %d, want 2 (round-half-even)", got)
	}
}

func TestX86JIT_FPU_FISTL_NearestPreservesTop(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset() // FCW default RC=00 nearest-even
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 2.5
	r.cpu.FPU.setTag(6, x87TagValid)

	// DB 15 <addr>: FIST dword [addr]
	r.compileAndRun(t, 0x1000, 0xDB, 0x15, 0x00, 0x50, 0x00, 0x00)

	got := int32(uint32(r.cpu.memory[0x5000]) | uint32(r.cpu.memory[0x5001])<<8 |
		uint32(r.cpu.memory[0x5002])<<16 | uint32(r.cpu.memory[0x5003])<<24)
	if got != 2 {
		t.Errorf("m32 after FIST(nearest-even) = %d, want 2", got)
	}
	if top := r.cpu.FPU.top(); top != 6 {
		t.Errorf("TOP after FIST = %d, want 6", top)
	}
}

func TestX86JIT_FISTStores_CodePageInvalidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		size uint32
	}{
		{"FIST m32", []byte{0xDB, 0x10}, 4},
		{"FISTP m32", []byte{0xDB, 0x18}, 4},
		{"FISTP m64", []byte{0xDF, 0x38}, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newX86JITTestRig(t)
			r.cpu.FPU = NewFPU_X87()
			r.ctx.FPUPtr = uintptr(unsafe.Pointer(r.cpu.FPU))
			r.cpu.FPU.Reset()
			r.cpu.FPU.setTop(6)
			r.cpu.FPU.regs[6] = 42
			r.cpu.FPU.setTag(6, x87TagValid)
			r.cpu.EAX = 0x5000
			r.codeBM[0x5000>>8] = 1
			r.ctx.NeedInval = 0

			r.compileAndRun(t, 0x1000, tc.code...)

			if r.ctx.NeedInval == 0 {
				t.Fatal("NeedInval = 0, want set after FIST store to code page")
			}
			if got, want := r.ctx.InvalAddr, uint32(0x5000); got != want {
				t.Fatalf("InvalAddr = %#x, want %#x", got, want)
			}
			if got, want := r.ctx.InvalSize, tc.size; got != want {
				t.Fatalf("InvalSize = %d, want %d", got, want)
			}
		})
	}
}

func TestX86JIT_FPU_FIADDL_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 1.5
	r.cpu.memory[0x5000] = 3 // int32 3

	// DA 05 <addr>: FIADD dword [addr]
	r.compileAndRun(t, 0x1000, 0xDA, 0x05, 0x00, 0x50, 0x00, 0x00)

	if r.cpu.FPU.regs[6] != 4.5 {
		t.Errorf("ST(0) = %f, want 4.5", r.cpu.FPU.regs[6])
	}
}

func TestX86JIT_FPU_FIMULL_mem32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 1.5
	r.cpu.memory[0x5000] = 4 // int32 4

	// DA 0D <addr>: FIMUL dword [addr]
	r.compileAndRun(t, 0x1000, 0xDA, 0x0D, 0x00, 0x50, 0x00, 0x00)

	if r.cpu.FPU.regs[6] != 6.0 {
		t.Errorf("ST(0) = %f, want 6.0", r.cpu.FPU.regs[6])
	}
}

// ---------------------------------------------------------------------------
// Control words and status word
// ---------------------------------------------------------------------------

func TestX86JIT_FPU_FLDCW_FNSTCW(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.memory[0x5000] = 0x7F
	r.cpu.memory[0x5001] = 0x0F // 0x0F7F

	// D9 2D <addr>: FLDCW [addr]; D9 3D <addr2>: FNSTCW [addr2]
	r.compileAndRun(t, 0x1000,
		0xD9, 0x2D, 0x00, 0x50, 0x00, 0x00,
		0xD9, 0x3D, 0x00, 0x60, 0x00, 0x00)

	if r.cpu.FPU.FCW != 0x0F7F {
		t.Errorf("FCW = %#x, want 0x0F7F", r.cpu.FPU.FCW)
	}
	got := uint16(r.cpu.memory[0x6000]) | uint16(r.cpu.memory[0x6001])<<8
	if got != 0x0F7F {
		t.Errorf("FNSTCW stored %#x, want 0x0F7F", got)
	}
}

func TestX86JIT_FPU_FNSTSW_AX(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.FSW = 0x4100
	r.cpu.EAX = 0xDEAD0000

	// DF E0: FNSTSW AX
	r.compileAndRun(t, 0x1000, 0xDF, 0xE0)

	if r.cpu.EAX != 0xDEAD4100 {
		t.Errorf("EAX = %#x, want 0xDEAD4100", r.cpu.EAX)
	}
}

// ---------------------------------------------------------------------------
// FNSTSW AX under non-default register maps: Tier-2 regions remap or
// spill guest EAX, so the emitter must go through the active map, not
// the fixed default slot.
// ---------------------------------------------------------------------------

func runFNSTSWWithRegMap(t *testing.T, regMap [8]byte) *x86JITTestRig {
	t.Helper()
	x86CompileRegMapOverrideForTest = &regMap
	t.Cleanup(func() { x86CompileRegMapOverrideForTest = nil })

	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.FSW = 0x4100
	r.cpu.EAX = 0xDEAD0000
	r.cpu.EBX = 0x0B0B0B0B

	// DF E0: FNSTSW AX
	r.compileAndRun(t, 0x1000, 0xDF, 0xE0)
	return r
}

func TestX86JIT_FPU_FNSTSW_AX_SpilledEAX(t *testing.T) {
	m := x86DefaultRegMap()
	m[0] = 0 // EAX spilled to jitRegs
	r := runFNSTSWWithRegMap(t, m)

	if r.cpu.EAX != 0xDEAD4100 {
		t.Errorf("EAX = %#x, want 0xDEAD4100 (spilled path)", r.cpu.EAX)
	}
	if r.cpu.EBX != 0x0B0B0B0B {
		t.Errorf("EBX = %#x, want 0x0B0B0B0B untouched", r.cpu.EBX)
	}
}

func TestX86JIT_FPU_FNSTSW_AX_RemappedEAX(t *testing.T) {
	m := x86DefaultRegMap()
	m[0], m[3] = m[3], m[0] // swap EAX and EBX host slots
	r := runFNSTSWWithRegMap(t, m)

	if r.cpu.EAX != 0xDEAD4100 {
		t.Errorf("EAX = %#x, want 0xDEAD4100 (remapped path)", r.cpu.EAX)
	}
	if r.cpu.EBX != 0x0B0B0B0B {
		t.Errorf("EBX = %#x, want 0x0B0B0B0B untouched (default-slot write would corrupt it)", r.cpu.EBX)
	}
}

// ---------------------------------------------------------------------------
// BSWAP r32 (0F C8+r, 486+): guests that byte-swap bus data (big-endian
// callers on the shared bus) execute this in their hottest loops.
// ---------------------------------------------------------------------------

func TestX86JIT_BSWAP_EAX(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0x12345678

	// 0F C8: BSWAP EAX
	r.compileAndRun(t, 0x1000, 0x0F, 0xC8)

	if r.cpu.EAX != 0x78563412 {
		t.Errorf("EAX = %#x, want 0x78563412", r.cpu.EAX)
	}
}

func TestX86JIT_BSWAP_EDI(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EDI = 0xAABBCCDD

	// 0F CF: BSWAP EDI (spilled register exercises the memory path)
	r.compileAndRun(t, 0x1000, 0x0F, 0xCF)

	if r.cpu.EDI != 0xDDCCBBAA {
		t.Errorf("EDI = %#x, want 0xDDCCBBAA", r.cpu.EDI)
	}
}

func TestX86Interp_BSWAP(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.EBX = 0x01020304
	writeCode(bus, 0x1000, 0x0F, 0xCB) // BSWAP EBX
	cpu.EIP = 0x1000
	cpu.Step()

	if cpu.EBX != 0x04030201 {
		t.Errorf("EBX = %#x, want 0x04030201", cpu.EBX)
	}
	if cpu.EIP != 0x1002 {
		t.Errorf("EIP = %#x, want 0x1002", cpu.EIP)
	}
}

// ---------------------------------------------------------------------------
// Compares: FCOM/FCOMP (reg and m32) and FTST set C0/C2/C3
// ---------------------------------------------------------------------------

const fpuCBits = uint16(0x4500) // C3|C2|C0

func TestX86JIT_FPU_FCOMS_mem32(t *testing.T) {
	cases := []struct {
		st0, mem float32
		want     uint16
	}{
		{1.0, 2.0, 0x0100}, // ST0 < mem: C0
		{2.0, 2.0, 0x4000}, // equal: C3
		{3.0, 2.0, 0x0000}, // ST0 > mem: none
	}
	for _, c := range cases {
		r := newX86JITTestRig(t)
		r.cpu.FPU.Reset()
		r.cpu.FPU.setTop(6)
		r.cpu.FPU.regs[6] = float64(c.st0)
		writeF32(r, 0x5000, c.mem)

		// D8 15 <addr>: FCOM dword [addr]
		r.compileAndRun(t, 0x1000, 0xD8, 0x15, 0x00, 0x50, 0x00, 0x00)

		if got := r.cpu.FPU.FSW & fpuCBits; got != c.want {
			t.Errorf("FCOM %v vs %v: C bits = %#x, want %#x", c.st0, c.mem, got, c.want)
		}
	}
}

func TestX86JIT_FPU_FCOMPS_mem32_Pops(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(6)
	r.cpu.FPU.regs[6] = 1.0
	writeF32(r, 0x5000, 2.0)

	// D8 1D <addr>: FCOMP dword [addr]
	r.compileAndRun(t, 0x1000, 0xD8, 0x1D, 0x00, 0x50, 0x00, 0x00)

	if got := r.cpu.FPU.FSW & fpuCBits; got != 0x0100 {
		t.Errorf("C bits = %#x, want C0", got)
	}
	if top := r.cpu.FPU.top(); top != 7 {
		t.Errorf("TOP after FCOMP = %d, want 7", top)
	}
}

func TestX86JIT_FPU_FCOM_RegReg(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 5.0 // ST(0)
	r.cpu.FPU.regs[1] = 5.0 // ST(1)
	r.cpu.FPU.setTop(0)

	// D8 D1: FCOM ST(1)
	r.compileAndRun(t, 0x1000, 0xD8, 0xD1)

	if got := r.cpu.FPU.FSW & fpuCBits; got != 0x4000 {
		t.Errorf("C bits = %#x, want C3 (equal)", got)
	}
}

// TestX86JIT_FPU_KernelShapedBlock runs the exact instruction shape GCC
// emits for a fixed-function transform row (FLD m32; FMUL m32; FLD m32;
// FMUL m32; FADDP; FLD m32; FMUL m32; FADDP; FADDS m32; FSTP m32) as a
// single JIT block. The rig executes only the natively compiled prefix,
// so a correct result proves every form in the chain compiles.
func TestX86JIT_FPU_KernelShapedBlock(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.setTop(0)

	// m[0..2] = 2, 3, 4; v[0..2] = 5, 6, 7; bias = 1
	// expect m·v + bias = 10 + 18 + 28 + 1 = 57
	writeF32(r, 0x5000, 2.0)
	writeF32(r, 0x5004, 3.0)
	writeF32(r, 0x5008, 4.0)
	writeF32(r, 0x5010, 5.0)
	writeF32(r, 0x5014, 6.0)
	writeF32(r, 0x5018, 7.0)
	writeF32(r, 0x5020, 1.0)

	code := []byte{}
	appendOp := func(b ...byte) { code = append(code, b...) }
	appendOp(d9Mem(0, 0x5000)...)   // FLD  m[0]
	appendOp(d8Mem32(1, 0x5010)...) // FMUL v[0]
	appendOp(d9Mem(0, 0x5004)...)   // FLD  m[1]
	appendOp(d8Mem32(1, 0x5014)...) // FMUL v[1]
	appendOp(0xDE, 0xC1)            // FADDP
	appendOp(d9Mem(0, 0x5008)...)   // FLD  m[2]
	appendOp(d8Mem32(1, 0x5018)...) // FMUL v[2]
	appendOp(0xDE, 0xC1)            // FADDP
	appendOp(d8Mem32(0, 0x5020)...) // FADD bias
	appendOp(d9Mem(3, 0x6000)...)   // FSTP result

	r.compileAndRun(t, 0x1000, code...)

	if got := readF32(r, 0x6000); got != 57.0 {
		t.Errorf("kernel-shaped block result = %f, want 57.0", got)
	}
	if top := r.cpu.FPU.top(); top != 0 {
		t.Errorf("TOP after balanced block = %d, want 0", top)
	}
}

func TestX86JIT_FPU_FTST(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = -1.0
	r.cpu.FPU.setTop(0)

	// D9 E4: FTST (compare ST0 with 0.0)
	r.compileAndRun(t, 0x1000, 0xD9, 0xE4)

	if got := r.cpu.FPU.FSW & fpuCBits; got != 0x0100 {
		t.Errorf("C bits = %#x, want C0 (ST0 < 0)", got)
	}
}
