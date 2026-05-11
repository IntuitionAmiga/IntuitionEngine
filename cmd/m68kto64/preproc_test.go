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
