//go:build js && wasm

package main

import (
	"encoding/binary"
	"os"
	"syscall/js"
	"unsafe"
)

var z80JitAvailable bool

const (
	z80WasmChainBlockBudget uint32 = 64
	z80WasmInterruptBudget  uint32 = 200
)

type z80WasmBlock struct {
	fn                js.Value
	source            []byte
	mappingGeneration uint64
	pageGenerations   [256]uint64
	sourcePages       [256]bool
}

type z80WasmRuntime struct {
	cpu     *CPU_Z80
	adapter *Z80BusAdapter
	wasm    js.Value
	u8array js.Value
	imports js.Value
	ctx     []byte
	ctxAddr int
	cache   map[uint16]*z80WasmBlock
	seen    [256]uint64
	unreg   func()
}

func z80WasmJITEnabled() bool {
	return os.Getenv("Z80_WASM_JIT") != "0" && js.Global().Get("__goMem").Truthy() && js.Global().Get("WebAssembly").Get("Module").Truthy()
}

func init() { z80JitAvailable = z80WasmJITEnabled() }

func z80JITBackend() string { return "wasm" }

func (cpu *CPU_Z80) z80JitExecute() {
	if cpu.Debug || !cpu.jitEnabled || !z80WasmJITEnabled() {
		cpu.Execute()
		return
	}
	cpu.ExecuteJITZ80()
}

func (cpu *CPU_Z80) freeZ80JIT() {}

// initZ80WasmDirectPages mirrors the frontend admission portion of the native
// bitmap setup. Wasm modules only lower register and static-control forms, but
// the shared region scanner must still refuse logical bank, VRAM and I/O code
// pages before caching a static chain.
func initZ80WasmDirectPages(cpu *CPU_Z80, adapter *Z80BusAdapter) {
	for page := range cpu.directPageBitmap {
		addr := uint16(page) << 8
		if adapter.flatMemory {
			cpu.directPageBitmap[page] = 0
			continue
		}
		if adapter.coprocFlat {
			cpu.directPageBitmap[page] = 0
			if addr >= adapter.coprocMailboxStart && addr < adapter.coprocMailboxEnd {
				cpu.directPageBitmap[page] = 1
			}
			continue
		}
		direct := !(page >= 0x20 && page <= 0xBF || page >= 0xF0)
		// Match initDirectPageBitmapZ80: a MachineBus MMIO handler may be
		// mapped on an otherwise ordinary Z80 page. Compiling fetches from it
		// would suppress fetch side effects and freeze dynamic opcodes.
		if direct {
			mbPage := int(translateIO8Bit(addr) >> 8)
			if mbPage < len(adapter.bus.ioPageBitmap) && adapter.bus.ioPageBitmap[mbPage] {
				direct = false
			}
		}
		if !direct {
			cpu.directPageBitmap[page] = 1
		} else {
			cpu.directPageBitmap[page] = 0
		}
	}
}

func newZ80WasmRuntime(cpu *CPU_Z80, adapter *Z80BusAdapter) *z80WasmRuntime {
	mem := js.Global().Get("__goMem")
	if adapter == nil || !mem.Truthy() {
		return nil
	}
	ctx := make([]byte, z80WasmCtxImageSize)
	rt := &z80WasmRuntime{
		cpu: cpu, adapter: adapter, wasm: js.Global().Get("WebAssembly"), u8array: js.Global().Get("Uint8Array"),
		ctx: ctx, ctxAddr: int(uintptr(unsafe.Pointer(&ctx[0]))), cache: map[uint16]*z80WasmBlock{},
	}
	env := js.Global().Get("Object").New()
	env.Set("mem", mem)
	rt.imports = js.Global().Get("Object").New()
	rt.imports.Set("env", env)
	// Physical writes are published by MachineBus, even when the writer is a
	// loader, debugger or another CPU. The browser dispatcher owns cache
	// mutation and drains that publication at an instruction boundary.
	rt.unreg = adapter.bus.RegisterZ80JITInvalidator(func(addr, size uint64) {
		if size == 0 {
			return
		}
		if adapter.coprocFlat {
			start, end := uint64(adapter.coprocBase), uint64(adapter.coprocBase)+z80AddressSpace
			if addr >= end || addr+size <= start {
				return
			}
			lo, hi := addr, addr+size
			if lo < start {
				lo = start
			}
			if hi > end {
				hi = end
			}
			addr, size = lo-start, hi-lo
		}
		if addr > 0xFFFF {
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
			cpu.jitCodeGeneration[page].Add(1)
		}
	})
	return rt
}

func (rt *z80WasmRuntime) drainPhysicalWrites() {
	changed := false
	for page := range rt.seen {
		generation := rt.cpu.jitCodeGeneration[page].Load()
		if generation != rt.seen[page] {
			rt.seen[page] = generation
			changed = true
		}
	}
	if changed {
		clear(rt.cache)
		rt.cpu.jitStats.invalidations.Add(1)
	}
}

func (rt *z80WasmRuntime) sourceMatches(pc uint16, block *z80WasmBlock) bool {
	if block.mappingGeneration != rt.adapter.mappingGeneration.Load() {
		return false
	}
	for page, generation := range block.pageGenerations {
		if block.sourcePages[page] && generation != rt.cpu.jitCodeGeneration[page].Load() {
			return false
		}
	}
	for i, value := range block.source {
		if rt.adapter.fetchRead(pc+uint16(i)) != value {
			return false
		}
	}
	return true
}

func (rt *z80WasmRuntime) compile(pc uint16) (block *z80WasmBlock) {
	payloads := z80FrontendScanBlock(rt.adapter.fetchRead, func(addr uint16) bool {
		return rt.cpu.directPageBitmap[addr>>8] == 0
	}, z80WasmFrontendAdmits, pc)
	if rt.cpu.jitSingleStep && len(payloads) > 1 {
		payloads = payloads[:1]
	}
	instrs := make([]z80WasmInstr, 0, len(payloads))
	source := make([]byte, 0, len(payloads)*2)
	cycles := uint32(0)
	for _, payload := range payloads {
		instr := z80WasmInstr{
			prefix: payload.Prefix, opcode: payload.Opcode,
			operand: byte(payload.Operand), operandHi: byte(payload.Operand >> 8),
			displacement: payload.Displacement,
			indexedCB:    (payload.Prefix == z80JITPrefixDD || payload.Prefix == z80JITPrefixFD) && payload.Bytes[1] == z80JITPrefixCB,
		}
		_, cost, _, err := z80WasmInstructionMeta(instr)
		if err != nil || len(instrs) >= int(z80WasmChainBlockBudget) || cycles+uint32(cost) > z80WasmInterruptBudget {
			break
		}
		instrs = append(instrs, instr)
		source = append(source, payload.Bytes[:payload.Length]...)
		cycles += uint32(cost)
	}
	if len(instrs) == 0 {
		return nil
	}
	module, err := z80WasmCompileBlock(instrs, pc)
	if err != nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			block = nil
		}
	}()
	u8 := rt.u8array.New(len(module))
	js.CopyBytesToJS(u8, module)
	mod := rt.wasm.Get("Module").New(u8)
	instance := rt.wasm.Get("Instance").New(mod, rt.imports)
	fn := instance.Get("exports").Get("block")
	if !fn.Truthy() {
		return nil
	}
	block = &z80WasmBlock{fn: fn, source: source, mappingGeneration: rt.adapter.mappingGeneration.Load()}
	for offset := range source {
		page := byte(uint16(pc+uint16(offset)) >> 8)
		block.sourcePages[page] = true
		rt.cpu.codePageBitmap[page] = 1
	}
	for page := range block.pageGenerations {
		if !block.sourcePages[page] {
			continue
		}
		block.pageGenerations[page] = rt.cpu.jitCodeGeneration[page].Load()
	}
	return block
}

func z80WasmFrontendAdmits(payload z80CanonicalHelperPayload) bool {
	// HALT is a dispatcher outcome, not a wasm instruction lowering. Admitting
	// it here would compile a no-op and let execution fall through the halted
	// boundary before an interrupt can wake the CPU.
	if payload.Prefix == z80JITPrefixNone && payload.Opcode == 0x76 {
		return false
	}
	instr := z80WasmInstr{
		prefix: payload.Prefix, opcode: payload.Opcode,
		operand: byte(payload.Operand), operandHi: byte(payload.Operand >> 8),
		displacement: payload.Displacement,
		indexedCB:    (payload.Prefix == z80JITPrefixDD || payload.Prefix == z80JITPrefixFD) && payload.Bytes[1] == z80JITPrefixCB,
	}
	_, _, _, err := z80WasmInstructionMeta(instr)
	return err == nil
}

// promoteStaticRegion uses the same untagged region policy as native. wasm
// retains separate modules per block, but compiles every static member before
// execution so the dispatcher cache contains the complete bounded chain.
func (rt *z80WasmRuntime) promoteStaticRegion(startPC uint16) int {
	pcs := z80FrontendRegionPlan(rt.adapter.fetchRead, func(pc uint16) bool {
		return rt.cpu.directPageBitmap[pc>>8] == 0
	}, z80WasmFrontendAdmits, startPC)
	if len(pcs) == 0 {
		return 0
	}
	for _, pc := range pcs {
		block := rt.cache[pc]
		if block != nil && rt.sourceMatches(pc, block) {
			continue
		}
		block = rt.compile(pc)
		if block == nil {
			return 0
		}
		rt.cache[pc] = block
	}
	rt.cpu.jitStats.regionPromotions.Add(1)
	return len(pcs)
}

func (rt *z80WasmRuntime) invoke(block *z80WasmBlock) (ok bool) {
	defer func() { ok = recover() == nil }()
	mem := rt.adapter.jitMemory()
	if len(mem) == 0 {
		return false
	}
	binary.LittleEndian.PutUint32(rt.ctx[z80WasmCtxOffCPUPtr:], uint32(uintptr(unsafe.Pointer(rt.cpu))))
	binary.LittleEndian.PutUint32(rt.ctx[z80WasmCtxOffMemPtr:], uint32(uintptr(unsafe.Pointer(&mem[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[z80WasmCtxOffDirectPageBitmap:], uint32(uintptr(unsafe.Pointer(&rt.cpu.directPageBitmap[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[z80WasmCtxOffCodePageBitmap:], uint32(uintptr(unsafe.Pointer(&rt.cpu.codePageBitmap[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[z80WasmCtxOffNeedBail:], 0)
	binary.LittleEndian.PutUint32(rt.ctx[z80WasmCtxOffNeedInval:], 0)
	binary.LittleEndian.PutUint32(rt.ctx[z80WasmCtxOffIFFDelay:], uint32(rt.cpu.iffDelay))
	block.fn.Invoke(rt.ctxAddr)
	rt.cpu.iffDelay = int(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffIFFDelay:]))
	return true
}

// z80WasmFrozenPayload snapshots the longest Z80 encoding before invoking the
// shared canonical helper. The helper's fetch bus never reads mutable code.
func z80WasmFrozenPayload(adapter *Z80BusAdapter, pc uint16) z80CanonicalHelperPayload {
	return z80CanonicalHelperPayloadFromFetch(adapter.fetchRead, pc)
}

func (cpu *CPU_Z80) ExecuteJITZ80() {
	adapter, ok := cpu.bus.(*Z80BusAdapter)
	if !ok || !z80WasmJITEnabled() {
		cpu.Execute()
		return
	}
	adapter.bus.SealMappings()
	initZ80WasmDirectPages(cpu, adapter)
	rt := newZ80WasmRuntime(cpu, adapter)
	if rt == nil {
		cpu.Execute()
		return
	}
	defer func() {
		if rt.unreg != nil {
			rt.unreg()
		}
	}()
	var yields uint32
	for cpu.running.Load() {
		if cpu.executionBoundary != nil {
			cpu.executionBoundary()
			if !cpu.running.Load() {
				return
			}
		}
		rt.drainPhysicalWrites()
		if cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			return
		}
		if cpu.nmiPending.Load() || (cpu.irqLine.Load() && cpu.IFF1) || cpu.iffDelay > 0 || cpu.Halted {
			cpu.Step()
			yields++
			if yields&0xFFF == 0 {
				hostCooperativeYield()
			}
			continue
		}
		pc := cpu.PC
		block := rt.cache[pc]
		if block != nil && !rt.sourceMatches(pc, block) {
			delete(rt.cache, pc)
			cpu.jitStats.invalidations.Add(1)
			block = nil
		}
		if block == nil && !cpu.jitSingleStep {
			if rt.promoteStaticRegion(pc) > 0 {
				block = rt.cache[pc]
			}
		}
		if block == nil {
			block = rt.compile(pc)
			if block != nil {
				rt.cache[pc] = block
			}
		}
		if block == nil {
			payload := z80WasmFrozenPayload(adapter, pc)
			if !z80CanonicalHelperPayloadComplete(payload) {
				cpu.Step()
			} else {
				payload.ExitReason = uint32(DeoptUnsupported)
				cpu.executeZ80CanonicalHelper(payload)
				cpu.jitStats.helperExits.Add(1)
			}
			if cpu.PerfEnabled {
				cpu.InstructionCount++
			}
			continue
		}
		if !rt.invoke(block) {
			delete(rt.cache, pc)
			cpu.jitStats.bailouts.Add(1)
			continue
		}
		if binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffNeedBail:]) != 0 {
			// The emitter has already completed the direct prefix of this block.
			// Retire that exact prefix before using the frozen helper at RetPC.
			cpu.PC = uint16(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffRetPC:]))
			cycles := binary.LittleEndian.Uint64(rt.ctx[z80WasmCtxOffRetCycles:])
			cpu.Cycles += cycles
			cpu.bus.Tick(int(cycles))
			cpu.R = (cpu.R & 0x80) | ((cpu.R + byte(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffRIncrements:]))) & 0x7F)
			if cpu.PerfEnabled {
				cpu.InstructionCount += uint64(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffRetCount:]))
			}
			payload := z80WasmFrozenPayload(adapter, cpu.PC)
			if !z80CanonicalHelperPayloadComplete(payload) {
				cpu.Step()
			} else {
				payload.ExitReason = uint32(DeoptMMIO)
				cpu.executeZ80CanonicalHelper(payload)
				cpu.jitStats.helperExits.Add(1)
			}
			if cpu.PerfEnabled {
				cpu.InstructionCount++
			}
			cpu.jitStats.bailouts.Add(1)
			continue
		}
		cpu.PC = uint16(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffRetPC:]))
		cycles := binary.LittleEndian.Uint64(rt.ctx[z80WasmCtxOffRetCycles:])
		cpu.Cycles += cycles
		// CPU_Z80.tick publishes elapsed T-states to attached devices. Native
		// blocks already do this in the shared post-call path; the wasm
		// dispatcher must do it too or device time diverges despite matching
		// CPU.Cycles.
		cpu.bus.Tick(int(cycles))
		cpu.R = (cpu.R & 0x80) | ((cpu.R + byte(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffRIncrements:]))) & 0x7F)
		cpu.jitStats.nativeEntries.Add(1)
		if cpu.PerfEnabled {
			cpu.InstructionCount += uint64(binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffRetCount:]))
		}
		if binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffNeedInval:]) != 0 {
			page := binary.LittleEndian.Uint32(rt.ctx[z80WasmCtxOffInvalPage:])
			cpu.jitCodeGeneration[page].Add(1)
			clear(rt.cache)
			cpu.jitStats.invalidations.Add(1)
		}
		yields++
		if yields&0xFFF == 0 {
			hostCooperativeYield()
		}
	}
}
