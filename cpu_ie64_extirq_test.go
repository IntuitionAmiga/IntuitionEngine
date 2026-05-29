// cpu_ie64_extirq_test.go - IE64 external interrupt pending/delivery tests

package main

import (
	"encoding/binary"
	"testing"
	"time"
)

// TestExternalIRQ_DroppedWhenDisabled: an edge pulse raised while interrupts are
// disabled is dropped at record time and never latched.
func TestExternalIRQ_DroppedWhenDisabled(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.interruptEnabled.Store(false)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)

	if cpu.pendingIRQMask.Load() != 0 {
		t.Fatalf("pending mask = 0x%X, want 0 (dropped at record time)", cpu.pendingIRQMask.Load())
	}
	if cpu.deliverPendingExternalInterrupt() {
		t.Fatal("delivered an interrupt that was raised while disabled")
	}
}

// TestExternalIRQ_NotNestedWhenInInterrupt: a pulse raised while an interrupt is
// already in flight is dropped at record time.
func TestExternalIRQ_NotNestedWhenInInterrupt(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.interruptEnabled.Store(true)
	cpu.inInterrupt.Store(true)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)

	if cpu.pendingIRQMask.Load() != 0 {
		t.Fatalf("pending mask = 0x%X, want 0 (dropped while inInterrupt)", cpu.pendingIRQMask.Load())
	}
}

// TestExternalIRQ_RecordedThenDisabled_DroppedAtDelivery pins the delivery-time
// gate independently: record while enabled, then disable before the poll. An
// implementation that omits the delivery-time gate would wrongly deliver here.
func TestExternalIRQ_RecordedThenDisabled_DroppedAtDelivery(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.interruptVector = uint64(PROG_START + 0x400)
	cpu.regs[31] = STACK_START
	cpu.PC = PROG_START
	cpu.interruptEnabled.Store(true)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
	if cpu.pendingIRQMask.Load() == 0 {
		t.Fatal("pulse while enabled should have recorded a pending mask")
	}

	cpu.interruptEnabled.Store(false)
	if cpu.deliverPendingExternalInterrupt() {
		t.Fatal("delivered despite interrupts disabled at delivery time")
	}
	if cpu.PC != PROG_START {
		t.Fatalf("PC changed to 0x%X, want unchanged 0x%X", cpu.PC, uint64(PROG_START))
	}
	if cpu.pendingIRQMask.Load() != 0 {
		t.Fatal("pending mask should be cleared after a dropped delivery")
	}
}

// TestExternalIRQ_RecordedThenInInterrupt_DroppedAtDelivery: same, but the gate
// trips on inInterrupt set between record and poll.
func TestExternalIRQ_RecordedThenInInterrupt_DroppedAtDelivery(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.interruptVector = uint64(PROG_START + 0x400)
	cpu.regs[31] = STACK_START
	cpu.PC = PROG_START
	cpu.interruptEnabled.Store(true)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
	cpu.inInterrupt.Store(true)

	if cpu.deliverPendingExternalInterrupt() {
		t.Fatal("delivered despite inInterrupt at delivery time")
	}
	if cpu.PC != PROG_START {
		t.Fatalf("PC changed to 0x%X, want unchanged", cpu.PC)
	}
}

// TestExternalIRQ_PulseWhileDisabled_EnableBeforePoll_Dropped: the record-time
// gate means a pulse raised while disabled stays dropped even if interrupts are
// enabled before the next poll.
func TestExternalIRQ_PulseWhileDisabled_EnableBeforePoll_Dropped(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.regs[31] = STACK_START
	cpu.PC = PROG_START
	cpu.interruptEnabled.Store(false)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
	cpu.interruptEnabled.Store(true)

	if cpu.deliverPendingExternalInterrupt() {
		t.Fatal("pulse raised while disabled was delivered after enabling")
	}
	if cpu.pendingIRQMask.Load() != 0 {
		t.Fatal("pending mask should be 0 (never recorded)")
	}
}

// TestExternalIRQ_PulseWhileInInterrupt_ClearBeforePoll_Dropped: same shape for
// the inInterrupt gate.
func TestExternalIRQ_PulseWhileInInterrupt_ClearBeforePoll_Dropped(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.regs[31] = STACK_START
	cpu.PC = PROG_START
	cpu.interruptEnabled.Store(true)
	cpu.inInterrupt.Store(true)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
	cpu.inInterrupt.Store(false)

	if cpu.deliverPendingExternalInterrupt() {
		t.Fatal("pulse raised while inInterrupt was delivered after clearing")
	}
}

// TestExternalIRQ_MMUOn_TrapEntry: MMU-on delivery takes a trap frame, records
// the cause in faultAddr, and vectors to intrVector.
func TestExternalIRQ_MMUOn_TrapEntry(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.mmuEnabled = true
	cpu.supervisorMode = true
	cpu.intrVector = uint64(PROG_START + 0x800)
	cpu.PC = PROG_START
	cpu.interruptEnabled.Store(true)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
	if !cpu.deliverPendingExternalInterrupt() {
		t.Fatal("MMU-on external interrupt was not delivered")
	}
	if cpu.faultCause != FAULT_TIMER {
		t.Fatalf("faultCause = %d, want FAULT_TIMER(%d)", cpu.faultCause, FAULT_TIMER)
	}
	if cpu.faultAddr != uint64(IntMaskBlitter) {
		t.Fatalf("faultAddr = 0x%X, want 0x%X (cause mask)", cpu.faultAddr, uint64(IntMaskBlitter))
	}
	if cpu.faultPC != PROG_START {
		t.Fatalf("faultPC = 0x%X, want 0x%X", cpu.faultPC, uint64(PROG_START))
	}
	if cpu.PC != cpu.intrVector {
		t.Fatalf("PC = 0x%X, want intrVector 0x%X", cpu.PC, cpu.intrVector)
	}
}

// TestIE64Sink_LevelMask_RecordsDerivedNotArg: level paths must record the
// derived unmasked-active set, not the call argument. Build the level state
// while interrupts are disabled (so the record-time gate drops everything),
// clear the mask, then enable and Ack one source; the still-active other source
// must be what gets recorded.
func TestIE64Sink_LevelMask_RecordsDerivedNotArg(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	sink := NewIE64InterruptSink(cpu)

	cpu.interruptEnabled.Store(false)
	var level LevelTriggeredInterruptSink = sink
	level.Assert(IntMaskVBI | IntMaskBlitter)
	level.SetMask(IntMaskBlitter, true)

	cpu.pendingIRQMask.Store(0)
	cpu.interruptEnabled.Store(true)

	level.Ack(IntMaskBlitter)

	if got := cpu.pendingIRQMask.Load(); got != uint32(IntMaskVBI) {
		t.Fatalf("recorded mask = 0x%X, want IntMaskVBI 0x%X (derived, not the Ack arg)", got, uint32(IntMaskVBI))
	}
}

// TestIE64Sink_LevelDeassertClearsPending: deasserting one of several active
// level causes before the CPU polls must drop that cause from the latch, not
// leave it stale.
func TestIE64Sink_LevelDeassertClearsPending(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.interruptEnabled.Store(true)
	var level LevelTriggeredInterruptSink = NewIE64InterruptSink(cpu)

	level.Assert(IntMaskVBI | IntMaskBlitter)
	if got := cpu.pendingIRQMask.Load(); got != uint32(IntMaskVBI|IntMaskBlitter) {
		t.Fatalf("after Assert, pending = 0x%X, want 0x%X", got, uint32(IntMaskVBI|IntMaskBlitter))
	}

	level.Deassert(IntMaskBlitter)
	if got := cpu.pendingIRQMask.Load(); got != uint32(IntMaskVBI) {
		t.Fatalf("after Deassert(Blitter), pending = 0x%X, want IntMaskVBI 0x%X (stale Blitter not cleared)", got, uint32(IntMaskVBI))
	}
}

// TestIE64Sink_LevelMaskClearsPending: masking an active level cause before the
// CPU polls must drop it from the latch.
func TestIE64Sink_LevelMaskClearsPending(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.interruptEnabled.Store(true)
	var level LevelTriggeredInterruptSink = NewIE64InterruptSink(cpu)

	level.Assert(IntMaskVBI | IntMaskBlitter)
	level.SetMask(IntMaskBlitter, true)
	if got := cpu.pendingIRQMask.Load(); got != uint32(IntMaskVBI) {
		t.Fatalf("after SetMask(Blitter), pending = 0x%X, want IntMaskVBI 0x%X (masked Blitter not cleared)", got, uint32(IntMaskVBI))
	}
}

// TestIE64Sink_LevelFullDeassert_NoSpuriousDelivery: a fully deasserted level
// must leave nothing pending and must not vector.
func TestIE64Sink_LevelFullDeassert_NoSpuriousDelivery(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.interruptVector = uint64(PROG_START + 0x400)
	cpu.regs[31] = STACK_START
	cpu.PC = PROG_START
	cpu.interruptEnabled.Store(true)
	var level LevelTriggeredInterruptSink = NewIE64InterruptSink(cpu)

	level.Assert(IntMaskBlitter)
	level.Deassert(IntMaskBlitter)
	if got := cpu.pendingIRQMask.Load(); got != 0 {
		t.Fatalf("after full deassert, pending = 0x%X, want 0", got)
	}
	if cpu.deliverPendingExternalInterrupt() {
		t.Fatal("fully deasserted level still vectored")
	}
	if cpu.PC != PROG_START {
		t.Fatalf("PC changed to 0x%X, want unchanged", cpu.PC)
	}
}

// TestExternalIRQ_ResetClearsPending: Reset must clear the pending latch with the
// rest of the interrupt state, so a stale IRQ from a prior run cannot vector
// after reset.
func TestExternalIRQ_ResetClearsPending(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.interruptEnabled.Store(true)
	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
	if cpu.pendingIRQMask.Load() == 0 {
		t.Fatal("setup: expected a pending IRQ before reset")
	}

	cpu.Reset()

	if cpu.pendingIRQMask.Load() != 0 {
		t.Fatalf("pendingIRQMask = 0x%X after Reset, want 0", cpu.pendingIRQMask.Load())
	}
}

// TestExternalIRQ_MMUOn_TrapStackOverflow_Halts: when trapEntry fails on a full
// trap stack during external IRQ delivery, deliver must report true (so callers
// yield to the loop-top halt check) and must not vector or run the interrupted
// instruction. trapHalted/running reflect the halt.
func TestExternalIRQ_MMUOn_TrapStackOverflow_Halts(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.mmuEnabled = true
	cpu.supervisorMode = true
	cpu.intrVector = uint64(PROG_START + 0x800)
	cpu.PC = PROG_START
	cpu.interruptEnabled.Store(true)
	cpu.running.Store(true)
	cpu.trapDepth = TrapStackDepth // force pushTrapFrame (trapEntry) to overflow

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)

	if !cpu.deliverPendingExternalInterrupt() {
		t.Fatal("deliver must return true on trap-stack overflow so the caller halts")
	}
	if !cpu.trapHalted {
		t.Fatal("trapHalted should be set after trap-stack overflow")
	}
	if cpu.running.Load() {
		t.Fatal("running should be cleared after trap-stack overflow")
	}
	if cpu.PC != PROG_START {
		t.Fatalf("PC = 0x%X, want unchanged 0x%X (must not vector)", cpu.PC, uint64(PROG_START))
	}
}

// TestInterpreter_ExternalIRQ_StackFailureHaltsImmediately: an external IRQ with
// R31 outside the guest backing fails the push without trapping, which is fatal.
// Execute must stop immediately (not run the interrupted instruction or spin to
// the next periodic running poll).
func TestInterpreter_ExternalIRQ_StackFailureHaltsImmediately(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	// MOVE R10, #0xAA at PROG_START. It must NOT execute: delivery is polled
	// before the fetch and fails fatally, so the loop breaks first.
	copy(cpu.memory[PROG_START:], ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 0xAA))
	copy(cpu.memory[PROG_START+IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu.PC = PROG_START
	cpu.interruptVector = uint64(PROG_START + 0x100)
	cpu.regs[31] = 0xFFFFFFFFFFFFFF00 // outside any backing: push fails non-trapping
	cpu.interruptEnabled.Store(true)
	cpu.running.Store(true)

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)

	done := make(chan struct{})
	go func() {
		cpu.Execute()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("Execute did not stop after fatal IRQ stack failure")
	}

	if cpu.running.Load() {
		t.Fatal("running should be cleared after fatal IRQ stack failure")
	}
	if cpu.regs[10] != 0 {
		t.Fatalf("R10 = 0x%X, want 0: interrupted instruction must not execute", cpu.regs[10])
	}
}

// TestInterpreter_ExternalIRQ_DeliveredBetweenInstructions: the interpreter
// Execute() loop must poll and deliver a pending external interrupt before the
// next instruction fetch.
func TestInterpreter_ExternalIRQ_DeliveredBetweenInstructions(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	handler := uint64(PROG_START + 0x100)
	// Body: two NOPs then HALT. The handler sets R10 then HALTs.
	copy(cpu.memory[PROG_START:], ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))
	copy(cpu.memory[PROG_START+IE64_INSTR_SIZE:], ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))
	copy(cpu.memory[PROG_START+2*IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	copy(cpu.memory[handler:], ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 0xBEEF))
	copy(cpu.memory[handler+IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu.PC = PROG_START
	cpu.interruptVector = handler
	cpu.regs[31] = STACK_START
	cpu.interruptEnabled.Store(true)
	cpu.running.Store(true)

	// Record a pending IRQ before the loop starts.
	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)

	done := make(chan struct{})
	go func() {
		cpu.Execute()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("interpreter external IRQ test timed out")
	}

	if cpu.regs[10] != 0xBEEF {
		t.Fatalf("R10 = 0x%X, want handler to run (0xBEEF)", cpu.regs[10])
	}
	if got := binary.LittleEndian.Uint64(cpu.memory[cpu.regs[31]:]); got != PROG_START {
		t.Fatalf("pushed return PC = 0x%X, want 0x%X", got, uint64(PROG_START))
	}
}
