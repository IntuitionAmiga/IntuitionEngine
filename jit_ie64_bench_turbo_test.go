//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

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
