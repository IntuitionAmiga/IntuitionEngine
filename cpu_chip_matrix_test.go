// cpu_chip_matrix_test.go - CPU/Chip I/O Access Verification Tests
//
// Comprehensive test suite to verify all 4 CPUs can correctly access
// all graphics chips, sound chips, and related I/O.

package main

import (
	"testing"
)

// =============================================================================
// 6502 CPU Tests
// =============================================================================

func Test6502_PSG_Access(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewBus6502Adapter(bus)

	// Test write to PSG register via 6502 address space ($D400)
	adapter.Write(0xD400, 0xAB) // PSG_A_FREQ_LO

	// Verify value is written to actual PSG address
	got := bus.Read8(PSG_BASE)
	if got != 0xAB {
		t.Errorf("6502 PSG write: got 0x%02X, want 0xAB", got)
	}

	// Test read back via 6502
	if adapter.Read(0xD400) != 0xAB {
		t.Errorf("6502 PSG read: got 0x%02X, want 0xAB", adapter.Read(0xD400))
	}
}

func Test6502_SID_Access(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewBus6502Adapter(bus)

	// Test write to SID register via 6502 address space ($D500)
	adapter.Write(0xD500, 0x12) // SID_V1_FREQ_LO

	// Verify value is written to actual SID address
	got := bus.Read8(SID_BASE)
	if got != 0x12 {
		t.Errorf("6502 SID write: got 0x%02X, want 0x12", got)
	}

	// Test read back via 6502
	if adapter.Read(0xD500) != 0x12 {
		t.Errorf("6502 SID read: got 0x%02X, want 0x12", adapter.Read(0xD500))
	}
}

func Test6502_POKEY_Access(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewBus6502Adapter(bus)

	// Test write to POKEY register via 6502 address space ($D200)
	adapter.Write(0xD200, 0x34) // POKEY_AUDF1

	// Verify value is written to actual POKEY address
	got := bus.Read8(POKEY_BASE)
	if got != 0x34 {
		t.Errorf("6502 POKEY write: got 0x%02X, want 0x34", got)
	}

	// Test read back via 6502
	if adapter.Read(0xD200) != 0x34 {
		t.Errorf("6502 POKEY read: got 0x%02X, want 0x34", adapter.Read(0xD200))
	}

	// Test AUDCTL register
	adapter.Write(0xD208, 0x55) // POKEY_AUDCTL
	if bus.Read8(POKEY_BASE+8) != 0x55 {
		t.Errorf("6502 POKEY AUDCTL: got 0x%02X, want 0x55", bus.Read8(POKEY_BASE+8))
	}
}

func Test6502_TED_Access(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewBus6502Adapter(bus)

	// Test write to TED register via 6502 address space ($D600)
	adapter.Write(0xD600, 0x56) // TED_FREQ1_LO

	// Verify value is written to actual TED address
	got := bus.Read8(TED_BASE)
	if got != 0x56 {
		t.Errorf("6502 TED write: got 0x%02X, want 0x56", got)
	}

	// Test read back via 6502
	if adapter.Read(0xD600) != 0x56 {
		t.Errorf("6502 TED read: got 0x%02X, want 0x56", adapter.Read(0xD600))
	}

	// Test control register
	adapter.Write(0xD603, 0x78) // TED_SND_CTRL
	if bus.Read8(TED_BASE+3) != 0x78 {
		t.Errorf("6502 TED SND_CTRL: got 0x%02X, want 0x78", bus.Read8(TED_BASE+3))
	}
}

func Test6502_ULA_Access(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewBus6502Adapter(bus)

	// Test write to ULA border register via 6502 address space ($D800)
	adapter.Write(0xD800, 0x03) // ULA_BORDER

	// Verify value is written to actual ULA address
	got := bus.Read8(ULA_BASE)
	if got != 0x03 {
		t.Errorf("6502 ULA write: got 0x%02X, want 0x03", got)
	}

	// Test read back via 6502
	if adapter.Read(0xD800) != 0x03 {
		t.Errorf("6502 ULA read: got 0x%02X, want 0x03", adapter.Read(0xD800))
	}

	// Test control register
	adapter.Write(0xD804, ULA_CTRL_ENABLE) // ULA_CTRL
	if bus.Read8(ULA_CTRL) != ULA_CTRL_ENABLE {
		t.Errorf("6502 ULA CTRL: got 0x%02X, want 0x%02X", bus.Read8(ULA_CTRL), ULA_CTRL_ENABLE)
	}
}

// =============================================================================
// Z80 CPU Tests (Port I/O)
// =============================================================================

func TestZ80_PSG_PortIO(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)

	// Select PSG register 0 and write value
	z80Bus.Out(Z80_PSG_PORT_SELECT, 0)
	z80Bus.Out(Z80_PSG_PORT_DATA, 0x9A)

	// Verify value is written to PSG_BASE
	got := bus.Read8(PSG_BASE)
	if got != 0x9A {
		t.Errorf("Z80 PSG port write: got 0x%02X, want 0x9A", got)
	}

	// Test read back via port I/O
	z80Bus.Out(Z80_PSG_PORT_SELECT, 0)
	if z80Bus.In(Z80_PSG_PORT_DATA) != 0x9A {
		t.Errorf("Z80 PSG port read: got 0x%02X, want 0x9A", z80Bus.In(Z80_PSG_PORT_DATA))
	}
}

func TestZ80_SID_PortIO(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)

	// Select SID register 0 and write value
	z80Bus.Out(Z80_SID_PORT_SELECT, 0)
	z80Bus.Out(Z80_SID_PORT_DATA, 0xBC)

	// Verify value is written to SID_BASE
	got := bus.Read8(SID_BASE)
	if got != 0xBC {
		t.Errorf("Z80 SID port write: got 0x%02X, want 0xBC", got)
	}

	// Test read back via port I/O
	z80Bus.Out(Z80_SID_PORT_SELECT, 0)
	if z80Bus.In(Z80_SID_PORT_DATA) != 0xBC {
		t.Errorf("Z80 SID port read: got 0x%02X, want 0xBC", z80Bus.In(Z80_SID_PORT_DATA))
	}
}

func TestZ80_POKEY_PortIO(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)

	// Select POKEY register 0 and write value
	z80Bus.Out(Z80_POKEY_PORT_SELECT, 0)
	z80Bus.Out(Z80_POKEY_PORT_DATA, 0xDE)

	// Verify value is written to POKEY_BASE
	got := bus.Read8(POKEY_BASE)
	if got != 0xDE {
		t.Errorf("Z80 POKEY port write: got 0x%02X, want 0xDE", got)
	}

	// Test read back via port I/O
	z80Bus.Out(Z80_POKEY_PORT_SELECT, 0)
	if z80Bus.In(Z80_POKEY_PORT_DATA) != 0xDE {
		t.Errorf("Z80 POKEY port read: got 0x%02X, want 0xDE", z80Bus.In(Z80_POKEY_PORT_DATA))
	}
}

func TestZ80_TED_PortIO(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)

	// Select TED register 0 and write value
	z80Bus.Out(Z80_TED_PORT_SELECT, 0)
	z80Bus.Out(Z80_TED_PORT_DATA, 0xEF)

	// Verify value is written to TED_BASE
	got := bus.Read8(TED_BASE)
	if got != 0xEF {
		t.Errorf("Z80 TED port write: got 0x%02X, want 0xEF", got)
	}

	// Test read back via port I/O
	z80Bus.Out(Z80_TED_PORT_SELECT, 0)
	if z80Bus.In(Z80_TED_PORT_DATA) != 0xEF {
		t.Errorf("Z80 TED port read: got 0x%02X, want 0xEF", z80Bus.In(Z80_TED_PORT_DATA))
	}
}

func TestZ80_ULA_PortIO(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)

	// Write border color via port 0xFE (bits 0-2 only)
	z80Bus.Out(Z80_ULA_PORT, 0x05) // Border color 5 (cyan)

	// Verify value is written to ULA_BORDER (masked to 3 bits)
	got := bus.Read8(ULA_BORDER)
	if got != 0x05 {
		t.Errorf("Z80 ULA port write: got 0x%02X, want 0x05", got)
	}

	// Test read back via port I/O (should return border color)
	if z80Bus.In(Z80_ULA_PORT) != 0x05 {
		t.Errorf("Z80 ULA port read: got 0x%02X, want 0x05", z80Bus.In(Z80_ULA_PORT))
	}
}

// =============================================================================
// M68K CPU Tests (Direct 32-bit access)
// =============================================================================

func TestM68K_PSG_Access(t *testing.T) {
	bus := NewMachineBus()

	// M68K accesses PSG directly at $F0C00
	bus.Write8(PSG_BASE, 0x11)

	got := bus.Read8(PSG_BASE)
	if got != 0x11 {
		t.Errorf("M68K PSG access: got 0x%02X, want 0x11", got)
	}
}

func TestM68K_SID_Access(t *testing.T) {
	bus := NewMachineBus()

	// M68K accesses SID directly at $F0E00
	bus.Write8(SID_BASE, 0x22)

	got := bus.Read8(SID_BASE)
	if got != 0x22 {
		t.Errorf("M68K SID access: got 0x%02X, want 0x22", got)
	}
}

func TestM68K_POKEY_Access(t *testing.T) {
	bus := NewMachineBus()

	// M68K accesses POKEY directly at $F0D00
	bus.Write8(POKEY_BASE, 0x33)

	got := bus.Read8(POKEY_BASE)
	if got != 0x33 {
		t.Errorf("M68K POKEY access: got 0x%02X, want 0x33", got)
	}
}

func TestM68K_TED_Access(t *testing.T) {
	bus := NewMachineBus()

	// M68K accesses TED directly at $F0F00
	bus.Write8(TED_BASE, 0x44)

	got := bus.Read8(TED_BASE)
	if got != 0x44 {
		t.Errorf("M68K TED access: got 0x%02X, want 0x44", got)
	}
}

func TestM68K_ULA_Access(t *testing.T) {
	bus := NewMachineBus()

	// M68K accesses ULA directly at $F2000
	bus.Write8(ULA_BASE, 0x55)

	got := bus.Read8(ULA_BASE)
	if got != 0x55 {
		t.Errorf("M68K ULA access: got 0x%02X, want 0x55", got)
	}
}

// =============================================================================
// Address Constant Verification Tests
// =============================================================================

func TestChipAddressConstants(t *testing.T) {
	// Verify 6502 address mappings
	if C6502_PSG_BASE != 0xD400 {
		t.Errorf("C6502_PSG_BASE = 0x%X, want 0xD400", C6502_PSG_BASE)
	}
	if C6502_SID_BASE != 0xD500 {
		t.Errorf("C6502_SID_BASE = 0x%X, want 0xD500", C6502_SID_BASE)
	}
	if C6502_POKEY_BASE != 0xD200 {
		t.Errorf("C6502_POKEY_BASE = 0x%X, want 0xD200", C6502_POKEY_BASE)
	}
	if C6502_TED_BASE != 0xD600 {
		t.Errorf("C6502_TED_BASE = 0x%X, want 0xD600", C6502_TED_BASE)
	}
	if C6502_ULA_BASE != 0xD800 {
		t.Errorf("C6502_ULA_BASE = 0x%X, want 0xD800", C6502_ULA_BASE)
	}

	// Verify Z80 port mappings
	if Z80_PSG_PORT_SELECT != 0xF0 {
		t.Errorf("Z80_PSG_PORT_SELECT = 0x%X, want 0xF0", Z80_PSG_PORT_SELECT)
	}
	if Z80_SID_PORT_SELECT != 0xE0 {
		t.Errorf("Z80_SID_PORT_SELECT = 0x%X, want 0xE0", Z80_SID_PORT_SELECT)
	}
	if Z80_POKEY_PORT_SELECT != 0xD0 {
		t.Errorf("Z80_POKEY_PORT_SELECT = 0x%X, want 0xD0", Z80_POKEY_PORT_SELECT)
	}
	if Z80_TED_PORT_SELECT != 0xF2 {
		t.Errorf("Z80_TED_PORT_SELECT = 0x%X, want 0xF2", Z80_TED_PORT_SELECT)
	}
	if Z80_ULA_PORT != 0xFE {
		t.Errorf("Z80_ULA_PORT = 0x%X, want 0xFE", Z80_ULA_PORT)
	}

	// Verify IE32/M68K base addresses
	if PSG_BASE != 0xF0C00 {
		t.Errorf("PSG_BASE = 0x%X, want 0xF0C00", PSG_BASE)
	}
	if SID_BASE != 0xF0E00 {
		t.Errorf("SID_BASE = 0x%X, want 0xF0E00", SID_BASE)
	}
	if POKEY_BASE != 0xF0D00 {
		t.Errorf("POKEY_BASE = 0x%X, want 0xF0D00", POKEY_BASE)
	}
	if TED_BASE != 0xF0F00 {
		t.Errorf("TED_BASE = 0x%X, want 0xF0F00", TED_BASE)
	}
	if ULA_BASE != 0xF2000 {
		t.Errorf("ULA_BASE = 0x%X, want 0xF2000", ULA_BASE)
	}
}

// =============================================================================
// Cross-CPU Consistency Tests
// =============================================================================

func TestCrossCPU_PSG_Consistency(t *testing.T) {
	bus := NewMachineBus()
	adapter6502 := NewBus6502Adapter(bus)
	z80Bus := NewZ80BusAdapter(bus)

	// Write via 6502
	adapter6502.Write(0xD400, 0xAA)

	// Read via M68K (direct)
	if bus.Read8(PSG_BASE) != 0xAA {
		t.Errorf("Cross-CPU PSG: M68K read got 0x%02X, want 0xAA", bus.Read8(PSG_BASE))
	}

	// Read via Z80
	z80Bus.Out(Z80_PSG_PORT_SELECT, 0)
	if z80Bus.In(Z80_PSG_PORT_DATA) != 0xAA {
		t.Errorf("Cross-CPU PSG: Z80 read got 0x%02X, want 0xAA", z80Bus.In(Z80_PSG_PORT_DATA))
	}
}

func TestCrossCPU_SID_Consistency(t *testing.T) {
	bus := NewMachineBus()
	adapter6502 := NewBus6502Adapter(bus)
	z80Bus := NewZ80BusAdapter(bus)

	// Write via M68K (direct)
	bus.Write8(SID_BASE, 0xBB)

	// Read via 6502
	if adapter6502.Read(0xD500) != 0xBB {
		t.Errorf("Cross-CPU SID: 6502 read got 0x%02X, want 0xBB", adapter6502.Read(0xD500))
	}

	// Read via Z80
	z80Bus.Out(Z80_SID_PORT_SELECT, 0)
	if z80Bus.In(Z80_SID_PORT_DATA) != 0xBB {
		t.Errorf("Cross-CPU SID: Z80 read got 0x%02X, want 0xBB", z80Bus.In(Z80_SID_PORT_DATA))
	}
}

func TestCrossCPU_POKEY_Consistency(t *testing.T) {
	bus := NewMachineBus()
	adapter6502 := NewBus6502Adapter(bus)
	z80Bus := NewZ80BusAdapter(bus)

	// Write via Z80
	z80Bus.Out(Z80_POKEY_PORT_SELECT, 0)
	z80Bus.Out(Z80_POKEY_PORT_DATA, 0xCC)

	// Read via M68K (direct)
	if bus.Read8(POKEY_BASE) != 0xCC {
		t.Errorf("Cross-CPU POKEY: M68K read got 0x%02X, want 0xCC", bus.Read8(POKEY_BASE))
	}

	// Read via 6502
	if adapter6502.Read(0xD200) != 0xCC {
		t.Errorf("Cross-CPU POKEY: 6502 read got 0x%02X, want 0xCC", adapter6502.Read(0xD200))
	}
}

func TestCrossCPU_TED_Consistency(t *testing.T) {
	bus := NewMachineBus()
	adapter6502 := NewBus6502Adapter(bus)
	z80Bus := NewZ80BusAdapter(bus)

	// Write via 6502
	adapter6502.Write(0xD600, 0xDD)

	// Read via M68K (direct)
	if bus.Read8(TED_BASE) != 0xDD {
		t.Errorf("Cross-CPU TED: M68K read got 0x%02X, want 0xDD", bus.Read8(TED_BASE))
	}

	// Read via Z80
	z80Bus.Out(Z80_TED_PORT_SELECT, 0)
	if z80Bus.In(Z80_TED_PORT_DATA) != 0xDD {
		t.Errorf("Cross-CPU TED: Z80 read got 0x%02X, want 0xDD", z80Bus.In(Z80_TED_PORT_DATA))
	}
}

func TestCrossCPU_ULA_Consistency(t *testing.T) {
	bus := NewMachineBus()
	adapter6502 := NewBus6502Adapter(bus)
	z80Bus := NewZ80BusAdapter(bus)

	// Write via Z80 port
	z80Bus.Out(Z80_ULA_PORT, 0x07) // Max border color

	// Read via M68K (direct)
	if bus.Read8(ULA_BORDER) != 0x07 {
		t.Errorf("Cross-CPU ULA: M68K read got 0x%02X, want 0x07", bus.Read8(ULA_BORDER))
	}

	// Read via 6502
	if adapter6502.Read(0xD800) != 0x07 {
		t.Errorf("Cross-CPU ULA: 6502 read got 0x%02X, want 0x07", adapter6502.Read(0xD800))
	}
}

// =============================================================================
// x86 CPU Tests (Port I/O)
// =============================================================================

func TestX86_PSG_PortIO(t *testing.T) {
	bus := NewMachineBus()
	x86Bus := NewX86BusAdapter(bus)

	// Select PSG register 0 and write value
	x86Bus.Out(X86_PORT_PSG_SELECT, 0)
	x86Bus.Out(X86_PORT_PSG_DATA, 0x9A)

	// Verify value is written to PSG_BASE
	got := bus.Read8(PSG_BASE)
	if got != 0x9A {
		t.Errorf("x86 PSG port write: got 0x%02X, want 0x9A", got)
	}

	// Test read back via port I/O
	x86Bus.Out(X86_PORT_PSG_SELECT, 0)
	if x86Bus.In(X86_PORT_PSG_DATA) != 0x9A {
		t.Errorf("x86 PSG port read: got 0x%02X, want 0x9A", x86Bus.In(X86_PORT_PSG_DATA))
	}
}

func TestX86_SID_PortIO(t *testing.T) {
	bus := NewMachineBus()
	x86Bus := NewX86BusAdapter(bus)

	// Select SID register 0 and write value
	x86Bus.Out(X86_PORT_SID_SELECT, 0)
	x86Bus.Out(X86_PORT_SID_DATA, 0xBC)

	// Verify value is written to SID_BASE
	got := bus.Read8(SID_BASE)
	if got != 0xBC {
		t.Errorf("x86 SID port write: got 0x%02X, want 0xBC", got)
	}

	// Test read back via port I/O
	x86Bus.Out(X86_PORT_SID_SELECT, 0)
	if x86Bus.In(X86_PORT_SID_DATA) != 0xBC {
		t.Errorf("x86 SID port read: got 0x%02X, want 0xBC", x86Bus.In(X86_PORT_SID_DATA))
	}
}

func TestX86_POKEY_PortIO(t *testing.T) {
	bus := NewMachineBus()
	x86Bus := NewX86BusAdapter(bus)

	// Write to POKEY register 0 via direct port mapping (0xD0 = AUDF1)
	x86Bus.Out(X86_PORT_POKEY_BASE, 0xDE)

	// Verify value is written to POKEY_BASE
	got := bus.Read8(POKEY_BASE)
	if got != 0xDE {
		t.Errorf("x86 POKEY port write: got 0x%02X, want 0xDE", got)
	}

	// Test read back via port I/O
	if x86Bus.In(X86_PORT_POKEY_BASE) != 0xDE {
		t.Errorf("x86 POKEY port read: got 0x%02X, want 0xDE", x86Bus.In(X86_PORT_POKEY_BASE))
	}
}

func TestX86_TED_PortIO(t *testing.T) {
	bus := NewMachineBus()
	x86Bus := NewX86BusAdapter(bus)

	// Select TED register 0 and write value
	x86Bus.Out(X86_PORT_TED_SELECT, 0)
	x86Bus.Out(X86_PORT_TED_DATA, 0xEF)

	// Verify value is written to TED_BASE
	got := bus.Read8(TED_BASE)
	if got != 0xEF {
		t.Errorf("x86 TED port write: got 0x%02X, want 0xEF", got)
	}

	// Test read back via port I/O
	x86Bus.Out(X86_PORT_TED_SELECT, 0)
	if x86Bus.In(X86_PORT_TED_DATA) != 0xEF {
		t.Errorf("x86 TED port read: got 0x%02X, want 0xEF", x86Bus.In(X86_PORT_TED_DATA))
	}
}

func TestX86_ULA_PortIO(t *testing.T) {
	bus := NewMachineBus()
	x86Bus := NewX86BusAdapter(bus)

	// Write border color via port 0xFE (same as Z80)
	x86Bus.Out(Z80_ULA_PORT, 0x05)

	// Verify value is written to ULA_BORDER
	got := bus.Read8(ULA_BORDER)
	if got != 0x05 {
		t.Errorf("x86 ULA port write: got 0x%02X, want 0x05", got)
	}

	// Test read back via port I/O
	if x86Bus.In(Z80_ULA_PORT) != 0x05 {
		t.Errorf("x86 ULA port read: got 0x%02X, want 0x05", x86Bus.In(Z80_ULA_PORT))
	}
}

func TestX86_ANTIC_PortIO(t *testing.T) {
	bus := NewMachineBus()
	x86Bus := NewX86BusAdapter(bus)

	// Select ANTIC register 0 (DMACTL) and write value
	x86Bus.Out(X86_PORT_ANTIC_SELECT, 0)
	x86Bus.Out(X86_PORT_ANTIC_DATA, 0x22)

	// Verify value is written to ANTIC_BASE (4-byte aligned)
	got := bus.Read8(ANTIC_BASE)
	if got != 0x22 {
		t.Errorf("x86 ANTIC port write: got 0x%02X, want 0x22", got)
	}

	// Test read back via port I/O
	x86Bus.Out(X86_PORT_ANTIC_SELECT, 0)
	if x86Bus.In(X86_PORT_ANTIC_DATA) != 0x22 {
		t.Errorf("x86 ANTIC port read: got 0x%02X, want 0x22", x86Bus.In(X86_PORT_ANTIC_DATA))
	}
}

func TestX86_GTIA_PortIO(t *testing.T) {
	bus := NewMachineBus()
	x86Bus := NewX86BusAdapter(bus)

	// Select GTIA register 0 (COLPF0) and write value
	x86Bus.Out(X86_PORT_GTIA_SELECT, 0)
	x86Bus.Out(X86_PORT_GTIA_DATA, 0x44)

	// Verify value is written to GTIA_BASE (4-byte aligned)
	got := bus.Read8(GTIA_BASE)
	if got != 0x44 {
		t.Errorf("x86 GTIA port write: got 0x%02X, want 0x44", got)
	}

	// Test read back via port I/O
	x86Bus.Out(X86_PORT_GTIA_SELECT, 0)
	if x86Bus.In(X86_PORT_GTIA_DATA) != 0x44 {
		t.Errorf("x86 GTIA port read: got 0x%02X, want 0x44", x86Bus.In(X86_PORT_GTIA_DATA))
	}
}

// =============================================================================
// Z80 ANTIC/GTIA Tests
// =============================================================================

func TestZ80_ANTIC_PortIO(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)

	// Select ANTIC register 0 (DMACTL) and write value
	z80Bus.Out(Z80_ANTIC_PORT_SELECT, 0)
	z80Bus.Out(Z80_ANTIC_PORT_DATA, 0x33)

	// Verify value is written to ANTIC_BASE (4-byte aligned)
	got := bus.Read8(ANTIC_BASE)
	if got != 0x33 {
		t.Errorf("Z80 ANTIC port write: got 0x%02X, want 0x33", got)
	}

	// Test read back via port I/O
	z80Bus.Out(Z80_ANTIC_PORT_SELECT, 0)
	if z80Bus.In(Z80_ANTIC_PORT_DATA) != 0x33 {
		t.Errorf("Z80 ANTIC port read: got 0x%02X, want 0x33", z80Bus.In(Z80_ANTIC_PORT_DATA))
	}
}

func TestZ80_GTIA_PortIO(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)

	// Select GTIA register 0 (COLPF0) and write value
	z80Bus.Out(Z80_GTIA_PORT_SELECT, 0)
	z80Bus.Out(Z80_GTIA_PORT_DATA, 0x55)

	// Verify value is written to GTIA_BASE (4-byte aligned)
	got := bus.Read8(GTIA_BASE)
	if got != 0x55 {
		t.Errorf("Z80 GTIA port write: got 0x%02X, want 0x55", got)
	}

	// Test read back via port I/O
	z80Bus.Out(Z80_GTIA_PORT_SELECT, 0)
	if z80Bus.In(Z80_GTIA_PORT_DATA) != 0x55 {
		t.Errorf("Z80 GTIA port read: got 0x%02X, want 0x55", z80Bus.In(Z80_GTIA_PORT_DATA))
	}
}

// =============================================================================
// Cross-CPU Tests including x86 and ANTIC/GTIA
// =============================================================================

func TestCrossCPU_PSG_WithX86(t *testing.T) {
	bus := NewMachineBus()
	adapter6502 := NewBus6502Adapter(bus)
	z80Bus := NewZ80BusAdapter(bus)
	x86Bus := NewX86BusAdapter(bus)

	// Write via x86
	x86Bus.Out(X86_PORT_PSG_SELECT, 0)
	x86Bus.Out(X86_PORT_PSG_DATA, 0x77)

	// Read via M68K (direct)
	if bus.Read8(PSG_BASE) != 0x77 {
		t.Errorf("Cross-CPU PSG with x86: M68K read got 0x%02X, want 0x77", bus.Read8(PSG_BASE))
	}

	// Read via 6502
	if adapter6502.Read(0xD400) != 0x77 {
		t.Errorf("Cross-CPU PSG with x86: 6502 read got 0x%02X, want 0x77", adapter6502.Read(0xD400))
	}

	// Read via Z80
	z80Bus.Out(Z80_PSG_PORT_SELECT, 0)
	if z80Bus.In(Z80_PSG_PORT_DATA) != 0x77 {
		t.Errorf("Cross-CPU PSG with x86: Z80 read got 0x%02X, want 0x77", z80Bus.In(Z80_PSG_PORT_DATA))
	}
}

func TestCrossCPU_ANTIC_AllCPUs(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)
	x86Bus := NewX86BusAdapter(bus)

	// Note: 6502 has PSG at $D400 which conflicts with ANTIC's authentic Atari address
	// So we test cross-CPU with M68K, Z80, and x86 only

	// Write via M68K (direct - 4-byte aligned)
	bus.Write8(ANTIC_BASE, 0x22)

	// Verify value at ANTIC_BASE
	if bus.Read8(ANTIC_BASE) != 0x22 {
		t.Errorf("Cross-CPU ANTIC: M68K read got 0x%02X, want 0x22", bus.Read8(ANTIC_BASE))
	}

	// Read via Z80
	z80Bus.Out(Z80_ANTIC_PORT_SELECT, 0)
	if z80Bus.In(Z80_ANTIC_PORT_DATA) != 0x22 {
		t.Errorf("Cross-CPU ANTIC: Z80 read got 0x%02X, want 0x22", z80Bus.In(Z80_ANTIC_PORT_DATA))
	}

	// Read via x86
	x86Bus.Out(X86_PORT_ANTIC_SELECT, 0)
	if x86Bus.In(X86_PORT_ANTIC_DATA) != 0x22 {
		t.Errorf("Cross-CPU ANTIC: x86 read got 0x%02X, want 0x22", x86Bus.In(X86_PORT_ANTIC_DATA))
	}
}

func TestCrossCPU_GTIA_AllCPUs(t *testing.T) {
	bus := NewMachineBus()
	z80Bus := NewZ80BusAdapter(bus)
	x86Bus := NewX86BusAdapter(bus)

	// Write via Z80
	z80Bus.Out(Z80_GTIA_PORT_SELECT, 0)
	z80Bus.Out(Z80_GTIA_PORT_DATA, 0x88)

	// Read via M68K (direct - 4-byte aligned)
	if bus.Read8(GTIA_BASE) != 0x88 {
		t.Errorf("Cross-CPU GTIA: M68K read got 0x%02X, want 0x88", bus.Read8(GTIA_BASE))
	}

	// Read via x86
	x86Bus.Out(X86_PORT_GTIA_SELECT, 0)
	if x86Bus.In(X86_PORT_GTIA_DATA) != 0x88 {
		t.Errorf("Cross-CPU GTIA: x86 read got 0x%02X, want 0x88", x86Bus.In(X86_PORT_GTIA_DATA))
	}
}

func TestCrossCPU_ULA_WithX86(t *testing.T) {
	bus := NewMachineBus()
	adapter6502 := NewBus6502Adapter(bus)
	z80Bus := NewZ80BusAdapter(bus)
	x86Bus := NewX86BusAdapter(bus)

	// Write via x86
	x86Bus.Out(Z80_ULA_PORT, 0x06)

	// Read via M68K (direct)
	if bus.Read8(ULA_BORDER) != 0x06 {
		t.Errorf("Cross-CPU ULA with x86: M68K read got 0x%02X, want 0x06", bus.Read8(ULA_BORDER))
	}

	// Read via 6502
	if adapter6502.Read(0xD800) != 0x06 {
		t.Errorf("Cross-CPU ULA with x86: 6502 read got 0x%02X, want 0x06", adapter6502.Read(0xD800))
	}

	// Read via Z80
	if z80Bus.In(Z80_ULA_PORT) != 0x06 {
		t.Errorf("Cross-CPU ULA with x86: Z80 read got 0x%02X, want 0x06", z80Bus.In(Z80_ULA_PORT))
	}
}
