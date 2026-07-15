//go:build arm64 && linux

package main

import (
	"encoding/binary"
	"math"
	"runtime"
	"testing"
	"unsafe"
)

func putIE64RegionInstrARM64(memory []byte, offset uint32, opcode, rd, rs, rt byte, imm32 uint32) {
	memory[offset] = opcode
	memory[offset+1] = rd << 3
	memory[offset+2] = rs << 3
	memory[offset+3] = rt << 3
	binary.LittleEndian.PutUint32(memory[offset+4:], imm32)
}

func putIE64RegionBRAARM64(memory []byte, offset, target uint32) {
	displacement := uint32(int32(target) - int32(offset))
	putIE64RegionInstrARM64(memory, offset, OP_BRA, 0, 0, 0, displacement)
}

func TestARM64Region_FormsAndCompilesNonMMUChain(t *testing.T) {
	memory := make([]byte, 0x1000)
	putIE64RegionInstrARM64(memory, 0x100, OP_ADD, 0, 0, 0, 0)
	putIE64RegionBRAARM64(memory, 0x108, 0x200)
	putIE64RegionInstrARM64(memory, 0x200, OP_ADD, 0, 0, 0, 0)
	putIE64RegionInstrARM64(memory, 0x208, OP_RTS64, 0, 0, 0, 0)

	region := ie64FormRegion(0x100, memory)
	if region == nil || len(region.blocks) != 2 {
		t.Fatalf("ie64FormRegion returned %#v, want two blocks", region)
	}

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := ie64CompileRegion(region, execMem, memory)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	if block.startPC != 0x100 || block.instrCount != 4 || len(block.coveredRanges) != 2 {
		t.Fatalf("compiled block = {startPC:%#x instrCount:%d ranges:%v}", block.startPC, block.instrCount, block.coveredRanges)
	}
	if block.execAddr == 0 || block.execSize == 0 {
		t.Fatalf("compiled native code = {addr:%#x size:%d}", block.execAddr, block.execSize)
	}
}

func TestARM64Region_FormsMMUVirtualChain(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)
	putIE64RegionInstrARM64(rig.cpu.memory, 0x100, OP_ADD, 0, 0, 0, 0)
	putIE64RegionBRAARM64(rig.cpu.memory, 0x108, 0x200)
	putIE64RegionInstrARM64(rig.cpu.memory, 0x200, OP_ADD, 0, 0, 0, 0)
	putIE64RegionInstrARM64(rig.cpu.memory, 0x208, OP_RTS64, 0, 0, 0, 0)

	region := ie64FormRegionMMU(rig.cpu, 0x100)
	if region == nil {
		t.Fatal("ie64FormRegionMMU returned nil for a valid virtual chain")
	}
	if len(region.blocks) != 2 || region.blockPCs[0] != 0x100 || region.blockPCs[1] != 0x200 {
		t.Fatalf("MMU region = {blockPCs:%v blocks:%d}, want virtual PCs [0x100 0x200]", region.blockPCs, len(region.blocks))
	}
}

func TestARM64Region_MMUMarksAtomicBail(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)
	putIE64RegionInstrARM64(rig.cpu.memory, 0x100, OP_ADD, 0, 0, 0, 0)
	putIE64RegionBRAARM64(rig.cpu.memory, 0x108, 0x200)
	putIE64RegionInstrARM64(rig.cpu.memory, 0x200, OP_CAS, 1, 2, 3, 0)

	region := ie64FormRegionMMU(rig.cpu, 0x100)
	if region == nil {
		t.Fatal("ie64FormRegionMMU returned nil for an atomic MMU region")
	}
	if !region.blocks[1][0].mmuBail {
		t.Fatal("MMU region did not mark OP_CAS for interpreter bailout")
	}
}

func TestARM64Region_MMUCompilesPhysicalCodeWithVirtualExits(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	mmuMap(rig.cpu, 0x1100, 4, flags)
	mmuMap(rig.cpu, 0x2200, 5, flags)
	putIE64RegionInstrARM64(rig.cpu.memory, 0x4100, OP_ADD, 1, 1, 2, 0)
	putIE64RegionBRAARM64(rig.cpu.memory, 0x4108, 0x5200)
	putIE64RegionInstrARM64(rig.cpu.memory, 0x5200, OP_ADD, 1, 1, 2, 0)
	putIE64RegionInstrARM64(rig.cpu.memory, 0x5208, OP_JMP, 0, 0, 0, 0x100000)

	region := ie64FormRegionMMU(rig.cpu, 0x1100)
	if region == nil || len(region.blocks) != 2 {
		t.Fatalf("ie64FormRegionMMU returned %#v, want two virtually linked blocks", region)
	}
	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := ie64CompileRegion(region, execMem, rig.cpu.memory)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	ctx := newJITContext(rig.cpu)
	ctx.MMUEnabled = 1
	rig.cpu.regs[2] = 1
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))

	if ctx.RetPC != 0x100000 || ctx.RetCount != 4 {
		t.Fatalf("MMU region exit = {PC:%#x count:%d}, want {PC:0x100000 count:4}", ctx.RetPC, ctx.RetCount)
	}
	if rig.cpu.regs[1] != 2 {
		t.Fatalf("R1 = %d, want both physical code blocks to execute", rig.cpu.regs[1])
	}
	wantRanges := [][2]uint64{{0x1100, 0x1110}, {0x2200, 0x2210}}
	if len(block.coveredRanges) != len(wantRanges) {
		t.Fatalf("covered ranges = %v, want %v", block.coveredRanges, wantRanges)
	}
	for i := range wantRanges {
		if block.coveredRanges[i] != wantRanges[i] {
			t.Fatalf("covered ranges = %v, want %v", block.coveredRanges, wantRanges)
		}
	}
	runtime.KeepAlive(ctx)
}

func TestARM64Region_BackEdgeRetainsBudgetExit(t *testing.T) {
	memory := make([]byte, 0x1000)
	putIE64RegionInstrARM64(memory, 0x100, OP_ADD, 0, 0, 0, 0)
	putIE64RegionBRAARM64(memory, 0x108, 0x200)
	putIE64RegionInstrARM64(memory, 0x200, OP_ADD, 0, 0, 0, 0)
	putIE64RegionBRAARM64(memory, 0x208, 0x100)

	region := ie64FormRegion(0x100, memory)
	if region == nil || len(region.blocks) != 2 {
		t.Fatalf("ie64FormRegion returned %#v, want two-block loop", region)
	}
	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := ie64CompileRegion(region, execMem, memory)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	if block.execSize < 100 {
		t.Fatalf("region code size = %d, budget exit sequence appears absent", block.execSize)
	}
}

func TestARM64Region_BackEdgeExecutesAndReturnsAtBudget(t *testing.T) {
	rig := newJITTestRig(t)
	memory := rig.cpu.memory
	putIE64RegionInstrARM64(memory, 0x100, OP_ADD, 0, 0, 0, 0)
	putIE64RegionBRAARM64(memory, 0x108, 0x200)
	putIE64RegionInstrARM64(memory, 0x200, OP_ADD, 0, 0, 0, 0)
	putIE64RegionBRAARM64(memory, 0x208, 0x100)

	region := ie64FormRegion(0x100, memory)
	block, err := ie64CompileRegion(region, rig.execMem, memory)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	rig.ctx.RegsPtr = uintptr(unsafe.Pointer(&rig.cpu.regs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&rig.cpu.memory[0]))
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))

	if rig.ctx.RetPC != 0x100 {
		t.Fatalf("RetPC = %#x, want loop head 0x100", rig.ctx.RetPC)
	}
	if rig.ctx.RetCount != jitBudget+1 {
		t.Fatalf("RetCount = %d, want %d at the first budget exit", rig.ctx.RetCount, jitBudget+1)
	}
	runtime.KeepAlive(rig.ctx)
	runtime.KeepAlive(rig.execMem)
}

func TestARM64Region_LaterHelperExitSpillsPriorBlockWrites(t *testing.T) {
	rig := newJITTestRig(t)
	memory := rig.cpu.memory
	copy(memory[0x100:], ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42))
	putIE64RegionBRAARM64(memory, 0x108, 0x200)
	copy(memory[0x200:], ie64Instr(OP_LOAD, 2, IE64_SIZE_Q, 0, 3, 0, 0))
	putIE64RegionBRAARM64(memory, 0x208, 0x300)
	putIE64RegionInstrARM64(memory, 0x300, OP_RTS64, 0, 0, 0, 0)

	region := ie64FormRegion(0x100, memory)
	block, err := ie64CompileRegion(region, rig.execMem, memory)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	const highAddress uint64 = 0x0000_0001_0000_8000
	rig.cpu.regs[3] = highAddress
	rig.ctx.RegsPtr = uintptr(unsafe.Pointer(&rig.cpu.regs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&rig.cpu.memory[0]))
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))

	if rig.ctx.NeedHelper != HELPER_LOAD {
		t.Fatal("later-block LOAD did not take its helper exit")
	}
	if rig.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d after later-block helper exit, want prior-block write 42", rig.cpu.regs[1])
	}
	runtime.KeepAlive(rig.ctx)
	runtime.KeepAlive(rig.execMem)
}

func TestARM64Region_ClearsPendingFPCCBetweenBlocks(t *testing.T) {
	rig := newJITTestRig(t)
	memory := rig.cpu.memory
	copy(memory[0x100:], ie64Instr(OP_FADD, 1, 0, 0, 2, 3, 0))
	putIE64RegionBRAARM64(memory, 0x108, 0x200)
	copy(memory[0x200:], ie64Instr(OP_DADD, 4, IE64_SIZE_L, 0, 6, 8, 0))
	copy(memory[0x208:], ie64Instr(OP_JMP, 0, 0, 0, 1, 0, 0))

	region := ie64FormRegion(0x100, memory)
	if region == nil || len(region.blocks) != 2 {
		t.Fatalf("ie64FormRegion returned %#v, want two FP blocks", region)
	}
	block, err := ie64CompileRegion(region, rig.execMem, memory)
	if err != nil {
		t.Fatalf("ie64CompileRegion: %v", err)
	}
	rig.cpu.regs[1] = 0x300
	rig.cpu.FPU.FPRegs[2] = math.Float32bits(-1)
	rig.cpu.FPU.FPRegs[3] = math.Float32bits(1)
	rig.cpu.FPU.setDPair(6, 1)
	rig.cpu.FPU.setDPair(8, 2)
	rig.ctx.RegsPtr = uintptr(unsafe.Pointer(&rig.cpu.regs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&rig.cpu.memory[0]))
	rig.ctx.FPUPtr = uintptr(unsafe.Pointer(rig.cpu.FPU))
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))

	if cc := rig.cpu.FPU.FPSR & 0x0F000000; cc != 0 {
		t.Fatalf("FPSR CC = %#x, want positive DADD result after earlier sunk zero result", cc)
	}
	runtime.KeepAlive(rig.ctx)
	runtime.KeepAlive(rig.execMem)
}
