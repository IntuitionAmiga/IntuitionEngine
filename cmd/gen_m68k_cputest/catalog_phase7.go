package main

// Phase 7: Long Mul/Div, CAS/CAS2, CALLM/RTM (~55 cases)
// Shards: muldiv_020, cas_ops, callm_rtm

func buildPhase7Shards() []shard {
	return []shard{
		shardMulDiv020(),
		shardCASops(),
		shardCALLMRTM(),
	}
}

func shardMulDiv020() shard {
	s := "muldiv_020"
	var cases []testCase

	// -----------------------------------------------------------------------
	// MULU.L — unsigned long multiply
	// -----------------------------------------------------------------------

	// MULU.L D1,D0 (32-bit result): 100 * 200 = 20000
	// Encoding: $4C01 (first word=$4C00|ea=D1), ext=$0000 (Dl=D0, 32-bit unsigned)
	cases = append(cases, regsr(s, "muldiv020_mulu_l_simple", "MULU.L D1,D0 100*200=20000",
		"MULU.L D1,D0 initial: D0=100 D1=200", "D0=$00004E20 SR(masked)=$0000",
		"d0", 0x00004E20, 0x000F, 0x0000,
		[]string{"moveq   #100,d0", "move.l  #200,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C01,$0000"}))

	// MULU.L D1,D0 (32-bit): $10000 * $10000 = $100000000, truncated to 0 → Z flag
	cases = append(cases, regsr(s, "muldiv020_mulu_l_overflow_z", "MULU.L D1,D0 overflow->0",
		"MULU.L D1,D0 initial: D0=$10000 D1=$10000", "D0=$00000000 SR(Z)=$0004",
		"d0", 0x00000000, 0x000F, 0x0004,
		[]string{"move.l  #$10000,d0", "move.l  #$10000,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C01,$0000"}))

	// MULU.L D1,D0 (32-bit): $FFFF * $FFFF = $FFFE0001
	cases = append(cases, regsr(s, "muldiv020_mulu_l_large", "MULU.L D1,D0 $FFFF*$FFFF",
		"MULU.L D1,D0 initial: D0=$FFFF D1=$FFFF", "D0=$FFFE0001 SR(N)=$0008",
		"d0", 0xFFFE0001, 0x000F, 0x0008,
		[]string{"move.l  #$FFFF,d0", "move.l  #$FFFF,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C01,$0000"}))

	// MULU.L 64-bit: $FFFFFFFF * 2 = $1FFFFFFFE → D2:D0 = $00000001:$FFFFFFFE
	// Encoding: $4C01, ext=(Dl<<12)|(1<<10)|Dh = (0<<12)|(1<<10)|2 = $0402
	cases = append(cases, testCase{
		ID: "muldiv020_mulu_l_64bit", Shard: s, Kind: kindInt,
		Name: "MULU.L D1,D2:D0 64-bit", Input: "MULU.L D1,D2:D0 $FFFFFFFF*2",
		Expected:   "D0=$FFFFFFFE D2=$00000001",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0xFFFFFFFE},
			{Reg: "d2", Value: 0x00000001},
		},
		Setup: []string{"move.l  #$FFFFFFFF,d0", "moveq   #2,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C01,$0402"},
	})

	// MULU.L 64-bit: $80000000 * 2 = $100000000 → D2:D0 = $00000001:$00000000
	cases = append(cases, testCase{
		ID: "muldiv020_mulu_l_64bit_power", Shard: s, Kind: kindInt,
		Name: "MULU.L D1,D2:D0 $80000000*2", Input: "MULU.L D1,D2:D0 $80000000*2",
		Expected:   "D0=$00000000 D2=$00000001",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0x00000000},
			{Reg: "d2", Value: 0x00000001},
		},
		Setup: []string{"move.l  #$80000000,d0", "moveq   #2,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C01,$0402"},
	})

	// MULU.L 64-bit: 0 * $FFFFFFFF = 0 → Z flag
	cases = append(cases, testCase{
		ID: "muldiv020_mulu_l_64bit_zero", Shard: s, Kind: kindInt,
		Name: "MULU.L D1,D2:D0 zero", Input: "MULU.L D1,D2:D0 0*$FFFFFFFF",
		Expected:   "D0=$00000000 D2=$00000000",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0x00000000},
			{Reg: "d2", Value: 0x00000000},
		},
		Setup: []string{"moveq   #0,d0", "move.l  #$FFFFFFFF,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C01,$0402"},
	})

	// -----------------------------------------------------------------------
	// MULS.L — signed long multiply
	// -----------------------------------------------------------------------

	// MULS.L D1,D0 (32-bit): (-2)*(-3)=6
	// Encoding: $4C01, ext=(Dl<<12)|(1<<11) = (0<<12)|$0800 = $0800
	cases = append(cases, regsr(s, "muldiv020_muls_l_neg_neg", "MULS.L D1,D0 (-2)*(-3)=6",
		"MULS.L D1,D0 initial: D0=$FFFFFFFE D1=$FFFFFFFD", "D0=$00000006 SR(masked)=$0000",
		"d0", 0x00000006, 0x000F, 0x0000,
		[]string{"move.l  #$FFFFFFFE,d0", "move.l  #$FFFFFFFD,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C01,$0800"}))

	// MULS.L D1,D0 (32-bit): (-5)*10=-50 → $FFFFFFCE
	cases = append(cases, regsr(s, "muldiv020_muls_l_neg_pos", "MULS.L D1,D0 (-5)*10=-50",
		"MULS.L D1,D0 initial: D0=$FFFFFFFB D1=$0A", "D0=$FFFFFFCE SR(N)=$0008",
		"d0", 0xFFFFFFCE, 0x000F, 0x0008,
		[]string{"move.l  #$FFFFFFFB,d0", "moveq   #$0A,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C01,$0800"}))

	// MULS.L D1,D0 (32-bit): 0*(-1)=0 → Z flag
	cases = append(cases, regsr(s, "muldiv020_muls_l_zero", "MULS.L D1,D0 0*(-1)=0",
		"MULS.L D1,D0 initial: D0=0 D1=$FFFFFFFF", "D0=$00000000 SR(Z)=$0004",
		"d0", 0x00000000, 0x000F, 0x0004,
		[]string{"moveq   #0,d0", "move.l  #$FFFFFFFF,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C01,$0800"}))

	// MULS.L 64-bit: $7FFFFFFF * 2 = $FFFFFFFE → D2:D0 = $00000000:$FFFFFFFE
	// Encoding: $4C01, ext=(Dl<<12)|(1<<11)|(1<<10)|Dh = (0<<12)|$0800|$0400|2 = $0C02
	cases = append(cases, testCase{
		ID: "muldiv020_muls_l_64bit_pos", Shard: s, Kind: kindInt,
		Name: "MULS.L D1,D2:D0 $7FFFFFFF*2", Input: "MULS.L D1,D2:D0 $7FFFFFFF*2",
		Expected:   "D0=$FFFFFFFE D2=$00000000",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0xFFFFFFFE},
			{Reg: "d2", Value: 0x00000000},
		},
		Setup: []string{"move.l  #$7FFFFFFF,d0", "moveq   #2,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C01,$0C02"},
	})

	// MULS.L 64-bit: $80000000 * 2 = -$100000000 → D2:D0 = $FFFFFFFF:$00000000
	cases = append(cases, testCase{
		ID: "muldiv020_muls_l_64bit_neg", Shard: s, Kind: kindInt,
		Name: "MULS.L D1,D2:D0 $80000000*2", Input: "MULS.L D1,D2:D0 $80000000*2",
		Expected:   "D0=$00000000 D2=$FFFFFFFF",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0x00000000},
			{Reg: "d2", Value: 0xFFFFFFFF},
		},
		Setup: []string{"move.l  #$80000000,d0", "moveq   #2,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C01,$0C02"},
	})

	// MULS.L 64-bit: (-1)*(-1) = 1 → D2:D0 = $00000000:$00000001
	cases = append(cases, testCase{
		ID: "muldiv020_muls_l_64bit_neg1sq", Shard: s, Kind: kindInt,
		Name: "MULS.L D1,D2:D0 (-1)*(-1)", Input: "MULS.L D1,D2:D0 (-1)*(-1)",
		Expected:   "D0=$00000001 D2=$00000000",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0x00000001},
			{Reg: "d2", Value: 0x00000000},
		},
		Setup: []string{"move.l  #$FFFFFFFF,d0", "move.l  #$FFFFFFFF,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C01,$0C02"},
	})

	// -----------------------------------------------------------------------
	// DIVU.L — unsigned long divide
	// -----------------------------------------------------------------------

	// DIVU.L D1,D0 (32÷32→32q): 20000 / 100 = 200
	// Encoding: first=$4C41 ($4C40|ea=D1), ext=(Dq<<12)|Dr = (0<<12)|0 = $0000
	// When Dr==Dq, remainder is not stored (quotient only)
	cases = append(cases, regsr(s, "muldiv020_divu_l_simple", "DIVU.L D1,D0 20000/100=200",
		"DIVU.L D1,D0 initial: D0=20000 D1=100", "D0=$000000C8 SR(masked)=$0000",
		"d0", 0x000000C8, 0x000F, 0x0000,
		[]string{"move.l  #20000,d0", "moveq   #100,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C41,$0000"}))

	// DIVU.L D1,D2:D0 — quotient and remainder: 17/5 → q=3, r=2
	// ext=(Dq<<12)|Dr, Dq=D0(0), Dr=D2(2): ext=$0002
	cases = append(cases, testCase{
		ID: "muldiv020_divu_l_remainder", Shard: s, Kind: kindInt,
		Name: "DIVU.L D1,D2:D0 17/5 q=3 r=2", Input: "DIVU.L D1,D2:D0 17/5",
		Expected:   "D0=$00000003 D2=$00000002",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0x00000003},
			{Reg: "d2", Value: 0x00000002},
		},
		Setup: []string{"moveq   #17,d0", "moveq   #5,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C41,$0002"},
	})

	// DIVU.L D1,D0: large dividend: $80000000 / 2 = $40000000
	cases = append(cases, regsr(s, "muldiv020_divu_l_large", "DIVU.L D1,D0 $80000000/2",
		"DIVU.L D1,D0 initial: D0=$80000000 D1=2", "D0=$40000000 SR(masked)=$0000",
		"d0", 0x40000000, 0x000F, 0x0000,
		[]string{"move.l  #$80000000,d0", "moveq   #2,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C41,$0000"}))

	// DIVU.L result zero: 0 / 5 = 0 → Z flag
	cases = append(cases, regsr(s, "muldiv020_divu_l_zero_quot", "DIVU.L D1,D0 0/5=0",
		"DIVU.L D1,D0 initial: D0=0 D1=5", "D0=$00000000 SR(Z)=$0004",
		"d0", 0x00000000, 0x000F, 0x0004,
		[]string{"moveq   #0,d0", "moveq   #5,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C41,$0000"}))

	// DIVU.L divide by zero → trap vector 5
	cases = append(cases, testCase{
		ID: "muldiv020_divu_l_divzero", Shard: s, Kind: kindInt,
		Name: "DIVU.L D1,D0 div-by-zero", Input: "DIVU.L D1,D0 D0=100 D1=0",
		Expected:   "trap taken (vector 5)",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 5,
		Setup: []string{
			"moveq   #5,d0",
			"jsr     ct_install_trap_handler",
			"moveq   #100,d0",
			"moveq   #0,d1",
		},
		Body: []string{"dc.w    $4C41,$0000"},
	})

	// -----------------------------------------------------------------------
	// DIVS.L — signed long divide
	// -----------------------------------------------------------------------

	// DIVS.L D1,D0 (32÷32→32q): -100 / 10 = -10
	// Encoding: first=$4C41, ext=(Dq<<12)|(1<<11)|Dr = (0<<12)|$0800|0 = $0800
	cases = append(cases, regsr(s, "muldiv020_divs_l_neg_div", "DIVS.L D1,D0 -100/10=-10",
		"DIVS.L D1,D0 initial: D0=$FFFFFF9C D1=10", "D0=$FFFFFFF6 SR(N)=$0008",
		"d0", 0xFFFFFFF6, 0x000F, 0x0008,
		[]string{"move.l  #$FFFFFF9C,d0", "moveq   #10,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C41,$0800"}))

	// DIVS.L D1,D0: 100 / (-10) = -10
	cases = append(cases, regsr(s, "muldiv020_divs_l_neg_divisor", "DIVS.L D1,D0 100/(-10)=-10",
		"DIVS.L D1,D0 initial: D0=100 D1=$FFFFFFF6", "D0=$FFFFFFF6 SR(N)=$0008",
		"d0", 0xFFFFFFF6, 0x000F, 0x0008,
		[]string{"moveq   #100,d0", "move.l  #$FFFFFFF6,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C41,$0800"}))

	// DIVS.L D1,D2:D0 — quotient and remainder: -17/5 → q=-3, r=-2
	// ext=(Dq<<12)|(1<<11)|Dr = (0<<12)|$0800|2 = $0802
	cases = append(cases, testCase{
		ID: "muldiv020_divs_l_remainder", Shard: s, Kind: kindInt,
		Name: "DIVS.L D1,D2:D0 -17/5 q=-3 r=-2", Input: "DIVS.L D1,D2:D0 -17/5",
		Expected:   "D0=$FFFFFFFD D2=$FFFFFFFE",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0xFFFFFFFD},
			{Reg: "d2", Value: 0xFFFFFFFE},
		},
		Setup: []string{"move.l  #$FFFFFFEF,d0", "moveq   #5,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C41,$0802"},
	})

	// DIVS.L D1,D0: 0 / (-1) = 0 → Z flag
	cases = append(cases, regsr(s, "muldiv020_divs_l_zero_quot", "DIVS.L D1,D0 0/(-1)=0",
		"DIVS.L D1,D0 initial: D0=0 D1=$FFFFFFFF", "D0=$00000000 SR(Z)=$0004",
		"d0", 0x00000000, 0x000F, 0x0004,
		[]string{"moveq   #0,d0", "move.l  #$FFFFFFFF,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C41,$0800"}))

	// DIVS.L divide by zero → trap vector 5
	cases = append(cases, testCase{
		ID: "muldiv020_divs_l_divzero", Shard: s, Kind: kindInt,
		Name: "DIVS.L D1,D0 div-by-zero", Input: "DIVS.L D1,D0 D0=50 D1=0",
		Expected:   "trap taken (vector 5)",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 5,
		Setup: []string{
			"moveq   #5,d0",
			"jsr     ct_install_trap_handler",
			"moveq   #50,d0",
			"moveq   #0,d1",
		},
		Body: []string{"dc.w    $4C41,$0800"},
	})

	// MULU.L with N flag: result has bit 31 set (32-bit mode)
	cases = append(cases, regsr(s, "muldiv020_mulu_l_nflag", "MULU.L D1,D0 N flag",
		"MULU.L D1,D0 initial: D0=$40000000 D1=3", "D0=$C0000000 SR(N)=$0008",
		"d0", 0xC0000000, 0x000F, 0x0008,
		[]string{"move.l  #$40000000,d0", "moveq   #3,d1", "moveq   #0,d2", "move.w  d2,ccr"},
		[]string{"dc.w    $4C01,$0000"}))

	// DIVU.L remainder-only check: 7/3 → q=2,r=1
	cases = append(cases, testCase{
		ID: "muldiv020_divu_l_rem_check", Shard: s, Kind: kindInt,
		Name: "DIVU.L D1,D2:D0 7/3 q=2 r=1", Input: "DIVU.L D1,D2:D0 7/3",
		Expected:   "D0=$00000002 D2=$00000001",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0x00000002},
			{Reg: "d2", Value: 0x00000001},
		},
		Setup: []string{"moveq   #7,d0", "moveq   #3,d1", "moveq   #0,d2", "moveq   #0,d3", "move.w  d3,ccr"},
		Body:  []string{"dc.w    $4C41,$0002"},
	})

	return shard{Name: s, Title: "Long Mul/Div (020)", Cases: cases}
}

func shardCASops() shard {
	s := "cas_ops"
	var cases []testCase

	// CAS memory buffers
	casL := []string{
		".cas_buf_l:",
		"                dc.l    $00000000",
		"                even",
	}
	casW := []string{
		".cas_buf_w:",
		"                dc.w    $0000",
		"                even",
	}
	casB := []string{
		".cas_buf_b:",
		"                dc.b    $00,$00",
		"                even",
	}
	cas2BufA := []string{
		".cas2_buf_a:",
		"                dc.l    $00000000",
		"                even",
	}
	cas2BufB := []string{
		".cas2_buf_b:",
		"                dc.l    $00000000",
		"                even",
	}

	// -----------------------------------------------------------------------
	// CAS.L Dc,Du,(An) — compare and swap long
	// Encoding: $0ED0|An_reg, ext=(Du<<6)|Dc
	// CAS.L D0,D1,(A0): first=$0ED0, ext=(1<<6)|0=$0040
	// -----------------------------------------------------------------------

	// CAS.L match → swap: [A0]=$1234, D0=$1234, D1=$5678 → [A0]=$5678, Z=1
	cases = append(cases, testCase{
		ID: "cas_ops_casl_match", Shard: s, Kind: kindInt,
		Name: "CAS.L match->swap", Input: "CAS.L D0,D1,(A0) [A0]=$1234 D0=$1234 D1=$5678",
		Expected:   "D0=$00001234 (mem=$5678)",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0x00001234}, // Dc unchanged on match
			{Reg: "d3", Value: 0x00005678}, // readback from memory
		},
		Setup: []string{
			"lea     .cas_buf_l(pc),a0",
			"move.l  #$00001234,(a0)",
			"move.l  #$00001234,d0",
			"move.l  #$00005678,d1",
		},
		Body: []string{
			"dc.w    $0ED0,$0040", // CAS.L D0,D1,(A0)
			"move.l  (a0),d3",     // readback
		},
		DataPool: casL,
	})

	// CAS.L mismatch → Dc updated: [A0]=$1234, D0=$FFFF, D1=$5678 → D0=$1234, Z=0
	cases = append(cases, testCase{
		ID: "cas_ops_casl_mismatch", Shard: s, Kind: kindInt,
		Name: "CAS.L mismatch->Dc update", Input: "CAS.L D0,D1,(A0) [A0]=$1234 D0=$FFFF",
		Expected:   "D0=$00001234 (mem unchanged)",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0x00001234}, // Dc gets memory value
			{Reg: "d3", Value: 0x00001234}, // memory unchanged
		},
		Setup: []string{
			"lea     .cas_buf_l(pc),a0",
			"move.l  #$00001234,(a0)",
			"move.l  #$0000FFFF,d0",
			"move.l  #$00005678,d1",
		},
		Body: []string{
			"dc.w    $0ED0,$0040",
			"move.l  (a0),d3",
		},
		DataPool: casL,
	})

	// CAS.L match sets Z flag
	cases = append(cases, testCase{
		ID: "cas_ops_casl_match_z", Shard: s, Kind: kindInt,
		Name: "CAS.L match Z=1", Input: "CAS.L D0,D1,(A0) match->Z=1",
		Expected:   "SR(Z)=$0004",
		ActualMode: "custom_sr_only",
		SRMask:     0x0004, ExpectSR: 0x0004,
		Setup: []string{
			"lea     .cas_buf_l(pc),a0",
			"move.l  #$AABBCCDD,(a0)",
			"move.l  #$AABBCCDD,d0",
			"move.l  #$11223344,d1",
			"moveq   #0,d2",
			"move.w  d2,ccr",
		},
		Body:     []string{"dc.w    $0ED0,$0040"},
		DataPool: casL,
	})

	// CAS.L mismatch clears Z flag
	cases = append(cases, testCase{
		ID: "cas_ops_casl_mismatch_noz", Shard: s, Kind: kindInt,
		Name: "CAS.L mismatch Z=0", Input: "CAS.L D0,D1,(A0) mismatch->Z=0",
		Expected:   "SR(Z)=$0000",
		ActualMode: "custom_sr_only",
		SRMask:     0x0004, ExpectSR: 0x0000,
		Setup: []string{
			"lea     .cas_buf_l(pc),a0",
			"move.l  #$AABBCCDD,(a0)",
			"move.l  #$11111111,d0",
			"move.l  #$22222222,d1",
			"moveq   #0,d2",
			"move.w  d2,ccr",
		},
		Body:     []string{"dc.w    $0ED0,$0040"},
		DataPool: casL,
	})

	// CAS.W match → swap
	// CAS.W D0,D1,(A0): first=$0CD0, ext=(1<<6)|0=$0040
	cases = append(cases, testCase{
		ID: "cas_ops_casw_match", Shard: s, Kind: kindInt,
		Name: "CAS.W match->swap", Input: "CAS.W D0,D1,(A0) [A0]=$ABCD D0=$ABCD D1=$1234",
		Expected:   "readback=$1234",
		ActualMode: "regonly",
		ExpectReg:  "d0", ExpectValue: 0x00001234,
		Setup: []string{
			"lea     .cas_buf_w(pc),a0",
			"move.w  #$ABCD,(a0)",
			"move.l  #$0000ABCD,d0",
			"move.l  #$00001234,d1",
		},
		Body: []string{
			"dc.w    $0CD0,$0040", // CAS.W D0,D1,(A0)
			"moveq   #0,d0",
			"move.w  (a0),d0", // readback
		},
		DataPool: casW,
	})

	// CAS.W mismatch → Dc updated
	cases = append(cases, testCase{
		ID: "cas_ops_casw_mismatch", Shard: s, Kind: kindInt,
		Name: "CAS.W mismatch->Dc update", Input: "CAS.W D0,D1,(A0) [A0]=$ABCD D0=$1111",
		Expected:   "D0(low16)=$ABCD",
		ActualMode: "regonly",
		ExpectReg:  "d0", ExpectValue: 0x0000ABCD,
		Setup: []string{
			"lea     .cas_buf_w(pc),a0",
			"move.w  #$ABCD,(a0)",
			"move.l  #$00001111,d0",
			"move.l  #$00002222,d1",
		},
		Body: []string{
			"dc.w    $0CD0,$0040",
			"andi.l  #$0000FFFF,d0", // mask to low word
		},
		DataPool: casW,
	})

	// CAS.B match → swap
	// CAS.B D0,D1,(A0): first=$0AD0, ext=(1<<6)|0=$0040
	cases = append(cases, testCase{
		ID: "cas_ops_casb_match", Shard: s, Kind: kindInt,
		Name: "CAS.B match->swap", Input: "CAS.B D0,D1,(A0) [A0]=$55 D0=$55 D1=$AA",
		Expected:   "readback=$AA",
		ActualMode: "regonly",
		ExpectReg:  "d0", ExpectValue: 0x000000AA,
		Setup: []string{
			"lea     .cas_buf_b(pc),a0",
			"move.b  #$55,(a0)",
			"move.l  #$00000055,d0",
			"move.l  #$000000AA,d1",
		},
		Body: []string{
			"dc.w    $0AD0,$0040", // CAS.B D0,D1,(A0)
			"moveq   #0,d0",
			"move.b  (a0),d0", // readback
		},
		DataPool: casB,
	})

	// CAS.B mismatch → Dc updated
	cases = append(cases, testCase{
		ID: "cas_ops_casb_mismatch", Shard: s, Kind: kindInt,
		Name: "CAS.B mismatch->Dc update", Input: "CAS.B D0,D1,(A0) [A0]=$55 D0=$33",
		Expected:   "D0(low8)=$55",
		ActualMode: "regonly",
		ExpectReg:  "d0", ExpectValue: 0x00000055,
		Setup: []string{
			"lea     .cas_buf_b(pc),a0",
			"move.b  #$55,(a0)",
			"move.l  #$00000033,d0",
			"move.l  #$000000AA,d1",
		},
		Body: []string{
			"dc.w    $0AD0,$0040",
			"andi.l  #$000000FF,d0",
		},
		DataPool: casB,
	})

	// CAS.L with zero value match
	cases = append(cases, testCase{
		ID: "cas_ops_casl_zero_match", Shard: s, Kind: kindInt,
		Name: "CAS.L zero match", Input: "CAS.L D0,D1,(A0) [A0]=0 D0=0 D1=$DEADBEEF",
		Expected:   "mem=$DEADBEEF",
		ActualMode: "regonly",
		ExpectReg:  "d0", ExpectValue: 0xDEADBEEF,
		Setup: []string{
			"lea     .cas_buf_l(pc),a0",
			"move.l  #$00000000,(a0)",
			"moveq   #0,d0",
			"move.l  #$DEADBEEF,d1",
		},
		Body: []string{
			"dc.w    $0ED0,$0040",
			"move.l  (a0),d0", // readback
		},
		DataPool: casL,
	})

	// -----------------------------------------------------------------------
	// CAS2.L — compare and swap 2 (dual)
	// Encoding: first=$0EFC (CAS2.L)
	// Ext word 1: (Rn1<<12)|(Du1<<6)|Dc1
	// Ext word 2: (Rn2<<12)|(Du2<<6)|Dc2
	// Rn bit 15=1 means An, bit 15=0 means Dn
	// -----------------------------------------------------------------------

	// CAS2.L both match → both swap
	// Dc1=D0, Du1=D2, Rn1=A0 → ext1=(8<<12|0)|(2<<6)|0 = $8080
	// Dc2=D1, Du2=D3, Rn2=A1 → ext2=(9<<12|0)|(3<<6)|1 = $90C1
	// Wait: Rn1=A0: bit15=1, reg=0 → bits 15:12 = 1000 = $8, so Rn1 field = $8
	// ext1 = ($8<<12)|(D2<<6)|D0 = $8000|$0080|$0000 = $8080
	// ext2 = ($9<<12)|(D3<<6)|D1 = $9000|$00C0|$0001 = $90C1
	cases = append(cases, testCase{
		ID: "cas_ops_cas2l_both_match", Shard: s, Kind: kindInt,
		Name: "CAS2.L both match->swap", Input: "CAS2.L D0:D1,D2:D3,(A0):(A1)",
		Expected:   "both swapped",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d4", Value: 0x11111111}, // readback [A0]
			{Reg: "d5", Value: 0x22222222}, // readback [A1]
		},
		Setup: []string{
			"lea     .cas2_buf_a(pc),a0",
			"lea     .cas2_buf_b(pc),a1",
			"move.l  #$AAAAAAAA,(a0)",
			"move.l  #$BBBBBBBB,(a1)",
			"move.l  #$AAAAAAAA,d0", // Dc1 = compare value for (A0)
			"move.l  #$BBBBBBBB,d1", // Dc2 = compare value for (A1)
			"move.l  #$11111111,d2", // Du1 = update value for (A0)
			"move.l  #$22222222,d3", // Du2 = update value for (A1)
		},
		Body: []string{
			"dc.w    $0EFC,$8080,$90C1",
			"move.l  (a0),d4",
			"move.l  (a1),d5",
		},
		DataPool: append(cas2BufA, cas2BufB...),
	})

	// CAS2.L first mismatch → no swap, both Dc updated
	cases = append(cases, testCase{
		ID: "cas_ops_cas2l_first_mismatch", Shard: s, Kind: kindInt,
		Name: "CAS2.L first mismatch->no swap", Input: "CAS2.L D0:D1,D2:D3,(A0):(A1) D0 wrong",
		Expected:   "D0=$AAAAAAAA D1=$BBBBBBBB (no swap)",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0xAAAAAAAA}, // Dc1 updated from (A0)
			{Reg: "d1", Value: 0xBBBBBBBB}, // Dc2 updated from (A1)
		},
		Setup: []string{
			"lea     .cas2_buf_a(pc),a0",
			"lea     .cas2_buf_b(pc),a1",
			"move.l  #$AAAAAAAA,(a0)",
			"move.l  #$BBBBBBBB,(a1)",
			"move.l  #$DEADBEEF,d0", // Dc1 wrong → mismatch
			"move.l  #$BBBBBBBB,d1", // Dc2 correct (but doesn't matter)
			"move.l  #$11111111,d2",
			"move.l  #$22222222,d3",
		},
		Body: []string{
			"dc.w    $0EFC,$8080,$90C1",
		},
		DataPool: append(cas2BufA, cas2BufB...),
	})

	// CAS2.L second mismatch → no swap
	cases = append(cases, testCase{
		ID: "cas_ops_cas2l_second_mismatch", Shard: s, Kind: kindInt,
		Name: "CAS2.L second mismatch->no swap", Input: "CAS2.L D0:D1,D2:D3,(A0):(A1) D1 wrong",
		Expected:   "D0=$AAAAAAAA D1=$BBBBBBBB (no swap)",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d0", Value: 0xAAAAAAAA}, // Dc1 updated
			{Reg: "d1", Value: 0xBBBBBBBB}, // Dc2 updated
		},
		Setup: []string{
			"lea     .cas2_buf_a(pc),a0",
			"lea     .cas2_buf_b(pc),a1",
			"move.l  #$AAAAAAAA,(a0)",
			"move.l  #$BBBBBBBB,(a1)",
			"move.l  #$AAAAAAAA,d0", // Dc1 correct
			"move.l  #$DEADBEEF,d1", // Dc2 wrong → mismatch
			"move.l  #$11111111,d2",
			"move.l  #$22222222,d3",
		},
		Body: []string{
			"dc.w    $0EFC,$8080,$90C1",
		},
		DataPool: append(cas2BufA, cas2BufB...),
	})

	// CAS2.W both match → both swap
	// CAS2.W: first=$0CFC
	// Same ext word layout, but operates on words
	cases = append(cases, testCase{
		ID: "cas_ops_cas2w_both_match", Shard: s, Kind: kindInt,
		Name: "CAS2.W both match->swap", Input: "CAS2.W D0:D1,D2:D3,(A0):(A1)",
		Expected:   "both swapped (word)",
		ActualMode: "multi_reg",
		ExpectRegs: []regExpect{
			{Reg: "d4", Value: 0x00001111}, // readback [A0] word
			{Reg: "d5", Value: 0x00002222}, // readback [A1] word
		},
		Setup: []string{
			"lea     .cas2_buf_a(pc),a0",
			"lea     .cas2_buf_b(pc),a1",
			"move.w  #$AAAA,(a0)",
			"move.w  #$BBBB,(a1)",
			"move.l  #$0000AAAA,d0",
			"move.l  #$0000BBBB,d1",
			"move.l  #$00001111,d2",
			"move.l  #$00002222,d3",
		},
		Body: []string{
			"dc.w    $0CFC,$8080,$90C1",
			"moveq   #0,d4",
			"move.w  (a0),d4",
			"moveq   #0,d5",
			"move.w  (a1),d5",
		},
		DataPool: append(cas2BufA, cas2BufB...),
	})

	// CAS.L with different register pairs: CAS.L D2,D3,(A0)
	// ext=(Du<<6)|Dc = (3<<6)|2 = $00C2
	cases = append(cases, testCase{
		ID: "cas_ops_casl_diff_regs", Shard: s, Kind: kindInt,
		Name: "CAS.L D2,D3,(A0) match", Input: "CAS.L D2,D3,(A0) [A0]=$42 D2=$42",
		Expected:   "mem=$BEEF",
		ActualMode: "regonly",
		ExpectReg:  "d0", ExpectValue: 0x0000BEEF,
		Setup: []string{
			"lea     .cas_buf_l(pc),a0",
			"move.l  #$00000042,(a0)",
			"moveq   #$42,d2",
			"move.l  #$0000BEEF,d3",
		},
		Body: []string{
			"dc.w    $0ED0,$00C2", // CAS.L D2,D3,(A0)
			"move.l  (a0),d0",
		},
		DataPool: casL,
	})

	return shard{Name: s, Title: "CAS/CAS2 Ops", Cases: cases}
}

func shardCALLMRTM() shard {
	s := "callm_rtm"
	var cases []testCase

	// CALLM and RTM are 68020-only instructions that were removed in 68030.
	// Most emulators (including ours) may treat them as unimplemented/illegal.
	// We test that they either execute or take a known exception.

	// RTM Dn: opcode = $06C0 | Dn
	// RTM D0: $06C0
	// Illegal/unimplemented instructions stack the faulting PC, so RTE without
	// resume would loop forever. ct_trap_resume redirects to after the opcode.
	cases = append(cases, testCase{
		ID: "callm_rtm_rtm_d0", Shard: s, Kind: kindInt,
		Name: "RTM D0 (020)", Input: "RTM D0 — expect trap or execute",
		Expected:   "trap taken (unimplemented)",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 11,
		Setup: []string{
			"moveq   #11,d0",
			"jsr     ct_install_trap_handler",
			"lea     .rtm_d0_resume(pc),a1",
			"move.l  a1,ct_trap_resume",
		},
		Body: []string{
			"dc.w    $06C0", // RTM D0
			".rtm_d0_resume:",
		},
	})

	// RTM An: opcode = $06C8 | An
	// RTM A0: $06C8
	cases = append(cases, testCase{
		ID: "callm_rtm_rtm_a0", Shard: s, Kind: kindInt,
		Name: "RTM A0 (020)", Input: "RTM A0 — expect trap or execute",
		Expected:   "trap taken (unimplemented)",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 11,
		Setup: []string{
			"moveq   #11,d0",
			"jsr     ct_install_trap_handler",
			"lea     .rtm_a0_resume(pc),a1",
			"move.l  a1,ct_trap_resume",
		},
		Body: []string{
			"dc.w    $06C8", // RTM A0
			".rtm_a0_resume:",
		},
	})

	// CALLM #0,(A0): opcode = $06D0 (ea=(A0)=010 000), followed by argument byte word
	// Actually CALLM: first = $06C0 | ea_mode_reg, for (A0) ea = 010|000 = $10
	// first = $06C0 | $10 = $06D0, second word = #data (argument count byte)
	cases = append(cases, testCase{
		ID: "callm_rtm_callm_a0", Shard: s, Kind: kindInt,
		Name: "CALLM #0,(A0) (020)", Input: "CALLM #0,(A0) — expect trap",
		Expected:   "trap taken (unimplemented)",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 11,
		Setup: []string{
			"moveq   #11,d0",
			"jsr     ct_install_trap_handler",
			"lea     .callm_a0_resume(pc),a1",
			"move.l  a1,ct_trap_resume",
			"lea     .callm_target(pc),a0",
		},
		Body: []string{
			"dc.w    $06D0,$0000", // CALLM #0,(A0)
			".callm_a0_resume:",
		},
		DataPool: []string{
			".callm_target:",
			"                dc.l    $00000000",
			"                even",
		},
	})

	// CALLM #4,d16(A0)
	// ea = d16(A0) = 101|000 = $28, first = $06C0 | $28 = $06E8
	cases = append(cases, testCase{
		ID: "callm_rtm_callm_d16", Shard: s, Kind: kindInt,
		Name: "CALLM #4,4(A0) (020)", Input: "CALLM #4,4(A0) — expect trap",
		Expected:   "trap taken (unimplemented)",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 11,
		Setup: []string{
			"moveq   #11,d0",
			"jsr     ct_install_trap_handler",
			"lea     .callm_d16_resume(pc),a1",
			"move.l  a1,ct_trap_resume",
			"lea     .callm_d16_base(pc),a0",
		},
		Body: []string{
			"dc.w    $06E8,$0004,$0004", // CALLM #4, 4(A0)
			".callm_d16_resume:",
		},
		DataPool: []string{
			".callm_d16_base:",
			"                dc.l    $00000000",
			"                dc.l    $00000000",
			"                even",
		},
	})

	// RTM D7: $06C7
	cases = append(cases, testCase{
		ID: "callm_rtm_rtm_d7", Shard: s, Kind: kindInt,
		Name: "RTM D7 (020)", Input: "RTM D7 — expect trap",
		Expected:   "trap taken (unimplemented)",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 11,
		Setup: []string{
			"moveq   #11,d0",
			"jsr     ct_install_trap_handler",
			"lea     .rtm_d7_resume(pc),a1",
			"move.l  a1,ct_trap_resume",
		},
		Body: []string{
			"dc.w    $06C7", // RTM D7
			".rtm_d7_resume:",
		},
	})

	// RTM A7: $06CF
	cases = append(cases, testCase{
		ID: "callm_rtm_rtm_a7", Shard: s, Kind: kindInt,
		Name: "RTM A7 (020)", Input: "RTM A7 — expect trap",
		Expected:   "trap taken (unimplemented)",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 11,
		Setup: []string{
			"moveq   #11,d0",
			"jsr     ct_install_trap_handler",
			"lea     .rtm_a7_resume(pc),a1",
			"move.l  a1,ct_trap_resume",
		},
		Body: []string{
			"dc.w    $06CF", // RTM A7
			".rtm_a7_resume:",
		},
	})

	return shard{Name: s, Title: "CALLM/RTM (020)", Cases: cases}
}
