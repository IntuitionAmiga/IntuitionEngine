//go:build js && wasm

package main

import (
	"sync"
	"syscall/js"
)

// IE32 wasm blocks are immutable functions of their emitted bytes. Retain a
// bounded set of compiled modules so dispatching a hot block does not repeat
// browser validation and compilation on every entry.
const ie32WasmModuleCacheLimit = 256

type ie32WasmCachedBlock struct {
	fn            js.Value
	cached        ie32NativeCachedBlock
	countedPlan   *ie32CountedLoopPlan
	countedCount  uint32
	countedStatic bool
}

var ie32WasmModules = struct {
	sync.Mutex
	modules   map[string]js.Value
	blocks    map[string]js.Value
	wasm      js.Value
	u8ctor    js.Value
	imports   js.Value
	runtimeOK bool
	execCache map[*CPU]map[uint32]ie32WasmCachedBlock
	hits      uint64
}{modules: make(map[string]js.Value), blocks: make(map[string]js.Value), execCache: make(map[*CPU]map[uint32]ie32WasmCachedBlock)}

func ie32WasmCachedEntry(cpu *CPU) (ie32WasmCachedBlock, bool) {
	ie32WasmModules.Lock()
	defer ie32WasmModules.Unlock()
	entry, ok := ie32WasmModules.execCache[cpu][cpu.PC]
	return entry, ok
}

func ie32RememberWasmCachedEntry(cpu *CPU, block []ie32DecodedInstruction, retired uint64, fn js.Value, plan *ie32CountedLoopPlan) {
	if cpu == nil || len(block) == 0 || !fn.Truthy() || retired == 0 {
		return
	}
	// The wasm execution cache has the same source-stamp-only validation as
	// native retained blocks. Do not retain an operation whose admission also
	// depended on mutable guest register or RAM state.
	if plan == nil && !ie32CacheableNativeBlock(block, int(retired)) {
		return
	}
	ranges := ie32DecodedBlockSourceRanges(block, len(block))
	entry := ie32WasmCachedBlock{fn: fn, cached: ie32NativeCachedBlock{pc: block[0].PC, retired: retired, stamp: ie32DecodedBlockSourceStamp(cpu.memory, block, len(block)), sourceRanges: ranges}, countedPlan: plan}
	if plan != nil {
		for i := plan.head - 1; i >= 0; i-- {
			in := block[i]
			if dst, ok := ie32ImmediateLoadDestination(in); ok && dst == plan.counter && in.AddrMode == ADDR_IMMEDIATE {
				entry.countedCount = in.Operand
				entry.countedStatic = true
				break
			}
		}
		if !entry.countedStatic {
			return
		}
	}
	ie32WasmModules.Lock()
	defer ie32WasmModules.Unlock()
	if len(ie32WasmModules.execCache) >= ie32WasmModuleCacheLimit {
		clear(ie32WasmModules.execCache)
	}
	if ie32WasmModules.execCache[cpu] == nil {
		ie32WasmModules.execCache[cpu] = make(map[uint32]ie32WasmCachedBlock)
	}
	ie32WasmModules.execCache[cpu][entry.cached.pc] = entry
}

func ie32DropWasmCachedEntry(cpu *CPU, pc uint32) {
	ie32WasmModules.Lock()
	defer ie32WasmModules.Unlock()
	delete(ie32WasmModules.execCache[cpu], pc)
}

// ie32WasmRuntimeObjects returns the immutable JavaScript objects shared by
// every IE32 dispatch. Go's exported WebAssembly.Memory can grow but retains
// its identity, so this import remains valid for the wasm runtime lifetime.
func ie32WasmRuntimeObjects() (wasm, u8ctor, imports js.Value, ok bool) {
	ie32WasmModules.Lock()
	defer ie32WasmModules.Unlock()
	if ie32WasmModules.runtimeOK {
		return ie32WasmModules.wasm, ie32WasmModules.u8ctor, ie32WasmModules.imports, true
	}
	mem := js.Global().Get("__goMem")
	wasm = js.Global().Get("WebAssembly")
	u8ctor = js.Global().Get("Uint8Array")
	if !mem.Truthy() || !wasm.Truthy() || !u8ctor.Truthy() {
		return js.Undefined(), js.Undefined(), js.Undefined(), false
	}
	env := js.Global().Get("Object").New()
	env.Set("mem", mem)
	imports = js.Global().Get("Object").New()
	imports.Set("env", env)
	ie32WasmModules.wasm = wasm
	ie32WasmModules.u8ctor = u8ctor
	ie32WasmModules.imports = imports
	ie32WasmModules.runtimeOK = true
	return wasm, u8ctor, imports, true
}

func ie32WasmModuleForBytes(wasm, u8ctor js.Value, bytes []byte) (js.Value, bool) {
	key := string(bytes)
	ie32WasmModules.Lock()
	defer ie32WasmModules.Unlock()
	if module, ok := ie32WasmModules.modules[key]; ok {
		ie32WasmModules.hits++
		return module, true
	}
	if len(ie32WasmModules.modules) >= ie32WasmModuleCacheLimit {
		clear(ie32WasmModules.modules)
		clear(ie32WasmModules.blocks)
	}
	u8 := u8ctor.New(len(bytes))
	js.CopyBytesToJS(u8, bytes)
	module := wasm.Get("Module").New(u8)
	ie32WasmModules.modules[key] = module
	return module, false
}

// ie32WasmBlockForBytes returns the reusable exported entrypoint for immutable
// emitted bytes.  All IE32 wasm blocks import the one Go linear memory and
// receive the CPU address as an argument, so one instantiated module is safe
// for every CPU and avoids per-dispatch WebAssembly.Instance construction.
func ie32WasmBlockForBytes(wasm, u8ctor, imports js.Value, bytes []byte) (js.Value, bool) {
	key := string(bytes)
	ie32WasmModules.Lock()
	defer ie32WasmModules.Unlock()
	if block, ok := ie32WasmModules.blocks[key]; ok {
		ie32WasmModules.hits++
		return block, true
	}
	module, ok := ie32WasmModules.modules[key]
	if !ok {
		if len(ie32WasmModules.modules) >= ie32WasmModuleCacheLimit {
			clear(ie32WasmModules.modules)
			clear(ie32WasmModules.blocks)
		}
		u8 := u8ctor.New(len(bytes))
		js.CopyBytesToJS(u8, bytes)
		module = wasm.Get("Module").New(u8)
		ie32WasmModules.modules[key] = module
	}
	block := wasm.Get("Instance").New(module, imports).Get("exports").Get("block")
	ie32WasmModules.blocks[key] = block
	return block, false
}

func ie32WasmModuleCacheHitCount() uint64 {
	ie32WasmModules.Lock()
	defer ie32WasmModules.Unlock()
	return ie32WasmModules.hits
}

func ie32ResetWasmModuleCacheForTest() {
	ie32WasmModules.Lock()
	defer ie32WasmModules.Unlock()
	clear(ie32WasmModules.modules)
	clear(ie32WasmModules.blocks)
	clear(ie32WasmModules.execCache)
	ie32WasmModules.hits = 0
}
