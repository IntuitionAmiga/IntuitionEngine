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
// IE64 uses this controller to drive the turbo region tier. Single-block
// promotion remains retired; hot non-MMU code promotes at region granularity
// where the planner can reason across loops and static control-flow edges.
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

// compileBlockMMU wraps compileBlock for MMU-enabled execution. Phase 5:
// all data, stack, and control-flow memory ops (LOAD/STORE/FLOAD/FSTORE/
// DLOAD/DSTORE/JSR/RTS/PUSH/POP/JSR_IND) now carry a runtime MMUEnabled
// check in their native emitters and route through the JITContext HELPER_*
// protocol when MMU is on (the dispatcher in jit_helper_dispatch.go services
// them via the interpreter's MMU-aware loadMem/storeMem/mmuStack* helpers).
// They no longer need a compile-time mmuBail.
//
// Atomics (CAS/XCHG/FAA/FAND/FOR/FXOR) remain an explicit interpreter-bail
// carveout: sequential consistency requires the Go runtime, and no helper
// op exists for them.
//
// Fused JSR/RTS leaf markers (ie64FusedJSRLeafCall / ie64FusedRTSLeafReturn,
// set at scan time on amd64) also still bail: their inlined fast path
// (jit_emit_amd64.go:1231) emits raw [MemBase+SP] stack traffic with no
// runtime MMUEnabled check, so under MMU it would bypass VA translation.
// Flagging mmuBail makes the fused fast path fall through to the normal
// OP_JSR64 / OP_RTS64 case, which bails to the interpreter.
func compileBlockMMU(instrs []JITInstr, startPC uint64, execMem *ExecMem) (*JITBlock, error) {
	compileBlockMMUInvocations.Add(1)
	for i := range instrs {
		switch instrs[i].opcode {
		case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
			instrs[i].mmuBail = true
		}
		if instrs[i].fusedFlag&(ie64FusedJSRLeafCall|ie64FusedRTSLeafReturn) != 0 {
			instrs[i].mmuBail = true
		}
	}
	return compileBlock(instrs, startPC, execMem)
}

// interpretOne executes one IE64 instruction at cpu.PC using the interpreter.
// Unlike StepOne(), this is designed to be called from within the JIT execution
// loop for instructions that can't be JIT-compiled (FPU, WAIT, HALT).
func (cpu *CPU64) interpretOne() int {
	return cpu.StepOne()
}

// ExecuteJIT is the main JIT execution loop. It replaces Execute() when
// JIT compilation is enabled.
func (cpu *CPU64) ExecuteJIT() {
	if !cpu.CoprocMode && cpu.PC < PROG_START {
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
	statsEnabled := ie64TurboStatsEnabled()
	statsBase := ie64TurboStatsLoad()

	// Use local running flag to avoid atomic load every iteration
	running := true
	checkCounter := uint32(0)

	for running {
		if cpu.debugHandleBreakIn(cpu.PC) {
			break
		}
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
			globalIE64TurboStats.invalidations.Add(1)
			cpu.jitCtx.RTSCache0PC = 0
			cpu.jitCtx.RTSCache0Addr = 0
			cpu.jitCtx.RTSCache1PC = 0
			cpu.jitCtx.RTSCache1Addr = 0
			cpu.jitCtx.RTSCache2PC = 0
			cpu.jitCtx.RTSCache2Addr = 0
			cpu.jitCtx.RTSCache3PC = 0
			cpu.jitCtx.RTSCache3Addr = 0
		}

		if cpu.timerEnabled.Load() {
			executed := cpu.interpretOne()
			if executed == 0 {
				cpu.running.Store(false)
				break
			}
			cpu.InstructionCount += uint64(executed)
			diagFallbackInstr += uint64(executed)
			if !cpu.running.Load() {
				break
			}
			continue
		}

		// Phase 4: full uint64 virtual PC through the JIT dispatcher.
		// Earlier phases (1/2/3) widened the emitter and block
		// infrastructure but kept ExecuteJIT in uint32 PC space, falling
		// back to the interpreter whenever cpu.PC > 0xFFFFFFFF or
		// translated phys > len(cpu.memory). With block fetch widened to
		// bus.ReadPhys64WithFault, the JIT now compiles code at any
		// physical address that the bus can serve.
		pcVirt := cpu.PC

		// MMU: translate virtual PC to full 64-bit physical for block fetch.
		pcPhys := pcVirt
		if cpu.mmuEnabled {
			phys, fault, cause := cpu.translateAddr(pcVirt, ACCESS_EXEC)
			if fault {
				cpu.trapFault(cause, pcVirt)
				continue // re-enter loop at trap handler PC
			}
			pcPhys = phys
		}

		if matched, retired := cpu.tryFastIE64MMIOPollLoop(); matched {
			if perfEnabled {
				cpu.InstructionCount += uint64(retired)
			}
			continue
		}

		// HALT detection. For low pcPhys inside the cpu.memory window,
		// keep the cheap byte index; for high pcPhys, fetch the 8-byte
		// instruction word through the bus so an unmapped page stops
		// the JIT cleanly rather than panicking on an out-of-range
		// slice index.
		memLen := uint64(len(cpu.memory))
		var opcode byte
		var instrWord uint64
		fetchedFromBus := false
		if pcPhys <= memLen-IE64_INSTR_SIZE {
			opcode = cpu.memory[pcPhys]
		} else {
			fetched, ok := cpu.bus.ReadPhys64WithFault(pcPhys)
			if !ok {
				// Unmapped physical instruction fetch — matches
				// interpreter behaviour (cpu_ie64.go fetch path) by
				// stopping execution. trapFault is not raised here
				// because translateAddr already returned ok for the
				// virtual page; an unmapped backing at the physical
				// address surfaces as a halt, not a translation trap.
				cpu.running.Store(false)
				break
			}
			instrWord = fetched
			opcode = byte(instrWord)
			fetchedFromBus = true
		}
		if opcode == OP_HALT64 {
			cpu.running.Store(false)
			break
		}

		// Turbo-program fast path is a low-memory specialisation: it
		// indexes cpu.memory[] directly and only matches a narrow
		// recognised pattern at PROG_START-aligned offsets. Skip it
		// for high pcPhys (which is necessarily outside cpu.memory)
		// and let the normal JIT block builder run.
		if !cpu.mmuEnabled && !fetchedFromBus && opcode == OP_MOVE && ie64TurboStartCandidate(cpu.memory, uint32(pcPhys)) && ie64TurboEnabled() {
			if matched, retired := cpu.tryIE64TurboProgram(uint32(pcPhys), statsEnabled); matched {
				if statsEnabled {
					globalIE64TurboStats.turboRegions.Add(1)
				}
				cpu.InstructionCount += retired
				if !cpu.running.Load() {
					break
				}
				continue
			}
		}
		_ = instrWord

		// Under MMU, different address spaces can execute different physical
		// code at the same virtual PC. Use the dedicated MMU cache map with
		// an exact (ptbr, vPC) composite key; the legacy lossy hash could
		// collide distinct {ptbr, pc} pairs into the same slot.
		var block *JITBlock
		if cpu.mmuEnabled {
			block = cpu.jitCache.GetMMU(cpu.ptbr, pcVirt)
		} else {
			block = cpu.jitCache.Get(pcVirt)
		}
		if block == nil {
			// Scan: low pcPhys uses the cpu.memory[] fast path;
			// high pcPhys (sparse backing or MMU mapping above the
			// legacy window) routes per-instruction through the
			// bus phys helper.
			var instrs []JITInstr
			highPhys := pcPhys > memLen-IE64_INSTR_SIZE
			if cpu.mmuEnabled {
				// Page boundary limit: don't scan past end of current 4 KiB page
				pageEnd := (pcPhys & ^uint64(MMU_PAGE_MASK)) + MMU_PAGE_SIZE
				if !highPhys && pageEnd <= memLen {
					instrs = scanBlockWithLimit(cpu.memory, pcPhys, pageEnd)
				} else {
					instrs = scanBlockBusWithLimit(cpu.bus, pcPhys, pageEnd)
				}
			} else {
				if !highPhys {
					instrs = scanBlock(cpu.memory, pcPhys)
				} else {
					instrs = scanBlockBus(cpu.bus, pcPhys)
				}
			}

			// Phase 4 interim guard. The non-MMU stack emitters
			// (PUSH/POP/JSR/RTS/JSR_IND) still emit raw [memBase+SP]
			// loads/stores with no high-address bail. With Phase 4
			// now JIT-compiling code at high pcPhys, a stack op
			// inside such a block whose SP is also in high RAM
			// would read/write past cpu.memory and corrupt or
			// crash the host. Until Phase 5 routes stack ops
			// through bus-aware helpers, bail any high-phys block
			// containing a stack op back to the interpreter
			// (which uses cpu.mmuStackRead/Write through the bus).
			if highPhys && containsStackOp(instrs) {
				cpu.interpretOne()
				cpu.InstructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
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
			globalIE64TurboStats.tier1Blocks.Add(1)
			// Tag the block with its compile-time PTBR so the chain
			// patcher can keep cross-address-space blocks isolated. Non-
			// MMU mode uses ptbr=0 implicitly, which preserves the
			// pre-MMU behavior.
			if cpu.mmuEnabled {
				block.ptbr = cpu.ptbr
			}
			if cpu.mmuEnabled {
				cpu.jitCache.PutMMU(cpu.ptbr, pcVirt, block)
			} else {
				cpu.jitCache.Put(block)
			}

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
				var target *JITBlock
				if cpu.mmuEnabled {
					target = cpu.jitCache.GetMMU(cpu.ptbr, slot.targetPC)
				} else {
					target = cpu.jitCache.Get(slot.targetPC)
				}
				if target != nil && target.chainEntry != 0 {
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
			if !cpu.mmuEnabled && pcPhys <= memLen-IE64_INSTR_SIZE && block.tier == ie64JITTier1 && ie64TierController.ShouldPromote(block.tier, block.execCount, block.ioBails, block.lastPromoteAt) {
				globalIE64TurboStats.turboCandidates.Add(1)
				block.lastPromoteAt = block.execCount
				if !ie64TurboEnabled() {
					globalIE64TurboStats.turboRejected.Add(1)
				} else if region := ie64FormRegion(pcPhys, cpu.memory); region != nil && len(region.blocks) >= 2 {
					newBlock, err := ie64CompileRegion(region, execMem, cpu.memory)
					if err == nil {
						newBlock.execCount = block.execCount
						newBlock.tier = ie64JITTierTurbo
						if cpu.mmuEnabled {
							newBlock.ptbr = cpu.ptbr
						}
						if cpu.mmuEnabled {
							cpu.jitCache.PutMMU(cpu.ptbr, pcVirt, newBlock)
						} else {
							cpu.jitCache.Put(newBlock)
						}
						if newBlock.chainEntry != 0 {
							if cpu.mmuEnabled {
								cpu.jitCache.PatchChainsToScoped(newBlock.startPC, newBlock.chainEntry, cpu.ptbr)
							} else {
								cpu.jitCache.PatchChainsTo(newBlock.startPC, newBlock.chainEntry)
							}
						}
						for i := range newBlock.chainSlots {
							slot := &newBlock.chainSlots[i]
							var target *JITBlock
							if cpu.mmuEnabled {
								target = cpu.jitCache.GetMMU(cpu.ptbr, slot.targetPC)
							} else {
								target = cpu.jitCache.Get(slot.targetPC)
							}
							if target != nil && target.chainEntry != 0 {
								PatchRel32At(slot.patchAddr, target.chainEntry)
							}
						}
						block = newBlock
						globalIE64TurboStats.turboRegions.Add(1)
					} else {
						globalIE64TurboStats.turboRejected.Add(1)
					}
				} else {
					globalIE64TurboStats.turboRejected.Add(1)
				}
			}
		}

		// Reset per-callNative chain dispatch state. Debug break-in checks
		// run in the Go dispatcher, so keep native chaining disabled while a
		// debug adapter is attached; otherwise a patched chain can jump over a
		// script breakpoint such as the AB3D64 _Vid_Present landmark.
		if cpu.debugBreakpointsActive != nil && cpu.debugBreakpointsActive() {
			cpu.jitCtx.ChainBudget = 0
		} else {
			cpu.jitCtx.ChainBudget = ie64ChainBudget
		}
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

		// Phase 4: refresh MMU mode for the native code. Phase 5 will
		// wire the emitted memory/stack helpers to check this field, so
		// the dispatcher must keep it in sync with cpu.mmuEnabled before
		// every callNative — a stale value would let an MMU-on block
		// direct-index virtual addresses as physical memory.
		if cpu.mmuEnabled {
			cpu.jitCtx.MMUEnabled = 1
		} else {
			cpu.jitCtx.MMUEnabled = 0
		}

		// Execute the native code block
		callNative(block.execAddr, uintptr(unsafe.Pointer(cpu.jitCtx)))

		// Phase 2 return channel: read full 64-bit PC from ctx.RetPC and
		// retired count from ctx.RetCount. The emitted native code writes
		// both before returning; the legacy regs[0]-packed channel is
		// still populated as a fallback signal for blocks compiled before
		// the Phase 2 rebuild (none exist after this commit, but keeping
		// the read defensive costs nothing).
		cpu.PC = cpu.jitCtx.RetPC
		cpu.jitCtx.RetPC = 0
		executed := uint64(cpu.jitCtx.RetCount)
		cpu.jitCtx.RetCount = 0
		cpu.regs[0] = 0
		// ChainCount accumulates instruction counts retired by chained
		// predecessor blocks. Add it so retired-instruction accounting is
		// uniform with interpreter mode.
		executed += uint64(cpu.jitCtx.ChainCount)
		cpu.jitCtx.ChainCount = 0

		// Phase 5: helper-exit dispatch. Emitted code that hit an MMU
		// translation, a high physical address, or any other case it
		// could not service locally has written a structured request to
		// JITContext via the HELPER_* protocol. Service it now via the
		// interpreter helpers. While Phase 5 is still in flight no
		// emitter sets NeedHelper, so the no-op fast path returns
		// immediately. Helpers handle their own PC advance and fault
		// propagation; we just account for the retired instruction and
		// suppress the I/O fallback below since the helper already
		// re-executed the bailing op.
		//
		// The dispatch happens BEFORE the legacy zero-count safety
		// fallback. A helper exit on the first instruction of a block
		// legitimately reports RetCount = 0; conflating that with a
		// missing return-channel write would synthesize a fake
		// block.instrCount on top of the helper's 0 or 1 and over-count
		// instructions that never ran.
		helperRetired, helperHandled := cpu.handleJITHelper()
		if helperHandled {
			executed += helperRetired
		} else if executed == 0 {
			// Legacy safety fallback for compiled blocks that did not
			// write the return channel. Suppressed when a helper has
			// already supplied authoritative accounting.
			executed = uint64(block.instrCount)
		}

		// Helpers can halt the CPU directly: missing/invalid FPU
		// (haltFPUFault) and non-trapping stack failures
		// (haltStackFault) call cpu.running.Store(false). The
		// dispatch loop's running check only fires every 4096
		// iterations, which would let the JIT execute more blocks
		// after the interpreter would have stopped. Promote the
		// retired count and exit immediately.
		if helperHandled && !cpu.running.Load() {
			cpu.InstructionCount += executed
			break
		}

		ioBail := cpu.jitCtx.NeedIOFallback != 0
		if helperHandled {
			// Helper already advanced past the offending instruction;
			// avoid double-handling via the I/O fallback path.
			ioBail = false
			cpu.jitCtx.NeedIOFallback = 0
		}
		if ioBail {
			diagIOBails++
			globalIE64TurboStats.ioBails.Add(1)
		}
		diagBlocksExec++
		diagBlockInstrs += uint64(block.instrCount)

		// Self-modifying code: invalidate cache
		if cpu.jitCtx.NeedInval != 0 {
			cpu.jitCache.Invalidate()
			execMem.Reset()
			cpu.jitCtx.NeedInval = 0
			globalIE64TurboStats.invalidations.Add(1)
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
	if statsEnabled {
		ie64TurboStatsLoad().Sub(statsBase).Print()
	}
}
