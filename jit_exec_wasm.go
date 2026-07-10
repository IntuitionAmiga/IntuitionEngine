//go:build js && wasm

// jit_exec_wasm.go - dispatcher loop for the IE64 wasm JIT backend.
//
// jitExecute on js/wasm interleaves the interpreter (StepOne) with compiled
// wasm blocks from jit_wasm_runtime.go. The loop mirrors the outer controls
// of CPU64.Execute: debugger break-in, trap-halt, periodic stop polls and
// the cooperative yield that hands the single wasm thread back to the JS
// event loop (which is also when compile promises resolve and blocks
// install).

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

// wasmJITSupported marks the presence of the IE64 wasm bytecode backend for
// shared wiring; the counterpart in jit_wasm_supported_other.go is false.
const wasmJITSupported = true

// jitExecute runs the wasm JIT dispatcher when the backend is enabled,
// otherwise the plain interpreter. Interpreter path when: -nojit was given
// (cpu.jitEnabled false, same field the native dispatcher honours), the
// IE64_WASM_JIT=0 kill switch is set, or the hosting page has not exposed
// the module memory.
func (cpu *CPU64) jitExecute() {
	if !cpu.jitEnabled || !wasmJITEnabled() {
		cpu.Execute()
		return
	}
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		cpu.Execute()
		return
	}
	cpu.wasmJITDispatch(rt)
}

// freeJIT: compiled blocks are ordinary JS-managed wasm instances; dropping
// the runtime is enough, nothing to unmap.
func (cpu *CPU64) freeJIT() {}

func (cpu *CPU64) wasmJITDispatch(rt *wasmJITRuntime) {
	// Fresh accounting per run, like Execute and ExecuteJIT.
	cpu.InstructionCount = 0
	checkCounter := uint32(0)
	for {
		if cpu.debugHandleBreakIn(cpu.PC) {
			return
		}
		if cpu.trapHalted {
			return
		}
		checkCounter++
		if checkCounter&0xFFF == 0 {
			if !cpu.running.Load() {
				return
			}
			// Parks the goroutine briefly: the JS event loop renders, takes
			// input, and resolves pending WebAssembly.instantiate promises,
			// which installs freshly compiled blocks.
			hostCooperativeYield()
		}

		// Interrupt delivery at the instruction boundary, before compiled
		// block entry or interpreter fetch: same priority rule as Execute's
		// loop and the native JIT dispatcher. Delivering here (not inside
		// StepOne) also keeps the accounting honest: a delivery consumes no
		// retired instruction. A delivered interrupt redirects PC, so
		// re-dispatch.
		if cpu.deliverPendingExternalInterrupt() {
			if !cpu.running.Load() {
				return
			}
			continue
		}

		// Compiled block at this PC? (peek refuses while the MMU is on.)
		if blk := rt.peek(cpu.PC); blk != nil {
			rt.runBlock(blk)
			continue
		}
		rt.noteHot(cpu.PC)

		if cpu.StepOne() == 0 {
			return
		}
		// No interrupt is pending here (delivered above), so StepOne retired
		// exactly one instruction (interpreter fallback for cold, unsupported
		// or MMU-on execution); account for it, matching the compiled path's
		// RetCount accounting.
		cpu.InstructionCount++
		if !cpu.running.Load() {
			return
		}
	}
}
