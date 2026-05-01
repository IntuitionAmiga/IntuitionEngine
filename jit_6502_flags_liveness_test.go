// jit_6502_flags_liveness_test.go - tightness gates for Phase 2c
// 6502 NZ liveness analysis.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func mkInstr(op byte) JIT6502Instr { return JIT6502Instr{opcode: op} }

func TestP65PeepholeFlags_LatestProducerLive(t *testing.T) {
	// LDA #imm × 2: only the second is live (block-exit materializes the
	// most recent NZ; the first is dead).
	live := p65PeepholeFlags([]JIT6502Instr{mkInstr(0xA9), mkInstr(0xA9)})
	if live[0] || !live[1] {
		t.Errorf("expected live=[false,true], got %v", live)
	}
}

func TestP65PeepholeFlags_ConsumerKeepsAlive(t *testing.T) {
	// LDA; BNE rel; LDA — first LDA is consumed by BNE → live; second
	// LDA is the latest producer → also live.
	live := p65PeepholeFlags([]JIT6502Instr{
		mkInstr(0xA9), // LDA
		mkInstr(0xD0), // BNE
		mkInstr(0xA9), // LDA
	})
	if !live[0] || !live[2] {
		t.Errorf("expected live[0] and live[2] true, got %v", live)
	}
}

func TestP65PeepholeFlags_PLPKillsDemand(t *testing.T) {
	// LDA imm; PLP; BNE — PLP is an SR overwriter, but PLP itself is
	// also bail-capable (stack-page access can fault into the
	// interpreter). Bail-capable instructions are implicit consumers
	// of upstream pending NZ because the bail epilogue materialises
	// the guest SR. So LDA stays live across PLP, even though the BNE
	// downstream of PLP is satisfied by PLP's overwrite.
	live := p65PeepholeFlags([]JIT6502Instr{
		mkInstr(0xA9), // LDA
		mkInstr(0x28), // PLP
		mkInstr(0xD0), // BNE
	})
	if !live[0] {
		t.Errorf("LDA before bail-capable PLP must remain live, got %v", live)
	}
}

func TestP65PeepholeFlags_StoreNotProducer(t *testing.T) {
	// LDA; STA; BNE — LDA is live (BNE consumes); STA does NOT touch NZ
	// so demand reaches LDA.
	live := p65PeepholeFlags([]JIT6502Instr{
		mkInstr(0xA9), // LDA
		mkInstr(0x85), // STA $zp (no NZ)
		mkInstr(0xD0), // BNE
	})
	if !live[0] {
		t.Errorf("LDA should be live (consumed by BNE through STA), got %v", live)
	}
}

func TestP65PeepholeFlags_DeadProducerBetweenLiveOnes(t *testing.T) {
	// LDA; LDA; BNE; LDA — middle LDA is consumed by BNE → live; first
	// LDA is shadowed by middle LDA → dead; last LDA is latest → live.
	live := p65PeepholeFlags([]JIT6502Instr{
		mkInstr(0xA9), // LDA  (dead — shadowed)
		mkInstr(0xA9), // LDA  (live — feeds BNE)
		mkInstr(0xD0), // BNE
		mkInstr(0xA9), // LDA  (live — latest)
	})
	if live[0] || !live[1] || !live[3] {
		t.Errorf("expected [F,T,_,T], got %v", live)
	}
}

func TestP65PeepholeFlags_PHPIsConsumer(t *testing.T) {
	// LDA; PHP — LDA is consumed by PHP (SR push exposes NZ to guest).
	live := p65PeepholeFlags([]JIT6502Instr{
		mkInstr(0xA9), // LDA
		mkInstr(0x08), // PHP
	})
	if !live[0] {
		t.Errorf("LDA before PHP should be live, got %v", live)
	}
}

func TestP65PeepholeFlags_AllBranchesConsume(t *testing.T) {
	// LDA imm; <branch> ; LDA imm
	// All 8 conditional branches must keep the first LDA live: even
	// BCC/BCS/BVC/BVS (whose conditions don't read NZ) can side-exit
	// the block, and the exit epilogue must materialise pending NZ.
	branches := []byte{0x10, 0x30, 0x50, 0x70, 0x90, 0xB0, 0xD0, 0xF0}
	for _, br := range branches {
		live := p65PeepholeFlags([]JIT6502Instr{
			mkInstr(0xA9), // LDA imm
			mkInstr(br),   // branch — potential exit
			mkInstr(0xA9), // LDA imm
		})
		if !live[0] {
			t.Errorf("LDA before branch %02X should be live, got %v", br, live)
		}
	}
}

func TestP65PeepholeFlags_UnconditionalExitsConsume(t *testing.T) {
	// LDA imm; JMP — JMP exits the block; pending NZ must materialize.
	exits := []byte{0x4C, 0x6C, 0x20, 0x60, 0x40, 0x00}
	for _, ex := range exits {
		live := p65PeepholeFlags([]JIT6502Instr{
			mkInstr(0xA9),
			mkInstr(ex),
		})
		if !live[0] {
			t.Errorf("LDA before exit %02X should be live, got %v", ex, live)
		}
	}
}

func TestP65PeepholeFlags_DecimalModeADCBails(t *testing.T) {
	// LDA imm; ADC imm — ADC #imm bails on decimal-mode flag, so it is
	// a hidden CCR consumer of upstream pending NZ. LDA must stay live
	// even though both are pure-immediate.
	live := p65PeepholeFlags([]JIT6502Instr{
		mkInstr(0xA9), // LDA #imm
		mkInstr(0x69), // ADC #imm — decimal-bail consumer
	})
	if !live[0] {
		t.Errorf("LDA before ADC #imm must be live (decimal-mode bail consumer), got %v", live)
	}
	// Same for SBC #imm.
	live = p65PeepholeFlags([]JIT6502Instr{
		mkInstr(0xA9),
		mkInstr(0xE9),
	})
	if !live[0] {
		t.Errorf("LDA before SBC #imm must be live (decimal-mode bail consumer), got %v", live)
	}
}

func TestP65PeepholeFlags_EmptyInput(t *testing.T) {
	if got := p65PeepholeFlags(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}

func TestP65NZConsumers_TrueForBranchesAndPHP(t *testing.T) {
	consumers := []byte{0x10, 0x30, 0xD0, 0xF0, 0x08}
	for _, op := range consumers {
		instrs := []JIT6502Instr{mkInstr(op)}
		if !p65NZConsumers(instrs, 0) {
			t.Errorf("opcode %02X should be NZ consumer", op)
		}
	}
	// LDA is producer not consumer.
	if p65NZConsumers([]JIT6502Instr{mkInstr(0xA9)}, 0) {
		t.Errorf("LDA should not be consumer")
	}
}
