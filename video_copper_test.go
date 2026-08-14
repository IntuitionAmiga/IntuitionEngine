package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newCopperTestRig(t *testing.T) (*VideoChip, *MachineBus) {
	t.Helper()

	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	t.Cleanup(func() { _ = video.Stop() })
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

func copperFrameStartCountForTest(video *VideoChip) uint64 {
	video.mu.Lock()
	defer video.mu.Unlock()
	return video.copperFrameStarts
}

func TestVideoChip_NeedsScanlineCompositingTracksCopper(t *testing.T) {
	video, bus := newCopperTestRig(t)

	if video.NeedsScanlineCompositing() {
		t.Fatal("default VideoChip should not need scanline compositing")
	}

	bus.Write32(COPPER_CTRL, copperCtrlEnable)
	if !video.NeedsScanlineCompositing() {
		t.Fatal("VideoChip with enabled copper should need scanline compositing")
	}

	bus.Write32(COPPER_CTRL, 0)
	if video.NeedsScanlineCompositing() {
		t.Fatal("VideoChip with disabled copper should not need scanline compositing")
	}
}

func writeWord8(bus *MachineBus, addr uint32, value uint32) {
	bus.Write8(addr, uint8(value))
	bus.Write8(addr+1, uint8(value>>8))
	bus.Write8(addr+2, uint8(value>>16))
	bus.Write8(addr+3, uint8(value>>24))
}

func startCopperFrame(video *VideoChip) {
	video.mu.Lock()
	video.copperStartFrameLocked()
	video.mu.Unlock()
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

func TestVideoChipCompositorFrameClockSuppressesPrivateCopper(t *testing.T) {
	video, bus := newCopperTestRig(t)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	listAddr := uint32(0x380)
	words := []uint32{
		copperMoveWord((BLT_CTRL - VIDEO_REG_BASE) / 4), bltCtrlStart,
		copperEndWord(),
	}
	for i, word := range words {
		bus.Write32(listAddr+uint32(i*4), word)
	}
	bus.Write32(VIDEO_CTRL, 1)
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable|copperCtrlReset)

	video.runPrivateRefreshTick()
	if got := video.BlitStartCount(); got != 1 {
		t.Fatalf("standalone private tick started %d blits, want 1", got)
	}

	video.acquireCompositorFrameClock()
	video.runPrivateRefreshTick()
	if got := video.BlitStartCount(); got != 1 {
		t.Fatalf("owned private tick started %d blits, want 1", got)
	}

	video.StartFrame()
	video.ProcessScanlineRange(0, VideoModes[MODE_640x480].height)
	video.FinishFrame()
	if got := video.BlitStartCount(); got != 2 {
		t.Fatalf("compositor frame started %d blits, want 2", got)
	}

	video.runPrivateRefreshTick()
	if got := video.BlitStartCount(); got != 2 {
		t.Fatalf("private tick between compositor frames started %d blits, want 2", got)
	}

	video.releaseCompositorFrameClock()
	video.runPrivateRefreshTick()
	if got := video.BlitStartCount(); got != 3 {
		t.Fatalf("released private tick started %d blits, want 3", got)
	}
}

type blockingCopperBus struct {
	memory  []byte
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingCopperBus) Read8(uint32) uint8     { return 0 }
func (b *blockingCopperBus) Write8(uint32, uint8)   {}
func (b *blockingCopperBus) Read16(uint32) uint16   { return 0 }
func (b *blockingCopperBus) Write16(uint32, uint16) {}
func (b *blockingCopperBus) Read32(uint32) uint32   { return 0 }
func (b *blockingCopperBus) Reset()                 {}
func (b *blockingCopperBus) GetMemory() []byte      { return b.memory }
func (b *blockingCopperBus) Write32(uint32, uint32) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
}

func TestVideoChipFrameClockAcquisitionWaitsForPrivateCopper(t *testing.T) {
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	t.Cleanup(func() { _ = video.Stop() })
	bus := &blockingCopperBus{
		memory:  make([]byte, 0x2000),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	binary.LittleEndian.PutUint32(bus.memory[0x1000:], copperSetBaseWord(VGA_BASE))
	binary.LittleEndian.PutUint32(bus.memory[0x1004:], copperMoveWord(0))
	binary.LittleEndian.PutUint32(bus.memory[0x1008:], 1)
	binary.LittleEndian.PutUint32(bus.memory[0x100C:], copperEndWord())
	video.AttachBus(bus)
	video.enabled.Store(true)
	video.mu.Lock()
	video.copperEnabled = true
	video.copperPtr = 0x1000
	video.mu.Unlock()

	privateDone := make(chan struct{})
	go func() {
		video.runPrivateRefreshTick()
		close(privateDone)
	}()
	<-bus.entered
	acquired := make(chan struct{})
	go func() {
		video.acquireCompositorFrameClock()
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("frame-clock acquisition returned while private Copper was running")
	case <-time.After(20 * time.Millisecond):
	}
	close(bus.release)
	<-privateDone
	<-acquired
	video.releaseCompositorFrameClock()
}

func TestRotatingCopperCubeUsesOneCompositorOwnedCopperFrame(t *testing.T) {
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	t.Cleanup(func() { _ = video.Stop() })
	video.AttachBus(bus)
	video.SetBigEndianMode(true)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.SetVideoStatusReader(video.HandleRead)

	vga := NewVGAEngine(bus)
	var dacWrites atomic.Int32
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, func(addr uint32, value uint32) {
		if addr == VGA_DAC_WINDEX || addr == VGA_DAC_DATA {
			dacWrites.Add(1)
		}
		vga.HandleWrite(addr, value)
	})
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	bus.MapIO(VGA_TEXT_WINDOW, VGA_TEXT_WINDOW+VGA_TEXT_SIZE-1, vga.HandleTextRead, vga.HandleTextWrite)

	bus.Write32(0, 0x00010000)
	bus.Write32(4, M68K_ENTRY_POINT)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = false
	cpu.PC = M68K_ENTRY_POINT
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x00010000
	programPath := filepath.Join("sdk", "examples", "prebuilt", "rotating_cube_copper_68k.ie68")
	program, err := os.ReadFile(programPath)
	if err != nil {
		t.Fatalf("read %s: %v", programPath, err)
	}
	cpu.LoadProgramBytes(program)
	const maxBootstrapSteps = 20_000_000
	steps := 0
	for ; steps < maxBootstrapSteps; steps++ {
		cpu.StepOne()
		if steps&0xFFF == 0 {
			video.mu.Lock()
			ptr, enabled := video.copperPtr, video.copperEnabled
			video.mu.Unlock()
			if enabled && ptr != 0 &&
				binary.BigEndian.Uint32(bus.memory[ptr+16:]) != 0 {
				break
			}
		}
	}
	video.mu.Lock()
	ptr, enabled := video.copperPtr, video.copperEnabled
	video.mu.Unlock()
	if !enabled || ptr == 0 {
		t.Fatalf("rotating cube did not enable Copper in %d instructions: ptr=%#x enabled=%v pc=%#x", steps, ptr, enabled, cpu.PC)
	}
	if binary.BigEndian.Uint32(bus.memory[ptr+16:]) == 0 {
		t.Fatalf("rotating cube did not update Copper colours in %d instructions: ptr=%#x pc=%#x", steps, ptr, cpu.PC)
	}

	comp := NewVideoCompositor(nil)
	comp.scheduler = NewManualVideoScheduler()
	comp.RegisterSource(video)
	comp.RegisterSource(vga)
	if err := comp.Start(); err != nil {
		t.Fatalf("compositor Start: %v", err)
	}
	t.Cleanup(func() { _ = comp.Close() })
	dacWrites.Store(0)
	comp.scheduler.TickManual()
	if got := dacWrites.Load(); got != 400 {
		t.Fatalf("rotating cube compositor frame wrote VGA DAC %d times, want 400", got)
	}

	colours := make(map[uint32]bool)
	for y := 0; y < VGA_MODE13H_HEIGHT; y += 2 {
		offset := y * VGA_MODE13H_WIDTH * 4
		colours[binary.LittleEndian.Uint32(vga.scanlineFrame[offset:])] = true
	}
	if len(colours) < 16 {
		t.Fatalf("rotating cube Copper raster produced %d sampled colours, want at least 16", len(colours))
	}

	video.runPrivateRefreshTick()
	if got := dacWrites.Load(); got != 400 {
		t.Fatalf("private VideoChip tick increased rotating cube VGA DAC writes to %d", got)
	}
}

func writeRawBlitterShadow(bus *MachineBus, addr, value uint32) {
	binary.LittleEndian.PutUint32(bus.memory[addr:addr+4], value)
}

func writeBlitterCommandPacket(bus *MachineBus, packet uint32, values map[uint32]uint32) {
	for addr, value := range values {
		binary.LittleEndian.PutUint32(bus.memory[packet+addr-BLT_OP:], value)
	}
}

func appendCopperShadowByte(words *[]uint32, addr uint32, value byte) {
	*words = append(*words,
		copperMoveWord((VIDEO_FB_BASE-VIDEO_REG_BASE)/4), addr,
		copperMoveWord((VIDEO_RASTER_COLOR-VIDEO_REG_BASE)/4), uint32(value),
		copperMoveWord((VIDEO_RASTER_CTRL-VIDEO_REG_BASE)/4), rasterCtrlStart,
	)
}

func appendCopperBlitterShadow(words *[]uint32, fields map[uint32]uint32) {
	for _, addr := range []uint32{
		BLT_OP, BLT_SRC, BLT_DST, BLT_WIDTH, BLT_HEIGHT, BLT_SRC_STRIDE, BLT_DST_STRIDE,
	} {
		value := fields[addr]
		for byteIndex := uint32(0); byteIndex < 4; byteIndex++ {
			appendCopperShadowByte(words, addr+byteIndex, byte(value>>(byteIndex*8)))
		}
	}
	value := fields[BLT_FLAGS]
	for byteIndex := uint32(0); byteIndex < 4; byteIndex++ {
		appendCopperShadowByte(words, BLT_FLAGS+byteIndex, byte(value>>(byteIndex*8)))
	}
}

func TestCopperStartsCommandLoadedThroughSharedMemory(t *testing.T) {
	video, bus := newCopperTestRig(t)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	listAddr := uint32(0x3000)
	packetAddr := uint32(0x4000)
	dstAddr := uint32(0x5000)
	packetSize := uint32(BLT_FLAGS + 4 - BLT_OP)

	writeBlitterCommandPacket(bus, packetAddr, map[uint32]uint32{
		BLT_OP:         bltOpFill,
		BLT_DST:        dstAddr,
		BLT_WIDTH:      1,
		BLT_HEIGHT:     1,
		BLT_COLOR:      0xAABBCCDD,
		BLT_FLAGS:      0,
		BLT_SRC_STRIDE: 0,
		BLT_DST_STRIDE: 0,
	})
	writeRawBlitterShadow(bus, BLT_OP, bltOpMemcopy)
	writeRawBlitterShadow(bus, BLT_SRC, packetAddr)
	writeRawBlitterShadow(bus, BLT_DST, BLT_OP)
	writeRawBlitterShadow(bus, BLT_WIDTH, packetSize)
	writeRawBlitterShadow(bus, BLT_HEIGHT, 1)

	words := []uint32{
		copperMoveWord((BLT_CTRL - VIDEO_REG_BASE) / 4), bltCtrlStart,
		copperMoveWord((BLT_CTRL - VIDEO_REG_BASE) / 4), bltCtrlStart,
		copperEndWord(),
	}
	for i, word := range words {
		bus.Write32(listAddr+uint32(i*4), word)
	}
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable|copperCtrlReset)

	video.RunCopperFrameForTest()

	if got := bus.Read32(dstAddr); got != 0xAABBCCDD {
		t.Fatalf("loaded command result = %#x, want 0xAABBCCDD", got)
	}
	if got := video.bltStartCount; got != 2 {
		t.Fatalf("blitter starts = %d, want 2", got)
	}
}

func TestCopperRasterSequencerRunsDifferentBlitterCommands(t *testing.T) {
	video, bus := newCopperTestRig(t)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	video.mu.Lock()
	video.copperManagedByCompositor = true
	video.mu.Unlock()

	listAddr := uint32(0x9000)
	fillDst := uint32(0x20000)
	copySrc := uint32(0x20100)
	copyDst := uint32(0x20200)
	bus.Write32(copySrc, 0x12345678)

	words := []uint32{
		copperMoveWord((VIDEO_COLOR_MODE - VIDEO_REG_BASE) / 4), 1,
		copperMoveWord((VIDEO_RASTER_Y - VIDEO_REG_BASE) / 4), 0,
		copperMoveWord((VIDEO_RASTER_HEIGHT - VIDEO_REG_BASE) / 4), 1,
	}
	appendCopperBlitterShadow(&words, map[uint32]uint32{
		BLT_OP: bltOpFill, BLT_DST: fillDst, BLT_WIDTH: 1, BLT_HEIGHT: 1,
		BLT_COLOR: 0xAABBCCDD, BLT_FLAGS: 0,
	})
	// BLT_COLOR follows the core shadow fields and is needed by FILL.
	for byteIndex := uint32(0); byteIndex < 4; byteIndex++ {
		appendCopperShadowByte(&words, BLT_COLOR+byteIndex, byte(uint32(0xAABBCCDD)>>(byteIndex*8)))
	}
	words = append(words, copperMoveWord((BLT_CTRL-VIDEO_REG_BASE)/4), bltCtrlStart)
	appendCopperBlitterShadow(&words, map[uint32]uint32{
		BLT_OP: bltOpCopy, BLT_SRC: copySrc, BLT_DST: copyDst, BLT_WIDTH: 1, BLT_HEIGHT: 1,
		BLT_FLAGS: 0,
	})
	words = append(words,
		copperMoveWord((BLT_CTRL-VIDEO_REG_BASE)/4), bltCtrlStart,
		copperMoveWord((VIDEO_COLOR_MODE-VIDEO_REG_BASE)/4), 0,
		copperEndWord(),
	)
	for i, word := range words {
		bus.Write32(listAddr+uint32(i*4), word)
	}
	bus.Write32(COPPER_PTR, listAddr)
	bus.Write32(COPPER_CTRL, copperCtrlEnable|copperCtrlReset)

	video.RunCopperFrameForTest()

	if got := bus.Read32(fillDst); got != 0xAABBCCDD {
		t.Fatalf("fill result = %#x, want 0xAABBCCDD; starts %d op %#x dst %#x width %#x height %#x colour %#x flags %#x", got, video.bltStartCount, bus.Read32(BLT_OP), bus.Read32(BLT_DST), bus.Read32(BLT_WIDTH), bus.Read32(BLT_HEIGHT), bus.Read32(BLT_COLOR), bus.Read32(BLT_FLAGS))
	}
	if got := bus.Read32(copyDst); got != 0x12345678 {
		t.Fatalf("copy result = %#x, want 0x12345678", got)
	}
	if got := video.bltStartCount; got != 2 {
		t.Fatalf("blitter starts = %d, want 2", got)
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

	if got := video.HandleRead(VIDEO_MODE); got != DEFAULT_VIDEO_MODE {
		t.Fatalf("expected VIDEO_MODE to remain default, got %d", got)
	}

	video.StepCopperRasterForTest(2, 0)
	if got := video.HandleRead(VIDEO_MODE); got != DEFAULT_VIDEO_MODE {
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
func newCopperVGATestRig(t *testing.T) (*VideoChip, *VGAEngine, *MachineBus) {
	t.Helper()

	bus := NewMachineBus()
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
