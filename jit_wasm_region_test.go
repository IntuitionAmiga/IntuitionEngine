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

func TestWasmObservedConditionalTakenAndColdExit(t *testing.T) {
	program := make([]byte, 40)
	putWasmRegionInstr(program, 0, OP_ADD, 1, 1, 2, 0)
	putWasmRegionInstr(program, 8, OP_BEQ, 0, 3, 4, 24)
	putWasmRegionInstr(program, 16, OP_ADD, 1, 1, 5, 0) // discarded tail
	putWasmRegionInstr(program, 24, OP_ADD, 1, 1, 5, 0) // discarded tail
	putWasmRegionInstr(program, 32, OP_ADD, 1, 1, 6, 0)
	blocks := []wasmRegionBlock{
		{pc: PROG_START, instrs: scanBlock(program, 0)[:2], kind: ie64ObservedConditional, hotTarget: PROG_START + 32, coldTarget: PROG_START + 16},
		{pc: PROG_START + 32, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 6}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(39)}}, hotTarget: PROG_START},
	}
	compile := func([]byte) ([]byte, error) { return wasmCompileBlocks(blocks) }
	taken := runWasmDiffCompiled(t, program, map[int]uint64{2: 1, 3: 7, 4: 7, 5: 100, 6: 2}, nil, compile)
	if taken.regs[1] != 3 || taken.retPC != PROG_START || taken.retCount != 4 {
		t.Fatalf("taken=%+v", taken)
	}
	cold := runWasmDiffCompiled(t, program, map[int]uint64{2: 1, 3: 7, 4: 8, 5: 100, 6: 2}, nil, compile)
	if cold.regs[1] != 1 || cold.retPC != PROG_START+16 || cold.retCount != 2 {
		t.Fatalf("cold=%+v", cold)
	}
}

// Regression pin for the native cold-exit outlining slice: wasm already
// keeps the cold exit inside the observed conditional's structured arm and
// continues the hot path directly after it. The hot run must retire through
// the whole region body (no exit at the conditional), and the cold run must
// exit at the cold target with the conditional counted.
func TestWasmObservedConditionalColdInArmHotFallsThrough(t *testing.T) {
	program := make([]byte, 40)
	putWasmRegionInstr(program, 0, OP_NOP64, 0, 0, 0, 0)
	putWasmRegionInstr(program, 8, OP_BNE, 0, 3, 4, 24)
	putWasmRegionInstr(program, 16, OP_ADD, 1, 1, 5, 0) // cold tail, not in region
	putWasmRegionInstr(program, 24, OP_ADD, 1, 1, 5, 0) // discarded
	putWasmRegionInstr(program, 32, OP_ADD, 1, 1, 6, 0)
	blocks := []wasmRegionBlock{
		{pc: PROG_START, instrs: scanBlock(program, 0)[:2], kind: ie64ObservedConditional, hotTarget: PROG_START + 32, coldTarget: PROG_START + 16},
		{pc: PROG_START + 32, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 6}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(39)}}, hotTarget: PROG_START},
	}
	compile := func([]byte) ([]byte, error) { return wasmCompileBlocks(blocks) }
	// Hot: BNE true. The path falls through the conditional into the next
	// block; the loop then consumes the chain budget, so the retired count
	// is far beyond the conditional's own index.
	hot := runWasmDiffCompiled(t, program, map[int]uint64{3: 1, 4: 2, 6: 2}, nil, compile)
	if hot.retPC != PROG_START || hot.retCount <= 2 || hot.regs[1] == 0 {
		t.Fatalf("hot=%+v, want fall-through into the loop body", hot)
	}
	// Cold: BNE false. The structured arm exits at the cold target with the
	// conditional retired.
	cold := runWasmDiffCompiled(t, program, map[int]uint64{3: 7, 4: 7, 6: 2}, nil, compile)
	if cold.retPC != PROG_START+16 || cold.retCount != 2 || cold.regs[1] != 0 {
		t.Fatalf("cold=%+v, want exit at cold target after 2 retired", cold)
	}
}

func TestWasmObservedIndirectHitAndMismatch(t *testing.T) {
	program := make([]byte, 24)
	putWasmRegionInstr(program, 0, OP_JMP, 0, 3, 0, ^uint32(7))
	putWasmRegionInstr(program, 16, OP_ADD, 1, 1, 2, 0)
	blocks := []wasmRegionBlock{
		{pc: PROG_START, instrs: []JITInstr{{opcode: OP_JMP, rs: 3, imm32: ^uint32(7)}}, kind: ie64ObservedIndirectJMP, hotTarget: PROG_START + 16, predictedTarget: PROG_START + 16},
		{pc: PROG_START + 16, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(23)}}, hotTarget: PROG_START},
	}
	compile := func([]byte) ([]byte, error) { return wasmCompileBlocks(blocks) }
	hit := runWasmDiffCompiled(t, program, map[int]uint64{1: 1, 2: 2, 3: PROG_START + 24}, nil, compile)
	if hit.regs[1] != 3 || hit.retPC != PROG_START {
		t.Fatalf("hit=%+v", hit)
	}
	mismatch := runWasmDiffCompiled(t, program, map[int]uint64{1: 1, 2: 2, 3: 0x1_0000_0010}, nil, compile)
	if mismatch.regs[1] != 1 || mismatch.retPC != 0x1_0000_0008 || mismatch.retCount != 1 {
		t.Fatalf("mismatch=%+v", mismatch)
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
