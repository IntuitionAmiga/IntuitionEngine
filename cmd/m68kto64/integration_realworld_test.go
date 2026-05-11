//go:build m68kto64_integration

// Real-world corpus smoke tests. Env-gated; not part of the default
// `go test ./cmd/m68kto64/...` run. Invoke with:
//
//   IE_M68KTO64_CORPUS_AB3D2=/path/to/alienbreed3d2 \
//     go test -tags m68kto64_integration -run TestRealworldCorpus ./cmd/m68kto64/
//
// Each registered corpus runs through Preprocess + ConvertLines, then through
// sdk/bin/ie64asm. The harness asserts:
//   - no `; ERROR:` lines in the m68kto64 output;
//   - ie64asm exit 0 (which proves the output assembles).
//
// Coverage caveat: a clean ie64asm exit proves the output is *syntactically*
// valid IE64; it does NOT prove semantic correctness. Bad lowering can still
// produce valid-but-wrong .ie64. Functional/runtime validation is owned by
// the per-port companion plan (for AB3D2 see how-do-you-propose-peaceful-
// rabbit.md) via boot + gameplay smoke.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealworldCorpus(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot locate repo root: %v", err)
	}
	asm := filepath.Join(repoRoot, "sdk", "bin", "ie64asm")
	if _, err := os.Stat(asm); err != nil {
		t.Skipf("sdk/bin/ie64asm not built (run `make ie64asm`): %v", err)
	}

	for _, c := range corpora {
		c := c
		t.Run(c.name, func(t *testing.T) {
			root := os.Getenv(c.envVar)
			if root == "" {
				t.Skipf("%s unset; corpus %s not configured", c.envVar, c.name)
			}
			rootSrc := filepath.Join(root, c.rootFile)
			if _, err := os.Stat(rootSrc); err != nil {
				t.Skipf("corpus %s root file not stat-able (%s): %v", c.name, rootSrc, err)
			}

			opts := DefaultPreprocOpts()
			opts.Defines = map[string]int64{}
			for k, v := range c.defines {
				opts.Defines[k] = v
			}
			for _, inc := range c.includes {
				opts.IncludeDirs = append(opts.IncludeDirs, filepath.Join(root, inc))
			}

			conv := NewConverter()
			var stderr bytes.Buffer
			source, errs := conv.ConvertFile(rootSrc, opts, &stderr)
			if errs > 0 {
				t.Fatalf("preproc/convert errs=%d\nstderr:\n%s", errs, stderr.String())
			}
			if strings.Contains(source, "; ERROR:") {
				t.Fatalf("output contains ; ERROR: lines (first 2000 chars):\n%s", source[:min(2000, len(source))])
			}
			if source == "" {
				t.Fatalf("empty output")
			}

			tmp := t.TempDir()
			out := filepath.Join(tmp, c.name+".ie64.s")
			if err := os.WriteFile(out, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(tmp, c.name+".bin")
			args := []string{}
			for _, inc := range c.includes {
				args = append(args, "-I", filepath.Join(root, inc))
			}
			args = append(args, out, "-o", bin)
			cmd := exec.Command(asm, args...)
			combined, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ie64asm rejected output:\n--- ie64asm ---\n%s\nerr: %v", combined, err)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
