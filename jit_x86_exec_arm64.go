// jit_x86_exec_arm64.go - Linux/arm64 x86 JIT dispatcher.
//
// It shares the frontend scanner and cache contract with amd64. A native block
// is cached only after the ARM64 emitter has accepted a complete direct prefix;
// all other instructions use the existing one-step interpreter boundary.

//go:build arm64 && linux

package main

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

const x86JitExecMemSize = 16 * 1024 * 1024

// x86ARM64PatchBranchAt patches one ARM64 B instruction at patchAddr. Unlike
// PatchRel32At, whose address names an amd64 displacement field, patchAddr is
// the address of the complete ARM64 instruction. The writable alias must be
// cleaned and the executable alias invalidated before a native chain can use
// the new target.
func x86ARM64PatchBranchAt(patchAddr, targetAddr uintptr) bool {
	if patchAddr&3 != 0 || targetAddr&3 != 0 {
		return false
	}
	delta := int64(targetAddr) - int64(patchAddr)
	if delta < -(1<<27) || delta >= 1<<27 {
		return false // imm26 signed words: +/- 128 MiB
	}
	p, writableAddr, ok := lookupWritableBytes(patchAddr, 4)
	if !ok {
		return false
	}
	binary.LittleEndian.PutUint32(p, arm64B(int32(delta)))
	flushICacheDual(writableAddr, patchAddr, 4)
	return true
}

func (cpu *CPU_X86) x86GetJITExecMem() *ExecMem {
	if cpu.x86JitExecMem == nil {
		return nil
	}
	return cpu.x86JitExecMem.(*ExecMem)
}

func (cpu *CPU_X86) initX86JIT() error {
	if cpu.x86JitExecMem != nil {
		return nil
	}
	if len(cpu.memory) == 0 {
		return fmt.Errorf("x86 JIT: cpu.memory not initialized (need X86BusAdapter)")
	}
	em, err := AllocExecMem(x86JitExecMemSize)
	if err != nil {
		return fmt.Errorf("x86 JIT init failed: %w", err)
	}
	cpu.x86JitExecMem = em
	cpu.x86JitCache = NewCodeCache()
	// CPUX86Runner supplies a bitmap built from the live MachineBus mappings
	// and adapter bank windows. Retain it verbatim: replacing it with only the
	// legacy hard-coded ranges lets native accesses bypass configured devices.
	// Match amd64's conservative fallback when a CPU was not created by a
	// runner (for example, a focused unit test).
	if cpu.x86JitIOBitmap == nil {
		pages := (len(cpu.memory) + 255) >> 8
		cpu.x86JitIOBitmap = make([]byte, pages)
		for addr := uint32(0xF000); addr < 0x10000; addr += 0x100 {
			if page := addr >> 8; page < uint32(len(cpu.x86JitIOBitmap)) {
				cpu.x86JitIOBitmap[page] = 1
			}
		}
		for addr := uint32(0xA0000); addr < 0xB0000; addr += 0x100 {
			if page := addr >> 8; page < uint32(len(cpu.x86JitIOBitmap)) {
				cpu.x86JitIOBitmap[page] = 1
			}
		}
	}
	cpu.x86JitCodeBM = make([]byte, len(cpu.x86JitIOBitmap))
	cpu.x86JitCtx = newX86JITContext(cpu, cpu.x86JitCodeBM, cpu.x86JitIOBitmap)
	return nil
}

func (cpu *CPU_X86) freeX86JIT() {
	if cpu.x86JitPersist {
		return
	}
	if em := cpu.x86GetJITExecMem(); em != nil {
		em.Free()
	}
	cpu.x86JitExecMem, cpu.x86JitCache, cpu.x86JitCtx = nil, nil, nil
	cpu.x86JitCodeBM = nil
}

// x86ARM64PatchBlockChains wires both directions when a block becomes live.
// The cache owns every inbound slot, while the explicit outbound pass handles
// targets compiled before their sources.  x86ARM64PatchBranchAt flushes the
// instruction cache for each rewritten B instruction.
func x86ARM64PatchBlockChains(cache *CodeCache, block *JITBlock) {
	if !x86BlockChainingEnabled || cache == nil || block == nil || block.chainEntry == 0 {
		return
	}
	cache.PatchChainsTo(block.startPC, block.chainEntry)
	for _, slot := range block.chainSlots {
		if target := cache.Get(slot.targetPC); target != nil && target.chainEntry != 0 {
			patchChainSlot(slot, target.chainEntry)
		}
	}
}

func (cpu *CPU_X86) X86ExecuteJIT() {
	if err := cpu.initX86JIT(); err != nil {
		panic(err)
	}
	defer cpu.freeX86JIT()
	cpu.syncJITRegsFromNamed()
	cpu.syncJITSegRegsFromNamed()
	ctx, em := cpu.x86JitCtx, cpu.x86GetJITExecMem()
	bounded := cpu.x86BudgetActive
	for cpu.Running() && !cpu.Halted {
		// jitRegs is canonical while native code runs, so the debugger must
		// observe a synchronised architectural snapshot at each dispatch
		// boundary, just as it does on amd64.
		if cpu.debugHandleBreakInJIT(uint64(cpu.EIP)) {
			cpu.deoptStats.Add(DeoptDebug)
			break
		}
		if bounded && cpu.x86InstrBudget <= 0 {
			break
		}
		// Native blocks bypass Step, which is normally responsible for
		// accepting pending interrupts. Synchronise before changing the
		// named interrupt state, then restore the native register view.
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
		pc := cpu.EIP
		if pc >= uint32(len(cpu.memory)) {
			cpu.Halted = true
			break
		}
		block := cpu.x86JitCache.Get(uint64(pc))
		if block == nil {
			cpu.jitStats.cacheMisses.Add(1)
			instrs := x86ScanBlock(cpu.memory, pc)
			// Bounded execution is used by deterministic shadow-parity checks.
			// A fresh compilation must not retire across its exact checkpoint.
			if bounded && len(instrs) > 1 {
				instrs = instrs[:1]
			}
			var err error
			if len(instrs) != 0 && !x86ARM64NeedsFallback(instrs) {
				block, err = x86CompileBlockForCPU(cpu, instrs, pc, em)
			}
			if err != nil || block == nil {
				cpu.syncJITRegsToNamed()
				cpu.syncJITSegRegsToNamed()
				cpu.x86RenormalizeFPUBoundary()
				helper := false
				if len(instrs) != 0 {
					if payload, ok := x86FPUHelperPayloadFor(instrs[0], cpu.memory, cpu.CS); ok {
						cpu.x86RunFPUHelper(payload)
						helper = true
					} else {
						cpu.Step()
					}
				} else {
					cpu.Step()
				}
				cpu.syncJITRegsFromNamed()
				cpu.syncJITSegRegsFromNamed()
				cpu.jitStats.instructionCount.Add(1)
				cpu.jitStats.fallbackInstructions.Add(1)
				if helper {
					cpu.jitStats.helperExits.Add(1)
				}
				if bounded {
					cpu.x86InstrBudget--
				}
				continue
			}
			cpu.x86JitCache.Put(block)
			cpu.jitStats.compiledBlocks.Add(1)
			x86MarkCodePagesForBlock(cpu.x86JitCodeBM, block)
			x86ARM64PatchBlockChains(cpu.x86JitCache, block)
		} else {
			cpu.jitStats.cacheHits.Add(1)
			if x86RegionPromotionEnabled && !bounded {
				// Region promotion is deliberately outside bounded shadow windows:
				// those callers require one-instruction compilation and exact
				// dispatcher observation.  The region compiler itself declines
				// back-edges, preserving dynamic loop accounting at block exits.
				block.execCount++
				if x86TierController.ShouldPromote(block.tier, block.execCount, block.ioBails, block.lastPromoteAt) {
					cpu.jitStats.regionCandidates.Add(1)
					block.lastPromoteAt = block.execCount
					region := x86FormRegion(pc, cpu.x86JitCache, cpu.memory)
					if region != nil && x86TierController.ShouldPromoteRegion(len(region.blocks)) {
						if promoted, err := x86CompileRegionForCPU(cpu, region, em); err == nil {
							promoted.execCount = block.execCount
							cpu.x86JitCache.Put(promoted)
							x86MarkCodePagesForBlock(cpu.x86JitCodeBM, promoted)
							x86ARM64PatchBlockChains(cpu.x86JitCache, promoted)
							block = promoted
							cpu.jitStats.compiledRegions.Add(1)
						}
					}
				}
			}
		}
		// A block cached during an unbounded run can be wider than the
		// remaining deterministic window. Replay one instruction rather than
		// retiring past the requested boundary.
		if bounded && int64(block.instrCount) > cpu.x86InstrBudget {
			cpu.syncJITRegsToNamed()
			cpu.syncJITSegRegsToNamed()
			cpu.x86RenormalizeFPUBoundary()
			cpu.Step()
			cpu.syncJITRegsFromNamed()
			cpu.syncJITSegRegsFromNamed()
			cpu.x86InstrBudget--
			cpu.jitStats.instructionCount.Add(1)
			cpu.jitStats.fallbackInstructions.Add(1)
			continue
		}
		ctx.RetCount = 0
		ctx.NeedIOFallback = 0
		ctx.NeedInval = 0
		ctx.ExitReason = x86JITExitNone
		// Initialise the native-chain ABI on every Go entry. ARM64 chain exits
		// consume this budget and fold instruction, cycle and device-tick
		// accounting into their eventual cold return.
		ctx.ChainCount = 0
		ctx.ChainCycles = 0
		ctx.ChainTicks = 0
		if bounded {
			ctx.ChainBudget = 1
		} else {
			ctx.ChainBudget = 65536
		}
		preESI, preEDI, preECX := cpu.jitRegs[6], cpu.jitRegs[7], cpu.jitRegs[1]
		cpu.jitStats.nativeEntries.Add(1)
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
		cpu.EIP = ctx.RetPC
		completed := int(ctx.RetCount)
		// A cold guard or canonical FPU helper can exit before the first native
		// instruction retires.  Its interpreter replay below owns that one
		// instruction's timing; substituting block.instrCount here would charge
		// it once as native work and once again through Step.
		if completed == 0 && ctx.NeedIOFallback == 0 {
			completed = block.instrCount
		}
		if completed > 0 && block.x86HasNativeFPU {
			cpu.x86FPUEnvLoaded = false
		}
		if ctx.ChainTicks == 0 && completed > len(block.x86CyclePrefix) {
			completed = len(block.x86CyclePrefix)
		}
		if ctx.ChainTicks != 0 {
			cpu.Cycles += uint64(ctx.ChainCycles)
			cpu.bus.Tick(int(ctx.ChainTicks))
		} else if completed > 0 {
			cpu.Cycles += block.x86CyclePrefix[completed-1]
			if len(block.x86TickPrefix) >= completed {
				cpu.bus.Tick(int(block.x86TickPrefix[completed-1]))
			}
		}
		// String handlers charge a cycle for every completed iteration. Native
		// string forms expose completion through their architectural pointer
		// delta; this mirrors the amd64 dispatcher without widening the context
		// ABI. REP forms will use the same path when admitted.
		for _, form := range block.x86DynamicCycles {
			iterations := uint64(1)
			if form.rep {
				iterations = uint64(preECX)
			}
			if form.width != 0 {
				before, after := preEDI, cpu.jitRegs[7]
				if form.source {
					before, after = preESI, cpu.jitRegs[6]
				}
				delta := after - before
				if after < before {
					delta = before - after
				}
				iterations = uint64(delta / form.width)
			}
			cpu.Cycles += iterations
			if iterations > 1 {
				cpu.bus.Tick(int(iterations - 1))
			}
		}
		if ctx.NeedInval != 0 {
			cpu.jitStats.invalidations.Add(1)
			// The native store is complete. Remove every cached block overlapping
			// its exact guest range before the next dispatch can observe it.
			if jitSMCRangeDisabled {
				cpu.jitStats.invalidatedBlocks.Add(uint64(cpu.x86JitCache.Len()))
				cpu.x86JitCache.Invalidate()
				em.Reset()
				clear(cpu.x86JitCodeBM)
				x86ClearRTSCache(ctx)
				cpu.jitStats.codeCacheResets.Add(1)
			} else if removed := x86InvalidateSMCRange(cpu.x86JitCache, cpu.x86JitCodeBM, ctx); removed != 0 {
				cpu.jitStats.invalidatedBlocks.Add(uint64(removed))
				resetExecMemWhenCacheEmpty(cpu.x86JitCache, em)
				if cpu.x86JitCache.Len() == 0 {
					cpu.jitStats.codeCacheResets.Add(1)
				}
			}
			ctx.NeedInval, ctx.InvalAddr, ctx.InvalSize = 0, 0, 0
		}
		fallbackRetired := 0
		if ctx.NeedIOFallback != 0 {
			cpu.syncJITRegsToNamed()
			cpu.syncJITSegRegsToNamed()
			if ctx.ExitReason == x86JITExitFPUHelper {
				cpu.jitStats.helperExits.Add(1)
				payload, ok := x86FPUHelperPayloadFromContext(ctx)
				if !ok {
					panic("x86 ARM64 JIT: native FPU helper exit without decoded payload")
				}
				cpu.x86RenormalizeFPUBoundary()
				cpu.x86RunFPUHelper(payload)
			} else {
				cpu.jitStats.ioBails.Add(1)
				// A guarded ordinary memory form has not touched guest state.
				// Replay it through the bus, including MMIO and boundary faults.
				cpu.Step()
			}
			cpu.syncJITRegsFromNamed()
			cpu.syncJITSegRegsFromNamed()
			fallbackRetired = 1
			cpu.jitStats.fallbackInstructions.Add(1)
			ctx.NeedIOFallback = 0
			ctx.ExitReason = x86JITExitNone
		}
		cpu.jitStats.nativeRetired.Add(uint64(completed))
		cpu.jitStats.instructionCount.Add(uint64(completed + fallbackRetired))
		if ctx.ChainCount != 0 {
			cpu.jitStats.chainExits.Add(1)
		}
		if bounded {
			cpu.x86InstrBudget -= int64(completed + fallbackRetired)
		}
	}
	cpu.x86RenormalizeFPUBoundary()
	cpu.syncJITRegsToNamed()
	cpu.syncJITSegRegsToNamed()
}

// x86ARM64NeedsFallback keeps the ARM64 emitter away from x87 memory forms.
// These operations have variable-width environment/BCD operands and their
// effective address is part of the interpreter-visible bus contract.  Keep
// this check local to the dispatcher as a defence in depth: the shared
// x86NeedsFallback predicate is also used by other backends and a decoder
// change must not accidentally make ARM64 execute one of these forms natively.
func x86ARM64NeedsFallback(instrs []X86JITInstr) bool {
	if x86NeedsFallback(instrs) {
		return true
	}
	if len(instrs) == 0 {
		return true
	}
	first := instrs[0]
	return first.opcode >= 0xD8 && first.opcode <= 0xDF && first.hasModRM && first.modrm>>6 != 3
}

// x86RunInterpreter is shared by the ARM64 dispatcher and callers that turn
// JIT execution off. It intentionally uses the interpreter's normal FPU
// boundary normalisation before every instruction.
func (cpu *CPU_X86) x86RunInterpreter() {
	bounded := cpu.x86BudgetActive
	yieldCheck := uint32(0)
	for cpu.Running() && !cpu.Halted {
		if bounded && cpu.x86InstrBudget <= 0 {
			return
		}
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

func (cpu *CPU_X86) x86RenormalizeFPUBoundary() {
	if cpu.FPU != nil && !cpu.x86FPUEnvLoaded {
		cpu.FPU.RenormalizeTags()
	}
}
