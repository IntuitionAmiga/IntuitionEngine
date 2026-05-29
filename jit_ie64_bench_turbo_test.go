//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// TestIE64BenchTurboBailsOnPendingIRQ: the turbo fast path runs a benchmark loop
// to completion with no per-instruction interrupt check, so it must not start
// while an external interrupt is pending (it would otherwise halt the program
// before the dispatcher delivers).
func TestIE64BenchTurboBailsOnPendingIRQ(t *testing.T) {
	instrs, _ := buildALUProgram(4)

	base := NewCPU64(NewMachineBus())
	loadBenchProgram(base, instrs)
	base.PC = PROG_START
	base.running.Store(true)
	if matched, _ := base.tryIE64TurboProgram(PROG_START, false); !matched {
		t.Fatal("baseline: ALU bench program should enter turbo with no pending IRQ")
	}

	pend := NewCPU64(NewMachineBus())
	loadBenchProgram(pend, instrs)
	pend.PC = PROG_START
	pend.running.Store(true)
	pend.pendingIRQMask.Store(uint32(IntMaskBlitter))
	if matched, _ := pend.tryIE64TurboProgram(PROG_START, false); matched {
		t.Fatal("turbo must not start while an external IRQ is pending")
	}
}

func TestIE64BenchTurboRejectsZeroIterationLoops(t *testing.T) {
	cases := []struct {
		name  string
		build func(uint32) ([][]byte, int)
	}{
		{name: "ALU", build: buildALUProgram},
		{name: "Memory", build: buildMemoryProgram},
		{name: "Call", build: buildCallProgram},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instrs, _ := tc.build(0)
			cpu := NewCPU64(NewMachineBus())
			loadBenchProgram(cpu, instrs)
			cpu.PC = PROG_START
			cpu.running.Store(true)

			if matched, retired := cpu.tryIE64TurboProgram(PROG_START, false); matched {
				t.Fatalf("zero-iteration %s loop entered turbo: retired=%d", tc.name, retired)
			}
		})
	}
}
