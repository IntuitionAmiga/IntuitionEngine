package main

import (
	"strings"
	"testing"
)

// (xxx).w short-absolute sign-extension — Phase F2.
//
// m68k (xxx).w forms a 16-bit signed address sign-extended to 32 bits before
// dereferencing. IE64 `la` zero-extends. Without an explicit sext.w, address
// like $FFFE.w would resolve to +65534 instead of -2 / 0xFFFFFFFE.
//
// Wrapper helper: maybeSignExtAbsW emits `sext.w reg, reg` after every `la
// reg, op.Disp` when op.Mode == AMAbsW. No-op for AMAbsL.

const signExtAnchor = "(xxx).w sign-extend"

// loadValue.b through the AMAbsW branch. Uses ($FFFE).w parser form
// (parenthesised — vasm-style bare `$FFFE.w` is not currently recognised
// as AMAbsW; see §12).
func TestAbsW_LoadByte_SignExtends(t *testing.T) {
	out := convertOneInstr(t, "\tmove.b ($FFFE).w,d0")
	mustContain(t, out, "la r16, $FFFE")
	mustContain(t, out, "sext.w r16, r16")
	mustContain(t, out, signExtAnchor)
}

// storeValue.l through the AMAbsW branch.
func TestAbsW_StoreLong_SignExtends(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l d0,($8000).w")
	mustContain(t, out, "la r16, $8000")
	mustContain(t, out, "sext.w r16, r16")
}

// AMAbsL must NOT sign-extend (positive sanity).
func TestAbsL_NoSignExtend(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l d0,($8000).l")
	mustNotContain(t, out, "sext.w r16, r16")
}

// FP memory load through AMAbsW.
func TestAbsW_FPMemLoad(t *testing.T) {
	out := convertOneInstr(t, "\tfmove.s ($FFFE).w,fp1")
	mustContain(t, out, "sext.w r16, r16")
}

// LEA writes the sign-extended address into An. LEA dst is An so the helper
// fires on the destination register.
func TestAbsW_LEA_SignExtends(t *testing.T) {
	out := convertOneInstr(t, "\tlea ($FFFE).w,a0")
	if !strings.Contains(out, "sext.w") {
		t.Errorf("LEA (xxx).w must sign-extend; got:\n%s", out)
	}
}

// JSR numeric (xxx).w: emitJsr's AMAbsW path uses la + sext.w + jmp
// (indirect) for numeric Disp, so the m68k jump target sign-extends
// correctly. Symbolic labels still resolve via `bra` because ie64asm
// patches them at assemble time and the address is already correct.
func TestAbsW_JSR_SignExtends(t *testing.T) {
	out := convertOneInstr(t, "\tjsr ($FFFE).w")
	mustContain(t, out, "la r17, $FFFE")
	mustContain(t, out, "sext.w r17, r17")
	mustContain(t, out, "jmp (r17)")
}

// JSR with symbolic label stays on the bra path — no sext.w needed.
func TestAbsW_JSR_LabelStaysBra(t *testing.T) {
	out := convertOneInstr(t, "\tjsr myfunc")
	if strings.Contains(out, "sext.w") {
		t.Errorf("JSR with label should not trigger sext.w; got:\n%s", out)
	}
}

// JMP numeric (xxx).w: matching JSR fix in emitJmp.
func TestAbsW_JMP_SignExtends(t *testing.T) {
	out := convertOneInstr(t, "\tjmp ($FFFE).w")
	mustContain(t, out, "la r17, $FFFE")
	mustContain(t, out, "sext.w r17, r17")
	mustContain(t, out, "jmp (r17)")
}

// PEA: no transpiler handler today — falls through to passthrough. PEA
// (xxx).w sign-extension is a separate gap that requires a real emitPea
// lowering first; tracked in §12.
//
// (Stub test left as a forward marker — re-enable when emitPea lands.)
