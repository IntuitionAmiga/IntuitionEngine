package main

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
)

// IE32JITStats is a stable snapshot of IE32 JIT execution provenance.  The
// fields deliberately describe executed generated-code entries rather than
// compilation intent, so automation cannot mistake interpreter fallbacks for
// JIT coverage.
type IE32JITStats struct {
	Backend                string
	NativeEntries          uint64
	Blocks                 uint64
	Regions                uint64
	HotRecompilations      uint64
	Instructions           uint64
	DirectInstructions     uint64
	HelperInstructions     uint64
	HelperExits            uint64
	HelperResumes          uint64
	Chains                 uint64
	ChainBudgetExits       uint64
	Deoptimizations        uint64
	HelperDeopts           uint64
	SourceStampDeopts      uint64
	CodeCacheResets        uint64
	Invalidations          uint64
	InvalidatedBlocks      uint64
	CacheHits              uint64
	ReturnCacheHits        uint64
	MMIOPollIterations     uint64
	MMIOPollParks          uint64
	MMIOStoreHelpers       uint64
	ResidentSpillsSaved    uint64
	CountedLoops           uint64
	ProfitabilityFallbacks uint64
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
	helperExits                atomic.Uint64
	helperResumes              atomic.Uint64
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
	invalidatedBlocks          atomic.Uint64
	cacheHits                  atomic.Uint64
	returnCacheHits            atomic.Uint64
	mmioPollIterations         atomic.Uint64
	mmioPollParks              atomic.Uint64
	mmioStoreHelpers           atomic.Uint64
	residentSpillsSaved        atomic.Uint64
	countedLoops               atomic.Uint64
	profitabilityFallbacks     atomic.Uint64
	transientFragments         map[uint32]uint8
	execMem                    *ExecMem
	nativeCache                map[uint32]ie32NativeCachedBlock
	nativeSourceLow            uint64
	nativeSourceHigh           uint64
	nativeSourceLowFast        atomic.Uint64
	nativeSourceHighFast       atomic.Uint64
	dispatchCacheHint          ie32NativeCachedBlock
	dispatchCacheHintValid     bool
	returnCache                [2]ie32NativeCachedBlock
	returnCachePending         bool
	resumeAfterHelper          bool
	busUnregister              func()
	// testStopAfter/testRetired provide a bounded guest-retirement controller
	// for tests. testExactRetirement disables retained-cache entries and prevents
	// another chained block from crossing the requested boundary. A non-exact
	// checkpoint permits benchmark runs to measure ordinary cache behaviour.
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
	stackPointer        uint32
	stackPointerGuard   bool
	sourceSnapshot      []byte
	sourceStart         uint64
	sourceEnd           uint64
	sourceRanges        []ie32JITInvalidation
	writeRanges         []ie32JITInvalidation
	returns             bool
}

// ie32CachedBlockWriteRanges records the fixed raw-RAM writes performed by a
// retained block. Cache admission has already rejected dynamic destinations,
// so dispatch can publish these ranges without rescanning the source bytes.
func ie32CachedBlockWriteRanges(block []ie32DecodedInstruction, retired int) []ie32JITInvalidation {
	if retired <= 0 || retired > len(block) {
		return nil
	}
	ranges := make([]ie32JITInvalidation, 0, retired)
	for _, in := range block[:retired] {
		switch in.Opcode {
		case STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
			if in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_REGISTER || in.AddrMode == ADDR_IMMEDIATE {
				ranges = append(ranges, ie32JITInvalidation{addr: uint64(in.Operand), size: WORD_SIZE})
			}
		case INC, DEC:
			if in.AddrMode == ADDR_DIRECT {
				ranges = append(ranges, ie32JITInvalidation{addr: uint64(in.Operand), size: WORD_SIZE})
			}
		}
	}
	return ranges
}

// ie32NativeCacheCandidate reports a block whose entry guards match. The
// lowering path compares the retained source snapshot before generated
// execution, rejecting an unpublished self-modifying write.
func (cpu *CPU) ie32NativeCacheCandidate(pc uint32) (ie32NativeCachedBlock, bool) {
	if cpu == nil || cpu.jit == nil || cpu.timerEnabled.Load() || cpu.jit.testExactRetirement || cpu.jit.returnCachePending || cpu.jit.nativeCache == nil {
		return ie32NativeCachedBlock{}, false
	}
	cached, ok := cpu.jit.nativeCache[pc]
	if !ok || cached.stackPointerGuard && cpu.SP != cached.stackPointer {
		return ie32NativeCachedBlock{}, false
	}
	return cached, true
}

// noteIE32DispatchCacheHint passes the entry already selected by the
// dispatcher to the immediate native-lowering call. The hint is CPU-thread
// local and single use, so it cannot outlive a dispatcher boundary.
func (cpu *CPU) noteIE32DispatchCacheHint(cached ie32NativeCachedBlock) {
	if cpu == nil || cpu.jit == nil {
		return
	}
	cpu.jit.dispatchCacheHint = cached
	cpu.jit.dispatchCacheHintValid = true
}

func (cpu *CPU) takeIE32DispatchCacheHint(pc uint32) (ie32NativeCachedBlock, bool) {
	if cpu == nil || cpu.jit == nil || !cpu.jit.dispatchCacheHintValid {
		return ie32NativeCachedBlock{}, false
	}
	cached := cpu.jit.dispatchCacheHint
	cpu.jit.dispatchCacheHint = ie32NativeCachedBlock{}
	cpu.jit.dispatchCacheHintValid = false
	if cached.pc != pc {
		return ie32NativeCachedBlock{}, false
	}
	return cached, true
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
		if cached.addr != 0 && cached.pc == cpu.PC && cpu.ie32CachedBlockUsable(cached) {
			cpu.jit.returnCacheHits.Add(1)
			return cached, true
		}
	}
	return ie32NativeCachedBlock{}, false
}

func (cpu *CPU) ie32CachedBlockUsable(cached ie32NativeCachedBlock) bool {
	if cpu == nil || !ie32CachedSourceMatches(cpu.memory, cached) {
		return false
	}
	return !cached.stackPointerGuard || cpu.SP == cached.stackPointer
}

// ie32JITInvalidation records a physical write. It is queued by arbitrary
// writers but consumed only by the owning IE32 dispatcher.
type ie32JITInvalidation struct {
	addr uint64
	size uint64
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

// ie32SourceSnapshot copies the exact source bytes admitted to a retained
// native block. Exact comparison avoids both stale raw writes and repeated
// per-byte hashing on hot cache entries.
func ie32SourceSnapshot(memory []byte, sources []ie32JITInvalidation) []byte {
	if len(sources) == 0 {
		return nil
	}
	bytesTotal := uint64(0)
	for _, source := range sources {
		end := source.addr + source.size
		if end > uint64(len(memory)) {
			return nil
		}
		bytesTotal += source.size
	}
	snapshot := make([]byte, 0, bytesTotal)
	for _, source := range sources {
		snapshot = append(snapshot, memory[source.addr:source.addr+source.size]...)
	}
	return snapshot
}

// ie32CachedSourceMatches checks every retained instruction byte against its
// compile-time snapshot. Source ranges may be discontiguous after jump chase.
func ie32CachedSourceMatches(memory []byte, cached ie32NativeCachedBlock) bool {
	if len(cached.sourceRanges) == 0 || len(cached.sourceSnapshot) == 0 {
		return false
	}
	offset := uint64(0)
	for _, source := range cached.sourceRanges {
		end := source.addr + source.size
		next := offset + source.size
		if end > uint64(len(memory)) || next > uint64(len(cached.sourceSnapshot)) ||
			!bytes.Equal(memory[source.addr:end], cached.sourceSnapshot[offset:next]) {
			return false
		}
		offset = next
	}
	return offset == uint64(len(cached.sourceSnapshot))
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
	sources := ie32DecodedBlockSourceRanges(block, retired)
	for _, in := range block[:retired] {
		if (in.AddrMode == ADDR_REG_IND && !in.rangeProvenRegisterIndirect) || in.AddrMode == ADDR_MEM_IND {
			return false
		}
		switch in.Opcode {
		case STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
			// A fixed ordinary-RAM destination is rechecked by the lowerer. It
			// may remain cached when it cannot rewrite any source instruction in
			// this block. Indirect stores remain excluded above because their
			// destination can change between entries.
			if ie32WriteOverlapsSourceRanges(in.Operand, WORD_SIZE, sources) {
				return false
			}
		case INC, DEC:
			// Register increments and decrements cannot alter source memory.
			// Fixed RAM forms use the same source-overlap rule as stores.
			if in.AddrMode != ADDR_REGISTER && ie32WriteOverlapsSourceRanges(in.Operand, WORD_SIZE, sources) {
				return false
			}
		case RTI:
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

func ie32BlockUsesFixedStackCursor(block []ie32DecodedInstruction, retired int) bool {
	if retired <= 0 || retired > len(block) {
		return false
	}
	for _, in := range block[:retired] {
		switch in.Opcode {
		case PUSH, POP, JSR, RTS:
			return true
		}
	}
	return false
}

// ie32WriteOverlapsSourceRanges reports whether a fixed generated write can
// alter an instruction that the retained block will execute. The caller has
// already rejected dynamic destinations, so the address remains stable across
// cache entries.
func ie32WriteOverlapsSourceRanges(addr uint32, size uint64, sources []ie32JITInvalidation) bool {
	end := uint64(addr) + size
	for _, source := range sources {
		sourceEnd := source.addr + source.size
		if uint64(addr) < sourceEnd && end > source.addr {
			return true
		}
	}
	return false
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
		cpu.jit.invalidatedBlocks.Add(uint64(len(cpu.jit.nativeCache)))
		cpu.dropIE32NativeCodeCache()
		clear(cpu.jit.transientFragments)
	} else {
		cpu.invalidateIE32NativeCodeRanges(ranges)
		cpu.invalidateIE32TransientFragments(ranges)
	}
	cpu.jit.invalidations.Add(1)
}

// invalidateIE32TransientFragments drops interpreter-fallback markers only
// when a write changes their instruction source. Data writes must not make a
// hot indirect fragment compile again on every loop iteration.
func (cpu *CPU) invalidateIE32TransientFragments(ranges []ie32JITInvalidation) {
	if cpu == nil || cpu.jit == nil || len(ranges) == 0 || len(cpu.jit.transientFragments) == 0 {
		return
	}
	for pc := range cpu.jit.transientFragments {
		for _, invalidation := range ranges {
			end := invalidation.addr + invalidation.size
			if invalidation.addr < uint64(pc)+INSTRUCTION_SIZE && end > uint64(pc) {
				delete(cpu.jit.transientFragments, pc)
				break
			}
		}
	}
}

func (cpu *CPU) invalidateIE32NativeCodeRanges(ranges []ie32JITInvalidation) {
	if cpu == nil || cpu.jit == nil || len(ranges) == 0 || len(cpu.jit.nativeCache) == 0 {
		return
	}
	if cpu.jit.nativeSourceHigh != 0 {
		mayOverlap := false
		for _, invalidation := range ranges {
			end := invalidation.addr + invalidation.size
			if invalidation.addr < cpu.jit.nativeSourceHigh && end > cpu.jit.nativeSourceLow {
				mayOverlap = true
				break
			}
		}
		if !mayOverlap {
			return
		}
	}
	for pc, cached := range cpu.jit.nativeCache {
		for _, invalidation := range ranges {
			if len(cached.sourceRanges) == 0 {
				end := invalidation.addr + invalidation.size
				if invalidation.addr < cached.sourceEnd && end > cached.sourceStart {
					delete(cpu.jit.nativeCache, pc)
					cpu.jit.invalidatedBlocks.Add(1)
					break
				}
				continue
			}
			for _, source := range cached.sourceRanges {
				end := invalidation.addr + invalidation.size
				sourceEnd := source.addr + source.size
				if invalidation.addr < sourceEnd && end > source.addr {
					delete(cpu.jit.nativeCache, pc)
					cpu.jit.invalidatedBlocks.Add(1)
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
		cpu.jit.nativeSourceLow = 0
		cpu.jit.nativeSourceHigh = 0
		cpu.jit.nativeSourceHighFast.Store(0)
		cpu.jit.nativeSourceLowFast.Store(0)
	}
}

// noteIE32NativeCacheSource records a conservative span around retained
// source bytes. It permits data writes outside executable source to bypass a
// full cache walk while leaving source rewrites on the exact-range path.
func (cpu *CPU) noteIE32NativeCacheSource(cached ie32NativeCachedBlock) {
	if cpu == nil || cpu.jit == nil {
		return
	}
	low, high := cached.sourceStart, cached.sourceEnd
	for _, source := range cached.sourceRanges {
		if low == 0 || source.addr < low {
			low = source.addr
		}
		if end := source.addr + source.size; end > high {
			high = end
		}
	}
	if low == 0 || high <= low {
		return
	}
	if cpu.jit.nativeSourceLow == 0 || low < cpu.jit.nativeSourceLow {
		cpu.jit.nativeSourceLow = low
	}
	if high > cpu.jit.nativeSourceHigh {
		cpu.jit.nativeSourceHigh = high
	}
	// Publishing the low bound before the high bound makes an intervening bus
	// callback conservatively retain the wider previous span. The callback
	// never skips a write until both bounds describe a retained source range.
	cpu.jit.nativeSourceLowFast.Store(cpu.jit.nativeSourceLow)
	cpu.jit.nativeSourceHighFast.Store(cpu.jit.nativeSourceHigh)
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
	cpu.jit.dispatchCacheHint = ie32NativeCachedBlock{}
	cpu.jit.dispatchCacheHintValid = false
	cpu.jit.nativeSourceLow = 0
	cpu.jit.nativeSourceHigh = 0
	cpu.jit.nativeSourceHighFast.Store(0)
	cpu.jit.nativeSourceLowFast.Store(0)
	cpu.jit.returnCache = [2]ie32NativeCachedBlock{}
	cpu.jit.returnCachePending = false
	cpu.jit.resumeAfterHelper = false
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
	cpu.jit.dispatchCacheHint = ie32NativeCachedBlock{}
	cpu.jit.dispatchCacheHintValid = false
	cpu.jit.nativeSourceLow = 0
	cpu.jit.nativeSourceHigh = 0
	cpu.jit.nativeSourceHighFast.Store(0)
	cpu.jit.nativeSourceLowFast.Store(0)
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
		Backend:                backend,
		NativeEntries:          cpu.jit.nativeEntries.Load(),
		Blocks:                 cpu.jit.blocks.Load(),
		Regions:                cpu.jit.regions.Load(),
		HotRecompilations:      cpu.jit.hotRecompilations.Load(),
		Instructions:           cpu.jit.instructions.Load(),
		DirectInstructions:     cpu.jit.directInstructions.Load(),
		HelperInstructions:     cpu.jit.helperInstructions.Load(),
		HelperExits:            cpu.jit.helperExits.Load(),
		HelperResumes:          cpu.jit.helperResumes.Load(),
		Chains:                 cpu.jit.chains.Load(),
		ChainBudgetExits:       cpu.jit.chainBudgetExits.Load(),
		Deoptimizations:        cpu.jit.deoptimizations.Load(),
		HelperDeopts:           cpu.jit.helperDeopts.Load(),
		SourceStampDeopts:      cpu.jit.sourceStampDeopts.Load(),
		CodeCacheResets:        cpu.jit.codeCacheResets.Load(),
		Invalidations:          cpu.jit.invalidations.Load(),
		InvalidatedBlocks:      cpu.jit.invalidatedBlocks.Load(),
		CacheHits:              cpu.jit.cacheHits.Load(),
		ReturnCacheHits:        cpu.jit.returnCacheHits.Load(),
		MMIOPollIterations:     cpu.jit.mmioPollIterations.Load(),
		MMIOPollParks:          cpu.jit.mmioPollParks.Load(),
		MMIOStoreHelpers:       cpu.jit.mmioStoreHelpers.Load(),
		ResidentSpillsSaved:    cpu.jit.residentSpillsSaved.Load(),
		CountedLoops:           cpu.jit.countedLoops.Load(),
		ProfitabilityFallbacks: cpu.jit.profitabilityFallbacks.Load(),
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
	cpu.jit.helperExits.Store(0)
	cpu.jit.helperResumes.Store(0)
	cpu.jit.chains.Store(0)
	cpu.jit.chainBudgetExits.Store(0)
	cpu.jit.deoptimizations.Store(0)
	cpu.jit.helperDeopts.Store(0)
	cpu.jit.sourceStampDeopts.Store(0)
	cpu.jit.codeCacheResets.Store(0)
	cpu.jit.invalidations.Store(0)
	cpu.jit.invalidatedBlocks.Store(0)
	cpu.jit.cacheHits.Store(0)
	cpu.jit.returnCacheHits.Store(0)
	cpu.jit.mmioPollIterations.Store(0)
	cpu.jit.mmioPollParks.Store(0)
	cpu.jit.mmioStoreHelpers.Store(0)
	cpu.jit.residentSpillsSaved.Store(0)
	cpu.jit.countedLoops.Store(0)
	cpu.jit.profitabilityFallbacks.Store(0)
	clear(cpu.jit.nativeCache)
	cpu.jit.dispatchCacheHint = ie32NativeCachedBlock{}
	cpu.jit.dispatchCacheHintValid = false
	cpu.jit.returnCache = [2]ie32NativeCachedBlock{}
	cpu.jit.returnCachePending = false
	cpu.jit.resumeAfterHelper = false
	clear(cpu.jit.hotBlocks)
	clear(cpu.jit.transientFragments)
	cpu.jit.seenInvalidationGeneration = cpu.jit.invalidationGeneration.Load()
	cpu.dropIE32NativeCodeCache()
}

// ie32JITShouldUseInterpreterForTransientFragment avoids repeatedly lowering
// a direct fragment whose admission depends on mutable guest state. The first
// execution establishes that the fragment is transient; later executions use
// the already efficient architectural interpreter until a cacheable entry is
// reached.
func (cpu *CPU) ie32JITShouldUseInterpreterForTransientFragment(pc uint32) bool {
	if cpu == nil || cpu.jit == nil || cpu.jit.transientFragments[pc] == 0 {
		return false
	}
	cpu.jit.profitabilityFallbacks.Add(1)
	return true
}

func (cpu *CPU) noteIE32TransientFragment(pc uint32) {
	if cpu == nil || cpu.jit == nil {
		return
	}
	if cpu.jit.transientFragments == nil {
		cpu.jit.transientFragments = make(map[uint32]uint8)
	}
	cpu.jit.transientFragments[pc] = 1
}
