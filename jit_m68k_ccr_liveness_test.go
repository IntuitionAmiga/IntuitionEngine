// jit_m68k_ccr_liveness_test.go - tightness gates for Phase 2b M68K
// CCR liveness analysis.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func mkM(op uint16) M68KJITInstr { return M68KJITInstr{opcode: op, group: uint8(op >> 12)} }

func TestM68KCCRLiveness_LatestMOVEBLive(t *testing.T) {
	// MOVE.B D0,D1 (0x1200) twice — only the second is live.
	live := m68kCCRLiveness([]M68KJITInstr{mkM(0x1200), mkM(0x1200)})
	if live[0] || !live[1] {
		t.Errorf("expected [false,true], got %v", live)
	}
}

func TestM68KCCRLiveness_BccConsumer(t *testing.T) {
	// MOVE.B D0,D1; BNE rel; MOVE.B D2,D3 — first MOVE consumed by BNE.
	// BNE = 0x66xx (cc=6), BSR = 0x61xx (cc=1, not consumer).
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x1200), // MOVE.B D0,D1
		mkM(0x6600), // BNE
		mkM(0x1400), // MOVE.B D0,D2
	})
	if !live[0] || !live[2] {
		t.Errorf("expected live[0] and live[2] true, got %v", live)
	}
}

func TestM68KCCRLiveness_CMPAddressRegisterBeforeBNE(t *testing.T) {
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x15B0), // MOVE.B 0(A0,A1.L),0(A2,A1.L)
		mkM(0x5389), // SUBQ.L #1,A1 (no CCR; address-register destination)
		mkM(0xB689), // CMP.L A1,D3
		mkM(0x66F4), // BNE.S back
	})
	if !live[2] {
		t.Fatalf("CMP.L A1,D3 before BNE must be live, got %v", live)
	}
}

func TestM68KCCRLiveness_BSRIsHiddenConsumer(t *testing.T) {
	// MOVE.B; BSR; MOVE.B — BSR reads no CCR, but it pushes the return
	// address (memory write with MMIO/SMC bail guards). A bail re-enters
	// the interpreter with the pre-BSR architectural CCR, so the first
	// MOVE.B must stay live. (Historically asserted dead; that was the
	// unsafe pre-whitelist model.)
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x1200), // MOVE.B
		mkM(0x6100), // BSR
		mkM(0x1400), // MOVE.B
	})
	if !live[0] {
		t.Errorf("MOVE.B before bail-capable BSR must stay live, got %v", live)
	}
}

func TestM68KCCRLiveness_BRANotConsumer(t *testing.T) {
	// MOVE.B; BRA; MOVE.B — first MOVE shadowed.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x1200),
		mkM(0x6000), // BRA
		mkM(0x1400),
	})
	if live[0] {
		t.Errorf("MOVE.B before BRA-only path should be dead, got %v", live)
	}
}

func TestM68KCCRLiveness_MOVEAIsNotProducer(t *testing.T) {
	// MOVEA.L D0,A1 — opcode 0x2240 (group 2, dst mode = 001 An).
	// MOVE.B D0,D1 (producer); MOVEA (no CCR); BNE — MOVE.B is live.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x1200), // MOVE.B
		mkM(0x2240), // MOVEA.L (no CCR effect)
		mkM(0x6600), // BNE
	})
	if !live[0] {
		t.Errorf("MOVE.B should be live (MOVEA does not shadow), got %v", live)
	}
}

func TestM68KCCRLiveness_MOVEQProducer(t *testing.T) {
	// MOVEQ #5,D0 = 0x7005. Group 7, bit 8 clear — producer.
	live := m68kCCRLiveness([]M68KJITInstr{mkM(0x7005), mkM(0x7005)})
	if live[0] || !live[1] {
		t.Errorf("MOVEQ shadow: expected [false,true], got %v", live)
	}
}

func TestM68KCCRLiveness_RTEOverwritesButStillObserves(t *testing.T) {
	// MOVE.B; RTE (0x4E73); BNE — RTE overwrites SR from the stack frame,
	// which kills downstream demand (BNE reads RTE's SR, not MOVE.B's).
	// But RTE's frame pop is a guarded memory read: a bail re-enters the
	// interpreter with the pre-RTE CCR, so the upstream MOVE.B must stay
	// live anyway. (Historically asserted dead; unsafe pre-whitelist
	// model.)
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x1200),
		mkM(0x4E73), // RTE
		mkM(0x6600), // BNE
	})
	if !live[0] {
		t.Errorf("MOVE.B before bail-capable RTE must stay live, got %v", live)
	}
}

func TestM68KCCRLiveness_UnknownOpcodeStaysLive(t *testing.T) {
	// JSR (0x4Eba etc.) is in group 4 — not in confident producer or
	// consumer set. Should be neither producer nor consumer; demand
	// passes through. MOVE.B before unknown JSR before BNE: MOVE.B is
	// live (BNE consumer demand reaches it).
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x1200), // MOVE.B
		mkM(0x4EB9), // JSR (xxx).L  (group 4, unknown to analyzer)
		mkM(0x6600), // BNE
	})
	if !live[0] {
		t.Errorf("MOVE.B should remain live across unknown JSR, got %v", live)
	}
}

func TestM68KCCRLiveness_ExtendedProducers(t *testing.T) {
	// Coverage gates for the broadened producer set.
	cases := []struct {
		name string
		op   uint16
	}{
		{"TST.W D0", 0x4A40},
		{"CMPI.W #imm,D0", 0x0C40},
		{"CLR.L D0", 0x4280},
		{"NEG.B D0", 0x4400},
		{"NOT.W D0", 0x4640},
		{"EXT.W D0", 0x4880},
		{"EXT.L D0", 0x48C0},
		{"ADDQ.W #1,D0", 0x5240},
		{"OR.W D0,D1", 0x8240},
		{"SUB.W D0,D1", 0x9240},
		{"CMP.W D0,D1", 0xB240},
		{"AND.W D0,D1", 0xC240},
		{"ADD.W D0,D1", 0xD240},
		{"ASL.W #1,D0", 0xE340},
		{"MOVE.B to CCR", 0x44C0},
		{"TRAPV", 0x4E76},
	}
	for _, c := range cases {
		writes, consumer, overwriter := m68kClassifyCCR(c.op)
		producer := writes != 0
		if c.op == 0x44C0 {
			// MOVE to CCR is an overwriter, not a producer.
			if !overwriter {
				t.Errorf("%s should be overwriter (got p=%v c=%v o=%v)", c.name, producer, consumer, overwriter)
			}
			continue
		}
		if c.op == 0x4E76 {
			if !consumer {
				t.Errorf("%s should be consumer (got p=%v c=%v o=%v)", c.name, producer, consumer, overwriter)
			}
			continue
		}
		if !producer {
			t.Errorf("%s should be producer (got p=%v c=%v o=%v)", c.name, producer, consumer, overwriter)
		}
	}
}

func TestM68KCCRLiveness_ArithProducerLiveAcrossLogical(t *testing.T) {
	// ADD.L D1,D0 (writes X+NZVC); AND.L D0,D0 (writes NZVC, preserves X).
	// AND shadows ADD's NZVC but NOT X — so ADD must be LIVE for the X
	// bit. This is the regression that motivated splitting CCR liveness
	// into X and NZVC demand.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xD081), // ADD.L D1,D0
		mkM(0xC080), // AND.L D0,D0
	})
	if !live[0] {
		t.Errorf("ADD before AND must be LIVE for X bit, got %v", live)
	}
	if !live[1] {
		t.Errorf("AND must be LIVE (block-exit consumer of NZVC), got %v", live)
	}
}

func TestM68KCCRLiveness_LogicalShadowedByLogical(t *testing.T) {
	// AND.L D0,D0 twice — both write NZVC only. Latest shadows prior.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xC080),
		mkM(0xC080),
	})
	if live[0] || !live[1] {
		t.Errorf("logical shadow: expected [false,true], got %v", live)
	}
}

func TestM68KCCRLiveness_XReadersAreConsumers(t *testing.T) {
	// Each X-reading instruction must be classified as a consumer so
	// upstream X-producers stay live. Encodings:
	//   NEGX.W D0      = 0x4040
	//   ADDX.W D1,D0   = 0xD141 (group D opmode 4 src-Dn-Dn pattern)
	//   SUBX.W D1,D0   = 0x9141
	//   ABCD D1,D0     = 0xC101 (group C, X-reader)
	//   SBCD D1,D0     = 0x8101 (group 8, X-reader)
	//   ROXL.W #1,D0   = 0xE350 (group E, register-form rtype=2)
	cases := []struct {
		name string
		op   uint16
	}{
		{"NEGX.W D0", 0x4040},
		{"ADDX.W D1,D0", 0xD141},
		{"SUBX.W D1,D0", 0x9141},
		{"ABCD D1,D0", 0xC101},
		{"SBCD D1,D0", 0x8101},
		{"ROXL.W #1,D0", 0xE350},
	}
	for _, c := range cases {
		_, consumer, _ := m68kClassifyCCR(c.op)
		if !consumer {
			t.Errorf("%s (opcode %#04X) must be classified as consumer (reads X)", c.name, c.op)
		}
	}
	// Liveness regression: ADD.L D1,D0; ROXL.W #1,D0; ADD.L D1,D0.
	// Without X-reader-as-consumer the trailing ADD shadows the first
	// ADD's CCR; with the rule, ROXL's X-read keeps the first ADD
	// live.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xD081), // ADD.L D1,D0
		mkM(0xE350), // ROXL.W #1,D0 (X-reader)
		mkM(0xD081), // ADD.L D1,D0
	})
	if !live[0] {
		t.Errorf("ADD before X-reader ROXL must remain live, got %v", live)
	}
}

func TestM68KCCRLiveness_BailCapableConsumerKeepsUpstreamLive(t *testing.T) {
	// ADD.L D1,D0 (producer, X+NZVC); MOVE.L (A0),D2 (memory load —
	// can bail to interpreter on MMIO/alignment); ADD.L D1,D0
	// (overwriter producer). Without the bail-as-consumer rule the
	// final producer would shadow the first → live[0]=false. With the
	// rule, the MOVE.L (A0),D2 reasserts demand for both X and NZVC
	// because the bail epilogue surfaces guest CCR to the interpreter.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xD081), // ADD.L D1,D0
		mkM(0x2410), // MOVE.L (A0),D2 — group 2 src mode (An), bail-capable
		mkM(0xD081), // ADD.L D1,D0
	})
	if !live[0] {
		t.Errorf("ADD before bail-capable MOVE must remain live (bail epilogue is hidden CCR consumer), got %v", live)
	}
}

func TestM68KCCRLiveness_ADDQToAnIsNotProducer(t *testing.T) {
	// CMP.W D0,D1 (0xB240); ADDQ.W #1,A0 (0x5248 — dst mode 1 An);
	// BNE rel (0x6600). ADDQ to An does NOT modify CCR per M68K
	// reference. Liveness must NOT shadow the CMP — BNE consumes
	// CMP's NZVC.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xB240), // CMP.W D0,D1
		mkM(0x5248), // ADDQ.W #1,A0 — dst mode 001
		mkM(0x6600), // BNE
	})
	if !live[0] {
		t.Errorf("CMP.W must remain live across ADDQ to An (no CCR), got %v", live)
	}

	// SUBQ.W #1,A0 (0x5348 — same dst mode pattern). Must also not
	// shadow upstream CMP.
	live = m68kCCRLiveness([]M68KJITInstr{
		mkM(0xB240),
		mkM(0x5348),
		mkM(0x6600),
	})
	if !live[0] {
		t.Errorf("CMP.W must remain live across SUBQ to An (no CCR), got %v", live)
	}

	// Sanity: ADDQ.W #1,D0 (0x5240 — dst mode 0 Dn) IS a producer and
	// should shadow upstream same-shape producer.
	w, _, _ := m68kClassifyCCR(0x5240)
	if w == 0 {
		t.Errorf("ADDQ.W #1,D0 must be classified as producer (writes!=0)")
	}
	w, _, _ = m68kClassifyCCR(0x5248)
	if w != 0 {
		t.Errorf("ADDQ.W #1,A0 must NOT be classified as producer (writes==0), got writes=%v", w)
	}
}

func TestM68KCCRLiveness_ArithShadowedByArith(t *testing.T) {
	// ADD.L D1,D0; ADD.L D1,D0 — both write X+NZVC. Latest shadows both
	// demands; prior is fully dead.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xD081),
		mkM(0xD081),
	})
	if live[0] || !live[1] {
		t.Errorf("arith shadow: expected [false,true], got %v", live)
	}
}

func TestM68KCCRLiveness_NonProducersStay(t *testing.T) {
	// SUBA/ADDA/LEA/JMP/MOVEM/RTS must NOT classify as producer.
	cases := []struct {
		name string
		op   uint16
	}{
		{"SUBA.W A0,A1", 0x92C8},        // group 9 opmode 3
		{"ADDA.L A0,A1", 0xD3C8},        // group D opmode 7
		{"LEA (xxx).L,A0", 0x41F9},      // group 4 LEA
		{"JMP (A0)", 0x4ED0},            // group 4 JMP
		{"MOVEM.L D0-D7,-(SP)", 0x48E7}, // group 4 MOVEM
		{"RTS", 0x4E75},
	}
	for _, c := range cases {
		writes, consumer, overwriter := m68kClassifyCCR(c.op)
		producer := writes != 0
		if producer {
			t.Errorf("%s should NOT be producer (got p=%v c=%v o=%v)", c.name, producer, consumer, overwriter)
		}
	}
}

func TestM68KAnalyzeBlockRegs_AddressArithmeticDoesNotWriteCCR(t *testing.T) {
	cases := []struct {
		name string
		op   uint16
	}{
		{"ADDA.L D0,A1", 0xD3C0},
		{"ADDA.L A0,A1", 0xD3C8},
		{"SUBA.L D0,A1", 0x93C0},
		{"SUBA.L A0,A1", 0x93C8},
		{"ADDQ.L #1,A0", 0x5288},
		{"SUBQ.L #1,A0", 0x5388},
	}
	for _, tc := range cases {
		regs := m68kAnalyzeBlockRegs([]M68KJITInstr{mkM(tc.op)})
		if regs.writesCCR {
			t.Fatalf("%s marked block as CCR-writing", tc.name)
		}
	}

	for _, op := range []uint16{0x5280, 0x5380, 0xD080, 0x9080} {
		regs := m68kAnalyzeBlockRegs([]M68KJITInstr{mkM(op)})
		if !regs.writesCCR {
			t.Fatalf("data-register arithmetic 0x%04X did not mark block as CCR-writing", op)
		}
	}
}

func TestM68KCCRLiveness_EmptyInput(t *testing.T) {
	if got := m68kCCRLiveness(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Register-only-safe whitelist gates (inverted hidden-consumer model)
// ---------------------------------------------------------------------------

func TestM68KCCRLiveness_RegisterOnlySafeWhitelist(t *testing.T) {
	safe := []struct {
		name string
		op   uint16
	}{
		{"MOVEQ #5,D0", 0x7005},
		{"MOVE.L D1,D0", 0x2001},
		{"MOVE.L A1,D0", 0x2009},
		{"MOVEA.L D0,A1", 0x2240},
		{"ADD.L D1,D0", 0xD081},
		{"ADD.L A1,D0", 0xD089},
		{"SUB.L D1,D0", 0x9081},
		{"CMP.L D1,D0", 0xB081},
		{"CMPA.L D0,A1", 0xB3C0},
		{"AND.L D1,D0", 0xC081},
		{"OR.L D1,D0", 0x8081},
		{"EOR.L D1,D0", 0xB380},
		{"ADDQ.L #1,D0", 0x5280},
		{"SUBQ.L #1,A0", 0x5388},
		{"ADDI.L #imm,D0", 0x0680},
		{"CMPI.L #imm,D0", 0x0C80},
		{"NEG.L D0", 0x4480},
		{"NOT.L D0", 0x4680},
		{"CLR.L D0", 0x4280},
		{"TST.L D0", 0x4A80},
		{"EXT.W D0", 0x4880},
		{"SWAP D0", 0x4840},
		{"NOP", 0x4E71},
		{"LSL.L #1,D0", 0xE388},
		{"ROXL.W #1,D0", 0xE350},
		{"BRA.S", 0x6000},
		{"BNE.S", 0x6600},
		{"Scc D0 (SNE)", 0x56C0},
		{"EXG D0,D1", 0xC141},
		{"MULU.W D1,D0", 0xC0C1},
		{"ADDX.L D1,D0", 0xD581},
		{"ADDA.L D0,A1", 0xD3C0},
	}
	for _, c := range safe {
		ji := mkM(c.op)
		if !m68kInstrCCRRegisterOnlySafe(&ji) {
			t.Errorf("%s (%#04X) must be register-only safe", c.name, c.op)
		}
	}
	unsafe := []struct {
		name string
		op   uint16
	}{
		{"MOVE.L (A0),D0", 0x2010},
		{"MOVE.L D0,(A1)", 0x2280},
		{"NEG.L (A0)", 0x4490},
		{"NOT.W (A0)+", 0x4658},
		{"CLR.B -(A0)", 0x4220},
		{"TST.L (A0)", 0x4A90},
		{"TAS (A0)", 0x4AD0},
		{"NBCD D0", 0x4800},
		{"BSR.S", 0x6100},
		{"JSR (A0)", 0x4E90},
		{"RTS", 0x4E75},
		{"DIVU.W D1,D0", 0x80C1},
		{"DIVS.W D1,D0", 0x81C1},
		{"CHK.W D1,D0", 0x4181},
		{"DBRA D0", 0x51C8},
		{"DBEQ D0", 0x57C8},
		{"TRAPcc (TRAPEQ.W)", 0x57FA},
		{"ABCD D1,D0", 0xC101},
		{"SBCD D1,D0", 0x8101},
		{"ADDX.L -(A1),-(A0)", 0xD189},
		{"CMPM.L (A1)+,(A0)+", 0xB189},
		{"LSL.W #1,(A0) memform", 0xE3D0},
		{"ADD.L D0,(A1)", 0xD191},
		{"Scc (A0)", 0x56D0},
		{"MOVE.L d16(A0),D0", 0x2028},
	}
	for _, c := range unsafe {
		ji := mkM(c.op)
		if m68kInstrCCRRegisterOnlySafe(&ji) {
			t.Errorf("%s (%#04X) must NOT be register-only safe", c.name, c.op)
		}
	}
}

func TestM68KCCRLiveness_MemoryProducerKeepsUpstreamLive(t *testing.T) {
	// ADD.L D1,D0; <memory producer>; ADD.L D1,D0 — the memory producer
	// can bail pre-commit, so the first ADD must stay live even though
	// the last ADD shadows its bits. This is the exact class that kept
	// the liveness gate disabled.
	for _, tc := range []struct {
		name string
		op   uint16
	}{
		{"NEG.L (A0)", 0x4490},
		{"NOT.L (A0)", 0x4690},
		{"CLR.L (A0)", 0x4290},
		{"TST.L (A0)", 0x4A90},
		{"TAS (A0)", 0x4AD0},
		{"DIVU.W D2,D3", 0x86C2},
		{"CHK.W D1,D0", 0x4181},
	} {
		live := m68kCCRLiveness([]M68KJITInstr{
			mkM(0xD081), // ADD.L D1,D0
			mkM(tc.op),
			mkM(0xD081), // ADD.L D1,D0
		})
		if !live[0] {
			t.Errorf("ADD before %s must stay live (pre-commit bail/exception observes CCR), got %v", tc.name, live)
		}
	}
}

func TestM68KCCRLiveness_RegisterOnlyRunStillElides(t *testing.T) {
	// The whole point of the gate: a register-only run lets the shadowed
	// producer die. ADD.L D1,D0; LEA-free reg-only filler; ADD.L D1,D0.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xD081), // ADD.L D1,D0 — shadowed by last ADD (X and NZVC)
		mkM(0x2400), // MOVE.L D0,D2 — reg-only, writes NZVC (partially shadows)
		mkM(0x2240), // MOVEA.L D0,A1 — reg-only, no CCR
		mkM(0xD081), // ADD.L D1,D0 — overwrites X+NZVC
	})
	if live[0] {
		t.Errorf("first ADD fully shadowed across register-only run must be dead, got %v", live)
	}
	if !live[3] {
		t.Errorf("final ADD must be live (block exit), got %v", live)
	}
}

func TestM68KCCRLiveness_DBccReassertsDemand(t *testing.T) {
	// ADD.L D1,D0; DBRA D7,disp; ADD.L D1,D0 — DBRA's taken path
	// chain-exits mid-block with R14 published to the successor, so the
	// first ADD must stay live even though DBRA reads no CCR.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xD081), // ADD.L D1,D0
		mkM(0x51CF), // DBRA D7
		mkM(0xD081), // ADD.L D1,D0
	})
	if !live[0] {
		t.Errorf("ADD before DBRA must stay live (chain exit publishes CCR), got %v", live)
	}
}

// ---------------------------------------------------------------------------
// Runtime parity for the enabled dead-CCR skip (JIT vs interpreter)
// ---------------------------------------------------------------------------

func TestM68KCCRLiveness_DeadSkipRuntimeParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, v := range [][2]uint32{
		{0xFFFFFFFF, 1}, {1, 1}, {0x7FFFFFFF, 1}, {0x80000000, 0x80000000}, {0, 0},
	} {
		// Shape 1: dead ADD (fully shadowed by later ADD across a
		// register-only run) — the skip must not change final CCR/regs.
		bccFusionCompare(t, "DeadSkip_RegRun", []uint16{
			0x203C, uint16(v[0] >> 16), uint16(v[0]), // MOVE.L #a,D0
			0x223C, uint16(v[1] >> 16), uint16(v[1]), // MOVE.L #b,D1
			0xD081,         // ADD.L D1,D0   (dead: shadowed below)
			0x2400,         // MOVE.L D0,D2  (reg-only)
			0x2240,         // MOVEA.L D0,A1 (reg-only, no CCR)
			0xD081,         // ADD.L D1,D0   (live)
			0x4E72, 0x2700, // STOP
		})
		// Shape 2: dead producer followed by EFLAGS-clobbering reg-only
		// instruction then live producer + consumer chain.
		bccFusionCompare(t, "DeadSkip_ThenBranch", []uint16{
			0x203C, uint16(v[0] >> 16), uint16(v[0]), // MOVE.L #a,D0
			0x223C, uint16(v[1] >> 16), uint16(v[1]), // MOVE.L #b,D1
			0x9081,         // SUB.L D1,D0   (dead: CMP below rewrites NZVC, ADD rewrites X)
			0x4840,         // SWAP D0       (reg-only producer, NZVC)
			0xD081,         // ADD.L D1,D0   (live: X+NZVC)
			0xB081,         // CMP.L D1,D0   (live, consumed)
			0x6706,         // BEQ.S +6
			0x7401,         // MOVEQ #1,D2
			0x4E72, 0x2700, // STOP
			0x7402,         // MOVEQ #2,D2
			0x4E72, 0x2700, // STOP
		})
		// Shape 3: dead skip before a memory instruction must NOT happen —
		// memory MOVE is a hidden consumer; parity must hold regardless.
		bccFusionCompare(t, "DeadSkip_MemGuard", []uint16{
			0x203C, uint16(v[0] >> 16), uint16(v[0]), // MOVE.L #a,D0
			0x223C, uint16(v[1] >> 16), uint16(v[1]), // MOVE.L #b,D1
			0x307C, 0x5000, // MOVEA.W #$5000,A0
			0xD081,         // ADD.L D1,D0   (must stay live: store below can bail)
			0x2080,         // MOVE.L D0,(A0)
			0xD081,         // ADD.L D1,D0
			0x4E72, 0x2700, // STOP
		})
	}
}

// ---------------------------------------------------------------------------
// Status-register move aliasing + CMPM classification (review P1/P2)
// ---------------------------------------------------------------------------

func TestM68KCCRLiveness_StatusMovesNotAliasedToDnForms(t *testing.T) {
	// Size field 11 under the group-4 hi-byte masks selects the SR/CCR
	// move forms, not NEGX/CLR/NEG/NOT byte forms:
	//   0x40C0 MOVE SR,Dn   — reads whole SR (consumer, no CCR write)
	//   0x42C0 MOVE CCR,Dn  — reads CCR (consumer, no CCR write)
	for _, tc := range []struct {
		name string
		op   uint16
	}{
		{"MOVE SR,D0", 0x40C0},
		{"MOVE CCR,D0", 0x42C0},
	} {
		writes, consumer, overwriter := m68kClassifyCCR(tc.op)
		if writes != 0 {
			t.Errorf("%s must not classify as CCR producer (writes=%v)", tc.name, writes)
		}
		if !consumer {
			t.Errorf("%s must classify as CCR consumer", tc.name)
		}
		if overwriter {
			t.Errorf("%s must not classify as overwriter", tc.name)
		}
	}
	// None of the four status-move forms may sit on the register-only-safe
	// whitelist: 0x40C0/0x42C0 read CCR, 0x46C0 (MOVE Dn,SR) can raise a
	// privilege exception that exposes the pre-instruction CCR, and
	// 0x44C0 (MOVE Dn,CCR) is kept off conservatively with its siblings.
	for _, op := range []uint16{0x40C0, 0x42C0, 0x44C0, 0x46C0} {
		ji := mkM(op)
		if m68kInstrCCRRegisterOnlySafe(&ji) {
			t.Errorf("status move %#04X must NOT be register-only safe", op)
		}
	}
}

func TestM68KCCRLiveness_MoveFromCCRKeepsProducerLive(t *testing.T) {
	// MOVE.L D1,D0 (NZVC producer); MOVE CCR,D2 — the CCR read must keep
	// the producer live. The old classification aliased 0x42C2 to CLR.B,
	// shadowing the producer's NZ/VC.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x2001), // MOVE.L D1,D0
		mkM(0x42C2), // MOVE CCR,D2
	})
	if !live[0] {
		t.Errorf("producer before MOVE CCR,Dn must stay live, got %v", live)
	}
}

func TestM68KCCRLiveness_CMPMWritesNZVC(t *testing.T) {
	// CMPM.L (A0)+,(A1)+ = 0xB389 — compare writes N/Z/V/C (X preserved).
	writes, _, _ := m68kClassifyCCR(0xB389)
	if writes&m68kCCRBitVC == 0 {
		t.Fatalf("CMPM must classify as V/C writer, got writes=%v", writes)
	}
	if writes&m68kCCRBitNZ == 0 {
		t.Fatalf("CMPM must classify as N/Z writer, got writes=%v", writes)
	}
	if writes&m68kCCRBitX != 0 {
		t.Fatalf("CMPM must preserve X, got writes=%v", writes)
	}
	// EOR.L D0,D0 (0xB180) still NZ-only (interpreter preserves X/V/C).
	writes, _, _ = m68kClassifyCCR(0xB180)
	if writes != m68kCCRBitNZ {
		t.Fatalf("EOR must stay NZ-only, got writes=%v", writes)
	}
}

func TestM68KCCRLiveness_CMPMLiveThroughNZShadowForVC(t *testing.T) {
	// CMPM.L (A0)+,(A1)+; EOR.L D0,D0 (shadows N/Z, preserves V/C); BVS.
	// BVS consumes CMPM's V — CMPM must stay live even though its N/Z
	// output is shadowed by the EOR.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0xB389), // CMPM.L (A0)+,(A1)+
		mkM(0xB180), // EOR.L D0,D0
		mkM(0x6900), // BVS
	})
	if !live[0] {
		t.Errorf("CMPM before NZ-shadowing EOR must stay live for V/C, got %v", live)
	}
}

func TestM68KCCRLiveness_CMPMRuntimeParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	// Memory compare via CMPM whose V/C survives an NZ-shadowing EOR into
	// a BVS. Values chosen to produce V=1 (0x80000000 - 1 overflows) and
	// V=0 variants.
	for _, v := range [][2]uint32{
		{0x80000000, 1}, // CMPM dst-src overflows → V=1
		{5, 3},          // plain → V=0
		{3, 5},          // borrow → C=1,V=0
	} {
		program := []uint16{
			0x307C, 0x5000, // MOVEA.W #$5000,A0
			0x327C, 0x5004, // MOVEA.W #$5004,A1
			0x20BC, uint16(v[1] >> 16), uint16(v[1]), // MOVE.L #src,(A0)
			0x22BC, uint16(v[0] >> 16), uint16(v[0]), // MOVE.L #dst,(A1)
			0xB389,         // CMPM.L (A0)+,(A1)+
			0xB180,         // EOR.L D0,D0 (shadows N/Z)
			0x6906,         // BVS.S +6
			0x7401,         // MOVEQ #1,D2
			0x4E72, 0x2700, // STOP
			0x7402,         // MOVEQ #2,D2
			0x4E72, 0x2700, // STOP
		}
		bccFusionCompare(t, "CMPM_VC", program)
	}
}
