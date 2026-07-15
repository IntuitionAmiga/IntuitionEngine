//go:build !js

// jit_wasm_ie64_diff_test.go - differential tests for the IE64 wasm JIT
// backend.
//
// Every test runs the same IE64 program twice: once through the real
// interpreter (StepOne on a CPU64 with a MachineBus) and once through
// wasmCompileBlock, executed under wazero against a synthetic linear memory
// that mirrors the JITContext layout newJITContext produces. Registers,
// RetPC, RetCount and guest RAM must match exactly.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"context"
	"math"
	"math/rand"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// Layout of the synthetic linear memory the generated block runs against.
// Mirrors what the generated code sees in the browser: JITContext, the
// register file, the FPU and guest RAM all live in one linear memory and the
// ctx fields hold their addresses.
const (
	wasmDiffCtxOff    = 0x100
	wasmDiffRegsOff   = 0x800
	wasmDiffFPUOff    = 0xC00
	wasmDiffBitmapOff = 0x10000
	wasmDiffSpansOff  = 0x34000
	wasmDiffGuestOff  = 0x40000
)

// wasmDiffResult captures the generated block's observable outputs.
type wasmDiffResult struct {
	regs     [32]uint64
	retPC    uint64
	retCount uint32
	guestRAM []byte // full guest RAM image after execution

	// Helper-exit protocol fields, read back from the ctx image.
	needHelper uint32
	helperSize uint32
	helperRd   uint32
	helperAddr uint64
	helperVal  uint64
	helperPC   uint64
	liveSP     uint64

	// SMC signalling fields.
	needInval uint32
	invalAddr uint64
	invalSize uint32

	// FPU state.
	fpregs [16]uint32
	fpcr   uint32
	fpsr   uint32
}

// runWasmDiffBlock scans, translates and executes program (raw IE64 bytes,
// laid down at PROG_START) under wazero. initRegs seeds the register file;
// tweak, when non-nil, runs after the ctx image is populated and before the
// block executes (e.g. to plant a code-page bitmap for SMC tests).
func runWasmDiffBlock(t *testing.T, program []byte, initRegs map[int]uint64, tweak func(api.Memory)) wasmDiffResult {
	return runWasmDiffCompiled(t, program, initRegs, tweak, nil)
}

// runWasmDiffCompiled is runWasmDiffBlock with an optional region compiler.
// The callback receives the donor guest memory after the program and sentinel
// have been installed.
func runWasmDiffCompiled(t *testing.T, program []byte, initRegs map[int]uint64, tweak func(api.Memory), compile func([]byte) ([]byte, error)) wasmDiffResult {
	t.Helper()

	// Donor machine: provides guest RAM image, the IO page bitmap, and the
	// scanner input, exactly as the browser runtime would.
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	copy(cpu.memory[PROG_START:], program)
	// Terminate the scan deterministically, then drop the HALT so the block
	// falls off the end (same pattern as the amd64 jitTestRig).
	copy(cpu.memory[PROG_START+len(program):], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	instrs := scanBlock(cpu.memory, PROG_START)
	// Strip only the appended sentinel, never a HALT that belongs to the
	// program itself (the scanner includes terminators).
	if n := len(instrs); n > 0 && instrs[n-1].opcode == OP_HALT64 && n*8 > len(program) {
		instrs = instrs[:n-1]
	}
	if compile == nil && len(instrs)*8 != len(program) {
		t.Fatalf("scanBlock decoded %d instrs, program has %d", len(instrs), len(program)/8)
	}

	var modBytes []byte
	var err error
	if compile != nil {
		modBytes, err = compile(cpu.memory)
	} else {
		modBytes, err = wasmCompileBlock(instrs, PROG_START)
	}
	if err != nil {
		t.Fatalf("wasmCompileBlock: %v", err)
	}

	// Env module: one big exported memory hosting ctx + regs + FPU + bitmap
	// + guest RAM.
	memBytes := uint64(wasmDiffGuestOff + len(cpu.memory))
	pages := uint32((memBytes + 0xFFFF) / 0x10000)
	envB := newWasmModuleBuilder()
	envB.defineMemory(pages)
	envB.exportMemory("mem")

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatalf("instantiate env: %v", err)
	}
	mem := env.ExportedMemory("mem")

	// Populate guest RAM and the IO page bitmap.
	if !mem.Write(wasmDiffGuestOff, cpu.memory) {
		t.Fatal("guest RAM write out of range")
	}
	if n := len(bus.ioPageBitmap); n > 0 {
		bmp := unsafe.Slice((*byte)(unsafe.Pointer(&bus.ioPageBitmap[0])), n)
		if !mem.Write(wasmDiffBitmapOff, bmp) {
			t.Fatal("bitmap write out of range")
		}
	}

	// JITContext image.
	putU64 := func(off uint32, v uint64) {
		if !mem.WriteUint64Le(wasmDiffCtxOff+off, v) {
			t.Fatalf("ctx write at +%d out of range", off)
		}
	}
	putU32 := func(off uint32, v uint32) {
		if !mem.WriteUint32Le(wasmDiffCtxOff+off, v) {
			t.Fatalf("ctx write at +%d out of range", off)
		}
	}
	putU64(jitCtxOffRegsPtr, wasmDiffRegsOff)
	putU64(jitCtxOffMemPtr, wasmDiffGuestOff)
	putU32(jitCtxOffMemSize, uint32(len(cpu.memory)))
	putU32(jitCtxOffIOStart, IO_REGION_START)
	putU64(jitCtxOffIOBitmapPtr, wasmDiffBitmapOff)
	putU64(jitCtxOffFPUPtr, wasmDiffFPUOff)
	// Code-page spans default to the full page ([0, 255] per page): identical
	// behaviour to a spanless probe. SMC tests narrow individual pages to
	// exercise the false-share skip.
	putU64(jitCtxOffCodePageSpansPtr, wasmDiffSpansOff)
	fullSpans := make([]byte, 0x4000)
	for i := 0; i < len(fullSpans); i += 2 {
		fullSpans[i] = 0x00
		fullSpans[i+1] = 0xFF
	}
	if !mem.Write(wasmDiffSpansOff, fullSpans) {
		t.Fatal("spans write out of range")
	}

	// Register file: reset defaults from the donor CPU (NewCPU64 seeds SP and
	// friends), then the test's overrides on top - identical to what the
	// interpreter side starts from.
	for i, v := range initRegs {
		cpu.regs[i] = v
	}
	for i := 0; i < 32; i++ {
		if !mem.WriteUint64Le(wasmDiffRegsOff+uint32(i)*8, cpu.regs[i]) {
			t.Fatal("reg write out of range")
		}
	}

	// FPU image mirrors the donor's (fresh) FPU.
	if cpu.FPU != nil {
		for i, v := range cpu.FPU.FPRegs {
			mem.WriteUint32Le(wasmDiffFPUOff+uint32(i)*4, v)
		}
		mem.WriteUint32Le(wasmDiffFPUOff+64, cpu.FPU.FPCR)
		mem.WriteUint32Le(wasmDiffFPUOff+68, cpu.FPU.FPSR)
	}

	if tweak != nil {
		tweak(mem)
	}

	mod, err := r.Instantiate(ctx, modBytes)
	if err != nil {
		t.Fatalf("instantiate block: %v", err)
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, wasmDiffCtxOff); err != nil {
		t.Fatalf("block call: %v", err)
	}

	var res wasmDiffResult
	for i := 0; i < 32; i++ {
		v, ok := mem.ReadUint64Le(wasmDiffRegsOff + uint32(i)*8)
		if !ok {
			t.Fatal("reg read out of range")
		}
		res.regs[i] = v
	}
	res.retPC, _ = mem.ReadUint64Le(wasmDiffCtxOff + jitCtxOffRetPC)
	rc, _ := mem.ReadUint32Le(wasmDiffCtxOff + jitCtxOffRetCount)
	res.retCount = rc
	res.needHelper, _ = mem.ReadUint32Le(wasmDiffCtxOff + jitCtxOffNeedHelper)
	res.helperSize, _ = mem.ReadUint32Le(wasmDiffCtxOff + jitCtxOffHelperSize)
	res.helperRd, _ = mem.ReadUint32Le(wasmDiffCtxOff + jitCtxOffHelperRd)
	res.helperAddr, _ = mem.ReadUint64Le(wasmDiffCtxOff + jitCtxOffHelperAddr)
	res.helperVal, _ = mem.ReadUint64Le(wasmDiffCtxOff + jitCtxOffHelperVal)
	res.helperPC, _ = mem.ReadUint64Le(wasmDiffCtxOff + jitCtxOffHelperPC)
	res.liveSP, _ = mem.ReadUint64Le(wasmDiffCtxOff + jitCtxOffLiveSP)
	res.needInval, _ = mem.ReadUint32Le(wasmDiffCtxOff + jitCtxOffNeedInval)
	res.invalAddr, _ = mem.ReadUint64Le(wasmDiffCtxOff + jitCtxOffInvalAddr)
	res.invalSize, _ = mem.ReadUint32Le(wasmDiffCtxOff + jitCtxOffInvalSize)
	for i := 0; i < 16; i++ {
		res.fpregs[i], _ = mem.ReadUint32Le(wasmDiffFPUOff + uint32(i)*4)
	}
	res.fpcr, _ = mem.ReadUint32Le(wasmDiffFPUOff + 64)
	res.fpsr, _ = mem.ReadUint32Le(wasmDiffFPUOff + 68)
	ram, ok := mem.Read(wasmDiffGuestOff, uint32(len(cpu.memory)))
	if !ok {
		t.Fatal("guest RAM read out of range")
	}
	res.guestRAM = ram
	return res
}

// runInterpDiff executes the same program on the real interpreter.
func runInterpDiff(t *testing.T, program []byte, initRegs map[int]uint64) *CPU64 {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	copy(cpu.memory[PROG_START:], program)
	// Same guest RAM image as the wasm side (which appends a HALT sentinel
	// for the scanner); the interpreter never executes it.
	copy(cpu.memory[PROG_START+len(program):], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	for i, v := range initRegs {
		cpu.regs[i] = v
	}
	cpu.PC = PROG_START
	steps := len(program) / 8
	for i := 0; i < steps; i++ {
		if cpu.StepOne() == 0 {
			t.Fatalf("interpreter stopped at step %d", i)
		}
	}
	return cpu
}

// diffRun is the main assertion: interpreter and generated code agree on
// registers, PC, retired count and guest RAM.
func diffRun(t *testing.T, initRegs map[int]uint64, instrs ...[]byte) {
	t.Helper()
	program := bytes.Join(instrs, nil)
	interp := runInterpDiff(t, program, initRegs)
	wres := runWasmDiffBlock(t, program, initRegs, nil)

	for i := 1; i < 32; i++ { // regs[0] excluded: dispatcher clears it after native runs
		if interp.regs[i] != wres.regs[i] {
			t.Errorf("R%d: interpreter %#x, wasm %#x", i, interp.regs[i], wres.regs[i])
		}
	}
	if wres.regs[0] != 0 {
		// The wasm backend never writes R0 (there is no legacy RetPC mirror).
		t.Errorf("R0: wasm wrote %#x, want 0", wres.regs[0])
	}
	if interp.PC != wres.retPC {
		t.Errorf("PC: interpreter %#x, wasm RetPC %#x", interp.PC, wres.retPC)
	}
	if want := uint32(len(program) / 8); wres.retCount != want {
		t.Errorf("RetCount: wasm %d, want %d", wres.retCount, want)
	}
	if !bytes.Equal(interp.memory, wres.guestRAM) {
		t.Errorf("guest RAM diverged")
	}
	if interp.FPU != nil {
		for i := 0; i < 16; i++ {
			if interp.FPU.FPRegs[i] != wres.fpregs[i] {
				t.Errorf("F%d: interpreter %#x, wasm %#x", i, interp.FPU.FPRegs[i], wres.fpregs[i])
			}
		}
		if interp.FPU.FPSR != wres.fpsr {
			t.Errorf("FPSR: interpreter %#x, wasm %#x", interp.FPU.FPSR, wres.fpsr)
		}
		if interp.FPU.FPCR != wres.fpcr {
			t.Errorf("FPCR: interpreter %#x, wasm %#x", interp.FPU.FPCR, wres.fpcr)
		}
	}
}

// ---------------------------------------------------------------------------
// Data movement
// ---------------------------------------------------------------------------

func TestWasmJIT_Diff_MoveFamily(t *testing.T) {
	init := map[int]uint64{2: 0xDEADBEEFCAFEBABE, 3: 0x1234}
	diffRun(t, init,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xFFFFFFFF), // imm zero-extends
		ie64Instr(OP_MOVE, 4, IE64_SIZE_B, 0, 2, 0, 0),          // reg, byte mask
		ie64Instr(OP_MOVE, 5, IE64_SIZE_W, 0, 2, 0, 0),
		ie64Instr(OP_MOVE, 6, IE64_SIZE_L, 0, 2, 0, 0),
		ie64Instr(OP_MOVE, 7, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_MOVT, 3, 0, 0, 0, 0, 0xAABBCCDD),  // high half RMW
		ie64Instr(OP_MOVEQ, 8, 0, 0, 0, 0, 0x80000000), // sign-extends
		ie64Instr(OP_LEA, 9, 0, 0, 2, 0, 0xFFFFFFFC),   // rs + sext(-4)
		ie64Instr(OP_LEA, 10, 0, 0, 0, 0, 0x00001000),  // R0 base
	)
}

func TestWasmJIT_Diff_R0Semantics(t *testing.T) {
	init := map[int]uint64{2: 99}
	diffRun(t, init,
		ie64Instr(OP_ADD, 0, IE64_SIZE_Q, 1, 2, 0, 5), // write to R0 discarded
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 0, 0, 2, 0), // R0 reads as zero
		ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 0, 0, 0, 0),
	)
}

// ---------------------------------------------------------------------------
// ALU
// ---------------------------------------------------------------------------

func TestWasmJIT_Diff_AddSubMul(t *testing.T) {
	init := map[int]uint64{2: 0xFFFFFFFFFFFFFFFF, 3: 7, 4: 0x8000000000000000}
	diffRun(t, init,
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_ADD, 5, IE64_SIZE_B, 1, 3, 0, 0xFF), // byte wrap
		ie64Instr(OP_SUB, 6, IE64_SIZE_Q, 0, 3, 2, 0),
		ie64Instr(OP_SUB, 7, IE64_SIZE_W, 1, 3, 0, 9),
		ie64Instr(OP_MULU, 8, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_MULS, 9, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_MULS, 10, IE64_SIZE_L, 1, 4, 0, 3),
		ie64Instr(OP_NEG, 11, IE64_SIZE_Q, 0, 4, 0, 0), // -MinInt64
		ie64Instr(OP_NEG, 12, IE64_SIZE_L, 0, 3, 0, 0),
	)
}

func TestWasmJIT_Diff_DivMod(t *testing.T) {
	init := map[int]uint64{
		2: 100, 3: 7,
		4: 0x8000000000000000, // MinInt64
		5: 0xFFFFFFFFFFFFFFFF, // -1
		6: 0,
	}
	diffRun(t, init,
		ie64Instr(OP_DIVU, 1, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_DIVU, 7, IE64_SIZE_Q, 0, 2, 6, 0), // div by zero -> 0
		ie64Instr(OP_DIVS, 8, IE64_SIZE_Q, 0, 4, 5, 0), // MinInt64 / -1 (Go wraps)
		ie64Instr(OP_DIVS, 9, IE64_SIZE_Q, 0, 2, 6, 0), // div by zero -> 0
		ie64Instr(OP_DIVS, 10, IE64_SIZE_Q, 0, 4, 3, 0),
		ie64Instr(OP_MOD64, 11, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_MOD64, 12, IE64_SIZE_Q, 0, 2, 6, 0), // mod by zero -> 0
		ie64Instr(OP_MODS, 13, IE64_SIZE_Q, 0, 4, 5, 0),  // MinInt64 % -1 -> 0
		ie64Instr(OP_MODS, 14, IE64_SIZE_B, 0, 4, 3, 0),  // sign-extends per size first
		ie64Instr(OP_MODS, 15, IE64_SIZE_L, 1, 5, 0, 10),
	)
}

func TestWasmJIT_Diff_MulHigh(t *testing.T) {
	init := map[int]uint64{
		2: 0xFFFFFFFFFFFFFFFF,
		3: 0x8000000000000000,
		4: 0x123456789ABCDEF0,
		5: 2,
	}
	diffRun(t, init,
		ie64Instr(OP_MULHU, 1, IE64_SIZE_Q, 0, 2, 2, 0),
		ie64Instr(OP_MULHU, 6, IE64_SIZE_Q, 0, 4, 4, 0),
		ie64Instr(OP_MULHU, 7, IE64_SIZE_Q, 0, 3, 5, 0),
		ie64Instr(OP_MULHS, 8, IE64_SIZE_Q, 0, 2, 2, 0),  // -1 * -1
		ie64Instr(OP_MULHS, 9, IE64_SIZE_Q, 0, 3, 5, 0),  // MinInt64 * 2
		ie64Instr(OP_MULHS, 10, IE64_SIZE_Q, 0, 4, 2, 0), // pos * -1
	)
}

func TestWasmJIT_Diff_Logic(t *testing.T) {
	init := map[int]uint64{2: 0xF0F0F0F0F0F0F0F0, 3: 0x0FF00FF00FF00FF0}
	diffRun(t, init,
		ie64Instr(OP_AND64, 1, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_OR64, 4, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_EOR, 5, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_NOT64, 6, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_AND64, 7, IE64_SIZE_B, 1, 2, 0, 0x3C),
		ie64Instr(OP_NOT64, 8, IE64_SIZE_W, 0, 3, 0, 0), // masked NOT
	)
}

func TestWasmJIT_Diff_Shifts(t *testing.T) {
	init := map[int]uint64{2: 0x8000000000000001, 3: 4, 4: 0xFFFF, 5: 63, 6: 64}
	diffRun(t, init,
		ie64Instr(OP_LSL, 1, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_LSL, 7, IE64_SIZE_B, 0, 4, 3, 0),
		ie64Instr(OP_LSR, 8, IE64_SIZE_Q, 0, 2, 5, 0),
		ie64Instr(OP_LSR, 9, IE64_SIZE_Q, 0, 2, 6, 0), // count 64 & 63 = 0
		ie64Instr(OP_ASR, 10, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_ASR, 11, IE64_SIZE_B, 1, 2, 0, 1), // int8 source
		ie64Instr(OP_ASR, 12, IE64_SIZE_W, 1, 4, 0, 3),
		ie64Instr(OP_ASR, 13, IE64_SIZE_L, 1, 2, 0, 31),
	)
}

func TestWasmJIT_Diff_Rotates(t *testing.T) {
	init := map[int]uint64{2: 0x80000000000000FF, 3: 3, 4: 0xA5, 5: 0}
	diffRun(t, init,
		ie64Instr(OP_ROL, 1, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_ROR, 6, IE64_SIZE_Q, 0, 2, 3, 0),
		ie64Instr(OP_ROL, 7, IE64_SIZE_B, 0, 4, 3, 0),
		ie64Instr(OP_ROR, 8, IE64_SIZE_B, 0, 4, 3, 0),
		ie64Instr(OP_ROL, 9, IE64_SIZE_W, 1, 2, 0, 5),
		ie64Instr(OP_ROR, 10, IE64_SIZE_L, 1, 2, 0, 9),
		ie64Instr(OP_ROL, 11, IE64_SIZE_L, 0, 4, 5, 0), // rotate by 0
	)
}

func TestWasmJIT_Diff_Bit32Ops(t *testing.T) {
	init := map[int]uint64{2: 0xFFFFFFFF00010000, 3: 0, 4: 0x12345678}
	diffRun(t, init,
		ie64Instr(OP_CLZ, 1, IE64_SIZE_Q, 0, 2, 0, 0), // operates on low 32
		ie64Instr(OP_CLZ, 5, IE64_SIZE_Q, 0, 3, 0, 0), // clz(0) = 32
		ie64Instr(OP_CTZ, 6, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_CTZ, 7, IE64_SIZE_Q, 0, 3, 0, 0),
		ie64Instr(OP_POPCNT, 8, IE64_SIZE_Q, 0, 4, 0, 0),
		ie64Instr(OP_BSWAP, 9, IE64_SIZE_Q, 0, 4, 0, 0),
		ie64Instr(OP_SEXT, 10, IE64_SIZE_B, 0, 4, 0, 0),
		ie64Instr(OP_SEXT, 11, IE64_SIZE_W, 0, 2, 0, 0),
		ie64Instr(OP_SEXT, 12, IE64_SIZE_L, 0, 2, 0, 0),
		ie64Instr(OP_SEXT, 13, IE64_SIZE_Q, 0, 2, 0, 0),
	)
}

// ---------------------------------------------------------------------------
// Property test: random supported ALU programs
// ---------------------------------------------------------------------------

func TestWasmJIT_Diff_RandomALU(t *testing.T) {
	ops := []byte{
		OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA,
		OP_ADD, OP_SUB, OP_MULU, OP_MULS, OP_DIVU, OP_DIVS,
		OP_MOD64, OP_MODS, OP_NEG, OP_MULHU, OP_MULHS,
		OP_AND64, OP_OR64, OP_EOR, OP_NOT64,
		OP_LSL, OP_LSR, OP_ASR, OP_CLZ, OP_CTZ, OP_POPCNT,
		OP_BSWAP, OP_SEXT, OP_ROL, OP_ROR,
	}
	interesting := []uint64{
		0, 1, 2, 7, 63, 64, 0xFF, 0x7FFF, 0x8000, 0xFFFF,
		0x7FFFFFFF, 0x80000000, 0xFFFFFFFF, 0x100000000,
		0x7FFFFFFFFFFFFFFF, 0x8000000000000000, 0xFFFFFFFFFFFFFFFF,
		0xDEADBEEFCAFEBABE, math.MaxUint64 - 1,
	}
	rng := rand.New(rand.NewSource(0x1E64))
	for run := 0; run < 40; run++ {
		init := map[int]uint64{}
		for r := 1; r < 16; r++ {
			init[r] = interesting[rng.Intn(len(interesting))]
		}
		var prog [][]byte
		n := 4 + rng.Intn(28)
		for i := 0; i < n; i++ {
			op := ops[rng.Intn(len(ops))]
			rd := byte(rng.Intn(16))
			rs := byte(rng.Intn(16))
			rt := byte(rng.Intn(16))
			size := byte(rng.Intn(4))
			xbit := byte(rng.Intn(2))
			imm := uint32(interesting[rng.Intn(len(interesting))])
			prog = append(prog, ie64Instr(op, rd, size, xbit, rs, rt, imm))
		}
		diffRun(t, init, prog...)
		if t.Failed() {
			t.Fatalf("random run %d diverged (seed fixed, rerun to reproduce)", run)
		}
	}
}

// diffRunSteps is diffRun for programs whose control flow exits the block
// early: the interpreter retires exactly `steps` instructions and the block
// must report the same RetCount and final PC.
func diffRunSteps(t *testing.T, initRegs map[int]uint64, steps int, instrs ...[]byte) {
	t.Helper()
	program := bytes.Join(instrs, nil)

	bus := NewMachineBus()
	interp := NewCPU64(bus)
	copy(interp.memory[PROG_START:], program)
	copy(interp.memory[PROG_START+len(program):], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	for i, v := range initRegs {
		interp.regs[i] = v
	}
	interp.PC = PROG_START
	for i := 0; i < steps; i++ {
		if interp.StepOne() == 0 {
			t.Fatalf("interpreter stopped at step %d", i)
		}
	}

	wres := runWasmDiffBlock(t, program, initRegs, nil)
	for i := 1; i < 32; i++ {
		if interp.regs[i] != wres.regs[i] {
			t.Errorf("R%d: interpreter %#x, wasm %#x", i, interp.regs[i], wres.regs[i])
		}
	}
	if interp.PC != wres.retPC {
		t.Errorf("PC: interpreter %#x, wasm RetPC %#x", interp.PC, wres.retPC)
	}
	if wres.retCount != uint32(steps) {
		t.Errorf("RetCount: wasm %d, want %d", wres.retCount, steps)
	}
	if !bytes.Equal(interp.memory, wres.guestRAM) {
		t.Errorf("guest RAM diverged")
	}
}

// ---------------------------------------------------------------------------
// Branches and terminators
// ---------------------------------------------------------------------------

func TestWasmJIT_Diff_BranchesTakenAndNot(t *testing.T) {
	// Every condition code, taken and not taken. Taken branches target a
	// point beyond the block so both engines stop at the same PC.
	cases := []struct {
		op    byte
		a, b  uint64
		taken bool
	}{
		{OP_BEQ, 5, 5, true},
		{OP_BEQ, 5, 6, false},
		{OP_BNE, 5, 6, true},
		{OP_BNE, 5, 5, false},
		{OP_BLT, ^uint64(1), 1, true}, // signed
		{OP_BLT, 1, ^uint64(1), false},
		{OP_BGE, 1, ^uint64(1), true},
		{OP_BGE, ^uint64(1), 1, false},
		{OP_BGT, 3, 2, true},
		{OP_BGT, 2, 2, false},
		{OP_BLE, 2, 2, true},
		{OP_BLE, 3, 2, false},
		{OP_BHI, ^uint64(1), 1, true}, // unsigned
		{OP_BHI, 1, ^uint64(1), false},
		{OP_BLS, 1, ^uint64(1), true},
		{OP_BLS, ^uint64(1), 1, false},
	}
	for _, c := range cases {
		init := map[int]uint64{2: c.a, 3: c.b}
		steps := 3 // lead-in ADD + branch + (trailing ADD when not taken)
		if c.taken {
			steps = 2
		}
		diffRunSteps(t, init, steps,
			ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 0, 0, 1),
			ie64Instr(c.op, 0, 0, 0, 2, 3, 0x40), // branch +64 from its own PC
			ie64Instr(OP_ADD, 5, IE64_SIZE_Q, 1, 0, 0, 2),
		)
	}
}

func TestWasmJIT_Diff_BranchBackwardTaken(t *testing.T) {
	// A taken backward branch exits the block with RetPC before the block:
	// negative displacement path.
	init := map[int]uint64{2: 1, 3: 1}
	diffRunSteps(t, init, 2,
		ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 0, 0, 1),
		ie64Instr(OP_BEQ, 0, 0, 0, 2, 3, 0xFFFFFF00), // -256 from branch PC
	)
}

func TestWasmJIT_Diff_BRAAndJMP(t *testing.T) {
	diffRunSteps(t, nil, 2,
		ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 0, 0, 9),
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 0x80),
	)
	init := map[int]uint64{2: 0x7000}
	diffRunSteps(t, init, 2,
		ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 0, 0, 9),
		ie64Instr(OP_JMP, 0, 0, 0, 2, 0, 0x10), // regs[2] + 0x10
	)
}

func TestWasmJIT_HALTExitsAtOwnPC(t *testing.T) {
	// HALT: RetPC is the HALT's own PC (the dispatcher re-checks the opcode
	// there), RetCount includes it.
	program := bytes.Join([][]byte{
		ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 0, 0, 1),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	}, nil)
	res := runWasmDiffBlock(t, program, nil, nil)
	if res.retPC != PROG_START+8 {
		t.Errorf("RetPC = %#x, want HALT PC %#x", res.retPC, uint64(PROG_START+8))
	}
	// The HALT itself is not retired by the block: the dispatcher interprets
	// it at RetPC (clearing cpu.running) and accounts for it there.
	if res.retCount != 1 {
		t.Errorf("RetCount = %d, want 1 (ADD only)", res.retCount)
	}
	if res.needHelper != 0 {
		t.Errorf("NeedHelper = %d, want 0", res.needHelper)
	}
}

// ---------------------------------------------------------------------------
// Stack operations
// ---------------------------------------------------------------------------

func TestWasmJIT_Diff_PushPop(t *testing.T) {
	init := map[int]uint64{2: 0x1122334455667788, 3: 0xCAFE}
	diffRun(t, init,
		ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0),
		ie64Instr(OP_PUSH64, 0, 0, 0, 3, 0, 0),
		ie64Instr(OP_PUSH64, 0, 0, 0, 31, 0, 0), // pushes the decremented SP
		ie64Instr(OP_POP64, 4, 0, 0, 0, 0, 0),
		ie64Instr(OP_POP64, 5, 0, 0, 0, 0, 0),
		ie64Instr(OP_POP64, 0, 0, 0, 0, 0, 0), // rd==0: SP moves, value dropped
	)
}

func TestWasmJIT_Diff_JSRAndRTS(t *testing.T) {
	// JSR is a terminator: single-instruction block. Return address lands on
	// the stack, SP decrements, PC = target.
	diffRunSteps(t, nil, 1,
		ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 0x100),
	)
	// RTS: seed the return address via a PUSH in the same run (PUSH is not a
	// terminator, so PUSH+RTS form one block; RTS pops it into PC).
	init := map[int]uint64{2: 0x4320}
	diffRunSteps(t, init, 2,
		ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),
	)
	// JSR_IND: target from register (+disp), return address pushed.
	init = map[int]uint64{5: 0x9000}
	diffRunSteps(t, init, 1,
		ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0x20),
	)
}

func TestWasmJIT_HelperExit_StackOutsideRAM(t *testing.T) {
	// SP parked in the IO region: PUSH must take the helper exit with the
	// pre-decrement SP in LiveSP and the pushed value in HelperVal.
	init := map[int]uint64{2: 0xBEEF, 31: 0xF0050}
	program := ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0)
	res := runWasmDiffBlock(t, program, init, markIOPage(t, 0xF0050-8))
	if res.needHelper != HELPER_PUSH {
		t.Fatalf("NeedHelper = %d, want HELPER_PUSH", res.needHelper)
	}
	if res.helperVal != 0xBEEF {
		t.Errorf("HelperVal = %#x, want 0xBEEF", res.helperVal)
	}
	if res.liveSP != 0xF0050 {
		t.Errorf("LiveSP = %#x, want pre-decrement 0xF0050", res.liveSP)
	}
	if res.regs[31] != 0xF0050 {
		t.Errorf("SP mutated to %#x before helper", res.regs[31])
	}

	// RTS with SP beyond guest RAM: bounds check helper.
	memSize := uint64(len(NewCPU64(NewMachineBus()).memory))
	init = map[int]uint64{31: memSize + 0x1000}
	program = ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)
	res = runWasmDiffBlock(t, program, init, nil)
	if res.needHelper != HELPER_RTS {
		t.Fatalf("NeedHelper = %d, want HELPER_RTS", res.needHelper)
	}
	if res.liveSP != memSize+0x1000 {
		t.Errorf("LiveSP = %#x, want %#x", res.liveSP, memSize+0x1000)
	}
}

func TestWasmJIT_SMC_PushProbesCodeBitmap(t *testing.T) {
	// A PUSH whose slot lands in a compiled code page must report SMC, same
	// as an ordinary STORE.
	spTop := uint64(0x50010)
	plant := func(mem api.Memory) {
		if !mem.WriteUint64Le(wasmDiffCtxOff+jitCtxOffCodePageBitmapPtr, 0x30000) {
			t.Fatal("ctx write")
		}
		if !mem.WriteUint32Le(wasmDiffCtxOff+jitCtxOffCodePageBitmapLen, uint32((spTop>>8)+8)) {
			t.Fatal("ctx write")
		}
		if !mem.WriteByte(0x30000+uint32((spTop-8)>>8), 1) {
			t.Fatal("bitmap write")
		}
	}
	init := map[int]uint64{2: 7, 31: spTop}
	program := ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0)
	res := runWasmDiffBlock(t, program, init, plant)
	if res.needInval != 1 || res.invalAddr != spTop-8 || res.invalSize != 8 {
		t.Errorf("PUSH SMC: NeedInval/Addr/Size = %d/%#x/%d, want 1/%#x/8",
			res.needInval, res.invalAddr, res.invalSize, spTop-8)
	}
}

// ---------------------------------------------------------------------------
// LOAD/STORE: RAM fast path, IO helper exits, SMC probe
// ---------------------------------------------------------------------------

func TestWasmJIT_Diff_LoadStoreRAM(t *testing.T) {
	// Scratch RAM well below IO_REGION_START via register base, all sizes,
	// negative displacement, zero-extension of narrow loads.
	init := map[int]uint64{2: 0x50000, 3: 0xDEADBEEFCAFEF00D}
	diffRun(t, init,
		ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_STORE, 3, IE64_SIZE_B, 0, 2, 0, 16),
		ie64Instr(OP_STORE, 3, IE64_SIZE_W, 0, 2, 0, 18),
		ie64Instr(OP_STORE, 3, IE64_SIZE_L, 0, 2, 0, 20),
		ie64Instr(OP_STORE, 3, IE64_SIZE_L, 0, 2, 0, 0xFFFFFFF8), // rs - 8
		ie64Instr(OP_LOAD, 4, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_LOAD, 5, IE64_SIZE_B, 0, 2, 0, 0), // 0x0D, zero-extended
		ie64Instr(OP_LOAD, 6, IE64_SIZE_W, 0, 2, 0, 0),
		ie64Instr(OP_LOAD, 7, IE64_SIZE_L, 0, 2, 0, 4), // high half
		ie64Instr(OP_LOAD, 8, IE64_SIZE_L, 0, 2, 0, 0xFFFFFFF8),
	)
}

// markIOPage returns a tweak that flags the page of addr as MMIO in the IO
// page bitmap image (a bare test bus has no devices, so no pages are set).
func markIOPage(t *testing.T, addr uint64) func(api.Memory) {
	return func(mem api.Memory) {
		if !mem.WriteByte(wasmDiffBitmapOff+uint32(addr>>8), 1) {
			t.Fatal("io bitmap write out of range")
		}
	}
}

func TestWasmJIT_Diff_LoadR0Skipped(t *testing.T) {
	// A LOAD with rd==0 is skipped entirely, even from an IO-region address:
	// no helper exit, no side effect, block completes.
	init := map[int]uint64{2: 0xF0048}
	diffRun(t, init,
		ie64Instr(OP_LOAD, 0, IE64_SIZE_L, 0, 2, 0, 0),
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 0, 0, 7),
	)
}

func TestWasmJIT_HelperExit_LoadMMIO(t *testing.T) {
	mmio := uint64(0xF0048)
	init := map[int]uint64{2: mmio, 3: 5}
	program := bytes.Join([][]byte{
		ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 3, 3, 0),  // retires before the bail
		ie64Instr(OP_LOAD, 1, IE64_SIZE_L, 0, 2, 0, 4), // MMIO -> helper exit
		ie64Instr(OP_ADD, 5, IE64_SIZE_Q, 1, 3, 0, 1),  // must NOT run
	}, nil)
	res := runWasmDiffBlock(t, program, init, markIOPage(t, mmio))

	bailPC := uint64(PROG_START + 8)
	if res.needHelper != HELPER_LOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_LOAD", res.needHelper)
	}
	if res.helperAddr != mmio+4 {
		t.Errorf("HelperAddr = %#x, want %#x", res.helperAddr, mmio+4)
	}
	if res.helperSize != uint32(IE64_SIZE_L) {
		t.Errorf("HelperSize = %d, want L", res.helperSize)
	}
	if res.helperRd != 1 {
		t.Errorf("HelperRd = %d, want 1", res.helperRd)
	}
	if res.helperPC != bailPC || res.retPC != bailPC {
		t.Errorf("HelperPC/RetPC = %#x/%#x, want both %#x", res.helperPC, res.retPC, bailPC)
	}
	if res.retCount != 1 {
		t.Errorf("RetCount = %d, want 1 (only the ADD retired)", res.retCount)
	}
	if res.liveSP != res.regs[31] {
		t.Errorf("LiveSP = %#x, regs[31] = %#x", res.liveSP, res.regs[31])
	}
	if res.regs[4] != 10 {
		t.Errorf("R4 = %d, want 10 (pre-bail ADD retired)", res.regs[4])
	}
	if res.regs[5] != 0 {
		t.Errorf("R5 = %d, want 0 (post-bail ADD must not run)", res.regs[5])
	}
}

func TestWasmJIT_HelperExit_StoreMMIOAndBounds(t *testing.T) {
	mmio := uint64(0xF0048)
	// MMIO store: HelperVal carries the size-masked value.
	init := map[int]uint64{2: mmio, 3: 0xAABBCCDD11223344}
	program := ie64Instr(OP_STORE, 3, IE64_SIZE_W, 0, 2, 0, 0)
	res := runWasmDiffBlock(t, program, init, markIOPage(t, mmio))
	if res.needHelper != HELPER_STORE {
		t.Fatalf("NeedHelper = %d, want HELPER_STORE", res.needHelper)
	}
	if res.helperVal != 0x3344 {
		t.Errorf("HelperVal = %#x, want masked 0x3344", res.helperVal)
	}
	if res.retCount != 0 || res.retPC != PROG_START {
		t.Errorf("RetCount/RetPC = %d/%#x, want 0/%#x", res.retCount, res.retPC, PROG_START)
	}

	// Out-of-bounds store (beyond MemSize): helper exit via the bounds check.
	memSize := uint64(len(NewCPU64(NewMachineBus()).memory))
	init = map[int]uint64{2: memSize - 4, 3: 42} // 8-byte store straddles the end
	program = ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 2, 0, 0)
	res = runWasmDiffBlock(t, program, init, nil)
	if res.needHelper != HELPER_STORE {
		t.Fatalf("bounds: NeedHelper = %d, want HELPER_STORE", res.needHelper)
	}
	if res.helperAddr != memSize-4 {
		t.Errorf("bounds: HelperAddr = %#x, want %#x", res.helperAddr, memSize-4)
	}
}

func TestWasmJIT_SMC_StoreProbe(t *testing.T) {
	// Plant a code-page bitmap covering the store target and check the
	// generated STORE reports the self-modifying write: the dirty store
	// commits, publishes its exact NeedInval/InvalAddr/InvalSize range and
	// ends the block immediately (so one block reports at most one range,
	// and no further instructions run from a possibly rewritten image).
	// A store to a clean page reports nothing.
	target := uint64(0x50000)
	bitmapLen := uint32((target >> 8) + 16)
	const smcBitmapOff = 0x30000 // spare room between bitmap and guest RAM

	plant := func(mem api.Memory) {
		if !mem.WriteUint64Le(wasmDiffCtxOff+jitCtxOffCodePageBitmapPtr, smcBitmapOff) {
			t.Fatal("ctx write")
		}
		if !mem.WriteUint32Le(wasmDiffCtxOff+jitCtxOffCodePageBitmapLen, bitmapLen) {
			t.Fatal("ctx write")
		}
		if !mem.WriteByte(smcBitmapOff+uint32(target>>8), 1) {
			t.Fatal("bitmap write")
		}
	}

	// Clean-page store, a dirty store, then a third store that must never
	// run: the dirty store ends the block.
	init := map[int]uint64{2: target, 3: 0x11, 4: target + 0x1000}
	program := bytes.Join([][]byte{
		ie64Instr(OP_STORE, 3, IE64_SIZE_L, 0, 4, 0, 0),  // clean page
		ie64Instr(OP_STORE, 3, IE64_SIZE_L, 0, 2, 0, 0),  // dirty: reports and exits
		ie64Instr(OP_STORE, 3, IE64_SIZE_B, 0, 2, 0, 64), // must not execute
	}, nil)
	res := runWasmDiffBlock(t, program, init, plant)
	if res.needHelper != 0 {
		t.Fatalf("unexpected helper exit %d", res.needHelper)
	}
	if res.needInval != 1 {
		t.Fatalf("NeedInval = %d, want 1", res.needInval)
	}
	if res.invalAddr != target || res.invalSize != 4 {
		t.Errorf("InvalAddr/Size = %#x/%d, want %#x/4 (exact first-hit range)",
			res.invalAddr, res.invalSize, target)
	}
	if res.retCount != 2 {
		t.Errorf("RetCount = %d, want 2 (dirty store retires, then the block exits)", res.retCount)
	}
	if want := PROG_START + 16; res.retPC != uint64(want) {
		t.Errorf("RetPC = %#x, want %#x (instruction after the dirty store)", res.retPC, want)
	}
	if res.guestRAM[target+64] != 0 {
		t.Errorf("instruction after the dirty store executed: [target+64] = %#x, want 0",
			res.guestRAM[target+64])
	}

	// Single dirty store: exact range reported.
	program = ie64Instr(OP_STORE, 3, IE64_SIZE_L, 0, 2, 0, 8)
	res = runWasmDiffBlock(t, program, init, plant)
	if res.needInval != 1 || res.invalAddr != target+8 || res.invalSize != 4 {
		t.Errorf("single dirty store: NeedInval/Addr/Size = %d/%#x/%d, want 1/%#x/4",
			res.needInval, res.invalAddr, res.invalSize, target+8)
	}

	// Page-straddling store whose FIRST page is clean but second is dirty.
	init2 := map[int]uint64{2: target - 4, 3: 0x1122334455667788}
	program = ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 2, 0, 0)
	res = runWasmDiffBlock(t, program, init2, plant)
	if res.needInval != 1 {
		t.Errorf("straddle: NeedInval = %d, want 1 (second page dirty)", res.needInval)
	}

	// No bitmap planted: nothing reported.
	res = runWasmDiffBlock(t, program, init2, nil)
	if res.needInval != 0 {
		t.Errorf("clean rig: NeedInval = %d, want 0", res.needInval)
	}
}

func TestWasmJIT_SMC_SpanSkipsFalseShare(t *testing.T) {
	// A store into a marked page but OUTSIDE the page's compiled-code span
	// is a false share: it must commit and continue with no SMC report and
	// no block exit. A store overlapping the span still reports. This is the
	// EhBASIC 0x1a9a8 pattern: one hot data word beside compiled code took
	// tens of thousands of chain exits a second before the span check.
	target := uint64(0x50000) // page 0x500, offset 0x00
	page := uint32(target >> 8)
	bitmapLen := uint32((target >> 8) + 16)
	const smcBitmapOff = 0x30000

	// Code span [0x90, 0x9F]; the data word sits at offset 0xA8.
	plant := func(mem api.Memory) {
		if !mem.WriteUint64Le(wasmDiffCtxOff+jitCtxOffCodePageBitmapPtr, smcBitmapOff) {
			t.Fatal("ctx write")
		}
		if !mem.WriteUint32Le(wasmDiffCtxOff+jitCtxOffCodePageBitmapLen, bitmapLen) {
			t.Fatal("ctx write")
		}
		if !mem.WriteByte(smcBitmapOff+page, 1) {
			t.Fatal("bitmap write")
		}
		if !mem.WriteByte(wasmDiffSpansOff+page*2, 0x90) {
			t.Fatal("span min write")
		}
		if !mem.WriteByte(wasmDiffSpansOff+page*2+1, 0x9F) {
			t.Fatal("span max write")
		}
	}

	// Store beside the span, then a follow-up that proves the block did NOT
	// exit.
	init := map[int]uint64{2: target, 3: 0x11}
	program := bytes.Join([][]byte{
		ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 2, 0, 0xA8), // false share: no exit
		ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 0, 0, 7),      // must still run
	}, nil)
	res := runWasmDiffBlock(t, program, init, plant)
	if res.needInval != 0 {
		t.Fatalf("false share reported SMC: NeedInval = %d, want 0", res.needInval)
	}
	if res.regs[4] != 7 {
		t.Errorf("block exited on false share: R4 = %d, want 7", res.regs[4])
	}
	if res.retCount != 2 {
		t.Errorf("RetCount = %d, want 2 (both instructions retired)", res.retCount)
	}

	// Store overlapping the span's first byte (write covers 0x8C..0x93).
	program = bytes.Join([][]byte{
		ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 2, 0, 0x8C),
		ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 0, 0, 7), // must NOT run
	}, nil)
	res = runWasmDiffBlock(t, program, init, plant)
	if res.needInval != 1 || res.invalAddr != target+0x8C || res.invalSize != 8 {
		t.Errorf("span overlap: NeedInval/Addr/Size = %d/%#x/%d, want 1/%#x/8",
			res.needInval, res.invalAddr, res.invalSize, target+0x8C)
	}
	if res.regs[4] != 0 {
		t.Errorf("dirty store did not end the block: R4 = %d, want 0", res.regs[4])
	}

	// Store just past the span's last byte: false share again.
	program = ie64Instr(OP_STORE, 3, IE64_SIZE_B, 0, 2, 0, 0xA0)
	res = runWasmDiffBlock(t, program, init, plant)
	if res.needInval != 0 {
		t.Errorf("byte past span: NeedInval = %d, want 0", res.needInval)
	}

	// Straddling store out of the PREVIOUS page into this one: covers
	// offsets 0xFC..0x103 of the pair, i.e. this page's bytes 0x00..0x03,
	// below the span: false share. The previous page is unmarked.
	init2 := map[int]uint64{2: target - 4, 3: 0x1122334455667788}
	program = ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 2, 0, 0)
	res = runWasmDiffBlock(t, program, init2, plant)
	if res.needInval != 0 {
		t.Errorf("straddle below span: NeedInval = %d, want 0", res.needInval)
	}

	// Same straddle, but the span starts at 0: dirty.
	plantAtZero := func(mem api.Memory) {
		plant(mem)
		if !mem.WriteByte(wasmDiffSpansOff+page*2, 0x00) {
			t.Fatal("span min write")
		}
	}
	res = runWasmDiffBlock(t, program, init2, plantAtZero)
	if res.needInval != 1 {
		t.Errorf("straddle into span at 0: NeedInval = %d, want 1", res.needInval)
	}
}

// ---------------------------------------------------------------------------
// FP64
// ---------------------------------------------------------------------------

// dloadBits builds STORE+DLOAD pairs that plant exact f64 bit patterns into
// FP pairs: regs[srcReg] holds the bits, regs[2] a RAM scratch base.
func dloadBits(pair byte, srcReg byte, scratchOff uint32) [][]byte {
	return [][]byte{
		ie64Instr(OP_STORE, srcReg, IE64_SIZE_Q, 0, 2, 0, scratchOff),
		ie64Instr(OP_DLOAD, pair, 0, 0, 2, 0, scratchOff),
	}
}

func TestWasmJIT_Diff_FP64Arith(t *testing.T) {
	init := map[int]uint64{
		2: 0x50000,
		3: math.Float64bits(1.5),
		4: math.Float64bits(-2.25),
		5: math.Float64bits(1e300),
	}
	var prog [][]byte
	prog = append(prog, dloadBits(0, 3, 0)...)
	prog = append(prog, dloadBits(2, 4, 8)...)
	prog = append(prog, dloadBits(4, 5, 16)...)
	prog = append(prog,
		ie64Instr(OP_DADD, 6, 0, 0, 0, 2, 0),  // D6 = D0 + D2
		ie64Instr(OP_DSUB, 8, 0, 0, 0, 2, 0),  // D8 = D0 - D2
		ie64Instr(OP_DMUL, 10, 0, 0, 2, 4, 0), // D10 = D2 * D4
		ie64Instr(OP_DDIV, 12, 0, 0, 0, 2, 0), // D12 = D0 / D2
		ie64Instr(OP_DMOV, 14, 0, 0, 6, 0, 0),
		ie64Instr(OP_DSTORE, 6, 0, 0, 2, 0, 24), // result bits back to RAM
	)
	diffRun(t, init, prog...)
}

func TestWasmJIT_Diff_FP64StickyExceptions(t *testing.T) {
	init := map[int]uint64{
		2: 0x50000,
		3: math.Float64bits(1.0),
		4: math.Float64bits(0.0),
		5: math.Float64bits(math.Inf(1)),
		6: math.Float64bits(1e308),
		7: math.Float64bits(1e-308),
		8: math.Float64bits(-1.0),
	}
	var prog [][]byte
	prog = append(prog, dloadBits(0, 3, 0)...)   // D0 = 1
	prog = append(prog, dloadBits(2, 4, 8)...)   // D2 = 0
	prog = append(prog, dloadBits(4, 5, 16)...)  // D4 = +Inf
	prog = append(prog, dloadBits(6, 6, 24)...)  // D6 = 1e308
	prog = append(prog, dloadBits(8, 7, 32)...)  // D8 = 1e-308
	prog = append(prog, dloadBits(10, 8, 40)...) // D10 = -1
	prog = append(prog,
		ie64Instr(OP_DDIV, 12, 0, 0, 0, 2, 0),   // 1/0: DZ
		ie64Instr(OP_DMUL, 12, 0, 0, 6, 6, 0),   // overflow: OE
		ie64Instr(OP_DMUL, 12, 0, 0, 8, 8, 0),   // underflow to 0: UE
		ie64Instr(OP_DSUB, 12, 0, 0, 4, 4, 0),   // Inf-Inf: IO (NaN result)
		ie64Instr(OP_DSQRT, 12, 0, 0, 10, 0, 0), // sqrt(-1): IO
	)
	diffRun(t, init, prog...)
}

func TestWasmJIT_Diff_FP64MoveAbsNegSqrtInt(t *testing.T) {
	init := map[int]uint64{
		2: 0x50000,
		3: math.Float64bits(-3.75),
		4: math.Float64bits(2.5), // ties-to-even at .5
		5: math.Float64bits(9.0),
	}
	var prog [][]byte
	prog = append(prog, dloadBits(0, 3, 0)...)
	prog = append(prog, dloadBits(2, 4, 8)...)
	prog = append(prog, dloadBits(4, 5, 16)...)
	prog = append(prog,
		ie64Instr(OP_DABS, 6, 0, 0, 0, 0, 0),
		ie64Instr(OP_DNEG, 8, 0, 0, 2, 0, 0),
		ie64Instr(OP_DSQRT, 10, 0, 0, 4, 0, 0),
		ie64Instr(OP_DINT, 12, 0, 0, 0, 0, 0), // nearest (FPCR default)
		ie64Instr(OP_DINT, 14, 0, 0, 2, 0, 0), // 2.5 -> 2 (ties to even)
		ie64Instr(OP_DMOV, 0, 0, 0, 10, 0, 0),
	)
	diffRun(t, init, prog...)
}

func TestWasmJIT_Diff_FP64CmpAndConverts(t *testing.T) {
	init := map[int]uint64{
		2:  0x50000,
		3:  math.Float64bits(1.0),
		4:  math.Float64bits(2.0),
		5:  math.Float64bits(math.NaN()),
		6:  math.Float64bits(9.223372036854775808e18), // exactly 2^63
		7:  math.Float64bits(1e19),                    // > MaxInt64
		8:  math.Float64bits(-1e19),                   // < MinInt64
		9:  42,
		10: ^uint64(6), // -7
	}
	var prog [][]byte
	prog = append(prog, dloadBits(0, 3, 0)...)
	prog = append(prog, dloadBits(2, 4, 8)...)
	prog = append(prog, dloadBits(4, 5, 16)...)
	prog = append(prog, dloadBits(6, 6, 24)...)
	prog = append(prog, dloadBits(8, 7, 32)...)
	prog = append(prog, dloadBits(10, 8, 40)...)
	prog = append(prog,
		ie64Instr(OP_DCMP, 11, 0, 0, 0, 2, 0), // 1 < 2: -1, CC_N
		ie64Instr(OP_DCMP, 12, 0, 0, 2, 0, 0), // 2 > 1: 1
		ie64Instr(OP_DCMP, 13, 0, 0, 0, 0, 0), // equal: 0, CC_Z
		ie64Instr(OP_DCMP, 14, 0, 0, 0, 4, 0), // vs NaN: 0, CC_NAN + IO
		ie64Instr(OP_DCVTIF, 0, 0, 0, 9, 0, 0),
		ie64Instr(OP_DCVTIF, 2, 0, 0, 10, 0, 0),
		ie64Instr(OP_DCVTFI, 15, 0, 0, 2, 0, 0),  // -7
		ie64Instr(OP_DCVTFI, 16, 0, 0, 4, 0, 0),  // NaN: 0 + IO
		ie64Instr(OP_DCVTFI, 17, 0, 0, 6, 0, 0),  // 2^63 exactly: interpreter quirk
		ie64Instr(OP_DCVTFI, 18, 0, 0, 8, 0, 0),  // > max: MaxInt64 + IO
		ie64Instr(OP_DCVTFI, 19, 0, 0, 10, 0, 0), // < min: MinInt64 + IO
	)
	diffRun(t, init, prog...)
}

func TestWasmJIT_HelperExit_FP64LoadStoreMMIO(t *testing.T) {
	mmio := uint64(0xF0048)
	init := map[int]uint64{2: mmio, 3: math.Float64bits(1.5), 4: 0x50000}
	// Seed D0 from RAM first, then DSTORE it to an MMIO page: helper exit
	// with the f64 bits in HelperVal.
	program := bytes.Join([][]byte{
		ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 4, 0, 0),
		ie64Instr(OP_DLOAD, 0, 0, 0, 4, 0, 0),
		ie64Instr(OP_DSTORE, 0, 0, 0, 2, 0, 0),
	}, nil)
	res := runWasmDiffBlock(t, program, init, markIOPage(t, mmio))
	if res.needHelper != HELPER_DSTORE {
		t.Fatalf("NeedHelper = %d, want HELPER_DSTORE", res.needHelper)
	}
	if res.helperVal != math.Float64bits(1.5) {
		t.Errorf("HelperVal = %#x, want bits of 1.5", res.helperVal)
	}
	if res.retCount != 2 {
		t.Errorf("RetCount = %d, want 2", res.retCount)
	}

	// DLOAD from MMIO: HELPER_DLOAD with the pair index in HelperRd.
	program = ie64Instr(OP_DLOAD, 4, 0, 0, 2, 0, 4)
	res = runWasmDiffBlock(t, program, init, markIOPage(t, mmio))
	if res.needHelper != HELPER_DLOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_DLOAD", res.needHelper)
	}
	if res.helperRd != 4 {
		t.Errorf("HelperRd = %d, want pair 4", res.helperRd)
	}
	if res.helperAddr != mmio+4 {
		t.Errorf("HelperAddr = %#x, want %#x", res.helperAddr, mmio+4)
	}
}

func TestWasmJIT_FP64InvalidPairRejected(t *testing.T) {
	// Odd pair index is an invalid encoding; the block must not compile
	// (the interpreter path reports the architectural fault).
	instrs := []JITInstr{{opcode: OP_DADD, rd: 3, rs: 0, rt: 2}}
	if _, err := wasmCompileBlock(instrs, PROG_START); err == nil {
		t.Fatal("block with odd FP pair compiled; want rejection")
	}
}

// ---------------------------------------------------------------------------
// Capability gate
// ---------------------------------------------------------------------------

func TestWasmJIT_UnsupportedOpcodeRejected(t *testing.T) {
	// OP_CAS is outside the milestone allowlist; the block must be rejected
	// before translation, never partially compiled.
	instrs := []JITInstr{
		{opcode: OP_ADD, rd: 1, size: IE64_SIZE_Q, xbit: 1, imm32: 1},
		{opcode: OP_CAS, rd: 2, rs: 3, rt: 4},
	}
	if _, err := wasmCompileBlock(instrs, PROG_START); err == nil {
		t.Fatal("block containing OP_CAS compiled; want unsupported-opcode error")
	}
	if _, err := wasmCompileBlock(nil, PROG_START); err == nil {
		t.Fatal("empty block compiled; want error")
	}
}
