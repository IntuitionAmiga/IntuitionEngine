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

import (
	"fmt"
	"syscall/js"
)

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
		if checkCounter&0x3F == 0 {
			if !cpu.running.Load() {
				return
			}
			// Parks the goroutine briefly: the JS event loop renders, takes
			// input, and resolves pending WebAssembly.instantiate promises,
			// which installs freshly compiled blocks. Every 64 iterations,
			// NOT 4096: a dispatcher iteration on the compiled path is an
			// entire chained run (up to 256 blocks), so a 4096-iteration
			// cadence starved the browser of frames and keyboard input the
			// moment the JIT tiered up. hostCooperativeYield self-throttles
			// by wall time, so the tighter check costs one time read.
			hostCooperativeYield()
			if rt.diag {
				js.Global().Set("__ieJITDiag", fmt.Sprintf(
					"pc=%#x gen=%d compiles=%d blockRuns=%d chainRuns=%d ic=%d blacklist=%d blocks=%d fall=%d smcNoDrop=%d enq=%d claimFail=%d genDrop=%d flushes=%d rangeDrops=%d slot=%d inflight=%d hot=%d pollRuns=%d helpers=%v",
					cpu.PC, rt.gen, rt.compiles, rt.blockRuns, rt.chainRuns,
					cpu.InstructionCount, len(rt.blacklist), len(rt.blocks),
					rt.fallSteps, rt.smcNoDrop, rt.enqueues, rt.claimFails,
					rt.genDrops, rt.flushes, rt.rangeDrops, rt.nextSlot,
					len(rt.inFlight), len(rt.hot), rt.pollRuns, rt.helperCnt)+
					fmt.Sprintf(" smcAddrs=%#x", rt.smcAddrRing))
			}
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

		// A sustained streak of LOAD helper exits at one PC is a guest
		// spinning on an MMIO register. Hand the loop to the parking poll
		// service: on this single-threaded build the polled bit can only
		// advance while the CPU goroutine is parked, so spinning through
		// compiled blocks burnt whole yield slices per observable edge
		// (WAIT-VSYNC demos ran at single-digit frame rates). The streak
		// leaves PC just past the serviced load, so re-enter at the loop
		// head; re-reading a polled status register is what the loop does
		// anyway.
		if rt.loadStreak >= wasmPollStreakThreshold && cpu.PC == rt.lastLoadPC+IE64_INSTR_SIZE {
			rt.loadStreak = 0
			pollPC := rt.lastLoadPC
			if matched, retired := cpu.wasmRunMMIOPollLoop(pollPC); matched {
				rt.pollRuns++
				// The LOAD that armed the service already retired through
				// the helper path and was counted by runBlock; the service
				// re-enters at the loop head, so its first iteration
				// re-executes that LOAD. Discount one instruction per
				// engagement that ran at least one iteration, keeping the
				// count at one per architecturally retired instruction. On
				// zero-iteration exits (stop or pending interrupt before
				// the first re-read) retired is 0 and the rewound LOAD is
				// counted by whoever executes it next.
				if retired > 0 {
					retired--
				}
				cpu.InstructionCount += uint64(retired)
				if rt.diag && rt.pollRuns&0x3FF == 1 {
					wasmConsoleLog(fmt.Sprintf("IE64 wasm JIT diag: poll service #%d pc=%#x retired=%d exitPC=%#x", rt.pollRuns, pollPC, retired, cpu.PC))
				}
				continue
			}
		}

		// Compiled block at this PC? (peek refuses while the MMU is on.)
		if blk := rt.peek(cpu.PC); blk != nil {
			rt.runBlock(blk)
			continue
		}
		rt.noteHot(cpu.PC)

		rt.fallSteps++
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
