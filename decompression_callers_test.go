package main

import (
	"os"
	"path/filepath"
	"testing"
)

func malformedLHAForCallerTest() []byte {
	data := buildLHALevel0("-lh0-", []byte("x"), []byte("x"))
	data[1] ^= 0xff
	return data
}

func TestPSGLoadDataReturnsMatchedLHAError(t *testing.T) {
	p := NewPSGPlayer(nil)
	if err := p.LoadData(malformedLHAForCallerTest()); err == nil {
		t.Fatal("expected malformed LHA error")
	}
}

func TestRenderPSGDataReturnsMatchedLHAError(t *testing.T) {
	if _, err := renderPSGData(malformedLHAForCallerTest(), 44100); err == nil {
		t.Fatal("expected malformed LHA error")
	}
}

func TestParseYMFileReturnsLHADecompressionError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.ym")
	if err := os.WriteFile(path, malformedLHAForCallerTest(), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseYMFile(path); err == nil {
		t.Fatal("expected malformed LHA error")
	}
}
