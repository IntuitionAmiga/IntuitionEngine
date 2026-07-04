// jit_mmu_microtlb_test.go - IE64 JIT MMU micro-TLB tests.

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestIE64JIT_MMUMixed_ParityWithInterpreter(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	t.Setenv("IE64_JIT_RESUME", "1")

	build := func(jit bool) *CPU64 {
		rig := newIE64TestRig()
		cpu := rig.cpu
		cpu.jitEnabled = jit
		cpu.CoprocMode = true
		setupIdentityMMU(cpu, 160)
		writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
		binary.LittleEndian.PutUint32(cpu.memory[0x7100:], 0xABCD1234)
		rig.loadInstructions(
			ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3100),
			ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),
			ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 1, 0, 0),
			ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		)
		return cpu
	}

	base := ie64JITStatsLoad()
	jitCPU := build(true)
	jitCPU.running.Store(true)
	jitCPU.jitExecute()
	diff := ie64JITStatsLoad().Sub(base)

	interpCPU := build(false)
	interpCPU.running.Store(true)
	interpCPU.Execute()

	if jitCPU.regs[2] != interpCPU.regs[2] || jitCPU.regs[3] != interpCPU.regs[3] {
		t.Fatalf("JIT/interpreter mismatch: JIT R2=0x%X R3=0x%X, interp R2=0x%X R3=0x%X",
			jitCPU.regs[2], jitCPU.regs[3], interpCPU.regs[2], interpCPU.regs[3])
	}
	if jitCPU.regs[2] != 0xABCD1234 || jitCPU.regs[3] != 0xABCD1234 {
		t.Fatalf("JIT regs R2=0x%X R3=0x%X, want both 0xABCD1234", jitCPU.regs[2], jitCPU.regs[3])
	}
	if diff.helperExits[HELPER_LOAD] != 1 {
		t.Fatalf("LOAD helper exits = %d, want 1 (second LOAD should hit native micro-TLB)", diff.helperExits[HELPER_LOAD])
	}
	if diff.helperResumes != 1 {
		t.Fatalf("helperResumes = %d, want 1", diff.helperResumes)
	}
}

func TestIE64JIT_MicroTLBStoreSMCRecordsVirtualAddress(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	r := newJITTestRig(t)
	cpu := r.cpu
	setupIdentityMMU(cpu, 160)
	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	copy(cpu.memory[PROG_START:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0))
	instrs := scanBlock(cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}
	block, err := compileBlockMMU(instrs[:1], PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlockMMU: %v", err)
	}

	const virt = 0x3100
	const phys = 0x7100
	cpu.regs[1] = virt
	cpu.regs[2] = 0xAABBCCDD

	bitmap := make([]byte, 0x40)
	bitmap[virt>>8] = 1
	r.ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&bitmap[0]))
	r.ctx.CodePageBitmapLen = uint32(len(bitmap))
	r.ctx.MMUEnabled = 1
	r.ctx.refreshMicroTLBPrefixes(cpu)
	idx := uint64(virt>>MMU_PAGE_SHIFT) & (jitCtxMicroTLBEntries - 1)
	r.ctx.MicroTLBKeys[idx] = ie64MicroTLBKey(cpu, virt, ACCESS_WRITE)
	r.ctx.MicroTLBPhys[idx] = phys & ^uint64(MMU_PAGE_MASK)

	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	if got := binary.LittleEndian.Uint32(cpu.memory[phys:]); got != 0xAABBCCDD {
		t.Fatalf("physical store = 0x%08X, want 0xAABBCCDD", got)
	}
	if r.ctx.NeedInval != 1 {
		t.Fatal("NeedInval was not set for STORE to marked virtual code page")
	}
	if r.ctx.InvalAddr != virt {
		t.Fatalf("InvalAddr = 0x%X, want virtual address 0x%X", r.ctx.InvalAddr, uint64(virt))
	}
	if r.ctx.InvalSize != 4 {
		t.Fatalf("InvalSize = %d, want 4", r.ctx.InvalSize)
	}
}

func TestIE64JIT_MicroTLBStoreSMCPhysicalAliasForcesFullInvalidation(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	r := newJITTestRig(t)
	cpu := r.cpu
	setupIdentityMMU(cpu, 160)
	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
	writePTE(cpu, 4, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	copy(cpu.memory[PROG_START:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0))
	instrs := scanBlock(cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}
	block, err := compileBlockMMU(instrs[:1], PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlockMMU: %v", err)
	}

	const vaCode = 0x3100
	const vaAlias = 0x4100
	const phys = 0x7100
	cpu.regs[1] = vaAlias
	cpu.regs[2] = 0x55667788

	virtBitmap := make([]byte, 0x80)
	virtBitmap[vaCode>>8] = 1
	if virtBitmap[vaAlias>>8] != 0 {
		t.Fatalf("test setup marked alias virtual page 0x%X", vaAlias>>8)
	}
	physBitmap := make([]byte, 0x80)
	physBitmap[phys>>8] = 1
	r.ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&virtBitmap[0]))
	r.ctx.CodePageBitmapLen = uint32(len(virtBitmap))
	r.ctx.PhysCodeBitmapPtr = uintptr(unsafe.Pointer(&physBitmap[0]))
	r.ctx.PhysCodeBitmapLen = uint32(len(physBitmap))
	r.ctx.MMUEnabled = 1
	r.ctx.refreshMicroTLBPrefixes(cpu)
	idx := uint64(vaAlias>>MMU_PAGE_SHIFT) & (jitCtxMicroTLBEntries - 1)
	r.ctx.MicroTLBKeys[idx] = ie64MicroTLBKey(cpu, vaAlias, ACCESS_WRITE)
	r.ctx.MicroTLBPhys[idx] = phys & ^uint64(MMU_PAGE_MASK)

	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	if got := binary.LittleEndian.Uint32(cpu.memory[phys:]); got != 0x55667788 {
		t.Fatalf("physical store = 0x%08X, want 0x55667788", got)
	}
	if r.ctx.NeedInval != 1 {
		t.Fatal("NeedInval was not set for STORE through a physical code alias")
	}
	if r.ctx.InvalSize != 0 {
		t.Fatalf("InvalSize = %d, want 0 for full alias invalidation", r.ctx.InvalSize)
	}
}

func TestIE64JIT_MicroTLBStoreSMCPTBRZeroPhysicalAlias(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	mmuTestResetPools()
	defer mmuTestResetPools()

	r := newJITTestRig(t)
	cpu := r.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0

	const (
		vaCode    = uint64(0x3100)
		vaAlias   = uint64(0x4100)
		phys      = uint64(0x20100)
		instrAddr = uint64(0x9000)
	)
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	writePTE(cpu, vaCode>>MMU_PAGE_SHIFT, makePTE(phys>>MMU_PAGE_SHIFT, flags))
	writePTE(cpu, vaAlias>>MMU_PAGE_SHIFT, makePTE(phys>>MMU_PAGE_SHIFT, flags))
	cpu.tlbFlush()

	copy(cpu.memory[instrAddr:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0))
	instrs := scanBlock(cpu.memory, instrAddr)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}
	storeBlock, err := compileBlockMMU(instrs[:1], instrAddr, r.execMem)
	if err != nil {
		t.Fatalf("compileBlockMMU: %v", err)
	}

	cpu.regs[1] = vaAlias
	cpu.regs[2] = 0x99AABBCC

	virtBitmap := make([]byte, 0x80)
	physBitmap := make([]byte, 0x300)
	cpu.jitCache = NewCodeCache()
	codeBlock := &JITBlock{startPC: vaCode, endPC: vaCode + IE64_INSTR_SIZE}
	cpu.jitCache.PutMMU(cpu.ptbr, vaCode, codeBlock)
	ie64MarkCodePagesForBlockContext(virtBitmap, r.ctx, codeBlock)
	ie64MarkPhysicalCodePagesForBlock(physBitmap, cpu.bus, codeBlock)
	if physBitmap[phys>>8] == 0 {
		t.Fatalf("physical bitmap page 0x%X was not marked for PTBR-zero MMU block", phys>>8)
	}
	if virtBitmap[vaAlias>>8] != 0 {
		t.Fatalf("test setup marked alias virtual page 0x%X", vaAlias>>8)
	}
	r.ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&virtBitmap[0]))
	r.ctx.CodePageBitmapLen = uint32(len(virtBitmap))
	r.ctx.PhysCodeBitmapPtr = uintptr(unsafe.Pointer(&physBitmap[0]))
	r.ctx.PhysCodeBitmapLen = uint32(len(physBitmap))
	r.ctx.MMUEnabled = 1
	r.ctx.refreshMicroTLBPrefixes(cpu)
	idx := uint64(vaAlias>>MMU_PAGE_SHIFT) & (jitCtxMicroTLBEntries - 1)
	r.ctx.MicroTLBKeys[idx] = ie64MicroTLBKey(cpu, vaAlias, ACCESS_WRITE)
	r.ctx.MicroTLBPhys[idx] = phys & ^uint64(MMU_PAGE_MASK)

	callNative(storeBlock.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	if got := binary.LittleEndian.Uint32(cpu.memory[phys:]); got != 0x99AABBCC {
		t.Fatalf("physical store = 0x%08X, want 0x99AABBCC", got)
	}
	if r.ctx.NeedInval != 1 {
		t.Fatal("NeedInval was not set for PTBR-zero physical code alias")
	}
	if r.ctx.InvalSize != 0 {
		t.Fatalf("InvalSize = %d, want 0 for full PTBR-zero alias invalidation", r.ctx.InvalSize)
	}
}

func TestIE64JIT_MicroTLBStoreSMCSkipsHighVirtualBitmapOutOfRange(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	r := newJITTestRig(t)
	cpu := r.cpu
	setupIdentityMMU(cpu, 160)

	copy(cpu.memory[PROG_START:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0))
	instrs := scanBlock(cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}
	block, err := compileBlockMMU(instrs[:1], PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlockMMU: %v", err)
	}

	const virt = uint64(0x1_0000_3100)
	const phys = uint64(0x7100)
	cpu.regs[1] = virt
	cpu.regs[2] = 0x11223344

	bitmap := make([]byte, 1)
	bitmap[0] = 1
	r.ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&bitmap[0]))
	r.ctx.CodePageBitmapLen = uint32(len(bitmap))
	r.ctx.MMUEnabled = 1
	r.ctx.refreshMicroTLBPrefixes(cpu)
	idx := (virt >> MMU_PAGE_SHIFT) & (jitCtxMicroTLBEntries - 1)
	r.ctx.MicroTLBKeys[idx] = ie64MicroTLBKey(cpu, virt, ACCESS_WRITE)
	r.ctx.MicroTLBPhys[idx] = phys & ^uint64(MMU_PAGE_MASK)

	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	if got := binary.LittleEndian.Uint32(cpu.memory[phys:]); got != 0x11223344 {
		t.Fatalf("physical store = 0x%08X, want 0x11223344", got)
	}
	if r.ctx.NeedInval != 0 {
		t.Fatalf("NeedInval = %d, want 0 for out-of-range virtual SMC bitmap page", r.ctx.NeedInval)
	}
	if r.ctx.InvalSize != 0 {
		t.Fatalf("InvalSize = %d, want 0 for out-of-range virtual SMC bitmap page", r.ctx.InvalSize)
	}
}

func TestIE64JIT_MicroTLB_FlushOnPTBRWrite(t *testing.T) {
	cpu := NewCPU64(NewMachineBus())
	cpu.jitCtx = newJITContext(cpu)
	cpu.jitCtx.MicroTLBKeys[0] = ie64MicroTLBValid | 0x123
	cpu.jitCtx.MicroTLBPhys[0] = 0x456000

	cpu.ptbr = 0x80000
	cpu.tlbFlush()

	if cpu.jitCtx.MicroTLBKeys[0] != 0 || cpu.jitCtx.MicroTLBPhys[0] != 0 {
		t.Fatalf("micro-TLB entry survived PTBR/TLB flush: key=0x%X phys=0x%X",
			cpu.jitCtx.MicroTLBKeys[0], cpu.jitCtx.MicroTLBPhys[0])
	}
}

func TestIE64JIT_MicroTLB_FlushOnInvalidate(t *testing.T) {
	cpu := NewCPU64(NewMachineBus())
	cpu.jitCtx = newJITContext(cpu)
	key3 := ie64MicroTLBKey(cpu, 3<<MMU_PAGE_SHIFT, ACCESS_READ)
	key4 := ie64MicroTLBKey(cpu, 4<<MMU_PAGE_SHIFT, ACCESS_READ)
	cpu.jitCtx.MicroTLBKeys[0] = key4
	cpu.jitCtx.MicroTLBPhys[0] = 0x4000
	cpu.jitCtx.MicroTLBKeys[3] = key3
	cpu.jitCtx.MicroTLBPhys[3] = 0x3000

	cpu.tlbInvalidate(3)

	if cpu.jitCtx.MicroTLBKeys[3] != 0 || cpu.jitCtx.MicroTLBPhys[3] != 0 {
		t.Fatalf("invalidated VPN entry survived: key=0x%X phys=0x%X",
			cpu.jitCtx.MicroTLBKeys[3], cpu.jitCtx.MicroTLBPhys[3])
	}
	if cpu.jitCtx.MicroTLBKeys[0] != key4 || cpu.jitCtx.MicroTLBPhys[0] != 0x4000 {
		t.Fatalf("unrelated VPN entry changed: key=0x%X phys=0x%X",
			cpu.jitCtx.MicroTLBKeys[0], cpu.jitCtx.MicroTLBPhys[0])
	}
}
