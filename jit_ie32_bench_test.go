package main

import "testing"

func BenchmarkIE32JIT_PureCachedBlock(b *testing.B) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		b.Skip("native IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 0x55)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute() // populate the pure-block cache outside the timed region
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.running.Store(true)
		cpu.Execute()
	}
	b.StopTimer()
	if got := cpu.JITStats().CacheHits; got < uint64(b.N) {
		b.Fatalf("cache hits=%d, want at least %d", got, b.N)
	}
}

func BenchmarkIE32Interpreter_ImmediateLoad(b *testing.B) {
	cpu := NewCPU(NewMachineBus())
	cpu.running.Store(false)
	if err := cpu.SetJITEnabled(false); err != nil {
		b.Fatalf("disable JIT: %v", err)
	}
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 0x55)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.running.Store(true)
		cpu.Execute()
	}
}
