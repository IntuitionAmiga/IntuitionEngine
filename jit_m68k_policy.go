// jit_m68k_policy.go - backend-neutral M68020 JIT dispatch and tier policy.
//
// Extracted from jit_m68k_exec.go (M68K JIT parity plan, milestone 2). The
// symbols here are pure policy: environment kill switches, warmup/tier
// gating, fallback-burst sizing, interrupt-sample cadence, hotness
// accounting, and the retired-instruction contract. None of them touch
// native executable memory or emitter state, so every M68020 backend
// (amd64, arm64, wasm) shares them. Native execution mechanics (chain
// patching, callNative dispatch, exec-mem ownership) stay in the
// backend-specific execution files.
//
// This file must stay free of build tags and emitter symbols.

package main

import (
	"os"
	"strconv"
)

const (
	m68kJitFallbackBurstMax = 512
	m68kJitWarmupDefault    = 2
)

func m68kJITFallbackBurstMax() int {
	if raw := os.Getenv("IE_M68K_JIT_FALLBACK_BURST"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return m68kJitFallbackBurstMax
}

func m68kFallbackBurstUntilInterruptSample(instructionCount uint64) int {
	max := m68kJITFallbackBurstMax()
	untilSample := 256 - int(instructionCount&0xFF)
	if untilSample <= 0 || untilSample > 256 {
		untilSample = 256
	}
	if max > untilSample {
		return untilSample
	}
	return max
}

func m68kJITDisableChains() bool {
	return os.Getenv("IE_M68K_JIT_DISABLE_CHAINS") == "1"
}

func m68kJITDisableRTSCache() bool {
	if os.Getenv("IE_M68K_JIT_DISABLE_RTS_CACHE") == "1" {
		return true
	}
	return os.Getenv("IE_M68K_JIT_ENABLE_RTS_CACHE") != "1"
}

// m68kJITStrictMode reports whether the JIT must fail loudly on a compiler
// error instead of falling back to one interpreter instruction. Used to catch
// emitter bugs during development (M68K_JIT_FALLBACK_REMOVAL_PLAN.md).
func m68kJITStrictMode() bool {
	return os.Getenv("IE_M68K_JIT_STRICT") == "1"
}

// m68kJITDiagBurstFallback re-enables the legacy multi-instruction interpreter
// burst for the production blocked/compile-failure paths. Default off: normal
// production fallback executes exactly one instruction and returns to the JIT
// dispatcher. This switch exists only for diagnostics/A-B comparison; no normal
// run should rely on it.
func m68kJITDiagBurstFallback() bool {
	return os.Getenv("IE_M68K_JIT_DIAG_BURST_FALLBACK") == "1"
}

func m68kJITDisableRegions() bool {
	return os.Getenv("IE_M68K_JIT_DISABLE_REGIONS") == "1"
}

func m68kJITDisableStaticJMPChase() bool {
	return os.Getenv("IE_M68K_JIT_DISABLE_STATIC_JMP_CHASE") == "1"
}

// m68kJITInterruptSampleIntervalDefault is the number of retired guest
// instructions between pending-interrupt samples in the chained native
// dispatch loop. 256 keeps IRQ latency tight enough for the AROS timer while
// amortising the checkPending cost across a block chain; it is deliberately the
// default rather than a larger value because raising it trades interrupt
// latency for throughput and the AROS boot is timing-sensitive.
const m68kJITInterruptSampleIntervalDefault = 256

// m68kJITInterruptSampleIntervalMax bounds the tunable interval. The default
// stays at 256; power users profiling throughput-bound, interrupt-light
// workloads can raise IE_M68K_JIT_IRQ_SAMPLE_INTERVAL up to this ceiling to cut
// sampling overhead in long native chains, accepting coarser IRQ latency.
const m68kJITInterruptSampleIntervalMax = 2048

func m68kJITInterruptSampleInterval() uint32 {
	if raw := os.Getenv("IE_M68K_JIT_IRQ_SAMPLE_INTERVAL"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n < 1 {
				return 1
			}
			if n > m68kJITInterruptSampleIntervalMax {
				return m68kJITInterruptSampleIntervalMax
			}
			return uint32(n)
		}
	}
	return m68kJITInterruptSampleIntervalDefault
}

func m68kJITCompileWarmupLimit() uint8 {
	if raw := os.Getenv("IE_M68K_JIT_COMPILE_WARMUP"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			switch {
			case n <= 1:
				return 1
			case n > 255:
				return 255
			default:
				return uint8(n)
			}
		}
	}
	return m68kJitWarmupDefault
}

func m68kBumpJITBlockHotness(block *JITBlock, increment uint32) {
	if block == nil {
		return
	}
	if increment == 0 {
		increment = 1
	}
	if ^uint32(0)-block.execCount < increment {
		block.execCount = ^uint32(0)
		return
	}
	block.execCount += increment
}

func m68kNativeHotnessIncrement(block *JITBlock, chainCount uint32) uint32 {
	if block == nil || chainCount == 0 || block.instrCount <= 0 {
		return 1
	}
	perEntry := uint32(block.instrCount)
	return (chainCount + perEntry - 1) / perEntry
}

// m68kJITRetiredInstructionCount computes the number of guest instructions a
// native execution retired. The accounting contract is uniform:
//   - ChainCount holds instructions retired in every block chained THROUGH,
//     plus in-block loop re-executions (accumulated by the chain-exit prologue
//     and the within-block loop emitters).
//   - RetCount holds the final (returning) block's own linear instruction count.
//   - Total retired = ChainCount + RetCount.
//
// Earlier code special-cased `retCount <= blockInstrCount` to add the two, else
// returned retCount alone. That dropped ChainCount whenever the final chained
// block was larger than the entry block (blockInstrCount is the ENTRY block's
// size, unrelated to the final block's retCount) — a distributed under-count
// that desynchronized the instruction-count-keyed interrupt cadence from the
// interpreter. The sum is always correct given the contract above.
func m68kJITRetiredInstructionCount(retCount, chainCount uint32, blockInstrCount int, exitSignal bool) uint64 {
	if retCount == 0 {
		if chainCount > 0 {
			return uint64(chainCount)
		}
		if !exitSignal && blockInstrCount > 0 {
			return uint64(blockInstrCount)
		}
		return 0
	}
	return uint64(chainCount) + uint64(retCount)
}

func (cpu *M68KCPU) m68kJITShouldWarmupInterpret(pc uint32) bool {
	if cpu == nil || cpu.m68kJitForceNative || cpu.m68kJitWarmupLimit <= 1 {
		return false
	}
	if cpu.m68kJitWarmupCounts == nil {
		cpu.m68kJitWarmupCounts = make(map[uint32]uint8, 4096)
	}
	count := cpu.m68kJitWarmupCounts[pc]
	if count+1 >= cpu.m68kJitWarmupLimit {
		cpu.m68kJitWarmupCounts[pc] = cpu.m68kJitWarmupLimit
		return false
	}
	cpu.m68kJitWarmupCounts[pc] = count + 1
	return true
}
