package main

import (
	"reflect"
	"testing"
)

func TestSplitComment(t *testing.T) {
	cases := []struct {
		in, code, comm string
	}{
		{"  move.l d0,d1 ; copy", "  move.l d0,d1", "copy"},
		{"move.l #';',d0", "move.l #';',d0", ""},                    // ';' inside string literal
		{`move.l #";",d0 ; trailing`, `move.l #";",d0`, "trailing"}, // double-quoted
		{"* whole line", "", "whole line"},
		{"   * indented star is comment", "", "indented star is comment"},
		{"x * y", "x * y", ""}, // mid-line star is multiplication, not comment
		{"", "", ""},
		{"label:", "label:", ""},
	}
	for _, c := range cases {
		gotCode, gotComm := SplitComment(c.in)
		if gotCode != c.code || gotComm != c.comm {
			t.Errorf("SplitComment(%q) = (%q,%q), want (%q,%q)", c.in, gotCode, gotComm, c.code, c.comm)
		}
	}
}

func TestSplitOperands(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"d0", []string{"d0"}},
		{"d0,d1", []string{"d0", "d1"}},
		{"#$1234,(a0,d1.w*4)", []string{"#$1234", "(a0,d1.w*4)"}},
		{"d0/d1/d3-d5,-(sp)", []string{"d0/d1/d3-d5", "-(sp)"}},
		{`"hello,world",d0`, []string{`"hello,world"`, "d0"}},
		{"  d0  ,  d1  ", []string{"d0", "d1"}},
	}
	for _, c := range cases {
		got := SplitOperands(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitOperands(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestSplitMnemonicSize(t *testing.T) {
	cases := []struct {
		in, mnem, size string
	}{
		{"move", "move", ""},
		{"MOVE", "move", ""},
		{"move.l", "move", ".l"},
		{"move.W", "move", ".w"},
		{"add.b", "add", ".b"},
		{"bra.s", "bra", ".s"},
		{"jsr", "jsr", ""},
		{"dc.l", "dc", ".l"},
		// FPU size suffixes (Phase 7.1) — single-letter, accepted.
		{"fmove.d", "fmove", ".d"},
		{"fmove.x", "fmove", ".x"},
		{"fmove.p", "fmove", ".p"},
	}
	for _, c := range cases {
		m, s := SplitMnemonicSize(c.in)
		if m != c.mnem || s != c.size {
			t.Errorf("SplitMnemonicSize(%q) = (%q,%q), want (%q,%q)", c.in, m, s, c.mnem, c.size)
		}
	}
}

func TestLexLine_Empty(t *testing.T) {
	for _, in := range []string{"", "   ", "\t", "  \t  "} {
		l := LexLine(in)
		if l.Kind != LineEmpty {
			t.Errorf("%q: kind = %v, want LineEmpty", in, l.Kind)
		}
	}
}

func TestLexLine_CommentOnly(t *testing.T) {
	for _, in := range []string{"; just a comment", "* whole line comment", "  ; indented"} {
		l := LexLine(in)
		if l.Kind != LineComment {
			t.Errorf("%q: kind = %v, want LineComment", in, l.Kind)
		}
		if l.Comment == "" {
			t.Errorf("%q: empty comment", in)
		}
	}
}

func TestLexLine_LabelOnly(t *testing.T) {
	for _, in := range []string{"label:", "foo", "loop:    ; tag"} {
		l := LexLine(in)
		if l.Kind != LineLabelOnly {
			t.Errorf("%q: kind = %v, want LineLabelOnly", in, l.Kind)
		}
		if l.Label == "" {
			t.Errorf("%q: empty label", in)
		}
	}
}

func TestLexLine_Instruction(t *testing.T) {
	cases := []struct {
		in    string
		label string
		mnem  string
		size  string
		ops   []string
	}{
		{"  move.l d0,d1", "", "move", ".l", []string{"d0", "d1"}},
		{"loop:  add.w #1,d0", "loop", "add", ".w", []string{"#1", "d0"}},
		{"\tjsr (a0)", "", "jsr", "", []string{"(a0)"}},
		{"foo  bra.s next", "foo", "bra", ".s", []string{"next"}},
		{"\tmovem.l d0-d7/a0-a6,-(sp)", "", "movem", ".l", []string{"d0-d7/a0-a6", "-(sp)"}},
		{"\trts", "", "rts", "", nil},
		{"\tnop ; idle", "", "nop", "", nil},
	}
	for _, c := range cases {
		l := LexLine(c.in)
		if l.Kind != LineInstruction {
			t.Errorf("%q: kind = %v, want LineInstruction", c.in, l.Kind)
			continue
		}
		if l.Label != c.label {
			t.Errorf("%q: label = %q, want %q", c.in, l.Label, c.label)
		}
		if l.Mnemonic != c.mnem {
			t.Errorf("%q: mnemonic = %q, want %q", c.in, l.Mnemonic, c.mnem)
		}
		if l.Size != c.size {
			t.Errorf("%q: size = %q, want %q", c.in, l.Size, c.size)
		}
		if !reflect.DeepEqual(l.Operands, c.ops) {
			t.Errorf("%q: operands = %#v, want %#v", c.in, l.Operands, c.ops)
		}
	}
}

func TestLexLine_Directive(t *testing.T) {
	cases := []struct {
		in   string
		mnem string
		size string
		ops  []string
	}{
		{"\tdc.l 1,2,3", "dc", ".l", []string{"1", "2", "3"}},
		{"\tdc.b \"hello,world\",0", "dc", ".b", []string{`"hello,world"`, "0"}},
		{"FOO  equ $1234", "equ", "", []string{"$1234"}},
		{"\tinclude defs.i", "include", "", []string{"defs.i"}},
		{"\tifd IS_IE", "ifd", "", []string{"IS_IE"}},
	}
	for _, c := range cases {
		l := LexLine(c.in)
		if l.Kind != LineDirective {
			t.Errorf("%q: kind = %v, want LineDirective", c.in, l.Kind)
			continue
		}
		if l.Mnemonic != c.mnem {
			t.Errorf("%q: mnemonic = %q, want %q", c.in, l.Mnemonic, c.mnem)
		}
		if l.Size != c.size {
			t.Errorf("%q: size = %q, want %q", c.in, l.Size, c.size)
		}
		if !reflect.DeepEqual(l.Operands, c.ops) {
			t.Errorf("%q: operands = %#v, want %#v", c.in, l.Operands, c.ops)
		}
	}
}
