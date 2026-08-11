//go:build js && wasm

// jit_wasm_runtime_js_test.go - node-run integration tests for the IE64 wasm
// JIT backend (make test-wasm-node). The repo-local runner
// (tools/wasm/wasm_exec_node_ie.js) exposes the module memory as __goMem,
// exactly like the demo page, so real blocks compile and execute under V8.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// wasmNodeProgram builds a counting loop hot enough to tier up mid-run:
//
//	1000: MOVE  R1, #0
//	1008: MOVEQ R2, #iters
//	1010: ADD   R1, R1, #3
//	1018: SUB   R2, R2, #1
//	1020: BNE   R2, R0, -0x10   ; back to 1010
//	1028: HALT
func wasmNodeProgram(iters uint32) []byte {
	return bytes.Join([][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_MOVEQ, 2, 0, 0, 0, 0, iters),
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 3),
		ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1),
		ie64Instr(OP_BNE, 0, 0, 0, 2, 0, 0xFFFFFFF0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	}, nil)
}

func newWasmNodeMachine(t testing.TB, program []byte) *CPU64 {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	copy(cpu.memory[PROG_START:], program)
	cpu.PC = PROG_START
	cpu.running.Store(true)
	return cpu
}

func waitForInstall(rt *wasmJITRuntime, pc uint64) bool {
	for i := 0; i < 400; i++ {
		if _, ok := rt.blocks[pc]; ok {
			return true
		}
		// Parks the goroutine: the node event loop turns and pending
		// WebAssembly.instantiate promises resolve.
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestWasmJIT_Node_EndToEndParity(t *testing.T) {
	const iters = 2_000_000
	program := wasmNodeProgram(iters)

	// Reference: pure interpreter.
	ref := newWasmNodeMachine(t, program)
	t0 := time.Now()
	ref.Execute()
	interpDur := time.Since(t0)

	// JIT dispatcher.
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable: __goMem not exposed by the node runner")
	}
	t0 = time.Now()
	cpu.wasmJITDispatch(rt)
	jitDur := time.Since(t0)

	if cpu.regs[1] != ref.regs[1] || cpu.regs[2] != ref.regs[2] {
		t.Errorf("state diverged: JIT R1/R2 = %d/%d, interpreter %d/%d",
			cpu.regs[1], cpu.regs[2], ref.regs[1], ref.regs[2])
	}
	if want := uint64(iters) * 3; cpu.regs[1] != want {
		t.Errorf("R1 = %d, want %d", cpu.regs[1], want)
	}
	if rt.compiles == 0 {
		t.Error("no blocks compiled during a 2M-iteration hot loop")
	}
	if rt.blockRuns == 0 {
		t.Error("no compiled block ever executed")
	}
	if rt.chainRuns == 0 {
		t.Error("chain driver never engaged (block entries all went through Invoke)")
	}
	stats := cpu.jitStats.snapshot()
	if stats.CompiledBlocks == 0 || stats.NativeEntries == 0 || stats.NativeRetired == 0 {
		t.Fatalf("IES statistics = {blocks:%d entries:%d retired:%d}, want WebAssembly execution provenance",
			stats.CompiledBlocks, stats.NativeEntries, stats.NativeRetired)
	}
	if got := ie64JITBackend(); got != "wasm" {
		t.Fatalf("IES backend = %q, want wasm", got)
	}
	t.Logf("compiles=%d blockRuns=%d chainRuns=%d interp=%v jit=%v speedup=%.2fx",
		rt.compiles, rt.blockRuns, rt.chainRuns, interpDur, jitDur,
		float64(interpDur)/float64(jitDur))
}

func TestWasmJIT_Node_StaticJumpChase(t *testing.T) {
	program := make([]byte, 0x28)
	copy(program[0x00:], ie64Instr(OP_JMP, 0, 0, 0, 0, 0, uint32(PROG_START+0x10)))
	copy(program[0x10:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 0x10))
	copy(program[0x20:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	cpu.wasmJITDispatch(rt)

	if cpu.PC != PROG_START+0x20 {
		t.Fatalf("PC = %#x, want %#x", cpu.PC, uint64(PROG_START+0x20))
	}
	if cpu.InstructionCount != 3 {
		t.Fatalf("InstructionCount = %d, want 3", cpu.InstructionCount)
	}
	if rt.fallSteps != 1 {
		t.Fatalf("interpreter fallback steps = %d, want only the landing HALT", rt.fallSteps)
	}
	if rt.hot[PROG_START] != 0 || rt.hot[PROG_START+0x10] != 0 || rt.enqueues != 0 {
		t.Fatalf("static jumps entered tiering: hot=%v enqueues=%d", rt.hot, rt.enqueues)
	}
}

func TestWasmJIT_Node_ForwardRegionParity(t *testing.T) {
	program := make([]byte, 0x148)
	copy(program[0x00:], ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 3))
	copy(program[0x08:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 0x118))
	copy(program[0x120:], ie64Instr(OP_EOR, 1, IE64_SIZE_Q, 1, 1, 0, 5))
	copy(program[0x128:], ie64Instr(OP_JMP, 0, 0, 0, 0, 0, uint32(PROG_START+0x140)))
	copy(program[0x140:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	ref := newWasmNodeMachine(t, program)
	ref.regs[1] = 7
	for ref.running.Load() {
		if ref.StepOne() == 0 {
			break
		}
	}

	cpu := newWasmNodeMachine(t, program)
	cpu.regs[1] = 7
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	rt.enqueueCompile(PROG_START)
	if !waitForInstall(rt, PROG_START) {
		t.Fatal("forward region did not install")
	}
	blk := rt.blocks[PROG_START]
	if blk.endPC != PROG_START+0x130 {
		t.Fatalf("region endPC = %#x, want %#x", blk.endPC, uint64(PROG_START+0x130))
	}
	rt.runBlock(blk)
	if cpu.regs[1] != ref.regs[1] {
		t.Fatalf("region R1 = %#x, interpreter %#x", cpu.regs[1], ref.regs[1])
	}
	if cpu.PC != PROG_START+0x140 || cpu.InstructionCount != 4 {
		t.Fatalf("region exit PC/count = %#x/%d, want %#x/4", cpu.PC, cpu.InstructionCount, uint64(PROG_START+0x140))
	}
}

func TestWasmJIT_Node_ObservedConditionalPromotionReusesSlot(t *testing.T) {
	program := make([]byte, 0x218)
	copy(program[0x000:], ie64Instr(OP_BEQ, 0, 0, 0, 3, 4, 0x200))
	copy(program[0x008:], ie64Instr(OP_JMP, 0, 0, 0, 0, 0, uint32(PROG_START+0x210)))
	copy(program[0x200:], ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 1))
	copy(program[0x208:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, ^uint32(0x207)))
	copy(program[0x210:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	rt.enqueueCompile(PROG_START)
	rt.enqueueCompile(PROG_START + 0x200)
	if !waitForInstall(rt, PROG_START) || !waitForInstall(rt, PROG_START+0x200) {
		t.Fatal("recording blocks did not install")
	}
	entry := rt.blocks[PROG_START]
	if !entry.observedTrigger {
		t.Fatal("external conditional was not marked for observed promotion")
	}
	oldSlot := entry.slot
	cpu.regs[3], cpu.regs[4] = 7, 7
	for entry.execs < wasmObservedPromotionThreshold {
		cpu.PC = PROG_START
		rt.runBlock(entry)
	}
	if !rt.observed.active || cpu.PC != PROG_START+0x200 {
		t.Fatalf("threshold did not seed recording: active=%v PC=%#x", rt.observed.active, cpu.PC)
	}
	rt.runBlock(rt.blocks[PROG_START+0x200])
	for i := 0; i < 400 && rt.blocks[PROG_START] == entry; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	promoted := rt.blocks[PROG_START]
	if promoted == entry {
		t.Fatal("observed conditional region did not replace entry")
	}
	if promoted.slot != oldSlot {
		t.Fatalf("promotion allocated slot %d, want reused %d", promoted.slot, oldSlot)
	}
	cpu.regs[3], cpu.regs[4], cpu.PC = 1, 2, PROG_START
	rt.runBlock(promoted)
	if cpu.PC != PROG_START+8 {
		t.Fatalf("cold side exit PC=%#x, want %#x", cpu.PC, uint64(PROG_START+8))
	}
}

func TestWasmJIT_Node_ObservedIndirectHitAndMiss(t *testing.T) {
	program := make([]byte, 0x210)
	copy(program[0x000:], ie64Instr(OP_JMP, 0, 0, 0, 3, 0, ^uint32(7)))
	copy(program[0x200:], ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 1))
	copy(program[0x208:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, ^uint32(0x207)))
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	rt.enqueueCompile(PROG_START)
	if !waitForInstall(rt, PROG_START) {
		t.Fatal("entry did not install")
	}
	old := rt.blocks[PROG_START]
	blocks := []wasmRegionBlock{
		{pc: PROG_START, instrs: []JITInstr{{opcode: OP_JMP, rs: 3, imm32: ^uint32(7)}}, kind: ie64ObservedIndirectJMP, hotTarget: PROG_START + 0x200, predictedTarget: PROG_START + 0x200},
		{pc: PROG_START + 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x207)}}},
	}
	rt.enqueuePromotion(PROG_START, old, blocks, rt.gen)
	for i := 0; i < 400 && rt.blocks[PROG_START] == old; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	promoted := rt.blocks[PROG_START]
	if promoted == old {
		t.Fatal("indirect observed region did not install")
	}
	cpu.PC, cpu.regs[2], cpu.regs[3] = PROG_START, 1, PROG_START+0x208
	rt.runBlock(promoted)
	if cpu.PC != PROG_START || cpu.regs[1] == 0 {
		t.Fatalf("hit PC=%#x R1=%d", cpu.PC, cpu.regs[1])
	}
	cpu.PC, cpu.regs[3] = PROG_START, 0x1_0000_0010
	rt.runBlock(promoted)
	if cpu.PC != 0x1_0000_0008 {
		t.Fatalf("mismatch PC=%#x", cpu.PC)
	}
}

func TestWasmJIT_Node_ForwardRegionRejectsSharedPageGap(t *testing.T) {
	program := make([]byte, 0x38)
	copy(program[0x00:], ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 3))
	copy(program[0x08:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 0x18))
	copy(program[0x20:], ie64Instr(OP_EOR, 1, IE64_SIZE_Q, 1, 1, 0, 5))
	copy(program[0x28:], ie64Instr(OP_JMP, 0, 0, 0, 0, 0, uint32(PROG_START+0x30)))

	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	rt.enqueueCompile(PROG_START)
	pending, ok := rt.inFlight[PROG_START]
	if !ok {
		t.Fatal("single-block fallback was not submitted")
	}
	if len(pending.ranges) != 1 || pending.endPC != PROG_START+0x10 {
		t.Fatalf("shared-page gap compiled as region: ranges=%v endPC=%#x", pending.ranges, pending.endPC)
	}
	page := PROG_START >> 8
	if cpu.jitCodePageSpans[page*2] != byte(PROG_START&0xff) || cpu.jitCodePageSpans[page*2+1] != byte((PROG_START+0x0f)&0xff) {
		t.Fatalf("single-block page span widened across gap: [%#x,%#x]", cpu.jitCodePageSpans[page*2], cpu.jitCodePageSpans[page*2+1])
	}
	if !waitForInstall(rt, PROG_START) {
		t.Fatal("single-block fallback did not install")
	}
}

func TestWasmJIT_Node_ForwardRegionMarksDisjointRanges(t *testing.T) {
	const targetOff = 0x20000
	program := make([]byte, targetOff+16)
	copy(program[0:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, targetOff))
	copy(program[targetOff:], ie64Instr(OP_JMP, 0, 0, 0, 0, 0, uint32(PROG_START+targetOff+8)))

	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	rt.enqueueCompile(PROG_START)
	middle := uint64(PROG_START + targetOff/2)
	if cpu.jitCodePageBitmap[middle>>8] != 0 {
		t.Fatal("forward-region gap was marked as compiled code")
	}
	if _, ok := rt.pageBlocks[middle>>8]; ok {
		t.Fatal("forward-region gap was added to the invalidation index")
	}
	gen := rt.gen
	rt.invalidateRange(middle, 8)
	if rt.gen != gen {
		t.Fatal("write in forward-region gap invalidated the pending compile")
	}
	if !waitForInstall(rt, PROG_START) {
		t.Fatal("gap write prevented the region from installing")
	}
	if cpu.jitCodePageBitmap[middle>>8] != 0 {
		t.Fatal("installed region marked its intervening gap")
	}
}

func TestWasmJIT_Node_TimerDisablesStaticJumpChase(t *testing.T) {
	program := bytes.Join([][]byte{
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 8),
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 8),
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 8),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	}, nil)
	cpu := newWasmNodeMachine(t, program)
	cpu.timerPeriod.Store(3)
	cpu.timerCount.Store(3)
	cpu.timerEnabled.Store(true)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	cpu.wasmJITDispatch(rt)

	if cpu.timerState.Load() != TIMER_EXPIRED {
		t.Fatalf("timer state = %d, want expired", cpu.timerState.Load())
	}
	if rt.fallSteps != 4 {
		t.Fatalf("interpreter fallback steps = %d, want all four instructions", rt.fallSteps)
	}
	if rt.compiles != 0 || rt.blockRuns != 0 {
		t.Fatalf("timer-enabled execution entered JIT: compiles=%d runs=%d", rt.compiles, rt.blockRuns)
	}
}

func BenchmarkWasmJIT_StaticJumpDispatch(b *testing.B) {
	const jumps = 16
	program := make([]byte, (jumps+1)*IE64_INSTR_SIZE)
	for i := 0; i < jumps; i++ {
		pc := uint64(PROG_START + i*IE64_INSTR_SIZE)
		next := uint32(pc + IE64_INSTR_SIZE)
		copy(program[i*IE64_INSTR_SIZE:], ie64Instr(OP_JMP, 0, 0, 0, 0, 0, next))
	}
	copy(program[jumps*IE64_INSTR_SIZE:], ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))

	b.Run("interpreter-dispatch", func(b *testing.B) {
		cpu := newWasmNodeMachine(b, program)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cpu.PC = PROG_START
			for j := 0; j < jumps; j++ {
				cpu.StepOne()
			}
		}
	})
	b.Run("bounded-chase", func(b *testing.B) {
		cpu := newWasmNodeMachine(b, program)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pc, retired := ie64ChaseStaticJumpsMemory(PROG_START, cpu.memory)
			if pc != PROG_START+jumps*IE64_INSTR_SIZE || retired != jumps {
				b.Fatal("chase result changed")
			}
		}
	})
}

func TestWasmJIT_Node_MMUGateEnqueueHalf(t *testing.T) {
	program := wasmNodeProgram(10)
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	cpu.mmuEnabled = true
	for i := 0; i < 100; i++ {
		rt.noteHot(PROG_START)
	}
	if len(rt.inFlight) != 0 || len(rt.blocks) != 0 || rt.compiles != 0 {
		t.Errorf("MMU-on noteHot enqueued work: inFlight=%d blocks=%d compiles=%d",
			len(rt.inFlight), len(rt.blocks), rt.compiles)
	}
}

func TestWasmJIT_Node_MMUGateEntryHalf(t *testing.T) {
	program := wasmNodeProgram(10)
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	// Install a block for PROG_START while the MMU is off.
	for i := 0; i < wasmJITHotThreshold; i++ {
		rt.noteHot(PROG_START)
	}
	if !waitForInstall(rt, PROG_START) {
		t.Fatal("block never installed")
	}
	blk := rt.blocks[PROG_START]

	// Entry half: MMU on, the installed block must not be entered.
	cpu.mmuEnabled = true
	if rt.tryBlock(PROG_START) {
		t.Fatal("tryBlock entered a compiled block while the MMU is enabled")
	}
	if blk.execs != 0 {
		t.Fatalf("block body ran %d times under MMU", blk.execs)
	}

	// Positive control: MMU off, the same block runs.
	cpu.mmuEnabled = false
	cpu.PC = PROG_START
	if !rt.tryBlock(PROG_START) {
		t.Fatal("tryBlock refused with MMU off")
	}
	if blk.execs != 1 {
		t.Fatalf("block execs = %d, want 1", blk.execs)
	}
}

func TestWasmJIT_Node_StaleCompileNotInstalled(t *testing.T) {
	// An SMC invalidation while a compile is in flight must prevent the
	// install: the module was built from bytes that may have changed. The
	// runtime is single-threaded, so the promise cannot resolve between the
	// enqueue and the invalidation below.
	program := wasmNodeProgram(10)
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	rt.enqueueCompile(PROG_START)
	if _, ok := rt.inFlight[PROG_START]; !ok {
		t.Fatal("compile not submitted")
	}
	rt.invalidateAll()
	if waitForInstall(rt, PROG_START) {
		t.Fatal("stale compile installed after invalidation")
	}
	if rt.compiles != 0 || len(rt.blocks) != 0 {
		t.Fatalf("stale install leaked: compiles=%d blocks=%d", rt.compiles, len(rt.blocks))
	}
	// The PC is not blacklisted and not stuck in flight: it can re-tier.
	if _, ok := rt.inFlight[PROG_START]; ok || rt.blacklist[PROG_START] {
		t.Fatal("PC cannot re-tier after a dropped stale compile")
	}
}

func TestWasmJIT_Node_StaleInFlightRangeInvalidation(t *testing.T) {
	// A store overlapping a compile still in flight must stop that compile
	// installing even when no installed block overlaps the store: the module
	// was built from the pre-store bytes. The pending range's pages are
	// marked at enqueue so the generated-store probe fires for them, and
	// invalidateRange checks pending ranges alongside installed blocks.
	program := wasmNodeProgram(10)
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	rt.enqueueCompile(PROG_START)
	if cpu.jitCodePageBitmap[PROG_START>>8] == 0 {
		t.Fatal("pending compile's pages not marked at enqueue")
	}
	rt.invalidateRange(PROG_START+8, 4) // overlaps only the pending range
	if waitForInstall(rt, PROG_START) {
		t.Fatal("stale in-flight compile installed after an overlapping write")
	}
	// The dropped compile's callback has resolved (waitForInstall parked the
	// goroutine); its pending-only page marks must be gone, or stores to
	// this page would take false-share exits for ever.
	if len(rt.inFlight) != 0 {
		t.Fatal("in-flight entry survived its callback")
	}
	if cpu.jitCodePageBitmap[PROG_START>>8] != 0 {
		t.Fatal("pending-only page mark leaked after the stale compile was dropped")
	}

	// Control: a same-page write past the pending range is a false share and
	// must not kill the innocent compile.
	cpu2 := newWasmNodeMachine(t, program)
	rt2 := newWasmJITRuntime(cpu2)
	if rt2 == nil {
		t.Fatal("runtime unavailable")
	}
	rt2.enqueueCompile(PROG_START)
	pending, ok := rt2.inFlight[PROG_START]
	if !ok {
		t.Fatal("compile not submitted")
	}
	rt2.invalidateRange(pending.endPC+0x40, 8)
	if !waitForInstall(rt2, PROG_START) {
		t.Fatal("false-share write killed an innocent in-flight compile")
	}
}

func TestWasmJIT_Node_SupersededCallbackLosesOwnership(t *testing.T) {
	// invalidateAll while a compile is pending, followed by a re-tier of the
	// same PC, leaves TWO callbacks outstanding for that PC. The older one
	// must not retire the newer pending entry or rebuild away its page
	// protection; ownership is decided by the token, not the PC.
	program := wasmNodeProgram(10)
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	rt.enqueueCompile(PROG_START)
	oldTok := rt.inFlight[PROG_START].token
	rt.invalidateAll() // orphans the first compile's callbacks
	rt.enqueueCompile(PROG_START)
	if rt.inFlight[PROG_START].token == oldTok {
		t.Fatal("superseding compile reused the ownership token")
	}

	// The orphaned callback's claim fails and leaves the newer entry and its
	// enqueue-time page marks untouched.
	if rt.claimInFlight(PROG_START, oldTok) {
		t.Fatal("stale callback claimed a superseded in-flight entry")
	}
	if _, ok := rt.inFlight[PROG_START]; !ok {
		t.Fatal("stale claim removed the newer pending entry")
	}
	if cpu.jitCodePageBitmap[PROG_START>>8] == 0 {
		t.Fatal("newer pending range lost its page mark")
	}

	// The newer compile installs; both real callbacks have resolved by then
	// (the orphan as a no-op), leaving exactly one block and a clean queue.
	if !waitForInstall(rt, PROG_START) {
		t.Fatal("superseding compile failed to install")
	}
	if len(rt.blocks) != 1 || len(rt.inFlight) != 0 {
		t.Fatalf("post-resolution state wrong: blocks=%d inFlight=%d",
			len(rt.blocks), len(rt.inFlight))
	}
}

func TestWasmJIT_Node_RangeInvalidation(t *testing.T) {
	// invalidateRange drops only blocks overlapping the written bytes. A
	// false share (data in the same 256-byte page as compiled code, which
	// the EhBASIC image contains) must keep every block; the old full flush
	// here caused a permanent recompile storm under RUN AOT.
	program := wasmNodeProgram(10)
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	for i := 0; i < wasmJITHotThreshold; i++ {
		rt.noteHot(PROG_START)
	}
	if !waitForInstall(rt, PROG_START) {
		t.Fatal("block never installed")
	}
	blk := rt.blocks[PROG_START]
	genBefore := rt.gen

	// False share: same page, past the compiled range.
	rt.invalidateRange(blk.endPC+0x40, 8)
	if _, ok := rt.blocks[PROG_START]; !ok {
		t.Fatal("false-share invalidation dropped a non-overlapping block")
	}
	if rt.gen != genBefore {
		t.Fatalf("false-share invalidation bumped the generation: %d -> %d", genBefore, rt.gen)
	}
	if cpu.jitCodePageBitmap[PROG_START>>8] == 0 {
		t.Fatal("false-share invalidation unmarked a live code page")
	}

	// Real overlap: the block goes, the generation moves, the page unmarks.
	rt.invalidateRange(PROG_START+8, 4)
	if _, ok := rt.blocks[PROG_START]; ok {
		t.Fatal("overlapping invalidation kept the block")
	}
	if rt.gen == genBefore {
		t.Fatal("overlapping invalidation did not bump the generation")
	}
	if cpu.jitCodePageBitmap[PROG_START>>8] != 0 {
		t.Fatal("code-page bitmap not rebuilt after the drop")
	}

	// Degraded report (size 0): full flush.
	rt.invalidateRange(0, 0)
	if len(rt.blocks) != 0 {
		t.Fatal("size-0 invalidation must flush everything")
	}
}

func TestWasmJIT_Node_FalseShareStoreLoop(t *testing.T) {
	// End-to-end: a hot loop whose STORE writes data in the same 256-byte
	// page as its own code. Every store trips the probe; none may flush the
	// block cache, and the loop must finish with the JIT engaged.
	//
	//	1000: MOVE  R3, #0x10C0    ; data slot, same page as the code
	//	1008: MOVE  R1, #0
	//	1010: MOVEQ R2, #iters
	//	1018: ADD   R1, R1, #3
	//	1020: STORE.Q R1, (R3)     ; false-share SMC hit once pages mark
	//	1028: SUB   R2, R2, #1
	//	1030: BNE   R2, R0, -0x18  ; back to 1018
	//	1038: HALT
	// Leave enough wall time for asynchronous WebAssembly.instantiate to
	// resolve even on fast hosts. The assertion is specifically end-to-end
	// JIT engagement, so a loop that can finish in one yield slice is not a
	// meaningful workload for this test.
	const iters = 200_000
	program := bytes.Join([][]byte{
		ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, PROG_START+0xC0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_MOVEQ, 2, 0, 0, 0, 0, iters),
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 3),
		ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 3, 0, 0),
		ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1),
		ie64Instr(OP_BNE, 0, 0, 0, 2, 0, 0xFFFFFFE8),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	}, nil)
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	cpu.wasmJITDispatch(rt)

	if want := uint64(iters) * 3; cpu.regs[1] != want {
		t.Fatalf("R1 = %d, want %d", cpu.regs[1], want)
	}
	if got := binary.LittleEndian.Uint64(cpu.memory[PROG_START+0xC0:]); got != uint64(iters)*3 {
		t.Fatalf("stored value = %d, want %d", got, uint64(iters)*3)
	}
	if rt.compiles == 0 || rt.blockRuns == 0 {
		t.Fatalf("JIT never engaged: compiles=%d blockRuns=%d", rt.compiles, rt.blockRuns)
	}
	if rt.gen != 0 {
		t.Fatalf("false-share stores flushed the cache %d times", rt.gen)
	}
	if len(rt.blocks) == 0 {
		t.Fatal("no blocks survived the run")
	}
}

func TestWasmJIT_Node_NoJITFlagHonoured(t *testing.T) {
	// cpu.jitEnabled false (the -nojit path in main) must route jitExecute to
	// the plain interpreter: no runtime is created, observable via the
	// code-page bitmap newWasmJITRuntime would allocate.
	program := wasmNodeProgram(50_000)
	cpu := newWasmNodeMachine(t, program)
	cpu.jitEnabled = false
	cpu.jitExecute()
	if cpu.jitCodePageBitmap != nil {
		t.Fatal("jitExecute created a wasm JIT runtime despite jitEnabled=false")
	}
	if want := uint64(50_000) * 3; cpu.regs[1] != want {
		t.Fatalf("interpreter path wrong result: R1 = %d, want %d", cpu.regs[1], want)
	}

	// Positive control: jitEnabled true creates the runtime.
	cpu2 := newWasmNodeMachine(t, program)
	cpu2.jitEnabled = true
	cpu2.jitExecute()
	if cpu2.jitCodePageBitmap == nil {
		t.Fatal("jitExecute did not engage the wasm JIT with jitEnabled=true")
	}
}

func TestWasmJIT_Node_HotHALTStillTerminates(t *testing.T) {
	// A HALT PC driven hot must never compile into a self-re-entering block:
	// programmes that halt repeatedly (machine restarts) would spin forever.
	program := wasmNodeProgram(1000)
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	haltPC := uint64(PROG_START + 5*8)
	for i := 0; i < wasmJITHotThreshold*2; i++ {
		rt.noteHot(haltPC)
	}
	time.Sleep(50 * time.Millisecond) // let any (wrong) compile land
	if _, ok := rt.blocks[haltPC]; ok {
		t.Fatal("a block compiled at a HALT PC")
	}
	if !rt.blacklist[haltPC] {
		t.Fatal("HALT PC not blacklisted; would re-enqueue forever")
	}
	// End to end: the run must terminate.
	done := make(chan struct{})
	go func() {
		cpu.wasmJITDispatch(rt)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("dispatcher never terminated with a hot HALT PC")
	}
	if want := uint64(1000) * 3; cpu.regs[1] != want {
		t.Fatalf("R1 = %d, want %d", cpu.regs[1], want)
	}
}

func TestWasmJIT_Node_InterruptBeforeBlock(t *testing.T) {
	// A pending external interrupt at a compiled-block boundary must be
	// delivered BEFORE the block runs: the pushed return PC is the block's
	// start, not a post-block PC.
	handler := uint64(PROG_START + 0x100)
	program := bytes.Join([][]byte{
		ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	}, nil)
	cpu := newWasmNodeMachine(t, program)
	copy(cpu.memory[handler:], ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 0xBEEF))
	copy(cpu.memory[handler+8:], ie64Instr(OP_RTI64, 0, 0, 0, 0, 0, 0))
	cpu.interruptVector = handler
	cpu.regs[31] = STACK_START
	cpu.interruptEnabled.Store(true)

	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	for i := 0; i < wasmJITHotThreshold; i++ {
		rt.noteHot(PROG_START)
	}
	if !waitForInstall(rt, PROG_START) {
		t.Fatal("block never installed")
	}

	NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
	cpu.wasmJITDispatch(rt)

	if cpu.regs[10] != 0xBEEF {
		t.Fatalf("handler never ran: R10 = %#x", cpu.regs[10])
	}
	pushed := binary.LittleEndian.Uint64(cpu.memory[STACK_START-8:])
	if pushed != PROG_START {
		t.Fatalf("interrupt delivered after compiled code ran: pushed PC = %#x, want block start %#x",
			pushed, uint64(PROG_START))
	}
}

func TestWasmJIT_Node_InstructionAccounting(t *testing.T) {
	// Cold run (below the hot threshold end to end): every instruction goes
	// through StepOne and must still be counted, from a fresh zero.
	program := wasmNodeProgram(2) // 2 iterations: well under any tier-up
	cpu := newWasmNodeMachine(t, program)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable")
	}
	cpu.InstructionCount = 999999 // stale value from a previous run
	cpu.wasmJITDispatch(rt)
	// MOVE + MOVEQ + 2x(ADD+SUB+BNE) + HALT = 9 retired instructions.
	if cpu.InstructionCount != 9 {
		t.Fatalf("InstructionCount = %d, want 9", cpu.InstructionCount)
	}

	// Interrupt delivery consumes no retired instruction: NOP+NOP+HALT plus
	// a handler (MOVE+RTI) retires exactly 5, the delivery itself 0.
	handler := uint64(PROG_START + 0x100)
	icpu := newWasmNodeMachine(t, bytes.Join([][]byte{
		ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	}, nil))
	copy(icpu.memory[handler:], ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 0xBEEF))
	copy(icpu.memory[handler+8:], ie64Instr(OP_RTI64, 0, 0, 0, 0, 0, 0))
	icpu.interruptVector = handler
	icpu.regs[31] = STACK_START
	icpu.interruptEnabled.Store(true)
	irt := newWasmJITRuntime(icpu)
	NewIE64InterruptSink(icpu).Pulse(IntMaskBlitter)
	icpu.wasmJITDispatch(irt)
	if icpu.regs[10] != 0xBEEF {
		t.Fatalf("handler never ran: R10 = %#x", icpu.regs[10])
	}
	if icpu.InstructionCount != 5 {
		t.Fatalf("interrupt-only step counted: InstructionCount = %d, want 5", icpu.InstructionCount)
	}

	// Hot run: compiled-path accounting matches the interpreter's total.
	const iters = 500_000
	ref := newWasmNodeMachine(t, wasmNodeProgram(iters))
	refRT := newWasmJITRuntime(ref)
	ref.wasmJITDispatch(refRT)
	want := uint64(2 + 3*iters + 1) // MOVE+MOVEQ, loop body, HALT
	if ref.InstructionCount != want {
		t.Fatalf("hot-run InstructionCount = %d, want %d", ref.InstructionCount, want)
	}
	if refRT.compiles == 0 {
		t.Fatal("hot run never compiled (accounting test lost its JIT half)")
	}
}

func TestWasmJIT_Node_KillSwitch(t *testing.T) {
	if !wasmJITEnabled() {
		t.Fatal("backend should be enabled by default under the node runner")
	}
	os.Setenv("IE64_WASM_JIT", "0")
	defer os.Unsetenv("IE64_WASM_JIT")
	if wasmJITEnabled() {
		t.Fatal("IE64_WASM_JIT=0 did not disable the backend")
	}
}

// wasmNodeJSRProgram mirrors the shape that killed rotozoomer_ie64.ie64 in
// the browser: SP set high, a main loop that JSRs into a subroutine holding
// an MMIO read and a short spin, then RTS back. The subroutine tiers up hot
// while the main-loop JSR blocks stay cool, so compiled RTS blocks return
// into interpreted callers every iteration.
//
//	1000: LEA   SP, #0xDF000
//	1008: MOVEQ R2, #iters
//	1010: JSR   +0x30            ; -> 1040
//	1018: ADD   R1, R1, #3
//	1020: SUB   R2, R2, #1
//	1028: BNE   R2, R0, -0x18    ; back to 1010
//	1030: HALT
//	1038: HALT                   ; pad
//	1040: LEA   R3, #0xF0008
//	1048: LOAD.L R4, (R3)        ; MMIO read
//	1050: MOVEQ R6, #8
//	1058: SUB   R6, R6, #1
//	1060: BNE   R6, R0, -0x8
//	1068: RTS
func wasmNodeJSRProgram(iters uint32) []byte {
	return bytes.Join([][]byte{
		ie64Instr(OP_LEA, 31, IE64_SIZE_Q, 1, 0, 0, 0xDF000),
		ie64Instr(OP_MOVEQ, 2, 0, 0, 0, 0, iters),
		ie64Instr(OP_JSR64, 0, IE64_SIZE_Q, 0, 0, 0, 0x30),
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 3),
		ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1),
		ie64Instr(OP_BNE, 0, 0, 0, 2, 0, 0xFFFFFFE8),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_LEA, 3, IE64_SIZE_Q, 1, 0, 0, 0xF0008),
		ie64Instr(OP_LOAD, 4, IE64_SIZE_L, 0, 3, 0, 0),
		ie64Instr(OP_MOVEQ, 6, 0, 0, 0, 0, 8),
		ie64Instr(OP_SUB, 6, IE64_SIZE_Q, 1, 6, 0, 1),
		ie64Instr(OP_BNE, 0, 0, 0, 6, 0, 0xFFFFFFF8),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),
	}, nil)
}

func TestWasmJIT_Node_JSRRTSMainLoop(t *testing.T) {
	const iters = 300_000
	program := wasmNodeJSRProgram(iters)

	ref := newWasmNodeMachine(t, program)
	ref.Execute()

	cpu := newWasmNodeMachine(t, program)
	// Mark the polled page as IO so every LOAD in the subroutine takes the
	// helper exit, as it does on the real machine where 0xF0008 is a video
	// register. The test bus has no handler there, so the read still sees
	// plain RAM and the interpreter reference behaves identically.
	cpu.bus.ioPageBitmap[0xF0008>>8] = true
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable: __goMem not exposed by the node runner")
	}
	cpu.wasmJITDispatch(rt)

	if rt.compiles == 0 {
		t.Fatal("nothing compiled; test shape lost its hot loop")
	}
	if rt.helperCnt[HELPER_LOAD] == 0 {
		t.Fatal("no LOAD helper exits; the poll never took the MMIO path this test exists to exercise")
	}
	if cpu.regs[1] != ref.regs[1] {
		t.Errorf("R1 diverged: JIT %d, interpreter %d", cpu.regs[1], ref.regs[1])
	}
	if want := uint64(iters) * 3; cpu.regs[1] != want {
		t.Errorf("R1 = %d, want %d (main loop died early)", cpu.regs[1], want)
	}
	if cpu.regs[31] != ref.regs[31] || cpu.regs[31] != 0xDF000 {
		t.Errorf("SP drifted: JIT %#x, interpreter %#x, want 0xDF000", cpu.regs[31], ref.regs[31])
	}
	if cpu.PC != ref.PC {
		t.Errorf("PC diverged: JIT %#x, interpreter %#x", cpu.PC, ref.PC)
	}
	t.Logf("compiles=%d blockRuns=%d helpers=%v", rt.compiles, rt.blockRuns, rt.helperCnt)
}

// TestWasmJIT_Node_StackOnMMIOPage pins the interpreter's stack semantics
// for the JIT helper path: PUSH/POP/JSR/RTS are raw RAM accesses even when
// SP sits inside an MMIO-mapped page, exactly like mmuStackWrite/mmuStackRead
// in the interpreter. The real machine maps the Voodoo texture aperture over
// 0xD0000-0xDFFFF with handlers that read texture memory rather than bus
// RAM, and rotozoomer_ie64.ie64 parks its stack at 0xDF000 inside it; a
// helper that routes the pop through the bus reads zeros and the guest
// jumps to PC 0. Any future wasm backend for the other CPUs must hold the
// same invariant: stack traffic never fires MMIO callbacks.
func TestWasmJIT_Node_StackOnMMIOPage(t *testing.T) {
	const iters = 300_000
	program := wasmNodeJSRProgram(iters)

	newMachine := func() *CPU64 {
		bus := NewMachineBus()
		// A Voodoo-texture-aperture stand-in: reads see device memory
		// (zeros), not bus RAM, and writes are swallowed.
		bus.MapIO(0xD0000, 0xDFFFF,
			func(addr uint32) uint32 { return 0 },
			func(addr uint32, value uint32) {})
		cpu := NewCPU64(bus)
		copy(cpu.memory[PROG_START:], program)
		cpu.PC = PROG_START
		cpu.running.Store(true)
		return cpu
	}

	ref := newMachine()
	ref.Execute()

	cpu := newMachine()
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable: __goMem not exposed by the node runner")
	}
	// Watchdog: the historical failure mode jumps to PC 0 and can wedge the
	// dispatcher; fail fast instead of eating the whole suite timeout.
	watchdog := time.AfterFunc(60*time.Second, func() { cpu.running.Store(false) })
	cpu.wasmJITDispatch(rt)
	if !watchdog.Stop() {
		t.Fatal("watchdog fired: JIT run wedged (stack semantics regression)")
	}

	if rt.compiles == 0 {
		t.Fatal("nothing compiled; test shape lost its hot loop")
	}
	if cpu.regs[1] != ref.regs[1] {
		t.Errorf("R1 diverged: JIT %d, interpreter %d", cpu.regs[1], ref.regs[1])
	}
	if want := uint64(iters) * 3; cpu.regs[1] != want {
		t.Errorf("R1 = %d, want %d (main loop died early)", cpu.regs[1], want)
	}
	if cpu.regs[31] != ref.regs[31] || cpu.regs[31] != 0xDF000 {
		t.Errorf("SP diverged: JIT %#x, interpreter %#x, want 0xDF000", cpu.regs[31], ref.regs[31])
	}
	if cpu.PC != ref.PC {
		t.Errorf("PC diverged: JIT %#x, interpreter %#x", cpu.PC, ref.PC)
	}
	t.Logf("compiles=%d helpers=%v", rt.compiles, rt.helperCnt)
}

// TestWasmJIT_Node_MMIOPollParks pins the parking poll service: a guest
// spinning LOAD/AND/BEQ on an MMIO status register must be recognised after
// a streak of LOAD helper exits and then park between re-reads rather than
// burning its yield slice, because on the single-threaded wasm build the
// polled bit only advances while the CPU goroutine is parked. Without the
// service this shape ran WAIT-VSYNC demos at single-digit frame rates.
//
//	1000: LEA    R1, #0xF0008
//	1008: LOAD.L R2, (R1)
//	1010: AND.L  R2, R2, #2
//	1018: BEQ    R2, R0, -16   ; spin while the bit is clear
//	1020: HALT
func TestWasmJIT_Node_MMIOPollParks(t *testing.T) {
	program := bytes.Join([][]byte{
		ie64Instr(OP_LEA, 1, IE64_SIZE_Q, 1, 0, 0, 0xF0008),
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_AND64, 2, IE64_SIZE_L, 1, 2, 0, 2),
		ie64Instr(OP_BEQ, 0, 0, 0, 2, 0, 0xFFFFFFF0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	}, nil)

	// The device side: a mapped status register whose bit appears after
	// 300 ms, which only happens if the CPU goroutine parks often enough
	// for the timer goroutine to run.
	var status atomic.Uint32
	bus := NewMachineBus()
	bus.MapIO(0xF0008, 0xF000B,
		func(addr uint32) uint32 { return status.Load() },
		func(addr uint32, value uint32) {})
	cpu := NewCPU64(bus)
	copy(cpu.memory[PROG_START:], program)
	cpu.PC = PROG_START
	cpu.running.Store(true)
	rt := newWasmJITRuntime(cpu)
	if rt == nil {
		t.Fatal("runtime unavailable: __goMem not exposed by the node runner")
	}
	time.AfterFunc(300*time.Millisecond, func() { status.Store(2) })

	watchdog := time.AfterFunc(60*time.Second, func() { cpu.running.Store(false) })
	cpu.wasmJITDispatch(rt)
	if !watchdog.Stop() {
		t.Fatal("watchdog fired: poll loop never completed")
	}

	if cpu.regs[2] != 2 {
		t.Errorf("R2 = %d, want 2 (loop exited without observing the bit)", cpu.regs[2])
	}
	if rt.pollRuns == 0 {
		t.Error("parking poll service never engaged; the guest spun through compiled blocks instead")
	}
	if rt.helperCnt[HELPER_LOAD] > 50_000 {
		t.Errorf("LOAD helper exits = %d; a parked poll should re-read at park cadence, not spin", rt.helperCnt[HELPER_LOAD])
	}
	t.Logf("pollRuns=%d loadHelpers=%d compiles=%d", rt.pollRuns, rt.helperCnt[HELPER_LOAD], rt.compiles)
}
