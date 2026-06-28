//go:build headless && m68k_test && amd64

package main

import (
	"sort"
	"testing"
	"time"
)

func TestM68KJIT_EmuTOSBootFallbackDiagnostics(t *testing.T) {
	video, _, cpu, loader := bootEmuTOSForBlitter(t)
	cpu.m68kJitPersist = true

	loader.StartTimer()
	defer loader.Stop()
	go cpu.M68KExecuteJIT()
	defer cpu.running.Store(false)

	time.Sleep(5 * time.Second)
	cpu.running.Store(false)
	time.Sleep(50 * time.Millisecond)

	t.Logf("EmuTOS JIT stats: native_blocks=%d fallback_instructions=%d bailouts=%d mmio_guard_exits=%d unsupported_one_exits=%d compile_failure_exits=%d native_helper_exits=%d native_exception_exits=%d last_pc=%08X last_opcode=%04X blitter_starts=%d",
		cpu.m68kJitNativeBlocksExecuted.Load(),
		cpu.m68kJitFallbackInstructions.Load(),
		cpu.m68kJitBailoutCount.Load(),
		cpu.m68kJitMMIOGuardExits.Load(),
		cpu.m68kJitUnsupportedOneExits.Load(),
		cpu.m68kJitCompileFailureExits.Load(),
		cpu.m68kJitNativeHelperExits.Load(),
		cpu.m68kJitNativeExceptionExits.Load(),
		cpu.m68kJitLastFallbackPC.Load(),
		cpu.m68kJitLastFallbackOpcode.Load(),
		video.BlitStartCount())

	type opcodeCount struct {
		opcode uint16
		count  uint64
	}
	var counts []opcodeCount
	for opcode := range cpu.m68kJitFallbackOpcodeCounts {
		count := cpu.m68kJitFallbackOpcodeCounts[opcode].Load()
		if count != 0 {
			counts = append(counts, opcodeCount{opcode: uint16(opcode), count: count})
		}
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].count == counts[j].count {
			return counts[i].opcode < counts[j].opcode
		}
		return counts[i].count > counts[j].count
	})
	if len(counts) > 16 {
		counts = counts[:16]
	}
	for _, c := range counts {
		t.Logf("EmuTOS JIT fallback opcode %04X pc=%08X count=%d", c.opcode, cpu.m68kJitFallbackOpcodePCs[c.opcode].Load(), c.count)
	}
	if cpu.m68kJitNativeBlocksExecuted.Load() == 0 {
		t.Fatalf("EmuTOS JIT executed no native blocks")
	}
}
