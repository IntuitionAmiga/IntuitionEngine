// jit_z80_exec.go - Z80 JIT dispatcher loop and CPU integration
//
// Static Z80 regions are promoted as a bounded set of independently emitted
// blocks with every static JP/JR edge patched before first execution. This
// retains each block's established DJNZ, flag-liveness and SMC exit contract,
// while avoiding the old lazy-only chain formation that could not establish a
// region invariant until a later dispatcher visit.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

import (
	"fmt"
	"time"
	"unsafe"
)

// jitZ80ExecMemSize is the executable memory pool size for Z80 JIT blocks.
// 2MB is sufficient since Z80 blocks are small (1-4 byte instructions).
const jitZ80ExecMemSize = 2 * 1024 * 1024

// These are shared frontend limits. A native or wasm chain must return to the
// dispatcher after either bound so interrupts, debugging and invalidations are
// observed at the same architectural boundary on every backend.
const (
	z80JITChainBlockBudget     uint32 = 64
	z80JITInterruptCycleBudget uint32 = 200
)

// getZ80JITExecMem returns the typed *ExecMem from the cpu's any field.
func (cpu *CPU_Z80) getZ80JITExecMem() *ExecMem {
	if cpu.jitExecMem == nil {
		return nil
	}
	return cpu.jitExecMem.(*ExecMem)
}

// initZ80JIT initializes JIT state on the CPU. Called once before execution.
// Must be called AFTER MachineBus.SealMappings().
func (cpu *CPU_Z80) initZ80JIT(adapter *Z80BusAdapter) error {
	if cpu.jitExecMem != nil {
		return nil // already initialized
	}
	execMem, err := AllocExecMem(jitZ80ExecMemSize)
	if err != nil {
		return fmt.Errorf("Z80 JIT init failed: %w", err)
	}
	cpu.jitExecMem = execMem
	cpu.jitCache = NewCodeCache()
	cpu.jitCtx = newZ80JITContext(cpu, adapter)
	cpu.jitSeenMappingGeneration = adapter.mappingGeneration.Load()
	cpu.jitBusUnregister = adapter.bus.RegisterZ80JITInvalidator(func(addr, size uint64) {
		if adapter.coprocFlat {
			start, end := uint64(adapter.coprocBase), uint64(adapter.coprocBase)+z80AddressSpace
			if addr >= end || addr+size <= start {
				return
			}
			lo, hi := addr, addr+size
			if lo < start {
				lo = start
			}
			if hi > end {
				hi = end
			}
			cpu.noteZ80JITWrite(lo-start, hi-lo)
			return
		}
		cpu.noteZ80JITWrite(addr, size)
	})
	return nil
}

// freeZ80JIT releases all JIT resources. If jitPersist is set (benchmarks),
// the code cache and exec memory are kept alive for reuse across runs.
func (cpu *CPU_Z80) freeZ80JIT() {
	if cpu.jitPersist {
		return
	}
	if cpu.jitExecMem != nil {
		if em, ok := cpu.jitExecMem.(*ExecMem); ok {
			em.Free()
		}
		cpu.jitExecMem = nil
	}
	cpu.jitCache = nil
	cpu.jitCtx = nil
	if cpu.jitBusUnregister != nil {
		cpu.jitBusUnregister()
		cpu.jitBusUnregister = nil
	}
}

// noteZ80JITWrite publishes the affected physical 256-byte pages. It is safe
// for every bus writer and never mutates a cache from that writer's goroutine.
func (cpu *CPU_Z80) noteZ80JITWrite(addr, size uint64) {
	if size == 0 || addr > 0xFFFF {
		return
	}
	end := addr + size - 1
	if end < addr {
		return
	}
	if end > 0xFFFF {
		end = 0xFFFF
	}
	cpu.jitDispatchGeneration.Add(1)
	cpu.jitPendingMu.Lock()
	if cpu.jitPendingPages == nil {
		cpu.jitPendingPages = make(map[uint8]struct{})
	}
	for page := addr >> 8; page <= end>>8; page++ {
		cpu.jitCodeGeneration[page].Add(1)
		cpu.jitPendingPages[uint8(page)] = struct{}{}
	}
	cpu.jitHasPending.Store(true)
	cpu.jitPendingMu.Unlock()
}

// drainZ80JITInvalidations applies generation changes at a dispatcher
// boundary. This is the only place an owning Z80 CPU mutates its cache in
// response to a physical write.
func (cpu *CPU_Z80) drainZ80JITInvalidations(ctx *Z80JITContext) {
	if !cpu.jitHasPending.Load() {
		return
	}
	cpu.jitPendingMu.Lock()
	pending := cpu.jitPendingPages
	cpu.jitPendingPages = nil
	cpu.jitHasPending.Store(false)
	cpu.jitPendingMu.Unlock()
	for queued := range pending {
		page := int(queued)
		generation := cpu.jitCodeGeneration[page].Load()
		if generation == cpu.jitSeenCodeGeneration[page] {
			continue
		}
		cpu.jitSeenCodeGeneration[page] = generation
		lo := uint64(page << 8)
		hi := lo + 256
		cpu.jitCache.UnpatchChainsInRange(lo, hi)
		cpu.jitCache.InvalidateRange(lo, hi)
		cpu.codePageBitmap[page] = 0
		cpu.jitStats.invalidations.Add(1)
		ctx.RTSCache0PC, ctx.RTSCache0Addr = 0, 0
		ctx.RTSCache1PC, ctx.RTSCache1Addr = 0, 0
	}
}

func (cpu *CPU_Z80) drainZ80JITMappingChange(adapter *Z80BusAdapter, ctx *Z80JITContext) {
	generation := adapter.mappingGeneration.Load()
	if generation == cpu.jitSeenMappingGeneration {
		return
	}
	cpu.jitSeenMappingGeneration = generation
	cpu.z80JITFlushAll(ctx)
	cpu.jitStats.invalidations.Add(1)
}

func z80BlockSourceMatches(mem []byte, pc uint16, block *JITBlock) bool {
	if block == nil || len(block.z80Source) == 0 || uint64(pc) != block.startPC {
		return false
	}
	for i, value := range block.z80Source {
		if mem[uint16(uint32(pc)+uint32(i))] != value {
			return false
		}
	}
	return true
}

func z80CaptureBlockGenerations(cpu *CPU_Z80, adapter *Z80BusAdapter, block *JITBlock) {
	block.z80MappingGeneration = adapter.mappingGeneration.Load()
	for offset := range block.z80Source {
		page := uint16(block.startPC+uint64(offset)) >> 8
		if !block.z80PhysicalCodePages[page] {
			block.z80PhysicalCodePages[page] = true
			block.z80CodePages = append(block.z80CodePages, uint8(page))
		}
		block.z80PageGenerations[page] = cpu.jitCodeGeneration[page].Load()
	}
}

func z80BlockGenerationsMatch(cpu *CPU_Z80, adapter *Z80BusAdapter, block *JITBlock) bool {
	if block.z80MappingGeneration != adapter.mappingGeneration.Load() {
		return false
	}
	for _, queued := range block.z80CodePages {
		page := int(queued)
		if block.z80PageGenerations[page] != cpu.jitCodeGeneration[page].Load() {
			return false
		}
	}
	return true
}

// z80PromoteStaticRegion eagerly compiles and patches a static JP/JR chain.
// It is deliberately a chain of normal JITBlocks, not a concatenated emitter
// buffer: every constituent retains its source stamp, physical page snapshots,
// mapping generation, loop precheck and existing bounded native exit. The
// shared Z80 profile limits a promoted region to four blocks and 128 guest
// instructions.
func (cpu *CPU_Z80) z80PromoteStaticRegion(adapter *Z80BusAdapter, mem []byte, startPC uint16) int {
	if cpu.jitCache == nil || cpu.getZ80JITExecMem() == nil || cpu.directPageBitmap[startPC>>8] != 0 {
		return 0
	}
	pcs := z80FrontendRegionPlan(
		func(pc uint16) byte { return mem[pc] },
		func(pc uint16) bool { return cpu.directPageBitmap[pc>>8] == 0 },
		z80NativeFrontendAdmits,
		startPC,
	)
	if len(pcs) == 0 {
		return 0
	}

	for _, pc := range pcs {
		if cached := cpu.jitCache.Get(uint64(pc)); cached != nil &&
			z80BlockSourceMatches(mem, pc, cached) && z80BlockGenerationsMatch(cpu, adapter, cached) {
			continue
		}
		instrs := z80JITScanBlock(mem, pc, len(mem), &cpu.directPageBitmap)
		if len(instrs) == 0 {
			return 0
		}
		block, err := compileBlockZ80(instrs, pc, cpu.getZ80JITExecMem(), &cpu.codePageBitmap)
		if err != nil {
			return 0
		}
		z80CaptureBlockGenerations(cpu, adapter, block)
		cpu.jitCache.Put(block)
	}
	// Put registers each block's inbound edges, then patch both pre-existing
	// and newly promoted static exits. This is the same CodeCache patch API
	// used by lazy chaining, exercised here before the first native entry.
	for _, pc := range pcs {
		if block := cpu.jitCache.Get(uint64(pc)); block != nil && block.chainEntry != 0 {
			cpu.jitCache.PatchChainsTo(block.startPC, block.chainEntry)
		}
	}
	cpu.jitStats.regionPromotions.Add(1)
	return len(pcs)
}

// z80NativeFrontendAdmits adapts the backend-neutral decoded payload to the
// established native lowering policy. DDCB/FDCB is represented differently
// by the immutable helper ABI and native emitter, so restore its CB marker.
func z80NativeFrontendAdmits(payload z80CanonicalHelperPayload) bool {
	instr := JITZ80Instr{
		opcode: payload.Opcode, prefix: payload.Prefix, displacement: payload.Displacement,
		operand: payload.Operand, hasOperand: payload.Length > 1, length: payload.Length,
		rIncrements: payload.RIncrements,
	}
	if (payload.Prefix == z80JITPrefixDD || payload.Prefix == z80JITPrefixFD) && payload.Bytes[1] == z80JITPrefixCB {
		instr.opcode, instr.cbSubOp = z80JITPrefixCB, payload.Opcode
	}
	return !z80JITNeedsFallback(&instr) && z80JITCanEmit(&instr)
}

// interpretZ80One executes one Z80 instruction at cpu.PC using the interpreter.
// This calls cpu.Step() which handles the full fetch/decode/execute cycle including:
// - fetchOpcode() advancing PC and incrementing R
// - prefix handlers with their own fetchOpcode() calls and R increments
// - finishInstruction() processing iffDelay for EI
// Bypassing Step() would corrupt PC, R, and delayed interrupt enable state.
func (cpu *CPU_Z80) interpretZ80One() {
	cpu.Step()
}

// z80JITFlushAll invalidates compiled state without immediately recycling its
// executable addresses. This avoids stale ARM64 instruction-cache aliases
// when mapping changes repeatedly invalidate the cache.
func (cpu *CPU_Z80) z80JITFlushAll(ctx *Z80JITContext) {
	cpu.jitCache.UnpatchChainsInRange(0, 0x10000)
	cpu.jitCache.Invalidate()
	ctx.RTSCache0PC = 0
	ctx.RTSCache0Addr = 0
	ctx.RTSCache1PC = 0
	ctx.RTSCache1Addr = 0
	for i := range cpu.codePageBitmap {
		cpu.codePageBitmap[i] = 0
	}
}

// z80JITResetAll additionally recycles the executable arena after cache
// pressure has proved that fresh addresses are unavailable.
func (cpu *CPU_Z80) z80JITResetAll(ctx *Z80JITContext) {
	cpu.z80JITFlushAll(ctx)
	if em := cpu.getZ80JITExecMem(); em != nil {
		em.Reset()
	}
}

// z80BankWindowsEnabled returns true if any Z80 bank window is active,
// indicating that interpreter writes could alias direct-memory code pages.
func z80BankWindowsEnabled(adapter *Z80BusAdapter) bool {
	return adapter.bank1Enable || adapter.bank2Enable || adapter.bank3Enable || adapter.vramEnabled
}

// ExecuteJITZ80 is the main JIT execution loop for the Z80.
func (cpu *CPU_Z80) ExecuteJITZ80() {
	// ── Resolve the Z80BusAdapter from the bus interface ──
	adapter, ok := cpu.bus.(*Z80BusAdapter)
	if !ok {
		// Not a Z80BusAdapter (e.g. test bus) — fall back to interpreter
		cpu.Execute()
		return
	}

	// ── Pre-loop invariant: seal MachineBus I/O mappings ──
	adapter.bus.SealMappings()

	// ── Initialize JIT (after SealMappings so ioPageBitmap is stable) ──
	if err := cpu.initZ80JIT(adapter); err != nil {
		fmt.Printf("Z80 JIT: %v, falling back to interpreter\n", err)
		cpu.Execute()
		return
	}
	defer cpu.freeZ80JIT()

	// AddressIsMMIOPredicate is set per call from the live bus at each
	// TryFastMMIOPoll site; wiring the shared global Z80PollPattern here is dead
	// and races across concurrent CPUs. See jit_exec.go.

	execMem := cpu.getZ80JITExecMem()
	ctx := cpu.jitCtx.(*Z80JITContext)
	mem := adapter.jitMemory()
	memSize := len(mem)

	// Performance measurement
	perfEnabled := cpu.PerfEnabled
	if perfEnabled {
		cpu.perfStartTime = time.Now()
		cpu.lastPerfReport = cpu.perfStartTime
		cpu.InstructionCount = 0
	}

	// Diagnostic counters
	var diagCacheHits uint64
	var diagCacheMisses uint64
	var diagFallbackInstr uint64
	var pendingNativeEntries uint64
	var pendingChainExits uint64
	flushHotStats := func() {
		if pendingNativeEntries != 0 {
			cpu.jitStats.nativeEntries.Add(pendingNativeEntries)
			pendingNativeEntries = 0
		}
		if pendingChainExits != 0 {
			cpu.jitStats.chainExits.Add(pendingChainExits)
			pendingChainExits = 0
		}
	}
	defer flushHotStats()

	interpretFallback := func(reason DeoptReason) {
		var interpT0 time.Time
		if perfAcctOn {
			interpT0 = time.Now()
		}
		cpu.interpretZ80One()
		if perfAcctOn {
			cpu.perfAcct.AddInterpSince(interpT0)
			cpu.perfAcct.AddInstrs(1)
			cpu.deoptStats.Add(reason)
		}
	}

	for cpu.running.Load() {
		if cpu.executionBoundary != nil {
			cpu.executionBoundary()
			if !cpu.running.Load() {
				break
			}
		}
		cpu.drainZ80JITInvalidations(ctx)
		cpu.drainZ80JITMappingChange(adapter, ctx)
		if cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			break
		}
		// ── Interrupt checks (matching interpreter order in cpu_z80.go:261-283) ──

		// NMI (highest priority)
		if cpu.nmiPending.Load() {
			cpu.serviceNMI()
			continue
		}

		// IRQ (if enabled)
		if cpu.irqLine.Load() && cpu.IFF1 {
			cpu.serviceIRQ()
			continue
		}

		// A bounded runner observes HALT at the next execution boundary and may
		// assert an interrupt there. Match Step's halted polling so that callback
		// gets another boundary; ordinary callers still return on terminal HALT.
		if cpu.Halted {
			if cpu.executionBoundary != nil {
				cpu.tick(4)
				continue
			}
			break
		}

		// ── EI delay handling ──
		// If iffDelay > 0, EI was recently executed. The interpreter's
		// finishInstruction() decrements iffDelay per instruction. Since EI is
		// a block terminator, iffDelay was set by the last native block.
		// Execute exactly one instruction via interpreter so finishInstruction()
		// can process the countdown with per-instruction accuracy.
		if cpu.iffDelay > 0 {
			interpretFallback(DeoptInterrupt)
			diagFallbackInstr++
			if perfEnabled {
				cpu.InstructionCount++
			}
			// After interpretZ80One, Step→finishInstruction decremented iffDelay.
			// If it reached 0, IFF1/IFF2 are now enabled. Next loop iteration
			// will check IRQ.
			if !cpu.running.Load() {
				break
			}
			continue
		}

		// ── PC page safety check ──
		// If the current PC is on a non-direct page (banked, VRAM, I/O),
		// the JIT scanner can't safely read opcodes from raw MachineBus memory.
		// Fall back to interpreter for this instruction.
		pc := cpu.PC
		if cpu.directPageBitmap[pc>>8] != 0 {
			if !cpu.jitSingleStep {
				if matched, retired, rInc := cpu.tryFastZ80MMIOPollLoop(adapter); matched {
					if rInc > 0 {
						r := cpu.R
						cpu.R = (r & 0x80) | ((r + byte(rInc)) & 0x7F)
					}
					if perfEnabled {
						cpu.InstructionCount += uint64(retired)
					}
					if perfAcctOn {
						cpu.perfAcct.AddInstrs(uint64(retired))
					}
					continue
				}
			}
			interpretFallback(DeoptMMIO)
			diagFallbackInstr++
			if perfEnabled {
				cpu.InstructionCount++
			}
			if !cpu.running.Load() {
				break
			}
			continue
		}

		// ── Block lookup ──
		block := cpu.jitCache.Get(uint64(pc))
		// Every MachineBus mutation publishes the physical page generation after
		// storing its bytes. A matching immutable generation snapshot therefore
		// proves the captured source stamp still applies without rescanning every
		// source byte on each hot dispatch.
		if block != nil && !z80BlockGenerationsMatch(cpu, adapter, block) {
			cpu.jitCache.UnpatchChainsInRange(block.startPC, block.endPC)
			cpu.jitCache.InvalidateRange(block.startPC, block.endPC)
			block = nil
		}
		if block == nil && !cpu.jitSingleStep {
			// Static region promotion compiles the complete bounded JP/JR chain
			// before its first entry. If the shape is not eligible, retain the
			// ordinary one-block compilation path below.
			if cpu.z80PromoteStaticRegion(adapter, mem, pc) > 0 {
				block = cpu.jitCache.Get(uint64(pc))
			}
		}
		if block == nil {
			// Scan block from raw memory (safe: PC is on a direct page)
			instrs := z80JITScanBlock(mem, pc, memSize, &cpu.directPageBitmap)
			if cpu.jitSingleStep && len(instrs) > 1 {
				instrs = instrs[:1]
			}

			if len(instrs) == 0 {
				// The scanner stopped at a form that needs host observation or has
				// no native lowering. Execute its immutable decoded image through
				// the canonical helper, rather than re-reading guest code through
				// the interpreter dispatch.
				payload := z80CanonicalHelperPayloadFromFetch(adapter.fetchRead, pc)
				if !z80CanonicalHelperPayloadComplete(payload) {
					interpretFallback(DeoptUnsupported)
					diagFallbackInstr++
				} else {
					payload.ExitReason = uint32(DeoptUnsupported)
					ctx.HelperPayload = payload
					var helperT0 time.Time
					if perfAcctOn {
						helperT0 = time.Now()
					}
					cpu.executeZ80CanonicalHelper(ctx.HelperPayload)
					if perfAcctOn {
						cpu.perfAcct.AddInterpSince(helperT0)
						cpu.perfAcct.AddInstrs(1)
					}
					if payload.Opcode != 0x76 || payload.Prefix != z80JITPrefixNone {
						cpu.jitStats.helperExits.Add(1)
					}
				}
				if perfEnabled {
					cpu.InstructionCount++
				}
				if !cpu.running.Load() {
					break
				}
				continue
			}

			// Compile the block
			var err error
			block, err = compileBlockZ80(instrs, pc, execMem, &cpu.codePageBitmap)
			if err != nil {
				// ExecMem likely exhausted — full reset and retry once
				cpu.z80JITResetAll(ctx)
				block, err = compileBlockZ80(instrs, pc, execMem, &cpu.codePageBitmap)
				if err != nil {
					// A backend without a lowering executes this form through the
					// frozen canonical helper. A genuine allocation failure also
					// remains semantically correct through that same path.
					payload := z80CanonicalHelperPayloadFromFetch(adapter.fetchRead, pc)
					if !z80CanonicalHelperPayloadComplete(payload) {
						interpretFallback(DeoptCachePressure)
						diagFallbackInstr++
					} else {
						payload.ExitReason = uint32(DeoptCachePressure)
						ctx.HelperPayload = payload
						var helperT0 time.Time
						if perfAcctOn {
							helperT0 = time.Now()
						}
						cpu.executeZ80CanonicalHelper(ctx.HelperPayload)
						if perfAcctOn {
							cpu.perfAcct.AddInterpSince(helperT0)
							cpu.perfAcct.AddInstrs(1)
						}
						if payload.Opcode != 0x76 || payload.Prefix != z80JITPrefixNone {
							cpu.jitStats.helperExits.Add(1)
						}
					}
					if perfEnabled {
						cpu.InstructionCount++
					}
					if !cpu.running.Load() {
						break
					}
					continue
				}
			}
			z80CaptureBlockGenerations(cpu, adapter, block)
			cpu.jitCache.Put(block)

			// Bidirectional chain patching
			if block.chainEntry != 0 {
				cpu.jitCache.PatchChainsTo(block.startPC, block.chainEntry)
			}
			for i := range block.chainSlots {
				slot := &block.chainSlots[i]
				if target := cpu.jitCache.Get(slot.targetPC); target != nil && target.chainEntry != 0 {
					PatchRel32At(slot.patchAddr, target.chainEntry)
				}
			}

			diagCacheMisses++
		} else {
			diagCacheHits++
		}

		// Update 2-entry MRU RTS cache before execution
		if block.chainEntry != 0 {
			ctx.RTSCache1PC = ctx.RTSCache0PC
			ctx.RTSCache1Addr = ctx.RTSCache0Addr
			ctx.RTSCache0PC = uint32(block.startPC)
			ctx.RTSCache0Addr = block.chainEntry
		}

		// Initialize chain state for this entry into native code.
		// All chain accumulators start at 0; native code ADDs to them.
		ctx.ChainBudget = z80JITChainBlockBudget
		if cpu.jitSingleStep {
			ctx.ChainBudget = 1
		}
		ctx.ExpectedDispatchGeneration = cpu.jitDispatchGeneration.Load()
		ctx.ExpectedMappingGeneration = adapter.mappingGeneration.Load()
		ctx.ChainCount = 0
		ctx.ChainCycles = 0
		ctx.ChainRIncrements = 0
		ctx.CycleBudget = z80JITInterruptCycleBudget

		// Execute the native code block (may chain across multiple blocks)
		var jitT0 time.Time
		if perfAcctOn {
			jitT0 = time.Now()
		}
		pendingNativeEntries++
		if pendingNativeEntries == 64 {
			cpu.jitStats.nativeEntries.Add(pendingNativeEntries)
			pendingNativeEntries = 0
		}
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
		if perfAcctOn {
			cpu.perfAcct.AddJitSince(jitT0)
		}
		block.execCount++

		// ── Process block results ──
		// RetPC/RetCount/RetCycles were committed by the exit path
		// (either chain exit unchained fallback, bail, selfmod, or plain epilogue).
		// All exits merge chained state before returning.
		cpu.PC = uint16(ctx.RetPC)
		cpu.Cycles += ctx.RetCycles
		cpu.bus.Tick(int(ctx.RetCycles))
		ctx.RetCycles = 0
		executed := ctx.RetCount
		ctx.RetCount = 0
		if ctx.ChainCount > uint32(block.instrCount) {
			pendingChainExits++
			if pendingChainExits == 64 {
				cpu.jitStats.chainExits.Add(pendingChainExits)
				pendingChainExits = 0
			}
		}

		// ── Handle NeedInval (self-mod: page-granular invalidation) ──
		if ctx.NeedInval != 0 {
			cpu.jitStats.invalidations.Add(1)
			recordBlockDeopt(&cpu.deoptStats, block, DeoptSMC)
			page := ctx.InvalPage
			lo := page << 8
			hi := lo + 256
			// Unpatch chain slots targeting invalidated range, then remove blocks
			cpu.jitCache.UnpatchChainsInRange(uint64(lo), uint64(hi))
			cpu.jitCache.InvalidateRange(uint64(lo), uint64(hi))
			ctx.NeedInval = 0
			// Clear RTS cache (invalidated blocks may have had chain entries)
			ctx.RTSCache0PC = 0
			ctx.RTSCache0Addr = 0
			ctx.RTSCache1PC = 0
			ctx.RTSCache1Addr = 0
		}

		// ── Handle NeedBail (re-execute current instruction via interpreter) ──
		if ctx.NeedBail != 0 {
			cpu.jitStats.bailouts.Add(1)
			ctx.NeedBail = 0
			recordBlockDeopt(&cpu.deoptStats, block, DeoptMMIO)
			// cpu.PC was already set to the bailing instruction's start PC.
			// Preserve its decoded image across this observation boundary: a
			// direct-page guard must not re-fetch mutable guest code.
			var interpT0 time.Time
			if perfAcctOn {
				interpT0 = time.Now()
			}
			payload := z80CanonicalHelperPayloadFromFetch(adapter.fetchRead, cpu.PC)
			if z80CanonicalHelperPayloadComplete(payload) {
				payload.ExitReason = uint32(DeoptMMIO)
				ctx.HelperPayload = payload
				cpu.executeZ80CanonicalHelper(ctx.HelperPayload)
				cpu.jitStats.helperExits.Add(1)
			} else {
				cpu.interpretZ80One()
			}
			if perfAcctOn {
				cpu.perfAcct.AddInterpSince(interpT0)
			}
			executed++
			if !cpu.running.Load() {
				break
			}
			// Z80BusAdapter resolves a banked write to its physical address before
			// MachineBus publishes it. The owning dispatcher drains that exact page
			// on the next boundary, so no conservative whole-cache flush is needed.
		}

		// ── Update R register ──
		// R was not updated by native code. ChainRIncrements accumulates
		// R increments from ALL blocks executed during this native run
		// (including chained blocks). Each exit path ADDs its block's
		// rIncrements to ChainRIncrements before returning to Go.
		rInc := ctx.ChainRIncrements
		ctx.ChainRIncrements = 0
		if rInc > 0 {
			r := cpu.R
			cpu.R = (r & 0x80) | ((r + byte(rInc)) & 0x7F)
		}

		// ── Bookkeeping ──
		if perfAcctOn {
			cpu.perfAcct.AddInstrs(uint64(executed))
		}
		if perfEnabled {
			cpu.InstructionCount += uint64(executed)

			now := time.Now()
			if now.Sub(cpu.lastPerfReport) >= time.Second {
				elapsed := now.Sub(cpu.perfStartTime).Seconds()
				if elapsed > 0 {
					mips := float64(cpu.InstructionCount) / elapsed / 1_000_000
					hitRate := float64(0)
					total := diagCacheHits + diagCacheMisses
					if total > 0 {
						hitRate = float64(diagCacheHits) / float64(total) * 100
					}
					fallbackPct := float64(0)
					if cpu.InstructionCount > 0 {
						fallbackPct = float64(diagFallbackInstr) / float64(cpu.InstructionCount) * 100
					}
					fmt.Printf("\rZ80 JIT: %.2f MIPS | cache %.0f%% | fallback %.1f%%   ",
						mips, hitRate, fallbackPct)
					cpu.lastPerfReport = now
				}
			}
		}
	}
}
