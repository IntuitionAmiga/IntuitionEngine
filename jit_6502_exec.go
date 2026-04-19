// jit_6502_exec.go - 6502 JIT dispatcher loop and CPU integration

//go:build (amd64 && (linux || windows)) || (arm64 && linux)

package main

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// jit6502ExecMemSize is the executable memory pool size for 6502 JIT blocks.
// 4MB is sufficient since 6502 blocks are small (1-3 byte instructions, ~10-60 bytes native each).
const jit6502ExecMemSize = 4 * 1024 * 1024

// getJIT6502ExecMem returns the typed *ExecMem from the cpu's any field.
func (cpu *CPU_6502) getJIT6502ExecMem() *ExecMem {
	if cpu.jitExecMem == nil {
		return nil
	}
	return cpu.jitExecMem.(*ExecMem)
}

// initJIT6502 initializes JIT state on the CPU. Called once before execution.
func (cpu *CPU_6502) initJIT6502() error {
	if cpu.jitExecMem != nil {
		return nil // already initialized
	}
	execMem, err := AllocExecMem(jit6502ExecMemSize)
	if err != nil {
		return fmt.Errorf("6502 JIT init failed: %w", err)
	}
	cpu.jitExecMem = execMem
	cpu.jitCache = NewCodeCache()
	cpu.jitCtx = newJIT6502Context(cpu)
	return nil
}

// freeJIT6502 releases all JIT resources. If jitPersist is set (benchmarks),
// the code cache and exec memory are kept alive for reuse across runs.
func (cpu *CPU_6502) freeJIT6502() {
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

// interpret6502One executes one 6502 instruction at cpu.PC using the interpreter.
// This is a thin wrapper that bypasses Step()'s redundant SealMappings/interrupt
// overhead. The opcode handler mutates cpu.Cycles directly.
func (cpu *CPU_6502) interpret6502One() {
	cpu.ensureOpcodeTableReady()
	opcode := cpu.readByte(cpu.PC)
	cpu.PC++
	cpu.opcodeTable[opcode](cpu)
}

// ExecuteJIT6502 is the main JIT execution loop for the 6502.
func (cpu *CPU_6502) ExecuteJIT6502() {
	// ── Pre-loop invariants ──
	cpu.ensureOpcodeTableReady()
	if adapter, ok := cpu.memory.(*Bus6502Adapter); ok {
		if mb, ok := adapter.bus.(*MachineBus); ok {
			mb.SealMappings()
		}
	}

	// fastAdapter must be available for JIT (direct memory access)
	if cpu.fastAdapter == nil {
		cpu.Execute()
		return
	}

	// Initialize JIT (allocate ExecMem, CodeCache, Context AFTER SealMappings)
	if err := cpu.initJIT6502(); err != nil {
		fmt.Printf("6502 JIT: %v, falling back to interpreter\n", err)
		cpu.Execute()
		return
	}
	defer cpu.freeJIT6502()

	execMem := cpu.getJIT6502ExecMem()
	ctx := cpu.jitCtx
	mem := cpu.fastAdapter.memDirect
	memSize := len(mem)

	// Initialize performance measurement
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

	cpu.executing.Store(true)
	defer cpu.executing.Store(false)

	for cpu.running.Load() {
		// ── Per-block checks (every block boundary) ──

		// Pause at instruction boundary if Reset() requests it
		if cpu.resetting.Load() {
			cpu.resetAck.Store(true)
			for cpu.resetting.Load() {
				runtime.Gosched()
			}
			cpu.resetAck.Store(false)
			continue
		}

		// Check for RDY line hold
		if !cpu.rdyLine.Load() {
			cpu.rdyHold = true
			runtime.Gosched()
			continue
		}
		cpu.rdyHold = false

		// ── Interrupt check (before instruction fetch, matching interpreter order) ──
		if cpu.nmiPending.Load() {
			cpu.handleInterrupt(NMI_VECTOR, true)
			cpu.nmiPending.Store(false)
		} else if cpu.irqPending.Load() && cpu.SR&INTERRUPT_FLAG == 0 {
			cpu.handleInterrupt(IRQ_VECTOR, false)
			cpu.irqPending.Store(false)
		}

		// ── Block lookup and execution ──
		pc := cpu.PC
		if int(pc) >= memSize {
			cpu.running.Store(false)
			break
		}

		// Try cached block
		block := cpu.jitCache.Get(uint32(pc))
		if block == nil {
			// Scan and potentially compile a new block
			instrs := jit6502ScanBlock(mem, pc, memSize)
			if jit6502NeedsFallback(instrs) {
				// BRK, RTI, KIL, undocumented — use interpreter for single instruction
				cpu.interpret6502One()
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				// Only count the instruction if the CPU is still running,
				// matching the interpreter's accounting (cpu_six5go2.go:1622)
				if perfEnabled {
					cpu.InstructionCount++
				}
				continue
			}

			var err error
			block, err = compileBlock6502(instrs, pc, execMem, &cpu.codePageBitmap)
			if err != nil {
				// ExecMem likely exhausted — full reset and retry once
				cpu.jitCache.Invalidate()
				execMem.Reset()
				for i := range cpu.codePageBitmap {
					cpu.codePageBitmap[i] = 0
				}
				ctx.RTSCache0PC = 0
				ctx.RTSCache0Addr = 0
				ctx.RTSCache1PC = 0
				ctx.RTSCache1Addr = 0
				block, err = compileBlock6502(instrs, pc, execMem, &cpu.codePageBitmap)
				if err != nil {
					// Genuine failure — interpret one instruction and continue
					cpu.interpret6502One()
					if perfEnabled {
						cpu.InstructionCount++
					}
					diagFallbackInstr++
					if !cpu.running.Load() {
						break
					}
					continue
				}
			}
			cpu.jitCache.Put(block)

			// Bidirectional chain patching:
			// 1. Existing blocks exiting to this block → patch their slots
			if block.chainEntry != 0 {
				cpu.jitCache.PatchChainsTo(block.startPC, block.chainEntry)
			}
			// 2. This block's exits targeting already-cached blocks → patch our slots
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

		// Initialize chain budget and count for this entry into native code
		ctx.ChainBudget = 64
		ctx.ChainCount = 0

		// Execute the native code block
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))

		// ── Process block results ──
		cpu.PC = uint16(ctx.RetPC)
		cpu.Cycles += ctx.RetCycles
		ctx.RetCycles = 0
		executed := ctx.RetCount
		if executed == 0 && ctx.ChainCount > 0 {
			executed = ctx.ChainCount
		}
		ctx.RetCount = 0

		// ── Handle NeedInval (self-mod: page-granular invalidation) ──
		if ctx.NeedInval != 0 {
			page := ctx.InvalPage
			lo := page << 8
			hi := lo + 256
			// Unpatch chain slots targeting invalidated range, then remove blocks
			cpu.jitCache.UnpatchChainsInRange(lo, hi)
			cpu.jitCache.InvalidateRange(lo, hi)
			// Conservative: leave codePageBitmap stale (false positives are safe).
			// Stale entries cleared on full ExecMem exhaustion reset.
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
			cpu.interpret6502One()
			executed++
			if !cpu.running.Load() {
				break
			}
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
					fmt.Printf("\r6502 JIT: %.2f MIPS | cache %.0f%% | fallback %.1f%%   ",
						mips, hitRate, fallbackPct)
					cpu.lastPerfReport = now
				}
			}
		}
	}
}
