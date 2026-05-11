package main

import (
	"testing"
)

// Phase 6 — MOVES / CALLM / RTM / RETM (68020 supervisor + module-call).
//
// No generic user-mode IE64 analog for the dropped semantics (alternate
// function-code space, module descriptors). Approximate lowerings:
//   MOVES.<sz> <ea>,Rn  → MOVE.<sz> <ea>,Rn      (FC space dropped)
//   MOVES.<sz> Rn,<ea>  → MOVE.<sz> Rn,<ea>
//   CALLM #n,<ea>       → JSR <ea>               (module descriptor dropped)
//   RTM Rn              → RTS                    (Rn ignored)
//   RETM #n             → RTS + sp adjust by #n
// All emit ⚠️ diagnostics; per Strict-mode policy, none error under -strict.

func TestMOVES_L_LoadFromIndirect(t *testing.T) {
	out := convertOneInstr(t, "\tmoves.l (a0),d1")
	mustContain(t, out, "load.l ")
	mustContain(t, out, "MOVES")
	mustContain(t, out, "FC-space dropped")
}

func TestMOVES_W_StoreToIndirect(t *testing.T) {
	out := convertOneInstr(t, "\tmoves.w d0,(a1)")
	mustContain(t, out, "store.w ")
	mustContain(t, out, "MOVES")
}

func TestCALLM_LowersToJSR(t *testing.T) {
	out := convertOneInstr(t, "\tcallm #4,(a0)")
	mustContain(t, out, "CALLM")
	mustContain(t, out, "module descriptor")
	// JSR lowering: push 4-byte return PC, jmp via target. Verify lowering shape.
	mustContain(t, out, "store.l ")
	mustContain(t, out, "jmp ")
}

func TestRTM_LowersToRTS(t *testing.T) {
	out := convertOneInstr(t, "\trtm d0")
	mustContain(t, out, "RTM")
	// RTS lowering pops 4-byte return from r30 and jumps.
	mustContain(t, out, "load.l ")
	mustContain(t, out, "jmp ")
}

func TestRETM_LowersToRTSWithStackAdjust(t *testing.T) {
	out := convertOneInstr(t, "\tretm #8")
	mustContain(t, out, "RETM")
	mustContain(t, out, "load.l ")
	mustContain(t, out, "jmp ")
	mustContain(t, out, "add.l r30, r30, #8") // stack frame teardown
}
