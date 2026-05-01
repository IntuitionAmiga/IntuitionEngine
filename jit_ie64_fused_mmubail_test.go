// jit_ie64_fused_mmubail_test.go - regression cover for the IE64 fused
// JSR/RTS leaf-pair MMU-bail honor.
//
// Reviewer P2 fix on the JIT-summit branch: the fused-leaf fast path
// at jit_emit_amd64.go:emitInstruction emits raw [MemBase+SP] stack
// pushes/pops to preserve architectural stack traffic for fused
// JSR/RTS pairs. Under MMU-on execution, compileBlockMMU sets
// ji.mmuBail=true on every OP_JSR64 / OP_RTS64; the unfused case
// short-circuits to emitBailToInterpreter, but the fused case
// previously checked only fusedFlag and silently bypassed mmuBail —
// performing direct, untranslated stack accesses against high-VA
// stacks. The fix gates both fused branches on `!ji.mmuBail` so a
// fused leaf in an MMU block falls through to the OP_JSR64 / OP_RTS64
// switch case, which then bails properly.
//
// This test pins that gate by emitting a fused-leaf instruction with
// mmuBail=true and asserting the emitted byte sequence does NOT
// contain the raw stack push/pop pattern. It is a structural / shape
// test rather than a runtime exec test because the fused-leaf path
// has no observable guest-state side effects on its own — the bug
// manifests only when the host-physical stack happens to alias
// translated guest VA, which requires a full MMU walk fixture.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"testing"
)

// emitFusedJSR_NonMMU returns the bytes the fast path emits when
// mmuBail is false. We capture this once as the "raw stack push"
// shape, then verify the mmuBail=true path does NOT match it.
func emitFusedJSR_NonMMU(t *testing.T) []byte {
	t.Helper()
	cb := &CodeBuffer{}
	ji := &JITInstr{
		opcode:    OP_JSR64,
		pcOffset:  0,
		fusedFlag: ie64FusedJSRLeafCall,
		mmuBail:   false,
	}
	br := &blockRegs{}
	emitInstruction(cb, ji, 0x1000, false, br, 0, 0, nil, nil)
	return cb.Bytes()
}

// TestIE64Fused_JSRLeaf_HonorsMMUBail asserts that a fused JSR-leaf
// with mmuBail=true does NOT emit the raw stack-push fast path.
//
// We compare the fused-with-mmuBail emit against the fused-without-
// mmuBail emit. If they're byte-equal, the gate is broken — both
// would skip the OP_JSR64 case's emitBailToInterpreter call and
// silently access guest stack memory under the MMU.
func TestIE64Fused_JSRLeaf_HonorsMMUBail(t *testing.T) {
	rawShape := emitFusedJSR_NonMMU(t)
	if len(rawShape) == 0 {
		t.Fatal("control: fused-JSR-no-mmu emitted no bytes; cannot baseline")
	}

	cb := &CodeBuffer{}
	ji := &JITInstr{
		opcode:    OP_JSR64,
		pcOffset:  0,
		fusedFlag: ie64FusedJSRLeafCall,
		mmuBail:   true,
	}
	br := &blockRegs{}
	emitInstruction(cb, ji, 0x1000, false, br, 0, 0, nil, nil)
	withBail := cb.Bytes()

	if bytes.Equal(rawShape, withBail) {
		t.Fatalf("emitInstruction(JSR fused, mmuBail=true) produced the same bytes as mmuBail=false (%d bytes). "+
			"The fused fast path is bypassing the MMU bail; under MMU-on execution a fused JSR leaf will "+
			"perform a raw [MemBase+SP] write that skips the uint64 VA walk and can alias high-VA stacks.",
			len(rawShape))
	}
}

// TestIE64Fused_RTSLeaf_HonorsMMUBail is the symmetric check for
// fused-RTS-leaf marker.
func TestIE64Fused_RTSLeaf_HonorsMMUBail(t *testing.T) {
	cbA := &CodeBuffer{}
	jiA := &JITInstr{
		opcode:    OP_RTS64,
		pcOffset:  4,
		fusedFlag: ie64FusedRTSLeafReturn,
		mmuBail:   false,
	}
	br := &blockRegs{}
	emitInstruction(cbA, jiA, 0x1000, false, br, 0, 1, nil, nil)
	rawShape := cbA.Bytes()
	if len(rawShape) == 0 {
		t.Fatal("control: fused-RTS-no-mmu emitted no bytes; cannot baseline")
	}

	cbB := &CodeBuffer{}
	jiB := &JITInstr{
		opcode:    OP_RTS64,
		pcOffset:  4,
		fusedFlag: ie64FusedRTSLeafReturn,
		mmuBail:   true,
	}
	emitInstruction(cbB, jiB, 0x1000, false, br, 0, 1, nil, nil)
	withBail := cbB.Bytes()

	if bytes.Equal(rawShape, withBail) {
		t.Fatalf("emitInstruction(RTS fused, mmuBail=true) produced the same bytes as mmuBail=false (%d bytes). "+
			"Fused RTS leaf is bypassing the MMU bail — the [MemBase+SP] read skips uint64 VA translation.",
			len(rawShape))
	}
}
