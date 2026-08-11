//go:build js && wasm

// jit_x86_exec_wasm.go - js/wasm x86 JIT dispatcher.
//
// The wasm backend incrementally lowers direct x86 forms into imported-memory
// WebAssembly modules so the browser path can exercise native chaining and
// region promotion. Unsupported blocks retain the ordinary one-instruction
// interpreter boundary.

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall/js"
	"unsafe"
)

type x86WasmJITBlock struct {
	fn     js.Value
	module js.Value
	slot   int
	meta   *JITBlock
}

type x86WasmJITRuntime struct {
	cpu      *CPU_X86
	ctx      *X86JITContext
	ctxPtr   int
	memObj   js.Value
	table    js.Value
	driver   js.Value
	pcCache  []byte
	nextSlot int
	blocks   map[uint32]*x86WasmJITBlock
}

func x86WasmJITEnabled() bool {
	if os.Getenv("X86_WASM_JIT") == "0" {
		return false
	}
	if !x86WasmCoverageReady {
		return false
	}
	if !wasmSIMDSupported() {
		return false
	}
	return js.Global().Get("__goMem").Truthy()
}

func (cpu *CPU_X86) x86GetWasmRuntime() *x86WasmJITRuntime {
	if cpu.x86JitExecMem == nil {
		return nil
	}
	rt, _ := cpu.x86JitExecMem.(*x86WasmJITRuntime)
	return rt
}

func newX86WasmJITRuntime(cpu *CPU_X86) *x86WasmJITRuntime {
	mem := js.Global().Get("__goMem")
	if !mem.Truthy() || len(cpu.memory) == 0 {
		return nil
	}
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
	ctx := newX86JITContext(cpu, cpu.x86JitCodeBM, cpu.x86JitIOBitmap)
	global := js.Global()
	tblDesc := global.Get("Object").New()
	tblDesc.Set("element", "anyfunc")
	tblDesc.Set("initial", x86WasmDriverTableInitial)
	rt := &x86WasmJITRuntime{
		cpu:     cpu,
		ctx:     ctx,
		ctxPtr:  int(uintptr(unsafe.Pointer(ctx))),
		memObj:  mem,
		table:   global.Get("WebAssembly").Get("Table").New(tblDesc),
		pcCache: make([]byte, x86WasmDriverCacheEntries*8),
		blocks:  map[uint32]*x86WasmJITBlock{},
	}
	rt.instantiateDriver()
	return rt
}

func (rt *x86WasmJITRuntime) instantiateDriver() {
	modBytes := x86WasmBuildDriverModule(uint32(uintptr(unsafe.Pointer(&rt.pcCache[0]))), x86WasmDriverCacheEntries-1)
	global := js.Global()
	u8 := global.Get("Uint8Array").New(len(modBytes))
	js.CopyBytesToJS(u8, modBytes)
	env := global.Get("Object").New()
	env.Set("mem", rt.memObj)
	env.Set("tab", rt.table)
	imports := global.Get("Object").New()
	imports.Set("env", env)
	module := global.Get("WebAssembly").Get("Module").New(u8)
	instance := global.Get("WebAssembly").Get("Instance").New(module, imports)
	rt.driver = instance.Get("exports").Get("drive")
}

func (rt *x86WasmJITRuntime) cacheStore(pc uint32, slot int) {
	idx := pc & (x86WasmDriverCacheEntries - 1)
	e := rt.pcCache[idx*8 : idx*8+8]
	binary.LittleEndian.PutUint32(e[0:], pc)
	binary.LittleEndian.PutUint32(e[4:], uint32(slot+1))
}

func (rt *x86WasmJITRuntime) cacheClear(pc uint32) {
	x86WasmDropDriverCacheEntry(rt.pcCache, x86WasmDriverCacheEntries-1, pc)
}

func (rt *x86WasmJITRuntime) pruneInvalidatedBlocks() int {
	return x86PruneAuxBlockCache(rt.cpu.x86JitCache, rt.blocks, func(block *x86WasmJITBlock) *JITBlock {
		if block == nil {
			return nil
		}
		return block.meta
	}, func(pc uint32, _ *x86WasmJITBlock) {
		rt.cacheClear(pc)
	})
}

func (rt *x86WasmJITRuntime) instantiateBlock(modBytes []byte) (js.Value, js.Value) {
	global := js.Global()
	u8 := global.Get("Uint8Array").New(len(modBytes))
	js.CopyBytesToJS(u8, modBytes)
	env := global.Get("Object").New()
	env.Set("mem", rt.memObj)
	imports := global.Get("Object").New()
	imports.Set("env", env)
	module := global.Get("WebAssembly").Get("Module").New(u8)
	instance := global.Get("WebAssembly").Get("Instance").New(module, imports)
	return instance.Get("exports").Get("block"), instance
}

func (rt *x86WasmJITRuntime) compileBlock(pc uint32, bounded bool) (*x86WasmJITBlock, error) {
	instrs := x86ScanBlock(rt.cpu.memory, pc)
	if len(instrs) == 0 || x86NeedsFallback(instrs) {
		return nil, nil
	}
	if bounded && len(instrs) > 1 {
		instrs = instrs[:1]
	}
	compiled, err := x86WasmCompileBlockModule(instrs, pc, rt.cpu.memory)
	if err != nil {
		return nil, nil
	}
	fn, instance := rt.instantiateBlock(compiled.module)
	slot := rt.nextSlot
	rt.nextSlot++
	if slot >= rt.table.Get("length").Int() {
		rt.table.Call("grow", x86WasmDriverTableInitial)
	}
	rt.table.Call("set", slot, fn)
	rt.cacheStore(pc, slot)
	block := &x86WasmJITBlock{fn: fn, module: instance, slot: slot, meta: compiled.block}
	rt.blocks[pc] = block
	rt.cpu.x86JitCache.Put(compiled.block)
	rt.cpu.jitStats.compiledBlocks.Add(1)
	x86MarkCodePagesForBlock(rt.cpu.x86JitCodeBM, compiled.block)
	return block, nil
}

func (rt *x86WasmJITRuntime) promoteRegion(pc uint32) *x86WasmJITBlock {
	var (
		compiled *x86WasmCompiledModule
		err      error
	)
	if region := x86FormRegion(pc, rt.cpu.x86JitCache, rt.cpu.memory); region != nil && x86TierController.ShouldPromoteRegion(len(region.blocks)) {
		compiled, err = x86WasmCompileRegionModule(region, rt.cpu.memory)
	}
	if compiled == nil {
		if region := x86WasmFormConditionalRegion(pc, rt.cpu.memory); region != nil && x86TierController.ShouldPromoteRegion(3) {
			compiled, err = x86WasmCompileConditionalRegionModule(region, rt.cpu.memory)
		}
	}
	if err != nil || compiled == nil {
		return nil
	}
	fn, instance := rt.instantiateBlock(compiled.module)
	slot := rt.nextSlot
	rt.nextSlot++
	if slot >= rt.table.Get("length").Int() {
		rt.table.Call("grow", x86WasmDriverTableInitial)
	}
	rt.table.Call("set", slot, fn)
	rt.cacheStore(pc, slot)
	block := &x86WasmJITBlock{fn: fn, module: instance, slot: slot, meta: compiled.block}
	rt.blocks[pc] = block
	rt.cpu.x86JitCache.Put(compiled.block)
	rt.cpu.jitStats.compiledRegions.Add(1)
	x86MarkCodePagesForBlock(rt.cpu.x86JitCodeBM, compiled.block)
	return block
}

func (rt *x86WasmJITRuntime) runBlock(block *x86WasmJITBlock) int {
	ctx := rt.ctx
	cpu := rt.cpu
	ctx.RetPC = cpu.EIP
	ctx.RetCount = 0
	ctx.ChainCount = 0
	ctx.ChainCycles = 0
	ctx.ChainTicks = 0
	if cpu.x86BudgetActive {
		ctx.ChainBudget = 1
	} else {
		ctx.ChainBudget = x86WasmChainBudget
	}
	ctx.NeedIOFallback = 0
	ctx.NeedInval = 0
	ctx.ExitReason = x86JITExitNone
	rt.cacheStore(cpu.EIP, block.slot)
	rt.driver.Invoke(rt.ctxPtr)
	cpu.jitStats.nativeEntries.Add(1)
	cpu.EIP = ctx.RetPC
	completed := int(ctx.RetCount + ctx.ChainCount)
	cpu.jitStats.nativeRetired.Add(uint64(completed))
	if ctx.ChainCount != 0 {
		cpu.jitStats.chainExits.Add(1)
	}
	if completed > len(block.meta.x86CyclePrefix) {
		completed = len(block.meta.x86CyclePrefix)
	}
	if ctx.ChainTicks != 0 {
		cpu.Cycles += uint64(ctx.ChainCycles)
		cpu.bus.Tick(int(ctx.ChainTicks))
	} else if completed > 0 {
		cpu.Cycles += block.meta.x86CyclePrefix[completed-1]
		if len(block.meta.x86TickPrefix) >= completed {
			cpu.bus.Tick(int(block.meta.x86TickPrefix[completed-1]))
		}
	}
	return completed
}

func (cpu *CPU_X86) initX86JIT() error {
	if cpu.x86GetWasmRuntime() != nil {
		return nil
	}
	rt := newX86WasmJITRuntime(cpu)
	if rt == nil {
		return fmt.Errorf("x86 wasm JIT unavailable")
	}
	cpu.x86JitExecMem = rt
	cpu.x86JitCtx = rt.ctx
	cpu.x86JitCache = NewCodeCache()
	return nil
}

func (cpu *CPU_X86) freeX86JIT() {
	if cpu.x86JitPersist {
		return
	}
	cpu.x86JitExecMem = nil
	cpu.x86JitCache = nil
	cpu.x86JitCtx = nil
	cpu.x86JitCodeBM = nil
}

func (cpu *CPU_X86) X86ExecuteJIT() {
	if err := cpu.initX86JIT(); err != nil {
		cpu.x86RunInterpreter()
		return
	}
	defer cpu.freeX86JIT()
	rt := cpu.x86GetWasmRuntime()
	ctx := rt.ctx
	cpu.syncJITRegsFromNamed()
	cpu.syncJITSegRegsFromNamed()
	bounded := cpu.x86BudgetActive
	yieldCheck := uint32(0)
	for cpu.Running() && !cpu.Halted {
		yieldCheck++
		if yieldCheck&0xFFF == 0 {
			hostCooperativeYield()
		}
		if cpu.debugHandleBreakInJIT(uint64(cpu.EIP)) {
			break
		}
		if bounded && cpu.x86InstrBudget <= 0 {
			break
		}
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
		if !bounded && cpu.tryFastMMIOPollLoopJIT() {
			continue
		}
		pc := cpu.EIP
		if pc >= uint32(len(cpu.memory)) {
			cpu.Halted = true
			break
		}
		block := rt.blocks[pc]
		if block == nil {
			cpu.jitStats.cacheMisses.Add(1)
			var err error
			block, err = rt.compileBlock(pc, bounded)
			if err != nil {
				block = nil
			}
		} else {
			cpu.jitStats.cacheHits.Add(1)
			if x86RegionPromotionEnabled && !bounded {
				block.meta.execCount++
				if x86TierController.ShouldPromote(block.meta.tier, block.meta.execCount, block.meta.ioBails, block.meta.lastPromoteAt) {
					cpu.jitStats.regionCandidates.Add(1)
					block.meta.lastPromoteAt = block.meta.execCount
					if promoted := rt.promoteRegion(pc); promoted != nil {
						promoted.meta.execCount = block.meta.execCount
						block = promoted
					}
				}
			}
		}
		if block == nil || (bounded && int64(block.meta.instrCount) > cpu.x86InstrBudget) {
			cpu.syncJITRegsToNamed()
			cpu.syncJITSegRegsToNamed()
			cpu.x86RenormalizeFPUBoundary()
			cpu.jitStats.instructionCount.Add(1)
			cpu.jitStats.fallbackInstructions.Add(1)
			cpu.Step()
			cpu.syncJITRegsFromNamed()
			cpu.syncJITSegRegsFromNamed()
			if bounded {
				cpu.x86InstrBudget--
			}
			continue
		}
		completed := rt.runBlock(block)
		cpu.jitStats.instructionCount.Add(uint64(completed))
		if bounded {
			cpu.x86InstrBudget -= int64(completed)
		}
		if ctx.NeedIOFallback != 0 || ctx.ExitReason != x86JITExitNone {
			cpu.syncJITRegsToNamed()
			cpu.syncJITSegRegsToNamed()
			cpu.x86RenormalizeFPUBoundary()
			if ctx.ExitReason == x86JITExitFPUHelper {
				cpu.jitStats.helperExits.Add(1)
				if payload, ok := x86FPUHelperPayloadFromContext(ctx); ok {
					cpu.x86RunFPUHelper(payload)
				} else {
					cpu.Step()
				}
			} else {
				cpu.jitStats.ioBails.Add(1)
				cpu.Step()
			}
			cpu.syncJITRegsFromNamed()
			cpu.syncJITSegRegsFromNamed()
			cpu.jitStats.instructionCount.Add(1)
			cpu.jitStats.fallbackInstructions.Add(1)
			if bounded {
				cpu.x86InstrBudget--
			}
		}
		if ctx.NeedInval != 0 {
			cpu.jitStats.invalidations.Add(1)
			if jitSMCRangeDisabled {
				cpu.jitStats.invalidatedBlocks.Add(uint64(len(rt.blocks)))
				cpu.x86JitCache.Invalidate()
				rt.blocks = map[uint32]*x86WasmJITBlock{}
				clear(cpu.x86JitCodeBM)
				x86WasmResetDriverCache(rt.pcCache, &rt.nextSlot)
				x86ClearRTSCache(ctx)
				cpu.jitStats.codeCacheResets.Add(1)
			} else {
				removed := x86InvalidateSMCRange(cpu.x86JitCache, cpu.x86JitCodeBM, ctx)
				cpu.jitStats.invalidatedBlocks.Add(uint64(removed))
				rt.pruneInvalidatedBlocks()
				if len(rt.blocks) == 0 {
					x86WasmResetDriverCache(rt.pcCache, &rt.nextSlot)
					cpu.jitStats.codeCacheResets.Add(1)
				}
			}
			ctx.NeedInval = 0
			ctx.InvalAddr = 0
			ctx.InvalSize = 0
		}
	}
	cpu.syncJITRegsToNamed()
	cpu.syncJITSegRegsToNamed()
	cpu.x86RenormalizeFPUBoundary()
}

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
	if cpu.FPU != nil {
		cpu.FPU.RenormalizeTags()
	}
}
