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
	slot   int      // chain-driver table slot
	execs  uint64   // per-block execution counter (tests, stats)
}

type wasmJITRuntime struct {
	cpu    *CPU64
	ctx    *JITContext
	ctxPtr int // linear-memory address of ctx, passed to every block

	memObj js.Value // the Go module's WebAssembly.Memory (env.mem import)

	blocks    map[uint64]*wasmJITBlock
	hot       map[uint64]uint32
	blacklist map[uint64]bool               // PCs whose blocks the translator rejected
	inFlight  map[uint64]wasmPendingCompile // compiles submitted, not yet installed
	// compileSeq issues the ownership token carried by each pending compile.
	// A callback may only retire the pc's entry (and touch the bitmap) when
	// its token still matches: after an invalidateAll clears the map and the
	// PC re-tiers, the OLD compile's callback must not delete the NEW
	// compile's entry or rebuild away its page protection.
	compileSeq uint64

	compiles  uint64 // completed installs (tests, stats)
	blockRuns uint64 // dispatcher entries into compiled code (tests, stats)
	chainRuns uint64 // entries that went through the in-wasm chain driver

	// diag (IE64_WASM_JIT_DIAG=1, ?jitdiag=1 on the demo page) publishes
	// dispatcher state to globalThis.__ieJITDiag and logs throttled
	// zero-progress exits, the signature of a dispatch livelock.
	diag      bool
	zeroProg  uint64
	helperCnt [16]uint64 // per-helper exit counts (diag)

	// MMIO poll-loop detection: consecutive LOAD helper exits at one PC arm
	// the parking poll service in the dispatcher (jit_mmio_poll_exec_wasm.go).
	lastLoadPC  uint64
	loadStreak  uint32
	pollRuns    uint64    // times the parking poll service engaged (diag)
	smcNoDrop   uint64    // false-share SMC exits (diag)
	fallSteps   uint64    // interpreter fallback steps (diag)
	enqueues    uint64    // compile submissions (diag)
	claimFails  uint64    // orphaned callbacks that lost ownership (diag)
	genDrops    uint64    // installs dropped by the generation check (diag)
	flushes     uint64    // invalidateAll calls (diag)
	rangeDrops  uint64    // blocks dropped by range invalidation (diag)
	smcAddrRing [8]uint64 // last false-share store addresses (diag)

	// In-wasm chaining: a shared funcref table holds every installed block,
	// and pcCache is a direct-mapped {pc, slot+1} table in linear memory the
	// driver module reads. driver is the exported drive() function once its
	// module instantiates.
	table    js.Value
	driver   js.Value
	pcCache  []byte
	nextSlot int

	// pageBlocks maps a 256-byte guest page to the PCs of installed blocks
	// touching it: the SMC overlap lookup in invalidateRange.
	pageBlocks map[uint64][]uint64

	// gen is the invalidation generation. Compiles capture it at submission
	// and their async install callbacks compare it on resolution: a compile
	// submitted before an SMC invalidation must not install afterwards, or
	// stale guest code would run.
	gen uint64
}

// wasmPendingCompile is an inFlight entry: the pending block's end and the
// ownership token its callbacks must present to retire it.
type wasmPendingCompile struct {
	endPC uint64
	token uint64
}

// claimInFlight retires pc's pending entry if the caller still owns it and
// reports whether it did. A stale callback (entry cleared or superseded by a
// newer compile for the same PC) gets false and must leave all shared state,
// including the code-page bitmap, untouched.
func (rt *wasmJITRuntime) claimInFlight(pc uint64, token uint64) bool {
	cur, ok := rt.inFlight[pc]
	if !ok || cur.token != token {
		return false
	}
	delete(rt.inFlight, pc)
	return true
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
	if len(cpu.jitCodePageSpans) == 0 {
		cpu.jitCodePageSpans = make([]byte, len(cpu.jitCodePageBitmap)*2)
		wasmResetCodePageSpans(cpu.jitCodePageSpans)
	}
	ctx.CodePageSpansPtr = uintptr(unsafe.Pointer(&cpu.jitCodePageSpans[0]))
	global := js.Global()
	tblDesc := global.Get("Object").New()
	tblDesc.Set("element", "anyfunc")
	tblDesc.Set("initial", wasmJITTableInitial)
	rt := &wasmJITRuntime{
		cpu:        cpu,
		ctx:        ctx,
		ctxPtr:     int(uintptr(unsafe.Pointer(ctx))),
		memObj:     mem,
		blocks:     map[uint64]*wasmJITBlock{},
		hot:        map[uint64]uint32{},
		blacklist:  map[uint64]bool{},
		inFlight:   map[uint64]wasmPendingCompile{},
		table:      global.Get("WebAssembly").Get("Table").New(tblDesc),
		pcCache:    make([]byte, wasmJITCacheEntries*16),
		pageBlocks: map[uint64][]uint64{},
		diag:       os.Getenv("IE64_WASM_JIT_DIAG") == "1",
	}
	rt.instantiateDriver()
	return rt
}

const (
	wasmJITCacheEntries = 16384 // power of two; 16 bytes per entry; sized above the RUN AOT working set to keep the driver's direct-mapped hit rate up
	wasmJITTableInitial = 256
	// wasmJITTableMaxSlots bounds the funcref table (and so the number of
	// simultaneously installed blocks) before a compacting full flush. It must
	// comfortably exceed a real workload's hot working set: RUN AOT of a large
	// BASIC programme installs 5000+ distinct blocks, and a cap below that
	// (4096, originally) put the runtime into a permanent flush-recompile
	// cycle that ran the demo at interpreter-fraction speed. The table costs
	// a few bytes per slot; the flush is compaction of slots leaked by range
	// invalidations, not a working-set limit.
	wasmJITTableMaxSlots = 65536
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
	if rt.blacklist[pc] {
		return
	}
	if _, ok := rt.inFlight[pc]; ok {
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

	rt.compileSeq++
	myToken := rt.compileSeq
	rt.inFlight[pc] = wasmPendingCompile{endPC: endPC, token: myToken}
	rt.enqueues++
	// Mark the pending range's pages now, not at install: a guest store into
	// bytes being compiled must trip the generated-store probe even when no
	// installed block shares the page, or the asynchronous install would
	// bring up a module built from the pre-store bytes.
	rt.markCodePages(pc, endPC)
	myGen := rt.gen
	var onOK, onErr js.Func
	release := func() {
		onOK.Release()
		onErr.Release()
	}
	onOK = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer release()
		if !rt.claimInFlight(pc, myToken) {
			// Ownership lost: the entry was cleared and a newer compile for
			// this PC may be pending. This module is stale by construction;
			// touch nothing, especially not the bitmap.
			rt.claimFails++
			return nil
		}
		if rt.gen != myGen {
			rt.genDrops++
			// Invalidated while compiling: the module was built from
			// instruction bytes that may have been overwritten. Drop it; the
			// PC re-tiers from scratch if it stays hot. The pending range's
			// enqueue-time page marks must not outlive it, or stores to
			// those pages would take false-share exits for ever.
			rt.rebuildCodePageBitmap()
			return nil
		}
		instance := args[0].Get("instance")
		fn := instance.Get("exports").Get("block")
		// Range invalidations leak table slots (dropped blocks keep theirs
		// until a full flush); compact by flushing when the table would grow
		// past its cap. This install proceeds afterwards: the generation
		// check above already vouched for its bytes.
		if rt.nextSlot >= wasmJITTableMaxSlots {
			rt.invalidateAll()
		}
		// Publish to the chain driver: table slot + pc cache entry.
		slot := rt.nextSlot
		rt.nextSlot++
		rt.blocks[pc] = &wasmJITBlock{
			fn:     fn,
			endPC:  endPC,
			module: instance,
			slot:   slot,
		}
		if lenNow := rt.table.Get("length").Int(); slot >= lenNow {
			rt.table.Call("grow", wasmJITTableInitial)
		}
		rt.table.Call("set", slot, fn)
		rt.cacheStore(pc, slot)
		rt.markCodePages(pc, endPC)
		rt.indexBlock(pc, endPC)
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
		if !rt.claimInFlight(pc, myToken) {
			return nil
		}
		rt.blacklist[pc] = true
		// A failed compile never becomes an installed block; drop its
		// enqueue-time page marks.
		rt.rebuildCodePageBitmap()
		wasmConsoleLog(fmt.Sprintf("IE64 wasm JIT: compile failed at %#x: %s", pc, args[0].Call("toString").String()))
		return nil
	})
	global.Get("WebAssembly").Call("instantiate", u8, imports).Call("then", onOK, onErr)
}

// markCodePages flags the block's 256-byte pages so generated STOREs (and
// the interpreter's own SMC accounting) detect self-modifying writes, and
// widens each page's [min, max] compiled-byte span. The store probe uses the
// span to let false shares (data beside code in the same page) pass without
// exiting.
func (rt *wasmJITRuntime) markCodePages(startPC, endPC uint64) {
	bmp := rt.cpu.jitCodePageBitmap
	spans := rt.cpu.jitCodePageSpans
	for page := startPC >> 8; page <= (endPC-1)>>8; page++ {
		if page >= uint64(len(bmp)) {
			continue
		}
		bmp[page] = 1
		lo := byte(0)
		if s := startPC; s > page<<8 {
			lo = byte(s & 255)
		}
		hi := byte(255)
		if e := endPC; e < (page+1)<<8 {
			hi = byte((e - 1) & 255)
		}
		if lo < spans[page*2] {
			spans[page*2] = lo
		}
		if hi > spans[page*2+1] {
			spans[page*2+1] = hi
		}
	}
}

// wasmResetCodePageSpans restores every span to the empty sentinel
// (min 0xFF, max 0x00), matching a cleared bitmap.
func wasmResetCodePageSpans(spans []byte) {
	for i := 0; i < len(spans); i += 2 {
		spans[i] = 0xFF
		spans[i+1] = 0
	}
}

// invalidateRange handles an SMC report from generated code: a committed
// store hit a marked 256-byte code page. The probe is page-granular, so the
// hit may be a false share (data living in the same page as compiled code,
// which EhBASIC's image does). Only blocks whose compiled range genuinely
// overlaps the written bytes are dropped; a hit that drops nothing keeps
// every block, avoiding the full-flush recompile storm that made sustained
// workloads (RUN AOT) hundreds of times slower than the interpreter.
// size 0 is the emitter's degraded report (several dirty stores in one
// block, exact range lost) and forces the full flush.
func (rt *wasmJITRuntime) invalidateRange(addr uint64, size uint32) {
	if size == 0 {
		rt.invalidateAll()
		return
	}
	// Overlap lookup goes through the page index, never a scan of the whole
	// block map: EhBASIC's assembler stores into a false-shared page a few
	// thousand times a second, and a full-map scan per exit throttled the
	// entire machine to interpreter-fraction speed.
	end := addr + uint64(size)
	// Collect first: unindexBlock swap-removes inside the very slices being
	// walked, so drops must not happen mid-iteration.
	var drops []uint64
	for page := addr >> 8; page <= (end-1)>>8; page++ {
		for _, pc := range rt.pageBlocks[page] {
			if blk, ok := rt.blocks[pc]; ok && pc < end && blk.endPC > addr {
				drops = append(drops, pc)
			}
		}
	}
	for _, pc := range drops {
		blk := rt.blocks[pc]
		if blk == nil {
			continue // straddling block collected via both pages
		}
		rt.rangeDrops++
		delete(rt.blocks, pc)
		rt.cacheDrop(pc)
		rt.unindexBlock(pc, blk.endPC)
	}
	// In-flight compiles were scanned from the pre-store bytes. One whose
	// range the store overlaps would install stale guest code from its
	// asynchronous callback; the map is at most a handful of entries.
	staleInFlight := false
	for pc, pending := range rt.inFlight {
		if pc < end && pending.endPC > addr {
			staleInFlight = true
			break
		}
	}
	if len(drops) == 0 && !staleInFlight {
		// False share: the page keeps its mark (it still holds compiled
		// code), so stores here keep exiting, but nothing recompiles.
		rt.smcNoDrop++
		if rt.diag {
			rt.smcAddrRing[rt.smcNoDrop&7] = addr
		}
		return
	}
	// The generation bump makes every pending install callback drop its
	// module; hot PCs re-tier from the current bytes.
	rt.gen++
	if len(drops) > 0 {
		rt.rebuildCodePageBitmap()
	}
}

// indexBlock records the block in the page index used by invalidateRange.
func (rt *wasmJITRuntime) indexBlock(pc, endPC uint64) {
	for page := pc >> 8; page <= (endPC-1)>>8; page++ {
		rt.pageBlocks[page] = append(rt.pageBlocks[page], pc)
	}
}

// unindexBlock removes the block from the page index.
func (rt *wasmJITRuntime) unindexBlock(pc, endPC uint64) {
	for page := pc >> 8; page <= (endPC-1)>>8; page++ {
		list := rt.pageBlocks[page]
		for i, p := range list {
			if p == pc {
				list[i] = list[len(list)-1]
				list = list[:len(list)-1]
				break
			}
		}
		if len(list) == 0 {
			delete(rt.pageBlocks, page)
		} else {
			rt.pageBlocks[page] = list
		}
	}
}

// cacheDrop clears pc's chain-driver cache entry, if it is the current
// occupant of its direct-mapped slot.
func (rt *wasmJITRuntime) cacheDrop(pc uint64) {
	idx := (pc >> 3) & (wasmJITCacheEntries - 1)
	e := rt.pcCache[idx*16 : idx*16+16]
	if *(*uint64)(unsafe.Pointer(&e[0])) == pc {
		clear(e)
	}
}

// rebuildCodePageBitmap re-derives the page marks from the live block set
// after a range invalidation, so pages owned only by dropped blocks stop
// tripping the store probes.
func (rt *wasmJITRuntime) rebuildCodePageBitmap() {
	clear(rt.cpu.jitCodePageBitmap)
	wasmResetCodePageSpans(rt.cpu.jitCodePageSpans)
	for pc, blk := range rt.blocks {
		rt.markCodePages(pc, blk.endPC)
	}
	// Pending compiles keep their marks too: a store into a range still
	// being compiled must go on tripping the probe.
	for pc, pending := range rt.inFlight {
		rt.markCodePages(pc, pending.endPC)
	}
}

// invalidateAll drops every compiled block and clears the code-page bitmap:
// the degraded (size-lost) SMC path and the table-slot compaction path.
func (rt *wasmJITRuntime) invalidateAll() {
	rt.gen++
	rt.flushes++
	rt.blocks = map[uint64]*wasmJITBlock{}
	rt.hot = map[uint64]uint32{}
	rt.inFlight = map[uint64]wasmPendingCompile{}
	rt.pageBlocks = map[uint64][]uint64{}
	clear(rt.pcCache) // the driver sees only misses until blocks re-tier
	rt.nextSlot = 0   // table slots are reused by the next generation
	// The blacklist survives: rejection reasons (unsupported opcodes) do not
	// change when code is rewritten at OTHER addresses, and a rewritten
	// blacklisted PC simply stays on the interpreter.
	clear(rt.cpu.jitCodePageBitmap)
	wasmResetCodePageSpans(rt.cpu.jitCodePageSpans)
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
		// Re-seat this block's pc cache entry: the cache is direct-mapped,
		// so a later install whose PC collides on the same index evicts it.
		// Without this the driver misses on its very first lookup, returns
		// with nothing retired and an unchanged PC, and the dispatcher
		// re-enters the same way forever (livelock observed on EhBASIC RUN
		// AOT, whose arena PCs collide with interpreter-core PCs).
		rt.cacheStore(cpu.PC, blk.slot)
		ctx.RetPC = cpu.PC
		ctx.ChainBudget = ie64ChainBudget
		rt.driver.Invoke(rt.ctxPtr)
		rt.chainRuns++
	} else {
		blk.fn.Invoke(rt.ctxPtr)
	}
	blk.execs++
	rt.blockRuns++

	entryPC := cpu.PC
	cpu.PC = ctx.RetPC
	executed := uint64(ctx.RetCount) + uint64(ctx.ChainCount)
	if rt.diag && executed == 0 && ctx.RetPC == entryPC && ctx.NeedHelper == 0 && ctx.NeedInval == 0 {
		rt.zeroProg++
		if rt.zeroProg&0xFFFF == 1 {
			wasmConsoleLog(fmt.Sprintf("IE64 wasm JIT diag: zero-progress exit #%d at pc=%#x", rt.zeroProg, entryPC))
		}
	}
	cpu.regs[0] = 0
	ctx.RetPC = 0
	ctx.RetCount = 0
	ctx.ChainCount = 0

	if ctx.NeedHelper != 0 {
		if ctx.NeedHelper < 16 {
			rt.helperCnt[ctx.NeedHelper]++
			if rt.diag && rt.helperCnt[ctx.NeedHelper]&0x3FFFF == 1 {
				wasmConsoleLog(fmt.Sprintf("IE64 wasm JIT diag: helper %d #%d pc=%#x addr=%#x",
					ctx.NeedHelper, rt.helperCnt[ctx.NeedHelper], ctx.HelperPC, ctx.HelperAddr))
			}
		}
		if rt.handleHelper() {
			executed++
		}
		ctx.NeedHelper = 0
	}
	if ctx.NeedInval != 0 {
		addr, size := ctx.InvalAddr, ctx.InvalSize
		ctx.NeedInval = 0
		ctx.InvalAddr = 0
		ctx.InvalSize = 0
		rt.invalidateRange(addr, size)
	}
	cpu.InstructionCount += executed
}

// markStackSMCWrite invalidates compiled blocks covering a raw stack write,
// the wasm equivalent of the native dispatcher's markJITSMCWrite: stack
// pushes bypass the generated-store SMC probe and the bus, so nothing else
// notices a push landing in a compiled code page.
func (rt *wasmJITRuntime) markStackSMCWrite(addr uint64) {
	bm := rt.cpu.jitCodePageBitmap
	if page := addr >> 8; page < uint64(len(bm)) && bm[page] != 0 {
		rt.invalidateRange(addr, 8)
	}
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

	// Stack helpers use the memBase/memSize variant of mmuStackWrite and
	// mmuStackRead, mirroring the native dispatcher (jit_helper_dispatch.go)
	// and the interpreter: stack traffic inside the memory window is a raw
	// RAM access and must never fire MMIO callbacks. The Voodoo texture
	// aperture at 0xD0000 is bitmap-marked IO, and a guest stack parked
	// there (rotozoomer_ie64.ie64 uses 0xDF000) would otherwise push into
	// RAM via the interpreter but pop through the device handler, returning
	// zeros and sending the guest to PC 0.
	var memBase unsafe.Pointer
	var memSize uint64
	if len(cpu.memory) > 0 {
		memBase = unsafe.Pointer(&cpu.memory[0])
		memSize = uint64(len(cpu.memory))
	}

	switch ctx.NeedHelper {
	case HELPER_LOAD:
		if ctx.HelperPC == rt.lastLoadPC {
			rt.loadStreak++
		} else {
			rt.lastLoadPC = ctx.HelperPC
			rt.loadStreak = 1
		}
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
		if !cpu.mmuStackWrite(cpu.regs[31], ctx.HelperVal, memBase, memSize) {
			cpu.regs[31] += 8
			cpu.trapped = false
			return false
		}
		rt.markStackSMCWrite(cpu.regs[31])
		cpu.PC += 8
		return true
	case HELPER_POP:
		val, ok := cpu.mmuStackRead(cpu.regs[31], memBase, memSize)
		if !ok {
			cpu.trapped = false
			return false
		}
		cpu.regs[31] += 8
		cpu.setReg(rd, val)
		cpu.PC += 8
		return true
	case HELPER_JSR, HELPER_JSR_IND:
		cpu.regs[31] -= 8
		if !cpu.mmuStackWrite(cpu.regs[31], ctx.HelperVal, memBase, memSize) {
			cpu.regs[31] += 8
			cpu.trapped = false
			return false
		}
		rt.markStackSMCWrite(cpu.regs[31])
		cpu.PC = ctx.HelperAddr
		return true
	case HELPER_RTS:
		val, ok := cpu.mmuStackRead(cpu.regs[31], memBase, memSize)
		if !ok {
			cpu.trapped = false
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
