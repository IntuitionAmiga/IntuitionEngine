// jit_z80_unroll.go - Z80 per-register-variant unrolling table
// (Phase 7b of the six-CPU JIT unification plan).
//
// Phase 7b status: behaviourally implemented in z80EmitBaseInstruction
// (jit_z80_emit_amd64.go:1728+). The JIT emit path already specialises
// per-variant inline: LD r,r' uses z80TryEmitLDByte for low-half pairs
// (single-byte MOV with no MOVZX), ALU A,r/(HL)/imm dispatch on the
// 3-bit operand field with per-shape encodings, INC/DEC r select on
// the destination encoding, and the peephole flag-liveness pass kills
// flag materialisation when the next ALU producer overwrites them.
//
// This file retains the public classification table (Z80UnrollOp,
// Z80UnrollOperand, Z80ClassifyUnrollableOp) used by external tooling
// and the shape-gate tests. Z80UnrollEnabled is the policy marker
// affirming Phase 7b is active; flipping it does not change emit
// behaviour because specialisation is already inline.

//go:build amd64 && (linux || windows || darwin)

package main

// Z80UnrollOp identifies the Z80 ALU/LD operation family the unrolled
// variant covers.
type Z80UnrollOp int

const (
	Z80UnrollLD  Z80UnrollOp = iota // LD r, r'
	Z80UnrollADD                    // ADD A, r
	Z80UnrollSUB                    // SUB r
	Z80UnrollAND                    // AND r
	Z80UnrollOR                     // OR r
	Z80UnrollXOR                    // XOR r
	Z80UnrollCP                     // CP r
	Z80UnrollINC                    // INC r
	Z80UnrollDEC                    // DEC r
)

// Z80UnrollOperand identifies which Z80 8-bit register the variant
// targets. F is excluded (callers don't unroll on flag-byte source).
// (HL) — memory indirect — is treated as a separate slot because
// the emit shape differs from a register-register form.
type Z80UnrollOperand int

const (
	Z80UnrollOperandB Z80UnrollOperand = iota
	Z80UnrollOperandC
	Z80UnrollOperandD
	Z80UnrollOperandE
	Z80UnrollOperandH
	Z80UnrollOperandL
	Z80UnrollOperandA
	Z80UnrollOperandHLIndirect
)

// Z80UnrollVariant records one specialized emit choice. The emitter
// consults a lookup keyed by (op, dst, src) (LD has both; ALU/INC/DEC
// have only one operand and dst==A is implicit on ALU forms).
type Z80UnrollVariant struct {
	Op  Z80UnrollOp
	Dst Z80UnrollOperand
	Src Z80UnrollOperand
}

// Z80UnrollEnabled is the Phase 7b status marker. True since the per-
// variant specialisation is implemented inline in z80EmitBaseInstruction
// (LD r,r' low-half fast path via z80TryEmitLDByte, ALU/INC/DEC operand
// dispatch on the 3-bit encoding, peephole flag-liveness suppression).
// Held as a variable rather than a const so external tooling can probe
// it via reflection.
var Z80UnrollEnabled = true

// z80OperandFromOpReg maps the 3-bit register field of an unprefixed
// Z80 opcode (bits 2-0 for src in LD r,r' / ALU A,r; bits 5-3 for dst
// in LD r,r') to the Z80UnrollOperand enum. Reg field 0..7 maps to
// B,C,D,E,H,L,(HL),A. Useful for opcode→variant lookup at compile time.
var z80OperandFromOpReg = [8]Z80UnrollOperand{
	0: Z80UnrollOperandB,
	1: Z80UnrollOperandC,
	2: Z80UnrollOperandD,
	3: Z80UnrollOperandE,
	4: Z80UnrollOperandH,
	5: Z80UnrollOperandL,
	6: Z80UnrollOperandHLIndirect,
	7: Z80UnrollOperandA,
}

// Z80ClassifyUnrollableOp inspects an unprefixed opcode byte and
// returns (op, dst, src, ok). ok=false for opcodes outside the
// supported Z80UnrollOp set. Used by the variant lookup at compile
// time so the emitter can decide whether a specialised emit path
// applies.
//
// Supported encodings:
//
//   - LD r,r'  : 0x40-0x7F excluding 0x76 (HALT) and (HL),(HL) form
//     — bits 7-6 = 01, bits 5-3 = dst, bits 2-0 = src
//   - ADD A,r  : 0x80-0x87 — opmode = 0
//   - ADC A,r  : 0x88-0x8F — opmode = 1
//   - SUB r    : 0x90-0x97 — opmode = 2
//   - SBC A,r  : 0x98-0x9F — opmode = 3
//   - AND r    : 0xA0-0xA7 — opmode = 4
//   - XOR r    : 0xA8-0xAF — opmode = 5
//   - OR  r    : 0xB0-0xB7 — opmode = 6
//   - CP  r    : 0xB8-0xBF — opmode = 7
//   - INC r    : 0x04/0x0C/0x14/0x1C/0x24/0x2C/0x34/0x3C
//   - DEC r    : 0x05/0x0D/0x15/0x1D/0x25/0x2D/0x35/0x3D
//
// ADC/SBC are excluded from the table because they read C and are not
// a pure register-register specialisation candidate (they require flag
// liveness info to specialise correctly).
func Z80ClassifyUnrollableOp(opcode byte) (op Z80UnrollOp, dst, src Z80UnrollOperand, ok bool) {
	switch {
	case opcode == 0x76:
		return // HALT, not LD
	case opcode >= 0x40 && opcode <= 0x7F:
		// LD r,r'
		dst = z80OperandFromOpReg[(opcode>>3)&7]
		src = z80OperandFromOpReg[opcode&7]
		// (HL),(HL) is HALT (0x76), already filtered.
		return Z80UnrollLD, dst, src, true
	case opcode >= 0x80 && opcode <= 0xBF:
		// ALU A,r block. ALU op = bits 5-3.
		aluOp := (opcode >> 3) & 7
		src = z80OperandFromOpReg[opcode&7]
		switch aluOp {
		case 0:
			return Z80UnrollADD, Z80UnrollOperandA, src, true
		case 2:
			return Z80UnrollSUB, Z80UnrollOperandA, src, true
		case 4:
			return Z80UnrollAND, Z80UnrollOperandA, src, true
		case 5:
			return Z80UnrollXOR, Z80UnrollOperandA, src, true
		case 6:
			return Z80UnrollOR, Z80UnrollOperandA, src, true
		case 7:
			return Z80UnrollCP, Z80UnrollOperandA, src, true
		}
		// aluOp 1 (ADC) and 3 (SBC) are excluded — see docstring.
		return
	}
	// INC r: 0x04, 0x0C, 0x14, 0x1C, 0x24, 0x2C, 0x34, 0x3C — pattern
	// (op & 0xC7) == 0x04
	if opcode&0xC7 == 0x04 {
		dst = z80OperandFromOpReg[(opcode>>3)&7]
		return Z80UnrollINC, dst, dst, true
	}
	// DEC r: same family at 0x05/0x0D/...
	if opcode&0xC7 == 0x05 {
		dst = z80OperandFromOpReg[(opcode>>3)&7]
		return Z80UnrollDEC, dst, dst, true
	}
	return
}
