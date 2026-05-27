//go:build headless

package main

import "testing"

func newM68KFlatTestVideoChip(t *testing.T) *VideoChip {
	t.Helper()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	return video
}

func TestM68KFlatProgramVideoConfigMakesLegacyVRAMApertureRAM(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	var writes int
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
		func(uint32) uint32 { return 0xEE },
		func(uint32, uint32) { writes++ })
	bus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1,
		func(uint32, uint8) { writes++ })

	applyM68KFlatProgramVideoConfig(bus, video)

	const addr = VRAM_START + 0x234
	bus.Write8(addr, 0x5A)

	if writes != 0 {
		t.Fatalf("VRAM aperture write dispatched to MMIO %d time(s), want plain RAM", writes)
	}
	if got := bus.memory[addr]; got != 0x5A {
		t.Fatalf("bus memory at 0x%X = 0x%02X, want 0x5A", addr, got)
	}
	if got := bus.Read8(addr); got != 0x5A {
		t.Fatalf("bus Read8 at 0x%X = 0x%02X, want RAM value 0x5A", addr, got)
	}
}

func TestM68KFlatProgramVideoConfigSamplesFramebufferFromBusMemory(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	video.Reset()
	applyM68KFlatProgramVideoConfig(bus, video)

	video.HandleWrite(VIDEO_MODE, MODE_320x200)
	video.HandleWrite(VIDEO_COLOR_MODE, 0)
	video.HandleWrite(VIDEO_FB_BASE, VRAM_START+0x40)
	video.HandleWrite(VIDEO_CTRL, 1)

	bus.memory[VRAM_START+0x40] = 0x11
	bus.memory[VRAM_START+0x41] = 0x22
	bus.memory[VRAM_START+0x42] = 0x33
	bus.memory[VRAM_START+0x43] = 0x44

	frame := video.GetFrame()
	if len(frame) < 4 {
		t.Fatalf("frame length=%d, want at least 4 bytes", len(frame))
	}
	if got := frame[:4]; got[0] != 0x11 || got[1] != 0x22 || got[2] != 0x33 || got[3] != 0x44 {
		t.Fatalf("frame prefix = %02X %02X %02X %02X, want 11 22 33 44", got[0], got[1], got[2], got[3])
	}
}

func TestRestoreLegacyVideoConfigRemapsVRAMAfterM68KFlatMode(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	applyM68KFlatProgramVideoConfig(bus, video)
	if video.directVRAM == nil {
		t.Fatalf("flat M68K config did not install direct VRAM")
	}

	restoreLegacyVideoConfig(bus, video)
	if video.directVRAM != nil {
		t.Fatalf("legacy restore left direct VRAM enabled")
	}
	bus.Write32(VRAM_START, 0x55667788)
	if got := video.HandleRead(VRAM_START); got != 0x55667788 {
		t.Fatalf("legacy VRAM read = 0x%08X, want 0x55667788 after remap", got)
	}
}

func TestProgramExecutorM68KReloadAppliesFlatVideoConfig(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	var writes int
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
		func(uint32) uint32 { return 0xAA },
		func(uint32, uint32) { writes++ })
	bus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1,
		func(uint32, uint8) { writes++ })

	exec := NewProgramExecutor(bus, nil, video, nil, nil, t.TempDir())
	if err := exec.prepareAndLaunch([]byte{0x60, 0xFE}, EXEC_TYPE_M68K); err != nil {
		t.Fatalf("prepareAndLaunch M68K: %v", err)
	}
	defer exec.stopRunningCPUs()

	const addr = VRAM_START + 0x345
	bus.Write8(addr, 0xC3)

	if writes != 0 {
		t.Fatalf("M68K reload left VRAM aperture mapped to MMIO; write count=%d", writes)
	}
	if got := bus.memory[addr]; got != 0xC3 {
		t.Fatalf("bus memory at 0x%X = 0x%02X, want 0xC3", addr, got)
	}
}

func TestProgramExecutorReloadTracksCPUForStopBeforeRemap(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	exec := NewProgramExecutor(bus, nil, video, nil, nil, t.TempDir())
	if err := exec.prepareAndLaunch([]byte{0x60, 0xFE}, EXEC_TYPE_M68K); err != nil {
		t.Fatalf("prepareAndLaunch M68K: %v", err)
	}
	defer exec.stopRunningCPUs()

	snap := runtimeStatus.snapshot()
	if snap.m68k == nil {
		t.Fatalf("M68K reload did not publish a runtime runner")
	}
	snap.m68k.execMu.Lock()
	tracked := snap.m68k.execActive
	snap.m68k.execMu.Unlock()
	if !tracked {
		t.Fatalf("M68K reload started an untracked CPU goroutine; stop cannot wait before remapping")
	}

	if err := exec.prepareAndLaunch([]byte{0xF4}, EXEC_TYPE_X86); err != nil {
		t.Fatalf("prepareAndLaunch X86: %v", err)
	}

	snap.m68k.execMu.Lock()
	stopped := !snap.m68k.execActive
	snap.m68k.execMu.Unlock()
	if !stopped {
		t.Fatalf("M68K reload remained active after launching the next EXEC program")
	}
}

func TestProgramExecutorLegacyReloadRestoresVRAMAfterM68KFlatMode(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	exec := NewProgramExecutor(bus, nil, video, nil, nil, t.TempDir())
	if err := exec.prepareAndLaunch([]byte{0x60, 0xFE}, EXEC_TYPE_M68K); err != nil {
		t.Fatalf("prepareAndLaunch M68K: %v", err)
	}
	defer exec.stopRunningCPUs()

	if err := exec.prepareAndLaunch([]byte{0xF4}, EXEC_TYPE_X86); err != nil {
		t.Fatalf("prepareAndLaunch X86: %v", err)
	}

	bus.Write32(VRAM_START, 0xAABBCCDD)
	if got := video.HandleRead(VRAM_START); got != 0xAABBCCDD {
		t.Fatalf("legacy EXEC VRAM read = 0x%08X, want 0xAABBCCDD after remap", got)
	}
}

func TestProgramExecutorLegacyReloadDoesNotPanicWithSealedLegacyVRAM(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	restoreLegacyVideoConfig(bus, video)
	bus.SealMappings()

	exec := NewProgramExecutor(bus, nil, video, nil, nil, t.TempDir())
	if err := exec.prepareAndLaunch([]byte{0xF4}, EXEC_TYPE_X86); err != nil {
		t.Fatalf("prepareAndLaunch X86: %v", err)
	}
	defer exec.stopRunningCPUs()

	bus.Write32(VRAM_START, 0x12345678)
	if got := video.HandleRead(VRAM_START); got != 0x12345678 {
		t.Fatalf("legacy EXEC VRAM read = 0x%08X, want 0x12345678 after sealed reload", got)
	}
}

func TestProgramExecutorLegacyReloadRestoresVRAMAfterSealedFlatM68K(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	restoreLegacyVideoConfig(bus, video)
	bus.SealMappings()

	exec := NewProgramExecutor(bus, nil, video, nil, nil, t.TempDir())
	if err := exec.prepareAndLaunch([]byte{0x60, 0xFE}, EXEC_TYPE_M68K); err != nil {
		t.Fatalf("prepareAndLaunch M68K: %v", err)
	}
	defer exec.stopRunningCPUs()

	if err := exec.prepareAndLaunch([]byte{0xF4}, EXEC_TYPE_X86); err != nil {
		t.Fatalf("prepareAndLaunch X86: %v", err)
	}

	bus.Write32(VRAM_START, 0x87654321)
	if got := video.HandleRead(VRAM_START); got != 0x87654321 {
		t.Fatalf("legacy EXEC VRAM read = 0x%08X, want 0x87654321 after sealed M68K reload", got)
	}
}
