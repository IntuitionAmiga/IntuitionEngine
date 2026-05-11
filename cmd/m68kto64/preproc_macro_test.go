package main

import (
	"bytes"
	"strings"
	"testing"
)

func runPreproc(t *testing.T, src string, opts PreprocOpts) (string, int) {
	t.Helper()
	var stderr bytes.Buffer
	r, errs := Preprocess([]byte(src), "test.s", opts, &stderr)
	if errs > 0 && opts.MaxMacroRecurs == 0 {
		t.Logf("stderr: %s", stderr.String())
	}
	return strings.Join(r.lines, "\n"), errs
}

func TestPreproc_MacroBasic(t *testing.T) {
	src := "FOO macro\n\tnop\n\tendm\n\tFOO\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	if !strings.Contains(out, "\tnop") {
		t.Errorf("macro body not expanded: %q", out)
	}
	if strings.Contains(out, "\tFOO") {
		t.Errorf("invocation should not appear in output: %q", out)
	}
	if strings.Contains(out, "FOO macro") {
		t.Errorf("macro definition line should be consumed: %q", out)
	}
}

func TestPreproc_MacroMnemonicFirst(t *testing.T) {
	// vasm/devpac allows `MACRO name` as an alternative to `name MACRO`.
	// Body must capture identically.
	src := "\tmacro FOO\n\tnop\n\tendm\n\tFOO\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	if !strings.Contains(out, "\tnop") {
		t.Errorf("macro body not expanded: %q", out)
	}
	if strings.Contains(out, "\tFOO\n") {
		t.Errorf("invocation should be consumed: %q", out)
	}
}

func TestPreproc_MacroArgs(t *testing.T) {
	src := "SHIFT macro\n\tmove.l \\1,\\2\n\tendm\n\tSHIFT d0,d1\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	if !strings.Contains(out, "move.l d0,d1") {
		t.Errorf("\\1/\\2 substitution: %q", out)
	}
}

func TestPreproc_MacroUniqLabel(t *testing.T) {
	src := "WAIT macro\n.loop\\@:\n\tdbra d0,.loop\\@\n\tendm\n\tWAIT\n\tWAIT\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	// Collect label-definition lines (`.loop_N:`) — there should be two
	// distinct counter values, one per invocation.
	var labelLines []string
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, ".loop_") && strings.HasSuffix(ln, ":") {
			labelLines = append(labelLines, ln)
		}
	}
	if len(labelLines) != 2 {
		t.Fatalf("expected 2 label lines, got %d: %v\n%q", len(labelLines), labelLines, out)
	}
	if labelLines[0] == labelLines[1] {
		t.Errorf("\\@ counter did not advance: both %q", labelLines[0])
	}
}

func TestPreproc_AltArgRejected(t *testing.T) {
	src := "BAD macro\n\tmove.l \\?,d0\n\tendm\n\tBAD\n"
	_, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs == 0 {
		t.Errorf("expected error for \\? alt-arg")
	}
}

func TestPreproc_MacroMEXIT(t *testing.T) {
	src := "M macro\n\tnop\n\tmexit\n\trts\n\tendm\n\tM\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	if !strings.Contains(out, "\tnop") {
		t.Errorf("nop before mexit should expand: %q", out)
	}
	if strings.Contains(out, "\trts") {
		t.Errorf("rts after mexit should be skipped: %q", out)
	}
}

func TestPreproc_MacroNested(t *testing.T) {
	src := "INNER macro\n\tnop\n\tendm\nOUTER macro\n\tINNER\n\tINNER\n\tendm\n\tOUTER\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	if strings.Count(out, "\tnop") != 2 {
		t.Errorf("nested expansion: expected 2 nops, got %q", out)
	}
}

func TestPreproc_MacroRecursionCap(t *testing.T) {
	src := "R macro\n\tR\n\tendm\n\tR\n"
	opts := DefaultPreprocOpts()
	opts.MaxMacroRecurs = 10
	_, errs := runPreproc(t, src, opts)
	if errs == 0 {
		t.Errorf("expected recursion-cap error")
	}
}

func TestPreproc_Rept(t *testing.T) {
	src := "\trept 3\n\tnop\n\tendr\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	if strings.Count(out, "\tnop") != 3 {
		t.Errorf("rept 3 → expected 3 nops, got %q", out)
	}
}

func TestPreproc_ReptUniqLabel(t *testing.T) {
	src := "\trept 2\n.lbl\\@:\n\tendr\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	// Two distinct counter values.
	if strings.Count(out, ".lbl_") != 2 {
		t.Errorf(".lbl_ count: %q", out)
	}
	lines := strings.Split(out, "\n")
	var labels []string
	for _, l := range lines {
		if strings.Contains(l, ".lbl_") {
			labels = append(labels, l)
		}
	}
	if len(labels) >= 2 && labels[0] == labels[1] {
		t.Errorf("rept \\@ did not advance: %v", labels)
	}
}

func TestPreproc_IfbInsideMacro(t *testing.T) {
	// ifb \1 yields true when arg blank.
	src := "M macro\n\tifb \\1\n\tnop\n\telse\n\trts\n\tendc\n\tendm\n\tM\n\tM d0\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	// First invocation: arg blank → emit nop.
	// Second invocation: arg d0 → emit rts.
	if !strings.Contains(out, "\tnop") {
		t.Errorf("ifb arg-blank branch missing: %q", out)
	}
	if !strings.Contains(out, "\trts") {
		t.Errorf("ifnb (arg present) branch missing: %q", out)
	}
}

func TestPreproc_MacroInsideRept(t *testing.T) {
	// \@ inside rept inside macro: each iteration gets a distinct counter.
	src := "M macro\n\trept 2\n.x\\@:\n\tendr\n\tendm\n\tM\n"
	out, errs := runPreproc(t, src, DefaultPreprocOpts())
	if errs != 0 {
		t.Fatalf("errs=%d out=%q", errs, out)
	}
	if strings.Count(out, ".x_") != 2 {
		t.Errorf("expected 2 .x_ labels from rept 2: %q", out)
	}
}
