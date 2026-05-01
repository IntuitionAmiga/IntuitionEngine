// jit_x86_adc_sbb_emit_test.go - regression cover for the ADC/SBB
// emit-side correctness fixes.
//
// Reviewer P1 #1: ADC/SBB emitters were running the host ADC/SBB op
// with whatever stale host CF was left by JIT bookkeeping (chain-budget
// DEC, NeedInval CMP, the per-instr flag-capture sequence itself). The
// guest CF lives in the savedEFlags stack slot, not host RFLAGS, so
// the guest's architectural CF input was being silently dropped. Fix:
// emit `BT [RSP+slot], 0` before the ALU on every ADC/SBB path, which
// loads guest CF from bit 0 of the slot into host RFLAGS' CF.
//
// Reviewer P1 #2: high-byte Grp1 Eb,Ib (e.g. ADD AH,1, SBB BH,1) ran
// the byte ALU on R8b — correctly setting host flags from the guest
// byte result — then immediately ran an AND/AND/SHL/OR sequence to
// merge the byte back into the full 32-bit guest reg. The merge
// clobbered host EFLAGS, and the compile loop's generic capture
// recorded merge bookkeeping flags instead of the guest ALU's. Fix:
// capture inside the high-byte branch immediately after the byte
// ALU, before the merge, with cs.flagCaptureDone=true so the generic
// capture skips this instruction.
//
// These tests pin the structural shape of both fixes by scanning
// emitted bytes for the BT pattern and the post-ALU capture call.
//
// Encoding cheatsheet:
//   - BT m32, imm8 = 0x0F 0xBA /4 [SIB+disp32] imm8.
//     For [RSP+disp32]: ModRM=0x84, SIB=0x24, disp32-le, imm8=00.
//   - PUSHFQ = 0x9C; POP = 0x58 (RAX); MOV [RSP+disp], EAX = pattern.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// btCFRestoreSequence returns the byte sequence x86EmitRestoreGuestCF
// emits: 0F BA 84 24 [disp32-le] 00. Tests scan for this in emitter
// output to confirm the CF restore landed.
func btCFRestoreSequence() []byte {
	seq := []byte{0x0F, 0xBA, 0x84, 0x24, 0, 0, 0, 0, 0x00}
	binary.LittleEndian.PutUint32(seq[4:8], uint32(x86AMD64OffSavedEFlags))
	return seq
}

// withCS wraps a CodeBuffer-emitting test with a default x86CompileState
// installed in x86CurrentCS, so emitters that consult x86CurrentCS
// (e.g. through x86GuestRegToHost) work cleanly.
func withCS(fn func(cb *CodeBuffer, cs *x86CompileState)) []byte {
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
	fn(cb, cs)
	return cb.Bytes()
}

// TestX86EmitRestoreGuestCF_Encoding pins the BT-from-slot encoding.
// A typo in any field would either point at the wrong stack offset
// (loading garbage as guest CF) or the wrong bit (loading a different
// flag and silently corrupting ADC/SBB inputs).
func TestX86EmitRestoreGuestCF_Encoding(t *testing.T) {
	cb := &CodeBuffer{}
	x86EmitRestoreGuestCF(cb)
	got := cb.Bytes()
	want := btCFRestoreSequence()
	if !bytes.Equal(got, want) {
		t.Fatalf("x86EmitRestoreGuestCF emitted %x, want %x", got, want)
	}
}

// TestX86EmitALU_AL_Ib_RestoresCFOnADCSBB asserts that ADC AL,imm8
// (0x14) and SBB AL,imm8 (0x1C) emit the BT-from-slot CF restore
// before the host ALU. Non-ADC/SBB variants (ADD/SUB/CMP/AND/OR/XOR)
// must NOT emit it (they don't read CF; the BT would clobber other
// host flags for nothing).
func TestX86EmitALU_AL_Ib_RestoresCFOnADCSBB(t *testing.T) {
	btSeq := btCFRestoreSequence()

	for _, op := range []byte{0x14, 0x1C} {
		out := withCS(func(cb *CodeBuffer, cs *x86CompileState) {
			ji := &X86JITInstr{
				opcode:   uint16(op),
				opcodePC: 0x1000,
				length:   2,
			}
			memory := make([]byte, 0x10000)
			memory[0x1000] = op
			memory[0x1001] = 0x42
			x86EmitALU_AL_Ib(cb, ji, memory, cs)
		})
		if !bytes.Contains(out, btSeq) {
			t.Errorf("op=%#x (ADC/SBB AL,Ib): missing CF restore (BT [RSP+slot],0). "+
				"Without it, the host ALU runs with stale CF and the guest's "+
				"architectural carry input is lost.", op)
		}
	}

	for _, op := range []byte{0x04, 0x0C, 0x24, 0x2C, 0x34, 0x3C} {
		out := withCS(func(cb *CodeBuffer, cs *x86CompileState) {
			ji := &X86JITInstr{
				opcode:   uint16(op),
				opcodePC: 0x1000,
				length:   2,
			}
			memory := make([]byte, 0x10000)
			memory[0x1000] = op
			memory[0x1001] = 0x42
			x86EmitALU_AL_Ib(cb, ji, memory, cs)
		})
		if bytes.Contains(out, btSeq) {
			t.Errorf("op=%#x: emits CF restore but does not read CF — "+
				"BT clobbers other host flags for nothing", op)
		}
	}
}

// TestX86EmitGrp1_Eb_Ib_HighByteCapturesPreMerge asserts that
// high-byte Grp1 (e.g. ADD AH, ADC CH, etc.) runs the post-byte-ALU
// capture (PUSHFQ+POP+MOV-to-slot) BEFORE the merge-back AND/AND/SHL/OR
// sequence. Without this, the merge clobbers host EFLAGS and the
// generic capture records bookkeeping flags instead of guest ALU
// output.
//
// Test strategy: emit ADD AH, 1 (op=0x80, modRM=0xC4 → mod=3, /op=0,
// r/m=4 (AH)). Look for the PUSHFQ (0x9C) byte. If present at all in
// the emitter output, the capture is wired in. The flagCaptureDone
// flag must also be set, so the compile loop's generic capture
// doesn't run a second time and overwrite the slot.
func TestX86EmitGrp1_Eb_Ib_HighByteCapturesPreMerge(t *testing.T) {
	memory := make([]byte, 0x10000)
	memory[0x1000] = 0x80
	memory[0x1001] = 0xC4 // mod=11, /op=0 (ADD), r/m=100 (AH)
	memory[0x1002] = 0x01

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
	ji := &X86JITInstr{
		opcode:   0x0080,
		opcodePC: 0x1000,
		length:   3,
		hasModRM: true,
		modrm:    0xC4,
	}
	if !x86EmitGrp1_Eb_Ib(cb, ji, memory, cs) {
		t.Fatal("x86EmitGrp1_Eb_Ib(ADD AH,1) returned false")
	}
	out := cb.Bytes()

	// PUSHFQ (0x9C) must appear — it's the start of x86EmitCaptureFlagsArith.
	if !bytes.Contains(out, []byte{0x9C}) {
		t.Fatal("ADD AH,1 emit: missing PUSHFQ — guest flag capture is not " +
			"running before the merge-back AND/SHL/OR clobber sequence")
	}

	// flagCaptureDone must be set so the compile loop skips the generic capture.
	if !cs.flagCaptureDone {
		t.Fatal("ADD AH,1 emit: cs.flagCaptureDone is false — the compile " +
			"loop will run a second capture after this emitter returns, " +
			"overwriting the correct guest flags with merge bookkeeping flags")
	}
}

// TestX86EmitGrp1_Eb_Ib_HighByteSBBRestoresCF asserts that SBB BH,
// imm8 (Grp1 Eb,Ib with /op=3, r/m=7=BH) emits the CF restore before
// the byte ALU.
func TestX86EmitGrp1_Eb_Ib_HighByteSBBRestoresCF(t *testing.T) {
	memory := make([]byte, 0x10000)
	memory[0x1000] = 0x80
	memory[0x1001] = 0xDF // mod=11, /op=3 (SBB), r/m=111 (BH)
	memory[0x1002] = 0x01

	out := withCS(func(cb *CodeBuffer, cs *x86CompileState) {
		ji := &X86JITInstr{
			opcode:   0x0080,
			opcodePC: 0x1000,
			length:   3,
			hasModRM: true,
			modrm:    0xDF,
		}
		x86EmitGrp1_Eb_Ib(cb, ji, memory, cs)
	})
	btSeq := btCFRestoreSequence()
	if !bytes.Contains(out, btSeq) {
		t.Fatal("SBB BH,1 emit: missing CF restore (BT [RSP+slot],0). " +
			"High-byte SBB reads stale host CF without it.")
	}
}
