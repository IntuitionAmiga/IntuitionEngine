package main

import (
	"testing"
	"unsafe"
)

func TestConfigureArosVRAM_UsesDedicatedArosRange(t *testing.T) {
	sysBus, err := NewMachineBusSized(arosDirectVRAMBase + arosDirectVRAMSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized() failed: %v", err)
	}
	videoChip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip() failed: %v", err)
	}

	got, err := configureArosVRAM(sysBus, videoChip)
	if err != nil {
		t.Fatalf("configureArosVRAM() failed: %v", err)
	}

	if len(got) != arosDirectVRAMSize {
		t.Fatalf("direct VRAM len = %d, want %d", len(got), arosDirectVRAMSize)
	}
	if arosDirectVRAMBase != 0x1E00000 || arosDirectVRAMSize != 0x4000000 {
		t.Fatalf("AROS direct VRAM contract got base=0x%X size=0x%X, want 0x1E00000/0x4000000",
			arosDirectVRAMBase, arosDirectVRAMSize)
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

func TestConfigureArosVRAM_RejectsUndersizedBus(t *testing.T) {
	sysBus := NewMachineBus()
	videoChip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip() failed: %v", err)
	}

	if _, err := configureArosVRAM(sysBus, videoChip); err == nil {
		t.Fatal("configureArosVRAM() succeeded on undersized 32 MiB bus")
	}
}

func TestConfigureArosVRAM_1920x1080RGBA32AndCLUT8(t *testing.T) {
	sysBus, err := NewMachineBusSized(arosDirectVRAMBase + arosDirectVRAMSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized() failed: %v", err)
	}
	videoChip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip() failed: %v", err)
	}
	videoChip.AttachBus(sysBus)
	sysBus.MapIO(VIDEO_CTRL, VIDEO_REG_END, videoChip.HandleRead, videoChip.HandleWrite)
	sysBus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, videoChip.HandleWrite8)
	if _, err := configureArosVRAM(sysBus, videoChip); err != nil {
		t.Fatalf("configureArosVRAM() failed: %v", err)
	}

	mode := VideoModes[MODE_1920x1080]
	sysBus.Write32(VIDEO_CTRL, 1)
	sysBus.Write32(VIDEO_MODE, MODE_1920x1080)
	sysBus.Write32(VIDEO_FB_BASE, arosDirectVRAMBase)
	sysBus.Write32(VIDEO_COLOR_MODE, 0)
	copy(sysBus.memory[arosDirectVRAMBase:arosDirectVRAMBase+4], []byte{0x11, 0x22, 0x33, 0x44})
	if frame := videoChip.FinishFrame(); len(frame) != mode.totalSize {
		t.Fatalf("RGBA32 frame len = %d, want %d", len(frame), mode.totalSize)
	}
	if got := sysBus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr != 0 {
		t.Fatalf("RGBA32 VIDEO_STATUS framebuffer error: 0x%X", got)
	}

	sysBus.Write32(VIDEO_COLOR_MODE, 1)
	sysBus.Write32(VIDEO_PAL_TABLE+3*4, 0x00A0B0C0)
	sysBus.memory[arosDirectVRAMBase] = 3
	frame := videoChip.FinishFrame()
	if len(frame) != mode.totalSize {
		t.Fatalf("CLUT8 frame len = %d, want %d", len(frame), mode.totalSize)
	}
	assertRGBA(t, frame, 0, 0xA0, 0xB0, 0xC0, 0xFF)
	if got := sysBus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr != 0 {
		t.Fatalf("CLUT8 VIDEO_STATUS framebuffer error: 0x%X", got)
	}
}
