package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGolden_RoundTrip runs every `*.s` file under cmd/m68kto64/golden/
// through the transpiler and then through `sdk/bin/ie64asm`, asserting that
// each assembles cleanly to a non-zero binary. The presence of `sdk/bin/ie64asm`
// is required; the test skips if the assembler is not built (CI / fresh
// checkouts that haven't run `make ie64asm` yet).
func TestGolden_RoundTrip(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot locate repo root: %v", err)
	}
	asm := filepath.Join(repoRoot, "sdk", "bin", "ie64asm")
	if _, err := os.Stat(asm); err != nil {
		t.Skipf("sdk/bin/ie64asm not built (run `make ie64asm`): %v", err)
	}

	srcs, err := filepath.Glob("golden/*.s")
	if err != nil || len(srcs) == 0 {
		t.Fatalf("no golden inputs found: %v", err)
	}

	for _, src := range srcs {
		src := src
		name := strings.TrimSuffix(filepath.Base(src), ".s")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(src)
			if err != nil {
				t.Fatalf("read %s: %v", src, err)
			}
			c := NewConverter()
			out, errs := c.ConvertSource(string(data))
			if errs > 0 {
				t.Fatalf("conversion errors:\n%s", out)
			}

			tmp := t.TempDir()
			ie64s := filepath.Join(tmp, name+".ie64.s")
			if err := os.WriteFile(ie64s, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(tmp, name+".bin")
			cmd := exec.Command(asm, ie64s, "-o", bin)
			combined, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ie64asm rejected output:\n--- transpiled ---\n%s\n--- ie64asm ---\n%s\nerr: %v",
					out, combined, err)
			}
			info, err := os.Stat(bin)
			if err != nil {
				t.Fatalf("output missing: %v", err)
			}
			if info.Size() == 0 {
				t.Fatalf("ie64asm produced 0-byte binary; transpiler likely emitted no encodable instructions")
			}
			t.Logf("%s: %d bytes", name, info.Size())
		})
	}
}

// findRepoRoot walks upward from the current working dir looking for a
// directory containing both `go.mod` and `sdk/bin/`.
func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
