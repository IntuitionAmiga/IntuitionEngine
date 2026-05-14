package main

import (
	"strings"
	"testing"
)

// More coverage: MOVEM other modes, indexed PC, shift mem RMW, fused signed
// branches, PACK/UNPK predec.

func TestMovem_LoadDispAn(t *testing.T) {
	out := convertOneInstr(t, "\tmovem.l 8(a0),d0-d2")
	mustContain(t, out, "lea r16, 8(r9)")
	mustContain(t, out, "load.l r1, (r16)")
	mustContain(t, out, "bswap.l r1, r1")
}

func TestMovem_StoreDispAn(t *testing.T) {
	out := convertOneInstr(t, "\tmovem.l d0-d2,8(a0)")
	mustContain(t, out, "lea r16, 8(r9)")
	mustContain(t, out, "bswap.l r20, r1")
	mustContain(t, out, "store.l r20, (r16)")
}

func TestMovem_LoadAbs(t *testing.T) {
	out := convertOneInstr(t, "\tmovem.l $F2000,d0-d1")
	mustContain(t, out, "la r16, $F2000")
}

func TestMovem_LoadWord_SignExtend(t *testing.T) {
	out := convertOneInstr(t, "\tmovem.w (sp)+,d0-d1")
	mustContain(t, out, "load.w")
	mustContain(t, out, "sext.w")
}

func TestEABase_Indirect(t *testing.T) {
	out := convertOneInstr(t, "\tmovem.l d0/d1,(a0)")
	// Indirect base — copy An into ScrEA.
	mustContain(t, out, "move.l r16, r9")
}

func TestEABase_PCRel(t *testing.T) {
	out := convertOneInstr(t, "\tmovem.l label(pc),d0/d1")
	mustContain(t, out, "la r16, label")
}

func TestMove_IndexPC(t *testing.T) {
	// Forces emitIndexCombine via PC-rel indexed source.
	out := convertOneInstr(t, "\tmove.l (label,pc,d0.l*2),d1")
	mustContain(t, out, "la r16, label")
	mustContain(t, out, "lsl.l r19, r19, #1") // index combine
}

func TestShift_Mem_RMW(t *testing.T) {
	out := convertOneInstr(t, "\tlsl.l #1,(a0)")
	mustContain(t, out, "load.l r18, (r9)")
	mustContain(t, out, "lsl.l r18, r18, #1")
	mustContain(t, out, "bswap.l r20, r18")
	mustContain(t, out, "store.l r20, (r9)")
}

func TestFused_CmpBgt_Word(t *testing.T) {
	out := convertSrc(t, "\tcmp.w d0,d1\n\tbgt L\nL:\n\trts\n")
	mustContain(t, out, "bgt r18, r17, L")
}

func TestFused_CmpBge(t *testing.T) {
	out := convertSrc(t, "\tcmp.l d0,d1\n\tbge L\nL:\n\trts\n")
	mustContain(t, out, "bge")
}

func TestFused_CmpBle(t *testing.T) {
	out := convertSrc(t, "\tcmp.l d0,d1\n\tble L\nL:\n\trts\n")
	mustContain(t, out, "ble")
}

func TestPack_PreDec(t *testing.T) {
	out := convertOneInstr(t, "\tpack -(a0),-(a1),#$0000")
	mustContain(t, out, "load.b r19, (r9)")
	mustContain(t, out, "store.b r19, (r10)")
}

func TestUnpk_PreDec(t *testing.T) {
	out := convertOneInstr(t, "\tunpk -(a0),-(a1),#$0000")
	mustContain(t, out, "load.b r19, (r9)")
	mustContain(t, out, "store.b r19, (r10)")
}

func TestParseOperand_AddrMode_String(t *testing.T) {
	// Smoke: every AddrMode.String() returns a non-empty label.
	for m := AMUnknown; m <= AMUSP; m++ {
		s := m.String()
		if s == "" {
			t.Errorf("AddrMode(%d).String() returned empty", m)
		}
	}
}

func TestEmitBccLine_FullCoverage(t *testing.T) {
	// Hit every kind via fused CMP+Bcc.
	for _, m := range []string{"beq", "bne", "blt", "bge", "bgt", "ble", "bhi", "bls"} {
		out := convertSrc(t, "\tcmp.l d0,d1\n\t"+m+" L\nL:\n\trts\n")
		if strings.Contains(out, "ERROR") {
			t.Errorf("%s: error\n%s", m, out)
		}
	}
}
