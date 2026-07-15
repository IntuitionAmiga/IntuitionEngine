package main

import "testing"

func loopIns(op, rd, rs, rt, size, x byte, imm int32, idx int) JITInstr {
	return JITInstr{opcode: op, rd: rd, rs: rs, rt: rt, size: size, xbit: x, imm32: uint32(imm), pcOffset: uint32(idx * IE64_INSTR_SIZE)}
}

func TestIE64LoopAnalysisInvariantMemory(t *testing.T) {
	ins := []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3, 0),
		loopIns(OP_LOAD, 3, 5, 0, IE64_SIZE_Q, 0, 16, 1),
		loopIns(OP_STORE, 3, 5, 0, IE64_SIZE_Q, 0, 16, 2),
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 3),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -24, 4),
	}
	p := ie64AnalyseLoop(ins, 0x100)
	if p == nil || len(p.accesses) != 1 || p.head != 1 || p.prefix != 1 {
		t.Fatalf("plan = %#v, want one deduplicated loop access", p)
	}
	if p.bounded {
		t.Fatal("memory loop must not also qualify as an integer-only bounded loop")
	}
}

func TestIE64LoopAnalysisBoundedCounter(t *testing.T) {
	ins := []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3, 0),
		loopIns(OP_ADD, 3, 3, 0, IE64_SIZE_Q, 1, 1, 1),
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 2),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -16, 3),
	}
	p := ie64AnalyseLoop(ins, 0x100)
	if p == nil || !p.bounded || p.count != 3 {
		t.Fatalf("plan = %#v, want bounded count 3", p)
	}
}

func TestIE64LoopAnalysisRejectsChangedBaseAndExtraEntry(t *testing.T) {
	base := []JITInstr{
		loopIns(OP_LOAD, 3, 5, 0, IE64_SIZE_Q, 0, 0, 0),
		loopIns(OP_ADD, 5, 5, 0, IE64_SIZE_Q, 1, 8, 1),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -16, 2),
	}
	if p := ie64AnalyseLoop(base, 0x100); p != nil {
		t.Fatalf("changed base accepted: %#v", p)
	}
	extra := []JITInstr{
		loopIns(OP_BRA, 0, 0, 0, 0, 0, 16, 0),
		loopIns(OP_LOAD, 3, 5, 0, IE64_SIZE_Q, 0, 0, 1),
		loopIns(OP_NOP64, 0, 0, 0, 0, 0, 0, 2),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -16, 3),
	}
	if p := ie64AnalyseLoop(extra, 0x100); p != nil {
		t.Fatalf("alternate entry accepted: %#v", p)
	}
}

func TestIE64BoundedLoopRejections(t *testing.T) {
	makeLoop := func(count uint32, body byte) []JITInstr {
		return []JITInstr{
			loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, int32(count), 0),
			loopIns(body, 3, 3, 0, IE64_SIZE_Q, 1, 1, 1),
			loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 2),
			loopIns(OP_BNE, 0, 2, 0, 0, 0, -16, 3),
		}
	}
	for _, tc := range []struct {
		name  string
		count uint32
		body  byte
	}{
		{"zero", 0, OP_ADD},
		{"oversized", uint32(ie64JITLoopBudget), OP_ADD},
		{"memory", 2, OP_LOAD},
		{"fpu", 2, OP_FADD},
		{"stack", 2, OP_PUSH64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if p := ie64AnalyseLoop(makeLoop(tc.count, tc.body), 0x100); p != nil && p.bounded {
				t.Fatalf("unsafe bounded loop accepted: %#v", p)
			}
		})
	}
}
