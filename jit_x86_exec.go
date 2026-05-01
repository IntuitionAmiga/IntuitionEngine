// jit_x86_exec.go - x86 JIT execution loop
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

// x86 JIT is amd64-only (per CLAUDE.md: only IE64 has arm64 JIT).
// The aspirational `arm64 && linux` tag from earlier wiring rounds was
// never followed by an arm64 emit/compile implementation, so cross-
// builds fail with "undefined: x86CompileBlock" / "x86CompileRegion".
// Narrow to amd64-only until the arm64 emitter actually lands.

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
	enableX86PollWiring(cpu)
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

	execMem := cpu.x86GetJITExecMem()
	ctx := cpu.x86JitCtx

	// Diagnostic counters
	var instructionCount uint64
	var diagCacheHits uint64
	var diagCacheMisses uint64
	var diagFallbackInstr uint64
	var diagIOBails uint64

	// Performance monitoring
	var perfStartTime time.Time
	var lastPerfReport time.Time
	perfEnabled := false // Can be toggled via cpu field if needed

	if perfEnabled {
		perfStartTime = time.Now()
		lastPerfReport = perfStartTime
	}

	// Sync named CPU fields -> jitRegs ONCE at JIT entry.
	// jitRegs is the canonical state during JIT execution.
	cpu.syncJITRegsFromNamed()
	cpu.syncJITSegRegsFromNamed()

	for cpu.Running() && !cpu.Halted {
		// Check for pending interrupt (named fields are stale; sync first)
		if cpu.irqPending.Load() && cpu.IF() {
			cpu.syncJITRegsToNamed()
			cpu.handleInterrupt(byte(cpu.irqVector.Load()))
			cpu.irqPending.Store(false)
			cpu.syncJITRegsFromNamed()
		}

		// MMIO status spin loops dominate demo wait time. Handle the common
		// MOV/TEST/Jcc-back pattern directly so JIT-enabled execution doesn't
		// bounce through one-instruction fallbacks for every poll.
		cpu.syncJITRegsToNamed()
		if cpu.tryFastMMIOPollLoop() {
			cpu.syncJITRegsFromNamed()
			continue
		}
		cpu.syncJITRegsFromNamed()

		pc := cpu.EIP

		// Bounds check
		if pc >= uint32(len(cpu.memory)) {
			fmt.Printf("x86 JIT: EIP out of bounds: 0x%08X\n", pc)
			cpu.Halted = true
			break
		}

		// Try cache lookup
		block := cpu.x86JitCache.Get(pc)
		if block == nil {
			// Scan block
			instrs := x86ScanBlock(cpu.memory, pc)
			if len(instrs) == 0 {
				// Interpreter fallback: sync jitRegs -> named, step, sync back
				cpu.syncJITRegsToNamed()
				var stepT0 time.Time
				if perfAcctOn {
					stepT0 = time.Now()
				}
				cpu.Step()
				if perfAcctOn {
					cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
				}
				cpu.syncJITRegsFromNamed()
				instructionCount++
				if perfAcctOn {
					cpu.perfAcct.AddInstrs(1)
				}
				diagFallbackInstr++
				continue
			}

			// Check if first instruction needs interpreter
			if x86NeedsFallback(instrs) {
				cpu.syncJITRegsToNamed()
				var stepT0 time.Time
				if perfAcctOn {
					stepT0 = time.Now()
				}
				cpu.Step()
				if perfAcctOn {
					cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
				}
				cpu.syncJITRegsFromNamed()
				instructionCount++
				if perfAcctOn {
					cpu.perfAcct.AddInstrs(1)
				}
				diagFallbackInstr++
				if cpu.Halted || !cpu.Running() {
					break
				}
				continue
			}

			// Compile block (pass bitmaps for compile-time page checks)
			x86CompileIOBitmap = cpu.x86JitIOBitmap
			x86CompileCodeBitmap = cpu.x86JitCodeBM
			var err error
			block, err = x86CompileBlock(instrs, pc, execMem, cpu.memory)
			if err != nil {
				// "no instructions compiled" means the first scanned instr
				// fell through every emit case — equivalent to an
				// x86NeedsFallback hit that the static list missed. Treat
				// as a per-instruction Step bail (the same protocol as
				// MMIO bail). Any other compile error is a real JIT bug
				// and panics so the gap is fixed at its source.
				if err.Error() == "no instructions compiled" {
					cpu.syncJITRegsToNamed()
					var stepT0 time.Time
					if perfAcctOn {
						stepT0 = time.Now()
					}
					cpu.Step()
					if perfAcctOn {
						cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
					}
					cpu.syncJITRegsFromNamed()
					instructionCount++
					if perfAcctOn {
						cpu.perfAcct.AddInstrs(1)
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
			if cpu.x86JitCodeBM != nil {
				startPage := block.startPC >> 8
				endPage := (block.endPC - 1) >> 8
				for p := startPage; p <= endPage; p++ {
					if p < uint32(len(cpu.x86JitCodeBM)) {
						cpu.x86JitCodeBM[p] = 1
					}
				}
			}

			// Patch chain slots bidirectionally -- only for compatible register maps
			if block.chainEntry != 0 {
				x86PatchCompatibleChainsTo(cpu.x86JitCache, block)
			}
			for i := range block.chainSlots {
				slot := &block.chainSlots[i]
				if target := cpu.x86JitCache.Get(slot.targetPC); target != nil && target.chainEntry != 0 {
					if target.regMap == block.regMap {
						PatchRel32At(slot.patchAddr, target.chainEntry)
					}
				}
			}

			diagCacheMisses++
		} else {
			diagCacheHits++

			// Hot-block detection via shared Phase 3 TierController.
			// Equivalent arithmetic to the prior inline gate (execCount >=
			// 64 && lastPromoteAt == 0 && ioBails*4 < execCount).
			block.execCount++
			if x86TierController.ShouldPromote(block.tier, block.execCount, block.ioBails, block.lastPromoteAt) {
				block.lastPromoteAt = block.execCount
				// Try multi-block region compilation first (only for 3+ block regions)
				x86CompileIOBitmap = cpu.x86JitIOBitmap
				x86CompileCodeBitmap = cpu.x86JitCodeBM
				region := x86FormRegion(pc, cpu.x86JitCache, cpu.memory)
				if region != nil && len(region.blocks) >= 3 {
					newBlock, err := x86CompileRegion(region, execMem, cpu.memory)
					if err == nil {
						newBlock.execCount = block.execCount
						cpu.x86JitCache.Put(newBlock)
						if newBlock.chainEntry != 0 {
							x86PatchCompatibleChainsTo(cpu.x86JitCache, newBlock)
						}
						block = newBlock
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
		// the running block — without that gate, a Tier-2 callee could
		// chain back into a Tier-1 caller (or vice versa) with mapped
		// guest registers reading the wrong host registers.
		if block.chainEntry != 0 {
			ctx.RTSCache1PC = ctx.RTSCache0PC
			ctx.RTSCache1Addr = ctx.RTSCache0Addr
			ctx.RTSCache1RegMap = ctx.RTSCache0RegMap
			ctx.RTSCache0PC = block.startPC
			ctx.RTSCache0Addr = block.chainEntry
			ctx.RTSCache0RegMap = x86RegMapToUint64(block.regMap)
		}

		// Execute native code block -- jitRegs is already canonical, no sync needed
		ctx.NeedInval = 0
		ctx.NeedIOFallback = 0
		ctx.ChainBudget = 65536
		ctx.ChainCount = 0
		var jitT0 time.Time
		if perfAcctOn {
			jitT0 = time.Now()
		}
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
			} else {
				executed = uint64(block.instrCount)
			}
		}

		// Profile counters
		if ctx.ChainCount > 0 {
			block.chainHits++
		}
		if ctx.ChainBudget <= 0 {
			block.unchainedExits++ // budget exhausted = unchained exit
		}

		// Self-modifying code: invalidate cache
		if ctx.NeedInval != 0 {
			cpu.x86JitCache.Invalidate()
			execMem.Reset()
			ctx.NeedInval = 0
			clear(cpu.x86JitCodeBM)
			// Clear RTS cache (old chain entry addresses invalid)
			ctx.RTSCache0PC = 0
			ctx.RTSCache0Addr = 0
			ctx.RTSCache1PC = 0
			ctx.RTSCache1Addr = 0
		}

		// I/O fallback: sync to named, interpreter step, sync back
		if ctx.NeedIOFallback != 0 {
			ctx.NeedIOFallback = 0
			block.ioBails++ // profile counter for promotion decisions
			cpu.syncJITRegsToNamed()
			var stepT0 time.Time
			if perfAcctOn {
				stepT0 = time.Now()
			}
			cpu.Step()
			if perfAcctOn {
				cpu.perfAcct.AddInterp(time.Since(stepT0).Nanoseconds())
			}
			cpu.syncJITRegsFromNamed()
			executed++
			diagIOBails++
			diagFallbackInstr++
			if cpu.Halted || !cpu.Running() {
				break
			}
		}

		instructionCount += executed
		if perfAcctOn {
			cpu.perfAcct.AddInstrs(executed)
		}

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
	cpu.syncJITRegsToNamed()
	cpu.syncJITSegRegsToNamed()
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

// x86RunInterpreter is the fallback interpreter loop. Used when JIT is
// disabled (CPUX86Runner.JITEnabled=false). Slice 4 retired the
// tryDemoAccelFrame rotozoomer-specific shortcut here — the general
// native JIT now drives that workload via x86JitExecute → X86ExecuteJIT.
// The interp-only path keeps only the workload-agnostic
// tryFastMMIOPollLoop fast match for status-poll loops.
func (cpu *CPU_X86) x86RunInterpreter() {
	for cpu.Running() && !cpu.Halted {
		if cpu.tryFastMMIOPollLoop() {
			continue
		}
		cpu.Step()
	}
}
