package main

// Phase 6: Bit Fields (~50 cases)
// Shards: bf_reg, bf_mem

func buildPhase6Shards() []shard {
	return []shard{
		shardBFReg(),
		shardBFMem(),
	}
}

func shardBFReg() shard {
	s := "bf_reg"
	var cases []testCase

	// -----------------------------------------------------------------------
	// BFTST register
	// -----------------------------------------------------------------------

	// BFTST D0{0:8} with D0=$FF000000 → field=$FF → N=1, Z=0
	// ext: Dn=0(unused), Do=0, offset=0, Dw=0, width=8 → $0008
	cases = append(cases, intCase(s, "bf_reg_bftst_0_8_ff", "BFTST D0{0:8}",
		"D0=$FF000000 offset=0 width=8", "SR: N=1 Z=0",
		"custom_sr_only", "", 0, 0x000C, 0x0008,
		[]string{"move.l  #$FF000000,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E8C0,$0008"}))

	// BFTST D0{0:32} with D0=$00000000 → Z=1
	// ext: offset=0, width=0(=32) → $0000
	cases = append(cases, intCase(s, "bf_reg_bftst_0_32_zero", "BFTST D0{0:32}",
		"D0=$00000000 offset=0 width=0(=32)", "SR: Z=1",
		"custom_sr_only", "", 0, 0x000C, 0x0004,
		[]string{"moveq   #0,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E8C0,$0000"}))

	// BFTST D0{24:8} with D0=$000000FF → field=$FF → N=1
	// ext: offset=24, width=8 → offset in bits 10:6 = 24 = 11000, width in 4:0 = 01000
	// = 0000_0_11000_0_01000 = $0608
	cases = append(cases, intCase(s, "bf_reg_bftst_24_8_ff", "BFTST D0{24:8}",
		"D0=$000000FF offset=24 width=8", "SR: N=1 Z=0",
		"custom_sr_only", "", 0, 0x000C, 0x0008,
		[]string{"move.l  #$000000FF,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E8C0,$0608"}))

	// BFTST D0{8:16} with D0=$00FFFF00 → field=$FFFF → N=1
	// ext: offset=8=01000 in 10:6, width=16=10000 in 4:0
	// = 0000_0_01000_0_10000 = $0210
	cases = append(cases, intCase(s, "bf_reg_bftst_8_16_ffff", "BFTST D0{8:16}",
		"D0=$00FFFF00 offset=8 width=16", "SR: N=1 Z=0",
		"custom_sr_only", "", 0, 0x000C, 0x0008,
		[]string{"move.l  #$00FFFF00,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E8C0,$0210"}))

	// -----------------------------------------------------------------------
	// BFEXTU register
	// -----------------------------------------------------------------------

	// BFEXTU D0{0:8},D1 with D0=$AB000000 → D1=$000000AB
	// ext: bits 15-12=D1(0001), Do=0, offset=0, Dw=0, width=8
	// = 0001_0_00000_0_01000 = $1008
	cases = append(cases, regsr(s, "bf_reg_bfextu_0_8", "BFEXTU D0{0:8},D1",
		"D0=$AB000000 offset=0 width=8", "D1=$000000AB SR: N=1",
		"d1", 0x000000AB, 0x000C, 0x0008,
		[]string{"move.l  #$AB000000,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9C0,$1008"}))

	// BFEXTU D0{16:16},D1 with D0=$1234ABCD → D1=$0000ABCD
	// ext: D1=0001, offset=16=10000 in 10:6, width=16=10000 in 4:0
	// = 0001_0_10000_0_10000 = $1410
	cases = append(cases, regsr(s, "bf_reg_bfextu_16_16", "BFEXTU D0{16:16},D1",
		"D0=$1234ABCD offset=16 width=16", "D1=$0000ABCD SR: N=1",
		"d1", 0x0000ABCD, 0x000C, 0x0008,
		[]string{"move.l  #$1234ABCD,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9C0,$1410"}))

	// BFEXTU D0{0:32},D1 with D0=$12345678 → D1=$12345678 (width=0 means 32)
	// ext: D1=0001, offset=0, width=0 → $1000
	cases = append(cases, regsr(s, "bf_reg_bfextu_0_32", "BFEXTU D0{0:32},D1",
		"D0=$12345678 offset=0 width=0(=32)", "D1=$12345678",
		"d1", 0x12345678, 0x000C, 0x0000,
		[]string{"move.l  #$12345678,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9C0,$1000"}))

	// BFEXTU D0{0:1},D1 with D0=$80000000 → D1=$00000001 (width=1)
	// ext: D1=0001, offset=0, width=1 → $1001
	cases = append(cases, regsr(s, "bf_reg_bfextu_0_1", "BFEXTU D0{0:1},D1",
		"D0=$80000000 offset=0 width=1", "D1=$00000001 SR: N=1",
		"d1", 0x00000001, 0x000C, 0x0008,
		[]string{"move.l  #$80000000,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9C0,$1001"}))

	// -----------------------------------------------------------------------
	// BFEXTS register
	// -----------------------------------------------------------------------

	// BFEXTS D0{0:8},D1 with D0=$80000000 → field=$80 → D1=$FFFFFF80
	// opcode=$EBC0, ext: D1=0001, offset=0, width=8 → $1008
	cases = append(cases, regsr(s, "bf_reg_bfexts_0_8_neg", "BFEXTS D0{0:8},D1",
		"D0=$80000000 offset=0 width=8", "D1=$FFFFFF80 SR: N=1",
		"d1", 0xFFFFFF80, 0x000C, 0x0008,
		[]string{"move.l  #$80000000,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EBC0,$1008"}))

	// BFEXTS D0{0:8},D1 with D0=$7F000000 → field=$7F → D1=$0000007F
	cases = append(cases, regsr(s, "bf_reg_bfexts_0_8_pos", "BFEXTS D0{0:8},D1",
		"D0=$7F000000 offset=0 width=8", "D1=$0000007F",
		"d1", 0x0000007F, 0x000C, 0x0000,
		[]string{"move.l  #$7F000000,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EBC0,$1008"}))

	// BFEXTS D0{16:8},D1 with D0=$0000FF00 → field=$FF → D1=$FFFFFFFF
	// ext: D1=0001, offset=16=10000, width=8=01000
	// = 0001_0_10000_0_01000 = $1408
	cases = append(cases, regsr(s, "bf_reg_bfexts_16_8", "BFEXTS D0{16:8},D1",
		"D0=$0000FF00 offset=16 width=8", "D1=$FFFFFFFF SR: N=1",
		"d1", 0xFFFFFFFF, 0x000C, 0x0008,
		[]string{"move.l  #$0000FF00,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EBC0,$1408"}))

	// -----------------------------------------------------------------------
	// BFSET register
	// -----------------------------------------------------------------------

	// BFSET D0{8:8} with D0=$00000000 → D0=$00FF0000
	// opcode=$EEC0, ext: offset=8=00001 in 10:6... wait, offset=8:
	// bits 10:6 encode offset: 8 = 00010 (5 bits, but only 5 bits 10:6)
	// Actually offset field is bits 10:6 (5 bits): 8 = 01000
	// Wait: bits 10,9,8,7,6 for offset. 8 in binary = 01000.
	// = 0000_0_01000_0_01000 = $0208
	cases = append(cases, regsr(s, "bf_reg_bfset_8_8", "BFSET D0{8:8}",
		"D0=$00000000 offset=8 width=8", "D0=$00FF0000 SR: Z=1",
		"d0", 0x00FF0000, 0x000C, 0x0004,
		[]string{"moveq   #0,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EEC0,$0208"}))

	// BFSET D0{0:32} with D0=$00000000 → D0=$FFFFFFFF
	cases = append(cases, regsr(s, "bf_reg_bfset_0_32", "BFSET D0{0:32}",
		"D0=$00000000 offset=0 width=0(=32)", "D0=$FFFFFFFF SR: Z=1",
		"d0", 0xFFFFFFFF, 0x000C, 0x0004,
		[]string{"moveq   #0,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EEC0,$0000"}))

	// -----------------------------------------------------------------------
	// BFCHG register
	// -----------------------------------------------------------------------

	// BFCHG D0{0:32} with D0=$FFFFFFFF → D0=$00000000
	// opcode=$EAC0, ext: offset=0, width=0(=32) → $0000
	cases = append(cases, regsr(s, "bf_reg_bfchg_0_32", "BFCHG D0{0:32}",
		"D0=$FFFFFFFF offset=0 width=0(=32)", "D0=$00000000 SR: N=1",
		"d0", 0x00000000, 0x000C, 0x0008,
		[]string{"move.l  #$FFFFFFFF,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EAC0,$0000"}))

	// BFCHG D0{0:8} with D0=$AA000000 → complement top 8 → D0=$55000000
	// ext: offset=0, width=8 → $0008
	cases = append(cases, regsr(s, "bf_reg_bfchg_0_8", "BFCHG D0{0:8}",
		"D0=$AA000000 offset=0 width=8", "D0=$55000000 SR: N=1",
		"d0", 0x55000000, 0x000C, 0x0008,
		[]string{"move.l  #$AA000000,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EAC0,$0008"}))

	// -----------------------------------------------------------------------
	// BFCLR register
	// -----------------------------------------------------------------------

	// BFCLR D0{0:8} with D0=$FF123456 → D0=$00123456
	// opcode=$ECC0, ext: offset=0, width=8 → $0008
	cases = append(cases, regsr(s, "bf_reg_bfclr_0_8", "BFCLR D0{0:8}",
		"D0=$FF123456 offset=0 width=8", "D0=$00123456 SR: N=1",
		"d0", 0x00123456, 0x000C, 0x0008,
		[]string{"move.l  #$FF123456,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $ECC0,$0008"}))

	// BFCLR D0{16:16} with D0=$1234FFFF → D0=$12340000
	// ext: offset=16=10000 in 10:6, width=16=10000 in 4:0
	// = 0000_0_10000_0_10000 = $0410
	cases = append(cases, regsr(s, "bf_reg_bfclr_16_16", "BFCLR D0{16:16}",
		"D0=$1234FFFF offset=16 width=16", "D0=$12340000 SR: N=1",
		"d0", 0x12340000, 0x000C, 0x0008,
		[]string{"move.l  #$1234FFFF,d0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $ECC0,$0410"}))

	// -----------------------------------------------------------------------
	// BFFFO register
	// -----------------------------------------------------------------------

	// BFFFO D0{0:32},D1 with D0=$00000000 → D1=32 (offset+width when no bit found)
	// opcode=$EDC0, ext: D1=0001, offset=0, width=0(=32) → $1000
	cases = append(cases, testCase{
		ID: "bf_reg_bfffo_zero", Shard: s, Kind: kindInt,
		Name:        "BFFFO D0{0:32},D1 all-zero",
		Input:       "D0=$00000000 offset=0 width=0(=32)",
		Expected:    "D1=$00000020 SR: Z=1",
		ActualMode:  "custom_d1_sr",
		ExpectReg:   "d1",
		ExpectValue: 0x00000020,
		SRMask:      0x000C,
		ExpectSR:    0x0004,
		Setup:       []string{"moveq   #0,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		Body:        []string{"dc.w    $EDC0,$1000"},
	})

	// BFFFO D0{0:32},D1 with D0=$FFFFFFFF → D1=0 (first one at position 0)
	cases = append(cases, testCase{
		ID: "bf_reg_bfffo_allones", Shard: s, Kind: kindInt,
		Name:        "BFFFO D0{0:32},D1 all-ones",
		Input:       "D0=$FFFFFFFF offset=0 width=0(=32)",
		Expected:    "D1=$00000000 SR: N=1",
		ActualMode:  "custom_d1_sr",
		ExpectReg:   "d1",
		ExpectValue: 0x00000000,
		SRMask:      0x000C,
		ExpectSR:    0x0008,
		Setup:       []string{"move.l  #$FFFFFFFF,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		Body:        []string{"dc.w    $EDC0,$1000"},
	})

	// BFFFO D0{0:32},D1 with D0=$00008000 → first one at bit 16 → D1=16
	cases = append(cases, testCase{
		ID: "bf_reg_bfffo_mid", Shard: s, Kind: kindInt,
		Name:        "BFFFO D0{0:32},D1 bit 16",
		Input:       "D0=$00008000 offset=0 width=0(=32)",
		Expected:    "D1=$00000010",
		ActualMode:  "custom_d1_sr",
		ExpectReg:   "d1",
		ExpectValue: 0x00000010,
		SRMask:      0x000C,
		ExpectSR:    0x0000,
		Setup:       []string{"move.l  #$00008000,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		Body:        []string{"dc.w    $EDC0,$1000"},
	})

	// BFFFO D0{8:16},D1 with D0=$00800000 → field starts at offset 8, first one at bit 0 of field → D1=8
	// ext: D1=0001, offset=8=00010 in 10:6... wait, 8 in 5 bits is 01000
	// bits 10:6 = 01000 → $1000 | (01000 << 6) | 10000
	// = 0001_0_01000_0_10000 = $1210
	cases = append(cases, testCase{
		ID: "bf_reg_bfffo_offset8", Shard: s, Kind: kindInt,
		Name:        "BFFFO D0{8:16},D1",
		Input:       "D0=$00800000 offset=8 width=16",
		Expected:    "D1=$00000008",
		ActualMode:  "custom_d1_sr",
		ExpectReg:   "d1",
		ExpectValue: 0x00000008,
		SRMask:      0x000C,
		ExpectSR:    0x0008,
		Setup:       []string{"move.l  #$00800000,d0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		Body:        []string{"dc.w    $EDC0,$1210"},
	})

	// -----------------------------------------------------------------------
	// BFINS register
	// -----------------------------------------------------------------------

	// BFINS D1,D0{0:32} with D1=$12345678, D0=$00000000 → D0=$12345678
	// opcode=$EFC0, ext: D1=0001, offset=0, width=0(=32) → $1000
	cases = append(cases, regsr(s, "bf_reg_bfins_0_32", "BFINS D1,D0{0:32}",
		"D1=$12345678 D0=$00000000 offset=0 width=0(=32)", "D0=$12345678",
		"d0", 0x12345678, 0x000C, 0x0000,
		[]string{"moveq   #0,d0", "move.l  #$12345678,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EFC0,$1000"}))

	// BFINS D1,D0{0:8} with D1=$000000AB, D0=$00000000 → D0=$AB000000
	// ext: D1=0001, offset=0, width=8 → $1008
	cases = append(cases, regsr(s, "bf_reg_bfins_0_8", "BFINS D1,D0{0:8}",
		"D1=$000000AB D0=$00000000 offset=0 width=8", "D0=$AB000000 SR: N=1",
		"d0", 0xAB000000, 0x000C, 0x0008,
		[]string{"moveq   #0,d0", "move.l  #$000000AB,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EFC0,$1008"}))

	// BFINS D1,D0{16:8} with D1=$000000CD, D0=$12340078 → D0=$1234CD78
	// ext: D1=0001, offset=16=10000 in 10:6, width=8=01000
	// = 0001_0_10000_0_01000 = $1408
	cases = append(cases, regsr(s, "bf_reg_bfins_16_8", "BFINS D1,D0{16:8}",
		"D1=$000000CD D0=$12340078 offset=16 width=8", "D0=$1234CD78 SR: N=1",
		"d0", 0x1234CD78, 0x000C, 0x0008,
		[]string{"move.l  #$12340078,d0", "move.l  #$000000CD,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EFC0,$1408"}))

	// -----------------------------------------------------------------------
	// Dynamic offset/width via registers
	// -----------------------------------------------------------------------

	// BFEXTU D0{D2:D3},D1 with D0=$AB000000, D2=0, D3=8 → D1=$000000AB
	// ext: D1=0001, Do=1(reg), bits 10:6=D2(00010), Dw=1(reg), bits 4:0=D3(00011)
	// = 0001_1_00010_1_00011 = $18A3
	cases = append(cases, regsr(s, "bf_reg_bfextu_dyn", "BFEXTU D0{D2:D3},D1",
		"D0=$AB000000 D2=0(offset) D3=8(width)", "D1=$000000AB SR: N=1",
		"d1", 0x000000AB, 0x000C, 0x0008,
		[]string{"move.l  #$AB000000,d0", "moveq   #0,d1", "moveq   #0,d2", "moveq   #8,d3", "moveq   #0,d4", "move.w  d4,ccr"},
		[]string{"dc.w    $E9C0,$18A3"}))

	// Dynamic offset with upper bits set: D2=$FFFFFF05, offset should be 5 (only bits 4:0 matter)
	// BFEXTU D0{D2:8},D1 with D0=$00F80000 (bit 5..12 = $F8) → extract at offset 5, width 8 → $7C
	// Wait: offset 5, width 8 from D0=$00F80000
	// D0 bits: 0000_0000_1111_1000_0000_0000_0000_0000
	// At offset 5, 8 bits: bits 5..12 = 00001111 = $0F... let me think again.
	// Bit 0 is MSB. So D0=$00F80000 = 0000_0000_1111_1000_0000_0000_0000_0000
	// offset 5 means starting at bit position 5 (from MSB): bits 5,6,7,8,9,10,11,12
	// = 0,0,0,1,1,1,1,1 = $1F... wait no.
	// Actually for register bit fields, the bit numbering starts from MSB as bit 0.
	// D0 = $00F80000 in binary: 00000000 11111000 00000000 00000000
	// offset=5, width=8: extract bits 5 through 12 (inclusive)
	// bit5=0, bit6=0, bit7=0, bit8=1, bit9=1, bit10=1, bit11=1, bit12=1 = 00011111 = $1F
	// ext for BFEXTU D0{D2:8},D1: D1=0001, Do=1, reg=D2(010), Dw=0, width=8(01000)
	// = 0001_1_00010_0_01000 = $1888
	cases = append(cases, regsr(s, "bf_reg_bfextu_dyn_offset_mask", "BFEXTU D0{D2:8},D1 upper bits",
		"D0=$00F80000 D2=$FFFFFF05(offset=5) width=8", "D1=$0000001F",
		"d1", 0x0000001F, 0x000C, 0x0000,
		[]string{"move.l  #$00F80000,d0", "moveq   #0,d1", "move.l  #$FFFFFF05,d2", "moveq   #0,d4", "move.w  d4,ccr"},
		[]string{"dc.w    $E9C0,$1888"}))

	// Offset wraps for register: D2=33 → effective offset = 33 mod 32 = 1
	// BFEXTU D0{D2:8},D1 with D0=$40000000, D2=33
	// offset=1, width=8: bits 1..8 = 10000000 = $80
	// ext: same encoding $1888
	cases = append(cases, regsr(s, "bf_reg_bfextu_dyn_offset_wrap", "BFEXTU D0{D2:8},D1 wrap",
		"D0=$40000000 D2=33(offset wraps to 1) width=8", "D1=$00000080 SR: N=1",
		"d1", 0x00000080, 0x000C, 0x0008,
		[]string{"move.l  #$40000000,d0", "moveq   #0,d1", "moveq   #33,d2", "moveq   #0,d4", "move.w  d4,ccr"},
		[]string{"dc.w    $E9C0,$1888"}))

	// Dynamic width: D3=0 means 32
	// BFEXTU D0{0:D3},D1 with D0=$ABCDEF01, D3=0 → extracts all 32 bits → D1=$ABCDEF01
	// ext: D1=0001, Do=0, offset=0, Dw=1, reg=D3(011)
	// = 0001_0_00000_1_00011 = $1023
	cases = append(cases, regsr(s, "bf_reg_bfextu_dyn_width_zero", "BFEXTU D0{0:D3},D1 width=0(32)",
		"D0=$ABCDEF01 D3=0(width=32)", "D1=$ABCDEF01 SR: N=1",
		"d1", 0xABCDEF01, 0x000C, 0x0008,
		[]string{"move.l  #$ABCDEF01,d0", "moveq   #0,d1", "moveq   #0,d3", "moveq   #0,d4", "move.w  d4,ccr"},
		[]string{"dc.w    $E9C0,$1023"}))

	return shard{Name: s, Title: "Bit Field Register", Cases: cases}
}

func shardBFMem() shard {
	s := "bf_mem"
	var cases []testCase

	bfData := []string{
		".bf_data:",
		"                dc.l    $ABCDEF01,$23456789",
		"                even",
	}

	bfWritable := []string{
		".bf_wdata:",
		"                dc.l    $ABCDEF01,$23456789",
		"                even",
	}

	bfZero := []string{
		".bf_zdata:",
		"                dc.l    $00000000,$00000000",
		"                even",
	}

	// -----------------------------------------------------------------------
	// BFTST memory
	// -----------------------------------------------------------------------

	// BFTST (A0){0:8} with data=$AB → field=$AB → N=1
	// opcode=$E8D0, ext: offset=0, width=8 → $0008
	cases = append(cases, intCase(s, "bf_mem_bftst_0_8", "BFTST (A0){0:8}",
		"[A0]=$ABCDEF01... offset=0 width=8", "SR: N=1 Z=0",
		"custom_sr_only", "", 0, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E8D0,$0008"}, bfData...))

	// BFTST (A0){0:32} with data=$ABCDEF01 → N=1
	cases = append(cases, intCase(s, "bf_mem_bftst_0_32", "BFTST (A0){0:32}",
		"[A0]=$ABCDEF01 offset=0 width=0(=32)", "SR: N=1 Z=0",
		"custom_sr_only", "", 0, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E8D0,$0000"}, bfData...))

	// -----------------------------------------------------------------------
	// BFEXTU memory
	// -----------------------------------------------------------------------

	// BFEXTU (A0){0:8},D1 → D1=$000000AB
	// opcode=$E9D0, ext: D1=0001, offset=0, width=8 → $1008
	cases = append(cases, regsr(s, "bf_mem_bfextu_0_8", "BFEXTU (A0){0:8},D1",
		"[A0]=$ABCDEF01... offset=0 width=8", "D1=$000000AB SR: N=1",
		"d1", 0x000000AB, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9D0,$1008"}, bfData...))

	// BFEXTU (A0){4:8},D1 → cross-byte: data $AB,$CD → nibbles $B,$C → $BC
	// ext: D1=0001, offset=4=00001 in 10:6... wait: 4 in 5 bits is 00100
	// = 0001_0_00100_0_01000... no. offset 4: bits 10:6 = 00100? No.
	// 4 in binary is 00100. Placed in bits 10:6: bit10=0,bit9=0,bit8=1,bit7=0,bit6=0
	// That means value in bits 10:6 = 4 << 6 = $0100... no, 4 << 6 = 256 = $0100
	// Full ext: $1000 | (4 << 6) | 8 = $1000 | $0100 | $0008 = $1108... wait no.
	// Let me be precise:
	// bits 15-12: 0001 (D1)
	// bit 11: 0 (Do=imm)
	// bits 10-6: 00100 (offset=4)
	// bit 5: 0 (Dw=imm)
	// bits 4-0: 01000 (width=8)
	// = 0001_0_00100_0_01000 = 0001 0001 0000 1000 = $1108
	// Hmm wait. Let me lay this out bit by bit:
	// bit15=0, bit14=0, bit13=0, bit12=1, bit11=0, bit10=0, bit9=0, bit8=1, bit7=0, bit6=0, bit5=0, bit4=0, bit3=1, bit2=0, bit1=0, bit0=0
	// = 0001 0001 0000 1000 = $1108
	// But the existing test in catalog_existing.go uses $1048 for offset=4, width=8:
	// Looking at: dc.w $E9D0,$1048
	// $1048 = 0001 0000 0100 1000
	// bit15-12: 0001 (D1)
	// bit11: 0 (Do=imm)
	// bit10-6: 00001 = 1? That can't be right for offset 4.
	// Wait: $1048 = 0001_0000_0100_1000
	// bits 15-12: 0001 = D1
	// bit 11: 0
	// bits 10-6: 00001 = 1... that's offset=1? But comment says offset=4.
	// Hmm. Let me recheck. $1048 in binary:
	// $1 = 0001, $0 = 0000, $4 = 0100, $8 = 1000
	// = 0001 0000 0100 1000
	// bit15=0 bit14=0 bit13=0 bit12=1 bit11=0 bit10=0 bit9=0 bit8=0 bit7=0 bit6=1 bit5=0 bit4=0 bit3=1 bit2=0 bit1=0 bit0=0
	// bits 10-6: 00001 = offset 1
	// bits 4-0: 01000 = width 8
	// So $1048 is offset=1, width=8, not offset=4 width=8. The existing test comment says {4:8} but uses offset=1... unless I'm misreading the existing test.
	// Let me re-read the existing test:
	// "BFEXTU (A0){4:8},D1" with ext $1048
	// Actually wait: maybe the encoding puts offset differently. Let me check Motorola docs.
	// The extension word: bits 10-6 is the offset field. For offset=4:
	// 4 = 00100, so bits 10:6 should be 00100
	// $1048 has bits 10:6 = 00001 = 1, not 4.
	// But the existing test claims offset=4 and expects $BC from $ABCD.
	// $ABCD at offset 1, width 8: byte 0=$AB bits 1-7 + byte 1=$CD bit 0
	// Offset 1 from $AB: 0101_0110 (shift left 1 from $AB=10101011)... this is getting confusing.
	// Let me just trust the existing code's encoding since it's tested and working.
	// $1048: offset field = bits 10:6 = (0x48 >> 6) & 0x1F = 1. Hmm.
	// Actually: 0x1048 >> 6 = 0x41, & 0x1F = 1. So offset=1, width=8.
	// But the test says {4:8} and expects $BC from $ABCD.
	// With offset 4 from $ABCD: nibble boundary: bits 4-11 = $BC. That makes sense.
	// So either the encoding in existing code is wrong but works due to some other reason,
	// or I'm miscalculating. Let me try differently:
	// $1048 = 0x1048. In the extension word for bit fields:
	// 15-12: register Dn = (0x1048 >> 12) & 0xF = 1 → D1
	// 11: Do = (0x1048 >> 11) & 1 = 0 → immediate offset
	// 10-6: offset = (0x1048 >> 6) & 0x1F = (0x1048/64) & 31 = (65.125) & 31 = 65 & 31... wait
	// 0x1048 = 4168 decimal. 4168 >> 6 = 65. 65 & 0x1F = 65 & 31 = 65 - 62 = 1. So offset=1?
	// Hmm, but $ABCD at offset 1, width 8:
	// Memory byte 0 = $AB = 10101011, byte 1 = $CD = 11001101
	// Starting at bit offset 1 (from bit 0=MSB of byte 0), 8 bits:
	// bits: 0101011_1 = from $AB skip 1 bit: 0101011, then 1 bit from $CD: 1
	// = 01010111 = $57... that doesn't match $BC.
	// So offset must be 4 in the existing test. Let me recompute $1048:
	// Maybe I have the bit layout wrong. Let me check more carefully.
	// Hmm, actually looking at the encoding in the prompt description:
	// "bits 10-6=offset(imm or reg#)"
	// 0x1048 = 0001 0000 0100 1000
	// That's 16 bits. Reading left to right:
	// [15:12] = 0001 = 1 (D1)
	// [11] = 0 (Do=imm)
	// [10:6] = 00001 = 1
	// [5] = 0 (Dw=imm)
	// [4:0] = 01000 = 8
	// So it IS offset=1, width=8 in the encoding.
	// But the NAME says {4:8}. The test might have a naming mismatch but work because
	// the actual opcode encoding is what matters. Or maybe offset=1 with 2-byte data $ABCD:
	// byte0=$AB=10101011, byte1=$CD=11001101
	// offset=1, width=8: skip 1 bit → 0101011 (7 bits from byte0) + 1 bit from byte1 = 01010111_1 → 01010111 = $57
	// That gives $57, not $BC.
	// Unless the existing test is using data "$ABCD" as a word:
	// .mem_data: dc.w $ABCD
	// At (A0), byte 0=$AB, byte 1=$CD
	// For {4:8}: offset=4 bits into byte 0, spans into byte 1
	// $AB = 1010 1011, $CD = 1100 1101
	// offset 4, 8 bits: 1011 1100 = $BC. Yes!
	// So for offset=4 the encoding should be 4 in bits 10:6.
	// 4 << 6 = 256 = $100. Plus D1 in 15:12 = $1000. Width=8 = $0008.
	// Total = $1000 | $0100 | $0008... wait $0100 is bit 8 set.
	// bits 10:6 = 00100 → bit8=1, rest=0 → masked value = 0000_0001_0000_0000 >> 6... no.
	// value 4 in bits 10:6: bit6 would hold the LSB.
	// 4 = 100 in binary = bit positions 10=0, 9=0, 8=1, 7=0, 6=0
	// So bit 8 is set → 0x0100
	// ext = 0x1000 | 0x0100 | 0x0008 = $1108
	// But the existing code uses $1048! So either: 1) the existing code has a bug that's
	// compensated elsewhere, or 2) I have the bit field layout wrong.
	// Let me re-examine: maybe the offset field is bits 10:6 counted differently.
	// In 68020 manual, the extension word is:
	// Bit 15-12: Dn register
	// Bit 11: Do (0=imm offset, 1=Dn offset)
	// Bit 10-6: offset (immediate value 0-31 or register number 0-7)
	// Bit 5: Dw (0=imm width, 1=Dn width)
	// Bit 4-0: width (immediate value, 0 means 32, or register number 0-7)
	//
	// For offset=4 immediate: bits 10:6 = 00100 (value 4)
	// Encoding: 0001_0_00100_0_01000
	// = 0001 0001 0000 1000 = $1108
	//
	// The existing catalog uses $1048 which encodes offset=1.
	// This is likely a bug in the existing catalog that happens to not be caught.
	// I'll use the correct encoding in my tests.

	// BFEXTU (A0){4:8},D1 → data $ABCDEF01: byte0=$AB byte1=$CD
	// offset=4, width=8: from $AB take low 4 bits (1011), from $CD take high 4 bits (1100)
	// = 1011_1100 = $BC
	// ext = $1108
	cases = append(cases, regsr(s, "bf_mem_bfextu_4_8", "BFEXTU (A0){4:8},D1",
		"[A0]=$ABCDEF01... offset=4 width=8 cross-byte", "D1=$000000BC SR: N=1",
		"d1", 0x000000BC, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9D0,$1108"}, bfData...))

	// BFEXTU (A0){0:32},D1 → D1=$ABCDEF01
	cases = append(cases, regsr(s, "bf_mem_bfextu_0_32", "BFEXTU (A0){0:32},D1",
		"[A0]=$ABCDEF01... offset=0 width=0(=32)", "D1=$ABCDEF01 SR: N=1",
		"d1", 0xABCDEF01, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9D0,$1000"}, bfData...))

	// BFEXTU (A0){12:16},D1 → spans 3 bytes
	// data: $AB $CD $EF $01
	// offset=12, width=16: start in byte 1 ($CD) at bit 4, span 16 bits
	// byte1 low 4 bits: $D (1101), byte2: $EF (11101111), byte3 high 4 bits: $0 (0000)
	// = 1101_1110_1111_0000 = $DEF0
	// ext: D1=0001, offset=12=01100, width=16=10000
	// = 0001_0_01100_0_10000
	// bit by bit: 0001 0011 0001 0000 = $1310
	cases = append(cases, regsr(s, "bf_mem_bfextu_12_16", "BFEXTU (A0){12:16},D1",
		"[A0]=$ABCDEF01... offset=12 width=16 cross 3 bytes", "D1=$0000DEF0 SR: N=1",
		"d1", 0x0000DEF0, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9D0,$1310"}, bfData...))

	// BFEXTU (A0){4:32},D1 → 5-byte span (critical edge case for bugs)
	// data: $AB $CD $EF $01 $23
	// offset=4, width=32: start at bit 4 of byte 0, read 32 bits
	// byte0 low 4: $B (1011), byte1: $CD, byte2: $EF, byte3: $01, byte4 high 4: $2 (0010)
	// = 1011_1100_1101_1110_1111_0000_0001_0010 = $BCDEF012
	// ext: D1=0001, offset=4=00100, width=0(=32)
	// = 0001_0_00100_0_00000
	// = 0001 0001 0000 0000 = $1100
	cases = append(cases, regsr(s, "bf_mem_bfextu_4_32", "BFEXTU (A0){4:32},D1",
		"[A0]=$ABCDEF01,$23... offset=4 width=32 (5-byte span)", "D1=$BCDEF012 SR: N=1",
		"d1", 0xBCDEF012, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E9D0,$1100"}, bfData...))

	// -----------------------------------------------------------------------
	// BFEXTS memory
	// -----------------------------------------------------------------------

	// BFEXTS (A0){0:8},D1 → field=$AB=10101011 → sign bit=1 → D1=$FFFFFFAB
	// opcode=$EBD0, ext: D1=0001, offset=0, width=8 → $1008
	cases = append(cases, regsr(s, "bf_mem_bfexts_0_8", "BFEXTS (A0){0:8},D1",
		"[A0]=$ABCDEF01... offset=0 width=8", "D1=$FFFFFFAB SR: N=1",
		"d1", 0xFFFFFFAB, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EBD0,$1008"}, bfData...))

	// BFEXTS (A0){8:8},D1 → field=$CD=11001101 → D1=$FFFFFFCD
	// ext: D1=0001, offset=8=01000, width=8=01000
	// = 0001_0_01000_0_01000
	// = 0001 0010 0000 1000 = $1208
	cases = append(cases, regsr(s, "bf_mem_bfexts_8_8", "BFEXTS (A0){8:8},D1",
		"[A0]=$ABCDEF01... offset=8 width=8", "D1=$FFFFFFCD SR: N=1",
		"d1", 0xFFFFFFCD, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $EBD0,$1208"}, bfData...))

	// -----------------------------------------------------------------------
	// BFCHG memory
	// -----------------------------------------------------------------------

	// BFCHG (A0){0:8} with data=$ABCDEF01 → complement byte 0 → $54CDEF01
	// opcode=$EAD0, ext: offset=0, width=8 → $0008
	// Read back byte 0 as longword to verify
	cases = append(cases, regonly(s, "bf_mem_bfchg_0_8", "BFCHG (A0){0:8}",
		"[A0]=$ABCDEF01... offset=0 width=8", "readback=$54CDEF01",
		"d0", 0x54CDEF01,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$ABCDEF01,(a0)", "move.l  #$23456789,4(a0)"},
		[]string{"dc.w    $EAD0,$0008", "move.l  (a0),d0"}, bfWritable...))

	// BFCHG (A0){0:32} with data=$ABCDEF01 → complement all → $543210FE
	cases = append(cases, regonly(s, "bf_mem_bfchg_0_32", "BFCHG (A0){0:32}",
		"[A0]=$ABCDEF01... offset=0 width=0(=32)", "readback=$543210FE",
		"d0", 0x543210FE,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$ABCDEF01,(a0)", "move.l  #$23456789,4(a0)"},
		[]string{"dc.w    $EAD0,$0000", "move.l  (a0),d0"}, bfWritable...))

	// -----------------------------------------------------------------------
	// BFCLR memory
	// -----------------------------------------------------------------------

	// BFCLR (A0){0:8} with data=$ABCDEF01 → clear byte 0 → $00CDEF01
	// opcode=$ECD0, ext: offset=0, width=8 → $0008
	cases = append(cases, regonly(s, "bf_mem_bfclr_0_8", "BFCLR (A0){0:8}",
		"[A0]=$ABCDEF01... offset=0 width=8", "readback=$00CDEF01",
		"d0", 0x00CDEF01,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$ABCDEF01,(a0)", "move.l  #$23456789,4(a0)"},
		[]string{"dc.w    $ECD0,$0008", "move.l  (a0),d0"}, bfWritable...))

	// BFCLR (A0){8:16} with data=$ABCDEF01 → clear bits 8..23 → $AB0000 01
	// ext: offset=8=01000, width=16=10000
	// = 0000_0_01000_0_10000
	// = 0000 0010 0001 0000 = $0210
	cases = append(cases, regonly(s, "bf_mem_bfclr_8_16", "BFCLR (A0){8:16}",
		"[A0]=$ABCDEF01... offset=8 width=16", "readback=$AB000001",
		"d0", 0xAB000001,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$ABCDEF01,(a0)", "move.l  #$23456789,4(a0)"},
		[]string{"dc.w    $ECD0,$0210", "move.l  (a0),d0"}, bfWritable...))

	// -----------------------------------------------------------------------
	// BFSET memory
	// -----------------------------------------------------------------------

	// BFSET (A0){0:8} with data=$00CDEF01 → set byte 0 → $FFCDEF01
	// opcode=$EED0, ext: offset=0, width=8 → $0008
	cases = append(cases, regonly(s, "bf_mem_bfset_0_8", "BFSET (A0){0:8}",
		"[A0]=$00CDEF01 offset=0 width=8", "readback=$FFCDEF01",
		"d0", 0xFFCDEF01,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$00CDEF01,(a0)", "move.l  #$23456789,4(a0)"},
		[]string{"dc.w    $EED0,$0008", "move.l  (a0),d0"}, bfWritable...))

	// BFSET (A0){16:16} with data=$ABCD0000 → set bits 16..31 → $ABCDFFFF
	// ext: offset=16=00100 in 10:6... 16=10000
	// bits 10:6 = 10000 → bit10=1
	// = 0000_0_10000_0_10000
	// = 0000 0100 0001 0000 = $0410
	cases = append(cases, regonly(s, "bf_mem_bfset_16_16", "BFSET (A0){16:16}",
		"[A0]=$ABCD0000 offset=16 width=16", "readback=$ABCDFFFF",
		"d0", 0xABCDFFFF,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$ABCD0000,(a0)", "move.l  #$23456789,4(a0)"},
		[]string{"dc.w    $EED0,$0410", "move.l  (a0),d0"}, bfWritable...))

	// -----------------------------------------------------------------------
	// BFFFO memory
	// -----------------------------------------------------------------------

	// BFFFO (A0){0:32},D1 with data=$ABCDEF01 → first one at bit 0 (MSB is 1) → D1=0
	// opcode=$EDD0, ext: D1=0001, offset=0, width=0(=32) → $1000
	cases = append(cases, testCase{
		ID: "bf_mem_bfffo_0_32", Shard: s, Kind: kindInt,
		Name:        "BFFFO (A0){0:32},D1",
		Input:       "[A0]=$ABCDEF01 offset=0 width=0(=32)",
		Expected:    "D1=$00000000 SR: N=1",
		ActualMode:  "custom_d1_sr",
		ExpectReg:   "d1",
		ExpectValue: 0x00000000,
		SRMask:      0x000C,
		ExpectSR:    0x0008,
		Setup:       []string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		Body:        []string{"dc.w    $EDD0,$1000"},
		DataPool:    bfData,
	})

	// BFFFO (A0){0:32},D1 with data=$00000000 → no ones → D1=32
	cases = append(cases, testCase{
		ID: "bf_mem_bfffo_zero", Shard: s, Kind: kindInt,
		Name:        "BFFFO (A0){0:32},D1 all-zero",
		Input:       "[A0]=$00000000 offset=0 width=0(=32)",
		Expected:    "D1=$00000020 SR: Z=1",
		ActualMode:  "custom_d1_sr",
		ExpectReg:   "d1",
		ExpectValue: 0x00000020,
		SRMask:      0x000C,
		ExpectSR:    0x0004,
		Setup:       []string{"lea     .bf_zdata(pc),a0", "move.l  #$00000000,(a0)", "move.l  #$00000000,4(a0)", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		Body:        []string{"dc.w    $EDD0,$1000"},
		DataPool:    bfZero,
	})

	// BFFFO (A0){0:32},D1 with data=$00010000 → first one at bit 15 → D1=15
	cases = append(cases, testCase{
		ID: "bf_mem_bfffo_bit15", Shard: s, Kind: kindInt,
		Name:        "BFFFO (A0){0:32},D1 bit 15",
		Input:       "[A0]=$00010000 offset=0 width=0(=32)",
		Expected:    "D1=$0000000F",
		ActualMode:  "custom_d1_sr",
		ExpectReg:   "d1",
		ExpectValue: 0x0000000F,
		SRMask:      0x000C,
		ExpectSR:    0x0000,
		Setup:       []string{"lea     .bf_zdata(pc),a0", "move.l  #$00010000,(a0)", "move.l  #$00000000,4(a0)", "moveq   #0,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		Body:        []string{"dc.w    $EDD0,$1000"},
		DataPool:    bfZero,
	})

	// -----------------------------------------------------------------------
	// BFINS memory
	// -----------------------------------------------------------------------

	// BFINS D1,(A0){0:8} with D1=$000000FF, data=$00000000 → byte 0 = $FF → $FF000000
	// opcode=$EFD0, ext: D1=0001, offset=0, width=8 → $1008
	cases = append(cases, regonly(s, "bf_mem_bfins_0_8", "BFINS D1,(A0){0:8}",
		"D1=$000000FF [A0]=$00000000 offset=0 width=8", "readback=$FF000000",
		"d0", 0xFF000000,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$00000000,(a0)", "move.l  #$00000000,4(a0)", "move.l  #$000000FF,d1"},
		[]string{"dc.w    $EFD0,$1008", "move.l  (a0),d0"}, bfWritable...))

	// BFINS D1,(A0){0:32} with D1=$DEADBEEF, data=$00000000 → $DEADBEEF
	// ext: D1=0001, offset=0, width=0(=32) → $1000
	cases = append(cases, regonly(s, "bf_mem_bfins_0_32", "BFINS D1,(A0){0:32}",
		"D1=$DEADBEEF [A0]=$00000000 offset=0 width=0(=32)", "readback=$DEADBEEF",
		"d0", 0xDEADBEEF,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$00000000,(a0)", "move.l  #$00000000,4(a0)", "move.l  #$DEADBEEF,d1"},
		[]string{"dc.w    $EFD0,$1000", "move.l  (a0),d0"}, bfWritable...))

	// BFINS D1,(A0){8:8} with D1=$000000AB, data=$12003400 → $12AB3400
	// ext: D1=0001, offset=8=01000, width=8=01000
	// = 0001_0_01000_0_01000 = $1208
	cases = append(cases, regonly(s, "bf_mem_bfins_8_8", "BFINS D1,(A0){8:8}",
		"D1=$000000AB [A0]=$12003400 offset=8 width=8", "readback=$12AB3400",
		"d0", 0x12AB3400,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$12003400,(a0)", "move.l  #$00000000,4(a0)", "move.l  #$000000AB,d1"},
		[]string{"dc.w    $EFD0,$1208", "move.l  (a0),d0"}, bfWritable...))

	// BFINS D1,(A0){4:32} — 5-byte span insert
	// D1=$12345678, data=$FF000000,$FF000000
	// offset=4, width=32: inserts 32 bits starting at bit 4
	// byte0 keeps high nibble ($F), then 32 bits of $12345678, then byte4 keeps low nibble
	// byte0: $F0 | $1 = $F1, byte1: $23, byte2: $45, byte3: $67, byte4: $8X where X = low nibble of old byte4
	// old byte4 = $FF, low nibble = $F
	// Result: $F1234567,$8F000000
	// ext: D1=0001, offset=4=00100, width=0(=32)
	// = 0001_0_00100_0_00000 = $1100
	cases = append(cases, regonly(s, "bf_mem_bfins_4_32", "BFINS D1,(A0){4:32} 5-byte span",
		"D1=$12345678 [A0]=$FF...,$FF... offset=4 width=32", "readback=$F1234567",
		"d0", 0xF1234567,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$FF000000,(a0)", "move.l  #$FF000000,4(a0)", "move.l  #$12345678,d1"},
		[]string{"dc.w    $EFD0,$1100", "move.l  (a0),d0"}, bfWritable...))

	// -----------------------------------------------------------------------
	// Dynamic offset/width with memory
	// -----------------------------------------------------------------------

	// BFEXTU (A0){D2:8},D1 where D2=8 → extract byte 1 = $CD
	// ext: D1=0001, Do=1(reg), reg=D2(010), Dw=0(imm), width=8(01000)
	// = 0001_1_00010_0_01000 = $1888
	cases = append(cases, regsr(s, "bf_mem_bfextu_dyn_off", "BFEXTU (A0){D2:8},D1",
		"[A0]=$ABCDEF01... D2=8(offset) width=8", "D1=$000000CD SR: N=1",
		"d1", 0x000000CD, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #8,d2", "moveq   #0,d4", "move.w  d4,ccr"},
		[]string{"dc.w    $E9D0,$1888"}, bfData...))

	// BFEXTU (A0){D2:D3},D1 where D2=0, D3=16 → extract first 16 bits = $ABCD
	// ext: D1=0001, Do=1, reg=D2(010), Dw=1, reg=D3(011)
	// = 0001_1_00010_1_00011 = $18A3
	cases = append(cases, regsr(s, "bf_mem_bfextu_dyn_both", "BFEXTU (A0){D2:D3},D1",
		"[A0]=$ABCDEF01... D2=0(offset) D3=16(width)", "D1=$0000ABCD SR: N=1",
		"d1", 0x0000ABCD, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #0,d2", "moveq   #16,d3", "moveq   #0,d4", "move.w  d4,ccr"},
		[]string{"dc.w    $E9D0,$18A3"}, bfData...))

	// Memory dynamic offset with large value: D2=32 means offset=32 → byte 4 onward
	// For memory, offset is NOT modulo 32 (unlike register). Offset 32 means byte 4.
	// data: $ABCDEF01 $23456789 → byte4=$23
	// BFEXTU (A0){D2:8},D1 with D2=32 → D1=$23
	cases = append(cases, regsr(s, "bf_mem_bfextu_dyn_off32", "BFEXTU (A0){D2:8},D1 off=32",
		"[A0]=$ABCDEF01,$23456789 D2=32(offset) width=8", "D1=$00000023",
		"d1", 0x00000023, 0x000C, 0x0000,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d1", "moveq   #32,d2", "moveq   #0,d4", "move.w  d4,ccr"},
		[]string{"dc.w    $E9D0,$1888"}, bfData...))

	// Memory dynamic offset negative: D2=-8 (= $FFFFFFF8) should act as signed → offset -8
	// For memory, the offset register is treated as a signed 32-bit value.
	// We'd need A0 to point 1 byte into data to read the previous byte.
	// data at .bf_data: $AB $CD $EF $01 $23 $45 $67 $89
	// lea .bf_data+1,a0 → A0 points to $CD
	// D2=-8 → offset=-8 bits → -1 byte → reads byte at A0-1 = $AB
	// ext: same $1888
	cases = append(cases, regsr(s, "bf_mem_bfextu_dyn_neg_off", "BFEXTU (A0){D2:8},D1 neg offset",
		"A0=data+1 D2=-8(offset) width=8 reads byte before A0", "D1=$000000AB SR: N=1",
		"d1", 0x000000AB, 0x000C, 0x0008,
		[]string{"lea     .bf_data+1(pc),a0", "moveq   #0,d1", "move.l  #-8,d2", "moveq   #0,d4", "move.w  d4,ccr"},
		[]string{"dc.w    $E9D0,$1888"}, bfData...))

	// -----------------------------------------------------------------------
	// Additional edge cases: cross-byte BFINS, BFCHG, BFCLR, BFSET
	// -----------------------------------------------------------------------

	// BFSET (A0){4:8} — cross-byte set
	// data=$00000000 → set bits 4..11 → byte0 gets low nibble set = $0F, byte1 gets high nibble set = $F0
	// Result: $0FF00000...
	// ext: offset=4=00100, width=8=01000
	// = 0000_0_00100_0_01000 = $0108
	cases = append(cases, regonly(s, "bf_mem_bfset_4_8", "BFSET (A0){4:8} cross-byte",
		"[A0]=$00000000 offset=4 width=8", "readback=$0FF00000",
		"d0", 0x0FF00000,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$00000000,(a0)", "move.l  #$00000000,4(a0)"},
		[]string{"dc.w    $EED0,$0108", "move.l  (a0),d0"}, bfWritable...))

	// BFCLR (A0){4:8} — cross-byte clear
	// data=$FFFFFFFF → clear bits 4..11 → $F00FFFFF
	cases = append(cases, regonly(s, "bf_mem_bfclr_4_8", "BFCLR (A0){4:8} cross-byte",
		"[A0]=$FFFFFFFF offset=4 width=8", "readback=$F00FFFFF",
		"d0", 0xF00FFFFF,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$FFFFFFFF,(a0)", "move.l  #$FFFFFFFF,4(a0)"},
		[]string{"dc.w    $ECD0,$0108", "move.l  (a0),d0"}, bfWritable...))

	// BFCHG (A0){4:8} — cross-byte complement
	// data=$A5A5A5A5 → bits 4..11: from $A5=$10100101 $A5=$10100101, bits 4..11 = 0101_1010 = $5A
	// complement = 1010_0101 = $A5... that's the same as original shifted. Let me use different data.
	// data=$FF00FF00 → byte0=$FF, byte1=$00
	// bits 4..11: $FF low nibble = $F, $00 high nibble = $0 → field = $F0
	// complement = $0F
	// New byte0: high nibble $F stays, low nibble becomes $0 → $F0
	// New byte1: high nibble becomes $F, low nibble $0 stays → $F0
	// Result: $F0F0FF00
	cases = append(cases, regonly(s, "bf_mem_bfchg_4_8", "BFCHG (A0){4:8} cross-byte",
		"[A0]=$FF00FF00 offset=4 width=8", "readback=$F0F0FF00",
		"d0", 0xF0F0FF00,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$FF00FF00,(a0)", "move.l  #$00000000,4(a0)"},
		[]string{"dc.w    $EAD0,$0108", "move.l  (a0),d0"}, bfWritable...))

	// BFINS (A0){4:8} — cross-byte insert
	// D1=$000000CC, data=$00000000 → insert $CC at bits 4..11
	// $CC = 11001100
	// byte0 = $00 high nibble + $C low nibble = $0C
	// byte1 = $C0 high nibble + $00 low nibble = $C0
	// Result: $0CC00000
	// ext: D1=0001, offset=4=00100, width=8=01000 → $1108
	cases = append(cases, regonly(s, "bf_mem_bfins_4_8", "BFINS D1,(A0){4:8} cross-byte",
		"D1=$000000CC [A0]=$00000000 offset=4 width=8", "readback=$0CC00000",
		"d0", 0x0CC00000,
		[]string{"lea     .bf_wdata(pc),a0", "move.l  #$00000000,(a0)", "move.l  #$00000000,4(a0)", "move.l  #$000000CC,d1"},
		[]string{"dc.w    $EFD0,$1108", "move.l  (a0),d0"}, bfWritable...))

	// BFTST (A0){4:32} — 5-byte span test, verify N flag from field MSB
	// data=$ABCDEF01,$23456789 → field at offset 4, width 32: $BCDEF012 → MSB=1 → N=1
	cases = append(cases, intCase(s, "bf_mem_bftst_4_32", "BFTST (A0){4:32} 5-byte",
		"[A0]=$ABCDEF01,$23... offset=4 width=32", "SR: N=1 Z=0",
		"custom_sr_only", "", 0, 0x000C, 0x0008,
		[]string{"lea     .bf_data(pc),a0", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $E8D0,$0100"}, bfData...))

	return shard{Name: s, Title: "Bit Field Memory", Cases: cases}
}
