package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDefaultAROSImagePath_PrefersBuiltROM(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir tempdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	mustWrite := func(rel string) {
		path := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte{0}, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", rel, err)
		}
	}

	mustWrite("sdk/roms/aros-ie-m68k.rom")

	if got := resolveDefaultAROSImagePath(); got != "sdk/roms/aros-ie-m68k.rom" {
		t.Fatalf("resolveDefaultAROSImagePath() = %q, want %q", got, "sdk/roms/aros-ie-m68k.rom")
	}
}
