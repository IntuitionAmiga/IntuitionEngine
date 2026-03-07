package main

import "fmt"

// Phase 8: FPU Control State, Conditionals & Core Formats (~60 cases)
// Shards: fpu_ctrl_state, fpu_cond, fpu_formats

func buildPhase8Shards() []shard {
	return []shard{
		shardFPUCtrlState(),
		shardFPUCond(),
		shardFPUFormats(),
	}
}

func shardFPUCtrlState() shard {
	s := "fpu_ctrl_state"
	fp := func(labels ...string) []string { return fpuPool(labels...) }
	var cases []testCase

	// -----------------------------------------------------------------------
	// FPCR rounding modes with FINT
	// FPCR bits 5:4: 00=RN, 01=RZ, 10=RM, 11=RP
	// -----------------------------------------------------------------------

	// 1. FINT(3.5) with RN (default, FPCR=0): banker's rounding → 4.0
	cases = append(cases, fpuCase(s, "fpu_ctrl_fint_rn_3_5", "FINT 3.5 RN->4.0",
		"FINT(3.5) FPCR=RN (banker's round)", "FP0=4.0",
		0x40100000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{
			"fmove.l #$00000000,fpcr",
			"fmove.d .fp_3_5(pc),fp0",
		},
		[]string{
			"fint.x  fp0,fp0",
			"fmove.l #$00000000,fpcr", // restore
		}, fp("3_5")...))

	// 2. FINT(3.5) with RZ (FPCR=$10): → 3.0
	cases = append(cases, fpuCase(s, "fpu_ctrl_fint_rz_3_5", "FINT 3.5 RZ->3.0",
		"FINT(3.5) FPCR=RZ", "FP0=3.0",
		0x40080000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{
			"fmove.l #$00000010,fpcr",
			"fmove.d .fp_3_5(pc),fp0",
		},
		[]string{
			"fint.x  fp0,fp0",
			"fmove.l #$00000000,fpcr",
		}, fp("3_5")...))

	// 3. FINT(3.5) with RP (FPCR=$30): → 4.0
	cases = append(cases, fpuCase(s, "fpu_ctrl_fint_rp_3_5", "FINT 3.5 RP->4.0",
		"FINT(3.5) FPCR=RP", "FP0=4.0",
		0x40100000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{
			"fmove.l #$00000030,fpcr",
			"fmove.d .fp_3_5(pc),fp0",
		},
		[]string{
			"fint.x  fp0,fp0",
			"fmove.l #$00000000,fpcr",
		}, fp("3_5")...))

	// 4. FINT(3.5) with RM (FPCR=$20): → 3.0
	cases = append(cases, fpuCase(s, "fpu_ctrl_fint_rm_3_5", "FINT 3.5 RM->3.0",
		"FINT(3.5) FPCR=RM", "FP0=3.0",
		0x40080000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{
			"fmove.l #$00000020,fpcr",
			"fmove.d .fp_3_5(pc),fp0",
		},
		[]string{
			"fint.x  fp0,fp0",
			"fmove.l #$00000000,fpcr",
		}, fp("3_5")...))

	// 5. FINT(-2.5) with RN: banker's → -2.0
	cases = append(cases, fpuCase(s, "fpu_ctrl_fint_rn_neg2_5", "FINT -2.5 RN->-2.0",
		"FINT(-2.5) FPCR=RN", "FP0=-2.0",
		0xC0000000, 0x00000000, 0x0F000000, 0x08000000,
		[]string{
			"fmove.l #$00000000,fpcr",
			"fmove.d .fp_neg_2_5(pc),fp0",
		},
		[]string{
			"fint.x  fp0,fp0",
			"fmove.l #$00000000,fpcr",
		}, fp("neg_2_5")...))

	// 6. FINT(-2.5) with RZ: → -2.0
	cases = append(cases, fpuCase(s, "fpu_ctrl_fint_rz_neg2_5", "FINT -2.5 RZ->-2.0",
		"FINT(-2.5) FPCR=RZ", "FP0=-2.0",
		0xC0000000, 0x00000000, 0x0F000000, 0x08000000,
		[]string{
			"fmove.l #$00000010,fpcr",
			"fmove.d .fp_neg_2_5(pc),fp0",
		},
		[]string{
			"fint.x  fp0,fp0",
			"fmove.l #$00000000,fpcr",
		}, fp("neg_2_5")...))

	// 7. FINT(-2.5) with RP: → -2.0
	cases = append(cases, fpuCase(s, "fpu_ctrl_fint_rp_neg2_5", "FINT -2.5 RP->-2.0",
		"FINT(-2.5) FPCR=RP", "FP0=-2.0",
		0xC0000000, 0x00000000, 0x0F000000, 0x08000000,
		[]string{
			"fmove.l #$00000030,fpcr",
			"fmove.d .fp_neg_2_5(pc),fp0",
		},
		[]string{
			"fint.x  fp0,fp0",
			"fmove.l #$00000000,fpcr",
		}, fp("neg_2_5")...))

	// 8. FINT(-2.5) with RM: → -3.0
	cases = append(cases, fpuCase(s, "fpu_ctrl_fint_rm_neg2_5", "FINT -2.5 RM->-3.0",
		"FINT(-2.5) FPCR=RM", "FP0=-3.0",
		0xC0080000, 0x00000000, 0x0F000000, 0x08000000,
		[]string{
			"fmove.l #$00000020,fpcr",
			"fmove.d .fp_neg_2_5(pc),fp0",
		},
		[]string{
			"fint.x  fp0,fp0",
			"fmove.l #$00000000,fpcr",
		}, fp("neg_2_5")...))

	// -----------------------------------------------------------------------
	// FMOVE control register round-trips
	// -----------------------------------------------------------------------

	// 9. FMOVE.L D0,FPCR / FMOVE.L FPCR,D1 round-trip
	cases = append(cases, testCase{
		ID: "fpu_ctrl_fpcr_roundtrip", Shard: s, Kind: kindInt,
		Name: "FPCR round-trip", Input: "Write $30 to FPCR, read back",
		Expected:    "D1=$00000030",
		ActualMode:  "custom_d1_sr",
		ExpectValue: 0x00000030, SRMask: 0x0000, ExpectSR: 0x0000,
		Setup: []string{
			"move.l  #$00000030,d0",
		},
		Body: []string{
			"fmove.l d0,fpcr",
			"fmove.l fpcr,d1",
			"fmove.l #$00000000,fpcr", // restore
		},
	})

	// 10. FMOVE.L D0,FPSR / FMOVE.L FPSR,D1 round-trip
	cases = append(cases, testCase{
		ID: "fpu_ctrl_fpsr_roundtrip", Shard: s, Kind: kindInt,
		Name: "FPSR round-trip", Input: "Write $08000000 to FPSR, read back",
		Expected:    "D1=$08000000",
		ActualMode:  "custom_d1_sr",
		ExpectValue: 0x08000000, SRMask: 0x0000, ExpectSR: 0x0000,
		Setup: []string{
			"move.l  #$08000000,d0",
		},
		Body: []string{
			"fmove.l d0,fpsr",
			"fmove.l fpsr,d1",
			"fmove.l #$00000000,fpsr", // restore
		},
	})

	// 11. FMOVE.L D0,FPIAR / FMOVE.L FPIAR,D1 round-trip
	cases = append(cases, testCase{
		ID: "fpu_ctrl_fpiar_roundtrip", Shard: s, Kind: kindInt,
		Name: "FPIAR round-trip", Input: "Write $00012345 to FPIAR, read back",
		Expected:    "D1=$00012345",
		ActualMode:  "custom_d1_sr",
		ExpectValue: 0x00012345, SRMask: 0x0000, ExpectSR: 0x0000,
		Setup: []string{
			"move.l  #$00012345,d0",
		},
		Body: []string{
			"fmove.l d0,fpiar",
			"fmove.l fpiar,d1",
		},
	})

	// -----------------------------------------------------------------------
	// FPSR condition codes after FCMP
	// FPSR bits 27:24 = N Z I NaN
	// -----------------------------------------------------------------------

	// 12. FCMP a > b → N=0,Z=0
	cases = append(cases, testCase{
		ID: "fpu_ctrl_fcmp_gt", Shard: s, Kind: kindInt,
		Name: "FCMP 42>10 FPSR N=0,Z=0", Input: "FCMP FP1,FP0 (42>10)",
		Expected:   "FPSR(NZ)=$00000000",
		ActualMode: "fpsr_only",
		FPSRMask:   0x0C000000, ExpectFPSR: 0x00000000,
		Setup: []string{
			"fmove.d .fp_42(pc),fp0",
			"fmove.d .fp_10(pc),fp1",
		},
		Body:     []string{"fcmp.x  fp1,fp0"},
		DataPool: fp("42", "10"),
	})

	// 13. FCMP a < b → N=1
	cases = append(cases, testCase{
		ID: "fpu_ctrl_fcmp_lt", Shard: s, Kind: kindInt,
		Name: "FCMP 10<42 FPSR N=1", Input: "FCMP FP1,FP0 (10<42)",
		Expected:   "FPSR(N)=$08000000",
		ActualMode: "fpsr_only",
		FPSRMask:   0x08000000, ExpectFPSR: 0x08000000,
		Setup: []string{
			"fmove.d .fp_10(pc),fp0",
			"fmove.d .fp_42(pc),fp1",
		},
		Body:     []string{"fcmp.x  fp1,fp0"},
		DataPool: fp("10", "42"),
	})

	// 14. FCMP a == b → Z=1
	cases = append(cases, testCase{
		ID: "fpu_ctrl_fcmp_eq", Shard: s, Kind: kindInt,
		Name: "FCMP 42==42 FPSR Z=1", Input: "FCMP FP1,FP0 (42==42)",
		Expected:   "FPSR(Z)=$04000000",
		ActualMode: "fpsr_only",
		FPSRMask:   0x04000000, ExpectFPSR: 0x04000000,
		Setup: []string{
			"fmove.d .fp_42(pc),fp0",
			"fmove.d .fp_42(pc),fp1",
		},
		Body:     []string{"fcmp.x  fp1,fp0"},
		DataPool: fp("42"),
	})

	// 15. FCMP with NaN → NaN bit set (FPSR bit 24)
	cases = append(cases, testCase{
		ID: "fpu_ctrl_fcmp_nan", Shard: s, Kind: kindInt,
		Name: "FCMP NaN FPSR NaN=1", Input: "FCMP FP1,FP0 (NaN)",
		Expected:   "FPSR(NaN)=$01000000",
		ActualMode: "fpsr_only",
		FPSRMask:   0x01000000, ExpectFPSR: 0x01000000,
		Setup: []string{
			"fmove.d .fp_nan(pc),fp0",
			"fmove.d .fp_42(pc),fp1",
		},
		Body:     []string{"fcmp.x  fp1,fp0"},
		DataPool: fp("nan", "42"),
	})

	return shard{Name: s, Title: "FPU Control State", Cases: cases}
}

func shardFPUCond() shard {
	s := "fpu_cond"
	fp := func(labels ...string) []string { return fpuPool(labels...) }
	var cases []testCase

	// -----------------------------------------------------------------------
	// FBcc — branch on FPU condition
	// Encoding: $F280 | condition_code, followed by 16-bit displacement
	// Condition codes: $01=EQ, $0E=NE, $02=OGT, $03=OGE, $04=OLT, $05=OLE,
	//                  $06=OGL, $07=OR, $08=UN, $0F=T, $00=F
	// Pattern: fcmp fp1,fp0; fbXX .taken; moveq #0,d0; bra.s .done; .taken: moveq #1,d0; .done:
	// -----------------------------------------------------------------------

	// Helper: fbcc test with a>b (42 > 10)
	fbccGT := func(id, name, cond string, taken bool, pool []string) testCase {
		var val uint32
		if taken {
			val = 1
		}
		return regonly(s, id, name,
			"FCMP FP1,FP0 (42>10) then "+cond,
			"D0=$00000001 (taken) or D0=$00000000",
			"d0", val,
			[]string{
				"fmove.d .fp_42(pc),fp0",
				"fmove.d .fp_10(pc),fp1",
				"fcmp.x  fp1,fp0",
			},
			[]string{
				cond + "  .taken_" + id,
				"moveq   #0,d0",
				"bra.s   .done_" + id,
				".taken_" + id + ":",
				"moveq   #1,d0",
				".done_" + id + ":",
			}, pool...)
	}

	// Helper: fbcc test with a==b (42 == 42)
	fbccEQ := func(id, name, cond string, taken bool, pool []string) testCase {
		var val uint32
		if taken {
			val = 1
		}
		return regonly(s, id, name,
			"FCMP FP1,FP0 (42==42) then "+cond,
			"D0=$00000001 (taken) or D0=$00000000",
			"d0", val,
			[]string{
				"fmove.d .fp_42(pc),fp0",
				"fmove.d .fp_42(pc),fp1",
				"fcmp.x  fp1,fp0",
			},
			[]string{
				cond + "  .taken_" + id,
				"moveq   #0,d0",
				"bra.s   .done_" + id,
				".taken_" + id + ":",
				"moveq   #1,d0",
				".done_" + id + ":",
			}, pool...)
	}

	// Helper: fbcc test with a<b (10 < 42)
	fbccLT := func(id, name, cond string, taken bool, pool []string) testCase {
		var val uint32
		if taken {
			val = 1
		}
		return regonly(s, id, name,
			"FCMP FP1,FP0 (10<42) then "+cond,
			"D0=$00000001 (taken) or D0=$00000000",
			"d0", val,
			[]string{
				"fmove.d .fp_10(pc),fp0",
				"fmove.d .fp_42(pc),fp1",
				"fcmp.x  fp1,fp0",
			},
			[]string{
				cond + "  .taken_" + id,
				"moveq   #0,d0",
				"bra.s   .done_" + id,
				".taken_" + id + ":",
				"moveq   #1,d0",
				".done_" + id + ":",
			}, pool...)
	}

	pool42_10 := fp("42", "10")
	pool42 := fp("42")

	// 1. FBEQ: equal → taken; gt → not taken
	cases = append(cases, fbccEQ("fpu_cond_fbeq_taken", "FBEQ taken (eq)", "fbeq", true, pool42))
	cases = append(cases, fbccGT("fpu_cond_fbeq_not", "FBEQ not taken (gt)", "fbeq", false, pool42_10))

	// 2. FBNE: gt → taken; eq → not taken
	cases = append(cases, fbccGT("fpu_cond_fbne_taken", "FBNE taken (gt)", "fbne", true, pool42_10))
	cases = append(cases, fbccEQ("fpu_cond_fbne_not", "FBNE not taken (eq)", "fbne", false, pool42))

	// 3. FBOGT: gt → taken; lt → not taken
	cases = append(cases, fbccGT("fpu_cond_fbogt_taken", "FBOGT taken (gt)", "fbogt", true, pool42_10))
	cases = append(cases, fbccLT("fpu_cond_fbogt_not", "FBOGT not taken (lt)", "fbogt", false, pool42_10))

	// 4. FBOLT: lt → taken; gt → not taken
	cases = append(cases, fbccLT("fpu_cond_fbolt_taken", "FBOLT taken (lt)", "fbolt", true, pool42_10))
	cases = append(cases, fbccGT("fpu_cond_fbolt_not", "FBOLT not taken (gt)", "fbolt", false, pool42_10))

	// 5. FBOGE: gt → taken; eq → taken
	cases = append(cases, fbccGT("fpu_cond_fboge_gt", "FBOGE taken (gt)", "fboge", true, pool42_10))
	cases = append(cases, fbccEQ("fpu_cond_fboge_eq", "FBOGE taken (eq)", "fboge", true, pool42))

	// 6. FBOLE: lt → taken; eq → taken
	cases = append(cases, fbccLT("fpu_cond_fbole_lt", "FBOLE taken (lt)", "fbole", true, pool42_10))
	cases = append(cases, fbccEQ("fpu_cond_fbole_eq", "FBOLE taken (eq)", "fbole", true, pool42))

	// 7. FBOR (ordered): gt → taken (ordered)
	cases = append(cases, fbccGT("fpu_cond_fbor_taken", "FBOR taken (ordered)", "fbor", true, pool42_10))

	// 8. FBUN (unordered): NaN → taken
	cases = append(cases, regonly(s, "fpu_cond_fbun_nan", "FBUN taken (NaN)",
		"FCMP NaN then FBUN", "D0=$00000001",
		"d0", 0x00000001,
		[]string{
			"fmove.d .fp_nan(pc),fp0",
			"fmove.d .fp_42(pc),fp1",
			"fcmp.x  fp1,fp0",
		},
		[]string{
			"fbun    .taken_fbun_nan",
			"moveq   #0,d0",
			"bra.s   .done_fbun_nan",
			".taken_fbun_nan:",
			"moveq   #1,d0",
			".done_fbun_nan:",
		}, fp("nan", "42")...))

	// FBUN: ordered → not taken
	cases = append(cases, fbccGT("fpu_cond_fbun_not", "FBUN not taken (ordered)", "fbun", false, pool42_10))

	// -----------------------------------------------------------------------
	// FScc — set byte on FPU condition
	// Encoding: $F240 | Dn, ext word = $0000 | condition
	// FScc Dn: if condition TRUE → Dn(low byte)=$FF, else $00
	// -----------------------------------------------------------------------

	// FScc helper
	fscc := func(id, name string, condCode uint16, setupFP []string, expectByte uint32, pool []string) testCase {
		return testCase{
			ID: id, Shard: s, Kind: kindInt,
			Name: name, Input: name,
			Expected:   "D0(low byte)",
			ActualMode: "regonly",
			ExpectReg:  "d0", ExpectValue: expectByte,
			Setup: append([]string{"moveq   #0,d0"}, setupFP...),
			Body: []string{
				// FScc D0: $F240, ext = condition
				"dc.w    $F240," + fmtHex16(condCode),
				"andi.l  #$000000FF,d0",
			},
			DataPool: pool,
		}
	}

	eqSetup := []string{
		"fmove.d .fp_42(pc),fp0",
		"fmove.d .fp_42(pc),fp1",
		"fcmp.x  fp1,fp0",
	}
	gtSetup := []string{
		"fmove.d .fp_42(pc),fp0",
		"fmove.d .fp_10(pc),fp1",
		"fcmp.x  fp1,fp0",
	}

	// 9. FSEQ: eq → $FF
	cases = append(cases, fscc("fpu_cond_fseq_true", "FSEQ D0 (eq->$FF)", 0x0001, eqSetup, 0xFF, pool42))
	// 10. FSEQ: gt → $00
	cases = append(cases, fscc("fpu_cond_fseq_false", "FSEQ D0 (gt->$00)", 0x0001, gtSetup, 0x00, pool42_10))
	// 11. FSNE: gt → $FF
	cases = append(cases, fscc("fpu_cond_fsne_true", "FSNE D0 (gt->$FF)", 0x000E, gtSetup, 0xFF, pool42_10))
	// 12. FSNE: eq → $00
	cases = append(cases, fscc("fpu_cond_fsne_false", "FSNE D0 (eq->$00)", 0x000E, eqSetup, 0x00, pool42))
	// 13. FSOGT: gt → $FF
	cases = append(cases, fscc("fpu_cond_fsogt_true", "FSOGT D0 (gt->$FF)", 0x0002, gtSetup, 0xFF, pool42_10))
	// 14. FSOLT: gt → $00
	cases = append(cases, fscc("fpu_cond_fsolt_false", "FSOLT D0 (gt->$00)", 0x0004, gtSetup, 0x00, pool42_10))
	// 15. FST: always → $FF
	cases = append(cases, fscc("fpu_cond_fst_true", "FST D0 (always->$FF)", 0x000F, gtSetup, 0xFF, pool42_10))
	// 16. FSF: never → $00
	cases = append(cases, fscc("fpu_cond_fsf_false", "FSF D0 (never->$00)", 0x0000, gtSetup, 0x00, pool42_10))

	// -----------------------------------------------------------------------
	// FDBcc — decrement and branch on FPU condition
	// Encoding: $F248 | Dn, ext = condition, then 16-bit displacement
	// If condition TRUE → no action. If FALSE → Dn-1, if Dn=-1 fall through, else branch.
	// -----------------------------------------------------------------------

	// 17. FDBEQ: condition false (gt), D1=2 → loops 3 times, D0 counts iterations
	// All FDBcc tests are musashiSkip: Musashi doesn't implement FDBcc (only FScc mode 0/5)
	cases = append(cases, musashiSkip(regonly(s, "fpu_cond_fdbeq_loop", "FDBEQ loop count=3",
		"FDBEQ D1,.loop (gt, D1=2, counts 3 iterations)", "D0=$00000003",
		"d0", 0x00000003,
		[]string{
			"fmove.d .fp_42(pc),fp0",
			"fmove.d .fp_10(pc),fp1",
			"fcmp.x  fp1,fp0", // gt → condition EQ is false
			"moveq   #0,d0",
			"moveq   #2,d1", // count = 2, loops 3 times (2,1,0 then -1 exits)
		},
		[]string{
			".loop_fdbeq:",
			"addq.l  #1,d0",
			"dc.w    $F249,$0001", // FDBEQ D1, displacement
			"dc.w    $FFFC",       // displacement = target - (opcode_addr+2) = -4
		}, fp("42", "10")...)))

	// 18. FDBEQ: condition true (eq) → exits immediately, no loop
	cases = append(cases, musashiSkip(regonly(s, "fpu_cond_fdbeq_exit", "FDBEQ exit (eq)",
		"FDBEQ D1,.loop (eq, exits immediately)", "D0=$00000001",
		"d0", 0x00000001,
		[]string{
			"fmove.d .fp_42(pc),fp0",
			"fmove.d .fp_42(pc),fp1",
			"fcmp.x  fp1,fp0", // eq → condition EQ is true
			"moveq   #0,d0",
			"moveq   #5,d1",
		},
		[]string{
			".loop_fdbeq2:",
			"addq.l  #1,d0",
			"dc.w    $F249,$0001", // FDBEQ D1
			"dc.w    $FFFC",       // displacement = target - (opcode_addr+2) = -4
		}, fp("42")...)))

	// 19. FDBNE: condition false (eq), D1=1 → loops 2 times
	cases = append(cases, musashiSkip(regonly(s, "fpu_cond_fdbne_loop", "FDBNE loop count=2",
		"FDBNE D1,.loop (eq, D1=1)", "D0=$00000002",
		"d0", 0x00000002,
		[]string{
			"fmove.d .fp_42(pc),fp0",
			"fmove.d .fp_42(pc),fp1",
			"fcmp.x  fp1,fp0", // eq → NE is false
			"moveq   #0,d0",
			"moveq   #1,d1",
		},
		[]string{
			".loop_fdbne:",
			"addq.l  #1,d0",
			"dc.w    $F249,$000E", // FDBNE D1
			"dc.w    $FFFC",       // displacement = target - (opcode_addr+2) = -4
		}, fp("42")...)))

	// 20. FDBT: condition T always true → never loops, exits immediately
	cases = append(cases, musashiSkip(regonly(s, "fpu_cond_fdbt_exit", "FDBT always exits",
		"FDBT D1,.loop (T, always exits)", "D0=$00000001",
		"d0", 0x00000001,
		[]string{
			"moveq   #0,d0",
			"moveq   #10,d1",
		},
		[]string{
			".loop_fdbt:",
			"addq.l  #1,d0",
			"dc.w    $F249,$000F", // FDBT D1
			"dc.w    $FFFC",       // displacement = target - (opcode_addr+2) = -4
		})))

	// 21. FDBF: condition F always false → pure count loop, D1=0 → loops 1 time
	cases = append(cases, musashiSkip(regonly(s, "fpu_cond_fdbf_count", "FDBF count-only loop",
		"FDBF D1,.loop (F, D1=0, loops 1 time)", "D0=$00000001",
		"d0", 0x00000001,
		[]string{
			"moveq   #0,d0",
			"moveq   #0,d1", // 0 → decrement to -1 → fall through after 1 loop
		},
		[]string{
			".loop_fdbf:",
			"addq.l  #1,d0",
			"dc.w    $F249,$0000", // FDBF D1
			"dc.w    $FFFC",       // displacement = target - (opcode_addr+2) = -4
		})))

	return shard{Name: s, Title: "FPU Conditionals", Cases: cases}
}

func shardFPUFormats() shard {
	s := "fpu_formats"
	fp := func(labels ...string) []string { return fpuPool(labels...) }
	var cases []testCase

	// -----------------------------------------------------------------------
	// FMOVE.L integer ↔ FP conversions
	// -----------------------------------------------------------------------

	// 1. FMOVE.L (A0),FP0 with [A0]=42 → FP0=42.0 ($4045000000000000)
	cases = append(cases, fpuCase(s, "fpu_fmt_fmove_l_to_fp", "FMOVE.L (A0),FP0 int->fp",
		"FMOVE.L (A0),FP0 [A0]=42", "FP0=42.0",
		0x40450000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{"lea     .int_42(pc),a0"},
		[]string{"fmove.l (a0),fp0"},
		".int_42:", "                dc.l    42", "                even"))

	// 2. FMOVE.L FP0,(A0) — store FP as integer, then readback
	// FP0=42.0 → [A0]=42
	cases = append(cases, regonly(s, "fpu_fmt_fmove_fp_to_l", "FMOVE.L FP0,(A0) fp->int",
		"FMOVE.L FP0,(A0) FP0=42.0", "D0=$0000002A",
		"d0", 0x0000002A,
		[]string{
			"fmove.d .fp_42(pc),fp0",
			"lea     .int_buf(pc),a0",
		},
		[]string{
			"fmove.l fp0,(a0)",
			"move.l  (a0),d0",
		},
		fp("42")...,
	))

	// Need an int buffer for write-back tests
	intBuf := []string{".int_buf:", "                dc.l    $00000000", "                even"}

	// 3. FMOVE.L (A0),FP0 with [A0]=0 → FP0=0.0 (Z flag)
	cases = append(cases, fpuCase(s, "fpu_fmt_fmove_l_zero", "FMOVE.L (A0),FP0 zero",
		"FMOVE.L (A0),FP0 [A0]=0", "FP0=0.0 Z=1",
		0x00000000, 0x00000000, 0x0F000000, 0x04000000,
		[]string{"lea     .int_zero(pc),a0"},
		[]string{"fmove.l (a0),fp0"},
		".int_zero:", "                dc.l    0", "                even"))

	// 4. FMOVE.L (A0),FP0 with [A0]=-1 → FP0=-1.0 (N flag)
	cases = append(cases, fpuCase(s, "fpu_fmt_fmove_l_neg", "FMOVE.L (A0),FP0 negative",
		"FMOVE.L (A0),FP0 [A0]=-1", "FP0=-1.0 N=1",
		0xBFF00000, 0x00000000, 0x0F000000, 0x08000000,
		[]string{"lea     .int_neg1(pc),a0"},
		[]string{"fmove.l (a0),fp0"},
		".int_neg1:", "                dc.l    $FFFFFFFF", "                even"))

	// -----------------------------------------------------------------------
	// FMOVE.D (double precision) round-trips
	// -----------------------------------------------------------------------

	// 5. FMOVE.D (A0),FP0: load 42.0 as double
	cases = append(cases, fpuCase(s, "fpu_fmt_fmoved_load", "FMOVE.D (A0),FP0",
		"FMOVE.D (A0),FP0 [A0]=42.0", "FP0=42.0",
		0x40450000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{"lea     .fp_42(pc),a0"},
		[]string{"fmove.d (a0),fp0"},
		fp("42")...))

	// 6. FMOVE.D FP0,(A0) then FMOVE.D (A0),FP1 — round-trip preserves value
	// Store 42.0 to memory, load back into FP1, compare
	cases = append(cases, fpuCase(s, "fpu_fmt_fmoved_roundtrip", "FMOVE.D round-trip",
		"FMOVE.D FP0,(A0) then FMOVE.D (A0),FP0", "FP0=42.0 (preserved)",
		0x40450000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{
			"fmove.d .fp_42(pc),fp0",
			"lea     .dbl_buf(pc),a0",
		},
		[]string{
			"fmove.d fp0,(a0)",         // store
			"fmove.d .fp_zero(pc),fp0", // clobber
			"fmove.d (a0),fp0",         // reload
		},
		append(fp("42", "zero"), ".dbl_buf:", "                dc.l    $00000000,$00000000", "                even")...))

	// -----------------------------------------------------------------------
	// FMOVE.S (single precision)
	// -----------------------------------------------------------------------

	// 7. FMOVE.S (A0),FP0: load single-precision 42.0 ($42280000)
	cases = append(cases, fpuCase(s, "fpu_fmt_fmoves_load", "FMOVE.S (A0),FP0",
		"FMOVE.S (A0),FP0 [A0]=42.0f", "FP0=42.0",
		0x40450000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{"lea     .sgl_42(pc),a0"},
		[]string{"fmove.s (a0),fp0"},
		".sgl_42:", "                dc.l    $42280000", "                even"))

	// 8. FMOVE.S FP0,(A0) then FMOVE.S (A0),FP0 — single round-trip
	cases = append(cases, fpuCase(s, "fpu_fmt_fmoves_roundtrip", "FMOVE.S round-trip",
		"FMOVE.S store and reload 42.0", "FP0=42.0 (single precision)",
		0x40450000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{
			"fmove.d .fp_42(pc),fp0",
			"lea     .sgl_buf(pc),a0",
		},
		[]string{
			"fmove.s fp0,(a0)",
			"fmove.d .fp_zero(pc),fp0",
			"fmove.s (a0),fp0",
		},
		append(fp("42", "zero"), ".sgl_buf:", "                dc.l    $00000000", "                even")...))

	// -----------------------------------------------------------------------
	// FMOVE.L integer round-trip: store FP as .L, load back, verify
	// -----------------------------------------------------------------------

	// 9. Round-trip: FP0=100.0 → FMOVE.L FP0,(A0) → [A0]=100 → FMOVE.L (A0),FP0 → 100.0
	cases = append(cases, fpuCase(s, "fpu_fmt_int_roundtrip", "FMOVE.L int round-trip",
		"FP0=100.0->int->fp", "FP0=100.0",
		0x40590000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{
			"fmove.d .fp_100(pc),fp0",
			"lea     .int_rt_buf(pc),a0",
		},
		[]string{
			"fmove.l fp0,(a0)",
			"fmove.d .fp_zero(pc),fp0",
			"fmove.l (a0),fp0",
		},
		append(fp("100", "zero"), ".int_rt_buf:", "                dc.l    $00000000", "                even")...))

	// -----------------------------------------------------------------------
	// FMOVE.W (word) format
	// -----------------------------------------------------------------------

	// 10. FMOVE.W (A0),FP0: load word 100 → FP0=100.0
	cases = append(cases, fpuCase(s, "fpu_fmt_fmovew_load", "FMOVE.W (A0),FP0",
		"FMOVE.W (A0),FP0 [A0]=100", "FP0=100.0",
		0x40590000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{"lea     .word_100(pc),a0"},
		[]string{"fmove.w (a0),fp0"},
		".word_100:", "                dc.w    100", "                even"))

	// 11. FMOVE.W (A0),FP0: negative word -1 → FP0=-1.0
	cases = append(cases, fpuCase(s, "fpu_fmt_fmovew_neg", "FMOVE.W (A0),FP0 neg",
		"FMOVE.W (A0),FP0 [A0]=-1", "FP0=-1.0",
		0xBFF00000, 0x00000000, 0x0F000000, 0x08000000,
		[]string{"lea     .word_neg1(pc),a0"},
		[]string{"fmove.w (a0),fp0"},
		".word_neg1:", "                dc.w    $FFFF", "                even"))

	// -----------------------------------------------------------------------
	// FMOVE.B (byte) format
	// -----------------------------------------------------------------------

	// 12. FMOVE.B (A0),FP0: load byte 10 → FP0=10.0
	cases = append(cases, fpuCase(s, "fpu_fmt_fmoveb_load", "FMOVE.B (A0),FP0",
		"FMOVE.B (A0),FP0 [A0]=10", "FP0=10.0",
		0x40240000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{"lea     .byte_10(pc),a0"},
		[]string{"fmove.b (a0),fp0"},
		".byte_10:", "                dc.b    10,0", "                even"))

	// -----------------------------------------------------------------------
	// FMOVECR special constants
	// -----------------------------------------------------------------------

	// 13. FMOVECR #$00 = pi (already tested in fpu_data, but include for completeness)
	cases = append(cases, fpuCase(s, "fpu_fmt_fmovecr_pi", "FMOVECR pi",
		"FMOVECR #$00,FP0", "FP0=pi",
		0x400921FB, 0x54442D18, 0x0F000000, 0x00000000,
		nil, []string{"fmovecr #$00,fp0"}))

	// 14. FMOVECR #$0B = log10(2) ≈ 0.30103
	cases = append(cases, fpuCase(s, "fpu_fmt_fmovecr_log10_2", "FMOVECR log10(2)",
		"FMOVECR #$0B,FP0", "FP0=log10(2)",
		0x3FD34413, 0x509F79FF, 0x0F000000, 0x00000000,
		nil, []string{"fmovecr #$0B,fp0"}))

	// 15. FMOVECR #$30 = ln(2) ≈ 0.693147
	cases = append(cases, fpuCase(s, "fpu_fmt_fmovecr_ln2", "FMOVECR ln(2)",
		"FMOVECR #$30,FP0", "FP0=ln(2)",
		0x3FE62E42, 0xFEFA39EF, 0x0F000000, 0x00000000,
		nil, []string{"fmovecr #$30,fp0"}))

	// 16. FMOVECR #$33 = 10^1 = 10.0
	cases = append(cases, fpuCase(s, "fpu_fmt_fmovecr_10", "FMOVECR 10^1",
		"FMOVECR #$33,FP0", "FP0=10.0",
		0x40240000, 0x00000000, 0x0F000000, 0x00000000,
		nil, []string{"fmovecr #$33,fp0"}))

	// -----------------------------------------------------------------------
	// Special value operations
	// -----------------------------------------------------------------------

	// 17. FCMP with NaN → unordered (NAN bit in FPSR)
	cases = append(cases, testCase{
		ID: "fpu_fmt_fcmp_nan_unordered", Shard: s, Kind: kindInt,
		Name: "FCMP NaN unordered", Input: "FCMP NaN,42 -> NAN bit",
		Expected:   "FPSR(NaN)=$01000000",
		ActualMode: "fpsr_only",
		FPSRMask:   0x01000000, ExpectFPSR: 0x01000000,
		Setup: []string{
			"fmove.d .fp_nan(pc),fp0",
			"fmove.d .fp_42(pc),fp1",
		},
		Body:     []string{"fcmp.x  fp1,fp0"},
		DataPool: fp("nan", "42"),
	})

	// 18. FSQRT(0) = +0
	cases = append(cases, fpuCase(s, "fpu_fmt_fsqrt_zero", "FSQRT(0)=+0",
		"FSQRT FP0 (FP0=0)", "FP0=0.0 Z=1",
		0x00000000, 0x00000000, 0x0F000000, 0x04000000,
		[]string{"fmove.d .fp_zero(pc),fp0"},
		[]string{"fsqrt.x fp0,fp0"},
		fp("zero")...))

	// 19. FABS(-42) = 42
	cases = append(cases, fpuCase(s, "fpu_fmt_fabs_neg", "FABS(-42)=42",
		"FABS FP0 (FP0=-42)", "FP0=42.0",
		0x40450000, 0x00000000, 0x0F000000, 0x00000000,
		[]string{"fmove.d .fp_neg_42(pc),fp0"},
		[]string{"fabs.x  fp0,fp0"},
		fp("neg_42")...))

	// 20. FNEG(42) = -42
	cases = append(cases, fpuCase(s, "fpu_fmt_fneg_pos", "FNEG(42)=-42",
		"FNEG FP0 (FP0=42)", "FP0=-42.0 N=1",
		0xC0450000, 0x00000000, 0x0F000000, 0x08000000,
		[]string{"fmove.d .fp_42(pc),fp0"},
		[]string{"fneg.x  fp0,fp0"},
		fp("42")...))

	// Add the int_buf pool for case 2
	cases[1].DataPool = append(cases[1].DataPool, intBuf...)

	return shard{Name: s, Title: "FPU Formats", Cases: cases}
}

// fmtHex16 formats a uint16 as $XXXX for dc.w emission.
func fmtHex16(v uint16) string {
	return fmt.Sprintf("$%04X", v)
}
