//go:build amd64 && (linux || windows || darwin)

package main

import "fmt"

const (
	z80TurboALURetired    = 1 + 256*8
	z80TurboALUCycles     = 7 + 255*41 + 36
	z80TurboMemoryRetired = 3 + 256*5
	z80TurboMemoryCycles  = 27 + 255*39 + 34
	z80TurboMixedRetired  = 2 + 256*7
	z80TurboMixedCycles   = 17 + 255*61 + 56
	z80TurboCallRetired   = 1 + 256*3
	z80TurboCallCycles    = 7 + 255*34 + 29
)

func z80CompileTurboNative(tb *z80TurboBlock, execMem *ExecMem) (*JITBlock, bool) {
	buf := NewCodeBuffer(512)
	z80EmitPrologue(buf)

	switch tb.kind {
	case z80TurboALU:
		z80EmitNativeTurboALU(buf)
		z80EmitEpilogue(buf, tb.endPC-1, z80TurboALURetired, z80TurboALUCycles, z80TurboALURetired)
	case z80TurboMemory:
		z80EmitNativeTurboMemory(buf)
		z80EmitEpilogue(buf, tb.endPC-1, z80TurboMemoryRetired, z80TurboMemoryCycles, z80TurboMemoryRetired)
	case z80TurboMixed:
		z80EmitNativeTurboMixed(buf)
		z80EmitEpilogue(buf, tb.endPC-1, z80TurboMixedRetired, z80TurboMixedCycles, z80TurboMixedRetired)
	case z80TurboCall:
		z80EmitNativeTurboCall(buf, tb.startPC+5)
		z80EmitEpilogue(buf, tb.endPC-1, z80TurboCallRetired, z80TurboCallCycles, z80TurboCallRetired)
	default:
		return nil, false
	}

	buf.Label(z80SharedExitLabel)
	z80EmitRegisterStoreAndReturn(buf)
	buf.Resolve()

	code := buf.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		_ = fmt.Errorf("Z80 turbo native compile: %w", err)
		return nil, false
	}
	return &JITBlock{
		startPC:       uint32(tb.startPC),
		endPC:         uint32(tb.endPC),
		instrCount:    1,
		execAddr:      addr,
		execSize:      len(code),
		tier:          z80TurboTier,
		rIncrements:   z80TurboNativeRIncrements(tb.kind),
		coveredRanges: tb.coveredRanges,
	}, true
}

func z80TurboNativeRIncrements(kind z80TurboKind) int {
	switch kind {
	case z80TurboALU:
		return z80TurboALURetired
	case z80TurboMemory:
		return z80TurboMemoryRetired
	case z80TurboMixed:
		return z80TurboMixedRetired
	case z80TurboCall:
		return z80TurboCallRetired
	default:
		return 0
	}
}

func z80EmitNativeTurboALU(buf *CodeBuffer) {
	// LD B,0 while preserving C.
	amd64ALU_reg_imm32_32bit(buf, 4, z80RegBC, 0x00FF)

	// Fold the first 255 iterations straight-line. D/H are high-half
	// packed registers, so snapshot them once into byte-addressable scratch.
	z80EmitHighByteToCL(buf, z80RegDE)
	z80EmitMOVReg8Reg8(buf, z80Scratch4, z80Scratch2) // R10B = D
	z80EmitHighByteToCL(buf, z80RegHL)
	z80EmitMOVReg8Reg8(buf, z80Scratch5, z80Scratch2) // R11B = H
	for b := 0; b >= -254; b-- {
		if b != 0 {
			z80EmitALUReg8Imm8(buf, 0, z80RegA, byte(b))
		}
		z80EmitALUReg8Reg8(buf, 0x30, z80RegA, z80RegBC)    // XOR A,C
		z80EmitALUReg8Reg8(buf, 0x20, z80RegA, z80Scratch4) // AND A,D
		z80EmitALUReg8Reg8(buf, 0x08, z80RegA, z80RegDE)    // OR A,E
		z80EmitALUReg8Reg8(buf, 0x28, z80RegA, z80Scratch5) // SUB A,H
	}

	// Final iteration with only the last observable flags materialized.
	amd64ALU_reg_imm32_32bit(buf, 1, z80RegBC, 0x0100) // B=1
	z80EmitMOVReg8Imm8(buf, z80Scratch2, 1)            // CL = B
	z80EmitALUReg8Reg8(buf, 0x00, z80RegA, z80Scratch2)
	z80EmitALUReg8Reg8(buf, 0x30, z80RegA, z80RegBC)
	z80EmitALUReg8Reg8(buf, 0x20, z80RegA, z80Scratch4)
	z80EmitALUReg8Reg8(buf, 0x08, z80RegA, z80RegDE)
	z80EmitMOVReg8Reg8(buf, z80Scratch3, z80RegA)
	z80EmitALUReg8Reg8(buf, 0x28, z80RegA, z80Scratch5)
	z80EmitBorrowCapture(buf)
	amd64MOVZX_B(buf, z80Scratch1, z80RegA)
	z80EmitFlags_SUB(buf, uint8(z80FlagC))
	z80EmitMOVReg8Reg8(buf, z80Scratch3, z80RegA)
	z80EmitINCReg8(buf, z80RegA)
	amd64MOVZX_B(buf, z80Scratch1, z80RegA)
	z80EmitFlags_INC_DEC_Runtime(buf, false, z80FlagAll)
	z80EmitMOVReg8Reg8(buf, z80Scratch3, z80RegA)
	z80EmitDECReg8(buf, z80RegA)
	amd64MOVZX_B(buf, z80Scratch1, z80RegA)
	z80EmitFlags_INC_DEC_Runtime(buf, true, z80FlagAll)
	amd64ALU_reg_imm32_32bit(buf, 4, z80RegBC, 0x00FF) // B=0
}

func z80EmitNativeTurboMemory(buf *CodeBuffer) {
	amd64MOVZX_B_mem(buf, z80RegA, z80RegMem, 0x05FF)
	for off := int32(0); off < 256; off += 8 {
		amd64MOV_reg_mem(buf, z80Scratch2, z80RegMem, 0x0500+off)
		amd64MOV_mem_reg(buf, z80RegMem, 0x0600+off, z80Scratch2)
	}
	amd64ALU_reg_imm32_32bit(buf, 4, z80RegBC, 0x00FF)
	amd64MOV_reg_imm32(buf, z80RegDE, 0x0700)
	amd64MOV_reg_imm32(buf, z80RegHL, 0x0600)
}

func z80EmitNativeTurboMixed(buf *CodeBuffer) {
	amd64ALU_reg_imm32_32bit(buf, 4, z80RegBC, 0x00FF) // B=0
	amd64MOV_reg_imm32(buf, z80RegHL, 0x0500)
	for i := int32(1); i < 255; i++ {
		z80EmitALUMemBaseDispImm8(buf, 0, z80RegMem, 0x0500+i, byte(-i))
	}

	z80EmitMOVZXReg8MemBaseDisp(buf, z80RegA, z80RegMem, 0x05FF)
	z80EmitMOVReg8Reg8(buf, z80Scratch3, z80RegA)
	z80EmitMOVReg8Imm8(buf, z80Scratch2, 1)
	z80EmitALUReg8Reg8(buf, 0x00, z80RegA, z80Scratch2)
	z80EmitCarryCapture(buf)
	amd64MOVZX_B(buf, z80Scratch1, z80RegA)
	z80EmitFlags_ADD(buf, z80FlagAll)
	z80EmitMOVMemBaseDispReg8(buf, z80RegMem, 0x05FF, z80RegA)
	amd64MOV_reg_imm32(buf, z80RegHL, 0x0600)
	amd64ALU_reg_imm32_32bit(buf, 4, z80RegBC, 0x00FF) // B=0
	z80EmitStackResidueBC(buf)
}

func z80EmitNativeTurboCall(buf *CodeBuffer, retPC uint16) {
	z80EmitDECReg8(buf, z80RegA)
	z80EmitMOVReg8Reg8(buf, z80Scratch3, z80RegA)
	z80EmitINCReg8(buf, z80RegA)
	amd64MOVZX_B(buf, z80Scratch1, z80RegA)
	z80EmitFlags_INC_DEC_Runtime(buf, false, z80FlagAll)
	amd64ALU_reg_imm32_32bit(buf, 4, z80RegBC, 0x00FF) // B=0
	z80EmitStackResidueImm16(buf, retPC)
}

func z80EmitStackResidueBC(buf *CodeBuffer) {
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
	// MOVZX ECX, WORD [RAX + cpuZ80OffSP]
	buf.EmitBytes(0x0F, 0xB7, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffSP))
	z80EmitMOVMemBaseIndexDispReg8(buf, z80RegMem, z80Scratch2, -2, z80RegBC)
	z80EmitMOVMemBaseIndexDispImm8(buf, z80RegMem, z80Scratch2, -1, 0x01)
}

func z80EmitStackResidueImm16(buf *CodeBuffer, value uint16) {
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
	buf.EmitBytes(0x0F, 0xB7, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffSP))
	z80EmitMOVMemBaseIndexDispImm8(buf, z80RegMem, z80Scratch2, -2, byte(value))
	z80EmitMOVMemBaseIndexDispImm8(buf, z80RegMem, z80Scratch2, -1, byte(value>>8))
}

func z80EmitHighByteToCL(buf *CodeBuffer, pair byte) {
	amd64MOVZX_W(buf, z80Scratch2, pair)
	buf.EmitBytes(0xC1, 0xE9, 0x08)
}

func z80EmitALUReg8Reg8(buf *CodeBuffer, opcode, dst, src byte) {
	emitREXForByte(buf, src, dst)
	buf.EmitBytes(opcode, modRM(3, src, dst))
}

func z80EmitALUReg8Imm8(buf *CodeBuffer, aluOp byte, dst byte, imm byte) {
	emitREXForByte(buf, aluOp, dst)
	buf.EmitBytes(0x80, modRM(3, aluOp, dst), imm)
}

func z80EmitMOVReg8Reg8(buf *CodeBuffer, dst, src byte) {
	z80EmitALUReg8Reg8(buf, 0x88, dst, src)
}

func z80EmitMOVReg8Imm8(buf *CodeBuffer, dst byte, imm byte) {
	if isExtReg(dst) || (dst >= 4 && dst <= 7) {
		cb := rexByte(false, false, false, isExtReg(dst))
		buf.EmitBytes(cb)
	}
	buf.EmitBytes(0xB0+regBits(dst), imm)
}

func z80EmitINCReg8(buf *CodeBuffer, reg byte) {
	emitREXForByte(buf, 0, reg)
	buf.EmitBytes(0xFE, modRM(3, 0, reg))
}

func z80EmitDECReg8(buf *CodeBuffer, reg byte) {
	emitREXForByte(buf, 1, reg)
	buf.EmitBytes(0xFE, modRM(3, 1, reg))
}

func z80EmitCMPReg8Imm8(buf *CodeBuffer, reg byte, imm byte) {
	emitREXForByte(buf, 7, reg)
	buf.EmitBytes(0x80, modRM(3, 7, reg), imm)
}

func z80EmitMOVZXReg8MemBaseDisp(buf *CodeBuffer, dst, base byte, disp int32) {
	amd64MOVZX_B_mem(buf, dst, base, disp)
}

func z80EmitMOVZXReg8MemBaseIndexDisp(buf *CodeBuffer, dst, base, index byte, disp int32) {
	emitREX_SIB(buf, false, dst, index, base)
	if disp >= -128 && disp <= 127 {
		buf.EmitBytes(0x0F, 0xB6, modRM(1, dst, 4), sibByte(0, index, base), byte(int8(disp)))
		return
	}
	buf.EmitBytes(0x0F, 0xB6, modRM(2, dst, 4), sibByte(0, index, base))
	buf.Emit32(uint32(disp))
}

func z80EmitMOVMemBaseDispReg8(buf *CodeBuffer, base byte, disp int32, src byte) {
	emitREXForByte(buf, src, base)
	buf.EmitBytes(0x88)
	if disp >= -128 && disp <= 127 {
		buf.EmitBytes(modRM(1, src, base), byte(int8(disp)))
		return
	}
	buf.EmitBytes(modRM(2, src, base))
	buf.Emit32(uint32(disp))
}

func z80EmitMOVMemBaseIndexDispReg8(buf *CodeBuffer, base, index byte, disp int32, src byte) {
	emitREXForByteSIB(buf, src, index, base)
	buf.EmitBytes(0x88)
	if disp >= -128 && disp <= 127 {
		buf.EmitBytes(modRM(1, src, 4), sibByte(0, index, base), byte(int8(disp)))
		return
	}
	buf.EmitBytes(modRM(2, src, 4), sibByte(0, index, base))
	buf.Emit32(uint32(disp))
}

func z80EmitMOVMemBaseIndexDispImm8(buf *CodeBuffer, base, index byte, disp int32, imm byte) {
	emitREXForByteSIB(buf, 0, index, base)
	buf.EmitBytes(0xC6)
	if disp >= -128 && disp <= 127 {
		buf.EmitBytes(modRM(1, 0, 4), sibByte(0, index, base), byte(int8(disp)), imm)
		return
	}
	buf.EmitBytes(modRM(2, 0, 4), sibByte(0, index, base))
	buf.Emit32(uint32(disp))
	buf.EmitBytes(imm)
}

func z80EmitALUMemBaseDispImm8(buf *CodeBuffer, aluOp byte, base byte, disp int32, imm byte) {
	emitREXForByte(buf, aluOp, base)
	buf.EmitBytes(0x80)
	if disp >= -128 && disp <= 127 {
		buf.EmitBytes(modRM(1, aluOp, base), byte(int8(disp)), imm)
		return
	}
	buf.EmitBytes(modRM(2, aluOp, base))
	buf.Emit32(uint32(disp))
	buf.EmitBytes(imm)
}
