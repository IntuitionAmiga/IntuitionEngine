//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"testing"
)

// Slice 5a of native M68K FPU JIT: m68kEmitNativeFPURegToReg emits an inline
// SSE2 implementation of a 68881 register-to-register arithmetic op. These
// tests validate the *composition* (which primitives, what order, correct
// register-file displacements and precision rounding) by re-emitting the
// expected sequence from the already byte-verified low-level encoders. The
// encoders themselves are byte-checked in TestAMD64_SSEsd/SSEcvt, so a mismatch
// here pinpoints a composition bug, not an encoding one.

func TestM68KEmitNativeFPU_BinaryOps(t *testing.T) {
	// fp[dst] = fp[dst] OP fp[src]; src=2, dst=3 → disps 16 and 24.
	const src, dst = 2, 3
	ops := []struct {
		op   m68kFPUNativeOp
		rmFn func(cb *CodeBuffer, dst, base byte, disp int32)
	}{
		{m68kFPUNativeFADD, amd64ADDSD_rm},
		{m68kFPUNativeFSUB, amd64SUBSD_rm},
		{m68kFPUNativeFMUL, amd64MULSD_rm},
		{m68kFPUNativeFDIV, amd64DIVSD_rm},
	}
	for _, o := range ops {
		got := emitToBytes(func(cb *CodeBuffer) {
			if !m68kEmitNativeFPURegToReg(cb, o.op, src, dst, m68kFPURoundExtended) {
				t.Fatalf("op %v: emit returned false", o.op)
			}
		})
		want := emitToBytes(func(cb *CodeBuffer) {
			amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, m68kCtxOffFPRegsPtr)
			amd64MOVSD_load(cb, 0, amd64RAX, dst*8)
			o.rmFn(cb, 0, amd64RAX, src*8)
			amd64MOVSD_store(cb, amd64RAX, dst*8, 0)
		})
		if !bytes.Equal(got, want) {
			t.Errorf("op %v: got % X, want % X", o.op, got, want)
		}
	}
}

func TestM68KEmitNativeFPU_MOVE(t *testing.T) {
	// FMOVE: fp[dst] = fp[src] — load src, no read of dst.
	const src, dst = 1, 4
	got := emitToBytes(func(cb *CodeBuffer) {
		if !m68kEmitNativeFPURegToReg(cb, m68kFPUNativeFMOVE, src, dst, m68kFPURoundExtended) {
			t.Fatal("emit returned false")
		}
	})
	want := emitToBytes(func(cb *CodeBuffer) {
		amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, m68kCtxOffFPRegsPtr)
		amd64MOVSD_load(cb, 0, amd64RAX, src*8)
		amd64MOVSD_store(cb, amd64RAX, dst*8, 0)
	})
	if !bytes.Equal(got, want) {
		t.Errorf("FMOVE: got % X, want % X", got, want)
	}
}

func TestM68KEmitNativeFPU_SQRT(t *testing.T) {
	// FSQRT: fp[dst] = sqrt(fp[src]) — sqrtsd from src, no read of dst.
	const src, dst = 2, 5
	got := emitToBytes(func(cb *CodeBuffer) {
		if !m68kEmitNativeFPURegToReg(cb, m68kFPUNativeFSQRT, src, dst, m68kFPURoundExtended) {
			t.Fatal("emit returned false")
		}
	})
	want := emitToBytes(func(cb *CodeBuffer) {
		amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, m68kCtxOffFPRegsPtr)
		amd64SQRTSD_rm(cb, 0, amd64RAX, src*8)
		amd64MOVSD_store(cb, amd64RAX, dst*8, 0)
	})
	if !bytes.Equal(got, want) {
		t.Errorf("FSQRT: got % X, want % X", got, want)
	}
}

func TestM68KEmitNativeFPU_SinglePrecisionRounding(t *testing.T) {
	// FSADD: extended path plus cvtsd2ss/cvtss2sd before the store.
	const src, dst = 2, 3
	got := emitToBytes(func(cb *CodeBuffer) {
		if !m68kEmitNativeFPURegToReg(cb, m68kFPUNativeFADD, src, dst, m68kFPURoundSingle) {
			t.Fatal("emit returned false")
		}
	})
	want := emitToBytes(func(cb *CodeBuffer) {
		amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, m68kCtxOffFPRegsPtr)
		amd64MOVSD_load(cb, 0, amd64RAX, dst*8)
		amd64ADDSD_rm(cb, 0, amd64RAX, src*8)
		amd64CVTSD2SS_rr(cb, 0, 0)
		amd64CVTSS2SD_rr(cb, 0, 0)
		amd64MOVSD_store(cb, amd64RAX, dst*8, 0)
	})
	if !bytes.Equal(got, want) {
		t.Errorf("FSADD: got % X, want % X", got, want)
	}
}

func TestM68KEmitNativeFPU_AbsNeg(t *testing.T) {
	const src, dst = 2, 5
	for _, c := range []struct {
		op   m68kFPUNativeOp
		mask uint64
		pdFn func(cb *CodeBuffer, dst, src byte)
	}{
		{m68kFPUNativeFABS, 0x7FFFFFFFFFFFFFFF, amd64ANDPD_rr},
		{m68kFPUNativeFNEG, 0x8000000000000000, amd64XORPD_rr},
	} {
		got := emitToBytes(func(cb *CodeBuffer) {
			if !m68kEmitNativeFPURegToReg(cb, c.op, src, dst, m68kFPURoundExtended) {
				t.Fatalf("op %v: emit returned false", c.op)
			}
		})
		want := emitToBytes(func(cb *CodeBuffer) {
			amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, m68kCtxOffFPRegsPtr)
			amd64MOVSD_load(cb, 0, amd64RAX, src*8)
			amd64MOV_reg_imm64(cb, amd64RCX, c.mask)
			amd64MOVQ_xmm_reg(cb, 1, amd64RCX)
			c.pdFn(cb, 0, 1)
			amd64MOVSD_store(cb, amd64RAX, dst*8, 0)
		})
		if !bytes.Equal(got, want) {
			t.Errorf("op %v: got % X, want % X", c.op, got, want)
		}
	}
}

func TestM68KEmitNativeFPU_UnsupportedReturnsFalse(t *testing.T) {
	// Transcendentals and the sentinel None have no native implementation; emit
	// must report false so the caller falls back to the FPU helper.
	if m68kEmitNativeFPURegToReg(NewCodeBuffer(32), m68kFPUNativeNone, 0, 1, m68kFPURoundExtended) {
		t.Error("m68kFPUNativeNone: emit returned true, want false (fallback)")
	}
}
