//go:build !amd64 || (!linux && !windows && !darwin)

package main

import (
	"testing"
	"time"
)

// runX86JITProgram loads x86 machine code at startPC, sets EIP, runs the JIT
// execution loop with a timeout, and returns the CPU for result inspection on
// non-amd64 hosts. Linux/arm64 uses it for native x86 backend correctness and
// benchmark tests; other hosts skip when the x86 JIT is unavailable.
func runX86JITProgram(t *testing.T, startPC uint32, code ...byte) *CPU_X86 {
	return runX86JITProgramWithSetup(t, startPC, nil, code...)
}

func runX86JITProgramWithSetup(t *testing.T, startPC uint32, setup func(*CPU_X86), code ...byte) *CPU_X86 {
	t.Helper()

	if !x86JitAvailable {
		t.Skip("x86 JIT not available on this platform")
	}

	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	if cpu.FPU == nil {
		cpu.FPU = NewFPU_X87()
	}
	cpu.memory = adapter.GetMemory()
	cpu.x86JitEnabled = true
	cpu.EIP = startPC
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	for i, b := range code {
		cpu.memory[startPC+uint32(i)] = b
	}
	if setup != nil {
		setup(cpu)
	}

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.Halted = false
		cpu.X86ExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("x86 JIT execution timed out")
	}

	return cpu
}

// runX86InterpreterProgram runs the supplied code through the interpreter on
// any host that can build the x86 CPU core. Shared wasm and cross-host tests
// use it as the architectural reference.
func runX86InterpreterProgram(t *testing.T, startPC uint32, code ...byte) *CPU_X86 {
	return runX86InterpreterProgramWithSetup(t, startPC, nil, code...)
}

func runX86InterpreterProgramWithSetup(t *testing.T, startPC uint32, setup func(*CPU_X86), code ...byte) *CPU_X86 {
	t.Helper()

	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	if cpu.FPU == nil {
		cpu.FPU = NewFPU_X87()
	}
	cpu.memory = adapter.GetMemory()
	cpu.EIP = startPC

	for i, b := range code {
		cpu.memory[startPC+uint32(i)] = b
	}
	if setup != nil {
		setup(cpu)
	}

	cpu.running.Store(true)
	cpu.Halted = false
	for cpu.Running() && !cpu.Halted {
		cpu.Step()
	}

	return cpu
}
