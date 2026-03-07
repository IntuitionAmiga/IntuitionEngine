package main

// Phase 4: Trap Capture + CHK/CHK2/CMP2 (~30 cases)
// Shards: trap_basic, chk2_cmp2

func buildPhase4Shards() []shard {
	return []shard{
		shardTrapBasic(),
		shardCHK2CMP2(),
	}
}

func shardTrapBasic() shard {
	s := "trap_basic"
	var cases []testCase

	// TRAP #0 — vector 32
	cases = append(cases, testCase{
		ID: "trap_basic_trap0", Shard: s, Kind: kindInt,
		Name: "TRAP #0", Input: "TRAP #0", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 32,
		Setup: []string{"moveq   #32,d0", "bsr     ct_install_trap_handler"},
		Body:  []string{"trap    #0"},
	})

	// TRAP #1 — vector 33
	cases = append(cases, testCase{
		ID: "trap_basic_trap1", Shard: s, Kind: kindInt,
		Name: "TRAP #1", Input: "TRAP #1", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 33,
		Setup: []string{"moveq   #33,d0", "bsr     ct_install_trap_handler"},
		Body:  []string{"trap    #1"},
	})

	// TRAP #2 — vector 34
	cases = append(cases, testCase{
		ID: "trap_basic_trap2", Shard: s, Kind: kindInt,
		Name: "TRAP #2", Input: "TRAP #2", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 34,
		Setup: []string{"moveq   #34,d0", "bsr     ct_install_trap_handler"},
		Body:  []string{"trap    #2"},
	})

	// TRAP #3 — vector 35
	cases = append(cases, testCase{
		ID: "trap_basic_trap3", Shard: s, Kind: kindInt,
		Name: "TRAP #3", Input: "TRAP #3", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 35,
		Setup: []string{"moveq   #35,d0", "bsr     ct_install_trap_handler"},
		Body:  []string{"trap    #3"},
	})

	// TRAPV with V=1 (taken) — vector 7
	cases = append(cases, testCase{
		ID: "trap_basic_trapv_taken", Shard: s, Kind: kindInt,
		Name: "TRAPV V=1", Input: "TRAPV with V flag set", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 7,
		Setup: []string{
			"moveq   #7,d0",
			"bsr     ct_install_trap_handler",
			"move.w  #$0002,ccr", // set V flag
		},
		Body: []string{"trapv"},
	})

	// TRAPV with V=0 (not taken) — vector 7
	cases = append(cases, testCase{
		ID: "trap_basic_trapv_not_taken", Shard: s, Kind: kindInt,
		Name: "TRAPV V=0", Input: "TRAPV with V flag clear", Expected: "no trap",
		ActualMode: "exception", ExpectTrap: false, TrapVector: 7,
		Setup: []string{
			"moveq   #7,d0",
			"bsr     ct_install_trap_handler",
			"moveq   #0,d0",
			"move.w  d0,ccr", // clear all flags
		},
		Body: []string{"trapv"},
	})

	// TRAPF.W (020) — never traps. Opcode: $51FA $0000
	cases = append(cases, testCase{
		ID: "trap_basic_trapf_w", Shard: s, Kind: kindInt,
		Name: "TRAPF.W (020)", Input: "TRAPF.W (never trap)", Expected: "no trap",
		ActualMode: "exception", ExpectTrap: false, TrapVector: 7,
		Setup: []string{
			"moveq   #7,d0",
			"bsr     ct_install_trap_handler",
		},
		Body: []string{"dc.w    $51FA,$0000"}, // TRAPF.W #0
	})

	// TRAPT.W (020) — always traps to vector 7. Opcode: $50FA $0000
	cases = append(cases, testCase{
		ID: "trap_basic_trapt_w", Shard: s, Kind: kindInt,
		Name: "TRAPT.W (020)", Input: "TRAPT.W (always trap)", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 7,
		Setup: []string{
			"moveq   #7,d0",
			"bsr     ct_install_trap_handler",
		},
		Body: []string{"dc.w    $50FA,$0000"}, // TRAPT.W #0
	})

	// TRAPF.L (020) — never traps. Opcode: $51FB $00000000
	cases = append(cases, testCase{
		ID: "trap_basic_trapf_l", Shard: s, Kind: kindInt,
		Name: "TRAPF.L (020)", Input: "TRAPF.L (never trap)", Expected: "no trap",
		ActualMode: "exception", ExpectTrap: false, TrapVector: 7,
		Setup: []string{
			"moveq   #7,d0",
			"bsr     ct_install_trap_handler",
		},
		Body: []string{"dc.w    $51FB,$0000,$0000"}, // TRAPF.L #0
	})

	// TRAPT (020) no operand — always traps to vector 7. Opcode: $50FC
	cases = append(cases, testCase{
		ID: "trap_basic_trapt_no_op", Shard: s, Kind: kindInt,
		Name: "TRAPT (020)", Input: "TRAPT (always trap, no operand)", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 7,
		Setup: []string{
			"moveq   #7,d0",
			"bsr     ct_install_trap_handler",
		},
		Body: []string{"dc.w    $50FC"}, // TRAPT (no operand)
	})

	// CHK.W D1,D0 — in-bounds (no trap)
	// D0=5, D1=10 (upper bound). 0 <= 5 <= 10 → no trap
	cases = append(cases, testCase{
		ID: "trap_basic_chk_w_inbounds", Shard: s, Kind: kindInt,
		Name: "CHK.W in-bounds", Input: "CHK.W D1,D0 D0=5 D1=10", Expected: "no trap",
		ActualMode: "exception", ExpectTrap: false, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"moveq   #5,d0",
			"moveq   #10,d1",
		},
		Body: []string{"chk.w   d1,d0"},
	})

	// CHK.W D1,D0 — D0 negative (trap vector 6)
	cases = append(cases, testCase{
		ID: "trap_basic_chk_w_neg", Shard: s, Kind: kindInt,
		Name: "CHK.W D0<0", Input: "CHK.W D1,D0 D0=-1 D1=10", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"moveq   #-1,d0",
			"moveq   #10,d1",
		},
		Body: []string{"chk.w   d1,d0"},
	})

	// CHK.W D1,D0 — D0 > bound (trap vector 6)
	cases = append(cases, testCase{
		ID: "trap_basic_chk_w_over", Shard: s, Kind: kindInt,
		Name: "CHK.W D0>bound", Input: "CHK.W D1,D0 D0=15 D1=10", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"moveq   #15,d0",
			"moveq   #10,d1",
		},
		Body: []string{"chk.w   d1,d0"},
	})

	// CHK.L D1,D0 (020) — in-bounds (no trap). Opcode: $4100 for CHK.L D0,D0 but
	// CHK.L D1,D0 = $4101 (size=long uses bits 8:6 = 100)
	// Actually CHK.L Dn,Dn: 0100 rrr 100 eee eee. D0 reg=000, size=100(long), ea=D1=000 001
	// = 0100 000 100 000 001 = $4101
	cases = append(cases, testCase{
		ID: "trap_basic_chk_l_inbounds", Shard: s, Kind: kindInt,
		Name: "CHK.L in-bounds (020)", Input: "CHK.L D1,D0 D0=100 D1=200", Expected: "no trap",
		ActualMode: "exception", ExpectTrap: false, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"moveq   #100,d0",
			"move.l  #200,d1",
		},
		Body: []string{"dc.w    $4101"}, // CHK.L D1,D0
	})

	// CHK.L D1,D0 (020) — out-of-bounds (trap)
	cases = append(cases, testCase{
		ID: "trap_basic_chk_l_over", Shard: s, Kind: kindInt,
		Name: "CHK.L D0>bound (020)", Input: "CHK.L D1,D0 D0=300 D1=200", Expected: "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"move.l  #300,d0",
			"move.l  #200,d1",
		},
		Body: []string{"dc.w    $4101"}, // CHK.L D1,D0
	})

	return shard{Name: s, Title: "Trap Basic", Cases: cases}
}

func shardCHK2CMP2() shard {
	s := "chk2_cmp2"
	var cases []testCase

	// Bounds data for CMP2/CHK2 tests
	// .bounds_b: lower=$10, upper=$50
	boundsB := []string{
		".bounds_b:",
		"                dc.b    $10,$50",
		"                even",
	}
	// .bounds_w: lower=$0100, upper=$0500
	boundsW := []string{
		".bounds_w:",
		"                dc.w    $0100,$0500",
		"                even",
	}
	// .bounds_l: lower=$00001000, upper=$00005000
	boundsL := []string{
		".bounds_l:",
		"                dc.l    $00001000,$00005000",
		"                even",
	}

	// CMP2.B (A0),D0 — in range (D0=$30, bounds $10..$50)
	// Opcode: $00D0 ext=$0000 → CMP2.B (A0),D0
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2b_inrange", Shard: s, Kind: kindInt,
		Name: "CMP2.B in range", Input: "CMP2.B (A0),D0 D0=$30 bounds=$10..$50",
		Expected: "C=0 Z=0",
		// After CMP2: C=0 (in range), Z=0 (not on boundary)
		ActualMode: "custom_sr_only",
		SRMask:     0x0005, ExpectSR: 0x0000,
		Setup: []string{
			"lea     .bounds_b(pc),a0",
			"moveq   #$30,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $00D0,$0000"}, // CMP2.B (A0),D0
		DataPool: boundsB,
	})

	// CMP2.B (A0),D0 — out of range (D0=$60, bounds $10..$50)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2b_outrange", Shard: s, Kind: kindInt,
		Name: "CMP2.B out of range", Input: "CMP2.B (A0),D0 D0=$60 bounds=$10..$50",
		Expected:   "C=1",
		ActualMode: "custom_sr_only",
		SRMask:     0x0001, ExpectSR: 0x0001, // C set
		Setup: []string{
			"lea     .bounds_b(pc),a0",
			"moveq   #$60,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $00D0,$0000"},
		DataPool: boundsB,
	})

	// CMP2.B (A0),D0 — exact lower bound (D0=$10)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2b_lower", Shard: s, Kind: kindInt,
		Name: "CMP2.B exact lower", Input: "CMP2.B (A0),D0 D0=$10 bounds=$10..$50",
		Expected:   "Z=1 C=0",
		ActualMode: "custom_sr_only",
		SRMask:     0x0005, ExpectSR: 0x0004, // Z set, C clear
		Setup: []string{
			"lea     .bounds_b(pc),a0",
			"moveq   #$10,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $00D0,$0000"},
		DataPool: boundsB,
	})

	// CMP2.B (A0),D0 — exact upper bound (D0=$50)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2b_upper", Shard: s, Kind: kindInt,
		Name: "CMP2.B exact upper", Input: "CMP2.B (A0),D0 D0=$50 bounds=$10..$50",
		Expected:   "Z=1 C=0",
		ActualMode: "custom_sr_only",
		SRMask:     0x0005, ExpectSR: 0x0004, // Z set, C clear
		Setup: []string{
			"lea     .bounds_b(pc),a0",
			"moveq   #$50,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $00D0,$0000"},
		DataPool: boundsB,
	})

	// CMP2.W (A0),D0 — in range (D0=$0300, bounds $0100..$0500)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2w_inrange", Shard: s, Kind: kindInt,
		Name: "CMP2.W in range", Input: "CMP2.W (A0),D0 D0=$0300 bounds=$0100..$0500",
		Expected:   "C=0 Z=0",
		ActualMode: "custom_sr_only",
		SRMask:     0x0005, ExpectSR: 0x0000,
		Setup: []string{
			"lea     .bounds_w(pc),a0",
			"move.l  #$0300,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $02D0,$0000"}, // CMP2.W (A0),D0
		DataPool: boundsW,
	})

	// CMP2.W (A0),D0 — out of range (D0=$0600)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2w_outrange", Shard: s, Kind: kindInt,
		Name: "CMP2.W out of range", Input: "CMP2.W (A0),D0 D0=$0600 bounds=$0100..$0500",
		Expected:   "C=1",
		ActualMode: "custom_sr_only",
		SRMask:     0x0001, ExpectSR: 0x0001,
		Setup: []string{
			"lea     .bounds_w(pc),a0",
			"move.l  #$0600,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $02D0,$0000"},
		DataPool: boundsW,
	})

	// CMP2.L (A0),D0 — in range (D0=$00003000, bounds $00001000..$00005000)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2l_inrange", Shard: s, Kind: kindInt,
		Name: "CMP2.L in range", Input: "CMP2.L (A0),D0 D0=$3000 bounds=$1000..$5000",
		Expected:   "C=0 Z=0",
		ActualMode: "custom_sr_only",
		SRMask:     0x0005, ExpectSR: 0x0000,
		Setup: []string{
			"lea     .bounds_l(pc),a0",
			"move.l  #$00003000,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $04D0,$0000"}, // CMP2.L (A0),D0
		DataPool: boundsL,
	})

	// CMP2.L (A0),D0 — out of range (D0=$00006000)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2l_outrange", Shard: s, Kind: kindInt,
		Name: "CMP2.L out of range", Input: "CMP2.L (A0),D0 D0=$6000 bounds=$1000..$5000",
		Expected:   "C=1",
		ActualMode: "custom_sr_only",
		SRMask:     0x0001, ExpectSR: 0x0001,
		Setup: []string{
			"lea     .bounds_l(pc),a0",
			"move.l  #$00006000,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $04D0,$0000"},
		DataPool: boundsL,
	})

	// CHK2.B (A0),D0 — in range (no trap). Ext word bit 11 = 1 → $0800
	cases = append(cases, testCase{
		ID: "chk2_cmp2_chk2b_inrange", Shard: s, Kind: kindInt,
		Name: "CHK2.B in range", Input: "CHK2.B (A0),D0 D0=$30 bounds=$10..$50",
		Expected:   "no trap",
		ActualMode: "exception", ExpectTrap: false, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"lea     .bounds_b(pc),a0",
			"moveq   #$30,d0",
		},
		Body:     []string{"dc.w    $00D0,$0800"}, // CHK2.B (A0),D0
		DataPool: boundsB,
	})

	// CHK2.B (A0),D0 — out of range (trap to vector 6)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_chk2b_outrange", Shard: s, Kind: kindInt,
		Name: "CHK2.B out of range", Input: "CHK2.B (A0),D0 D0=$60 bounds=$10..$50",
		Expected:   "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"lea     .bounds_b(pc),a0",
			"moveq   #$60,d0",
		},
		Body:     []string{"dc.w    $00D0,$0800"},
		DataPool: boundsB,
	})

	// CHK2.L (A0),D0 — in range (no trap)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_chk2l_inrange", Shard: s, Kind: kindInt,
		Name: "CHK2.L in range", Input: "CHK2.L (A0),D0 D0=$3000 bounds=$1000..$5000",
		Expected:   "no trap",
		ActualMode: "exception", ExpectTrap: false, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"lea     .bounds_l(pc),a0",
			"move.l  #$00003000,d0",
		},
		Body:     []string{"dc.w    $04D0,$0800"}, // CHK2.L (A0),D0
		DataPool: boundsL,
	})

	// CHK2.L (A0),D0 — out of range (trap)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_chk2l_outrange", Shard: s, Kind: kindInt,
		Name: "CHK2.L out of range", Input: "CHK2.L (A0),D0 D0=$6000 bounds=$1000..$5000",
		Expected:   "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 6,
		Setup: []string{
			"moveq   #6,d0",
			"bsr     ct_install_trap_handler",
			"lea     .bounds_l(pc),a0",
			"move.l  #$00006000,d0",
		},
		Body:     []string{"dc.w    $04D0,$0800"},
		DataPool: boundsL,
	})

	// CMP2.B (A0),A0 — address register variant
	// Ext word: A/D=1 (bit 15), Rn=A0=000 → $8000
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2b_areg", Shard: s, Kind: kindInt,
		Name: "CMP2.B (A0),A0 addr reg", Input: "CMP2.B (A0),A0 bounds=$10..$50 A0=$30",
		Expected:   "C=0 Z=0",
		ActualMode: "custom_sr_only",
		SRMask:     0x0005, ExpectSR: 0x0000,
		Setup: []string{
			// Put bounds in a temp location, load A0 with bounds addr, execute, then check
			"lea     .bounds_b_areg(pc),a1",
			"move.l  a1,a0",
			// We need A0 to hold bounds address during CMP2, but the register being
			// compared is also A0. On 68020, CMP2 reads bounds from (ea) first, then
			// compares Rn. So A0 = pointer to bounds. The value compared is A0 itself.
			// A0 = address of bounds_b_areg. We need that address to be in $10..$50 range.
			// That's unlikely. Instead, use A1 as the ea base and A0 as the compared reg.
			// CMP2.B (A1),A0: ea = (A1) = 010 001, ext = $8000 (A/D=1, Rn=A0=000)
			// Opcode = $00 | 010 001 = $00C9... wait:
			// CMP2.B format: 0000 0ss0 11 eee eee, ext word
			// size=00(byte), ea=(A1)=010 001
			// = 0000 0000 1101 0001 = $00D1
			"moveq   #$30,d0",
			"move.l  d0,a0", // A0 = $00000030 (value to compare)
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body: []string{"dc.w    $00D1,$8000"}, // CMP2.B (A1),A0
		DataPool: []string{
			".bounds_b_areg:",
			"                dc.b    $10,$50",
			"                even",
		},
	})

	// CMP2.B — below lower bound (D0=$05, bounds $10..$50)
	cases = append(cases, testCase{
		ID: "chk2_cmp2_cmp2b_below", Shard: s, Kind: kindInt,
		Name: "CMP2.B below lower", Input: "CMP2.B (A0),D0 D0=$05 bounds=$10..$50",
		Expected:   "C=1",
		ActualMode: "custom_sr_only",
		SRMask:     0x0001, ExpectSR: 0x0001,
		Setup: []string{
			"lea     .bounds_b(pc),a0",
			"moveq   #$05,d0",
			"moveq   #0,d1",
			"move.w  d1,ccr",
		},
		Body:     []string{"dc.w    $00D0,$0000"},
		DataPool: boundsB,
	})

	return shard{Name: s, Title: "CHK2/CMP2", Cases: cases}
}
