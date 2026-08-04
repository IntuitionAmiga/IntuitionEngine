package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

func TestRunWritesImageToStandardOutput(t *testing.T) {
	dir := t.TempDir()
	objPath := filepath.Join(dir, "start.o")
	obj := &ie64obj.Object{Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8)}}, Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1}}}
	b, err := obj.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	code := run([]string{"-o", "-", objPath})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(got) != 8 {
		t.Fatalf("run/output = %d/%d bytes, want 0/8", code, len(got))
	}
	if _, err := os.Stat(filepath.Join(dir, "-")); !os.IsNotExist(err) {
		t.Fatalf("- became an output file: %v", err)
	}
}

func TestRunWritesImageAndMap(t *testing.T) {
	dir := t.TempDir()
	objPath := filepath.Join(dir, "start.o")
	out := filepath.Join(dir, "a.ie64")
	mp := filepath.Join(dir, "a.map")
	obj := &ie64obj.Object{Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8)}}, Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1}}}
	b, err := obj.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(objPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-o", out, "--map", mp, objPath}); code != 0 {
		t.Fatalf("run = %d", code)
	}
	if got, err := os.ReadFile(out); err != nil || len(got) != 8 {
		t.Fatalf("image len/error = %d/%v", len(got), err)
	}
	if got, err := os.ReadFile(mp); err != nil || len(got) == 0 {
		t.Fatalf("map len/error = %d/%v", len(got), err)
	}
}

func TestRunRefusesOutputInputAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same.o")
	if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-o", path, path}); code == 0 {
		t.Fatal("expected failure")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "keep" {
		t.Fatalf("input changed to %q", got)
	}
}

func TestRunRefusesMapCollisions(t *testing.T) {
	dir := t.TempDir()
	objPath := filepath.Join(dir, "start.o")
	out := filepath.Join(dir, "a.ie64")
	obj := &ie64obj.Object{Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8)}}, Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1}}}
	b, err := obj.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	inputLink := filepath.Join(dir, "input-link")
	if err := os.Symlink(objPath, inputLink); err != nil {
		t.Fatal(err)
	}
	inputHardlink := filepath.Join(dir, "input-hardlink")
	if err := os.Link(objPath, inputHardlink); err != nil {
		t.Fatal(err)
	}
	for _, mapPath := range []string{objPath, inputLink, inputHardlink, out} {
		if code := run([]string{"-o", out, "--map", mapPath, objPath}); code == 0 {
			t.Fatalf("map collision %s was accepted", mapPath)
		}
		got, err := os.ReadFile(objPath)
		if err != nil || string(got) != string(b) {
			t.Fatalf("input changed after map collision: %v", err)
		}
	}
}
