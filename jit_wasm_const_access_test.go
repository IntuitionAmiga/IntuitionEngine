//go:build !js

package main

import "testing"

func TestWasmJIT_ConstLowRAMAccessElidesChecks(t *testing.T) {
	compile := func(op, rs byte) int {
		t.Helper()
		ins := JITInstr{opcode: op, rd: 1, rs: rs, size: IE64_SIZE_Q, imm32: 0x1000}
		mod, err := wasmCompileBlock([]JITInstr{ins}, PROG_START)
		if err != nil {
			t.Fatalf("compile opcode %#x rs %d: %v", op, rs, err)
		}
		return len(mod)
	}

	for _, op := range []byte{OP_LOAD, OP_STORE} {
		constant := compile(op, 0)
		dynamic := compile(op, 2)
		if constant+20 >= dynamic {
			t.Errorf("opcode %#x constant module = %d bytes, dynamic = %d; checks were not elided", op, constant, dynamic)
		}
	}
}
