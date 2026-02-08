package main

import (
	"testing"
)

// Test 1: RAM read/write
func TestSAPPlaybackBus6502_RAMReadWrite(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false) // mono, PAL
	bus.Write(0x1000, 0x42)
	if bus.Read(0x1000) != 0x42 {
		t.Errorf("expected 0x42, got 0x%02X", bus.Read(0x1000))
	}
}

// Test 2: RAM in upper region (non-ROM for SAP)
func TestSAPPlaybackBus6502_RAMUpperRegion(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.Write(0xE000, 0x55)
	if bus.Read(0xE000) != 0x55 {
		t.Errorf("expected 0x55, got 0x%02X", bus.Read(0xE000))
	}
}

// Test 3: POKEY write capture (mono)
func TestSAPPlaybackBus6502_POKEYWriteCapture(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.StartFrame()

	bus.Write(0xD200, 0x10) // AUDF1
	bus.Write(0xD201, 0xAF) // AUDC1

	events := bus.CollectEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Reg != 0 {
		t.Errorf("expected reg 0 (AUDF1), got %d", events[0].Reg)
	}
	if events[0].Value != 0x10 {
		t.Errorf("expected value 0x10, got 0x%02X", events[0].Value)
	}
	if events[1].Reg != 1 {
		t.Errorf("expected reg 1 (AUDC1), got %d", events[1].Reg)
	}
	if events[1].Value != 0xAF {
		t.Errorf("expected value 0xAF, got 0x%02X", events[1].Value)
	}
}

// Test 4: POKEY mirroring (mono: 16x mirrors)
func TestSAPPlaybackBus6502_POKEYMirror(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.StartFrame()

	bus.Write(0xD210, 0x20) // Mirror of D200+0 = AUDF1
	bus.Write(0xD220, 0x30) // Another mirror

	events := bus.CollectEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Both should be AUDF1 (reg 0)
	if events[0].Reg != 0 {
		t.Errorf("expected reg 0 for mirror, got %d", events[0].Reg)
	}
	if events[1].Reg != 0 {
		t.Errorf("expected reg 0 for mirror, got %d", events[1].Reg)
	}
}

// Test 5: Stereo POKEY (two chips)
func TestSAPPlaybackBus6502_StereoPOKEY(t *testing.T) {
	bus := newSAPPlaybackBus6502(true, false) // stereo, PAL
	bus.StartFrame()

	bus.Write(0xD200, 0x10) // Left POKEY AUDF1
	bus.Write(0xD210, 0x20) // Right POKEY AUDF1

	events := bus.CollectEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Chip != 0 {
		t.Errorf("expected chip 0 (left), got %d", events[0].Chip)
	}
	if events[1].Chip != 1 {
		t.Errorf("expected chip 1 (right), got %d", events[1].Chip)
	}
}

// Test 6: GTIA PAL detection
func TestSAPPlaybackBus6502_GTIA_PAL(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false) // PAL
	// PAL returns 0x01 at $D014 (CONSOL)
	if bus.Read(0xD014) != 0x01 {
		t.Errorf("expected PAL value 0x01, got 0x%02X", bus.Read(0xD014))
	}
}

// Test 7: GTIA NTSC detection
func TestSAPPlaybackBus6502_GTIA_NTSC(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, true) // NTSC
	// NTSC returns 0x0F at $D014 (CONSOL)
	if bus.Read(0xD014) != 0x0F {
		t.Errorf("expected NTSC value 0x0F, got 0x%02X", bus.Read(0xD014))
	}
}

// Test 8: VCOUNT reads
func TestSAPPlaybackBus6502_VCOUNT(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.scanline = 100
	// VCOUNT = scanline / 2
	if bus.Read(0xD40B) != 50 {
		t.Errorf("expected VCOUNT 50, got %d", bus.Read(0xD40B))
	}
}

// Test 9: VCOUNT wraparound
func TestSAPPlaybackBus6502_VCOUNT_Wraparound(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.scanline = 312 // PAL max
	if bus.Read(0xD40B) != 156 {
		t.Errorf("expected VCOUNT 156, got %d", bus.Read(0xD40B))
	}
}

// Test 10: Cycle counting
func TestSAPPlaybackBus6502_CycleCounting(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.StartFrame()

	bus.AddCycles(100)
	bus.Write(0xD200, 0x10)

	events := bus.CollectEvents()
	if len(events) < 1 {
		t.Fatal("expected at least one event")
	}
	if events[0].Cycle != 100 {
		t.Errorf("expected cycle 100, got %d", events[0].Cycle)
	}
}

// Test 11: WSYNC stall behavior
func TestSAPPlaybackBus6502_WSYNC(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.scanline = 50
	bus.AddCycles(50) // Mid-scanline

	bus.Write(0xD40A, 0x00) // WSYNC write

	// Should have advanced to next scanline boundary
	if bus.scanline != 51 {
		t.Errorf("expected scanline 51 after WSYNC, got %d", bus.scanline)
	}
}

// Test 12: Load binary blocks
func TestSAPPlaybackBus6502_LoadBlocks(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	blocks := []SAPBlock{
		{Start: 0x1000, End: 0x1003, Data: []byte{0xA9, 0x42, 0x60, 0x00}},
	}
	bus.LoadBlocks(blocks)

	if bus.Read(0x1000) != 0xA9 {
		t.Errorf("expected 0xA9, got 0x%02X", bus.Read(0x1000))
	}
	if bus.Read(0x1001) != 0x42 {
		t.Errorf("expected 0x42, got 0x%02X", bus.Read(0x1001))
	}
	if bus.Read(0x1002) != 0x60 {
		t.Errorf("expected 0x60, got 0x%02X", bus.Read(0x1002))
	}
}

// Test 13: POKEY register readback (KBCODE, etc)
func TestSAPPlaybackBus6502_POKEYReadback(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	// Most POKEY reads return 0 except special cases
	// RANDOM ($D20A) should return pseudo-random value
	val := bus.Read(0xD20A)
	// Just verify it doesn't panic and returns something
	_ = val
}

// Test 14: ANTIC NMIST/NMIEN
func TestSAPPlaybackBus6502_ANTICReads(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	// NMIST at $D40F - status register
	// Should return something reasonable
	val := bus.Read(0xD40F)
	_ = val
}

// Test 15: Zero page operations
func TestSAPPlaybackBus6502_ZeroPage(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.Write(0x00, 0xAA)
	bus.Write(0xFF, 0x55)
	if bus.Read(0x00) != 0xAA {
		t.Errorf("expected 0xAA at $00, got 0x%02X", bus.Read(0x00))
	}
	if bus.Read(0xFF) != 0x55 {
		t.Errorf("expected 0x55 at $FF, got 0x%02X", bus.Read(0xFF))
	}
}

// Test 16: All POKEY registers captured
func TestSAPPlaybackBus6502_AllPOKEYRegisters(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false)
	bus.StartFrame()

	// Write to all POKEY registers
	for i := 0; i < 9; i++ {
		bus.Write(uint16(0xD200+i), uint8(i*16))
	}

	events := bus.CollectEvents()
	if len(events) != 9 {
		t.Fatalf("expected 9 events, got %d", len(events))
	}
	for i, e := range events {
		if e.Reg != uint8(i) {
			t.Errorf("event %d: expected reg %d, got %d", i, i, e.Reg)
		}
		if e.Value != uint8(i*16) {
			t.Errorf("event %d: expected value 0x%02X, got 0x%02X", i, i*16, e.Value)
		}
	}
}

// Test 17: Cycle to scanline conversion
func TestSAPPlaybackBus6502_CycleToScanline(t *testing.T) {
	bus := newSAPPlaybackBus6502(false, false) // PAL
	// PAL: 114 cycles per scanline
	bus.AddCycles(114)
	if bus.scanline != 1 {
		t.Errorf("expected scanline 1 after 114 cycles, got %d", bus.scanline)
	}
	bus.AddCycles(114 * 10)
	if bus.scanline != 11 {
		t.Errorf("expected scanline 11, got %d", bus.scanline)
	}
}
