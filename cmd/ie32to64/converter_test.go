package main

import (
	"os"
	"strings"
	"testing"
)

// ============================================================================
// Register Mapping Tests
// ============================================================================

func TestRegisterMapping(t *testing.T) {
	c := NewConverter()
	expected := map[string]string{
		"A": "r1", "X": "r2", "Y": "r3", "Z": "r4",
		"B": "r5", "C": "r6", "D": "r7", "E": "r8",
		"F": "r9", "G": "r10", "H": "r11", "S": "r12",
		"T": "r13", "U": "r14", "V": "r15", "W": "r16",
	}
	for ie32, ie64 := range expected {
		got, err := c.MapRegister(ie32)
		if err != nil {
			t.Errorf("MapRegister(%q) returned error: %v", ie32, err)
		}
		if got != ie64 {
			t.Errorf("MapRegister(%q) = %q, want %q", ie32, got, ie64)
		}
	}
}

func TestRegisterMapping_CaseInsensitive(t *testing.T) {
	c := NewConverter()
	got, err := c.MapRegister("a")
	if err != nil {
		t.Fatalf("MapRegister(\"a\") returned error: %v", err)
	}
	if got != "r1" {
		t.Errorf("MapRegister(\"a\") = %q, want \"r1\"", got)
	}
}

func TestRegisterMapping_Unknown(t *testing.T) {
	c := NewConverter()
	_, err := c.MapRegister("Q")
	if err == nil {
		t.Error("MapRegister(\"Q\") should return error")
	}
}

// ============================================================================
// SplitComment Tests
// ============================================================================

func TestSplitComment(t *testing.T) {
	code, comment := SplitComment("LDA #42 ; load")
	if code != "LDA #42" {
		t.Errorf("code = %q, want %q", code, "LDA #42")
	}
	if comment != "load" {
		t.Errorf("comment = %q, want %q", comment, "load")
	}
}

func TestSplitComment_NoComment(t *testing.T) {
	code, comment := SplitComment("LDA #42")
	if code != "LDA #42" {
		t.Errorf("code = %q, want %q", code, "LDA #42")
	}
	if comment != "" {
		t.Errorf("comment = %q, want empty", comment)
	}
}

func TestSplitComment_CommentOnly(t *testing.T) {
	code, comment := SplitComment("; comment here")
	if code != "" {
		t.Errorf("code = %q, want empty", code)
	}
	if comment != "comment here" {
		t.Errorf("comment = %q, want %q", comment, "comment here")
	}
}

func TestSplitComment_InString(t *testing.T) {
	code, comment := SplitComment(`.ascii "a;b"`)
	if code != `.ascii "a;b"` {
		t.Errorf("code = %q, want %q", code, `.ascii "a;b"`)
	}
	if comment != "" {
		t.Errorf("comment = %q, want empty", comment)
	}
}

func TestSplitComment_LabelWithComment(t *testing.T) {
	code, comment := SplitComment("foo: ; setup")
	if code != "foo:" {
		t.Errorf("code = %q, want %q", code, "foo:")
	}
	if comment != "setup" {
		t.Errorf("comment = %q, want %q", comment, "setup")
	}
}

// ============================================================================
// ClassifyLine Tests
// ============================================================================

func TestClassifyLine_Empty(t *testing.T) {
	if got := ClassifyLine(""); got != LineEmpty {
		t.Errorf("ClassifyLine(\"\") = %v, want LineEmpty", got)
	}
}

func TestClassifyLine_Label(t *testing.T) {
	if got := ClassifyLine("main_loop:"); got != LineLabel {
		t.Errorf("ClassifyLine(\"main_loop:\") = %v, want LineLabel", got)
	}
}

func TestClassifyLine_DotLabel(t *testing.T) {
	if got := ClassifyLine(".wait_start:"); got != LineLabel {
		t.Errorf("ClassifyLine(\".wait_start:\") = %v, want LineLabel (%v)", got, LineDirective)
	}
}

func TestClassifyLine_Directive(t *testing.T) {
	if got := ClassifyLine(".org 0x1000"); got != LineDirective {
		t.Errorf("ClassifyLine(\".org 0x1000\") = %v, want LineDirective", got)
	}
}

func TestClassifyLine_Instruction(t *testing.T) {
	if got := ClassifyLine("LDA #42"); got != LineInstruction {
		t.Errorf("ClassifyLine(\"LDA #42\") = %v, want LineInstruction", got)
	}
}

// ============================================================================
// ClassifyOperand Tests
// ============================================================================

func TestClassifyOperand_Immediate(t *testing.T) {
	if got := ClassifyOperand("#42"); got != OpImmediate {
		t.Errorf("got %v, want OpImmediate", got)
	}
}

func TestClassifyOperand_Direct(t *testing.T) {
	if got := ClassifyOperand("@0x5000"); got != OpDirect {
		t.Errorf("got %v, want OpDirect", got)
	}
}

func TestClassifyOperand_RegIndirect(t *testing.T) {
	if got := ClassifyOperand("[B]"); got != OpRegIndirect {
		t.Errorf("got %v, want OpRegIndirect", got)
	}
}

func TestClassifyOperand_RegOffset(t *testing.T) {
	if got := ClassifyOperand("[B+8]"); got != OpRegIndirect {
		t.Errorf("got %v, want OpRegIndirect", got)
	}
}

func TestClassifyOperand_BareNumber(t *testing.T) {
	if got := ClassifyOperand("0x5000"); got != OpBare {
		t.Errorf("got %v, want OpBare", got)
	}
}

func TestClassifyOperand_BareSymbol(t *testing.T) {
	if got := ClassifyOperand("MY_CONST"); got != OpBare {
		t.Errorf("got %v, want OpBare", got)
	}
}

func TestClassifyOperandWithReg_Register(t *testing.T) {
	c := NewConverter()
	if got := c.ClassifyOperandWithReg("A"); got != OpRegister {
		t.Errorf("got %v, want OpRegister", got)
	}
}

// ============================================================================
// Directive Conversion Tests
// ============================================================================

func TestConvertDirective_Org(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(".org 0x1000")
	if got != "org 0x1000" {
		t.Errorf("got %q, want %q", got, "org 0x1000")
	}
}

func TestConvertDirective_Equ(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(".equ NAME 0x1234")
	if got != "NAME equ 0x1234" {
		t.Errorf("got %q, want %q", got, "NAME equ 0x1234")
	}
}

func TestConvertDirective_EquExpr(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(".equ ENTRIES RING_BASE + 0x08")
	if got != "ENTRIES equ RING_BASE + 0x08" {
		t.Errorf("got %q, want %q", got, "ENTRIES equ RING_BASE + 0x08")
	}
}

func TestConvertDirective_Word(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(".word 1, 2")
	if got != "dc.l 1, 2" {
		t.Errorf("got %q, want %q", got, "dc.l 1, 2")
	}
}

func TestConvertDirective_Byte(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(".byte 0x12")
	if got != "dc.b 0x12" {
		t.Errorf("got %q, want %q", got, "dc.b 0x12")
	}
}

func TestConvertDirective_Space(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(".space 256")
	if got != "ds.b 256" {
		t.Errorf("got %q, want %q", got, "ds.b 256")
	}
}

func TestConvertDirective_Ascii(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(`.ascii "hello"`)
	if got != `dc.b "hello"` {
		t.Errorf("got %q, want %q", got, `dc.b "hello"`)
	}
}

func TestConvertDirective_Incbin(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(`.incbin "data.bin"`)
	if got != `incbin "data.bin"` {
		t.Errorf("got %q, want %q", got, `incbin "data.bin"`)
	}
}

func TestConvertDirective_IncbinWithArgs(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(`.incbin "data.bin", 0x100, 0x200`)
	if got != `incbin "data.bin", 0x100, 0x200` {
		t.Errorf("got %q, want %q", got, `incbin "data.bin", 0x100, 0x200`)
	}
}

func TestConvertDirective_Include(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(`.include "ie32.inc"`)
	if got != `include "ie64.inc"` {
		t.Errorf("got %q, want %q", got, `include "ie64.inc"`)
	}
}

func TestConvertDirective_IncludeOther(t *testing.T) {
	c := NewConverter()
	got := c.ConvertDirective(`.include "mylib.inc"`)
	if got != `include "mylib.inc"` {
		t.Errorf("got %q, want %q", got, `include "mylib.inc"`)
	}
}

// ============================================================================
// Zero-Operand Instruction Tests
// ============================================================================

func TestConvert_NOP(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    NOP")
	expectLines(t, got, []string{"    nop"})
}

func TestConvert_HALT(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    HALT")
	expectLines(t, got, []string{"    halt"})
}

func TestConvert_RTS(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    RTS")
	expectLines(t, got, []string{"    rts"})
}

func TestConvert_SEI_CLI_RTI(t *testing.T) {
	c := NewConverter()
	expectLines(t, c.ConvertLine("    SEI"), []string{"    sei"})
	expectLines(t, c.ConvertLine("    CLI"), []string{"    cli"})
	expectLines(t, c.ConvertLine("    RTI"), []string{"    rti"})
}

// ============================================================================
// Register-Specific Load Tests
// ============================================================================

func TestConvert_LDA_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA #42")
	expectLines(t, got, []string{"    move.l r1, #42"})
}

func TestConvert_LDA_Direct(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA @0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    load.l r1, (r17)",
	})
}

func TestConvert_LDA_BareAddr(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA 0x5000")
	expectLines(t, got, []string{"    move.l r1, #0x5000"})
}

func TestConvert_LDA_RegIndirect(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA [B]")
	expectLines(t, got, []string{"    load.l r1, (r5)"})
}

func TestConvert_LDA_RegOffset(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA [B+8]")
	expectLines(t, got, []string{"    load.l r1, 8(r5)"})
}

func TestConvert_LDX_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDX #10")
	expectLines(t, got, []string{"    move.l r2, #10"})
}

func TestConvert_LDA_Register(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA X")
	// LDA with a register operand - load register value into A
	expectLines(t, got, []string{"    move.l r1, r2"})
}

// ============================================================================
// Register-Specific Store Tests
// ============================================================================

func TestConvert_STA_Direct(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    STA @0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    store.l r1, (r17)",
	})
}

func TestConvert_STA_BareAddr(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    STA 0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    store.l r1, (r17)",
	})
}

func TestConvert_STA_RegIndirect(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    STA [B]")
	expectLines(t, got, []string{"    store.l r1, (r5)"})
}

func TestConvert_STB_Direct(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    STB @0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    store.l r5, (r17)",
	})
}

// ============================================================================
// Generic LOAD/STORE Tests
// ============================================================================

func TestConvert_LOAD_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LOAD A, #42")
	expectLines(t, got, []string{"    move.l r1, #42"})
}

func TestConvert_LOAD_BareNumber(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LOAD A, 0x5000")
	expectLines(t, got, []string{"    move.l r1, #0x5000"})
}

func TestConvert_LOAD_BareEquate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LOAD A, MY_CONST")
	expectLines(t, got, []string{"    move.l r1, #MY_CONST"})
}

func TestConvert_LOAD_Direct(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LOAD A, @0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    load.l r1, (r17)",
	})
}

func TestConvert_LOAD_Register(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LOAD A, X")
	expectLines(t, got, []string{"    move.l r1, r2"})
}

func TestConvert_LOAD_RegIndirect(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LOAD A, [B]")
	expectLines(t, got, []string{"    load.l r1, (r5)"})
}

func TestConvert_LOAD_RegOffset(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LOAD A, [B+16]")
	expectLines(t, got, []string{"    load.l r1, 16(r5)"})
}

func TestConvert_STORE_Direct(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    STORE A, @0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    store.l r1, (r17)",
	})
}

func TestConvert_STORE_BareAddr(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    STORE A, 0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    store.l r1, (r17)",
	})
}

func TestConvert_STORE_RegIndirect(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    STORE A, [B]")
	expectLines(t, got, []string{"    store.l r1, (r5)"})
}

func TestConvert_STORE_Expression(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    STORE A, RING_BASE + 1")
	expectLines(t, got, []string{
		"    la r17, RING_BASE + 1",
		"    store.l r1, (r17)",
	})
}

// ============================================================================
// ALU Instruction Tests
// ============================================================================

func TestConvert_ADD_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    ADD A, #10")
	expectLines(t, got, []string{"    add.l r1, r1, #10"})
}

func TestConvert_ADD_Register(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    ADD A, X")
	expectLines(t, got, []string{"    add.l r1, r1, r2"})
}

func TestConvert_ADD_BareEquate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    ADD A, MY_CONST")
	expectLines(t, got, []string{"    add.l r1, r1, #MY_CONST"})
}

func TestConvert_ADD_Direct(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    ADD A, @addr")
	expectLines(t, got, []string{
		"    la r17, addr",
		"    load.l r17, (r17)",
		"    add.l r1, r1, r17",
	})
}

func TestConvert_ADD_RegIndirect(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    ADD A, [B]")
	expectLines(t, got, []string{
		"    load.l r17, (r5)",
		"    add.l r1, r1, r17",
	})
}

func TestConvert_ADD_RegIndirectOffset(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    ADD A, [B+8]")
	expectLines(t, got, []string{
		"    load.l r17, 8(r5)",
		"    add.l r1, r1, r17",
	})
}

func TestConvert_SUB_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    SUB A, #5")
	expectLines(t, got, []string{"    sub.l r1, r1, #5"})
}

func TestConvert_MUL_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    MUL A, #3")
	expectLines(t, got, []string{"    mulu.l r1, r1, #3"})
}

func TestConvert_DIV_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    DIV A, #2")
	expectLines(t, got, []string{"    divu.l r1, r1, #2"})
}

func TestConvert_MOD_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    MOD A, #7")
	expectLines(t, got, []string{"    mod.l r1, r1, #7"})
}

func TestConvert_AND_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    AND A, #0xFF")
	expectLines(t, got, []string{"    and.l r1, r1, #0xFF"})
}

func TestConvert_OR_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    OR A, #0x80")
	expectLines(t, got, []string{"    or.l r1, r1, #0x80"})
}

func TestConvert_XOR_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    XOR A, #0xFF")
	expectLines(t, got, []string{"    eor.l r1, r1, #0xFF"})
}

func TestConvert_SHL_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    SHL A, #4")
	expectLines(t, got, []string{"    lsl.l r1, r1, #4"})
}

func TestConvert_SHR_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    SHR A, #4")
	expectLines(t, got, []string{"    lsr.l r1, r1, #4"})
}

func TestConvert_NOT(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    NOT A")
	expectLines(t, got, []string{"    not.l r1, r1"})
}

// ============================================================================
// INC/DEC Tests
// ============================================================================

func TestConvert_INC_Register(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    INC A")
	expectLines(t, got, []string{"    add.l r1, r1, #1"})
}

func TestConvert_DEC_Register(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    DEC A")
	expectLines(t, got, []string{"    sub.l r1, r1, #1"})
}

func TestConvert_INC_Direct(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    INC @0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    load.l r18, (r17)",
		"    add.l r18, r18, #1",
		"    store.l r18, (r17)",
	})
}

func TestConvert_DEC_Direct(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    DEC @0x5000")
	expectLines(t, got, []string{
		"    la r17, 0x5000",
		"    load.l r18, (r17)",
		"    sub.l r18, r18, #1",
		"    store.l r18, (r17)",
	})
}

// ============================================================================
// Jump/Branch Tests
// ============================================================================

func TestConvert_JMP_Label(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JMP main_loop")
	expectLines(t, got, []string{"    bra main_loop"})
}

func TestConvert_JMP_Equate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JMP MY_HANDLER")
	expectLines(t, got, []string{"    bra MY_HANDLER"})
}

func TestConvert_JSR_Label(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JSR subroutine")
	expectLines(t, got, []string{"    jsr subroutine"})
}

func TestConvert_JSR_Equate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JSR INIT_ROUTINE")
	expectLines(t, got, []string{"    jsr INIT_ROUTINE"})
}

func TestConvert_JNZ(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JNZ A, label")
	expectLines(t, got, []string{"    bnez r1, label"})
}

func TestConvert_JZ(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JZ A, label")
	expectLines(t, got, []string{"    beqz r1, label"})
}

func TestConvert_JGT(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JGT A, label")
	expectLines(t, got, []string{"    bgtz r1, label"})
}

func TestConvert_JGE(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JGE A, label")
	expectLines(t, got, []string{"    bgez r1, label"})
}

func TestConvert_JLT(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JLT A, label")
	expectLines(t, got, []string{"    bltz r1, label"})
}

func TestConvert_JLE(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    JLE A, label")
	expectLines(t, got, []string{"    blez r1, label"})
}

// ============================================================================
// Stack Tests
// ============================================================================

func TestConvert_PUSH(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    PUSH A")
	expectLines(t, got, []string{"    push r1"})
}

func TestConvert_POP(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    POP A")
	expectLines(t, got, []string{"    pop r1"})
}

// ============================================================================
// WAIT Tests
// ============================================================================

func TestConvert_WAIT_Immediate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    WAIT #100")
	expectLines(t, got, []string{"    wait #100"})
}

func TestConvert_WAIT_ImmEquate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    WAIT #DELAY_US")
	expectLines(t, got, []string{"    wait #DELAY_US"})
}

func TestConvert_WAIT_BareEquate(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    WAIT DELAY_US")
	expectLines(t, got, []string{"    wait #DELAY_US"})
}

func TestConvert_WAIT_BareHex(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    WAIT 0x1000")
	expectLines(t, got, []string{"    wait #0x1000"})
}

func TestConvert_WAIT_BareLabel(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    WAIT delay_val")
	expectLines(t, got, []string{"    wait #delay_val"})
}

func TestConvert_WAIT_Register_Error(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    WAIT A")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 lines for error, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "ERROR") {
		t.Errorf("expected ERROR in first line, got %q", got[0])
	}
}

func TestConvert_WAIT_Direct_Error(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    WAIT @0x5000")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 lines for error, got %d", len(got))
	}
	if !strings.Contains(got[0], "ERROR") {
		t.Errorf("expected ERROR in first line, got %q", got[0])
	}
}

func TestConvert_WAIT_RegInd_Error(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    WAIT [B]")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 lines for error, got %d", len(got))
	}
	if !strings.Contains(got[0], "ERROR") {
		t.Errorf("expected ERROR in first line, got %q", got[0])
	}
}

// ============================================================================
// Label and Comment Passthrough Tests
// ============================================================================

func TestConvert_LabelPassthrough(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("main_loop:")
	expectLines(t, got, []string{"main_loop:"})
}

func TestConvert_DotLabelPassthrough(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine(".wait_start:")
	expectLines(t, got, []string{".wait_start:"})
}

func TestConvert_LabelWithComment(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("main_loop: ; setup")
	if len(got) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "main_loop:") || !strings.Contains(got[0], "; setup") {
		t.Errorf("got %q, want label with comment", got[0])
	}
}

func TestConvert_CommentPreserved(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("; my comment")
	expectLines(t, got, []string{"; my comment"})
}

func TestConvert_InlineComment(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA #42 ; load")
	if len(got) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "move.l r1, #42") || !strings.Contains(got[0], "; load") {
		t.Errorf("got %q, want instruction with comment", got[0])
	}
}

func TestConvert_BlankLine(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("")
	expectLines(t, got, []string{""})
}

func TestConvert_Indentation(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA #42")
	expectLines(t, got, []string{"    move.l r1, #42"})
}

// ============================================================================
// File-Level Tests
// ============================================================================

func TestConvert_FileHeader(t *testing.T) {
	c := NewConverter()
	output := c.ConvertFile("NOP")
	if !strings.HasPrefix(output, "; Converted from IE32 by ie32to64") {
		t.Errorf("output should start with header, got: %q", output[:60])
	}
}

func TestConvert_FileHeader_Omitted(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	output := c.ConvertFile("NOP")
	if strings.HasPrefix(output, "; Converted") {
		t.Errorf("output should NOT start with header when noHeader=true")
	}
}

func TestConvert_EquateInOperand(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LDA #MY_CONST")
	expectLines(t, got, []string{"    move.l r1, #MY_CONST"})
}

func TestConvert_SizeFlag_Q(t *testing.T) {
	c := NewConverter()
	c.sizeSuffix = ".q"
	got := c.ConvertLine("    ADD A, #10")
	expectLines(t, got, []string{"    add.q r1, r1, #10"})
}

func TestConvert_UnknownMnemonic(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    MOVE X, A")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 lines for error, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "ERROR") {
		t.Errorf("expected ERROR in first line, got %q", got[0])
	}
	if !strings.Contains(got[0], "MOVE") {
		t.Errorf("error should mention 'MOVE', got %q", got[0])
	}
}

func TestConvert_Typo(t *testing.T) {
	c := NewConverter()
	got := c.ConvertLine("    LAOD A, #42")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 lines for error, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "ERROR") {
		t.Errorf("expected ERROR in first line, got %q", got[0])
	}
}

// ============================================================================
// Full File Conversion Tests
// ============================================================================

func TestConvertFile_CoprocCaller(t *testing.T) {
	c := NewConverter()
	output, err := c.ConvertFileFromPath("../../sdk/examples/asm/coproc_caller_ie32.asm")
	if err != nil {
		t.Fatalf("ConvertFileFromPath: %v", err)
	}

	// Spot-check key translations
	checks := []string{
		"include \"ie64.inc\"",          // .include ie32.inc → include ie64.inc
		"org 0x1000",                    // .org → org
		"move.l r1, #COPROC_CPU_IE32",   // LOAD A, #CONST → move.l r1, #CONST
		"la r17, COPROC_CPU_TYPE",       // STORE A, BARE → la r17, ...
		"store.l r1, (r17)",             // ... + store
		"move.l r1, #COPROC_CMD_STATUS", // LOAD A, BARE_EQUATE → move.l r1, #EQUATE
		"bnez r1, error",                // JNZ A, label → bnez r1, label
		"move.l r1, #10",                // LOAD A, #10
		"sub.l r1, r1, #COPROC_ST_OK",   // SUB A, #CONST
		"beqz r1, done",                 // JZ A, done
		"bra poll_loop",                 // JMP poll_loop → bra poll_loop
		"halt",                          // HALT → halt
		"move.l r2, #COPROC_TICKET",     // LOAD X, BARE → move.l r2, #...
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing expected pattern: %q", check)
		}
	}

	if c.errors > 0 {
		t.Errorf("expected 0 errors, got %d", c.errors)
	}
}

func TestConvertFile_Rotozoomer(t *testing.T) {
	c := NewConverter()
	output, err := c.ConvertFileFromPath("../../sdk/examples/asm/rotozoomer.asm")
	if err != nil {
		t.Fatalf("ConvertFileFromPath: %v", err)
	}

	// Spot-check key translations for the rotozoomer
	checks := []string{
		"include \"ie64.inc\"",
		"TEXTURE_BASE equ 0x500000", // .equ → NAME equ
		"TEX_TR equ 0x500200",
		"org 0x1000",
		"move.l r1, #1",      // LDA #1
		"la r17, VIDEO_CTRL", // STA @VIDEO_CTRL
		"store.l r1, (r17)",
		"jsr generate_texture",         // JSR
		"bra main_loop",                // JMP → bra
		"and.l r1, r1, #STATUS_VBLANK", // AND A, #STATUS_VBLANK
		"bnez r1, wait_end",
		"beqz r1, wait_start",
		"lsr.l r1, r1, #8", // SHR A, #8 → lsr.l
		"and.l r1, r1, #255",
		"lsl.l r1, r1, #2", // SHL A, #2 → lsl.l
		"add.l r1, r1, #sine_table",
		"load.l r1, (r1)",           // LDA [A] → load.l r1, (r1)
		"push r6",                   // PUSH C
		"pop r6",                    // POP C
		"eor.l r1, r1, #0xFFFFFFFF", // XOR A, #0xFFFFFFFF → eor.l
		"mulu.l r1, r1, r5",         // MUL A, B → mulu.l r1, r1, r5
		"dc.l 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92", // .word → dc.l
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing expected pattern: %q", check)
		}
	}

	if c.errors > 0 {
		t.Errorf("expected 0 errors, got %d", c.errors)
	}
}

func TestConvertFile_Rotozoomer_Golden(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	output, err := c.ConvertFileFromPath("../../sdk/examples/asm/rotozoomer.asm")
	if err != nil {
		t.Fatalf("ConvertFileFromPath: %v", err)
	}

	goldenPath := "testdata/rotozoomer_ie64_expected.asm"
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		// Generate golden file if it doesn't exist
		t.Logf("Golden file not found at %s - generating", goldenPath)
		if err := os.WriteFile(goldenPath, []byte(output), 0644); err != nil {
			t.Fatalf("Failed to write golden file: %v", err)
		}
		t.Skip("Generated golden file - re-run test to validate")
	}

	if output != string(golden) {
		// Find first difference for a helpful error
		outLines := strings.Split(output, "\n")
		goldLines := strings.Split(string(golden), "\n")
		for i := 0; i < len(outLines) && i < len(goldLines); i++ {
			if outLines[i] != goldLines[i] {
				t.Errorf("mismatch at line %d:\n  got:  %q\n  want: %q", i+1, outLines[i], goldLines[i])
				break
			}
		}
		if len(outLines) != len(goldLines) {
			t.Errorf("line count mismatch: got %d, want %d", len(outLines), len(goldLines))
		}
	}
}

// ============================================================================
// Helpers
// ============================================================================

func expectLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("expected %d lines, got %d:\n  got:  %v\n  want: %v", len(want), len(got), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("line %d:\n  got:  %q\n  want: %q", i, got[i], want[i])
		}
	}
}
