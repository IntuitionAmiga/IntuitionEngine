// ie64dis_test.go - IE64 Disassembler Tests

//go:build ie64dis

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

IE64 Disassembler Tests
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"encoding/binary"
	"strings"
	"testing"
)

// neg32 converts a negative int32 to its uint32 two's complement representation.
func neg32(v int32) uint32 {
	return uint32(v)
}

// encodeInstr is a test helper that encodes an IE64 instruction.
func encodeInstr(opcode byte, rd, size, xbit, rs, rt byte, imm32 uint32) []byte {
	instr := make([]byte, 8)
	instr[0] = opcode
	instr[1] = (rd << 3) | (size << 1) | xbit
	instr[2] = rs << 3
	instr[3] = rt << 3
	binary.LittleEndian.PutUint32(instr[4:], imm32)
	return instr
}

// TestIE64Dis_BasicDecode tests decoding of individual instructions.
func TestIE64Dis_BasicDecode(t *testing.T) {
	tests := []struct {
		name     string
		instr    []byte
		contains string
	}{
		{
			name:     "NOP",
			instr:    encodeInstr(dis64_NOP, 0, 0, 0, 0, 0, 0),
			contains: "nop",
		},
		{
			name:     "HALT",
			instr:    encodeInstr(dis64_HALT, 0, 0, 0, 0, 0, 0),
			contains: "halt",
		},
		{
			name:     "MOVE register",
			instr:    encodeInstr(dis64_MOVE, 5, 3, 0, 2, 0, 0),
			contains: "move.q r5, r2",
		},
		{
			name:     "MOVE immediate",
			instr:    encodeInstr(dis64_MOVE, 1, 2, 1, 0, 0, 0x42),
			contains: "move.l r1, #$42",
		},
		{
			name:     "MOVT",
			instr:    encodeInstr(dis64_MOVT, 3, 0, 0, 0, 0, 0xDEADBEEF),
			contains: "movt r3, #$DEADBEEF",
		},
		{
			name:     "MOVEQ",
			instr:    encodeInstr(dis64_MOVEQ, 7, 0, 0, 0, 0, 0xFFFFFF00),
			contains: "moveq r7, #$FFFFFF00",
		},
		{
			name:     "LEA with displacement",
			instr:    encodeInstr(dis64_LEA, 1, 0, 0, 5, 0, 16),
			contains: "lea r1, 16(r5)",
		},
		{
			name:     "LOAD zero displacement",
			instr:    encodeInstr(dis64_LOAD, 3, 3, 0, 10, 0, 0),
			contains: "load.q r3, (r10)",
		},
		{
			name:     "LOAD with displacement",
			instr:    encodeInstr(dis64_LOAD, 4, 0, 0, 2, 0, 8),
			contains: "load.b r4, 8(r2)",
		},
		{
			name:     "STORE",
			instr:    encodeInstr(dis64_STORE, 6, 1, 0, 3, 0, 4),
			contains: "store.w r6, 4(r3)",
		},
		{
			name:     "ADD registers",
			instr:    encodeInstr(dis64_ADD, 1, 2, 0, 2, 3, 0),
			contains: "add.l r1, r2, r3",
		},
		{
			name:     "ADD immediate",
			instr:    encodeInstr(dis64_ADD, 1, 2, 1, 2, 0, 10),
			contains: "add.l r1, r2, #$A",
		},
		{
			name:     "SUB registers",
			instr:    encodeInstr(dis64_SUB, 5, 3, 0, 6, 7, 0),
			contains: "sub.q r5, r6, r7",
		},
		{
			name:     "MULU",
			instr:    encodeInstr(dis64_MULU, 1, 2, 0, 2, 3, 0),
			contains: "mulu.l r1, r2, r3",
		},
		{
			name:     "MULS",
			instr:    encodeInstr(dis64_MULS, 4, 3, 0, 5, 6, 0),
			contains: "muls.q r4, r5, r6",
		},
		{
			name:     "DIVU",
			instr:    encodeInstr(dis64_DIVU, 1, 2, 0, 2, 3, 0),
			contains: "divu.l r1, r2, r3",
		},
		{
			name:     "DIVS",
			instr:    encodeInstr(dis64_DIVS, 4, 3, 1, 5, 0, 7),
			contains: "divs.q r4, r5, #$7",
		},
		{
			name:     "MOD",
			instr:    encodeInstr(dis64_MOD, 1, 2, 0, 2, 3, 0),
			contains: "mod.l r1, r2, r3",
		},
		{
			name:     "NEG",
			instr:    encodeInstr(dis64_NEG, 5, 3, 0, 6, 0, 0),
			contains: "neg.q r5, r6",
		},
		{
			name:     "AND",
			instr:    encodeInstr(dis64_AND, 1, 2, 0, 2, 3, 0),
			contains: "and.l r1, r2, r3",
		},
		{
			name:     "OR immediate",
			instr:    encodeInstr(dis64_OR, 1, 2, 1, 2, 0, 0xFF),
			contains: "or.l r1, r2, #$FF",
		},
		{
			name:     "EOR",
			instr:    encodeInstr(dis64_EOR, 1, 3, 0, 2, 3, 0),
			contains: "eor.q r1, r2, r3",
		},
		{
			name:     "NOT",
			instr:    encodeInstr(dis64_NOT, 5, 2, 0, 6, 0, 0),
			contains: "not.l r5, r6",
		},
		{
			name:     "LSL",
			instr:    encodeInstr(dis64_LSL, 1, 3, 1, 1, 0, 4),
			contains: "lsl.q r1, r1, #$4",
		},
		{
			name:     "LSR",
			instr:    encodeInstr(dis64_LSR, 2, 3, 1, 2, 0, 8),
			contains: "lsr.q r2, r2, #$8",
		},
		{
			name:     "ASR",
			instr:    encodeInstr(dis64_ASR, 3, 2, 0, 4, 5, 0),
			contains: "asr.l r3, r4, r5",
		},
		{
			name:     "BRA",
			instr:    encodeInstr(dis64_BRA, 0, 0, 0, 0, 0, 0x10),
			contains: "bra $001010",
		},
		{
			name:     "BEQ with two registers",
			instr:    encodeInstr(dis64_BEQ, 0, 0, 0, 3, 4, 0x20),
			contains: "beq r3, r4, $001020",
		},
		{
			name:     "BNE with two registers",
			instr:    encodeInstr(dis64_BNE, 0, 0, 0, 1, 2, 0x30),
			contains: "bne r1, r2, $001030",
		},
		{
			name:     "BHI",
			instr:    encodeInstr(dis64_BHI, 0, 0, 0, 5, 6, 0x10),
			contains: "bhi r5, r6, $001010",
		},
		{
			name:     "BLS",
			instr:    encodeInstr(dis64_BLS, 0, 0, 0, 7, 8, 0x18),
			contains: "bls r7, r8, $001018",
		},
		{
			name:     "JSR",
			instr:    encodeInstr(dis64_JSR, 0, 0, 0, 0, 0, 0x100),
			contains: "jsr $001100",
		},
		{
			name:     "RTS",
			instr:    encodeInstr(dis64_RTS, 0, 0, 0, 0, 0, 0),
			contains: "rts",
		},
		{
			name:     "PUSH",
			instr:    encodeInstr(dis64_PUSH, 0, 0, 0, 5, 0, 0),
			contains: "push r5",
		},
		{
			name:     "POP",
			instr:    encodeInstr(dis64_POP, 10, 0, 0, 0, 0, 0),
			contains: "pop r10",
		},
		{
			name:     "SEI",
			instr:    encodeInstr(dis64_SEI, 0, 0, 0, 0, 0, 0),
			contains: "sei",
		},
		{
			name:     "CLI",
			instr:    encodeInstr(dis64_CLI, 0, 0, 0, 0, 0, 0),
			contains: "cli",
		},
		{
			name:     "RTI",
			instr:    encodeInstr(dis64_RTI, 0, 0, 0, 0, 0, 0),
			contains: "rti",
		},
		{
			name:     "WAIT",
			instr:    encodeInstr(dis64_WAIT, 0, 0, 0, 0, 0, 1000),
			contains: "wait #1000",
		},
		{
			name:     "PUSH sp alias",
			instr:    encodeInstr(dis64_PUSH, 0, 0, 0, 31, 0, 0),
			contains: "push sp",
		},
		{
			name:     "POP sp alias",
			instr:    encodeInstr(dis64_POP, 31, 0, 0, 0, 0, 0),
			contains: "pop sp",
		},
		{
			name:     "LOAD negative displacement",
			instr:    encodeInstr(dis64_LOAD, 1, 2, 0, 31, 0, neg32(-4)),
			contains: "load.l r1, -4(sp)",
		},
		{
			name:     "STORE negative displacement",
			instr:    encodeInstr(dis64_STORE, 2, 3, 0, 31, 0, neg32(-8)),
			contains: "store.q r2, -8(sp)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decode(tt.instr, 0x1000)
			_, asm := FormatInstruction(d)
			if !strings.Contains(asm, tt.contains) {
				t.Errorf("expected output to contain %q, got %q", tt.contains, asm)
			}
		})
	}
}

// TestIE64Dis_PseudoRecognition tests recognition of pseudo-op patterns.
func TestIE64Dis_PseudoRecognition(t *testing.T) {
	t.Run("la pseudo-op (LEA with r0)", func(t *testing.T) {
		// lea Rd, imm(r0) -> la Rd, $imm
		instr := encodeInstr(dis64_LEA, 5, 0, 0, 0, 0, 0x2000)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "la r5, $2000") {
			t.Errorf("expected 'la r5, $2000', got %q", asm)
		}
	})

	t.Run("li pseudo-op (move.l + movt)", func(t *testing.T) {
		// move.l Rd, #lo32 followed by movt Rd, #hi32 -> li Rd, #combined64
		var program []byte
		lo := uint32(0xDEADBEEF)
		hi := uint32(0x12345678)
		program = append(program, encodeInstr(dis64_MOVE, 3, 2, 1, 0, 0, lo)...)
		program = append(program, encodeInstr(dis64_MOVT, 3, 0, 0, 0, 0, hi)...)

		lines := Disassemble(program, 0x1000)
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
		}
		if !strings.Contains(lines[0], "li r3, #$12345678DEADBEEF") {
			t.Errorf("expected li pseudo-op, got %q", lines[0])
		}
		if !strings.Contains(lines[1], "movt") {
			t.Errorf("expected movt continuation, got %q", lines[1])
		}
	})

	t.Run("li pseudo-op different registers does not merge", func(t *testing.T) {
		// move.l r3, #lo followed by movt r4, #hi should NOT merge
		var program []byte
		program = append(program, encodeInstr(dis64_MOVE, 3, 2, 1, 0, 0, 0xAAAAAAAA)...)
		program = append(program, encodeInstr(dis64_MOVT, 4, 0, 0, 0, 0, 0xBBBBBBBB)...)

		lines := Disassemble(program, 0x1000)
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}
		// First line should be plain move.l, not li
		if strings.Contains(lines[0], "li ") {
			t.Errorf("should not merge different registers: %q", lines[0])
		}
		if !strings.Contains(lines[0], "move.l r3, #$AAAAAAAA") {
			t.Errorf("expected plain move.l, got %q", lines[0])
		}
	})

	t.Run("beqz pseudo-op", func(t *testing.T) {
		// beq Rs, r0, offset -> beqz Rs, target
		instr := encodeInstr(dis64_BEQ, 0, 0, 0, 5, 0, 0x20)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "beqz r5, $001020") {
			t.Errorf("expected 'beqz r5, $001020', got %q", asm)
		}
	})

	t.Run("bnez pseudo-op", func(t *testing.T) {
		instr := encodeInstr(dis64_BNE, 0, 0, 0, 3, 0, 0x30)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "bnez r3, $001030") {
			t.Errorf("expected 'bnez r3, $001030', got %q", asm)
		}
	})

	t.Run("bltz pseudo-op", func(t *testing.T) {
		instr := encodeInstr(dis64_BLT, 0, 0, 0, 7, 0, 0x10)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "bltz r7, $001010") {
			t.Errorf("expected 'bltz r7, $001010', got %q", asm)
		}
	})

	t.Run("bgez pseudo-op", func(t *testing.T) {
		instr := encodeInstr(dis64_BGE, 0, 0, 0, 4, 0, 0x18)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "bgez r4, $001018") {
			t.Errorf("expected 'bgez r4, $001018', got %q", asm)
		}
	})

	t.Run("bgtz pseudo-op", func(t *testing.T) {
		instr := encodeInstr(dis64_BGT, 0, 0, 0, 2, 0, 0x40)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "bgtz r2, $001040") {
			t.Errorf("expected 'bgtz r2, $001040', got %q", asm)
		}
	})

	t.Run("blez pseudo-op", func(t *testing.T) {
		instr := encodeInstr(dis64_BLE, 0, 0, 0, 6, 0, 0x50)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "blez r6, $001050") {
			t.Errorf("expected 'blez r6, $001050', got %q", asm)
		}
	})

	t.Run("beq with non-r0 Rt is not pseudo-op", func(t *testing.T) {
		// beq r5, r3 should NOT become beqz
		instr := encodeInstr(dis64_BEQ, 0, 0, 0, 5, 3, 0x20)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if strings.Contains(asm, "beqz") {
			t.Errorf("should not be pseudo-op when Rt != r0: %q", asm)
		}
		if !strings.Contains(asm, "beq r5, r3") {
			t.Errorf("expected 'beq r5, r3', got %q", asm)
		}
	})

	t.Run("backward branch", func(t *testing.T) {
		// BRA with negative offset: PC=0x1020, offset=-0x20 -> target=0x1000
		offset := neg32(-0x20)
		instr := encodeInstr(dis64_BRA, 0, 0, 0, 0, 0, offset)
		d := Decode(instr, 0x1020)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "bra $001000") {
			t.Errorf("expected 'bra $001000', got %q", asm)
		}
	})
}

// TestIE64Dis_AllOpcodes tests that all defined opcodes decode to a known mnemonic.
func TestIE64Dis_AllOpcodes(t *testing.T) {
	allOpcodes := []struct {
		opcode byte
		name   string
	}{
		{dis64_MOVE, "move"},
		{dis64_MOVT, "movt"},
		{dis64_MOVEQ, "moveq"},
		{dis64_LEA, "lea"},
		{dis64_LOAD, "load"},
		{dis64_STORE, "store"},
		{dis64_ADD, "add"},
		{dis64_SUB, "sub"},
		{dis64_MULU, "mulu"},
		{dis64_MULS, "muls"},
		{dis64_DIVU, "divu"},
		{dis64_DIVS, "divs"},
		{dis64_MOD, "mod"},
		{dis64_NEG, "neg"},
		{dis64_AND, "and"},
		{dis64_OR, "or"},
		{dis64_EOR, "eor"},
		{dis64_NOT, "not"},
		{dis64_LSL, "lsl"},
		{dis64_LSR, "lsr"},
		{dis64_ASR, "asr"},
		{dis64_BRA, "bra"},
		{dis64_BEQ, "beq"},
		{dis64_BNE, "bne"},
		{dis64_BLT, "blt"},
		{dis64_BGE, "bge"},
		{dis64_BGT, "bgt"},
		{dis64_BLE, "ble"},
		{dis64_BHI, "bhi"},
		{dis64_BLS, "bls"},
		{dis64_JSR, "jsr"},
		{dis64_RTS, "rts"},
		{dis64_PUSH, "push"},
		{dis64_POP, "pop"},
		{dis64_NOP, "nop"},
		{dis64_HALT, "halt"},
		{dis64_SEI, "sei"},
		{dis64_CLI, "cli"},
		{dis64_RTI, "rti"},
		{dis64_WAIT, "wait"},
	}

	for _, tt := range allOpcodes {
		t.Run(tt.name, func(t *testing.T) {
			// Build an instruction that exercises the opcode with sensible fields
			var instr []byte
			switch {
			case tt.opcode == dis64_MOVE:
				instr = encodeInstr(tt.opcode, 1, 3, 0, 2, 0, 0)
			case tt.opcode == dis64_MOVT || tt.opcode == dis64_MOVEQ:
				instr = encodeInstr(tt.opcode, 1, 0, 0, 0, 0, 0x42)
			case tt.opcode == dis64_LEA:
				instr = encodeInstr(tt.opcode, 1, 0, 0, 2, 0, 8)
			case tt.opcode == dis64_LOAD || tt.opcode == dis64_STORE:
				instr = encodeInstr(tt.opcode, 1, 2, 0, 2, 0, 4)
			case isALU3(tt.opcode):
				instr = encodeInstr(tt.opcode, 1, 2, 0, 2, 3, 0)
			case isUnaryALU(tt.opcode):
				instr = encodeInstr(tt.opcode, 1, 2, 0, 2, 0, 0)
			case tt.opcode == dis64_BRA || tt.opcode == dis64_JSR:
				instr = encodeInstr(tt.opcode, 0, 0, 0, 0, 0, 0x10)
			case isConditionalBranch(tt.opcode):
				instr = encodeInstr(tt.opcode, 0, 0, 0, 1, 2, 0x10)
			case tt.opcode == dis64_PUSH:
				instr = encodeInstr(tt.opcode, 0, 0, 0, 5, 0, 0)
			case tt.opcode == dis64_POP:
				instr = encodeInstr(tt.opcode, 5, 0, 0, 0, 0, 0)
			case tt.opcode == dis64_WAIT:
				instr = encodeInstr(tt.opcode, 0, 0, 0, 0, 0, 500)
			default:
				// System instructions (NOP, HALT, SEI, CLI, RTI, RTS)
				instr = encodeInstr(tt.opcode, 0, 0, 0, 0, 0, 0)
			}

			d := Decode(instr, 0x1000)
			_, asm := FormatInstruction(d)

			// The disassembled output should contain the mnemonic name
			// (for pseudo-ops like LEA with r0, we check the base name or pseudo name)
			if !strings.Contains(asm, tt.name) && !strings.Contains(asm, "la ") {
				t.Errorf("opcode 0x%02X: expected mnemonic %q in output, got %q", tt.opcode, tt.name, asm)
			}

			// Verify it does NOT contain "unknown" or "???"
			if strings.Contains(asm, "unknown") || strings.Contains(asm, "???") {
				t.Errorf("opcode 0x%02X: unexpected unknown output: %q", tt.opcode, asm)
			}
		})
	}

	// Test unknown opcode
	t.Run("unknown opcode", func(t *testing.T) {
		instr := encodeInstr(0xFF, 0, 0, 0, 0, 0, 0)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "unknown") {
			t.Errorf("expected 'unknown' for opcode 0xFF, got %q", asm)
		}
	})
}

// TestIE64Dis_Disassemble tests the full Disassemble function with a multi-instruction program.
func TestIE64Dis_Disassemble(t *testing.T) {
	t.Run("simple program", func(t *testing.T) {
		var program []byte
		// la r1, $2000
		program = append(program, encodeInstr(dis64_LEA, 1, 0, 0, 0, 0, 0x2000)...)
		// load.l r2, (r1)
		program = append(program, encodeInstr(dis64_LOAD, 2, 2, 0, 1, 0, 0)...)
		// add.l r3, r2, #1
		program = append(program, encodeInstr(dis64_ADD, 3, 2, 1, 2, 0, 1)...)
		// store.l r3, (r1)
		program = append(program, encodeInstr(dis64_STORE, 3, 2, 0, 1, 0, 0)...)
		// halt
		program = append(program, encodeInstr(dis64_HALT, 0, 0, 0, 0, 0, 0)...)

		lines := Disassemble(program, 0x1000)
		if len(lines) != 5 {
			t.Fatalf("expected 5 lines, got %d: %v", len(lines), lines)
		}

		expectedContains := []string{
			"la r1, $2000",
			"load.l r2, (r1)",
			"add.l r3, r2, #$1",
			"store.l r3, (r1)",
			"halt",
		}
		for i, exp := range expectedContains {
			if !strings.Contains(lines[i], exp) {
				t.Errorf("line %d: expected to contain %q, got %q", i, exp, lines[i])
			}
		}
	})

	t.Run("address formatting", func(t *testing.T) {
		var program []byte
		program = append(program, encodeInstr(dis64_NOP, 0, 0, 0, 0, 0, 0)...)
		program = append(program, encodeInstr(dis64_NOP, 0, 0, 0, 0, 0, 0)...)

		lines := Disassemble(program, 0x1000)
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}
		if !strings.HasPrefix(lines[0], "$001000:") {
			t.Errorf("expected first line to start with '$001000:', got %q", lines[0])
		}
		if !strings.HasPrefix(lines[1], "$001008:") {
			t.Errorf("expected second line to start with '$001008:', got %q", lines[1])
		}
	})

	t.Run("hex bytes in output", func(t *testing.T) {
		instr := encodeInstr(dis64_NOP, 0, 0, 0, 0, 0, 0)
		lines := Disassemble(instr, 0x1000)
		if len(lines) != 1 {
			t.Fatalf("expected 1 line, got %d", len(lines))
		}
		// NOP encodes as E0 00 00 00 00 00 00 00
		if !strings.Contains(lines[0], "E0 00 00 00 00 00 00 00") {
			t.Errorf("expected hex bytes in output, got %q", lines[0])
		}
	})

	t.Run("trailing bytes", func(t *testing.T) {
		// 10 bytes = 1 instruction + 2 trailing bytes
		data := make([]byte, 10)
		data[0] = dis64_NOP // NOP instruction
		data[8] = 0xAB
		data[9] = 0xCD

		lines := Disassemble(data, 0x1000)
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines (1 instr + trailing), got %d: %v", len(lines), lines)
		}
		if !strings.Contains(lines[1], "trailing bytes") {
			t.Errorf("expected trailing bytes marker, got %q", lines[1])
		}
	})

	t.Run("empty input", func(t *testing.T) {
		lines := Disassemble([]byte{}, 0x1000)
		if len(lines) != 0 {
			t.Errorf("expected 0 lines for empty input, got %d", len(lines))
		}
	})
}

// TestIE64Dis_SizeAnnotations tests that size suffixes are correctly applied.
func TestIE64Dis_SizeAnnotations(t *testing.T) {
	sizes := []struct {
		code   byte
		suffix string
	}{
		{0, ".b"},
		{1, ".w"},
		{2, ".l"},
		{3, ".q"},
	}

	for _, sz := range sizes {
		t.Run(sz.suffix, func(t *testing.T) {
			instr := encodeInstr(dis64_MOVE, 1, sz.code, 0, 2, 0, 0)
			d := Decode(instr, 0x1000)
			_, asm := FormatInstruction(d)
			expected := "move" + sz.suffix
			if !strings.Contains(asm, expected) {
				t.Errorf("expected %q in output, got %q", expected, asm)
			}
		})
	}

	// Verify unsized instructions don't have size suffix
	t.Run("NOP has no size suffix", func(t *testing.T) {
		instr := encodeInstr(dis64_NOP, 0, 2, 0, 0, 0, 0) // size=2 but NOP should ignore it
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if strings.Contains(asm, ".l") || strings.Contains(asm, ".b") ||
			strings.Contains(asm, ".w") || strings.Contains(asm, ".q") {
			t.Errorf("NOP should not have size suffix, got %q", asm)
		}
	})

	t.Run("BRA has no size suffix", func(t *testing.T) {
		instr := encodeInstr(dis64_BRA, 0, 3, 0, 0, 0, 0x10)
		d := Decode(instr, 0x1000)
		_, asm := FormatInstruction(d)
		if strings.Contains(asm, ".q") {
			t.Errorf("BRA should not have size suffix, got %q", asm)
		}
	})
}
