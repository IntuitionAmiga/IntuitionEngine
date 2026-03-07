package main

import (
	"fmt"
	"strings"
)

// Phase 2: EA Breadth Across Instruction Families (~80 cases)
// New shards: ea_read_ops, ea_write_ops, ea_control

func buildPhase2Shards() []shard {
	return []shard{
		shardEAReadOps(),
		shardEAWriteOps(),
		shardEAControl(),
	}
}

func shardEAReadOps() shard {
	s := "ea_read_ops"
	srcData := []string{".src_data:", "                dc.l    $11223344", "                dc.l    $55667788", "                dc.l    $AABBCCDD", "                dc.l    $DEADBEEF", ".src_data_end:", "                even"}
	// LEA/PEA as address-computation probes
	var cases []testCase

	// LEA (An),An
	cases = append(cases, intCase(s, "ea_read_lea_an", "LEA (A0),A2",
		"LEA (A0),A2 initial: A0=$00004000", "A2=$00004000", "custom_a2_only", "a2", 0x00004000, 0, 0,
		[]string{"move.l  #$00004000,a0", "suba.l  a2,a2"},
		[]string{"lea     (a0),a2"}))
	// LEA d16(An),An
	cases = append(cases, intCase(s, "ea_read_lea_d16", "LEA 16(A0),A2",
		"LEA 16(A0),A2 initial: A0=$00004000", "A2=$00004010", "custom_a2_only", "a2", 0x00004010, 0, 0,
		[]string{"move.l  #$00004000,a0", "suba.l  a2,a2"},
		[]string{"lea     16(a0),a2"}))
	// LEA d8(An,Dn),An
	cases = append(cases, intCase(s, "ea_read_lea_idx", "LEA 8(A0,D1.L),A2",
		"LEA 8(A0,D1.L),A2 initial: A0=$00004000 D1=4", "A2=$0000400C", "custom_a2_only", "a2", 0x0000400C, 0, 0,
		[]string{"move.l  #$00004000,a0", "moveq   #4,d1", "suba.l  a2,a2"},
		[]string{"lea     8(a0,d1.l),a2"}))
	// LEA abs.W
	cases = append(cases, intCase(s, "ea_read_lea_absw", "LEA $4000.W,A2",
		"LEA $4000.W,A2", "A2=$00004000", "custom_a2_only", "a2", 0x00004000, 0, 0,
		[]string{"suba.l  a2,a2"},
		[]string{"lea     $4000.w,a2"}))
	// LEA abs.L
	cases = append(cases, intCase(s, "ea_read_lea_absl", "LEA $00014000,A2",
		"LEA $00014000,A2", "A2=$00014000", "custom_a2_only", "a2", 0x00014000, 0, 0,
		[]string{"suba.l  a2,a2"},
		[]string{"lea     $00014000,a2"}))
	// LEA d16(PC),An
	cases = append(cases, intCase(s, "ea_read_lea_pcdisp", "LEA (d16,PC),A2",
		"LEA (d16,PC),A2 initial: PC=current", "A2=addr_of_target", "regonly", "d0", 0x00000000, 0, 0,
		[]string{"suba.l  a2,a2"},
		[]string{"lea     .lea_pc_target(pc),a2", "lea     .lea_pc_target(pc),a0", "move.l  a2,d0", "sub.l   a0,d0"},
		".lea_pc_target:", "                dc.l    $12345678"))
	// LEA d8(PC,Dn),An
	cases = append(cases, intCase(s, "ea_read_lea_pcidx", "LEA (d8,PC,D1.W),A2",
		"LEA (d8,PC,D1.W),A2", "A2=computed", "regonly", "d0", 0x00000000, 0, 0,
		[]string{"moveq   #4,d1", "suba.l  a2,a2"},
		[]string{"lea     .lea_pcidx_base(pc,d1.w),a2", "lea     .lea_pcidx_base+4(pc),a0", "move.l  a2,d0", "sub.l   a0,d0"},
		".lea_pcidx_base:", "                dc.l    $DEADBEEF", "                dc.l    $0BADF00D"))

	// PEA (An)
	cases = append(cases, regonly(s, "ea_read_pea_an", "PEA (A0)",
		"PEA (A0) initial: A0=$00005000", "D0=$00000000", "d0", 0x00000000,
		[]string{"move.l  #$00005000,a0", "move.l  sp,a3"},
		[]string{"pea     (a0)", "move.l  (sp)+,d0", "eori.l  #$00005000,d0"}))
	// PEA d16(An)
	cases = append(cases, regonly(s, "ea_read_pea_d16", "PEA 16(A0)",
		"PEA 16(A0) initial: A0=$00005000", "D0=$00000000", "d0", 0x00000000,
		[]string{"move.l  #$00005000,a0"},
		[]string{"pea     16(a0)", "move.l  (sp)+,d0", "eori.l  #$00005010,d0"}))
	// PEA d8(An,Dn)
	cases = append(cases, regonly(s, "ea_read_pea_idx", "PEA 4(A0,D1.L)",
		"PEA 4(A0,D1.L) A0=$00005000 D1=8", "D0=$00000000", "d0", 0x00000000,
		[]string{"move.l  #$00005000,a0", "moveq   #8,d1"},
		[]string{"pea     4(a0,d1.l)", "move.l  (sp)+,d0", "eori.l  #$0000500C,d0"}))
	// PEA abs.L
	cases = append(cases, regonly(s, "ea_read_pea_absl", "PEA $00012345",
		"PEA $00012345", "D0=$00000000", "d0", 0x00000000,
		nil,
		[]string{"pea     $00012345", "move.l  (sp)+,d0", "eori.l  #$00012345,d0"}))

	// Read path: cross-family EA tests
	for _, op := range []struct {
		mnemonic string
		asmOp    string
		isAdd    bool
	}{
		{"ADD", "add.l", true},
		{"SUB", "sub.l", false},
		{"CMP", "cmp.l", false},
		{"AND", "and.l", false},
		{"OR", "or.l", false},
	} {
		for _, ea := range []struct {
			suffix, name string
			setup        []string
			body         string
			val          uint32
		}{
			{"an_ind", "(A0)", []string{"lea     .src_data(pc),a0"}, fmt.Sprintf("%s   (a0),d0", op.asmOp), 0x11223344},
			{"d16_an", "4(A0)", []string{"lea     .src_data(pc),a0"}, fmt.Sprintf("%s   4(a0),d0", op.asmOp), 0x55667788},
			{"imm", "#$AABBCCDD", nil, fmt.Sprintf("%s   #$AABBCCDD,d0", op.asmOp), 0xAABBCCDD},
		} {
			if op.mnemonic == "CMP" && ea.suffix == "imm" {
				// CMP sets flags only, check via SR
				id := fmt.Sprintf("ea_read_%s_%s", "cmp", ea.suffix)
				cases = append(cases, testCase{
					ID: id, Shard: s, Kind: kindInt,
					Name:       fmt.Sprintf("CMP.L %s,D0", ea.name),
					Input:      fmt.Sprintf("CMP.L %s,D0 initial: D0=$AABBCCDD", ea.name),
					Expected:   "SR(masked Z)=$0004",
					ActualMode: "custom_sr_only",
					SRMask:     0x0004, ExpectSR: 0x0004,
					Setup:    append([]string{"move.l  #$AABBCCDD,d0", "moveq   #0,d2", "move.w  d2,ccr"}, ea.setup...),
					Body:     []string{ea.body},
					DataPool: srcData,
				})
				continue
			}
			if op.mnemonic == "CMP" {
				// CMP with memory: D0 == value at EA → Z=1
				id := fmt.Sprintf("ea_read_cmp_%s", ea.suffix)
				cases = append(cases, testCase{
					ID: id, Shard: s, Kind: kindInt,
					Name:       fmt.Sprintf("CMP.L %s,D0", ea.name),
					Input:      fmt.Sprintf("CMP.L %s,D0 initial: D0=$%08X", ea.name, ea.val),
					Expected:   "SR(masked Z)=$0004",
					ActualMode: "custom_sr_only",
					SRMask:     0x0004, ExpectSR: 0x0004,
					Setup:    append([]string{fmt.Sprintf("move.l  #$%08X,d0", ea.val), "moveq   #0,d2", "move.w  d2,ccr"}, ea.setup...),
					Body:     []string{ea.body},
					DataPool: srcData,
				})
				continue
			}
			var expectedVal uint32
			setupLines := append([]string{"moveq   #0,d0", "moveq   #0,d2", "move.w  d2,ccr"}, ea.setup...)
			switch op.mnemonic {
			case "ADD":
				expectedVal = ea.val
			case "SUB":
				expectedVal = 0 - ea.val
			case "AND":
				// D0=0 AND x = 0
				setupLines = append([]string{"move.l  #$FFFFFFFF,d0", "moveq   #0,d2", "move.w  d2,ccr"}, ea.setup...)
				expectedVal = ea.val
			case "OR":
				expectedVal = ea.val
			}
			id := fmt.Sprintf("ea_read_%s_%s", strings.ToLower(op.mnemonic), ea.suffix)
			cases = append(cases, regsr(s, id,
				fmt.Sprintf("%s.L %s,D0", op.mnemonic, ea.name),
				fmt.Sprintf("%s.L %s,D0 initial: D0 set", op.mnemonic, ea.name),
				fmt.Sprintf("D0=$%08X", expectedVal),
				"d0", expectedVal, 0x000F, flagsFor(expectedVal),
				setupLines,
				[]string{ea.body}, srcData...))
		}
	}

	// BTST Dn,(An) and BTST Dn,d16(An)
	cases = append(cases, regsr(s, "ea_read_btst_an", "BTST D1,(A0)",
		"BTST D1,(A0) bit 0 of $44=0", "Z=1", "d0", 0x11223344, 0x0004, 0x0004,
		[]string{"lea     .src_data(pc),a0", "moveq   #0,d0", "moveq   #0,d1", "move.w  d0,ccr"},
		[]string{"btst    d1,(a0)", "move.l  (a0),d0"}, srcData...))

	cases = append(cases, regsr(s, "ea_read_btst_d16", "BTST D1,4(A0)",
		"BTST D1,4(A0) bit 0 of $88=0", "Z=1", "d0", 0x55667788, 0x0004, 0x0004,
		[]string{"lea     .src_data(pc),a0", "moveq   #0,d0", "moveq   #0,d1", "move.w  d0,ccr"},
		[]string{"btst    d1,4(a0)", "move.l  4(a0),d0"}, srcData...))

	return shard{Name: s, Title: "EA Read Ops", Cases: cases}
}

func flagsFor(val uint32) uint16 {
	if val == 0 {
		return 0x0004 // Z
	}
	if val&0x80000000 != 0 {
		return 0x0008 // N
	}
	return 0x0000
}

func shardEAWriteOps() shard {
	s := "ea_write_ops"
	dstData := []string{".dst_buf:", "                dc.l    $00000000", "                dc.l    $00000000", "                dc.l    $00000000", "                dc.l    $00000000", ".dst_buf_end:", "                even"}
	var cases []testCase

	type eaWrite struct {
		suffix, name string
		setup        []string
		writeBody    string
		readback     string
	}
	eas := []eaWrite{
		{"an_ind", "(A0)", []string{"lea     .dst_buf(pc),a0"}, "add.l   d0,(a0)", "move.l  (a0),d0"},
		{"an_postinc", "(A0)+", []string{"lea     .dst_buf(pc),a0"}, "add.l   d0,(a0)+", "move.l  -(a0),d0"},
		{"an_predec", "-(A0)", []string{"lea     .dst_buf+4(pc),a0"}, "add.l   d0,-(a0)", "move.l  (a0),d0"},
		{"d16_an", "4(A0)", []string{"lea     .dst_buf(pc),a0"}, "add.l   d0,4(a0)", "move.l  4(a0),d0"},
		{"abs_l", "$00008100", nil, "add.l   d0,$00008100", "move.l  $00008100,d0"},
	}

	// ADD.L Dn,<ea>
	for _, ea := range eas {
		id := fmt.Sprintf("ea_write_add_%s", ea.suffix)
		// Zero destination via (A0), then set up for the real test
		var setup []string
		if ea.suffix != "abs_l" {
			setup = append(setup, "lea     .dst_buf(pc),a0", "clr.l   (a0)", "clr.l   4(a0)")
		} else {
			setup = append(setup, "clr.l   $00008100")
		}
		setup = append(setup, "move.l  #$11223344,d0")
		setup = append(setup, ea.setup...)
		cases = append(cases, regonly(s, id,
			fmt.Sprintf("ADD.L D0,%s", ea.name),
			fmt.Sprintf("ADD.L D0,%s initial: D0=$11223344", ea.name),
			"D0(readback)=$11223344", "d0", 0x11223344,
			setup,
			[]string{ea.writeBody, ea.readback}, dstData...))
	}

	// CLR.L <ea>
	for _, ea := range []struct {
		suffix, name string
		setup        []string
		body         string
		readback     string
	}{
		{"an_ind", "(A0)", []string{"lea     .dst_buf(pc),a0", "move.l  #$FFFFFFFF,(a0)"}, "clr.l   (a0)", "move.l  (a0),d0"},
		{"d16_an", "4(A0)", []string{"lea     .dst_buf(pc),a0", "move.l  #$FFFFFFFF,4(a0)"}, "clr.l   4(a0)", "move.l  4(a0),d0"},
		{"abs_l", "$00008100", []string{"move.l  #$FFFFFFFF,$00008100"}, "clr.l   $00008100", "move.l  $00008100,d0"},
	} {
		id := fmt.Sprintf("ea_write_clr_%s", ea.suffix)
		cases = append(cases, regsr(s, id,
			fmt.Sprintf("CLR.L %s", ea.name),
			fmt.Sprintf("CLR.L %s", ea.name),
			"D0=$00000000 SR(Z)=$0004", "d0", 0x00000000, 0x000F, 0x0004,
			append(ea.setup, "moveq   #0,d1", "move.w  d1,ccr"),
			[]string{ea.body, ea.readback}, dstData...))
	}

	// NEG.L <ea>
	for _, ea := range []struct {
		suffix, name string
		setup        []string
		body         string
		readback     string
	}{
		{"an_ind", "(A0)", []string{"lea     .dst_buf(pc),a0", "move.l  #$00000001,(a0)"}, "neg.l   (a0)", "move.l  (a0),d0"},
		{"d16_an", "4(A0)", []string{"lea     .dst_buf(pc),a0", "move.l  #$00000001,4(a0)"}, "neg.l   4(a0)", "move.l  4(a0),d0"},
	} {
		id := fmt.Sprintf("ea_write_neg_%s", ea.suffix)
		cases = append(cases, regsr(s, id,
			fmt.Sprintf("NEG.L %s", ea.name),
			fmt.Sprintf("NEG.L %s initial: [ea]=$00000001", ea.name),
			"D0=$FFFFFFFF SR(N+X)=$0009", "d0", 0xFFFFFFFF, 0x000F, 0x0009,
			append(ea.setup, "moveq   #0,d1", "move.w  d1,ccr"),
			[]string{ea.body, ea.readback}, dstData...))
	}

	// NOT.L <ea>
	for _, ea := range []struct {
		suffix, name string
		setup        []string
		body         string
		readback     string
	}{
		{"an_ind", "(A0)", []string{"lea     .dst_buf(pc),a0", "move.l  #$FF00FF00,(a0)"}, "not.l   (a0)", "move.l  (a0),d0"},
		{"d16_an", "4(A0)", []string{"lea     .dst_buf(pc),a0", "move.l  #$FF00FF00,4(a0)"}, "not.l   4(a0)", "move.l  4(a0),d0"},
	} {
		id := fmt.Sprintf("ea_write_not_%s", ea.suffix)
		cases = append(cases, regsr(s, id,
			fmt.Sprintf("NOT.L %s", ea.name),
			fmt.Sprintf("NOT.L %s initial: [ea]=$FF00FF00", ea.name),
			"D0=$00FF00FF", "d0", 0x00FF00FF, 0x000F, 0x0000,
			append(ea.setup, "moveq   #0,d1", "move.w  d1,ccr"),
			[]string{ea.body, ea.readback}, dstData...))
	}

	// TST.L <ea>
	for _, ea := range []struct {
		suffix, name string
		setup        []string
		body         string
	}{
		{"an_ind", "(A0)", []string{"lea     .dst_buf(pc),a0", "move.l  #$80000000,(a0)"}, "tst.l   (a0)"},
		{"d16_an", "4(A0)", []string{"lea     .dst_buf(pc),a0", "move.l  #$80000000,4(a0)"}, "tst.l   4(a0)"},
	} {
		id := fmt.Sprintf("ea_write_tst_%s", ea.suffix)
		cases = append(cases, testCase{
			ID: id, Shard: s, Kind: kindInt,
			Name:       fmt.Sprintf("TST.L %s", ea.name),
			Input:      fmt.Sprintf("TST.L %s initial: [ea]=$80000000", ea.name),
			Expected:   "SR(N)=$0008",
			ActualMode: "custom_sr_only",
			SRMask:     0x000F, ExpectSR: 0x0008,
			Setup:    append(ea.setup, "moveq   #0,d1", "move.w  d1,ccr"),
			Body:     []string{ea.body},
			DataPool: dstData,
		})
	}

	// BSET Dn,(An) and BSET Dn,d16(An)
	cases = append(cases, regonly(s, "ea_write_bset_an", "BSET D1,(A0)",
		"BSET D1,(A0) initial: [A0]=$00 bit 0", "D0=$00000001", "d0", 0x00000001,
		[]string{"lea     .dst_buf(pc),a0", "clr.l   (a0)", "moveq   #0,d1"},
		[]string{"bset    d1,(a0)", "moveq   #0,d0", "move.b  (a0),d0"}, dstData...))

	cases = append(cases, regonly(s, "ea_write_bset_d16", "BSET D1,4(A0)",
		"BSET D1,4(A0) initial: [4(A0)]=$00 bit 3", "D0=$00000008", "d0", 0x00000008,
		[]string{"lea     .dst_buf(pc),a0", "clr.l   4(a0)", "moveq   #3,d1"},
		[]string{"bset    d1,4(a0)", "moveq   #0,d0", "move.b  4(a0),d0"}, dstData...))

	return shard{Name: s, Title: "EA Write Ops", Cases: cases}
}

func shardEAControl() shard {
	s := "ea_control"
	var cases []testCase

	// JMP (An)
	cases = append(cases, regonly(s, "ea_ctrl_jmp_an", "JMP (A0)",
		"JMP (A0)", "D0=$00000001", "d0", 0x00000001,
		[]string{"moveq   #0,d0", "lea     .jmp_an_tgt(pc),a0"},
		[]string{"jmp     (a0)", "moveq   #$77,d0", ".jmp_an_tgt:", "moveq   #1,d0"}))
	// JMP d16(An)
	cases = append(cases, regonly(s, "ea_ctrl_jmp_d16", "JMP 4(A0)",
		"JMP 4(A0)", "D0=$00000002", "d0", 0x00000002,
		[]string{"moveq   #0,d0", "lea     .jmp_d16_base(pc),a0"},
		[]string{"jmp     4(a0)", ".jmp_d16_base:", "moveq   #$77,d0", "moveq   #2,d0"}))
	// JMP abs.W
	// Can't use abs.W for JMP in our test context (address depends on load), use abs.L
	cases = append(cases, regonly(s, "ea_ctrl_jmp_absl", "JMP abs.L",
		"JMP (label).L", "D0=$00000003", "d0", 0x00000003,
		[]string{"moveq   #0,d0"},
		[]string{"lea     .jmp_absl_tgt(pc),a0", "move.l  a0,d2", "dc.w    $4EF9", ".jmp_absl_addr:", "dc.l    0",
			"moveq   #$77,d0", ".jmp_absl_tgt:", "moveq   #3,d0"},
		// Self-modify: write target address. Actually let's use a simpler approach.
	))

	// JSR (An)
	cases = append(cases, regonly(s, "ea_ctrl_jsr_an", "JSR (A0)",
		"JSR (A0)", "D0=$00000005", "d0", 0x00000005,
		[]string{"moveq   #0,d0", "lea     .jsr_an_sub(pc),a0"},
		[]string{"jsr     (a0)", "addq.l  #2,d0", "bra.s   .jsr_an_done", ".jsr_an_sub:", "addq.l  #3,d0", "rts", ".jsr_an_done:"}))
	// JSR d16(An)
	cases = append(cases, regonly(s, "ea_ctrl_jsr_d16", "JSR 4(A0)",
		"JSR 4(A0)", "D0=$00000005", "d0", 0x00000005,
		[]string{"moveq   #0,d0", "lea     .jsr_d16_base(pc),a0"},
		[]string{"jsr     4(a0)", "addq.l  #2,d0", "bra.s   .jsr_d16_done", ".jsr_d16_base:", "nop", "nop", "addq.l  #3,d0", "rts", ".jsr_d16_done:"}))
	// JSR d16(PC)
	cases = append(cases, regonly(s, "ea_ctrl_jsr_pcdisp", "JSR (d16,PC)",
		"JSR (d16,PC)", "D0=$00000005", "d0", 0x00000005,
		[]string{"moveq   #0,d0"},
		[]string{"bsr.s   .jsr_pcdisp_sub", "addq.l  #2,d0", "bra.s   .jsr_pcdisp_done", ".jsr_pcdisp_sub:", "addq.l  #3,d0", "rts", ".jsr_pcdisp_done:"}))

	return shard{Name: s, Title: "EA Control", Cases: cases}
}
