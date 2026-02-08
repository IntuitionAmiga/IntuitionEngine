package main

import "testing"

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
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemCPC, nil)
	bus.Out(0x12F4, 0x03)
	bus.Out(0x34F6, 0x99)
	if len(bus.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(bus.writes))
	}
	if bus.writes[0].Reg != 0x03 || bus.writes[0].Value != 0x99 {
		t.Fatalf("unexpected write: %+v", bus.writes[0])
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

// BenchmarkAYZ80_IsSelectPort_CPC benchmarks CPC port matching
func BenchmarkAYZ80_IsSelectPort_CPC(b *testing.B) {
	var ram [0x10000]byte
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemCPC, nil)

	ports := []uint16{0xF4, 0x12F4, 0xF6, 0x34F6, 0x1234}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		port := ports[i%len(ports)]
		_ = bus.isAYSelectPort(port)
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

// BenchmarkAYZ80_Out_CPC benchmarks CPC OUT instruction
func BenchmarkAYZ80_Out_CPC(b *testing.B) {
	var ram [0x10000]byte
	writer := &ayZ80TestWriter{}
	bus := newAyPlaybackBusZ80(&ram, ayZXSystemCPC, writer)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bus.writes = bus.writes[:0]
		bus.Out(0xF4, byte(i))
		bus.Out(0xF6, byte(i>>8))
	}
}
