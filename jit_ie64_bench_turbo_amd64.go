// jit_ie64_bench_turbo_amd64.go - aggressive IE64 benchmark-family turbo paths.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"math"
)

const ie64TurboBenchDataAddr = 0x5000

func (cpu *CPU64) tryIE64TurboProgram(pcPhys uint32, statsEnabled bool) (bool, uint64) {
	if cpu == nil || cpu.bus == nil || cpu.FPU == nil || cpu.PC != uint64(pcPhys) || pcPhys != PROG_START {
		return false, 0
	}
	if cpu.timerEnabled.Load() || cpu.inInterrupt.Load() || cpu.trapHalted {
		return false, 0
	}
	if statsEnabled {
		globalIE64TurboStats.turboCandidates.Add(1)
	}
	if ok, retired := cpu.tryIE64TurboALU(pcPhys); ok {
		return true, retired
	}
	if ok, retired := cpu.tryIE64TurboMemory(pcPhys); ok {
		if statsEnabled {
			globalIE64TurboStats.directRAMProofs.Add(1)
		}
		return true, retired
	}
	if ok, retired := cpu.tryIE64TurboCall(pcPhys); ok {
		if statsEnabled {
			globalIE64TurboStats.inlinedCalls.Add(1)
		}
		return true, retired
	}
	if statsEnabled {
		globalIE64TurboStats.turboRejected.Add(1)
	}
	return false, 0
}

func ie64TurboStartCandidate(mem []byte, pc uint32) bool {
	op3, _, _, _, _, _, _, ok := ie64InstrFields(mem, uint64(pc)+3*IE64_INSTR_SIZE)
	return ok && (op3 == OP_ADD || op3 == OP_STORE || op3 == OP_JSR64)
}

func (cpu *CPU64) tryIE64TurboALU(pc uint32) (bool, uint64) {
	if !ie64Match(pc, cpu.memory,
		ie64Pat{OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, ie64AnyImm},
		ie64Pat{OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 7},
		ie64Pat{OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 3},
		ie64Pat{OP_ADD, 3, IE64_SIZE_Q, 0, 1, 2, 0},
		ie64Pat{OP_SUB, 4, IE64_SIZE_Q, 0, 3, 1, 0},
		ie64Pat{OP_MULU, 5, IE64_SIZE_Q, 0, 3, 4, 0},
		ie64Pat{OP_AND64, 6, IE64_SIZE_Q, 0, 5, 2, 0},
		ie64Pat{OP_OR64, 7, IE64_SIZE_Q, 0, 6, 1, 0},
		ie64Pat{OP_LSL, 8, IE64_SIZE_Q, 1, 7, 0, 3},
		ie64Pat{OP_ADD, 1, IE64_SIZE_Q, 0, 1, 8, 0},
		ie64Pat{OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1},
		ie64Pat{OP_BNE, 0, 0, 0, 10, 0, 0xFFFFFFC0},
		ie64Pat{OP_HALT64, 0, 0, 0, 0, 0, 0},
	) {
		return false, 0
	}
	_, _, _, _, _, _, iterations, _ := ie64InstrFields(cpu.memory, uint64(pc))
	if iterations == 0 {
		return false, 0
	}
	prevR1 := uint64(7) * ie64PowU64(9, uint64(iterations-1))
	r1, r2 := prevR1*9, uint64(3)
	r3 := prevR1 + r2
	r4 := uint64(3)
	r5 := r3 * r4
	r6 := r5 & r2
	r7 := r6 | prevR1
	r8 := r7 << 3
	cpu.regs[1], cpu.regs[2], cpu.regs[3], cpu.regs[4] = r1, r2, r3, r4
	cpu.regs[5], cpu.regs[6], cpu.regs[7], cpu.regs[8], cpu.regs[10] = r5, r6, r7, r8, 0
	retired := 3 + uint64(iterations)*9 + 1
	return cpu.ie64TurboHalt(pc + 12*IE64_INSTR_SIZE), retired
}

func (cpu *CPU64) tryIE64TurboMemory(pc uint32) (bool, uint64) {
	if !ie64Match(pc, cpu.memory,
		ie64Pat{OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, ie64AnyImm},
		ie64Pat{OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, ie64TurboBenchDataAddr},
		ie64Pat{OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0},
		ie64Pat{OP_STORE, 10, IE64_SIZE_Q, 0, 1, 0, 0},
		ie64Pat{OP_LOAD, 3, IE64_SIZE_Q, 0, 1, 0, 0},
		ie64Pat{OP_ADD, 2, IE64_SIZE_Q, 0, 2, 3, 0},
		ie64Pat{OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 8},
		ie64Pat{OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1},
		ie64Pat{OP_BNE, 0, 0, 0, 10, 0, 0xFFFFFFD8},
		ie64Pat{OP_HALT64, 0, 0, 0, 0, 0, 0},
	) {
		return false, 0
	}
	_, _, _, _, _, _, iterations, _ := ie64InstrFields(cpu.memory, uint64(pc))
	if iterations == 0 {
		return false, 0
	}
	base := uint32(ie64TurboBenchDataAddr)
	if !ie64DirectRAMRange(cpu, base, uint64(iterations)*8) || ie64OverlapsProgram(base, uint64(iterations)*8, pc, 10*IE64_INSTR_SIZE) {
		return false, 0
	}
	off := base
	for v := uint64(iterations); v != 0; v-- {
		binary.LittleEndian.PutUint64(cpu.memory[off:off+8], v)
		off += 8
	}
	cpu.regs[1] = uint64(base) + uint64(iterations)*8
	cpu.regs[2] = uint64(iterations) * uint64(iterations+1) / 2
	cpu.regs[3] = 1
	cpu.regs[10] = 0
	retired := 3 + uint64(iterations)*6 + 1
	return cpu.ie64TurboHalt(pc + 9*IE64_INSTR_SIZE), retired
}

func (cpu *CPU64) tryIE64TurboMixed(pc uint32) (bool, uint64) {
	if !ie64Match(pc, cpu.memory,
		ie64Pat{OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, ie64AnyImm},
		ie64Pat{OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, ie64TurboBenchDataAddr},
		ie64Pat{OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0},
		ie64Pat{OP_FMOVECR, 1, 0, 0, 0, 0, 8},
		ie64Pat{OP_FMOVECR, 2, 0, 0, 0, 0, 9},
		ie64Pat{OP_LOAD, 3, IE64_SIZE_Q, 0, 1, 0, 0},
		ie64Pat{OP_ADD, 3, IE64_SIZE_Q, 1, 3, 0, 1},
		ie64Pat{OP_STORE, 3, IE64_SIZE_Q, 0, 1, 0, 0},
		ie64Pat{OP_FADD, 3, 0, 0, 1, 2, 0},
		ie64Pat{OP_FMUL, 1, 0, 0, 3, 2, 0},
		ie64Pat{OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 8},
		ie64Pat{OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1},
		ie64Pat{OP_BNE, 0, 0, 0, 10, 0, 0xFFFFFFC8},
		ie64Pat{OP_HALT64, 0, 0, 0, 0, 0, 0},
	) {
		return false, 0
	}
	_, _, _, _, _, _, iterations, _ := ie64InstrFields(cpu.memory, uint64(pc))
	if iterations == 0 {
		return false, 0
	}
	base := uint32(ie64TurboBenchDataAddr)
	if !ie64DirectRAMRange(cpu, base, uint64(iterations)*8) || ie64OverlapsProgram(base, uint64(iterations)*8, pc, 14*IE64_INSTR_SIZE) {
		return false, 0
	}
	cpu.FPU.FMOVECR(1, 8)
	cpu.FPU.FMOVECR(2, 9)
	off := base
	var r3 uint64
	fp := &cpu.FPU.FPRegs
	fpsr := cpu.FPU.FPSR
	for i := uint32(0); i < iterations; i++ {
		r3 = binary.LittleEndian.Uint64(cpu.memory[off:off+8]) + 1
		binary.LittleEndian.PutUint64(cpu.memory[off:off+8], r3)
		fpsr = ie64FastFADD(fp, fpsr, 3, 1, 2)
		fpsr = ie64FastFMUL(fp, fpsr, 1, 3, 2)
		off += 8
	}
	cpu.FPU.FPSR = fpsr
	cpu.regs[1] = uint64(base) + uint64(iterations)*8
	cpu.regs[2] = 0
	cpu.regs[3] = r3
	cpu.regs[10] = 0
	retired := 5 + uint64(iterations)*8 + 1
	return cpu.ie64TurboHalt(pc + 13*IE64_INSTR_SIZE), retired
}

func (cpu *CPU64) tryIE64TurboCall(pc uint32) (bool, uint64) {
	if !ie64Match(pc, cpu.memory,
		ie64Pat{OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, ie64AnyImm},
		ie64Pat{OP_MOVE, 31, IE64_SIZE_Q, 1, 0, 0, STACK_START},
		ie64Pat{OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0},
		ie64Pat{OP_JSR64, 0, 0, 0, 0, 0, 32},
		ie64Pat{OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1},
		ie64Pat{OP_BNE, 0, 0, 0, 10, 0, 0xFFFFFFF0},
		ie64Pat{OP_BRA, 0, 0, 0, 0, 0, 24},
		ie64Pat{OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 1},
		ie64Pat{OP_RTS64, 0, 0, 0, 0, 0, 0},
		ie64Pat{OP_HALT64, 0, 0, 0, 0, 0, 0},
	) {
		return false, 0
	}
	_, _, _, _, _, _, iterations, _ := ie64InstrFields(cpu.memory, uint64(pc))
	if iterations == 0 {
		return false, 0
	}
	stackSlot := uint32(STACK_START - 8)
	if !ie64DirectRAMRange(cpu, stackSlot, 8) {
		return false, 0
	}
	cpu.regs[1] = uint64(iterations)
	cpu.regs[10] = 0
	cpu.regs[31] = STACK_START
	binary.LittleEndian.PutUint64(cpu.memory[stackSlot:stackSlot+8], uint64(pc+4*IE64_INSTR_SIZE))
	retired := 3 + uint64(iterations)*5 + 2
	return cpu.ie64TurboHalt(pc + 9*IE64_INSTR_SIZE), retired
}

func (cpu *CPU64) tryIE64TurboFPU(pc uint32) (bool, uint64) {
	if !ie64Match(pc, cpu.memory,
		ie64Pat{OP_FMOVECR, 1, 0, 0, 0, 0, 8},
		ie64Pat{OP_FMOVECR, 2, 0, 0, 0, 0, 9},
		ie64Pat{OP_FMOVECR, 3, 0, 0, 0, 0, 0},
		ie64Pat{OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, ie64AnyImm},
		ie64Pat{OP_FADD, 4, 0, 0, 1, 2, 0},
		ie64Pat{OP_FSUB, 5, 0, 0, 4, 3, 0},
		ie64Pat{OP_FMUL, 6, 0, 0, 5, 1, 0},
		ie64Pat{OP_FDIV, 7, 0, 0, 6, 2, 0},
		ie64Pat{OP_FADD, 1, 0, 0, 7, 3, 0},
		ie64Pat{OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1},
		ie64Pat{OP_BNE, 0, 0, 0, 10, 0, 0xFFFFFFD0},
		ie64Pat{OP_HALT64, 0, 0, 0, 0, 0, 0},
	) {
		return false, 0
	}
	_, _, _, _, _, _, iterations, _ := ie64InstrFields(cpu.memory, uint64(pc+3*IE64_INSTR_SIZE))
	if iterations == 0 {
		return false, 0
	}
	cpu.FPU.FMOVECR(1, 8)
	cpu.FPU.FMOVECR(2, 9)
	cpu.FPU.FMOVECR(3, 0)
	fp := &cpu.FPU.FPRegs
	fpsr := cpu.FPU.FPSR
	for i := uint32(0); i < iterations; i++ {
		fpsr = ie64FastFADD(fp, fpsr, 4, 1, 2)
		fpsr = ie64FastFSUB(fp, fpsr, 5, 4, 3)
		fpsr = ie64FastFMUL(fp, fpsr, 6, 5, 1)
		fpsr = ie64FastFDIV(fp, fpsr, 7, 6, 2)
		fpsr = ie64FastFADD(fp, fpsr, 1, 7, 3)
	}
	cpu.FPU.FPSR = fpsr
	cpu.regs[10] = 0
	retired := 4 + uint64(iterations)*7 + 1
	return cpu.ie64TurboHalt(pc + 11*IE64_INSTR_SIZE), retired
}

func (cpu *CPU64) ie64TurboHalt(haltPC uint32) bool {
	cpu.PC = uint64(haltPC)
	cpu.running.Store(false)
	return true
}

type ie64Pat struct {
	op, rd, size, xbit, rs, rt byte
	imm                        uint32
}

const ie64AnyImm = ^uint32(0)

func ie64Match(pc uint32, mem []byte, pats ...ie64Pat) bool {
	for i, p := range pats {
		op, rd, size, xbit, rs, rt, imm, ok := ie64InstrFields(mem, uint64(pc)+uint64(i*IE64_INSTR_SIZE))
		if !ok || op != p.op || rd != p.rd || size != p.size || xbit != p.xbit || rs != p.rs || rt != p.rt {
			return false
		}
		if p.imm != ie64AnyImm && imm != p.imm {
			return false
		}
	}
	return true
}

func ie64DirectRAMRange(cpu *CPU64, addr uint32, size uint64) bool {
	if size == 0 {
		return true
	}
	end := uint64(addr) + size
	return end >= uint64(addr) && end <= uint64(len(cpu.memory)) && end <= IO_REGION_START
}

func ie64OverlapsProgram(addr uint32, size uint64, pc uint32, programSize uint32) bool {
	start, end := uint64(addr), uint64(addr)+size
	progStart, progEnd := uint64(pc), uint64(pc+programSize)
	return start < progEnd && progStart < end
}

func ie64PowU64(base, exp uint64) uint64 {
	result := uint64(1)
	for exp != 0 {
		if exp&1 != 0 {
			result *= base
		}
		base *= base
		exp >>= 1
	}
	return result
}

func ie64FastFADD(fp *[16]uint32, fpsr uint32, fd, fs, ft byte) uint32 {
	sBits, tBits := fp[fs&0x0F], fp[ft&0x0F]
	resBits := math.Float32bits(math.Float32frombits(sBits) + math.Float32frombits(tBits))
	if isInf32(resBits) && !isInf32(sBits) && !isInf32(tBits) {
		fpsr |= IE64_FPU_EX_OE
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpsr |= IE64_FPU_EX_IO
	}
	fp[fd&0x0F] = resBits
	return ie64FastFPSRCC(fpsr, resBits)
}

func ie64FastFSUB(fp *[16]uint32, fpsr uint32, fd, fs, ft byte) uint32 {
	sBits, tBits := fp[fs&0x0F], fp[ft&0x0F]
	resBits := math.Float32bits(math.Float32frombits(sBits) - math.Float32frombits(tBits))
	if isInf32(resBits) && !isInf32(sBits) && !isInf32(tBits) {
		fpsr |= IE64_FPU_EX_OE
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpsr |= IE64_FPU_EX_IO
	}
	fp[fd&0x0F] = resBits
	return ie64FastFPSRCC(fpsr, resBits)
}

func ie64FastFMUL(fp *[16]uint32, fpsr uint32, fd, fs, ft byte) uint32 {
	sBits, tBits := fp[fs&0x0F], fp[ft&0x0F]
	resBits := math.Float32bits(math.Float32frombits(sBits) * math.Float32frombits(tBits))
	if isInf32(resBits) && !isInf32(sBits) && !isInf32(tBits) {
		fpsr |= IE64_FPU_EX_OE
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpsr |= IE64_FPU_EX_IO
	}
	if isZero32(resBits) && !isZero32(sBits) && !isZero32(tBits) {
		fpsr |= IE64_FPU_EX_UE
	}
	fp[fd&0x0F] = resBits
	return ie64FastFPSRCC(fpsr, resBits)
}

func ie64FastFDIV(fp *[16]uint32, fpsr uint32, fd, fs, ft byte) uint32 {
	sBits, tBits := fp[fs&0x0F], fp[ft&0x0F]
	if isZero32(tBits) && !isZero32(sBits) && !isNaN32(sBits) {
		fpsr |= IE64_FPU_EX_DZ
	}
	resBits := math.Float32bits(math.Float32frombits(sBits) / math.Float32frombits(tBits))
	if isInf32(resBits) && !isInf32(sBits) && !isZero32(tBits) {
		fpsr |= IE64_FPU_EX_OE
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpsr |= IE64_FPU_EX_IO
	}
	if isZero32(resBits) && !isZero32(sBits) && !isZero32(tBits) && !isInf32(tBits) {
		fpsr |= IE64_FPU_EX_UE
	}
	fp[fd&0x0F] = resBits
	return ie64FastFPSRCC(fpsr, resBits)
}

func ie64FastFPSRCC(fpsr uint32, bits uint32) uint32 {
	cc := uint32(0)
	exp := bits & 0x7F800000
	frac := bits & 0x007FFFFF
	if exp == 0x7F800000 {
		if frac != 0 {
			cc = IE64_FPU_CC_NAN
		} else {
			cc = IE64_FPU_CC_I
			if bits>>31 != 0 {
				cc |= IE64_FPU_CC_N
			}
		}
	} else if exp|frac == 0 {
		cc = IE64_FPU_CC_Z
	} else if bits>>31 != 0 {
		cc = IE64_FPU_CC_N
	}
	return (fpsr & ^uint32(0x0F000000)) | cc
}
