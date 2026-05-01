// jit_exec.go - JIT dispatcher loop and CPU64 integration

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"
)

// compileBlockMMUInvocations counts how many times compileBlockMMU has
// been called. Used by the Phase 4d dispatch test
// (TestPhase4d_DispatchActuallyRoutesMMUWorkloadThroughMMUCompiler) to
// prove that ExecuteJIT routes MMU-on workloads through compileBlockMMU
// at runtime, not just structurally. Increments are atomic so the
// counter can be read concurrently with JIT execution without races.
var compileBlockMMUInvocations atomic.Uint64

// JIT configuration constants
const (
	jitExecMemSize = 16 * 1024 * 1024 // 16MB executable memory pool
)

// ie64TierController is the shared Phase 3 promotion controller bound
// to IE64's reference RegPressureProfile. Exec-loop cache-hit gate
// delegates to ShouldPromote so threshold tweaks apply uniformly.
//
// B.2.b uses this controller to drive region promotion only — IE64
// Tier-2 reg-map promotion (B.2.c) is retired with the same
// architectural blocker as M68K's B.1.c (no spare host scratch for
// per-block pinning of additional spilled regs without an emit-path
// refactor exceeding the slice budget).
var ie64TierController = NewTierController(IE64RegProfile)

// jitExecMem returns the typed *ExecMem from the cpu's any field.
func (cpu *CPU64) getJITExecMem() *ExecMem {
	if cpu.jitExecMem == nil {
		return nil
	}
	return cpu.jitExecMem.(*ExecMem)
}

// initJIT initializes JIT state on the CPU. Called once before execution.
func (cpu *CPU64) initJIT() error {
	if cpu.jitExecMem != nil {
		return nil // already initialized
	}
	execMem, err := AllocExecMem(jitExecMemSize)
	if err != nil {
		return fmt.Errorf("JIT init failed: %w", err)
	}
	cpu.jitExecMem = execMem
	cpu.jitCache = NewCodeCache()
	cpu.jitCtx = newJITContext(cpu)
	return nil
}

// freeJIT releases all JIT resources. If jitPersist is set (used by benchmarks),
// the code cache and exec memory are kept alive for reuse across runs.
func (cpu *CPU64) freeJIT() {
	if cpu.jitPersist {
		return
	}
	if em := cpu.getJITExecMem(); em != nil {
		em.Free()
		cpu.jitExecMem = nil
	}
	cpu.jitCache = nil
	cpu.jitCtx = nil
}

// compileBlockMMU wraps compileBlock, marking all guest-memory-touching
// instructions for interpreter bail. Used when MMU is enabled.
func compileBlockMMU(instrs []JITInstr, startPC uint32, execMem *ExecMem) (*JITBlock, error) {
	compileBlockMMUInvocations.Add(1)
	for i := range instrs {
		switch instrs[i].opcode {
		case OP_LOAD, OP_STORE, OP_FLOAD, OP_FSTORE, OP_DLOAD, OP_DSTORE,
			OP_JSR64, OP_RTS64, OP_PUSH64, OP_POP64, OP_JSR_IND,
			OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
			instrs[i].mmuBail = true
		}
	}
	return compileBlock(instrs, startPC, execMem)
}

// interpretOne executes one IE64 instruction at cpu.PC using the interpreter.
// Unlike StepOne(), this is designed to be called from within the JIT execution
// loop for instructions that can't be JIT-compiled (FPU, WAIT, HALT).
func (cpu *CPU64) interpretOne() {
	cpu.StepOne()
}

// ExecuteJIT is the main JIT execution loop. It replaces Execute() when
// JIT compilation is enabled.
func (cpu *CPU64) ExecuteJIT() {
	if !cpu.CoprocMode && (cpu.PC < PROG_START || cpu.PC >= STACK_START) {
		fmt.Printf("IE64 JIT: Invalid initial PC value: 0x%08x\n", cpu.PC)
		cpu.running.Store(false)
		return
	}

	if err := cpu.initJIT(); err != nil {
		fmt.Printf("IE64 JIT: %v, falling back to interpreter\n", err)
		cpu.Execute()
		return
	}
	defer cpu.freeJIT()

	enableIE64PollWiring(cpu)

	execMem := cpu.getJITExecMem()

	// Initialize performance measurement
	cpu.perfStartTime = time.Now()
	cpu.lastPerfReport = cpu.perfStartTime
	cpu.InstructionCount = 0
	perfEnabled := cpu.PerfEnabled

	// Diagnostic counters
	var diagBlocksExec uint64    // JIT blocks executed via callNative
	var diagBlockInstrs uint64   // sum of instrCount for JIT-executed blocks
	var diagCacheHits uint64     // block found in cache
	var diagCacheMisses uint64   // block compiled (cache miss)
	var diagFallbackInstr uint64 // instructions via interpretOne
	var diagIOBails uint64       // I/O fallback bail count

	// Use local running flag to avoid atomic load every iteration
	running := true
	checkCounter := uint32(0)

	for running {
		// M15.6 G2 Phase 2c-trap: trap-frame stack overflow sets
		// trapHalted in pushTrapFrame. Poll it every iteration so a
		// runaway nested-fault kernel bug stops the JIT dispatcher on
		// the same block boundary it failed, not at the next periodic
		// cpu.running poll.
		if cpu.trapHalted {
			break
		}
		// Periodic check of external stop signal
		checkCounter++
		if checkCounter&0xFFF == 0 && !cpu.running.Load() {
			break
		}

		// MMU state change: flush cache before any block lookup
		// This must be at the TOP of the loop because interpretOne() paths
		// (needsFallback, compilation failure) use continue and skip post-callNative checks.
		if cpu.jitNeedInval {
			cpu.jitCache.Invalidate()
			execMem.Reset()
			cpu.jitNeedInval = false
			cpu.jitCtx.RTSCache0PC = 0
			cpu.jitCtx.RTSCache0Addr = 0
			cpu.jitCtx.RTSCache1PC = 0
			cpu.jitCtx.RTSCache1Addr = 0
			cpu.jitCtx.RTSCache2PC = 0
			cpu.jitCtx.RTSCache2Addr = 0
			cpu.jitCtx.RTSCache3PC = 0
			cpu.jitCtx.RTSCache3Addr = 0
		}

		// PLAN_MAX_RAM.md slice 4 design: drop the legacy IE64_ADDR_MASK
		// truncation so fault reporting and the MMU walk see the full
		// 64-bit VA. The JIT compiler still works in uint32 VA / phys
		// space; falls back to the interpreter for any VA or translated
		// physical address that does not fit in the legacy 32 MB window.
		// JIT-side widening to uint64 PC and high-phys block fetch is a
		// later phase; this change just removes the aliasing hazard.
		if cpu.PC > 0xFFFFFFFF {
			cpu.interpretOne()
			cpu.InstructionCount++
			diagFallbackInstr++
			if !cpu.running.Load() {
				break
			}
			continue
		}
		pcVirt := uint32(cpu.PC)

		// MMU: translate virtual PC to physical for block fetch
		pcPhys := pcVirt
		if cpu.mmuEnabled {
			phys, fault, cause := cpu.translateAddr(uint64(pcVirt), ACCESS_EXEC)
			if fault {
				cpu.trapFault(cause, uint64(pcVirt))
				continue // re-enter loop at trap handler PC
			}
			memLen := uint64(len(cpu.memory))
			// Subtraction form: phys+8 wraps near MaxUint64 and would
			// admit a high physical address into the cpu.memory fast
			// path. memLen is always >= IE64_INSTR_SIZE so the
			// subtraction never underflows.
			if phys > memLen-IE64_INSTR_SIZE {
				// High-phys executable page: fall back to the interpreter,
				// whose fetch path routes through bus.ReadPhys64. The JIT
				// block builder is not yet wired for high-phys fetch.
				cpu.interpretOne()
				cpu.InstructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
			}
			pcPhys = uint32(phys)
		}

		// Bounds check (on physical address). PLAN_MAX_RAM.md slice 4:
		// when the MMU is disabled and pcPhys lands above the legacy
		// bus.memory[] window, fall back to the interpreter rather
		// than halting. The interpreter fetch path routes high-phys
		// reads through bus.ReadPhys64WithFault and can execute code
		// placed in backed RAM above 32 MB; the JIT block builder
		// scans cpu.memory[] directly and is not yet wired for high
		// phys.
		if uint64(pcPhys)+IE64_INSTR_SIZE > uint64(len(cpu.memory)) {
			cpu.interpretOne()
			cpu.InstructionCount++
			diagFallbackInstr++
			if !cpu.running.Load() {
				break
			}
			continue
		}

		// Check for HALT at current PC (physical)
		opcode := cpu.memory[pcPhys]
		if opcode == OP_HALT64 {
			cpu.running.Store(false)
			break
		}

		// Under MMU, different address spaces can execute different physical
		// code at the same virtual PC. Scope cache entries by PTBR to avoid
		// cross-task aliasing when the OS switches page tables.
		// PLAN_MAX_RAM.md slice 4: cpu.ptbr is uint64; the legacy
		// `(ptbr<<32) | pcVirt` packing dropped any PTBR bits above bit 31
		// and aliased two tasks whose page tables share the low 32 bits
		// of their physical address. Mix the full 64-bit PTBR via a
		// golden-ratio multiplicative hash so high-memory PTBRs produce
		// distinct cache keys.
		cacheKey := uint64(pcVirt)
		if cpu.mmuEnabled {
			cacheKey = (cpu.ptbr * 0x9E3779B97F4A7C15) ^ uint64(pcVirt)
		}
		block := cpu.jitCache.GetKey(cacheKey)
		if block == nil {
			// Scan from physical memory
			var instrs []JITInstr
			if cpu.mmuEnabled {
				// Page boundary limit: don't scan past end of current 4 KiB page
				pageEnd := (pcPhys & ^uint32(MMU_PAGE_MASK)) + MMU_PAGE_SIZE
				instrs = scanBlockWithLimit(cpu.memory, pcPhys, pageEnd)
			} else {
				instrs = scanBlock(cpu.memory, pcPhys)
			}

			if needsFallback(instrs) {
				// FPU, WAIT, RTI, MMU ops — use interpreter for single instruction
				cpu.interpretOne()
				cpu.InstructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
			}

			var err error
			if cpu.mmuEnabled {
				// Compile with virtual startPC (branch offsets are virtual)
				block, err = compileBlockMMU(instrs, pcVirt, execMem)
			} else {
				block, err = compileBlock(instrs, pcPhys, execMem)
			}
			if err != nil {
				// Compilation failed (e.g., exec mem exhausted) — interpret
				cpu.interpretOne()
				cpu.InstructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
			}
			// Tag the block with its compile-time PTBR so the chain
			// patcher can keep cross-address-space blocks isolated. Non-
			// MMU mode uses ptbr=0 implicitly, which preserves the
			// pre-MMU behavior.
			if cpu.mmuEnabled {
				block.ptbr = cpu.ptbr
			}
			cpu.jitCache.PutKey(cacheKey, block)

			// Bidirectional chain patching, scoped by MMU PTBR when
			// enabled. Two address spaces can share a virtual PC; without
			// scoping, a block compiled in one PTBR could chain into
			// physical code from another PTBR.
			//   1. Existing blocks holding chain slots that target this
			//      block get their JMP rel32 patched to our chainEntry.
			//   2. Our own outbound chain slots that target already-cached
			//      blocks get patched here.
			if block.chainEntry != 0 {
				if cpu.mmuEnabled {
					cpu.jitCache.PatchChainsToScoped(block.startPC, block.chainEntry, cpu.ptbr)
				} else {
					cpu.jitCache.PatchChainsTo(block.startPC, block.chainEntry)
				}
			}
			for i := range block.chainSlots {
				slot := &block.chainSlots[i]
				targetKey := uint64(slot.targetPC)
				if cpu.mmuEnabled {
					targetKey = (cpu.ptbr * 0x9E3779B97F4A7C15) ^ uint64(slot.targetPC)
				}
				if target := cpu.jitCache.GetKey(targetKey); target != nil && target.chainEntry != 0 {
					PatchRel32At(slot.patchAddr, target.chainEntry)
				}
			}

			diagCacheMisses++
		} else {
			diagCacheHits++

			// Hot-block detection — when execCount crosses the shared
			// TierController threshold, attempt multi-block region
			// compilation. Region promotion overwrites the entry-PC
			// cache slot with a single JITBlock spanning the whole
			// region; in-region BRA/JMP targets become direct JMP
			// rel32 jumps (skipping chain-exit reg-spill on the hot
			// intra-region branch).
			block.execCount++
			// Region promotion is currently MMU-disabled: ie64FormRegion
			// scans cpu.memory at flat physical indices and the in-region
			// successor walker assumes virtual==physical. Under MMU each
			// scanned block's virtual successor PC would need a separate
			// page-table walk to resolve to its physical address; that
			// machinery is not in place yet, and naively passing pcVirt
			// would scan unrelated bytes (when the virtual PC maps to a
			// different physical page) or silently disable valid
			// regions. Keep region promotion to non-MMU mode until the
			// scanner gains MMU awareness.
			if !cpu.mmuEnabled && block.tier == 0 && ie64TierController.ShouldPromote(block.tier, block.execCount, block.ioBails, block.lastPromoteAt) {
				block.lastPromoteAt = block.execCount
				region := ie64FormRegion(pcPhys, cpu.memory)
				if region != nil && len(region.blocks) >= 2 {
					newBlock, err := ie64CompileRegion(region, execMem, cpu.memory)
					if err == nil {
						newBlock.execCount = block.execCount
						newBlock.tier = 1
						if cpu.mmuEnabled {
							newBlock.ptbr = cpu.ptbr
						}
						cpu.jitCache.PutKey(cacheKey, newBlock)
						if newBlock.chainEntry != 0 {
							if cpu.mmuEnabled {
								cpu.jitCache.PatchChainsToScoped(newBlock.startPC, newBlock.chainEntry, cpu.ptbr)
							} else {
								cpu.jitCache.PatchChainsTo(newBlock.startPC, newBlock.chainEntry)
							}
						}
						for i := range newBlock.chainSlots {
							slot := &newBlock.chainSlots[i]
							targetKey := uint64(slot.targetPC)
							if cpu.mmuEnabled {
								targetKey = (cpu.ptbr * 0x9E3779B97F4A7C15) ^ uint64(slot.targetPC)
							}
							if target := cpu.jitCache.GetKey(targetKey); target != nil && target.chainEntry != 0 {
								PatchRel32At(slot.patchAddr, target.chainEntry)
							}
						}
						block = newBlock
					}
				}
			}
		}

		// Reset per-callNative chain dispatch state.
		cpu.jitCtx.ChainBudget = ie64ChainBudget
		cpu.jitCtx.ChainCount = 0

		// Update 4-entry MRU RTS cache: shift entries down and write the
		// just-resolved block at slot 0. RTS in chained-running blocks
		// probes these slots for fast unchained-RET avoidance.
		if block.chainEntry != 0 {
			cpu.jitCtx.RTSCache3PC = cpu.jitCtx.RTSCache2PC
			cpu.jitCtx.RTSCache3Addr = cpu.jitCtx.RTSCache2Addr
			cpu.jitCtx.RTSCache2PC = cpu.jitCtx.RTSCache1PC
			cpu.jitCtx.RTSCache2Addr = cpu.jitCtx.RTSCache1Addr
			cpu.jitCtx.RTSCache1PC = cpu.jitCtx.RTSCache0PC
			cpu.jitCtx.RTSCache1Addr = cpu.jitCtx.RTSCache0Addr
			cpu.jitCtx.RTSCache0PC = block.startPC
			cpu.jitCtx.RTSCache0Addr = block.chainEntry
		}

		// Execute the native code block
		callNative(block.execAddr, uintptr(unsafe.Pointer(cpu.jitCtx)))

		// Read packed PC+count from regs[0] (return channel).
		// Lower 32 bits = next PC, upper 32 bits = retired instruction count.
		combined := cpu.regs[0]
		cpu.regs[0] = 0
		cpu.PC = uint64(uint32(combined))
		executed := combined >> 32
		// ChainCount accumulates instruction counts retired by chained
		// predecessor blocks. Add it so retired-instruction accounting is
		// uniform with interpreter mode.
		executed += uint64(cpu.jitCtx.ChainCount)
		cpu.jitCtx.ChainCount = 0
		if executed == 0 {
			executed = uint64(block.instrCount) // safety fallback
		}

		ioBail := cpu.jitCtx.NeedIOFallback != 0
		if ioBail {
			diagIOBails++
		}
		diagBlocksExec++
		diagBlockInstrs += uint64(block.instrCount)

		// Self-modifying code: invalidate cache
		if cpu.jitCtx.NeedInval != 0 {
			cpu.jitCache.Invalidate()
			execMem.Reset()
			cpu.jitCtx.NeedInval = 0
			cpu.jitCtx.RTSCache0PC = 0
			cpu.jitCtx.RTSCache0Addr = 0
			cpu.jitCtx.RTSCache1PC = 0
			cpu.jitCtx.RTSCache1Addr = 0
			cpu.jitCtx.RTSCache2PC = 0
			cpu.jitCtx.RTSCache2Addr = 0
			cpu.jitCtx.RTSCache3PC = 0
			cpu.jitCtx.RTSCache3Addr = 0
		}

		// I/O fallback: re-execute the bailing instruction via interpreter
		if ioBail {
			cpu.jitCtx.NeedIOFallback = 0
			cpu.interpretOne()
			executed++ // count the re-executed instruction
			diagFallbackInstr++
			if !cpu.running.Load() {
				break
			}
		}

		// Timer check: decrement by number of executed instructions
		if cpu.timerEnabled.Load() {
			count := cpu.timerCount.Load()
			if count > 0 {
				if executed >= count {
					cpu.timerCount.Store(0)
					cpu.handleTimerJIT()
				} else {
					cpu.timerCount.Store(count - executed)
				}
			}
		}

		// Retired instruction count (uniform with interpreter)
		cpu.InstructionCount += executed

		// Performance reporting
		if perfEnabled && checkCounter&0x3FFF == 0 {
			now := time.Now()
			if now.Sub(cpu.lastPerfReport) >= time.Second {
				elapsed := now.Sub(cpu.perfStartTime).Seconds()
				mips := float64(cpu.InstructionCount) / elapsed / 1_000_000
				avgBlock := float64(0)
				if diagBlocksExec > 0 {
					avgBlock = float64(diagBlockInstrs) / float64(diagBlocksExec)
				}
				hitRate := float64(0)
				if diagCacheHits+diagCacheMisses > 0 {
					hitRate = float64(diagCacheHits) / float64(diagCacheHits+diagCacheMisses) * 100
				}
				fallbackPct := float64(0)
				if cpu.InstructionCount > 0 {
					fallbackPct = float64(diagFallbackInstr) / float64(cpu.InstructionCount) * 100
				}
				fmt.Printf("\rIE64 JIT: %.2f MIPS | avg blk %.1f | cache %.0f%% | fallback %.1f%% | io %d   ",
					mips, avgBlock, hitRate, fallbackPct, diagIOBails)
				cpu.lastPerfReport = now
			}
		}
	}
}

// handleTimerJIT handles timer expiry at JIT block boundaries.
// Called when TIMER_COUNT has reached 0.
func (cpu *CPU64) handleTimerJIT() {
	cpu.timerState.Store(TIMER_EXPIRED)
	if cpu.interruptEnabled.Load() && !cpu.inInterrupt.Load() {
		if cpu.mmuEnabled && cpu.intrVector != 0 {
			// ERET-model interrupt entry. On trap-frame stack overflow
			// trapEntry halts the CPU (sets trapHalted + cpu.running=false);
			// leave PC alone so the dispatcher's trapHalted check breaks
			// out on the next iteration rather than vectoring into the
			// interrupt handler on top of a halted CPU.
			if !cpu.trapEntry() {
				return
			}
			cpu.faultPC = cpu.PC
			cpu.faultAddr = 0
			cpu.faultCause = FAULT_TIMER
			cpu.PC = cpu.intrVector
		} else {
			// Legacy push-PC/RTI model
			cpu.inInterrupt.Store(true)
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			memSize := uint64(len(cpu.memory))
			if uint64(sp)+8 <= memSize {
				memBase := unsafe.Pointer(&cpu.memory[0])
				*(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp))) = cpu.PC
				cpu.PC = cpu.interruptVector
			}
		}
	}
	// Reload timer
	if cpu.timerEnabled.Load() {
		cpu.timerCount.Store(cpu.timerPeriod.Load())
	}
}
