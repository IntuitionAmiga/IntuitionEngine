package main

import (
	"testing"
	"unsafe"
)

// TestCPUIE64CacheLineLayout asserts that the IE64 execution-state atomics
// (running/debug) and the timer atomics live on different cache lines so
// guest dispatch reads of running don't false-share with timer interrupt
// arithmetic. Phase 7d sibling test for 6502's TestCPU6502CacheLineLayout.
//
// IE64's struct (cpu_ie64.go:399-413) intentionally puts the register file
// on lines 0-3, execution state (PC + running + debug + cycleCounter +
// _pad4) on line 4, and timer atomics on line 5. This test pins that
// invariant.
func TestCPUIE64CacheLineLayout(t *testing.T) {
	var cpu CPU64
	runningLine := unsafe.Offsetof(cpu.running) / 64
	timerLine := unsafe.Offsetof(cpu.timerCount) / 64
	if runningLine == timerLine {
		t.Fatalf("IE64 running and timerCount share cache line %d — Phase 7d rule says distinct lines",
			runningLine)
	}
}
