package main

import (
	"encoding/binary"
	"testing"
)

func TestIE32InterruptSinkVectorsToHandler(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	const handler = uint32(PROG_START + 0x200)
	bus.Write32(VECTOR_TABLE, handler)
	cpu.PC = PROG_START
	cpu.interruptEnabled.Store(true)

	NewIE32InterruptSink(cpu).Pulse(IntMaskBlitter)

	if cpu.PC != handler {
		t.Fatalf("IE32 interrupt PC = 0x%X, want 0x%X", cpu.PC, handler)
	}
	if !cpu.inInterrupt.Load() {
		t.Fatal("IE32 interrupt did not set inInterrupt")
	}
	if got := bus.Read32(cpu.SP); got != PROG_START {
		t.Fatalf("IE32 pushed return PC = 0x%X, want 0x%X", got, uint32(PROG_START))
	}
}

func TestIE64InterruptSinkVectorsToLegacyHandler(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	const handler = uint64(PROG_START + 0x400)
	cpu.PC = PROG_START
	cpu.interruptVector = handler
	cpu.interruptEnabled.Store(true)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)

	if cpu.PC != handler {
		t.Fatalf("IE64 interrupt PC = 0x%X, want 0x%X", cpu.PC, handler)
	}
	if !cpu.inInterrupt.Load() {
		t.Fatal("IE64 interrupt did not set inInterrupt")
	}
	if got := binary.LittleEndian.Uint64(bus.memory[cpu.regs[31]:]); got != PROG_START {
		t.Fatalf("IE64 pushed return PC = 0x%X, want 0x%X", got, uint64(PROG_START))
	}
}

func TestCPU6502InterruptSinkAssertsIRQ(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)

	NewCPU6502InterruptSink(cpu).Pulse(IntMaskBlitter)

	if !cpu.irqPending.Load() {
		t.Fatal("6502 IRQ was not asserted")
	}
}

func TestLevelTriggered_HeldUntilAck(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	sink := NewCPU6502InterruptSink(cpu)

	var level LevelTriggeredInterruptSink = sink
	level.Assert(IntMaskBlitter)
	if !cpu.irqPending.Load() {
		t.Fatal("Assert did not raise IRQ")
	}

	cpu.SetIRQLine(false)
	level.Ack(IntMaskBlitter)
	if !cpu.irqPending.Load() {
		t.Fatal("Ack did not reassert held level interrupt")
	}

	level.Deassert(IntMaskBlitter)
	cpu.SetIRQLine(false)
	level.Ack(IntMaskBlitter)
	if cpu.irqPending.Load() {
		t.Fatal("Ack reasserted after Deassert")
	}
}

func TestLevelTriggered_Mask(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	sink := NewCPU6502InterruptSink(cpu)

	var level LevelTriggeredInterruptSink = sink
	level.SetMask(IntMaskBlitter, true)
	level.Assert(IntMaskBlitter)
	if cpu.irqPending.Load() {
		t.Fatal("masked Assert raised IRQ")
	}

	level.SetMask(IntMaskBlitter, false)
	if !cpu.irqPending.Load() {
		t.Fatal("unmask did not raise held interrupt")
	}
}

func TestLevelTriggered_PreservesOtherSourcesOnAckDeassertMask(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	sink := NewCPU6502InterruptSink(cpu)
	var level LevelTriggeredInterruptSink = sink

	level.Assert(IntMaskVBI)
	level.Assert(IntMaskBlitter)
	cpu.SetIRQLine(false)
	level.Ack(IntMaskBlitter)
	if !cpu.irqPending.Load() {
		t.Fatal("Ack dropped another active interrupt source")
	}

	cpu.SetIRQLine(false)
	level.Deassert(IntMaskBlitter)
	if !cpu.irqPending.Load() {
		t.Fatal("Deassert dropped another active interrupt source")
	}

	cpu.SetIRQLine(false)
	level.SetMask(IntMaskBlitter, true)
	if !cpu.irqPending.Load() {
		t.Fatal("Masking one source dropped another active interrupt source")
	}

	cpu.SetIRQLine(false)
	level.SetMask(IntMaskVBI, true)
	if cpu.irqPending.Load() {
		t.Fatal("all active sources masked but IRQ remained asserted")
	}
}

func TestPulseSink_StillWorks(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	var sink InterruptSink = NewCPU6502InterruptSink(cpu)

	sink.Pulse(IntMaskBlitter)
	if !cpu.irqPending.Load() {
		t.Fatal("Pulse did not preserve edge-compatible IRQ behavior")
	}
}

func TestNewVideoInterruptSinksNilSafe(t *testing.T) {
	NewIE32InterruptSink(nil).Pulse(IntMaskBlitter)
	NewIE64InterruptSink(nil).Pulse(IntMaskBlitter)
	NewCPU6502InterruptSink(nil).Pulse(IntMaskBlitter)
	var ie32 *IE32InterruptSink
	var ie64 *IE64InterruptSink
	var c6502 *CPU6502InterruptSink
	ie32.Pulse(IntMaskBlitter)
	ie64.Pulse(IntMaskBlitter)
	c6502.Pulse(IntMaskBlitter)
}
