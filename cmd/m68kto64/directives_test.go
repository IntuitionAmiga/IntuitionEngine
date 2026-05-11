package main

import (
	"strings"
	"testing"
)

func TestDirective_PassThrough(t *testing.T) {
	cases := []string{
		"\tdc.b 1,2,3",
		"\tdc.w $1234,$5678",
		"\tdc.l 0,0,0",
		"\tds.l 16",
		"\tinclude defs.i",
		"\tincbin \"image.bin\"",
		"\torg $10000",
	}
	for _, in := range cases {
		out := convertSrc(t, in+"\n")
		// Mnemonic must survive; size must survive too.
		l := LexLine(in)
		want := l.Mnemonic + l.Size
		if !strings.Contains(out, want) {
			t.Errorf("input %q: output missing %q\n%s", in, want, out)
		}
	}
}

func TestDirective_IfdIsIE_Rewrite(t *testing.T) {
	out := convertSrc(t, "\tifd IS_IE\n\tnop\n\tendc\n")
	mustContain(t, out, "if 1")
	mustContain(t, out, "endif")
}

func TestDirective_IfndIsIE_Rewrite(t *testing.T) {
	out := convertSrc(t, "\tifnd IS_IE\n\tnop\n\tendc\n")
	mustContain(t, out, "if 0")
	mustContain(t, out, "endif")
}

func TestDirective_IfdOther_AssumedUndefined(t *testing.T) {
	out := convertSrc(t, "\tifd FOO\n\tdc.l 1\n\tendc\n")
	// ie64asm has no defined() predicate — non-IS_IE ifd assumed undefined.
	mustContain(t, out, "if 0")
}

func TestDirective_SectionDropped(t *testing.T) {
	cases := []string{
		"\tsection .bss,bss",
		"\tsection .bsschip,bss_c",
		"\tsection .datachip,data_c",
		"\tsection .text,code",
		"\tsection .data,data",
		"\tsection \"text\",code",
	}
	for _, in := range cases {
		out := convertSrc(t, in+"\n")
		if !strings.Contains(out, "dropped section") {
			t.Errorf("section not dropped:\ninput: %q\nout: %q", in, out)
		}
		if strings.Contains(out, "\tsection") {
			t.Errorf("section directive should not survive:\nout: %q", out)
		}
	}
}

func TestDirective_XdefDropped(t *testing.T) {
	out := convertSrc(t, "\txdef foo,bar\n")
	mustContain(t, out, "dropped xdef")
	mustNotContain(t, out, "\txdef foo,bar")
}

func TestDirective_XrefDropped(t *testing.T) {
	out := convertSrc(t, "\txref baz\n")
	mustContain(t, out, "dropped xref")
}

func TestDirective_Even(t *testing.T) {
	out := convertSrc(t, "\teven\n")
	mustContain(t, out, "align 2")
}

func TestDirective_Equ(t *testing.T) {
	out := convertSrc(t, "FOO\tequ $1234\n")
	mustContain(t, out, "FOO:")
	mustContain(t, out, "equ $1234")
}

// Post-Phase-E: macros are expanded transpile-time by the preprocessor. The
// macro definition is consumed; only the invocation's expanded body reaches
// ConvertLines. \1..\9 are substituted; \@ resolves to a unique counter.
func TestDirective_Macro_Expanded(t *testing.T) {
	src := "MYMAC\tmacro\n\tmove.l \\1,\\2\n\tendm\n\tMYMAC d0,d1\n"
	out := preprocSrc(t, src, DefaultPreprocOpts())
	mustContain(t, out, "move.l")
	mustNotContain(t, out, "macro")
	mustNotContain(t, out, "endm")
	mustNotContain(t, out, "\\1")
	mustNotContain(t, out, "MYMAC d0,d1")
}

// Post-Phase-E: rept bodies are unrolled transpile-time. The rept/endr
// directive pair is consumed; only the expanded copies reach ConvertLines.
func TestDirective_Rept_Expanded(t *testing.T) {
	out := preprocSrc(t, "\trept 4\n\tnop\n\tendr\n", DefaultPreprocOpts())
	mustNotContain(t, out, "rept 4")
	mustNotContain(t, out, "endr")
	// 4 nop lowerings (`; nop` plus the lowered instruction text).
	if strings.Count(out, "nop") < 4 {
		t.Errorf("expected 4 nop expansions, got: %q", out)
	}
}
