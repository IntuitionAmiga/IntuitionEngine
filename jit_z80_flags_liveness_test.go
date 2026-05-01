// jit_z80_flags_liveness_test.go - tightness gates for Phase 2c Z80
// F-flag liveness analysis.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func mkZ(op byte) JITZ80Instr       { return JITZ80Instr{opcode: op} }
func mkZPfx(op, p byte) JITZ80Instr { return JITZ80Instr{opcode: op, prefix: p} }

func TestZ80FlagsLiveness_LatestProducerLive(t *testing.T) {
	// ADD A,B (0x80) × 2: only the second is live.
	live := z80FlagsLiveness([]JITZ80Instr{mkZ(0x80), mkZ(0x80)})
	if live[0] || !live[1] {
		t.Errorf("expected [false,true], got %v", live)
	}
}

func TestZ80FlagsLiveness_JRCcKeepsAlive(t *testing.T) {
	// ADD A,B; JR Z,e; ADD A,B — first ADD consumed by JR Z; second ADD latest.
	live := z80FlagsLiveness([]JITZ80Instr{
		mkZ(0x80), // ADD A,B
		mkZ(0x28), // JR Z,e
		mkZ(0x80), // ADD A,B
	})
	if !live[0] || !live[2] {
		t.Errorf("expected live[0] and live[2] true, got %v", live)
	}
}

func TestZ80FlagsLiveness_POPAFKillsDemand(t *testing.T) {
	// ADD A,B; POP AF; JR Z — POP AF overwrites F so ADD's F is dead.
	live := z80FlagsLiveness([]JITZ80Instr{
		mkZ(0x80), // ADD
		mkZ(0xF1), // POP AF
		mkZ(0x28), // JR Z (still a consumer but demand resets to its own true)
	})
	if live[0] {
		t.Errorf("ADD before POP AF should be dead, got %v", live)
	}
}

func TestZ80FlagsLiveness_ADCIsBothProducerAndConsumer(t *testing.T) {
	// ADD; ADC; ADD; JR Z — the middle ADC reads C from first ADD then
	// writes new F. Last ADD also live (latest before JR Z consumer).
	live := z80FlagsLiveness([]JITZ80Instr{
		mkZ(0x80), // ADD
		mkZ(0x88), // ADC A,B
		mkZ(0x80), // ADD
		mkZ(0x28), // JR Z
	})
	if !live[0] {
		t.Errorf("ADD feeding ADC should be live, got %v", live)
	}
	if !live[2] {
		t.Errorf("ADD before JR Z should be live, got %v", live)
	}
	// ADC itself is a producer and is consumed by JR Z's demand chain
	// only if no producer between — but ADD at idx 2 shadows it. So
	// ADC's live should be false.
	if live[1] {
		t.Errorf("ADC shadowed by later ADD should be dead, got %v", live)
	}
}

func TestZ80FlagsLiveness_CBRotateClassified(t *testing.T) {
	// CB-prefixed rotate (top2=00) is a producer + reads C. With a
	// later same-block producer (ADD A,B) it is shadowed → live=false.
	live := z80FlagsLiveness([]JITZ80Instr{
		mkZPfx(0x00, 0xCB), // RLC B
		mkZ(0x80),          // ADD A,B (latest producer)
	})
	if live[0] {
		t.Errorf("CB rotate shadowed by ADD should be dead, got %v", live)
	}
	if !live[1] {
		t.Errorf("trailing ADD must remain live, got %v", live)
	}
}

func TestZ80FlagsLiveness_CBSETIsNotProducer(t *testing.T) {
	// CB-prefixed SET (top2=11, e.g. SET 0,B = 0xCB 0xC0). Doesn't
	// touch F, so demand passes through and the upstream ADD reaches
	// the JR Z consumer.
	live := z80FlagsLiveness([]JITZ80Instr{
		mkZ(0x80),          // ADD A,B
		mkZPfx(0xC0, 0xCB), // SET 0,B (no F effect)
		mkZ(0x28),          // JR Z,e
	})
	if !live[0] {
		t.Errorf("ADD before SET-and-JR-Z should remain live, got %v", live)
	}
}

func TestZ80FlagsLiveness_EDNEGProducer(t *testing.T) {
	// ED 44 = NEG. Two NEGs in a row: first is shadowed.
	live := z80FlagsLiveness([]JITZ80Instr{
		mkZPfx(0x44, 0xED),
		mkZPfx(0x44, 0xED),
	})
	if live[0] || !live[1] {
		t.Errorf("NEG shadow: expected [false,true], got %v", live)
	}
}

func TestZ80FlagsLiveness_DDFDStaysConservative(t *testing.T) {
	// DD/FD indexed (e.g. ADD A,(IX+d)) — emitter doesn't enumerate
	// these in tables; conservative treatment keeps live=true.
	live := z80FlagsLiveness([]JITZ80Instr{
		mkZPfx(0x86, 0xDD), // ADD A,(IX+d)
		mkZ(0x80),          // ADD A,B
	})
	if !live[0] {
		t.Errorf("DD-prefixed should stay conservatively live, got %v", live)
	}
}

func TestZ80FlagsLiveness_PUSHAFConsumes(t *testing.T) {
	// ADD; PUSH AF — PUSH AF is consumer (exposes F to guest stack).
	live := z80FlagsLiveness([]JITZ80Instr{mkZ(0x80), mkZ(0xF5)})
	if !live[0] {
		t.Errorf("ADD before PUSH AF should be live, got %v", live)
	}
}

func TestZ80FlagsLiveness_RLAConsumesC(t *testing.T) {
	// SCF (writes C); RLA (reads C, writes new F); JR C (reads C from RLA)
	// SCF is producer; RLA reads it; SCF is consumer-fed → live.
	live := z80FlagsLiveness([]JITZ80Instr{
		mkZ(0x37), // SCF
		mkZ(0x17), // RLA
		mkZ(0x38), // JR C,e
	})
	if !live[0] {
		t.Errorf("SCF feeding RLA should be live, got %v", live)
	}
}

func TestZ80FlagsLiveness_EmptyInput(t *testing.T) {
	if got := z80FlagsLiveness(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}
