//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"strings"
	"testing"
	"time"
)

func TestMonitorRequestBreakIn_EntersMonitorFromIE64JIT(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	installIE64BreakInLoop(cpu)

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)
	mon.StartBreakpointListener()
	adapter.Resume()
	mon.RequestBreakIn()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mon.mu.Lock()
		active := mon.state == MonitorActive
		var sawBreakIn bool
		for _, line := range mon.outputLines {
			if strings.Contains(line.Text, "BREAK-IN") {
				sawBreakIn = true
				break
			}
		}
		mon.mu.Unlock()
		if active && sawBreakIn {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("monitor did not enter on JIT break-in request")
}

func TestIE64JITResumeUsesWorkerForExecutionBreakpoints(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	target := uint64(PROG_START + 16)
	copy(cpu.memory[PROG_START:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 16))
	copy(cpu.memory[target:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 0))

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)
	mon.StartBreakpointListener()
	adapter.SetBreakpoint(target)
	adapter.Resume()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mon.mu.Lock()
		active := mon.state == MonitorActive
		focusedID := mon.focusedID
		mon.mu.Unlock()
		if active && focusedID == 0 && cpu.PC == target {
			if adapter.trapRunning.Load() {
				t.Fatal("IE64 JIT execution breakpoint used trapLoop instead of worker/JIT resume")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("JIT execution breakpoint did not stop at target; pc=0x%X", cpu.PC)
}

func TestIE64JITDebugBreakInSeesChainedCallTargets(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	target := uint32(PROG_START + 0x100)
	copy(cpu.memory[PROG_START:], ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, target-PROG_START))
	back := uint32(^uint32(7))
	copy(cpu.memory[PROG_START+IE64_INSTR_SIZE:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, back))
	for i := uint32(0); i < 5; i++ {
		copy(cpu.memory[target+i*IE64_INSTR_SIZE:], ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))
	}
	copy(cpu.memory[target+5*IE64_INSTR_SIZE:], ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0))

	hits := 0
	cpu.debugBreakIn = func(pc uint64) bool {
		if pc != uint64(target) {
			return false
		}
		hits++
		return hits >= 2
	}

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatalf("JIT debug break-in did not observe repeated call target; hits=%d pc=0x%X", hits, cpu.PC)
	}
	if hits < 2 {
		t.Fatalf("debug break-in target hits = %d, want at least 2", hits)
	}
}
