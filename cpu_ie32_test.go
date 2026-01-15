package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadProgramVisibleToPeripherals verifies that programs loaded via
// LoadProgram() are visible to peripherals reading through the SystemBus.
func TestLoadProgramVisibleToPeripherals(t *testing.T) {
	bus := NewSystemBus()
	cpu := NewCPU(bus)

	// Create a minimal test program with known data at offset 8 (relative to PROG_START)
	// Program: 8 bytes padding + 4 bytes data (0xCAFEBABE)
	program := make([]byte, 12)
	program[0] = 0xEE // NOP opcode as placeholder
	binary.LittleEndian.PutUint32(program[8:], 0xCAFEBABE)

	// Write to temp file and load
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.iex")
	if err := os.WriteFile(tmpFile, program, 0644); err != nil {
		t.Fatalf("failed to write test program: %v", err)
	}

	if err := cpu.LoadProgram(tmpFile); err != nil {
		t.Fatalf("failed to load program: %v", err)
	}

	// Data should be at PROG_START + 8 = 0x1008
	// Read through bus (as a peripheral would)
	got := bus.Read32(PROG_START + 8)
	if got != 0xCAFEBABE {
		t.Fatalf("Bus read 0x%08X at 0x%X, expected 0xCAFEBABE - program data not visible to peripherals", got, PROG_START+8)
	}
}

// TestCPUMemoryWriteVisibleToBus verifies that direct memory writes
// by the CPU are visible through the bus.
func TestCPUMemoryWriteVisibleToBus(t *testing.T) {
	bus := NewSystemBus()
	cpu := NewCPU(bus)

	// CPU writes directly to memory
	cpu.Write32(0x2000, 0x12345678)

	// Read through bus - should see the same value
	got := bus.Read32(0x2000)
	if got != 0x12345678 {
		t.Fatalf("Bus read 0x%08X, expected 0x12345678 - CPU writes not visible to bus", got)
	}
}
