package main

import "testing"

func TestIE64ObservedRecorderCaptureAndReset(t *testing.T) {
	var r ie64ObservedRecorder
	r.start(0x100, true, 7)
	if done, reject := r.appendSuccessor(0x180); done || reject {
		t.Fatalf("first successor done=%v reject=%v", done, reject)
	}
	if done, reject := r.appendSuccessor(0x100); !done || reject {
		t.Fatalf("loop closure done=%v reject=%v", done, reject)
	}
	if got := r.path(); len(got) != 2 || got[0] != 0x100 || got[1] != 0x180 {
		t.Fatalf("path=%#x, want [0x100 0x180]", got)
	}
	r.reset()
	if r.active || r.count != 0 || len(r.path()) != 0 {
		t.Fatalf("reset left recorder active: %+v", r)
	}
}

func TestIE64ObservedRecorderIsAllocationFree(t *testing.T) {
	var r ie64ObservedRecorder
	if allocs := testing.AllocsPerRun(1000, func() {
		r.start(0x100, true, 7)
		r.appendSuccessor(0x180)
		r.appendSuccessor(0x100)
		r.reset()
	}); allocs != 0 { t.Fatalf("recorder allocations = %v", allocs) }
}

func TestIE64ObservedRecorderEightDistinctBlocks(t *testing.T) {
	var r ie64ObservedRecorder
	r.start(0x100, false, 1)
	for i := 1; i < ie64ObservedMaxBlocks; i++ {
		done, reject := r.appendSuccessor(0x100 + uint64(i)*0x10)
		if reject || done != (i == ie64ObservedMaxBlocks-1) {
			t.Fatalf("append %d: done=%v reject=%v", i, done, reject)
		}
	}
}

func TestIE64ObservedRecorderRejectsAmbiguousRevisit(t *testing.T) {
	var r ie64ObservedRecorder
	r.start(0x100, false, 1)
	r.appendSuccessor(0x180)
	if done, reject := r.appendSuccessor(0x180); done || !reject {
		t.Fatalf("non-entry revisit done=%v reject=%v", done, reject)
	}
}

func TestIE64ObservedTriggerSelection(t *testing.T) {
	entry := []JITInstr{
		{opcode: OP_BEQ, pcOffset: 0, imm32: 0x40},
		{opcode: OP_BRA, pcOffset: 8, imm32: 0x18},
	}
	if got := ie64ObservedTrigger(entry, 0x100, 0x110); got != ie64ObservedConditional {
		t.Fatalf("external conditional trigger=%v", got)
	}
	ordinary := []JITInstr{{opcode: OP_BRA, imm32: 0x20}}
	if got := ie64ObservedTrigger(ordinary, 0x100, 0x108); got != ie64ObservedNone {
		t.Fatalf("ordinary BRA trigger=%v", got)
	}
	indirect := []JITInstr{{opcode: OP_JMP, rs: 3, imm32: ^uint32(7)}}
	if got := ie64ObservedTrigger(indirect, 0x100, 0x108); got != ie64ObservedIndirectJMP {
		t.Fatalf("indirect JMP trigger=%v", got)
	}
	indirect[0].rs = 0
	if got := ie64ObservedTrigger(indirect, 0x100, 0x108); got != ie64ObservedNone {
		t.Fatalf("JMP R0 trigger=%v", got)
	}
}

func TestIE64BuildObservedRegionTruncatesConditional(t *testing.T) {
	mem := make([]byte, 0x400)
	pred := []JITInstr{
		{opcode: OP_NOP64, pcOffset: 0},
		{opcode: OP_BEQ, pcOffset: 8, imm32: 0x78}, // 0x108 -> 0x180
		{opcode: OP_ADD, pcOffset: 16},
		{opcode: OP_BRA, pcOffset: 24, imm32: 0x68},
	}
	succ := []JITInstr{{opcode: OP_BRA, pcOffset: 0, imm32: ^uint32(0x7f)}} // 0x180 -> 0x100
	copyBefore := append([]JITInstr(nil), pred...)
	region, err := ie64BuildObservedRegion([]uint64{0x100, 0x180}, map[uint64][]JITInstr{0x100: pred, 0x180: succ}, uint64(len(mem)))
	if err != nil {
		t.Fatal(err)
	}
	if len(region.blocks[0].instrs) != 2 || region.blocks[0].hotTarget != 0x180 || region.blocks[0].coldTarget != 0x110 {
		t.Fatalf("conditional block=%+v", region.blocks[0])
	}
	if len(pred) != len(copyBefore) || pred[2].opcode != copyBefore[2].opcode {
		t.Fatal("Tier 1 instruction slice was mutated")
	}
	if region.instrCount != 3 {
		t.Fatalf("instrCount=%d, want 3", region.instrCount)
	}
}

func TestIE64BuildObservedRegionRejectsInvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		pred []JITInstr
		next uint64
	}{
		{"impossible", []JITInstr{{opcode: OP_BEQ, imm32: 0x20}}, 0x180},
		{"ambiguous", []JITInstr{{opcode: OP_BEQ, imm32: 0x80}, {opcode: OP_BNE, pcOffset: 8, imm32: 0x78}}, 0x180},
		{"call", []JITInstr{{opcode: OP_JSR64, imm32: 0x80}}, 0x180},
		{"high target", []JITInstr{{opcode: OP_BEQ, imm32: 0x80}}, 0x180},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := uint64(0x400)
			if tt.name == "high target" {
				window = 0x180
			}
			_, err := ie64BuildObservedRegion([]uint64{0x100, tt.next}, map[uint64][]JITInstr{0x100: tt.pred, tt.next: {{opcode: OP_BRA, imm32: ^uint32(0x7f)}}}, window)
			if err == nil {
				t.Fatal("accepted invalid transition")
			}
		})
	}
}
