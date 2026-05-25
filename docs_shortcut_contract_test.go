package main

import (
	"os"
	"strings"
	"testing"
)

func TestDocsShortcutMappingHasNoStaleF11Phrases(t *testing.T) {
	files := []string{
		"README.md",
		"sdk/docs/compositor.md",
		"sdk/docs/architecture.md",
	}
	stale := []string{
		"F11 toggles fullscreen",
		"F11 toggles between fullscreen and windowed",
		"| `F11` | Toggle fullscreen. |",
		"| `Shift+F11` | Toggle fit/stretch scaling",
		"`Shift+F11` toggles stretch-fill",
		"can be stretch-filled with `Shift+F11`",
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(body)
		for _, phrase := range stale {
			if strings.Contains(text, phrase) {
				t.Fatalf("%s contains stale shortcut phrase %q", file, phrase)
			}
		}
	}
}
