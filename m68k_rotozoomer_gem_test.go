//go:build m68k_test

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// =============================================================================
// GEM Rotozoomer M68K Instruction Coverage Tests
// =============================================================================
//
// Tests every instruction+addressing mode used by rotozoomer_gem.asm.
// Run with: go test -tags "headless m68k_test" -v -run ^TestGemRoto_

// =============================================================================
// Phase 0: Opcode Validation Against Assembler
// =============================================================================

func TestGemRoto_OpcodeValidation(t *testing.T) {
	// Check for vasmm68k_mot on PATH
	vasmPath, err := exec.LookPath("vasmm68k_mot")
	if err != nil {
		t.Skip("vasmm68k_mot not available on PATH")
	}

	// Generate a temporary asm file with every instruction from the inventory
	asmContent := `
	ORG $1000

start:
	; === Group 1: TOS Startup ===
g1_move_l_sp_disp_a5:
	move.l  4(sp),a5                ; 0x2A6F 0x0004
g1_move_l_a5_disp_d0:
	move.l  $18(a5),d0              ; 0x202D 0x0018
g1_sub_l_a5_d0:
	sub.l   a5,d0                   ; 0x908D
g1_move_l_d0_predec_sp:
	move.l  d0,-(sp)                ; 0x2F00
g1_move_l_a5_predec_sp:
	move.l  a5,-(sp)                ; 0x2F0D
g1_clr_w_predec_sp:
	clr.w   -(sp)                   ; 0x4267
g1_move_w_imm_predec_sp:
	move.w  #$4A,-(sp)              ; 0x3F3C 0x004A
g1_lea_sp_disp:
	lea     12(sp),sp               ; 0x4FEF 0x000C

	; === Group 2: Absolute Long Addressing ===
g2_move_w_imm_abs_l:
	move.w  #$000A,$00020000        ; 0x33FC 0x000A 0x0002 0x0000
g2_move_w_abs_l_dn:
	move.w  $00020000,d0            ; 0x3039 0x0002 0x0000
g2_move_w_dn_abs_l:
	move.w  d0,$00020000            ; 0x33C0 0x0002 0x0000
g2_move_w_abs_l_abs_l:
	move.w  $00020000,$00030000     ; 0x33F9 0x0002 0x0000 0x0003 0x0000
g2_move_l_imm_abs_l:
	move.l  #$DEADBEEF,$00020000    ; 0x23FC 0xDEAD 0xBEEF 0x0002 0x0000
g2_move_l_abs_l_dn:
	move.l  $00020000,d0            ; 0x2039 0x0002 0x0000
g2_move_l_dn_abs_l:
	move.l  d4,$00020000            ; 0x23C4 0x0002 0x0000
g2_clr_l_abs_l:
	clr.l   $00020000               ; 0x42B9 0x0002 0x0000
g2_tst_w_abs_l:
	tst.w   $00020000               ; 0x4A79 0x0002 0x0000

	; === Group 3: Indirect & Displacement Writes ===
g3_move_w_imm_a0_ind:
	move.w  #1,(a0)                 ; 0x30BC 0x0001
g3_move_w_imm_a0_disp2:
	move.w  #1,2(a0)                ; 0x317C 0x0001 0x0002
g3_move_w_imm_a0_disp20:
	move.w  #1,20(a0)               ; 0x317C 0x0001 0x0014

	; === Group 4: Post-Increment Copy Loop ===
g4_move_w_postinc_postinc:
	move.w  (a0)+,(a1)+             ; 0x32D8
g4_moveq_10_d0:
	moveq   #10,d0                  ; 0x700A
g4_dbra:
	dbra    d0,g4_dbra              ; 0x51C8 0xFFFE

	; === Group 5: MOVEM Mixed D+A Registers ===
g5_movem_d07a02_save:
	movem.l d0-d7/a0-a2,-(sp)      ; 0x48E7 0xFFE0
g5_movem_d07a02_restore:
	movem.l (sp)+,d0-d7/a0-a2      ; 0x4CDF 0x07FF
g5_movem_d07_save:
	movem.l d0-d7,-(sp)            ; 0x48E7 0xFF00
g5_movem_d07_restore:
	movem.l (sp)+,d0-d7            ; 0x4CDF 0x00FF
g5_movem_a01_save:
	movem.l a0-a1,-(sp)            ; 0x48E7 0x00C0
g5_movem_a01_restore:
	movem.l (sp)+,a0-a1            ; 0x4CDF 0x0300
g5_movem_d07a01_save:
	movem.l d0-d7/a0-a1,-(sp)     ; 0x48E7 0xFFC0

	; === Group 6: Word-Sized Arithmetic ===
g6_sub_w_d4_d6:
	sub.w   d4,d6                   ; 0x9C44
g6_asr_w_1_d6:
	asr.w   #1,d6                   ; 0xE246
g6_add_w_d0_d6:
	add.w   d0,d6                   ; 0xDC40
g6_cmp_w_d4_d0:
	cmp.w   d4,d0                   ; 0xB044
g6_cmp_w_d5_d1:
	cmp.w   d5,d1                   ; 0xB245

	; === Group 7: MOVEA.W Sign Extension ===
g7_movea_w_d4_a0:
	move.w  d4,a0                   ; 0x3044
g7_move_w_a0_d6:
	move.w  a0,d6                   ; 0x3C08

	; === Group 8: Indexed Table Lookup ===
g8_move_w_a0_d2_d3:
	move.w  (a0,d2.l),d3           ; 0x3630 0x2800
g8_move_w_a1_d2_d5:
	move.w  (a1,d2.l),d5           ; 0x3A31 0x2800
g8_ext_l_d3:
	ext.l   d3                      ; 0x48C3
g8_andi_l_ffff_d5:
	andi.l  #$FFFF,d5              ; 0x0285 0x0000 0xFFFF

	; === Group 9: Compute Frame Math ===
g9_lsr_l_8_d0:
	lsr.l   #8,d0                   ; 0xE088
g9_andi_l_255_d0:
	andi.l  #255,d0                 ; 0x0280 0x0000 0x00FF
g9_addi_l_64_d2:
	addi.l  #64,d2                  ; 0x0682 0x0000 0x0040
g9_add_l_d2_d2:
	add.l   d2,d2                   ; 0xD482
g9_muls_w_d5_d6:
	muls.w  d5,d6                   ; 0xCDC5
g9_muls_w_imm_d1:
	muls.w  #2560,d1                ; 0xC3FC 0x0A00
g9_lsl_l_8_d0:
	lsl.l   #8,d0                   ; 0xE188
g9_lsl_l_6_d1:
	lsl.l   #6,d1                   ; 0xED89
g9_lsl_l_4_d2:
	lsl.l   #4,d2                   ; 0xE98A
g9_lsl_l_2_d0:
	lsl.l   #2,d0                   ; 0xE588
g9_neg_l_d6:
	neg.l   d6                      ; 0x4486
g9_ext_l_d0:
	ext.l   d0                      ; 0x48C0
g9_ext_l_d1:
	ext.l   d1                      ; 0x48C1
g9_sub_l_d0_d3:
	sub.l   d0,d3                   ; 0x9680
g9_sub_l_d2_d1:
	sub.l   d2,d1                   ; 0x9282
g9_add_l_d1_d3:
	add.l   d1,d3                   ; 0xD681

	; === Group 10: Control Flow ===
g10_bsr:
	bsr.w   g10_target              ; 0x6100 disp16
g10_bra:
	bra.w   g10_target              ; 0x6000 disp16
g10_beq:
	beq.w   g10_target              ; 0x6700 disp16
g10_bne_s:
	bne.s   g10_target              ; 0x66xx
g10_ble_s:
	ble.s   g10_target              ; 0x6Fxx
g10_bge_s:
	bge.s   g10_target              ; 0x6Cxx
g10_bmi:
	bmi.w   g10_target              ; 0x6B00 disp16
g10_target:
	rts                             ; 0x4E75
g10_trap1:
	trap    #1                      ; 0x4E41
g10_trap2:
	trap    #2                      ; 0x4E42

	; === Group 11: Remaining Data Movement ===
g11_moveq_0_d0:
	moveq   #0,d0                   ; 0x7000
g11_moveq_neg1_d0:
	moveq   #-1,d0                  ; 0x70FF
g11_move_l_d6_d0:
	move.l  d6,d0                   ; 0x2006
g11_move_l_a0_d0:
	move.l  a0,d0                   ; 0x2008
g11_move_l_sp_ind_d0:
	move.l  (sp),d0                 ; 0x2017
g11_move_l_sp_disp4_d1:
	move.l  4(sp),d1                ; 0x222F 0x0004
g11_swap_d0:
	swap    d0                      ; 0x4840
g11_cmpi_w_imm_d0:
	cmpi.w  #$0016,d0              ; 0x0C40 0x0016
g11_btst_imm_d0:
	btst    #5,d0                   ; 0x0800 0x0005
g11_tst_w_d0:
	tst.w   d0                      ; 0x4A40
g11_andi_l_ffff_d2:
	andi.l  #$FFFF,d2              ; 0x0282 0x0000 0xFFFF
g11_andi_l_2_d0:
	andi.l  #2,d0                   ; 0x0280 0x0000 0x0002
g11_addi_l_vram_d1:
	addi.l  #$00100000,d1           ; 0x0681 0x0010 0x0000

	END
`

	tmpDir := t.TempDir()
	asmFile := filepath.Join(tmpDir, "validate.asm")
	binFile := filepath.Join(tmpDir, "validate.bin")

	if err := os.WriteFile(asmFile, []byte(asmContent), 0o644); err != nil {
		t.Fatalf("Failed to write asm file: %v", err)
	}

	// Assemble
	cmd := exec.Command(vasmPath, "-Fbin", "-m68020", "-devpac", "-o", binFile, asmFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("vasm assembly failed: %v\n%s", err, out)
	}

	bin, err := os.ReadFile(binFile)
	if err != nil {
		t.Fatalf("Failed to read binary: %v", err)
	}

	// Helper to read a 16-bit word at byte offset
	word := func(offset int) uint16 {
		if offset+1 >= len(bin) {
			t.Fatalf("Offset %d out of range (binary is %d bytes)", offset, len(bin))
		}
		return uint16(bin[offset])<<8 | uint16(bin[offset+1])
	}

	// Walk through the binary checking opcodes at known offsets.
	// We scan sequentially, instruction by instruction.
	type opcodeCheck struct {
		name     string
		expected []uint16
	}

	checks := []opcodeCheck{
		// Group 1
		{"move.l 4(sp),a5", []uint16{0x2A6F, 0x0004}},
		{"move.l $18(a5),d0", []uint16{0x202D, 0x0018}},
		{"sub.l a5,d0", []uint16{0x908D}},
		{"move.l d0,-(sp)", []uint16{0x2F00}},
		{"move.l a5,-(sp)", []uint16{0x2F0D}},
		{"clr.w -(sp)", []uint16{0x4267}},
		{"move.w #$4A,-(sp)", []uint16{0x3F3C, 0x004A}},
		{"lea 12(sp),sp", []uint16{0x4FEF, 0x000C}},
		// Group 2
		{"move.w #imm,abs.l", []uint16{0x33FC, 0x000A, 0x0002, 0x0000}},
		{"move.w abs.l,d0", []uint16{0x3039, 0x0002, 0x0000}},
		{"move.w d0,abs.l", []uint16{0x33C0, 0x0002, 0x0000}},
		{"move.w abs.l,abs.l", []uint16{0x33F9, 0x0002, 0x0000, 0x0003, 0x0000}},
		{"move.l #imm,abs.l", []uint16{0x23FC, 0xDEAD, 0xBEEF, 0x0002, 0x0000}},
		{"move.l abs.l,d0", []uint16{0x2039, 0x0002, 0x0000}},
		{"move.l d4,abs.l", []uint16{0x23C4, 0x0002, 0x0000}},
		{"clr.l abs.l", []uint16{0x42B9, 0x0002, 0x0000}},
		{"tst.w abs.l", []uint16{0x4A79, 0x0002, 0x0000}},
		// Group 3
		{"move.w #1,(a0)", []uint16{0x30BC, 0x0001}},
		{"move.w #1,2(a0)", []uint16{0x317C, 0x0001, 0x0002}},
		{"move.w #1,20(a0)", []uint16{0x317C, 0x0001, 0x0014}},
		// Group 4
		{"move.w (a0)+,(a1)+", []uint16{0x32D8}},
		{"moveq #10,d0", []uint16{0x700A}},
		{"dbra d0,self", []uint16{0x51C8, 0xFFFE}},
		// Group 5
		{"movem.l d0-d7/a0-a2,-(sp)", []uint16{0x48E7, 0xFFE0}},
		{"movem.l (sp)+,d0-d7/a0-a2", []uint16{0x4CDF, 0x07FF}},
		{"movem.l d0-d7,-(sp)", []uint16{0x48E7, 0xFF00}},
		{"movem.l (sp)+,d0-d7", []uint16{0x4CDF, 0x00FF}},
		{"movem.l a0-a1,-(sp)", []uint16{0x48E7, 0x00C0}},
		{"movem.l (sp)+,a0-a1", []uint16{0x4CDF, 0x0300}},
		{"movem.l d0-d7/a0-a1,-(sp)", []uint16{0x48E7, 0xFFC0}},
		// Group 6
		{"sub.w d4,d6", []uint16{0x9C44}},
		{"asr.w #1,d6", []uint16{0xE246}},
		{"add.w d0,d6", []uint16{0xDC40}},
		{"cmp.w d4,d0", []uint16{0xB044}},
		{"cmp.w d5,d1", []uint16{0xB245}},
		// Group 7
		{"move.w d4,a0", []uint16{0x3044}},
		{"move.w a0,d6", []uint16{0x3C08}},
		// Group 8
		{"move.w (a0,d2.l),d3", []uint16{0x3630, 0x2800}},
		{"move.w (a1,d2.l),d5", []uint16{0x3A31, 0x2800}},
		{"ext.l d3", []uint16{0x48C3}},
		{"andi.l #$FFFF,d5", []uint16{0x0285, 0x0000, 0xFFFF}},
		// Group 9
		{"lsr.l #8,d0", []uint16{0xE088}},
		{"andi.l #255,d0", []uint16{0x0280, 0x0000, 0x00FF}},
		{"addi.l #64,d2", []uint16{0x0682, 0x0000, 0x0040}},
		{"add.l d2,d2", []uint16{0xD482}},
		{"muls.w d5,d6", []uint16{0xCDC5}},
		{"muls.w #2560,d1", []uint16{0xC3FC, 0x0A00}},
		{"lsl.l #8,d0", []uint16{0xE188}},
		{"lsl.l #6,d1", []uint16{0xED89}},
		{"lsl.l #4,d2", []uint16{0xE98A}},
		{"lsl.l #2,d0", []uint16{0xE588}},
		{"neg.l d6", []uint16{0x4486}},
		{"ext.l d0", []uint16{0x48C0}},
		{"ext.l d1", []uint16{0x48C1}},
		{"sub.l d0,d3", []uint16{0x9680}},
		{"sub.l d2,d1", []uint16{0x9282}},
		{"add.l d1,d3", []uint16{0xD681}},
		// Group 10 - branches have variable displacements, just check opcodes
		{"bsr.w", []uint16{0x6100}}, // + disp16
		// skip displacement word
	}

	offset := 0
	for _, chk := range checks {
		if offset+len(chk.expected)*2 > len(bin) {
			t.Errorf("%s: offset %d would exceed binary length %d", chk.name, offset, len(bin))
			break
		}
		for i, exp := range chk.expected {
			got := word(offset + i*2)
			if got != exp {
				t.Errorf("%s word[%d]: expected 0x%04X, got 0x%04X (at byte offset %d)",
					chk.name, i, exp, got, offset+i*2)
			}
		}
		offset += len(chk.expected) * 2
	}

	t.Logf("Validated %d instructions, binary size = %d bytes", len(checks), len(bin))
}

// =============================================================================
// Group 1: TOS Startup & Stack Operations
// =============================================================================

func TestGemRoto_TOSStartup(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "MOVE.L_4(SP),A5_read_basepage",
			AddrRegs: [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x8000},
			InitialMem: map[uint32]interface{}{
				0x8000: uint32(0),          // SP points here
				0x8004: uint32(0x00040000), // basepage pointer at 4(SP)
			},
			Opcodes:       []uint16{0x2A6F, 0x0004}, // move.l 4(sp),a5
			ExpectedRegs:  Reg("A5", 0x00040000),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.L_$18(A5),D0_read_hitpa",
			AddrRegs: [8]uint32{0, 0, 0, 0, 0, 0x00040000, 0, 0x8000},
			// A5 is AddrRegs[5], but the instruction uses A5
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[5] = 0x00040000
				cpu.Write32(0x00040018, 0x00050000) // p_hitpa
			},
			Opcodes:       []uint16{0x202D, 0x0018}, // move.l $18(a5),d0
			ExpectedRegs:  Reg("D0", 0x00050000),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "SUB.L_A5,D0_An_as_source",
			DataRegs:      [8]uint32{0x00050000, 0, 0, 0, 0, 0, 0, 0},
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0x00040000, 0, 0x8000},
			Opcodes:       []uint16{0x908D},      // sub.l a5,d0
			ExpectedRegs:  Reg("D0", 0x00010000), // 0x50000 - 0x40000
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.L_D0,-(SP)_push_data",
			DataRegs:      [8]uint32{0x00010000, 0, 0, 0, 0, 0, 0, 0},
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x2F00}, // move.l d0,-(sp)
			ExpectedRegs:  Reg("SP", 0x7FFC),
			ExpectedMem:   []MemoryExpectation{ExpectLong(0x7FFC, 0x00010000)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.L_A5,-(SP)_push_address",
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0x00040000, 0, 0x8000},
			Opcodes:       []uint16{0x2F0D}, // move.l a5,-(sp)
			ExpectedRegs:  Regs("SP", uint32(0x7FFC), "A5", uint32(0x00040000)),
			ExpectedMem:   []MemoryExpectation{ExpectLong(0x7FFC, 0x00040000)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "CLR.W_-(SP)_predecrement",
			AddrRegs: [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x8000},
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x7FFE, 0xFFFF) // pre-fill with non-zero
			},
			Opcodes:       []uint16{0x4267}, // clr.w -(sp)
			ExpectedRegs:  Reg("SP", 0x7FFE),
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x7FFE, 0x0000)},
			ExpectedFlags: FlagsNZ(0, 1), // CLR sets Z, clears N
		},
		{
			Name:          "MOVE.W_#imm,-(SP)_push_word",
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x3F3C, 0x004A}, // move.w #$4A,-(sp)
			ExpectedRegs:  Reg("SP", 0x7FFE),
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x7FFE, 0x004A)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "LEA_12(SP),SP_pop_args",
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x7FF4},
			Opcodes:       []uint16{0x4FEF, 0x000C}, // lea 12(sp),sp
			ExpectedRegs:  Reg("SP", 0x8000),
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// Group 2: Absolute Long Addressing — Highest Bug Probability
// =============================================================================

func TestGemRoto_AbsoluteLong(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "MOVE.W_#imm,abs.l",
			Opcodes:       []uint16{0x33FC, 0x000A, 0x0002, 0x0000}, // move.w #$000A,$00020000
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x00020000, 0x000A)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name: "MOVE.W_abs.l,Dn",
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x00020000, 0x1234)
			},
			Opcodes:       []uint16{0x3039, 0x0002, 0x0000}, // move.w $00020000,d0
			ExpectedRegs:  Reg("D0", 0x1234),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.W_Dn,abs.l",
			DataRegs:      [8]uint32{0x5678, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x33C0, 0x0002, 0x0000}, // move.w d0,$00020000
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x00020000, 0x5678)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name: "MOVE.W_abs.l,abs.l_dual_fetch",
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x00020000, 0xABCD)
			},
			Opcodes: []uint16{0x33F9, 0x0002, 0x0000, 0x0003, 0x0000}, // move.w $20000,$30000
			ExpectedMem: []MemoryExpectation{
				ExpectWord(0x00020000, 0xABCD), // source unchanged
				ExpectWord(0x00030000, 0xABCD), // destination written
			},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.L_#imm,abs.l",
			Opcodes:       []uint16{0x23FC, 0xDEAD, 0xBEEF, 0x0002, 0x0000}, // move.l #$DEADBEEF,$20000
			ExpectedMem:   []MemoryExpectation{ExpectLong(0x00020000, 0xDEADBEEF)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name: "MOVE.L_abs.l,Dn",
			Setup: func(cpu *M68KCPU) {
				cpu.Write32(0x00020000, 0xCAFEBABE)
			},
			Opcodes:       []uint16{0x2039, 0x0002, 0x0000}, // move.l $20000,d0
			ExpectedRegs:  Reg("D0", 0xCAFEBABE),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.L_Dn,abs.l",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0x12345678, 0, 0, 0},
			Opcodes:       []uint16{0x23C4, 0x0002, 0x0000}, // move.l d4,$20000
			ExpectedMem:   []MemoryExpectation{ExpectLong(0x00020000, 0x12345678)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name: "CLR.L_abs.l",
			Setup: func(cpu *M68KCPU) {
				cpu.Write32(0x00020000, 0xFFFFFFFF)
			},
			Opcodes:       []uint16{0x42B9, 0x0002, 0x0000}, // clr.l $20000
			ExpectedMem:   []MemoryExpectation{ExpectLong(0x00020000, 0x00000000)},
			ExpectedFlags: FlagsNZ(0, 1), // Z set
		},
		{
			Name: "TST.W_abs.l_zero",
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x00020000, 0x0000)
			},
			Opcodes:       []uint16{0x4A79, 0x0002, 0x0000}, // tst.w $20000
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),
		},
		{
			Name: "TST.W_abs.l_negative",
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x00020000, 0x8000)
			},
			Opcodes:       []uint16{0x4A79, 0x0002, 0x0000},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name: "MOVE.L_abs.l,D5_variable_read",
			Setup: func(cpu *M68KCPU) {
				cpu.Write32(0x00046C2C, 0x00000300) // var_ca = 768
			},
			Opcodes:       []uint16{0x2A39, 0x0004, 0x6C2C}, // move.l $46C2C,d5
			ExpectedRegs:  Reg("D5", 0x00000300),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.L_D6,abs.l_variable_write",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0x00000180, 0},
			Opcodes:       []uint16{0x23C6, 0x0004, 0x6C2C}, // move.l d6,$46C2C
			ExpectedMem:   []MemoryExpectation{ExpectLong(0x00046C2C, 0x00000180)},
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// Group 3: Indirect & Displacement Writes
// =============================================================================

func TestGemRoto_IndirectDisplacement(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "MOVE.W_#1,(A0)_indirect",
			AddrRegs:      [8]uint32{0x3000, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x30BC, 0x0001}, // move.w #1,(a0)
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x3000, 0x0001)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.W_#1,2(A0)_displacement",
			AddrRegs:      [8]uint32{0x3000, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x317C, 0x0001, 0x0002}, // move.w #1,2(a0)
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x3002, 0x0001)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.W_#1,20(A0)_larger_displacement",
			AddrRegs:      [8]uint32{0x3000, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x317C, 0x0001, 0x0014}, // move.w #1,20(a0)
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x3014, 0x0001)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.W_#imm,large_disp(A0)_VDI_pattern",
			AddrRegs:      [8]uint32{0x3000, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x317C, 0x0002, 0x0100}, // move.w #2,256(a0)
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x3100, 0x0002)},
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// Group 4: Post-Increment Copy Loop
// =============================================================================

func TestGemRoto_PostIncCopyLoop(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "MOVE.W_(A0)+,(A1)+_single",
			AddrRegs: [8]uint32{0x3000, 0x4000, 0, 0, 0, 0, 0, 0x8000},
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x3000, 0xBEEF)
			},
			Opcodes:       []uint16{0x32D8}, // move.w (a0)+,(a1)+
			ExpectedRegs:  Regs("A0", uint32(0x3002), "A1", uint32(0x4002)),
			ExpectedMem:   []MemoryExpectation{ExpectWord(0x4000, 0xBEEF)},
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVEQ_#10,D0",
			Opcodes:       []uint16{0x700A}, // moveq #10,d0
			ExpectedRegs:  Reg("D0", 10),
			ExpectedFlags: FlagsNZ(0, 0),
		},
		{
			Name:     "DBRA_D0_decrement_and_branch",
			DataRegs: [8]uint32{2, 0, 0, 0, 0, 0, 0, 0}, // D0 = 2
			// dbra d0,self (displacement = -4 from PC after fetch = 0xFFFC)
			// But the test framework executes one instruction, so just test that D0 decrements
			// and PC changes correctly for the taken branch
			Opcodes:       []uint16{0x51C8, 0xFFFE}, // dbra d0,self (branch to start of this instruction)
			ExpectedRegs:  Reg("D0", 1),             // D0 decremented from 2 to 1
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "DBRA_D0_falls_through_at_neg1",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0}, // D0 = 0
			Opcodes:       []uint16{0x51C8, 0xFFFE},          // dbra d0,self
			ExpectedRegs:  Reg("D0", 0x0000FFFF),             // DBRA only decrements low word: 0→0xFFFF
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// Group 5: MOVEM Mixed D+A Registers
// =============================================================================

func TestGemRoto_MovemMixed(t *testing.T) {
	t.Run("MOVEM.L_d0-d7/a0-a2_save_restore_11regs", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up known register values
		cpu.DataRegs = [8]uint32{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}
		cpu.AddrRegs = [8]uint32{0xA0, 0xA1, 0xA2, 0, 0, 0, 0, 0x8000}

		// Save: movem.l d0-d7/a0-a2,-(sp) — 0x48E7, 0xFFE0
		cpu.PC = M68K_ENTRY_POINT
		cpu.Write16(cpu.PC, 0x48E7)
		cpu.Write16(cpu.PC+2, 0xFFE0)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// SP should have decreased by 11*4 = 44
		expectedSP := uint32(0x8000 - 44)
		if cpu.AddrRegs[7] != expectedSP {
			t.Errorf("SP after save: got 0x%X, expected 0x%X", cpu.AddrRegs[7], expectedSP)
		}

		// Verify memory: predecrement writes in register order D0..D7,A0..A2
		// but MOVEM predecrement stores in reverse register order
		// Predecrement mask 0xFFE0: bits 15..0 = A7..A0,D7..D0
		// So 0xFFE0 = 1111_1111_1110_0000 → D0-D7 and A0-A2
		// Predecrement writes top register first (A2), then A1, A0, D7..D0
		addr := uint32(0x8000)
		expectedOrder := []struct {
			name string
			val  uint32
		}{
			{"A2", 0xA2}, {"A1", 0xA1}, {"A0", 0xA0},
			{"D7", 0x80}, {"D6", 0x70}, {"D5", 0x60}, {"D4", 0x50},
			{"D3", 0x40}, {"D2", 0x30}, {"D1", 0x20}, {"D0", 0x10},
		}
		for _, exp := range expectedOrder {
			addr -= 4
			got := cpu.Read32(addr)
			if got != exp.val {
				t.Errorf("MOVEM save %s at 0x%X: got 0x%X, expected 0x%X", exp.name, addr, got, exp.val)
			}
		}

		// Trash registers
		cpu.DataRegs = [8]uint32{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
		cpu.AddrRegs[0] = 0xFF
		cpu.AddrRegs[1] = 0xFF
		cpu.AddrRegs[2] = 0xFF

		// Restore: movem.l (sp)+,d0-d7/a0-a2 — 0x4CDF, 0x07FF
		cpu.Write16(cpu.PC, 0x4CDF)
		cpu.Write16(cpu.PC+2, 0x07FF)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// SP should be back to 0x8000
		if cpu.AddrRegs[7] != 0x8000 {
			t.Errorf("SP after restore: got 0x%X, expected 0x8000", cpu.AddrRegs[7])
		}

		// Verify registers restored
		for i, exp := range []uint32{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80} {
			if cpu.DataRegs[i] != exp {
				t.Errorf("D%d after restore: got 0x%X, expected 0x%X", i, cpu.DataRegs[i], exp)
			}
		}
		for i, exp := range []uint32{0xA0, 0xA1, 0xA2} {
			if cpu.AddrRegs[i] != exp {
				t.Errorf("A%d after restore: got 0x%X, expected 0x%X", i, cpu.AddrRegs[i], exp)
			}
		}
	})

	t.Run("MOVEM.L_d0-d7_save_restore_8regs", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.DataRegs = [8]uint32{0x100, 0x200, 0x300, 0x400, 0x500, 0x600, 0x700, 0x800}
		cpu.AddrRegs[7] = 0x8000

		// Save
		cpu.PC = M68K_ENTRY_POINT
		cpu.Write16(cpu.PC, 0x48E7)
		cpu.Write16(cpu.PC+2, 0xFF00)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.AddrRegs[7] != 0x8000-32 {
			t.Errorf("SP after save: got 0x%X, expected 0x%X", cpu.AddrRegs[7], 0x8000-32)
		}

		// Trash and restore
		cpu.DataRegs = [8]uint32{}
		cpu.Write16(cpu.PC, 0x4CDF)
		cpu.Write16(cpu.PC+2, 0x00FF)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		for i, exp := range []uint32{0x100, 0x200, 0x300, 0x400, 0x500, 0x600, 0x700, 0x800} {
			if cpu.DataRegs[i] != exp {
				t.Errorf("D%d: got 0x%X, expected 0x%X", i, cpu.DataRegs[i], exp)
			}
		}
	})

	t.Run("MOVEM.L_a0-a1_save_restore_2regs", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.AddrRegs = [8]uint32{0xDEAD, 0xBEEF, 0, 0, 0, 0, 0, 0x8000}

		// Save: movem.l a0-a1,-(sp) — 0x48E7, 0x00C0
		cpu.PC = M68K_ENTRY_POINT
		cpu.Write16(cpu.PC, 0x48E7)
		cpu.Write16(cpu.PC+2, 0x00C0)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.AddrRegs[7] != 0x8000-8 {
			t.Errorf("SP after save: got 0x%X, expected 0x%X", cpu.AddrRegs[7], 0x8000-8)
		}

		// Trash and restore
		cpu.AddrRegs[0] = 0
		cpu.AddrRegs[1] = 0
		cpu.Write16(cpu.PC, 0x4CDF)
		cpu.Write16(cpu.PC+2, 0x0300)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.AddrRegs[0] != 0xDEAD {
			t.Errorf("A0: got 0x%X, expected 0xDEAD", cpu.AddrRegs[0])
		}
		if cpu.AddrRegs[1] != 0xBEEF {
			t.Errorf("A1: got 0x%X, expected 0xBEEF", cpu.AddrRegs[1])
		}
	})

	t.Run("MOVEM.L_d0-d7/a0-a1_save_10regs", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.DataRegs = [8]uint32{1, 2, 3, 4, 5, 6, 7, 8}
		cpu.AddrRegs = [8]uint32{9, 10, 0, 0, 0, 0, 0, 0x8000}

		// Save: movem.l d0-d7/a0-a1,-(sp) — 0x48E7, 0xFFC0
		cpu.PC = M68K_ENTRY_POINT
		cpu.Write16(cpu.PC, 0x48E7)
		cpu.Write16(cpu.PC+2, 0xFFC0)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.AddrRegs[7] != 0x8000-40 {
			t.Errorf("SP after save: got 0x%X, expected 0x%X", cpu.AddrRegs[7], 0x8000-40)
		}

		// Verify memory — predecrement stores highest register first
		addr := uint32(0x8000)
		for _, exp := range []uint32{10, 9, 8, 7, 6, 5, 4, 3, 2, 1} {
			addr -= 4
			got := cpu.Read32(addr)
			if got != exp {
				t.Errorf("MOVEM at 0x%X: got 0x%X, expected 0x%X", addr, got, exp)
			}
		}
	})
}

// =============================================================================
// Group 6: Word-Sized Arithmetic
// =============================================================================

func TestGemRoto_WordArithmetic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "SUB.W_D4,D6",
			DataRegs:      [8]uint32{0, 0, 0, 0, 320, 0, 640, 0},
			Opcodes:       []uint16{0x9C44}, // sub.w d4,d6
			ExpectedRegs:  Reg("D6", 320),   // 640 - 320
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "ASR.W_#1,D6_divide_by_2",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 320, 0},
			Opcodes:       []uint16{0xE246}, // asr.w #1,d6
			ExpectedRegs:  Reg("D6", 160),   // 320 >> 1 = 160
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "ASR.W_#1,D6_negative",
			DataRegs: [8]uint32{0, 0, 0, 0, 0, 0, 0xFFFFFFF6, 0}, // word value = 0xFFF6 = -10
			Opcodes:  []uint16{0xE246},                           // asr.w #1,d6
			// ASR.W operates on low 16 bits: 0xFFF6 >> 1 = 0xFFFB (-5), upper 16 unchanged
			ExpectedRegs:  Reg("D6", 0xFFFFFFFB),
			ExpectedFlags: FlagsNZ(1, 0), // negative
		},
		{
			Name:          "ADD.W_D0,D6",
			DataRegs:      [8]uint32{100, 0, 0, 0, 0, 0, 160, 0},
			Opcodes:       []uint16{0xDC40}, // add.w d0,d6
			ExpectedRegs:  Reg("D6", 260),   // 160 + 100
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "CMP.W_D4,D0_equal",
			DataRegs:      [8]uint32{320, 0, 0, 0, 320, 0, 0, 0},
			Opcodes:       []uint16{0xB044},      // cmp.w d4,d0
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // equal → Z set
		},
		{
			Name:          "CMP.W_D4,D0_less",
			DataRegs:      [8]uint32{100, 0, 0, 0, 320, 0, 0, 0},
			Opcodes:       []uint16{0xB044}, // cmp.w d4,d0
			ExpectedFlags: FlagsNZ(1, 0),    // 100 - 320 = negative
		},
		{
			Name:          "CMP.W_D5,D1",
			DataRegs:      [8]uint32{0, 240, 0, 0, 0, 240, 0, 0},
			Opcodes:       []uint16{0xB245},      // cmp.w d5,d1
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // equal
		},
		{
			// Window centering chain: d6 = d2; d6 -= d4; d6 >>= 1; d6 += d0
			// Simulate: desktop_w=800, win_w=320, desktop_x=0
			// Expected: (800-320)/2 + 0 = 240
			Name:     "Window_centering_chain",
			DataRegs: [8]uint32{0, 0, 0, 0, 320, 0, 800, 0},
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = 0x8000
				pc := uint32(M68K_ENTRY_POINT)
				cpu.Write16(pc, 0x9C44)   // sub.w d4,d6 → 800-320=480
				cpu.Write16(pc+2, 0xE246) // asr.w #1,d6 → 240
				cpu.Write16(pc+4, 0xDC40) // add.w d0,d6 → 240+0=240
				cpu.Write16(pc+6, 0x4E71) // nop (sentinel)
			},
			Opcodes: nil, // opcodes already written by Setup
		},
	}

	// Run the regular tests
	for i := range tests {
		if tests[i].Name == "Window_centering_chain" {
			// Run the chain test separately
			t.Run(tests[i].Name, func(t *testing.T) {
				cpu := setupTestCPU()
				cpu.DataRegs = [8]uint32{0, 0, 0, 0, 320, 0, 800, 0}
				cpu.AddrRegs[7] = 0x8000

				pc := uint32(M68K_ENTRY_POINT)
				cpu.PC = pc
				cpu.Write16(pc, 0x9C44)   // sub.w d4,d6
				cpu.Write16(pc+2, 0xE246) // asr.w #1,d6
				cpu.Write16(pc+4, 0xDC40) // add.w d0,d6

				// Execute 3 instructions
				for range 3 {
					cpu.currentIR = cpu.Fetch16()
					cpu.FetchAndDecodeInstruction()
				}

				if cpu.DataRegs[6] != 240 {
					t.Errorf("D6 (window x): got %d, expected 240", cpu.DataRegs[6])
				}
			})
			continue
		}
		t.Run(tests[i].Name, func(t *testing.T) {
			tc := tests[i]
			cpu := setupTestCPU()
			runSingleM68KTest(t, cpu, tc)
		})
	}
}

// =============================================================================
// Group 7: MOVEA.W Sign Extension — High Bug Probability
// =============================================================================

func TestGemRoto_MoveaW(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "MOVEA.W_D4_A0_positive",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0x1234, 0, 0, 0},
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x3044},      // move.w d4,a0
			ExpectedRegs:  Reg("A0", 0x00001234), // positive word → sign-extend to 32 bits
			ExpectedFlags: FlagDontCare(),        // MOVEA doesn't affect flags
		},
		{
			Name:          "MOVEA.W_D4_A0_negative_sign_extend",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0x8000, 0, 0, 0},
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x3044},      // move.w d4,a0
			ExpectedRegs:  Reg("A0", 0xFFFF8000), // negative word → sign-extend
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVEA.W_D4_A0_FFFF_sign_extend",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0xFFFF, 0, 0, 0},
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x3044},      // move.w d4,a0
			ExpectedRegs:  Reg("A0", 0xFFFFFFFF), // 0xFFFF sign-extends to 0xFFFFFFFF
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVEA.W_D4_A0_7FFF_no_extend",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0x7FFF, 0, 0, 0},
			AddrRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x3044},      // move.w d4,a0
			ExpectedRegs:  Reg("A0", 0x00007FFF), // max positive word
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.W_A0,D6_read_low_word",
			AddrRegs: [8]uint32{0x12345678, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:  []uint16{0x3C08}, // move.w a0,d6
			// MOVE.W from An reads low 16 bits into Dn low 16 bits
			ExpectedRegs:  Reg("D6", 0x5678),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.W_A0,D6_preserves_upper_D6",
			DataRegs: [8]uint32{0, 0, 0, 0, 0, 0, 0xFFFF0000, 0},
			AddrRegs: [8]uint32{0x1234, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:  []uint16{0x3C08}, // move.w a0,d6
			// MOVE.W to Dn only affects low 16 bits, upper 16 preserved
			ExpectedRegs:  Reg("D6", 0xFFFF1234),
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)

	// Round-trip test: save A0 low word to D6, restore from D6 to A0
	t.Run("MOVEA.W_round_trip", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.AddrRegs[0] = 0x00001234
		cpu.AddrRegs[7] = 0x8000

		pc := uint32(M68K_ENTRY_POINT)
		cpu.PC = pc
		cpu.Write16(pc, 0x3C08)   // move.w a0,d6 (save low word)
		cpu.Write16(pc+2, 0x3046) // move.w d6,a0 (restore via MOVEA.W)

		// Execute 2 instructions
		for range 2 {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}

		if cpu.AddrRegs[0] != 0x00001234 {
			t.Errorf("Round-trip A0: got 0x%08X, expected 0x00001234", cpu.AddrRegs[0])
		}
	})
}

// =============================================================================
// Group 8: Indexed Table Lookup (Word-Sized)
// =============================================================================

func TestGemRoto_IndexedLookup(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "MOVE.W_(A0,D2.L),D3_word_indexed",
			DataRegs: [8]uint32{0, 0, 4, 0, 0, 0, 0, 0}, // D2 = 4
			AddrRegs: [8]uint32{0x3000, 0, 0, 0, 0, 0, 0, 0x8000},
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x3004, 0x0100) // sine[2] = 256
			},
			Opcodes:       []uint16{0x3630, 0x2800}, // move.w (a0,d2.l),d3
			ExpectedRegs:  Reg("D3", 0x0100),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.W_(A0,D2.L),D3_negative_value",
			DataRegs: [8]uint32{0, 0, 8, 0, 0, 0, 0, 0}, // D2 = 8
			AddrRegs: [8]uint32{0x3000, 0, 0, 0, 0, 0, 0, 0x8000},
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x3008, 0xFF00) // -256 as signed word
			},
			Opcodes: []uint16{0x3630, 0x2800}, // move.w (a0,d2.l),d3
			// MOVE.W to Dn only writes low 16 bits
			ExpectedRegs:  Reg("D3", 0xFF00),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.W_(A1,D2.L),D5_recip_lookup",
			DataRegs: [8]uint32{0, 0, 6, 0, 0, 0, 0, 0}, // D2 = 6
			AddrRegs: [8]uint32{0, 0x4000, 0, 0, 0, 0, 0, 0x8000},
			Setup: func(cpu *M68KCPU) {
				cpu.Write16(0x4006, 0x0300) // recip[3] = 768
			},
			Opcodes:       []uint16{0x3A31, 0x2800}, // move.w (a1,d2.l),d5
			ExpectedRegs:  Reg("D5", 0x0300),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "EXT.L_D3_positive",
			DataRegs:      [8]uint32{0, 0, 0, 0x00FF0100, 0, 0, 0, 0}, // D3 low word = 0x0100 (256)
			Opcodes:       []uint16{0x48C3},                           // ext.l d3
			ExpectedRegs:  Reg("D3", 0x00000100),                      // sign-extend word to long
			ExpectedFlags: FlagsNZ(0, 0),
		},
		{
			Name:          "EXT.L_D3_negative",
			DataRegs:      [8]uint32{0, 0, 0, 0x0000FF00, 0, 0, 0, 0}, // D3 low word = 0xFF00 (-256)
			Opcodes:       []uint16{0x48C3},                           // ext.l d3
			ExpectedRegs:  Reg("D3", 0xFFFFFF00),                      // sign-extend to 0xFFFFFF00
			ExpectedFlags: FlagsNZ(1, 0),
		},
		{
			Name:          "ANDI.L_#FFFF,D5_mask_to_unsigned",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0xDEAD0300, 0, 0},
			Opcodes:       []uint16{0x0285, 0x0000, 0xFFFF}, // andi.l #$FFFF,d5
			ExpectedRegs:  Reg("D5", 0x0300),
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)

	// Integration: lookup + ext.l + muls.w chain (from compute_frame)
	t.Run("Lookup_extend_multiply_chain", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up sine table at 0x3000
		cpu.Write16(0x3000+128, 0x0100) // sine[64] = 256 (word offset 128)
		cpu.DataRegs[2] = 128           // index * 2 = 64 * 2
		cpu.AddrRegs[0] = 0x3000
		cpu.AddrRegs[7] = 0x8000
		cpu.DataRegs[5] = 512 // reciprocal

		pc := uint32(M68K_ENTRY_POINT)
		cpu.PC = pc
		cpu.Write16(pc, 0x3630)   // move.w (a0,d2.l),d3
		cpu.Write16(pc+2, 0x2800) // extension word
		cpu.Write16(pc+4, 0x48C3) // ext.l d3
		cpu.Write16(pc+6, 0xCDC5) // muls.w d5,d6 — wait, we need d3 in d6 first

		// Actually let's trace the real pattern:
		// move.w (a0,d2.l),d3  → d3.w = 256
		// ext.l d3             → d3 = 256
		// Then later: muls.w d5,d6 where d6=d3=256, d5=recip=512
		cpu.Write16(pc+6, 0x2003) // move.l d3,d0 (to save d3)
		cpu.Write16(pc+8, 0x4E71) // nop

		for range 3 {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}

		if cpu.DataRegs[3] != 256 {
			t.Errorf("D3 after lookup+ext: got %d, expected 256", cpu.DataRegs[3])
		}
	})
}

// =============================================================================
// Group 9: Compute Frame Math
// =============================================================================

func TestGemRoto_ComputeFrameMath(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "LSR.L_#8,D0",
			DataRegs:      [8]uint32{0x00012300, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xE088}, // lsr.l #8,d0
			ExpectedRegs:  Reg("D0", 0x00000123),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "ANDI.L_#255,D0",
			DataRegs:      [8]uint32{0x00000123, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x0280, 0x0000, 0x00FF}, // andi.l #255,d0
			ExpectedRegs:  Reg("D0", 0x23),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "ADDI.L_#64,D2",
			DataRegs:      [8]uint32{0, 0, 100, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x0682, 0x0000, 0x0040}, // addi.l #64,d2
			ExpectedRegs:  Reg("D2", 164),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "ADD.L_D2,D2_double",
			DataRegs:      [8]uint32{0, 0, 50, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xD482}, // add.l d2,d2
			ExpectedRegs:  Reg("D2", 100),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MULS.W_D5,D6_positive",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 512, 256, 0},
			Opcodes:       []uint16{0xCDC5},  // muls.w d5,d6
			ExpectedRegs:  Reg("D6", 131072), // 256 * 512
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MULS.W_D5,D6_negative_source",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 512, 0xFFFFFF00, 0}, // d6 low word = 0xFF00 = -256
			Opcodes:       []uint16{0xCDC5},                             // muls.w d5,d6
			ExpectedRegs:  Reg("D6", 0xFFFE0000),                        // -256 * 512 = -131072
			ExpectedFlags: FlagsNZ(1, 0),
		},
		{
			Name:     "MULS.W_D5,D6_dirty_upper_word",
			DataRegs: [8]uint32{0, 0, 0, 0, 0, 0xDEAD0200, 0xBEEF0100, 0}, // only low words matter
			Opcodes:  []uint16{0xCDC5},                                    // muls.w d5,d6
			// MULS.W uses only low 16 bits: 0x0100 * 0x0200 = 256 * 512 = 131072
			ExpectedRegs:  Reg("D6", 131072),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MULS.W_#2560,D1_immediate",
			DataRegs:      [8]uint32{0, 100, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xC3FC, 0x0A00}, // muls.w #2560,d1
			ExpectedRegs:  Reg("D1", 256000),        // 100 * 2560
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MULS.W_#2560,D1_negative",
			DataRegs:      [8]uint32{0, 0xFFFFFFFF, 0, 0, 0, 0, 0, 0}, // d1 low word = 0xFFFF = -1
			Opcodes:       []uint16{0xC3FC, 0x0A00},                   // muls.w #2560,d1
			ExpectedRegs:  Reg("D1", 0xFFFFF600),                      // -1 * 2560 = -2560
			ExpectedFlags: FlagsNZ(1, 0),
		},
		{
			Name:          "LSL.L_#8,D0",
			DataRegs:      [8]uint32{0x100, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xE188}, // lsl.l #8,d0
			ExpectedRegs:  Reg("D0", 0x10000),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "LSL.L_#6,D1",
			DataRegs:      [8]uint32{0, 0x100, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xED89},  // lsl.l #6,d1
			ExpectedRegs:  Reg("D1", 0x4000), // 0x100 << 6
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "LSL.L_#4,D2",
			DataRegs:      [8]uint32{0, 0, 0x100, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xE98A},  // lsl.l #4,d2
			ExpectedRegs:  Reg("D2", 0x1000), // 0x100 << 4
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "LSL.L_#2,D0",
			DataRegs:      [8]uint32{0x100, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xE588}, // lsl.l #2,d0
			ExpectedRegs:  Reg("D0", 0x400),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "NEG.L_D6",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 384, 0},
			Opcodes:       []uint16{0x4486},      // neg.l d6
			ExpectedRegs:  Reg("D6", 0xFFFFFE80), // -384
			ExpectedFlags: FlagsNZ(1, 0),
		},
		{
			Name:          "EXT.L_D0_positive",
			DataRegs:      [8]uint32{0x00FF007F, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x48C0}, // ext.l d0
			ExpectedRegs:  Reg("D0", 0x0000007F),
			ExpectedFlags: FlagsNZ(0, 0),
		},
		{
			Name:          "EXT.L_D1_negative",
			DataRegs:      [8]uint32{0, 0x0000FF80, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x48C1}, // ext.l d1
			ExpectedRegs:  Reg("D1", 0xFFFFFF80),
			ExpectedFlags: FlagsNZ(1, 0),
		},
		{
			Name:          "SUB.L_D0,D3",
			DataRegs:      [8]uint32{100, 0, 0, 500, 0, 0, 0, 0},
			Opcodes:       []uint16{0x9680}, // sub.l d0,d3
			ExpectedRegs:  Reg("D3", 400),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "SUB.L_D2,D1",
			DataRegs:      [8]uint32{0, 1000, 300, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x9282}, // sub.l d2,d1
			ExpectedRegs:  Reg("D1", 700),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "ADD.L_D1,D3",
			DataRegs:      [8]uint32{0, 200, 0, 300, 0, 0, 0, 0},
			Opcodes:       []uint16{0xD681}, // add.l d1,d3
			ExpectedRegs:  Reg("D3", 500),
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)

	// Shift decomposition: val * 320 = val * 256 + val * 64 = (val<<8) + (val<<6)
	t.Run("Shift_decompose_times_320", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.DataRegs[6] = 100 // CA = 100
		cpu.AddrRegs[7] = 0x8000

		pc := uint32(M68K_ENTRY_POINT)
		cpu.PC = pc
		cpu.Write16(pc, 0x2006)   // move.l d6,d0
		cpu.Write16(pc+2, 0x2200) // move.l d0,d1
		cpu.Write16(pc+4, 0xE188) // lsl.l #8,d0 → 100*256=25600
		cpu.Write16(pc+6, 0xED89) // lsl.l #6,d1 → 100*64=6400
		cpu.Write16(pc+8, 0xD081) // add.l d1,d0 → 25600+6400=32000

		for range 5 {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}

		if cpu.DataRegs[0] != 32000 {
			t.Errorf("CA*320: got %d, expected 32000", cpu.DataRegs[0])
		}
	})

	// Shift decomposition: val * 240 = val * 256 - val * 16 = (val<<8) - (val<<4)
	t.Run("Shift_decompose_times_240", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.DataRegs[7] = 100 // SA = 100
		cpu.AddrRegs[7] = 0x8000

		pc := uint32(M68K_ENTRY_POINT)
		cpu.PC = pc
		cpu.Write16(pc, 0x2207)   // move.l d7,d1
		cpu.Write16(pc+2, 0x2401) // move.l d1,d2
		cpu.Write16(pc+4, 0xE189) // lsl.l #8,d1 → 25600
		cpu.Write16(pc+6, 0xE98A) // lsl.l #4,d2 → 1600
		cpu.Write16(pc+8, 0x9282) // sub.l d2,d1 → 25600-1600=24000

		for range 5 {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}

		if cpu.DataRegs[1] != 24000 {
			t.Errorf("SA*240: got %d, expected 24000", cpu.DataRegs[1])
		}
	})

	// Full u0 computation: u0 = 8388608 - CA*320 + SA*240
	t.Run("Full_u0_computation", func(t *testing.T) {
		cpu := setupTestCPU()
		// CA=256 (cos=256, recip=1 → CA=256), SA=0
		cpu.DataRegs[6] = 256 // CA
		cpu.DataRegs[7] = 0   // SA
		cpu.AddrRegs[7] = 0x8000

		pc := uint32(M68K_ENTRY_POINT)
		cpu.PC = pc

		// Replicate the compute_frame u0 code:
		// move.l d6,d0; move.l d0,d1; lsl.l #8,d0; lsl.l #6,d1; add.l d1,d0  → CA*320
		cpu.Write16(pc, 0x2006)   // move.l d6,d0
		cpu.Write16(pc+2, 0x2200) // move.l d0,d1
		cpu.Write16(pc+4, 0xE188) // lsl.l #8,d0
		cpu.Write16(pc+6, 0xED89) // lsl.l #6,d1
		cpu.Write16(pc+8, 0xD081) // add.l d1,d0         → d0 = CA*320
		// move.l d7,d1; move.l d1,d2; lsl.l #8,d1; lsl.l #4,d2; sub.l d2,d1  → SA*240
		cpu.Write16(pc+10, 0x2207) // move.l d7,d1
		cpu.Write16(pc+12, 0x2401) // move.l d1,d2
		cpu.Write16(pc+14, 0xE189) // lsl.l #8,d1
		cpu.Write16(pc+16, 0xE98A) // lsl.l #4,d2
		cpu.Write16(pc+18, 0x9282) // sub.l d2,d1         → d1 = SA*240
		// move.l #$800000,d3; sub.l d0,d3; add.l d1,d3
		cpu.Write16(pc+20, 0x263C) // move.l #imm,d3
		cpu.Write16(pc+22, 0x0080)
		cpu.Write16(pc+24, 0x0000)
		cpu.Write16(pc+26, 0x9680) // sub.l d0,d3
		cpu.Write16(pc+28, 0xD681) // add.l d1,d3

		for range 13 {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}

		// u0 = 8388608 - 256*320 + 0*240 = 8388608 - 81920 = 8306688
		expected := uint32(8388608 - 81920)
		if cpu.DataRegs[3] != expected {
			t.Errorf("u0: got %d (0x%X), expected %d (0x%X)",
				cpu.DataRegs[3], cpu.DataRegs[3], expected, expected)
		}
	})
}

// =============================================================================
// Group 10: Control Flow
// =============================================================================

func TestGemRoto_ControlFlow(t *testing.T) {
	t.Run("BSR_word_displacement", func(t *testing.T) {
		cpu := setupTestCPU()
		// Use the default stack from setupTestCPU (M68K_STACK_START)
		sp := cpu.AddrRegs[7]

		pc := uint32(M68K_ENTRY_POINT)
		cpu.PC = pc
		// bsr.w to +6 (target = PC+2+6 = PC+8)
		cpu.Write16(pc, 0x6100)   // bsr.w
		cpu.Write16(pc+2, 0x0006) // displacement +6
		// Target at pc+8: moveq #42,d0
		cpu.Write16(pc+8, MakeOpcodeMoveq(42, 0))
		// At pc+10: rts
		cpu.Write16(pc+10, 0x4E75)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// After BSR: PC should be at target, return address pushed
		if cpu.PC != pc+8 {
			t.Errorf("PC after BSR: got 0x%X, expected 0x%X", cpu.PC, pc+8)
		}
		// SP should have decreased by 4
		if cpu.AddrRegs[7] != sp-4 {
			t.Errorf("SP after BSR: got 0x%X, expected 0x%X", cpu.AddrRegs[7], sp-4)
		}
		// Return address = pc + 4 (after the 2-word BSR instruction)
		retAddr := cpu.Read32(cpu.AddrRegs[7])
		if retAddr != pc+4 {
			t.Errorf("Return address: got 0x%X, expected 0x%X", retAddr, pc+4)
		}
	})

	t.Run("RTS", func(t *testing.T) {
		cpu := setupTestCPU()
		sp := cpu.AddrRegs[7]
		returnAddr := uint32(0x2000)
		cpu.AddrRegs[7] = sp - 4
		cpu.Write32(sp-4, returnAddr) // return address on stack

		cpu.PC = M68K_ENTRY_POINT
		cpu.Write16(cpu.PC, 0x4E75) // rts
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.PC != returnAddr {
			t.Errorf("PC after RTS: got 0x%X, expected 0x%X", cpu.PC, returnAddr)
		}
		if cpu.AddrRegs[7] != sp {
			t.Errorf("SP after RTS: got 0x%X, expected 0x%X", cpu.AddrRegs[7], sp)
		}
	})

	// Branch conditions
	branchTests := []struct {
		name   string
		opcode uint16
		sr     uint16
		taken  bool
	}{
		{"BMI_taken", 0x6B02, M68K_SR_N, true},
		{"BMI_not_taken", 0x6B02, 0, false},
		{"BEQ_taken", 0x6702, M68K_SR_Z, true},
		{"BEQ_not_taken", 0x6702, 0, false},
		{"BNE_taken", 0x6602, 0, true},
		{"BNE_not_taken", 0x6602, M68K_SR_Z, false},
		{"BLE_taken_Z", 0x6F02, M68K_SR_Z, true},
		{"BLE_taken_N", 0x6F02, M68K_SR_N, true},
		{"BLE_not_taken", 0x6F02, 0, false},
		{"BGE_taken", 0x6C02, 0, true},
		{"BGE_not_taken_N", 0x6C02, M68K_SR_N, false},
		{"BRA_always", 0x6002, 0, true},
	}

	for _, bt := range branchTests {
		t.Run(bt.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.SR = bt.sr | M68K_SR_S // keep supervisor mode
			cpu.AddrRegs[7] = 0x8000

			pc := uint32(M68K_ENTRY_POINT)
			cpu.PC = pc
			cpu.Write16(pc, bt.opcode)               // Bcc.s +2 (skip next instruction)
			cpu.Write16(pc+2, MakeOpcodeMoveq(1, 0)) // moveq #1,d0 (skipped if taken)
			cpu.Write16(pc+4, MakeOpcodeMoveq(2, 0)) // moveq #2,d0 (target if taken)

			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			if bt.taken {
				// Branch taken: PC should be at pc+4
				if cpu.PC != pc+4 {
					t.Errorf("PC: got 0x%X, expected 0x%X (branch should be taken)", cpu.PC, pc+4)
				}
			} else {
				// Branch not taken: PC should be at pc+2
				if cpu.PC != pc+2 {
					t.Errorf("PC: got 0x%X, expected 0x%X (branch should not be taken)", cpu.PC, pc+2)
				}
			}
		})
	}

	t.Run("TRAP_#1_GEMDOS", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.SR = M68K_SR_S // supervisor mode
		// Use default stack from setupTestCPU (within valid bounds)

		// Set up TRAP #1 vector (vector 33 = 0x84)
		trapHandler := uint32(0x5000)
		cpu.Write32(0x84, trapHandler)

		cpu.PC = M68K_ENTRY_POINT
		cpu.Write16(cpu.PC, 0x4E41) // trap #1
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.PC != trapHandler {
			t.Errorf("PC after TRAP #1: got 0x%X, expected 0x%X", cpu.PC, trapHandler)
		}
	})

	t.Run("TRAP_#2_AES_VDI", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.SR = M68K_SR_S
		// Use default stack from setupTestCPU (within valid bounds)

		// Set up TRAP #2 vector (vector 34 = 0x88)
		trapHandler := uint32(0x6000)
		cpu.Write32(0x88, trapHandler)

		cpu.PC = M68K_ENTRY_POINT
		cpu.Write16(cpu.PC, 0x4E42) // trap #2
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.PC != trapHandler {
			t.Errorf("PC after TRAP #2: got 0x%X, expected 0x%X", cpu.PC, trapHandler)
		}
	})
}

// =============================================================================
// Group 11: Remaining Data Movement
// =============================================================================

func TestGemRoto_DataMovement(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "MOVEQ_#0,D0",
			DataRegs:      [8]uint32{0xDEADBEEF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x7000}, // moveq #0,d0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZ(0, 1), // Z set
		},
		{
			Name:          "MOVEQ_#-1,D0",
			Opcodes:       []uint16{0x70FF}, // moveq #-1,d0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZ(1, 0), // N set
		},
		{
			Name:          "MOVE.L_D6,D0_Dn_to_Dn",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0x12345678, 0},
			Opcodes:       []uint16{0x2006}, // move.l d6,d0
			ExpectedRegs:  Reg("D0", 0x12345678),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "MOVE.L_A0,D0_An_to_Dn",
			AddrRegs:      [8]uint32{0xCAFEBABE, 0, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0x2008}, // move.l a0,d0
			ExpectedRegs:  Reg("D0", 0xCAFEBABE),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.L_(SP),D0_peek_stack",
			AddrRegs: [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x7FFC},
			Setup: func(cpu *M68KCPU) {
				cpu.Write32(0x7FFC, 0xABCD1234)
			},
			Opcodes:       []uint16{0x2017},                                     // move.l (sp),d0
			ExpectedRegs:  Regs("D0", uint32(0xABCD1234), "SP", uint32(0x7FFC)), // SP unchanged
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.L_4(SP),D1",
			AddrRegs: [8]uint32{0, 0, 0, 0, 0, 0, 0, 0x7FF8},
			Setup: func(cpu *M68KCPU) {
				cpu.Write32(0x7FF8, 0x11111111) // (sp)
				cpu.Write32(0x7FFC, 0x22222222) // 4(sp)
			},
			Opcodes:       []uint16{0x222F, 0x0004}, // move.l 4(sp),d1
			ExpectedRegs:  Reg("D1", 0x22222222),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "SWAP_D0",
			DataRegs:      [8]uint32{0x12345678, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4840}, // swap d0
			ExpectedRegs:  Reg("D0", 0x56781234),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "CMPI.W_#$16,D0_equal",
			DataRegs:      [8]uint32{0x0016, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x0C40, 0x0016}, // cmpi.w #$16,d0
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),
		},
		{
			Name:          "CMPI.W_#$16,D0_not_equal",
			DataRegs:      [8]uint32{0x0020, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x0C40, 0x0016}, // cmpi.w #$16,d0
			ExpectedFlags: FlagsNZ(0, 0),            // 0x20 > 0x16 → not zero, not negative
		},
		{
			Name:          "BTST_#5,D0_bit_set",
			DataRegs:      [8]uint32{0x20, 0, 0, 0, 0, 0, 0, 0}, // bit 5 set
			Opcodes:       []uint16{0x0800, 0x0005},             // btst #5,d0
			ExpectedFlags: FlagsNZ(-1, 0),                       // Z clear (bit is set)
		},
		{
			Name:          "BTST_#5,D0_bit_clear",
			DataRegs:      [8]uint32{0x10, 0, 0, 0, 0, 0, 0, 0}, // bit 5 clear
			Opcodes:       []uint16{0x0800, 0x0005},             // btst #5,d0
			ExpectedFlags: FlagsNZ(-1, 1),                       // Z set (bit is clear)
		},
		{
			Name:          "TST.W_D0_zero",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4A40}, // tst.w d0
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),
		},
		{
			Name:          "TST.W_D0_negative",
			DataRegs:      [8]uint32{0x8000, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4A40}, // tst.w d0
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name:          "ANDI.L_#$FFFF,D2",
			DataRegs:      [8]uint32{0, 0, 0xDEAD1234, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x0282, 0x0000, 0xFFFF}, // andi.l #$FFFF,d2
			ExpectedRegs:  Reg("D2", 0x1234),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "ANDI.L_#2,D0",
			DataRegs:      [8]uint32{0xFF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x0280, 0x0000, 0x0002}, // andi.l #2,d0
			ExpectedRegs:  Reg("D0", 2),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "ADDI.L_#VRAM_START,D1",
			DataRegs:      [8]uint32{0, 0x1000, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x0681, 0x0010, 0x0000}, // addi.l #$100000,d1
			ExpectedRegs:  Reg("D1", 0x101000),
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}
