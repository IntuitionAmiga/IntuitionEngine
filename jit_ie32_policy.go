package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// IE32JITStats is a stable snapshot of IE32 JIT execution provenance.  The
// fields deliberately describe executed generated-code entries rather than
// compilation intent, so automation cannot mistake interpreter fallbacks for
// JIT coverage.
type IE32JITStats struct {
	Backend             string
	NativeEntries       uint64
	Blocks              uint64
	Regions             uint64
	HotRecompilations   uint64
	Instructions        uint64
	DirectInstructions  uint64
	HelperInstructions  uint64
	Chains              uint64
	ChainBudgetExits    uint64
	Deoptimizations     uint64
	HelperDeopts        uint64
	SourceStampDeopts   uint64
	CodeCacheResets     uint64
	Invalidations       uint64
	CacheHits           uint64
	ReturnCacheHits     uint64
	MMIOPollIterations  uint64
	ResidentSpillsSaved uint64
	CountedLoops        uint64
}

type ie32JITState struct {
	nativeEntries              atomic.Uint64
	blocks                     atomic.Uint64
	regions                    atomic.Uint64
	hotRecompilations          atomic.Uint64
	hotBlocks                  map[uint32]uint32
	instructions               atomic.Uint64
	directInstructions         atomic.Uint64
	helperInstructions         atomic.Uint64
	chains                     atomic.Uint64
	chainBudgetExits           atomic.Uint64
	deoptimizations            atomic.Uint64
	helperDeopts               atomic.Uint64
	sourceStampDeopts          atomic.Uint64
	codeCacheResets            atomic.Uint64
	invalidationGeneration     atomic.Uint64
	seenInvalidationGeneration uint64
	invalidationMu             sync.Mutex
	pendingInvalidations       []ie32JITInvalidation
	pendingInvalidateAll       bool
	invalidations              atomic.Uint64
	cacheHits                  atomic.Uint64
	returnCacheHits            atomic.Uint64
	mmioPollIterations         atomic.Uint64
	residentSpillsSaved        atomic.Uint64
	countedLoops               atomic.Uint64
	execMem                    *ExecMem
	nativeCache                map[uint32]ie32NativeCachedBlock
	returnCache                [2]ie32NativeCachedBlock
	returnCachePending         bool
	busUnregister              func()
	// testStopAfter/testRetired provide a bounded guest-retirement controller
	// for tests. testExactRetirement additionally prevents the dispatcher from
	// entering another chained block before it observes the requested boundary.
	// Zero leaves production execution unchanged.
	testStopAfter       uint64
	testRetired         uint64
	testExactRetirement bool
}

type ie32NativeCachedBlock struct {
	pc                  uint32
	addr                uintptr
	retired             uint64
	residentSpillsSaved uint64
	stamp               uint64
	sourceStart         uint64
	sourceEnd           uint64
	sourceRanges        []ie32JITInvalidation
}

func (cpu *CPU) rememberIE32ReturnCache(cached ie32NativeCachedBlock) {
	if cpu == nil || cpu.jit == nil || cached.addr == 0 {
		return
	}
	cpu.jit.returnCache[1] = cpu.jit.returnCache[0]
	cpu.jit.returnCache[0] = cached
}

func (cpu *CPU) takeIE32ReturnCache() (ie32NativeCachedBlock, bool) {
	if cpu == nil || cpu.jit == nil || !cpu.jit.returnCachePending {
		return ie32NativeCachedBlock{}, false
	}
	cpu.jit.returnCachePending = false
	for _, cached := range cpu.jit.returnCache {
		if cached.addr != 0 && cached.pc == cpu.PC && cached.stamp == ie32CachedSourceStamp(cpu.memory, cached) {
			cpu.jit.returnCacheHits.Add(1)
			return cached, true
		}
	}
	return ie32NativeCachedBlock{}, false
}

// ie32JITInvalidation records a physical write. It is queued by arbitrary
// writers but consumed only by the owning IE32 dispatcher.
type ie32JITInvalidation struct {
	addr uint64
	size uint64
}

// ie32BlockSourceStamp is a deterministic byte stamp for a retained native
// block. It is deliberately independent of the bus generation so a cache
// cannot execute stale code if an integration path forgot to publish a write.
func ie32BlockSourceStamp(memory []byte, pc uint32, retired uint64) uint64 {
	start := uint64(pc)
	end := start + retired*INSTRUCTION_SIZE
	if start >= uint64(len(memory)) || end > uint64(len(memory)) {
		return 0
	}
	stamp := uint64(1469598103934665603)
	for _, b := range memory[start:end] {
		stamp ^= uint64(b)
		stamp *= 1099511628211
	}
	return stamp
}

func ie32DecodedBlockSourceStamp(memory []byte, block []ie32DecodedInstruction, retired int) uint64 {
	if retired <= 0 || retired > len(block) {
		return 0
	}
	stamp := uint64(1469598103934665603)
	for _, in := range block[:retired] {
		start := uint64(in.PC)
		end := start + INSTRUCTION_SIZE
		if end > uint64(len(memory)) {
			return 0
		}
		for _, b := range memory[start:end] {
			stamp ^= uint64(b)
			stamp *= 1099511628211
		}
	}
	return stamp
}

func ie32DecodedBlockSourceRanges(block []ie32DecodedInstruction, retired int) []ie32JITInvalidation {
	if retired <= 0 || retired > len(block) {
		return nil
	}
	ranges := make([]ie32JITInvalidation, 0, retired)
	for _, in := range block[:retired] {
		ranges = append(ranges, ie32JITInvalidation{addr: uint64(in.PC), size: INSTRUCTION_SIZE})
	}
	return ranges
}

func ie32CachedSourceStamp(memory []byte, cached ie32NativeCachedBlock) uint64 {
	if len(cached.sourceRanges) == 0 {
		return ie32BlockSourceStamp(memory, uint32(cached.sourceStart), cached.retired)
	}
	stamp := uint64(1469598103934665603)
	for _, source := range cached.sourceRanges {
		end := source.addr + source.size
		if end > uint64(len(memory)) {
			return 0
		}
		for _, b := range memory[source.addr:end] {
			stamp ^= uint64(b)
			stamp *= 1099511628211
		}
	}
	return stamp
}

func (cpu *CPU) ie32JITTestRetire(count uint64) bool {
	if cpu == nil || cpu.jit == nil || cpu.jit.testStopAfter == 0 || count == 0 {
		return false
	}
	cpu.jit.testRetired += count
	if cpu.jit.testRetired < cpu.jit.testStopAfter {
		return false
	}
	cpu.running.Store(false)
	return true
}

const ie32JITHotBlockThreshold = 2

// ie32JITShouldCompileRegion records a dispatcher-owned block admission. The
// first compilation stays compact; the second uncached execution promotes a
// static-jump candidate to the bounded region scanner.
func (cpu *CPU) ie32JITShouldCompileRegion(pc uint32) bool {
	if cpu == nil || cpu.jit == nil {
		return false
	}
	if cpu.jit.hotBlocks == nil {
		cpu.jit.hotBlocks = make(map[uint32]uint32)
	}
	cpu.jit.hotBlocks[pc]++
	if cpu.jit.hotBlocks[pc] == ie32JITHotBlockThreshold {
		cpu.jit.hotRecompilations.Add(1)
	}
	return cpu.jit.hotBlocks[pc] >= ie32JITHotBlockThreshold
}

func ie32CacheableNativeBlock(block []ie32DecodedInstruction, retired int) bool {
	if retired == 0 || retired > len(block) {
		return false
	}
	for _, in := range block[:retired] {
		if in.AddrMode == ADDR_REG_IND || in.AddrMode == ADDR_MEM_IND {
			return false
		}
		switch in.Opcode {
		case STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW,
			INC, DEC, PUSH, POP, JSR, RTS, RTI:
			return false
		case DIV, MOD:
			// Direct and register divisors were proved non-zero only when this
			// block was emitted. Their values may change without modifying the
			// source bytes, so a retained block could fault (x64) or diverge
			// (ARM64) after a cache hit.
			if in.AddrMode != ADDR_IMMEDIATE {
				return false
			}
		case SHL, SHR:
			// The register count is similarly an admission-time fact. Host
			// variable shifts mask it, while IE32 yields zero for counts >= 32.
			if in.AddrMode == ADDR_REGISTER {
				return false
			}
		}
	}
	return true
}

func (cpu *CPU) drainIE32JITInvalidations() {
	if cpu == nil || cpu.jit == nil {
		return
	}
	gen := cpu.jit.invalidationGeneration.Load()
	if gen == cpu.jit.seenInvalidationGeneration {
		return
	}
	cpu.jit.seenInvalidationGeneration = gen
	cpu.jit.invalidationMu.Lock()
	invalidateAll := cpu.jit.pendingInvalidateAll
	ranges := cpu.jit.pendingInvalidations
	cpu.jit.pendingInvalidateAll = false
	cpu.jit.pendingInvalidations = nil
	cpu.jit.invalidationMu.Unlock()
	if invalidateAll {
		cpu.dropIE32NativeCodeCache()
	} else {
		cpu.invalidateIE32NativeCodeRanges(ranges)
	}
	cpu.jit.invalidations.Add(1)
}

func (cpu *CPU) invalidateIE32NativeCodeRanges(ranges []ie32JITInvalidation) {
	if cpu == nil || cpu.jit == nil || len(ranges) == 0 || len(cpu.jit.nativeCache) == 0 {
		return
	}
	for pc, cached := range cpu.jit.nativeCache {
		for _, invalidation := range ranges {
			if len(cached.sourceRanges) == 0 {
				end := invalidation.addr + invalidation.size
				if invalidation.addr < cached.sourceEnd && end > cached.sourceStart {
					delete(cpu.jit.nativeCache, pc)
					break
				}
				continue
			}
			for _, source := range cached.sourceRanges {
				end := invalidation.addr + invalidation.size
				sourceEnd := source.addr + source.size
				if invalidation.addr < sourceEnd && end > source.addr {
					delete(cpu.jit.nativeCache, pc)
					break
				}
			}
		}
	}
	cpu.jit.returnCache = [2]ie32NativeCachedBlock{}
	// No retained entry can reach this arena after the final cache entry is
	// removed, and draining runs at a dispatcher boundary, so reclaiming it is
	// safe. A later block receives a fresh arena on demand.
	if len(cpu.jit.nativeCache) == 0 && cpu.jit.execMem != nil {
		cpu.jit.execMem.Free()
		cpu.jit.execMem = nil
		cpu.jitMarker = 0
	}
}

func (cpu *CPU) dropIE32NativeCodeCache() {
	if cpu == nil || cpu.jit == nil {
		return
	}
	if cpu.jit.execMem != nil {
		cpu.jit.execMem.Free()
		cpu.jit.execMem = nil
	}
	clear(cpu.jit.nativeCache)
	cpu.jit.returnCache = [2]ie32NativeCachedBlock{}
	cpu.jit.returnCachePending = false
	cpu.jit.invalidationMu.Lock()
	cpu.jit.pendingInvalidations = nil
	cpu.jit.pendingInvalidateAll = false
	cpu.jit.invalidationMu.Unlock()
	cpu.jitMarker = 0
}

// writeIE32NativeCode recovers once from an exhausted CPU-owned arena. It is
// called before the candidate block executes, at a dispatcher boundary.
func (cpu *CPU) writeIE32NativeCode(code []byte) (uintptr, bool) {
	if cpu == nil || cpu.jit == nil || cpu.jit.execMem == nil {
		return 0, false
	}
	addr, err := cpu.jit.execMem.Write(code)
	if err == nil {
		return addr, true
	}
	clear(cpu.jit.nativeCache)
	cpu.jit.execMem.Reset()
	cpu.jitMarker = 0
	cpu.jit.codeCacheResets.Add(1)
	addr, err = cpu.jit.execMem.Write(code)
	return addr, err == nil
}

func (cpu *CPU) SetJITEnabled(enabled bool) error {
	if cpu == nil {
		return fmt.Errorf("IE32 CPU unavailable")
	}
	if cpu.running.Load() {
		return fmt.Errorf("cannot change IE32 JIT while CPU is running")
	}
	if enabled && !ie32JITRuntimeAvailable() {
		return fmt.Errorf("IE32 JIT unavailable on this platform")
	}
	cpu.jitEnabled = enabled
	return nil
}

func (cpu *CPU) JITStats() IE32JITStats {
	if cpu == nil || cpu.jit == nil {
		return IE32JITStats{Backend: "none"}
	}
	backend := "none"
	if ie32JITRuntimeAvailable() {
		backend = ie32JITBackend
	}
	return IE32JITStats{
		Backend:             backend,
		NativeEntries:       cpu.jit.nativeEntries.Load(),
		Blocks:              cpu.jit.blocks.Load(),
		Regions:             cpu.jit.regions.Load(),
		HotRecompilations:   cpu.jit.hotRecompilations.Load(),
		Instructions:        cpu.jit.instructions.Load(),
		DirectInstructions:  cpu.jit.directInstructions.Load(),
		HelperInstructions:  cpu.jit.helperInstructions.Load(),
		Chains:              cpu.jit.chains.Load(),
		ChainBudgetExits:    cpu.jit.chainBudgetExits.Load(),
		Deoptimizations:     cpu.jit.deoptimizations.Load(),
		HelperDeopts:        cpu.jit.helperDeopts.Load(),
		SourceStampDeopts:   cpu.jit.sourceStampDeopts.Load(),
		CodeCacheResets:     cpu.jit.codeCacheResets.Load(),
		Invalidations:       cpu.jit.invalidations.Load(),
		CacheHits:           cpu.jit.cacheHits.Load(),
		ReturnCacheHits:     cpu.jit.returnCacheHits.Load(),
		MMIOPollIterations:  cpu.jit.mmioPollIterations.Load(),
		ResidentSpillsSaved: cpu.jit.residentSpillsSaved.Load(),
		CountedLoops:        cpu.jit.countedLoops.Load(),
	}
}

func (cpu *CPU) resetJITStats() {
	if cpu == nil || cpu.jit == nil {
		return
	}
	cpu.jit.nativeEntries.Store(0)
	cpu.jit.blocks.Store(0)
	cpu.jit.regions.Store(0)
	cpu.jit.hotRecompilations.Store(0)
	cpu.jit.instructions.Store(0)
	cpu.jit.directInstructions.Store(0)
	cpu.jit.helperInstructions.Store(0)
	cpu.jit.chains.Store(0)
	cpu.jit.chainBudgetExits.Store(0)
	cpu.jit.deoptimizations.Store(0)
	cpu.jit.helperDeopts.Store(0)
	cpu.jit.sourceStampDeopts.Store(0)
	cpu.jit.codeCacheResets.Store(0)
	cpu.jit.invalidations.Store(0)
	cpu.jit.cacheHits.Store(0)
	cpu.jit.returnCacheHits.Store(0)
	cpu.jit.mmioPollIterations.Store(0)
	cpu.jit.residentSpillsSaved.Store(0)
	cpu.jit.countedLoops.Store(0)
	clear(cpu.jit.nativeCache)
	cpu.jit.returnCache = [2]ie32NativeCachedBlock{}
	cpu.jit.returnCachePending = false
	clear(cpu.jit.hotBlocks)
	cpu.jit.seenInvalidationGeneration = cpu.jit.invalidationGeneration.Load()
	cpu.dropIE32NativeCodeCache()
}
