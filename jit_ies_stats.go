package main

import (
	"runtime"
	"sync/atomic"
)

// cpuJITStats contains the backend-neutral counters exposed through
// cpu.jit_stats(). Each CPU owns its counters so workers and tests cannot
// contribute to another CPU's report.
type cpuJITStats struct {
	instructionCount          atomic.Uint64
	nativeEntries             atomic.Uint64
	nativeRetired             atomic.Uint64
	compiledBlocks            atomic.Uint64
	compiledRegions           atomic.Uint64
	regionCandidates          atomic.Uint64
	regionRejections          atomic.Uint64
	fallbackInstructions      atomic.Uint64
	helperExits               atomic.Uint64
	helperResumes             atomic.Uint64
	helperResumeCancellations atomic.Uint64
	ioBails                   atomic.Uint64
	invalidations             atomic.Uint64
	invalidatedBlocks         atomic.Uint64
	chainExits                atomic.Uint64
	cacheHits                 atomic.Uint64
	cacheMisses               atomic.Uint64
	codeCacheResets           atomic.Uint64
	spills                    atomic.Uint64
	fpuSpills                 atomic.Uint64
	directRAMProofs           atomic.Uint64
	inlinedCalls              atomic.Uint64
}

type cpuJITStatsSnapshot struct {
	InstructionCount          uint64
	NativeEntries             uint64
	NativeRetired             uint64
	CompiledBlocks            uint64
	CompiledRegions           uint64
	RegionCandidates          uint64
	RegionRejections          uint64
	FallbackInstructions      uint64
	HelperExits               uint64
	HelperResumes             uint64
	HelperResumeCancellations uint64
	IOBails                   uint64
	Invalidations             uint64
	InvalidatedBlocks         uint64
	ChainExits                uint64
	CacheHits                 uint64
	CacheMisses               uint64
	CodeCacheResets           uint64
	Spills                    uint64
	FPUSpills                 uint64
	DirectRAMProofs           uint64
	InlinedCalls              uint64
}

func (s *cpuJITStats) snapshot() cpuJITStatsSnapshot {
	return cpuJITStatsSnapshot{
		InstructionCount:          s.instructionCount.Load(),
		NativeEntries:             s.nativeEntries.Load(),
		NativeRetired:             s.nativeRetired.Load(),
		CompiledBlocks:            s.compiledBlocks.Load(),
		CompiledRegions:           s.compiledRegions.Load(),
		RegionCandidates:          s.regionCandidates.Load(),
		RegionRejections:          s.regionRejections.Load(),
		FallbackInstructions:      s.fallbackInstructions.Load(),
		HelperExits:               s.helperExits.Load(),
		HelperResumes:             s.helperResumes.Load(),
		HelperResumeCancellations: s.helperResumeCancellations.Load(),
		IOBails:                   s.ioBails.Load(),
		Invalidations:             s.invalidations.Load(),
		InvalidatedBlocks:         s.invalidatedBlocks.Load(),
		ChainExits:                s.chainExits.Load(),
		CacheHits:                 s.cacheHits.Load(),
		CacheMisses:               s.cacheMisses.Load(),
		CodeCacheResets:           s.codeCacheResets.Load(),
		Spills:                    s.spills.Load(),
		FPUSpills:                 s.fpuSpills.Load(),
		DirectRAMProofs:           s.directRAMProofs.Load(),
		InlinedCalls:              s.inlinedCalls.Load(),
	}
}

func (s *cpuJITStats) reset() {
	s.instructionCount.Store(0)
	s.nativeEntries.Store(0)
	s.nativeRetired.Store(0)
	s.compiledBlocks.Store(0)
	s.compiledRegions.Store(0)
	s.regionCandidates.Store(0)
	s.regionRejections.Store(0)
	s.fallbackInstructions.Store(0)
	s.helperExits.Store(0)
	s.helperResumes.Store(0)
	s.helperResumeCancellations.Store(0)
	s.ioBails.Store(0)
	s.invalidations.Store(0)
	s.invalidatedBlocks.Store(0)
	s.chainExits.Store(0)
	s.cacheHits.Store(0)
	s.cacheMisses.Store(0)
	s.codeCacheResets.Store(0)
	s.spills.Store(0)
	s.fpuSpills.Store(0)
	s.directRAMProofs.Store(0)
	s.inlinedCalls.Store(0)
}

func ie64JITBackend() string {
	if wasmJITSupported {
		return "wasm"
	}
	if jitAvailable {
		return "native"
	}
	return "none"
}

func x86JITBackend() string {
	if !x86JitAvailable {
		return "none"
	}
	if runtime.GOOS == "js" && runtime.GOARCH == "wasm" {
		return "wasm"
	}
	return "native"
}
