// jit_z80_unroll_test.go - shape gates for the Phase 7b Z80 unroller
// classification table.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func TestZ80ClassifyUnrollableOp_LD_RR(t *testing.T) {
	// LD A,B = 0x78 → op=LD dst=A src=B
	op, dst, src, ok := Z80ClassifyUnrollableOp(0x78)
	if !ok || op != Z80UnrollLD || dst != Z80UnrollOperandA || src != Z80UnrollOperandB {
		t.Errorf("LD A,B: got op=%d dst=%d src=%d ok=%v", op, dst, src, ok)
	}
	// LD B,(HL) = 0x46 → op=LD dst=B src=(HL)
	op, dst, src, ok = Z80ClassifyUnrollableOp(0x46)
	if !ok || op != Z80UnrollLD || dst != Z80UnrollOperandB || src != Z80UnrollOperandHLIndirect {
		t.Errorf("LD B,(HL): got op=%d dst=%d src=%d ok=%v", op, dst, src, ok)
	}
}

func TestZ80ClassifyUnrollableOp_HALTExcluded(t *testing.T) {
	// 0x76 is HALT, not LD (HL),(HL). Must not classify as unrollable.
	if _, _, _, ok := Z80ClassifyUnrollableOp(0x76); ok {
		t.Error("HALT (0x76) must not classify as unrollable")
	}
}

func TestZ80ClassifyUnrollableOp_ALUBlock(t *testing.T) {
	cases := []struct {
		op   byte
		want Z80UnrollOp
		src  Z80UnrollOperand
	}{
		{0x80, Z80UnrollADD, Z80UnrollOperandB},
		{0x90, Z80UnrollSUB, Z80UnrollOperandB},
		{0xA0, Z80UnrollAND, Z80UnrollOperandB},
		{0xA8, Z80UnrollXOR, Z80UnrollOperandB},
		{0xB0, Z80UnrollOR, Z80UnrollOperandB},
		{0xB8, Z80UnrollCP, Z80UnrollOperandB},
		{0x87, Z80UnrollADD, Z80UnrollOperandA}, // ADD A,A
		{0xBE, Z80UnrollCP, Z80UnrollOperandHLIndirect},
	}
	for _, c := range cases {
		op, dst, src, ok := Z80ClassifyUnrollableOp(c.op)
		if !ok || op != c.want || dst != Z80UnrollOperandA || src != c.src {
			t.Errorf("op %02X: got op=%d dst=%d src=%d ok=%v want op=%d src=%d",
				c.op, op, dst, src, ok, c.want, c.src)
		}
	}
}

func TestZ80ClassifyUnrollableOp_ADCSBCExcluded(t *testing.T) {
	// ADC/SBC require flag-liveness info; not in the unrollable set.
	for _, op := range []byte{0x88, 0x8F, 0x98, 0x9F} {
		if _, _, _, ok := Z80ClassifyUnrollableOp(op); ok {
			t.Errorf("ADC/SBC opcode %02X must not classify as unrollable", op)
		}
	}
}

func TestZ80ClassifyUnrollableOp_INCDEC(t *testing.T) {
	// INC B = 0x04, INC (HL) = 0x34, INC A = 0x3C
	op, dst, _, ok := Z80ClassifyUnrollableOp(0x04)
	if !ok || op != Z80UnrollINC || dst != Z80UnrollOperandB {
		t.Errorf("INC B: got op=%d dst=%d ok=%v", op, dst, ok)
	}
	op, dst, _, ok = Z80ClassifyUnrollableOp(0x3C)
	if !ok || op != Z80UnrollINC || dst != Z80UnrollOperandA {
		t.Errorf("INC A: got op=%d dst=%d ok=%v", op, dst, ok)
	}
	// DEC E = 0x1D
	op, dst, _, ok = Z80ClassifyUnrollableOp(0x1D)
	if !ok || op != Z80UnrollDEC || dst != Z80UnrollOperandE {
		t.Errorf("DEC E: got op=%d dst=%d ok=%v", op, dst, ok)
	}
}

func TestZ80ClassifyUnrollableOp_NotUnrollable(t *testing.T) {
	// Opcodes outside the supported set must return ok=false.
	for _, op := range []byte{
		0x00, // NOP
		0xC3, // JP nn
		0xCD, // CALL nn
		0xC9, // RET
		0xF1, // POP AF
		0xF5, // PUSH AF
		0x06, // LD B,n
		0x0E, // LD C,n
		0x21, // LD HL,nn
	} {
		if _, _, _, ok := Z80ClassifyUnrollableOp(op); ok {
			t.Errorf("opcode %02X must not classify as unrollable", op)
		}
	}
}
