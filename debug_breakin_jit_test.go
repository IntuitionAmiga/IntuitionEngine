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
