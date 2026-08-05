package main

import (
	"os"
	"strings"
	"testing"
)

func TestP65JITDocumentationPlatformClaims(t *testing.T) {
	paths := []string{"sdk/docs/6502_JIT.md", "sdk/docs/architecture.md", "sdk/docs/iescript.md"}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		if strings.Contains(text, "x86-64/macOS, and x86-64/Windows") || strings.Contains(text, "6502 JIT<br/>amd64") {
			t.Errorf("%s contains obsolete non-Linux 6502 JIT availability claim", path)
		}
	}

	doc, err := os.ReadFile("sdk/docs/6502_JIT.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "Linux AMD64") || !strings.Contains(string(doc), "Windows and macOS retain 6502 interpreter support only") {
		t.Fatal("6502 JIT documentation does not state Linux-only JIT and retained desktop interpreter support")
	}
	if !strings.Contains(string(doc), "`--nojit`") || strings.Contains(string(doc), "`--no-jit`") {
		t.Fatal("6502 JIT documentation does not use the registered --nojit flag")
	}
}
