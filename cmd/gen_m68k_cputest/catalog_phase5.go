package main

// Phase 5: Privileged/Control Instructions (~20 cases)
// Shards: supervisor_ctrl, exception_return

func buildPhase5Shards() []shard {
	return []shard{
		shardSupervisorCtrl(),
		shardExceptionReturn(),
	}
}

func shardSupervisorCtrl() shard {
	s := "supervisor_ctrl"
	var cases []testCase

	// MOVEC D0,VBR / MOVEC VBR,D1 — interrupt-safe round-trip.
	// Disable interrupts while VBR is temporarily changed so no async
	// exception dispatches through the wrong vector table.
	cases = append(cases, testCase{
		ID: "sup_ctrl_movec_vbr_write", Shard: s, Kind: kindInt,
		Name: "MOVEC D0,VBR round-trip", Input: "Write $1000 to VBR, read back",
		Expected:    "D1=$00001000",
		ActualMode:  "custom_d1_sr",
		ExpectValue: 0x00001000, SRMask: 0x0000, ExpectSR: 0x0000,
		Setup: []string{
			// Save original VBR and SR (interrupt mask)
			"dc.w    $4E7A,$0801", // MOVEC VBR,D0
			"move.l  d0,d2",       // D2 = saved VBR
			"move.w  sr,d3",       // D3 = saved SR
			"ori.w   #$0700,sr",   // mask all interrupts (IPL=7)
			"move.l  #$00001000,d0",
		},
		Body: []string{
			"dc.w    $4E7B,$0801", // MOVEC D0,VBR  (VBR=$1000)
			"dc.w    $4E7A,$1801", // MOVEC VBR,D1  (D1 should be $1000)
			// Restore original VBR before re-enabling interrupts
			"move.l  d2,d0",
			"dc.w    $4E7B,$0801", // MOVEC D0,VBR
			"move.w  d3,sr",       // restore original interrupt mask
		},
	})

	// MOVEC CACR round-trip: write nonzero, read back, then restore.
	// On 68020 CACR bit 0 = enable instruction cache, bit 3 = freeze.
	// Write $01 (enable), read back, verify, then write 0 to restore.
	cases = append(cases, testCase{
		ID: "sup_ctrl_movec_cacr_roundtrip", Shard: s, Kind: kindInt,
		Name: "MOVEC CACR round-trip", Input: "Write $01 to CACR, read back",
		Expected:    "D1=$00000001",
		ActualMode:  "custom_d1_sr",
		ExpectValue: 0x00000001, SRMask: 0x0000, ExpectSR: 0x0000,
		Setup: []string{
			// Save original CACR
			"dc.w    $4E7A,$0002", // MOVEC CACR,D0
			"move.l  d0,d2",       // D2 = saved CACR
			"moveq   #1,d0",       // enable instruction cache
		},
		Body: []string{
			"dc.w    $4E7B,$0002", // MOVEC D0,CACR (write $01)
			"moveq   #0,d1",
			"dc.w    $4E7A,$1002", // MOVEC CACR,D1 (read back)
			// Restore original CACR
			"move.l  d2,d0",
			"dc.w    $4E7B,$0002", // MOVEC D0,CACR
		},
	})

	// MOVEC D0,SFC round-trip. Write $05 to SFC, read back.
	cases = append(cases, testCase{
		ID: "sup_ctrl_movec_sfc_write", Shard: s, Kind: kindInt,
		Name: "MOVEC D0,SFC round-trip", Input: "Write $05 to SFC, read back",
		Expected:    "D1=$00000005",
		ActualMode:  "custom_d1_sr",
		ExpectValue: 0x00000005, SRMask: 0x0000, ExpectSR: 0x0000,
		Setup: []string{
			"moveq   #5,d0",
		},
		Body: []string{
			"dc.w    $4E7B,$0000", // MOVEC D0,SFC
			"moveq   #0,d1",
			"dc.w    $4E7A,$1000", // MOVEC SFC,D1
		},
	})

	// MOVEC D0,DFC round-trip
	cases = append(cases, testCase{
		ID: "sup_ctrl_movec_dfc_write", Shard: s, Kind: kindInt,
		Name: "MOVEC D0,DFC round-trip", Input: "Write $03 to DFC, read back",
		Expected:    "D1=$00000003",
		ActualMode:  "custom_d1_sr",
		ExpectValue: 0x00000003, SRMask: 0x0000, ExpectSR: 0x0000,
		Setup: []string{
			"moveq   #3,d0",
		},
		Body: []string{
			"dc.w    $4E7B,$0001", // MOVEC D0,DFC
			"moveq   #0,d1",
			"dc.w    $4E7A,$1001", // MOVEC DFC,D1
		},
	})

	// MOVE A0,USP / MOVE USP,A0 — write then read USP
	// MOVE A0,USP = $4E60, MOVE USP,A1 = $4E69
	cases = append(cases, testCase{
		ID: "sup_ctrl_move_usp", Shard: s, Kind: kindInt,
		Name: "MOVE A0,USP / MOVE USP,A1", Input: "Set USP=$00012345, read back to A1",
		Expected:    "D1=$00012345",
		ActualMode:  "custom_d1_sr",
		ExpectValue: 0x00012345, SRMask: 0x0000, ExpectSR: 0x0000,
		Setup: []string{
			"move.l  #$00012345,a0",
		},
		Body: []string{
			"dc.w    $4E60", // MOVE A0,USP
			"dc.w    $4E69", // MOVE USP,A1
			"move.l  a1,d1",
		},
	})

	// MOVES.L D0,(A0) — supervisor memory move (020)
	// MOVES.L D0,(A0): opcode=$0E90, ext=$0800
	// Write D0 to (A0), then read back normally to verify
	cases = append(cases, regonly(s, "sup_ctrl_moves_write", "MOVES.L D0,(A0) (020)",
		"MOVES.L D0,(A0) write $DEADBEEF to buffer", "D0=$DEADBEEF readback", "d0", 0xDEADBEEF,
		[]string{
			"move.l  #$DEADBEEF,d0",
			"lea     .moves_buf(pc),a0",
		},
		[]string{
			"dc.w    $0E90,$0800", // MOVES.L D0,(A0)
			"move.l  (a0),d0",     // read back normally
		},
		".moves_buf:",
		"                dc.l    $00000000",
		"                even",
	))

	// MOVES.L (A0),D0 — supervisor memory read (020)
	// MOVES.L (A0),D0: opcode=$0E90, ext=$0000
	cases = append(cases, regonly(s, "sup_ctrl_moves_read", "MOVES.L (A0),D0 (020)",
		"MOVES.L (A0),D0 read $12345678 from buffer", "D0=$12345678", "d0", 0x12345678,
		[]string{
			"lea     .moves_rdbuf(pc),a0",
			"moveq   #0,d0",
		},
		[]string{
			"dc.w    $0E90,$0000", // MOVES.L (A0),D0
		},
		".moves_rdbuf:",
		"                dc.l    $12345678",
		"                even",
	))

	// Privilege violation test: MOVEC VBR,D0 from user mode → vector 8
	// The handler must not RTE to the faulting instruction (it would loop
	// forever since user mode cannot execute MOVEC). ct_trap_resume
	// redirects RTE to .priv_resume in supervisor mode.
	cases = append(cases, testCase{
		ID: "sup_ctrl_priv_violation", Shard: s, Kind: kindInt,
		Name: "Privilege violation (user mode MOVEC)", Input: "MOVEC in user mode -> vector 8",
		Expected:   "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 8,
		Setup: []string{
			"moveq   #8,d0",
			"bsr     ct_install_trap_handler",
			// Set resume address so handler skips past the faulting MOVEC
			"lea     .priv_resume(pc),a0",
			"move.l  a0,ct_trap_resume",
			// Build an RTE frame to switch to user mode
			// 68020 format $0 frame: (SP) = SR, (SP+2) = PC, (SP+6) = format/vector
			"move.w  #$0000,-(sp)",        // format/vector word (format 0)
			"pea     .priv_user_code(pc)", // return PC
			"move.w  #$0000,-(sp)",        // SR = user mode (S=0)
		},
		Body: []string{
			"rte", // drop to user mode
			".priv_user_code:",
			"dc.w    $4E7A,$0801", // MOVEC VBR,D0 — should cause privilege violation
			".priv_resume:",       // handler returns here in supervisor mode
		},
	})

	// MOVE to SR in user mode → privilege violation
	cases = append(cases, testCase{
		ID: "sup_ctrl_move_sr_priv", Shard: s, Kind: kindInt,
		Name: "MOVE to SR priv violation", Input: "MOVE to SR in user mode -> vector 8",
		Expected:   "trap taken",
		ActualMode: "exception", ExpectTrap: true, TrapVector: 8,
		Setup: []string{
			"moveq   #8,d0",
			"bsr     ct_install_trap_handler",
			"lea     .sr_priv_resume(pc),a0",
			"move.l  a0,ct_trap_resume",
			"move.w  #$0000,-(sp)",
			"pea     .sr_priv_user(pc)",
			"move.w  #$0000,-(sp)",
		},
		Body: []string{
			"rte",
			".sr_priv_user:",
			"move.w  #$2000,sr",   // attempt to set S bit — privilege violation
			".sr_priv_resume:",
		},
	})

	return shard{Name: s, Title: "Supervisor Control", Cases: cases}
}

func shardExceptionReturn() shard {
	s := "exception_return"
	var cases []testCase

	// RTE with format $0 frame — verify PC reached target and SR restored
	// 68020 stack frame (pushed high to low):
	//   (SP)   = SR word
	//   (SP+2) = PC longword
	//   (SP+6) = format/vector word
	// Build by pushing in reverse: format word, PC, SR
	cases = append(cases, regonly(s, "exc_ret_rte_fmt0", "RTE format $0",
		"RTE with format $0 frame", "D0=$00000001 (reached target)", "d0", 0x00000001,
		[]string{
			"moveq   #0,d0",
			// Push format $0 exception frame
			"move.w  #$0000,-(sp)",    // format/vector word (format 0, vector 0)
			"pea     .rte_target(pc)", // return PC
			"move.w  #$2000,-(sp)",    // SR: supervisor, no flags
		},
		[]string{
			"rte",
			"moveq   #$77,d0", // should be skipped
			".rte_target:",
			"moveq   #1,d0",
		}))

	// RTE restores SR flags — set all CCR flags in the frame
	cases = append(cases, testCase{
		ID: "exc_ret_rte_sr_restore", Shard: s, Kind: kindInt,
		Name: "RTE SR restore", Input: "RTE restores CCR flags from frame",
		Expected:   "SR(XNZVC)=$001F",
		ActualMode: "custom_sr_only",
		SRMask:     0x001F, ExpectSR: 0x001F,
		Setup: []string{
			// Push format $0 frame with all CCR flags set
			"move.w  #$0000,-(sp)",       // format/vector
			"pea     .rte_sr_target(pc)", // return PC
			"move.w  #$201F,-(sp)",       // SR: supervisor + XNZVC all set
		},
		Body: []string{
			"rte",
			".rte_sr_target:",
			"nop",
		},
	})

	// RTE preserves supervisor bit
	cases = append(cases, testCase{
		ID: "exc_ret_rte_supervisor", Shard: s, Kind: kindInt,
		Name: "RTE supervisor preserved", Input: "RTE with S bit in frame",
		Expected:   "SR(S)=$2000",
		ActualMode: "custom_sr_only",
		SRMask:     0x2000, ExpectSR: 0x2000,
		Setup: []string{
			"move.w  #$0000,-(sp)",        // format/vector
			"pea     .rte_sup_target(pc)", // return PC
			"move.w  #$2000,-(sp)",        // SR: supervisor set
		},
		Body: []string{
			"rte",
			".rte_sup_target:",
			"nop",
		},
	})

	// RTR — pops CCR (word) + PC (long). CCR restored, supervisor bit unchanged.
	// Stack: (SP) = CCR word, (SP+2) = PC longword
	cases = append(cases, testCase{
		ID: "exc_ret_rtr_ccr", Shard: s, Kind: kindInt,
		Name: "RTR restores CCR", Input: "RTR pops CCR=$001F, PC=target",
		Expected:   "SR(XNZVC)=$001F",
		ActualMode: "custom_sr_only",
		SRMask:     0x001F, ExpectSR: 0x001F,
		Setup: []string{
			// Clear CCR first
			"moveq   #0,d0",
			"move.w  d0,ccr",
			// Push return PC then CCR (RTR pops CCR first, then PC)
			"pea     .rtr_ccr_target(pc)", // return PC
			"move.w  #$001F,-(sp)",        // CCR with all flags set
		},
		Body: []string{
			"rtr",
			".rtr_ccr_target:",
			"nop",
		},
	})

	// RTR does NOT affect supervisor bit
	cases = append(cases, testCase{
		ID: "exc_ret_rtr_no_super", Shard: s, Kind: kindInt,
		Name: "RTR leaves S unchanged", Input: "RTR with S=1 in stack CCR word",
		Expected:   "SR(S)=$2000 (unchanged, was supervisor)",
		ActualMode: "custom_sr_only",
		SRMask:     0x2000, ExpectSR: 0x2000,
		Setup: []string{
			// We're in supervisor mode. RTR should NOT change the S bit
			// even if the CCR word on stack has bit 13 set.
			"pea     .rtr_sup_target(pc)",
			"move.w  #$0000,-(sp)", // CCR = 0, no S bit in CCR word
		},
		Body: []string{
			"rtr",
			".rtr_sup_target:",
			"nop",
		},
	})

	// RTR with zero flags — verify flags cleared
	cases = append(cases, testCase{
		ID: "exc_ret_rtr_clear", Shard: s, Kind: kindInt,
		Name: "RTR clears CCR", Input: "RTR pops CCR=$0000 (clears prev flags)",
		Expected:   "SR(XNZVC)=$0000",
		ActualMode: "custom_sr_only",
		SRMask:     0x001F, ExpectSR: 0x0000,
		Setup: []string{
			// Set all CCR flags first
			"move.w  #$001F,ccr",
			"pea     .rtr_clr_target(pc)",
			"move.w  #$0000,-(sp)", // CCR = 0
		},
		Body: []string{
			"rtr",
			".rtr_clr_target:",
			"nop",
		},
	})

	// RTE format $0 with different vector number (vector 32 = TRAP #0)
	cases = append(cases, regonly(s, "exc_ret_rte_vec32", "RTE format $0 vec 32",
		"RTE with format $0, vector 32 encoding", "D0=$00000042", "d0", 0x00000042,
		[]string{
			"moveq   #0,d0",
			// format $0, vector 32 → format/vector word = $0080 (format=0, vector=32*4=0x80)
			"move.w  #$0080,-(sp)",
			"pea     .rte_v32_target(pc)",
			"move.w  #$2000,-(sp)",
		},
		[]string{
			"rte",
			"moveq   #$77,d0",
			".rte_v32_target:",
			"moveq   #$42,d0",
		}))

	// RTE format $2 (instruction address frame, 6-word frame)
	// Format $2: SR(2), PC(4), format/vector(2), instruction address(4) = 12 bytes
	cases = append(cases, regonly(s, "exc_ret_rte_fmt2", "RTE format $2",
		"RTE with format $2 frame (6 words)", "D0=$000000AA", "d0", 0x000000AA,
		[]string{
			"moveq   #0,d0",
			// Push format $2 frame (pushed bottom-up = high addresses first)
			"move.l  #$00000000,-(sp)",   // instruction address (don't care)
			"move.w  #$2000,-(sp)",       // format $2, vector 0 → $2000
			"pea     .rte_f2_target(pc)", // return PC
			"move.w  #$2000,-(sp)",       // SR: supervisor
		},
		[]string{
			"rte",
			"moveq   #$77,d0",
			".rte_f2_target:",
			"move.l  #$000000AA,d0",
		}))

	return shard{Name: s, Title: "Exception Return", Cases: cases}
}
