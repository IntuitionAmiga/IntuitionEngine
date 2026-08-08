//go:build js && wasm

package main

import (
	"syscall/js"
	"testing"
)

func TestIE32WasmModuleCacheReusesCompiledModule(t *testing.T) {
	bytes, err := compileIE32WasmBlock([]ie32DecodedInstruction{
		{PC: PROG_START, Opcode: LDA, AddrMode: ADDR_IMMEDIATE, Operand: 0x1234},
	})
	if err != nil {
		t.Fatalf("compile block: %v", err)
	}
	wasm := js.Global().Get("WebAssembly")
	u8ctor := js.Global().Get("Uint8Array")
	ie32ResetWasmModuleCacheForTest()
	first, hit := ie32WasmModuleForBytes(wasm, u8ctor, bytes)
	if hit {
		t.Fatal("first module lookup was a cache hit")
	}
	second, hit := ie32WasmModuleForBytes(wasm, u8ctor, bytes)
	if !hit {
		t.Fatal("second identical module lookup missed cache")
	}
	if !first.Equal(second) {
		t.Fatal("cache returned a different compiled WebAssembly.Module")
	}
	if got := ie32WasmModuleCacheHitCount(); got != 1 {
		t.Fatalf("module cache hits=%d, want 1", got)
	}
}

func TestIE32WasmModuleCacheReusesInstantiatedBlock(t *testing.T) {
	bytes, err := compileIE32WasmBlock([]ie32DecodedInstruction{
		{PC: PROG_START, Opcode: LDA, AddrMode: ADDR_IMMEDIATE, Operand: 0x1234},
	})
	if err != nil {
		t.Fatalf("compile block: %v", err)
	}
	wasm := js.Global().Get("WebAssembly")
	u8ctor := js.Global().Get("Uint8Array")
	env := js.Global().Get("Object").New()
	env.Set("mem", js.Global().Get("__goMem"))
	imports := js.Global().Get("Object").New()
	imports.Set("env", env)
	ie32ResetWasmModuleCacheForTest()
	first, hit := ie32WasmBlockForBytes(wasm, u8ctor, imports, bytes)
	if hit || !first.Truthy() {
		t.Fatal("first instantiated block lookup did not create an entrypoint")
	}
	second, hit := ie32WasmBlockForBytes(wasm, u8ctor, imports, bytes)
	if !hit || !first.Equal(second) {
		t.Fatal("second instantiated block lookup did not reuse the entrypoint")
	}
	if got := ie32WasmModuleCacheHitCount(); got != 1 {
		t.Fatalf("instantiated block cache hits=%d, want 1", got)
	}
}

func TestIE32WasmRuntimeObjectsAreStable(t *testing.T) {
	wasm, u8ctor, imports, ok := ie32WasmRuntimeObjects()
	if !ok || !wasm.Truthy() || !u8ctor.Truthy() || !imports.Truthy() {
		t.Fatal("IE32 wasm runtime objects unavailable")
	}
	nextWasm, nextU8, nextImports, ok := ie32WasmRuntimeObjects()
	if !ok || !wasm.Equal(nextWasm) || !u8ctor.Equal(nextU8) || !imports.Equal(nextImports) {
		t.Fatal("IE32 wasm runtime objects were not reused")
	}
}

func TestIE32WasmCPUDisposeDropsExecutionCacheEntries(t *testing.T) {
	ie32ResetWasmModuleCacheForTest()
	cpu := NewCPU(NewMachineBus())
	ie32WasmModules.Lock()
	ie32WasmModules.execCache[cpu] = map[uint32]ie32WasmCachedBlock{PROG_START: {}}
	ie32WasmModules.Unlock()
	cpu.Dispose()
	ie32WasmModules.Lock()
	_, retained := ie32WasmModules.execCache[cpu]
	ie32WasmModules.Unlock()
	if retained {
		t.Fatal("disposing IE32 CPU retained wasm execution-cache entries")
	}
}
