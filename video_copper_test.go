package main

import "testing"

func newCopperTestRig(t *testing.T) (*VideoChip, *SystemBus) {
	t.Helper()

	bus := NewSystemBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, COPPER_STATUS+3, video.HandleRead, video.HandleWrite)
	return video, bus
}

func copperWaitWord(y, x uint16) uint32 {
	return (uint32(copperOpcodeWait) << copperOpcodeShift) | (uint32(y) << copperYShift) | uint32(x)
}

func copperMoveWord(regIndex uint32) uint32 {
	return (uint32(copperOpcodeMove) << copperOpcodeShift) | (regIndex << copperRegShift)
}

func copperEndWord() uint32 {
	return uint32(copperOpcodeEnd) << copperOpcodeShift
}

func writeWord8(bus *SystemBus, addr uint32, value uint32) {
	bus.Write8(addr, uint8(value))
	bus.Write8(addr+1, uint8(value>>8))
	bus.Write8(addr+2, uint8(value>>16))
	bus.Write8(addr+3, uint8(value>>24))
}

func startCopperFrame(video *VideoChip) {
	video.mutex.Lock()
	video.copperStartFrameLocked()
	video.mutex.Unlock()
}

func TestCopperEndStops(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x200)

	bus.Write32(listAddr, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	status := video.HandleRead(COPPER_STATUS)
	if status&copperStatusHalted == 0 {
		t.Fatalf("expected copper halted, status=0x%X", status)
	}
}

func TestCopperRegisters(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x1C0)

	bus.Write32(listAddr, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	if got := video.HandleRead(COPPER_PC); got != listAddr+4 {
		t.Fatalf("expected COPPER_PC=0x%X, got 0x%X", listAddr+4, got)
	}
	status := video.HandleRead(COPPER_STATUS)
	if status&copperStatusHalted == 0 {
		t.Fatalf("expected halted status, got 0x%X", status)
	}
}

func TestCopperMoveWritesVideoReg(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x300)
	regIndex := uint32((VIDEO_CTRL - VIDEO_REG_BASE) / 4)

	bus.Write32(listAddr, copperMoveWord(regIndex))
	bus.Write32(listAddr+4, 1)
	bus.Write32(listAddr+8, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	if got := video.HandleRead(VIDEO_CTRL); got != 1 {
		t.Fatalf("expected VIDEO_CTRL=1, got %d", got)
	}
}

func TestCopperWaitDefers(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x400)
	regIndex := uint32((VIDEO_MODE - VIDEO_REG_BASE) / 4)

	bus.Write32(listAddr, copperWaitWord(2, 10))
	bus.Write32(listAddr+4, copperMoveWord(regIndex))
	bus.Write32(listAddr+8, MODE_800x600)
	bus.Write32(listAddr+12, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	startCopperFrame(video)
	video.StepCopperRasterForTest(1, 0)

	if got := video.HandleRead(VIDEO_MODE); got != MODE_640x480 {
		t.Fatalf("expected VIDEO_MODE to remain default, got %d", got)
	}

	video.StepCopperRasterForTest(2, 0)
	if got := video.HandleRead(VIDEO_MODE); got != MODE_640x480 {
		t.Fatalf("expected VIDEO_MODE to remain default before X threshold, got %d", got)
	}

	video.StepCopperRasterForTest(2, 10)
	if got := video.HandleRead(VIDEO_MODE); got != MODE_800x600 {
		t.Fatalf("expected VIDEO_MODE to update at wait point, got %d", got)
	}
}

func TestCopperLittleEndianListWrites(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x500)
	regIndex := uint32((VIDEO_MODE - VIDEO_REG_BASE) / 4)

	writeWord8(bus, listAddr, copperMoveWord(regIndex))
	writeWord8(bus, listAddr+4, MODE_800x600)
	writeWord8(bus, listAddr+8, copperEndWord())
	writeWord8(bus, COPPER_PTR, listAddr)
	writeWord8(bus, COPPER_PTR+1, listAddr>>8)
	writeWord8(bus, COPPER_PTR+2, listAddr>>16)
	writeWord8(bus, COPPER_PTR+3, listAddr>>24)
	bus.Write8(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	if got := video.HandleRead(VIDEO_MODE); got != MODE_800x600 {
		t.Fatalf("expected VIDEO_MODE=MODE_800x600, got %d", got)
	}
}

func TestCopperIE32ListExecution(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x600)
	regIndex := uint32((VIDEO_MODE - VIDEO_REG_BASE) / 4)

	bus.Write32(listAddr, copperMoveWord(regIndex))
	bus.Write32(listAddr+4, MODE_800x600)
	bus.Write32(listAddr+8, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	if got := video.HandleRead(VIDEO_MODE); got != MODE_800x600 {
		t.Fatalf("expected VIDEO_MODE=MODE_800x600, got %d", got)
	}
}

func TestCopper6502ListExecution(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x700)
	regIndex := uint32((VIDEO_MODE - VIDEO_REG_BASE) / 4)

	writeWord8(bus, listAddr, copperMoveWord(regIndex))
	writeWord8(bus, listAddr+4, MODE_800x600)
	writeWord8(bus, listAddr+8, copperEndWord())
	writeWord8(bus, COPPER_PTR, listAddr)
	writeWord8(bus, COPPER_PTR+1, listAddr>>8)
	writeWord8(bus, COPPER_PTR+2, listAddr>>16)
	writeWord8(bus, COPPER_PTR+3, listAddr>>24)
	bus.Write8(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	if got := video.HandleRead(VIDEO_MODE); got != MODE_800x600 {
		t.Fatalf("expected VIDEO_MODE=MODE_800x600, got %d", got)
	}
}

func TestCopperZ80ListExecution(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x800)
	regIndex := uint32((VIDEO_MODE - VIDEO_REG_BASE) / 4)

	writeWord8(bus, listAddr, copperMoveWord(regIndex))
	writeWord8(bus, listAddr+4, MODE_800x600)
	writeWord8(bus, listAddr+8, copperEndWord())
	writeWord8(bus, COPPER_PTR, listAddr)
	writeWord8(bus, COPPER_PTR+1, listAddr>>8)
	writeWord8(bus, COPPER_PTR+2, listAddr>>16)
	writeWord8(bus, COPPER_PTR+3, listAddr>>24)
	bus.Write8(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	if got := video.HandleRead(VIDEO_MODE); got != MODE_800x600 {
		t.Fatalf("expected VIDEO_MODE=MODE_800x600, got %d", got)
	}
}

func TestCopperM68KListExecution(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0x900)
	regIndex := uint32((VIDEO_MODE - VIDEO_REG_BASE) / 4)

	bus.Write32(listAddr, copperMoveWord(regIndex))
	bus.Write32(listAddr+4, MODE_800x600)
	bus.Write32(listAddr+8, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	if got := video.HandleRead(VIDEO_MODE); got != MODE_800x600 {
		t.Fatalf("expected VIDEO_MODE=MODE_800x600, got %d", got)
	}
}
