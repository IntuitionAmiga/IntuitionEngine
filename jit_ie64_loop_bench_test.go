//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func BenchmarkIE64_BoundedCounterLoop_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available")
	}
	const count = 1000
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	buildIE64BoundedLoop(count)(cpu.memory)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[2], cpu.regs[3] = 0, 0
		cpu.running.Store(true)
		cpu.jitExecute()
	}
	b.ReportMetric(float64(1+count*3+1), "instructions/op")
}
