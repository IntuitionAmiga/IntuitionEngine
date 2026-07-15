//go:build !js

package main

import "testing"

func TestWasmGPRPlanSelectsHotRegistersAndTracksDirty(t *testing.T) {
	instrs := []JITInstr{
		{opcode: OP_ADD, rd: 7, rs: 7, rt: 9, size: IE64_SIZE_Q},
		{opcode: OP_EOR, rd: 7, rs: 7, rt: 5, size: IE64_SIZE_Q},
		{opcode: OP_BNE, rs: 7, rt: 0},
	}
	plan := wasmBuildGPRPlan(instrs)
	if plan == nil {
		t.Fatal("helper-free integer block did not receive a GPR plan")
	}
	if plan.local(7) == 0 || !plan.dirty[7] {
		t.Fatalf("hot written R7 not resident and dirty: %+v", plan)
	}
	if plan.local(9) == 0 || plan.dirty[9] {
		t.Fatalf("read-only R9 residency/dirty state wrong: %+v", plan)
	}
	if plan.local(0) != 0 || plan.local(31) != 0 {
		t.Fatal("R0 or SP became a wasm local resident")
	}
}

func TestWasmGPRPlanRejectsHelperBlocks(t *testing.T) {
	if plan := wasmBuildGPRPlan([]JITInstr{{opcode: OP_LOAD, rd: 1, rs: 2, size: IE64_SIZE_Q}}); plan != nil {
		t.Fatalf("memory/helper block received GPR residency: %+v", plan)
	}
}
