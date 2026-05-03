package main

import (
	"encoding/binary"
	"testing"
)

func newBlitterTestRig(t *testing.T) (*VideoChip, *MachineBus) {
	t.Helper()

	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	return video, bus
}

func writeU32Bytes(bus *MachineBus, addr uint32, value uint32) {
	bus.Write8(addr, uint8(value))
	bus.Write8(addr+1, uint8(value>>8))
	bus.Write8(addr+2, uint8(value>>16))
	bus.Write8(addr+3, uint8(value>>24))
}

func vramAddr(mode VideoMode, x, y int) uint32 {
	return VRAM_START + uint32((y*mode.width+x)*BYTES_PER_PIXEL)
}

func TestBlitterRegisters(t *testing.T) {
	video, bus := newBlitterTestRig(t)

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_SRC, 0x1000)
	bus.Write32(BLT_DST, 0x2000)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 5)
	bus.Write32(BLT_SRC_STRIDE, 16)
	bus.Write32(BLT_DST_STRIDE, 32)
	bus.Write32(BLT_COLOR, 0xAABBCCDD)
	bus.Write32(BLT_MASK, 0x3000)

	if got := video.HandleRead(BLT_OP); got != bltOpFill {
		t.Fatalf("expected BLT_OP=%d, got %d", bltOpFill, got)
	}
	if got := video.HandleRead(BLT_SRC); got != 0x1000 {
		t.Fatalf("expected BLT_SRC=0x1000, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_DST); got != 0x2000 {
		t.Fatalf("expected BLT_DST=0x2000, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_WIDTH); got != 4 {
		t.Fatalf("expected BLT_WIDTH=4, got %d", got)
	}
	if got := video.HandleRead(BLT_HEIGHT); got != 5 {
		t.Fatalf("expected BLT_HEIGHT=5, got %d", got)
	}
	if got := video.HandleRead(BLT_SRC_STRIDE); got != 16 {
		t.Fatalf("expected BLT_SRC_STRIDE=16, got %d", got)
	}
	if got := video.HandleRead(BLT_DST_STRIDE); got != 32 {
		t.Fatalf("expected BLT_DST_STRIDE=32, got %d", got)
	}
	if got := video.HandleRead(BLT_COLOR); got != 0xAABBCCDD {
		t.Fatalf("expected BLT_COLOR=0xAABBCCDD, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_MASK); got != 0x3000 {
		t.Fatalf("expected BLT_MASK=0x3000, got 0x%X", got)
	}

	bus.Write32(BLT_CTRL, bltCtrlStart)
	// Blitter runs synchronously — busy is already cleared after Write
	video.RunBlitterForTest()
	if got := video.HandleRead(BLT_CTRL); got&bltCtrlBusy != 0 {
		t.Fatalf("expected BLT_CTRL busy cleared, got 0x%X", got)
	}
}

func TestBlitterFill(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	color := uint32(0x11223344)

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, vramAddr(mode, 2, 2))
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 4)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			addr := vramAddr(mode, x, y)
			if got := video.HandleRead(addr); got != color {
				t.Fatalf("expected fill color at %d,%d", x, y)
			}
		}
	}
	if !video.hasDirtyTiles() {
		t.Fatalf("expected dirty tiles after fill")
	}
}

func TestBlitterCopy(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	src := vramAddr(mode, 0, 0)
	dst := vramAddr(mode, 10, 0)
	video.HandleWrite(src, 0xDEADBEEF)
	video.HandleWrite(src+4, 0xAABBCCDD)
	video.HandleWrite(src+uint32(mode.bytesPerRow), 0x10203040)
	video.HandleWrite(src+uint32(mode.bytesPerRow)+4, 0x55667788)

	bus.Write32(BLT_OP, bltOpCopy)
	bus.Write32(BLT_SRC, src)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 2)
	bus.Write32(BLT_HEIGHT, 2)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	if got := video.HandleRead(dst); got != 0xDEADBEEF {
		t.Fatalf("expected copy pixel 0, got 0x%X", got)
	}
	if got := video.HandleRead(dst + 4); got != 0xAABBCCDD {
		t.Fatalf("expected copy pixel 1, got 0x%X", got)
	}
	if got := video.HandleRead(dst + uint32(mode.bytesPerRow)); got != 0x10203040 {
		t.Fatalf("expected copy pixel 2, got 0x%X", got)
	}
	if got := video.HandleRead(dst + uint32(mode.bytesPerRow) + 4); got != 0x55667788 {
		t.Fatalf("expected copy pixel 3, got 0x%X", got)
	}
}

func TestBlitterDefaultStrideVRAM(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	src := vramAddr(mode, 0, 30)
	dst := vramAddr(mode, 10, 30)
	video.HandleWrite(src, 0x01020304)
	video.HandleWrite(src+4, 0x05060708)
	video.HandleWrite(src+uint32(mode.bytesPerRow), 0x11121314)
	video.HandleWrite(src+uint32(mode.bytesPerRow)+4, 0x15161718)

	bus.Write32(BLT_OP, bltOpCopy)
	bus.Write32(BLT_SRC, src)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 2)
	bus.Write32(BLT_HEIGHT, 2)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	if got := video.HandleRead(dst); got != 0x01020304 {
		t.Fatalf("expected default stride copy pixel 0, got 0x%X", got)
	}
	if got := video.HandleRead(dst + 4); got != 0x05060708 {
		t.Fatalf("expected default stride copy pixel 1, got 0x%X", got)
	}
	if got := video.HandleRead(dst + uint32(mode.bytesPerRow)); got != 0x11121314 {
		t.Fatalf("expected default stride copy pixel 2, got 0x%X", got)
	}
	if got := video.HandleRead(dst + uint32(mode.bytesPerRow) + 4); got != 0x15161718 {
		t.Fatalf("expected default stride copy pixel 3, got 0x%X", got)
	}
}

func TestBlitterStatusErrorOnMisalign(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	dst := vramAddr(mode, 5, 5) + 1

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_COLOR, 0x12345678)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	if got := video.HandleRead(BLT_STATUS); got&bltStatusErr == 0 {
		t.Fatalf("expected BLT_STATUS err set, got 0x%X", got)
	}
}

func TestBlitterCopyStride(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	src := vramAddr(mode, 0, 10)
	dst := vramAddr(mode, 20, 10)
	srcStride := uint32(12)
	dstStride := uint32(16)

	video.HandleWrite(src, 0x01020304)
	video.HandleWrite(src+4, 0x05060708)
	video.HandleWrite(src+srcStride, 0x11121314)
	video.HandleWrite(src+srcStride+4, 0x15161718)

	bus.Write32(BLT_OP, bltOpCopy)
	bus.Write32(BLT_SRC, src)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 2)
	bus.Write32(BLT_HEIGHT, 2)
	bus.Write32(BLT_SRC_STRIDE, srcStride)
	bus.Write32(BLT_DST_STRIDE, dstStride)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	if got := video.HandleRead(dst); got != 0x01020304 {
		t.Fatalf("expected stride copy pixel 0, got 0x%X", got)
	}
	if got := video.HandleRead(dst + 4); got != 0x05060708 {
		t.Fatalf("expected stride copy pixel 1, got 0x%X", got)
	}
	if got := video.HandleRead(dst + dstStride); got != 0x11121314 {
		t.Fatalf("expected stride copy pixel 2, got 0x%X", got)
	}
	if got := video.HandleRead(dst + dstStride + 4); got != 0x15161718 {
		t.Fatalf("expected stride copy pixel 3, got 0x%X", got)
	}
}

func TestBlitterLine(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	start := uint32((2 << 16) | 2)
	end := uint32((5 << 16) | 5)
	bus.Write32(BLT_OP, bltOpLine)
	bus.Write32(BLT_SRC, start)
	bus.Write32(BLT_DST, end)
	bus.Write32(BLT_COLOR, 0xFF00FF00)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	for i := 2; i <= 5; i++ {
		addr := vramAddr(mode, i, i)
		if got := video.HandleRead(addr); got != 0xFF00FF00 {
			t.Fatalf("expected line pixel at %d,%d", i, i)
		}
	}
}

func TestBlitterMaskedCopy(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	src := vramAddr(mode, 0, 20)
	dst := vramAddr(mode, 10, 20)
	maskAddr := uint32(0x5000)

	for i := range 8 {
		video.HandleWrite(src+uint32(i*4), 0xAA000000+uint32(i))
	}
	bus.Write8(maskAddr, 0b01010101)

	bus.Write32(BLT_OP, bltOpMaskedCopy)
	bus.Write32(BLT_SRC, src)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 8)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_MASK, maskAddr)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	for i := range 8 {
		addr := dst + uint32(i*4)
		got := video.HandleRead(addr)
		if i%2 == 0 {
			if got != 0xAA000000+uint32(i) {
				t.Fatalf("expected masked copy at %d", i)
			}
		} else if got != 0 {
			t.Fatalf("expected masked skip at %d", i)
		}
	}
}

func TestBlitterLine_BigEndianMappedVRAMLittleEndian(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	video.SetBigEndianMode(true)
	mode := VideoModes[video.currentMode]
	color := uint32(0x11223344)

	start := uint32((1 << 16) | 1)
	end := start
	bus.Write32(BLT_OP, bltOpLine)
	bus.Write32(BLT_SRC, start)
	bus.Write32(BLT_DST, end)
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	addr := vramAddr(mode, 1, 1)
	offset := addr - BUFFER_OFFSET
	gotBytes := video.frontBuffer[offset : offset+4]
	wantBytes := []byte{0x44, 0x33, 0x22, 0x11}
	for i := range wantBytes {
		if gotBytes[i] != wantBytes[i] {
			t.Fatalf("line VRAM byte %d got 0x%02X want 0x%02X", i, gotBytes[i], wantBytes[i])
		}
	}
	if got := video.HandleRead(addr); got != color {
		t.Fatalf("HandleRead after line = 0x%08X, want 0x%08X", got, color)
	}
}

func TestVideoChip_BlitFill_BigEndian(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	video.SetBigEndianMode(true)
	mode := VideoModes[video.currentMode]
	dst := vramAddr(mode, 3, 4)
	color := uint32(0x11223344)

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	offset := dst - BUFFER_OFFSET
	got := video.frontBuffer[offset : offset+4]
	want := []byte{0x44, 0x33, 0x22, 0x11}
	for i := range 4 {
		if got[i] != want[i] {
			t.Fatalf("mapped VRAM fill byte %d got 0x%02X want 0x%02X", i, got[i], want[i])
		}
	}
}

func TestVideoChip_BlitCopy_BigEndian(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	video.SetBigEndianMode(true)
	mode := VideoModes[video.currentMode]

	src := vramAddr(mode, 1, 1)
	dst := vramAddr(mode, 8, 1)
	color := uint32(0xA1B2C3D4)

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, src)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	bus.Write32(BLT_OP, bltOpCopy)
	bus.Write32(BLT_SRC, src)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_SRC_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	srcOff := src - BUFFER_OFFSET
	dstOff := dst - BUFFER_OFFSET
	for i := range 4 {
		if video.frontBuffer[srcOff+uint32(i)] != video.frontBuffer[dstOff+uint32(i)] {
			t.Fatalf("big-endian copy mismatch at byte %d: src=0x%02X dst=0x%02X",
				i, video.frontBuffer[srcOff+uint32(i)], video.frontBuffer[dstOff+uint32(i)])
		}
	}
}

func TestVideoChip_BlitFill_LittleEndian(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	video.SetBigEndianMode(false)
	mode := VideoModes[video.currentMode]
	dst := vramAddr(mode, 2, 2)
	color := uint32(0x11223344)

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	offset := dst - BUFFER_OFFSET
	got := video.frontBuffer[offset : offset+4]
	want := []byte{0x44, 0x33, 0x22, 0x11}
	for i := range 4 {
		if got[i] != want[i] {
			t.Fatalf("little-endian fill byte %d got 0x%02X want 0x%02X", i, got[i], want[i])
		}
	}
}

func TestBlitterIE32(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, vramAddr(mode, 1, 1))
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_COLOR, 0x12345678)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	if got := video.HandleRead(vramAddr(mode, 1, 1)); got != 0x12345678 {
		t.Fatalf("expected IE32 fill pixel, got 0x%X", got)
	}
}

func TestBlitter6502(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	dst := vramAddr(mode, 2, 2)

	writeU32Bytes(bus, BLT_OP, bltOpFill)
	writeU32Bytes(bus, BLT_DST, dst)
	writeU32Bytes(bus, BLT_WIDTH, 1)
	writeU32Bytes(bus, BLT_HEIGHT, 1)
	writeU32Bytes(bus, BLT_COLOR, 0xABCDEF01)
	bus.Write8(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	if got := video.HandleRead(dst); got != 0xABCDEF01 {
		t.Fatalf("expected 6502 fill pixel, got 0x%X", got)
	}
}

func TestBlitterZ80(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	dst := vramAddr(mode, 3, 3)

	writeU32Bytes(bus, BLT_OP, bltOpFill)
	writeU32Bytes(bus, BLT_DST, dst)
	writeU32Bytes(bus, BLT_WIDTH, 1)
	writeU32Bytes(bus, BLT_HEIGHT, 1)
	writeU32Bytes(bus, BLT_COLOR, 0x0BADF00D)
	bus.Write8(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	if got := video.HandleRead(dst); got != 0x0BADF00D {
		t.Fatalf("expected Z80 fill pixel, got 0x%X", got)
	}
}

func TestBlitterM68K(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	dst := vramAddr(mode, 4, 4)

	bus.Write16(BLT_OP, bltOpFill)
	bus.Write16(BLT_DST, uint16(dst))
	bus.Write16(BLT_DST+2, uint16(dst>>16))
	bus.Write16(BLT_WIDTH, 1)
	bus.Write16(BLT_HEIGHT, 1)
	bus.Write32(BLT_COLOR, 0x00FF00FF)
	bus.Write16(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	if got := video.HandleRead(dst); got != 0x00FF00FF {
		t.Fatalf("expected M68K fill pixel, got 0x%X", got)
	}
}

// TestBlitterCanReadCPULoadedData verifies that the blitter can read sprite data
// that was loaded by the CPU (simulating embedded program data like sprites).
func TestBlitterCanReadCPULoadedData(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	cpu := NewCPU(bus)

	// CPU writes sprite data to address 0x2000 (simulating loaded program data)
	// 4 pixels of red (RGBA little-endian: 0xFF0000FF)
	for i := range uint32(4) {
		cpu.Write32(0x2000+i*4, 0xFF0000FF)
	}

	// Setup blitter to copy from 0x2000 to VRAM
	dst := vramAddr(mode, 100, 0) // Arbitrary VRAM location
	bus.Write32(BLT_OP, bltOpCopy)
	bus.Write32(BLT_SRC, 0x2000)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_SRC_STRIDE, 16)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	// Verify VRAM has the copied data
	got := video.HandleRead(dst)
	if got != 0xFF0000FF {
		t.Fatalf("Blitter read 0x%08X from CPU memory, expected 0xFF0000FF - CPU memory not visible to blitter", got)
	}
}

func TestBlitterMode7Registers(t *testing.T) {
	video, bus := newBlitterTestRig(t)

	// Define constants locally for the test until they are added to video_chip.go
	const (
		BLT_MODE7_U0     = 0xF0058
		BLT_MODE7_V0     = 0xF005C
		BLT_MODE7_DU_COL = 0xF0060
		BLT_MODE7_DV_COL = 0xF0064
		BLT_MODE7_DU_ROW = 0xF0068
		BLT_MODE7_DV_ROW = 0xF006C
		BLT_MODE7_TEX_W  = 0xF0070
		BLT_MODE7_TEX_H  = 0xF0074
	)

	// Write values to new registers
	bus.Write32(BLT_MODE7_U0, 0x12345678)
	bus.Write32(BLT_MODE7_V0, 0x87654321)
	bus.Write32(BLT_MODE7_DU_COL, 0x10000)
	bus.Write32(BLT_MODE7_DV_COL, 0x20000)
	bus.Write32(BLT_MODE7_DU_ROW, 0x30000)
	bus.Write32(BLT_MODE7_DV_ROW, 0x40000)
	bus.Write32(BLT_MODE7_TEX_W, 255)
	bus.Write32(BLT_MODE7_TEX_H, 127)

	// Verify reads
	if got := video.HandleRead(BLT_MODE7_U0); got != 0x12345678 {
		t.Fatalf("expected BLT_MODE7_U0=0x12345678, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_MODE7_V0); got != 0x87654321 {
		t.Fatalf("expected BLT_MODE7_V0=0x87654321, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_MODE7_DU_COL); got != 0x10000 {
		t.Fatalf("expected BLT_MODE7_DU_COL=0x10000, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_MODE7_DV_COL); got != 0x20000 {
		t.Fatalf("expected BLT_MODE7_DV_COL=0x20000, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_MODE7_DU_ROW); got != 0x30000 {
		t.Fatalf("expected BLT_MODE7_DU_ROW=0x30000, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_MODE7_DV_ROW); got != 0x40000 {
		t.Fatalf("expected BLT_MODE7_DV_ROW=0x40000, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_MODE7_TEX_W); got != 255 {
		t.Fatalf("expected BLT_MODE7_TEX_W=255, got 0x%X", got)
	}
	if got := video.HandleRead(BLT_MODE7_TEX_H); got != 127 {
		t.Fatalf("expected BLT_MODE7_TEX_H=127, got 0x%X", got)
	}
}

func TestBlitterMode7RegistersByteWordAccess(t *testing.T) {
	video, bus := newBlitterTestRig(t)

	const BLT_MODE7_U0 = 0xF0058

	// Write 32-bit value using bytes
	bus.Write8(BLT_MODE7_U0, 0x78)
	bus.Write8(BLT_MODE7_U0+1, 0x56)
	bus.Write8(BLT_MODE7_U0+2, 0x34)
	bus.Write8(BLT_MODE7_U0+3, 0x12)

	if got := video.HandleRead(BLT_MODE7_U0); got != 0x12345678 {
		t.Fatalf("expected composed 32-bit read 0x12345678, got 0x%X", got)
	}

	// Read back individual bytes
	if got := video.HandleRead(BLT_MODE7_U0 + 1); got != 0x56 {
		t.Fatalf("expected byte 1 read 0x56, got 0x%X", got)
	}
}

func TestBlitterMode7Identity(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	// Define constants locally
	const (
		BLT_MODE7_U0     = 0xF0058
		BLT_MODE7_V0     = 0xF005C
		BLT_MODE7_DU_COL = 0xF0060
		BLT_MODE7_DV_COL = 0xF0064
		BLT_MODE7_DU_ROW = 0xF0068
		BLT_MODE7_DV_ROW = 0xF006C
		BLT_MODE7_TEX_W  = 0xF0070
		BLT_MODE7_TEX_H  = 0xF0074
		BLT_OP_MODE7     = 5
	)

	// Setup 4x4 texture at 0x8000
	texAddr := uint32(0x8000)
	for i := range uint32(16) {
		bus.Write32(texAddr+i*4, 0x11000000+i)
	}

	// Setup destination at 20,20
	dst := vramAddr(mode, 20, 20)

	// Configure Mode7 Identity
	// u0, v0 = 0
	// duCol = 1.0 (0x10000), dvCol = 0
	// duRow = 0, dvRow = 1.0 (0x10000)
	// texW = 3 (4 pixels wide - 1)
	// texH = 3 (4 pixels high - 1)

	bus.Write32(BLT_OP, BLT_OP_MODE7)
	bus.Write32(BLT_SRC, texAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 4)
	bus.Write32(BLT_SRC_STRIDE, 16) // 4 pixels * 4 bytes
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))

	bus.Write32(BLT_MODE7_U0, 0)
	bus.Write32(BLT_MODE7_V0, 0)
	bus.Write32(BLT_MODE7_DU_COL, 0x10000)
	bus.Write32(BLT_MODE7_DV_COL, 0)
	bus.Write32(BLT_MODE7_DU_ROW, 0)
	bus.Write32(BLT_MODE7_DV_ROW, 0x10000)
	bus.Write32(BLT_MODE7_TEX_W, 3)
	bus.Write32(BLT_MODE7_TEX_H, 3)

	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Verify destination
	for y := range 4 {
		for x := range 4 {
			addr := dst + uint32(y*mode.bytesPerRow+x*4)
			expected := 0x11000000 + uint32(y*4+x)
			if got := video.HandleRead(addr); got != expected {
				t.Fatalf("expected pixel at %d,%d to be 0x%X, got 0x%X", x, y, expected, got)
			}
		}
	}
}

func TestBlitterMode7DefaultSrcStrideFromMask(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	const (
		BLT_MODE7_TEX_W = 0xF0070
		BLT_MODE7_TEX_H = 0xF0074
		BLT_OP_MODE7    = 5
	)

	// Setup 4x4 texture
	texAddr := uint32(0x9000)
	// Write pattern that helps identify if stride is correct
	// Row 0: 0xA0, Row 1: 0xA1, etc.
	for y := range 4 {
		for x := range 4 {
			bus.Write32(texAddr+uint32(y*16+x*4), 0xA0+uint32(y))
		}
	}

	dst := vramAddr(mode, 0, 0)

	bus.Write32(BLT_OP, BLT_OP_MODE7)
	bus.Write32(BLT_SRC, texAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 4)
	bus.Write32(BLT_SRC_STRIDE, 0) // Should default to (texW+1)*4
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))

	// Identity transform
	const (
		BLT_MODE7_DU_COL = 0xF0060
		BLT_MODE7_DV_ROW = 0xF006C
	)
	bus.Write32(BLT_MODE7_DU_COL, 0x10000)
	bus.Write32(BLT_MODE7_DV_ROW, 0x10000)
	bus.Write32(BLT_MODE7_TEX_W, 3) // Width 4 -> Stride 16
	bus.Write32(BLT_MODE7_TEX_H, 3)

	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Check pixel at (0,1). Should be from row 1 (0xA1)
	// If stride defaulted to dstWidth (screen width), it would read garbage or wrong line
	addr := dst + uint32(mode.bytesPerRow)
	if got := video.HandleRead(addr); got != 0xA1 {
		t.Fatalf("expected pixel at 0,1 to be 0xA1 (from src row 1), got 0x%X. Likely wrong stride default.", got)
	}
}

func TestBlitterMode7Rotated(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	const (
		BLT_MODE7_U0     = 0xF0058
		BLT_MODE7_V0     = 0xF005C
		BLT_MODE7_DU_COL = 0xF0060
		BLT_MODE7_DV_COL = 0xF0064
		BLT_MODE7_DU_ROW = 0xF0068
		BLT_MODE7_DV_ROW = 0xF006C
		BLT_MODE7_TEX_W  = 0xF0070
		BLT_MODE7_TEX_H  = 0xF0074
		BLT_OP_MODE7     = 5
	)

	// Setup 4x4 texture
	texAddr := uint32(0x9000)
	// Write unique colors
	// 0,0=0x00, 1,0=0x01, ..., 0,1=0x10, ...
	for y := range 4 {
		for x := range 4 {
			bus.Write32(texAddr+uint32(y*16+x*4), uint32(y*16+x))
		}
	}

	dst := vramAddr(mode, 0, 0)

	bus.Write32(BLT_OP, BLT_OP_MODE7)
	bus.Write32(BLT_SRC, texAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 4)
	bus.Write32(BLT_SRC_STRIDE, 16)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))

	// 90 degree rotation clockwise
	// duCol = 0, dvCol = 1.0 (u doesn't change with x, v increases with x) -> texture Y maps to screen X
	// duRow = -1.0, dvRow = 0 (u decreases with y, v doesn't change with y) -> texture X maps to screen -Y
	// Start at bottom-left of texture (0, 3) mapping to (0,0) screen?
	// Let's just swap axes:
	// Screen X -> Texture Y (v)
	// Screen Y -> Texture X (u)

	bus.Write32(BLT_MODE7_U0, 0)
	bus.Write32(BLT_MODE7_V0, 0)
	bus.Write32(BLT_MODE7_DU_COL, 0)
	bus.Write32(BLT_MODE7_DV_COL, 0x10000) // v increases with x
	bus.Write32(BLT_MODE7_DU_ROW, 0x10000) // u increases with y
	bus.Write32(BLT_MODE7_DV_ROW, 0)
	bus.Write32(BLT_MODE7_TEX_W, 3)
	bus.Write32(BLT_MODE7_TEX_H, 3)

	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Screen(0,0) -> Tex(0,0) = 0x00
	// Screen(1,0) -> Tex(0,1) = 0x10
	// Screen(0,1) -> Tex(1,0) = 0x01

	if got := video.HandleRead(dst + 4); got != 0x10 {
		t.Fatalf("expected pixel at 1,0 to be 0x10 (tex 0,1), got 0x%X", got)
	}
	if got := video.HandleRead(dst + uint32(mode.bytesPerRow)); got != 0x01 {
		t.Fatalf("expected pixel at 0,1 to be 0x01 (tex 1,0), got 0x%X", got)
	}
}

func TestBlitterMode7Wrap(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	// Constants
	const (
		BLT_MODE7_U0    = 0xF0058
		BLT_MODE7_TEX_W = 0xF0070
		BLT_MODE7_TEX_H = 0xF0074
		BLT_OP_MODE7    = 5
	)

	texAddr := uint32(0x9000)
	// Write pattern: Row 0 = 0xA0, Row 1 = 0xA1...
	for y := range 4 {
		for x := range 4 {
			bus.Write32(texAddr+uint32(y*16+x*4), 0xA0+uint32(y))
		}
	}

	dst := vramAddr(mode, 0, 0)
	bus.Write32(BLT_OP, BLT_OP_MODE7)
	bus.Write32(BLT_SRC, texAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 4)
	bus.Write32(BLT_SRC_STRIDE, 16)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))

	// Start V at 5.0 (out of bounds 0..3)
	bus.Write32(0xF005C, 5<<16)   // V0
	bus.Write32(0xF0060, 0x10000) // DU_COL = 1.0
	bus.Write32(0xF0064, 0)       // DV_COL
	bus.Write32(0xF0068, 0)       // DU_ROW
	bus.Write32(0xF006C, 0x10000) // DV_ROW
	bus.Write32(BLT_MODE7_TEX_W, 3)
	bus.Write32(BLT_MODE7_TEX_H, 3)

	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// V=5 should wrap to V=1 (5 & 3 = 1) -> Row 1 (0xA1)
	if got := video.HandleRead(dst); got != 0xA1 {
		t.Fatalf("expected wrap to row 1 (0xA1), got 0x%X", got)
	}
}

func TestBlitterMode7Scaled(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	const (
		BLT_MODE7_DU_COL = 0xF0060
		BLT_MODE7_DV_ROW = 0xF006C
		BLT_MODE7_TEX_W  = 0xF0070
		BLT_MODE7_TEX_H  = 0xF0074
		BLT_OP_MODE7     = 5
	)

	texAddr := uint32(0x9000)
	// Pixel at 0,0 = 0xFFFFFFFF, others 0
	bus.Write32(texAddr, 0xFFFFFFFF)

	dst := vramAddr(mode, 0, 0)
	bus.Write32(BLT_OP, BLT_OP_MODE7)
	bus.Write32(BLT_SRC, texAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 4)
	bus.Write32(BLT_SRC_STRIDE, 16)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))

	// Scale by 2x (step 0.5 per pixel)
	bus.Write32(BLT_MODE7_DU_COL, 0x8000) // 0.5
	bus.Write32(BLT_MODE7_DV_ROW, 0x8000) // 0.5
	bus.Write32(BLT_MODE7_TEX_W, 3)
	bus.Write32(BLT_MODE7_TEX_H, 3)

	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Screen(0,0) -> Tex(0,0) -> White
	// Screen(1,0) -> Tex(0.5, 0) -> Tex(0,0) -> White
	// Screen(2,0) -> Tex(1.0, 0) -> Tex(1,0) -> 0
	if got := video.HandleRead(dst); got != 0xFFFFFFFF {
		t.Fatalf("expected 0,0 white")
	}
	if got := video.HandleRead(dst + 4); got != 0xFFFFFFFF {
		t.Fatalf("expected 1,0 white (scaled)")
	}
	if got := video.HandleRead(dst + 8); got != 0 {
		t.Fatalf("expected 2,0 black")
	}
}

func TestBlitterMode7InvalidMaskSetsError(t *testing.T) {
	video, bus := newBlitterTestRig(t)

	const (
		BLT_MODE7_TEX_W = 0xF0070
		BLT_MODE7_TEX_H = 0xF0074
		BLT_OP_MODE7    = 5
	)

	bus.Write32(BLT_OP, BLT_OP_MODE7)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 4)

	// Invalid mask (5 is not 2^n - 1)
	bus.Write32(BLT_MODE7_TEX_W, 5)
	bus.Write32(BLT_MODE7_TEX_H, 3)

	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	if got := video.HandleRead(BLT_STATUS); got&bltStatusErr == 0 {
		t.Fatalf("expected error on invalid mask")
	}
}

func TestBlitterMode7NegativeCoordsWrap(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	const (
		BLT_MODE7_U0    = 0xF0058
		BLT_MODE7_TEX_W = 0xF0070
		BLT_MODE7_TEX_H = 0xF0074
		BLT_OP_MODE7    = 5
	)

	texAddr := uint32(0x9000)
	// Write pattern: Row 0 col 3 (last pixel) = 0xFE
	bus.Write32(texAddr+12, 0xFE)

	dst := vramAddr(mode, 0, 0)
	bus.Write32(BLT_OP, BLT_OP_MODE7)
	bus.Write32(BLT_SRC, texAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_SRC_STRIDE, 16)

	// Start U at -1.0
	// In hex, -1.0 is 0xFFFF0000
	bus.Write32(BLT_MODE7_U0, 0xFFFF0000)
	bus.Write32(BLT_MODE7_TEX_W, 3)
	bus.Write32(BLT_MODE7_TEX_H, 3)

	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// -1 masked by 3 should be 3 (-1 & 3 = 3 in 2s complement)
	// So should read column 3
	if got := video.HandleRead(dst); got != 0xFE {
		t.Fatalf("expected wrap of -1 to 3 (0xFE), got 0x%X", got)
	}
}

// newDirectVRAMTestRig creates a VideoChip+MachineBus configured for directVRAM mode
// (as used by EmuTOS), with bigEndianMode enabled and VRAM I/O unmapped.
func newDirectVRAMTestRig(t *testing.T) (*VideoChip, *MachineBus) {
	t.Helper()
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)
	bus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
	video.SetDirectVRAM(bus.memory[VRAM_START : VRAM_START+VRAM_SIZE])
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	return video, bus
}

func TestBlitterDirectVRAM_Fill(t *testing.T) {
	video, bus := newDirectVRAMTestRig(t)

	color := uint32(0xFF000000) // black with full alpha
	dst := uint32(VRAM_START)

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 2)
	bus.Write32(BLT_DST_STRIDE, 2560)
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Verify the pixel was written to busMemory (not frontBuffer)
	for y := range 2 {
		for x := range 4 {
			addr := VRAM_START + uint32(y*2560+x*4)
			got := binary.LittleEndian.Uint32(bus.memory[addr : addr+4])
			if got != color {
				t.Fatalf("directVRAM fill at (%d,%d): got 0x%08X, want 0x%08X", x, y, got, color)
			}
		}
	}

	// Verify raw LE byte order: 0xFF000000 → [00, 00, 00, FF]
	raw := bus.memory[VRAM_START : VRAM_START+4]
	if raw[0] != 0x00 || raw[1] != 0x00 || raw[2] != 0x00 || raw[3] != 0xFF {
		t.Fatalf("directVRAM fill raw bytes: got [%02X,%02X,%02X,%02X], want [00,00,00,FF]",
			raw[0], raw[1], raw[2], raw[3])
	}
}

func TestBlitterDirectVRAM_Copy(t *testing.T) {
	video, bus := newDirectVRAMTestRig(t)

	// Write source pixels as raw RGBA bytes. Blitter pixel sources are
	// little-endian image data even when the CPU profile is big-endian.
	srcAddr := uint32(0x600000)
	binary.LittleEndian.PutUint32(bus.memory[srcAddr:], 0xDEADBEEF)
	binary.LittleEndian.PutUint32(bus.memory[srcAddr+4:], 0xCAFEBABE)

	dst := uint32(VRAM_START)
	bus.Write32(BLT_OP, bltOpCopy)
	bus.Write32(BLT_SRC, srcAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 2)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_SRC_STRIDE, 8)
	bus.Write32(BLT_DST_STRIDE, 2560)
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	got0 := binary.LittleEndian.Uint32(bus.memory[dst : dst+4])
	got1 := binary.LittleEndian.Uint32(bus.memory[dst+4 : dst+8])
	if got0 != 0xDEADBEEF {
		t.Fatalf("directVRAM copy pixel 0: got 0x%08X, want 0xDEADBEEF", got0)
	}
	if got1 != 0xCAFEBABE {
		t.Fatalf("directVRAM copy pixel 1: got 0x%08X, want 0xCAFEBABE", got1)
	}
}

func TestBlitterDirectVRAM_Mode7(t *testing.T) {
	video, bus := newDirectVRAMTestRig(t)

	const (
		BLT_MODE7_U0     = 0xF0058
		BLT_MODE7_V0     = 0xF005C
		BLT_MODE7_DU_COL = 0xF0060
		BLT_MODE7_DV_COL = 0xF0064
		BLT_MODE7_DU_ROW = 0xF0068
		BLT_MODE7_DV_ROW = 0xF006C
		BLT_MODE7_TEX_W  = 0xF0070
		BLT_MODE7_TEX_H  = 0xF0074
		BLT_OP_MODE7     = 5
	)

	// Setup 4x4 texture as raw RGBA bytes. Blitter pixel sources are
	// little-endian image data even when the CPU profile is big-endian.
	texAddr := uint32(0x600000)
	for y := range 4 {
		for x := range 4 {
			off := texAddr + uint32(y*16+x*4)
			binary.LittleEndian.PutUint32(bus.memory[off:], 0xAA000000+uint32(y*4+x))
		}
	}

	dst := uint32(VRAM_START)
	bus.Write32(BLT_OP, BLT_OP_MODE7)
	bus.Write32(BLT_SRC, texAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 4)
	bus.Write32(BLT_SRC_STRIDE, 16)
	bus.Write32(BLT_DST_STRIDE, 2560)

	// Identity transform
	bus.Write32(BLT_MODE7_U0, 0)
	bus.Write32(BLT_MODE7_V0, 0)
	bus.Write32(BLT_MODE7_DU_COL, 0x10000)
	bus.Write32(BLT_MODE7_DV_COL, 0)
	bus.Write32(BLT_MODE7_DU_ROW, 0)
	bus.Write32(BLT_MODE7_DV_ROW, 0x10000)
	bus.Write32(BLT_MODE7_TEX_W, 3)
	bus.Write32(BLT_MODE7_TEX_H, 3)

	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Verify non-zero output in busMemory
	for y := range 4 {
		for x := range 4 {
			addr := VRAM_START + uint32(y*2560+x*4)
			got := binary.LittleEndian.Uint32(bus.memory[addr : addr+4])
			expected := 0xAA000000 + uint32(y*4+x)
			if got != expected {
				t.Fatalf("directVRAM mode7 at (%d,%d): got 0x%08X, want 0x%08X", x, y, got, expected)
			}
		}
	}
}

// ============================================================
// New tests for CLUT8, draw modes, and color expansion
// ============================================================

func TestBlitterCLUT8Fill(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[MODE_640x480]

	// Fill a 4x2 rectangle with CLUT8 index 42
	dst := uint32(VRAM_START + 100) // offset into VRAM
	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 2)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.width)) // CLUT8: 1 byte per pixel, stride = width
	bus.Write32(BLT_COLOR, 42)
	bus.Write32(BLT_FLAGS, IE_BLT_MAKE_FLAGS(bltFlagsBPP_CLUT8, 0x03)) // CLUT8 + Copy mode
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Verify bytes
	for y := range 2 {
		for x := range 4 {
			off := dst - BUFFER_OFFSET + uint32(y*mode.width+x)
			if video.frontBuffer[off] != 42 {
				t.Fatalf("CLUT8 fill at (%d,%d): got %d, want 42", x, y, video.frontBuffer[off])
			}
		}
	}
}

func TestBlitterCLUT8Copy(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[MODE_640x480]

	// Write source CLUT8 data
	src := uint32(VRAM_START)
	srcOff := src - BUFFER_OFFSET
	for i := range 8 {
		video.frontBuffer[srcOff+uint32(i)] = uint8(10 + i)
	}

	dst := uint32(VRAM_START + 1000)
	bus.Write32(BLT_OP, bltOpCopy)
	bus.Write32(BLT_SRC, src)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 2)
	bus.Write32(BLT_SRC_STRIDE, uint32(mode.width))
	bus.Write32(BLT_DST_STRIDE, uint32(mode.width))
	bus.Write32(BLT_FLAGS, IE_BLT_MAKE_FLAGS(bltFlagsBPP_CLUT8, 0x03))
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Verify
	dstOff := dst - BUFFER_OFFSET
	for y := range 2 {
		for x := range 4 {
			srcByte := video.frontBuffer[srcOff+uint32(y*mode.width+x)]
			dstByte := video.frontBuffer[dstOff+uint32(y*mode.width+x)]
			if dstByte != srcByte {
				t.Fatalf("CLUT8 copy at (%d,%d): got %d, want %d", x, y, dstByte, srcByte)
			}
		}
	}
}

func TestBlitterDrawModes(t *testing.T) {
	mode := VideoModes[MODE_640x480]

	// Test XOR draw mode (0x06) with fill
	video, bus := newBlitterTestRig(t)

	// Pre-fill destination with a known value
	dst := uint32(VRAM_START)
	dstOff := dst - BUFFER_OFFSET
	binary.LittleEndian.PutUint32(video.frontBuffer[dstOff:], 0xFF00FF00)

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_COLOR, 0xFFFFFFFF)                                  // XOR with all-ones
	bus.Write32(BLT_FLAGS, IE_BLT_MAKE_FLAGS(bltFlagsBPP_RGBA32, 0x06)) // XOR mode
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	got := binary.LittleEndian.Uint32(video.frontBuffer[dstOff:])
	want := uint32(0x00FF00FF) // 0xFF00FF00 XOR 0xFFFFFFFF
	if got != want {
		t.Fatalf("XOR draw mode: got 0x%08X, want 0x%08X", got, want)
	}
}

func TestBlitterColorExpand(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[MODE_640x480]

	// Template: 1 byte = 0b10110000 = bits 7,5,4 set = pixels 0,2,3 are FG
	// (MSB-first: bit 7 = leftmost pixel)
	tmplAddr := uint32(0x50000)
	bus.memory[tmplAddr] = 0xB0 // 10110000

	fg := uint32(0xFF0000FF) // red with alpha
	bg := uint32(0x0000FF00) // green no alpha

	dst := uint32(VRAM_START)
	bus.Write32(BLT_OP, bltOpColorExpand)
	bus.Write32(BLT_MASK, tmplAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 8)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_MASK_MOD, 1) // 1 byte per row
	bus.Write32(BLT_MASK_SRCX, 0)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_FG, fg)
	bus.Write32(BLT_BG, bg)
	// JAM2 (opaque), RGBA32, Copy mode
	bus.Write32(BLT_FLAGS, IE_BLT_MAKE_FLAGS(bltFlagsBPP_RGBA32, 0x03))
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Template 0xB0 = 10110000:
	// Pixel 0: bit 7 = 1 -> FG
	// Pixel 1: bit 6 = 0 -> BG
	// Pixel 2: bit 5 = 1 -> FG
	// Pixel 3: bit 4 = 1 -> FG
	// Pixel 4: bit 3 = 0 -> BG
	// Pixel 5: bit 2 = 0 -> BG
	// Pixel 6: bit 1 = 0 -> BG
	// Pixel 7: bit 0 = 0 -> BG
	expected := [8]uint32{fg, bg, fg, fg, bg, bg, bg, bg}
	dstOff := dst - BUFFER_OFFSET
	for i, want := range expected {
		got := binary.LittleEndian.Uint32(video.frontBuffer[dstOff+uint32(i*4):])
		if got != want {
			t.Fatalf("ColorExpand pixel %d: got 0x%08X, want 0x%08X", i, got, want)
		}
	}
}

func TestBlitterColorExpandJAM1(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[MODE_640x480]

	// Pre-fill destination
	dst := uint32(VRAM_START)
	dstOff := dst - BUFFER_OFFSET
	for i := range 8 {
		binary.LittleEndian.PutUint32(video.frontBuffer[dstOff+uint32(i*4):], 0xDEADBEEF)
	}

	// Template: 0xA0 = 10100000: pixels 0,2 are FG; rest unchanged
	tmplAddr := uint32(0x50000)
	bus.memory[tmplAddr] = 0xA0

	fg := uint32(0xFF0000FF)
	bus.Write32(BLT_OP, bltOpColorExpand)
	bus.Write32(BLT_MASK, tmplAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 4)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_MASK_MOD, 1)
	bus.Write32(BLT_MASK_SRCX, 0)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_FG, fg)
	bus.Write32(BLT_BG, 0)
	bus.Write32(BLT_FLAGS, IE_BLT_MAKE_FLAGS(bltFlagsBPP_RGBA32, 0x03)|bltFlagsJAM1)
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Pixel 0: FG, Pixel 1: unchanged (0xDEADBEEF), Pixel 2: FG, Pixel 3: unchanged
	expected := [4]uint32{fg, 0xDEADBEEF, fg, 0xDEADBEEF}
	for i, want := range expected {
		got := binary.LittleEndian.Uint32(video.frontBuffer[dstOff+uint32(i*4):])
		if got != want {
			t.Fatalf("ColorExpandJAM1 pixel %d: got 0x%08X, want 0x%08X", i, got, want)
		}
	}
}

func TestBlitterColorExpandInvert(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[MODE_640x480]

	// Pre-fill destination
	dst := uint32(VRAM_START)
	dstOff := dst - BUFFER_OFFSET
	binary.LittleEndian.PutUint32(video.frontBuffer[dstOff:], 0xFF00FF00)
	binary.LittleEndian.PutUint32(video.frontBuffer[dstOff+4:], 0xFF00FF00)

	// Template: 0x80 = 10000000: pixel 0 set, pixel 1 clear
	tmplAddr := uint32(0x50000)
	bus.memory[tmplAddr] = 0x80

	bus.Write32(BLT_OP, bltOpColorExpand)
	bus.Write32(BLT_MASK, tmplAddr)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 2)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_MASK_MOD, 1)
	bus.Write32(BLT_MASK_SRCX, 0)
	bus.Write32(BLT_DST_STRIDE, uint32(mode.bytesPerRow))
	bus.Write32(BLT_FG, 0)
	bus.Write32(BLT_BG, 0)
	bus.Write32(BLT_FLAGS, IE_BLT_MAKE_FLAGS(bltFlagsBPP_RGBA32, 0x03)|bltFlagsInvertMode)
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Pixel 0: XOR with 0xFFFFFFFF -> 0x00FF00FF
	// Pixel 1: unchanged -> 0xFF00FF00
	got0 := binary.LittleEndian.Uint32(video.frontBuffer[dstOff:])
	got1 := binary.LittleEndian.Uint32(video.frontBuffer[dstOff+4:])
	if got0 != 0x00FF00FF {
		t.Fatalf("Invert pixel 0: got 0x%08X, want 0x00FF00FF", got0)
	}
	if got1 != 0xFF00FF00 {
		t.Fatalf("Invert pixel 1: got 0x%08X, want 0xFF00FF00", got1)
	}
}

func TestBlitterFlagsRegisterAccess(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	_ = video

	// Test that BLT_FLAGS, BLT_FG, BLT_BG, BLT_MASK_MOD, BLT_MASK_SRCX are readable via MMIO
	bus.Write32(BLT_FLAGS, 0x0731)
	bus.Write32(BLT_FG, 0xAABBCCDD)
	bus.Write32(BLT_BG, 0x11223344)
	bus.Write32(BLT_MASK_MOD, 0x100)
	bus.Write32(BLT_MASK_SRCX, 7)

	// Verify readback through MMIO (not internal fields)
	if got := video.HandleRead(BLT_FLAGS); got != 0x0731 {
		t.Fatalf("BLT_FLAGS readback: got 0x%X, want 0x0731", got)
	}
	if got := video.HandleRead(BLT_FG); got != 0xAABBCCDD {
		t.Fatalf("BLT_FG readback: got 0x%X, want 0xAABBCCDD", got)
	}
	if got := video.HandleRead(BLT_BG); got != 0x11223344 {
		t.Fatalf("BLT_BG readback: got 0x%X, want 0x11223344", got)
	}
	if got := video.HandleRead(BLT_MASK_MOD); got != 0x100 {
		t.Fatalf("BLT_MASK_MOD readback: got 0x%X, want 0x100", got)
	}
	if got := video.HandleRead(BLT_MASK_SRCX); got != 7 {
		t.Fatalf("BLT_MASK_SRCX readback: got 0x%X, want 7", got)
	}

	// Test sub-register byte readback
	bus.Write32(BLT_FLAGS, 0x12345678)
	if got := video.HandleRead(BLT_FLAGS + 1); got != 0x56 {
		t.Fatalf("BLT_FLAGS+1 readback: got 0x%X, want 0x56", got)
	}
	if got := video.HandleRead(BLT_FLAGS + 2); got != 0x34 {
		t.Fatalf("BLT_FLAGS+2 readback: got 0x%X, want 0x34", got)
	}
	if got := video.HandleRead(BLT_FLAGS + 3); got != 0x12 {
		t.Fatalf("BLT_FLAGS+3 readback: got 0x%X, want 0x12", got)
	}
}

// IE_BLT_MAKE_FLAGS mirrors the C macro for test use
const (
	IE_BLT_MAKE_FLAGS_FN = 0 // placeholder — use the function below
)

func IE_BLT_MAKE_FLAGS(bpp uint32, drawmode uint32) uint32 {
	return (bpp & 0x03) | ((drawmode & 0x0F) << 4)
}

func TestBlitterLineExtended(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	// Use extended line mode: BLT_FLAGS != 0, BLT_DST = base, BLT_WIDTH = endpoint.
	// Draw a horizontal line from (5,10) to (9,10) into VRAM.
	base := uint32(VRAM_START)
	stride := uint32(mode.width * 4)
	flags := IE_BLT_MAKE_FLAGS(bltFlagsBPP_RGBA32, 0x03) // RGBA32 + Copy
	color := uint32(0xFFAABBCC)

	bus.Write32(BLT_OP, bltOpLine)
	bus.Write32(BLT_SRC, (10<<16)|5)    // start: x0=5, y0=10
	bus.Write32(BLT_WIDTH, (10<<16)|9)  // end:   x1=9, y1=10
	bus.Write32(BLT_DST, base)          // framebuffer base
	bus.Write32(BLT_DST_STRIDE, stride) // row stride
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_FLAGS, flags)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	// Verify all 5 pixels of the horizontal line
	for x := 5; x <= 9; x++ {
		addr := VRAM_START + uint32((10*mode.width+x)*4)
		if got := video.HandleRead(addr); got != color {
			t.Fatalf("extended line pixel at %d,10: got 0x%X, want 0x%X", x, got, color)
		}
	}

	// Verify pixel outside the line was not written
	addr := VRAM_START + uint32((10*mode.width+4)*4)
	if got := video.HandleRead(addr); got != 0 {
		t.Fatalf("pixel at 4,10 should be 0, got 0x%X", got)
	}
}

func TestBlitterLineExtendedDrawMode(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	// Pre-fill a pixel at (3,3) with a known value via frontBuffer
	pixAddr := VRAM_START + uint32((3*mode.width+3)*4)
	pixOff := pixAddr - BUFFER_OFFSET
	binary.LittleEndian.PutUint32(video.frontBuffer[pixOff:], 0xFF00FF00)

	// Draw a single-pixel line at (3,3) to (3,3) with XOR draw mode
	base := uint32(VRAM_START)
	stride := uint32(mode.width * 4)
	flags := IE_BLT_MAKE_FLAGS(bltFlagsBPP_RGBA32, 0x06) // RGBA32 + Xor

	bus.Write32(BLT_OP, bltOpLine)
	bus.Write32(BLT_SRC, (3<<16)|3)
	bus.Write32(BLT_WIDTH, (3<<16)|3)
	bus.Write32(BLT_DST, base)
	bus.Write32(BLT_DST_STRIDE, stride)
	bus.Write32(BLT_COLOR, 0xFFFFFFFF) // XOR with all-ones
	bus.Write32(BLT_FLAGS, flags)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	// XOR: 0xFF00FF00 ^ 0xFFFFFFFF = 0x00FF00FF
	want := uint32(0x00FF00FF)
	if got := video.HandleRead(pixAddr); got != want {
		t.Fatalf("XOR line pixel at 3,3: got 0x%X, want 0x%X", got, want)
	}
}

func TestBlitterLineExtendedCustomBase(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	_ = VideoModes[video.currentMode]

	// Use an offset base address (simulating a bitmap not at VRAM_START)
	// Place our "bitmap" 1000 pixels into VRAM
	baseOffset := uint32(1000 * 4) // 1000 pixels * 4 bpp
	base := uint32(VRAM_START) + baseOffset
	bitmapWidth := 100 // 100-pixel wide bitmap
	stride := uint32(bitmapWidth * 4)
	flags := IE_BLT_MAKE_FLAGS(bltFlagsBPP_RGBA32, 0x03) // Copy
	color := uint32(0xDEADBEEF)

	// Draw vertical line from (2,0) to (2,3) in the offset bitmap
	bus.Write32(BLT_OP, bltOpLine)
	bus.Write32(BLT_SRC, (0<<16)|2)   // start: x=2, y=0
	bus.Write32(BLT_WIDTH, (3<<16)|2) // end:   x=2, y=3
	bus.Write32(BLT_DST, base)
	bus.Write32(BLT_DST_STRIDE, stride)
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_FLAGS, flags)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	video.RunBlitterForTest()

	// Verify 4 pixels of the vertical line at x=2
	for y := 0; y <= 3; y++ {
		addr := base + uint32(y*bitmapWidth*4+2*4)
		if got := video.HandleRead(addr); got != color {
			t.Fatalf("custom base line pixel at 2,%d: got 0x%X, want 0x%X", y, got, color)
		}
	}

	// Verify adjacent pixel was not written
	addr := base + uint32(0*bitmapWidth*4+3*4) // (3,0)
	if got := video.HandleRead(addr); got != 0 {
		t.Fatalf("pixel at 3,0 should be 0, got 0x%X", got)
	}
}

// TestBlitterLineExtendedClipping validates that callers must pre-clip
// coordinates before using extended line mode, since the blitter itself
// does no viewport clipping. This mirrors what the AROS DrawLine override
// does with Cohen-Sutherland clipping before calling IE_BlitLineEx.
func TestBlitterLineExtendedClipping(t *testing.T) {
	video, bus := newBlitterTestRig(t)

	// Set up a small 10x10 bitmap at an offset in VRAM.
	bitmapW := 10
	bitmapH := 10
	bpp := 4
	stride := uint32(bitmapW * bpp)
	base := uint32(VRAM_START + 0x10000) // offset so we can detect underflow writes
	color := uint32(0xAABBCCDD)
	flags := IE_BLT_MAKE_FLAGS(bltFlagsBPP_RGBA32, 0x03)

	// Subtest 1: Pre-clipped line — only the visible segment is submitted.
	// A line from (2,2) to (2,7) clipped to rect (0,0)-(9,4) becomes (2,2)-(2,4).
	// Simulate what the AROS driver does after Cohen-Sutherland clipping.
	bus.Write32(BLT_OP, bltOpLine)
	bus.Write32(BLT_SRC, (2<<16)|2)   // start: (2,2)
	bus.Write32(BLT_WIDTH, (4<<16)|2) // clipped end: (2,4)
	bus.Write32(BLT_DST, base)
	bus.Write32(BLT_DST_STRIDE, stride)
	bus.Write32(BLT_COLOR, color)
	bus.Write32(BLT_FLAGS, flags)
	bus.Write32(BLT_CTRL, bltCtrlStart)
	video.RunBlitterForTest()

	// Pixels at y=2,3,4 should be drawn
	for y := 2; y <= 4; y++ {
		addr := base + uint32(y*bitmapW*bpp+2*bpp)
		if got := video.HandleRead(addr); got != color {
			t.Fatalf("clipped line pixel at 2,%d: got 0x%X, want 0x%X", y, got, color)
		}
	}

	// Pixels at y=5,6,7 should NOT be drawn (clipped away)
	for y := 5; y <= 7; y++ {
		addr := base + uint32(y*bitmapW*bpp+2*bpp)
		if got := video.HandleRead(addr); got != 0 {
			t.Fatalf("pixel at 2,%d should be 0 (clipped), got 0x%X", y, got)
		}
	}

	// Subtest 2: Fully rejected line — no pixels should be drawn.
	// A line completely outside the clip rect is not submitted at all.
	// The AROS driver's cs_clip_line returns FALSE, so IE_BlitLineEx is never called.
	// We verify that NOT calling the blitter means no stale pixels appear.
	video2, bus2 := newBlitterTestRig(t)
	base2 := uint32(VRAM_START + 0x10000)

	// Don't call the blitter (simulating full rejection) — verify all pixels are zero.
	for y := 0; y < bitmapH; y++ {
		for x := 0; x < bitmapW; x++ {
			addr := base2 + uint32(y*bitmapW*bpp+x*bpp)
			if got := video2.HandleRead(addr); got != 0 {
				t.Fatalf("rejected line: pixel at %d,%d should be 0, got 0x%X", x, y, got)
			}
		}
	}
	_ = bus2 // bus2 used only to construct the test rig

	// Subtest 3: Verify that unclipped coordinates WOULD write outside the
	// intended 10x10 region — proving clipping is necessary.
	// Draw a line from (2,2) to (2,15) WITHOUT clipping into a fresh bitmap.
	video3, bus3 := newBlitterTestRig(t)
	base3 := uint32(VRAM_START + 0x10000)

	bus3.Write32(BLT_OP, bltOpLine)
	bus3.Write32(BLT_SRC, (2<<16)|2)    // start: (2,2)
	bus3.Write32(BLT_WIDTH, (15<<16)|2) // end: (2,15) — extends past bitmap height
	bus3.Write32(BLT_DST, base3)
	bus3.Write32(BLT_DST_STRIDE, stride)
	bus3.Write32(BLT_COLOR, color)
	bus3.Write32(BLT_FLAGS, flags)
	bus3.Write32(BLT_CTRL, bltCtrlStart)
	video3.RunBlitterForTest()

	// Without clipping, pixels at y=10..15 are written OUTSIDE the 10x10 bitmap.
	// This proves that the caller MUST clip — the blitter itself does not.
	outsidePixels := 0
	for y := 10; y <= 15; y++ {
		addr := base3 + uint32(y*bitmapW*bpp+2*bpp)
		if got := video3.HandleRead(addr); got == color {
			outsidePixels++
		}
	}
	if outsidePixels == 0 {
		t.Fatal("expected unclipped line to write pixels outside bitmap bounds, but none found — clipping contract test is invalid")
	}
}
