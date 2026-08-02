// jit_x86_exec_arm64.go - Linux/arm64 x86 JIT dispatcher.
//
// It shares the frontend scanner and cache contract with amd64. A native block
// is cached only after the ARM64 emitter has accepted a complete direct prefix;
// all other instructions use the existing one-step interpreter boundary.

//go:build arm64 && linux

package main

import (
	"fmt"
	"unsafe"
)

const x86JitExecMemSize = 16 * 1024 * 1024

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
			instrs := x86ScanBlock(cpu.memory, pc)
			// Bounded execution is used by deterministic shadow-parity checks.
			// A fresh compilation must not retire across its exact checkpoint.
			if bounded && len(instrs) > 1 {
				instrs = instrs[:1]
			}
			var err error
			if len(instrs) != 0 && !x86NeedsFallback(instrs) {
				block, err = x86CompileBlockForCPU(cpu, instrs, pc, em)
			}
			if err != nil || block == nil {
				cpu.syncJITRegsToNamed()
				cpu.syncJITSegRegsToNamed()
				cpu.x86RenormalizeFPUBoundary()
				if len(instrs) != 0 {
					if payload, ok := x86FPUHelperPayloadFor(instrs[0], cpu.memory, cpu.CS); ok {
						cpu.x86RunFPUHelper(payload)
					} else {
						cpu.Step()
					}
				} else {
					cpu.Step()
				}
				cpu.syncJITRegsFromNamed()
				cpu.syncJITSegRegsFromNamed()
				if bounded {
					cpu.x86InstrBudget--
				}
				continue
			}
			cpu.x86JitCache.Put(block)
			x86MarkCodePagesForBlock(cpu.x86JitCodeBM, block)
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
			continue
		}
		ctx.RetCount = 0
		ctx.NeedIOFallback = 0
		ctx.NeedInval = 0
		ctx.ExitReason = x86JITExitNone
		preESI, preEDI, preECX := cpu.jitRegs[6], cpu.jitRegs[7], cpu.jitRegs[1]
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
		cpu.EIP = ctx.RetPC
		completed := int(ctx.RetCount)
		if completed == 0 {
			completed = block.instrCount
		}
		if completed > len(block.x86CyclePrefix) {
			completed = len(block.x86CyclePrefix)
		}
		if completed > 0 {
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
			// The native store is complete. Remove every cached block overlapping
			// its exact guest range before the next dispatch can observe it.
			if jitSMCRangeDisabled {
				cpu.x86JitCache.Invalidate()
				em.Reset()
				clear(cpu.x86JitCodeBM)
				x86ClearRTSCache(ctx)
			} else if removed := x86InvalidateSMCRange(cpu.x86JitCache, cpu.x86JitCodeBM, ctx); removed != 0 {
				resetExecMemWhenCacheEmpty(cpu.x86JitCache, em)
			}
			ctx.NeedInval, ctx.InvalAddr, ctx.InvalSize = 0, 0, 0
		}
		fallbackRetired := 0
		if ctx.NeedIOFallback != 0 {
			cpu.syncJITRegsToNamed()
			cpu.syncJITSegRegsToNamed()
			if ctx.ExitReason == x86JITExitFPUHelper {
				payload, ok := x86FPUHelperPayloadFromContext(ctx)
				if !ok {
					panic("x86 ARM64 JIT: native FPU helper exit without decoded payload")
				}
				cpu.x86RenormalizeFPUBoundary()
				cpu.x86RunFPUHelper(payload)
			} else {
				// A guarded ordinary memory form has not touched guest state.
				// Replay it through the bus, including MMIO and boundary faults.
				cpu.Step()
			}
			cpu.syncJITRegsFromNamed()
			cpu.syncJITSegRegsFromNamed()
			fallbackRetired = 1
			ctx.NeedIOFallback = 0
			ctx.ExitReason = x86JITExitNone
		}
		if bounded {
			cpu.x86InstrBudget -= int64(completed + fallbackRetired)
		}
	}
	cpu.x86RenormalizeFPUBoundary()
	cpu.syncJITRegsToNamed()
	cpu.syncJITSegRegsToNamed()
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
		if cpu.tryFastMMIOPollLoop() {
			continue
		}
		cpu.x86RenormalizeFPUBoundary()
		cpu.Step()
		if bounded {
			cpu.x86InstrBudget--
			if cpu.x86InstrBudget <= 0 {
				return
			}
		}
	}
}

func (cpu *CPU_X86) x86RenormalizeFPUBoundary() {
	if cpu.FPU != nil {
		cpu.FPU.RenormalizeTags()
	}
}
