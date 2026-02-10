// ie64asm_test.go

//go:build ie64

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

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// encodeInstr builds an expected 8-byte instruction for comparison.
//
// Byte layout (little-endian):
//
//	Byte 0: opcode
//	Byte 1: Rd[4:0] (5 bits) | Size[1:0] (2 bits) | X (1 bit)
//	Byte 2: Rs[4:0] (5 bits) | unused (3 bits)
//	Byte 3: Rt[4:0] (5 bits) | unused (3 bits)
//	Bytes 4-7: imm32 (32-bit LE)
func encodeInstr(opcode byte, rd, size, xbit, rs, rt byte, imm32 uint32) []byte {
	instr := make([]byte, 8)
	instr[0] = opcode
	instr[1] = (rd << 3) | (size << 1) | xbit
	instr[2] = rs << 3
	instr[3] = rt << 3
	binary.LittleEndian.PutUint32(instr[4:], imm32)
	return instr
}

// assembleString is a test helper that assembles a string and returns the
// binary output.
func assembleString(t *testing.T, src string) []byte {
	t.Helper()
	asm := NewIE64Assembler()
	result, err := asm.Assemble(src)
	if err != nil {
		t.Fatalf("assembly failed: %v", err)
	}
	return result
}

// assembleExpectError is a test helper that expects assembly to fail.
func assembleExpectError(t *testing.T, src string) error {
	t.Helper()
	asm := NewIE64Assembler()
	_, err := asm.Assemble(src)
	if err == nil {
		t.Fatal("expected assembly error, got nil")
	}
	return err
}

// IE64 size codes
const (
	szB byte = 0 // .B
	szW byte = 1 // .W
	szL byte = 2 // .L
	szQ byte = 3 // .Q  (default)
)

// IE64 opcodes — must match the constants defined in ie64asm.go.
const (
	opMOVE  = 0x01
	opMOVT  = 0x02
	opMOVEQ = 0x03
	opLEA   = 0x04

	opLOAD  = 0x10
	opSTORE = 0x11

	opADD  = 0x20
	opSUB  = 0x21
	opMULU = 0x22
	opMULS = 0x23
	opDIVU = 0x24
	opDIVS = 0x25
	opMOD  = 0x26
	opNEG  = 0x27

	opAND = 0x30
	opOR  = 0x31
	opEOR = 0x32
	opNOT = 0x33
	opLSL = 0x34
	opLSR = 0x35
	opASR = 0x36

	opBRA = 0x40
	opBEQ = 0x41
	opBNE = 0x42
	opBLT = 0x43
	opBGE = 0x44
	opBGT = 0x45
	opBLE = 0x46
	opBHI = 0x47
	opBLS = 0x48
	opJMP = 0x49

	opJSR    = 0x50
	opRTS    = 0x51
	opPUSH   = 0x52
	opPOP    = 0x53
	opJSRInd = 0x54

	opFMOV    = 0x60
	opFLOAD   = 0x61
	opFSTORE  = 0x62
	opFADD    = 0x63
	opFSUB    = 0x64
	opFMUL    = 0x65
	opFDIV    = 0x66
	opFMOD    = 0x67
	opFABS    = 0x68
	opFNEG    = 0x69
	opFSQRT   = 0x6A
	opFINT    = 0x6B
	opFCMP    = 0x6C
	opFCVTIF  = 0x6D
	opFCVTFI  = 0x6E
	opFMOVI   = 0x6F
	opFMOVO   = 0x70
	opFSIN    = 0x71
	opFCOS    = 0x72
	opFTAN    = 0x73
	opFATAN   = 0x74
	opFLOG    = 0x75
	opFEXP    = 0x76
	opFPOW    = 0x77
	opFMOVECR = 0x78
	opFMOVSR  = 0x79
	opFMOVCR  = 0x7A
	opFMOVSC  = 0x7B
	opFMOVCC  = 0x7C

	opNOP  = 0xE0
	opHALT = 0xE1
	opSEI  = 0xE2
	opCLI  = 0xE3
	opRTI  = 0xE4
	opWAIT = 0xE5
)

// assertBytes compares actual binary output with expected bytes at a given
// offset, producing a clear diff on failure.
func assertBytes(t *testing.T, got []byte, offset int, expected []byte, label string) {
	t.Helper()
	end := offset + len(expected)
	if end > len(got) {
		t.Fatalf("%s: output too short — want %d bytes at offset %d, got %d total bytes",
			label, len(expected), offset, len(got))
	}
	actual := got[offset:end]
	if !bytes.Equal(actual, expected) {
		t.Errorf("%s: mismatch at offset %d\n  got:  %02x\n  want: %02x",
			label, offset, actual, expected)
	}
}

// assertLen verifies the binary output length.
func assertLen(t *testing.T, got []byte, want int, label string) {
	t.Helper()
	if len(got) != want {
		t.Fatalf("%s: output length = %d, want %d", label, len(got), want)
	}
}

// ---------------------------------------------------------------------------
// Step 3a: Core assembler structure + directives
// ---------------------------------------------------------------------------

func TestIE64Asm_Org(t *testing.T) {
	// Origin only affects address calculations; the binary output starts at
	// offset 0 regardless.  We verify by placing a label at the org address
	// and using it as an immediate — the label value should equal the org.
	src := `
		org $2000
start:
		move.q r1, #start
`
	bin := assembleString(t, src)
	// The label "start" = $2000.  Instruction: move.q r1, #$2000
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0x2000)
	assertBytes(t, bin, 0, want, "org $2000 / move.q r1, #start")
}

func TestIE64Asm_Equ(t *testing.T) {
	src := `
SCREEN equ $A0000
		move.q r1, #SCREEN
`
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0xA0000)
	assertBytes(t, bin, 0, want, "equ SCREEN")
}

func TestIE64Asm_Labels(t *testing.T) {
	// Default org = $1000.  Each instruction = 8 bytes.
	// first:  @ $1000
	// nop     @ $1000  (8 bytes)
	// second: @ $1008
	// nop     @ $1008  (8 bytes)
	// We use label values as immediates.
	src := `
first:
		nop
second:
		nop
		move.q r1, #first
		move.q r2, #second
`
	bin := assembleString(t, src)
	assertLen(t, bin, 4*8, "labels")

	wantFirst := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0x1000)
	assertBytes(t, bin, 16, wantFirst, "label first")

	wantSecond := encodeInstr(opMOVE, 2, szQ, 1, 0, 0, 0x1008)
	assertBytes(t, bin, 24, wantSecond, "label second")
}

func TestIE64Asm_DCB(t *testing.T) {
	src := `
		dc.b 1,2,3
`
	bin := assembleString(t, src)
	assertLen(t, bin, 3, "dc.b")
	assertBytes(t, bin, 0, []byte{1, 2, 3}, "dc.b 1,2,3")
}

func TestIE64Asm_DCW(t *testing.T) {
	src := `
		dc.w $1234,$5678
`
	bin := assembleString(t, src)
	assertLen(t, bin, 4, "dc.w")
	// Little-endian 16-bit values
	assertBytes(t, bin, 0, []byte{0x34, 0x12, 0x78, 0x56}, "dc.w")
}

func TestIE64Asm_DCL(t *testing.T) {
	src := `
		dc.l $12345678
`
	bin := assembleString(t, src)
	assertLen(t, bin, 4, "dc.l")
	assertBytes(t, bin, 0, []byte{0x78, 0x56, 0x34, 0x12}, "dc.l")
}

func TestIE64Asm_DCQ(t *testing.T) {
	src := `
		dc.q $CAFEBABE_DEADBEEF
`
	bin := assembleString(t, src)
	assertLen(t, bin, 8, "dc.q")
	// Little-endian 64-bit: low dword first
	expected := make([]byte, 8)
	binary.LittleEndian.PutUint64(expected, 0xCAFEBABE_DEADBEEF)
	assertBytes(t, bin, 0, expected, "dc.q")
}

func TestIE64Asm_DSB(t *testing.T) {
	src := `
		ds.b 16
`
	bin := assembleString(t, src)
	assertLen(t, bin, 16, "ds.b 16")
	for i := 0; i < 16; i++ {
		if bin[i] != 0 {
			t.Fatalf("ds.b 16: byte %d = %02x, want 0x00", i, bin[i])
		}
	}
}

func TestIE64Asm_ASCII_InDCB(t *testing.T) {
	src := `
		dc.b "Hello"
`
	bin := assembleString(t, src)
	assertLen(t, bin, 5, "dc.b string")
	assertBytes(t, bin, 0, []byte("Hello"), "dc.b \"Hello\"")
}

func TestIE64Asm_StringEscapes(t *testing.T) {
	src := `
		dc.b "a\n\t\0\\\""
`
	bin := assembleString(t, src)
	expected := []byte{'a', '\n', '\t', 0, '\\', '"'}
	assertLen(t, bin, len(expected), "string escapes")
	assertBytes(t, bin, 0, expected, "string escapes")
}

func TestIE64Asm_Incbin(t *testing.T) {
	tmpDir := t.TempDir()
	binFile := filepath.Join(tmpDir, "data.bin")
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE}
	if err := os.WriteFile(binFile, payload, 0644); err != nil {
		t.Fatal(err)
	}

	src := `
		incbin "` + binFile + `"
`
	bin := assembleString(t, src)
	assertLen(t, bin, len(payload), "incbin")
	assertBytes(t, bin, 0, payload, "incbin")
}

func TestIE64Asm_Include(t *testing.T) {
	tmpDir := t.TempDir()
	incFile := filepath.Join(tmpDir, "defs.inc")
	incContent := "MAGIC equ $BEEF\n"
	if err := os.WriteFile(incFile, []byte(incContent), 0644); err != nil {
		t.Fatal(err)
	}

	src := `
		include "` + incFile + `"
		move.q r1, #MAGIC
`
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0xBEEF)
	assertBytes(t, bin, 0, want, "include + equ")
}

func TestIE64Asm_Align(t *testing.T) {
	// dc.b 1 emits 1 byte, then align 8 pads to 8 byte boundary.
	src := `
		dc.b 1
		align 8
		dc.b 2
`
	bin := assembleString(t, src)
	// byte 0 = 1, bytes 1-7 = 0 (padding), byte 8 = 2
	if len(bin) != 9 {
		t.Fatalf("align: output length = %d, want 9", len(bin))
	}
	if bin[0] != 1 {
		t.Errorf("align: byte 0 = %02x, want 01", bin[0])
	}
	for i := 1; i < 8; i++ {
		if bin[i] != 0 {
			t.Errorf("align: padding byte %d = %02x, want 0x00", i, bin[i])
		}
	}
	if bin[8] != 2 {
		t.Errorf("align: byte 8 = %02x, want 02", bin[8])
	}
}

func TestIE64Asm_HexFormats(t *testing.T) {
	// Both $FF and 0xFF should produce the same value.
	srcDollar := `
		move.q r1, #$FF
`
	srcPrefix := `
		move.q r1, #0xFF
`
	binD := assembleString(t, srcDollar)
	binP := assembleString(t, srcPrefix)
	if !bytes.Equal(binD, binP) {
		t.Errorf("hex formats differ:\n  $FF:  %02x\n  0xFF: %02x", binD, binP)
	}
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0xFF)
	assertBytes(t, binD, 0, want, "$FF")
}

func TestIE64Asm_Comments(t *testing.T) {
	src := `
		; This is a full-line comment
		nop ; trailing comment
`
	bin := assembleString(t, src)
	assertLen(t, bin, 8, "comments")
	want := encodeInstr(opNOP, 0, 0, 0, 0, 0, 0)
	assertBytes(t, bin, 0, want, "nop with comment")
}

func TestIE64Asm_CaseInsensitive_Mnemonics(t *testing.T) {
	// MOVE.Q, Move.q, move.q should all produce the same binary.
	variants := []string{
		"MOVE.Q r1, r2",
		"Move.q r1, r2",
		"move.q r1, r2",
		"move.Q r1, r2",
	}
	var results [][]byte
	for _, v := range variants {
		bin := assembleString(t, v)
		results = append(results, bin)
	}
	for i := 1; i < len(results); i++ {
		if !bytes.Equal(results[0], results[i]) {
			t.Errorf("mnemonic case: variant %q differs from %q\n  %02x\n  %02x",
				variants[0], variants[i], results[0], results[i])
		}
	}
}

func TestIE64Asm_CaseInsensitive_Registers(t *testing.T) {
	// R1, r1 should produce the same encoding.
	src1 := "move.q R1, R2"
	src2 := "move.q r1, r2"
	bin1 := assembleString(t, src1)
	bin2 := assembleString(t, src2)
	if !bytes.Equal(bin1, bin2) {
		t.Errorf("register case:\n  R1,R2: %02x\n  r1,r2: %02x", bin1, bin2)
	}
	// Also test SP / sp
	srcSP := "push SP"
	srcsp := "push sp"
	binSP := assembleString(t, srcSP)
	binsp := assembleString(t, srcsp)
	if !bytes.Equal(binSP, binsp) {
		t.Errorf("SP case:\n  SP: %02x\n  sp: %02x", binSP, binsp)
	}
}

func TestIE64Asm_CaseInsensitive_Directives(t *testing.T) {
	src1 := "ORG $2000\nDC.B 1,2,3"
	src2 := "org $2000\ndc.b 1,2,3"
	bin1 := assembleString(t, src1)
	bin2 := assembleString(t, src2)
	if !bytes.Equal(bin1, bin2) {
		t.Errorf("directive case:\n  upper: %02x\n  lower: %02x", bin1, bin2)
	}
}

func TestIE64Asm_CaseInsensitive_MacroNames(t *testing.T) {
	src := `
clear		macro
		move.q \1, r0
		endm

		CLEAR r5
`
	bin := assembleString(t, src)
	// Should expand to: move.q r5, r0
	want := encodeInstr(opMOVE, 5, szQ, 0, 0, 0, 0)
	assertBytes(t, bin, 0, want, "macro case insensitive")
}

func TestIE64Asm_LabelsCaseSensitive(t *testing.T) {
	// Labels ARE case-sensitive: MyLabel and mylabel are different.
	src := `
MyLabel:
		nop
mylabel:
		nop
		move.q r1, #MyLabel
		move.q r2, #mylabel
`
	bin := assembleString(t, src)
	assertLen(t, bin, 4*8, "case sensitive labels")
	wantUpper := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0x1000)
	wantLower := encodeInstr(opMOVE, 2, szQ, 1, 0, 0, 0x1008)
	assertBytes(t, bin, 16, wantUpper, "MyLabel")
	assertBytes(t, bin, 24, wantLower, "mylabel")
}

// ---------------------------------------------------------------------------
// Step 3b: Expression evaluation
// ---------------------------------------------------------------------------

func TestIE64Asm_Expr_Addition(t *testing.T) {
	src := "move.q r1, #320+200"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 520)
	assertBytes(t, bin, 0, want, "320+200")
}

func TestIE64Asm_Expr_Multiplication(t *testing.T) {
	src := "move.q r1, #320*200"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 64000)
	assertBytes(t, bin, 0, want, "320*200")
}

func TestIE64Asm_Expr_Shifts(t *testing.T) {
	src := "move.q r1, #1<<16"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 65536)
	assertBytes(t, bin, 0, want, "1<<16")
}

func TestIE64Asm_Expr_Parentheses(t *testing.T) {
	src := "move.q r1, #(320*4)+16"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 1296)
	assertBytes(t, bin, 0, want, "(320*4)+16")
}

func TestIE64Asm_Expr_Equates(t *testing.T) {
	src := `
WIDTH equ 320
HEIGHT equ 200
		move.q r1, #WIDTH*HEIGHT
`
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 64000)
	assertBytes(t, bin, 0, want, "equ in expr")
}

func TestIE64Asm_Expr_Labels(t *testing.T) {
	// Label address used in expression: data+4 means data label + 4
	src := `
		nop
data:
		dc.l $DEADBEEF
		org $1000
		move.q r1, #data+4
`
	// This test relies on the assembler resolving "data" to its address ($1008)
	// and adding 4, producing $100C.
	// Actually let's use a simpler test that doesn't mix data and code offsets.
	src = `
start:
		nop
next:
		nop
		move.q r1, #next-start
`
	bin := assembleString(t, src)
	// next - start = $1008 - $1000 = 8
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 8)
	assertBytes(t, bin, 16, want, "label expr subtraction")
}

func TestIE64Asm_Expr_BitwiseOps(t *testing.T) {
	// OR
	src := "move.q r1, #$FF00|$00FF"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0xFFFF)
	assertBytes(t, bin, 0, want, "$FF00|$00FF")

	// AND
	src = "move.q r1, #$FFFF&$0F0F"
	bin = assembleString(t, src)
	want = encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0x0F0F)
	assertBytes(t, bin, 0, want, "$FFFF&$0F0F")
}

func TestIE64Asm_Expr_UnaryNot(t *testing.T) {
	// Bitwise NOT of $FF = $FFFFFF00 (32-bit)
	src := "move.q r1, #~$FF"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0xFFFFFF00)
	assertBytes(t, bin, 0, want, "~$FF")
}

// ---------------------------------------------------------------------------
// Step 3c: Instruction encoding
// ---------------------------------------------------------------------------

func TestIE64Asm_Move_Reg(t *testing.T) {
	// move.q r1, r2 — register-to-register
	src := "move.q r1, r2"
	bin := assembleString(t, src)
	// opcode=MOVE, rd=1, size=Q(3), X=0, rs=2, rt=0, imm=0
	want := encodeInstr(opMOVE, 1, szQ, 0, 2, 0, 0)
	assertBytes(t, bin, 0, want, "move.q r1, r2")
}

func TestIE64Asm_Move_Imm(t *testing.T) {
	// move.q r1, #$42 — immediate
	src := "move.q r1, #$42"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0x42)
	assertBytes(t, bin, 0, want, "move.q r1, #$42")
}

func TestIE64Asm_Movt(t *testing.T) {
	// movt r1, #$CAFE — move to top half
	src := "movt r1, #$CAFE"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVT, 1, szQ, 1, 0, 0, 0xCAFE)
	assertBytes(t, bin, 0, want, "movt r1, #$CAFE")
}

func TestIE64Asm_Moveq(t *testing.T) {
	// moveq r1, #-1 — quick move (sign-extended)
	src := "moveq r1, #-1"
	bin := assembleString(t, src)
	want := encodeInstr(opMOVEQ, 1, szQ, 1, 0, 0, 0xFFFFFFFF)
	assertBytes(t, bin, 0, want, "moveq r1, #-1")
}

func TestIE64Asm_Load(t *testing.T) {
	// load.l r1, (r2) — base register, no offset
	src := "load.l r1, (r2)"
	bin := assembleString(t, src)
	want := encodeInstr(opLOAD, 1, szL, 0, 2, 0, 0)
	assertBytes(t, bin, 0, want, "load.l r1, (r2)")

	// load.l r1, 8(r2) — base + displacement
	src = "load.l r1, 8(r2)"
	bin = assembleString(t, src)
	want = encodeInstr(opLOAD, 1, szL, 1, 2, 0, 8)
	assertBytes(t, bin, 0, want, "load.l r1, 8(r2)")
}

func TestIE64Asm_Store(t *testing.T) {
	// store.l r1, (r2) — base register, no offset
	src := "store.l r1, (r2)"
	bin := assembleString(t, src)
	want := encodeInstr(opSTORE, 1, szL, 0, 2, 0, 0)
	assertBytes(t, bin, 0, want, "store.l r1, (r2)")

	// store.l r1, 8(r2) — base + displacement
	src = "store.l r1, 8(r2)"
	bin = assembleString(t, src)
	want = encodeInstr(opSTORE, 1, szL, 1, 2, 0, 8)
	assertBytes(t, bin, 0, want, "store.l r1, 8(r2)")
}

func TestIE64Asm_Lea(t *testing.T) {
	// lea r1, 16(r2) — load effective address
	src := "lea r1, 16(r2)"
	bin := assembleString(t, src)
	want := encodeInstr(opLEA, 1, szQ, 1, 2, 0, 16)
	assertBytes(t, bin, 0, want, "lea r1, 16(r2)")
}

func TestIE64Asm_Add_Reg(t *testing.T) {
	// add.q r1, r2, r3 — three-register
	src := "add.q r1, r2, r3"
	bin := assembleString(t, src)
	want := encodeInstr(opADD, 1, szQ, 0, 2, 3, 0)
	assertBytes(t, bin, 0, want, "add.q r1, r2, r3")
}

func TestIE64Asm_Add_Imm(t *testing.T) {
	// add.q r1, r2, #10 — register + immediate
	src := "add.q r1, r2, #10"
	bin := assembleString(t, src)
	want := encodeInstr(opADD, 1, szQ, 1, 2, 0, 10)
	assertBytes(t, bin, 0, want, "add.q r1, r2, #10")
}

func TestIE64Asm_Sizes(t *testing.T) {
	tests := []struct {
		suffix string
		size   byte
	}{
		{".b", szB},
		{".w", szW},
		{".l", szL},
		{".q", szQ},
	}
	for _, tc := range tests {
		src := "move" + tc.suffix + " r1, r2"
		bin := assembleString(t, src)
		want := encodeInstr(opMOVE, 1, tc.size, 0, 2, 0, 0)
		assertBytes(t, bin, 0, want, "size "+tc.suffix)
	}
}

func TestIE64Asm_AllArithmetic(t *testing.T) {
	// Three-operand: sub, mulu, muls, divu, divs, mod
	threeOp := []struct {
		mnem   string
		opcode byte
	}{
		{"sub", opSUB},
		{"mulu", opMULU},
		{"muls", opMULS},
		{"divu", opDIVU},
		{"divs", opDIVS},
		{"mod", opMOD},
	}
	for _, tc := range threeOp {
		src := tc.mnem + ".q r1, r2, r3"
		bin := assembleString(t, src)
		want := encodeInstr(tc.opcode, 1, szQ, 0, 2, 3, 0)
		assertBytes(t, bin, 0, want, tc.mnem+" reg")

		// Immediate form
		src = tc.mnem + ".q r1, r2, #7"
		bin = assembleString(t, src)
		want = encodeInstr(tc.opcode, 1, szQ, 1, 2, 0, 7)
		assertBytes(t, bin, 0, want, tc.mnem+" imm")
	}

	// Two-operand: neg rd, rs
	src := "neg.q r1, r2"
	bin := assembleString(t, src)
	want := encodeInstr(opNEG, 1, szQ, 0, 2, 0, 0)
	assertBytes(t, bin, 0, want, "neg")
}

func TestIE64Asm_AllLogical(t *testing.T) {
	threeOp := []struct {
		mnem   string
		opcode byte
	}{
		{"and", opAND},
		{"or", opOR},
		{"eor", opEOR},
	}
	for _, tc := range threeOp {
		src := tc.mnem + ".q r1, r2, r3"
		bin := assembleString(t, src)
		want := encodeInstr(tc.opcode, 1, szQ, 0, 2, 3, 0)
		assertBytes(t, bin, 0, want, tc.mnem+" reg")

		src = tc.mnem + ".q r1, r2, #$FF"
		bin = assembleString(t, src)
		want = encodeInstr(tc.opcode, 1, szQ, 1, 2, 0, 0xFF)
		assertBytes(t, bin, 0, want, tc.mnem+" imm")
	}

	// Two-operand: not rd, rs
	src := "not.q r1, r2"
	bin := assembleString(t, src)
	want := encodeInstr(opNOT, 1, szQ, 0, 2, 0, 0)
	assertBytes(t, bin, 0, want, "not")
}

func TestIE64Asm_AllShifts(t *testing.T) {
	shifts := []struct {
		mnem   string
		opcode byte
	}{
		{"lsl", opLSL},
		{"lsr", opLSR},
		{"asr", opASR},
	}
	for _, tc := range shifts {
		// Register form
		src := tc.mnem + ".q r1, r2, r3"
		bin := assembleString(t, src)
		want := encodeInstr(tc.opcode, 1, szQ, 0, 2, 3, 0)
		assertBytes(t, bin, 0, want, tc.mnem+" reg")

		// Immediate form
		src = tc.mnem + ".q r1, r2, #4"
		bin = assembleString(t, src)
		want = encodeInstr(tc.opcode, 1, szQ, 1, 2, 0, 4)
		assertBytes(t, bin, 0, want, tc.mnem+" imm")
	}
}

func TestIE64Asm_Branch(t *testing.T) {
	// bne r1, r0, label
	// Layout: label: @ $1000, bne @ $1008
	// Offset = target - PC = $1000 - $1008 = -8 = 0xFFFFFFF8
	src := `
label:
		nop
		bne r1, r0, label
`
	bin := assembleString(t, src)
	assertLen(t, bin, 16, "branch")
	// BNE uses rs (byte2) and rt (byte3) per CPU decode: cpu.regs[rs] != cpu.regs[rt]
	want := encodeInstr(opBNE, 0, szQ, 0, 1, 0, 0xFFFFFFF8)
	assertBytes(t, bin, 8, want, "bne backward")
}

func TestIE64Asm_Bra(t *testing.T) {
	// bra label — unconditional, forward reference
	// bra @ $1000, nop @ $1008, label: @ $1010
	// Offset = $1010 - $1000 = 16 = 0x10
	src := `
		bra label
		nop
label:
		nop
`
	bin := assembleString(t, src)
	assertLen(t, bin, 24, "bra")
	want := encodeInstr(opBRA, 0, szQ, 0, 0, 0, 0x10)
	assertBytes(t, bin, 0, want, "bra forward")
}

func TestIE64Asm_Jsr_Rts(t *testing.T) {
	// jsr target — similar to bra but saves return address
	// rts — return from subroutine (no operands)
	src := `
		jsr sub
		halt
sub:
		nop
		rts
`
	bin := assembleString(t, src)
	assertLen(t, bin, 4*8, "jsr+rts")

	// jsr @ $1000, target sub @ $1010 => offset = $1010 - $1000 = 16
	wantJsr := encodeInstr(opJSR, 0, szQ, 0, 0, 0, 0x10)
	assertBytes(t, bin, 0, wantJsr, "jsr sub")

	wantRts := encodeInstr(opRTS, 0, 0, 0, 0, 0, 0)
	assertBytes(t, bin, 24, wantRts, "rts")
}

func TestIE64Asm_Push_Pop(t *testing.T) {
	src := `
		push r5
		pop r5
`
	bin := assembleString(t, src)
	assertLen(t, bin, 16, "push/pop")
	// PUSH uses rs (byte2) per CPU: cpu.regs[rs]; POP uses rd (byte1) per CPU: cpu.setReg(rd, ...)
	wantPush := encodeInstr(opPUSH, 0, szQ, 0, 5, 0, 0)
	wantPop := encodeInstr(opPOP, 5, szQ, 0, 0, 0, 0)
	assertBytes(t, bin, 0, wantPush, "push r5")
	assertBytes(t, bin, 8, wantPop, "pop r5")
}

func TestIE64Asm_System(t *testing.T) {
	mnemonics := []struct {
		mnem   string
		opcode byte
	}{
		{"nop", opNOP},
		{"halt", opHALT},
		{"sei", opSEI},
		{"cli", opCLI},
		{"rti", opRTI},
		// wait removed — it requires an operand and sets xbit=1, tested separately
	}
	for _, tc := range mnemonics {
		bin := assembleString(t, tc.mnem)
		assertLen(t, bin, 8, tc.mnem)
		want := encodeInstr(tc.opcode, 0, 0, 0, 0, 0, 0)
		assertBytes(t, bin, 0, want, tc.mnem)
	}
}

// ---------------------------------------------------------------------------
// Step 3d: Local labels
// ---------------------------------------------------------------------------

func TestIE64Asm_LocalLabel(t *testing.T) {
	// .loop is scoped to the global label "main_func"
	src := `
main_func:
.loop:
		nop
		bra .loop
`
	bin := assembleString(t, src)
	assertLen(t, bin, 16, "local label")
	// bra @ $1008, target .loop @ $1000 => offset = $1000 - $1008 = -8 = 0xFFFFFFF8
	want := encodeInstr(opBRA, 0, szQ, 0, 0, 0, 0xFFFFFFF8)
	assertBytes(t, bin, 8, want, "bra .loop")
}

func TestIE64Asm_LocalLabel_MultiScope(t *testing.T) {
	// Same local label name ".loop" in different global scopes must resolve
	// to different addresses.
	src := `
func_a:
.loop:
		nop
		bra .loop

func_b:
.loop:
		nop
		bra .loop
`
	bin := assembleString(t, src)
	assertLen(t, bin, 32, "multi-scope local labels")

	// func_a's bra @ $1008 -> .loop @ $1000 => -8
	wantA := encodeInstr(opBRA, 0, szQ, 0, 0, 0, 0xFFFFFFF8)
	assertBytes(t, bin, 8, wantA, "func_a .loop")

	// func_b's bra @ $1018 -> .loop @ $1010 => -8
	wantB := encodeInstr(opBRA, 0, szQ, 0, 0, 0, 0xFFFFFFF8)
	assertBytes(t, bin, 24, wantB, "func_b .loop")
}

func TestIE64Asm_LocalLabel_Forward(t *testing.T) {
	// Forward reference to a local label.
	src := `
main_func:
		bra .skip
		nop
.skip:
		halt
`
	bin := assembleString(t, src)
	assertLen(t, bin, 24, "local label forward")
	// bra @ $1000, target .skip @ $1010 => offset = $1010 - $1000 = 16
	want := encodeInstr(opBRA, 0, szQ, 0, 0, 0, 0x10)
	assertBytes(t, bin, 0, want, "bra .skip forward")
}

// ---------------------------------------------------------------------------
// Step 3e: Macros
// ---------------------------------------------------------------------------

func TestIE64Asm_Macro_NoArgs(t *testing.T) {
	src := `
pushall		macro
		push r1
		push r2
		push r3
		endm

		pushall
`
	bin := assembleString(t, src)
	assertLen(t, bin, 3*8, "macro no args")
	for i, reg := range []byte{1, 2, 3} {
		// PUSH uses rs field (byte2) per CPU: cpu.regs[rs]
		want := encodeInstr(opPUSH, 0, szQ, 0, reg, 0, 0)
		assertBytes(t, bin, i*8, want, "pushall expansion")
	}
}

func TestIE64Asm_Macro_WithArgs(t *testing.T) {
	src := `
loadimm		macro
		move.q \1, #\2
		endm

		loadimm r5, $DEAD
`
	bin := assembleString(t, src)
	assertLen(t, bin, 8, "macro with args")
	want := encodeInstr(opMOVE, 5, szQ, 1, 0, 0, 0xDEAD)
	assertBytes(t, bin, 0, want, "loadimm r5, $DEAD")
}

func TestIE64Asm_Macro_Expansion(t *testing.T) {
	src := `
addi		macro
		add.q \1, \1, #\2
		endm

		addi r3, 100
`
	bin := assembleString(t, src)
	assertLen(t, bin, 8, "macro expansion")
	want := encodeInstr(opADD, 3, szQ, 1, 3, 0, 100)
	assertBytes(t, bin, 0, want, "addi r3, 100")
}

func TestIE64Asm_Macro_Nested(t *testing.T) {
	src := `
clear		macro
		move.q \1, r0
		endm

clearall	macro
		clear r1
		clear r2
		clear r3
		endm

		clearall
`
	bin := assembleString(t, src)
	assertLen(t, bin, 3*8, "nested macros")
	for i, reg := range []byte{1, 2, 3} {
		want := encodeInstr(opMOVE, reg, szQ, 0, 0, 0, 0)
		assertBytes(t, bin, i*8, want, "clearall expansion")
	}
}

func TestIE64Asm_Macro_NARG(t *testing.T) {
	// narg gives the number of arguments passed to the macro.
	// Depending on narg, we conditionally assemble different code.
	src := `
flex		macro
		move.q \1, r0
		if narg>1
		move.q \2, r0
		endif
		endm

		flex r5
`
	bin := assembleString(t, src)
	// Only 1 arg, so only one move should be emitted.
	assertLen(t, bin, 8, "narg=1")
	want := encodeInstr(opMOVE, 5, szQ, 0, 0, 0, 0)
	assertBytes(t, bin, 0, want, "flex r5 (1 arg)")

	// Now with 2 args
	src2 := `
flex		macro
		move.q \1, r0
		if narg>1
		move.q \2, r0
		endif
		endm

		flex r5, r6
`
	bin2 := assembleString(t, src2)
	assertLen(t, bin2, 16, "narg=2")
	want1 := encodeInstr(opMOVE, 5, szQ, 0, 0, 0, 0)
	want2 := encodeInstr(opMOVE, 6, szQ, 0, 0, 0, 0)
	assertBytes(t, bin2, 0, want1, "flex r5 (2 args, first)")
	assertBytes(t, bin2, 8, want2, "flex r6 (2 args, second)")
}

// ---------------------------------------------------------------------------
// Step 3f: Conditional assembly + rept + set
// ---------------------------------------------------------------------------

func TestIE64Asm_If_True(t *testing.T) {
	src := `
		if 1
		nop
		endif
`
	bin := assembleString(t, src)
	assertLen(t, bin, 8, "if 1")
	want := encodeInstr(opNOP, 0, 0, 0, 0, 0, 0)
	assertBytes(t, bin, 0, want, "if 1 -> nop")
}

func TestIE64Asm_If_False(t *testing.T) {
	src := `
		if 0
		nop
		endif
`
	bin := assembleString(t, src)
	assertLen(t, bin, 0, "if 0")
}

func TestIE64Asm_If_Else(t *testing.T) {
	src := `
		if 0
		nop
		else
		halt
		endif
`
	bin := assembleString(t, src)
	assertLen(t, bin, 8, "if/else")
	want := encodeInstr(opHALT, 0, 0, 0, 0, 0, 0)
	assertBytes(t, bin, 0, want, "else branch -> halt")
}

func TestIE64Asm_If_Nested(t *testing.T) {
	src := `
		if 1
		if 0
		nop
		else
		halt
		endif
		sei
		endif
`
	bin := assembleString(t, src)
	// Inner if 0 takes else => halt. Then sei from outer if 1.
	assertLen(t, bin, 16, "nested if")
	wantHalt := encodeInstr(opHALT, 0, 0, 0, 0, 0, 0)
	wantSei := encodeInstr(opSEI, 0, 0, 0, 0, 0, 0)
	assertBytes(t, bin, 0, wantHalt, "inner else -> halt")
	assertBytes(t, bin, 8, wantSei, "outer if -> sei")
}

func TestIE64Asm_Rept(t *testing.T) {
	src := `
		rept 4
		nop
		endr
`
	bin := assembleString(t, src)
	assertLen(t, bin, 4*8, "rept 4")
	wantNop := encodeInstr(opNOP, 0, 0, 0, 0, 0, 0)
	for i := 0; i < 4; i++ {
		assertBytes(t, bin, i*8, wantNop, "rept nop")
	}
}

func TestIE64Asm_Set(t *testing.T) {
	// "set" creates a reassignable symbol, unlike "equ" which is fixed.
	src := `
counter set 0
		move.q r1, #counter
counter set 10
		move.q r2, #counter
`
	bin := assembleString(t, src)
	assertLen(t, bin, 16, "set")
	want0 := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0)
	want10 := encodeInstr(opMOVE, 2, szQ, 1, 0, 0, 10)
	assertBytes(t, bin, 0, want0, "set counter=0")
	assertBytes(t, bin, 8, want10, "set counter=10")
}

func TestIE64Asm_Set_InExpr(t *testing.T) {
	src := `
base set $A0000
offset set $100
		move.q r1, #base+offset
`
	bin := assembleString(t, src)
	want := encodeInstr(opMOVE, 1, szQ, 1, 0, 0, 0xA0100)
	assertBytes(t, bin, 0, want, "set in expr")
}

// ---------------------------------------------------------------------------
// Step 3g: Pseudo-instructions
// ---------------------------------------------------------------------------

func TestIE64Asm_La(t *testing.T) {
	// la r1, $A0000 expands to lea r1, $A0000(r0)
	src := "la r1, $A0000"
	bin := assembleString(t, src)
	assertLen(t, bin, 8, "la")
	want := encodeInstr(opLEA, 1, szQ, 1, 0, 0, 0xA0000)
	assertBytes(t, bin, 0, want, "la r1, $A0000")
}

func TestIE64Asm_Li_32bit(t *testing.T) {
	// li r1, #$DEADBEEF with value fitting in 32 bits => move.l r1, #$DEADBEEF
	src := "li r1, #$DEADBEEF"
	bin := assembleString(t, src)
	assertLen(t, bin, 8, "li 32-bit")
	want := encodeInstr(opMOVE, 1, szL, 1, 0, 0, 0xDEADBEEF)
	assertBytes(t, bin, 0, want, "li r1, #$DEADBEEF")
}

func TestIE64Asm_Li_64bit(t *testing.T) {
	// li r1, #$CAFEBABE_DEADBEEF => two instructions:
	//   move.l r1, #$DEADBEEF   (low 32 bits)
	//   movt r1, #$CAFEBABE     (high 32 bits)
	src := "li r1, #$CAFEBABE_DEADBEEF"
	bin := assembleString(t, src)
	assertLen(t, bin, 16, "li 64-bit")
	wantLow := encodeInstr(opMOVE, 1, szL, 1, 0, 0, 0xDEADBEEF)
	wantHigh := encodeInstr(opMOVT, 1, szQ, 1, 0, 0, 0xCAFEBABE)
	assertBytes(t, bin, 0, wantLow, "li low half")
	assertBytes(t, bin, 8, wantHigh, "li high half")
}

func TestIE64Asm_Beqz(t *testing.T) {
	// beqz r1, label => beq r1, r0, label
	src := `
		beqz r1, target
		nop
target:
		halt
`
	bin := assembleString(t, src)
	assertLen(t, bin, 24, "beqz")
	// beqz expands to beq r1, r0, target; CPU uses rs/rt in byte2/byte3
	want := encodeInstr(opBEQ, 0, szQ, 0, 1, 0, 0x10)
	assertBytes(t, bin, 0, want, "beqz r1, target")
}

func TestIE64Asm_Bnez(t *testing.T) {
	src := `
		bnez r1, target
		nop
target:
		halt
`
	bin := assembleString(t, src)
	assertLen(t, bin, 24, "bnez")
	want := encodeInstr(opBNE, 0, szQ, 0, 1, 0, 0x10)
	assertBytes(t, bin, 0, want, "bnez r1, target")
}

func TestIE64Asm_Bltz(t *testing.T) {
	src := `
		bltz r1, target
		nop
target:
		halt
`
	bin := assembleString(t, src)
	assertLen(t, bin, 24, "bltz")
	want := encodeInstr(opBLT, 0, szQ, 0, 1, 0, 0x10)
	assertBytes(t, bin, 0, want, "bltz r1, target")
}

func TestIE64Asm_Bgez(t *testing.T) {
	src := `
		bgez r1, target
		nop
target:
		halt
`
	bin := assembleString(t, src)
	assertLen(t, bin, 24, "bgez")
	want := encodeInstr(opBGE, 0, szQ, 0, 1, 0, 0x10)
	assertBytes(t, bin, 0, want, "bgez r1, target")
}

func TestIE64Asm_Bgtz(t *testing.T) {
	src := `
		bgtz r1, target
		nop
target:
		halt
`
	bin := assembleString(t, src)
	assertLen(t, bin, 24, "bgtz")
	want := encodeInstr(opBGT, 0, szQ, 0, 1, 0, 0x10)
	assertBytes(t, bin, 0, want, "bgtz r1, target")
}

func TestIE64Asm_Blez(t *testing.T) {
	src := `
		blez r1, target
		nop
target:
		halt
`
	bin := assembleString(t, src)
	assertLen(t, bin, 24, "blez")
	want := encodeInstr(opBLE, 0, szQ, 0, 1, 0, 0x10)
	assertBytes(t, bin, 0, want, "blez r1, target")
}

// ---------------------------------------------------------------------------
// Step 3h: Integration / error handling
// ---------------------------------------------------------------------------

func TestIE64Asm_FullProgram(t *testing.T) {
	// A complete program: clear a framebuffer region.
	//   org $1000
	//   SCREEN equ $A0000
	//   COUNT  equ 64000
	//
	//   start:
	//       la r1, SCREEN       ; r1 = base address
	//       move.q r2, #COUNT   ; r2 = pixel count
	//       move.q r3, #$1F     ; r3 = color (blue)
	//   .loop:
	//       store.b r3, (r1)    ; write pixel
	//       add.q r1, r1, #1    ; advance pointer
	//       sub.q r2, r2, #1    ; decrement counter
	//       bnez r2, .loop      ; loop until done
	//       halt
	src := `
		org $1000
SCREEN equ $A0000
COUNT  equ 64000

start:
		la r1, SCREEN
		move.q r2, #COUNT
		move.q r3, #$1F
.loop:
		store.b r3, (r1)
		add.q r1, r1, #1
		sub.q r2, r2, #1
		bnez r2, .loop
		halt
`
	bin := assembleString(t, src)
	// 8 instructions * 8 bytes = 64 bytes
	assertLen(t, bin, 8*8, "full program")

	// Verify each instruction:
	// 0: la r1, $A0000 => lea r1, $A0000(r0)
	assertBytes(t, bin, 0, encodeInstr(opLEA, 1, szQ, 1, 0, 0, 0xA0000), "la r1, SCREEN")

	// 8: move.q r2, #64000
	assertBytes(t, bin, 8, encodeInstr(opMOVE, 2, szQ, 1, 0, 0, 64000), "move.q r2, #COUNT")

	// 16: move.q r3, #$1F
	assertBytes(t, bin, 16, encodeInstr(opMOVE, 3, szQ, 1, 0, 0, 0x1F), "move.q r3, #$1F")

	// 24: store.b r3, (r1) — size .b = 0, X=0 (register indirect, no offset)
	assertBytes(t, bin, 24, encodeInstr(opSTORE, 3, szB, 0, 1, 0, 0), "store.b r3, (r1)")

	// 32: add.q r1, r1, #1
	assertBytes(t, bin, 32, encodeInstr(opADD, 1, szQ, 1, 1, 0, 1), "add.q r1, r1, #1")

	// 40: sub.q r2, r2, #1
	assertBytes(t, bin, 40, encodeInstr(opSUB, 2, szQ, 1, 2, 0, 1), "sub.q r2, r2, #1")

	// 48: bnez r2, .loop => bne r2, r0, .loop
	// .loop @ $1018 ($1000 + 3*8 = $1018), bnez @ $1030 ($1000 + 6*8 = $1030)
	// offset = $1018 - $1030 = -24 = 0xFFFFFFE8
	// bnez r2, .loop => bne r2, r0 — CPU uses rs (byte2) for first reg
	assertBytes(t, bin, 48, encodeInstr(opBNE, 0, szQ, 0, 2, 0, 0xFFFFFFE8), "bnez r2, .loop")

	// 56: halt
	assertBytes(t, bin, 56, encodeInstr(opHALT, 0, 0, 0, 0, 0, 0), "halt")
}

func TestIE64Asm_ErrorHandling(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "undefined label",
			src:  "bra nowhere",
		},
		{
			name: "bad register",
			src:  "move.q r99, r1",
		},
		{
			name: "invalid register name",
			src:  "move.q rx, r1",
		},
		{
			name: "unknown mnemonic",
			src:  "foobar r1, r2",
		},
		{
			name: "missing operand",
			src:  "move.q r1",
		},
		{
			name: "bad immediate",
			src:  "move.q r1, #xyz",
		},
		{
			name: "duplicate equ",
			src:  "FOO equ 1\nFOO equ 2",
		},
		{
			name: "invalid size suffix",
			src:  "move.x r1, r2",
		},
		{
			name: "unclosed string",
			src:  `dc.b "hello`,
		},
		{
			name: "bad hex literal",
			src:  "move.q r1, #$ZZZZ",
		},
		{
			name: "unmatched endif",
			src:  "endif",
		},
		{
			name: "unmatched endm",
			src:  "endm",
		},
		{
			name: "if without endif",
			src:  "if 1\nnop",
		},
		{
			name: "rept without endr",
			src:  "rept 3\nnop",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assembleExpectError(t, tc.src)
		})
	}
}

// ---------------------------------------------------------------------------
// Step 3h: Listing mode + lint warnings
// ---------------------------------------------------------------------------

func TestIE64Asm_ListingMode(t *testing.T) {
	src := `
		org $1000
		move.q r1, #42
		nop
`
	asm := NewIE64Assembler()
	asm.SetListingMode(true)
	_, err := asm.Assemble(src)
	if err != nil {
		t.Fatalf("assembly failed: %v", err)
	}
	listing := asm.GetListing()
	if len(listing) == 0 {
		t.Fatal("listing mode produced no output")
	}
	// Listing should contain address, hex bytes, and source
	joined := strings.Join(listing, "\n")
	if !strings.Contains(joined, "00001000") {
		t.Errorf("listing missing address 00001000, got:\n%s", joined)
	}
	if !strings.Contains(joined, "move.q") && !strings.Contains(joined, "MOVE") {
		t.Errorf("listing missing source mnemonic, got:\n%s", joined)
	}
	if !strings.Contains(joined, "nop") && !strings.Contains(joined, "NOP") {
		t.Errorf("listing missing nop, got:\n%s", joined)
	}
}

func TestIE64Asm_Lint_PseudoOpWarning(t *testing.T) {
	src := `la r1, $A0000`
	asm := NewIE64Assembler()
	_, err := asm.Assemble(src)
	if err != nil {
		t.Fatalf("assembly failed: %v", err)
	}
	warnings := asm.GetWarnings()
	found := false
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "pseudo") && strings.Contains(strings.ToLower(w), "lea") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected pseudo-op lowering warning for 'la', got warnings: %v", warnings)
	}
}

func TestIE64Asm_Lint_SizeTruncation(t *testing.T) {
	src := `move.b r1, #$1FF`
	asm := NewIE64Assembler()
	_, err := asm.Assemble(src)
	if err != nil {
		t.Fatalf("assembly failed: %v", err)
	}
	warnings := asm.GetWarnings()
	found := false
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "truncat") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected size truncation warning for move.b with #$1FF, got warnings: %v", warnings)
	}
}

// ===========================================================================
// JMP (register-indirect jump)
// ===========================================================================

func TestIE64Asm_Jmp_Register(t *testing.T) {
	// jmp (r5) -> opcode 0x49, rs=5, imm32=0
	bin := assembleString(t, "jmp (r5)")
	expected := encodeInstr(opJMP, 0, 0, 0, 5, 0, 0)
	assertBytes(t, bin, 0, expected, "jmp (r5)")
}

func TestIE64Asm_Jmp_SP(t *testing.T) {
	// jmp (sp) -> rs=31
	bin := assembleString(t, "jmp (sp)")
	expected := encodeInstr(opJMP, 0, 0, 0, 31, 0, 0)
	assertBytes(t, bin, 0, expected, "jmp (sp)")
}

func TestIE64Asm_Jmp_Displacement(t *testing.T) {
	// jmp 16(r5) -> imm32=16
	bin := assembleString(t, "jmp 16(r5)")
	expected := encodeInstr(opJMP, 0, 0, 0, 5, 0, 16)
	assertBytes(t, bin, 0, expected, "jmp 16(r5)")
}

func TestIE64Asm_Jmp_NegativeDisplacement(t *testing.T) {
	// jmp -8(r5) -> imm32=0xFFFFFFF8
	bin := assembleString(t, "jmp -8(r5)")
	expected := encodeInstr(opJMP, 0, 0, 0, 5, 0, 0xFFFFFFF8)
	assertBytes(t, bin, 0, expected, "jmp -8(r5)")
}

func TestIE64Asm_Jmp_Error_NoOperand(t *testing.T) {
	assembleExpectError(t, "jmp")
}

func TestIE64Asm_Jmp_Error_Label(t *testing.T) {
	// jmp label should error — use bra for label-based jumps
	assembleExpectError(t, "jmp target\ntarget:\nhalt")
}

// ===========================================================================
// JSR Indirect (register-indirect subroutine call)
// ===========================================================================

func TestIE64Asm_Jsr_Indirect_Register(t *testing.T) {
	// jsr (r5) -> opcode 0x54, rs=5, imm32=0
	bin := assembleString(t, "jsr (r5)")
	expected := encodeInstr(opJSRInd, 0, 0, 0, 5, 0, 0)
	assertBytes(t, bin, 0, expected, "jsr (r5)")
}

func TestIE64Asm_Jsr_Indirect_Displacement(t *testing.T) {
	// jsr 16(r5) -> opcode 0x54, imm32=16
	bin := assembleString(t, "jsr 16(r5)")
	expected := encodeInstr(opJSRInd, 0, 0, 0, 5, 0, 16)
	assertBytes(t, bin, 0, expected, "jsr 16(r5)")
}

func TestIE64Asm_Jsr_Label_Unchanged(t *testing.T) {
	// jsr label should still emit opcode 0x50 (PC-relative)
	src := "jsr target\nhalt\ntarget:\nhalt"
	bin := assembleString(t, src)
	if bin[0] != opJSR {
		t.Fatalf("opcode = 0x%02X, want 0x%02X (jsr label should use PC-relative)", bin[0], opJSR)
	}
}

func TestIE64Asm_Jsr_Rts_Indirect_Roundtrip(t *testing.T) {
	// Full program encoding check
	src := "jsr (r5)\nrts"
	bin := assembleString(t, src)
	expected1 := encodeInstr(opJSRInd, 0, 0, 0, 5, 0, 0)
	expected2 := encodeInstr(opRTS, 0, 0, 0, 0, 0, 0)
	assertBytes(t, bin, 0, expected1, "jsr (r5)")
	assertBytes(t, bin, 8, expected2, "rts")
}

func TestIE64Asm_FPU(t *testing.T) {
	t.Run("FMOV", func(t *testing.T) {
		src := "fmov f1, f2"
		bin := assembleString(t, src)
		want := encodeInstr(opFMOV, 1, szL, 0, 2, 0, 0)
		assertBytes(t, bin, 0, want, "fmov f1, f2")
	})

	t.Run("FLOAD", func(t *testing.T) {
		src := "fload f5, 16(r10)"
		bin := assembleString(t, src)
		want := encodeInstr(opFLOAD, 5, szL, 1, 10, 0, 16)
		assertBytes(t, bin, 0, want, "fload f5, 16(r10)")
	})

	t.Run("FSTORE", func(t *testing.T) {
		// Regression: ensure fstore is freg, mem and freg is encoded in rd
		src := "fstore f5, 8(r10)"
		bin := assembleString(t, src)
		want := encodeInstr(opFSTORE, 5, szL, 1, 10, 0, 8)
		assertBytes(t, bin, 0, want, "fstore f5, 8(r10)")
	})

	t.Run("FADD", func(t *testing.T) {
		src := "fadd f0, f1, f2"
		bin := assembleString(t, src)
		want := encodeInstr(opFADD, 0, szL, 0, 1, 2, 0)
		assertBytes(t, bin, 0, want, "fadd f0, f1, f2")
	})

	t.Run("FCMP", func(t *testing.T) {
		src := "fcmp r1, f4, f5"
		bin := assembleString(t, src)
		want := encodeInstr(opFCMP, 1, szL, 0, 4, 5, 0)
		assertBytes(t, bin, 0, want, "fcmp r1, f4, f5")
	})

	t.Run("FMOVI", func(t *testing.T) {
		src := "fmovi f0, r8"
		bin := assembleString(t, src)
		want := encodeInstr(opFMOVI, 0, szL, 0, 8, 0, 0)
		assertBytes(t, bin, 0, want, "fmovi f0, r8")
	})

	t.Run("FMOVO", func(t *testing.T) {
		src := "fmovo r8, f0"
		bin := assembleString(t, src)
		want := encodeInstr(opFMOVO, 8, szL, 0, 0, 0, 0)
		assertBytes(t, bin, 0, want, "fmovo r8, f0")
	})

	t.Run("FCVTIF", func(t *testing.T) {
		src := "fcvtif f0, r8"
		bin := assembleString(t, src)
		want := encodeInstr(opFCVTIF, 0, szL, 0, 8, 0, 0)
		assertBytes(t, bin, 0, want, "fcvtif f0, r8")
	})

	t.Run("FCVTFI", func(t *testing.T) {
		src := "fcvtfi r8, f0"
		bin := assembleString(t, src)
		want := encodeInstr(opFCVTFI, 8, szL, 0, 0, 0, 0)
		assertBytes(t, bin, 0, want, "fcvtfi r8, f0")
	})

	t.Run("FMOVSR", func(t *testing.T) {
		src := "fmovsr r8"
		bin := assembleString(t, src)
		want := encodeInstr(opFMOVSR, 8, szL, 0, 0, 0, 0)
		assertBytes(t, bin, 0, want, "fmovsr r8")
	})

	t.Run("RejectSizeSuffix", func(t *testing.T) {
		src := "fadd.l f0, f1, f2"
		err := assembleExpectError(t, src)
		if !strings.Contains(err.Error(), "size suffixes not allowed") {
			t.Errorf("Expected size suffix error, got: %v", err)
		}
	})

	t.Run("RejectMemoryFirstFSTORE", func(t *testing.T) {
		// Ensure the old memory-first order is rejected
		src := "fstore 8(r10), f5"
		assembleExpectError(t, src)
	})
}

// ---------------------------------------------------------------------------
// Ensure unused imports are consumed (prevent compiler warnings in test-only
// builds where some helpers might not reference every import directly).
// ---------------------------------------------------------------------------

var _ = strings.TrimSpace
var _ = os.TempDir
var _ = filepath.Join
var _ = binary.LittleEndian
