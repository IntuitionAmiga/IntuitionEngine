//go:build headless

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type assemblerExample struct {
	source string
	arch   string
}

var assemblerExamples = []assemblerExample{
	{"sdk/examples/asm/coproc_caller_ie32.asm", "ie32"},
	{"sdk/examples/asm/coproc_service_ie32.asm", "ie32"},
	{"sdk/examples/asm/robocop_intro.asm", "ie32"},
	{"sdk/examples/asm/rotozoomer.asm", "ie32"},
	{"sdk/examples/asm/vga_mode12h_bars.asm", "ie32"},
	{"sdk/examples/asm/vga_mode13h_fire.asm", "ie32"},
	{"sdk/examples/asm/vga_modex_circles.asm", "ie32"},
	{"sdk/examples/asm/vga_text_hello.asm", "ie32"},
	{"sdk/examples/asm/voodoo_mega_demo.asm", "ie32"},
	{"sdk/examples/asm/ehbasic_ie64.asm", "ie64"},
	{"sdk/examples/asm/iewarp_service.asm", "ie64"},
	{"sdk/examples/asm/mandelbrot_ie64.asm", "ie64"},
	{"sdk/examples/asm/rotozoomer_ie64.asm", "ie64"},
	{"sdk/examples/asm/ula_demo_ie64.asm", "ie64"},
	{"sdk/intuitionos/iexec/iexec.s", "ie64"},
	{"sdk/intuitionos/iexec/runtime_builder.s", "ie64"},
}

func TestAssemblerExamples(t *testing.T) {
	root := assemblerExamplesRepoRoot(t)
	tmp := t.TempDir()
	ie32 := filepath.Join(tmp, "ie32asm")
	ie64 := filepath.Join(tmp, "ie64asm")
	buildAssemblerBinary(t, root, ie32, nil)
	buildAssemblerBinary(t, root, ie64, []string{"ie64"})

	goldenPath := filepath.Join(root, "assembler", "testdata", "golden_hashes.txt")
	golden := readAssemblerGoldenHashes(t, goldenPath)
	regen := os.Getenv("IE_REGEN_GOLDEN") == "1"
	regenFilter := os.Getenv("IE_REGEN_GOLDEN_FILTER")
	var newGolden []string
	replacements := map[string]string{}

	for _, ex := range assemblerExamples {
		t.Run(filepath.Base(ex.source), func(t *testing.T) {
			assertAssemblerManifestStillMatches(t, root, ex)
			out := filepath.Join(tmp, strings.ReplaceAll(ex.source, "/", "_")+".bin")
			src := filepath.Join(root, ex.source)
			bin := ie32
			if ex.arch == "ie64" {
				bin = ie64
			}
			cmd := exec.Command(bin, "-I", filepath.Join(root, "sdk", "include"), "-o", out, filepath.Base(src))
			cmd.Dir = filepath.Dir(src)
			if got, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s failed: %v\n%s", ex.source, err, got)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			hash := hex.EncodeToString(sum[:])
			if regen {
				if regenFilter != "" {
					matched, err := assemblerGoldenFilterMatches(regenFilter, ex.source)
					if err != nil {
						t.Fatalf("invalid IE_REGEN_GOLDEN_FILTER %q: %v", regenFilter, err)
					}
					if matched {
						replacements[ex.source] = hash
					}
				} else {
					newGolden = append(newGolden, ex.source+" "+hash)
				}
				return
			}
			if golden[ex.source] != hash {
				t.Fatalf("%s hash = %s, want %s", ex.source, hash, golden[ex.source])
			}
		})
	}
	if regen {
		var out string
		if regenFilter != "" {
			out = mergeAssemblerGoldenHashes(t, goldenPath, replacements)
		} else {
			out = strings.Join(newGolden, "\n") + "\n"
		}
		if err := os.WriteFile(goldenPath, []byte(out), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func assemblerGoldenFilterMatches(pattern, source string) (bool, error) {
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(source, strings.TrimSuffix(pattern, "**")), nil
	}
	return filepath.Match(pattern, source)
}

func buildAssemblerBinary(t *testing.T, root, out string, tags []string) {
	t.Helper()
	args := []string{"build"}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, " "))
	}
	args = append(args, "-o", out, "./assembler")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/intuition-go-build", "GOPATH=/tmp/intuition-go")
	if got, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build assembler failed: %v\n%s", err, got)
	}
}

func readAssemblerGoldenHashes(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid golden hash line: %q", line)
		}
		out[fields[0]] = fields[1]
	}
	return out
}

func mergeAssemblerGoldenHashes(t *testing.T, path string, replacements map[string]string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) != 2 {
			t.Fatalf("invalid golden hash line: %q", trimmed)
		}
		if hash, ok := replacements[fields[0]]; ok {
			suffix := ""
			if strings.HasSuffix(line, "\n") {
				suffix = "\n"
			}
			lines[i] = fields[0] + " " + hash + suffix
			delete(replacements, fields[0])
		}
	}
	if len(replacements) != 0 {
		t.Fatalf("filtered regen produced hashes not present in golden file: %v", replacements)
	}
	return strings.Join(lines, "")
}

func assertAssemblerManifestStillMatches(t *testing.T, root string, ex assemblerExample) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ex.source))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	want := "ie32"
	if strings.Contains(text, `include "ie64.inc"`) || strings.Contains(text, `include "ie64_fp.inc"`) ||
		strings.Contains(text, `.include "ie64.inc"`) || strings.Contains(text, `.include "ie64_fp.inc"`) {
		want = "ie64"
	}
	if strings.Contains(text, `include "ie32.inc"`) || strings.Contains(text, `.include "ie32.inc"`) {
		want = "ie32"
	}
	if want != ex.arch {
		t.Fatalf("manifest arch for %s = %s, Makefile-style scan wants %s", ex.source, ex.arch, want)
	}
}

func assemblerExamplesRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
