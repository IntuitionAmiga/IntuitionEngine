package main

import (
	"testing"
	"unsafe"
)

func TestConfigureArosVRAM_UsesDedicatedArosRange(t *testing.T) {
	sysBus := NewMachineBus()
	videoChip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip() failed: %v", err)
	}

	got := configureArosVRAM(sysBus, videoChip)

	if len(got) != arosDirectVRAMSize {
		t.Fatalf("direct VRAM len = %d, want %d", len(got), arosDirectVRAMSize)
	}
	if len(videoChip.directVRAM) != arosDirectVRAMSize {
		t.Fatalf("video chip direct VRAM len = %d, want %d", len(videoChip.directVRAM), arosDirectVRAMSize)
	}

	wantBase := uintptr(unsafe.Pointer(&sysBus.memory[arosDirectVRAMBase]))
	gotBase := uintptr(unsafe.Pointer(&got[0]))
	chipBase := uintptr(unsafe.Pointer(&videoChip.directVRAM[0]))
	if gotBase != wantBase || chipBase != wantBase {
		t.Fatalf("AROS VRAM base mismatch: helper=0x%X chip=0x%X want=0x%X", gotBase, chipBase, wantBase)
	}
	if !videoChip.bigEndianMode {
		t.Fatal("AROS VRAM must enable big-endian mode")
	}
	if videoChip.busMemory == nil {
		t.Fatal("AROS VRAM must use bus-backed video memory")
	}
}
