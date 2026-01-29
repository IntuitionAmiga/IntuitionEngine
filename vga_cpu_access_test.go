// vga_cpu_access_test.go - TDD tests for Z80 and 6502 VGA register access

package main

import (
	"testing"
)

// =============================================================================
// Z80 VGA Port I/O Tests
// =============================================================================

func TestZ80_VGA_PortOut_Mode(t *testing.T) {
	// Setup: Create system bus with VGA engine
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: OUT (0xA0), value → VGA_MODE
	z80Bus.Out(Z80_VGA_PORT_MODE, VGA_MODE_13H)

	// Verify: VGA mode should be set
	mode := vga.HandleRead(VGA_MODE)
	if mode != VGA_MODE_13H {
		t.Errorf("VGA mode via Z80 port: got 0x%02X, want 0x%02X", mode, VGA_MODE_13H)
	}
}

func TestZ80_VGA_PortIn_Status(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: IN A, (0xA1) → VGA_STATUS
	status := z80Bus.In(Z80_VGA_PORT_STATUS)

	// Verify: Should return valid status (at least no crash)
	// Status bits: bit 0 = vsync, bit 3 = retrace
	// We just verify we get a byte back without error
	_ = status // Status value varies based on timing
}

func TestZ80_VGA_PortOut_Control(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: OUT (0xA2), value → VGA_CTRL
	z80Bus.Out(Z80_VGA_PORT_CTRL, VGA_CTRL_ENABLE)

	// Verify: VGA control should be set
	ctrl := vga.HandleRead(VGA_CTRL)
	if ctrl != VGA_CTRL_ENABLE {
		t.Errorf("VGA control via Z80 port: got 0x%02X, want 0x%02X", ctrl, VGA_CTRL_ENABLE)
	}
}

func TestZ80_VGA_PortOut_DAC(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: Set palette entry 10 via Z80 ports
	// First set write index
	z80Bus.Out(Z80_VGA_PORT_DAC_WIDX, 10)

	// Write R, G, B values
	z80Bus.Out(Z80_VGA_PORT_DAC_DATA, 63) // Red
	z80Bus.Out(Z80_VGA_PORT_DAC_DATA, 32) // Green
	z80Bus.Out(Z80_VGA_PORT_DAC_DATA, 0)  // Blue

	// Verify: Palette entry 10 should be set
	r, g, b := vga.GetPaletteEntry(10)
	if r != 63 || g != 32 || b != 0 {
		t.Errorf("VGA palette via Z80 port: got (%d,%d,%d), want (63,32,0)", r, g, b)
	}
}

func TestZ80_VGA_PortOut_Sequencer(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: Set sequencer map mask via Z80 ports
	z80Bus.Out(Z80_VGA_PORT_SEQ_IDX, VGA_SEQ_MAPMASK_R)
	z80Bus.Out(Z80_VGA_PORT_SEQ_DATA, 0x0F) // All planes

	// Verify: Sequencer register should be set
	idx := vga.HandleRead(VGA_SEQ_INDEX)
	data := vga.HandleRead(VGA_SEQ_DATA)
	if idx != VGA_SEQ_MAPMASK_R {
		t.Errorf("VGA seq index via Z80 port: got 0x%02X, want 0x%02X", idx, VGA_SEQ_MAPMASK_R)
	}
	if data != 0x0F {
		t.Errorf("VGA seq data via Z80 port: got 0x%02X, want 0x0F", data)
	}
}

func TestZ80_VGA_PortOut_CRTC(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: Set CRTC start address via Z80 ports
	z80Bus.Out(Z80_VGA_PORT_CRTC_IDX, VGA_CRTC_START_HI)
	z80Bus.Out(Z80_VGA_PORT_CRTC_DATA, 0x12)

	// Verify: CRTC register should be set
	idx := vga.HandleRead(VGA_CRTC_INDEX)
	data := vga.HandleRead(VGA_CRTC_DATA)
	if idx != VGA_CRTC_START_HI {
		t.Errorf("VGA CRTC index via Z80 port: got 0x%02X, want 0x%02X", idx, VGA_CRTC_START_HI)
	}
	if data != 0x12 {
		t.Errorf("VGA CRTC data via Z80 port: got 0x%02X, want 0x12", data)
	}
}

func TestZ80_VGA_PortOut_GC(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: Set Graphics Controller read map via Z80 ports
	z80Bus.Out(Z80_VGA_PORT_GC_IDX, VGA_GC_READ_MAP_R)
	z80Bus.Out(Z80_VGA_PORT_GC_DATA, 0x02) // Plane 2

	// Verify: GC register should be set
	idx := vga.HandleRead(VGA_GC_INDEX)
	data := vga.HandleRead(VGA_GC_DATA)
	if idx != VGA_GC_READ_MAP_R {
		t.Errorf("VGA GC index via Z80 port: got 0x%02X, want 0x%02X", idx, VGA_GC_READ_MAP_R)
	}
	if data != 0x02 {
		t.Errorf("VGA GC data via Z80 port: got 0x%02X, want 0x02", data)
	}
}

// =============================================================================
// 6502 VGA Memory-Mapped Tests
// =============================================================================

func Test6502_VGA_Write_Mode(t *testing.T) {
	// Setup: Create system bus with VGA engine
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: STA $D700 → VGA_MODE
	adapter.Write(C6502_VGA_MODE, VGA_MODE_13H)

	// Verify: VGA mode should be set
	mode := vga.HandleRead(VGA_MODE)
	if mode != VGA_MODE_13H {
		t.Errorf("VGA mode via 6502: got 0x%02X, want 0x%02X", mode, VGA_MODE_13H)
	}
}

func Test6502_VGA_Read_Status(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: LDA $D701 → VGA_STATUS
	status := adapter.Read(C6502_VGA_STATUS)

	// Verify: Should return valid status (at least no crash)
	_ = status // Status value varies based on timing
}

func Test6502_VGA_Write_Control(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: STA $D702 → VGA_CTRL
	adapter.Write(C6502_VGA_CTRL, VGA_CTRL_ENABLE)

	// Verify: VGA control should be set
	ctrl := vga.HandleRead(VGA_CTRL)
	if ctrl != VGA_CTRL_ENABLE {
		t.Errorf("VGA control via 6502: got 0x%02X, want 0x%02X", ctrl, VGA_CTRL_ENABLE)
	}
}

func Test6502_VGA_DAC(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: Set palette entry 10 via 6502 memory-mapped registers
	// First set write index
	adapter.Write(C6502_VGA_DAC_WIDX, 10)

	// Write R, G, B values
	adapter.Write(C6502_VGA_DAC_DATA, 63) // Red
	adapter.Write(C6502_VGA_DAC_DATA, 32) // Green
	adapter.Write(C6502_VGA_DAC_DATA, 0)  // Blue

	// Verify: Palette entry 10 should be set
	r, g, b := vga.GetPaletteEntry(10)
	if r != 63 || g != 32 || b != 0 {
		t.Errorf("VGA palette via 6502: got (%d,%d,%d), want (63,32,0)", r, g, b)
	}
}

func Test6502_VGA_Sequencer(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: Set sequencer map mask via 6502 memory-mapped registers
	adapter.Write(C6502_VGA_SEQ_IDX, VGA_SEQ_MAPMASK_R)
	adapter.Write(C6502_VGA_SEQ_DATA, 0x0F) // All planes

	// Verify: Sequencer register should be set
	idx := vga.HandleRead(VGA_SEQ_INDEX)
	data := vga.HandleRead(VGA_SEQ_DATA)
	if idx != VGA_SEQ_MAPMASK_R {
		t.Errorf("VGA seq index via 6502: got 0x%02X, want 0x%02X", idx, VGA_SEQ_MAPMASK_R)
	}
	if data != 0x0F {
		t.Errorf("VGA seq data via 6502: got 0x%02X, want 0x0F", data)
	}
}

func Test6502_VGA_CRTC(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: Set CRTC start address via 6502 memory-mapped registers
	adapter.Write(C6502_VGA_CRTC_IDX, VGA_CRTC_START_HI)
	adapter.Write(C6502_VGA_CRTC_DATA, 0x12)

	// Verify: CRTC register should be set
	idx := vga.HandleRead(VGA_CRTC_INDEX)
	data := vga.HandleRead(VGA_CRTC_DATA)
	if idx != VGA_CRTC_START_HI {
		t.Errorf("VGA CRTC index via 6502: got 0x%02X, want 0x%02X", idx, VGA_CRTC_START_HI)
	}
	if data != 0x12 {
		t.Errorf("VGA CRTC data via 6502: got 0x%02X, want 0x12", data)
	}
}

func Test6502_VGA_GC(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: Set Graphics Controller read map via 6502 memory-mapped registers
	adapter.Write(C6502_VGA_GC_IDX, VGA_GC_READ_MAP_R)
	adapter.Write(C6502_VGA_GC_DATA, 0x02) // Plane 2

	// Verify: GC register should be set
	idx := vga.HandleRead(VGA_GC_INDEX)
	data := vga.HandleRead(VGA_GC_DATA)
	if idx != VGA_GC_READ_MAP_R {
		t.Errorf("VGA GC index via 6502: got 0x%02X, want 0x%02X", idx, VGA_GC_READ_MAP_R)
	}
	if data != 0x02 {
		t.Errorf("VGA GC data via 6502: got 0x%02X, want 0x02", data)
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestZ80_VGA_Mode13h_Integration(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: Full Mode 13h setup sequence via Z80 ports
	// 1. Set Mode 13h
	z80Bus.Out(Z80_VGA_PORT_MODE, VGA_MODE_13H)

	// 2. Enable VGA
	z80Bus.Out(Z80_VGA_PORT_CTRL, VGA_CTRL_ENABLE)

	// 3. Set a custom palette entry
	z80Bus.Out(Z80_VGA_PORT_DAC_WIDX, 1)
	z80Bus.Out(Z80_VGA_PORT_DAC_DATA, 63) // Red
	z80Bus.Out(Z80_VGA_PORT_DAC_DATA, 0)  // Green
	z80Bus.Out(Z80_VGA_PORT_DAC_DATA, 0)  // Blue

	// Verify mode
	mode := vga.HandleRead(VGA_MODE)
	if mode != VGA_MODE_13H {
		t.Errorf("Mode 13h setup: got mode 0x%02X, want 0x%02X", mode, VGA_MODE_13H)
	}

	// Verify control
	ctrl := vga.HandleRead(VGA_CTRL)
	if ctrl != VGA_CTRL_ENABLE {
		t.Errorf("Mode 13h setup: got ctrl 0x%02X, want 0x%02X", ctrl, VGA_CTRL_ENABLE)
	}

	// Verify palette
	r, g, b := vga.GetPaletteEntry(1)
	if r != 63 || g != 0 || b != 0 {
		t.Errorf("Mode 13h palette: got (%d,%d,%d), want (63,0,0)", r, g, b)
	}
}

func Test6502_VGA_Mode13h_Integration(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: Full Mode 13h setup sequence via 6502 memory-mapped I/O
	// 1. Set Mode 13h
	adapter.Write(C6502_VGA_MODE, VGA_MODE_13H)

	// 2. Enable VGA
	adapter.Write(C6502_VGA_CTRL, VGA_CTRL_ENABLE)

	// 3. Set a custom palette entry
	adapter.Write(C6502_VGA_DAC_WIDX, 1)
	adapter.Write(C6502_VGA_DAC_DATA, 63) // Red
	adapter.Write(C6502_VGA_DAC_DATA, 0)  // Green
	adapter.Write(C6502_VGA_DAC_DATA, 0)  // Blue

	// Verify mode
	mode := vga.HandleRead(VGA_MODE)
	if mode != VGA_MODE_13H {
		t.Errorf("Mode 13h setup: got mode 0x%02X, want 0x%02X", mode, VGA_MODE_13H)
	}

	// Verify control
	ctrl := vga.HandleRead(VGA_CTRL)
	if ctrl != VGA_CTRL_ENABLE {
		t.Errorf("Mode 13h setup: got ctrl 0x%02X, want 0x%02X", ctrl, VGA_CTRL_ENABLE)
	}

	// Verify palette
	r, g, b := vga.GetPaletteEntry(1)
	if r != 63 || g != 0 || b != 0 {
		t.Errorf("Mode 13h palette: got (%d,%d,%d), want (63,0,0)", r, g, b)
	}
}

// Test that existing PSG I/O still works after VGA changes
func TestZ80_PSG_StillWorks_AfterVGA(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	z80Bus := NewZ80SystemBusWithVGA(bus, vga)

	// Test: PSG register select and write
	z80Bus.Out(Z80_PSG_PORT_SELECT, 0x07) // Mixer register
	z80Bus.Out(Z80_PSG_PORT_DATA, 0x38)   // Tone ABC enabled

	// Verify: PSG register should be written
	value := bus.Read8(PSG_BASE + 0x07)
	if value != 0x38 {
		t.Errorf("PSG register after VGA changes: got 0x%02X, want 0x38", value)
	}
}

// Test that existing PSG/SID still work for 6502 after VGA changes
func Test6502_PSG_StillWorks_AfterVGA(t *testing.T) {
	// Setup
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	adapter := NewMemoryBusAdapter_6502WithVGA(bus, vga)

	// Test: PSG write via 6502
	adapter.Write(C6502_PSG_BASE, 0x42) // First PSG register

	// Verify: PSG register should be written
	value := bus.Read8(PSG_BASE)
	if value != 0x42 {
		t.Errorf("PSG register via 6502 after VGA changes: got 0x%02X, want 0x42", value)
	}
}
