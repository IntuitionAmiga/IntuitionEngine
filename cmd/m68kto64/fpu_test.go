package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Phase 7.2 — FMOVE / FMOVEM / FMOVECR
// =====================================================================

func TestFMove_FPmFPn(t *testing.T) {
	out := convertSrc(t, "\tfmove.x fp1,fp2\n")
	mustContain(t, out, "dmov f4, f2")
}

func TestFMove_S_LoadFromMem(t *testing.T) {
	out := convertSrc(t, "\tfmove.s (a0),fp0\n")
	mustContain(t, out, "fload f0, (r9)")
}

func TestFMove_D_LoadFromMem(t *testing.T) {
	out := convertSrc(t, "\tfmove.d (a0),fp0\n")
	mustContain(t, out, "dload f0, (r9)")
}

func TestFMove_X_DegradesToD(t *testing.T) {
	out := convertSrc(t, "\tfmove.x (a0),fp0\n")
	mustContain(t, out, ".X degraded to .D")
	mustContain(t, out, "dload f0, (r9)")
}

func TestFMove_S_StoreToMem(t *testing.T) {
	out := convertSrc(t, "\tfmove.s fp1,(a1)\n")
	mustContain(t, out, "fstore f2, (r10)")
}

func TestFMove_L_IntToFP(t *testing.T) {
	out := convertSrc(t, "\tfmove.l d0,fp0\n")
	mustContain(t, out, "dcvtif f0, r1")
}

func TestFMove_W_IntSignExtend(t *testing.T) {
	out := convertSrc(t, "\tfmove.w d0,fp0\n")
	mustContain(t, out, "sext.w")
	mustContain(t, out, "dcvtif f0,")
}

func TestFMove_L_FPToInt(t *testing.T) {
	out := convertSrc(t, "\tfmove.l fp0,d1\n")
	mustContain(t, out, "dcvtfi r17, f0")
}

func TestFMove_PostInc_StepSize_S(t *testing.T) {
	out := convertSrc(t, "\tfmove.s (a0)+,fp0\n")
	mustContain(t, out, "fload f0, (r9)")
	mustContain(t, out, "add.l r9, r9, #4")
}

func TestFMove_PostInc_StepSize_D(t *testing.T) {
	out := convertSrc(t, "\tfmove.d (a0)+,fp0\n")
	mustContain(t, out, "dload f0, (r9)")
	mustContain(t, out, "add.l r9, r9, #8")
}

func TestFMove_PreDec_StepSize_D(t *testing.T) {
	out := convertSrc(t, "\tfmove.d -(a0),fp0\n")
	mustContain(t, out, "sub.l r9, r9, #8")
	mustContain(t, out, "dload f0, (r9)")
}

func TestFMove_FPSR_To_Dn_Composes(t *testing.T) {
	out := convertSrc(t, "\tfmove.l fpsr,d0\n")
	mustContain(t, out, "fmovsr r17")
	mustContain(t, out, "lsl.l r18, r29, #24")
	mustContain(t, out, "or.l r1, r17, r18")
}

func TestFMove_Dn_To_FPSR_Splits(t *testing.T) {
	out := convertSrc(t, "\tfmove.l d0,fpsr\n")
	mustContain(t, out, "lsr.l r17, r1, #24")
	mustContain(t, out, "and.l r29, r17, #$F")
	mustContain(t, out, "fmovsc r1")
}

func TestFMove_FPCR_DirectAccess(t *testing.T) {
	out := convertSrc(t, "\tfmove.l fpcr,d0\n")
	mustContain(t, out, "fmovcr r1")
	out2 := convertSrc(t, "\tfmove.l d0,fpcr\n")
	mustContain(t, out2, "fmovcc r1")
}

func TestFMove_FPIAR_ReturnsZero(t *testing.T) {
	out := convertSrc(t, "\tfmove.l fpiar,d0\n")
	mustContain(t, out, "FPIAR read returns 0")
}

func TestFMove_FPIAR_WriteIgnored(t *testing.T) {
	out := convertSrc(t, "\tfmove.l d0,fpiar\n")
	mustContain(t, out, "FPIAR write ignored")
}

func TestFMoveCR_ROMOffset_IE64ROMHit(t *testing.T) {
	out := convertSrc(t, "\tfmovecr fp0,#$00\n") // Pi
	mustContain(t, out, "fmovecr f0, #0")
}

func TestFMoveCR_ROMOffset_ConstPoolFallback(t *testing.T) {
	out := convertSrc(t, "\tfmovecr fp1,#$32\n") // 1.0
	mustContain(t, out, "dload f2,")
	mustContain(t, out, "__m68kto64_fp_const_pool_")
	mustContain(t, out, "dc.d 1.0")
}

func TestFMoveCR_UnknownOffset_Substitutes_Zero(t *testing.T) {
	out := convertSrc(t, "\tfmovecr fp0,#$7F\n")
	mustContain(t, out, "substituted 0.0")
}

func TestFMovem_StoreList_PreDec_ReverseOrder(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x fp0/fp2,-(a0)\n")
	mustContain(t, out, "sub.l r9, r9, #8")
	mustContain(t, out, "dstore f4, (r9)") // FP2 stored first (highest)
	mustContain(t, out, "dstore f0, (r9)") // FP0 stored last
}

func TestFMovem_LoadList_PostInc(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x (a0)+,fp0/fp2\n")
	mustContain(t, out, "dload f0, (r9)")
	mustContain(t, out, "add.l r9, r9, #8")
	mustContain(t, out, "dload f4, (r9)")
}

// =====================================================================
// Phase 7.3 — FADD/FSUB/FMUL/FDIV/FMOD/FNEG/FABS/FSQRT/FINT/FINTRZ/FSCALE
// =====================================================================

func TestFAdd_FPmFPn(t *testing.T) {
	out := convertSrc(t, "\tfadd.x fp1,fp2\n")
	mustContain(t, out, "dadd f4, f4, f2")
}

func TestFSub_FPmFPn(t *testing.T) {
	out := convertSrc(t, "\tfsub.x fp1,fp2\n")
	mustContain(t, out, "dsub f4, f4, f2")
}

func TestFMul_FPmFPn(t *testing.T) {
	out := convertSrc(t, "\tfmul.x fp1,fp2\n")
	mustContain(t, out, "dmul f4, f4, f2")
}

func TestFDiv_FPmFPn(t *testing.T) {
	out := convertSrc(t, "\tfdiv.x fp1,fp2\n")
	mustContain(t, out, "ddiv f4, f4, f2")
}

func TestFMod_FPmFPn(t *testing.T) {
	out := convertSrc(t, "\tfmod.x fp1,fp2\n")
	mustContain(t, out, "dmod f4, f4, f2")
}

func TestFSglMul_SinglePrecisionNative(t *testing.T) {
	out := convertSrc(t, "\tfsglmul.s fp0,fp1\n")
	mustContain(t, out, "fmul f2, f2,")
}

func TestFNeg_OneOperand(t *testing.T) {
	out := convertSrc(t, "\tfneg.x fp0\n")
	mustContain(t, out, "dneg f0, f0")
}

func TestFAbs_TwoOperand(t *testing.T) {
	out := convertSrc(t, "\tfabs.x fp1,fp2\n")
	mustContain(t, out, "dabs f4, f2")
}

func TestFSqrt_TwoOperand(t *testing.T) {
	out := convertSrc(t, "\tfsqrt.x fp0,fp1\n")
	mustContain(t, out, "dsqrt f2, f0")
}

func TestFInt_UsesFPCR(t *testing.T) {
	out := convertSrc(t, "\tfint.x fp0,fp1\n")
	mustContain(t, out, "dint f2, f0")
}

func TestFIntrz_SaveRestoreFPCR(t *testing.T) {
	out := convertSrc(t, "\tfintrz.x fp0,fp1\n")
	mustContain(t, out, "fmovcr r17")
	mustContain(t, out, "__m68kto64_fpcr_save")
	mustContain(t, out, "round-toward-zero")
	mustContain(t, out, "dint f2, f0")
	mustContain(t, out, "fmovcc r17")
}

func TestFScale_BitPatternRoundTrip(t *testing.T) {
	out := convertSrc(t, "\tfscale.x fp0,fp1\n")
	mustContain(t, out, "dcvtfi r17, f0")
	mustContain(t, out, "add.l r18, r17, #1023")
	mustContain(t, out, "lsl.q r18, r18, #52")
	mustContain(t, out, "__m68kto64_fp_scratch_q")
	mustContain(t, out, "dmul f2, f2, f10")
}

// =====================================================================
// Phase 7.4 — FCMP / FTST / FBcc / FDBcc / FScc / FTRAPcc + ShadowFPCC
// =====================================================================

func TestFCmp_WritesShadowFPCC_WhenLive(t *testing.T) {
	// fcmp without an adjacent FBcc-fusable consumer + an intervening label
	// → fuse suppressed, liveness sees the FBcc downstream, shadow emitted.
	out := convertSrc(t, "\tfcmp.x fp1,fp2\nL:\n\tfbeq target\n")
	mustContain(t, out, "dcmp r17, f4, f2")
	mustContain(t, out, "or.l r29,") // shadow update
}

func TestFCmp_ShadowElided_WhenDead(t *testing.T) {
	// fcmp with no downstream cc consumer → liveness elides shadow.
	out := convertSrc(t, "\tfcmp.x fp1,fp2\n")
	mustContain(t, out, "dcmp r17, f4, f2")
	if strings.Contains(out, "; bit2 (Z)") {
		t.Errorf("dead fcmp should not emit shadow:\n%s", out)
	}
}

func TestFTst_AgainstZero(t *testing.T) {
	out := convertSrc(t, "\tftst.x fp0\n")
	// FTST loads the zero constant into f12 (ScrFP2) — fp0 already lives in f0.
	mustContain(t, out, "dload f12,")
	mustContain(t, out, "dcmp r17, f0, f12")
}

func TestFBeq_Standalone_TestsZBit(t *testing.T) {
	out := convertSrc(t, "\tfbeq target\n")
	// Z bit extraction (bit 2 of ShadowFPCC=r29) lands in ShadowTmp2 (r23).
	mustContain(t, out, "lsr.l r23, r29, #2")
	mustContain(t, out, "bnez r17, target")
}

func TestFBst_AlwaysTaken(t *testing.T) {
	out := convertSrc(t, "\tfbst target\n")
	mustContain(t, out, "bra target")
}

func TestFBsf_NeverTaken(t *testing.T) {
	out := convertSrc(t, "\tfbsf target\n")
	mustContain(t, out, "never taken")
}

func TestFBseq_RaisesIOFlag(t *testing.T) {
	out := convertSrc(t, "\tfbseq target\n")
	// Read-modify-write of FPSR to set IO bit (bit 0).
	mustContain(t, out, "fmovsr")
	mustContain(t, out, "or.l r18, r18, #1")
	mustContain(t, out, "fmovsc r18")
}

func TestFDBeq_DecrementAndBranch(t *testing.T) {
	out := convertSrc(t, "\tfdbeq d0,target\n")
	mustContain(t, out, "sub.l r1, r1, #1")
	mustContain(t, out, "bne ") // branch when Dn != -1
}

func TestFSeq_StoresByte(t *testing.T) {
	out := convertSrc(t, "\tfseq (a0)\n")
	// Result expanded to 0x00/0xFF and stored as byte.
	mustContain(t, out, "neg.l r17,")
	mustContain(t, out, "store.b r17, (r9)")
}

func TestFTrapeq_Syscall33(t *testing.T) {
	out := convertSrc(t, "\tftrapeq\n")
	mustContain(t, out, "syscall #33")
}

func TestFTrap_F_Syscall32(t *testing.T) {
	out := convertSrc(t, "\tftrapf\n")
	mustContain(t, out, "syscall #32")
}

// =====================================================================
// Phase 7.5 — Transcendentals
// =====================================================================

func TestFSin_Direct(t *testing.T) {
	out := convertSrc(t, "\tfsin.x fp0,fp1\n")
	mustContain(t, out, "fsin f2, f2")
}

func TestFCos_Direct(t *testing.T) {
	out := convertSrc(t, "\tfcos.x fp0,fp1\n")
	mustContain(t, out, "fcos f2, f2")
}

func TestFEtox_FExp(t *testing.T) {
	out := convertSrc(t, "\tfetox.x fp0,fp1\n")
	mustContain(t, out, "fexp f2, f2")
}

func TestFLogn_FLog(t *testing.T) {
	out := convertSrc(t, "\tflogn.x fp0,fp1\n")
	mustContain(t, out, "flog f2, f2")
}

func TestFLog10_DividesByLn10(t *testing.T) {
	out := convertSrc(t, "\tflog10.x fp0,fp1\n")
	mustContain(t, out, "ln(10)")
	mustContain(t, out, "ddiv f2, f2, f10")
}

func TestFSinh_HyperbolicIdentity(t *testing.T) {
	out := convertSrc(t, "\tfsinh.x fp0,fp1\n")
	mustContain(t, out, "dneg f12,")
	mustContain(t, out, "dsub f2, f10, f12")
	mustContain(t, out, "0.5")
}

func TestFAtanh_HalfLogIdentity(t *testing.T) {
	out := convertSrc(t, "\tfatanh.x fp0,fp1\n")
	mustContain(t, out, "dadd f12, f10,") // 1+x
	mustContain(t, out, "dsub f10, f10,") // 1-x
	mustContain(t, out, "ddiv f2, f12, f10")
	mustContain(t, out, "flog f2, f2")
	mustContain(t, out, "0.5")
}

func TestFTwotox_ScaledExp(t *testing.T) {
	out := convertSrc(t, "\tftwotox.x fp0,fp1\n")
	mustContain(t, out, "ln(2)")
	mustContain(t, out, "fexp f2, f2")
}

// =====================================================================
// Phase 7.6 — FNOP / FSAVE / FRESTORE / .P
// =====================================================================

func TestFNop_Comment(t *testing.T) {
	out := convertSrc(t, "\tfnop\n")
	mustContain(t, out, "nop (FPU)")
}

// FSAVE / FRESTORE are now fully lowered (transpiler-private 80-byte state
// frame, magic-verified). The "stripped" diagnostic test is obsolete and
// replaced by the comprehensive coverage in fsave_test.go.
//
// FSAVE under -strict no longer errors — the full lowering covers the
// single-context user-mode case without semantic loss.

func TestFMove_P_UnsupportedDiag(t *testing.T) {
	out := convertSrc(t, "\tfmove.p fp0,(a0)\n")
	mustContain(t, out, ".P unsupported")
}

func TestFMove_P_StrictErrors(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	_, errs := c.ConvertSource("\tfmove.p fp0,(a0)\n")
	if errs == 0 {
		t.Errorf("strict .P should error")
	}
}

// =====================================================================
// FPU footer — memory slots and constant pool emit when used.
// =====================================================================

func TestFPFooter_EmitsSlots_WhenFPUsed(t *testing.T) {
	out := convertSrc(t, "\tfadd.x fp0,fp1\n")
	mustContain(t, out, "__m68kto64_fpcr_save:")
	mustContain(t, out, "__m68kto64_fp_scratch_q:")
	mustContain(t, out, "; ---- m68kto64 FPU footer ----")
}

func TestFPFooter_NotEmittedForIntegerOnly(t *testing.T) {
	out := convertSrc(t, "\tmove.l #1,d0\n")
	if strings.Contains(out, "FPU footer") {
		t.Errorf("integer-only program should not emit FPU footer:\n%s", out)
	}
}

// =====================================================================
// Locked syscall # mapping (full FTRAPcc range).
// =====================================================================

func TestFTrapcc_AllCCKindsAssignedLockedSyscallNumbers(t *testing.T) {
	for i, cc := range fpFTrapccOrder {
		num, ok := fpFTrapccSyscall(cc)
		if !ok || num != 32+i {
			t.Errorf("fpFTrapccSyscall(%q) = (%d,%v), want (%d,true)", cc, num, ok, 32+i)
		}
	}
}
