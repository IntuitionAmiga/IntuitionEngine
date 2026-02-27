package main

import (
	"encoding/binary"
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
	if cpu.stackLowerBound != 0 {
		t.Errorf("stackLowerBound: got 0x%X, want 0", cpu.stackLowerBound)
	}
	if cpu.stackUpperBound != DEFAULT_MEMORY_SIZE {
		t.Errorf("stackUpperBound: got 0x%X, want 0x%X", cpu.stackUpperBound, DEFAULT_MEMORY_SIZE)
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
	tests := []struct {
		pc   uint32
		want bool
	}{
		{0, false},
		{0xFFFFFFFF, false},
		{0x00001000, true}, // Valid RAM address
		{arosROMBase + 100, true},
		{0x00000FFF, false}, // Below 0x1000
		{DEFAULT_MEMORY_SIZE, false},
	}
	for _, tt := range tests {
		got := isValidAROSVector(tt.pc)
		if got != tt.want {
			t.Errorf("isValidAROSVector(0x%08X) = %v, want %v", tt.pc, got, tt.want)
		}
	}
}
