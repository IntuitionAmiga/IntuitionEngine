package main

import (
	"strings"
	"testing"
	"time"
)

func TestBreakInRequest_AdapterStopsBeforeStep_IE64(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)
	cpu.PC = PROG_START

	adapter := NewDebugIE64(cpu)
	events := make(chan BreakpointEvent, 1)
	adapter.SetBreakpointChannel(events, 3)

	adapter.RequestBreakIn()
	if !adapter.BreakInRequested() {
		t.Fatal("break-in request flag was not set")
	}

	pcBefore := adapter.GetPC()
	if cycles := adapter.Step(); cycles != 0 {
		t.Fatalf("Step cycles = %d, want 0 for pre-instruction break-in", cycles)
	}
	if got := adapter.GetPC(); got != pcBefore {
		t.Fatalf("PC changed across break-in: got %#x, want %#x", got, pcBefore)
	}
	if adapter.BreakInRequested() {
		t.Fatal("break-in request flag was not consumed")
	}

	select {
	case ev := <-events:
		if !ev.IsBreakIn {
			t.Fatalf("event IsBreakIn = false, event=%+v", ev)
		}
		if ev.CPUID != 3 {
			t.Fatalf("event CPUID = %d, want 3", ev.CPUID)
		}
		if ev.Address != pcBefore {
			t.Fatalf("event address = %#x, want %#x", ev.Address, pcBefore)
		}
	default:
		t.Fatal("expected break-in event")
	}
}

func TestMonitorRequestBreakIn_EntersMonitor(t *testing.T) {
	mon, cpu := newTestMonitor()
	installIE64BreakInLoop(cpu)

	mon.StartBreakpointListener()
	entry := mon.cpus[mon.focusedID]
	if entry == nil {
		t.Fatal("no focused CPU")
	}
	entry.CPU.Resume()

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
	t.Fatal("monitor did not enter on break-in request")
}

func installIE64BreakInLoop(cpu *CPU64) {
	for addr := PROG_START; addr < PROG_START+80; addr += 8 {
		cpu.memory[addr] = OP_NOP64
	}
	cpu.memory[PROG_START+80] = OP_BRA
	offsetSigned := int32(-88)
	offset := uint32(offsetSigned)
	cpu.memory[PROG_START+84] = byte(offset)
	cpu.memory[PROG_START+85] = byte(offset >> 8)
	cpu.memory[PROG_START+86] = byte(offset >> 16)
	cpu.memory[PROG_START+87] = byte(offset >> 24)
}

func TestMonitorRequestBreakIn_DoesNotResumeStoppedCPU(t *testing.T) {
	mon, cpu := newTestMonitor()
	cpu.running.Store(false)
	mon.StartBreakpointListener()

	mon.RequestBreakIn()
	time.Sleep(10 * time.Millisecond)

	mon.mu.Lock()
	active := mon.state == MonitorActive
	wasRunning := mon.wasRunning[mon.focusedID]
	mon.mu.Unlock()

	if active {
		t.Fatal("break-in on stopped CPU should not activate monitor")
	}
	if wasRunning {
		t.Fatal("stopped CPU was recorded as previously running")
	}

	mon.Deactivate()
	if cpu.running.Load() {
		t.Fatal("stopped CPU was resumed by break-in/deactivate")
	}
}
