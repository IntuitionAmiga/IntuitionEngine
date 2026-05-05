package main

import (
	"encoding/binary"
	"os"
	"testing"
	"time"
)

// buildAROSTestROM creates a minimal test ROM with the given initial SSP and PC
// in big-endian M68K format, sized to the specified length.
func buildAROSTestROM(size int, sp uint32, pc uint32) []byte {
	rom := make([]byte, size)
	binary.BigEndian.PutUint32(rom[0:4], sp)
	binary.BigEndian.PutUint32(rom[4:8], pc)
	return rom
}

func TestAROSLoader_LoadROM(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)

	const romSize = 256 * 1024
	wantPC := uint32(arosROMBase + 0x100)
	rom := buildAROSTestROM(romSize, 0x00020000, wantPC)

	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}

	// Verify ROM data was loaded at the correct base address.
	// The loader byte-swaps big-endian words into bus memory.
	gotByte := bus.Read8(arosROMBase)
	if gotByte != rom[0] {
		t.Errorf("ROM first byte at 0x%X: got 0x%02X, want 0x%02X", arosROMBase, gotByte, rom[0])
	}

	// Verify reset vectors.
	if got := cpu.Read32(0); got != arosBootSP {
		t.Errorf("SSP at address 0: got 0x%08X, want 0x%08X", got, arosBootSP)
	}
	if cpu.PC != wantPC {
		t.Errorf("PC: got 0x%08X, want 0x%08X", cpu.PC, wantPC)
	}
	if cpu.AddrRegs[7] != arosBootSP {
		t.Errorf("A7: got 0x%08X, want 0x%08X", cpu.AddrRegs[7], arosBootSP)
	}
}

func TestAROSLoader_LoadROM_PreservesWordOffsets(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)

	rom := buildAROSTestROM(0x40, 0x00020000, arosROMBase+0x100)
	words := map[int]uint16{
		0x08: 0x6606,
		0x0A: 0x4CDF,
		0x0C: 0x0C04,
		0x0E: 0x4E75,
		0x10: 0x226B,
		0x12: 0x000E,
		0x14: 0x246B,
		0x16: 0x0012,
		0x18: 0x487A,
	}
	for off, word := range words {
		binary.BigEndian.PutUint16(rom[off:off+2], word)
	}

	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}

	base := uint32(arosROMBase)
	for off, want := range words {
		addr := base + uint32(off)
		if got := cpu.Read16(addr); got != want {
			t.Fatalf("cpu.Read16(0x%08X) = 0x%04X, want 0x%04X", addr, got, want)
		}
	}
}

func TestAROSLoader_LoadROM_RealROMProbe(t *testing.T) {
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)
	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}

	// Structural checks (ROM-version-independent):
	// (1) Reset SSP at vector 0 must be a valid stack-top in RAM (non-zero, even).
	// (2) Reset PC at vector 1 must point inside the loaded ROM range.
	// (3) ROM payload at the load offset must contain non-zero bytes
	//     (sanity that the ROM body actually got mapped).
	resetSSP := cpu.Read32(0)
	resetPC := cpu.Read32(4)
	if resetSSP == 0 || (resetSSP&1) != 0 {
		t.Fatalf("reset SSP at vector 0 = 0x%08X, want non-zero even value", resetSSP)
	}
	if resetPC < arosROMBase || uint64(resetPC) >= uint64(arosROMBase)+uint64(len(rom)) {
		t.Fatalf("reset PC at vector 1 = 0x%08X, want inside ROM [0x%08X, 0x%08X)",
			resetPC, arosROMBase, uint64(arosROMBase)+uint64(len(rom)))
	}
	// Probe a small window inside the ROM body for non-zero content.
	nonZero := false
	for off := uint32(0x100); off < 0x200; off += 2 {
		if cpu.Read16(arosROMBase+off) != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatalf("ROM body at arosROMBase+0x100..0x200 is all zero; ROM not mapped")
	}
}

func TestAROSLoader_LoadROM_TooSmall(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)

	rom := make([]byte, 4) // Too small (needs at least 8 bytes)
	if err := loader.LoadROM(rom); err == nil {
		t.Fatal("expected error for ROM < 8 bytes, got nil")
	}
}

func TestAROSLoader_ROMPageNoIOCollision(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)

	const romSize = 128 * 1024
	rom := buildAROSTestROM(romSize, 0x1000, 0x2000)

	base := uint32(arosROMBase)
	startPage := base >> 8
	endPage := (base + uint32(romSize) - 1) >> 8
	for p := startPage; p <= endPage; p++ {
		if bus.ioPageBitmap[p] {
			t.Fatalf("unexpected I/O mapping on ROM page 0x%X", p)
		}
	}

	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}
}

func TestAROSLoader_VectorSetup(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)

	wantSP := uint32(0x00012345)
	wantPC := uint32(arosROMBase + 0x200)
	rom := buildAROSTestROM(64*1024, wantSP, wantPC)

	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}

	// The loader overrides SSP with arosBootSP, but PC comes from ROM.
	if cpu.AddrRegs[7] != arosBootSP {
		t.Errorf("A7 got 0x%08X, want arosBootSP 0x%08X", cpu.AddrRegs[7], arosBootSP)
	}
	if cpu.PC != wantPC {
		t.Errorf("PC got 0x%08X, want 0x%08X", cpu.PC, wantPC)
	}
}

func TestAROSLoader_StackBoundsDisabled(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)

	rom := buildAROSTestROM(64*1024, 0x20000, arosROMBase+0x100)
	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}

	// AROS moves SP throughout boot — stack bounds should be wide open.
	// PLAN_MAX_RAM slice 10h: AROS_PROFILE_TOP raised to 2 GiB; on a 32
	// MiB legacy test bus the profile clamp pulls the cap down to the
	// bus size, so the loader installs the clamped value.
	if cpu.stackLowerBound != 0 {
		t.Errorf("stackLowerBound: got 0x%X, want 0", cpu.stackLowerBound)
	}
	want := uint32(len(bus.GetMemory()))
	if cpu.stackUpperBound != want {
		t.Errorf("stackUpperBound: got 0x%X, want 0x%X (clamped to bus.memory)",
			cpu.stackUpperBound, want)
	}
}

func TestAROSLoader_TimerFires(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)
	defer loader.Stop()

	// Install valid interrupt vectors so arming check passes.
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x00001000)
	cpu.Write32(uint32(M68K_VEC_LEVEL5)*4, 0x00001000)
	cpu.SetRunning(true)
	cpu.pendingInterrupt.Store(0)
	loader.StartTimer()

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cpu.pendingInterrupt.Load() != 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timer did not assert any interrupt bits")
}

func TestAROSLoader_TimerArmingCheck(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)
	defer loader.Stop()

	// Do NOT install vectors — arming should stay false.
	cpu.SetRunning(true)
	cpu.pendingInterrupt.Store(0)
	loader.StartTimer()

	// Wait enough time for at least one timer tick.
	time.Sleep(30 * time.Millisecond)
	if got := cpu.pendingInterrupt.Load(); got != 0 {
		t.Fatalf("timer should not fire without valid vectors, got pending=0x%X", got)
	}

	// Now install vectors and verify arming works.
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x00001000)
	cpu.Write32(uint32(M68K_VEC_LEVEL5)*4, 0x00001000)
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cpu.pendingInterrupt.Load() != 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timer did not fire after installing valid vectors")
}

func TestAROSLoader_RevalidatesIRQArming(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x00001000)
	loader.refreshIRQArming()
	if !loader.l4Armed {
		t.Fatalf("L4 did not arm with valid vector")
	}
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0xFFFFFFFF)
	loader.refreshIRQArming()
	if loader.l4Armed {
		t.Fatalf("L4 stayed armed after vector became invalid")
	}
}

func TestAROSLoader_DebugWatchOptIn(t *testing.T) {
	t.Setenv("IE_AROS_DEBUG", "")
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)
	rom := buildAROSTestROM(64*1024, 0x20000, arosROMBase+0x100)
	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}
	if cpu.DebugWatchFn != nil {
		t.Fatalf("DebugWatchFn installed without IE_AROS_DEBUG=1")
	}
}

func TestAROSLoader_TimerStopsOnCancel(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)

	loader.StartTimer()
	time.Sleep(10 * time.Millisecond)
	loader.Stop()
	stable := cpu.pendingInterrupt.Load()
	time.Sleep(30 * time.Millisecond)
	if got := cpu.pendingInterrupt.Load(); got != stable {
		t.Fatalf("pending interrupt changed after Stop: before 0x%X, after 0x%X", stable, got)
	}
}

func TestAROSLoader_TimerPausedWhileCPUStopped(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)
	defer loader.Stop()

	// Install valid vectors.
	cpu.Write32(uint32(M68K_VEC_LEVEL4)*4, 0x00001000)
	cpu.Write32(uint32(M68K_VEC_LEVEL5)*4, 0x00001000)
	loader.StartTimer()
	cpu.pendingInterrupt.Store(0)
	cpu.SetRunning(false) // CPU paused

	time.Sleep(30 * time.Millisecond)
	if got := cpu.pendingInterrupt.Load(); got != 0 {
		t.Fatalf("should not fire while CPU stopped, got 0x%X", got)
	}

	// Resume CPU and verify timer fires.
	cpu.SetRunning(true)
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cpu.pendingInterrupt.Load() != 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timer did not resume after CPU restart")
}

func TestAROSLoader_IsValidVector(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)
	tests := []struct {
		pc   uint32
		want bool
	}{
		{0, false},
		{0xFFFFFFFF, false},
		{0x00001000, true}, // Valid RAM address
		{arosROMBase + 100, true},
		{0x00000FFF, false}, // Below 0x1000
		{AROS_PROFILE_TOP, false},
		{AROS_PROFILE_TOP + 0x1000, false},
	}
	for _, tt := range tests {
		got := loader.isValidVector(tt.pc)
		if got != tt.want {
			t.Errorf("loader.isValidVector(0x%08X) = %v, want %v", tt.pc, got, tt.want)
		}
	}
}

func TestAROSLoader_LoadROM_InstallsProfileTopOfRAM(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	loader := NewAROSLoader(bus, cpu, nil)
	// Tiny synthetic ROM so the test never depends on a live image.
	rom := make([]byte, 16)
	rom[0], rom[1], rom[2], rom[3] = 0x00, 0x02, 0x00, 0x00 // SP = 0x00020000
	rom[4], rom[5], rom[6], rom[7] = 0x00, 0x60, 0x00, 0x10 // PC = arosROMBase + 0x10
	if err := loader.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM failed: %v", err)
	}
	// PLAN_MAX_RAM slice 10h: AROS_PROFILE_TOP raised to 2 GiB; on a
	// legacy 32 MiB test bus the profile clamp pulls TopOfRAM down to
	// len(bus.memory). Assert the loader installs the clamped value.
	wantTop := uint32(len(bus.GetMemory()))
	if got := cpu.ProfileTopOfRAM(); got != wantTop {
		t.Fatalf("cpu profile top = 0x%X, want 0x%X (clamped to bus.memory)",
			got, wantTop)
	}
	if got := cpu.stackUpperBound; got != wantTop {
		t.Fatalf("stackUpperBound = 0x%X, want 0x%X", got, wantTop)
	}
	if loader.profile.VRAMBase != arosDirectVRAMBase {
		t.Fatalf("loader profile VRAMBase = 0x%X, want 0x%X (direct VRAM contract)",
			loader.profile.VRAMBase, arosDirectVRAMBase)
	}
	if loader.profile.VRAMEnd != arosDirectVRAMBase+arosDirectVRAMSize {
		t.Fatalf("loader profile VRAMEnd = 0x%X, want 0x%X",
			loader.profile.VRAMEnd, arosDirectVRAMBase+arosDirectVRAMSize)
	}
}
