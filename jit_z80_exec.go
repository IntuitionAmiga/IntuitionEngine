// jit_z80_exec.go - Z80 JIT dispatcher loop and CPU integration
//
// Closure-plan B.3.b: Z80 region promotion is intentionally absent.
// The existing chain-patching layer (z80EmitChainExit's patchable JMP
// rel32 → next block's chainEntry, with ChainCycles / ChainCount /
// ChainRIncrements accumulating across the chain and merging at the
// shared exit) already provides the runtime equivalent of
// region-fused execution. Single-JITBlock region fusion would add
// only marginal cache-locality wins (~2-3% on linear hot loops)
// against significant emit-pipeline cost: per-block invariants
// (cs.loopInfo / DJNZ pre-check, cs.djnzDeferredFlags peephole,
// hasBackwardBranch chainEntry mode, per-block flagsNeeded) do not
// compose across blocks without either a deep refactor or strict
// rejection of the DJNZ-loop blocks that dominate Z80 demos. The
// retire matches the precedent set by 6502 (B.4) — chain-patching
// covers it; revisit only if Phase 9 gate flags Z80 lagging.

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

// z80JITFlushAll performs a full cache flush: unpatch all chains, clear all
// blocks, reset executable memory, clear RTS cache, and clear code page bitmap.
func (cpu *CPU_Z80) z80JITFlushAll(ctx *Z80JITContext) {
	cpu.jitCache.UnpatchChainsInRange(0, 0x10000)
	cpu.jitCache.Invalidate()
	if em := cpu.getZ80JITExecMem(); em != nil {
		em.Reset()
	}
	ctx.RTSCache0PC = 0
	ctx.RTSCache0Addr = 0
	ctx.RTSCache1PC = 0
	ctx.RTSCache1Addr = 0
	for i := range cpu.codePageBitmap {
		cpu.codePageBitmap[i] = 0
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

	enableZ80PollWiring(adapter)

	execMem := cpu.getZ80JITExecMem()
	ctx := cpu.jitCtx.(*Z80JITContext)
	mem := adapter.bus.GetMemory()
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

	for cpu.running.Load() {
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

		// HALT: CPU is halted, tick and yield.
		// For benchmarks and normal use, break out after detecting HALT
		// so the caller can check cpu.Halted and decide what to do.
		if cpu.Halted {
			cpu.tick(4)
			break
		}

		// ── EI delay handling ──
		// If iffDelay > 0, EI was recently executed. The interpreter's
		// finishInstruction() decrements iffDelay per instruction. Since EI is
		// a block terminator, iffDelay was set by the last native block.
		// Execute exactly one instruction via interpreter so finishInstruction()
		// can process the countdown with per-instruction accuracy.
		if cpu.iffDelay > 0 {
			cpu.interpretZ80One()
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
			// Banked-write aliasing guard (same as NeedBail path below)
			if z80BankWindowsEnabled(adapter) {
				cpu.z80JITFlushAll(ctx)
			}
			continue
		}

		// ── PC page safety check ──
		// If the current PC is on a non-direct page (banked, VRAM, I/O),
		// the JIT scanner can't safely read opcodes from raw MachineBus memory.
		// Fall back to interpreter for this instruction.
		pc := cpu.PC
		if cpu.directPageBitmap[pc>>8] != 0 {
			if matched, retired, rInc := cpu.tryFastZ80MMIOPollLoop(adapter); matched {
				if rInc > 0 {
					r := cpu.R
					cpu.R = (r & 0x80) | ((r + byte(rInc)) & 0x7F)
				}
				if perfEnabled {
					cpu.InstructionCount += uint64(retired)
				}
				continue
			}
			cpu.interpretZ80One()
			diagFallbackInstr++
			if perfEnabled {
				cpu.InstructionCount++
			}
			if !cpu.running.Load() {
				break
			}
			if z80BankWindowsEnabled(adapter) {
				cpu.z80JITFlushAll(ctx)
			}
			continue
		}

		// ── Block lookup ──
		block := cpu.jitCache.Get(uint32(pc))
		if block == nil {
			// Scan block from raw memory (safe: PC is on a direct page)
			instrs := z80JITScanBlock(mem, pc, memSize, &cpu.directPageBitmap)

			if len(instrs) == 0 {
				// First instruction needs fallback (I/O, HALT, etc.)
				cpu.interpretZ80One()
				diagFallbackInstr++
				if perfEnabled {
					cpu.InstructionCount++
				}
				if !cpu.running.Load() {
					break
				}
				if z80BankWindowsEnabled(adapter) {
					cpu.z80JITFlushAll(ctx)
				}
				continue
			}

			// Compile the block
			var err error
			block, err = compileBlockZ80(instrs, pc, execMem, &cpu.codePageBitmap)
			if err != nil {
				// ExecMem likely exhausted — full reset and retry once
				cpu.z80JITFlushAll(ctx)
				block, err = compileBlockZ80(instrs, pc, execMem, &cpu.codePageBitmap)
				if err != nil {
					// Genuine failure — interpret one instruction and continue
					cpu.interpretZ80One()
					diagFallbackInstr++
					if perfEnabled {
						cpu.InstructionCount++
					}
					if !cpu.running.Load() {
						break
					}
					continue
				}
			}
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
			ctx.RTSCache0PC = block.startPC
			ctx.RTSCache0Addr = block.chainEntry
		}

		// Initialize chain state for this entry into native code.
		// All chain accumulators start at 0; native code ADDs to them.
		ctx.ChainBudget = 256
		ctx.ChainCount = 0
		ctx.ChainCycles = 0
		ctx.ChainRIncrements = 0
		ctx.CycleBudget = 2000 // ~500us at 4MHz — interrupt responsiveness budget

		// Execute the native code block (may chain across multiple blocks)
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
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

		// ── Handle NeedInval (self-mod: page-granular invalidation) ──
		if ctx.NeedInval != 0 {
			page := ctx.InvalPage
			lo := page << 8
			hi := lo + 256
			// Unpatch chain slots targeting invalidated range, then remove blocks
			cpu.jitCache.UnpatchChainsInRange(lo, hi)
			cpu.jitCache.InvalidateRange(lo, hi)
			ctx.NeedInval = 0
			// Clear RTS cache (invalidated blocks may have had chain entries)
			ctx.RTSCache0PC = 0
			ctx.RTSCache0Addr = 0
			ctx.RTSCache1PC = 0
			ctx.RTSCache1Addr = 0
		}

		// ── Handle NeedBail (re-execute current instruction via interpreter) ──
		if ctx.NeedBail != 0 {
			ctx.NeedBail = 0
			// cpu.PC was already set to the bailing instruction's start PC
			cpu.interpretZ80One()
			executed++
			if !cpu.running.Load() {
				break
			}
			// Banked-write aliasing guard: if any bank window is enabled,
			// the interpreter's Write() may have aliased a physical page
			// that has JIT-compiled code. Flush everything conservatively.
			if z80BankWindowsEnabled(adapter) {
				cpu.z80JITFlushAll(ctx)
			}
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
