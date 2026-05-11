package main

import (
	"strings"
	"testing"
)

// MOVEP — Phase F5. Strided byte transfer between Dn and d(An), stride 2.

// Lexer must recognise movep as its own mnemonic, not eaten as `move`+suffix.
func TestMOVEP_LexerRecognises(t *testing.T) {
	l := LexLine("\tmovep.w d0,$10(a0)")
	if l.Kind != LineInstruction {
		t.Fatalf("movep lexed as %v, want LineInstruction", l.Kind)
	}
	if l.Mnemonic != "movep" {
		t.Errorf("mnemonic=%q want movep", l.Mnemonic)
	}
	if l.Size != ".w" {
		t.Errorf("size=%q want .w", l.Size)
	}
	if len(l.Operands) != 2 {
		t.Errorf("operands=%v want 2", l.Operands)
	}
}

func TestMOVEP_W_Store(t *testing.T) {
	out := convertOneInstr(t, "\tmovep.w d0,$10(a0)")
	mustContain(t, out, "lea r16, $10(r9)") // EA = a0+disp
	mustContain(t, out, "lsr.l r17, r1, #8")
	mustContain(t, out, "store.b r17, 0(r16)")
	mustContain(t, out, "store.b r1, 2(r16)") // low byte stored at stride 2
}

func TestMOVEP_L_Store(t *testing.T) {
	out := convertOneInstr(t, "\tmovep.l d0,$10(a0)")
	mustContain(t, out, "lsr.l r17, r1, #24")
	mustContain(t, out, "store.b r17, 0(r16)")
	mustContain(t, out, "lsr.l r17, r1, #16")
	mustContain(t, out, "store.b r17, 2(r16)")
	mustContain(t, out, "lsr.l r17, r1, #8")
	mustContain(t, out, "store.b r17, 4(r16)")
	mustContain(t, out, "store.b r1, 6(r16)")
}

func TestMOVEP_W_Load(t *testing.T) {
	out := convertOneInstr(t, "\tmovep.w $10(a0),d0")
	mustContain(t, out, "load.b r17, 0(r16)")
	mustContain(t, out, "lsl.l r17, r17, #8")
	mustContain(t, out, "load.b r18, 2(r16)")
	// Result merged into low 16 of d0 (r1), upper preserved.
	mustContain(t, out, "and.l r1, r1, #$FFFF0000")
	mustContain(t, out, "or.l r1, r1, r17")
}

func TestMOVEP_L_Load(t *testing.T) {
	out := convertOneInstr(t, "\tmovep.l $10(a0),d0")
	// 4 byte loads, full overwrite.
	mustContain(t, out, "move.l r1, #0")
	mustContain(t, out, "load.b r17, 0(r16)")
	mustContain(t, out, "load.b r17, 2(r16)")
	mustContain(t, out, "load.b r17, 4(r16)")
	mustContain(t, out, "load.b r17, 6(r16)")
	// Highest-byte shift.
	mustContain(t, out, "lsl.l r17, r17, #24")
}

func TestMOVEP_RejectsNonDispAn(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	out, errs := c.ConvertSource("\tmovep.w d0,(a0)\n")
	if errs == 0 || !strings.Contains(out, "ERROR") {
		t.Errorf("MOVEP with bare (a0) should error; got:\n%s", out)
	}
}

func TestMOVEP_StrictMode_NoError(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	out, errs := c.ConvertSource("\tmovep.w d0,$10(a0)\n")
	if errs != 0 {
		t.Errorf("-strict should accept MOVEP (exact lowering); got %d errs:\n%s", errs, out)
	}
}
