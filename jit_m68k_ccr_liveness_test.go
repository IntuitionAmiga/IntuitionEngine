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

func TestM68KCCRLiveness_BSRNotConsumer(t *testing.T) {
	// MOVE.B; BSR (cc=1, no CCR read); MOVE.B — first MOVE shadowed by
	// last MOVE since BSR is not a consumer.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x1200), // MOVE.B
		mkM(0x6100), // BSR
		mkM(0x1400), // MOVE.B
	})
	if live[0] {
		t.Errorf("MOVE.B before BSR-only path should be dead, got %v", live)
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

func TestM68KCCRLiveness_RTEKillsDemand(t *testing.T) {
	// MOVE.B; RTE (0x4E73); BNE — RTE overwrites SR, demand killed.
	live := m68kCCRLiveness([]M68KJITInstr{
		mkM(0x1200),
		mkM(0x4E73), // RTE
		mkM(0x6600), // BNE
	})
	if live[0] {
		t.Errorf("MOVE.B before RTE should be dead, got %v", live)
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

func TestM68KCCRLiveness_EmptyInput(t *testing.T) {
	if got := m68kCCRLiveness(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}
