//go:build !wasm

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestX86JIT_WebDemoDistributionIncludesParityWorkloads(t *testing.T) {
	manifestPath := filepath.Join("intuitionengine.com", "assets", "MANIFEST")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read %s: %v", manifestPath, err)
	}
	manifest := string(manifestBytes)
	for _, rel := range []string{
		"Demos/x86/rotozoomer_x86.ie86",
		"Demos/x86/antic_plasma_x86.ie86",
		"Demos/x86/iedoom.ie86",
	} {
		if !strings.Contains(manifest, rel) {
			t.Fatalf("%s missing manifest entry %q", manifestPath, rel)
		}
		full := filepath.Join("intuitionengine.com", "assets", rel)
		st, err := os.Stat(full)
		if err != nil {
			t.Fatalf("missing distribution copy %s: %v", full, err)
		}
		if st.IsDir() {
			t.Fatalf("distribution copy %s is a directory", full)
		}
	}
}
