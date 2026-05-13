package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymbolSidecar_LoadsVICELabelNextToMedia(t *testing.T) {
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "song.sid")
	labelPath := filepath.Join(dir, "song.lbl")
	if err := os.WriteFile(mediaPath, []byte("dummy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(labelPath, []byte("al C:1000 .play\n"), 0600); err != nil {
		t.Fatal(err)
	}

	symbols := NewSymbolTable()
	loaded, err := loadVICELabelSidecar(symbols, "6502", mediaPath, 0)
	if err != nil {
		t.Fatalf("loadVICELabelSidecar: %v", err)
	}
	if loaded != labelPath {
		t.Fatalf("loaded sidecar = %q, want %q", loaded, labelPath)
	}
	if addr, ok := symbols.Lookup("6502", "play"); !ok || addr != 0x1000 {
		t.Fatalf("symbol play = %#x, %v; want 0x1000,true", addr, ok)
	}
}

func TestMediaLoader_LoadSymbolSidecarUsesMediaCPU(t *testing.T) {
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "tune.ym")
	if err := os.WriteFile(mediaPath, []byte("dummy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tune.lbl"), []byte("al 0:0040 .init\n"), 0600); err != nil {
		t.Fatal(err)
	}

	symbols := NewSymbolTable()
	loader := NewMediaLoader(nil, nil, dir, nil, nil, nil, nil, nil, nil, nil)
	loader.SetSymbolTable(symbols)
	loader.loadSymbolSidecar(mediaPath, MEDIA_TYPE_PSG)

	if addr, ok := symbols.Lookup("Z80", "init"); !ok || addr != 0x40 {
		t.Fatalf("Z80 init = %#x, %v; want 0x40,true", addr, ok)
	}
}
