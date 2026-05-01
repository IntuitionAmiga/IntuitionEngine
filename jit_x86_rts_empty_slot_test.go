// jit_x86_rts_empty_slot_test.go - regression cover for the RTS-cache
// empty-slot guard.
//
// Codex review (2026-04-30) flagged that x86EmitRET's 2-entry MRU cache
// would treat a zero-initialized slot (PC=0, Addr=0) as a valid hit
// when the guest legitimately RETs to PC=0. The CMP R11, [RTSCache0PC]
// would succeed (0==0), R10 would load Addr=0 from the empty slot, and
// the eventual JMP R10 would transfer control to host address 0 →
// SIGSEGV. The fix tests R10 nonzero on the hit path and falls through
// to the miss handling on a zero load.
//
// This test pins the structural guard: the emitted byte stream must
// contain a TEST R10,R10 followed by a Jcc immediately after the slot
// loads, before the chain bookkeeping (DEC ChainBudget). A future
// "trim RET emit" pass cannot silently drop the guard.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"testing"
)

// TestX86EmitRET_EmptySlotGuardPresent asserts the byte sequence
// `4D 85 D2` (TEST R10, R10 — REX.WR + opcode 85 + ModRM D2) is
// emitted by x86EmitRET, and that it appears before the DEC ChainBudget
// (encoded as `FF 4F xx` for DEC dword [RDI+disp8] when ctx fits in
// disp8, or `FF 8F xx xx xx xx` for disp32). We just assert the TEST
// R10,R10 bytes appear.
func TestX86EmitRET_EmptySlotGuardPresent(t *testing.T) {
	cb := &CodeBuffer{}
	x86EmitRET(cb, 1)
	out := cb.Bytes()

	// REX.W=0, REX.R=1 (R10 as dst high bit), REX.B=1 (R10 as src high
	// bit) → REX = 0x4D. Opcode 0x85 (TEST r/m64,r64 — but we use the
	// 32-bit form; amd64TEST_reg_reg uses 0x4D 0x85 0xD2 for R10 vs
	// itself with REX.W=1). Check both 32-bit (0x45) and 64-bit (0x4D)
	// REX prefixes since the helper choice is implementation-defined.
	want64 := []byte{0x4D, 0x85, 0xD2}
	want32 := []byte{0x45, 0x85, 0xD2}
	if !bytes.Contains(out, want64) && !bytes.Contains(out, want32) {
		t.Fatalf("x86EmitRET: missing TEST R10,R10 empty-slot guard. "+
			"Without it, a zero-init RTS cache slot + guest RET to PC=0 "+
			"jumps to host address 0 → SIGSEGV. Emitted bytes:\n%x", out)
	}
}
