package main

import "sync"

type IE64InterruptSink struct {
	cpu   *CPU64
	mu    sync.Mutex
	level interruptLevelState
}

func NewIE64InterruptSink(cpu *CPU64) *IE64InterruptSink {
	return &IE64InterruptSink{cpu: cpu}
}

// Pulse is edge-triggered: the call argument is the cause. It carries no level
// state, so it records the argument directly.
func (s *IE64InterruptSink) Pulse(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.cpu.handleExternalInterrupt(mask)
}

// Assert/Deassert/Ack/SetMask are level-triggered. Each reconciles the pending
// latch with the current level state via applyLevel: the cause to record is the
// derived unmasked-active set (pendingMask), NOT the call argument (e.g. with
// VBI and Blitter both active, Ack(Blitter) must still leave VBI pending), and
// causes that are no longer pending (deasserted or masked) are cleared from the
// latch so a level change before the CPU polls does not deliver a stale cause.
// The level state is plain (non-atomic) and device goroutines may call
// concurrently, so the whole reconcile happens under s.mu.
func (s *IE64InterruptSink) Assert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.mu.Lock()
	s.level.assert(mask)
	s.applyLevelLocked(mask)
}

func (s *IE64InterruptSink) Deassert(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.mu.Lock()
	s.level.deassert(mask)
	s.applyLevelLocked(mask)
}

func (s *IE64InterruptSink) Ack(mask InterruptMask) {
	if s == nil || s.cpu == nil {
		return
	}
	s.mu.Lock()
	s.level.ack(mask)
	s.applyLevelLocked(mask)
}

func (s *IE64InterruptSink) SetMask(mask InterruptMask, masked bool) {
	if s == nil || s.cpu == nil {
		return
	}
	s.mu.Lock()
	s.level.setMask(mask, masked)
	s.applyLevelLocked(mask)
}

// applyLevelLocked reconciles the CPU pending latch with the level state after a
// change to the causes in arg. Must be called with s.mu held; it releases the
// lock. Bits in arg that are no longer pending (deasserted or masked) are
// cleared from pendingIRQMask; the still-pending set is then recorded (gated by
// handleExternalInterrupt). Clearing and recording happen under the lock so the
// latch stays consistent with the level state against concurrent sink calls.
func (s *IE64InterruptSink) applyLevelLocked(arg InterruptMask) {
	pending := s.level.pendingMask()
	if clear := uint32(arg) &^ uint32(pending); clear != 0 {
		s.cpu.pendingIRQMask.And(^clear)
	}
	if pending != 0 {
		s.cpu.handleExternalInterrupt(pending)
	}
	s.mu.Unlock()
}

// handleExternalInterrupt records a pending external interrupt. It is called
// from device goroutines and must not touch architectural CPU state (PC, stack,
// inInterrupt) directly; those are owned by the CPU goroutine and delivered via
// deliverPendingExternalInterrupt at instruction/block boundaries. The gate here
// preserves pulse-drop timing: an interrupt raised while interrupts are disabled
// (or one is already in flight) is dropped at raise time, exactly as the old
// synchronous path did, rather than being latched for later.
func (cpu *CPU64) handleExternalInterrupt(mask InterruptMask) {
	if cpu == nil || !cpu.interruptEnabled.Load() || cpu.inInterrupt.Load() {
		return
	}
	cpu.pendingIRQMask.Or(uint32(mask))
}
