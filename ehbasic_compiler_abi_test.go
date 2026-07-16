package main

import "testing"

// TestIE64BasicCompilerABIStackContract fixes the stack convention used by the
// replacement compiler before its first typed helper is introduced. IE64 JSR,
// RTS, PUSH and POP each move R31 by eight bytes. Consequently a compiled call
// pads by one qword before JSR so the helper observes a 16-byte-aligned entry
// stack. Stack arguments remain right-to-left 16-byte slots above that pad.
func TestIE64BasicCompilerABIStackContract(t *testing.T) {
	asmBin := buildAssembler(t)
	body := `    move.q  r18, #0x11223344
    move.q  r6, r31
    push    r0                      ; ABI call pad
    move.q  r7, r31
    jsr     .helper
    move.q  r9, r31
    pop     r1                      ; remove call pad
    la      r2, 0x030000
    store.q r6, 0(r2)              ; caller SP before pad
    store.q r7, 8(r2)              ; caller SP immediately before JSR
    store.q r8, 16(r2)             ; helper entry SP
    store.q r9, 24(r2)             ; caller SP immediately after RTS
    store.q r31, 32(r2)            ; balanced final SP
    store.q r1, 40(r2)             ; pad value restored by POP
    bra     .done
.helper:
    move.q  r8, r31
    push    r18
    move.q  r18, #0
    pop     r18
    rts
.done:`

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(1_000_000)

	before := h.bus.Read64(0x030000)
	beforeJSR := h.bus.Read64(0x030008)
	entry := h.bus.Read64(0x030010)
	afterRTS := h.bus.Read64(0x030018)
	final := h.bus.Read64(0x030020)
	pad := h.bus.Read64(0x030028)

	if before%16 != 0 {
		t.Fatalf("initial R31 = %#x, want 16-byte alignment", before)
	}
	if beforeJSR != before-8 {
		t.Fatalf("PUSH effect: R31 = %#x, want %#x", beforeJSR, before-8)
	}
	if entry != before-16 || entry%16 != 0 {
		t.Fatalf("JSR helper entry R31 = %#x, want aligned %#x", entry, before-16)
	}
	if afterRTS != beforeJSR {
		t.Fatalf("RTS effect: R31 = %#x, want %#x", afterRTS, beforeJSR)
	}
	if final != before {
		t.Fatalf("balanced call R31 = %#x, want %#x", final, before)
	}
	if pad != 0 {
		t.Fatalf("POP restored pad %#x, want zero", pad)
	}
	if got := h.cpu.regs[18]; got != 0x11223344 {
		t.Fatalf("callee-saved register R18 = %#x, want %#x", got, uint64(0x11223344))
	}
}
