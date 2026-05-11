package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreproc_ByteIdentityPassthrough — Phase A guard: for directive-free
// inputs, ConvertFile must produce byte-identical output to the legacy
// ConvertSource(string(data)) path. Covers all three golden tiers; tier2 byte
// identity holds until macros become transpile-time-expanded in Phase E.
func TestPreproc_ByteIdentityPassthrough(t *testing.T) {
	tiers := []string{
		"testdata/golden_pre_extension/tier1_no_directives",
		"testdata/golden_pre_extension/tier2_with_macros",
		"testdata/golden_pre_extension/tier3_conditionals",
	}
	for _, dir := range tiers {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			files, err := filepath.Glob(filepath.Join(dir, "*.s"))
			if err != nil || len(files) == 0 {
				t.Fatalf("no fixtures in %s: %v", dir, err)
			}
			for _, f := range files {
				f := f
				t.Run(filepath.Base(f), func(t *testing.T) {
					data, err := os.ReadFile(f)
					if err != nil {
						t.Fatal(err)
					}
					// Baseline: legacy ConvertSource path.
					base := NewConverter()
					wantSrc, wantErrs := base.ConvertSource(string(data))

					// New path: ConvertFile (which routes through Preprocess).
					alt := NewConverter()
					var stderr bytes.Buffer
					gotSrc, gotErrs := alt.ConvertFile(f, DefaultPreprocOpts(), &stderr)

					if gotSrc != wantSrc {
						t.Errorf("byte-diff between ConvertSource and ConvertFile for %s\nwant len=%d, got len=%d\nstderr: %s", f, len(wantSrc), len(gotSrc), stderr.String())
					}
					if gotErrs != wantErrs {
						t.Errorf("error count mismatch: want %d, got %d", wantErrs, gotErrs)
					}
				})
			}
		})
	}
}

// TestPreproc_NoTrailingNewline — directive-free input without trailing \n
// must still round-trip byte-identical.
func TestPreproc_NoTrailingNewline(t *testing.T) {
	src := "\tnop\n\trts" // no trailing \n
	base := NewConverter()
	want, _ := base.ConvertSource(src)

	tmp := filepath.Join(t.TempDir(), "in.s")
	if err := os.WriteFile(tmp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	alt := NewConverter()
	var stderr bytes.Buffer
	got, _ := alt.ConvertFile(tmp, DefaultPreprocOpts(), &stderr)
	if got != want {
		t.Errorf("no-trailing-newline byte diff:\nwant: %q\n got: %q", want, got)
	}
}

// TestPreproc_LoneCRRejected — classic Mac line endings cause an explicit
// preprocessor error (no silent normalization).
func TestPreproc_LoneCRRejected(t *testing.T) {
	src := "nop\rrts\n" // lone \r between lines
	var stderr bytes.Buffer
	_, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
	if errs == 0 {
		t.Fatalf("expected error for lone CR; got none, stderr=%q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "lone CR") {
		t.Errorf("error message should mention lone CR; got: %q", stderr.String())
	}
}

// TestPreproc_EquCaptured — Phase B: equ binding is recorded in symtab.
func TestPreproc_EquCaptured(t *testing.T) {
	src := "FOO equ 5\n\tnop\n"
	var stderr bytes.Buffer
	r, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("unexpected errors: %s", stderr.String())
	}
	v, ok := r.symtab.Get("FOO")
	if !ok {
		t.Fatalf("FOO not in symtab")
	}
	if v != 5 {
		t.Errorf("FOO = %d, want 5", v)
	}
}

// TestPreproc_SetMutable — set / = are mutable, equ redefinition errors.
func TestPreproc_SetMutable(t *testing.T) {
	type tc struct {
		name      string
		src       string
		wantErrs  bool
		wantFinal int64
	}
	cases := []tc{
		{"set/set", "FOO set 1\nFOO set 2\n", false, 2},
		{"=/=", "FOO = 1\nFOO = 2\n", false, 2},
		{"equ/equ", "FOO equ 1\nFOO equ 2\n", true, 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer
			r, errs := Preprocess([]byte(c.src), "test.s", DefaultPreprocOpts(), &stderr)
			gotErr := errs > 0
			if gotErr != c.wantErrs {
				t.Errorf("errs=%d, want err=%v, stderr=%s", errs, c.wantErrs, stderr.String())
			}
			v, _ := r.symtab.Get("FOO")
			if v != c.wantFinal {
				t.Errorf("FOO=%d, want %d", v, c.wantFinal)
			}
		})
	}
}

// TestPreproc_DefineFlag — -D values parse correctly.
func TestPreproc_DefineFlag(t *testing.T) {
	cases := []struct {
		defs map[string]int64
		want map[string]int64
	}{
		{map[string]int64{"FOO": 1}, map[string]int64{"FOO": 1}},
		{map[string]int64{"FOO": 5}, map[string]int64{"FOO": 5}},
		{map[string]int64{"FOO": 0xff}, map[string]int64{"FOO": 255}},
		{map[string]int64{"FOO": 0x10}, map[string]int64{"FOO": 16}},
		{map[string]int64{"FOO": 5, "BAR": 7}, map[string]int64{"FOO": 5, "BAR": 7}},
	}
	for i, c := range cases {
		opts := DefaultPreprocOpts()
		opts.Defines = c.defs
		var stderr bytes.Buffer
		r, errs := Preprocess([]byte("\tnop\n"), "test.s", opts, &stderr)
		if errs != 0 {
			t.Fatalf("case %d: %s", i, stderr.String())
		}
		for k, want := range c.want {
			got, ok := r.symtab.Get(k)
			if !ok {
				t.Errorf("case %d: %s missing", i, k)
				continue
			}
			if got != want {
				t.Errorf("case %d: %s=%d, want %d", i, k, got, want)
			}
		}
	}
}

// TestPreproc_DefinePrecedence — -D mutable; source set overrides; source equ
// errors as redefinition.
func TestPreproc_DefinePrecedence(t *testing.T) {
	type tc struct {
		name     string
		src      string
		defs     map[string]int64
		wantErrs bool
		wantFoo  int64
	}
	cases := []tc{
		{"D_then_set", "FOO set 2\n", map[string]int64{"FOO": 1}, false, 2},
		{"D_then_assign", "FOO = 2\n", map[string]int64{"FOO": 1}, false, 2},
		{"D_then_equ_errors", "FOO equ 2\n", map[string]int64{"FOO": 1}, true, 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			opts := DefaultPreprocOpts()
			opts.Defines = c.defs
			var stderr bytes.Buffer
			r, errs := Preprocess([]byte(c.src), "test.s", opts, &stderr)
			if (errs > 0) != c.wantErrs {
				t.Errorf("errs=%d want err=%v stderr=%s", errs, c.wantErrs, stderr.String())
			}
			v, _ := r.symtab.Get("FOO")
			if v != c.wantFoo {
				t.Errorf("FOO=%d want %d", v, c.wantFoo)
			}
		})
	}
}

// TestPreproc_IsIeSeeded — IS_IE auto-seeded to 1, suppressible via opt.
func TestPreproc_IsIeSeeded(t *testing.T) {
	var stderr bytes.Buffer
	r, _ := Preprocess([]byte("\tnop\n"), "test.s", DefaultPreprocOpts(), &stderr)
	if v, ok := r.symtab.Get("IS_IE"); !ok || v != 1 {
		t.Errorf("IS_IE=%d ok=%v, want 1/true", v, ok)
	}
	opts := DefaultPreprocOpts()
	opts.NoDefaultSeeds = true
	r2, _ := Preprocess([]byte("\tnop\n"), "test.s", opts, &stderr)
	if _, ok := r2.symtab.Get("IS_IE"); ok {
		t.Errorf("IS_IE should be absent under -no-default-seeds")
	}
}

// TestPreproc_IfdGeneric — Phase C: ifd/ifnd queries symtab (not legacy IS_IE
// literal match).
func TestPreproc_IfdGeneric(t *testing.T) {
	t.Run("defined", func(t *testing.T) {
		src := "FOO equ 1\n\tifd FOO\n\tnop\n\tendc\n"
		var stderr bytes.Buffer
		r, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
		if errs != 0 {
			t.Fatalf("errs=%d: %s", errs, stderr.String())
		}
		joined := strings.Join(r.lines, "\n")
		if !strings.Contains(joined, "if 1") {
			t.Errorf("expected 'if 1' in output: %q", joined)
		}
		if !strings.Contains(joined, "endif") {
			t.Errorf("expected 'endif' in output: %q", joined)
		}
	})
	t.Run("undefined", func(t *testing.T) {
		src := "\tifd BAR\n\tnop\n\tendc\n"
		var stderr bytes.Buffer
		r, _ := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
		joined := strings.Join(r.lines, "\n")
		if !strings.Contains(joined, "if 0") {
			t.Errorf("expected 'if 0' for undefined symbol: %q", joined)
		}
	})
	t.Run("ifnd_undefined", func(t *testing.T) {
		src := "\tifnd BAR\n\tnop\n\tendc\n"
		var stderr bytes.Buffer
		r, _ := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
		joined := strings.Join(r.lines, "\n")
		if !strings.Contains(joined, "if 1") {
			t.Errorf("expected 'if 1' (ifnd of undefined): %q", joined)
		}
	})
}

func TestPreproc_NestedCond(t *testing.T) {
	src := "FOO equ 1\n\tifd FOO\n\tifd BAR\n\tnop\n\tendc\n\tendc\n"
	var stderr bytes.Buffer
	r, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d: %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	// Outer: if 1, inner: if 0 (BAR undefined), two endifs.
	if strings.Count(joined, "if 1") < 1 {
		t.Errorf("outer 'if 1' missing: %q", joined)
	}
	if strings.Count(joined, "if 0") < 1 {
		t.Errorf("inner 'if 0' missing: %q", joined)
	}
	if strings.Count(joined, "endif") < 2 {
		t.Errorf("expected 2 'endif': %q", joined)
	}
}

func TestPreproc_ElseBranch(t *testing.T) {
	src := "\tifd UNDEFINED\n\tnop\n\telse\n\trts\n\tendc\n"
	var stderr bytes.Buffer
	r, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d: %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if !strings.Contains(joined, "if 0") || !strings.Contains(joined, "else") || !strings.Contains(joined, "endif") {
		t.Errorf("missing wrappers: %q", joined)
	}
}

func TestPreproc_IfExpr(t *testing.T) {
	src := "FOO equ 5\n\tif FOO > 3\n\tnop\n\tendc\n"
	var stderr bytes.Buffer
	r, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d: %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if !strings.Contains(joined, "if 1") {
		t.Errorf("expected 'if 1' for FOO>3 when FOO=5: %q", joined)
	}
}

func TestPreproc_ElseIf(t *testing.T) {
	// First-true latch: when FOO=2, the FOO==2 branch wins. Subsequent
	// elseif predicates emit 'elseif 0' (latched) even if literally true.
	src := "FOO equ 2\n\tif FOO == 1\n\tdc.l 1\n\telseif FOO == 2\n\tdc.l 2\n\telseif FOO == 2\n\tdc.l 3\n\telse\n\tdc.l 4\n\tendc\n"
	var stderr bytes.Buffer
	r, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d: %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	// Expected wrappers: if 0, elseif 1, elseif 0, else, endif.
	if !strings.Contains(joined, "if 0") {
		t.Errorf("first wrapper should be 'if 0': %q", joined)
	}
	if strings.Count(joined, "elseif 1") != 1 {
		t.Errorf("expected exactly one 'elseif 1' (first-true latch): %q", joined)
	}
	if strings.Count(joined, "elseif 0") < 1 {
		t.Errorf("expected 'elseif 0' on latched-out branch: %q", joined)
	}
}

func TestPreproc_IsIeLegacy(t *testing.T) {
	// Preserves the IS_IE-seed convention: ifd IS_IE → if 1, body kept.
	src := "\tifd IS_IE\n\tnop\n\tendc\n"
	var stderr bytes.Buffer
	r, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d: %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if !strings.Contains(joined, "if 1") {
		t.Errorf("ifd IS_IE should emit 'if 1': %q", joined)
	}
	if !strings.Contains(joined, "\tnop") {
		t.Errorf("active-branch body must survive (Model A): %q", joined)
	}
}

func TestPreproc_StripCondMode(t *testing.T) {
	// Model B: wrappers stripped, inactive bodies dropped.
	src := "\tifd IS_IE\n\tnop\n\telse\n\trts\n\tendc\n"
	opts := DefaultPreprocOpts()
	opts.StripCond = true
	var stderr bytes.Buffer
	r, errs := Preprocess([]byte(src), "test.s", opts, &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d: %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if strings.Contains(joined, "if 1") || strings.Contains(joined, "if 0") || strings.Contains(joined, "endif") || strings.Contains(joined, "else") {
		t.Errorf("Model B should strip all wrappers: %q", joined)
	}
	if !strings.Contains(joined, "\tnop") {
		t.Errorf("active body should survive: %q", joined)
	}
	if strings.Contains(joined, "\trts") {
		t.Errorf("inactive body should be dropped: %q", joined)
	}
}

func TestPreproc_UnterminatedIf(t *testing.T) {
	src := "\tifd IS_IE\n\tnop\n"
	var stderr bytes.Buffer
	_, errs := Preprocess([]byte(src), "test.s", DefaultPreprocOpts(), &stderr)
	if errs == 0 {
		t.Errorf("expected error for unterminated if-chain")
	}
}

// TestPreproc_CRLFNormalized — CRLF line endings normalize to LF and produce
// the same output as an LF-only input would.
func TestPreproc_CRLFNormalized(t *testing.T) {
	lfSrc := "\tnop\n\trts\n"
	crlfSrc := "\tnop\r\n\trts\r\n"

	base := NewConverter()
	wantLF, _ := base.ConvertSource(lfSrc)

	tmp := filepath.Join(t.TempDir(), "crlf.s")
	if err := os.WriteFile(tmp, []byte(crlfSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewConverter()
	var stderr bytes.Buffer
	gotCRLF, errs := c.ConvertFile(tmp, DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("CRLF normalization unexpectedly errored: %s", stderr.String())
	}
	if gotCRLF != wantLF {
		t.Errorf("CRLF normalized output differs from LF baseline:\nLF:   %q\nCRLF: %q", wantLF, gotCRLF)
	}
}
