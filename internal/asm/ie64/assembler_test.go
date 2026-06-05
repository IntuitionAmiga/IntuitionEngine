package ie64

import (
	"bytes"
	"strings"
	"testing"
)

const (
	testOPMove = 0x01
	testOPLea  = 0x04
	testOPBne  = 0x42
	testOPJSR  = 0x50
	testOPDsin = 0x91
	testOPDpow = 0x97
	testOPNop  = 0xE0
	testSizeL  = 2
	testSizeQ  = 3
)

func testEncode(opcode byte, rd, size, xbit, rs, rt byte, imm32 uint32) []byte {
	return []byte{
		opcode,
		(rd << 3) | (size << 1) | xbit,
		rs << 3,
		rt << 3,
		byte(imm32),
		byte(imm32 >> 8),
		byte(imm32 >> 16),
		byte(imm32 >> 24),
	}
}

func requireAssembled(t *testing.T, origin uint64, line string) []byte {
	t.Helper()
	res := AssembleInstruction(origin, line)
	if len(res.Diagnostics) != 0 {
		t.Fatalf("AssembleInstruction(%q) diagnostics = %#v", line, res.Diagnostics)
	}
	if res.Origin != origin {
		t.Fatalf("result origin = %#x, want %#x", res.Origin, origin)
	}
	if len(res.Bytes) != 8 {
		t.Fatalf("len(bytes) = %d, want 8 (% X)", len(res.Bytes), res.Bytes)
	}
	return res.Bytes
}

func requireDiagnostic(t *testing.T, line string, want string) {
	t.Helper()
	res := AssembleInstruction(0x1000, line)
	if len(res.Bytes) != 0 {
		t.Fatalf("AssembleInstruction(%q) bytes = % X, want no bytes", line, res.Bytes)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatalf("AssembleInstruction(%q) produced no diagnostic", line)
	}
	got := res.Diagnostics[0].Message
	if !strings.Contains(got, want) {
		t.Fatalf("AssembleInstruction(%q) diagnostic = %q, want containing %q", line, got, want)
	}
}

func TestAssembleInstructionRepresentativeEncodings(t *testing.T) {
	tests := []struct {
		line string
		want []byte
	}{
		{"move.l r2,#42", testEncode(testOPMove, 2, testSizeL, 1, 0, 0, 42)},
		{"move.q r3,r4", testEncode(testOPMove, 3, testSizeQ, 0, 4, 0, 0)},
		{"la r5,$2000", testEncode(testOPLea, 5, testSizeQ, 1, 0, 0, 0x2000)},
		{"nop", testEncode(testOPNop, 0, 0, 0, 0, 0, 0)},
	}
	for _, tt := range tests {
		got := requireAssembled(t, 0x1000, tt.line)
		if !bytes.Equal(got, tt.want) {
			t.Fatalf("%s bytes = % X, want % X", tt.line, got, tt.want)
		}
	}
}

func TestAssembleInstructionFP64Transcendentals(t *testing.T) {
	tests := []struct {
		line string
		want []byte
	}{
		{"dsin f0, f2", testEncode(testOPDsin, 0, testSizeL, 0, 2, 0, 0)},
		{"dpow f0, f2, f4", testEncode(testOPDpow, 0, testSizeL, 0, 2, 4, 0)},
	}
	for _, tt := range tests {
		got := requireAssembled(t, 0x1000, tt.line)
		if !bytes.Equal(got, tt.want) {
			t.Fatalf("%s bytes = % X, want % X", tt.line, got, tt.want)
		}
	}

	requireDiagnostic(t, "dsin f1, f2", "destination must use an even-numbered FP register")
	requireDiagnostic(t, "dpow f0, f2, f3", "source must use an even-numbered FP register")
}

func TestAssembleInstructionHighOriginPCRelative(t *testing.T) {
	origin := uint64(0x0000000100000000)

	for _, tt := range []struct {
		name string
		line string
		want []byte
	}{
		{
			name: "forward bne",
			line: "bne r1,r2,$0000000100000010",
			want: testEncode(testOPBne, 0, testSizeQ, 0, 1, 2, 0x10),
		},
		{
			name: "backward jsr",
			line: "jsr $00000000FFFFFFF8",
			want: testEncode(testOPJSR, 0, testSizeQ, 0, 0, 0, 0xFFFFFFF8),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := requireAssembled(t, origin, tt.line)
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("bytes = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestAssembleInstructionExpressionShiftPrecedenceMatchesIE64ASM(t *testing.T) {
	for _, tt := range []struct {
		line  string
		imm32 uint32
	}{
		{"move.l r1,#1+2<<3", 0x18},
		{"move.l r1,#1<<2+1", 0x08},
		{"move.l r1,#2*3<<1+1", 0x18},
	} {
		t.Run(tt.line, func(t *testing.T) {
			got := requireAssembled(t, 0x1000, tt.line)
			want := testEncode(testOPMove, 1, testSizeL, 1, 0, 0, tt.imm32)
			if !bytes.Equal(got, want) {
				t.Fatalf("%s bytes = % X, want % X", tt.line, got, want)
			}
		})
	}
}

func TestAssembleInstructionExpressionBitwiseOperatorsMatchIE64ASM(t *testing.T) {
	for _, tt := range []struct {
		line  string
		imm32 uint32
	}{
		{"move.l r1,#$F0|$0F", 0xFF},
		{"move.l r1,#$FF&$0F", 0x0F},
		{"move.l r1,#$F0^$FF", 0x0F},
		{"move.l r1,#$80|$03<<2+1", 0x98},
	} {
		t.Run(tt.line, func(t *testing.T) {
			got := requireAssembled(t, 0x1000, tt.line)
			want := testEncode(testOPMove, 1, testSizeL, 1, 0, 0, tt.imm32)
			if !bytes.Equal(got, want) {
				t.Fatalf("%s bytes = % X, want % X", tt.line, got, want)
			}
		})
	}
}

func TestAssembleInstructionExpressionCompareAndLogicalOperatorsMatchIE64ASM(t *testing.T) {
	for _, tt := range []struct {
		line  string
		imm32 uint32
	}{
		{"move.l r1,#1==1", 1},
		{"move.l r1,#1!=1", 0},
		{"move.l r1,#1<2", 1},
		{"move.l r1,#2>1", 1},
		{"move.l r1,#2<=1", 0},
		{"move.l r1,#2>=2", 1},
		{"move.l r1,#1||0", 1},
		{"move.l r1,#1&&0", 0},
		{"move.l r1,#!0", 1},
		{"move.l r1,#!(1==1)", 0},
		{"move.l r1,#$F0|$0F==$FF", 1},
		{"move.l r1,#1==1&&0", 0},
		{"move.l r1,#0||2<<3==16", 1},
	} {
		t.Run(tt.line, func(t *testing.T) {
			got := requireAssembled(t, 0x1000, tt.line)
			want := testEncode(testOPMove, 1, testSizeL, 1, 0, 0, tt.imm32)
			if !bytes.Equal(got, want) {
				t.Fatalf("%s bytes = % X, want % X", tt.line, got, want)
			}
		})
	}
}

func TestAssembleInstructionRejectsHighOriginBranchOutOfRange(t *testing.T) {
	res := AssembleInstruction(0x0000000100000000, "bra $0000000200000000")
	if len(res.Bytes) != 0 {
		t.Fatalf("bytes = % X, want no bytes", res.Bytes)
	}
	if len(res.Diagnostics) == 0 || !strings.Contains(res.Diagnostics[0].Message, "out of signed 32-bit range") {
		t.Fatalf("diagnostics = %#v, want signed-range diagnostic", res.Diagnostics)
	}
}

func TestAssembleInstructionRejectsMonitorUnsupportedForms(t *testing.T) {
	for _, tt := range []struct {
		line string
		want string
	}{
		{"", "no instruction assembled"},
		{"; comment", "no instruction assembled"},
		{"label:", "labels are not supported in monitor assemble mode"},
		{"label: nop", "labels are not supported in monitor assemble mode"},
		{"org $2000", "directives are not supported in monitor assemble mode"},
		{".org $2000", "directives are not supported in monitor assemble mode"},
		{"dc.b 1", "directives are not supported in monitor assemble mode"},
		{"include \"x.i\"", "include is not supported in monitor assemble mode"},
		{"incbin \"x.bin\"", "incbin is not supported in monitor assemble mode"},
		{"li r1,#$12345678", "pseudo-instruction expands to more than one instruction"},
		{"nop; nop", "multiple instructions on one line are not supported"},
		{"definitelybad r1", "unknown instruction"},
	} {
		t.Run(tt.line, func(t *testing.T) {
			requireDiagnostic(t, tt.line, tt.want)
		})
	}
}
