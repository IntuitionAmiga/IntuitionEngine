package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAROSDrivePath_PrefersGeneratedAROSVisionTree(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir tempdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	mustWriteStartup := func(rel string) {
		path := filepath.Join(tmp, rel, "S", "Startup-Sequence")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("; startup\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", rel, err)
		}
	}

	mustWriteStartup("AROS")
	mustWriteStartup("build/arosvision-probe/AROS")

	got, err := resolveAROSDrivePath("", "")
	if err != nil {
		t.Fatalf("resolveAROSDrivePath() failed: %v", err)
	}
	want := filepath.Join(tmp, "build", "arosvision-probe", "AROS")
	if got != want {
		t.Fatalf("resolveAROSDrivePath() = %q, want %q", got, want)
	}
}
