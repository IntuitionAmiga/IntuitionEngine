//go:build !js

package main

import (
	"encoding/binary"
	"testing"

	"github.com/tetratelabs/wazero/api"
)

func putWasmRegionInstr(mem []byte, pc uint64, opcode, rd, rs, rt byte, imm32 uint32) {
	mem[pc] = opcode
	mem[pc+1] = rd << 3
	mem[pc+2] = rs << 3
	mem[pc+3] = rt << 3
	binary.LittleEndian.PutUint32(mem[pc+4:], imm32)
}

func putWasmRegionBRA(mem []byte, pc, target uint64) {
	putWasmRegionInstr(mem, pc, OP_BRA, 0, 0, 0, uint32(int32(target-pc)))
}

func TestWasmFormRegionFollowsForwardBRAWithinLimits(t *testing.T) {
	mem := make([]byte, 0x400)
	putWasmRegionInstr(mem, 0x100, OP_ADD, 7, 7, 9, 0)
	putWasmRegionBRA(mem, 0x108, 0x180)
	putWasmRegionInstr(mem, 0x180, OP_EOR, 7, 7, 5, 0)
	putWasmRegionInstr(mem, 0x188, OP_RTS64, 0, 0, 0, 0)

	region := wasmFormRegion(mem, 0x100)
	if len(region) != 2 {
		t.Fatalf("wasmFormRegion formed %d blocks, want 2", len(region))
	}
	if region[0].pc != 0x100 || region[1].pc != 0x180 {
		t.Fatalf("region PCs = %#x, %#x", region[0].pc, region[1].pc)
	}
	if _, err := wasmCompileBlocks(region); err != nil {
		t.Fatalf("wasmCompileBlocks: %v", err)
	}
}

func TestWasmRegionResidencyPlansSpanInternalEdge(t *testing.T) {
	blocks := []wasmRegionBlock{
		{pc: 0x100, instrs: []JITInstr{
			{opcode: OP_ADD, rd: 7, rs: 7, rt: 9, size: IE64_SIZE_Q},
			{opcode: OP_BRA, imm32: 0x78, pcOffset: 8},
		}},
		{pc: 0x180, instrs: []JITInstr{
			{opcode: OP_EOR, rd: 7, rs: 7, rt: 5, size: IE64_SIZE_Q},
			{opcode: OP_JMP, rs: 0, imm32: 0x240, pcOffset: 8},
		}},
	}
	flat := append(append([]JITInstr{}, blocks[0].instrs...), blocks[1].instrs...)
	plan := wasmBuildGPRPlan(flat)
	if plan == nil || plan.local(7) == 0 || !plan.dirty[7] {
		t.Fatalf("cross-block R7 did not receive dirty residency: %+v", plan)
	}
	if _, err := wasmCompileBlocks(blocks); err != nil {
		t.Fatalf("wasmCompileBlocks: %v", err)
	}
}

func TestWasmRegionExecutesStructuredBackEdgeWithinBudget(t *testing.T) {
	mem := make([]byte, 0x300)
	putWasmRegionInstr(mem, 0x100, OP_ADD, 1, 1, 2, 0)
	putWasmRegionBRA(mem, 0x108, 0x180)
	putWasmRegionInstr(mem, 0x180, OP_SUB, 1, 1, 2, 0)
	putWasmRegionBRA(mem, 0x188, 0x100)
	region := wasmFormRegion(mem, 0x100)
	if len(region) != 2 {
		t.Fatalf("forward prefix formed %d blocks, want 2", len(region))
	}
	if _, err := wasmCompileBlocks(region); err != nil {
		t.Fatalf("wasmCompileBlocks: %v", err)
	}
}

func TestWasmRegionBackEdgeRetiredCountAndResidency(t *testing.T) {
	program := make([]byte, 32)
	putWasmRegionInstr(program, 0, OP_ADD, 1, 1, 2, 0)
	putWasmRegionBRA(program, 8, 16)
	putWasmRegionInstr(program, 16, OP_ADD, 1, 1, 2, 0)
	putWasmRegionBRA(program, 24, 0)

	result := runWasmDiffCompiled(t, program, map[int]uint64{1: 0, 2: 1}, func(mem api.Memory) {
		mem.WriteUint32Le(wasmDiffCtxOff+jitCtxOffChainBudget, 2)
	}, func(memory []byte) ([]byte, error) {
		return wasmCompileBlocks(wasmFormRegion(memory, PROG_START))
	})
	if result.regs[1] != 6 {
		t.Fatalf("R1 = %d, want 6 after three region iterations", result.regs[1])
	}
	if result.retPC != PROG_START {
		t.Fatalf("RetPC = %#x, want loop target %#x", result.retPC, PROG_START)
	}
	if result.retCount != 12 {
		t.Fatalf("RetCount = %d, want 12", result.retCount)
	}
}

func TestWasmRegionBackEdgeDynamicExitCount(t *testing.T) {
	program := make([]byte, 40)
	putWasmRegionInstr(program, 0, OP_ADD, 1, 1, 2, 0)
	putWasmRegionBRA(program, 8, 16)
	putWasmRegionInstr(program, 16, OP_ADD, 1, 1, 2, 0)
	putWasmRegionInstr(program, 24, OP_BEQ, 0, 1, 3, 16)
	putWasmRegionBRA(program, 32, 0)

	result := runWasmDiffCompiled(t, program, map[int]uint64{2: 1, 3: 4}, func(mem api.Memory) {
		mem.WriteUint32Le(wasmDiffCtxOff+jitCtxOffChainBudget, 8)
	}, func(memory []byte) ([]byte, error) {
		return wasmCompileBlocks(wasmFormRegion(memory, PROG_START))
	})
	if result.regs[1] != 4 {
		t.Fatalf("R1 = %d, want 4 at the second-iteration branch", result.regs[1])
	}
	if result.retPC != PROG_START+40 {
		t.Fatalf("RetPC = %#x, want external target %#x", result.retPC, PROG_START+40)
	}
	if result.retCount != 9 {
		t.Fatalf("RetCount = %d, want 9", result.retCount)
	}
}

func TestWasmFPSRCCLivenessElidesOverwrittenNonFaultingUpdate(t *testing.T) {
	instrs := []JITInstr{
		{opcode: OP_DADD, rd: 2, rs: 2, rt: 4},
		{opcode: OP_DMUL, rd: 6, rs: 6, rt: 8},
	}
	got := wasmFPSRCCLive(instrs)
	if got[0] || !got[1] {
		t.Fatalf("FPSR CC liveness = %v, want [false true]", got)
	}
}

func TestWasmFPSRCCLivenessKeepsUpdateBeforeObservableExit(t *testing.T) {
	instrs := []JITInstr{
		{opcode: OP_DADD, rd: 2, rs: 2, rt: 4},
		{opcode: OP_BEQ, rs: 1, rt: 0},
		{opcode: OP_DMUL, rd: 6, rs: 6, rt: 8},
	}
	got := wasmFPSRCCLive(instrs)
	if !got[0] || !got[2] {
		t.Fatalf("FPSR CC liveness across branch = %v, want [true false true]", got)
	}
}

func TestWasmFPSRCCLivenessTreatsMemoryAsFaultBarrier(t *testing.T) {
	instrs := []JITInstr{
		{opcode: OP_DADD, rd: 2, rs: 2, rt: 4},
		{opcode: OP_DLOAD, rd: 6, rs: 1},
		{opcode: OP_DMUL, rd: 8, rs: 8, rt: 10},
	}
	got := wasmFPSRCCLive(instrs)
	if !got[0] || !got[2] {
		t.Fatalf("FPSR CC liveness across memory = %v", got)
	}
}
