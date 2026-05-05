package main

import (
	"os"
	"strings"
	"testing"
)

func TestPinnedFrontendDeps(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("ReadFile(go.mod): %v", err)
	}

	mod := string(data)
	want := []string{
		"github.com/hajimehoshi/ebiten/v2 v2.10.0-alpha.11.0.20260419134110-e144fc3fc9ad",
		"github.com/ebitengine/purego v0.11.0-alpha.2",
	}

	for _, needle := range want {
		if !strings.Contains(mod, needle) {
			t.Fatalf("go.mod missing pinned dependency %q", needle)
		}
	}
}
