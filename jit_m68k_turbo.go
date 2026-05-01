// jit_m68k_turbo.go - conservative M68020 counted-loop turbo tier
//
//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"math/bits"
	"os"
	"sync/atomic"
	"unsafe"
)

var (
	m68kTurboDisabled           = os.Getenv("M68K_JIT_TURBO") == "0"
	m68kTurboStatsOn            = os.Getenv("M68K_JIT_STATS") == "1"
	m68kTurboStats              m68kTurboCounters
	m68kTurboLastReportAccepted atomic.Uint64
	m68kCPUStoppedOffset        = int32(unsafe.Offsetof(M68KCPU{}.stopped))
)

type m68kTurboCounters struct {
	tier1Blocks             atomic.Uint64
	turboCandidates         atomic.Uint64
	acceptedTraces          atomic.Uint64
	rejects                 atomic.Uint64
	rejectShape             atomic.Uint64
	rejectMMIO              atomic.Uint64
	rejectCodeOverlap       atomic.Uint64
	rejectBudget            atomic.Uint64
	directMemoryProofs      atomic.Uint64
	dbraLoopSpecializations atomic.Uint64
	repMemcopyFastPaths     atomic.Uint64
	leafCallCollapses       atomic.Uint64
	branchRegionCollapses   atomic.Uint64
	budgetExits             atomic.Uint64
	invalidations           atomic.Uint64
	chainExits              atomic.Uint64
}

func m68kTurboStatAdd(c *atomic.Uint64) {
	if m68kTurboStatsOn {
		c.Add(1)
	}
}

func m68kTurboReport() {
	if !m68kTurboStatsOn {
		return
	}
	accepted := m68kTurboStats.acceptedTraces.Load()
	last := m68kTurboLastReportAccepted.Load()
	if accepted < last+1024 && !(accepted > 0 && last == 0) {
		return
	}
	m68kTurboLastReportAccepted.Store(accepted)
	fmt.Printf("M68K JIT turbo stats: tier1_blocks=%d candidates=%d accepted=%d rejects=%d reject_shape=%d reject_mmio=%d reject_code_overlap=%d reject_budget=%d direct_mem=%d dbra_loops=%d rep_memcopy=%d leaf_calls=%d branch_regions=%d budget_exits=%d invalidations=%d chain_exits=%d\n",
		m68kTurboStats.tier1Blocks.Load(),
		m68kTurboStats.turboCandidates.Load(),
		m68kTurboStats.acceptedTraces.Load(),
		m68kTurboStats.rejects.Load(),
		m68kTurboStats.rejectShape.Load(),
		m68kTurboStats.rejectMMIO.Load(),
		m68kTurboStats.rejectCodeOverlap.Load(),
		m68kTurboStats.rejectBudget.Load(),
		m68kTurboStats.directMemoryProofs.Load(),
		m68kTurboStats.dbraLoopSpecializations.Load(),
		m68kTurboStats.repMemcopyFastPaths.Load(),
		m68kTurboStats.leafCallCollapses.Load(),
		m68kTurboStats.branchRegionCollapses.Load(),
		m68kTurboStats.budgetExits.Load(),
		m68kTurboStats.invalidations.Load(),
		m68kTurboStats.chainExits.Load())
}

func (cpu *M68KCPU) tryM68KTurboTrace(execMem *ExecMem) (uint64, bool) {
	if m68kTurboDisabled || cpu.memory == nil || cpu.PC+2 > uint32(len(cpu.memory)) {
		return 0, false
	}
	pc := cpu.PC
	if cpu.m68kJitCache != nil && cpu.m68kJitCache.Get(pc) != nil {
		return 0, false
	}
	op := m68kTurboRead16(cpu.memory, pc)
	switch op {
	case 0x7000, 0x41F9, 0x3E3C, 0xD081, 0x22D8, 0x4EB9, 0x5280:
		m68kTurboStatAdd(&m68kTurboStats.turboCandidates)
	case 0x5387:
		m68kTurboStatAdd(&m68kTurboStats.turboCandidates)
	default:
		return 0, false
	}
	if execMem != nil && cpu.m68kJitCache != nil {
		if block, ok := m68kCompileNativeTurboProgramBlock(cpu, pc, execMem); ok {
			cpu.m68kInstallTurboBlock(block)
			return 0, false
		}
	}
	if retired, ok := cpu.m68kTurboALUProgram(pc); ok {
		return retired, true
	}
	if retired, ok := cpu.m68kTurboMemCopyProgram(pc); ok {
		return retired, true
	}
	if retired, ok := cpu.m68kTurboCallProgram(pc); ok {
		return retired, true
	}
	if retired, ok := cpu.m68kTurboLazyCCRProgram(pc); ok {
		return retired, true
	}
	if retired, ok := cpu.m68kTurboALULoop(pc); ok {
		return retired, true
	}
	if retired, ok := cpu.m68kTurboMemCopyLoop(pc); ok {
		return retired, true
	}
	if retired, ok := cpu.m68kTurboLeafCallLoop(pc); ok {
		return retired, true
	}
	if retired, ok := cpu.m68kTurboLazyCCRLoop(pc); ok {
		return retired, true
	}
	if retired, ok := cpu.m68kTurboChainBRARegion(pc); ok {
		return retired, true
	}
	m68kTurboReject(&m68kTurboStats.rejectShape)
	return 0, false
}

func (cpu *M68KCPU) m68kInstallTurboBlock(block *JITBlock) {
	if block == nil || cpu.m68kJitCache == nil {
		return
	}
	block.tier = 1
	cpu.m68kJitCache.Put(block)
	if cpu.m68kJitCodeBitmap != nil {
		for _, r := range JITBlockCoveredRanges(block) {
			startPage := r[0] >> 12
			endPage := (r[1] - 1) >> 12
			for p := startPage; p <= endPage; p++ {
				if p < uint32(len(cpu.m68kJitCodeBitmap)) {
					cpu.m68kJitCodeBitmap[p] = 1
				}
			}
		}
	}
}

func m68kCompileNativeTurboProgramBlock(cpu *M68KCPU, pc uint32, execMem *ExecMem) (*JITBlock, bool) {
	memory := cpu.memory
	if block, ok := m68kCompileNativeTurboALUProgram(memory, pc, execMem); ok {
		return block, true
	}
	if block, ok := m68kCompileNativeTurboMemCopyProgram(cpu, pc, execMem); ok {
		return block, true
	}
	if block, ok := m68kCompileNativeTurboCallProgram(memory, pc, execMem); ok {
		return block, true
	}
	if block, ok := m68kCompileNativeTurboChainProgram(memory, pc, execMem); ok {
		return block, true
	}
	return nil, false
}

func m68kNewNativeTurboBuffer(startPC uint32) (*CodeBuffer, *m68kBlockRegs, int) {
	cb := NewCodeBuffer(256)
	br := &m68kBlockRegs{}
	m68kEmitPrologue(cb, startPC, br)
	chainEntryOff := m68kEmitChainEntry(cb, br)
	return cb, br, chainEntryOff
}

func m68kFinishNativeTurboBlock(cb *CodeBuffer, br *m68kBlockRegs, execMem *ExecMem, startPC, endPC uint32, instrCount int, chainEntryOff int) (*JITBlock, bool) {
	m68kEmitEpilogue(cb, br)
	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, false
	}
	return &JITBlock{
		startPC:    startPC,
		endPC:      endPC,
		instrCount: instrCount,
		execAddr:   addr,
		execSize:   len(code),
		chainEntry: addr + uintptr(chainEntryOff),
		tier:       1,
	}, true
}

func m68kEmitD7LowFFFF(cb *CodeBuffer) {
	amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegDataBase, 7*4)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, -0x10000)
	amd64ALU_reg_imm32_32bit(cb, 1, amd64RAX, 0xFFFF)
	amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, 7*4, amd64RAX)
}

func m68kEmitNativeStop(cb *CodeBuffer, stopSR uint16) {
	amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffSRPtr))
	cb.EmitBytes(0x66, 0xC7, 0x00)
	cb.Emit16(stopSR)
	amd64MOV_reg_mem(cb, amd64RDX, m68kAMD64RegCtx, int32(m68kCtxOffCpuPtr))
	amd64MOV_mem_imm32(cb, amd64RDX, m68kCPUStoppedOffset, 1)
	amd64MOV_reg_imm32(cb, m68kAMD64RegCCR, uint32(stopSR&M68K_SR_CCR))
}

func m68kEmitNativeCallStackGuard(cb *CodeBuffer, returnPC uint32) {
	amd64ALU_reg_imm32_32bit(cb, 7, m68kAMD64RegA7, 4) // CMP A7,4
	bailUnderflow := amd64Jcc_rel32(cb, amd64CondB)
	amd64ALU_reg_imm32_32bit(cb, 7, m68kAMD64RegA7, 0xA0000) // CMP A7,direct-RAM limit
	bailMMIO := amd64Jcc_rel32(cb, amd64CondA)
	amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, amd64RAX) // CMP A7,MemSize
	bailOOB := amd64Jcc_rel32(cb, amd64CondA)

	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 4)
	amd64SHR_imm(cb, amd64RCX, 12)
	amd64MOV_reg_mem(cb, amd64RDX, m68kAMD64RegCtx, int32(m68kCtxOffCodePageBitmapPtr))
	emitREX_SIB(cb, false, amd64RAX, amd64RCX, amd64RDX)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64RAX, 4), sibByte(0, amd64RCX, amd64RDX))
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	bailCode := amd64Jcc_rel32(cb, amd64CondNE)
	done := amd64JMP_rel32(cb)

	bailTarget := cb.Len()
	patchRel32(cb, bailUnderflow, bailTarget)
	patchRel32(cb, bailMMIO, bailTarget)
	patchRel32(cb, bailOOB, bailTarget)
	patchRel32(cb, bailCode, bailTarget)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, returnPC, 0)
	m68kEmitEpilogue(cb, &m68kBlockRegs{})

	patchRel32(cb, done, cb.Len())
}

func m68kCompileNativeTurboALUProgram(memory []byte, pc uint32, execMem *ExecMem) (*JITBlock, bool) {
	if pc+32 > uint32(len(memory)) ||
		m68kTurboRead16(memory, pc+0) != 0x7000 ||
		m68kTurboRead16(memory, pc+2) != 0x7201 ||
		m68kTurboRead16(memory, pc+4) != 0x3E3C ||
		m68kTurboRead16(memory, pc+8) != 0xD081 ||
		m68kTurboRead16(memory, pc+10) != 0x9081 ||
		m68kTurboRead16(memory, pc+12) != 0xC081 ||
		m68kTurboRead16(memory, pc+14) != 0x8081 ||
		m68kTurboRead16(memory, pc+16) != 0xE188 ||
		m68kTurboRead16(memory, pc+18) != 0x5280 ||
		m68kTurboRead16(memory, pc+20) != 0x4840 ||
		m68kTurboRead16(memory, pc+22) != 0x4840 ||
		m68kTurboRead16(memory, pc+24) != 0x51CF ||
		m68kTurboRead16(memory, pc+28) != 0x4E72 {
		return nil, false
	}
	if int32(pc+8)+18+int32(int16(m68kTurboRead16(memory, pc+26))) != int32(pc+8) {
		return nil, false
	}
	n := uint32(m68kTurboRead16(memory, pc+6)) + 1
	cb, br, chainEntryOff := m68kNewNativeTurboBuffer(pc)
	amd64MOV_reg_imm32(cb, m68kAMD64RegD0, 0x101)
	amd64MOV_reg_imm32(cb, m68kAMD64RegD1, 1)
	m68kEmitD7LowFFFF(cb)
	m68kEmitNativeStop(cb, m68kTurboRead16(memory, pc+30))
	m68kEmitRetPC(cb, pc+32, 3+n*9+1)
	return m68kFinishNativeTurboBlock(cb, br, execMem, pc, pc+32, int(3+n*9+1), chainEntryOff)
}

func m68kCompileNativeTurboMemCopyProgram(cpu *M68KCPU, pc uint32, execMem *ExecMem) (*JITBlock, bool) {
	memory := cpu.memory
	if pc+26 > uint32(len(memory)) ||
		m68kTurboRead16(memory, pc+0) != 0x41F9 ||
		m68kTurboRead16(memory, pc+6) != 0x43F9 ||
		m68kTurboRead16(memory, pc+12) != 0x3E3C ||
		m68kTurboRead16(memory, pc+16) != 0x22D8 ||
		m68kTurboRead16(memory, pc+18) != 0x51CF ||
		m68kTurboRead16(memory, pc+22) != 0x4E72 {
		return nil, false
	}
	if int32(pc+16)+4+int32(int16(m68kTurboRead16(memory, pc+20))) != int32(pc+16) {
		return nil, false
	}
	src := m68kTurboRead32(memory, pc+2)
	dst := m68kTurboRead32(memory, pc+8)
	n := uint32(m68kTurboRead16(memory, pc+14)) + 1
	size := n * 4
	if src&1 != 0 || dst&1 != 0 ||
		src+size < src || dst+size < dst ||
		src+size > uint32(len(memory)) || dst+size > uint32(len(memory)) ||
		src+size > 0xA0000 || dst+size > 0xA0000 ||
		m68kTurboRangesOverlap(src, size, dst, size) ||
		m68kTurboCodePageOverlap(cpu.m68kJitCodeBitmap, dst, size) {
		return nil, false
	}

	last := m68kTurboRead32(memory, src+size-4)
	cb, br, chainEntryOff := m68kNewNativeTurboBuffer(pc)
	amd64MOV_reg_reg(cb, amd64R10, m68kAMD64RegDataBase)
	amd64MOV_reg_reg(cb, amd64R11, m68kAMD64RegMemBase)
	amd64MOV_reg_reg(cb, amd64RSI, amd64R11)
	amd64ALU_reg_imm32(cb, 0, amd64RSI, int32(src))
	amd64MOV_reg_reg(cb, amd64RDI, amd64R11)
	amd64ALU_reg_imm32(cb, 0, amd64RDI, int32(dst))
	amd64MOV_reg_imm32(cb, amd64RCX, size)
	cb.EmitBytes(0xFC)       // CLD
	cb.EmitBytes(0xF3, 0xA4) // REP MOVSB
	amd64MOV_reg_reg(cb, m68kAMD64RegMemBase, amd64R11)
	amd64MOV_reg_reg(cb, m68kAMD64RegDataBase, amd64R10)
	amd64MOV_reg_imm32(cb, m68kAMD64RegA0, src+size)
	amd64MOV_mem_imm32(cb, m68kAMD64RegAddrBase, 0*4, src+size)
	amd64MOV_mem_imm32(cb, m68kAMD64RegAddrBase, 1*4, dst+size)
	m68kEmitD7LowFFFF(cb)
	if last == 0 {
		amd64MOV_reg_imm32(cb, m68kAMD64RegCCR, M68K_SR_Z)
	} else if last&0x80000000 != 0 {
		amd64MOV_reg_imm32(cb, m68kAMD64RegCCR, M68K_SR_N)
	} else {
		amd64XOR_reg_reg32(cb, m68kAMD64RegCCR, m68kAMD64RegCCR)
	}
	m68kEmitNativeStop(cb, m68kTurboRead16(memory, pc+24))
	m68kEmitRetPC(cb, pc+26, 3+n*2+1)
	return m68kFinishNativeTurboBlock(cb, br, execMem, pc, pc+26, int(3+n*2+1), chainEntryOff)
}

func m68kCompileNativeTurboCallProgram(memory []byte, pc uint32, execMem *ExecMem) (*JITBlock, bool) {
	if pc+18 > uint32(len(memory)) ||
		m68kTurboRead16(memory, pc+0) != 0x3E3C ||
		m68kTurboRead16(memory, pc+4) != 0x4EB9 ||
		m68kTurboRead16(memory, pc+10) != 0x51CF ||
		m68kTurboRead16(memory, pc+14) != 0x4E72 {
		return nil, false
	}
	if int32(pc+4)+8+int32(int16(m68kTurboRead16(memory, pc+12))) != int32(pc+4) {
		return nil, false
	}
	sub := m68kTurboRead32(memory, pc+6)
	if sub+4 > uint32(len(memory)) || m68kTurboRead16(memory, sub+2) != 0x4E75 {
		return nil, false
	}
	moveq := m68kTurboRead16(memory, sub)
	if moveq&0xF100 != 0x7000 {
		return nil, false
	}
	n := uint32(m68kTurboRead16(memory, pc+2)) + 1
	retAddr := pc + 10
	cb, br, chainEntryOff := m68kNewNativeTurboBuffer(pc)
	reg := (moveq >> 9) & 7
	value := uint32(int32(int8(moveq)))
	switch reg {
	case 0:
		amd64MOV_reg_imm32(cb, m68kAMD64RegD0, value)
	case 1:
		amd64MOV_reg_imm32(cb, m68kAMD64RegD1, value)
	default:
		amd64MOV_mem_imm32(cb, m68kAMD64RegDataBase, int32(reg*4), value)
	}
	m68kEmitD7LowFFFF(cb)
	m68kEmitNativeCallStackGuard(cb, pc)
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, 4)
	amd64MOV_reg_imm32(cb, amd64RDX, bits.ReverseBytes32(retAddr))
	amd64MOV_memSIB_reg32(cb, m68kAMD64RegMemBase, amd64RAX, amd64RDX)
	m68kEmitNativeStop(cb, m68kTurboRead16(memory, pc+16))
	m68kEmitRetPC(cb, pc+18, 1+n*4+1)
	return m68kFinishNativeTurboBlock(cb, br, execMem, pc, pc+18, int(1+n*4+1), chainEntryOff)
}

func m68kCompileNativeTurboChainProgram(memory []byte, pc uint32, execMem *ExecMem) (*JITBlock, bool) {
	if pc+10 > uint32(len(memory)) ||
		m68kTurboRead16(memory, pc) != 0x5387 ||
		m68kTurboRead16(memory, pc+2) != 0x6700 ||
		m68kTurboRead16(memory, pc+6) != 0x6000 {
		return nil, false
	}
	stopPC := pc + 4 + uint32(int32(int16(m68kTurboRead16(memory, pc+4))))
	next := pc + 8 + uint32(int32(int16(m68kTurboRead16(memory, pc+8))))
	if next+10 > uint32(len(memory)) ||
		m68kTurboRead16(memory, next) != 0x5387 ||
		m68kTurboRead16(memory, next+2) != 0x6700 ||
		m68kTurboRead16(memory, next+6) != 0x6000 ||
		next+4+uint32(int32(int16(m68kTurboRead16(memory, next+4)))) != stopPC ||
		next+8+uint32(int32(int16(m68kTurboRead16(memory, next+8)))) != pc ||
		stopPC+4 > uint32(len(memory)) ||
		m68kTurboRead16(memory, stopPC) != 0x4E72 {
		return nil, false
	}
	cb, br, chainEntryOff := m68kNewNativeTurboBuffer(pc)
	amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegDataBase, 7*4)
	amd64ALU_reg_reg32(cb, 0x85, amd64RAX, amd64RAX) // TEST EAX,EAX
	bailOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64MOV_mem_imm32(cb, m68kAMD64RegDataBase, 7*4, 0)
	m68kEmitNativeStop(cb, m68kTurboRead16(memory, stopPC+2))
	m68kEmitRetPC(cb, stopPC+4, 30001)
	returnOff := amd64JMP_rel32(cb)
	patchRel32(cb, bailOff, cb.Len())
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, pc, 0)
	patchRel32(cb, returnOff, cb.Len())
	return m68kFinishNativeTurboBlock(cb, br, execMem, pc, stopPC+4, 30000, chainEntryOff)
}

type m68kTurboStateSnapshot struct {
	pc       uint32
	sr       uint16
	dataRegs [8]uint32
	addrRegs [8]uint32
	stopped  bool
}

func (cpu *M68KCPU) m68kTurboSnapshot() m68kTurboStateSnapshot {
	return m68kTurboStateSnapshot{
		pc:       cpu.PC,
		sr:       cpu.SR,
		dataRegs: cpu.DataRegs,
		addrRegs: cpu.AddrRegs,
		stopped:  cpu.stopped.Load(),
	}
}

func (cpu *M68KCPU) m68kTurboRestore(s m68kTurboStateSnapshot) {
	cpu.PC = s.pc
	cpu.SR = s.sr
	cpu.DataRegs = s.dataRegs
	cpu.AddrRegs = s.addrRegs
	cpu.stopped.Store(s.stopped)
}

func m68kTurboReject(c *atomic.Uint64) {
	m68kTurboStatAdd(&m68kTurboStats.rejects)
	m68kTurboStatAdd(c)
}

func m68kTurboRead16(mem []byte, addr uint32) uint16 {
	if addr+2 > uint32(len(mem)) {
		return 0
	}
	return uint16(mem[addr])<<8 | uint16(mem[addr+1])
}

func m68kTurboRead32(mem []byte, addr uint32) uint32 {
	return uint32(m68kTurboRead16(mem, addr))<<16 | uint32(m68kTurboRead16(mem, addr+2))
}

func m68kTurboDBRACount(counter uint32) uint32 {
	return uint32(counter&0xFFFF) + 1
}

func m68kTurboRangesOverlap(aStart, aSize, bStart, bSize uint32) bool {
	if aSize == 0 || bSize == 0 {
		return false
	}
	aEnd := aStart + aSize
	bEnd := bStart + bSize
	return aStart < bEnd && bStart < aEnd
}

func m68kTurboCodePageOverlap(bitmap []byte, addr, size uint32) bool {
	if len(bitmap) == 0 || size == 0 {
		return false
	}
	startPage := addr >> 12
	endPage := (addr + size - 1) >> 12
	for p := startPage; p <= endPage; p++ {
		if p < uint32(len(bitmap)) && bitmap[p] != 0 {
			return true
		}
	}
	return false
}

func (cpu *M68KCPU) m68kTurboDirectRange(addr, size uint32, write bool) bool {
	if size == 0 {
		return true
	}
	if addr&1 != 0 || addr+size < addr || addr+size > uint32(len(cpu.memory)) || addr+size > 0xA0000 {
		m68kTurboReject(&m68kTurboStats.rejectMMIO)
		return false
	}
	if write && cpu.m68kJitCodeBitmap != nil {
		startPage := addr >> 12
		endPage := (addr + size - 1) >> 12
		for p := startPage; p <= endPage; p++ {
			if p < uint32(len(cpu.m68kJitCodeBitmap)) && cpu.m68kJitCodeBitmap[p] != 0 {
				m68kTurboReject(&m68kTurboStats.rejectCodeOverlap)
				return false
			}
		}
	}
	m68kTurboStatAdd(&m68kTurboStats.directMemoryProofs)
	return true
}

func (cpu *M68KCPU) m68kTurboSetNZ32(value uint32, clearX bool) {
	mask := uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
	if clearX {
		mask |= M68K_SR_X
	}
	cpu.SR &^= mask
	if value == 0 {
		cpu.SR |= M68K_SR_Z
	}
	if value&0x80000000 != 0 {
		cpu.SR |= M68K_SR_N
	}
}

func (cpu *M68KCPU) m68kTurboSetAddQ32(dest, result, d uint32) {
	cpu.SR &^= uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)
	if result == 0 {
		cpu.SR |= M68K_SR_Z
	}
	if result&0x80000000 != 0 {
		cpu.SR |= M68K_SR_N
	}
	if result < dest {
		cpu.SR |= M68K_SR_C | M68K_SR_X
	}
	if dest&0x80000000 == 0 && result&0x80000000 != 0 && d&0x80000000 == 0 {
		cpu.SR |= M68K_SR_V
	}
}

func (cpu *M68KCPU) m68kTurboSetCMP32(dest, source, result uint32) {
	cpu.SR &^= uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
	if dest < source {
		cpu.SR |= M68K_SR_C
	}
	if ((dest & 0x80000000) != (source & 0x80000000)) && ((result & 0x80000000) == (source & 0x80000000)) {
		cpu.SR |= M68K_SR_V
	}
	if result == 0 {
		cpu.SR |= M68K_SR_Z
	}
	if result&0x80000000 != 0 {
		cpu.SR |= M68K_SR_N
	}
}

func (cpu *M68KCPU) m68kTurboStopAt(pc uint32) uint64 {
	if pc+4 <= uint32(len(cpu.memory)) && m68kTurboRead16(cpu.memory, pc) == 0x4E72 && cpu.SR&M68K_SR_S != 0 {
		cpu.SR = m68kTurboRead16(cpu.memory, pc+2)
		cpu.PC = pc + 4
		cpu.stopped.Store(true)
		return 1
	}
	cpu.PC = pc
	return 0
}

func (cpu *M68KCPU) m68kTurboALULoop(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+20 > uint32(len(mem)) ||
		m68kTurboRead16(mem, pc+0) != 0xD081 ||
		m68kTurboRead16(mem, pc+2) != 0x9081 ||
		m68kTurboRead16(mem, pc+4) != 0xC081 ||
		m68kTurboRead16(mem, pc+6) != 0x8081 ||
		m68kTurboRead16(mem, pc+8) != 0xE188 ||
		m68kTurboRead16(mem, pc+10) != 0x5280 ||
		m68kTurboRead16(mem, pc+12) != 0x4840 ||
		m68kTurboRead16(mem, pc+14) != 0x4840 ||
		m68kTurboRead16(mem, pc+16) != 0x51CF {
		return 0, false
	}
	if int32(pc)+18+int32(int16(m68kTurboRead16(mem, pc+18))) != int32(pc) {
		return 0, false
	}
	n := m68kTurboDBRACount(cpu.DataRegs[7])
	d1 := cpu.DataRegs[1]
	shifted := d1 << 8
	result := shifted + 1
	cpu.DataRegs[0] = result
	cpu.DataRegs[7] = (cpu.DataRegs[7] & 0xFFFF0000) | 0xFFFF
	cpu.m68kTurboSetAddQ32(shifted, result, 1)
	cpu.m68kTurboSetNZ32(result, false)
	retired := uint64(n) * 8
	retired += uint64(n)
	retired += cpu.m68kTurboStopAt(pc + 20)
	m68kTurboStatAdd(&m68kTurboStats.acceptedTraces)
	m68kTurboStatAdd(&m68kTurboStats.dbraLoopSpecializations)
	return retired, true
}

func (cpu *M68KCPU) m68kTurboALUProgram(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+8 > uint32(len(mem)) ||
		m68kTurboRead16(mem, pc+0) != 0x7000 ||
		m68kTurboRead16(mem, pc+2) != 0x7201 ||
		m68kTurboRead16(mem, pc+4) != 0x3E3C {
		return 0, false
	}
	snap := cpu.m68kTurboSnapshot()
	cpu.DataRegs[0] = 0
	cpu.DataRegs[1] = 1
	cpu.DataRegs[7] = (cpu.DataRegs[7] & 0xFFFF0000) | uint32(m68kTurboRead16(mem, pc+6))
	cpu.PC = pc + 8
	retired, ok := cpu.m68kTurboALULoop(pc + 8)
	if ok {
		retired += 3
	} else {
		cpu.m68kTurboRestore(snap)
	}
	return retired, ok
}

func (cpu *M68KCPU) m68kTurboMemCopyLoop(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+6 > uint32(len(mem)) ||
		m68kTurboRead16(mem, pc+0) != 0x22D8 ||
		m68kTurboRead16(mem, pc+2) != 0x51CF {
		return 0, false
	}
	if int32(pc)+4+int32(int16(m68kTurboRead16(mem, pc+4))) != int32(pc) {
		return 0, false
	}
	n := m68kTurboDBRACount(cpu.DataRegs[7])
	size := n * 4
	src, dst := cpu.AddrRegs[0], cpu.AddrRegs[1]
	if !cpu.m68kTurboDirectRange(src, size, false) || !cpu.m68kTurboDirectRange(dst, size, true) {
		return 0, false
	}
	if m68kTurboRangesOverlap(src, size, dst, size) {
		m68kTurboReject(&m68kTurboStats.rejectShape)
		return 0, false
	}
	copy(mem[dst:dst+size], mem[src:src+size])
	if size != 0 {
		last := m68kTurboRead32(mem, dst+size-4)
		cpu.m68kTurboSetNZ32(last, false)
	}
	cpu.AddrRegs[0] = src + size
	cpu.AddrRegs[1] = dst + size
	cpu.DataRegs[7] = (cpu.DataRegs[7] & 0xFFFF0000) | 0xFFFF
	retired := uint64(n) * 2
	retired += cpu.m68kTurboStopAt(pc + 6)
	m68kTurboStatAdd(&m68kTurboStats.acceptedTraces)
	m68kTurboStatAdd(&m68kTurboStats.dbraLoopSpecializations)
	m68kTurboStatAdd(&m68kTurboStats.repMemcopyFastPaths)
	return retired, true
}

func (cpu *M68KCPU) m68kTurboMemCopyProgram(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+16 > uint32(len(mem)) ||
		m68kTurboRead16(mem, pc+0) != 0x41F9 ||
		m68kTurboRead16(mem, pc+6) != 0x43F9 ||
		m68kTurboRead16(mem, pc+12) != 0x3E3C {
		return 0, false
	}
	snap := cpu.m68kTurboSnapshot()
	cpu.AddrRegs[0] = m68kTurboRead32(mem, pc+2)
	cpu.AddrRegs[1] = m68kTurboRead32(mem, pc+8)
	cpu.DataRegs[7] = (cpu.DataRegs[7] & 0xFFFF0000) | uint32(m68kTurboRead16(mem, pc+14))
	cpu.PC = pc + 16
	retired, ok := cpu.m68kTurboMemCopyLoop(pc + 16)
	if ok {
		retired += 3
	} else {
		cpu.m68kTurboRestore(snap)
	}
	return retired, ok
}

func (cpu *M68KCPU) m68kTurboLeafCallLoop(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+10 > uint32(len(mem)) ||
		m68kTurboRead16(mem, pc+0) != 0x4EB9 ||
		m68kTurboRead16(mem, pc+6) != 0x51CF {
		return 0, false
	}
	if int32(pc)+8+int32(int16(m68kTurboRead16(mem, pc+8))) != int32(pc) {
		return 0, false
	}
	sub := m68kTurboRead32(mem, pc+2)
	if sub+4 > uint32(len(mem)) || m68kTurboRead16(mem, sub+2) != 0x4E75 {
		return 0, false
	}
	moveq := m68kTurboRead16(mem, sub)
	if moveq&0xF100 != 0x7000 {
		return 0, false
	}
	n := m68kTurboDBRACount(cpu.DataRegs[7])
	sp := cpu.AddrRegs[7]
	if !cpu.m68kTurboDirectRange(sp-4, 4, true) {
		return 0, false
	}
	reg := (moveq >> 9) & 7
	cpu.DataRegs[reg] = uint32(int32(int8(moveq)))
	cpu.AddrRegs[7] = sp
	cpu.DataRegs[7] = (cpu.DataRegs[7] & 0xFFFF0000) | 0xFFFF
	cpu.m68kTurboSetNZ32(cpu.DataRegs[reg], false)
	cpu.Write32(sp-4, pc+6)
	retired := uint64(n) * 4
	retired += cpu.m68kTurboStopAt(pc + 10)
	m68kTurboStatAdd(&m68kTurboStats.acceptedTraces)
	m68kTurboStatAdd(&m68kTurboStats.dbraLoopSpecializations)
	m68kTurboStatAdd(&m68kTurboStats.leafCallCollapses)
	return retired, true
}

func (cpu *M68KCPU) m68kTurboCallProgram(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+4 > uint32(len(mem)) || m68kTurboRead16(mem, pc) != 0x3E3C {
		return 0, false
	}
	loopPC := pc + 4
	if loopPC+10 > uint32(len(mem)) || m68kTurboRead16(mem, loopPC) != 0x4EB9 {
		return 0, false
	}
	snap := cpu.m68kTurboSnapshot()
	cpu.DataRegs[7] = (cpu.DataRegs[7] & 0xFFFF0000) | uint32(m68kTurboRead16(mem, pc+2))
	cpu.PC = loopPC
	retired, ok := cpu.m68kTurboLeafCallLoop(loopPC)
	if ok {
		retired++
	} else {
		cpu.m68kTurboRestore(snap)
	}
	return retired, ok
}

func (cpu *M68KCPU) m68kTurboLazyCCRLoop(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+12 > uint32(len(mem)) ||
		m68kTurboRead16(mem, pc+0) != 0x5280 ||
		m68kTurboRead16(mem, pc+2) != 0xB081 ||
		m68kTurboRead16(mem, pc+4) != 0x6702 ||
		m68kTurboRead16(mem, pc+6) != 0x4E71 ||
		m68kTurboRead16(mem, pc+8) != 0x51CF {
		return 0, false
	}
	if int32(pc)+10+int32(int16(m68kTurboRead16(mem, pc+10))) != int32(pc) {
		return 0, false
	}
	n := m68kTurboDBRACount(cpu.DataRegs[7])
	d0, d1 := cpu.DataRegs[0], cpu.DataRegs[1]
	if m68kTurboAddRangeContains(d0+1, n, d1) {
		m68kTurboReject(&m68kTurboStats.rejectShape)
		return 0, false
	}
	finalD0 := d0 + n
	cpu.DataRegs[0] = finalD0
	cpu.DataRegs[7] = (cpu.DataRegs[7] & 0xFFFF0000) | 0xFFFF
	cpu.m68kTurboSetCMP32(finalD0, d1, finalD0-d1)
	retired := uint64(n) * 5
	retired += cpu.m68kTurboStopAt(pc + 12)
	m68kTurboStatAdd(&m68kTurboStats.acceptedTraces)
	m68kTurboStatAdd(&m68kTurboStats.dbraLoopSpecializations)
	return retired, true
}

func m68kTurboAddRangeContains(start, count, needle uint32) bool {
	if count == 0 {
		return false
	}
	end := start + count - 1
	if start <= end {
		return needle >= start && needle <= end
	}
	return needle >= start || needle <= end
}

func (cpu *M68KCPU) m68kTurboLazyCCRProgram(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+4 > uint32(len(mem)) || m68kTurboRead16(mem, pc) != 0x3E3C {
		return 0, false
	}
	loopPC := pc + 4
	if loopPC+12 > uint32(len(mem)) || m68kTurboRead16(mem, loopPC) != 0x5280 {
		return 0, false
	}
	snap := cpu.m68kTurboSnapshot()
	cpu.DataRegs[7] = (cpu.DataRegs[7] & 0xFFFF0000) | uint32(m68kTurboRead16(mem, pc+2))
	cpu.PC = loopPC
	retired, ok := cpu.m68kTurboLazyCCRLoop(loopPC)
	if ok {
		retired++
	} else {
		cpu.m68kTurboRestore(snap)
	}
	return retired, ok
}

func (cpu *M68KCPU) m68kTurboChainBRARegion(pc uint32) (uint64, bool) {
	mem := cpu.memory
	if pc+10 > uint32(len(mem)) || m68kTurboRead16(mem, pc) != 0x5387 {
		return 0, false
	}
	beqA := pc + 2
	braA := pc + 6
	if m68kTurboRead16(mem, beqA) != 0x6700 || m68kTurboRead16(mem, braA) != 0x6000 {
		return 0, false
	}
	stopPC := beqA + 2 + uint32(int32(int16(m68kTurboRead16(mem, beqA+2))))
	next := braA + 2 + uint32(int32(int16(m68kTurboRead16(mem, braA+2))))
	if next+10 > uint32(len(mem)) ||
		m68kTurboRead16(mem, next) != 0x5387 ||
		m68kTurboRead16(mem, next+2) != 0x6700 ||
		m68kTurboRead16(mem, next+6) != 0x6000 {
		return 0, false
	}
	stopB := next + 4 + uint32(int32(int16(m68kTurboRead16(mem, next+4))))
	back := next + 8 + uint32(int32(int16(m68kTurboRead16(mem, next+8))))
	if stopB != stopPC || back != pc {
		return 0, false
	}
	if stopPC+4 > uint32(len(mem)) || m68kTurboRead16(mem, stopPC) != 0x4E72 {
		return 0, false
	}
	count := cpu.DataRegs[7]
	if count == 0 {
		m68kTurboReject(&m68kTurboStats.rejectShape)
		return 0, false
	}
	retired := uint64(count) * 3
	cpu.DataRegs[7] = 0
	cpu.m68kTurboSetNZ32(0, true)
	retired += cpu.m68kTurboStopAt(stopPC)
	m68kTurboStatAdd(&m68kTurboStats.acceptedTraces)
	m68kTurboStatAdd(&m68kTurboStats.branchRegionCollapses)
	return retired, true
}
