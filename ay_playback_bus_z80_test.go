package main

import "testing"

func TestZ80PlaybackAdapterPreservesFlat64KiBMemory(t *testing.T) {
	var ram [0x10000]byte
	ram[0xF000] = 0xA5
	cpu, playback, err := newPlaybackZ80CPU(&ram, ayZXSystemSpectrum, nil)
	if err != nil {
		t.Fatal(err)
	}
	adapter := cpu.bus.(*Z80BusAdapter)

	if got := adapter.Read(0xF000); got != 0xA5 {
		t.Fatalf("Read(F000) = %02X, want flat playback byte A5", got)
	}
	if got := adapter.fetchRead(0xF000); got != 0xA5 {
		t.Fatalf("fetchRead(F000) = %02X, want flat playback byte A5", got)
	}
	beforeGeneration := adapter.mappingGeneration.Load()
	adapter.Write(Z80_BANK1_REG_LO, 0x5A)
	if got := playback.ram[Z80_BANK1_REG_LO]; got != 0x5A {
		t.Fatalf("Write(bank-control address) stored %02X, want flat byte 5A", got)
	}
	if got := adapter.mappingGeneration.Load(); got != beforeGeneration {
		t.Fatalf("flat playback write changed mapping generation: %d -> %d", beforeGeneration, got)
	}
	adapter.Write(Z80_VRAM_BANK_REG, 0x33)
	if playback.ram[Z80_VRAM_BANK_REG] != 0x33 || adapter.bank1Enable || adapter.vramEnabled {
		t.Fatalf("flat playback write activated machine mappings: bank1=%v vram=%v", adapter.bank1Enable, adapter.vramEnabled)
	}
}

func TestZ80PlaybackJITExecutesFlatHighMemory(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 JIT unavailable")
	}
	var ram [0x10000]byte
	copy(ram[0xF000:], []byte{0x3E, 0x42, 0x32, 0x00, 0xF7, 0x76}) // LD A,42; LD (F700),A; HALT
	cpu, playback, err := newPlaybackZ80CPU(&ram, ayZXSystemSpectrum, nil)
	if err != nil {
		t.Fatal(err)
	}
	cpu.PC = 0xF000
	cpu.debugBreakIn = func(uint64) bool { return cpu.Halted }
	cpu.SetRunning(true)
	cpu.z80JitExecute()

	if cpu.A != 0x42 || playback.ram[0xF700] != 0x42 {
		t.Fatalf("flat high-memory execution: A=%02X [F700]=%02X", cpu.A, playback.ram[0xF700])
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("flat high-memory program did not enter the Z80 JIT")
	}
}

type ayZ80TestWriter struct {
	regs [PSG_REG_COUNT]byte
}

func (w *ayZ80TestWriter) WriteRegister(reg uint8, value uint8) {
	if reg < PSG_REG_COUNT {
		w.regs[reg] = value
	}
}

func TestAYZ80BusSpectrumPorts(t *testing.T) {
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemSpectrum, nil)
	bus.Out(0xFFFD, 0x07)
	bus.Out(0xBFFD, 0x55)
	if len(bus.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(bus.writes))
	}
	write := bus.writes[0]
	if write.Reg != 0x07 || write.Value != 0x55 {
		t.Fatalf("unexpected write: %+v", write)
	}
}

func TestAYZ80BusCPCPorts(t *testing.T) {
	// CPC PPI protocol: latch register via Port A, select via Port C 0xC0,
	// latch data via Port A, write via Port C 0x80.
	// Uses OUT (C),r where B=0xF4/0xF6, so port high byte is the PPI chip select.
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemCPC, nil)

	// Select register 3: latch 0x03 to Port A, then control 0xC0 to Port C
	bus.Out(0xF400, 0x03) // Port A: latch register number
	bus.Out(0xF600, 0xC0) // Port C: select register (bits 7:6 = 11)
	// Write value 0x99: latch to Port A, then control 0x80 to Port C
	bus.Out(0xF400, 0x99) // Port A: latch data value
	bus.Out(0xF600, 0x80) // Port C: write data (bits 7:6 = 10)

	if len(bus.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(bus.writes))
	}
	if bus.writes[0].Reg != 0x03 || bus.writes[0].Value != 0x99 {
		t.Fatalf("unexpected write: reg=%d val=0x%02X, want reg=3 val=0x99", bus.writes[0].Reg, bus.writes[0].Value)
	}
}

func TestAYZ80BusCPCPortsOutNA(t *testing.T) {
	// CPC PPI protocol via OUT (n),A — port in low byte (0xF4/0xF6)
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemCPC, nil)

	bus.Out(0x00F4, 0x07) // Port A via low byte: latch register 7
	bus.Out(0x00F6, 0xC0) // Port C via low byte: select register
	bus.Out(0x00F4, 0x55) // Port A: latch data 0x55
	bus.Out(0x00F6, 0x80) // Port C: write data

	if len(bus.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(bus.writes))
	}
	if bus.writes[0].Reg != 0x07 || bus.writes[0].Value != 0x55 {
		t.Fatalf("unexpected write: reg=%d val=0x%02X, want reg=7 val=0x55", bus.writes[0].Reg, bus.writes[0].Value)
	}
}

func TestAYZ80BusCPCPortsMultipleRegs(t *testing.T) {
	// Write to multiple PSG registers via PPI protocol
	var ram [0x10000]byte
	writer := &ayZ80TestWriter{}
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemCPC, writer)

	// Write register 0 = 0xAA
	bus.Out(0xF400, 0x00)
	bus.Out(0xF600, 0xC0)
	bus.Out(0xF400, 0xAA)
	bus.Out(0xF600, 0x80)

	// Write register 7 = 0x38
	bus.Out(0xF400, 0x07)
	bus.Out(0xF600, 0xC0)
	bus.Out(0xF400, 0x38)
	bus.Out(0xF600, 0x80)

	if len(bus.writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(bus.writes))
	}
	if writer.regs[0] != 0xAA {
		t.Fatalf("engine reg 0 not updated: got 0x%02X", writer.regs[0])
	}
	if writer.regs[7] != 0x38 {
		t.Fatalf("engine reg 7 not updated: got 0x%02X", writer.regs[7])
	}
}

func TestAYZ80BusCPCInactiveControl(t *testing.T) {
	// Port C with bits 7:6 = 00 or 01 should be no-ops
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemCPC, nil)

	bus.Out(0xF400, 0x07)
	bus.Out(0xF600, 0x00) // Inactive
	bus.Out(0xF600, 0x40) // Inactive

	if len(bus.writes) != 0 {
		t.Fatalf("expected 0 writes for inactive control, got %d", len(bus.writes))
	}
}

func TestAYZ80BusMSXPorts(t *testing.T) {
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemMSX, nil)
	bus.Out(0x00A0, 0x0D)
	bus.Out(0x00A1, 0x7F)
	if len(bus.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(bus.writes))
	}
	if bus.writes[0].Reg != 0x0D || bus.writes[0].Value != 0x7F {
		t.Fatalf("unexpected write: %+v", bus.writes[0])
	}
}

func TestAYZ80BusIgnoresUnknownPorts(t *testing.T) {
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemSpectrum, nil)
	bus.Out(0x1234, 0x01)
	bus.Out(0x5678, 0x02)
	if len(bus.writes) != 0 {
		t.Fatalf("expected 0 writes, got %d", len(bus.writes))
	}
}

func TestAYZ80BusSpectrumMaskedPorts(t *testing.T) {
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemSpectrum, nil)
	bus.Out(0xC0FD, 0x0A)
	bus.Out(0x80FD, 0x66)
	if len(bus.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(bus.writes))
	}
	if bus.writes[0].Reg != 0x0A || bus.writes[0].Value != 0x66 {
		t.Fatalf("unexpected write: %+v", bus.writes[0])
	}
}

func TestAYZ80BusPSGEngineIntegration(t *testing.T) {
	var ram [0x10000]byte
	writer := &ayZ80TestWriter{}
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemSpectrum, writer)
	bus.Out(0xFFFD, 0x02)
	bus.Out(0xBFFD, 0xAA)
	if writer.regs[2] != 0xAA {
		t.Fatalf("engine register not updated")
	}
}

// BenchmarkAYZ80_IsSelectPort_Spectrum benchmarks Spectrum port matching
func BenchmarkAYZ80_IsSelectPort_Spectrum(b *testing.B) {
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemSpectrum, nil)

	ports := []uint16{0xFFFD, 0xC0FD, 0x8000, 0x1234, 0xBFFD}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		port := ports[i%len(ports)]
		_ = bus.isAYSelectPort(port)
	}
}

// BenchmarkAYZ80_PPIPort_CPC benchmarks CPC PPI port identification
func BenchmarkAYZ80_PPIPort_CPC(b *testing.B) {
	ports := []uint16{0xF400, 0xF600, 0x00F4, 0x00F6, 0x1234}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		port := ports[i%len(ports)]
		_ = cpcPPIPort(port)
	}
}

// BenchmarkAYZ80_IsSelectPort_MSX benchmarks MSX port matching
func BenchmarkAYZ80_IsSelectPort_MSX(b *testing.B) {
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemMSX, nil)

	ports := []uint16{0xA0, 0x00A0, 0xA1, 0x00A1, 0x1234}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		port := ports[i%len(ports)]
		_ = bus.isAYSelectPort(port)
	}
}

// BenchmarkAYZ80_Out_Spectrum benchmarks Spectrum OUT instruction
func BenchmarkAYZ80_Out_Spectrum(b *testing.B) {
	var ram [0x10000]byte
	writer := &ayZ80TestWriter{}
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemSpectrum, writer)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bus.writes = bus.writes[:0] // Reset writes without allocation
		bus.Out(0xFFFD, byte(i))
		bus.Out(0xBFFD, byte(i>>8))
	}
}

// BenchmarkAYZ80_Out_CPC benchmarks CPC OUT via PPI protocol (select + write)
func BenchmarkAYZ80_Out_CPC(b *testing.B) {
	var ram [0x10000]byte
	writer := &ayZ80TestWriter{}
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemCPC, writer)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bus.writes = bus.writes[:0]
		bus.Out(0xF400, byte(i&0x0F)) // Latch register
		bus.Out(0xF600, 0xC0)         // Select
		bus.Out(0xF400, byte(i>>8))   // Latch data
		bus.Out(0xF600, 0x80)         // Write
	}
}
