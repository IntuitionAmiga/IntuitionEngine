// jit_x86_exec.go - x86 JIT execution loop
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

// This is the amd64 x86 JIT dispatcher. Linux/ARM64 has its corresponding
// emitter and dispatcher in the arm64-tagged x86 files.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"time"
	"unsafe"
)

// x86 JIT configuration
const (
	x86JitExecMemSize = 16 * 1024 * 1024 // 16MB executable memory pool
)

// x86GetJITExecMem returns the typed *ExecMem from the cpu's any field.
func (cpu *CPU_X86) x86GetJITExecMem() *ExecMem {
	if cpu.x86JitExecMem == nil {
		return nil
	}
	return cpu.x86JitExecMem.(*ExecMem)
}

// initX86JIT initializes JIT state. Called once before execution.
func (cpu *CPU_X86) initX86JIT() error {
	if cpu.x86JitExecMem != nil {
		return nil // already initialized
	}

	// Ensure memory is available
	if cpu.memory == nil {
		return fmt.Errorf("x86 JIT: cpu.memory not initialized (need X86BusAdapter)")
	}

	execMem, err := AllocExecMem(x86JitExecMemSize)
	if err != nil {
		return fmt.Errorf("x86 JIT init failed: %w", err)
	}
	cpu.x86JitExecMem = execMem
	cpu.x86JitCache = NewCodeCache()

	// Build I/O bitmap (256-byte page granularity)
	// We need the adapter to build the bitmap. If we can't get it,
	// mark everything above a safe threshold as I/O.
	if cpu.x86JitIOBitmap == nil {
		// Default conservative bitmap: mark 0xF000+ and 0xA0000+ as I/O.
		// PLAN_MAX_RAM slice 10g: bus-driven sizing replaces the retired
		// 32 MiB x86AddressSpace constant. cpu.memory is the bus-allocated
		// slice, so its length is the active address space cap.
		bitmapSize := len(cpu.memory) >> 8
		if bitmapSize == 0 {
			bitmapSize = 1
		}
		cpu.x86JitIOBitmap = make([]byte, bitmapSize)
		// Mark translateIO region: 0xF000-0xFFFF
		for addr := uint32(0xF000); addr < 0x10000; addr += 0x100 {
			page := addr >> 8
			if page < uint32(len(cpu.x86JitIOBitmap)) {
				cpu.x86JitIOBitmap[page] = 1
			}
		}
		// Mark VGA VRAM: 0xA0000-0xAFFFF
		for addr := uint32(0xA0000); addr < 0xB0000; addr += 0x100 {
			page := addr >> 8
			if page < uint32(len(cpu.x86JitIOBitmap)) {
				cpu.x86JitIOBitmap[page] = 1
			}
		}
	}

	// Code page bitmap for self-mod detection
	cpu.x86JitCodeBM = make([]byte, len(cpu.x86JitIOBitmap))

	cpu.x86JitCtx = newX86JITContext(cpu, cpu.x86JitCodeBM, cpu.x86JitIOBitmap)
	// AddressIsMMIOPredicate is set per call from the live CPU at each
	// TryFastMMIOPoll site (cpu_x86_poll_match_jit.go); wiring the shared global
	// X86PollPattern here is dead and races across concurrent CPUs. See jit_exec.go.
	return nil
}

// freeX86JIT releases all JIT resources.
func (cpu *CPU_X86) freeX86JIT() {
	if cpu.x86JitPersist {
		return
	}
	if em := cpu.x86GetJITExecMem(); em != nil {
		em.Free()
		cpu.x86JitExecMem = nil
	}
	cpu.x86JitCache = nil
	cpu.x86JitCtx = nil
	cpu.x86JitCodeBM = nil
}

// X86ExecuteJIT is the main JIT execution loop for the x86 CPU.
func (cpu *CPU_X86) X86ExecuteJIT() {
	if err := cpu.initX86JIT(); err != nil {
		panic(fmt.Sprintf("x86 JIT init failed: %v", err))
	}
	defer cpu.freeX86JIT()
	if x86JITStatsOn {
		defer x86JITStatsReport()
	}

	execMem := cpu.x86GetJITExecMem()
	ctx := cpu.x86JitCtx

	// Diagnostic counters
	var instructionCount uint64
	var diagCacheHits uint64
	var diagCacheMisses uint64
	var diagFallbackInstr uint64
	var diagIOBails uint64
	accountRetired := func(executed, nativeRetired uint64) {
		instructionCount += executed
		cpu.jitStats.nativeRetired.Add(nativeRetired)
		cpu.jitStats.instructionCount.Add(executed)
		if perfAcctOn {
			cpu.perfAcct.AddInstrs(executed)
		}
	}

	// Performance monitoring
	var perfStartTime time.Time
	var lastPerfReport time.Time
	perfEnabled := x86JITStatsOn // temporary: X86_JIT_STATS=1 enables live+exit profiling

	if perfEnabled {
		perfStartTime = time.Now()
		lastPerfReport = perfStartTime
	}

	// Sync named CPU fields -> jitRegs ONCE at JIT entry.
	// jitRegs is the canonical state during JIT execution.
	cpu.syncJITRegsFromNamed()
	cpu.syncJITSegRegsFromNamed()

	bounded := cpu.x86BudgetActive
	var budgetPrevCount uint64
	for cpu.Running() && !cpu.Halted {
		if cpu.debugHandleBreakInJIT(uint64(cpu.EIP)) {
			cpu.deoptStats.Add(DeoptDebug)
			cpu.syncJITSegRegsToNamed()
			cpu.x86RenormalizeFPUBoundary()
			return
		}
		if bounded {
			delta := instructionCount - budgetPrevCount
			budgetPrevCount = instructionCount
			cpu.x86InstrBudget -= int64(delta)
			if cpu.x86InstrBudget <= 0 {
				cpu.deoptStats.Add(DeoptDebug)
				cpu.syncJITRegsToNamed()
				cpu.syncJITSegRegsToNamed()
				cpu.x86RenormalizeFPUBoundary()
				return
			}
		}
		// Check for pending interrupt (named fields are stale; sync first)
		if cpu.nmiPending.Load() {
			cpu.syncJITRegsToNamed()
			cpu.handleInterrupt(0x02)
			cpu.nmiPending.Store(false)
			cpu.syncJITRegsFromNamed()
		} else if cpu.irqPending.Load() {
			cpu.syncJITRegsToNamed()
			if !cpu.IF() {
				cpu.syncJITRegsFromNamed()
			} else {
				cpu.handleInterrupt(byte(cpu.irqVector.Load()))
				cpu.irqPending.Store(false)
				cpu.syncJITRegsFromNamed()
			}
		}

		// MMIO status spin loops dominate demo wait time. Handle the common
		// MOV/TEST/Jcc-back pattern directly so JIT-enabled execution doesn't
		// bounce through one-instruction fallbacks for every poll.
		if !bounded && cpu.tryFastMMIOPollLoopJIT() {
			continue
		}

		pc := cpu.EIP

		// Bounds check
		if pc >= uint32(len(cpu.memory)) {
			fmt.Printf("x86 JIT: EIP out of bounds: 0x%08X\n", pc)
			cpu.Halted = true
			break
		}

		// Try cache lookup
		block := cpu.x86JitCache.Get(uint64(pc))
		if block == nil {
			cpu.jitStats.cacheMisses.Add(1)
			// Scan block
			instrs := x86ScanBlock(cpu.memory, pc)
			if len(instrs) == 0 {
				// Interpreter fallback: sync jitRegs -> named, step, sync back
				x86RecordFallbackOpcode(cpu.memory, pc)
				cpu.syncJITRegsToNamed()
				var stepT0 time.Time
				if perfAcctOn {
					stepT0 = time.Now()
				}
				cpu.x86RenormalizeFPUBoundary()
				cpu.Step()
				if perfAcctOn {
					cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
				}
				cpu.syncJITRegsFromNamed()
				instructionCount++
				cpu.jitStats.instructionCount.Add(1)
				cpu.jitStats.fallbackInstructions.Add(1)
				if perfAcctOn {
					cpu.perfAcct.AddInstrs(1)
					cpu.deoptStats.Add(DeoptUnsupported)
				}
				diagFallbackInstr++
				continue
			}
			// Deterministic shadow windows compare architected state after an
			// exact instruction count. Compile one instruction at a time in that
			// test-only bounded mode so a native block can never retire across the
			// requested checkpoint. Ordinary execution keeps full basic blocks.
			if bounded {
				instrs = instrs[:1]
			}

			if x86NeedsFallback(instrs) {
				x86RecordFallbackOpcode(cpu.memory, pc)
				cpu.syncJITRegsToNamed()
				var stepT0 time.Time
				if perfAcctOn {
					stepT0 = time.Now()
				}
				cpu.x86RenormalizeFPUBoundary()
				cpu.Step()
				if perfAcctOn {
					cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
				}
				cpu.syncJITRegsFromNamed()
				instructionCount++
				cpu.jitStats.instructionCount.Add(1)
				cpu.jitStats.fallbackInstructions.Add(1)
				if perfAcctOn {
					cpu.perfAcct.AddInstrs(1)
					cpu.deoptStats.Add(DeoptUnsupported)
				}
				diagFallbackInstr++
				if cpu.Halted || !cpu.Running() {
					break
				}
				continue
			}

			// Compile against this CPU's immutable safety-input snapshot.
			var err error
			block, err = x86CompileBlockForCPU(cpu, instrs, pc, execMem)
			if err != nil {
				// "no instructions compiled" means the first scanned instr
				// fell through every emit case, equivalent to an
				// x86NeedsFallback hit that the static list missed. Treat
				// as a per-instruction Step bail (the same protocol as
				// MMIO bail). Any other compile error is a real JIT bug
				// and panics so the gap is fixed at its source.
				if err.Error() == "no instructions compiled" {
					x86RecordFallbackOpcode(cpu.memory, pc)
					cpu.syncJITRegsToNamed()
					var stepT0 time.Time
					if perfAcctOn {
						stepT0 = time.Now()
					}
					cpu.x86RenormalizeFPUBoundary()
					if payload, ok := x86FPUHelperPayloadFor(instrs[0], cpu.memory, cpu.CS); ok {
						cpu.x86RunFPUHelper(payload)
					} else {
						cpu.Step()
					}
					if perfAcctOn {
						cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
					}
					cpu.syncJITRegsFromNamed()
					instructionCount++
					cpu.jitStats.instructionCount.Add(1)
					cpu.jitStats.fallbackInstructions.Add(1)
					if _, ok := x86FPUHelperPayloadFor(instrs[0], cpu.memory, cpu.CS); ok {
						cpu.jitStats.helperExits.Add(1)
					}
					if perfAcctOn {
						cpu.perfAcct.AddInstrs(1)
						cpu.deoptStats.Add(DeoptUnsupported)
					}
					diagFallbackInstr++
					if cpu.Halted || !cpu.Running() {
						break
					}
					continue
				}
				panic(fmt.Sprintf("x86 JIT: compile failed at PC=0x%08X: %v "+
					"(scanned %d instrs starting %02X %02X %02X %02X)",
					pc, err, len(instrs), cpu.memory[pc], cpu.memory[pc+1],
					cpu.memory[pc+2], cpu.memory[pc+3]))
			}

			// Cache block and mark code pages
			cpu.x86JitCache.Put(block)
			cpu.jitStats.compiledBlocks.Add(1)
			if x86JITStatsOn {
				x86JITStats.tier1Blocks.Add(1)
			}
			if cpu.x86JitCodeBM != nil {
				x86MarkCodePagesForBlock(cpu.x86JitCodeBM, block)
			}

			// Patch chain slots bidirectionally -- only for compatible register maps
			if !bounded && x86BlockChainingEnabled && block.chainEntry != 0 {
				x86PatchCompatibleChainsTo(cpu.x86JitCache, block)
			}
			if !bounded && x86BlockChainingEnabled {
				for i := range block.chainSlots {
					slot := &block.chainSlots[i]
					if target := cpu.x86JitCache.Get(slot.targetPC); target != nil && target.chainEntry != 0 {
						if target.regMap == block.regMap {
							PatchRel32At(slot.patchAddr, target.chainEntry)
						}
					}
				}
			}

			diagCacheMisses++
		} else {
			diagCacheHits++
			cpu.jitStats.cacheHits.Add(1)

			// Hot-block detection via shared Phase 3 TierController.
			// Equivalent arithmetic to the prior inline gate (execCount >=
			// 64 && lastPromoteAt == 0 && ioBails*4 < execCount).
			block.execCount++
			if x86RegionPromotionEnabled && x86TierController.ShouldPromote(block.tier, block.execCount, block.ioBails, block.lastPromoteAt) {
				block.lastPromoteAt = block.execCount
				// Try multi-block region compilation first (only for 3+ block regions)
				if x86JITStatsOn {
					x86JITStats.regionCandidates.Add(1)
				}
				cpu.jitStats.regionCandidates.Add(1)
				region := x86FormRegion(pc, cpu.x86JitCache, cpu.memory)
				if region != nil && x86TierController.ShouldPromoteRegion(len(region.blocks)) {
					newBlock, err := x86CompileRegionForCPU(cpu, region, execMem)
					if err == nil {
						newBlock.execCount = block.execCount
						cpu.x86JitCache.Put(newBlock)
						if x86BlockChainingEnabled && newBlock.chainEntry != 0 {
							x86RetargetPromotionChainsTo(cpu.x86JitCache, newBlock)
							x86InvalidateRTSCacheForPC(ctx, uint32(newBlock.startPC))
						}
						block = newBlock
						cpu.jitStats.compiledRegions.Add(1)
					}
				}
				// Single-block Tier-2 recompile is a no-op while
				// per-block regalloc is forced to default; the
				// recompiled block would be byte-identical to the
				// original. Region promotion (above) still runs for
				// 3+ block hot regions.
			}
		}

		// Update RTS cache: shift entry 0 → 1, write new entry 0. Each
		// slot carries the target block's regMap so the native RET
		// probe can reject hits whose host-register layout differs from
		// the running block. Without that gate, a Tier-2 callee could
		// chain back into a Tier-1 caller (or vice versa) with mapped
		// guest registers reading the wrong host registers.
		if !bounded && x86RTSChainingEnabled && block.chainEntry != 0 {
			ctx.RTSCache1PC = ctx.RTSCache0PC
			ctx.RTSCache1Addr = ctx.RTSCache0Addr
			ctx.RTSCache1RegMap = ctx.RTSCache0RegMap
			ctx.RTSCache0PC = uint32(block.startPC)
			ctx.RTSCache0Addr = block.chainEntry
			ctx.RTSCache0RegMap = x86RegMapToUint64(block.regMap)
		} else {
			ctx.RTSCache0PC = 0
			ctx.RTSCache0Addr = 0
			ctx.RTSCache0RegMap = 0
			ctx.RTSCache1PC = 0
			ctx.RTSCache1Addr = 0
			ctx.RTSCache1RegMap = 0
		}

		if bounded && int64(block.instrCount) > cpu.x86InstrBudget {
			cpu.syncJITRegsToNamed()
			var stepT0 time.Time
			if perfAcctOn {
				stepT0 = time.Now()
			}
			cpu.x86RenormalizeFPUBoundary()
			cpu.Step()
			if perfAcctOn {
				cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
			}
			cpu.syncJITRegsFromNamed()
			instructionCount++
			if perfAcctOn {
				cpu.perfAcct.AddInstrs(1)
				cpu.deoptStats.Add(DeoptDebug)
			}
			diagFallbackInstr++
			cpu.jitStats.instructionCount.Add(1)
			cpu.jitStats.fallbackInstructions.Add(1)
			if cpu.Halted || !cpu.Running() {
				break
			}
			continue
		}

		// Execute native code block -- jitRegs is already canonical, no sync needed
		ctx.NeedInval = 0
		ctx.NeedIOFallback = 0
		ctx.ExitReason = x86JITExitNone
		ctx.InvalAddr = 0
		ctx.InvalSize = 0
		// In bounded mode (deterministic-step harness), force the native
		// dispatch to return after a single block by setting ChainBudget=1.
		// Otherwise a chained run could retire many thousands of guest
		// instructions before the outer loop revisits the budget check,
		// blowing past the harness's per-checkpoint window. ChainBudget=1
		// still allows the block itself to retire its full instrCount
		// (block granularity is the irreducible overshoot, documented
		// in jit_x86_shadow_parity_test.go); the outer-loop budget
		// subtraction then catches up on the next iteration.
		if bounded {
			ctx.ChainBudget = 0
		} else {
			ctx.ChainBudget = 65536
		}
		// Shadow-parity windows require an exact instruction boundary. A native
		// block is otherwise indivisible, so do not enter one that would cross
		// the remaining budget: execute one canonical interpreter step instead
		// and rescan. Normal JIT execution never takes this path.
		if bounded && int64(block.instrCount) > cpu.x86InstrBudget {
			cpu.syncJITRegsToNamed()
			cpu.x86RenormalizeFPUBoundary()
			cpu.Step()
			cpu.syncJITRegsFromNamed()
			instructionCount++
			diagFallbackInstr++
			cpu.jitStats.instructionCount.Add(1)
			cpu.jitStats.fallbackInstructions.Add(1)
			if cpu.Halted || !cpu.Running() {
				break
			}
			continue
		}
		ctx.ChainCount = 0
		preECX, preESI, preEDI := cpu.jitRegs[1], cpu.jitRegs[6], cpu.jitRegs[7]
		var jitT0 time.Time
		if perfAcctOn {
			jitT0 = time.Now()
		}
		cpu.jitStats.nativeEntries.Add(1)
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
		if perfAcctOn {
			cpu.perfAcct.AddJit(time.Since(jitT0).Nanoseconds())
		}

		// Read return values from context (jitRegs updated by native code)
		cpu.EIP = ctx.RetPC
		executed := uint64(ctx.RetCount)
		if executed == 0 {
			if ctx.ChainCount > 0 {
				executed = uint64(ctx.ChainCount)
			} else if ctx.NeedIOFallback == 0 {
				executed = uint64(block.instrCount)
			}
		}
		nativeRetired := executed
		// Native blocks do not call the Go interpreter handlers that normally
		// charge CPU.Cycles. Publish the matching completed-prefix charge here.
		// A deferred bail can retire only a prefix, so never use the whole block
		// total unless the context says it completed that many instructions.
		var nativeTicks uint64
		if executed != 0 && len(block.x86CyclePrefix) != 0 {
			completed := executed
			if completed > uint64(len(block.x86CyclePrefix)) {
				completed = uint64(len(block.x86CyclePrefix))
			}
			cpu.Cycles += block.x86CyclePrefix[completed-1]
			if len(block.x86TickPrefix) >= int(completed) {
				nativeTicks = block.x86TickPrefix[completed-1]
			}
		}
		// REP string handlers charge one cycle per completed iteration. Native
		// lowering admits only forward forms, so the resulting ESI or EDI delta
		// is the exact count without extending the stable emitted context ABI.
		// REP consumes ECX, so every following REP in the same straight-line
		// block sees the remaining count. This covers consecutive admitted
		// string forms rather than silently dropping all dynamic accounting when
		// a block contains more than one. The first form's address delta also
		// preserves early-stop CMPS/SCAS semantics.
		remainingECX := preECX
		for i, form := range block.x86DynamicCycles {
			iterations := uint64(1)
			if form.rep {
				iterations = uint64(remainingECX)
				if i == 0 && form.width != 0 {
					before, after := preEDI, cpu.jitRegs[7]
					if form.source {
						before, after = preESI, cpu.jitRegs[6]
					}
					iterations = uint64(uint32(after-before) / form.width)
				}
				remainingECX = 0
			}
			cpu.Cycles += iterations
			if iterations > 0 {
				nativeTicks += iterations - 1
			}
		}
		if nativeTicks != 0 {
			cpu.bus.Tick(int(nativeTicks))
		}

		// Profile counters
		if ctx.ChainCount > 0 {
			block.chainHits++
			if x86JITStatsOn {
				x86JITStats.chainExits.Add(1)
			}
			cpu.jitStats.chainExits.Add(1)
		}
		if ctx.ChainBudget <= 0 {
			block.unchainedExits++ // budget exhausted = unchained exit
		}

		// Self-modifying code: invalidate cache
		if ctx.NeedInval != 0 {
			recordBlockDeopt(&cpu.deoptStats, block, DeoptSMC)
			if jitSMCRangeDisabled {
				cpu.jitStats.invalidatedBlocks.Add(uint64(cpu.x86JitCache.Len()))
				cpu.x86JitCache.Invalidate()
				execMem.Reset()
				clear(cpu.x86JitCodeBM)
				x86ClearRTSCache(ctx)
				cpu.jitStats.codeCacheResets.Add(1)
			} else {
				removed := x86InvalidateSMCRange(cpu.x86JitCache, cpu.x86JitCodeBM, ctx)
				cpu.jitStats.invalidatedBlocks.Add(uint64(removed))
				if removed != 0 {
					resetExecMemWhenCacheEmpty(cpu.x86JitCache, execMem)
					if cpu.x86JitCache.Len() == 0 {
						cpu.jitStats.codeCacheResets.Add(1)
					}
				}
			}
			if x86JITStatsOn {
				x86JITStats.invalidations.Add(1)
			}
			cpu.jitStats.invalidations.Add(1)
			ctx.NeedInval = 0
			ctx.InvalAddr = 0
			ctx.InvalSize = 0
		}

		// I/O fallback: sync to named, then either execute a recognized
		// MMIO byte write directly or fall back to the full interpreter.
		if ctx.NeedIOFallback != 0 {
			ctx.NeedIOFallback = 0
			if payload, ok := x86FPUHelperPayloadFromContext(ctx); ok {
				cpu.jitStats.helperExits.Add(1)
				ctx.ExitReason = x86JITExitNone
				recordBlockDeopt(&cpu.deoptStats, block, DeoptUnsupported)
				cpu.syncJITRegsToNamed()
				var stepT0 time.Time
				if perfAcctOn {
					stepT0 = time.Now()
				}
				cpu.x86RenormalizeFPUBoundary()
				cpu.x86RunFPUHelper(payload)
				if perfAcctOn {
					cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
				}
				diagFallbackInstr++
				executed++
				// This path continues above the shared retirement accounting below.
				// Publish the native prefix plus the helper instruction first so a
				// bounded JIT run cannot execute past its requested instruction count.
				accountRetired(executed, nativeRetired)
				cpu.jitStats.fallbackInstructions.Add(1)
				cpu.syncJITRegsFromNamed()
				if cpu.Halted || !cpu.Running() {
					break
				}
				continue
			}
			recordBlockDeopt(&cpu.deoptStats, block, DeoptMMIO)
			cpu.jitStats.ioBails.Add(1)
			block.ioBails++ // profile counter for promotion decisions
			if !bounded {
				if fastCount, ok := cpu.tryFastMMIOWriteFallbackJIT(); ok {
					executed += fastCount
					cpu.jitStats.fallbackInstructions.Add(fastCount)
				} else {
					cpu.syncJITRegsToNamed()
					var stepT0 time.Time
					if perfAcctOn {
						stepT0 = time.Now()
					}
					cpu.x86RenormalizeFPUBoundary()
					cpu.Step()
					if perfAcctOn {
						cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
					}
					diagFallbackInstr++
					cpu.jitStats.fallbackInstructions.Add(1)
					executed++
					cpu.syncJITRegsFromNamed()
				}
			} else {
				cpu.syncJITRegsToNamed()
				var stepT0 time.Time
				if perfAcctOn {
					stepT0 = time.Now()
				}
				cpu.x86RenormalizeFPUBoundary()
				cpu.Step()
				if perfAcctOn {
					cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
				}
				diagFallbackInstr++
				cpu.jitStats.fallbackInstructions.Add(1)
				executed++
				cpu.syncJITRegsFromNamed()
			}
			diagIOBails++
			if cpu.Halted || !cpu.Running() {
				accountRetired(executed, nativeRetired)
				break
			}
		}

		accountRetired(executed, nativeRetired)

		// Performance monitoring
		if perfEnabled {
			now := time.Now()
			if now.Sub(lastPerfReport) >= time.Second {
				elapsed := now.Sub(perfStartTime).Seconds()
				mips := float64(instructionCount) / elapsed / 1_000_000
				hitRate := float64(0)
				if diagCacheHits+diagCacheMisses > 0 {
					hitRate = float64(diagCacheHits) / float64(diagCacheHits+diagCacheMisses) * 100
				}
				fallbackPct := float64(0)
				if instructionCount > 0 {
					fallbackPct = float64(diagFallbackInstr) / float64(instructionCount) * 100
				}
				fmt.Printf("\rx86 JIT: %.2f MIPS | cache %.0f%% | fallback %.1f%% | io %d   ",
					mips, hitRate, fallbackPct, diagIOBails)
				lastPerfReport = now
			}
		}
	}

	// Sync jitRegs -> named fields ONCE at JIT exit
	cpu.x86RenormalizeFPUBoundary()
	cpu.syncJITRegsToNamed()
	cpu.syncJITSegRegsToNamed()

	if x86JITStatsOn {
		hitRate := float64(0)
		if diagCacheHits+diagCacheMisses > 0 {
			hitRate = float64(diagCacheHits) / float64(diagCacheHits+diagCacheMisses) * 100
		}
		fbPct := float64(0)
		if instructionCount > 0 {
			fbPct = float64(diagFallbackInstr) / float64(instructionCount) * 100
		}
		fmt.Printf("\nx86 JIT exit: instrs=%d fallback=%d (%.2f%%) io_bails=%d cache_hit=%.1f%%\n",
			instructionCount, diagFallbackInstr, fbPct, diagIOBails, hitRate)
		// jit_ns/interp_ns come from PerfAcct, which only accumulates when
		// IE_PERF_ACCT=1. Print them only then, else they are misleading zeros.
		// jit_ns/interp_ns and the deopt taxonomy only accumulate when
		// IE_PERF_ACCT=1; print them only then to avoid misleading zeros.
		if perfAcctOn {
			acct := cpu.perfAcct.Snapshot()
			fmt.Printf("x86 JIT timing: jit_ns=%d interp_ns=%d\n", acct.JitNs, acct.InterpNs)
			fmt.Printf("x86 JIT %s\n", cpu.deoptStats.String())
		}
		x86FallbackOpcodeReport()
	}
}

// x86RegMapToUint64 packs a [8]byte regMap into a uint64 for runtime
// equality compares. Layout: byte i ↔ uint64 byte i. Two regMaps are
// compatible iff their packed forms are bitwise-equal.
func x86RegMapToUint64(rm [8]byte) uint64 {
	return uint64(rm[0]) |
		uint64(rm[1])<<8 |
		uint64(rm[2])<<16 |
		uint64(rm[3])<<24 |
		uint64(rm[4])<<32 |
		uint64(rm[5])<<40 |
		uint64(rm[6])<<48 |
		uint64(rm[7])<<56
}

// x86PatchCompatibleChainsTo patches chain slots in cached blocks that target
// the given block's startPC, but ONLY when the source block has a compatible
// (identical) register mapping. This prevents corrupting guest state when
// Tier 2 blocks with different register allocations are chained together.
func x86PatchCompatibleChainsTo(cache *CodeCache, target *JITBlock) {
	for _, source := range cache.blocks {
		if source.regMap != target.regMap {
			continue // incompatible register maps -- skip
		}
		for _, slot := range source.chainSlots {
			if slot.targetPC == target.startPC && slot.patchAddr != 0 {
				PatchRel32At(slot.patchAddr, target.chainEntry)
			}
		}
	}
}

// x86RetargetPromotionChainsTo is the replacement-block variant of
// x86PatchCompatibleChainsTo. Region promotion can leave older native code
// mapped after the cache entry is replaced, so every live inbound slot for the
// promoted PC must be rewritten: compatible sources jump to the new entry,
// incompatible sources fall through to the dispatcher.
func x86RetargetPromotionChainsTo(cache *CodeCache, target *JITBlock) {
	if cache == nil || target == nil {
		return
	}
	for _, source := range cache.blocks {
		for _, slot := range source.chainSlots {
			if slot.targetPC != target.startPC || slot.patchAddr == 0 {
				continue
			}
			if source.regMap == target.regMap && target.chainEntry != 0 {
				PatchRel32At(slot.patchAddr, target.chainEntry)
				continue
			}
			PatchRel32At(slot.patchAddr, slot.patchAddr+4)
		}
	}
}

func x86InvalidateRTSCacheForPC(ctx *X86JITContext, pc uint32) {
	if ctx == nil {
		return
	}
	if ctx.RTSCache0PC != pc && ctx.RTSCache1PC != pc {
		return
	}
	ctx.RTSCache0PC = 0
	ctx.RTSCache0Addr = 0
	ctx.RTSCache0RegMap = 0
	ctx.RTSCache1PC = 0
	ctx.RTSCache1Addr = 0
	ctx.RTSCache1RegMap = 0
}

// tryFastMMIOWriteFallback handles the byte-store MMIO instructions that are
// common in raster-effect loops after the native block has already bailed on an
// I/O page. It preserves the architectural effect of the interpreter handlers
// for these no-prefix forms while avoiding the full fetch/decode Step path.
func (cpu *CPU_X86) tryFastMMIOWriteFallback() (uint64, bool) {
	return cpu.tryFastMMIOWriteFallbackWithRegs(false)
}

func (cpu *CPU_X86) tryFastMMIOWriteFallbackJIT() (uint64, bool) {
	return cpu.tryFastMMIOWriteFallbackWithRegs(true)
}

func (cpu *CPU_X86) tryFastMMIOWriteFallbackWithRegs(useJITRegs bool) (uint64, bool) {
	var executed uint64
	for executed < 64 {
		if cpu.hasPendingX86Interrupt() {
			return executed, executed != 0
		}

		pc := cpu.EIP
		if pc >= uint32(len(cpu.memory)) {
			break
		}

		switch cpu.memory[pc] {
		case 0xA2: // MOV moffs8, AL
			if pc+5 > uint32(len(cpu.memory)) {
				return executed, executed != 0
			}
			addr := readLE32(cpu.memory, pc+1)
			if !cpu.isX86GuestIOPage(addr) {
				return executed, executed != 0
			}
			cpu.write8(addr, cpu.getReg8ForFallback(useJITRegs, 0))
			cpu.finishFastMMIOWrite(pc + 5)
			executed++

		case 0x88: // MOV Eb, Gb
			if pc+6 > uint32(len(cpu.memory)) {
				return executed, executed != 0
			}
			modrm := cpu.memory[pc+1]
			if modrm>>6 != 0 || modrm&7 != 5 {
				return executed, executed != 0
			}
			addr := readLE32(cpu.memory, pc+2)
			if !cpu.isX86GuestIOPage(addr) {
				return executed, executed != 0
			}
			cpu.write8(addr, cpu.getReg8ForFallback(useJITRegs, (modrm>>3)&7))
			cpu.finishFastMMIOWrite(pc + 6)
			executed++

		case 0xC6: // MOV Eb, Ib
			if pc+7 > uint32(len(cpu.memory)) {
				return executed, executed != 0
			}
			modrm := cpu.memory[pc+1]
			if modrm>>6 != 0 || modrm&7 != 5 || (modrm>>3)&7 != 0 {
				return executed, executed != 0
			}
			addr := readLE32(cpu.memory, pc+2)
			if !cpu.isX86GuestIOPage(addr) {
				return executed, executed != 0
			}
			cpu.write8(addr, cpu.memory[pc+6])
			cpu.finishFastMMIOWrite(pc + 7)
			executed++

		default:
			if cpu.memory[pc] >= 0xB0 && cpu.memory[pc] <= 0xB7 { // MOV r8, imm8
				if pc+2 > uint32(len(cpu.memory)) {
					return executed, executed != 0
				}
				cpu.setReg8ForFallback(useJITRegs, cpu.memory[pc]-0xB0, cpu.memory[pc+1])
				cpu.finishFastMMIOWrite(pc + 2)
				executed++
				continue
			}
			return executed, executed != 0
		}
	}

	return executed, executed != 0
}

func (cpu *CPU_X86) getReg8ForFallback(useJITRegs bool, idx byte) byte {
	if !useJITRegs {
		return cpu.getReg8(idx)
	}
	regIdx, shift := x86JITReg8Index(idx)
	return byte(cpu.jitRegs[regIdx] >> shift)
}

func (cpu *CPU_X86) setReg8ForFallback(useJITRegs bool, idx byte, value byte) {
	if !useJITRegs {
		cpu.setReg8(idx, value)
		return
	}
	regIdx, shift := x86JITReg8Index(idx)
	mask := uint32(0xFF) << shift
	cpu.jitRegs[regIdx] = (cpu.jitRegs[regIdx] &^ mask) | (uint32(value) << shift)
}

func (cpu *CPU_X86) hasPendingX86Interrupt() bool {
	return cpu.nmiPending.Load() || (cpu.irqPending.Load() && cpu.IF())
}

func (cpu *CPU_X86) isX86GuestIOPage(addr uint32) bool {
	page := addr >> 8
	if page < uint32(len(cpu.x86JitIOBitmap)) && cpu.x86JitIOBitmap[page] != 0 {
		return true
	}
	return false
}

func (cpu *CPU_X86) finishFastMMIOWrite(nextPC uint32) {
	cpu.EIP = nextPC
	cpu.Cycles++
	cpu.bus.Tick(1)
}

// x86RunInterpreter is the fallback interpreter loop. Used when JIT is
// disabled (CPUX86Runner.DisableJIT=true). Keeps only the
// workload-agnostic tryFastMMIOPollLoop fast match for status-poll
// loops; no per-program shortcuts.
func (cpu *CPU_X86) x86RunInterpreter() {
	bounded := cpu.x86BudgetActive
	yieldCheck := uint32(0)
	for cpu.Running() && !cpu.Halted {
		if bounded && cpu.x86InstrBudget <= 0 {
			return
		}
		// Once per 4096 instructions: on js/wasm park briefly so the browser
		// event loop runs (no-op on native builds).
		yieldCheck++
		if yieldCheck&0xFFF == 0 {
			hostCooperativeYield()
		}
		if !bounded && cpu.tryFastMMIOPollLoop() {
			continue
		}
		cpu.x86RenormalizeFPUBoundary()
		cpu.Step()
		cpu.jitStats.instructionCount.Add(1)
		if bounded {
			cpu.x86InstrBudget--
			if cpu.x86InstrBudget <= 0 {
				return
			}
		}
	}
}

// x86RenormalizeFPUBoundary restores the interpreter's 3-way FTW tag
// classification at every JIT->interpreter handoff (single-instruction
// fallback steps and JIT exit). The JIT tracks tags only as
// empty-vs-occupied; see FPU_X87.RenormalizeTags.
func (cpu *CPU_X86) x86RenormalizeFPUBoundary() {
	if cpu.FPU != nil {
		cpu.FPU.RenormalizeTags()
	}
}
