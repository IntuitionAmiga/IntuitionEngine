//go:build js && wasm

package main

// ie32DropWasmCPUEntries releases JS entrypoints that are keyed by a CPU no
// longer reachable from the emulator. It is called only after that CPU stops.
func ie32DropWasmCPUEntries(cpu *CPU) {
	if cpu == nil {
		return
	}
	ie32WasmModules.Lock()
	delete(ie32WasmModules.execCache, cpu)
	ie32WasmModules.Unlock()
}
