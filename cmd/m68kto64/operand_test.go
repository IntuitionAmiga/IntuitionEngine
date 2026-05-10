package main

import (
	"fmt"
	"testing"
	"testing/quick"
)

func TestParseOperand_Direct(t *testing.T) {
	cases := []struct {
		in   string
		mode AddrMode
		ie64 string
	}{
		{"d0", AMDataReg, "r1"},
		{"D7", AMDataReg, "r8"},
		{"a0", AMAddrReg, "r9"},
		{"A6", AMAddrReg, "r15"},
		{"a7", AMAddrReg, "r30"},
		{"sp", AMAddrReg, "r30"},
		{"fp", AMAddrReg, "r15"},
	}
	for _, c := range cases {
		op, err := ParseOperand(c.in)
		if err != nil {
			t.Errorf("%q: err = %v", c.in, err)
			continue
		}
		if op.Mode != c.mode {
			t.Errorf("%q: mode = %v, want %v", c.in, op.Mode, c.mode)
		}
		if op.Reg.IE64 != c.ie64 {
			t.Errorf("%q: IE64 = %q, want %q", c.in, op.Reg.IE64, c.ie64)
		}
	}
}

func TestParseOperand_Immediate(t *testing.T) {
	cases := []struct {
		in, imm string
	}{
		{"#0", "0"},
		{"#$1234", "$1234"},
		{"#-1", "-1"},
		{"#FOO", "FOO"},
		{"#FOO+8", "FOO+8"},
	}
	for _, c := range cases {
		op, err := ParseOperand(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if op.Mode != AMImmediate {
			t.Errorf("%q: mode = %v", c.in, op.Mode)
		}
		if op.Imm != c.imm {
			t.Errorf("%q: imm = %q, want %q", c.in, op.Imm, c.imm)
		}
	}
}

func TestParseOperand_Indirect(t *testing.T) {
	op, err := ParseOperand("(a0)")
	if err != nil || op.Mode != AMIndirect || op.Reg.IE64 != "r9" {
		t.Errorf("(a0): mode=%v reg=%q err=%v", op.Mode, op.Reg.IE64, err)
	}
	op, _ = ParseOperand("(sp)")
	if op.Mode != AMIndirect || op.Reg.IE64 != "r30" {
		t.Errorf("(sp): mode=%v reg=%q", op.Mode, op.Reg.IE64)
	}
}

func TestParseOperand_PostInc(t *testing.T) {
	op, err := ParseOperand("(a3)+")
	if err != nil || op.Mode != AMPostInc || op.Reg.IE64 != "r12" {
		t.Errorf("(a3)+: mode=%v reg=%q err=%v", op.Mode, op.Reg.IE64, err)
	}
	op, _ = ParseOperand("(sp)+")
	if op.Mode != AMPostInc || op.Reg.IE64 != "r30" {
		t.Errorf("(sp)+: mode=%v reg=%q", op.Mode, op.Reg.IE64)
	}
}

func TestParseOperand_PreDec(t *testing.T) {
	op, err := ParseOperand("-(a4)")
	if err != nil || op.Mode != AMPreDec || op.Reg.IE64 != "r13" {
		t.Errorf("-(a4): mode=%v reg=%q err=%v", op.Mode, op.Reg.IE64, err)
	}
	op, _ = ParseOperand("-(sp)")
	if op.Mode != AMPreDec || op.Reg.IE64 != "r30" {
		t.Errorf("-(sp): mode=%v reg=%q", op.Mode, op.Reg.IE64)
	}
}

func TestParseOperand_DispAn(t *testing.T) {
	for _, in := range []string{"(8,a0)", "8(a0)"} {
		op, err := ParseOperand(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if op.Mode != AMDispAn || op.Reg.IE64 != "r9" {
			t.Errorf("%q: mode=%v reg=%q", in, op.Mode, op.Reg.IE64)
		}
		if op.Disp != "8" {
			t.Errorf("%q: disp=%q", in, op.Disp)
		}
	}
	op, _ := ParseOperand("(FOO,a6)")
	if op.Mode != AMDispAn || op.Disp != "FOO" || op.Reg.IE64 != "r15" {
		t.Errorf("(FOO,a6): %+v", op)
	}
}

func TestParseOperand_IndexAn(t *testing.T) {
	cases := []struct {
		in        string
		disp      string
		base      string
		idx       string
		idxSize   string
		idxScale  int
	}{
		{"(8,a0,d1.w*4)", "8", "r9", "r2", "w", 4},
		{"8(a0,d1.w*4)", "8", "r9", "r2", "w", 4},
		{"(0,a0,d2.l)", "0", "r9", "r3", "l", 1},
		{"(a0,d3)", "", "r9", "r4", "w", 1},
	}
	for _, c := range cases {
		op, err := ParseOperand(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if op.Mode != AMIndexAn {
			t.Errorf("%q: mode=%v want AMIndexAn", c.in, op.Mode)
			continue
		}
		if op.Reg.IE64 != c.base {
			t.Errorf("%q: base=%q want %q", c.in, op.Reg.IE64, c.base)
		}
		if op.Disp != c.disp {
			t.Errorf("%q: disp=%q want %q", c.in, op.Disp, c.disp)
		}
		if op.Index.Reg.IE64 != c.idx {
			t.Errorf("%q: idx=%q want %q", c.in, op.Index.Reg.IE64, c.idx)
		}
		if op.Index.Size != c.idxSize {
			t.Errorf("%q: idxsize=%q want %q", c.in, op.Index.Size, c.idxSize)
		}
		if op.Index.Scale != c.idxScale {
			t.Errorf("%q: idxscale=%d want %d", c.in, op.Index.Scale, c.idxScale)
		}
	}
}

func TestParseOperand_PCRel(t *testing.T) {
	op, err := ParseOperand("(8,pc)")
	if err != nil {
		t.Fatal(err)
	}
	if op.Mode != AMDispPC || op.Reg.Class != RegPC || op.Disp != "8" {
		t.Errorf("(8,pc): %+v", op)
	}
	op, _ = ParseOperand("(8,pc,d0.w)")
	if op.Mode != AMIndexPC || op.Reg.Class != RegPC {
		t.Errorf("(8,pc,d0.w): %+v", op)
	}
	op, _ = ParseOperand("8(pc)")
	if op.Mode != AMDispPC || op.Disp != "8" {
		t.Errorf("8(pc): %+v", op)
	}
}

func TestParseOperand_Absolute(t *testing.T) {
	cases := []struct {
		in   string
		mode AddrMode
		disp string
	}{
		{"($1000).w", AMAbsW, "$1000"},
		{"($12345678).l", AMAbsL, "$12345678"},
		{"FOO", AMAbsL, "FOO"},
		{"FOO+8", AMAbsL, "FOO+8"},
		{"$F2000", AMAbsL, "$F2000"},
	}
	for _, c := range cases {
		op, err := ParseOperand(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if op.Mode != c.mode {
			t.Errorf("%q: mode=%v want %v", c.in, op.Mode, c.mode)
		}
		if op.Disp != c.disp {
			t.Errorf("%q: disp=%q want %q", c.in, op.Disp, c.disp)
		}
	}
}

func TestParseOperand_RegList(t *testing.T) {
	for _, in := range []string{"d0-d7/a0-a6", "d2/d4", "a0-a3", "d0/a0"} {
		op, err := ParseOperand(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if op.Mode != AMRegList {
			t.Errorf("%q: mode=%v want AMRegList", in, op.Mode)
		}
		if op.List != in {
			t.Errorf("%q: list=%q", in, op.List)
		}
	}
}

func TestParseOperand_CCRSR(t *testing.T) {
	for in, want := range map[string]AddrMode{
		"ccr": AMCCR,
		"sr":  AMSR,
		"usp": AMUSP,
	} {
		op, err := ParseOperand(in)
		if err != nil || op.Mode != want {
			t.Errorf("%q: mode=%v err=%v want %v", in, op.Mode, err, want)
		}
	}
}

func TestParseOperand_Errors(t *testing.T) {
	for _, in := range []string{"", "#", "-(d0)", "(d0)+"} {
		if _, err := ParseOperand(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

// Property test: every operand string we generate from a known shape must
// parse back to the same Mode classification.
func TestParseOperand_RoundtripProperty(t *testing.T) {
	gen := func(seed int) (string, AddrMode) {
		// Pick a shape from a small set; seed picks within.
		shapes := []struct {
			fmt  string
			mode AddrMode
		}{
			{"d%d", AMDataReg},
			{"a%d", AMAddrReg},
			{"(a%d)", AMIndirect},
			{"(a%d)+", AMPostInc},
			{"-(a%d)", AMPreDec},
			{"%d(a0)", AMDispAn},
			{"#%d", AMImmediate},
		}
		s := shapes[seed%len(shapes)]
		idx := seed % 8
		return fmt.Sprintf(s.fmt, idx), s.mode
	}
	f := func(seed uint16) bool {
		in, want := gen(int(seed))
		op, err := ParseOperand(in)
		if err != nil {
			t.Logf("parse %q: %v", in, err)
			return false
		}
		return op.Mode == want
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
