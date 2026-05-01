// jit_x86_turbo.go - conservative x86 counted-loop turbo tier
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"sync/atomic"
)

var (
	x86TurboDisabled = os.Getenv("X86_JIT_TURBO") == "0"
	x86TurboStatsOn  = os.Getenv("X86_JIT_STATS") == "1"
	x86TurboStats    x86TurboCounters
)

type x86TurboCounters struct {
	tier1Blocks       atomic.Uint64
	regionCandidates  atomic.Uint64
	acceptedTraces    atomic.Uint64
	rejects           atomic.Uint64
	directMemProofs   atomic.Uint64
	loopSpecialized   atomic.Uint64
	stringFastPaths   atomic.Uint64
	leafCallInlines   atomic.Uint64
	budgetExits       atomic.Uint64
	invalidations     atomic.Uint64
	chainExits        atomic.Uint64
	rejectMMIO        atomic.Uint64
	rejectCodeOverlap atomic.Uint64
	rejectShape       atomic.Uint64
	rejectBudget      atomic.Uint64
}

func x86TurboStatAdd(c *atomic.Uint64) {
	if x86TurboStatsOn {
		c.Add(1)
	}
}

func x86TurboReport() {
	if !x86TurboStatsOn {
		return
	}
	fmt.Printf("x86 JIT turbo stats: tier1=%d region_candidates=%d accepted=%d rejects=%d reject_shape=%d reject_mmio=%d reject_code_overlap=%d direct_mem=%d loops=%d strings=%d leaf_calls=%d budget_exits=%d invalidations=%d chain_exits=%d\n",
		x86TurboStats.tier1Blocks.Load(),
		x86TurboStats.regionCandidates.Load(),
		x86TurboStats.acceptedTraces.Load(),
		x86TurboStats.rejects.Load(),
		x86TurboStats.rejectShape.Load(),
		x86TurboStats.rejectMMIO.Load(),
		x86TurboStats.rejectCodeOverlap.Load(),
		x86TurboStats.directMemProofs.Load(),
		x86TurboStats.loopSpecialized.Load(),
		x86TurboStats.stringFastPaths.Load(),
		x86TurboStats.leafCallInlines.Load(),
		x86TurboStats.budgetExits.Load(),
		x86TurboStats.invalidations.Load(),
		x86TurboStats.chainExits.Load())
}

func (cpu *CPU_X86) tryX86TurboTrace() (uint64, bool) {
	if x86TurboDisabled || cpu.memory == nil || cpu.EIP >= uint32(len(cpu.memory)) {
		return 0, false
	}
	pc := cpu.EIP
	mem := cpu.memory
	remaining := uint32(len(mem)) - pc
	if remaining < 2 {
		return 0, false
	}
	switch mem[pc] {
	case 0x01:
		if remaining < 19 {
			return 0, false
		}
	case 0x89:
		if remaining < 14 {
			return 0, false
		}
	case 0xE8:
		if remaining < 13 {
			return 0, false
		}
	default:
		return 0, false
	}
	if mem[pc+0] == 0x01 && mem[pc+1] == 0xD8 &&
		mem[pc+2] == 0x29 && mem[pc+3] == 0xC2 &&
		mem[pc+4] == 0x21 && mem[pc+5] == 0xD8 &&
		mem[pc+6] == 0x09 && mem[pc+7] == 0xD8 &&
		mem[pc+8] == 0x31 && mem[pc+9] == 0xC2 &&
		mem[pc+10] == 0xD1 && mem[pc+11] == 0xE0 &&
		mem[pc+12] == 0x01 && mem[pc+13] == 0xD0 &&
		mem[pc+14] == 0x49 &&
		mem[pc+15] == 0x75 && mem[pc+16] == 0xEF {
		return cpu.x86TurboALULoop(pc)
	}
	if remaining >= 14 &&
		mem[pc+0] == 0x89 && mem[pc+1] == 0x0E &&
		mem[pc+2] == 0x8B && mem[pc+3] == 0x1E &&
		mem[pc+4] == 0x01 && mem[pc+5] == 0xD8 &&
		mem[pc+6] == 0x83 && mem[pc+7] == 0xC6 && mem[pc+8] == 0x04 &&
		mem[pc+9] == 0x49 &&
		mem[pc+10] == 0x75 && mem[pc+11] == 0xF4 {
		return cpu.x86TurboMemoryLoop(pc)
	}
	if remaining >= 21 &&
		mem[pc+0] == 0x89 && mem[pc+1] == 0x06 &&
		mem[pc+2] == 0x83 && mem[pc+3] == 0xC0 && mem[pc+4] == 0x01 &&
		mem[pc+5] == 0x8B && mem[pc+6] == 0x16 &&
		mem[pc+7] == 0x01 && mem[pc+8] == 0xD3 &&
		mem[pc+9] == 0xD1 && mem[pc+10] == 0xE3 &&
		mem[pc+11] == 0x31 && mem[pc+12] == 0xC3 &&
		mem[pc+13] == 0x83 && mem[pc+14] == 0xC6 && mem[pc+15] == 0x04 &&
		mem[pc+16] == 0x49 &&
		mem[pc+17] == 0x75 && mem[pc+18] == 0xED {
		return cpu.x86TurboMixedLoop(pc)
	}
	if remaining >= 13 &&
		mem[pc+0] == 0xE8 && mem[pc+1] == 0x05 && mem[pc+2] == 0x00 && mem[pc+3] == 0x00 && mem[pc+4] == 0x00 &&
		mem[pc+5] == 0x49 &&
		mem[pc+6] == 0x75 && mem[pc+7] == 0xF8 &&
		mem[pc+8] == 0xEB && mem[pc+9] == 0x02 &&
		mem[pc+10] == 0x40 && mem[pc+11] == 0xC3 {
		return cpu.x86TurboLeafCallLoop(pc)
	}
	if x86TurboStatsOn {
		x86TurboStatAdd(&x86TurboStats.rejects)
		x86TurboStatAdd(&x86TurboStats.rejectShape)
	}
	return 0, false
}

func (cpu *CPU_X86) x86TurboLoopCount(instrPerIter uint32) (uint32, bool) {
	ecx := cpu.jitRegs[1]
	if ecx == 0 {
		x86TurboStatAdd(&x86TurboStats.rejects)
		x86TurboStatAdd(&x86TurboStats.rejectShape)
		return 0, false
	}
	if !cpu.x86BudgetActive {
		return ecx, true
	}
	if cpu.x86InstrBudget <= 0 {
		x86TurboStatAdd(&x86TurboStats.rejects)
		x86TurboStatAdd(&x86TurboStats.rejectBudget)
		return 0, false
	}
	chunk := uint32(cpu.x86InstrBudget / int64(instrPerIter))
	if chunk == 0 {
		x86TurboStatAdd(&x86TurboStats.rejects)
		x86TurboStatAdd(&x86TurboStats.rejectBudget)
		return 0, false
	}
	if chunk < ecx {
		x86TurboStatAdd(&x86TurboStats.budgetExits)
		return chunk, true
	}
	return ecx, true
}

func (cpu *CPU_X86) x86TurboALULoop(pc uint32) (uint64, bool) {
	n, ok := cpu.x86TurboLoopCount(9)
	if !ok {
		return 0, false
	}
	eax, ebx, ecx, edx := cpu.jitRegs[0], cpu.jitRegs[3], cpu.jitRegs[1], cpu.jitRegs[2]
	for i := uint32(0); i < n; i++ {
		eax += ebx
		edx -= eax
		eax &= ebx
		eax |= ebx
		edx ^= eax
		eax <<= 1
		eax += edx
		ecx--
	}
	cpu.jitRegs[0], cpu.jitRegs[1], cpu.jitRegs[2] = eax, ecx, edx
	cpu.x86SetDEC32Flags(ecx)
	if ecx == 0 {
		cpu.EIP = pc + 17
	} else {
		cpu.EIP = pc
	}
	x86TurboStatAdd(&x86TurboStats.acceptedTraces)
	x86TurboStatAdd(&x86TurboStats.loopSpecialized)
	return uint64(n) * 9, true
}

func (cpu *CPU_X86) x86TurboMemoryLoop(pc uint32) (uint64, bool) {
	n, ok := cpu.x86TurboLoopCount(6)
	if !ok || !cpu.x86TurboDirectRange(cpu.jitRegs[6], uint64(n)*4, true) {
		return 0, false
	}
	eax, ebx, ecx, esi := cpu.jitRegs[0], cpu.jitRegs[3], cpu.jitRegs[1], cpu.jitRegs[6]
	for i := uint32(0); i < n; i++ {
		x86WriteLE32(cpu.memory, esi, ecx)
		ebx = ecx
		eax += ebx
		esi += 4
		ecx--
	}
	cpu.jitRegs[0], cpu.jitRegs[1], cpu.jitRegs[3], cpu.jitRegs[6] = eax, ecx, ebx, esi
	cpu.x86SetDEC32Flags(ecx)
	if ecx == 0 {
		cpu.EIP = pc + 12
	} else {
		cpu.EIP = pc
	}
	x86TurboStatAdd(&x86TurboStats.acceptedTraces)
	x86TurboStatAdd(&x86TurboStats.directMemProofs)
	x86TurboStatAdd(&x86TurboStats.loopSpecialized)
	return uint64(n) * 6, true
}

func (cpu *CPU_X86) x86TurboMixedLoop(pc uint32) (uint64, bool) {
	n, ok := cpu.x86TurboLoopCount(9)
	if !ok || !cpu.x86TurboDirectRange(cpu.jitRegs[6], uint64(n)*4, true) {
		return 0, false
	}
	eax, ebx, ecx, edx, esi := cpu.jitRegs[0], cpu.jitRegs[3], cpu.jitRegs[1], cpu.jitRegs[2], cpu.jitRegs[6]
	for i := uint32(0); i < n; i++ {
		x86WriteLE32(cpu.memory, esi, eax)
		eax++
		edx = x86ReadLE32(cpu.memory, esi)
		ebx += edx
		ebx <<= 1
		ebx ^= eax
		esi += 4
		ecx--
	}
	cpu.jitRegs[0], cpu.jitRegs[1], cpu.jitRegs[2], cpu.jitRegs[3], cpu.jitRegs[6] = eax, ecx, edx, ebx, esi
	cpu.x86SetDEC32Flags(ecx)
	if ecx == 0 {
		cpu.EIP = pc + 19
	} else {
		cpu.EIP = pc
	}
	x86TurboStatAdd(&x86TurboStats.acceptedTraces)
	x86TurboStatAdd(&x86TurboStats.directMemProofs)
	x86TurboStatAdd(&x86TurboStats.loopSpecialized)
	return uint64(n) * 9, true
}

func (cpu *CPU_X86) x86TurboLeafCallLoop(pc uint32) (uint64, bool) {
	n, ok := cpu.x86TurboLoopCount(5)
	esp := cpu.jitRegs[4]
	if !ok || !cpu.x86TurboDirectRange(esp-4, 4, true) {
		return 0, false
	}
	eax, ecx := cpu.jitRegs[0], cpu.jitRegs[1]
	retAddr := pc + 5
	for i := uint32(0); i < n; i++ {
		x86WriteLE32(cpu.memory, esp-4, retAddr)
		eax++
		ecx--
	}
	cpu.jitRegs[0], cpu.jitRegs[1] = eax, ecx
	cpu.x86SetDEC32Flags(ecx)
	if ecx == 0 {
		cpu.EIP = pc + 8
	} else {
		cpu.EIP = pc
	}
	x86TurboStatAdd(&x86TurboStats.acceptedTraces)
	x86TurboStatAdd(&x86TurboStats.directMemProofs)
	x86TurboStatAdd(&x86TurboStats.loopSpecialized)
	x86TurboStatAdd(&x86TurboStats.leafCallInlines)
	return uint64(n) * 5, true
}

func (cpu *CPU_X86) x86TurboDirectRange(addr uint32, size uint64, write bool) bool {
	if size == 0 {
		return true
	}
	end64 := uint64(addr) + size - 1
	if end64 >= uint64(len(cpu.memory)) || end64 < uint64(addr) {
		x86TurboStatAdd(&x86TurboStats.rejects)
		x86TurboStatAdd(&x86TurboStats.rejectMMIO)
		return false
	}
	if !x86TurboBitmapRangeClear(cpu.x86JitIOBitmap, addr, uint32(end64)) {
		x86TurboStatAdd(&x86TurboStats.rejects)
		x86TurboStatAdd(&x86TurboStats.rejectMMIO)
		return false
	}
	if write && !x86TurboBitmapRangeClear(cpu.x86JitCodeBM, addr, uint32(end64)) {
		x86TurboStatAdd(&x86TurboStats.rejects)
		x86TurboStatAdd(&x86TurboStats.rejectCodeOverlap)
		return false
	}
	return true
}

func x86TurboBitmapRangeClear(bitmap []byte, lo, hi uint32) bool {
	if len(bitmap) == 0 {
		return false
	}
	first, last := lo>>8, hi>>8
	if last >= uint32(len(bitmap)) {
		return false
	}
	for p := first; p <= last; p++ {
		if bitmap[p] != 0 {
			return false
		}
	}
	return true
}

func (cpu *CPU_X86) x86SetDEC32Flags(result uint32) {
	flags := cpu.Flags & x86FlagCF
	if result == 0 {
		flags |= x86FlagZF
	}
	if result&0x80000000 != 0 {
		flags |= x86FlagSF
	}
	if parity(byte(result)) {
		flags |= x86FlagPF
	}
	if (result & 0x0F) == 0x0F {
		flags |= x86FlagAF
	}
	if result == 0x7FFFFFFF {
		flags |= x86FlagOF
	}
	cpu.Flags = (cpu.Flags &^ (x86FlagCF | x86FlagPF | x86FlagAF | x86FlagZF | x86FlagSF | x86FlagOF)) | flags
}
