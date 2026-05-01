// ie32_benchmark_test.go - IE32 interpreter microbenchmarks.
//
// Mirrors the uniform shape used by the other CPU backends so the
// build_all_cpu_benchmarks.sh + run_all_cpu_benches.sh pivot table can
// include an IE32 row. IE32 has no JIT (per CLAUDE.md), so only the
// _Interpreter variant is provided; the table's JIT column shows "-"
// for IE32.
//
// Each bench:
//   - Constructs a small IE32 program ending in HALT.
//   - Resets PC + running state per b.Loop iter.
//   - Calls cpu.Execute() (the unsafe-pointer fast interpreter loop).
//   - Reports `instructions/op` + `MIPS_host` so the awk pivot in
//     run_all_cpu_benches.sh picks them up the same way as the other
//     backends.
//
// IE32 instruction layout (8 bytes):
//   byte 0 : opcode
//   byte 1 : destination register index (0-15: A,X,Y,Z,B,C,D,E,F,G,H,S,T,U,V,W)
//   byte 2 : addressing mode (ADDR_IMMEDIATE/REGISTER/REG_IND/MEM_IND/DIRECT)
//   byte 3 : padding
//   bytes 4-7 : 32-bit operand (little-endian)
//
// Programs load at PROG_START (0x1000) and must fit within
// [PROG_START, STACK_START).

package main

import (
	"encoding/binary"
	"io"
	"os"
	"testing"
)

const (
	ie32BenchRegA = 0
	ie32BenchRegX = 1
	ie32BenchRegB = 4
	ie32BenchRegC = 5
)

// ie32WriteInstr encodes one 8-byte IE32 instruction at offset off
// within prog.
func ie32WriteInstr(prog []byte, off int, opcode, reg, addrMode byte, operand uint32) {
	prog[off] = opcode
	prog[off+1] = reg
	prog[off+2] = addrMode
	prog[off+3] = 0
	binary.LittleEndian.PutUint32(prog[off+4:off+8], operand)
}

// ie32SetupBench builds a CPU + bus, loads program, returns cpu. Caller
// resets PC + running between b.Loop iterations.
func ie32SetupBench(b *testing.B, program []byte) *CPU {
	b.Helper()
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	cpu.LoadProgramBytes(program)
	return cpu
}

// ie32SilenceStdout redirects os.Stdout to /dev/null for the duration
// of the bench. Necessary because cpu_ie32.go's HALT case prints
// "HALT executed at PC=..." unconditionally — without redirection a
// 3-second bench dumps millions of lines to the terminal.
func ie32SilenceStdout(b *testing.B) {
	b.Helper()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatalf("open /dev/null: %v", err)
	}
	old := os.Stdout
	os.Stdout = devnull
	b.Cleanup(func() {
		os.Stdout = old
		_ = devnull.Close()
	})
	// Drain anything already buffered by tests upstream.
	_, _ = io.Discard.Write(nil)
}

// ie32BenchALU: tight ADD/SUB/AND/XOR loop. Counter in B decrements
// to zero, JNZ loops, then HALT.
//
// Body per iter (6 instructions, all retired every iter including the
// final JNZ that fails on the last iter and falls through to HALT):
//
//	ADD A,#1 ; SUB A,#1 ; AND A,#FF ; XOR A,#0 ; SUB B,#1 ; JNZ B,loop
//
// Total: 1 LOAD B,#iters + 6*256 body + 1 HALT.
const ie32ALUBodyOpsPerIter = 6
const ie32ALULoopIters = 256
const ie32ALUInstrCount = 1 /* LOAD */ + ie32ALUBodyOpsPerIter*ie32ALULoopIters + 1 /* HALT */

func ie32BuildALUProgram() []byte {
	const bodyStart = 8 // PROG_START + 8
	const bodyOffset = bodyStart - PROG_START
	prog := make([]byte, 1<<13)
	off := 0
	// LOAD B, #256
	ie32WriteInstr(prog, off, LOAD, ie32BenchRegB, ADDR_IMMEDIATE, ie32ALULoopIters)
	off += INSTRUCTION_SIZE
	loopAddr := uint32(PROG_START) + uint32(off)
	// Body (5 ALU + 1 counter SUB):
	ie32WriteInstr(prog, off, ADD, ie32BenchRegA, ADDR_IMMEDIATE, 1)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, SUB, ie32BenchRegA, ADDR_IMMEDIATE, 1)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, AND, ie32BenchRegA, ADDR_IMMEDIATE, 0xFF)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, XOR, ie32BenchRegA, ADDR_IMMEDIATE, 0)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, SUB, ie32BenchRegB, ADDR_IMMEDIATE, 1)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, JNZ, ie32BenchRegB, ADDR_IMMEDIATE, loopAddr)
	off += INSTRUCTION_SIZE
	// HALT
	ie32WriteInstr(prog, off, HALT, 0, 0, 0)
	off += INSTRUCTION_SIZE
	_ = bodyOffset
	return prog[:off]
}

func BenchmarkIE32_ALU_Interpreter(b *testing.B) {
	ie32SilenceStdout(b)
	program := ie32BuildALUProgram()
	cpu := ie32SetupBench(b, program)

	for b.Loop() {
		cpu.PC = PROG_START
		cpu.A = 0
		cpu.B = 0
		cpu.running.Store(true)
		cpu.Execute()
	}
	totalInstrs := ie32ALUInstrCount
	b.ReportMetric(float64(totalInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, totalInstrs)
}

// ie32BenchMemory: load + store loop over a 1KB scratch region.
//
//	LOAD X, #0
//	loop: LOAD A, [scratch + X*?]   — IE32 lacks scaled indexing here, so:
//	                                  we use ADDR_DIRECT with rotating address
//	STORE A to scratch
//	ADD X, #4
//	SUB B, #1
//	JNZ B, loop
//	HALT
//
// Simpler: 256 iters of LOAD/STORE/ADD/SUB/JNZ on fixed addresses.
const ie32MemLoopIters = 256
const ie32MemBodyOps = 5

func ie32BuildMemProgram() []byte {
	prog := make([]byte, 1<<13)
	const scratchAddr = 0x4000 // safely below STACK_START
	off := 0
	// LOAD B, #iters
	ie32WriteInstr(prog, off, LOAD, ie32BenchRegB, ADDR_IMMEDIATE, ie32MemLoopIters)
	off += INSTRUCTION_SIZE
	loopAddr := uint32(PROG_START) + uint32(off)
	// Body: LOAD A,[scratch] ; STORE A->[scratch] ; ADD A,#1 ; SUB B,#1 ; JNZ B,loop
	ie32WriteInstr(prog, off, LOAD, ie32BenchRegA, ADDR_DIRECT, scratchAddr)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, STORE, ie32BenchRegA, ADDR_DIRECT, scratchAddr)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, ADD, ie32BenchRegA, ADDR_IMMEDIATE, 1)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, SUB, ie32BenchRegB, ADDR_IMMEDIATE, 1)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, JNZ, ie32BenchRegB, ADDR_IMMEDIATE, loopAddr)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, HALT, 0, 0, 0)
	off += INSTRUCTION_SIZE
	return prog[:off]
}

func BenchmarkIE32_Memory_Interpreter(b *testing.B) {
	ie32SilenceStdout(b)
	program := ie32BuildMemProgram()
	cpu := ie32SetupBench(b, program)

	for b.Loop() {
		cpu.PC = PROG_START
		cpu.A = 0
		cpu.B = 0
		cpu.running.Store(true)
		cpu.Execute()
	}
	totalInstrs := 1 + ie32MemBodyOps*ie32MemLoopIters + 1
	b.ReportMetric(float64(totalInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, totalInstrs)
}

// ie32BenchMixed: ALU + memory + branch mix. 4 ALU + 1 LOAD + 1 STORE +
// 1 SUB counter + 1 JNZ per iter, 256 iters.
const ie32MixedBodyOps = 7
const ie32MixedLoopIters = 256

func ie32BuildMixedProgram() []byte {
	prog := make([]byte, 1<<13)
	const scratchAddr = 0x4100
	off := 0
	ie32WriteInstr(prog, off, LOAD, ie32BenchRegB, ADDR_IMMEDIATE, ie32MixedLoopIters)
	off += INSTRUCTION_SIZE
	loopAddr := uint32(PROG_START) + uint32(off)
	ie32WriteInstr(prog, off, LOAD, ie32BenchRegA, ADDR_DIRECT, scratchAddr)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, ADD, ie32BenchRegA, ADDR_IMMEDIATE, 7)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, XOR, ie32BenchRegA, ADDR_IMMEDIATE, 0x55)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, AND, ie32BenchRegA, ADDR_IMMEDIATE, 0xFF)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, STORE, ie32BenchRegA, ADDR_DIRECT, scratchAddr)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, SUB, ie32BenchRegB, ADDR_IMMEDIATE, 1)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, JNZ, ie32BenchRegB, ADDR_IMMEDIATE, loopAddr)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, HALT, 0, 0, 0)
	off += INSTRUCTION_SIZE
	return prog[:off]
}

func BenchmarkIE32_Mixed_Interpreter(b *testing.B) {
	ie32SilenceStdout(b)
	program := ie32BuildMixedProgram()
	cpu := ie32SetupBench(b, program)

	for b.Loop() {
		cpu.PC = PROG_START
		cpu.A = 0
		cpu.B = 0
		cpu.running.Store(true)
		cpu.Execute()
	}
	totalInstrs := 1 + ie32MixedBodyOps*ie32MixedLoopIters + 1
	b.ReportMetric(float64(totalInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, totalInstrs)
}

// ie32BenchCall: JSR / RTS in a loop. Each iteration does:
//
//	ADD A, #1   (in callee)
//	RTS
//
// Caller: JSR target ; SUB B,#1 ; JNZ B,loop ; HALT
//
// Per iter: 4 instrs (JSR + ADD + RTS + SUB + JNZ = 5; the "ADD" is
// inside the callee). Counted as 5 body ops × 256 iters.
const ie32CallBodyOps = 5
const ie32CallLoopIters = 256

func ie32BuildCallProgram() []byte {
	prog := make([]byte, 1<<13)
	off := 0
	// LOAD B, #iters
	ie32WriteInstr(prog, off, LOAD, ie32BenchRegB, ADDR_IMMEDIATE, ie32CallLoopIters)
	off += INSTRUCTION_SIZE
	loopAddr := uint32(PROG_START) + uint32(off)
	// Reserve PC for the JSR; we patch its operand once we know the
	// callee address.
	jsrSiteOff := off
	ie32WriteInstr(prog, off, JSR, 0, ADDR_IMMEDIATE, 0)
	off += INSTRUCTION_SIZE
	// SUB B, #1
	ie32WriteInstr(prog, off, SUB, ie32BenchRegB, ADDR_IMMEDIATE, 1)
	off += INSTRUCTION_SIZE
	// JNZ B, loop
	ie32WriteInstr(prog, off, JNZ, ie32BenchRegB, ADDR_IMMEDIATE, loopAddr)
	off += INSTRUCTION_SIZE
	// HALT
	ie32WriteInstr(prog, off, HALT, 0, 0, 0)
	off += INSTRUCTION_SIZE
	// Callee starts here: ADD A,#1 ; RTS
	calleeAddr := uint32(PROG_START) + uint32(off)
	ie32WriteInstr(prog, off, ADD, ie32BenchRegA, ADDR_IMMEDIATE, 1)
	off += INSTRUCTION_SIZE
	ie32WriteInstr(prog, off, RTS, 0, 0, 0)
	off += INSTRUCTION_SIZE
	// Patch JSR operand with callee address.
	binary.LittleEndian.PutUint32(prog[jsrSiteOff+4:jsrSiteOff+8], calleeAddr)
	return prog[:off]
}

func BenchmarkIE32_Call_Interpreter(b *testing.B) {
	ie32SilenceStdout(b)
	program := ie32BuildCallProgram()
	cpu := ie32SetupBench(b, program)

	for b.Loop() {
		cpu.PC = PROG_START
		cpu.A = 0
		cpu.B = 0
		cpu.SP = STACK_START
		cpu.running.Store(true)
		cpu.Execute()
	}
	totalInstrs := 1 + ie32CallBodyOps*ie32CallLoopIters + 1
	b.ReportMetric(float64(totalInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, totalInstrs)
}
