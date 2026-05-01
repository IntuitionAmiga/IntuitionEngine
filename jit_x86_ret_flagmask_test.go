// jit_x86_ret_flagmask_test.go - regression cover for the RET cache-hit
// flag-restore mask.
//
// Reviewer P1 fix on the JIT-summit branch: the RET cache-hit chain JMP
// was restoring the entire guest Flags word into host RFLAGS via POPFQ,
// which leaked guest DF/TF/IOPL/etc. into host code. The fix restores
// only the visible condition bits (CF/PF/AF/ZF/SF/OF =
// x86VisibleFlagsMask) and re-applies the host's non-visible bits
// unchanged. These tests pin the masked-restore sequence so a future
// "simplify the flag dance" patch cannot silently revert to the unsafe
// POPFQ-of-everything pattern.
//
// Coverage:
//
//   - Structural: scan x86EmitRET's emitted bytes for the
//     mask/host-merge/POPFQ sequence (AND ~visible + AND visible + OR
//     + PUSH + POPFQ). The exact 4-byte AND-imm32 forms encode the
//     constants directly, so a regression that drops or replaces the
//     mask shows up immediately.
//
//   - Constants: assert x86VisibleFlagsMask retains its CF/PF/AF/ZF/SF/OF
//     value (0x08D5). A typo widening this mask would let DF (bit 10,
//     0x400) leak into host RFLAGS.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestX86RET_FlagMask_VisibleConstant pins the visible-flags mask. Adding
// DF (0x400) or TF (0x100) to this mask would re-introduce the leak the
// reviewer P1 fix closed.
func TestX86RET_FlagMask_VisibleConstant(t *testing.T) {
	const (
		CF = uint32(0x0001)
		PF = uint32(0x0004)
		AF = uint32(0x0010)
		ZF = uint32(0x0040)
		SF = uint32(0x0080)
		OF = uint32(0x0800)
	)
	want := CF | PF | AF | ZF | SF | OF // = 0x08D5
	if x86VisibleFlagsMask != want {
		t.Fatalf("x86VisibleFlagsMask = %#x, want %#x (CF|PF|AF|ZF|SF|OF). "+
			"Widening the mask leaks guest DF/TF/IOPL/system bits into "+
			"host RFLAGS via the RET cache-hit POPFQ path; do NOT add DF "+
			"(0x400) or TF (0x100) here without redesigning the merge.",
			x86VisibleFlagsMask, want)
	}
	// Sanity: DF and TF must NOT be set in the mask.
	const DF = uint32(0x0400)
	const TF = uint32(0x0100)
	if x86VisibleFlagsMask&DF != 0 {
		t.Errorf("x86VisibleFlagsMask includes DF — guest STD would set host direction flag, breaking MOVS/STOS in subsequent host code")
	}
	if x86VisibleFlagsMask&TF != 0 {
		t.Errorf("x86VisibleFlagsMask includes TF — guest TF=1 would single-step trap the host on the next instruction")
	}
}

// TestX86RET_FlagMask_InverseConstant pins the inverse mask used by the
// host-bits preservation step. A bug that miscomputes ~visible — for
// example using uint32(^visible) where the int32 cast truncates the
// high half — would corrupt the mask.
func TestX86RET_FlagMask_InverseConstant(t *testing.T) {
	want := -int32(x86VisibleFlagsMask) - 1 // two's-complement of ~mask
	if x86InvVisibleFlagsMaskI32 != want {
		t.Fatalf("x86InvVisibleFlagsMaskI32 = %d, want %d (= ^x86VisibleFlagsMask reinterpreted as int32)",
			x86InvVisibleFlagsMaskI32, want)
	}
	// Bit-pattern check: the int32 value should sign-extend to a 64-bit
	// constant whose low 32 bits are ~mask.
	asUint32 := uint32(x86InvVisibleFlagsMaskI32)
	if asUint32 != ^x86VisibleFlagsMask {
		t.Errorf("x86InvVisibleFlagsMaskI32 reinterpreted = %#x, want %#x", asUint32, ^x86VisibleFlagsMask)
	}
}

// TestX86EmitRET_FlagMaskSequence asserts the masked flag-restore
// sequence is present in x86EmitRET's emitted bytes. The sequence we
// require:
//
//	PUSHFQ                  (0x9C)
//	POP RAX                 (0x58)
//	AND EAX, ~visible       (0x25 imm32 = ~x86VisibleFlagsMask)
//	MOV ECX, [RSP+slot]     (somewhere; load saved guest flags)
//	AND ECX, visible        (0x81 0xE1 imm32 = x86VisibleFlagsMask)
//	OR EAX, ECX             (0x09 0xC8)
//	PUSH RAX                (0x50)
//	POPFQ                   (0x9D)
//
// We don't pin the exact byte offsets — the surrounding RET emit changes
// often — but we pin the existence of the AND-EAX-imm32 with the inverse
// mask AND the AND-ECX-imm32 with the visible mask in the same emitted
// buffer. If either disappears, the masked restore is gone.
func TestX86EmitRET_FlagMaskSequence(t *testing.T) {
	// Set up a minimal compile state so x86EmitRET's regMap-uint64
	// load (when re-enabled) and other state-dependent bits don't
	// crash. We don't need a fully-valid block; we only need
	// x86CurrentCS to be non-nil with a default regMap.
	cs := &x86CompileState{
		flagState:    x86FlagsDead,
		regMap:       x86DefaultRegMap(),
		flagsNeeded:  []bool{},
		flagShadowed: []bool{},
	}
	prevCS := x86CurrentCS
	x86CurrentCS = cs
	defer func() { x86CurrentCS = prevCS }()

	cb := &CodeBuffer{}
	x86EmitRET(cb, 1)
	emitted := cb.Bytes()

	// amd64ALU_reg_imm32_32bit emits the standard 0x81-form for imm32
	// values that don't fit in imm8 (~0xfffff72a does not fit in int8;
	// 0x08D5 also does not). The encoding is `[REX] 0x81 modRM imm32-le`
	// where modRM = 0xC0 | (aluOp<<3) | reg. For AND (aluOp=4) on EAX
	// (reg=0), modRM = 0xE0; on ECX (reg=1), modRM = 0xE1. RAX/RCX < 8
	// so no REX prefix.
	invMask := uint32(x86InvVisibleFlagsMaskI32)
	andEAXSeq := []byte{0x81, 0xE0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(andEAXSeq[2:], invMask)
	if !bytes.Contains(emitted, andEAXSeq) {
		t.Errorf("x86EmitRET output: missing 'AND EAX, ~x86VisibleFlagsMask' "+
			"(0x81 0xE0 imm32 with imm32=%#x). The host-RFLAGS preservation "+
			"step is gone; the chain JMP would write masked guest flags only "+
			"and clobber host system bits.", invMask)
	}

	visMask := x86VisibleFlagsMask
	andECXSeq := []byte{0x81, 0xE1, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(andECXSeq[2:], visMask)
	if !bytes.Contains(emitted, andECXSeq) {
		t.Errorf("x86EmitRET output: missing 'AND ECX, x86VisibleFlagsMask' "+
			"(0x81 0xE1 imm32 with imm32=%#x). The visible-bits selection step "+
			"is gone; non-visible guest flags (DF/TF/IOPL/etc.) would leak into "+
			"host RFLAGS via POPFQ.", visMask)
	}

	// PUSHFQ (0x9C) and POPFQ (0x9D) must both be present in the emitted
	// RET. PUSHFQ saves host RFLAGS; POPFQ installs the merged result.
	// The plain PUSH RAX (0x50) before POPFQ ensures the merged value
	// is what gets popped.
	if bytes.Count(emitted, []byte{0x9C}) < 1 {
		t.Error("x86EmitRET output: missing PUSHFQ (0x9C). Cannot read host RFLAGS to preserve system bits.")
	}
	if bytes.Count(emitted, []byte{0x9D}) < 1 {
		t.Error("x86EmitRET output: missing POPFQ (0x9D). Flags never installed into host RFLAGS.")
	}
}
