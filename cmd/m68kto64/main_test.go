package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_Basic(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in.s")
	out := filepath.Join(tmp, "out.s")
	if err := os.WriteFile(in, []byte("\tmove.l #1,d0\n\trts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	rc := run([]string{"-no-header", "-o", out, in}, &stderr)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "move.l r1, r17") {
		t.Errorf("output missing expected ops:\n%s", body)
	}
}

func TestRun_NoArgs(t *testing.T) {
	var stderr bytes.Buffer
	rc := run(nil, &stderr)
	if rc != 1 {
		t.Errorf("expected rc=1, got %d", rc)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("expected usage in stderr:\n%s", stderr.String())
	}
}

func TestRun_BadFlag(t *testing.T) {
	var stderr bytes.Buffer
	rc := run([]string{"--no-such-flag"}, &stderr)
	if rc != 2 {
		t.Errorf("expected rc=2 for bad flag, got %d", rc)
	}
}

func TestRun_MissingInput(t *testing.T) {
	var stderr bytes.Buffer
	rc := run([]string{"-o", "/tmp/x.s", "/nonexistent/path/file.s"}, &stderr)
	if rc != 1 {
		t.Errorf("expected rc=1 for missing input, got %d", rc)
	}
	if !strings.Contains(stderr.String(), "error reading") {
		t.Errorf("expected read-error message:\n%s", stderr.String())
	}
}

func TestRun_DefaultOutputName(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "input.s")
	if err := os.WriteFile(in, []byte("\trts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	rc := run([]string{in}, &stderr)
	if rc != 0 {
		t.Fatalf("rc=%d %s", rc, stderr.String())
	}
	wantOut := filepath.Join(tmp, "input_ie64.s")
	if _, err := os.Stat(wantOut); err != nil {
		t.Errorf("default output %s missing: %v", wantOut, err)
	}
}

func TestRun_StrictModeReportsErrors(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in.s")
	if err := os.WriteFile(in, []byte("\tclr.l a0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	rc := run([]string{"-strict", "-o", filepath.Join(tmp, "out.s"), in}, &stderr)
	if rc == 0 {
		t.Errorf("strict mode should reject clr.l a0; rc=%d", rc)
	}
	if !strings.Contains(stderr.String(), "conversion error") {
		t.Errorf("expected conversion-error message:\n%s", stderr.String())
	}
}

func TestMainEntry(t *testing.T) {
	savedArgs, savedExit := osArgs, osExit
	defer func() { osArgs = savedArgs; osExit = savedExit }()
	var got int
	osExit = func(c int) { got = c }
	osArgs = []string{"prog"} // no input args → run returns 1
	main()
	if got != 1 {
		t.Errorf("main() exit code: got %d, want 1", got)
	}
}

// TestMain_AcceptsAllFlags — Phase A scaffold guard: every preproc CLI flag
// parses cleanly with default behavior. Repeated -I and -D accumulate.
// Activation behavior is asserted in later phases.
func TestMain_AcceptsAllFlags(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in.s")
	out := filepath.Join(tmp, "out.s")
	if err := os.WriteFile(in, []byte("\trts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{"-I", "/tmp/inc1", "-I", "/tmp/inc2"},
		{"-D", "FOO"},
		{"-D", "FOO=5"},
		{"-D", "FOO=$ff"},
		{"-D", "FOO=0x10"},
		{"-D", "FOO=%101"},
		{"-D", "FOO=1", "-D", "BAR=2"},
		{"-strip-cond"},
		{"-max-macro-recurs", "500"},
		{"-Werror-unknown-mnemonic=false"},
		{"-no-default-seeds"},
		{"-I", "/tmp/a", "-D", "X=1", "-strip-cond", "-no-default-seeds"},
	}
	for _, args := range cases {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			full := append(append([]string{}, args...), "-o", out, in)
			var stderr bytes.Buffer
			rc := run(full, &stderr)
			if rc != 0 {
				t.Fatalf("rc=%d for args=%v stderr=%s", rc, args, stderr.String())
			}
		})
	}
}

// TestMain_RejectsBadDefine — whitespace around `=`, empty name, bad literal.
func TestMain_RejectsBadDefine(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in.s")
	if err := os.WriteFile(in, []byte("\trts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"FOO = 5",  // whitespace around =
		"=5",       // empty name
		"FOO=abc",  // non-numeric value
		"FOO=",     // empty value
	}
	for _, def := range cases {
		def := def
		t.Run(def, func(t *testing.T) {
			var stderr bytes.Buffer
			rc := run([]string{"-D", def, "-o", filepath.Join(tmp, "out.s"), in}, &stderr)
			if rc == 0 {
				t.Errorf("expected rejection of -D %q; rc=0 stderr=%s", def, stderr.String())
			}
		})
	}
}

func TestRun_WriteFails(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in.s")
	if err := os.WriteFile(in, []byte("\trts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Output to a directory that doesn't exist — WriteFile fails.
	var stderr bytes.Buffer
	rc := run([]string{"-o", filepath.Join(tmp, "missing", "out.s"), in}, &stderr)
	if rc != 1 {
		t.Errorf("expected rc=1 for write failure; got %d", rc)
	}
	if !strings.Contains(stderr.String(), "error writing") {
		t.Errorf("expected write-error in stderr:\n%s", stderr.String())
	}
}
