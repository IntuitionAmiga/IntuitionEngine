package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAROSDrivePath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore chdir: %v", err)
		}
	})

	makeArosTree := func(root string) string {
		t.Helper()
		path := filepath.Join(root, "S", "Startup-Sequence")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
		if err := os.WriteFile(path, []byte("echo boot\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
		return root
	}

	t.Run("cwd repo-style tree", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Chdir(root); err != nil {
			t.Fatalf("Chdir(%q): %v", root, err)
		}

		want := makeArosTree(filepath.Join(root, "AROS", "bin", "ie-m68k", "bin", "ie-m68k", "AROS"))
		got, err := resolveAROSDrivePath("", filepath.Join(t.TempDir(), "IntuitionEngine"))
		if err != nil {
			t.Fatalf("resolveAROSDrivePath(): %v", err)
		}
		if got != want {
			t.Fatalf("resolveAROSDrivePath() = %q, want %q", got, want)
		}
		if !isAROSDrivePath(got) {
			t.Fatalf("resolved path %q is not a valid AROS drive", got)
		}
	})

	t.Run("exe sibling build tree", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Chdir(t.TempDir()); err != nil {
			t.Fatalf("Chdir(temp): %v", err)
		}

		want := makeArosTree(filepath.Join(root, "AROS", "bin", "ie-m68k", "bin", "ie-m68k", "AROS"))
		exePath := filepath.Join(root, "dist", "bin", "IntuitionEngine")
		if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(exePath), err)
		}

		got, err := resolveAROSDrivePath("", exePath)
		if err != nil {
			t.Fatalf("resolveAROSDrivePath(): %v", err)
		}
		if got != want {
			t.Fatalf("resolveAROSDrivePath() = %q, want %q", got, want)
		}
		if !isAROSDrivePath(got) {
			t.Fatalf("resolved path %q is not a valid AROS drive", got)
		}
	})

	t.Run("explicit invalid rejected", func(t *testing.T) {
		if got, err := resolveAROSDrivePath(filepath.Join(t.TempDir(), "missing"), ""); err == nil || got != "" {
			t.Fatalf("resolveAROSDrivePath(invalid) = (%q,%v), want empty error", got, err)
		}
	})

	t.Run("omitted missing returns error", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Chdir(root); err != nil {
			t.Fatalf("Chdir(%q): %v", root, err)
		}
		if got, err := resolveAROSDrivePath("", filepath.Join(root, "bin", "IntuitionEngine")); err == nil || got != "" {
			t.Fatalf("resolveAROSDrivePath(missing) = (%q,%v), want empty error", got, err)
		}
	})
}
