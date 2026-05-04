//go:build !ie64 && !ie64dis

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assembleIE32ForTest(t *testing.T, src string) []byte {
	t.Helper()
	asm := NewAssembler()
	asm.basePath = t.TempDir()
	bin, err := asm.assemble(src)
	if err != nil {
		t.Fatalf("assemble failed: %v", err)
	}
	return bin
}

func TestIE32RegIndirectOffsetEncoding(t *testing.T) {
	bin := assembleIE32ForTest(t, "LOAD A, [X+16]\nLOAD B, [X-16]\n")
	if got := bin[4:8]; !bytes.Equal(got, []byte{0x11, 0, 0, 0}) {
		t.Fatalf("[X+16] operand = % X, want 11 00 00 00", got)
	}
	if got := bin[12:16]; !bytes.Equal(got, []byte{0xF1, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf("[X-16] operand = % X, want F1 FF FF FF", got)
	}

	asm := NewAssembler()
	if _, err := asm.assemble("LOAD A, [X+4]\n"); err == nil || !strings.Contains(err.Error(), "multiple of 16") {
		t.Fatalf("expected 16-byte offset alignment error, got %v", err)
	}
}

func TestIE32LabelInstructionSameLineAndDuplicateWarning(t *testing.T) {
	asm := NewAssembler()
	bin, err := asm.assemble("start: LDA #target\nstart:\ntarget: HALT\n")
	if err != nil {
		t.Fatalf("assemble failed: %v", err)
	}
	if got := bin[4:8]; !bytes.Equal(got, []byte{0x08, 0x10, 0, 0}) {
		t.Fatalf("LDA target operand = % X, want 08 10 00 00", got)
	}
	if len(asm.warnings.Warnings()) != 1 || !strings.Contains(asm.warnings.Warnings()[0], "duplicate-labels") {
		t.Fatalf("expected duplicate-labels warning, got %v", asm.warnings.Warnings())
	}
}

func TestIE32ExpressionsEscapesAndRanges(t *testing.T) {
	bin := assembleIE32ForTest(t, `
.equ BASE target + 4
LDA #-1
target: .word BASE, -1, 0xFFFFFFFF
.ascii "A\n\x42"
.asciz "C"
`)
	if got := bin[4:8]; !bytes.Equal(got, []byte{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf("negative immediate encoded as % X", got)
	}
	if got := bin[8:20]; !bytes.Equal(got, []byte{0x0C, 0x10, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf(".word bytes = % X", got)
	}
	if got := bin[20:25]; !bytes.Equal(got, []byte{'A', '\n', 'B', 'C', 0}) {
		t.Fatalf("string bytes = %q (% X)", got, got)
	}

	asm := NewAssembler()
	if _, err := asm.assemble(".word -2147483649\n"); err == nil || !strings.Contains(err.Error(), "out of 32-bit range") {
		t.Fatalf("expected .word range error, got %v", err)
	}
}

func TestIE32EquateAvailableForDirectiveSizing(t *testing.T) {
	bin := assembleIE32ForTest(t, `
.equ SIZE 4
.space SIZE
HALT
`)
	if len(bin) != 12 {
		t.Fatalf("binary len = %d, want 12", len(bin))
	}
	if got := bin[4]; got != HALT {
		t.Fatalf("HALT opcode at offset 4 = 0x%02X, want 0x%02X", got, HALT)
	}
}

func TestIE32ForwardEquateRecomputesDirectiveSizing(t *testing.T) {
	bin := assembleIE32ForTest(t, `
.equ N end - start
.space N
start: HALT
end: HALT
`)
	if len(bin) != 24 {
		t.Fatalf("binary len = %d, want 24", len(bin))
	}
	if got := bin[8]; got != HALT {
		t.Fatalf("first HALT opcode at offset 8 = 0x%02X, want 0x%02X", got, HALT)
	}
	if got := bin[16]; got != HALT {
		t.Fatalf("second HALT opcode at offset 16 = 0x%02X, want 0x%02X", got, HALT)
	}
}

func TestIE32SpaceUsesFullExpression(t *testing.T) {
	bin := assembleIE32ForTest(t, `
.equ SIZE 4
.space SIZE + 4
HALT
`)
	if len(bin) != 16 {
		t.Fatalf("binary len = %d, want 16", len(bin))
	}
	if got := bin[8]; got != HALT {
		t.Fatalf("HALT opcode at offset 8 = 0x%02X, want 0x%02X", got, HALT)
	}
}

func TestIE32OperandExpressionRangeErrors(t *testing.T) {
	for name, src := range map[string]string{
		"positive overflow": "LDA #$1_0000_0000\n",
		"negative overflow": "LDA #-2147483649\n",
		"direct overflow":   "LDA @$1_0000_0000\n",
		"bare overflow":     "LDA $1_0000_0000\n",
	} {
		t.Run(name, func(t *testing.T) {
			asm := NewAssembler()
			_, err := asm.assemble(src)
			if err == nil || !strings.Contains(err.Error(), "out of 32-bit range") {
				t.Fatalf("expected range error, got %v", err)
			}
		})
	}
}

func TestIE32OrgOverlapAndIncbinCache(t *testing.T) {
	asm := NewAssembler()
	if _, err := asm.assemble(".org 0x1008\n.word 1\n.org 0x1008\n.word 2\n"); err == nil || !strings.Contains(err.Error(), "overlapping emit") {
		t.Fatalf("expected overlap error, got %v", err)
	}

	dir := t.TempDir()
	payload := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(payload, []byte{1, 2, 3, 4}, 0644); err != nil {
		t.Fatal(err)
	}
	asm = NewAssembler()
	asm.basePath = dir
	bin, err := asm.assemble(`.incbin "payload.bin", 1, 2`)
	if err != nil {
		t.Fatalf("incbin assemble failed: %v", err)
	}
	if got := bin[:2]; !bytes.Equal(got, []byte{2, 3}) {
		t.Fatalf("incbin bytes = % X, want 02 03", got)
	}
}

func TestIE32IncbinChangedWarnsOnSecondPass1ButUsesCache(t *testing.T) {
	dir := t.TempDir()
	payload := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(payload, []byte{1, 2, 3}, 0644); err != nil {
		t.Fatal(err)
	}
	asm := NewAssembler()
	asm.basePath = dir
	asm.pass = 1
	if size := asm.calcDirectiveSize(`.incbin "payload.bin"`); size != 3 {
		t.Fatalf("first incbin size = %d, want 3", size)
	}
	if err := os.WriteFile(payload, []byte{9, 9, 9}, 0644); err != nil {
		t.Fatal(err)
	}
	if size := asm.calcDirectiveSize(`.incbin "payload.bin"`); size != 3 {
		t.Fatalf("second incbin size = %d, want 3", size)
	}
	if got := asm.warnings.Warnings(); len(got) != 1 || !strings.Contains(got[0], "incbin-changed") {
		t.Fatalf("expected incbin-changed warning, got %v", got)
	}
	if !bytes.Equal(asm.incbinCache[payload], []byte{1, 2, 3}) {
		t.Fatalf("incbin cache served changed bytes")
	}
}
