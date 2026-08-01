// jit_x86_pushf_first_instr_test.go - PUSHF native admission.
//
// PUSHF reads the JIT frame's saved EFLAGS rather than cpu.Flags. That makes a
// first instruction safe after a chain transition, where cpu.Flags need not
// yet be materialised.
//
// The emitter validates the prospective stack span before committing ESP and
// uses the standard self-modifying-code check after the store.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
)

func TestX86EmitPUSHF_EmitsAsFirstInstr(t *testing.T) {
	cb := &CodeBuffer{}
	cs := &x86CompileState{flagState: x86FlagsDead}
	ji := &X86JITInstr{opcode: 0x9C, length: 1}
	if !x86EmitPUSHF(cb, ji, cs, 0) {
		t.Fatal("x86EmitPUSHF: first instruction must emit from saved EFLAGS")
	}
	if cb.Len() == 0 {
		t.Fatal("x86EmitPUSHF emitted no bytes")
	}
}

func TestX86EmitPUSHF_EmitsAtNonZeroIdx(t *testing.T) {
	cb := &CodeBuffer{}
	cs := &x86CompileState{flagState: x86FlagsDead}
	ji := &X86JITInstr{opcode: 0x9C, length: 1}
	if !x86EmitPUSHF(cb, ji, cs, 1) {
		t.Fatal("x86EmitPUSHF: expected emit at instrIdx=1")
	}
	if cb.Len() == 0 {
		t.Fatal("x86EmitPUSHF: instrIdx=1 returned true but emitted 0 bytes")
	}
}

func TestX86EmitPUSHF_EmitsWhenFlagStateLive(t *testing.T) {
	cb := &CodeBuffer{}
	cs := &x86CompileState{flagState: x86FlagsLiveArith}
	ji := &X86JITInstr{opcode: 0x9C, length: 1}
	if !x86EmitPUSHF(cb, ji, cs, 0) {
		t.Fatal("x86EmitPUSHF: expected emit at instrIdx=0 with live flags")
	}
}
