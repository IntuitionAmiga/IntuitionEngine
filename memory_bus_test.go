package main

import (
	"encoding/binary"
	"testing"
)

// TestMemoryBusGetMemory verifies that SystemBus exposes its memory slice
// via GetMemory() for direct access by CPU cores.
func TestMemoryBusGetMemory(t *testing.T) {
	bus := NewSystemBus()

	mem := bus.GetMemory()
	if mem == nil {
		t.Fatal("GetMemory() returned nil")
	}
	if len(mem) != DEFAULT_MEMORY_SIZE {
		t.Fatalf("GetMemory() length %d, expected %d", len(mem), DEFAULT_MEMORY_SIZE)
	}

	// Write through bus, read through memory slice
	bus.Write32(0x1000, 0x12345678)
	got := binary.LittleEndian.Uint32(mem[0x1000:])
	if got != 0x12345678 {
		t.Fatalf("Direct memory read 0x%08X, expected 0x12345678", got)
	}
}

// TestCPUMemoryVisibleToBus verifies that data written by the CPU
// is visible when read through the SystemBus (as peripherals would).
func TestCPUMemoryVisibleToBus(t *testing.T) {
	bus := NewSystemBus()
	cpu := NewCPU(bus)

	// Write through CPU's memory at address 0x2000
	cpu.Write32(0x2000, 0xDEADBEEF)

	// Read through bus - should see the same value
	got := bus.Read32(0x2000)
	if got != 0xDEADBEEF {
		t.Fatalf("Bus read 0x%08X, expected 0xDEADBEEF - memory not shared", got)
	}
}
