package main

// Legacy AOT removal contract from IE64_BASIC_COMPILER_REPLACEMENT_PLAN.md.
// After both milestones pass the obsolete implementation is deleted: no
// runtime blob, no AOT support includes, no blob embedding, no build rules
// that regenerate the deleted artefacts, and no live source references to
// any of them. These tests pin that end state.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Files the plan lists for deletion once no live consumer remains.
var obsoleteAOTFiles = []string{
	"sdk/include/ehbasic_aot.inc",
	"sdk/include/aot_consttab.inc",
	"sdk/include/aot_runtime_blob.asm",
	"sdk/include/aot_runtime_blob.bin",
	"sdk/include/aot_runtime_stubs.inc",
	"runtime_blob_embed.go",
	"tools/gen_runtime_blob",
}

func TestLegacyAOTSupportFilesAreRemoved(t *testing.T) {
	repo := repoRootDir(t)
	for _, rel := range obsoleteAOTFiles {
		if _, err := os.Stat(filepath.Join(repo, rel)); !os.IsNotExist(err) {
			t.Errorf("obsolete legacy AOT artefact still present: %s", rel)
		}
	}
}

func TestNoLiveSourceReferencesLegacyAOTArtefacts(t *testing.T) {
	repo := repoRootDir(t)
	// Live source trees that must not reference the deleted artefacts. Plan
	// documents themselves may still name them.
	roots := []string{"sdk/include", "sdk/examples/asm", "assembler", "tools", "cmd"}
	needles := []string{"aot_runtime_blob", "aot_runtime_stubs", "aot_consttab", "ehbasic_aot.inc"}
	for _, root := range roots {
		err := filepath.Walk(filepath.Join(repo, root), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			switch filepath.Ext(path) {
			case ".inc", ".asm", ".go", ".s":
			default:
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			for _, n := range needles {
				if strings.Contains(string(data), n) {
					t.Errorf("%s still references %q", strings.TrimPrefix(path, repo+"/"), n)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestGoRuntimeDoesNotEmbedBasicRuntimeBlob(t *testing.T) {
	repo := repoRootDir(t)
	entries, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		// Tests may keep the blob name inside forbidden-reference lists; only
		// shipping runtime sources must be clean.
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repo, e.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "aot_runtime_blob") {
			t.Errorf("%s still embeds or seeds the legacy runtime blob", e.Name())
		}
	}
}

func TestMakefileHasNoRuntimeBlobRules(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRootDir(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"aot-runtime-blob", "gen_runtime_blob", "aot_runtime_blob"} {
		if strings.Contains(string(data), needle) {
			t.Errorf("Makefile still contains legacy blob rule reference %q", needle)
		}
	}
}

// The private in-guest assembler keeps its generated constant table; it is a
// live non-AOT consumer, so the table and its generator survive under names
// that reflect that ownership.
func TestAssemblerConstantTableRetainedUnderNewName(t *testing.T) {
	repo := repoRootDir(t)
	if _, err := os.Stat(filepath.Join(repo, "sdk", "include", "ehbasic_assembler_consttab.inc")); err != nil {
		t.Errorf("assembler constant table missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "tools", "gen_assembler_consttab", "main.go"))
	if err != nil {
		t.Fatalf("constant table generator missing: %v", err)
	}
	if !strings.Contains(string(data), "ehbasic_assembler_consttab.inc") {
		t.Error("constant table generator does not emit the renamed table")
	}
}
