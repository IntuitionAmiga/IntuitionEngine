package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../x", "assembler/../x.go", "/tmp/x.go"} {
		if _, err := safeJoin(root, rel); err == nil {
			t.Fatalf("safeJoin(%q) succeeded, want error", rel)
		}
	}
}

func TestGeneratorWritesKnownTargetsOnly(t *testing.T) {
	root := t.TempDir()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"gen_ie64_opmeta", "-out", root}
	main()

	want := map[string]bool{}
	for _, target := range targets() {
		want[target.path] = true
		if _, err := os.Stat(filepath.Join(root, target.path)); err != nil {
			t.Fatalf("%s not written: %v", target.path, err)
		}
	}

	var got []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		got = append(got, rel)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("written files = %v, want exactly %v", got, want)
	}
	for _, rel := range got {
		if !want[rel] {
			t.Fatalf("unexpected generated file %s", rel)
		}
	}
}
