//go:build headless

package main

import (
	"bytes"
	"testing"
)

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

func TestIE64FlatProgramVideoConfigStillSamplesFramebufferFromBusMemory(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	video.Reset()
	applyIE64FlatProgramVideoConfig(bus, video)

	video.HandleWrite(VIDEO_MODE, MODE_320x200)
	video.HandleWrite(VIDEO_COLOR_MODE, 0)
	video.HandleWrite(VIDEO_FB_BASE, VRAM_START+0x20)
	video.HandleWrite(VIDEO_CTRL, 1)

	bus.memory[VRAM_START+0x20] = 0xDE
	bus.memory[VRAM_START+0x21] = 0xAD
	bus.memory[VRAM_START+0x22] = 0xBE
	bus.memory[VRAM_START+0x23] = 0xEF

	frame := video.GetFrame()
	if len(frame) < 4 {
		t.Fatalf("frame length=%d, want at least 4 bytes", len(frame))
	}
	if got := frame[:4]; got[0] != 0xDE || got[1] != 0xAD || got[2] != 0xBE || got[3] != 0xEF {
		t.Fatalf("IE64 frame prefix = %02X %02X %02X %02X, want DE AD BE EF", got[0], got[1], got[2], got[3])
	}
}

func TestX86FlatProgramVideoConfigMakesLegacyVRAMApertureRAM(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	var writes int
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
		func(uint32) uint32 { return 0xEE },
		func(uint32, uint32) { writes++ })
	bus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1,
		func(uint32, uint8) { writes++ })

	applyX86FlatProgramVideoConfig(bus, video)

	const addr = VRAM_START + 0x456
	bus.Write8(addr, 0xA5)

	if writes != 0 {
		t.Fatalf("VRAM aperture write dispatched to MMIO %d time(s), want plain RAM", writes)
	}
	if got := bus.memory[addr]; got != 0xA5 {
		t.Fatalf("bus memory at 0x%X = 0x%02X, want 0xA5", addr, got)
	}
	if got := bus.Read8(addr); got != 0xA5 {
		t.Fatalf("bus Read8 at 0x%X = 0x%02X, want RAM value 0xA5", addr, got)
	}
}

func TestX86FlatProgramVideoConfigSamplesFramebufferFromBusMemory(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	video.Reset()
	applyX86FlatProgramVideoConfig(bus, video)

	video.HandleWrite(VIDEO_MODE, MODE_320x200)
	video.HandleWrite(VIDEO_COLOR_MODE, 0)
	video.HandleWrite(VIDEO_FB_BASE, VRAM_START+0x60)
	video.HandleWrite(VIDEO_CTRL, 1)

	bus.memory[VRAM_START+0x60] = 0x01
	bus.memory[VRAM_START+0x61] = 0x23
	bus.memory[VRAM_START+0x62] = 0x45
	bus.memory[VRAM_START+0x63] = 0x67

	frame := video.GetFrame()
	if len(frame) < 4 {
		t.Fatalf("frame length=%d, want at least 4 bytes", len(frame))
	}
	if got := frame[:4]; got[0] != 0x01 || got[1] != 0x23 || got[2] != 0x45 || got[3] != 0x67 {
		t.Fatalf("x86 frame prefix = %02X %02X %02X %02X, want 01 23 45 67", got[0], got[1], got[2], got[3])
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

func TestRestoreLegacyVideoConfigClearsFlatFramebufferBase(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	video.Reset()
	applyX86FlatProgramVideoConfig(bus, video)
	video.HandleWrite(VIDEO_MODE, MODE_320x200)
	video.HandleWrite(VIDEO_COLOR_MODE, 0)
	video.HandleWrite(VIDEO_FB_BASE, VRAM_START+0x100)
	video.HandleWrite(VIDEO_CTRL, 1)

	bus.memory[VRAM_START+0x100] = 0xAA
	bus.memory[VRAM_START+0x101] = 0xBB
	bus.memory[VRAM_START+0x102] = 0xCC
	bus.memory[VRAM_START+0x103] = 0xDD

	restoreLegacyVideoConfig(bus, video)
	if got := video.HandleRead(VIDEO_FB_BASE); got != 0 {
		t.Fatalf("VIDEO_FB_BASE after legacy restore = 0x%X, want 0", got)
	}

	bus.Write32(VRAM_START, 0x11223344)
	frame := video.GetFrame()
	if len(frame) < 4 {
		t.Fatalf("frame length=%d, want at least 4 bytes", len(frame))
	}
	if got := frame[:4]; got[0] != 0x44 || got[1] != 0x33 || got[2] != 0x22 || got[3] != 0x11 {
		t.Fatalf("legacy frame prefix = %02X %02X %02X %02X, want 44 33 22 11", got[0], got[1], got[2], got[3])
	}
}

func TestResetVideoProfileBasicBootRestoresTerminalFrontBuffer(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	applyX86FlatProgramVideoConfig(bus, video)
	video.Reset()

	if err := applyResetVideoConfigAfterVideoReset(bus, video, "ie64", true, false); err != nil {
		t.Fatalf("applyResetVideoConfigAfterVideoReset BASIC: %v", err)
	}
	if video.directVRAM != nil {
		t.Fatalf("BASIC reset left direct VRAM enabled")
	}
	if !bus.IsIOAddress(VRAM_START) {
		t.Fatalf("BASIC reset did not restore legacy VRAM I/O mapping")
	}
	if got := video.HandleRead(VIDEO_FB_BASE); got != 0 {
		t.Fatalf("BASIC reset VIDEO_FB_BASE = 0x%X, want 0", got)
	}

	term := NewTerminalMMIO()
	vt := NewVideoTerminal(video, term)
	defer vt.Stop()
	video.HandleWrite(VIDEO_CTRL, 1)
	term.HandleWrite(TERM_OUT, uint32('R'))

	frame := video.GetFrame()
	front := video.GetFrontBuffer()
	if len(frame) == 0 {
		t.Fatalf("GetFrame returned an empty frame after terminal output")
	}
	if !bytes.Equal(frame, front) {
		t.Fatalf("BASIC reset frame is not the terminal front buffer; frame len=%d front len=%d", len(frame), len(front))
	}
}

func TestResetVideoProfileBareIE64KeepsFlatProgramVideoPath(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	restoreLegacyVideoConfig(bus, video)
	video.Reset()

	if err := applyResetVideoConfigAfterVideoReset(bus, video, "ie64", false, false); err != nil {
		t.Fatalf("applyResetVideoConfigAfterVideoReset IE64: %v", err)
	}
	if video.directVRAM == nil {
		t.Fatalf("bare IE64 reset did not install direct VRAM")
	}
	if bus.IsIOAddress(VRAM_START) {
		t.Fatalf("bare IE64 reset left legacy VRAM mapped as I/O")
	}
}

func TestResetVideoProfileIntuitionOSRestoresKernelTerminalVideoPath(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	applyIE64FlatProgramVideoConfig(bus, video)
	video.Reset()

	if err := applyResetVideoConfigAfterVideoReset(bus, video, "intuitionos", false, false); err != nil {
		t.Fatalf("applyResetVideoConfigAfterVideoReset IntuitionOS: %v", err)
	}
	if video.directVRAM != nil {
		t.Fatalf("IntuitionOS reset left flat-program direct VRAM enabled")
	}
	if !bus.IsIOAddress(VRAM_START) {
		t.Fatalf("IntuitionOS reset did not restore legacy VRAM I/O mapping")
	}
	if got := video.HandleRead(VIDEO_FB_BASE); got != 0 {
		t.Fatalf("IntuitionOS reset VIDEO_FB_BASE = 0x%X, want 0", got)
	}
	if video.bigEndianMode {
		t.Fatalf("IntuitionOS reset left video in big-endian mode")
	}

	term := NewTerminalMMIO()
	vt := NewVideoTerminal(video, term)
	defer vt.Stop()
	video.HandleWrite(VIDEO_CTRL, 1)
	term.HandleWrite(TERM_OUT, uint32('I'))

	frame := video.GetFrame()
	front := video.GetFrontBuffer()
	if len(frame) == 0 {
		t.Fatalf("GetFrame returned an empty frame after IntuitionOS terminal output")
	}
	if !bytes.Equal(frame, front) {
		t.Fatalf("IntuitionOS reset frame is not the terminal front buffer; frame len=%d front len=%d", len(frame), len(front))
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

func TestProgramExecutorX86ReloadAppliesFlatVideoConfig(t *testing.T) {
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
	if err := exec.prepareAndLaunch([]byte{0xF4}, EXEC_TYPE_X86); err != nil {
		t.Fatalf("prepareAndLaunch X86: %v", err)
	}
	defer exec.stopRunningCPUs()

	const addr = VRAM_START + 0x567
	bus.Write8(addr, 0x3C)

	if writes != 0 {
		t.Fatalf("x86 reload left VRAM aperture mapped to MMIO; write count=%d", writes)
	}
	if got := bus.memory[addr]; got != 0x3C {
		t.Fatalf("bus memory at 0x%X = 0x%02X, want 0x3C", addr, got)
	}
}

func TestX86FlatProgramVideoConfigClearsJITBitmapVRAMPagesBeforeRunnerBuild(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	restoreLegacyVideoConfig(bus, video)
	legacyRunner := NewCPUX86Runner(bus, &CPUX86Config{JITEnabled: true})
	vramPage := int(VRAM_START >> 8)
	if vramPage >= len(legacyRunner.cpu.x86JitIOBitmap) {
		t.Fatalf("VRAM page 0x%X outside x86 JIT bitmap len=%d", vramPage, len(legacyRunner.cpu.x86JitIOBitmap))
	}
	if got := legacyRunner.cpu.x86JitIOBitmap[vramPage]; got == 0 {
		t.Fatalf("legacy x86 JIT bitmap page 0x%X = %d, want MMIO before flat remap", vramPage, got)
	}

	applyX86FlatProgramVideoConfig(bus, video)
	flatRunner := NewCPUX86Runner(bus, &CPUX86Config{JITEnabled: true})
	if got := flatRunner.cpu.x86JitIOBitmap[vramPage]; got != 0 {
		t.Fatalf("flat x86 JIT bitmap page 0x%X = %d, want RAM after flat remap", vramPage, got)
	}
}

func TestX86FlatProgramVideoConfigUnsealsBeforeUnmappingVRAM(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	restoreLegacyVideoConfig(bus, video)
	bus.SealMappings()

	applyX86FlatProgramVideoConfig(bus, video)
	restoreLegacyVideoConfig(bus, video)

	bus.Write32(VRAM_START, 0xCAFEBABE)
	if got := video.HandleRead(VRAM_START); got != 0xCAFEBABE {
		t.Fatalf("legacy VRAM read = 0x%08X, want 0xCAFEBABE after sealed x86 flat remap", got)
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

	if err := exec.prepareAndLaunch([]byte{0x76}, EXEC_TYPE_Z80); err != nil {
		t.Fatalf("prepareAndLaunch Z80: %v", err)
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

	if err := exec.prepareAndLaunch([]byte{0x76}, EXEC_TYPE_Z80); err != nil {
		t.Fatalf("prepareAndLaunch Z80: %v", err)
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
	if err := exec.prepareAndLaunch([]byte{0x76}, EXEC_TYPE_Z80); err != nil {
		t.Fatalf("prepareAndLaunch Z80: %v", err)
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

	if err := exec.prepareAndLaunch([]byte{0x76}, EXEC_TYPE_Z80); err != nil {
		t.Fatalf("prepareAndLaunch Z80: %v", err)
	}

	bus.Write32(VRAM_START, 0x87654321)
	if got := video.HandleRead(VRAM_START); got != 0x87654321 {
		t.Fatalf("legacy EXEC VRAM read = 0x%08X, want 0x87654321 after sealed M68K reload", got)
	}
}

func TestProgramExecutorLegacyReloadRestoresVRAMAfterX86FlatMode(t *testing.T) {
	bus := NewMachineBus()
	video := newM68KFlatTestVideoChip(t)
	defer video.Stop()

	exec := NewProgramExecutor(bus, nil, video, nil, nil, t.TempDir())
	if err := exec.prepareAndLaunch([]byte{0xF4}, EXEC_TYPE_X86); err != nil {
		t.Fatalf("prepareAndLaunch X86: %v", err)
	}
	defer exec.stopRunningCPUs()

	if err := exec.prepareAndLaunch([]byte{0x76}, EXEC_TYPE_Z80); err != nil {
		t.Fatalf("prepareAndLaunch Z80: %v", err)
	}

	bus.Write32(VRAM_START, 0x11223344)
	if got := video.HandleRead(VRAM_START); got != 0x11223344 {
		t.Fatalf("legacy EXEC VRAM read = 0x%08X, want 0x11223344 after x86 flat reload", got)
	}
}
