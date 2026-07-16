//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import "testing"

func buildIE64BoundedLoop(count uint32) func([]byte) {
	return func(mem []byte) {
		pc := uint64(PROG_START)
		put := func(ins []byte) { copy(mem[pc:], ins); pc += IE64_INSTR_SIZE }
		put(ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, count))
		put(ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 3, 0, 7))
		put(ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
		back := int32(-16)
		put(ie64Instr(OP_BNE, 0, 0, 0, 2, 0, uint32(back)))
		put(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
}

func TestJIT_vs_Interpreter_BoundedCounterLoop(t *testing.T) {
	for _, count := range []uint32{1, 2, (ie64JITLoopBudget - 1) / 3} {
		assertFPParity(t, "bounded", buildIE64BoundedLoop(count))
	}
}

func TestJIT_vs_Interpreter_InvariantMemoryLoop(t *testing.T) {
	build := func(mem []byte) {
		pc := uint64(PROG_START)
		put := func(x []byte) { copy(mem[pc:], x); pc += 8 }
		put(ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 1, 0, 0, 0x3000))
		put(ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 3))
		put(ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 0, 5, 0, 0))
		put(ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 3, 0, 1))
		put(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 5, 0, 0))
		put(ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
		back := int32(-32)
		put(ie64Instr(OP_BNE, 0, 0, 0, 2, 0, uint32(back)))
		put(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	assertFPParity(t, "invariant-memory", build)
}
