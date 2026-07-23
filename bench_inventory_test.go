package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestBenchmarkInventory_DocumentedBenchesExist parses the Programme Benchmark
// Inventory table in architecture.md and asserts that every benchmark it names
// still exists as a function in the sources. This closes the documentation ->
// code direction: a benchmark rename that is not reflected in the inventory,
// or an inventory entry that never existed, fails here rather than leaving a
// dead gate in the docs.
func TestBenchmarkInventory_DocumentedBenchesExist(t *testing.T) {
	docBytes, err := os.ReadFile("sdk/docs/architecture.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(docBytes)

	const heading = "### Programme Benchmark Inventory"
	start := strings.Index(doc, heading)
	if start < 0 {
		t.Fatalf("architecture.md missing %q section", heading)
	}
	section := doc[start:]
	if end := strings.Index(section[len(heading):], "\n### "); end >= 0 {
		section = section[:len(heading)+end]
	}

	// Only backtick-quoted names are inventory entries; the column header and
	// prose word "Benchmarks" must not be mistaken for one.
	benchRe := regexp.MustCompile("`(Benchmark[A-Za-z0-9_]+)`")
	matches := benchRe.FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		t.Fatal("no benchmark names found in the inventory section")
	}

	code := productionAndTestGoSources(t)
	seen := map[string]bool{}
	for _, m := range matches {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if !strings.Contains(code, "func "+name+"(") {
			t.Errorf("inventory names %s but no such benchmark function exists", name)
		}
	}
}

// productionAndTestGoSources concatenates every .go file in the repo root,
// tests included, since benchmark functions live in _test.go files.
func productionAndTestGoSources(t *testing.T) string {
	t.Helper()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return sb.String()
}
