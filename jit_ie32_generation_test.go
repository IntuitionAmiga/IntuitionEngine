package main

import "testing"

func TestIE32JIT_BusWritePublishesInvalidationGeneration(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	before := cpu.jit.invalidationGeneration.Load()
	bus.Write8(PROG_START, NOP)
	if got := cpu.jit.invalidationGeneration.Load(); got != before+1 {
		t.Fatalf("generation = %d, want %d", got, before+1)
	}
}

func TestIE32JIT_BusWriteIsDrainedAtNextExecutionBoundary(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if ie32JITBackend == "native" && cpu.jitMarker == 0 {
		t.Fatal("missing initial generated entry")
	}
	bus.Write8(PROG_START, NOP)
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	if got := cpu.jit.invalidations.Load(); got != 1 {
		t.Fatalf("drained invalidations = %d, want 1", got)
	}
	if ie32JITBackend == "native" && cpu.jitMarker == 0 {
		t.Fatal("invalidation did not rebuild generated entry")
	}
}

func TestIE32JIT_ExternalWriteDropsPureBlockCache(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if len(cpu.jit.nativeCache) != 1 {
		t.Fatalf("initial pure cache entries=%d, want 1", len(cpu.jit.nativeCache))
	}
	bus.Write8(PROG_START, NOP)
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	if got := cpu.jit.invalidations.Load(); got != 1 {
		t.Fatalf("invalidations=%d, want 1", got)
	}
	if got := cpu.JITStats().CacheHits; got != 0 {
		t.Fatalf("stale cache hit count=%d, want 0", got)
	}
	if cpu.A != 1 {
		t.Fatalf("stale cached LDA executed after write, A=%d", cpu.A)
	}
}

func TestIE32JIT_NonOverlappingWriteRetainsPureBlockCache(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	first := uint32(PROG_START)
	second := first + 4*INSTRUCTION_SIZE
	putIE32Instruction(cpu.memory, first, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[first+INSTRUCTION_SIZE] = HALT
	putIE32Instruction(cpu.memory, second, LDA, 0, ADDR_IMMEDIATE, 2)
	cpu.memory[second+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	cpu.PC = second
	cpu.running.Store(true)
	cpu.Execute()
	if len(cpu.jit.nativeCache) != 2 {
		t.Fatalf("initial pure cache entries=%d, want 2", len(cpu.jit.nativeCache))
	}
	bus.Write8(first, NOP)
	cpu.drainIE32JITInvalidations()
	if _, ok := cpu.jit.nativeCache[first]; ok {
		t.Fatal("overlapping retained block survived invalidation")
	}
	if _, ok := cpu.jit.nativeCache[second]; !ok {
		t.Fatal("non-overlapping retained block was discarded")
	}
	if got := cpu.JITStats().InvalidatedBlocks; got != 1 {
		t.Fatalf("invalidated retained blocks=%d, want 1", got)
	}
}

func TestIE32JIT_DataWritePreservesRetainedReturnCache(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.jit.returnCache[0].addr == 0 {
		t.Fatal("initial retained block did not populate the return cache")
	}
	cpu.Write32(0x8800, 0xC0FFEE)
	cpu.drainIE32JITInvalidations()
	if _, ok := cpu.jit.nativeCache[PROG_START]; !ok {
		t.Fatal("data write evicted an unrelated retained block")
	}
	if cpu.jit.returnCache[0].addr == 0 {
		t.Fatal("data write cleared an unrelated retained return block")
	}
}

func TestIE32JIT_UnrelatedDataWriteSkipsInvalidationPublication(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	before := cpu.jit.invalidationGeneration.Load()
	bus.Write32(0x8800, 0xC0FFEE)
	if got := cpu.jit.invalidationGeneration.Load(); got != before {
		t.Fatalf("unrelated data write published generation %d, want %d", got, before)
	}
}

func TestIE32JIT_StaticRegionTracksDiscontiguousSources(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	start := uint32(PROG_START)
	target := start + 3*INSTRUCTION_SIZE
	putIE32Instruction(cpu.memory, start, JMP, 0, ADDR_IMMEDIATE, target)
	putIE32Instruction(cpu.memory, target, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[target+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	cpu.PC = start
	cpu.running.Store(true)
	cpu.Execute()
	if _, ok := cpu.jit.nativeCache[start]; !ok {
		t.Fatal("static region was not retained")
	}
	bus.Write8(start+INSTRUCTION_SIZE, NOP) // skipped by the chased JMP
	cpu.drainIE32JITInvalidations()
	if _, ok := cpu.jit.nativeCache[start]; !ok {
		t.Fatal("write to skipped bytes evicted static region")
	}
	bus.Write8(target, NOP)
	cpu.drainIE32JITInvalidations()
	if _, ok := cpu.jit.nativeCache[start]; ok {
		t.Fatal("write to emitted target bytes retained static region")
	}
}

func TestIE32JIT_DynamicWriteInvalidatesEveryRetainedBlock(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if len(cpu.jit.nativeCache) != 1 {
		t.Fatalf("initial pure cache entries=%d, want 1", len(cpu.jit.nativeCache))
	}
	// A zero-sized publication represents an unresolved generated destination.
	cpu.publishIE32JITWrite(0, 0)
	cpu.drainIE32JITInvalidations()
	if len(cpu.jit.nativeCache) != 0 {
		t.Fatal("dynamic write retained a pure block cache entry")
	}
}

func TestIE32JIT_SourceWriteClearsTransientFragmentFallback(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	cpu.noteIE32TransientFragment(PROG_START)
	cpu.publishIE32JITWrite(PROG_START, INSTRUCTION_SIZE)
	cpu.drainIE32JITInvalidations()
	if cpu.jit.transientFragments[PROG_START] != 0 {
		t.Fatal("source rewrite retained transient-fragment fallback")
	}
}

func TestIE32JIT_CPUWriteDropsPureBlockCache(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if len(cpu.jit.nativeCache) != 1 {
		t.Fatalf("initial pure cache entries=%d, want 1", len(cpu.jit.nativeCache))
	}
	cpu.Write32(PROG_START, uint32(NOP))
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	if got := cpu.jit.invalidations.Load(); got != 1 {
		t.Fatalf("invalidations=%d, want 1", got)
	}
	if got := cpu.JITStats().CacheHits; got != 0 {
		t.Fatalf("stale CPU-write cache hit count=%d, want 0", got)
	}
}

func TestIE32JIT_SourceStampRejectsUnpublishedCodeWrite(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if len(cpu.jit.nativeCache) != 1 {
		t.Fatalf("initial pure cache entries=%d, want 1", len(cpu.jit.nativeCache))
	}
	if got := cpu.JITStats().SourceStampDeopts; got != 0 {
		t.Fatalf("initial execution source-stamp deoptimizations=%d, want 0", got)
	}
	// Simulate a faulty raw host writer: no CPU or MachineBus publication.
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 2)
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	if cpu.A != 2 {
		t.Fatalf("stale source executed A=%d, want 2", cpu.A)
	}
	if got := cpu.JITStats().CacheHits; got != 0 {
		t.Fatalf("source-stamp mismatch cache hits=%d, want 0", got)
	}
	if got := cpu.JITStats().SourceStampDeopts; got != 1 {
		t.Fatalf("source-stamp deoptimizations=%d, want 1", got)
	}
	if got := len(cpu.jit.nativeCache); got != 0 {
		t.Fatalf("source-stamp mismatch retained stale native entries=%d", got)
	}
}

func TestIE32CachedSourceMatchesRejectsChangedDiscontiguousInstruction(t *testing.T) {
	memory := make([]byte, 0x400)
	first := ie32JITInvalidation{addr: 0x100, size: INSTRUCTION_SIZE}
	second := ie32JITInvalidation{addr: 0x200, size: INSTRUCTION_SIZE}
	for index := uint64(0); index < INSTRUCTION_SIZE; index++ {
		memory[first.addr+index] = byte(index + 1)
		memory[second.addr+index] = byte(index + 17)
	}
	cached := ie32NativeCachedBlock{sourceRanges: []ie32JITInvalidation{first, second}}
	cached.sourceSnapshot = ie32SourceSnapshot(memory, cached.sourceRanges)
	if !ie32CachedSourceMatches(memory, cached) {
		t.Fatal("fresh source snapshot was rejected")
	}
	memory[second.addr+3] ^= 0xFF
	if ie32CachedSourceMatches(memory, cached) {
		t.Fatal("changed discontiguous source instruction was accepted")
	}
}

func TestIE32JITDispatchCacheHintIsOneShotAndPCBound(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	cached := ie32NativeCachedBlock{pc: PROG_START, retired: 3}
	cpu.noteIE32DispatchCacheHint(cached)
	if got, ok := cpu.takeIE32DispatchCacheHint(PROG_START); !ok || got.retired != 3 {
		t.Fatalf("dispatch cache hint=%+v ok=%v, want retained entry", got, ok)
	}
	if _, ok := cpu.takeIE32DispatchCacheHint(PROG_START); ok {
		t.Fatal("dispatch cache hint was reused")
	}
	cpu.noteIE32DispatchCacheHint(cached)
	if _, ok := cpu.takeIE32DispatchCacheHint(PROG_START + INSTRUCTION_SIZE); ok {
		t.Fatal("dispatch cache hint matched a different PC")
	}
}

func TestIE32JIT_GeneratedStoreDrainsBeforeChainedTarget(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	target := uint32(PROG_START + 4*INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, target, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[target+INSTRUCTION_SIZE] = HALT
	cpu.PC = target
	cpu.Execute()
	if len(cpu.jit.nativeCache) != 1 {
		t.Fatalf("target pure cache entries=%d, want 1", len(cpu.jit.nativeCache))
	}

	// Rewrite the cached target to NOP and jump to it. The dispatcher must
	// drain the write generation before it attempts the target's cached block.
	cpu.A = uint32(NOP)
	putIE32Instruction(cpu.memory, PROG_START, STORE, REG_A, ADDR_DIRECT, target)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, JMP, 0, ADDR_DIRECT, target)
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	if got := cpu.A; got != uint32(NOP) {
		t.Fatalf("stale target cache executed, A=%#x want NOP", got)
	}
	if got := cpu.jit.invalidations.Load(); got == 0 {
		t.Fatal("generated code write was not drained before target dispatch")
	}
}

func TestIE32JIT_GeneratedFirstIndirectStoreRetainsUnrelatedCache(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	target := uint32(PROG_START + 8*INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, target, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[target+INSTRUCTION_SIZE] = HALT
	cpu.PC = target
	cpu.Execute()
	if _, ok := cpu.jit.nativeCache[target]; !ok {
		t.Fatal("failed to establish unrelated cached block")
	}

	cpu.A = 0xBEEF
	cpu.X = 0x8800
	putIE32Instruction(cpu.memory, PROG_START, STORE, REG_A, ADDR_REG_IND, REG_X)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	if got := cpu.Read32(0x8800); got != 0xBEEF {
		t.Fatalf("generated indirect store=%#x, want %#x", got, uint32(0xBEEF))
	}
	if _, ok := cpu.jit.nativeCache[target]; !ok {
		t.Fatal("exact generated indirect store evicted unrelated cache entry")
	}
}

func TestIE32JIT_SelfOverwritingGeneratedStorePublishesInvalidation(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.A = uint32(NOP)
	putIE32Instruction(cpu.memory, PROG_START, STORE, REG_A, ADDR_DIRECT, PROG_START)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.jit.invalidations.Load(); got != 1 {
		t.Fatalf("self-overwriting store invalidations=%d, want 1", got)
	}
}

func TestIE32JIT_ProgramLoadInvalidatesOtherCPUCache(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	loader := NewCPU(bus)
	cached := NewCPU(bus)
	putIE32Instruction(cached.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cached.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cached.Execute()
	if len(cached.jit.nativeCache) != 1 {
		t.Fatalf("initial pure cache entries=%d, want 1", len(cached.jit.nativeCache))
	}
	image := make([]byte, 2*INSTRUCTION_SIZE)
	image[0] = LDA
	image[ADDRMODE_OFFSET] = ADDR_IMMEDIATE
	image[OPERAND_OFFSET] = 2
	image[INSTRUCTION_SIZE] = HALT
	loader.LoadProgramBytes(image)
	cached.PC = PROG_START
	cached.running.Store(true)
	cached.Execute()
	if cached.A != 2 {
		t.Fatalf("stale cached program executed A=%d, want 2", cached.A)
	}
	if got := cached.jit.invalidations.Load(); got == 0 {
		t.Fatal("program load did not invalidate other IE32 CPU")
	}
}

func TestIE32JIT_ExhaustedCodeCacheResetsAndRetries(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("allocate small executable arena: %v", err)
	}
	defer em.Free()
	cpu.jit.execMem = em
	if _, err := em.Write(make([]byte, 4088)); err != nil {
		t.Fatalf("fill executable arena: %v", err)
	}
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 0x55)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 0x55 {
		t.Fatalf("recovered generated block A=%#x, want %#x", cpu.A, uint32(0x55))
	}
	if got := cpu.JITStats().CodeCacheResets; got != 1 {
		t.Fatalf("code-cache resets=%d, want 1", got)
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("direct instructions=%d, want 1", got)
	}
}

func TestIE32JITStatsReportsInvalidations(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	cpu.jit.invalidations.Store(3)
	if got := cpu.JITStats().Invalidations; got != 3 {
		t.Fatalf("stats invalidations = %d, want 3", got)
	}
}
