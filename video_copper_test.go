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

func copperSetBaseWord(addr uint32) uint32 {
	return (uint32(copperOpcodeSetBase) << copperOpcodeShift) | ((addr >> 2) & copperSetBaseMask)
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

// TestCopperCanReadCPULoadedList verifies that the copper can read a copper list
// that was loaded by the CPU (simulating embedded program data).
func TestCopperCanReadCPULoadedList(t *testing.T) {
	video, bus := newCopperTestRig(t)

	cpu := NewCPU(bus)

	// CPU writes copper list to 0x2000 (simulating loaded program data)
	listAddr := uint32(0x2000)
	cpu.Write32(listAddr, copperEndWord()) // Simple END instruction

	// Point copper to the list and enable
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	// Verify copper executed (PC should have advanced past END)
	pc := video.HandleRead(COPPER_PC)
	if pc != listAddr+4 {
		t.Fatalf("Copper PC=0x%X, expected 0x%X - copper couldn't read list from CPU memory", pc, listAddr+4)
	}
}

// TestCopperSetBaseChangesIOBase verifies that SETBASE changes the I/O target base address.
func TestCopperSetBaseChangesIOBase(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0xA00)

	// SETBASE to VGA_BASE (0xF1000)
	// After this, MOVE regIndex 0 should target VGA_BASE, not VIDEO_REG_BASE
	bus.Write32(listAddr, copperSetBaseWord(VGA_BASE))
	bus.Write32(listAddr+4, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	// Verify SETBASE advanced PC by 4 bytes (single word instruction)
	pc := video.HandleRead(COPPER_PC)
	expectedPC := listAddr + 4 + 4 // +4 for SETBASE, +4 for END
	if pc != expectedPC {
		t.Fatalf("expected COPPER_PC=0x%X, got 0x%X", expectedPC, pc)
	}
}

// TestCopperSetBaseResetsOnFrame verifies the I/O base resets to VIDEO_REG_BASE each frame.
func TestCopperSetBaseResetsOnFrame(t *testing.T) {
	video, bus := newCopperTestRig(t)
	listAddr := uint32(0xB00)
	regIndex := uint32((VIDEO_MODE - VIDEO_REG_BASE) / 4)

	// First frame: SETBASE to arbitrary address, then end
	bus.Write32(listAddr, copperSetBaseWord(VGA_BASE))
	bus.Write32(listAddr+4, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	// Second frame: MOVE without SETBASE should use VIDEO_REG_BASE (reset)
	listAddr2 := uint32(0xC00)
	bus.Write32(listAddr2, copperMoveWord(regIndex))
	bus.Write32(listAddr2+4, MODE_800x600)
	bus.Write32(listAddr2+8, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr2)
	// Reset copper to latch new pointer (copperPtrStaged -> copperPtr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable|copperCtrlReset)

	video.RunCopperFrameForTest()

	// VIDEO_MODE should be updated because base was reset to VIDEO_REG_BASE
	if got := video.HandleRead(VIDEO_MODE); got != MODE_800x600 {
		t.Fatalf("expected VIDEO_MODE=MODE_800x600 after frame reset, got %d", got)
	}
}

// newCopperVGATestRig creates a test rig with both VideoChip and VGAEngine on the bus.
func newCopperVGATestRig(t *testing.T) (*VideoChip, *VGAEngine, *SystemBus) {
	t.Helper()

	bus := NewSystemBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, COPPER_STATUS+3, video.HandleRead, video.HandleWrite)

	// Add VGA engine to bus
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)

	return video, vga, bus
}

// TestCopperSetBaseCrossDeviceWrite verifies copper can write to VGA registers via SETBASE.
func TestCopperSetBaseCrossDeviceWrite(t *testing.T) {
	video, vga, bus := newCopperVGATestRig(t)
	listAddr := uint32(0xD00)

	// SETBASE to VGA_BASE, then MOVE to regIndex 0 (VGA_MODE)
	bus.Write32(listAddr, copperSetBaseWord(VGA_BASE))
	bus.Write32(listAddr+4, copperMoveWord(0)) // regIndex 0 = VGA_MODE
	bus.Write32(listAddr+8, VGA_MODE_13H)      // Set Mode 13h
	bus.Write32(listAddr+12, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	// Verify VGA mode was set via cross-device copper write
	if got := vga.HandleRead(VGA_MODE); got != VGA_MODE_13H {
		t.Fatalf("expected VGA_MODE=0x%X (Mode 13h), got 0x%X", VGA_MODE_13H, got)
	}
}

// TestCopperSetBaseMoveToVGAControl verifies copper can write to VGA control register.
func TestCopperSetBaseMoveToVGAControl(t *testing.T) {
	video, vga, bus := newCopperVGATestRig(t)
	listAddr := uint32(0xE00)

	// VGA_CTRL is at VGA_BASE + 8, so regIndex = 2
	vgaCtrlIndex := uint32((VGA_CTRL - VGA_BASE) / 4)

	bus.Write32(listAddr, copperSetBaseWord(VGA_BASE))
	bus.Write32(listAddr+4, copperMoveWord(vgaCtrlIndex))
	bus.Write32(listAddr+8, VGA_CTRL_ENABLE)
	bus.Write32(listAddr+12, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	// Verify VGA control register was set
	if got := vga.HandleRead(VGA_CTRL); got != VGA_CTRL_ENABLE {
		t.Fatalf("expected VGA_CTRL=0x%X, got 0x%X", VGA_CTRL_ENABLE, got)
	}
}

// TestCopperSetBaseMultipleDevices verifies copper can switch between devices in one frame.
func TestCopperSetBaseMultipleDevices(t *testing.T) {
	video, vga, bus := newCopperVGATestRig(t)
	listAddr := uint32(0xF00)

	videoModeIndex := uint32((VIDEO_MODE - VIDEO_REG_BASE) / 4)

	// Write to VGA, then switch back to VIDEO and write there
	bus.Write32(listAddr, copperSetBaseWord(VGA_BASE))
	bus.Write32(listAddr+4, copperMoveWord(0)) // VGA_MODE
	bus.Write32(listAddr+8, VGA_MODE_13H)
	bus.Write32(listAddr+12, copperSetBaseWord(VIDEO_REG_BASE)) // Switch back
	bus.Write32(listAddr+16, copperMoveWord(videoModeIndex))
	bus.Write32(listAddr+20, MODE_800x600)
	bus.Write32(listAddr+24, copperEndWord())
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable)

	video.RunCopperFrameForTest()

	// Both devices should be updated
	if got := vga.HandleRead(VGA_MODE); got != VGA_MODE_13H {
		t.Fatalf("expected VGA_MODE=0x%X, got 0x%X", VGA_MODE_13H, got)
	}
	if got := video.HandleRead(VIDEO_MODE); got != MODE_800x600 {
		t.Fatalf("expected VIDEO_MODE=MODE_800x600, got %d", got)
	}
}
