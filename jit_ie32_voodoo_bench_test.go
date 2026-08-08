package main

import (
	"os"
	"testing"
)

// The initial table and star setup exceed a short synthetic checkpoint. This
// window reaches the steady render loop before the timed pass begins.
const ie32VoodooBenchmarkRetirement = uint64(250_000)

func BenchmarkIE32VoodooMegaDemo_Interpreter(b *testing.B) {
	benchmarkIE32VoodooMegaDemo(b, true)
}

func BenchmarkIE32VoodooMegaDemo_JIT(b *testing.B) {
	benchmarkIE32VoodooMegaDemo(b, false)
}

// benchmarkIE32VoodooMegaDemo measures the shipped IE32 binary with the real
// Voodoo MMIO mapping. Setup is outside the timed region so the pair measures
// guest execution rather than program loading or device construction.
func benchmarkIE32VoodooMegaDemo(b *testing.B, disableJIT bool) {
	b.Helper()
	program, err := os.ReadFile("sdk/examples/prebuilt/voodoo_mega_demo.iex")
	if err != nil {
		b.Fatalf("read Voodoo mega demo: %v", err)
	}
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		bus := NewMachineBus()
		voodoo, err := NewVoodooEngine(bus)
		if err != nil {
			b.Fatalf("create Voodoo: %v", err)
		}
		bus.MapIO(VOODOO_BASE, VOODOO_END, voodoo.HandleRead, voodoo.HandleWrite)
		bus.MapIOByteRead(VOODOO_BASE, VOODOO_END, voodoo.HandleRead8)
		bus.MapIOByte(VOODOO_BASE, VOODOO_END, voodoo.HandleWrite8)
		bus.MapIO64(VOODOO_BASE, VOODOO_END, voodoo.HandleRead64, voodoo.HandleWrite64)
		cpu := newIE32CPUConfigured(bus, disableJIT)
		cpu.LoadProgramBytes(program)
		if disableJIT {
			for retired := uint64(0); retired < ie32VoodooBenchmarkRetirement; retired++ {
				if cpu.StepOne() == 0 || !cpu.running.Load() {
					b.Fatalf("interpreter warm-up stopped at instruction %d PC=%#x", retired, cpu.PC)
				}
				cpu.InstructionCount++
			}
			b.StartTimer()
			for retired := uint64(0); retired < ie32VoodooBenchmarkRetirement; retired++ {
				if cpu.StepOne() == 0 || !cpu.running.Load() {
					b.Fatalf("interpreter timed run stopped at instruction %d PC=%#x", retired, cpu.PC)
				}
				cpu.InstructionCount++
			}
		} else {
			cpu.jit.testStopAfter = ie32VoodooBenchmarkRetirement
			cpu.Execute()
			if cpu.jit.testRetired < ie32VoodooBenchmarkRetirement {
				b.Fatalf("JIT warm-up retired %d instructions, want at least %d", cpu.jit.testRetired, ie32VoodooBenchmarkRetirement)
			}
			cpu.jit.testStopAfter += ie32VoodooBenchmarkRetirement
			cpu.running.Store(true)
			b.StartTimer()
			cpu.Execute()
			if cpu.jit.testRetired < 2*ie32VoodooBenchmarkRetirement {
				b.Fatalf("JIT timed run retired %d instructions, want at least %d", cpu.jit.testRetired, 2*ie32VoodooBenchmarkRetirement)
			}
			stats := cpu.JITStats()
			if stats.NativeEntries == 0 {
				b.Fatal("Voodoo benchmark did not enter generated IE32 code")
			}
			if stats.CacheHits == 0 {
				b.Fatalf("Voodoo benchmark did not reuse generated IE32 code after warm-up: blocks=%d direct=%d helpers=%d exits=%d resumes=%d fallback=%d retained=%d", stats.Blocks, stats.DirectInstructions, stats.HelperInstructions, stats.HelperExits, stats.HelperResumes, stats.ProfitabilityFallbacks, len(cpu.jit.nativeCache))
			}
			hotPC, hotCount := uint32(0), uint32(0)
			var hotCompiles uint64
			for pc, count := range cpu.jit.hotBlocks {
				hotCompiles += uint64(count)
				if count > hotCount {
					hotPC, hotCount = pc, count
				}
			}
			b.Logf("Voodoo JIT provenance blocks=%d direct=%d helpers=%d mmio_store_helpers=%d exits=%d resumes=%d cache_hits=%d invalidations=%d invalidated_blocks=%d deopts=%d source_deopts=%d resets=%d fallback=%d retained=%d transient=%d hot=%d hot_compiles=%d hottest_pc=%#x hottest_compiles=%d", stats.Blocks, stats.DirectInstructions, stats.HelperInstructions, stats.MMIOStoreHelpers, stats.HelperExits, stats.HelperResumes, stats.CacheHits, stats.Invalidations, stats.InvalidatedBlocks, stats.Deoptimizations, stats.SourceStampDeopts, stats.CodeCacheResets, stats.ProfitabilityFallbacks, len(cpu.jit.nativeCache), len(cpu.jit.transientFragments), len(cpu.jit.hotBlocks), hotCompiles, hotPC, hotCount)
		}
		b.StopTimer()
		voodoo.Destroy()
		b.StartTimer()
	}
}
