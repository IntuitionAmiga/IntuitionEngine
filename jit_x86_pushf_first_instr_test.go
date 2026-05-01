// jit_x86_pushf_first_instr_test.go — PUSHF first-instruction gate.
//
// PUSHF reads cpu.Flags directly. cpu.Flags is materialized only at
// unchained exit (full epilogue → MergeFlagsToGuest). Chained transitions
// (CALL/JMP/Jcc rel8 forward chain) skip the merge, so a chained-into
// block whose first instruction is PUSHF would observe stale cpu.Flags.
//
// The compile-side gate (in x86EmitPUSHF) bails when instrIdx==0 and
// flagState==Dead, forcing the dispatcher path which runs the merge
// before re-entering the block. This test pins the gate.
//
// Codex review (2026-04-30, post Jcc-rel8 chain landing) flagged the
// generalization: any cached block whose first instruction can observe
// host EFLAGS or cpu.Flags must not be reachable via chain unless the
// flag state has been materialized. Jcc/SETcc/CMOVcc are already gated
// by their own flagState!=Live bail. ADC/SBB/RCL/RCR restore CF from
// savedEFlags unconditionally. PUSHF is the remaining cpu.Flags reader;
// this gate closes the surface.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
)

// TestX86EmitPUSHF_BailsAsFirstInstr asserts that x86EmitPUSHF returns
// false (compile bail) when called as the first instruction of a block
// with flagState==Dead. Without the gate, chain-entry would observe
// stale cpu.Flags. The bail forces the block compile to error with
// "no instructions compiled", which the dispatcher catches at
// jit_x86_exec.go:203 and falls back to one-instruction Step — that
// path runs in the interpreter which always reads up-to-date cpu.Flags.
func TestX86EmitPUSHF_BailsAsFirstInstr(t *testing.T) {
	cb := &CodeBuffer{}
	cs := &x86CompileState{flagState: x86FlagsDead}
	ji := &X86JITInstr{opcode: 0x9C, length: 1}
	if x86EmitPUSHF(cb, ji, cs, 0) {
		t.Fatal("x86EmitPUSHF: expected bail (return false) at instrIdx=0 " +
			"with flagState=Dead — without gate, a chained-into block " +
			"starting with PUSHF would read stale cpu.Flags")
	}
	if cb.Len() != 0 {
		t.Fatalf("x86EmitPUSHF: expected no bytes emitted on bail, got %d", cb.Len())
	}
}

// TestX86EmitPUSHF_EmitsAtNonZeroIdx asserts the gate is narrow: PUSHF
// at instrIdx>0 emits normally regardless of flagState. The prior
// in-block instruction's compile path is responsible for cpu.Flags
// freshness when needed — the gate is only needed at chain boundaries.
func TestX86EmitPUSHF_EmitsAtNonZeroIdx(t *testing.T) {
	cb := &CodeBuffer{}
	cs := &x86CompileState{flagState: x86FlagsDead}
	ji := &X86JITInstr{opcode: 0x9C, length: 1}
	if !x86EmitPUSHF(cb, ji, cs, 1) {
		t.Fatal("x86EmitPUSHF: expected emit (return true) at instrIdx=1 — " +
			"gate is meant to only fire on first instruction")
	}
	if cb.Len() == 0 {
		t.Fatal("x86EmitPUSHF: instrIdx=1 returned true but emitted 0 bytes")
	}
}

// TestX86EmitPUSHF_EmitsWhenFlagStateLive asserts the gate is narrow on
// flagState too: PUSHF at instrIdx==0 with flagState==Live (e.g. inline
// fallback path that materialized flags) emits normally. Live flag state
// is only achievable mid-block in normal compile, but the test pins the
// contract for any future emit path that pre-establishes flagState.
func TestX86EmitPUSHF_EmitsWhenFlagStateLive(t *testing.T) {
	cb := &CodeBuffer{}
	cs := &x86CompileState{flagState: x86FlagsLiveArith}
	ji := &X86JITInstr{opcode: 0x9C, length: 1}
	if !x86EmitPUSHF(cb, ji, cs, 0) {
		t.Fatal("x86EmitPUSHF: expected emit (return true) at instrIdx=0 " +
			"with flagState=LiveArith — gate fires only on Dead state")
	}
}
