// jit_6502_exec.go - 6502 JIT dispatcher loop and CPU integration

// Native 6502 JIT execution is implemented for Linux AMD64 and ARM64.
// Other targets use the dispatcher stub until their backend is available.

//go:build (amd64 || arm64) && linux

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
	if adapter, ok := cpu.memory.(*Bus6502Adapter); ok {
		if bus, ok := adapter.bus.(*MachineBus); ok {
			cpu.jitBusUnregister = bus.RegisterP65JITInvalidator(cpu.noteP65JITWrite)
		}
	}
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
	if cpu.jitBusUnregister != nil {
		cpu.jitBusUnregister()
		cpu.jitBusUnregister = nil
	}
}

// noteP65JITWrite is safe for any bus writer. It publishes affected 256-byte
// 6502 code pages but never changes the cache itself.
func (cpu *CPU_6502) noteP65JITWrite(addr, size uint64) {
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
	for page := addr >> 8; page <= end>>8; page++ {
		cpu.jitCodeGen[page].Add(1)
	}
	cpu.jitDispatchGen.Add(1)
}

// drainP65JITInvalidations performs the CPU-owned cache mutation promised by
// the bus callback contract. It runs only at dispatcher block boundaries.
func (cpu *CPU_6502) drainP65JITInvalidations(ctx *JIT6502Context) {
	for page := range cpu.jitCodeGen {
		generation := cpu.jitCodeGen[page].Load()
		if generation == cpu.jitSeenCodeGen[page] {
			continue
		}
		cpu.jitSeenCodeGen[page] = generation
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

func p65BlockSourceMatches(mem []byte, pc uint16, block *JITBlock) bool {
	if block == nil || len(block.p65Source) == 0 || uint64(pc) != block.startPC {
		return false
	}
	for i, value := range block.p65Source {
		if mem[uint16(uint32(pc)+uint32(i))] != value {
			return false
		}
	}
	return true
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

	// AddressIsMMIOPredicate is set per call from the live bus at each
	// TryFastMMIOPoll site; wiring the shared global P65PollPattern here is dead
	// and races across concurrent CPUs. See jit_exec.go.

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
	interpretFallback := func(reason DeoptReason) {
		var interpT0 time.Time
		if perfAcctOn {
			interpT0 = time.Now()
		}
		cpu.interpret6502One()
		if perfAcctOn {
			cpu.perfAcct.AddInterpSince(interpT0)
			cpu.perfAcct.AddInstrs(1)
			cpu.deoptStats.Add(reason)
		}
	}

	cpu.executing.Store(true)
	defer cpu.executing.Store(false)

	for cpu.running.Load() {
		if cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			break
		}
		cpu.drainP65JITInvalidations(ctx)
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
		// BRK is native only when no fault observer is attached. The observer
		// must see the original opcode PC before BRK changes architectural
		// state, which is exactly the interpreter's observation point.
		if mem[pc] == 0x00 && cpu.debugFaults != nil {
			interpretFallback(DeoptUnsupported)
			diagFallbackInstr++
			if !cpu.running.Load() {
				break
			}
			if perfEnabled {
				cpu.InstructionCount++
			}
			if cpu.jitTestRetire(1) {
				break
			}
			continue
		}
		// The MMIO poll accelerator retires a whole recognised loop at once.
		// Deterministic parity tests request an exact guest-instruction boundary,
		// so execute the normal one-instruction native/bail path in that mode.
		if cpu.jitTestStopAfter == 0 {
			if adapter, ok := cpu.memory.(*Bus6502Adapter); ok {
				if matched, retired := cpu.tryFast6502MMIOPollLoop(adapter); matched {
					if perfEnabled {
						cpu.InstructionCount += uint64(retired)
					}
					if perfAcctOn {
						cpu.perfAcct.AddInstrs(uint64(retired))
					}
					continue
				}
			}
		}

		// Try cached block
		block := cpu.jitCache.Get(uint64(pc))
		if block != nil && !p65BlockSourceMatches(mem, pc, block) {
			cpu.jitCache.UnpatchChainsInRange(block.startPC, block.endPC)
			cpu.jitCache.InvalidateRange(block.startPC, block.endPC)
			block = nil
		}
		if block == nil {
			// Scan and potentially compile a new block
			instrs := jit6502ScanBlockLimit(mem, pc, memSize, cpu.jitTestBlockLimit)
			if jit6502NeedsFallback(instrs) {
				// BRK, RTI, KIL, undocumented — use interpreter for single instruction
				interpretFallback(DeoptUnsupported)
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				// Only count the instruction if the CPU is still running,
				// matching the interpreter's accounting (cpu_six5go2.go:1622)
				if perfEnabled {
					cpu.InstructionCount++
				}
				if cpu.jitTestRetire(1) {
					break
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
					interpretFallback(DeoptCachePressure)
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
			cpu.jitStats.tier1Blocks.Add(1)

			// Bidirectional chain patching:
			// 1. Existing blocks exiting to this block → patch their slots
			if block.chainEntry != 0 {
				cpu.jitCache.PatchChainsTo(block.startPC, block.chainEntry)
			}
			// 2. This block's exits targeting already-cached blocks → patch our slots
			for i := range block.chainSlots {
				slot := &block.chainSlots[i]
				if target := cpu.jitCache.Get(slot.targetPC); target != nil && target.chainEntry != 0 {
					patchChainSlot(*slot, target.chainEntry)
				}
			}

			diagCacheMisses++
		} else {
			diagCacheHits++
		}

		block.execCount++

		// Update 2-entry MRU RTS cache before execution
		if block.chainEntry != 0 {
			ctx.RTSCache1PC = ctx.RTSCache0PC
			ctx.RTSCache1Addr = ctx.RTSCache0Addr
			ctx.RTSCache0PC = uint32(block.startPC)
			ctx.RTSCache0Addr = block.chainEntry
		}

		// Initialize chain budget and count for this entry into native code.
		// 6502 blocks are tiny and call-heavy code pays heavily for every
		// callNative round trip, so allow longer patched chains while still
		// returning well below jitBudget for interrupt/reset polling.
		ctx.ChainBudget = 1024
		if cpu.jitTestStopAfter != 0 {
			ctx.ChainBudget = 1
		}
		ctx.ChainCount = 0
		ctx.DispatchGeneration = cpu.jitDispatchGen.Load()
		ctx.BackendMarker = 0

		// Execute the native code block
		var jitT0 time.Time
		if perfAcctOn {
			jitT0 = time.Now()
		}
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
		cpu.jitStats.nativeEntries.Add(1)
		if perfAcctOn {
			cpu.perfAcct.AddJitSince(jitT0)
		}

		// ── Process block results ──
		cpu.PC = uint16(ctx.RetPC)
		cpu.Cycles += ctx.RetCycles
		ctx.RetCycles = 0
		executed := ctx.RetCount
		if executed == 0 && ctx.ChainCount > 0 {
			executed = ctx.ChainCount
		}
		if ctx.ChainCount > 0 {
			cpu.jitStats.chainExits.Add(1)
		}
		ctx.RetCount = 0

		// ── Handle NeedInval (self-mod: page-granular invalidation) ──
		if ctx.NeedInval != 0 {
			recordBlockDeopt(&cpu.deoptStats, block, DeoptSMC)
			cpu.jitStats.invalidations.Add(1)
			page := ctx.InvalPage
			// Native stores bypass MachineBus.Write8. Once a store has reached
			// the SMC exit, publish the affected physical page to every other
			// 6502 cache as well as invalidating this CPU's local block map.
			if adapter, ok := cpu.memory.(*Bus6502Adapter); ok {
				if bus, ok := adapter.bus.(*MachineBus); ok {
					bus.notifyP65JITRAMWrite(uint64(page)<<8, 256)
				}
			}
			lo := page << 8
			hi := lo + 256
			// Unpatch chain slots targeting invalidated range, then remove blocks
			cpu.jitCache.UnpatchChainsInRange(uint64(lo), uint64(hi))
			cpu.jitCache.InvalidateRange(uint64(lo), uint64(hi))
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
			recordBlockDeopt(&cpu.deoptStats, block, DeoptMMIO)
			block.ioBails++
			cpu.jitStats.bails.Add(1)
			var interpT0 time.Time
			if perfAcctOn {
				interpT0 = time.Now()
			}
			cpu.interpret6502One()
			if perfAcctOn {
				cpu.perfAcct.AddInterpSince(interpT0)
			}
			executed++
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
					fmt.Printf("\r6502 JIT: %.2f MIPS | cache %.0f%% | fallback %.1f%%   ",
						mips, hitRate, fallbackPct)
					cpu.lastPerfReport = now
				}
			}
		}
		if cpu.jitTestRetire(executed) {
			break
		}
	}
}
