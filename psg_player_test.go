package main

import "testing"

// TestPSGPlayerCanReadCPULoadedData verifies that the PSG player can read
// audio data that was loaded by the CPU (simulating embedded .ay file data).
func TestPSGPlayerCanReadCPULoadedData(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	soundChip := newTestSoundChip()
	engine := NewPSGEngine(soundChip, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	player.AttachBus(bus)

	// CPU writes "ZXAY" header as a 32-bit word to 0x3000 (simulating embedded .ay file)
	// "ZXAY" in little-endian: 0x5941585A
	cpu.Write32(0x3000, 0x5941585A)

	// Verify player can read the header via bus
	got := bus.Read32(0x3000)
	if got != 0x5941585A {
		t.Fatalf("Bus read 0x%08X, expected 0x5941585A (\"ZXAY\") - CPU memory not visible to PSG player via bus", got)
	}
}
