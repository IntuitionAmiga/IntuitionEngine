// jit_m68k_dispatch_wasm.go - M68020 wasm JIT execution dispatcher (js/wasm).
//
// Milestone 5. This wires the wasm M68020 codegen backend (jit_m68k_wasm_emit.go)
// into a browser dispatcher. Each supported basic-block prefix is translated to
// a small wasm module exporting block(ctx i32) -> (), compiled synchronously on
// the main thread (new WebAssembly.Module; the blocks are far below the 4 KiB
// main-thread sync-compile limit), instantiated against Go's own linear memory
// (globalThis.__goMem, imported as env.mem) and cached by guest PC.
//
// Context image. The emitted block reads the register-file bases and status
// pointers from a context image at fixed byte offsets (the m68kCtxOff*
// constants). Rather than hand the block the M68KJITContext struct, whose field
// offsets differ between the amd64 test host (8-byte uintptr) and wasm32
// (4-byte uintptr), the dispatcher builds a dedicated byte image laid out
// exactly per those constants and fills the pointer fields with the guest CPU's
// real Go addresses. Because env.mem is Go's linear memory, a Go address is
// directly the linear-memory offset the block dereferences. This is the very
// same layout the wazero differential tests populate, so the browser path
// executes byte-for-byte what the tests verify.
//
// Self-modifying code. Correctness rests on a stamp check before every entry:
// the guest bytes a block covers are copied at compile time and compared against
// live memory before the block runs, so a modified block recompiles rather than
// executing stale code. Milestone 6 adds structured in-block loops, which can
// re-execute their own body without returning to the dispatcher, so the
// dispatcher also maintains a per-4KiB code-page bitmap (CodePageBitmapPtr): a
// store into a compiled-code page sets NeedInval, a loop exits at that point,
// and the dispatcher drops the block cache.
//
// Interpreter fallback. A block that hits a guarded I/O access or a RAM bound
// exits before the faulting instruction's side effects with NeedIOFallback set,
// the faulting PC and the partial retired count; the dispatcher interprets that
// one instruction. Cold, unsupported or not-yet-compiled PCs interpret directly.

//go:build js && wasm

package main

import (
	"encoding/binary"
	"os"
	"syscall/js"
	"unsafe"
)

// m68kWasmCtxImageSize covers every context field the emitter reads or writes
// (the highest is FPIARPtr at 432), rounded up.
const m68kWasmCtxImageSize = 480

// m68kWasmMinPrefix: single-instruction blocks are still worth caching on wasm
// because a taken branch or a tight DBcc loop is exactly one instruction and
// benefits most from avoiding the interpreter, but a zero-length prefix (first
// instruction unsupported) always interprets.
const m68kWasmMinPrefix = 1

// m68kWasmYieldMask parks the goroutine periodically so the JS event loop can
// render and take input. A dispatcher iteration retires at most one block, so
// the cadence is tight, mirroring the IE64 wasm dispatcher's lesson.
const m68kWasmYieldMask = 0x3F

type m68kWasmBlock struct {
	fn         js.Value
	startPC    uint32
	endPC      uint32
	instrCount int
	guest      []byte // snapshot of the covered guest bytes for the SMC stamp
}

type m68kWasmRuntime struct {
	cpu       *M68KCPU
	memObj    js.Value
	imports   js.Value
	wasm      js.Value
	u8array   js.Value
	ctxImage  []byte
	ctxAddr   int
	ceiling   uint32 // profile-visible RAM ceiling (ProfileTopOfRAM); the guard bound
	cache     map[uint32]*m68kWasmBlock
	blacklist map[uint32]bool
	// codePageBitmap marks each 4 KiB guest page holding compiled code. Stores
	// into a marked page set NeedInval, which lets a structured in-block loop
	// (milestone 6) exit before re-running self-modified code; the dispatcher
	// then drops the cache. Straight-line blocks stay safe via the pre-entry
	// stamp check either way.
	codePageBitmap []byte
}

// m68kWasmJITEnabled reports whether the wasm M68020 JIT should run: default on,
// killed by M68K_WASM_JIT=0, and requires the hosting page to have exposed Go's
// module memory as globalThis.__goMem.
func m68kWasmJITEnabled() bool {
	if os.Getenv("M68K_WASM_JIT") == "0" {
		return false
	}
	return js.Global().Get("__goMem").Truthy()
}

// m68kJitExecute runs the wasm M68020 JIT dispatcher when enabled, otherwise the
// interpreter.
func (cpu *M68KCPU) m68kJitExecute() {
	if !m68kWasmJITEnabled() {
		cpu.ExecuteInstruction()
		return
	}
	rt := newM68KWasmRuntime(cpu)
	if rt == nil {
		cpu.ExecuteInstruction()
		return
	}
	rt.dispatch()
}

// freeM68KJIT: compiled blocks are ordinary JS-managed wasm instances held by
// the runtime's cache; dropping the runtime is enough.
func (cpu *M68KCPU) freeM68KJIT() {}

func newM68KWasmRuntime(cpu *M68KCPU) *m68kWasmRuntime {
	mem := js.Global().Get("__goMem")
	if !mem.Truthy() || len(cpu.memory) == 0 {
		return nil
	}
	// The guarded memory path needs the I/O page bitmap before any block runs.
	cpu.m68kBuildJITIOPageBitmap()

	g := js.Global()
	env := g.Get("Object").New()
	env.Set("mem", mem)
	imports := g.Get("Object").New()
	imports.Set("env", env)

	rt := &m68kWasmRuntime{
		cpu:       cpu,
		memObj:    mem,
		imports:   imports,
		wasm:      g.Get("WebAssembly"),
		u8array:   g.Get("Uint8Array"),
		ctxImage:  make([]byte, m68kWasmCtxImageSize),
		ceiling:   cpu.ProfileTopOfRAM(),
		cache:     map[uint32]*m68kWasmBlock{},
		blacklist: map[uint32]bool{},
	}
	rt.codePageBitmap = make([]byte, (rt.ceiling>>12)+1)
	rt.ctxAddr = int(uintptr(unsafe.Pointer(&rt.ctxImage[0])))
	rt.fillStaticCtx()
	return rt
}

// fillStaticCtx writes the pointer and size fields that stay fixed for the whole
// run. Pointer fields occupy 8-byte slots but hold a 32-bit wasm address in the
// low word; the high word stays zero, so the block's i32.load reads the address.
func (rt *m68kWasmRuntime) fillStaticCtx() {
	cpu := rt.cpu
	putPtr := func(off int, p unsafe.Pointer) {
		binary.LittleEndian.PutUint32(rt.ctxImage[off:], uint32(uintptr(p)))
	}
	putU32 := func(off int, v uint32) {
		binary.LittleEndian.PutUint32(rt.ctxImage[off:], v)
	}
	putPtr(m68kCtxOffDataRegsPtr, unsafe.Pointer(&cpu.DataRegs[0]))
	putPtr(m68kCtxOffAddrRegsPtr, unsafe.Pointer(&cpu.AddrRegs[0]))
	putPtr(m68kCtxOffMemPtr, unsafe.Pointer(&cpu.memory[0]))
	// Guard bound is the profile-visible RAM ceiling, NOT the backing slice: when
	// active RAM is smaller than the backing allocation, an access at or beyond
	// ProfileTopOfRAM must bail so the interpreter raises the same bus error,
	// rather than silently reading architecturally inaccessible backing memory.
	putU32(m68kCtxOffMemSize, rt.ceiling)
	putPtr(m68kCtxOffSRPtr, unsafe.Pointer(&cpu.SR))
	if len(cpu.m68kJitIOPageBitmap) > 0 {
		putPtr(m68kCtxOffIOPageBitmapPtr, unsafe.Pointer(&cpu.m68kJitIOPageBitmap[0]))
		putU32(m68kCtxOffIOPageBitmapLen, uint32(len(cpu.m68kJitIOPageBitmap)))
	}
	if cpu.FPU != nil {
		putPtr(m68kCtxOffFPRegsPtr, unsafe.Pointer(&cpu.FPU.fp[0]))
		putPtr(m68kCtxOffFPSRPtr, unsafe.Pointer(&cpu.FPU.FPSR))
		putPtr(m68kCtxOffFPCRPtr, unsafe.Pointer(&cpu.FPU.FPCR))
		putPtr(m68kCtxOffFPIARPtr, unsafe.Pointer(&cpu.FPU.FPIAR))
	}
	// Stack floor/ceiling for the milestone 6 push/pop guards (BSR/JSR/RTS).
	putPtr(m68kCtxOffStackLowerBoundPtr, unsafe.Pointer(&cpu.stackLowerBound))
	putPtr(m68kCtxOffStackUpperBoundPtr, unsafe.Pointer(&cpu.stackUpperBound))
	// Code-page bitmap: stores into compiled-code pages set NeedInval so
	// structured in-block loops exit before re-running self-modified code.
	putPtr(m68kCtxOffCodePageBitmapPtr, unsafe.Pointer(&rt.codePageBitmap[0]))
}

func (rt *m68kWasmRuntime) ctxU32(off int) uint32 {
	return binary.LittleEndian.Uint32(rt.ctxImage[off:])
}
func (rt *m68kWasmRuntime) setCtxU32(off int, v uint32) {
	binary.LittleEndian.PutUint32(rt.ctxImage[off:], v)
}

// compile scans, admits and translates the block at pc, then compiles and
// instantiates it synchronously. Returns nil if the prefix is too short or the
// browser rejected the module.
func (rt *m68kWasmRuntime) compile(pc uint32) (blk *m68kWasmBlock) {
	cpu := rt.cpu
	if pc >= rt.ceiling {
		return nil
	}
	instrs := m68kScanBlock(cpu.memory, pc)
	// Never compile an instruction that extends past the profile-visible RAM
	// ceiling: fetching it is itself a bus error the interpreter must raise.
	kept := 0
	for i := range instrs {
		if pc+instrs[i].pcOffset+uint32(instrs[i].length) > rt.ceiling {
			break
		}
		kept = i + 1
	}
	instrs = instrs[:kept]
	prefix := m68kWasmSupportedPrefix(instrs, cpu.memory, pc)
	if prefix < m68kWasmMinPrefix {
		return nil
	}
	instrs = instrs[:prefix]
	modBytes, err := m68kWasmCompileBlock(instrs, cpu.memory, pc)
	if err != nil {
		return nil
	}
	var endPC uint32 = pc
	for i := range instrs {
		endPC = pc + instrs[i].pcOffset + uint32(instrs[i].length)
	}

	// Synchronous compile + instantiate; a JS exception surfaces as a Go panic.
	defer func() {
		if recover() != nil {
			blk = nil
		}
	}()
	u8 := rt.u8array.New(len(modBytes))
	js.CopyBytesToJS(u8, modBytes)
	mod := rt.wasm.Get("Module").New(u8)
	inst := rt.wasm.Get("Instance").New(mod, rt.imports)
	fn := inst.Get("exports").Get("block")
	if !fn.Truthy() {
		return nil
	}
	guest := make([]byte, endPC-pc)
	copy(guest, cpu.memory[pc:endPC])
	for page := pc >> 12; page <= (endPC-1)>>12 && int(page) < len(rt.codePageBitmap); page++ {
		rt.codePageBitmap[page] = 1
	}
	return &m68kWasmBlock{fn: fn, startPC: pc, endPC: endPC, instrCount: prefix, guest: guest}
}

// stampMatches verifies the block's covered guest bytes are unchanged.
func (rt *m68kWasmRuntime) stampMatches(blk *m68kWasmBlock) bool {
	if int(blk.endPC) > len(rt.cpu.memory) {
		return false
	}
	live := rt.cpu.memory[blk.startPC:blk.endPC]
	for i := range blk.guest {
		if live[i] != blk.guest[i] {
			return false
		}
	}
	return true
}

// m68kWasmCheckPending delivers a pending exception or interrupt at the block
// boundary, mirroring the arm64 dispatcher's boundary handling.
func (cpu *M68KCPU) m68kWasmCheckPending() bool {
	delivered := false
	if pendingException := cpu.pendingException.Load(); pendingException != 0 {
		cpu.pendingException.Store(0)
		cpu.ProcessException(uint8(pendingException))
		delivered = true
	}
	if pending := cpu.pendingInterrupt.Load(); pending != 0 {
		ipl := uint32((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
		for level := uint32(7); level >= 1; level-- {
			if pending&(1<<level) != 0 && (level > ipl || level == 7) {
				if cpu.ProcessInterrupt(uint8(level)) {
					delivered = true
					for {
						old := cpu.pendingInterrupt.Load()
						if cpu.pendingInterrupt.CompareAndSwap(old, old&^(1<<level)) {
							break
						}
					}
				}
				break
			}
		}
	}
	return delivered
}

func (rt *m68kWasmRuntime) dispatch() {
	cpu := rt.cpu
	if cpu.PerfEnabled {
		cpu.InstructionCount = 0
	}
	var instructionCount uint64
	checkCounter := uint32(0)

	for cpu.running.Load() {
		if cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			break
		}
		if cpu.stopped.Load() {
			if cpu.m68kWasmCheckPending() {
				cpu.stopped.Store(false)
				cpu.stopSpinCount.Store(0)
				continue
			}
			if cpu.StoppedIdleHook != nil {
				cpu.StoppedIdleHook(cpu)
			}
			hostCooperativeYield()
			continue
		}

		checkCounter++
		if checkCounter&m68kWasmYieldMask == 0 {
			hostCooperativeYield()
			if !cpu.running.Load() {
				break
			}
		}

		cpu.runInstructionCountHook(instructionCount)
		cpu.m68kWasmCheckPending()
		if !cpu.running.Load() || cpu.stopped.Load() {
			continue
		}

		pc := cpu.PC
		blk := rt.cache[pc]
		if blk != nil && !rt.stampMatches(blk) {
			delete(rt.cache, pc)
			blk = nil
		}
		if blk == nil && !rt.blacklist[pc] && pc < rt.ceiling && pc&1 == 0 {
			if compiled := rt.compile(pc); compiled != nil {
				rt.cache[pc] = compiled
				blk = compiled
			} else {
				rt.blacklist[pc] = true
			}
		}
		if blk == nil {
			instructionCount += uint64(cpu.StepOne())
			if cpu.PerfEnabled {
				cpu.InstructionCount = instructionCount
			}
			continue
		}

		// Run the compiled block. Clear the dynamic result fields first.
		rt.setCtxU32(m68kCtxOffRetPC, 0)
		rt.setCtxU32(m68kCtxOffRetCount, 0)
		rt.setCtxU32(m68kCtxOffNeedIOFallback, 0)
		rt.setCtxU32(m68kCtxOffNeedInval, 0)
		if !rt.invoke(blk) {
			// A JS trap (should not happen: guests bound-check every access).
			// Fall back to interpreting one instruction and drop the block.
			delete(rt.cache, pc)
			instructionCount += uint64(cpu.StepOne())
			continue
		}
		cpu.PC = rt.ctxU32(m68kCtxOffRetPC)

		if rt.ctxU32(m68kCtxOffNeedInval) != 0 {
			// A store hit a compiled-code page (a loop's early exit, or any
			// block writing over code). Drop every cached block and the page
			// marks; live blocks re-mark on recompile. The stamp check would
			// catch a stale straight-line block anyway; this keeps loops safe.
			rt.cache = map[uint32]*m68kWasmBlock{}
			rt.blacklist = map[uint32]bool{}
			for i := range rt.codePageBitmap {
				rt.codePageBitmap[i] = 0
			}
		}

		if rt.ctxU32(m68kCtxOffNeedIOFallback) != 0 {
			// The block exited before a guarded access's side effects; RetCount
			// holds the retired predecessors and PC the faulting instruction.
			instructionCount += uint64(rt.ctxU32(m68kCtxOffRetCount))
			instructionCount += uint64(cpu.StepOne())
			if cpu.PerfEnabled {
				cpu.InstructionCount = instructionCount
			}
			continue
		}

		// RetCount is authoritative: a structured in-block loop (milestone 6)
		// retires a dynamic multiple of the block's instruction count.
		instructionCount += uint64(rt.ctxU32(m68kCtxOffRetCount))
		if cpu.PerfEnabled {
			cpu.InstructionCount = instructionCount
		}
	}
	if cpu.PerfEnabled {
		cpu.InstructionCount = instructionCount
	}
}

// invoke calls the block's exported function with the context address. A JS
// exception (a linear-memory trap) surfaces as a Go panic and is reported as
// failure so the dispatcher can recover.
func (rt *m68kWasmRuntime) invoke(blk *m68kWasmBlock) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	blk.fn.Invoke(rt.ctxAddr)
	return true
}
