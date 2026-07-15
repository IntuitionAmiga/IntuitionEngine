//go:build !js

package main

import "testing"

func TestWasmFPPlanUsesPairOwnership(t *testing.T) {
	instrs := []JITInstr{
		{opcode: OP_DADD, rd: 4, rs: 4, rt: 6},
		{opcode: OP_DMUL, rd: 4, rs: 4, rt: 2},
		{opcode: OP_DCMP, rd: 1, rs: 4, rt: 6},
	}
	plan := wasmBuildFPPlan(instrs, wasmGPRLocalBase)
	if plan == nil {
		t.Fatal("helper-free FP64 block did not receive an FP plan")
	}
	if plan.local(4) == 0 || !plan.dirty[2] {
		t.Fatalf("written D4 pair not resident and dirty: %+v", plan)
	}
	if plan.local(6) == 0 || plan.dirty[3] {
		t.Fatalf("read-only D6 pair residency/dirty state wrong: %+v", plan)
	}
	if plan.local(5) != plan.local(4) {
		t.Fatal("odd alias of D4 does not resolve to the same owner")
	}
}

func TestWasmFPPlanRejectsMemoryBlocks(t *testing.T) {
	if plan := wasmBuildFPPlan([]JITInstr{{opcode: OP_DLOAD, rd: 2, rs: 1}}, wasmGPRLocalBase); plan != nil {
		t.Fatalf("memory/helper block received FP residency: %+v", plan)
	}
}
