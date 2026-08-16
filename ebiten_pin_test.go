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
		"github.com/hajimehoshi/ebiten/v2 v2.10.0-alpha.13",
		"github.com/ebitengine/purego v0.11.0-alpha.8",
	}

	for _, needle := range want {
		if !strings.Contains(mod, needle) {
			t.Fatalf("go.mod missing pinned dependency %q", needle)
		}
	}
}
