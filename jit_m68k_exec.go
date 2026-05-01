// jit_m68k_exec.go - M68020 JIT execution loop

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// M68K JIT configuration
const (
	m68kJitExecMemSize = 16 * 1024 * 1024 // 16MB executable memory pool
)

// m68kTierController is the shared Phase 3 promotion controller bound
// to M68K's reference RegPressureProfile. Cache-hit gate in the exec
// loop delegates to ShouldPromote so threshold tweaks apply uniformly.
//
// B.1.b uses this controller to drive region promotion only — no
// per-block Tier-2 reg-map promotion lands until B.1.c. The
// PromoteBlock allocator stub stays a no-op for now (see
// jit_tier_backends.go).
var m68kTierController = NewTierController(M68KRegProfile)

// m68kGetJITExecMem returns the typed *ExecMem from the cpu's any field.
func (cpu *M68KCPU) m68kGetJITExecMem() *ExecMem {
	if cpu.m68kJitExecMem == nil {
		return nil
	}
	return cpu.m68kJitExecMem.(*ExecMem)
}

// initM68KJIT initializes JIT state. Called once before execution.
func (cpu *M68KCPU) initM68KJIT() error {
	if cpu.m68kJitExecMem != nil {
		return nil // already initialized
	}
	execMem, err := AllocExecMem(m68kJitExecMemSize)
	if err != nil {
		return fmt.Errorf("M68K JIT init failed: %w", err)
	}
	cpu.m68kJitExecMem = execMem
	cpu.m68kJitCache = NewCodeCache()
	cpu.m68kJitCodeBitmap = make([]byte, (uint32(len(cpu.memory))+4095)>>12)
	cpu.m68kJitCtx = newM68KJITContext(cpu, cpu.m68kJitCodeBitmap)
	return nil
}

// freeM68KJIT releases all JIT resources. If m68kJitPersist is set,
// the code cache and exec memory are kept alive for reuse across benchmark runs.
func (cpu *M68KCPU) freeM68KJIT() {
	if cpu.m68kJitPersist {
		return
	}
	if em := cpu.m68kGetJITExecMem(); em != nil {
		em.Free()
		cpu.m68kJitExecMem = nil
	}
	cpu.m68kJitCache = nil
	cpu.m68kJitCtx = nil
	cpu.m68kJitCodeBitmap = nil
}

// m68kInterpretOne executes one M68K instruction at cpu.PC using the interpreter.
func (cpu *M68KCPU) m68kInterpretOne() {
	cpu.StepOne()
}

// M68KExecuteJIT is the main JIT execution loop for the M68020.
// It replaces ExecuteInstruction() when JIT compilation is enabled.
func (cpu *M68KCPU) M68KExecuteJIT() {
	// Correctness-first temporary mode: when M68K JIT is enabled, route runtime
	// execution through the interpreter until native block coverage is rebuilt
	// behind the full M68K test matrix.
	if !cpu.m68kJitForceNative {
		cpu.ExecuteInstruction()
		return
	}

	if err := cpu.initM68KJIT(); err != nil {
		fmt.Printf("M68K JIT: %v, falling back to interpreter\n", err)
		cpu.ExecuteInstruction()
		return
	}
	defer cpu.freeM68KJIT()

	enableM68KPollWiring(cpu)

	execMem := cpu.m68kGetJITExecMem()
	ctx := cpu.m68kJitCtx

	// Initialize performance measurement
	instructionCount := uint64(0)
	if cpu.PerfEnabled {
		cpu.perfStartTime = time.Now()
		cpu.lastPerfReport = cpu.perfStartTime
		cpu.InstructionCount = 0
	}

	// Diagnostic counters
	var diagCacheHits uint64
	var diagCacheMisses uint64
	var diagFallbackInstr uint64
	var diagIOBails uint64

	for cpu.running.Load() {
		// ── STOP state handling (replicates cpu_m68k.go:2358-2405) ──
		if cpu.stopped.Load() {
			pendingException := cpu.pendingException.Load()
			if pendingException != 0 {
				cpu.pendingException.Store(0)
				cpu.stopped.Store(false)
				cpu.stopSpinCount.Store(0)
				cpu.ProcessException(uint8(pendingException))
			}

			woke := false
			pending := cpu.pendingInterrupt.Load()
			if pending != 0 {
				ipl := uint32((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
				for level := uint32(7); level >= 1; level-- {
					if pending&(1<<level) != 0 && (level > ipl || level == 7) {
						cpu.stopped.Store(false)
						cpu.stopSpinCount.Store(0)
						if cpu.ProcessInterrupt(uint8(level)) {
							woke = true
							for {
								old := cpu.pendingInterrupt.Load()
								if cpu.pendingInterrupt.CompareAndSwap(old, old&^(1<<level)) {
									break
								}
							}
						} else {
							cpu.stopped.Store(true)
						}
						break
					}
				}
			}

			if !woke {
				spins := cpu.stopSpinCount.Add(1)
				if spins >= 5000 && spins%5000 == 0 {
					cpu.stopWatchdogHits.Add(1)
				}
			}

			runtime.Gosched()
			continue // back to top — do NOT fall through to fetch
		}

		// ── Normal instruction execution ──
		pc := cpu.PC

		// Bounds check
		if pc+2 > uint32(len(cpu.memory)) {
			fmt.Printf("M68K JIT: PC out of bounds: 0x%08X\n", pc)
			cpu.running.Store(false)
			break
		}

		// Odd PC check
		if pc&1 != 0 {
			cpu.ProcessException(M68K_VEC_ADDRESS_ERROR)
			continue
		}

		if matched, retired := cpu.tryFastM68KMMIOPollLoop(); matched {
			instructionCount += uint64(retired)
			continue
		}

		// Try cache lookup
		block := cpu.m68kJitCache.Get(pc)
		if block == nil {
			// Scan block
			instrs := m68kScanBlock(cpu.memory, pc)
			if len(instrs) == 0 {
				cpu.m68kInterpretOne()
				instructionCount++
				diagFallbackInstr++
				continue
			}

			// Check if this block should stay in the interpreter.
			if m68kNeedsFallback(instrs) || (!cpu.m68kJitForceNative && m68kNeedsConservativeFallback(cpu.memory, pc, instrs)) {
				cpu.m68kInterpretOne()
				instructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
			}

			// Compile block
			var err error
			block, err = m68kCompileBlockWithMem(instrs, pc, execMem, cpu.memory)
			if err != nil {
				cpu.m68kInterpretOne()
				instructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
			}

			// Cache block and mark code pages
			cpu.m68kJitCache.Put(block)
			if cpu.m68kJitCodeBitmap != nil {
				startPage := block.startPC >> 12
				endPage := (block.endPC - 1) >> 12
				for p := startPage; p <= endPage; p++ {
					if p < uint32(len(cpu.m68kJitCodeBitmap)) {
						cpu.m68kJitCodeBitmap[p] = 1
					}
				}
			}

			// Patch chain slots bidirectionally:
			// 1. Existing blocks exiting to this block → patch their slots to our chainEntry
			if block.chainEntry != 0 {
				cpu.m68kJitCache.PatchChainsTo(block.startPC, block.chainEntry)
			}
			// 2. This block's exits targeting already-cached blocks → patch our slots
			for i := range block.chainSlots {
				slot := &block.chainSlots[i]
				if target := cpu.m68kJitCache.Get(slot.targetPC); target != nil && target.chainEntry != 0 {
					PatchRel32At(slot.patchAddr, target.chainEntry)
				}
			}
			diagCacheMisses++
		} else {
			diagCacheHits++

			// Hot-block detection — when execCount crosses the shared
			// TierController threshold, attempt multi-block region
			// compilation. Region promotion overwrites the entry-PC cache
			// slot with a single JITBlock spanning the whole region;
			// in-region chain exits become internal jumps eliminating
			// dispatcher round-trips on the hot path.
			block.execCount++
			if block.tier == 0 && m68kTierController.ShouldPromote(block.tier, block.execCount, block.ioBails, block.lastPromoteAt) {
				block.lastPromoteAt = block.execCount
				region := m68kFormRegion(pc, cpu.memory)
				if region != nil && len(region.blocks) >= 2 {
					newBlock, err := m68kCompileRegion(region, execMem, cpu.memory)
					if err == nil {
						newBlock.execCount = block.execCount
						newBlock.tier = 1
						cpu.m68kJitCache.Put(newBlock)
						if newBlock.chainEntry != 0 {
							cpu.m68kJitCache.PatchChainsTo(newBlock.startPC, newBlock.chainEntry)
						}
						// Patch the region's outgoing chain slots against
						// already-cached external successors.
						for i := range newBlock.chainSlots {
							slot := &newBlock.chainSlots[i]
							if target := cpu.m68kJitCache.Get(slot.targetPC); target != nil && target.chainEntry != 0 {
								PatchRel32At(slot.patchAddr, target.chainEntry)
							}
						}
						// Mark every page covered by the region's constituent
						// blocks so SMC invalidation fires for non-monotonic
						// regions (e.g. 0x100→0x5000→0x200) whose canonical
						// [startPC, endPC) span would skip middle blocks.
						if cpu.m68kJitCodeBitmap != nil {
							for _, r := range JITBlockCoveredRanges(newBlock) {
								startPage := r[0] >> 12
								endPage := (r[1] - 1) >> 12
								for p := startPage; p <= endPage; p++ {
									if p < uint32(len(cpu.m68kJitCodeBitmap)) {
										cpu.m68kJitCodeBitmap[p] = 1
									}
								}
							}
						}
						block = newBlock
					}
				}
			}
		}

		// Update 4-entry MRU RTS cache: shift entries down and write new
		// to entry 0. When RTS pops a return address, it probes all four
		// entries for a chain hit before bailing to the Go dispatcher.
		if block.chainEntry != 0 {
			ctx.RTSCache3PC = ctx.RTSCache2PC
			ctx.RTSCache3Addr = ctx.RTSCache2Addr
			ctx.RTSCache2PC = ctx.RTSCache1PC
			ctx.RTSCache2Addr = ctx.RTSCache1Addr
			ctx.RTSCache1PC = ctx.RTSCache0PC
			ctx.RTSCache1Addr = ctx.RTSCache0Addr
			ctx.RTSCache0PC = block.startPC
			ctx.RTSCache0Addr = block.chainEntry
		}

		// Execute native code block
		ctx.NeedInval = 0
		ctx.NeedIOFallback = 0
		ctx.ChainBudget = 256 // blocks before returning to Go for interrupt check
		ctx.ChainCount = 0
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))

		// Read return values from context
		cpu.PC = ctx.RetPC
		executed := uint64(ctx.RetCount)
		if executed == 0 {
			// ChainCount may have accumulated instructions from chained blocks
			if ctx.ChainCount > 0 {
				executed = uint64(ctx.ChainCount)
			} else {
				executed = uint64(block.instrCount) // safety fallback
			}
		}

		// Self-modifying code: invalidate cache
		if ctx.NeedInval != 0 {
			cpu.m68kJitCache.Invalidate()
			execMem.Reset()
			ctx.NeedInval = 0
			// Clear code page bitmap
			clear(cpu.m68kJitCodeBitmap)
			// Clear RTS inline cache (old chainEntry addresses are now invalid)
			ctx.RTSCache0PC = 0
			ctx.RTSCache0Addr = 0
			ctx.RTSCache1PC = 0
			ctx.RTSCache1Addr = 0
			ctx.RTSCache2PC = 0
			ctx.RTSCache2Addr = 0
			ctx.RTSCache3PC = 0
			ctx.RTSCache3Addr = 0
		}

		// I/O fallback: re-execute the bailing instruction via interpreter
		if ctx.NeedIOFallback != 0 {
			ctx.NeedIOFallback = 0
			cpu.m68kInterpretOne()
			executed++ // count the re-executed instruction
			diagIOBails++
			diagFallbackInstr++
			if !cpu.running.Load() {
				break
			}
		}

		instructionCount += executed

		// ── Interrupt/exception checking (replicates cpu_m68k.go:2457-2485) ──
		if cpu.stopped.Load() {
			continue // defer to STOP handler at top
		}

		pendingException := cpu.pendingException.Load()
		if pendingException != 0 {
			cpu.pendingException.Store(0)
			cpu.ProcessException(uint8(pendingException))
		}

		pending := cpu.pendingInterrupt.Load()
		if pending != 0 {
			ipl := uint32((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
			for level := uint32(7); level >= 1; level-- {
				if pending&(1<<level) != 0 && (level > ipl || level == 7) {
					if cpu.ProcessInterrupt(uint8(level)) {
						for {
							old := cpu.pendingInterrupt.Load()
							if cpu.pendingInterrupt.CompareAndSwap(old, old&^(1<<level)) {
								break
							}
						}
					}
					break
				}
			}
		}

		// Performance monitoring
		if cpu.PerfEnabled {
			cpu.InstructionCount = instructionCount
			now := time.Now()
			if now.Sub(cpu.lastPerfReport) >= time.Second {
				elapsed := now.Sub(cpu.perfStartTime).Seconds()
				mips := float64(instructionCount) / elapsed / 1_000_000
				hitRate := float64(0)
				if diagCacheHits+diagCacheMisses > 0 {
					hitRate = float64(diagCacheHits) / float64(diagCacheHits+diagCacheMisses) * 100
				}
				fallbackPct := float64(0)
				if instructionCount > 0 {
					fallbackPct = float64(diagFallbackInstr) / float64(instructionCount) * 100
				}
				fmt.Printf("\rM68K JIT: %.2f MIPS | cache %.0f%% | fallback %.1f%% | io %d   ",
					mips, hitRate, fallbackPct, diagIOBails)
				cpu.lastPerfReport = now
			}
		}
	}

	if cpu.PerfEnabled {
		cpu.InstructionCount = instructionCount
	}
}
