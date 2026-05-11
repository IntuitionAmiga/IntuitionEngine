package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConverter_UnknownMnemonicPassthrough — when WerrorUnknownMnem is off,
// the unknown-mnemonic fallthrough passes the line through verbatim (legacy
// behavior preserved for migration users).
func TestConverter_UnknownMnemonicPassthrough(t *testing.T) {
	c := NewConverter()
	c.werrorUnknownMnem = false
	out, errs := c.ConvertSource("\tunknown_op arg1,arg2\n")
	if errs != 0 {
		t.Fatalf("unexpected errs=%d", errs)
	}
	if !strings.Contains(out, "unknown_op") {
		t.Errorf("passthrough should preserve mnemonic: %q", out)
	}
}

// TestConverter_UnknownMnemonicErrors — when WerrorUnknownMnem is on, the
// converter rejects unknown mnemonics with a `; ERROR:` diagnostic.
func TestConverter_UnknownMnemonicErrors(t *testing.T) {
	c := NewConverter()
	c.werrorUnknownMnem = true
	out, errs := c.ConvertSource("\tunknown_op arg1,arg2\n")
	if errs == 0 {
		t.Errorf("expected unknown-mnemonic error; got out=%q", out)
	}
	if !strings.Contains(out, "; ERROR:") {
		t.Errorf("expected ; ERROR: marker: %q", out)
	}
}

// TestConvertFile_WerrorDefaultOn — ConvertFile path uses opts.WerrorUnknownMnem
// (default true per DefaultPreprocOpts) so unknown mnemonics surfacing past
// the preprocessor are errors.
func TestConvertFile_WerrorDefaultOn(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "x.s")
	if err := os.WriteFile(in, []byte("\tunknown_op d0,d1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewConverter()
	var stderr bytes.Buffer
	out, errs := c.ConvertFile(in, DefaultPreprocOpts(), &stderr)
	if errs == 0 {
		t.Errorf("Werror default-on should reject unknown mnemonic; out=%q", out)
	}
}

// TestConvertFile_WerrorOff — opts.WerrorUnknownMnem=false restores legacy
// passthrough.
func TestConvertFile_WerrorOff(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "x.s")
	if err := os.WriteFile(in, []byte("\tunknown_op d0,d1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewConverter()
	opts := DefaultPreprocOpts()
	opts.WerrorUnknownMnem = false
	var stderr bytes.Buffer
	out, errs := c.ConvertFile(in, opts, &stderr)
	if errs != 0 {
		t.Errorf("Werror off should not error; errs=%d stderr=%s out=%q", errs, stderr.String(), out)
	}
	if !strings.Contains(out, "unknown_op") {
		t.Errorf("passthrough missing: %q", out)
	}
}
