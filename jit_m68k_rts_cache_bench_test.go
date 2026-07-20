// jit_m68k_rts_cache_bench_test.go - End-to-end benchmarks for the opt-in
// RTS/indirect-target MRU cache (IE_M68K_JIT_ENABLE_RTS_CACHE=1). Each
// benchmark runs a call-heavy stop program through the production JIT
// dispatcher with the cache off (default policy) and on, so the measured
// delta is exactly what flipping the default would change.
//
// Three shapes bracket the policy decision:
//   - Mono: one JSR abs.L call site, monomorphic return target. Best case:
//     every RTS hits slot 0 and chains natively past the dispatcher.
//   - Poly10: ten distinct call sites cycling every iteration. Worst case:
//     ten live return targets overflow the eight-entry MRU, so the probe
//     costs its comparisons and always misses.
//   - IndirectMono: JSR (A0) with a stable target. Exercises the
//     indirect-target specialisation that rides the same policy switch.
//
// The callee body is five single-word instructions, one past the JSR leaf
// fusion cap, so the calls stay real dynamic JSR/RTS traffic instead of
// being inlined by m68kFuseJSRLeafCalls.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"time"
)

const m68kRTSBenchCalls = 4096

// m68kRTSBenchCallee writes the shared non-fusable subroutine at pc:
// ADDQ.L #1,D0 x5 then RTS.
func m68kRTSBenchCallee(cpu *M68KCPU, pc uint32) {
	writeM68KWords(cpu, pc, 0x5280, 0x5280, 0x5280, 0x5280, 0x5280, 0x4E75)
}

func newM68KRTSBenchCPU(b *testing.B, startPC uint32) *M68KCPU {
	b.Helper()
	bus := NewMachineBus()
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	cpu.m68kJitWarmupLimit = 1
	return cpu
}

func runM68KJITBenchUntilStopped(b *testing.B, cpu *M68KCPU) {
	b.Helper()
	cpu.running.Store(true)
	done := make(chan struct{})
	go func() {
		cpu.M68KExecuteJIT()
		close(done)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		select {
		case <-done:
			if !cpu.stopped.Load() {
				b.Fatalf("JIT returned before STOP: PC=0x%08X", cpu.PC)
			}
			cpu.running.Store(false)
			return
		default:
			time.Sleep(50 * time.Microsecond)
		}
	}
	cpu.running.Store(false)
	<-done
	if !cpu.stopped.Load() {
		b.Fatalf("stop program timed out: PC=0x%08X", cpu.PC)
	}
}

// Monomorphic: MOVE.W #N-1,D7; loop: JSR $2000.L; DBRA D7,loop; STOP.
func m68kRTSBenchProgramMono(cpu *M68KCPU) uint32 {
	const startPC = uint32(0x1000)
	writeM68KStopProgram(cpu, startPC,
		0x3E3C, m68kRTSBenchCalls-1, // MOVE.W #N-1,D7
		0x4EB9, 0x0000, 0x2000, // JSR $2000.L
		0x51CF, 0xFFF8, // DBRA D7,loop
	)
	m68kRTSBenchCallee(cpu, 0x2000)
	return startPC
}

// Ten call sites per iteration: ten distinct return PCs cycle every loop,
// overflowing the eight-entry MRU.
func m68kRTSBenchProgramPoly10(cpu *M68KCPU) uint32 {
	const startPC = uint32(0x1000)
	words := []uint16{0x3E3C, m68kRTSBenchCalls/10 - 1}
	for i := 0; i < 10; i++ {
		words = append(words, 0x4EB9, 0x0000, 0x2000)
	}
	// DBRA displacement: base is the word after the DBRA opcode.
	loopTop := startPC + 4
	dbraExt := startPC + 4 + 10*6 + 2
	words = append(words, 0x51CF, uint16(loopTop-dbraExt))
	writeM68KStopProgram(cpu, startPC, words...)
	m68kRTSBenchCallee(cpu, 0x2000)
	return startPC
}

// Indirect: A0 = callee; loop: JSR (A0); DBRA D7,loop; STOP.
func m68kRTSBenchProgramIndirect(cpu *M68KCPU) uint32 {
	const startPC = uint32(0x1000)
	writeM68KStopProgram(cpu, startPC,
		0x3E3C, m68kRTSBenchCalls-1, // MOVE.W #N-1,D7
		0x4E90,         // JSR (A0)
		0x51CF, 0xFFFC, // DBRA D7,loop
	)
	m68kRTSBenchCallee(cpu, 0x2000)
	cpu.AddrRegs[0] = 0x2000
	return startPC
}

func benchmarkM68KRTSCache(b *testing.B, program func(*M68KCPU) uint32, enable bool) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available")
	}
	if enable {
		b.Setenv("IE_M68K_JIT_ENABLE_RTS_CACHE", "1")
	} else {
		b.Setenv("IE_M68K_JIT_ENABLE_RTS_CACHE", "")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		cpu := newM68KRTSBenchCPU(b, 0x1000)
		cpu.m68kJitEnabled = true
		startPC := program(cpu)
		cpu.PC = startPC
		b.StartTimer()
		runM68KJITBenchUntilStopped(b, cpu)
		b.StopTimer()
		// Every shape performs a whole number of calls adding 5 to D0 each.
		if cpu.DataRegs[0] == 0 || cpu.DataRegs[0]%5 != 0 {
			b.Fatalf("bad D0 after run: %d", cpu.DataRegs[0])
		}
		b.StartTimer()
	}
}

func BenchmarkM68KJIT_RTSCacheMonoOff(b *testing.B) {
	benchmarkM68KRTSCache(b, m68kRTSBenchProgramMono, false)
}
func BenchmarkM68KJIT_RTSCacheMonoOn(b *testing.B) {
	benchmarkM68KRTSCache(b, m68kRTSBenchProgramMono, true)
}
func BenchmarkM68KJIT_RTSCachePoly10Off(b *testing.B) {
	benchmarkM68KRTSCache(b, m68kRTSBenchProgramPoly10, false)
}
func BenchmarkM68KJIT_RTSCachePoly10On(b *testing.B) {
	benchmarkM68KRTSCache(b, m68kRTSBenchProgramPoly10, true)
}
func BenchmarkM68KJIT_RTSCacheIndirectOff(b *testing.B) {
	benchmarkM68KRTSCache(b, m68kRTSBenchProgramIndirect, false)
}
func BenchmarkM68KJIT_RTSCacheIndirectOn(b *testing.B) {
	benchmarkM68KRTSCache(b, m68kRTSBenchProgramIndirect, true)
}
