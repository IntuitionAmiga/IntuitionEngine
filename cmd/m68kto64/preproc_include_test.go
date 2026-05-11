package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPreproc_IncludeBasic(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "b.i"), "\tnop\n")
	writeFile(t, filepath.Join(tmp, "a.s"), "\tinclude \"b.i\"\n\trts\n")
	data, _ := os.ReadFile(filepath.Join(tmp, "a.s"))
	var stderr bytes.Buffer
	r, errs := Preprocess(data, filepath.Join(tmp, "a.s"), DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if !strings.Contains(joined, "\tnop") {
		t.Errorf("included body missing: %q", joined)
	}
	if !strings.Contains(joined, "\trts") {
		t.Errorf("post-include line missing: %q", joined)
	}
}

func TestPreproc_IncludeNested(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "c.i"), "\tnop\n")
	writeFile(t, filepath.Join(tmp, "b.i"), "\tinclude \"c.i\"\n")
	writeFile(t, filepath.Join(tmp, "a.s"), "\tinclude \"b.i\"\n")
	data, _ := os.ReadFile(filepath.Join(tmp, "a.s"))
	var stderr bytes.Buffer
	r, errs := Preprocess(data, filepath.Join(tmp, "a.s"), DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if !strings.Contains(joined, "\tnop") {
		t.Errorf("nested-include body missing: %q", joined)
	}
}

func TestPreproc_IncludeCycle(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "a.s"), "\tinclude \"b.i\"\n")
	writeFile(t, filepath.Join(tmp, "b.i"), "\tinclude \"a.s\"\n")
	data, _ := os.ReadFile(filepath.Join(tmp, "a.s"))
	var stderr bytes.Buffer
	_, errs := Preprocess(data, filepath.Join(tmp, "a.s"), DefaultPreprocOpts(), &stderr)
	if errs == 0 {
		t.Errorf("expected cycle error")
	}
	if !strings.Contains(stderr.String(), "cycle") {
		t.Errorf("expected cycle diagnostic; got: %q", stderr.String())
	}
}

func TestPreproc_IncludeDiamond(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "d.i"), "\tdc.l 42\n")
	writeFile(t, filepath.Join(tmp, "b.i"), "\tinclude \"d.i\"\n")
	writeFile(t, filepath.Join(tmp, "c.i"), "\tinclude \"d.i\"\n")
	writeFile(t, filepath.Join(tmp, "a.s"), "\tinclude \"b.i\"\n\tinclude \"c.i\"\n")
	data, _ := os.ReadFile(filepath.Join(tmp, "a.s"))
	var stderr bytes.Buffer
	r, errs := Preprocess(data, filepath.Join(tmp, "a.s"), DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("diamond should succeed; errs=%d %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if strings.Count(joined, "dc.l 42") != 2 {
		t.Errorf("expected d.i body twice (diamond); got: %q", joined)
	}
}

func TestPreproc_IncludeMinusI(t *testing.T) {
	tmp := t.TempDir()
	incDir := filepath.Join(tmp, "inc")
	writeFile(t, filepath.Join(incDir, "lib.i"), "\tnop\n")
	writeFile(t, filepath.Join(tmp, "main.s"), "\tinclude \"lib.i\"\n")
	data, _ := os.ReadFile(filepath.Join(tmp, "main.s"))

	opts := DefaultPreprocOpts()
	opts.IncludeDirs = []string{incDir}
	var stderr bytes.Buffer
	r, errs := Preprocess(data, filepath.Join(tmp, "main.s"), opts, &stderr)
	if errs != 0 {
		t.Fatalf("errs=%d %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if !strings.Contains(joined, "\tnop") {
		t.Errorf("-I-resolved include missing: %q", joined)
	}

	// Missing file: error.
	writeFile(t, filepath.Join(tmp, "bad.s"), "\tinclude \"missing.i\"\n")
	data2, _ := os.ReadFile(filepath.Join(tmp, "bad.s"))
	var stderr2 bytes.Buffer
	_, errs2 := Preprocess(data2, filepath.Join(tmp, "bad.s"), opts, &stderr2)
	if errs2 == 0 {
		t.Errorf("expected error for missing include")
	}
}

func TestPreproc_IncbinVerbatim(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "a.s"), "\tincbin \"image.bin\"\n")
	data, _ := os.ReadFile(filepath.Join(tmp, "a.s"))
	var stderr bytes.Buffer
	r, errs := Preprocess(data, filepath.Join(tmp, "a.s"), DefaultPreprocOpts(), &stderr)
	if errs != 0 {
		t.Fatalf("incbin should pass through with no diagnostic; errs=%d %s", errs, stderr.String())
	}
	joined := strings.Join(r.lines, "\n")
	if !strings.Contains(joined, "incbin \"image.bin\"") {
		t.Errorf("incbin line not preserved verbatim: %q", joined)
	}
}
