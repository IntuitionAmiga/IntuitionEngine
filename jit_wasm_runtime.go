//go:build js && wasm

// jit_wasm_runtime.go - runtime half of the IE64 wasm JIT backend.
//
// Owns the compiled-block cache, the async compile queue and the helper-exit
// dispatch for blocks produced by wasmCompileBlock. Blocks are wasm modules
// compiled by the browser's engine via WebAssembly.instantiate; they import
// the Go program's own linear memory (exposed by the hosting page as
// globalThis.__goMem) and mutate CPU state in place through the JITContext,
// exactly like the native backends.
//
// MMU gate: while cpu.mmuEnabled is true this runtime neither enters
// installed blocks nor enqueues compiles. Both halves of the gate live here,
// in tryBlock and noteHot, so they hold for any caller.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"math"
	"os"
	"syscall/js"
	"unsafe"
)

// wasmJITHotThreshold is how many dispatcher visits a block-start PC needs
// before a compile is enqueued. Compiles are asynchronous, so the threshold
// only filters cold code; the interpreter keeps running regardless.
const wasmJITHotThreshold = 8

type wasmJITBlock struct {
	fn     js.Value // exported "block" function
	endPC  uint64   // first byte after the compiled range
	module js.Value // keeps the instance alive explicitly
	execs  uint64   // per-block execution counter (tests, stats)
}

type wasmJITRuntime struct {
	cpu    *CPU64
	ctx    *JITContext
	ctxPtr int // linear-memory address of ctx, passed to every block

	memObj js.Value // the Go module's WebAssembly.Memory (env.mem import)

	blocks    map[uint64]*wasmJITBlock
	hot       map[uint64]uint32
	blacklist map[uint64]bool // PCs whose blocks the translator rejected
	inFlight  map[uint64]bool // compiles submitted, not yet installed

	compiles  uint64 // completed installs (tests, stats)
	blockRuns uint64 // dispatcher entries into compiled code (tests, stats)
	chainRuns uint64 // entries that went through the in-wasm chain driver

	// In-wasm chaining: a shared funcref table holds every installed block,
	// and pcCache is a direct-mapped {pc, slot+1} table in linear memory the
	// driver module reads. driver is the exported drive() function once its
	// module instantiates.
	table    js.Value
	driver   js.Value
	pcCache  []byte
	nextSlot int

	// gen is the invalidation generation. Compiles capture it at submission
	// and their async install callbacks compare it on resolution: a compile
	// submitted before an SMC invalidation must not install afterwards, or
	// stale guest code would run.
	gen uint64
}

// wasmConsoleLog writes through the JS console. Safe inside js.FuncOf
// callbacks, unlike fmt.Printf, whose file write is an asynchronous syscall
// under node and deadlocks when the event loop is blocked by the very
// handler doing the printing.
func wasmConsoleLog(msg string) {
	js.Global().Get("console").Call("log", msg)
}

// wasmJITEnabled reports whether the backend should run: default on, killed
// by IE64_WASM_JIT=0 (the demo page maps ?jit=0 onto it), and requires the
// hosting page to have exposed the module memory.
func wasmJITEnabled() bool {
	if os.Getenv("IE64_WASM_JIT") == "0" {
		return false
	}
	return js.Global().Get("__goMem").Truthy()
}

func newWasmJITRuntime(cpu *CPU64) *wasmJITRuntime {
	mem := js.Global().Get("__goMem")
	if !mem.Truthy() {
		return nil
	}
	ctx := newJITContext(cpu)
	if len(cpu.jitCodePageBitmap) == 0 {
		cpu.jitCodePageBitmap = make([]byte, len(cpu.memory)/256)
		ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&cpu.jitCodePageBitmap[0]))
		ctx.CodePageBitmapLen = uint32(len(cpu.jitCodePageBitmap))
	}
	global := js.Global()
	tblDesc := global.Get("Object").New()
	tblDesc.Set("element", "anyfunc")
	tblDesc.Set("initial", wasmJITTableInitial)
	rt := &wasmJITRuntime{
		cpu:       cpu,
		ctx:       ctx,
		ctxPtr:    int(uintptr(unsafe.Pointer(ctx))),
		memObj:    mem,
		blocks:    map[uint64]*wasmJITBlock{},
		hot:       map[uint64]uint32{},
		blacklist: map[uint64]bool{},
		inFlight:  map[uint64]bool{},
		table:     global.Get("WebAssembly").Get("Table").New(tblDesc),
		pcCache:   make([]byte, wasmJITCacheEntries*16),
	}
	rt.instantiateDriver()
	return rt
}

const (
	wasmJITCacheEntries = 4096 // power of two; 16 bytes per entry
	wasmJITTableInitial = 256
)

// instantiateDriver compiles the chain driver asynchronously. Until it
// resolves, tryBlock falls back to one Invoke per block.
func (rt *wasmJITRuntime) instantiateDriver() {
	base := uint32(uintptr(unsafe.Pointer(&rt.pcCache[0])))
	modBytes := wasmBuildDriverModule(base, wasmJITCacheEntries-1)
	global := js.Global()
	u8 := global.Get("Uint8Array").New(len(modBytes))
	js.CopyBytesToJS(u8, modBytes)
	env := global.Get("Object").New()
	env.Set("mem", rt.memObj)
	env.Set("tab", rt.table)
	imports := global.Get("Object").New()
	imports.Set("env", env)
	var onOK, onErr js.Func
	onOK = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer onOK.Release()
		defer onErr.Release()
		rt.driver = args[0].Get("instance").Get("exports").Get("drive")
		return nil
	})
	onErr = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer onOK.Release()
		defer onErr.Release()
		wasmConsoleLog("IE64 wasm JIT: driver compile failed: " + args[0].Call("toString").String())
		return nil
	})
	global.Get("WebAssembly").Call("instantiate", u8, imports).Call("then", onOK, onErr)
}

// cacheStore publishes a pc -> table slot mapping for the driver.
func (rt *wasmJITRuntime) cacheStore(pc uint64, slot int) {
	idx := (pc >> 3) & (wasmJITCacheEntries - 1)
	e := rt.pcCache[idx*16 : idx*16+16]
	*(*uint64)(unsafe.Pointer(&e[0])) = pc
	*(*uint32)(unsafe.Pointer(&e[8])) = uint32(slot + 1)
}

// peek returns the installed block covering pc, or nil when the dispatcher
// must interpret. Entry half of the MMU gate lives here so every caller is
// gated.
func (rt *wasmJITRuntime) peek(pc uint64) *wasmJITBlock {
	if rt.cpu.mmuEnabled {
		return nil
	}
	return rt.blocks[pc]
}

// tryBlock runs the installed block covering pc, if any. Returns false when
// the dispatcher must interpret instead.
func (rt *wasmJITRuntime) tryBlock(pc uint64) bool {
	blk := rt.peek(pc)
	if blk == nil {
		return false
	}
	rt.runBlock(blk)
	return true
}

// noteHot counts a dispatcher visit and enqueues a compile at the threshold.
// Enqueue half of the MMU gate.
func (rt *wasmJITRuntime) noteHot(pc uint64) {
	if rt.cpu.mmuEnabled {
		return
	}
	if rt.blacklist[pc] || rt.inFlight[pc] {
		return
	}
	if _, ok := rt.blocks[pc]; ok {
		return
	}
	rt.hot[pc]++
	if rt.hot[pc] >= wasmJITHotThreshold {
		delete(rt.hot, pc)
		rt.enqueueCompile(pc)
	}
}

// enqueueCompile scans, translates and submits a block for async compilation.
// The install callback runs when the promise resolves, i.e. during a
// cooperative yield; the interpreter keeps executing until then.
func (rt *wasmJITRuntime) enqueueCompile(pc uint64) {
	cpu := rt.cpu
	if pc >= uint64(len(cpu.memory)) {
		rt.blacklist[pc] = true
		return
	}
	instrs := scanBlock(cpu.memory, pc)
	// Compile the longest supported prefix; the instruction after it is
	// interpreted when the block falls off its end.
	cut := len(instrs)
	for i := range instrs {
		if !wasmSupportedOpcode(instrs[i].opcode) || instrs[i].fusedFlag != 0 || instrs[i].mmuBail {
			cut = i
			break
		}
		if isIE64FPUOpcode(instrs[i].opcode) && cpu.FPU == nil {
			// FPU instructions on an FPU-less CPU halt architecturally;
			// leave them to the interpreter.
			cut = i
			break
		}
	}
	instrs = instrs[:cut]
	if len(instrs) == 0 {
		rt.blacklist[pc] = true
		return
	}
	modBytes, err := wasmCompileBlock(instrs, pc)
	if err != nil {
		rt.blacklist[pc] = true
		return
	}
	endPC := pc + uint64(len(instrs))*8

	global := js.Global()
	u8 := global.Get("Uint8Array").New(len(modBytes))
	js.CopyBytesToJS(u8, modBytes)
	env := global.Get("Object").New()
	env.Set("mem", rt.memObj)
	imports := global.Get("Object").New()
	imports.Set("env", env)

	rt.inFlight[pc] = true
	myGen := rt.gen
	var onOK, onErr js.Func
	release := func() {
		onOK.Release()
		onErr.Release()
	}
	onOK = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer release()
		delete(rt.inFlight, pc)
		if rt.gen != myGen {
			// Invalidated while compiling: the module was built from
			// instruction bytes that may have been overwritten. Drop it; the
			// PC re-tiers from scratch if it stays hot.
			return nil
		}
		instance := args[0].Get("instance")
		fn := instance.Get("exports").Get("block")
		rt.blocks[pc] = &wasmJITBlock{
			fn:     fn,
			endPC:  endPC,
			module: instance,
		}
		// Publish to the chain driver: table slot + pc cache entry.
		slot := rt.nextSlot
		rt.nextSlot++
		if lenNow := rt.table.Get("length").Int(); slot >= lenNow {
			rt.table.Call("grow", wasmJITTableInitial)
		}
		rt.table.Call("set", slot, fn)
		rt.cacheStore(pc, slot)
		rt.markCodePages(pc, endPC)
		rt.compiles++
		if rt.compiles == 1 {
			// One-shot diagnostic; the browser e2e harness greps for it.
			// console.log, NOT fmt.Printf: Go print syscalls are
			// asynchronous under node and block the event-loop handler
			// this callback runs on, deadlocking the whole runtime.
			wasmConsoleLog(fmt.Sprintf("IE64 wasm JIT: first block installed at %#x", pc))
		}
		return nil
	})
	onErr = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer release()
		delete(rt.inFlight, pc)
		rt.blacklist[pc] = true
		wasmConsoleLog(fmt.Sprintf("IE64 wasm JIT: compile failed at %#x: %s", pc, args[0].Call("toString").String()))
		return nil
	})
	global.Get("WebAssembly").Call("instantiate", u8, imports).Call("then", onOK, onErr)
}

// markCodePages flags the block's 256-byte pages so generated STOREs (and
// the interpreter's own SMC accounting) detect self-modifying writes.
func (rt *wasmJITRuntime) markCodePages(startPC, endPC uint64) {
	bmp := rt.cpu.jitCodePageBitmap
	for page := startPC >> 8; page <= (endPC-1)>>8; page++ {
		if page < uint64(len(bmp)) {
			bmp[page] = 1
		}
	}
}

// invalidateAll drops every compiled block and clears the code-page bitmap.
// The wasm backend always performs full invalidation on an SMC report: block
// count in browser workloads is modest and correctness beats bookkeeping.
func (rt *wasmJITRuntime) invalidateAll() {
	rt.gen++
	rt.blocks = map[uint64]*wasmJITBlock{}
	rt.hot = map[uint64]uint32{}
	rt.inFlight = map[uint64]bool{}
	clear(rt.pcCache) // the driver sees only misses until blocks re-tier
	rt.nextSlot = 0   // table slots are reused by the next generation
	// The blacklist survives: rejection reasons (unsupported opcodes) do not
	// change when code is rewritten at OTHER addresses, and a rewritten
	// blacklisted PC simply stays on the interpreter.
	clear(rt.cpu.jitCodePageBitmap)
}

// runBlock refreshes the exit-protocol fields, enters compiled code and
// applies the results, servicing helper exits and SMC reports. When the
// chain driver is ready the entry runs through it, dispatching
// block-to-block inside wasm until budget, miss, helper or invalidation.
func (rt *wasmJITRuntime) runBlock(blk *wasmJITBlock) {
	cpu := rt.cpu
	ctx := rt.ctx
	ctx.RetPC = 0
	ctx.RetCount = 0
	ctx.ChainCount = 0
	ctx.NeedHelper = 0
	ctx.NeedIOFallback = 0
	ctx.MMUEnabled = 0

	if rt.driver.Truthy() {
		ctx.RetPC = cpu.PC
		ctx.ChainBudget = ie64ChainBudget
		rt.driver.Invoke(rt.ctxPtr)
		rt.chainRuns++
	} else {
		blk.fn.Invoke(rt.ctxPtr)
	}
	blk.execs++
	rt.blockRuns++

	cpu.PC = ctx.RetPC
	executed := uint64(ctx.RetCount) + uint64(ctx.ChainCount)
	cpu.regs[0] = 0
	ctx.RetPC = 0
	ctx.RetCount = 0
	ctx.ChainCount = 0

	if ctx.NeedHelper != 0 {
		if rt.handleHelper() {
			executed++
		}
		ctx.NeedHelper = 0
	}
	if ctx.NeedInval != 0 {
		ctx.NeedInval = 0
		ctx.InvalAddr = 0
		ctx.InvalSize = 0
		rt.invalidateAll()
	}
	cpu.InstructionCount += executed
}

// handleHelper services the helper exits this backend's translator emits,
// mirroring the native dispatcher's semantics (jit_helper_dispatch.go).
// Returns true when the bailing instruction retired.
func (rt *wasmJITRuntime) handleHelper() bool {
	cpu := rt.cpu
	ctx := rt.ctx

	// LiveSP first, then PC at the bailing instruction, then a pending-IRQ
	// poll: if an interrupt fires here it wins and the instruction re-runs
	// after RTI.
	cpu.regs[31] = ctx.LiveSP
	cpu.PC = ctx.HelperPC
	if cpu.deliverPendingExternalInterrupt() {
		return false
	}

	size := byte(ctx.HelperSize)
	rd := byte(ctx.HelperRd)
	switch ctx.NeedHelper {
	case HELPER_LOAD:
		val := cpu.loadMem(ctx.HelperAddr, size)
		if cpu.trapped {
			cpu.trapped = false
			return false
		}
		cpu.setReg(rd, val)
		cpu.PC += 8
		return true
	case HELPER_STORE:
		cpu.storeMem(ctx.HelperAddr, ctx.HelperVal, size)
		if cpu.trapped {
			cpu.trapped = false
			return false
		}
		cpu.PC += 8
		return true
	case HELPER_PUSH:
		cpu.regs[31] -= 8
		if !cpu.mmuStackWriteU64(cpu.regs[31], ctx.HelperVal) {
			cpu.regs[31] += 8
			return false
		}
		cpu.PC += 8
		return true
	case HELPER_POP:
		val, ok := cpu.mmuStackReadU64(cpu.regs[31])
		if !ok {
			return false
		}
		cpu.regs[31] += 8
		cpu.setReg(rd, val)
		cpu.PC += 8
		return true
	case HELPER_JSR, HELPER_JSR_IND:
		cpu.regs[31] -= 8
		if !cpu.mmuStackWriteU64(cpu.regs[31], ctx.HelperVal) {
			cpu.regs[31] += 8
			return false
		}
		cpu.PC = ctx.HelperAddr
		return true
	case HELPER_RTS:
		val, ok := cpu.mmuStackReadU64(cpu.regs[31])
		if !ok {
			return false
		}
		cpu.regs[31] += 8
		cpu.PC = val
		return true
	case HELPER_DLOAD:
		val := cpu.loadMem(ctx.HelperAddr, IE64_SIZE_Q)
		if cpu.trapped {
			cpu.trapped = false
			return false
		}
		cpu.FPU.setDPair(rd, math.Float64frombits(val))
		cpu.FPU.setConditionCodesBits64(val)
		cpu.PC += 8
		return true
	case HELPER_DSTORE:
		cpu.storeMem(ctx.HelperAddr, ctx.HelperVal, IE64_SIZE_Q)
		if cpu.trapped {
			cpu.trapped = false
			return false
		}
		cpu.PC += 8
		return true
	}
	fmt.Printf("IE64 wasm JIT: unknown helper %d at %#x\n", ctx.NeedHelper, ctx.HelperPC)
	return false
}
