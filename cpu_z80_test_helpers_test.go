package main

import "testing"

type z80TestBus struct {
	mem   [0x10000]byte
	io    [0x10000]byte
	ticks uint64
}

func (b *z80TestBus) Read(addr uint16) byte {
	return b.mem[addr]
}

func (b *z80TestBus) Write(addr uint16, value byte) {
	b.mem[addr] = value
}

func (b *z80TestBus) In(port uint16) byte {
	return b.io[port]
}

func (b *z80TestBus) Out(port uint16, value byte) {
	b.io[port] = value
}

func (b *z80TestBus) Tick(cycles int) {
	b.ticks += uint64(cycles)
}

type cpuZ80TestRig struct {
	bus *z80TestBus
	cpu *CPU_Z80
}

func newCPUZ80TestRig() *cpuZ80TestRig {
	bus := &z80TestBus{}
	cpu := NewCPU_Z80(bus)
	return &cpuZ80TestRig{
		bus: bus,
		cpu: cpu,
	}
}

func (r *cpuZ80TestRig) resetAndLoad(start uint16, program []byte) {
	r.bus = &z80TestBus{}
	r.cpu = NewCPU_Z80(r.bus)
	for i, value := range program {
		r.bus.mem[start+uint16(i)] = value
	}
	r.cpu.PC = start
}

func requireZ80EqualU16(t *testing.T, name string, got, want uint16) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = 0x%04X, want 0x%04X", name, got, want)
	}
}

func requireZ80EqualU8(t *testing.T, name string, got, want byte) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = 0x%02X, want 0x%02X", name, got, want)
	}
}
