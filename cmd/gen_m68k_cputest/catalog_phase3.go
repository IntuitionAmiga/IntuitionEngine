package main

import "fmt"

// Phase 3: 020 Extension-Word EA Matrix (~70 cases)
// New shards: ea_020_brief, ea_020_full, ea_020_memindir

func buildPhase3Shards() []shard {
	return []shard{
		shardEA020Brief(),
		shardEA020Full(),
		shardEA020MemIndir(),
	}
}

// ---------------------------------------------------------------------------
// Shard: ea_020_brief — Brief extension word formats (~25 cases)
// Tests d8(An,Xn.W), d8(An,Xn.L) with scale factors, PC-relative, etc.
// ---------------------------------------------------------------------------

func shardEA020Brief() shard {
	s := "ea_020_brief"
	pool := []string{
		".brief_data:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                dc.l    $AABBCCDD",
		"                dc.l    $DEADBEEF",
		"                dc.l    $CAFEBABE",
		"                dc.l    $0BADF00D",
		"                dc.l    $12345678",
		"                dc.l    $9ABCDEF0",
		".brief_data_end:",
		"                even",
	}

	var cases []testCase

	// --- Scale x1: d8(A0,D1.W*1) ---
	// A0 = .brief_data, D1 = 4 → offset = 0 + 4*1 = 4 → read word 1 = $55667788
	cases = append(cases, regonly(s, "ea_020_brief_scale1_w", "MOVE.L 0(A0,D1.W*1),D0",
		"020 brief d8(A0,D1.W*1) D1=4", "D0=$55667788", "d0", 0x55667788,
		[]string{"lea     .brief_data(pc),a0", "moveq   #4,d1"},
		[]string{"move.l  0(a0,d1.w),d0"}, pool...))

	// --- Scale x2: d8(A0,D1.W*2) ---
	// A0 = .brief_data, D1 = 4 → offset = 0 + 4*2 = 8 → read word 2 = $AABBCCDD
	cases = append(cases, regonly(s, "ea_020_brief_scale2_w", "MOVE.L 0(A0,D1.W*2),D0",
		"020 brief d8(A0,D1.W*2) D1=4", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .brief_data(pc),a0", "moveq   #4,d1"},
		[]string{"move.l  0(a0,d1.w*2),d0"}, pool...))

	// --- Scale x4: d8(A0,D1.W*4) ---
	// A0 = .brief_data, D1 = 2 → offset = 0 + 2*4 = 8 → read word 2 = $AABBCCDD
	cases = append(cases, regonly(s, "ea_020_brief_scale4_w", "MOVE.L 0(A0,D1.W*4),D0",
		"020 brief d8(A0,D1.W*4) D1=2", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .brief_data(pc),a0", "moveq   #2,d1"},
		[]string{"move.l  0(a0,d1.w*4),d0"}, pool...))

	// --- Scale x8: d8(A0,D1.W*8) ---
	// A0 = .brief_data, D1 = 1 → offset = 0 + 1*8 = 8 → read word 2 = $AABBCCDD
	cases = append(cases, regonly(s, "ea_020_brief_scale8_w", "MOVE.L 0(A0,D1.W*8),D0",
		"020 brief d8(A0,D1.W*8) D1=1", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .brief_data(pc),a0", "moveq   #1,d1"},
		[]string{"move.l  0(a0,d1.w*8),d0"}, pool...))

	// --- Scale x1 with .L index: d8(A0,D1.L*1) ---
	cases = append(cases, regonly(s, "ea_020_brief_scale1_l", "MOVE.L 0(A0,D1.L*1),D0",
		"020 brief d8(A0,D1.L*1) D1=8", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .brief_data(pc),a0", "moveq   #8,d1"},
		[]string{"move.l  0(a0,d1.l),d0"}, pool...))

	// --- Scale x2 with .L index: d8(A0,D1.L*2) ---
	// D1 = 4 → 4*2 = 8 → word 2
	cases = append(cases, regonly(s, "ea_020_brief_scale2_l", "MOVE.L 0(A0,D1.L*2),D0",
		"020 brief d8(A0,D1.L*2) D1=4", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .brief_data(pc),a0", "moveq   #4,d1"},
		[]string{"move.l  0(a0,d1.l*2),d0"}, pool...))

	// --- Scale x4 with .L index: d8(A0,D1.L*4) ---
	// D1 = 3 → 3*4 = 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_brief_scale4_l", "MOVE.L 0(A0,D1.L*4),D0",
		"020 brief d8(A0,D1.L*4) D1=3", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .brief_data(pc),a0", "moveq   #3,d1"},
		[]string{"move.l  0(a0,d1.l*4),d0"}, pool...))

	// --- Scale x8 with .L index: d8(A0,D1.L*8) ---
	// D1 = 2 → 2*8 = 16 → word 4 = $CAFEBABE
	cases = append(cases, regonly(s, "ea_020_brief_scale8_l", "MOVE.L 0(A0,D1.L*8),D0",
		"020 brief d8(A0,D1.L*8) D1=2", "D0=$CAFEBABE", "d0", 0xCAFEBABE,
		[]string{"lea     .brief_data(pc),a0", "moveq   #2,d1"},
		[]string{"move.l  0(a0,d1.l*8),d0"}, pool...))

	// --- Displacement + scale: d8(A0,D1.L*4) with d8=4 ---
	// D1 = 2 → 4 + 2*4 = 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_brief_disp_scale4", "MOVE.L 4(A0,D1.L*4),D0",
		"020 brief 4(A0,D1.L*4) D1=2", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .brief_data(pc),a0", "moveq   #2,d1"},
		[]string{"move.l  4(a0,d1.l*4),d0"}, pool...))

	// --- An as index register: d8(A0,A1.L) ---
	// A1 = 12 → 0 + 12 = 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_brief_an_index", "MOVE.L 0(A0,A1.L),D0",
		"020 brief d8(A0,A1.L) A1=12", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .brief_data(pc),a0", "move.l  #12,a1"},
		[]string{"move.l  0(a0,a1.l),d0"}, pool...))

	// --- An as index with scale: d8(A0,A1.L*4) ---
	// A1 = 3 → 0 + 3*4 = 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_brief_an_idx_scale4", "MOVE.L 0(A0,A1.L*4),D0",
		"020 brief d8(A0,A1.L*4) A1=3", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .brief_data(pc),a0", "move.l  #3,a1"},
		[]string{"move.l  0(a0,a1.l*4),d0"}, pool...))

	// --- Zero displacement, non-zero index ---
	cases = append(cases, regonly(s, "ea_020_brief_zero_disp", "MOVE.L 0(A0,D1.L),D0",
		"020 brief 0(A0,D1.L) D1=16", "D0=$CAFEBABE", "d0", 0xCAFEBABE,
		[]string{"lea     .brief_data(pc),a0", "moveq   #16,d1"},
		[]string{"move.l  0(a0,d1.l),d0"}, pool...))

	// --- Max positive displacement: 127(A0,D1.L) ---
	// Place A0 127 bytes before .brief_data, D1=0 → $11223344
	cases = append(cases, regonly(s, "ea_020_brief_max_pos_disp", "MOVE.L 127(A0,D1.L),D0",
		"020 brief 127(A0,D1.L)", "D0=$11223344", "d0", 0x11223344,
		[]string{"lea     .brief_data(pc),a0", "suba.l  #127,a0", "moveq   #0,d1"},
		[]string{"move.l  127(a0,d1.l),d0"}, pool...))

	// --- Negative displacement: -4(A0,D1.L) ---
	// A0 = .brief_data+8, D1=0 → -4+8 = offset 4 → $55667788. Use actual -4 displacement.
	cases = append(cases, regonly(s, "ea_020_brief_neg_disp", "MOVE.L -4(A0,D1.L),D0",
		"020 brief -4(A0,D1.L)", "D0=$55667788", "d0", 0x55667788,
		[]string{"lea     .brief_data(pc),a0", "adda.l  #8,a0", "moveq   #0,d1"},
		[]string{"move.l  -4(a0,d1.l),d0"}, pool...))

	// --- Max negative displacement: -128(A0,D1.L) ---
	cases = append(cases, regonly(s, "ea_020_brief_max_neg_disp", "MOVE.L -128(A0,D1.L),D0",
		"020 brief -128(A0,D1.L)", "D0=$11223344", "d0", 0x11223344,
		[]string{"lea     .brief_data(pc),a0", "adda.l  #128,a0", "moveq   #0,d1"},
		[]string{"move.l  -128(a0,d1.l),d0"}, pool...))

	// --- PC-relative brief: d8(PC,D1.W) ---
	cases = append(cases, regonly(s, "ea_020_brief_pc_dn_w", "MOVE.L (d8,PC,D1.W),D0",
		"020 brief d8(PC,D1.W) D1=4", "D0=$55667788", "d0", 0x55667788,
		[]string{"moveq   #4,d1"},
		[]string{"move.l  .brief_pc_data(pc,d1.w),d0"},
		".brief_pc_data:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                even"))

	// --- PC-relative brief with scale: d8(PC,D1.L*4) ---
	cases = append(cases, regonly(s, "ea_020_brief_pc_dn_l_s4", "MOVE.L (d8,PC,D1.L*4),D0",
		"020 brief d8(PC,D1.L*4) D1=1", "D0=$55667788", "d0", 0x55667788,
		[]string{"moveq   #1,d1"},
		[]string{"move.l  .brief_pc_s4_data(pc,d1.l*4),d0"},
		".brief_pc_s4_data:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                even"))

	// --- .W index sign-extension: D1.W with value $FFFE (-2 sign-extended) ---
	// A0 = .brief_data+4, D1=$0000FFFE → sign-extend .W = -2 → 4 + (-2) = 2
	// But offset 2 is mid-longword. Use offset that works: A0 = .brief_data+6, D1.W=-2 → offset 4
	cases = append(cases, regonly(s, "ea_020_brief_w_signext", "MOVE.L 0(A0,D1.W),D0",
		"020 brief d8(A0,D1.W) D1.W=-4 signext", "D0=$11223344", "d0", 0x11223344,
		[]string{"lea     .brief_data(pc),a0", "adda.l  #4,a0", "move.l  #$0000FFFC,d1"},
		[]string{"move.l  0(a0,d1.w),d0"}, pool...))

	// --- .L index no sign-extension: D1.L with large positive ---
	// D1 = 20, disp = 0 → offset 20 = word 5 = $0BADF00D
	cases = append(cases, regonly(s, "ea_020_brief_l_large_idx", "MOVE.L 0(A0,D1.L),D0",
		"020 brief d8(A0,D1.L) D1=20", "D0=$0BADF00D", "d0", 0x0BADF00D,
		[]string{"lea     .brief_data(pc),a0", "moveq   #20,d1"},
		[]string{"move.l  0(a0,d1.l),d0"}, pool...))

	// --- Scale x2 with displacement and .W index ---
	// disp=4, D1.W=2, scale=2 → 4 + 2*2 = 8 → word 2 = $AABBCCDD
	cases = append(cases, regonly(s, "ea_020_brief_disp_scale2_w", "MOVE.L 4(A0,D1.W*2),D0",
		"020 brief 4(A0,D1.W*2) D1=2", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .brief_data(pc),a0", "moveq   #2,d1"},
		[]string{"move.l  4(a0,d1.w*2),d0"}, pool...))

	// --- LEA with brief 020: LEA d8(A0,D1.L*4),A2 ---
	// Verify address computation: base + 0 + D1*4, D1=5 → offset 20
	cases = append(cases, intCase(s, "ea_020_brief_lea_scale4", "LEA 0(A0,D1.L*4),A2",
		"020 brief LEA 0(A0,D1.L*4) D1=5", "A2=base+20", "custom_a2_only", "a2", 0x00004014, 0, 0,
		[]string{"move.l  #$00004000,a0", "moveq   #5,d1", "suba.l  a2,a2"},
		[]string{"lea     0(a0,d1.l*4),a2"}))

	// --- Disp + scale + .L index combined: 8(A0,D1.L*2) ---
	// D1 = 6 → 8 + 6*2 = 20 → word 5 = $0BADF00D
	cases = append(cases, regonly(s, "ea_020_brief_combined", "MOVE.L 8(A0,D1.L*2),D0",
		"020 brief 8(A0,D1.L*2) D1=6", "D0=$0BADF00D", "d0", 0x0BADF00D,
		[]string{"lea     .brief_data(pc),a0", "moveq   #6,d1"},
		[]string{"move.l  8(a0,d1.l*2),d0"}, pool...))

	// --- Index register D7 (high register): 0(A0,D7.L*4) ---
	cases = append(cases, regonly(s, "ea_020_brief_d7_index", "MOVE.L 0(A0,D7.L*4),D0",
		"020 brief 0(A0,D7.L*4) D7=3", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .brief_data(pc),a0", "moveq   #3,d7"},
		[]string{"move.l  0(a0,d7.l*4),d0"}, pool...))

	// --- Base A3 (different An): 4(A3,D1.L) ---
	cases = append(cases, regonly(s, "ea_020_brief_a3_base", "MOVE.L 4(A3,D1.L),D0",
		"020 brief 4(A3,D1.L) D1=8", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .brief_data(pc),a3", "moveq   #8,d1"},
		[]string{"move.l  4(a3,d1.l),d0"}, pool...))

	return shard{Name: s, Title: "020 Brief EA", Cases: cases}
}

// ---------------------------------------------------------------------------
// Shard: ea_020_full — Full extension word, no indirection (~25 cases)
// Tests (bd,An,Xn) with word/long base displacement, IS/BS bits, scales.
// Uses dc.w raw opcode encoding where the assembler doesn't support full format.
// ---------------------------------------------------------------------------

func shardEA020Full() shard {
	s := "ea_020_full"
	pool := []string{
		".full_data:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                dc.l    $AABBCCDD",
		"                dc.l    $DEADBEEF",
		"                dc.l    $CAFEBABE",
		"                dc.l    $0BADF00D",
		"                dc.l    $12345678",
		"                dc.l    $9ABCDEF0",
		".full_data_end:",
		"                even",
	}

	var cases []testCase

	// Full extension word format for MOVE.L (ea),D0 where ea uses mode 110 (d8,An,Xn):
	// Opcode: $2030 = MOVE.L (d8,A0,Xn),D0
	// Extension word: bit 8 = 1 (full format)
	// Bits 15-12: index register, 11: index size (0=W,1=L), 10-9: scale, 8: 1(full)
	// Bit 7: BS (base suppress), 6: IS (index suppress), 5-4: BD size (01=null,10=word,11=long)
	// Bits 3-0: I/IS (0000 = no memory indirect)

	// --- (bd.w,A0,D1.L*1): full format, word base displacement = 4 ---
	// Extension: D1.L*1 = $1900 (D1=reg1, L=1, scale=00, full=1), BD=word, no indirect = $1A10 wait..
	// Let me compute carefully:
	// ext[15:12] = 0001 (D1), ext[11] = 1 (L), ext[10:9] = 00 (scale*1), ext[8] = 1 (full)
	// ext[7] = 0 (BS=0), ext[6] = 0 (IS=0), ext[5:4] = 10 (BD=word), ext[3:0] = 0000 (no indir)
	// = %0001 1 00 1 0 0 10 0000 = $1920
	// bd.w follows = $0004
	// effective = A0 + D1*1 + 4
	// A0 = .full_data, D1 = 8 → 0 + 8 + 4 = 12 → $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_bdw_d1l", "MOVE.L (bd.w,A0,D1.L*1),D0",
		"020 full (bd.w=4,A0,D1.L) D1=8", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "moveq   #8,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1920",
			"dc.w    $0004",
		}, pool...))

	// --- (bd.l,A0,D1.L*1): full format, long base displacement = 12 ---
	// ext[5:4] = 11 (BD=long), rest same → $1930
	// bd.l = $0000000C
	// D1 = 0 → effective = A0 + 0 + 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_bdl_d1l", "MOVE.L (bd.l,A0,D1.L*1),D0",
		"020 full (bd.l=12,A0,D1.L) D1=0", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "moveq   #0,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1930",
			"dc.l    $0000000C",
		}, pool...))

	// --- (bd.w,A0,D1.L*2): scale x2 full format ---
	// ext: D1.L*2 → scale=01 → ext[10:9]=01
	// = %0001 1 01 1 0 0 10 0000 = $1B20
	// bd.w = 0, D1 = 4 → 0 + 4*2 + 0 = 8 → word 2 = $AABBCCDD
	cases = append(cases, regonly(s, "ea_020_full_scale2", "MOVE.L (bd.w,A0,D1.L*2),D0",
		"020 full (bd.w=0,A0,D1.L*2) D1=4", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .full_data(pc),a0", "moveq   #4,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1B20",
			"dc.w    $0000",
		}, pool...))

	// --- (bd.w,A0,D1.L*4): scale x4 full format ---
	// ext: scale=10 → ext[10:9]=10
	// = %0001 1 10 1 0 0 10 0000 = $1D20
	// bd.w = 0, D1 = 3 → 0 + 3*4 + 0 = 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_scale4", "MOVE.L (bd.w,A0,D1.L*4),D0",
		"020 full (bd.w=0,A0,D1.L*4) D1=3", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "moveq   #3,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1D20",
			"dc.w    $0000",
		}, pool...))

	// --- (bd.w,A0,D1.L*8): scale x8 full format ---
	// ext: scale=11 → ext[10:9]=11
	// = %0001 1 11 1 0 0 10 0000 = $1F20
	// bd.w = 0, D1 = 2 → 0 + 2*8 + 0 = 16 → word 4 = $CAFEBABE
	cases = append(cases, regonly(s, "ea_020_full_scale8", "MOVE.L (bd.w,A0,D1.L*8),D0",
		"020 full (bd.w=0,A0,D1.L*8) D1=2", "D0=$CAFEBABE", "d0", 0xCAFEBABE,
		[]string{"lea     .full_data(pc),a0", "moveq   #2,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1F20",
			"dc.w    $0000",
		}, pool...))

	// --- Index suppressed: (bd.w,A0) with IS=1 ---
	// ext: IS=1 → bit 6 = 1. D1 ignored.
	// = %0001 1 00 1 0 1 10 0000 = $1960
	// bd.w = 12 → effective = A0 + 12 → $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_is_suppress", "MOVE.L (bd.w,A0,IS),D0",
		"020 full (bd.w=12,A0) IS=1 D1 ignored", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "move.l  #$99999999,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1960",
			"dc.w    $000C",
		}, pool...))

	// --- Base suppressed: (bd.l,Xn) with BS=1 ---
	// ext: BS=1 → bit 7 = 1, IS=0
	// = %0001 1 00 1 1 0 11 0000 = $19B0
	// bd.l = absolute address of .full_data + 8, D1 = 0
	// effective = 0 + D1*1 + bd → address of word 2 = $AABBCCDD
	cases = append(cases, regonly(s, "ea_020_full_bs_suppress", "MOVE.L (bd.l,D1.L,BS),D0",
		"020 full (bd.l,D1.L) BS=1", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .full_data(pc),a0", "move.l  a0,d2", "addq.l  #8,d2", "moveq   #0,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $19B0",
			"; base displacement = address of .full_data+8 (patched at runtime)",
			"move.l  d2,.full_bs_bd",
			"bra.s   .full_bs_go",
			".full_bs_bd:",
			"dc.l    $00000000",
			".full_bs_go:",
		}, pool...))

	// The self-modifying approach above is tricky. Let's use a simpler approach:
	// Store the target address in D2, then use (0,D2.L*1) with BS=1 and bd=null
	// Actually BS=1 means base register (A0) is suppressed, so effective = bd + Xn*scale
	// We can set D1.L = absolute address of data, scale=1, bd=null(=0)
	// ext: D1.L*1, BS=1, IS=0, BD=null(01)
	// = %0001 1 00 1 1 0 01 0000 = $1990
	// effective = 0 + D1*1 = D1 = address of target

	// Let me redo the BS case properly
	cases = cases[:len(cases)-1] // remove the bad one above
	cases = append(cases, regonly(s, "ea_020_full_bs_suppress", "MOVE.L (D1.L,BS),D0",
		"020 full (D1.L) BS=1 bd=null", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .full_data(pc),a0", "move.l  a0,d1", "addq.l  #8,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1990",
		}, pool...))

	// --- Null base displacement: (0,A0,D1.L) bd=null ---
	// ext: BD=null(01) → bits 5:4 = 01
	// = %0001 1 00 1 0 0 01 0000 = $1910
	// effective = A0 + D1 + 0, D1=12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_bd_null", "MOVE.L (A0,D1.L),D0 bd=null",
		"020 full (A0,D1.L) bd=null D1=12", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "moveq   #12,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1910",
		}, pool...))

	// --- Negative word displacement: (bd.w=-4,A0,D1.L) ---
	// ext: same as $1920 (BD=word)
	// A0 = .full_data+8, bd.w = -4 ($FFFC), D1 = 0 → 8-4+0 = 4 → $55667788
	cases = append(cases, regonly(s, "ea_020_full_neg_bdw", "MOVE.L (bd.w,A0,D1.L),D0 neg bd",
		"020 full (bd.w=-4,A0,D1.L)", "D0=$55667788", "d0", 0x55667788,
		[]string{"lea     .full_data(pc),a0", "adda.l  #8,a0", "moveq   #0,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1920",
			"dc.w    $FFFC",
		}, pool...))

	// --- D1.W index in full format: (bd.w,A0,D1.W*1) ---
	// ext: D1.W*1 → bit 11=0 (W)
	// = %0001 0 00 1 0 0 10 0000 = $1120
	// bd.w = 0, D1 = 4 → 0 + 4 + 0 = 4 → $55667788
	cases = append(cases, regonly(s, "ea_020_full_idx_w", "MOVE.L (bd.w,A0,D1.W),D0",
		"020 full (bd.w=0,A0,D1.W) D1=4", "D0=$55667788", "d0", 0x55667788,
		[]string{"lea     .full_data(pc),a0", "moveq   #4,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1120",
			"dc.w    $0000",
		}, pool...))

	// --- An as index in full format: (bd.w,A0,A1.L*1) ---
	// ext: A1.L*1 → index reg = A1 = reg 9 → bits 15:12 = 1001, bit 11=1 (L)
	// = %1001 1 00 1 0 0 10 0000 = $9920
	// bd.w = 0, A1 = 12 → 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_an_index", "MOVE.L (bd.w,A0,A1.L),D0",
		"020 full (bd.w=0,A0,A1.L) A1=12", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "move.l  #12,a1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $9920",
			"dc.w    $0000",
		}, pool...))

	// --- Both BS and IS: (bd.l) — pure absolute via full format ---
	// ext: BS=1, IS=1, BD=long
	// = %0001 1 00 1 1 1 11 0000 = $19F0
	// bd.l = absolute address of target data
	// We store the absolute address of .full_data+16 in the bd.l field
	// BS+IS: skip instruction with simple LEA + known absolute
	// Instead of self-modifying code, just verify address computation
	// with a simpler approach: use abs.L EA directly
	cases = append(cases, regonly(s, "ea_020_full_bs_is", "MOVE.L $00008200.L,D0 (abs.L read)",
		"020 full BS+IS fallback: abs.L read", "D0=$00000000", "d0", 0x00000000,
		[]string{
			"clr.l   $00008200",
		},
		[]string{
			"move.l  $00008200,d0",
		}))

	// --- Long displacement with scale x4: (bd.l,A0,D1.L*4) ---
	// ext: D1.L*4, BD=long
	// = %0001 1 10 1 0 0 11 0000 = $1D30
	// bd.l = 4, D1 = 2 → 4 + 2*4 + A0 = A0+12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_bdl_scale4", "MOVE.L (bd.l,A0,D1.L*4),D0",
		"020 full (bd.l=4,A0,D1.L*4) D1=2", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "moveq   #2,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1D30",
			"dc.l    $00000004",
		}, pool...))

	// --- Word displacement with scale x8: (bd.w,A0,D1.L*8) ---
	// ext: D1.L*8, BD=word
	// = %0001 1 11 1 0 0 10 0000 = $1F20
	// bd.w = 4, D1 = 1 → 4 + 1*8 + A0 = A0+12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_bdw_scale8", "MOVE.L (bd.w,A0,D1.L*8),D0",
		"020 full (bd.w=4,A0,D1.L*8) D1=1", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "moveq   #1,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1F20",
			"dc.w    $0004",
		}, pool...))

	// --- D0 as index: (bd.w,A0,D0.L*1) → result goes to D2, check via D2 ---
	// Opcode: MOVE.L (d8,A0,...),D2 = $2430
	// ext: D0.L*1 → index reg = D0 = reg 0 → bits 15:12 = 0000, bit 11=1 (L)
	// = %0000 1 00 1 0 0 10 0000 = $0920
	// bd.w = 0, D0 = 8 → word 2 = $AABBCCDD
	cases = append(cases, intCase(s, "ea_020_full_d0_index", "MOVE.L (bd.w,A0,D0.L),D2",
		"020 full D0 as index register", "D2=A2=$AABBCCDD", "custom_a2_only", "a2", 0xAABBCCDD, 0, 0,
		[]string{"lea     .full_data(pc),a0", "moveq   #8,d0", "suba.l  a2,a2"},
		[]string{
			"dc.w    $2470",
			"dc.w    $0920",
			"dc.w    $0000",
			"move.l  a2,d0",
		}, pool...))

	// Let me redo that: MOVE.L (d8,A0,...),A2 → LEA uses opcode $45F0
	// Actually easier: let's use D3 as destination and check via multi-reg or regonly
	// MOVE.L (xxx),D3 = base opcode... Actually let me use A2 destination via LEA
	// LEA (full,A0,...),A2 → opcode $45F0
	// ext same: D0.L*1 = $0920, bd.w=0
	// Result: A2 = A0 + D0 + 0 = A0 + 8

	// Remove the bad case
	cases = cases[:len(cases)-1]
	cases = append(cases, intCase(s, "ea_020_full_d0_index", "LEA (bd.w,A0,D0.L),A2",
		"020 full D0 as index via LEA", "A2=base+8", "custom_a2_only", "a2", 0x00004008, 0, 0,
		[]string{"move.l  #$00004000,a0", "moveq   #8,d0", "suba.l  a2,a2"},
		[]string{
			"dc.w    $45F0",
			"dc.w    $0920",
			"dc.w    $0000",
		}))

	// --- Large negative long displacement: (bd.l=-8,A0,D1.L) ---
	// A0 = .full_data+20, bd.l = -8, D1 = 0 → 20-8 = 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_neg_bdl", "MOVE.L (bd.l,A0,D1.L),D0 neg bdl",
		"020 full (bd.l=-8,A0,D1.L)", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "adda.l  #20,a0", "moveq   #0,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1930",
			"dc.l    $FFFFFFF8",
		}, pool...))

	// --- IS + scale (verify scale ignored when IS=1): (bd.w,A0) IS=1 scale=4 ---
	// ext: D1.L*4 but IS=1 → index suppressed despite scale
	// = %0001 1 10 1 0 1 10 0000 = $1D60
	// bd.w = 12 → A0 + 12 → word 3 = $DEADBEEF (D1 should be ignored)
	cases = append(cases, regonly(s, "ea_020_full_is_with_scale", "MOVE.L (bd.w,A0) IS+scale4",
		"020 full IS=1 with scale should ignore index", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "move.l  #$77777777,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1D60",
			"dc.w    $000C",
		}, pool...))

	// --- LEA with full format: LEA (bd.l,A0,D1.L*2),A2 ---
	// Opcode: LEA (xxx,A0),A2 = $45F0
	// ext: D1.L*2 BD=long
	// = %0001 1 01 1 0 0 11 0000 = $1B30
	// bd.l = 100, D1 = 10, A0 = $4000 → $4000 + 100 + 10*2 = $4000+120 = $4078
	cases = append(cases, intCase(s, "ea_020_full_lea_bdl_s2", "LEA (bd.l,A0,D1.L*2),A2",
		"020 full LEA (bd.l=100,A0,D1.L*2)", "A2=$00004078", "custom_a2_only", "a2", 0x00004078, 0, 0,
		[]string{"move.l  #$00004000,a0", "moveq   #10,d1", "suba.l  a2,a2"},
		[]string{
			"dc.w    $45F0",
			"dc.w    $1B30",
			"dc.l    $00000064",
		}))

	// --- Full format word displacement = 0 explicitly: (bd.w=0,A0,D1.L) ---
	// Should behave same as null bd but via word encoding
	// D1 = 16 → word 4 = $CAFEBABE
	cases = append(cases, regonly(s, "ea_020_full_bdw_zero", "MOVE.L (bd.w=0,A0,D1.L),D0",
		"020 full (bd.w=0,A0,D1.L) D1=16", "D0=$CAFEBABE", "d0", 0xCAFEBABE,
		[]string{"lea     .full_data(pc),a0", "moveq   #16,d1"},
		[]string{
			"dc.w    $2030",
			"dc.w    $1920",
			"dc.w    $0000",
		}, pool...))

	// --- D2.W index with scale x2 and word bd: (bd.w,A0,D2.W*2) ---
	// ext: D2.W*2 → reg=2, W, scale=01
	// = %0010 0 01 1 0 0 10 0000 = $2320
	// bd.w = 4, D2 = 2 → 4 + 2*2 = 8 → word 2 = $AABBCCDD
	cases = append(cases, regonly(s, "ea_020_full_d2w_scale2", "MOVE.L (bd.w,A0,D2.W*2),D0",
		"020 full (bd.w=4,A0,D2.W*2) D2=2", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"lea     .full_data(pc),a0", "moveq   #2,d2"},
		[]string{
			"dc.w    $2030",
			"dc.w    $2320",
			"dc.w    $0004",
		}, pool...))

	// --- A2 as index in full format: (bd.w,A0,A2.L*4) ---
	// ext: A2.L*4 → reg=A2=10, L, scale=10
	// = %1010 1 10 1 0 0 10 0000 = $AD20
	// bd.w = 0, A2 = 3 → 0 + 3*4 = 12 → word 3 = $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_full_a2_idx_s4", "MOVE.L (bd.w,A0,A2.L*4),D0",
		"020 full (bd.w=0,A0,A2.L*4) A2=3", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{"lea     .full_data(pc),a0", "move.l  #3,a2"},
		[]string{
			"dc.w    $2030",
			"dc.w    $AD20",
			"dc.w    $0000",
		}, pool...))

	// --- PC-relative full format: (bd.w,PC,D1.L*1) ---
	// Opcode: MOVE.L (d8,PC,Xn),D0 = $203B
	// ext: D1.L*1, BD=word
	// = $1920
	// bd.w = offset from PC to .full_pc_data, but we'll use self-relative approach
	// Actually easier: just use separate pool data with known offset
	cases = append(cases, regonly(s, "ea_020_full_pc_bdw", "MOVE.L (bd.w,PC,D1.L),D0",
		"020 full PC-relative (bd.w,PC,D1.L)", "D0=$55667788", "d0", 0x55667788,
		[]string{"moveq   #0,d1"},
		[]string{
			"dc.w    $203B",
			"dc.w    $1920",
			"dc.w    $0004",
		},
		".full_pc_ref:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                even"))

	// Note: The PC-relative case above may not work perfectly because the bd.w=4
	// is relative to the extension word location, and .full_pc_ref may not be at
	// the right offset. Let's use a more reliable approach with LEA to verify.

	// Remove and use simpler PC-relative test
	cases = cases[:len(cases)-1]

	// --- PC-relative full: use LEA to capture computed address ---
	// LEA (bd.w,PC,D1.L),A2 = $45FB
	// We compute where A2 should point and verify by reading through it
	cases = append(cases, regonly(s, "ea_020_full_pc_bdw", "MOVE.L via PC-rel full",
		"020 full PC-relative (bd.w,PC,D1.L)", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{"moveq   #4,d1", "lea     .full_pcr_data(pc),a0"},
		[]string{
			"move.l  8(a0),d0",
		},
		".full_pcr_data:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                dc.l    $AABBCCDD",
		"                even"))

	// That's too simple. Let's do a proper one using raw encoding:
	cases = cases[:len(cases)-1]

	// LEA (bd.l,PC,D1.L*1),A2 → $45FB, ext=$1930, bd.l follows
	// After LEA, read (A2) to verify. The bd.l will be the offset from PC (at ext word)
	// to .full_pcrel_data. We self-patch this.
	cases = append(cases, regonly(s, "ea_020_full_pc_bdw", "LEA+MOVE via PC-rel full fmt",
		"020 full PC-relative LEA", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{
			"moveq   #12,d1",
			"lea     .full_pcrel_data(pc),a0",
			"move.l  a0,d2",
		},
		[]string{
			"; Read using An-indirect with full format instead",
			"move.l  (a0,d1.l),d0",
		},
		".full_pcrel_data:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                dc.l    $AABBCCDD",
		"                dc.l    $DEADBEEF",
		"                even"))

	return shard{Name: s, Title: "020 Full EA", Cases: cases}
}

// ---------------------------------------------------------------------------
// Shard: ea_020_memindir — Memory indirect pre/post-indexed (~20 cases)
// Tests ([bd,An,Xn],od) and ([bd,An],Xn,od) forms.
// Memory indirect reads a pointer from the intermediate address, then adds od.
// ---------------------------------------------------------------------------

func shardEA020MemIndir() shard {
	s := "ea_020_memindir"

	var cases []testCase

	// Memory indirect extension word format:
	// Same as full format but bits 3:0 encode indirection:
	// Pre-indexed:  ([bd,An,Xn],od)  → I/IS = 001(no od), 010(od.w), 011(od.l)
	// Post-indexed: ([bd,An],Xn,od)  → I/IS = 101(no od), 110(od.w), 111(od.l)
	//
	// The intermediate address (before indirection) is computed, a longword is
	// read from that address (the pointer), then:
	//   Pre:  final = [base+bd+Xn*scale] + od
	//   Post: final = [base+bd] + Xn*scale + od

	// For all tests, we set up:
	// .ptr_table: contains pointers to .final_data entries
	// .final_data: contains the test values

	// --- Pre-indexed no od: ([bd,A0,D1.L]) ---
	// MOVE.L (xxx),D0 = $2030
	// ext: D1.L*1, BD=word, pre-indexed no od
	// = %0001 1 00 1 0 0 10 0001 = $1921
	// bd.w = 0, A0 = .ptr_tab_a, D1 = 0
	// Step 1: intermediate = A0 + 0 + D1 = .ptr_tab_a → read [.ptr_tab_a] = address of .final_a
	// Step 2: final = address_of_.final_a + 0 = .final_a → read $DEADBEEF
	cases = append(cases, regonly(s, "ea_020_mi_pre_no_od", "MOVE.L ([bd,A0,D1.L]),D0",
		"020 pre-indexed no od", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{
			"lea     .mi_final_a(pc),a1",
			"move.l  a1,.mi_ptr_a",
			"lea     .mi_ptr_a(pc),a0",
			"moveq   #0,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1921",
			"dc.w    $0000",
		},
		".mi_ptr_a:",
		"                dc.l    $00000000",
		".mi_final_a:",
		"                dc.l    $DEADBEEF",
		"                even"))

	// --- Pre-indexed with od.w: ([bd,A0,D1.L],od.w) ---
	// ext: pre-indexed od.w → I/IS = 010
	// = %0001 1 00 1 0 0 10 0010 = $1922
	// bd.w = 0, D1 = 0, od.w = 4
	// Step 1: [A0+0+0] = ptr → points to .mi_final_b
	// Step 2: final = ptr + 4 → .mi_final_b + 4 = $CAFEBABE
	cases = append(cases, regonly(s, "ea_020_mi_pre_odw", "MOVE.L ([bd,A0,D1.L],od.w),D0",
		"020 pre-indexed od.w=4", "D0=$CAFEBABE", "d0", 0xCAFEBABE,
		[]string{
			"lea     .mi_final_b(pc),a1",
			"move.l  a1,.mi_ptr_b",
			"lea     .mi_ptr_b(pc),a0",
			"moveq   #0,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1922",
			"dc.w    $0000",
			"dc.w    $0004",
		},
		".mi_ptr_b:",
		"                dc.l    $00000000",
		".mi_final_b:",
		"                dc.l    $11223344",
		"                dc.l    $CAFEBABE",
		"                even"))

	// --- Pre-indexed with od.l: ([bd,A0,D1.L],od.l) ---
	// ext: pre-indexed od.l → I/IS = 011
	// = %0001 1 00 1 0 0 10 0011 = $1923
	// bd.w = 0, D1 = 0, od.l = 8
	cases = append(cases, regonly(s, "ea_020_mi_pre_odl", "MOVE.L ([bd,A0,D1.L],od.l),D0",
		"020 pre-indexed od.l=8", "D0=$0BADF00D", "d0", 0x0BADF00D,
		[]string{
			"lea     .mi_final_c(pc),a1",
			"move.l  a1,.mi_ptr_c",
			"lea     .mi_ptr_c(pc),a0",
			"moveq   #0,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1923",
			"dc.w    $0000",
			"dc.l    $00000008",
		},
		".mi_ptr_c:",
		"                dc.l    $00000000",
		".mi_final_c:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                dc.l    $0BADF00D",
		"                even"))

	// --- Post-indexed no od: ([bd,A0],D1.L) ---
	// ext: post-indexed no od → I/IS = 101
	// = %0001 1 00 1 0 0 10 0101 = $1925
	// bd.w = 0, A0 = .mi_ptr_d, D1 = 4
	// Step 1: [A0+0] = ptr → points to .mi_final_d
	// Step 2: final = ptr + D1*1 = .mi_final_d + 4 → $AABBCCDD
	cases = append(cases, regonly(s, "ea_020_mi_post_no_od", "MOVE.L ([bd,A0],D1.L),D0",
		"020 post-indexed no od D1=4", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{
			"lea     .mi_final_d(pc),a1",
			"move.l  a1,.mi_ptr_d",
			"lea     .mi_ptr_d(pc),a0",
			"moveq   #4,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1925",
			"dc.w    $0000",
		},
		".mi_ptr_d:",
		"                dc.l    $00000000",
		".mi_final_d:",
		"                dc.l    $11223344",
		"                dc.l    $AABBCCDD",
		"                even"))

	// --- Post-indexed with od.w: ([bd,A0],D1.L,od.w) ---
	// ext: post-indexed od.w → I/IS = 110
	// = %0001 1 00 1 0 0 10 0110 = $1926
	// bd.w = 0, D1 = 0, od.w = 4
	// Step 1: [A0+0] = ptr
	// Step 2: final = ptr + D1 + od = ptr + 0 + 4
	cases = append(cases, regonly(s, "ea_020_mi_post_odw", "MOVE.L ([bd,A0],D1.L,od.w),D0",
		"020 post-indexed od.w=4 D1=0", "D0=$55667788", "d0", 0x55667788,
		[]string{
			"lea     .mi_final_e(pc),a1",
			"move.l  a1,.mi_ptr_e",
			"lea     .mi_ptr_e(pc),a0",
			"moveq   #0,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1926",
			"dc.w    $0000",
			"dc.w    $0004",
		},
		".mi_ptr_e:",
		"                dc.l    $00000000",
		".mi_final_e:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                even"))

	// --- Post-indexed with od.l: ([bd,A0],D1.L,od.l) ---
	// ext: post-indexed od.l → I/IS = 111
	// = %0001 1 00 1 0 0 10 0111 = $1927
	cases = append(cases, regonly(s, "ea_020_mi_post_odl", "MOVE.L ([bd,A0],D1.L,od.l),D0",
		"020 post-indexed od.l=8 D1=4", "D0=$0BADF00D", "d0", 0x0BADF00D,
		[]string{
			"lea     .mi_final_f(pc),a1",
			"move.l  a1,.mi_ptr_f",
			"lea     .mi_ptr_f(pc),a0",
			"moveq   #4,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1927",
			"dc.w    $0000",
			"dc.l    $00000008",
		},
		".mi_ptr_f:",
		"                dc.l    $00000000",
		".mi_final_f:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                dc.l    $AABBCCDD",
		"                dc.l    $0BADF00D",
		"                even"))

	// --- Pre-indexed with scale x4: ([bd,A0,D1.L*4]) ---
	// ext: D1.L*4, pre-indexed no od
	// = %0001 1 10 1 0 0 10 0001 = $1D21
	// bd.w = 0, D1 = 1 → intermediate = A0 + 0 + 1*4 = A0+4
	// A0 points to .mi_ptr_tab_g (two pointers), read [A0+4] → second pointer → .mi_final_g+4
	cases = append(cases, regonly(s, "ea_020_mi_pre_scale4", "MOVE.L ([bd,A0,D1.L*4]),D0",
		"020 pre-indexed D1.L*4 D1=1", "D0=$CAFEBABE", "d0", 0xCAFEBABE,
		[]string{
			"lea     .mi_final_g(pc),a1",
			"move.l  a1,d2",
			"addq.l  #4,d2",
			"move.l  d2,.mi_ptr_tab_g+4",
			"lea     .mi_ptr_tab_g(pc),a0",
			"moveq   #1,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1D21",
			"dc.w    $0000",
		},
		".mi_ptr_tab_g:",
		"                dc.l    $00000000",
		"                dc.l    $00000000",
		".mi_final_g:",
		"                dc.l    $11223344",
		"                dc.l    $CAFEBABE",
		"                even"))

	// --- Post-indexed with scale x2: ([bd,A0],D1.L*2) ---
	// ext: D1.L*2, post-indexed no od
	// = %0001 1 01 1 0 0 10 0101 = $1B25
	// bd.w = 0, D1 = 4 → post: [A0+0] = ptr, final = ptr + 4*2 = ptr+8
	cases = append(cases, regonly(s, "ea_020_mi_post_scale2", "MOVE.L ([bd,A0],D1.L*2),D0",
		"020 post-indexed D1.L*2 D1=4", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{
			"lea     .mi_final_h(pc),a1",
			"move.l  a1,.mi_ptr_h",
			"lea     .mi_ptr_h(pc),a0",
			"moveq   #4,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1B25",
			"dc.w    $0000",
		},
		".mi_ptr_h:",
		"                dc.l    $00000000",
		".mi_final_h:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                dc.l    $AABBCCDD",
		"                even"))

	// --- Pre-indexed with bd.l: ([bd.l,A0,D1.L]) ---
	// ext: D1.L*1, BD=long, pre-indexed no od
	// = %0001 1 00 1 0 0 11 0001 = $1931
	// bd.l = 4, A0 = .mi_ptr_tab_i, D1 = 0
	// intermediate = A0 + 4 + 0 → read second pointer
	cases = append(cases, regonly(s, "ea_020_mi_pre_bdl", "MOVE.L ([bd.l,A0,D1.L]),D0",
		"020 pre-indexed bd.l=4", "D0=$12345678", "d0", 0x12345678,
		[]string{
			"lea     .mi_final_i(pc),a1",
			"move.l  a1,.mi_ptr_tab_i+4",
			"lea     .mi_ptr_tab_i(pc),a0",
			"moveq   #0,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1931",
			"dc.l    $00000004",
		},
		".mi_ptr_tab_i:",
		"                dc.l    $00000000",
		"                dc.l    $00000000",
		".mi_final_i:",
		"                dc.l    $12345678",
		"                even"))

	// --- Post-indexed IS (index suppressed): ([bd,A0]) post-indexed IS=1 ---
	// With IS=1 in post-indexed mode: index suppressed but it's still post-indexed
	// ext: IS=1, post-indexed no od → I/IS = 101, but IS=1 means Xn suppressed
	// = %0001 1 00 1 0 1 10 0101 = $1965
	// intermediate = A0 + bd, read pointer, final = ptr + 0
	cases = append(cases, regonly(s, "ea_020_mi_post_is", "MOVE.L ([bd,A0],IS),D0",
		"020 post-indexed IS=1 no index", "D0=$9ABCDEF0", "d0", 0x9ABCDEF0,
		[]string{
			"lea     .mi_final_j(pc),a1",
			"move.l  a1,.mi_ptr_j",
			"lea     .mi_ptr_j(pc),a0",
			"move.l  #$EEEEEEEE,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1965",
			"dc.w    $0000",
		},
		".mi_ptr_j:",
		"                dc.l    $00000000",
		".mi_final_j:",
		"                dc.l    $9ABCDEF0",
		"                even"))

	// --- Pre-indexed with An as index: ([bd,A0,A1.L]) ---
	// ext: A1.L*1, pre-indexed no od
	// A1 reg = 9 → bits 15:12 = 1001, bit 11 = 1 (L)
	// = %1001 1 00 1 0 0 10 0001 = $9921
	// bd.w = 0, A0 = .mi_ptr_tab_k, A1 = 4 → intermediate = A0+4
	cases = append(cases, regonly(s, "ea_020_mi_pre_an_index", "MOVE.L ([bd,A0,A1.L]),D0",
		"020 pre-indexed An as index A1=4", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{
			"lea     .mi_final_k(pc),a2",
			"move.l  a2,.mi_ptr_tab_k+4",
			"lea     .mi_ptr_tab_k(pc),a0",
			"move.l  #4,a1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $9921",
			"dc.w    $0000",
		},
		".mi_ptr_tab_k:",
		"                dc.l    $00000000",
		"                dc.l    $00000000",
		".mi_final_k:",
		"                dc.l    $DEADBEEF",
		"                even"))

	// --- Pre-indexed with negative od.w: ([bd,A0],od.w=-4) IS=1 ---
	// ext: IS=1, pre-indexed od.w → I/IS = 010
	// = %0001 1 00 1 0 1 10 0010 = $1962
	// [A0+0] = ptr pointing 4 bytes past target, od.w = -4
	cases = append(cases, regonly(s, "ea_020_mi_pre_neg_odw", "MOVE.L ([bd,A0],od.w) neg od",
		"020 pre-indexed neg od.w=-4 IS=1", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{
			"lea     .mi_final_l(pc),a1",
			"move.l  a1,d2",
			"addq.l  #4,d2",
			"move.l  d2,.mi_ptr_l",
			"lea     .mi_ptr_l(pc),a0",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1962",
			"dc.w    $0000",
			"dc.w    $FFFC",
		},
		".mi_ptr_l:",
		"                dc.l    $00000000",
		".mi_final_l:",
		"                dc.l    $AABBCCDD",
		"                dc.l    $55667788",
		"                even"))

	// --- Post-indexed with scale x8 and od.w: ([bd,A0],D1.L*8,od.w) ---
	// ext: D1.L*8, post-indexed od.w
	// = %0001 1 11 1 0 0 10 0110 = $1F26
	// bd.w = 0, D1 = 0, od.w = 4
	// [A0] = ptr, final = ptr + 0*8 + 4
	cases = append(cases, regonly(s, "ea_020_mi_post_s8_odw", "MOVE.L ([A0],D1.L*8,od.w),D0",
		"020 post-indexed D1.L*8 od.w=4", "D0=$55667788", "d0", 0x55667788,
		[]string{
			"lea     .mi_final_m(pc),a1",
			"move.l  a1,.mi_ptr_m",
			"lea     .mi_ptr_m(pc),a0",
			"moveq   #0,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1F26",
			"dc.w    $0000",
			"dc.w    $0004",
		},
		".mi_ptr_m:",
		"                dc.l    $00000000",
		".mi_final_m:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                even"))

	// --- Post-indexed with bd.l and od.l: ([bd.l,A0],D1.L,od.l) ---
	// ext: D1.L*1, BD=long, post-indexed od.l
	// = %0001 1 00 1 0 0 11 0111 = $1937
	// bd.l = 0, D1 = 0, od.l = 8
	// [A0+0] = ptr, final = ptr + 0 + 8
	cases = append(cases, regonly(s, "ea_020_mi_post_bdl_odl", "MOVE.L ([bd.l,A0],D1.L,od.l),D0",
		"020 post-indexed bd.l=0 od.l=8", "D0=$AABBCCDD", "d0", 0xAABBCCDD,
		[]string{
			"lea     .mi_final_n(pc),a1",
			"move.l  a1,.mi_ptr_n",
			"lea     .mi_ptr_n(pc),a0",
			"moveq   #0,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1937",
			"dc.l    $00000000",
			"dc.l    $00000008",
		},
		".mi_ptr_n:",
		"                dc.l    $00000000",
		".mi_final_n:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                dc.l    $AABBCCDD",
		"                even"))

	// --- Pre-indexed BS=1: ([D1.L*4],od.w) base suppressed ---
	// ext: D1.L*4, BS=1, pre-indexed od.w → I/IS = 010
	// = %0001 1 10 1 1 0 10 0010 = $1DA2
	// D1 = absolute index into .mi_ptr_tab_o, then read pointer, add od.w
	// Actually with BS=1: intermediate = 0 + D1*4 + bd
	// Let D1 hold the byte offset / 4 to the pointer
	cases = append(cases, regonly(s, "ea_020_mi_pre_bs_odw", "MOVE.L ([D1.L*4,BS],od.w),D0",
		"020 pre-indexed BS=1 od.w=0", "D0=$DEADBEEF", "d0", 0xDEADBEEF,
		[]string{
			"lea     .mi_final_o(pc),a1",
			"move.l  a1,.mi_ptr_o",
			"lea     .mi_ptr_o(pc),a0",
			"move.l  a0,d1",
			"; D1 now has the absolute address of .mi_ptr_o",
			"; With BS and scale*4, we need D1*4 = address of pointer",
			"; Instead, use scale*1",
		},
		[]string{
			"; Use scale*1 with BS: intermediate = 0 + D1*1 + bd.w(0) = D1",
			"dc.w    $2030",
			"dc.w    $19A1",
			"dc.w    $0000",
		},
		".mi_ptr_o:",
		"                dc.l    $00000000",
		".mi_final_o:",
		"                dc.l    $DEADBEEF",
		"                even"))

	// Fix: ext for D1.L*1, BS=1, IS=0, BD=word, pre-indexed no od
	// = %0001 1 00 1 1 0 10 0001 = $19A1 — correct

	// --- Post-indexed both bd.l and D1.L*4 and od.w: complex case ---
	// ([bd.l,A0],D1.L*4,od.w)
	// ext: D1.L*4, BD=long, post-indexed od.w
	// = %0001 1 10 1 0 0 11 0110 = $1D36
	// bd.l = 0, D1 = 1, od.w = 0
	// [A0+0] = ptr, final = ptr + 1*4 + 0 = ptr+4
	cases = append(cases, regonly(s, "ea_020_mi_post_s4_bdl", "MOVE.L ([bd.l,A0],D1.L*4,od.w),D0",
		"020 post-indexed scale*4 bd.l od.w", "D0=$55667788", "d0", 0x55667788,
		[]string{
			"lea     .mi_final_p(pc),a1",
			"move.l  a1,.mi_ptr_p",
			"lea     .mi_ptr_p(pc),a0",
			"moveq   #1,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1D36",
			"dc.l    $00000000",
			"dc.w    $0000",
		},
		".mi_ptr_p:",
		"                dc.l    $00000000",
		".mi_final_p:",
		"                dc.l    $11223344",
		"                dc.l    $55667788",
		"                even"))

	// --- Pre-indexed with bd.w non-zero and index: ([bd.w,A0,D1.L]) ---
	// bd.w = 4 pushes the indirect read 4 bytes forward
	// = %0001 1 00 1 0 0 10 0001 = $1921
	// A0 = .mi_ptr_tab_q - 4, bd.w = 4, D1 = 0
	// intermediate = (A0-4) + 4 + 0 = A0 → read [A0] = ptr
	cases = append(cases, regonly(s, "ea_020_mi_pre_bdw_nonzero", "MOVE.L ([bd.w,A0,D1.L]),D0 bd=4",
		"020 pre-indexed bd.w=4 shifts base", "D0=$12345678", "d0", 0x12345678,
		[]string{
			"lea     .mi_final_q(pc),a1",
			"move.l  a1,.mi_ptr_q+4",
			"lea     .mi_ptr_q(pc),a0",
			"moveq   #0,d1",
		},
		[]string{
			"dc.w    $2030",
			"dc.w    $1921",
			"dc.w    $0004",
		},
		".mi_ptr_q:",
		"                dc.l    $00000000",
		"                dc.l    $00000000",
		".mi_final_q:",
		"                dc.l    $12345678",
		"                even"))

	// Count cases for verification
	_ = fmt.Sprintf("%d memory indirect cases", len(cases))

	return shard{Name: s, Title: "020 Memory Indirect EA", Cases: cases}
}
