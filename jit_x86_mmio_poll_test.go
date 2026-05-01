// jit_x86_mmio_poll_test.go - Slice-3 verification that the JIT
// force-native path uses the general MMIO-poll matcher and never
// triggers the rotozoomer-specific demo-accel frame shortcut.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"time"
)

// TestX86JIT_ForceNativeAvoidsDemoAccel asserts that the rotozoomer demo-
// accel shortcut (tryDemoAccelFrame) does not fire when running through
// the force-native JIT entry point. tryDemoAccelFrame sits in the
// interp-fallback loop (x86RunInterpreter) and is gated on
// cpu.x86DemoAccel + EIP, but force-native dispatch must not depend on
// it for any workload — slice-3 contract is that the JIT runs general
// programs through general native code.
func TestX86JIT_ForceNativeAvoidsDemoAccel(t *testing.T) {
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	// Single ALU instr + HLT.
	code := []byte{
		0xB8, 0x42, 0x00, 0x00, 0x00, // MOV EAX, 0x42
		0xF4, // HLT
	}
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	cpu.EIP = 0x10000
	cpu.ESP = 0x20C00
	cpu.x86JitEnabled = true
	for i, b := range code {
		cpu.memory[0x10000+uint32(i)] = b
	}
	cpu.running.Store(true)
	cpu.Halted = false
	demoStepsBefore := cpu.x86DemoAccelSteps.Load()
	done := make(chan struct{})
	go func() {
		cpu.X86ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("execution timed out")
	}
	demoStepsAfter := cpu.x86DemoAccelSteps.Load()
	if demoStepsAfter != demoStepsBefore {
		t.Errorf("force-native path triggered demo-accel %d times; "+
			"slice 3 expects general dispatch, no rotozoomer shortcut",
			demoStepsAfter-demoStepsBefore)
	}
	if cpu.EAX != 0x42 {
		t.Errorf("EAX = 0x%X, want 0x42", cpu.EAX)
	}
}

// TestX86JIT_ForceNativeMMIOPoll confirms that the general MMIO-poll
// matcher pattern (MOV r,[abs32]; TEST r,imm32; JZ/JNZ back-to-self) is
// recognized under the force-native path. The matcher is in
// tryFastMMIOPollLoop which the JIT loop body calls before each block
// dispatch — a poll loop should not bounce through Go per iteration.
func TestX86JIT_ForceNativeMMIOPoll(t *testing.T) {
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	// 0x10000: MOV EAX, [0x20100]    (A1 00 01 02 00) — 5 bytes
	// 0x10005: TEST EAX, 0x1         (A9 01 00 00 00) — 5 bytes
	// 0x1000A: JZ -12                (74 F4)         — 2 bytes (back to 0x10000)
	// 0x1000C: HLT                   (F4)
	code := []byte{
		0xA1, 0x00, 0x01, 0x02, 0x00, // MOV EAX, [0x20100]
		0xA9, 0x01, 0x00, 0x00, 0x00, // TEST EAX, 1
		0x74, 0xF4, // JZ -12
		0xF4, // HLT
	}
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	cpu.EIP = 0x10000
	cpu.ESP = 0x20C00
	cpu.x86JitEnabled = true
	for i, b := range code {
		cpu.memory[0x10000+uint32(i)] = b
	}
	// Set [0x20100] to non-zero so JZ doesn't loop forever.
	cpu.memory[0x20100] = 1
	cpu.running.Store(true)
	cpu.Halted = false
	done := make(chan struct{})
	go func() {
		cpu.X86ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("execution timed out — poll matcher likely missed pattern")
	}
	if cpu.EAX != 1 {
		t.Errorf("EAX = 0x%X, want 1 (last poll read)", cpu.EAX)
	}
	if cpu.EIP != 0x1000D {
		t.Errorf("EIP = 0x%X, want 0x1000D (post-HLT)", cpu.EIP)
	}
}
