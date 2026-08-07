package main

import (
	"fmt"
	"runtime"
	"testing"
)

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
	cpu *z80ParityTestCPU
}

type z80ParityTestCPU struct {
	*CPU_Z80
	rig *cpuZ80TestRig
}

type z80ParityBus struct {
	ram   *[0x10000]byte
	io    [0x10000]byte
	ticks uint64
}

func (b *z80ParityBus) Read(addr uint16) byte         { return b.ram[addr] }
func (b *z80ParityBus) Write(addr uint16, value byte) { b.ram[addr] = value }
func (b *z80ParityBus) In(port uint16) byte           { return b.io[port] }
func (b *z80ParityBus) Out(port uint16, value byte)   { b.io[port] = value }
func (b *z80ParityBus) Tick(cycles int)               { b.ticks += uint64(cycles) }

func cloneZ80ArchitecturalState(dst, src *CPU_Z80) {
	dst.A, dst.F, dst.B, dst.C = src.A, src.F, src.B, src.C
	dst.D, dst.E, dst.H, dst.L = src.D, src.E, src.H, src.L
	dst.A2, dst.F2, dst.B2, dst.C2 = src.A2, src.F2, src.B2, src.C2
	dst.D2, dst.E2, dst.H2, dst.L2 = src.D2, src.E2, src.H2, src.L2
	dst.IX, dst.IY, dst.SP, dst.PC = src.IX, src.IY, src.SP, src.PC
	dst.I, dst.R, dst.IM, dst.WZ = src.I, src.R, src.IM, src.WZ
	dst.IFF1, dst.IFF2, dst.Halted = src.IFF1, src.IFF2, src.Halted
	dst.Cycles, dst.iffDelay, dst.irqServices = src.Cycles, src.iffDelay, src.irqServices
	dst.irqLine.Store(src.irqLine.Load())
	dst.nmiLine.Store(src.nmiLine.Load())
	dst.nmiPending.Store(src.nmiPending.Load())
	dst.irqVector.Store(src.irqVector.Load())
}

func z80ArchitecturalMismatch(a, b *CPU_Z80) string {
	if a.AF() != b.AF() || a.BC() != b.BC() || a.DE() != b.DE() || a.HL() != b.HL() ||
		a.A2 != b.A2 || a.F2 != b.F2 || a.B2 != b.B2 || a.C2 != b.C2 ||
		a.D2 != b.D2 || a.E2 != b.E2 || a.H2 != b.H2 || a.L2 != b.L2 ||
		a.IX != b.IX || a.IY != b.IY || a.SP != b.SP || a.PC != b.PC ||
		a.I != b.I || a.R != b.R || a.IM != b.IM || a.WZ != b.WZ ||
		a.IFF1 != b.IFF1 || a.IFF2 != b.IFF2 || a.Halted != b.Halted ||
		a.Cycles != b.Cycles || a.iffDelay != b.iffDelay || a.irqServices != b.irqServices ||
		a.irqLine.Load() != b.irqLine.Load() || a.nmiLine.Load() != b.nmiLine.Load() ||
		a.nmiPending.Load() != b.nmiPending.Load() || a.irqVector.Load() != b.irqVector.Load() {
		return fmt.Sprintf("interpreter PC=%04x AF=%04x BC=%04x DE=%04x HL=%04x IX=%04x IY=%04x SP=%04x F'=%02x IFF=%v/%v IM=%d R=%02x WZ=%04x cycles=%d; JIT PC=%04x AF=%04x BC=%04x DE=%04x HL=%04x IX=%04x IY=%04x SP=%04x F'=%02x IFF=%v/%v IM=%d R=%02x WZ=%04x cycles=%d",
			a.PC, a.AF(), a.BC(), a.DE(), a.HL(), a.IX, a.IY, a.SP, a.F2, a.IFF1, a.IFF2, a.IM, a.R, a.WZ, a.Cycles,
			b.PC, b.AF(), b.BC(), b.DE(), b.HL(), b.IX, b.IY, b.SP, b.F2, b.IFF1, b.IFF2, b.IM, b.R, b.WZ, b.Cycles)
	}
	return ""
}

func (c *z80ParityTestCPU) Step() {
	if !z80JitAvailable || (runtime.GOOS != "linux" && runtime.GOOS != "js") {
		c.CPU_Z80.Step()
		return
	}
	machine, err := NewMachineBusSized(0x10000)
	if err != nil {
		panic(err)
	}
	copy(machine.GetMemory(), c.rig.bus.mem[:])
	backing := (*[0x10000]byte)(machine.GetMemory())
	parityBus := &z80ParityBus{ram: backing, io: c.rig.bus.io, ticks: c.rig.bus.ticks}
	adapter := newZ80PlaybackAdapter(machine, parityBus)
	jitCPU := NewCPU_Z80(adapter)
	cloneZ80ArchitecturalState(jitCPU, c.CPU_Z80)
	jitCPU.jitEnabled = true
	jitCPU.jitSingleStep = true
	firstBoundary := true
	jitCPU.executionBoundary = func() {
		if firstBoundary {
			firstBoundary = false
			return
		}
		jitCPU.SetRunning(false)
	}
	jitCPU.SetRunning(true)
	jitCPU.z80JitExecute()

	c.CPU_Z80.Step()
	if mismatch := z80ArchitecturalMismatch(c.CPU_Z80, jitCPU); mismatch != "" {
		panic("pre-existing Z80 test interpreter/JIT mismatch: " + mismatch)
	}
	if c.rig.bus.mem != *backing {
		panic("pre-existing Z80 test interpreter/JIT memory mismatch")
	}
	if c.rig.bus.io != parityBus.io {
		panic("pre-existing Z80 test interpreter/JIT port I/O mismatch")
	}
	if c.rig.bus.ticks != parityBus.ticks {
		panic(fmt.Sprintf("pre-existing Z80 test interpreter/JIT bus ticks mismatch: interpreter=%d JIT=%d", c.rig.bus.ticks, parityBus.ticks))
	}
}

func newCPUZ80TestRig() *cpuZ80TestRig {
	bus := &z80TestBus{}
	cpu := NewCPU_Z80(bus)
	rig := &cpuZ80TestRig{bus: bus}
	rig.cpu = &z80ParityTestCPU{CPU_Z80: cpu, rig: rig}
	return rig
}

func (r *cpuZ80TestRig) resetAndLoad(start uint16, program []byte) {
	r.bus = &z80TestBus{}
	r.cpu = &z80ParityTestCPU{CPU_Z80: NewCPU_Z80(r.bus), rig: r}
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
