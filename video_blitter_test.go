package main

import "testing"

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
	if got := video.HandleRead(BLT_CTRL); got&bltCtrlBusy == 0 {
		t.Fatalf("expected BLT_CTRL busy set, got 0x%X", got)
	}

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
