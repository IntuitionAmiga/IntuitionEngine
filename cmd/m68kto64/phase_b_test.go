package main

import "testing"

// =====================================================================
// TRAPV
// =====================================================================

func TestTrapV_GuardsOnShadowV(t *testing.T) {
	out := convertSrc(t, "\ttrapv\n")
	mustContain(t, out, "beqz r27") // skip if V=0
	mustContain(t, out, "syscall #18")
}

// =====================================================================
// BCD: ABCD / SBCD / NBCD
// =====================================================================

func TestAbcd_DnDn(t *testing.T) {
	out := convertSrc(t, "\tabcd d0,d1\n")
	// Adds dst+src+X (shadow C); BCD low/high adjust.
	mustContain(t, out, "and.l r19, r2, #$FF") // dst masked
	mustContain(t, out, "and.l r17, r1, #$FF") // src masked
	mustContain(t, out, "add.l r19, r19, r17")
	mustContain(t, out, "add.l r19, r19, r28") // + X (ShadowX = r28)
}

func TestAbcd_PreDecPreDec(t *testing.T) {
	out := convertSrc(t, "\tabcd -(a0),-(a1)\n")
	mustContain(t, out, "sub.l r9, r9, #1")  // src predec
	mustContain(t, out, "sub.l r10, r10, #1") // dst predec
	mustContain(t, out, "store.b r19, (r10)")
}

func TestSbcd_DnDn(t *testing.T) {
	out := convertSrc(t, "\tsbcd d0,d1\n")
	mustContain(t, out, "sub.l r19, r19, r17")
	mustContain(t, out, "sub.l r19, r19, r28") // chain-in X (ShadowX = r28)
}

func TestNbcd_Dn(t *testing.T) {
	out := convertSrc(t, "\tnbcd d0\n")
	mustContain(t, out, "move.l r17, #0")
	mustContain(t, out, "sub.l r19, r17, r19") // 0 - dst
}

// =====================================================================
// PACK / UNPK
// =====================================================================

func TestPack_DnDn(t *testing.T) {
	out := convertSrc(t, "\tpack d0,d1,#$0000\n")
	mustContain(t, out, "and.l r19, r1, #$FFFF")
	mustContain(t, out, "lsr.l r17, r19, #4")
	mustContain(t, out, "and.l r17, r17, #$F0")
}

func TestUnpk_DnDn(t *testing.T) {
	out := convertSrc(t, "\tunpk d0,d1,#$0000\n")
	mustContain(t, out, "and.l r19, r1, #$FF")
	mustContain(t, out, "lsl.l r17, r17, #4")
}

// =====================================================================
// CAS
// =====================================================================

func TestCas_LoadCmpStore(t *testing.T) {
	out := convertSrc(t, "\tcas.l d0,d1,(a0)\n")
	mustContain(t, out, "load.l r18, (r16)")
	mustContain(t, out, "store.l r2, (r16)") // store Du on equal
	mustContain(t, out, "move.l r25, #0")    // Z set on equal
	mustContain(t, out, "move.l r25, #1")    // Z cleared on neq
}

// =====================================================================
// Bit-field ops
// =====================================================================

func TestBfins_Dn(t *testing.T) {
	out := convertSrc(t, "\tbfins d0,d1{#0:#8}\n")
	mustContain(t, out, "and.l r2, r2") // mask off field
	mustContain(t, out, "or.l r2, r2, r19")
}

func TestBfclr_Dn(t *testing.T) {
	out := convertSrc(t, "\tbfclr d0{#0:#4}\n")
	mustContain(t, out, "not.l r18, r18")
	mustContain(t, out, "and.l r1, r1, r18")
}

func TestBfset_Dn(t *testing.T) {
	out := convertSrc(t, "\tbfset d0{#4:#4}\n")
	mustContain(t, out, "or.l r1, r1, r18")
}

func TestBfchg_Dn(t *testing.T) {
	out := convertSrc(t, "\tbfchg d0{#0:#1}\n")
	mustContain(t, out, "eor.l r1, r1, r18")
}

func TestBftst_Dn_SetsShadows(t *testing.T) {
	out := convertSrc(t, "\tbftst d0{#0:#8}\n")
	// Extracts field, then runs shadow-NZ-from-result.
	mustContain(t, out, "sext.l r24, r19")
	mustContain(t, out, "move.l r25, r19")
}

func TestBfffo_Dn(t *testing.T) {
	out := convertSrc(t, "\tbfffo d0{#0:#8},d1\n")
	mustContain(t, out, "clz r2, r19")
}
