// Code generated from executeOpcodeSwitch; DO NOT EDIT.
package main

import "fmt"

func (cpu_6502 *CPU_6502) initOpcodeTableGenerated() {
	for i := 0; i < 256; i++ {
		cpu_6502.opcodeTable[i] = op6502Unknown
	}
	cpu_6502.opcodeTable[0xA9] = op6502_A9
	cpu_6502.opcodeTable[0xA5] = op6502_A5
	cpu_6502.opcodeTable[0xB5] = op6502_B5
	cpu_6502.opcodeTable[0xAD] = op6502_AD
	cpu_6502.opcodeTable[0xBD] = op6502_BD
	cpu_6502.opcodeTable[0xB9] = op6502_B9
	cpu_6502.opcodeTable[0xA1] = op6502_A1
	cpu_6502.opcodeTable[0xB1] = op6502_B1
	cpu_6502.opcodeTable[0xA2] = op6502_A2
	cpu_6502.opcodeTable[0xA6] = op6502_A6
	cpu_6502.opcodeTable[0xB6] = op6502_B6
	cpu_6502.opcodeTable[0xAE] = op6502_AE
	cpu_6502.opcodeTable[0xBE] = op6502_BE
	cpu_6502.opcodeTable[0xA0] = op6502_A0
	cpu_6502.opcodeTable[0xA4] = op6502_A4
	cpu_6502.opcodeTable[0xB4] = op6502_B4
	cpu_6502.opcodeTable[0xAC] = op6502_AC
	cpu_6502.opcodeTable[0xBC] = op6502_BC
	cpu_6502.opcodeTable[0x85] = op6502_85
	cpu_6502.opcodeTable[0x95] = op6502_95
	cpu_6502.opcodeTable[0x8D] = op6502_8D
	cpu_6502.opcodeTable[0x9D] = op6502_9D
	cpu_6502.opcodeTable[0x99] = op6502_99
	cpu_6502.opcodeTable[0x81] = op6502_81
	cpu_6502.opcodeTable[0x91] = op6502_91
	cpu_6502.opcodeTable[0x86] = op6502_86
	cpu_6502.opcodeTable[0x96] = op6502_96
	cpu_6502.opcodeTable[0x8E] = op6502_8E
	cpu_6502.opcodeTable[0x84] = op6502_84
	cpu_6502.opcodeTable[0x94] = op6502_94
	cpu_6502.opcodeTable[0x8C] = op6502_8C
	cpu_6502.opcodeTable[0xAA] = op6502_AA
	cpu_6502.opcodeTable[0x8A] = op6502_8A
	cpu_6502.opcodeTable[0xA8] = op6502_A8
	cpu_6502.opcodeTable[0x98] = op6502_98
	cpu_6502.opcodeTable[0xBA] = op6502_BA
	cpu_6502.opcodeTable[0x9A] = op6502_9A
	cpu_6502.opcodeTable[0x48] = op6502_48
	cpu_6502.opcodeTable[0x68] = op6502_68
	cpu_6502.opcodeTable[0x08] = op6502_08
	cpu_6502.opcodeTable[0x28] = op6502_28
	cpu_6502.opcodeTable[0x69] = op6502_69
	cpu_6502.opcodeTable[0x65] = op6502_65
	cpu_6502.opcodeTable[0x75] = op6502_75
	cpu_6502.opcodeTable[0x6D] = op6502_6D
	cpu_6502.opcodeTable[0x7D] = op6502_7D
	cpu_6502.opcodeTable[0x79] = op6502_79
	cpu_6502.opcodeTable[0x61] = op6502_61
	cpu_6502.opcodeTable[0x71] = op6502_71
	cpu_6502.opcodeTable[0xE9] = op6502_E9
	cpu_6502.opcodeTable[0xE5] = op6502_E5
	cpu_6502.opcodeTable[0xF5] = op6502_F5
	cpu_6502.opcodeTable[0xED] = op6502_ED
	cpu_6502.opcodeTable[0xFD] = op6502_FD
	cpu_6502.opcodeTable[0xF9] = op6502_F9
	cpu_6502.opcodeTable[0xE1] = op6502_E1
	cpu_6502.opcodeTable[0xF1] = op6502_F1
	cpu_6502.opcodeTable[0xE6] = op6502_E6
	cpu_6502.opcodeTable[0xF6] = op6502_F6
	cpu_6502.opcodeTable[0xEE] = op6502_EE
	cpu_6502.opcodeTable[0xFE] = op6502_FE
	cpu_6502.opcodeTable[0xC6] = op6502_C6
	cpu_6502.opcodeTable[0xD6] = op6502_D6
	cpu_6502.opcodeTable[0xCE] = op6502_CE
	cpu_6502.opcodeTable[0xDE] = op6502_DE
	cpu_6502.opcodeTable[0xE8] = op6502_E8
	cpu_6502.opcodeTable[0xC8] = op6502_C8
	cpu_6502.opcodeTable[0xCA] = op6502_CA
	cpu_6502.opcodeTable[0x88] = op6502_88
	cpu_6502.opcodeTable[0x29] = op6502_29
	cpu_6502.opcodeTable[0x25] = op6502_25
	cpu_6502.opcodeTable[0x35] = op6502_35
	cpu_6502.opcodeTable[0x2D] = op6502_2D
	cpu_6502.opcodeTable[0x3D] = op6502_3D
	cpu_6502.opcodeTable[0x39] = op6502_39
	cpu_6502.opcodeTable[0x21] = op6502_21
	cpu_6502.opcodeTable[0x31] = op6502_31
	cpu_6502.opcodeTable[0x09] = op6502_09
	cpu_6502.opcodeTable[0x05] = op6502_05
	cpu_6502.opcodeTable[0x15] = op6502_15
	cpu_6502.opcodeTable[0x0D] = op6502_0D
	cpu_6502.opcodeTable[0x1D] = op6502_1D
	cpu_6502.opcodeTable[0x19] = op6502_19
	cpu_6502.opcodeTable[0x01] = op6502_01
	cpu_6502.opcodeTable[0x11] = op6502_11
	cpu_6502.opcodeTable[0x49] = op6502_49
	cpu_6502.opcodeTable[0x45] = op6502_45
	cpu_6502.opcodeTable[0x55] = op6502_55
	cpu_6502.opcodeTable[0x4D] = op6502_4D
	cpu_6502.opcodeTable[0x5D] = op6502_5D
	cpu_6502.opcodeTable[0x59] = op6502_59
	cpu_6502.opcodeTable[0x41] = op6502_41
	cpu_6502.opcodeTable[0x51] = op6502_51
	cpu_6502.opcodeTable[0x24] = op6502_24
	cpu_6502.opcodeTable[0x2C] = op6502_2C
	cpu_6502.opcodeTable[0x0A] = op6502_0A
	cpu_6502.opcodeTable[0x06] = op6502_06
	cpu_6502.opcodeTable[0x16] = op6502_16
	cpu_6502.opcodeTable[0x0E] = op6502_0E
	cpu_6502.opcodeTable[0x1E] = op6502_1E
	cpu_6502.opcodeTable[0x4A] = op6502_4A
	cpu_6502.opcodeTable[0x46] = op6502_46
	cpu_6502.opcodeTable[0x56] = op6502_56
	cpu_6502.opcodeTable[0x4E] = op6502_4E
	cpu_6502.opcodeTable[0x5E] = op6502_5E
	cpu_6502.opcodeTable[0x2A] = op6502_2A
	cpu_6502.opcodeTable[0x26] = op6502_26
	cpu_6502.opcodeTable[0x36] = op6502_36
	cpu_6502.opcodeTable[0x2E] = op6502_2E
	cpu_6502.opcodeTable[0x3E] = op6502_3E
	cpu_6502.opcodeTable[0x6A] = op6502_6A
	cpu_6502.opcodeTable[0x66] = op6502_66
	cpu_6502.opcodeTable[0x76] = op6502_76
	cpu_6502.opcodeTable[0x6E] = op6502_6E
	cpu_6502.opcodeTable[0x7E] = op6502_7E
	cpu_6502.opcodeTable[0xC9] = op6502_C9
	cpu_6502.opcodeTable[0xC5] = op6502_C5
	cpu_6502.opcodeTable[0xD5] = op6502_D5
	cpu_6502.opcodeTable[0xCD] = op6502_CD
	cpu_6502.opcodeTable[0xDD] = op6502_DD
	cpu_6502.opcodeTable[0xD9] = op6502_D9
	cpu_6502.opcodeTable[0xC1] = op6502_C1
	cpu_6502.opcodeTable[0xD1] = op6502_D1
	cpu_6502.opcodeTable[0xE0] = op6502_E0
	cpu_6502.opcodeTable[0xE4] = op6502_E4
	cpu_6502.opcodeTable[0xEC] = op6502_EC
	cpu_6502.opcodeTable[0xC0] = op6502_C0
	cpu_6502.opcodeTable[0xC4] = op6502_C4
	cpu_6502.opcodeTable[0xCC] = op6502_CC
	cpu_6502.opcodeTable[0x90] = op6502_90
	cpu_6502.opcodeTable[0xB0] = op6502_B0
	cpu_6502.opcodeTable[0xF0] = op6502_F0
	cpu_6502.opcodeTable[0xD0] = op6502_D0
	cpu_6502.opcodeTable[0x30] = op6502_30
	cpu_6502.opcodeTable[0x10] = op6502_10
	cpu_6502.opcodeTable[0x70] = op6502_70
	cpu_6502.opcodeTable[0x50] = op6502_50
	cpu_6502.opcodeTable[0x4C] = op6502_4C
	cpu_6502.opcodeTable[0x6C] = op6502_6C
	cpu_6502.opcodeTable[0x20] = op6502_20
	cpu_6502.opcodeTable[0x60] = op6502_60
	cpu_6502.opcodeTable[0x18] = op6502_18
	cpu_6502.opcodeTable[0x38] = op6502_38
	cpu_6502.opcodeTable[0x58] = op6502_58
	cpu_6502.opcodeTable[0x78] = op6502_78
	cpu_6502.opcodeTable[0xB8] = op6502_B8
	cpu_6502.opcodeTable[0xD8] = op6502_D8
	cpu_6502.opcodeTable[0xF8] = op6502_F8
	cpu_6502.opcodeTable[0x00] = op6502_00
	cpu_6502.opcodeTable[0x40] = op6502_40
	cpu_6502.opcodeTable[0xEA] = op6502_EA
	cpu_6502.opcodeTable[0x1A] = op6502_1A
	cpu_6502.opcodeTable[0x3A] = op6502_1A
	cpu_6502.opcodeTable[0x5A] = op6502_1A
	cpu_6502.opcodeTable[0x7A] = op6502_1A
	cpu_6502.opcodeTable[0xDA] = op6502_1A
	cpu_6502.opcodeTable[0xFA] = op6502_1A
	cpu_6502.opcodeTable[0x80] = op6502_80
	cpu_6502.opcodeTable[0x82] = op6502_80
	cpu_6502.opcodeTable[0x89] = op6502_80
	cpu_6502.opcodeTable[0xC2] = op6502_80
	cpu_6502.opcodeTable[0xE2] = op6502_80
	cpu_6502.opcodeTable[0x04] = op6502_04
	cpu_6502.opcodeTable[0x44] = op6502_04
	cpu_6502.opcodeTable[0x64] = op6502_04
	cpu_6502.opcodeTable[0x14] = op6502_14
	cpu_6502.opcodeTable[0x34] = op6502_14
	cpu_6502.opcodeTable[0x54] = op6502_14
	cpu_6502.opcodeTable[0x74] = op6502_14
	cpu_6502.opcodeTable[0xD4] = op6502_14
	cpu_6502.opcodeTable[0xF4] = op6502_14
	cpu_6502.opcodeTable[0x0C] = op6502_0C
	cpu_6502.opcodeTable[0x1C] = op6502_1C
	cpu_6502.opcodeTable[0x3C] = op6502_1C
	cpu_6502.opcodeTable[0x5C] = op6502_1C
	cpu_6502.opcodeTable[0x7C] = op6502_1C
	cpu_6502.opcodeTable[0xDC] = op6502_1C
	cpu_6502.opcodeTable[0xFC] = op6502_1C
	cpu_6502.opcodeTable[0xA7] = op6502_A7
	cpu_6502.opcodeTable[0xB7] = op6502_B7
	cpu_6502.opcodeTable[0xAF] = op6502_AF
	cpu_6502.opcodeTable[0xBF] = op6502_BF
	cpu_6502.opcodeTable[0xA3] = op6502_A3
	cpu_6502.opcodeTable[0xB3] = op6502_B3
	cpu_6502.opcodeTable[0x87] = op6502_87
	cpu_6502.opcodeTable[0x97] = op6502_97
	cpu_6502.opcodeTable[0x8F] = op6502_8F
	cpu_6502.opcodeTable[0x83] = op6502_83
	cpu_6502.opcodeTable[0xEB] = op6502_EB
	cpu_6502.opcodeTable[0xC7] = op6502_C7
	cpu_6502.opcodeTable[0xD7] = op6502_D7
	cpu_6502.opcodeTable[0xCF] = op6502_CF
	cpu_6502.opcodeTable[0xDF] = op6502_DF
	cpu_6502.opcodeTable[0xDB] = op6502_DB
	cpu_6502.opcodeTable[0xC3] = op6502_C3
	cpu_6502.opcodeTable[0xD3] = op6502_D3
	cpu_6502.opcodeTable[0xE7] = op6502_E7
	cpu_6502.opcodeTable[0xF7] = op6502_F7
	cpu_6502.opcodeTable[0xEF] = op6502_EF
	cpu_6502.opcodeTable[0xFF] = op6502_FF
	cpu_6502.opcodeTable[0xFB] = op6502_FB
	cpu_6502.opcodeTable[0xE3] = op6502_E3
	cpu_6502.opcodeTable[0xF3] = op6502_F3
	cpu_6502.opcodeTable[0x07] = op6502_07
	cpu_6502.opcodeTable[0x17] = op6502_17
	cpu_6502.opcodeTable[0x0F] = op6502_0F
	cpu_6502.opcodeTable[0x1F] = op6502_1F
	cpu_6502.opcodeTable[0x1B] = op6502_1B
	cpu_6502.opcodeTable[0x03] = op6502_03
	cpu_6502.opcodeTable[0x13] = op6502_13
	cpu_6502.opcodeTable[0x27] = op6502_27
	cpu_6502.opcodeTable[0x37] = op6502_37
	cpu_6502.opcodeTable[0x2F] = op6502_2F
	cpu_6502.opcodeTable[0x3F] = op6502_3F
	cpu_6502.opcodeTable[0x3B] = op6502_3B
	cpu_6502.opcodeTable[0x23] = op6502_23
	cpu_6502.opcodeTable[0x33] = op6502_33
	cpu_6502.opcodeTable[0x47] = op6502_47
	cpu_6502.opcodeTable[0x57] = op6502_57
	cpu_6502.opcodeTable[0x4F] = op6502_4F
	cpu_6502.opcodeTable[0x5F] = op6502_5F
	cpu_6502.opcodeTable[0x5B] = op6502_5B
	cpu_6502.opcodeTable[0x43] = op6502_43
	cpu_6502.opcodeTable[0x53] = op6502_53
	cpu_6502.opcodeTable[0x67] = op6502_67
	cpu_6502.opcodeTable[0x77] = op6502_77
	cpu_6502.opcodeTable[0x6F] = op6502_6F
	cpu_6502.opcodeTable[0x7F] = op6502_7F
	cpu_6502.opcodeTable[0x7B] = op6502_7B
	cpu_6502.opcodeTable[0x63] = op6502_63
	cpu_6502.opcodeTable[0x73] = op6502_73
	cpu_6502.opcodeTable[0x0B] = op6502_0B
	cpu_6502.opcodeTable[0x2B] = op6502_0B
	cpu_6502.opcodeTable[0x4B] = op6502_4B
	cpu_6502.opcodeTable[0x6B] = op6502_6B
	cpu_6502.opcodeTable[0xCB] = op6502_CB
	cpu_6502.opcodeTable[0x9F] = op6502_9F
	cpu_6502.opcodeTable[0x93] = op6502_93
	cpu_6502.opcodeTable[0x9E] = op6502_9E
	cpu_6502.opcodeTable[0x9C] = op6502_9C
	cpu_6502.opcodeTable[0x9B] = op6502_9B
	cpu_6502.opcodeTable[0xBB] = op6502_BB
	cpu_6502.opcodeTable[0xAB] = op6502_AB
	cpu_6502.opcodeTable[0x02] = op6502_02
	cpu_6502.opcodeTable[0x12] = op6502_02
	cpu_6502.opcodeTable[0x22] = op6502_02
	cpu_6502.opcodeTable[0x32] = op6502_02
	cpu_6502.opcodeTable[0x42] = op6502_02
	cpu_6502.opcodeTable[0x52] = op6502_02
	cpu_6502.opcodeTable[0x62] = op6502_02
	cpu_6502.opcodeTable[0x72] = op6502_02
	cpu_6502.opcodeTable[0x92] = op6502_02
	cpu_6502.opcodeTable[0xB2] = op6502_02
	cpu_6502.opcodeTable[0xD2] = op6502_02
	cpu_6502.opcodeTable[0xF2] = op6502_02
}

func op6502Unknown(cpu_6502 *CPU_6502) {
	fmt.Printf("Unknown opcode at PC=%04X\n", cpu_6502.PC-1)
	cpu_6502.running.Store(false)
}

func op6502_A9(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 2

}

func op6502_A5(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.getZeroPage())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 3

}

func op6502_B5(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.getZeroPageX())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_AD(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.getAbsolute())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_BD(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	cpu_6502.A = cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_B9(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.A = cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_A1(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.getIndirectX())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_B1(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getIndirectY()
	cpu_6502.A = cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 5
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_A2(cpu_6502 *CPU_6502) {
	cpu_6502.X = cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 2

}

func op6502_A6(cpu_6502 *CPU_6502) {
	cpu_6502.X = cpu_6502.readByte(cpu_6502.getZeroPage())
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 3

}

func op6502_B6(cpu_6502 *CPU_6502) {
	cpu_6502.X = cpu_6502.readByte(cpu_6502.getZeroPageY())
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 4

}

func op6502_AE(cpu_6502 *CPU_6502) {
	cpu_6502.X = cpu_6502.readByte(cpu_6502.getAbsolute())
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 4

}

func op6502_BE(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.X = cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_A0(cpu_6502 *CPU_6502) {
	cpu_6502.Y = cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.updateNZ(cpu_6502.Y)
	cpu_6502.Cycles += 2

}

func op6502_A4(cpu_6502 *CPU_6502) {
	cpu_6502.Y = cpu_6502.readByte(cpu_6502.getZeroPage())
	cpu_6502.updateNZ(cpu_6502.Y)
	cpu_6502.Cycles += 3

}

func op6502_B4(cpu_6502 *CPU_6502) {
	cpu_6502.Y = cpu_6502.readByte(cpu_6502.getZeroPageX())
	cpu_6502.updateNZ(cpu_6502.Y)
	cpu_6502.Cycles += 4

}

func op6502_AC(cpu_6502 *CPU_6502) {
	cpu_6502.Y = cpu_6502.readByte(cpu_6502.getAbsolute())
	cpu_6502.updateNZ(cpu_6502.Y)
	cpu_6502.Cycles += 4

}

func op6502_BC(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	cpu_6502.Y = cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.Y)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

	// Store operations
}

func op6502_85(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getZeroPage(), cpu_6502.A)
	cpu_6502.Cycles += 3

}

func op6502_95(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getZeroPageX(), cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_8D(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getAbsolute(), cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_9D(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	cpu_6502.writeByte(addr, cpu_6502.A)
	cpu_6502.Cycles += 5

}

func op6502_99(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	cpu_6502.writeByte(addr, cpu_6502.A)
	cpu_6502.Cycles += 5

}

func op6502_81(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getIndirectX(), cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_91(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getIndirectY()
	cpu_6502.writeByte(addr, cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_86(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getZeroPage(), cpu_6502.X)
	cpu_6502.Cycles += 3

}

func op6502_96(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getZeroPageY(), cpu_6502.X)
	cpu_6502.Cycles += 4

}

func op6502_8E(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getAbsolute(), cpu_6502.X)
	cpu_6502.Cycles += 4

}

func op6502_84(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getZeroPage(), cpu_6502.Y)
	cpu_6502.Cycles += 3

}

func op6502_94(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getZeroPageX(), cpu_6502.Y)
	cpu_6502.Cycles += 4

}

func op6502_8C(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getAbsolute(), cpu_6502.Y)
	cpu_6502.Cycles += 4

	// Register Transfer Operations
}

func op6502_AA(cpu_6502 *CPU_6502) {
	cpu_6502.X = cpu_6502.A
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 2

}

func op6502_8A(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.X
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 2

}

func op6502_A8(cpu_6502 *CPU_6502) {
	cpu_6502.Y = cpu_6502.A
	cpu_6502.updateNZ(cpu_6502.Y)
	cpu_6502.Cycles += 2

}

func op6502_98(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.Y
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 2

}

func op6502_BA(cpu_6502 *CPU_6502) {
	cpu_6502.X = cpu_6502.SP
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 2

}

func op6502_9A(cpu_6502 *CPU_6502) {
	cpu_6502.SP = cpu_6502.X
	cpu_6502.Cycles += 2

	// Stack Operations
}

func op6502_48(cpu_6502 *CPU_6502) {
	cpu_6502.push(cpu_6502.A)
	cpu_6502.Cycles += 3

}

func op6502_68(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.pop()
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_08(cpu_6502 *CPU_6502) {
	cpu_6502.push(cpu_6502.SR | BREAK_FLAG | UNUSED_FLAG)
	cpu_6502.Cycles += 3

}

func op6502_28(cpu_6502 *CPU_6502) {
	cpu_6502.SR = (cpu_6502.pop() & 0xEF) | UNUSED_FLAG
	cpu_6502.Cycles += 4

	// Arithmetic Operations
}

func op6502_69(cpu_6502 *CPU_6502) {
	cpu_6502.adc(cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	cpu_6502.Cycles += 2

}

func op6502_65(cpu_6502 *CPU_6502) {
	cpu_6502.adc(cpu_6502.readByte(cpu_6502.getZeroPage()))
	cpu_6502.Cycles += 3

}

func op6502_75(cpu_6502 *CPU_6502) {
	cpu_6502.adc(cpu_6502.readByte(cpu_6502.getZeroPageX()))
	cpu_6502.Cycles += 4

}

func op6502_6D(cpu_6502 *CPU_6502) {
	cpu_6502.adc(cpu_6502.readByte(cpu_6502.getAbsolute()))
	cpu_6502.Cycles += 4

}

func op6502_7D(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	cpu_6502.adc(cpu_6502.readByte(addr))
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_79(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.adc(cpu_6502.readByte(addr))
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_61(cpu_6502 *CPU_6502) {
	cpu_6502.adc(cpu_6502.readByte(cpu_6502.getIndirectX()))
	cpu_6502.Cycles += 6

}

func op6502_71(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getIndirectY()
	cpu_6502.adc(cpu_6502.readByte(addr))
	cpu_6502.Cycles += 5
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_E9(cpu_6502 *CPU_6502) {
	cpu_6502.sbc(cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	cpu_6502.Cycles += 2

}

func op6502_E5(cpu_6502 *CPU_6502) {
	cpu_6502.sbc(cpu_6502.readByte(cpu_6502.getZeroPage()))
	cpu_6502.Cycles += 3

}

func op6502_F5(cpu_6502 *CPU_6502) {
	cpu_6502.sbc(cpu_6502.readByte(cpu_6502.getZeroPageX()))
	cpu_6502.Cycles += 4

}

func op6502_ED(cpu_6502 *CPU_6502) {
	cpu_6502.sbc(cpu_6502.readByte(cpu_6502.getAbsolute()))
	cpu_6502.Cycles += 4

}

func op6502_FD(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	cpu_6502.sbc(cpu_6502.readByte(addr))
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_F9(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.sbc(cpu_6502.readByte(addr))
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_E1(cpu_6502 *CPU_6502) {
	cpu_6502.sbc(cpu_6502.readByte(cpu_6502.getIndirectX()))
	cpu_6502.Cycles += 6

}

func op6502_F1(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getIndirectY()
	cpu_6502.sbc(cpu_6502.readByte(addr))
	cpu_6502.Cycles += 5
	if crossed {
		cpu_6502.Cycles++
	}

	// Increment/Decrement Operations
}

func op6502_E6(cpu_6502 *CPU_6502) {
	cpu_6502.inc(cpu_6502.getZeroPage())
	cpu_6502.Cycles += 5

}

func op6502_F6(cpu_6502 *CPU_6502) {
	cpu_6502.inc(cpu_6502.getZeroPageX())
	cpu_6502.Cycles += 6

}

func op6502_EE(cpu_6502 *CPU_6502) {
	cpu_6502.inc(cpu_6502.getAbsolute())
	cpu_6502.Cycles += 6

}

func op6502_FE(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	cpu_6502.inc(addr)
	cpu_6502.Cycles += 7

}

func op6502_C6(cpu_6502 *CPU_6502) {
	cpu_6502.dec(cpu_6502.getZeroPage())
	cpu_6502.Cycles += 5

}

func op6502_D6(cpu_6502 *CPU_6502) {
	cpu_6502.dec(cpu_6502.getZeroPageX())
	cpu_6502.Cycles += 6

}

func op6502_CE(cpu_6502 *CPU_6502) {
	cpu_6502.dec(cpu_6502.getAbsolute())
	cpu_6502.Cycles += 6

}

func op6502_DE(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	cpu_6502.dec(addr)
	cpu_6502.Cycles += 7

}

func op6502_E8(cpu_6502 *CPU_6502) {
	cpu_6502.X++
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 2

}

func op6502_C8(cpu_6502 *CPU_6502) {
	cpu_6502.Y++
	cpu_6502.updateNZ(cpu_6502.Y)
	cpu_6502.Cycles += 2

}

func op6502_CA(cpu_6502 *CPU_6502) {
	cpu_6502.X--
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 2

}

func op6502_88(cpu_6502 *CPU_6502) {
	cpu_6502.Y--
	cpu_6502.updateNZ(cpu_6502.Y)
	cpu_6502.Cycles += 2

	// Logical Operations
}

func op6502_29(cpu_6502 *CPU_6502) {
	cpu_6502.A &= cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 2

}

func op6502_25(cpu_6502 *CPU_6502) {
	cpu_6502.A &= cpu_6502.readByte(cpu_6502.getZeroPage())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 3

}

func op6502_35(cpu_6502 *CPU_6502) {
	cpu_6502.A &= cpu_6502.readByte(cpu_6502.getZeroPageX())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_2D(cpu_6502 *CPU_6502) {
	cpu_6502.A &= cpu_6502.readByte(cpu_6502.getAbsolute())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_3D(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	cpu_6502.A &= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_39(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.A &= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_21(cpu_6502 *CPU_6502) {
	cpu_6502.A &= cpu_6502.readByte(cpu_6502.getIndirectX())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_31(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getIndirectY()
	cpu_6502.A &= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 5
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_09(cpu_6502 *CPU_6502) {
	cpu_6502.A |= cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 2

}

func op6502_05(cpu_6502 *CPU_6502) {
	cpu_6502.A |= cpu_6502.readByte(cpu_6502.getZeroPage())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 3

}

func op6502_15(cpu_6502 *CPU_6502) {
	cpu_6502.A |= cpu_6502.readByte(cpu_6502.getZeroPageX())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_0D(cpu_6502 *CPU_6502) {
	cpu_6502.A |= cpu_6502.readByte(cpu_6502.getAbsolute())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_1D(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	cpu_6502.A |= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_19(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.A |= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_01(cpu_6502 *CPU_6502) {
	cpu_6502.A |= cpu_6502.readByte(cpu_6502.getIndirectX())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_11(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getIndirectY()
	cpu_6502.A |= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 5
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_49(cpu_6502 *CPU_6502) {
	cpu_6502.A ^= cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 2

}

func op6502_45(cpu_6502 *CPU_6502) {
	cpu_6502.A ^= cpu_6502.readByte(cpu_6502.getZeroPage())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 3

}

func op6502_55(cpu_6502 *CPU_6502) {
	cpu_6502.A ^= cpu_6502.readByte(cpu_6502.getZeroPageX())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_4D(cpu_6502 *CPU_6502) {
	cpu_6502.A ^= cpu_6502.readByte(cpu_6502.getAbsolute())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_5D(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	cpu_6502.A ^= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_59(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.A ^= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_41(cpu_6502 *CPU_6502) {
	cpu_6502.A ^= cpu_6502.readByte(cpu_6502.getIndirectX())
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_51(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getIndirectY()
	cpu_6502.A ^= cpu_6502.readByte(addr)
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 5
	if crossed {
		cpu_6502.Cycles++
	}

	// Bit Operations
}

func op6502_24(cpu_6502 *CPU_6502) {
	value := cpu_6502.readByte(cpu_6502.getZeroPage())
	cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A&value == 0)
	cpu_6502.setFlag(OVERFLOW_FLAG, value&0x40 != 0)
	cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
	cpu_6502.Cycles += 3

}

func op6502_2C(cpu_6502 *CPU_6502) {
	value := cpu_6502.readByte(cpu_6502.getAbsolute())
	cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A&value == 0)
	cpu_6502.setFlag(OVERFLOW_FLAG, value&0x40 != 0)
	cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
	cpu_6502.Cycles += 4

	// Shift/Rotate Operations
}

func op6502_0A(cpu_6502 *CPU_6502) {
	cpu_6502.asl(0, true)
	cpu_6502.Cycles += 2

}

func op6502_06(cpu_6502 *CPU_6502) {
	cpu_6502.asl(cpu_6502.getZeroPage(), false)
	cpu_6502.Cycles += 5

}

func op6502_16(cpu_6502 *CPU_6502) {
	cpu_6502.asl(cpu_6502.getZeroPageX(), false)
	cpu_6502.Cycles += 6

}

func op6502_0E(cpu_6502 *CPU_6502) {
	cpu_6502.asl(cpu_6502.getAbsolute(), false)
	cpu_6502.Cycles += 6

}

func op6502_1E(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	cpu_6502.asl(addr, false)
	cpu_6502.Cycles += 7

}

func op6502_4A(cpu_6502 *CPU_6502) {
	cpu_6502.lsr(0, true)
	cpu_6502.Cycles += 2

}

func op6502_46(cpu_6502 *CPU_6502) {
	cpu_6502.lsr(cpu_6502.getZeroPage(), false)
	cpu_6502.Cycles += 5

}

func op6502_56(cpu_6502 *CPU_6502) {
	cpu_6502.lsr(cpu_6502.getZeroPageX(), false)
	cpu_6502.Cycles += 6

}

func op6502_4E(cpu_6502 *CPU_6502) {
	cpu_6502.lsr(cpu_6502.getAbsolute(), false)
	cpu_6502.Cycles += 6

}

func op6502_5E(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	cpu_6502.lsr(addr, false)
	cpu_6502.Cycles += 7

}

func op6502_2A(cpu_6502 *CPU_6502) {
	cpu_6502.rol(0, true)
	cpu_6502.Cycles += 2

}

func op6502_26(cpu_6502 *CPU_6502) {
	cpu_6502.rol(cpu_6502.getZeroPage(), false)
	cpu_6502.Cycles += 5

}

func op6502_36(cpu_6502 *CPU_6502) {
	cpu_6502.rol(cpu_6502.getZeroPageX(), false)
	cpu_6502.Cycles += 6

}

func op6502_2E(cpu_6502 *CPU_6502) {
	cpu_6502.rol(cpu_6502.getAbsolute(), false)
	cpu_6502.Cycles += 6

}

func op6502_3E(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	cpu_6502.rol(addr, false)
	cpu_6502.Cycles += 7

}

func op6502_6A(cpu_6502 *CPU_6502) {
	cpu_6502.ror(0, true)
	cpu_6502.Cycles += 2

}

func op6502_66(cpu_6502 *CPU_6502) {
	cpu_6502.ror(cpu_6502.getZeroPage(), false)
	cpu_6502.Cycles += 5

}

func op6502_76(cpu_6502 *CPU_6502) {
	cpu_6502.ror(cpu_6502.getZeroPageX(), false)
	cpu_6502.Cycles += 6

}

func op6502_6E(cpu_6502 *CPU_6502) {
	cpu_6502.ror(cpu_6502.getAbsolute(), false)
	cpu_6502.Cycles += 6

}

func op6502_7E(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	cpu_6502.ror(addr, false)
	cpu_6502.Cycles += 7

	// Compare Operations
}

func op6502_C9(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	cpu_6502.Cycles += 2

}

func op6502_C5(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.getZeroPage()))
	cpu_6502.Cycles += 3

}

func op6502_D5(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.getZeroPageX()))
	cpu_6502.Cycles += 4

}

func op6502_CD(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.getAbsolute()))
	cpu_6502.Cycles += 4

}

func op6502_DD(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(addr))
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_D9(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(addr))
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_C1(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.getIndirectX()))
	cpu_6502.Cycles += 6

}

func op6502_D1(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getIndirectY()
	cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(addr))
	cpu_6502.Cycles += 5
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_E0(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.X, cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	cpu_6502.Cycles += 2

}

func op6502_E4(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.X, cpu_6502.readByte(cpu_6502.getZeroPage()))
	cpu_6502.Cycles += 3

}

func op6502_EC(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.X, cpu_6502.readByte(cpu_6502.getAbsolute()))
	cpu_6502.Cycles += 4

}

func op6502_C0(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.Y, cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	cpu_6502.Cycles += 2

}

func op6502_C4(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.Y, cpu_6502.readByte(cpu_6502.getZeroPage()))
	cpu_6502.Cycles += 3

}

func op6502_CC(cpu_6502 *CPU_6502) {
	cpu_6502.compare(cpu_6502.Y, cpu_6502.readByte(cpu_6502.getAbsolute()))
	cpu_6502.Cycles += 4

	// Branch Operations
}

func op6502_90(cpu_6502 *CPU_6502) {
	cpu_6502.branch(cpu_6502.SR&CARRY_FLAG == 0)

}

func op6502_B0(cpu_6502 *CPU_6502) {
	cpu_6502.branch(cpu_6502.SR&CARRY_FLAG != 0)

}

func op6502_F0(cpu_6502 *CPU_6502) {
	cpu_6502.branch(cpu_6502.SR&ZERO_FLAG != 0)

}

func op6502_D0(cpu_6502 *CPU_6502) {
	cpu_6502.branch(cpu_6502.SR&ZERO_FLAG == 0)

}

func op6502_30(cpu_6502 *CPU_6502) {
	cpu_6502.branch(cpu_6502.SR&NEGATIVE_FLAG != 0)

}

func op6502_10(cpu_6502 *CPU_6502) {
	cpu_6502.branch(cpu_6502.SR&NEGATIVE_FLAG == 0)

}

func op6502_70(cpu_6502 *CPU_6502) {
	cpu_6502.branch(cpu_6502.SR&OVERFLOW_FLAG != 0)

}

func op6502_50(cpu_6502 *CPU_6502) {
	cpu_6502.branch(cpu_6502.SR&OVERFLOW_FLAG == 0)

	// Jump/Call Operations
}

func op6502_4C(cpu_6502 *CPU_6502) {
	cpu_6502.PC = cpu_6502.getAbsolute()
	cpu_6502.Cycles += 3

}

func op6502_6C(cpu_6502 *CPU_6502) {
	low := cpu_6502.readByte(cpu_6502.PC)
	high := cpu_6502.readByte(cpu_6502.PC + 1)
	addr := uint16(low) | uint16(high)<<8
	// 6502 bug: wraps within page
	low2 := cpu_6502.readByte(addr)
	high2 := cpu_6502.readByte((addr & 0xFF00) | ((addr + 1) & 0x00FF))
	cpu_6502.PC = uint16(low2) | uint16(high2)<<8
	cpu_6502.Cycles += 5

}

func op6502_20(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getAbsolute()
	cpu_6502.push16(cpu_6502.PC - 1)
	cpu_6502.PC = addr
	cpu_6502.Cycles += 6

}

func op6502_60(cpu_6502 *CPU_6502) {
	cpu_6502.PC = cpu_6502.pop16() + 1
	cpu_6502.Cycles += 6

	// Flag Operations
}

func op6502_18(cpu_6502 *CPU_6502) {
	cpu_6502.SR &^= CARRY_FLAG
	cpu_6502.Cycles += 2

}

func op6502_38(cpu_6502 *CPU_6502) {
	cpu_6502.SR |= CARRY_FLAG
	cpu_6502.Cycles += 2

}

func op6502_58(cpu_6502 *CPU_6502) {
	cpu_6502.SR &^= INTERRUPT_FLAG
	cpu_6502.Cycles += 2

}

func op6502_78(cpu_6502 *CPU_6502) {
	cpu_6502.SR |= INTERRUPT_FLAG
	cpu_6502.Cycles += 2

}

func op6502_B8(cpu_6502 *CPU_6502) {
	cpu_6502.SR &^= OVERFLOW_FLAG
	cpu_6502.Cycles += 2

}

func op6502_D8(cpu_6502 *CPU_6502) {
	cpu_6502.SR &^= DECIMAL_FLAG
	cpu_6502.Cycles += 2

}

func op6502_F8(cpu_6502 *CPU_6502) {
	cpu_6502.SR |= DECIMAL_FLAG
	cpu_6502.Cycles += 2

	// Special Operations
}

func op6502_00(cpu_6502 *CPU_6502) {
	cpu_6502.PC++
	cpu_6502.push16(cpu_6502.PC)
	cpu_6502.push(cpu_6502.SR | BREAK_FLAG | UNUSED_FLAG)
	cpu_6502.setFlag(INTERRUPT_FLAG, true)
	cpu_6502.SR &= ^byte(BREAK_FLAG)
	cpu_6502.PC = cpu_6502.read16(IRQ_VECTOR)
	cpu_6502.Cycles += 7

}

func op6502_40(cpu_6502 *CPU_6502) {
	cpu_6502.SR = (cpu_6502.pop() & 0xEF) | UNUSED_FLAG
	cpu_6502.PC = cpu_6502.pop16()
	cpu_6502.Cycles += 6

}

func op6502_EA(cpu_6502 *CPU_6502) {
	cpu_6502.Cycles += 2

	// Unofficial NOPs
}

func op6502_1A(cpu_6502 *CPU_6502) {
	cpu_6502.Cycles += 2

}

func op6502_80(cpu_6502 *CPU_6502) {
	cpu_6502.PC++
	cpu_6502.Cycles += 2

}

func op6502_04(cpu_6502 *CPU_6502) {
	cpu_6502.PC++
	cpu_6502.Cycles += 3

}

func op6502_14(cpu_6502 *CPU_6502) {
	cpu_6502.PC++
	cpu_6502.Cycles += 4

}

func op6502_0C(cpu_6502 *CPU_6502) {
	cpu_6502.PC += 2
	cpu_6502.Cycles += 4

}

func op6502_1C(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteX()
	_ = cpu_6502.readByte(addr)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

	// Unofficial opcodes
}

func op6502_A7(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.getZeroPage())
	cpu_6502.X = cpu_6502.A
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 3

}

func op6502_B7(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.getZeroPageY())
	cpu_6502.X = cpu_6502.A
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_AF(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.getAbsolute())
	cpu_6502.X = cpu_6502.A
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4

}

func op6502_BF(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	cpu_6502.A = cpu_6502.readByte(addr)
	cpu_6502.X = cpu_6502.A
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_A3(cpu_6502 *CPU_6502) {
	cpu_6502.A = cpu_6502.readByte(cpu_6502.getIndirectX())
	cpu_6502.X = cpu_6502.A
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_B3(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getIndirectY()
	cpu_6502.A = cpu_6502.readByte(addr)
	cpu_6502.X = cpu_6502.A
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 5
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_87(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getZeroPage(), cpu_6502.A&cpu_6502.X)
	cpu_6502.Cycles += 3

}

func op6502_97(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getZeroPageY(), cpu_6502.A&cpu_6502.X)
	cpu_6502.Cycles += 4

}

func op6502_8F(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getAbsolute(), cpu_6502.A&cpu_6502.X)
	cpu_6502.Cycles += 4

}

func op6502_83(cpu_6502 *CPU_6502) {
	cpu_6502.writeByte(cpu_6502.getIndirectX(), cpu_6502.A&cpu_6502.X)
	cpu_6502.Cycles += 6

}

func op6502_EB(cpu_6502 *CPU_6502) {
	cpu_6502.sbc(cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	cpu_6502.Cycles += 2

}

func op6502_C7(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPage()
	val := cpu_6502.readByte(addr) - 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.compare(cpu_6502.A, val)
	cpu_6502.Cycles += 5

}

func op6502_D7(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPageX()
	val := cpu_6502.readByte(addr) - 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.compare(cpu_6502.A, val)
	cpu_6502.Cycles += 6

}

func op6502_CF(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getAbsolute()
	val := cpu_6502.readByte(addr) - 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.compare(cpu_6502.A, val)
	cpu_6502.Cycles += 6

}

func op6502_DF(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	val := cpu_6502.readByte(addr) - 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.compare(cpu_6502.A, val)
	cpu_6502.Cycles += 7

}

func op6502_DB(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	val := cpu_6502.readByte(addr) - 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.compare(cpu_6502.A, val)
	cpu_6502.Cycles += 7

}

func op6502_C3(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getIndirectX()
	val := cpu_6502.readByte(addr) - 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.compare(cpu_6502.A, val)
	cpu_6502.Cycles += 8

}

func op6502_D3(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getIndirectY()
	val := cpu_6502.readByte(addr) - 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.compare(cpu_6502.A, val)
	cpu_6502.Cycles += 8

}

func op6502_E7(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPage()
	val := cpu_6502.readByte(addr) + 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.sbc(val)
	cpu_6502.Cycles += 5

}

func op6502_F7(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPageX()
	val := cpu_6502.readByte(addr) + 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.sbc(val)
	cpu_6502.Cycles += 6

}

func op6502_EF(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getAbsolute()
	val := cpu_6502.readByte(addr) + 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.sbc(val)
	cpu_6502.Cycles += 6

}

func op6502_FF(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	val := cpu_6502.readByte(addr) + 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.sbc(val)
	cpu_6502.Cycles += 7

}

func op6502_FB(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	val := cpu_6502.readByte(addr) + 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.sbc(val)
	cpu_6502.Cycles += 7

}

func op6502_E3(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getIndirectX()
	val := cpu_6502.readByte(addr) + 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.sbc(val)
	cpu_6502.Cycles += 8

}

func op6502_F3(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getIndirectY()
	val := cpu_6502.readByte(addr) + 1
	cpu_6502.writeByte(addr, val)
	cpu_6502.sbc(val)
	cpu_6502.Cycles += 8

}

func op6502_07(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPage()
	val := cpu_6502.asl(addr, false)
	cpu_6502.A |= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 5

}

func op6502_17(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPageX()
	val := cpu_6502.asl(addr, false)
	cpu_6502.A |= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_0F(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getAbsolute()
	val := cpu_6502.asl(addr, false)
	cpu_6502.A |= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_1F(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	val := cpu_6502.asl(addr, false)
	cpu_6502.A |= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 7

}

func op6502_1B(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	val := cpu_6502.asl(addr, false)
	cpu_6502.A |= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 7

}

func op6502_03(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getIndirectX()
	val := cpu_6502.asl(addr, false)
	cpu_6502.A |= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 8

}

func op6502_13(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getIndirectY()
	val := cpu_6502.asl(addr, false)
	cpu_6502.A |= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 8

}

func op6502_27(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPage()
	val := cpu_6502.rol(addr, false)
	cpu_6502.A &= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 5

}

func op6502_37(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPageX()
	val := cpu_6502.rol(addr, false)
	cpu_6502.A &= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_2F(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getAbsolute()
	val := cpu_6502.rol(addr, false)
	cpu_6502.A &= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_3F(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	val := cpu_6502.rol(addr, false)
	cpu_6502.A &= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 7

}

func op6502_3B(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	val := cpu_6502.rol(addr, false)
	cpu_6502.A &= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 7

}

func op6502_23(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getIndirectX()
	val := cpu_6502.rol(addr, false)
	cpu_6502.A &= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 8

}

func op6502_33(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getIndirectY()
	val := cpu_6502.rol(addr, false)
	cpu_6502.A &= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 8

}

func op6502_47(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPage()
	val := cpu_6502.lsr(addr, false)
	cpu_6502.A ^= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 5

}

func op6502_57(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPageX()
	val := cpu_6502.lsr(addr, false)
	cpu_6502.A ^= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_4F(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getAbsolute()
	val := cpu_6502.lsr(addr, false)
	cpu_6502.A ^= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 6

}

func op6502_5F(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	val := cpu_6502.lsr(addr, false)
	cpu_6502.A ^= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 7

}

func op6502_5B(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	val := cpu_6502.lsr(addr, false)
	cpu_6502.A ^= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 7

}

func op6502_43(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getIndirectX()
	val := cpu_6502.lsr(addr, false)
	cpu_6502.A ^= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 8

}

func op6502_53(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getIndirectY()
	val := cpu_6502.lsr(addr, false)
	cpu_6502.A ^= val
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 8

}

func op6502_67(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPage()
	val := cpu_6502.ror(addr, false)
	cpu_6502.adc(val)
	cpu_6502.Cycles += 5

}

func op6502_77(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getZeroPageX()
	val := cpu_6502.ror(addr, false)
	cpu_6502.adc(val)
	cpu_6502.Cycles += 6

}

func op6502_6F(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getAbsolute()
	val := cpu_6502.ror(addr, false)
	cpu_6502.adc(val)
	cpu_6502.Cycles += 6

}

func op6502_7F(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	val := cpu_6502.ror(addr, false)
	cpu_6502.adc(val)
	cpu_6502.Cycles += 7

}

func op6502_7B(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	val := cpu_6502.ror(addr, false)
	cpu_6502.adc(val)
	cpu_6502.Cycles += 7

}

func op6502_63(cpu_6502 *CPU_6502) {
	addr := cpu_6502.getIndirectX()
	val := cpu_6502.ror(addr, false)
	cpu_6502.adc(val)
	cpu_6502.Cycles += 8

}

func op6502_73(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getIndirectY()
	val := cpu_6502.ror(addr, false)
	cpu_6502.adc(val)
	cpu_6502.Cycles += 8

	// ANC
}

func op6502_0B(cpu_6502 *CPU_6502) {
	cpu_6502.A &= cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x80 != 0)
	cpu_6502.Cycles += 2

}

func op6502_4B(cpu_6502 *CPU_6502) {
	cpu_6502.A &= cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&1 != 0)
	cpu_6502.A >>= 1
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.Cycles += 2

}

func op6502_6B(cpu_6502 *CPU_6502) {
	cpu_6502.A &= cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	carry := cpu_6502.SR&CARRY_FLAG != 0
	carryBit := byte(0)
	if carry {
		carryBit = 0x80
	}
	cpu_6502.A = (cpu_6502.A >> 1) | carryBit
	cpu_6502.updateNZ(cpu_6502.A)
	cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x40 != 0)
	cpu_6502.setFlag(OVERFLOW_FLAG, ((cpu_6502.A>>6)^(cpu_6502.A>>5))&1 != 0)
	cpu_6502.Cycles += 2

}

func op6502_CB(cpu_6502 *CPU_6502) {
	val := cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	result := int(cpu_6502.A&cpu_6502.X) - int(val)
	cpu_6502.X = byte(result)
	cpu_6502.setFlag(CARRY_FLAG, result >= 0)
	cpu_6502.updateNZ(cpu_6502.X)
	cpu_6502.Cycles += 2

}

func op6502_9F(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	value := cpu_6502.A & cpu_6502.X & byte((addr>>8)+1)
	cpu_6502.writeByte(addr, value)
	cpu_6502.Cycles += 5

}

func op6502_93(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getIndirectY()
	value := cpu_6502.A & cpu_6502.X & byte((addr>>8)+1)
	cpu_6502.writeByte(addr, value)
	cpu_6502.Cycles += 6

}

func op6502_9E(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteY()
	value := cpu_6502.X & byte((addr>>8)+1)
	if (addr&0xFF)+uint16(cpu_6502.Y) > 0xFF {
		value &= byte(addr >> 8)
	}
	cpu_6502.writeByte(addr, value)
	cpu_6502.Cycles += 5

}

func op6502_9C(cpu_6502 *CPU_6502) {
	addr, _ := cpu_6502.getAbsoluteX()
	value := cpu_6502.Y & byte((addr>>8)+1)
	if (addr&0xFF)+uint16(cpu_6502.X) > 0xFF {
		value &= byte(addr >> 8)
	}
	cpu_6502.writeByte(addr, value)
	cpu_6502.Cycles += 5

}

func op6502_9B(cpu_6502 *CPU_6502) {
	cpu_6502.SP = cpu_6502.A & cpu_6502.X
	addr, _ := cpu_6502.getAbsoluteY()
	value := cpu_6502.SP & byte((addr>>8)+1)
	cpu_6502.writeByte(addr, value)
	cpu_6502.Cycles += 5

}

func op6502_BB(cpu_6502 *CPU_6502) {
	addr, crossed := cpu_6502.getAbsoluteY()
	value := cpu_6502.readByte(addr) & cpu_6502.SP
	cpu_6502.A = value
	cpu_6502.X = value
	cpu_6502.SP = value
	cpu_6502.updateNZ(value)
	cpu_6502.Cycles += 4
	if crossed {
		cpu_6502.Cycles++
	}

}

func op6502_AB(cpu_6502 *CPU_6502) {
	val := cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	cpu_6502.A = val
	cpu_6502.X = val
	cpu_6502.updateNZ(val)
	cpu_6502.Cycles += 2

}

func op6502_02(cpu_6502 *CPU_6502) {
	cpu_6502.running.Store(false)

}
