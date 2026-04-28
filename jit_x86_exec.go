// jit_x86_exec.go - x86 JIT execution loop
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

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
		fmt.Printf("x86 JIT: %v, falling back to interpreter\n", err)
		cpu.x86RunInterpreter()
		return
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
				cpu.Step()
				cpu.syncJITRegsFromNamed()
				instructionCount++
				diagFallbackInstr++
				continue
			}

			// Check if first instruction needs interpreter
			if x86NeedsFallback(instrs) {
				cpu.syncJITRegsToNamed()
				cpu.Step()
				cpu.syncJITRegsFromNamed()
				instructionCount++
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
				cpu.syncJITRegsToNamed()
				cpu.Step()
				cpu.syncJITRegsFromNamed()
				instructionCount++
				diagFallbackInstr++
				if cpu.Halted || !cpu.Running() {
					break
				}
				continue
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

			// Hot-block detection with profile-guided promotion
			block.execCount++
			shouldPromote := false
			if block.tier == 0 && block.execCount >= x86Tier2Threshold {
				// Only promote if: genuinely hot, not recently promoted, and not I/O heavy
				if block.lastPromoteAt == 0 { // never promoted
					if block.ioBails*4 < block.execCount { // less than 25% I/O bail rate
						shouldPromote = true
					}
				}
			}
			if shouldPromote {
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
				} else {
					// Fall back to single-block Tier 2
					instrs := x86ScanBlock(cpu.memory, pc)
					if len(instrs) > 0 && !x86NeedsFallback(instrs) {
						newBlock, err := x86CompileBlock(instrs, pc, execMem, cpu.memory, 1)
						if err == nil {
							newBlock.execCount = block.execCount
							newBlock.tier = 1
							cpu.x86JitCache.Put(newBlock)
							if newBlock.chainEntry != 0 {
								x86PatchCompatibleChainsTo(cpu.x86JitCache, newBlock)
							}
							for i := range newBlock.chainSlots {
								slot := &newBlock.chainSlots[i]
								if target := cpu.x86JitCache.Get(slot.targetPC); target != nil && target.chainEntry != 0 {
									if target.regMap == newBlock.regMap {
										PatchRel32At(slot.patchAddr, target.chainEntry)
									}
								}
							}
							block = newBlock
						}
					}
				}
			}
		}

		// Update RTS cache: shift entry 0 → 1, write new entry 0
		if block.chainEntry != 0 {
			ctx.RTSCache1PC = ctx.RTSCache0PC
			ctx.RTSCache1Addr = ctx.RTSCache0Addr
			ctx.RTSCache0PC = block.startPC
			ctx.RTSCache0Addr = block.chainEntry
		}

		// Execute native code block -- jitRegs is already canonical, no sync needed
		ctx.NeedInval = 0
		ctx.NeedIOFallback = 0
		ctx.ChainBudget = 64
		ctx.ChainCount = 0
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))

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
			cpu.Step()
			cpu.syncJITRegsFromNamed()
			executed++
			diagIOBails++
			diagFallbackInstr++
			if cpu.Halted || !cpu.Running() {
				break
			}
		}

		instructionCount += executed

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

// x86RunInterpreter is the fallback interpreter loop.
func (cpu *CPU_X86) x86RunInterpreter() {
	for cpu.Running() && !cpu.Halted {
		if cpu.x86JitEnabled && cpu.tryDemoAccelFrame() {
			continue
		}
		if cpu.tryFastMMIOPollLoop() {
			continue
		}
		cpu.Step()
	}
}
