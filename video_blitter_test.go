package main

import "testing"

func newBlitterTestRig(t *testing.T) (*VideoChip, *SystemBus) {
	t.Helper()

	bus := NewSystemBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	return video, bus
}

func writeU32Bytes(bus *SystemBus, addr uint32, value uint32) {
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

	for i := 0; i < 8; i++ {
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

	for i := 0; i < 8; i++ {
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
