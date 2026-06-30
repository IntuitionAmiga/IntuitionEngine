//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"
)

const m68kDiffStartPC = uint32(0x1000)
const m68kDiffBusSize = uint64(16 << 20)

type m68kDiffMemWatch struct {
	addr uint32
	size int
}

type m68kDiffCase struct {
	name             string
	words            []uint16
	setup            func(*M68KCPU)
	watch            []m68kDiffMemWatch
	requireProdSafe  bool
	requireNativeRun bool
}

type m68kFPUDiffCase struct {
	name  string
	words []uint16
	setup func(*M68KCPU)
	watch []m68kDiffMemWatch
}

func newM68KDiffTestProgramCPU(t *testing.T, startPC uint32) *M68KCPU {
	t.Helper()

	bus, err := NewMachineBusSized(m68kDiffBusSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	cpu.m68kJitWarmupLimit = 1
	return cpu
}

func newM68KDiffJITTestRig(t *testing.T) *m68kJITTestRig {
	t.Helper()

	bus, err := NewMachineBusSized(m68kDiffBusSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x00001000)
	cpu := NewM68KCPU(bus)

	em, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	t.Cleanup(func() { em.Free() })

	bitmap := make([]byte, (uint32(len(cpu.memory))+4095)>>12)
	pageMin := make([]uint16, len(bitmap))
	pageMax := make([]uint16, len(bitmap))
	ctx := newM68KJITContext(cpu, bitmap, pageMin, pageMax)

	return &m68kJITTestRig{cpu: cpu, execMem: em, ctx: ctx, bitmap: bitmap}
}

func TestM68KJIT_Differential_ProductionRegisterDirectOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase

	for dst := uint16(0); dst < 8; dst++ {
		for _, imm := range []uint16{0x00, 0x01, 0x7F, 0x80, 0xFF} {
			opcode := uint16(0x7000 | dst<<9 | imm)
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("MOVEQ_%02X_D%d", imm, dst), opcode))
		}
	}

	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		for src := uint16(0); src < 8; src++ {
			for dst := uint16(0); dst < 8; dst++ {
				opcode := m68kDiffMoveOpcode(size, M68K_AM_DR, src, M68K_AM_DR, dst)
				cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("MOVE_%s_D%d_D%d", m68kDiffSizeName(size), src, dst), opcode))
			}
		}
	}

	for _, family := range []struct {
		name string
		base uint16
	}{
		{name: "ADD", base: 0xD000},
		{name: "SUB", base: 0x9000},
		{name: "CMP", base: 0xB000},
		{name: "OR", base: 0x8000},
		{name: "AND", base: 0xC000},
	} {
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			for src := uint16(0); src < 8; src++ {
				for dst := uint16(0); dst < 8; dst++ {
					opcode := family.base | dst<<9 | uint16(size)<<6 | src
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_%s_D%d_D%d", family.name, m68kDiffSizeName(size), src, dst), opcode))
				}
			}
		}
	}

	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		for src := uint16(0); src < 8; src++ {
			for dst := uint16(0); dst < 8; dst++ {
				opcode := uint16(0xB000 | src<<9 | uint16(size+4)<<6 | dst)
				cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("EOR_%s_D%d_D%d", m68kDiffSizeName(size), src, dst), opcode))
			}
		}
	}

	for _, base := range []struct {
		name string
		op   uint16
	}{
		{name: "ADDQ", op: 0x5000},
		{name: "SUBQ", op: 0x5100},
	} {
		for _, imm := range []uint16{1, 3, 8} {
			encodedImm := imm
			if encodedImm == 8 {
				encodedImm = 0
			}
			for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
				for reg := uint16(0); reg < 8; reg++ {
					opcode := base.op | encodedImm<<9 | uint16(size)<<6 | uint16(M68K_AM_DR)<<3 | reg
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_%d_%s_D%d", base.name, imm, m68kDiffSizeName(size), reg), opcode))
				}
				for reg := uint16(0); reg < 7; reg++ {
					opcode := base.op | encodedImm<<9 | uint16(size)<<6 | uint16(M68K_AM_AR)<<3 | reg
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_%d_%s_A%d", base.name, imm, m68kDiffSizeName(size), reg), opcode))
				}
			}
		}
	}

	for reg := uint16(0); reg < 8; reg++ {
		cases = append(cases,
			m68kNativeDiffCase(fmt.Sprintf("SWAP_D%d", reg), 0x4840|reg),
			m68kNativeDiffCase(fmt.Sprintf("EXT_W_D%d", reg), 0x4880|reg),
			m68kNativeDiffCase(fmt.Sprintf("EXT_L_D%d", reg), 0x48C0|reg),
			m68kNativeDiffCase(fmt.Sprintf("EXTB_L_D%d", reg), 0x49C0|reg),
		)
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("CLR_%s_D%d", m68kDiffSizeName(size), reg), 0x4200|uint16(size)<<6|reg))
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("NEG_%s_D%d", m68kDiffSizeName(size), reg), 0x4400|uint16(size)<<6|reg))
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("NOT_%s_D%d", m68kDiffSizeName(size), reg), 0x4600|uint16(size)<<6|reg))
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("TST_%s_D%d", m68kDiffSizeName(size), reg), 0x4A00|uint16(size)<<6|reg))
		}
	}

	for _, base := range []struct {
		name string
		op   uint16
	}{
		{name: "ADDA", op: 0xD000},
		{name: "SUBA", op: 0x9000},
	} {
		for _, opSize := range []struct {
			name   string
			opmode uint16
		}{
			{name: "W", opmode: 3},
			{name: "L", opmode: 7},
		} {
			for dst := uint16(0); dst < 7; dst++ {
				for src := uint16(0); src < 8; src++ {
					cases = append(cases,
						m68kNativeDiffCase(fmt.Sprintf("%s_%s_D%d_A%d", base.name, opSize.name, src, dst), base.op|dst<<9|opSize.opmode<<6|uint16(M68K_AM_DR)<<3|src),
						m68kNativeDiffCase(fmt.Sprintf("%s_%s_A%d_A%d", base.name, opSize.name, src, dst), base.op|dst<<9|opSize.opmode<<6|uint16(M68K_AM_AR)<<3|src),
					)
				}
			}
		}
	}

	for _, dir := range []struct {
		name string
		bit  uint16
	}{
		{name: "ROR", bit: 0},
		{name: "ROL", bit: 1},
	} {
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			for _, count := range []uint16{1, 4, 8} {
				encodedCount := count
				if encodedCount == 8 {
					encodedCount = 0
				}
				for reg := uint16(0); reg < 8; reg++ {
					opcode := uint16(0xE000 | encodedCount<<9 | dir.bit<<8 | uint16(size)<<6 | 3<<3 | reg)
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_%d_%s_D%d", dir.name, count, m68kDiffSizeName(size), reg), opcode))
				}
			}
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ForceNativeSimpleEAModes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		a2Base   = uint32(0x3200)
		a3Base   = uint32(0x3300)
		absShort = uint32(0x3400)
		absLong  = uint32(0x00036000)
	)

	cases := []m68kDiffCase{
		m68kNativeDiffCase("MOVE_L_A2ind_D0", m68kDiffMoveOpcode(M68K_SIZE_LONG, M68K_AM_AR_IND, 2, M68K_AM_DR, 0)),
		m68kNativeDiffCase("MOVE_L_A2post_D0", m68kDiffMoveOpcode(M68K_SIZE_LONG, M68K_AM_AR_POST, 2, M68K_AM_DR, 0)),
		m68kNativeDiffCase("MOVE_L_d16A2_D0", m68kDiffMoveOpcode(M68K_SIZE_LONG, M68K_AM_AR_DISP, 2, M68K_AM_DR, 0), 0x0010),
		m68kNativeDiffCase("MOVE_L_absW_D0", m68kDiffMoveOpcode(M68K_SIZE_LONG, 7, 0, M68K_AM_DR, 0), uint16(absShort)),
		m68kNativeDiffCase("MOVE_L_absL_D0", m68kDiffMoveOpcode(M68K_SIZE_LONG, 7, 1, M68K_AM_DR, 0), uint16(absLong>>16), uint16(absLong&0xFFFF)),
		m68kNativeDiffCase("MOVE_L_imm_D0", m68kDiffMoveOpcode(M68K_SIZE_LONG, 7, 4, M68K_AM_DR, 0), 0x89AB, 0xCDEF),
		m68kNativeDiffCase("MOVE_L_D0_A3ind", m68kDiffMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, M68K_AM_AR_IND, 3)),
		m68kNativeDiffCase("MOVE_L_D0_A3post", m68kDiffMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, M68K_AM_AR_POST, 3)),
		m68kNativeDiffCase("MOVE_L_D0_d16A3", m68kDiffMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, M68K_AM_AR_DISP, 3), 0x0014),
		m68kNativeDiffCase("MOVE_L_A6_d16A7", 0x2F4E, 0x0004),
		m68kNativeDiffCase("MOVE_L_D0_absW", m68kDiffMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, 7, 0), uint16(absShort+0x20)),
		m68kNativeDiffCase("MOVE_L_D0_absL", m68kDiffMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, 7, 1), uint16((absLong+0x20)>>16), uint16((absLong+0x20)&0xFFFF)),
		m68kNativeDiffCase("CMPI_L_A2ind", 0x0C92, 0x0102, 0x0304),
		m68kNativeDiffCase("CMPI_L_A2post", 0x0C9A, 0x0102, 0x0304),
		m68kNativeDiffCase("CMPI_L_d16A2", 0x0CAA, 0x0102, 0x0304, 0x0010),
		m68kNativeDiffCase("ADD_L_A2ind_D1", 0xD292),
		m68kNativeDiffCase("ADD_L_A1_D3", 0xD689),
		m68kNativeDiffCase("SUB_L_A2ind_D1", 0x9292),
		m68kNativeDiffCase("SUB_L_A2_D3", 0x968A),
		m68kNativeDiffCase("CMP_L_A2ind_D1", 0xB292),
	}

	for i := range cases {
		cases[i].setup = func(cpu *M68KCPU) {
			cpu.AddrRegs[2] = a2Base
			cpu.AddrRegs[3] = a3Base
			cpu.Write32(a2Base, 0x01020304)
			cpu.Write32(a2Base+0x10, 0x11223344)
			cpu.Write32(absShort, 0x55667788)
			cpu.Write32(absShort+0x20, 0xA5A5A5A5)
			cpu.Write32(absLong, 0x99AABBCC)
			cpu.Write32(absLong+0x20, 0x5A5A5A5A)
		}
		cases[i].watch = []m68kDiffMemWatch{
			{addr: a2Base, size: M68K_SIZE_LONG},
			{addr: a2Base + 0x10, size: M68K_SIZE_LONG},
			{addr: a3Base, size: M68K_SIZE_LONG},
			{addr: a3Base + 0x14, size: M68K_SIZE_LONG},
			{addr: absShort, size: M68K_SIZE_LONG},
			{addr: absShort + 0x20, size: M68K_SIZE_LONG},
			{addr: absLong, size: M68K_SIZE_LONG},
			{addr: absLong + 0x20, size: M68K_SIZE_LONG},
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ProductionImmediateOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase

	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "ORI", base: 0x0000},
		{name: "ANDI", base: 0x0200},
		{name: "EORI", base: 0x0A00},
	} {
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			for reg := uint16(0); reg < 8; reg++ {
				for _, imm := range m68kDiffRepresentativeImms(size) {
					opcode := op.base | uint16(size)<<6 | uint16(M68K_AM_DR)<<3 | reg
					words := append([]uint16{opcode}, m68kDiffImmWords(size, imm)...)
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_%s_%X_D%d", op.name, m68kDiffSizeName(size), imm, reg), words...))
				}
			}
		}
	}

	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		for reg := uint16(0); reg < 8; reg++ {
			for _, imm := range m68kDiffRepresentativeImms(size) {
				opcode := uint16(0x0C00 | uint16(size)<<6 | uint16(M68K_AM_DR)<<3 | reg)
				words := append([]uint16{opcode}, m68kDiffImmWords(size, imm)...)
				cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("CMPI_%s_%X_D%d", m68kDiffSizeName(size), imm, reg), words...))
			}
		}
	}

	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "SUBI", base: 0x0400},
		{name: "ADDI", base: 0x0600},
	} {
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			for reg := uint16(0); reg < 8; reg++ {
				for _, imm := range m68kDiffRepresentativeImms(size) {
					opcode := op.base | uint16(size)<<6 | uint16(M68K_AM_DR)<<3 | reg
					words := append([]uint16{opcode}, m68kDiffImmWords(size, imm)...)
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_%s_%X_D%d", op.name, m68kDiffSizeName(size), imm, reg), words...))
				}
			}
		}
	}

	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		imm := m68kDiffCompareImm(size)
		for _, ea := range []struct {
			name string
			mode uint16
			reg  uint16
			ext  []uint16
		}{
			{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
			{name: "A2post", mode: M68K_AM_AR_POST, reg: 2},
			{name: "A2pre", mode: M68K_AM_AR_PRE, reg: 2},
			{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
			{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
			{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
		} {
			opcode := uint16(0x0C00 | uint16(size)<<6 | ea.mode<<3 | ea.reg)
			words := append([]uint16{opcode}, m68kDiffImmWords(size, imm)...)
			words = append(words, ea.ext...)
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("CMPI_%s_%s", m68kDiffSizeName(size), ea.name), words...))
		}
	}

	for i := range cases {
		cases[i].setup = m68kDiffSimpleEASetup
		cases[i].watch = m68kDiffSimpleEAWatch()
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ProductionMultiplyDivideOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kMultiplyDivideDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func m68kMultiplyDivideDiffCases() []m68kDiffCase {
	const sourceLong = uint32(0x3200)
	return []m68kDiffCase{
		{
			name:  "MULU_W_imm_D0",
			words: []uint16{0xC0FC, 0xFFFF},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x0000FFFF
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "MULS_W_imm_D0",
			words: []uint16{0xC1FC, 0xFFFF},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x00000010
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "DIVU_W_imm_D0",
			words: []uint16{0x80FC, 0x000A},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 101
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "DIVS_W_imm_D0",
			words: []uint16{0x81FC, 0xFFFE},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0xFFFFFFF9
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "MULL_L_D3_D2",
			words: []uint16{0x4C03, 0x2000},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 7
				cpu.DataRegs[2] = 6
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "MULL_L_D0_D2_D1_signed64",
			words: []uint16{0x4C00, 0x1C02},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0xFFFFFFFD
				cpu.DataRegs[1] = 7
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "MULL_L_A2post_D0",
			words: []uint16{0x4C1A, 0x0800},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 3
				cpu.AddrRegs[2] = sourceLong
				cpu.Write32(sourceLong, 14)
			},
			watch:            []m68kDiffMemWatch{{addr: sourceLong, size: M68K_SIZE_LONG}},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "DIVL_L_D0_D1_D2",
			words: []uint16{0x4C40, 0x2001},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 4
				cpu.DataRegs[2] = 14
				cpu.DataRegs[1] = 0x11111111
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "DIVL_L_D0_D2_D1_signed64",
			words: []uint16{0x4C40, 0x1C02},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 3
				cpu.DataRegs[2] = 0xFFFFFFFF
				cpu.DataRegs[1] = 0xFFFFFFEB
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
		{
			name:  "DIVL_L_A0post_D1_D2",
			words: []uint16{0x4C58, 0x2001},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[2] = 14
				cpu.AddrRegs[0] = sourceLong
				cpu.Write32(sourceLong, 4)
			},
			watch:            []m68kDiffMemWatch{{addr: sourceLong, size: M68K_SIZE_LONG}},
			requireProdSafe:  true,
			requireNativeRun: true,
		},
	}
}

func TestM68KJIT_Differential_ProductionMoveAndArithmeticEASizes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase
	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		for _, ea := range []struct {
			name string
			mode uint16
			reg  uint16
			ext  []uint16
		}{
			{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
			{name: "A2post", mode: M68K_AM_AR_POST, reg: 2},
			{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
			{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
			{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
			{name: "imm", mode: 7, reg: 4, ext: m68kDiffImmWords(size, m68kDiffCompareImm(size))},
		} {
			opcode := m68kDiffMoveOpcode(size, ea.mode, ea.reg, M68K_AM_DR, 0)
			words := append([]uint16{opcode}, ea.ext...)
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("MOVE_%s_%s_D0", m68kDiffSizeName(size), ea.name), words...))
		}

		if size == M68K_SIZE_WORD || size == M68K_SIZE_LONG {
			moveAImm := uint32(0x01020304)
			if size == M68K_SIZE_WORD {
				moveAImm = 0x8001
			}
			for _, dst := range []uint16{1, 6} {
				for _, ea := range []struct {
					name string
					mode uint16
					reg  uint16
					ext  []uint16
				}{
					{name: "D0", mode: M68K_AM_DR, reg: 0},
					{name: "A2", mode: M68K_AM_AR, reg: 2},
					{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
					{name: "A2post", mode: M68K_AM_AR_POST, reg: 2},
					{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
					{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
					{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
					{name: "imm", mode: 7, reg: 4, ext: m68kDiffImmWords(size, moveAImm)},
				} {
					opcode := m68kDiffMoveOpcode(size, ea.mode, ea.reg, M68K_AM_AR, dst)
					words := append([]uint16{opcode}, ea.ext...)
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("MOVEA_%s_%s_A%d", m68kDiffSizeName(size), ea.name, dst), words...))
				}
			}
		}

		for _, ea := range []struct {
			name string
			mode uint16
			reg  uint16
			ext  []uint16
		}{
			{name: "A3ind", mode: M68K_AM_AR_IND, reg: 3},
			{name: "A3post", mode: M68K_AM_AR_POST, reg: 3},
			{name: "d16A3", mode: M68K_AM_AR_DISP, reg: 3, ext: []uint16{0x0014}},
			{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3420}},
			{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6020}},
		} {
			opcode := m68kDiffMoveOpcode(size, M68K_AM_DR, 0, ea.mode, ea.reg)
			words := append([]uint16{opcode}, ea.ext...)
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("MOVE_%s_D0_%s", m68kDiffSizeName(size), ea.name), words...))
		}

		for _, family := range []struct {
			name string
			base uint16
		}{
			{name: "ADD", base: 0xD000},
			{name: "SUB", base: 0x9000},
			{name: "CMP", base: 0xB000},
		} {
			for _, ea := range []struct {
				name string
				mode uint16
				reg  uint16
				ext  []uint16
			}{
				{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
				{name: "A2post", mode: M68K_AM_AR_POST, reg: 2},
				{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
				{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
				{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
				{name: "imm", mode: 7, reg: 4, ext: m68kDiffImmWords(size, m68kDiffCompareImm(size))},
			} {
				opcode := family.base | 1<<9 | uint16(size)<<6 | ea.mode<<3 | ea.reg
				words := append([]uint16{opcode}, ea.ext...)
				cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_%s_%s_D1", family.name, m68kDiffSizeName(size), ea.name), words...))
			}
		}

		if size != M68K_SIZE_BYTE {
			for src := uint16(0); src < 7; src++ {
				opcode := uint16(0xB000 | 1<<9 | uint16(size)<<6 | uint16(M68K_AM_AR)<<3 | src)
				cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("CMP_%s_A%d_D1", m68kDiffSizeName(size), src), opcode))
			}
		}
	}

	for i := range cases {
		cases[i].setup = m68kDiffSimpleEASetup
		cases[i].watch = m68kDiffSimpleEAWatch()
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_AROSClearLoopMoveLongD2PostincA0(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const dst = uint32(0x8AD300)
	tc := m68kDiffCase{
		name: "MOVE_L_D2_A0post_allocator_clear_shape",
		words: []uint16{
			0x20C2, // MOVE.L D2,(A0)+
			0x20C2, // MOVE.L D2,(A0)+
			0x20C2, // MOVE.L D2,(A0)+
			0x20C2, // MOVE.L D2,(A0)+
		},
		setup: func(cpu *M68KCPU) {
			cpu.DataRegs[2] = 0
			cpu.AddrRegs[0] = dst
			for off := uint32(0); off < 0x20; off += 4 {
				cpu.Write32(dst+off, 0xA5000000|off)
			}
		},
		watch: []m68kDiffMemWatch{
			{addr: dst, size: M68K_SIZE_LONG},
			{addr: dst + 4, size: M68K_SIZE_LONG},
			{addr: dst + 8, size: M68K_SIZE_LONG},
			{addr: dst + 12, size: M68K_SIZE_LONG},
			{addr: dst + 16, size: M68K_SIZE_LONG},
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}

	runM68KJITDifferentialBlock(t, tc, 4)
}

func TestM68KJIT_Differential_AROSListSpliceBlock6063E8(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		head = uint32(0x8AD300)
		node = uint32(0x8AD360)
		next = uint32(0x8AD3C0)
	)
	tc := m68kDiffCase{
		name: "AROS_list_splice_006063D4_to_006063E8",
		words: []uint16{
			0x2F0A,         // MOVE.L A2,-(A7)
			0x45E8, 0x0004, // LEA 4(A0),A2
			0x228A,                 // MOVE.L A2,(A1)
			0x2368, 0x0008, 0x0004, // MOVE.L 8(A0),4(A1)
			0x2468, 0x0008, // MOVEA.L 8(A0),A2
			0x2489,         // MOVE.L A1,(A2)
			0x2149, 0x0008, // MOVE.L A1,8(A0)
		},
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = head
			cpu.AddrRegs[1] = node
			cpu.AddrRegs[2] = 0xA2A2A2A2
			cpu.AddrRegs[7] = 0x10000
			cpu.Write32(head+4, 0x11111111)
			cpu.Write32(head+8, next)
			cpu.Write32(node+0, 0x22222222)
			cpu.Write32(node+4, 0x33333333)
			cpu.Write32(next+0, 0x44444444)
			cpu.Write32(0x0FFFC, 0x55555555)
		},
		watch: []m68kDiffMemWatch{
			{addr: head + 4, size: M68K_SIZE_LONG},
			{addr: head + 8, size: M68K_SIZE_LONG},
			{addr: node + 0, size: M68K_SIZE_LONG},
			{addr: node + 4, size: M68K_SIZE_LONG},
			{addr: next + 0, size: M68K_SIZE_LONG},
			{addr: 0x0FFFC, size: M68K_SIZE_LONG},
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}

	runM68KJITDifferentialBlock(t, tc, 7)
}

func TestM68KJIT_Differential_AROSListSpliceBlock60616C(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		head  = uint32(0x8ADC10)
		node  = uint32(0x953898)
		next  = uint32(0x8ADC50)
		stack = uint32(0x10000)
	)
	tc := m68kDiffCase{
		name: "AROS_list_splice_0060616C",
		words: []uint16{
			0x2F0A,         // MOVE.L A2,-(A7)
			0x2290,         // MOVE.L (A0),(A1)
			0x2348, 0x0004, // MOVE.L A0,4(A1)
			0x2450,         // MOVEA.L (A0),A2
			0x2549, 0x0004, // MOVE.L A1,4(A2)
			0x2089, // MOVE.L A1,(A0)
			0x245F, // MOVEA.L (A7)+,A2
		},
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = head
			cpu.AddrRegs[1] = node
			cpu.AddrRegs[2] = 0xA2A2A2A2
			cpu.AddrRegs[7] = stack
			cpu.Write32(head+0, next)
			cpu.Write32(head+4, 0x11111111)
			cpu.Write32(node+0, 0x22222222)
			cpu.Write32(node+4, 0x33333333)
			cpu.Write32(next+0, 0x44444444)
			cpu.Write32(next+4, 0x55555555)
			cpu.Write32(stack-4, 0x66666666)
		},
		watch: []m68kDiffMemWatch{
			{addr: head + 0, size: M68K_SIZE_LONG},
			{addr: head + 4, size: M68K_SIZE_LONG},
			{addr: node + 0, size: M68K_SIZE_LONG},
			{addr: node + 4, size: M68K_SIZE_LONG},
			{addr: next + 0, size: M68K_SIZE_LONG},
			{addr: next + 4, size: M68K_SIZE_LONG},
			{addr: stack - 4, size: M68K_SIZE_LONG},
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}

	runM68KJITDifferentialBlock(t, tc, 7)
}

func TestM68KJIT_Differential_PEANEGXStackTrampolineJMP(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		stack  = uint32(0x10000)
		target = uint32(0x23456)
	)
	tc := m68kDiffCase{
		name: "PEA_NEGX_predec_stack_trampoline_JMP",
		words: []uint16{
			0x487A, 0x0006, // PEA 6(PC)
			0x40E7, // NEGX.B -(A7)
			0x4ED5, // JMP (A5)
		},
		setup: func(cpu *M68KCPU) {
			cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_Z
			cpu.AddrRegs[5] = target
			cpu.AddrRegs[7] = stack
			for off := uint32(0); off < 16; off += 4 {
				cpu.Write32(stack-16+off, 0xA5000000|off)
			}
		},
		watch: []m68kDiffMemWatch{
			{addr: stack - 8, size: M68K_SIZE_LONG},
			{addr: stack - 4, size: M68K_SIZE_LONG},
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}

	runM68KJITDifferentialBlock(t, tc, 3)
}

// TestM68KJIT_Differential_NEGByteXFlag reproduces an AROS verifier divergence:
// MOVE.B 15(A2),D0; NEG.B D0; BNE. For a zero operand NEG must clear X (X=C=0);
// a stale lazy-X slot would leave X set. Pre-dirties X/C to expose it.
func TestM68KJIT_Differential_NEGByteXFlag(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const opAddr = uint32(0x9000)
	for _, sub := range []struct {
		name   string
		opByte uint8
	}{
		{"operand_zero_X_clear", 0x00},
		{"operand_nonzero_X_set", 0x40},
	} {
		opByte := sub.opByte
		tc := m68kDiffCase{
			name: "NEG_B_Xflag_" + sub.name,
			words: []uint16{
				0x102A, 0x000F, // MOVE.B 15(A2),D0
				0x4400, // NEG.B D0
				0x6604, // BNE.S *+6
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[2] = opAddr
				cpu.Write8(opAddr+15, opByte)
				cpu.SR |= M68K_SR_X | M68K_SR_C
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		}
		runM68KJITDifferentialBlock(t, tc, 3)
	}
}

// TestM68KJIT_Differential_MoveToCCRThenPreserveX reproduces the AROS block at
// 0x006506F6 (MOVE D0,CCR) followed by an X-preserving MOVEQ. MOVE-to-CCR
// installs X into R14 bit 4 but must also write the lazy X stack slot, else the
// following MOVEQ→flagsLiveLogi rebuilds CCR from the stale slot and keeps the
// pre-MOVE X. Pre-dirties X=1 with D0's X bit clear to expose it.
func TestM68KJIT_Differential_MoveToCCRThenPreserveX(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, sub := range []struct {
		name string
		d0   uint32
	}{
		{"d0_X_clear", 0x00000004}, // CCR = Z only; X must end 0
		{"d0_X_set", 0x00000014},   // CCR = X|Z; X must end 1
	} {
		d0 := sub.d0
		tc := m68kDiffCase{
			name: "MOVE_D0_CCR_then_MOVEQ_" + sub.name,
			words: []uint16{
				0x44C0, // MOVE D0,CCR
				0x7201, // MOVEQ #1,D1 (preserves X, clears Z)
				0x6702, // BEQ.S *+4 (Z=0 → not taken, fall through, materialize)
			},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = d0
				cpu.SR |= M68K_SR_X | M68K_SR_C
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		}
		runM68KJITDifferentialBlock(t, tc, 3)
	}
}

// TestM68KJIT_Differential_TSTThenMOVEAPostincPreservesZ reproduces the AROS
// epilogue at 0x0060B18C: TST.L D0; MOVEA.L (A7)+,A6; UNLK A5; RTS. TST defers
// N/Z in host EFLAGS (flagsLiveLogi); the following MOVEA postinc clobbers
// EFLAGS via the A7+=4 ADD before materialization, so without forcing
// materialization the Z from TST is lost at the RTS exit.
func TestM68KJIT_Differential_TSTThenMOVEAPostincPreservesZ(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const stack = uint32(0x20000)
	for _, sub := range []struct {
		name string
		d0   uint32
	}{
		{"d0_zero_Z_set", 0x00000000},
		{"d0_nonzero_Z_clear", 0x00000001},
	} {
		d0 := sub.d0
		tc := m68kDiffCase{
			// TST sets Z; MOVEA postinc must preserve it so the BEQ branches
			// the same way as the interpreter. A lost Z flips the branch → PC
			// mismatch, a direct functional check beyond the SR compare.
			name: "TST_L_D0_then_MOVEA_A7post_BEQ_" + sub.name,
			words: []uint16{
				0x4A80, // TST.L D0
				0x2C5F, // MOVEA.L (A7)+,A6
				0x6702, // BEQ.S *+4
			},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = d0
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack, 0xCAFEBABE) // popped into A6
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		}
		runM68KJITDifferentialBlock(t, tc, 3)
	}
}

// TestM68KJIT_Differential_RegCountShiftFlags reproduces AROS blocks
// 0x00703604 / 0x007035FA and sweeps every register-count shift/rotate
// (ASx/LSx/ROx/ROXx, .L) over counts including 0 and >=32. The interpreter is
// the oracle: ExecShiftRotate returns immediately on count 0 leaving ALL CCR
// unchanged (including ROXx — it does NOT set C=X), which the native count-0
// skip used to drop, leaving stale flags. Pre-dirties X/C to expose it.
func TestM68KJIT_Differential_RegCountShiftFlags(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	ops := []struct {
		name   string
		opcode uint16
	}{
		{"ASR_L", 0xE0A1}, {"ASL_L", 0xE1A1},
		{"LSR_L", 0xE0A9}, {"LSL_L", 0xE1A9},
		{"ROR_L", 0xE0B9}, {"ROL_L", 0xE1B9},
	}
	for _, op := range ops {
		for _, count := range []uint32{0, 1, 4, 15, 16, 31, 32, 33, 40, 63, 64, 0xFFFFFFF0} {
			c := count
			opcode := op.opcode
			name := fmt.Sprintf("MOVEQ16_%s_D0_D1_count_%d", op.name, c)
			tc := m68kDiffCase{
				name: name,
				words: []uint16{
					0x7210, // MOVEQ #16,D1
					opcode, // <shift>.L D0,D1
				},
				setup: func(cpu *M68KCPU) {
					cpu.DataRegs[0] = c
					cpu.SR |= M68K_SR_X | M68K_SR_C
				},
				requireProdSafe:  true,
				requireNativeRun: true,
			}
			t.Run(name, func(t *testing.T) { runM68KJITDifferentialBlock(t, tc, 2) })
		}
	}
}

// TestM68KJIT_ROXRegisterCountNative asserts ROXL/ROXR with a register count is
// admitted to the production-native path AND matches the interpreter across all
// counts (including 0 and >= the operand width). The earlier exclusion feared
// the native LOOP counter (RCX) aliasing the rotate target, but the M68K JIT's
// static register map only ever allocates D0->RBX and D1->RBP; every other data
// register resolves into RAX, so the rotate target is never RCX. Count 0 leaves
// all CCR unchanged, matching ExecShiftRotate's early return.
func TestM68KJIT_ROXRegisterCountNative(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	regCount := []uint16{0xE0B1, 0xE1B1, 0xE070, 0xE170, 0xE030, 0xE130} // ROXR/ROXL .L/.W/.B Dm,Dn
	for _, op := range regCount {
		ji := &M68KJITInstr{opcode: op}
		if !m68kInstrProductionNativeSafe(ji) {
			t.Fatalf("ROX register-count opcode 0x%04X must be production-native safe", op)
		}
	}
	// ROX immediate (#n,Dn) also stays native.
	for _, op := range []uint16{0xE210, 0xE310, 0xE250, 0xE350} { // ROXR/ROXL #1,Dn .B/.W
		ji := &M68KJITInstr{opcode: op}
		if !m68kInstrProductionNativeSafe(ji) {
			t.Fatalf("ROX immediate opcode 0x%04X should remain production-native safe", op)
		}
	}

	// Differential: ROXR/ROXL .B/.W/.L D0,D1 over counts spanning 0, 1, the
	// operand width, the modulus boundary, and >63 (masked to 6 bits). Pre-dirty
	// X and C so count-0 (leaves CCR unchanged) and the X-feed are both exercised.
	ops := []struct {
		name   string
		opcode uint16
	}{
		{"ROXR_B", 0xE030}, {"ROXL_B", 0xE130},
		{"ROXR_W", 0xE070}, {"ROXL_W", 0xE170},
		{"ROXR_L", 0xE0B1}, {"ROXL_L", 0xE1B1},
	}
	for _, op := range ops {
		for _, count := range []uint32{0, 1, 2, 7, 8, 9, 16, 17, 31, 32, 33, 40, 63, 64, 0xFFFFFFF0} {
			c := count
			opcode := op.opcode
			name := fmt.Sprintf("%s_D0_D1_count_%d", op.name, c)
			tc := m68kDiffCase{
				name: name,
				words: []uint16{
					0x7255, // MOVEQ #$55,D1 (non-trivial bit pattern)
					opcode, // ROXx D0,D1
				},
				setup: func(cpu *M68KCPU) {
					cpu.DataRegs[0] = c
					cpu.SR |= M68K_SR_X | M68K_SR_C
				},
				requireProdSafe:  true,
				requireNativeRun: true,
			}
			t.Run(name, func(t *testing.T) { runM68KJITDifferentialBlock(t, tc, 2) })
		}
	}
}

// TestM68KJIT_Differential_AROSBlock0061FBD4 reproduces the verifier C-flag
// divergence at 0x0061FBD4: a CMP.L/BCC whose carry, if materialized wrong,
// flips a memory-writing branch and corrupts an exec list. Drives the BNE-not-
// taken + BCC-taken exit and checks full SR parity.
func TestM68KJIT_Differential_AROSBlock0061FBD4(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const base = uint32(0x9000)
	tc := m68kDiffCase{
		name: "AROS_0061FBD4_cmp_bcc_carry",
		words: []uint16{
			0x2802,         // MOVE.L D2,D4
			0x0244, 0xFC00, // ANDI.W #$FC00,D4
			0xB284,                 // CMP.L D4,D1
			0x6616,                 // BNE.S $+0x18 (not taken: D1==D4)
			0x0282, 0x0000, 0x03FF, // ANDI.L #$000003FF,D2
			0xB480, // CMP.L D0,D2
			0x64D0, // BCC.S backward (taken: D2>=D0 unsigned, C=0)
		},
		setup: func(cpu *M68KCPU) {
			cpu.DataRegs[2] = 0x000003FF // after ANDI.L stays 0x3FF
			cpu.DataRegs[1] = 0          // == D4 (= D2&0xFC00 word = 0) so BNE not taken
			cpu.DataRegs[0] = 0x100      // D2(0x3FF) >= D0(0x100) → C=0, BCC taken
			cpu.AddrRegs[0] = base
			cpu.AddrRegs[1] = base
			cpu.AddrRegs[2] = base
			cpu.SR |= M68K_SR_C // pre-dirty C to expose a stale materialized carry
		},
		requireNativeRun: true,
	}
	runM68KJITDifferentialBlock(t, tc, 7)
}

func TestM68KJIT_Differential_ProductionMiscEffectiveAddressOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase

	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		cases = append(cases,
			m68kNativeDiffCase(fmt.Sprintf("TST_%s_A1", m68kDiffSizeName(size)), 0x4A00|uint16(size)<<6|uint16(M68K_AM_AR)<<3|1),
			m68kNativeDiffCase(fmt.Sprintf("TST_%s_d16A2", m68kDiffSizeName(size)), 0x4A00|uint16(size)<<6|uint16(M68K_AM_AR_DISP)<<3|2, 0x0010),
		)
	}

	for _, dst := range []uint16{0, 1, 2, 6} {
		for _, ea := range []struct {
			name string
			mode uint16
			reg  uint16
			ext  []uint16
		}{
			{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
			{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
			{name: "idxA2D0", mode: M68K_AM_AR_INDEX, reg: 2, ext: []uint16{0x0804}},
			{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
			{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
		} {
			opcode := uint16(0x41C0 | dst<<9 | ea.mode<<3 | ea.reg)
			words := append([]uint16{opcode}, ea.ext...)
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("LEA_%s_A%d", ea.name, dst), words...))
		}
	}

	for _, ea := range []struct {
		name string
		mode uint16
		reg  uint16
		ext  []uint16
	}{
		{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
		{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
		{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
		{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
	} {
		opcode := uint16(0x4840 | ea.mode<<3 | ea.reg)
		words := append([]uint16{opcode}, ea.ext...)
		cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("PEA_%s", ea.name), words...))
	}

	for i := range cases {
		cases[i].setup = m68kDiffEffectiveAddressSetup
		cases[i].watch = append(m68kDiffSimpleEAWatch(), m68kDiffMemWatch{addr: 0x0000FFFC, size: M68K_SIZE_LONG})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ProductionBitTestOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase
	for reg := uint16(0); reg < 8; reg++ {
		for _, bit := range []uint16{0, 1, 7, 8, 31, 32, 39, 255} {
			opcode := uint16(0x0800 | uint16(M68K_AM_DR)<<3 | reg)
			cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("BTST_%d_D%d", bit, reg), opcode, bit))
		}
	}
	for _, bit := range []uint16{0, 1, 7, 8, 15, 255} {
		opcode := uint16(0x0800 | uint16(M68K_AM_AR_DISP)<<3 | 2)
		cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("BTST_%d_d16A2", bit), opcode, bit, 0x0010))
	}

	for i := range cases {
		cases[i].setup = m68kDiffSimpleEASetup
		cases[i].watch = m68kDiffSimpleEAWatch()
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ProductionSpecialDataOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kSpecialDataDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_CHK2CMP2HelperOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kCHK2CMP2DiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialHelperSingle(t, tc, m68kJITHelperCHK2CMP2)
		})
	}
}

func TestM68KJIT_Differential_CASCAS2HelperOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kCASCAS2DiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialHelperSingle(t, tc, m68kJITHelperCASCAS2)
		})
	}
}

func TestM68KJIT_Differential_MOVESHelperOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kMOVESDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialHelperSingle(t, tc, m68kJITHelperMOVES)
		})
	}
}

func TestM68KJIT_Differential_TRAPccHelperOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kTRAPccDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialHelperSingle(t, tc, m68kJITHelperTRAPcc)
		})
	}
}

func TestM68KJIT_Differential_BKPTHelperOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kBKPTDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialHelperSingle(t, tc, m68kJITHelperBKPT)
		})
	}
}

func TestM68KJIT_Differential_CALLMHelperOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kCALLMDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialHelperSingle(t, tc, m68kJITHelperCALLM)
		})
	}
}

func TestM68KJIT_Differential_RTMHelperOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kRTMDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialHelperSingle(t, tc, m68kJITHelperRTM)
		})
	}
}

func TestM68KJIT_Differential_MOVECNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kMOVECDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_TASNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kTASDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_MOVEPNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kMOVEPDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_NBCDNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kNBCDDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ABCDSBCDNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kABCDSBCDDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_EXGNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kEXGDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_CHKNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kCHKDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_UnaryEANativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kUnaryEADiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_AddressGenerationNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kAddressGenerationDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_SccNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kSccDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ImmediateNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kImmediateDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ADDQSUBQNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kADDQSUBQDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ExtendedArithmeticNativeOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kExtendedArithmeticDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ADDCarryFeedsSUBXMask(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tc := m68kDiffCase{
		name: "ADD_L_D3_D3_then_SUBX_L_D3_D3",
		words: []uint16{
			0xD683, // ADD.L D3,D3
			0x9783, // SUBX.L D3,D3
		},
		setup: func(cpu *M68KCPU) {
			cpu.SR = M68K_SR_S | M68K_SR_Z
			cpu.DataRegs[3] = 0xFFFFFEEF
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}

	runM68KJITDifferentialBlock(t, tc, 2)
}

func m68kExtendedArithmeticDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase

	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "ADDX", base: 0xD100},
		{name: "SUBX", base: 0x9100},
	} {
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			for _, regs := range []struct {
				rx uint16
				ry uint16
			}{
				{rx: 0, ry: 1},
				{rx: 2, ry: 3},
				{rx: 7, ry: 7},
			} {
				for _, sr := range []uint16{M68K_SR_S | M68K_SR_Z, M68K_SR_S | M68K_SR_X | M68K_SR_Z | M68K_SR_C} {
					opcode := op.base | regs.ry<<9 | uint16(size)<<6 | regs.rx
					name := fmt.Sprintf("%s_%s_D%d_D%d_SR%02X", op.name, m68kDiffSizeName(size), regs.rx, regs.ry, sr&0x1F)
					cases = append(cases, m68kExtendedArithmeticDiffCase(name, sr, opcode))

					opcode = op.base | regs.ry<<9 | uint16(size)<<6 | 1<<3 | regs.rx
					name = fmt.Sprintf("%s_%s_predec_A%d_A%d_SR%02X", op.name, m68kDiffSizeName(size), regs.rx, regs.ry, sr&0x1F)
					cases = append(cases, m68kExtendedArithmeticDiffCase(name, sr, opcode))
				}
			}
		}
	}

	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		for _, regs := range []struct {
			rx uint16
			ry uint16
		}{
			{rx: 2, ry: 3},
			{rx: 3, ry: 2},
			{rx: 7, ry: 7},
		} {
			opcode := uint16(0xB108) | regs.ry<<9 | uint16(size)<<6 | regs.rx
			cases = append(cases, m68kExtendedArithmeticDiffCase(fmt.Sprintf("CMPM_%s_A%d_A%d", m68kDiffSizeName(size), regs.rx, regs.ry), M68K_SR_S|M68K_SR_X|M68K_SR_N|M68K_SR_Z, opcode))
		}
	}

	return cases
}

func m68kExtendedArithmeticDiffCase(name string, sr uint16, words ...uint16) m68kDiffCase {
	tc := m68kNativeDiffCase(name, words...)
	tc.setup = m68kDiffExtendedArithmeticSetup(sr)
	tc.watch = m68kDiffExtendedArithmeticWatch()
	return tc
}

func m68kADDQSUBQDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	eas := []struct {
		name string
		mode uint16
		reg  uint16
		ext  []uint16
	}{
		{name: "D0", mode: M68K_AM_DR, reg: 0},
		{name: "D1", mode: M68K_AM_DR, reg: 1},
		{name: "D7", mode: M68K_AM_DR, reg: 7},
		{name: "A0", mode: M68K_AM_AR, reg: 0},
		{name: "A2", mode: M68K_AM_AR, reg: 2},
		{name: "A7", mode: M68K_AM_AR, reg: 7},
		{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
		{name: "A2post", mode: M68K_AM_AR_POST, reg: 2},
		{name: "A2pre", mode: M68K_AM_AR_PRE, reg: 2},
		{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
		{name: "d8A2X", mode: M68K_AM_AR_INDEX, reg: 2, ext: []uint16{0x0004}},
		{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
		{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
	}
	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "ADDQ", base: 0x5000},
		{name: "SUBQ", base: 0x5100},
	} {
		for _, data := range []uint16{1, 3, 8} {
			encodedData := data
			if encodedData == 8 {
				encodedData = 0
			}
			for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
				for _, ea := range eas {
					opcode := op.base | encodedData<<9 | uint16(size)<<6 | ea.mode<<3 | ea.reg
					words := append([]uint16{opcode}, ea.ext...)
					cases = append(cases, m68kADDQSUBQDiffCase(fmt.Sprintf("%s_%d_%s_%s", op.name, data, m68kDiffSizeName(size), ea.name), words...))
				}
			}
		}
	}
	return cases
}

func m68kADDQSUBQDiffCase(name string, words ...uint16) m68kDiffCase {
	tc := m68kNativeDiffCase(name, words...)
	tc.setup = m68kDiffQuickSetup
	tc.watch = append(m68kDiffSimpleEAWatch(), m68kDiffMemWatch{addr: 0x3204, size: M68K_SIZE_LONG})
	return tc
}

func m68kImmediateDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase

	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "ORI", base: 0x0000},
		{name: "ANDI", base: 0x0200},
		{name: "SUBI", base: 0x0400},
		{name: "ADDI", base: 0x0600},
		{name: "EORI", base: 0x0A00},
		{name: "CMPI", base: 0x0C00},
	} {
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			for reg := uint16(0); reg < 8; reg++ {
				for _, imm := range m68kDiffRepresentativeImms(size) {
					opcode := op.base | uint16(size)<<6 | uint16(M68K_AM_DR)<<3 | reg
					words := append([]uint16{opcode}, m68kDiffImmWords(size, imm)...)
					cases = append(cases, m68kImmediateDiffCase(fmt.Sprintf("%s_%s_%X_D%d", op.name, m68kDiffSizeName(size), imm, reg), words...))
				}
			}
		}
	}

	writableEAs := []struct {
		name string
		mode uint16
		reg  uint16
		ext  []uint16
	}{
		{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
		{name: "A2post", mode: M68K_AM_AR_POST, reg: 2},
		{name: "A2pre", mode: M68K_AM_AR_PRE, reg: 2},
		{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
		{name: "d8A2X", mode: M68K_AM_AR_INDEX, reg: 2, ext: []uint16{0x0004}},
		{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
		{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
	}
	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "ORI", base: 0x0000},
		{name: "ANDI", base: 0x0200},
		{name: "SUBI", base: 0x0400},
		{name: "ADDI", base: 0x0600},
		{name: "EORI", base: 0x0A00},
		{name: "CMPI", base: 0x0C00},
	} {
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			imm := m68kDiffCompareImm(size)
			for _, ea := range writableEAs {
				opcode := op.base | uint16(size)<<6 | ea.mode<<3 | ea.reg
				words := append([]uint16{opcode}, m68kDiffImmWords(size, imm)...)
				words = append(words, ea.ext...)
				cases = append(cases, m68kImmediateDiffCase(fmt.Sprintf("%s_%s_%s", op.name, m68kDiffSizeName(size), ea.name), words...))
			}
			if op.name == "CMPI" {
				opcode := op.base | uint16(size)<<6 | 7<<3 | 3
				words := append([]uint16{opcode}, m68kDiffImmWords(size, imm)...)
				words = append(words, 0x0004)
				cases = append(cases, m68kImmediateDiffCase(fmt.Sprintf("CMPI_%s_pc8X", m68kDiffSizeName(size)), words...))
			}
		}
	}

	return cases
}

func m68kImmediateDiffCase(name string, words ...uint16) m68kDiffCase {
	tc := m68kNativeDiffCase(name, words...)
	tc.setup = m68kDiffImmediateSetup
	tc.watch = append(m68kDiffSimpleEAWatch(), m68kDiffMemWatch{addr: 0x3204, size: M68K_SIZE_LONG})
	return tc
}

func m68kSccDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	eas := []struct {
		name string
		mode uint16
		reg  uint16
		ext  []uint16
	}{
		{name: "D0", mode: M68K_AM_DR, reg: 0},
		{name: "D1", mode: M68K_AM_DR, reg: 1},
		{name: "D7", mode: M68K_AM_DR, reg: 7},
		{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
		{name: "A2post", mode: M68K_AM_AR_POST, reg: 2},
		{name: "A2pre", mode: M68K_AM_AR_PRE, reg: 2},
		{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
		{name: "d8A2X", mode: M68K_AM_AR_INDEX, reg: 2, ext: []uint16{0x0004}},
		{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
		{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
	}
	for cond := uint16(0); cond < 16; cond++ {
		for _, sr := range []uint16{M68K_SR_S, M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C} {
			for _, ea := range eas {
				opcode := uint16(0x50C0) | cond<<8 | ea.mode<<3 | ea.reg
				words := append([]uint16{opcode}, ea.ext...)
				name := fmt.Sprintf("Scc_%X_SR%02X_%s", cond, sr&0x1F, ea.name)
				cases = append(cases, m68kSccDiffCase(name, sr, words...))
			}
		}
	}
	return cases
}

func m68kSccDiffCase(name string, sr uint16, words ...uint16) m68kDiffCase {
	tc := m68kNativeDiffCase(name, words...)
	tc.setup = m68kDiffSccSetup(sr)
	tc.watch = append(m68kDiffSimpleEAWatch(), m68kDiffMemWatch{addr: 0x3204, size: M68K_SIZE_LONG})
	return tc
}

func m68kAddressGenerationDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	eas := []struct {
		name string
		mode uint16
		reg  uint16
		ext  []uint16
	}{
		{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
		{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
		{name: "d8A2X", mode: M68K_AM_AR_INDEX, reg: 2, ext: []uint16{0x0004}},
		{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
		{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
		{name: "pc16", mode: 7, reg: 2, ext: []uint16{0x0020}},
		{name: "pc8X", mode: 7, reg: 3, ext: []uint16{0x0004}},
	}
	for _, dst := range []uint16{0, 1, 2, 6} {
		for _, ea := range eas {
			words := append([]uint16{0x41C0 | dst<<9 | ea.mode<<3 | ea.reg}, ea.ext...)
			cases = append(cases, m68kAddressGenerationDiffCase(fmt.Sprintf("LEA_%s_A%d", ea.name, dst), words...))
		}
	}
	for _, ea := range eas {
		words := append([]uint16{0x4840 | ea.mode<<3 | ea.reg}, ea.ext...)
		cases = append(cases, m68kAddressGenerationDiffCase(fmt.Sprintf("PEA_%s", ea.name), words...))
	}
	return cases
}

func m68kAddressGenerationDiffCase(name string, words ...uint16) m68kDiffCase {
	tc := m68kNativeDiffCase(name, words...)
	tc.setup = m68kDiffEffectiveAddressSetup
	tc.watch = append(m68kDiffSimpleEAWatch(), m68kDiffMemWatch{addr: 0x0000FFFC, size: M68K_SIZE_LONG})
	return tc
}

func m68kUnaryEADiffCases() []m68kDiffCase {
	var cases []m68kDiffCase

	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "NEGX", base: 0x4000},
		{name: "CLR", base: 0x4200},
		{name: "NEG", base: 0x4400},
		{name: "NOT", base: 0x4600},
		{name: "TST", base: 0x4A00},
	} {
		for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
			for reg := uint16(0); reg < 8; reg++ {
				if op.name == "TST" {
					cases = append(cases, m68kUnaryEADiffCase(fmt.Sprintf("%s_%s_A%d", op.name, m68kDiffSizeName(size), reg), op.base|uint16(size)<<6|uint16(M68K_AM_AR)<<3|reg))
				}
				cases = append(cases, m68kUnaryEADiffCase(fmt.Sprintf("%s_%s_D%d", op.name, m68kDiffSizeName(size), reg), op.base|uint16(size)<<6|uint16(M68K_AM_DR)<<3|reg))
			}
			for _, ea := range []struct {
				name string
				mode uint16
				reg  uint16
				ext  []uint16
			}{
				{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
				{name: "A2post", mode: M68K_AM_AR_POST, reg: 2},
				{name: "A2pre", mode: M68K_AM_AR_PRE, reg: 2},
				{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
				{name: "d8A2X", mode: M68K_AM_AR_INDEX, reg: 2, ext: []uint16{0x0004}},
				{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
				{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
			} {
				words := append([]uint16{op.base | uint16(size)<<6 | ea.mode<<3 | ea.reg}, ea.ext...)
				cases = append(cases, m68kUnaryEADiffCase(fmt.Sprintf("%s_%s_%s", op.name, m68kDiffSizeName(size), ea.name), words...))
			}
			if op.name == "TST" {
				cases = append(cases,
					m68kUnaryEADiffCase(fmt.Sprintf("TST_%s_pc16", m68kDiffSizeName(size)), op.base|uint16(size)<<6|7<<3|2, 0x0020),
					m68kUnaryEADiffCase(fmt.Sprintf("TST_%s_pc8X", m68kDiffSizeName(size)), op.base|uint16(size)<<6|7<<3|3, 0x0004),
				)
			}
		}
	}

	return cases
}

func m68kUnaryEADiffCase(name string, words ...uint16) m68kDiffCase {
	tc := m68kNativeDiffCase(name, words...)
	tc.setup = m68kDiffUnaryEASetup
	tc.watch = m68kDiffSimpleEAWatch()
	return tc
}

func m68kCHKDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, size := range []struct {
		name string
		base uint16
	}{
		{name: "W", base: 0x4180},
		{name: "L", base: 0x4100},
	} {
		add := func(name string, opcode uint16, extra ...uint16) {
			words := []uint16{opcode}
			words = append(words, extra...)
			cases = append(cases, m68kDiffCase{
				name:             fmt.Sprintf("CHK_%s_%s", size.name, name),
				words:            words,
				setup:            m68kDiffCHKSetup,
				watch:            m68kDiffCHKWatch(),
				requireProdSafe:  true,
				requireNativeRun: true,
			})
		}
		add("D1_D0", size.base|uint16(M68K_AM_DR)<<3|1)
		add("A2ind_D0", size.base|uint16(M68K_AM_AR_IND)<<3|2)
		add("d16A2_D0", size.base|uint16(M68K_AM_AR_DISP)<<3|2, 0x0010)
		add("d8A2X_D0", size.base|uint16(M68K_AM_AR_INDEX)<<3|2, 0x0004)
		add("absW_D0", size.base|7<<3|0, 0x3400)
		add("absL_D0", size.base|7<<3|1, 0x0003, 0x6000)
		add("pc16_D0", size.base|7<<3|2, 0x0020)
	}
	return cases
}

func m68kEXGDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for rx := uint16(0); rx < 8; rx++ {
		for ry := uint16(0); ry < 8; ry++ {
			cases = append(cases, m68kDiffCase{
				name:             fmt.Sprintf("EXG_D%d_D%d", rx, ry),
				words:            []uint16{0xC140 | rx<<9 | ry},
				setup:            m68kDiffEXGSetup,
				requireProdSafe:  true,
				requireNativeRun: true,
			})
			cases = append(cases, m68kDiffCase{
				name:             fmt.Sprintf("EXG_A%d_A%d", rx, ry),
				words:            []uint16{0xC148 | rx<<9 | ry},
				setup:            m68kDiffEXGSetup,
				requireProdSafe:  true,
				requireNativeRun: true,
			})
			cases = append(cases, m68kDiffCase{
				name:             fmt.Sprintf("EXG_D%d_A%d", rx, ry),
				words:            []uint16{0xC188 | rx<<9 | ry},
				setup:            m68kDiffEXGSetup,
				requireProdSafe:  true,
				requireNativeRun: true,
			})
		}
	}
	return cases
}

func m68kABCDSBCDDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "ABCD", base: 0xC100},
		{name: "SBCD", base: 0x8100},
	} {
		for rx := uint16(0); rx < 8; rx++ {
			for ry := uint16(0); ry < 8; ry++ {
				cases = append(cases, m68kDiffCase{
					name:             fmt.Sprintf("%s_D%d_D%d", op.name, rx, ry),
					words:            []uint16{op.base | ry<<9 | rx},
					setup:            m68kDiffABCDSBCDSetup,
					watch:            m68kDiffABCDSBCDWatch(),
					requireProdSafe:  true,
					requireNativeRun: true,
				})
				cases = append(cases, m68kDiffCase{
					name:             fmt.Sprintf("%s_predec_A%d_A%d", op.name, rx, ry),
					words:            []uint16{op.base | ry<<9 | 0x0008 | rx},
					setup:            m68kDiffABCDSBCDSetup,
					watch:            m68kDiffABCDSBCDWatch(),
					requireProdSafe:  true,
					requireNativeRun: true,
				})
			}
		}
	}
	return cases
}

func m68kNBCDDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	add := func(name string, opcode uint16, extra ...uint16) {
		words := []uint16{opcode}
		words = append(words, extra...)
		cases = append(cases, m68kDiffCase{
			name:             name,
			words:            words,
			setup:            m68kDiffNBCDSetup,
			watch:            m68kDiffNBCDWatch(),
			requireProdSafe:  true,
			requireNativeRun: true,
		})
	}
	for _, reg := range []uint16{0, 1, 7} {
		add(fmt.Sprintf("NBCD_D%d", reg), 0x4800|reg)
	}
	add("NBCD_A2ind", 0x4812)
	add("NBCD_A2postinc", 0x481A)
	add("NBCD_A2predec", 0x4822)
	add("NBCD_d16A2", 0x482A, 0x0010)
	add("NBCD_d8A2X", 0x4832, 0x0004)
	add("NBCD_absW", 0x4838, 0x3400)
	add("NBCD_absL", 0x4839, 0x0003, 0x6000)
	return cases
}

func m68kMOVEPDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, size := range []struct {
		name  string
		read  uint16
		write uint16
	}{
		{name: "W", read: 4, write: 6},
		{name: "L", read: 5, write: 7},
	} {
		for _, dreg := range []uint16{0, 1, 7} {
			for _, areg := range []uint16{0, 2, 6} {
				for _, disp := range []uint16{0x0000, 0x0010, 0xFFF8} {
					readOpcode := uint16(0x0108) | dreg<<9 | size.read<<6 | areg
					writeOpcode := uint16(0x0108) | dreg<<9 | size.write<<6 | areg
					cases = append(cases, m68kDiffCase{
						name:             fmt.Sprintf("MOVEP_%s_d16A%d_D%d_%04X", size.name, areg, dreg, disp),
						words:            []uint16{readOpcode, disp},
						setup:            m68kDiffMOVEPSetup,
						watch:            m68kDiffMOVEPWatch(),
						requireProdSafe:  true,
						requireNativeRun: true,
					})
					cases = append(cases, m68kDiffCase{
						name:             fmt.Sprintf("MOVEP_%s_D%d_d16A%d_%04X", size.name, dreg, areg, disp),
						words:            []uint16{writeOpcode, disp},
						setup:            m68kDiffMOVEPSetup,
						watch:            m68kDiffMOVEPWatch(),
						requireProdSafe:  true,
						requireNativeRun: true,
					})
				}
			}
		}
	}
	return cases
}

func m68kTASDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	add := func(name string, opcode uint16, extra ...uint16) {
		words := []uint16{opcode}
		words = append(words, extra...)
		cases = append(cases, m68kDiffCase{
			name:             name,
			words:            words,
			setup:            m68kDiffTASSetup,
			watch:            m68kDiffTASWatch(),
			requireProdSafe:  true,
			requireNativeRun: true,
		})
	}
	add("TAS_D0", 0x4AC0)
	add("TAS_A2ind", 0x4AD2)
	add("TAS_A2postinc", 0x4ADA)
	add("TAS_A2predec", 0x4AE2)
	add("TAS_d16A2", 0x4AEA, 0x0010)
	add("TAS_d8A2X", 0x4AF2, 0x0004)
	add("TAS_absW", 0x4AF8, 0x3400)
	add("TAS_absL", 0x4AF9, 0x0003, 0x6000)
	return cases
}

func m68kMOVECDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, creg := range m68kMOVECControlRegs() {
		for _, reg := range []struct {
			name string
			num  uint16
		}{
			{name: "D1", num: 1},
			{name: "A2", num: 0xA},
		} {
			cases = append(cases, m68kDiffCase{
				name:             fmt.Sprintf("MOVEC_%s_to_%s", creg.name, reg.name),
				words:            []uint16{0x4E7A, reg.num<<12 | creg.code},
				setup:            m68kDiffMOVECSetup,
				watch:            m68kDiffMOVECWatch(),
				requireProdSafe:  true,
				requireNativeRun: true,
			})
			cases = append(cases, m68kDiffCase{
				name:             fmt.Sprintf("MOVEC_%s_to_%s", reg.name, creg.name),
				words:            []uint16{0x4E7B, reg.num<<12 | creg.code},
				setup:            m68kDiffMOVECSetup,
				watch:            m68kDiffMOVECWatch(),
				requireProdSafe:  true,
				requireNativeRun: true,
			})
		}
	}
	return cases
}

func m68kMOVECControlRegs() []struct {
	name string
	code uint16
} {
	return []struct {
		name string
		code uint16
	}{
		{name: "SFC", code: M68K_CR_SFC},
		{name: "DFC", code: M68K_CR_DFC},
		{name: "CACR", code: M68K_CR_CACR},
		{name: "CAAR", code: M68K_CR_CAAR},
		{name: "USP", code: M68K_CR_USP},
		{name: "VBR", code: M68K_CR_VBR},
		{name: "MSP", code: M68K_CR_MSP},
		{name: "ISP", code: M68K_CR_ISP},
	}
}

func m68kRTMDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for reg := uint16(0); reg < 8; reg++ {
		cases = append(cases, m68kDiffCase{
			name:            fmt.Sprintf("RTM_D%d", reg),
			words:           []uint16{0x06C0 | reg},
			setup:           m68kDiffRTMSetup,
			watch:           m68kDiffRTMWatch(),
			requireProdSafe: true,
		})
		cases = append(cases, m68kDiffCase{
			name:            fmt.Sprintf("RTM_A%d", reg),
			words:           []uint16{0x06C8 | reg},
			setup:           m68kDiffRTMSetup,
			watch:           m68kDiffRTMWatch(),
			requireProdSafe: true,
		})
	}
	return cases
}

func m68kCALLMDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, ea := range m68kInventoryMemoryEAForms() {
		opcode := uint16(0x06C0) | ea.mode<<3 | ea.reg
		words := []uint16{opcode, 0x0003}
		words = append(words, ea.ext...)
		cases = append(cases, m68kDiffCase{
			name:            fmt.Sprintf("CALLM_%s", ea.name),
			words:           words,
			setup:           m68kDiffCALLMSetup,
			watch:           m68kDiffCALLMWatch(),
			requireProdSafe: true,
		})
	}
	return cases
}

func m68kBKPTDiffCases() []m68kDiffCase {
	cases := make([]m68kDiffCase, 0, 8)
	for n := uint16(0); n < 8; n++ {
		cases = append(cases, m68kDiffCase{
			name:            fmt.Sprintf("BKPT_%d", n),
			words:           []uint16{0x4848 | n},
			setup:           m68kDiffBKPTSetup,
			watch:           m68kDiffBKPTWatch(),
			requireProdSafe: true,
		})
	}
	return cases
}

func m68kTRAPccDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, cond := range []struct {
		name string
		bits uint16
		sr   uint16
	}{
		{name: "T_taken", bits: 0x0, sr: 0},
		{name: "F_not_taken", bits: 0x1, sr: 0},
		{name: "EQ_taken", bits: 0x7, sr: M68K_SR_Z},
		{name: "NE_not_taken", bits: 0x6, sr: M68K_SR_Z},
	} {
		for _, form := range []struct {
			name  string
			reg   uint16
			extra []uint16
		}{
			{name: "W", reg: 2, extra: []uint16{0x1234}},
			{name: "L", reg: 3, extra: []uint16{0x1234, 0x5678}},
			{name: "none", reg: 4},
		} {
			opcode := uint16(0x50F8) | cond.bits<<8 | form.reg
			words := []uint16{opcode}
			words = append(words, form.extra...)
			cases = append(cases, m68kDiffCase{
				name:            fmt.Sprintf("TRAP%s_%s", cond.name, form.name),
				words:           words,
				setup:           m68kDiffTRAPccSetup(cond.sr),
				watch:           m68kDiffTRAPccWatch(),
				requireProdSafe: true,
			})
		}
	}
	return cases
}

func m68kMOVESDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, size := range []struct {
		name string
		bits uint16
	}{
		{name: "B", bits: 0 << 6},
		{name: "W", bits: 1 << 6},
		{name: "L", bits: 2 << 6},
	} {
		for _, direction := range []struct {
			name string
			ext  uint16
		}{
			{name: "mem_to_D1", ext: 0x1000},
			{name: "D1_to_mem", ext: 0x1800},
		} {
			for _, ea := range m68kInventoryMemoryEAForms() {
				opcode := uint16(0x0E00) | size.bits | ea.mode<<3 | ea.reg
				words := []uint16{opcode, direction.ext}
				words = append(words, ea.ext...)
				cases = append(cases, m68kDiffCase{
					name:            fmt.Sprintf("MOVES_%s_%s_%s", size.name, direction.name, ea.name),
					words:           words,
					setup:           m68kDiffMOVESSetup,
					watch:           m68kDiffMOVESWatch(),
					requireProdSafe: true,
				})
			}
		}
	}
	return cases
}

func m68kCASCAS2DiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, size := range []struct {
		name string
		base uint16
	}{
		{name: "B", base: 0x0AC0},
		{name: "W", base: 0x0CC0},
		{name: "L", base: 0x0EC0},
	} {
		for _, ea := range m68kInventoryMemoryEAForms() {
			opcode := size.base | ea.mode<<3 | ea.reg
			words := []uint16{opcode, 0x0040}
			words = append(words, ea.ext...)
			cases = append(cases, m68kDiffCase{
				name:            fmt.Sprintf("CAS_%s_%s", size.name, ea.name),
				words:           words,
				setup:           m68kDiffCASCAS2Setup,
				watch:           m68kDiffCASCAS2Watch(),
				requireProdSafe: true,
			})
		}
	}
	cases = append(cases,
		m68kDiffCase{
			name:            "CAS2_W_generated",
			words:           []uint16{0x0CFC, 0xA040, 0xB082},
			setup:           m68kDiffCASCAS2Setup,
			watch:           m68kDiffCASCAS2Watch(),
			requireProdSafe: true,
		},
		m68kDiffCase{
			name:            "CAS2_L_generated",
			words:           []uint16{0x0EFC, 0xA040, 0xB082},
			setup:           m68kDiffCASCAS2Setup,
			watch:           m68kDiffCASCAS2Watch(),
			requireProdSafe: true,
		},
	)
	return cases
}

func m68kCHK2CMP2DiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, size := range []struct {
		name string
		base uint16
	}{
		{name: "B", base: 0x00C0},
		{name: "W", base: 0x02C0},
		{name: "L", base: 0x04C0},
	} {
		for _, kind := range []struct {
			name string
			ext  uint16
		}{
			{name: "CMP2", ext: 0x0000},
			{name: "CHK2", ext: 0x0800},
		} {
			for _, ea := range m68kInventoryMemoryEAForms() {
				opcode := size.base | ea.mode<<3 | ea.reg
				words := []uint16{opcode, kind.ext}
				words = append(words, ea.ext...)
				tc := m68kDiffCase{
					name:            fmt.Sprintf("%s_%s_%s", kind.name, size.name, ea.name),
					words:           words,
					setup:           m68kDiffCHK2CMP2Setup,
					watch:           m68kDiffCHK2CMP2Watch(),
					requireProdSafe: true,
				}
				cases = append(cases, tc)
			}
		}
	}
	return cases
}

func m68kSpecialDataDiffCases() []m68kDiffCase {
	cases := []m68kDiffCase{
		m68kNativeDiffCase("MOVEP_W_d16A2_D0", 0x010A, 0x0010),
		m68kNativeDiffCase("MOVEP_L_d16A2_D1", 0x034A, 0x0010),
		m68kNativeDiffCase("MOVEP_W_D0_d16A2", 0x018A, 0x0010),
		m68kNativeDiffCase("MOVEP_L_D1_d16A2", 0x03CA, 0x0010),
		m68kNativeDiffCase("CHK_W_D0_D1_in_range", 0x4380),
		m68kNativeDiffCase("CHK_L_D0_D1_in_range", 0x4300),
		m68kNativeDiffCase("EXG_D0_D1", 0xC141),
		m68kNativeDiffCase("EXG_A0_A1", 0xC149),
		m68kNativeDiffCase("EXG_D0_A1", 0xC189),
		m68kNativeDiffCase("ABCD_D0_D1", 0xC300),
		m68kNativeDiffCase("SBCD_D0_D1", 0x8300),
		m68kNativeDiffCase("ABCD_predec_A0_A1", 0xC308),
		m68kNativeDiffCase("SBCD_predec_A0_A1", 0x8308),
		m68kNativeDiffCase("NBCD_D0", 0x4800),
		m68kNativeDiffCase("NBCD_A2ind", 0x4812),
		m68kNativeDiffCase("TAS_D0", 0x4AC0),
		m68kNativeDiffCase("TAS_A2ind", 0x4AD2),
	}
	cases = append(cases, m68kPACKUNPKDiffCases()...)

	for i := range cases {
		cases[i].setup = m68kDiffSpecialDataSetup
		cases[i].watch = m68kDiffSpecialDataWatch()
	}
	return cases
}

func m68kPACKUNPKDiffCases() []m68kDiffCase {
	var cases []m68kDiffCase
	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "PACK", base: 0x8140},
		{name: "UNPK", base: 0x8180},
	} {
		for _, mode := range []struct {
			name string
			bit  uint16
		}{
			{name: "Dn_Dn", bit: 0},
			{name: "predec", bit: 1 << 3},
		} {
			for rx := uint16(0); rx < 8; rx++ {
				for ry := uint16(0); ry < 8; ry++ {
					adjustment := uint16(0)
					if (rx+ry)&1 != 0 {
						adjustment = 0x0009
					}
					opcode := op.base | mode.bit | rx | ry<<9
					cases = append(cases, m68kNativeDiffCase(
						fmt.Sprintf("%s_%s_rx%d_ry%d_adj%X", op.name, mode.name, rx, ry, adjustment),
						opcode, adjustment,
					))
				}
			}
		}
	}
	return cases
}

func TestM68KJIT_Differential_ProductionBitFieldOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kBitFieldDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func m68kBitFieldDiffCases() []m68kDiffCase {
	type bitFieldOp struct {
		name       string
		base       uint16
		registerOK bool
		memoryOK   bool
	}
	ops := []bitFieldOp{
		{name: "BFTST", base: 0xE8C0, registerOK: true, memoryOK: true},
		{name: "BFEXTU", base: 0xE9C0, registerOK: true, memoryOK: true},
		{name: "BFEXTS", base: 0xEBC0, registerOK: true, memoryOK: true},
		{name: "BFCHG", base: 0xEAC0, registerOK: true, memoryOK: true},
		{name: "BFCLR", base: 0xECC0, registerOK: true, memoryOK: true},
		{name: "BFFFO", base: 0xEDC0, registerOK: true, memoryOK: true},
		{name: "BFSET", base: 0xEEC0, registerOK: true, memoryOK: true},
		{name: "BFINS", base: 0xEFC0, registerOK: true, memoryOK: true},
	}
	fields := []struct {
		name string
		ext  uint16
	}{
		{name: "off0_width1_D1", ext: 0x1001},
		{name: "off0_width8_D1", ext: 0x1008},
		{name: "off4_width12_D2", ext: 0x210C},
		{name: "off7_width16_D3", ext: 0x31D0},
		{name: "off0_width32_D4", ext: 0x4000},
	}
	memEAs := []struct {
		name string
		mode uint16
		reg  uint16
		ext  []uint16
	}{
		{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
		{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
		{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
		{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
	}

	var cases []m68kDiffCase
	for _, op := range ops {
		if op.registerOK {
			for reg := uint16(0); reg < 8; reg++ {
				for _, field := range fields {
					opcode := op.base | uint16(M68K_AM_DR)<<3 | reg
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_D%d_%s", op.name, reg, field.name), opcode, field.ext))
				}
			}
		}
		if op.memoryOK {
			for _, ea := range memEAs {
				for _, field := range fields {
					opcode := op.base | ea.mode<<3 | ea.reg
					words := []uint16{opcode, field.ext}
					words = append(words, ea.ext...)
					cases = append(cases, m68kNativeDiffCase(fmt.Sprintf("%s_%s_%s", op.name, ea.name, field.name), words...))
				}
			}
		}
	}

	for i := range cases {
		cases[i].setup = m68kDiffBitFieldSetup
		cases[i].watch = m68kDiffBitFieldWatch()
	}
	return cases
}

func TestM68KJIT_Differential_ProductionStackFrameOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase
	cases = append(cases, m68kNativeDiffCase("CLR_L_predec_A7", 0x42A7))
	for reg := uint16(0); reg < 7; reg++ {
		cases = append(cases,
			m68kNativeDiffCase(fmt.Sprintf("LINK_W_A%d_neg16", reg), 0x4E50|reg, 0xFFF0),
			m68kNativeDiffCase(fmt.Sprintf("LINK_W_A%d_pos8", reg), 0x4E50|reg, 0x0008),
			m68kNativeDiffCase(fmt.Sprintf("UNLK_A%d", reg), 0x4E58|reg),
		)
	}

	for i := range cases {
		cases[i].setup = m68kDiffStackFrameSetup
		cases[i].watch = m68kDiffStackFrameWatch()
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_RTEFormat0UserReturnSwapsStacks(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		frameSP = uint32(0x2000)
		userSP  = uint32(0x3000)
		retPC   = uint32(0x1234)
	)

	setup := func(cpu *M68KCPU) {
		cpu.PC = startPC
		cpu.SR = M68K_SR_S | M68K_SR_C | M68K_SR_V
		cpu.inException.Store(true)
		cpu.AddrRegs[7] = frameSP
		cpu.USP = userSP
		cpu.SSP = 0
		cpu.Write16(frameSP, M68K_SR_X|M68K_SR_Z)
		cpu.Write32(frameSP+2, retPC)
		cpu.Write16(frameSP+6, 0)
	}

	interp := newM68KDiffTestProgramCPU(t, startPC)
	setup(interp)
	m68kDiffWriteProgram(interp, startPC, 0x4E73)
	if cycles := interp.StepOne(); cycles == 0 {
		t.Fatal("interpreter did not execute RTE")
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	setup(jit)
	m68kDiffWriteProgram(jit, startPC, 0x4E73)

	instrs := m68kScanBlock(jit.memory, startPC)
	if len(instrs) != 1 || instrs[0].opcode != 0x4E73 {
		t.Fatalf("decoded RTE block = %d instructions, first=%04X", len(instrs), instrs[0].opcode)
	}
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatal("RTE format-0 block is not admitted by production-native gate")
	}

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, startPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.USPPtr = uintptr(unsafe.Pointer(&jit.USP))
	rig.ctx.SSPPtr = uintptr(unsafe.Pointer(&jit.SSP))
	rig.ctx.InExceptionPtr = uintptr(unsafe.Pointer(&jit.inException))
	rig.ctx.RTECountPtr = uintptr(unsafe.Pointer(&jit.rteCount))
	rig.ctx.RetPC = 0
	rig.ctx.RetCount = 0
	rig.ctx.NeedIOFallback = 0
	rig.ctx.Use68000Frame = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	if rig.ctx.NeedIOFallback != 0 {
		t.Fatal("native RTE requested interpreter fallback for format-0 user return")
	}
	jit.PC = rig.ctx.RetPC

	assertM68KCoreStateEqual(t, jit, interp)
	if jit.USP != interp.USP || jit.SSP != interp.SSP {
		t.Fatalf("stack pointer store mismatch: USP=%08X/%08X SSP=%08X/%08X",
			jit.USP, interp.USP, jit.SSP, interp.SSP)
	}
	if got, want := rig.ctx.RetCount, uint32(1); got != want {
		t.Fatalf("RetCount=%d, want %d", got, want)
	}
	if got, want := jit.rteCount.Load(), interp.rteCount.Load(); got != want {
		t.Fatalf("rteCount=%d, want %d", got, want)
	}
	if jit.inException.Load() {
		t.Fatal("native RTE left inException set")
	}
}

func TestM68KJIT_Differential_ProductionMOVEMOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kMOVEMDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func m68kMOVEMDiffCases() []m68kDiffCase {
	return []m68kDiffCase{
		{
			name:             "MOVEM_L_D0D1A0_predec_A7",
			words:            []uint16{0x48E7, 0xC080},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_A7_in_mask_predec_A7",
			words:            []uint16{0x48E7, 0x0081},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_A2A3A4A6_predec_A7",
			words:            []uint16{0x48E7, 0x003A},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_D2D3D4A2A3_predec_A7",
			words:            []uint16{0x48E7, 0x3830},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_postinc_A7_D0D1A0",
			words:            []uint16{0x4CDF, 0x0103},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_postinc_A7_D2D3D4A6",
			words:            []uint16{0x4CDF, 0x401C},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_W_postinc_A7_D0A0_signextend",
			words:            []uint16{0x4C9F, 0x0101},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_D1A2_store_A3",
			words:            []uint16{0x48D3, 0x0202},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_D1D7A2A6_store_A1",
			words:            []uint16{0x48D1, 0x7CFE},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_postinc_A0_D1D7A2A6",
			words:            []uint16{0x4CD8, 0x7CFE},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffMOVEMSetup,
			watch:            m68kDiffMOVEMWatch(),
		},
		{
			name:             "MOVEM_L_d16A5_D2D3D4D5D6A2A3A4_restore",
			words:            []uint16{0x4CED, 0x1C7C, 0xFFE0},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup: func(cpu *M68KCPU) {
				m68kDiffMOVEMSetup(cpu)
				cpu.AddrRegs[5] = 0x4000
				values := []uint32{
					0xD2000002, 0xD3000003, 0xD4000004, 0xD5000005,
					0xD6000006, 0xA2000002, 0xA3000003, 0xA4000004,
				}
				for i, v := range values {
					cpu.Write32(0x3FE0+uint32(i*4), v)
				}
			},
			watch: append(m68kDiffMOVEMWatch(),
				m68kDiffMemWatch{addr: 0x3FE0, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FE4, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FE8, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FEC, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FF0, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FF4, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FF8, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FFC, size: M68K_SIZE_LONG},
			),
		},
		{
			name:             "MOVEM_L_d16A5_D2A2A3A4A6_restore",
			words:            []uint16{0x4CED, 0x5C04, 0xFFE0},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup: func(cpu *M68KCPU) {
				m68kDiffMOVEMSetup(cpu)
				cpu.AddrRegs[5] = 0x4000
				values := []uint32{
					0xD2000002, 0xA2000002, 0xA3000003, 0xA4000004, 0xA6000006,
				}
				for i, v := range values {
					cpu.Write32(0x3FE0+uint32(i*4), v)
				}
			},
			watch: append(m68kDiffMOVEMWatch(),
				m68kDiffMemWatch{addr: 0x3FE0, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FE4, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FE8, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FEC, size: M68K_SIZE_LONG},
				m68kDiffMemWatch{addr: 0x3FF0, size: M68K_SIZE_LONG},
			),
		},
	}
}

func TestM68KJIT_Differential_ProductionIndexedDestinationMoveOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cases := []m68kDiffCase{}
	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD} {
		cases = append(cases, m68kNativeDiffCase(
			fmt.Sprintf("MOVE_%s_D1_idxA3D0", m68kDiffSizeName(size)),
			m68kDiffMoveOpcode(size, M68K_AM_DR, 1, M68K_AM_AR_INDEX, 3), 0x0804,
		))
	}
	cases = append(cases, m68kDiffCase{
		name:             "MOVE_B_idxA0A1L_idxA2A1L_negative",
		words:            []uint16{m68kDiffMoveOpcode(M68K_SIZE_BYTE, M68K_AM_AR_INDEX, 0, M68K_AM_AR_INDEX, 2), 0x9800, 0x9800},
		requireProdSafe:  false,
		requireNativeRun: true,
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = 0x7000
			cpu.AddrRegs[1] = 0xFFFFFFFF
			cpu.AddrRegs[2] = 0x7100
			cpu.Write8(0x6FFF, 0xA7)
			cpu.Write8(0x70FF, 0x11)
			cpu.Write8(0x7000, 0x22)
			cpu.Write8(0x7100, 0x33)
		},
		watch: []m68kDiffMemWatch{
			{addr: 0x6FFF, size: M68K_SIZE_BYTE},
			{addr: 0x70FF, size: M68K_SIZE_BYTE},
			{addr: 0x7100, size: M68K_SIZE_BYTE},
		},
	})

	for i := range cases {
		if cases[i].setup == nil {
			cases[i].setup = m68kDiffEffectiveAddressSetup
		}
		if cases[i].watch == nil {
			cases[i].watch = append(m68kDiffSimpleEAWatch(), m68kDiffMemWatch{addr: 0x3308, size: M68K_SIZE_LONG})
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ForceNativeMovePredecrementDestinations(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase
	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		for _, dstReg := range []uint16{3, 7} {
			opcode := m68kDiffMoveOpcode(size, M68K_AM_DR, 0, M68K_AM_AR_PRE, dstReg)
			cases = append(cases, m68kDiffCase{
				name:             fmt.Sprintf("MOVE_%s_D0_predec_A%d", m68kDiffSizeName(size), dstReg),
				words:            []uint16{opcode},
				requireProdSafe:  false,
				requireNativeRun: true,
			})

			immWords := m68kDiffImmWords(size, m68kDiffCompareImm(size))
			words := append([]uint16{m68kDiffMoveOpcode(size, 7, 4, M68K_AM_AR_PRE, dstReg)}, immWords...)
			cases = append(cases, m68kDiffCase{
				name:             fmt.Sprintf("MOVE_%s_imm_predec_A%d", m68kDiffSizeName(size), dstReg),
				words:            words,
				requireProdSafe:  false,
				requireNativeRun: true,
			})
		}
	}

	for i := range cases {
		cases[i].setup = m68kDiffPredecrementMoveSetup
		cases[i].watch = m68kDiffPredecrementMoveWatch()
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ProductionMoveMemoryToMemoryPostincrement(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase
	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		cases = append(cases, m68kDiffCase{
			name:             fmt.Sprintf("MOVE_%s_A2post_A3post", m68kDiffSizeName(size)),
			words:            []uint16{m68kDiffMoveOpcode(size, M68K_AM_AR_POST, 2, M68K_AM_AR_POST, 3)},
			requireProdSafe:  true,
			requireNativeRun: true,
			setup:            m68kDiffPostincrementMemToMemSetup,
			watch:            m68kDiffPostincrementMemToMemWatch(),
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ProductionImmediateShiftOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "ASR", base: 0xE000},
		{name: "LSR", base: 0xE008},
		{name: "ASL", base: 0xE100},
		{name: "LSL", base: 0xE108},
	} {
		sizes := []int{M68K_SIZE_LONG}
		if op.name == "LSR" || op.name == "LSL" {
			sizes = []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG}
		}
		for _, value := range []uint32{0x00000001, 0x40000000, 0x80000001, 0xFFFFFFFF} {
			for _, count := range []uint16{1, 2, 8} {
				for _, size := range sizes {
					for _, reg := range []uint16{0, 7} {
						encodedCount := count
						if encodedCount == 8 {
							encodedCount = 0
						}
						opcode := op.base | encodedCount<<9 | uint16(size)<<6 | reg
						tc := m68kNativeDiffCase(fmt.Sprintf("%s_%s_%d_D%d_%08X", op.name, m68kDiffSizeName(size), count, reg, value), opcode)
						tc.requireProdSafe = true
						tc.requireNativeRun = true
						tc.setup = func(cpu *M68KCPU) {
							cpu.DataRegs[reg] = value
							cpu.DataRegs[2] = uint32(count)
						}
						t.Run(tc.name, func(t *testing.T) {
							runM68KJITDifferentialSingle(t, tc)
						})
					}
				}
			}
		}
	}
}

// TestM68KJIT_Differential_ProductionRegisterCountLongShiftOpcodes verifies
// that register-count long shifts (ASR/LSR/ASL/LSL.L with the shift count in a
// data register) are admitted as production-native and produce state identical
// to the interpreter for a representative value/count matrix. Mirrors
// TestM68KJIT_Differential_ProductionImmediateShiftOpcodes.
func TestM68KJIT_Differential_ProductionRegisterCountLongShiftOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, op := range []struct {
		name string
		base uint16
	}{
		{name: "ASR", base: 0xE000},
		{name: "LSR", base: 0xE008},
		{name: "ASL", base: 0xE100},
		{name: "LSL", base: 0xE108},
	} {
		for _, value := range []uint32{0x00000001, 0x40000000, 0x80000001, 0xFFFFFFFF} {
			for _, count := range []uint16{0, 1, 2, 33} {
				value := value
				count := count
				// Shift register D2, data register D0, register-count form (bit 5 set).
				opcode := op.base | 2<<9 | 1<<5 | uint16(M68K_SIZE_LONG)<<6
				tc := m68kNativeDiffCase(fmt.Sprintf("%s_L_D2_D0_%08X_count%d", op.name, value, count), opcode)
				tc.requireProdSafe = true
				tc.requireNativeRun = true
				tc.setup = func(cpu *M68KCPU) {
					cpu.DataRegs[0] = value
					cpu.DataRegs[2] = uint32(count)
				}
				t.Run(tc.name, func(t *testing.T) {
					runM68KJITDifferentialSingle(t, tc)
				})
			}
		}
	}
}

func TestM68KJIT_Differential_ProductionSccOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	var cases []m68kDiffCase
	for _, sr := range []uint16{
		0,
		M68K_SR_C,
		M68K_SR_Z,
		M68K_SR_N,
		M68K_SR_V,
		M68K_SR_N | M68K_SR_V,
		M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C,
	} {
		for cond := uint16(0); cond < 16; cond++ {
			for _, reg := range []uint16{0, 7} {
				opcode := uint16(0x50C0 | cond<<8 | uint16(M68K_AM_DR)<<3 | reg)
				tc := m68kNativeDiffCase(fmt.Sprintf("Scc_%X_D%d_SR%02X", cond, reg, sr&0x1F), opcode)
				tc.setup = m68kDiffSetupSR(sr)
				cases = append(cases, tc)
			}
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_SccMemoryDestinationExecutesNative(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	words := []uint16{
		0x176A, 0x0127, 0x0011, // MOVE.B 295(A2),17(A3)
		0x50EA, 0x0127, // ST 295(A2)
	}

	interp := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(interp)
	interp.AddrRegs[2] = 0x8000
	interp.AddrRegs[3] = 0x9000
	interp.Write8(0x8000+0x0127, 0x80)
	interp.Write8(0x9000+0x0011, 0x22)
	m68kDiffWriteProgram(interp, m68kDiffStartPC, words...)
	// Oracle: step the interpreter through both the leading MOVE.B and the
	// Scc-to-memory (ST) so we can compare both destination bytes.
	if cycles := interp.StepOne(); cycles == 0 {
		t.Fatal("interpreter did not execute leading MOVE.B")
	}
	if cycles := interp.StepOne(); cycles == 0 {
		t.Fatal("interpreter did not execute Scc-to-memory (ST)")
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	jit.AddrRegs[2] = 0x8000
	jit.AddrRegs[3] = 0x9000
	jit.Write8(0x8000+0x0127, 0x80)
	jit.Write8(0x9000+0x0011, 0x22)
	m68kDiffWriteProgram(jit, m68kDiffStartPC, words...)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	if len(instrs) < 2 {
		t.Fatalf("decoded %d instructions, want at least 2", len(instrs))
	}
	instrs = instrs[:2]

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	// Scc-to-memory (ST d16(A2)) is now compiled natively: the whole two-
	// instruction block runs without an interpreter bailout.
	if rig.ctx.NeedIOFallback != 0 {
		t.Fatalf("Scc memory destination took IO fallback %d times; want native execution", rig.ctx.NeedIOFallback)
	}
	// Leading MOVE.B destination byte must match the interpreter oracle.
	if got, want := jit.Read8(0x9000+0x0011), interp.Read8(0x9000+0x0011); got != want {
		t.Fatalf("leading MOVE.B destination=0x%02X want 0x%02X", got, want)
	}
	// ST sets the destination byte to 0xFF (true); compare against the oracle.
	if got, want := jit.Read8(0x8000+0x0127), interp.Read8(0x8000+0x0127); got != want {
		t.Fatalf("Scc destination=0x%02X want 0x%02X", got, want)
	}
}

func assertM68KJITOpcodeFallsBackAtInstruction(t *testing.T, opcode uint16, setup func(*M68KCPU)) {
	t.Helper()

	want := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(want)
	if setup != nil {
		setup(want)
	}
	m68kDiffWriteProgram(want, m68kDiffStartPC, opcode)

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	if setup != nil {
		setup(jit)
	}
	m68kDiffWriteProgram(jit, m68kDiffStartPC, opcode)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	if len(instrs) == 0 {
		t.Fatal("m68kScanBlock returned no instructions")
	}
	instrs = instrs[:1]
	if m68kInstrProductionNativeSafe(&instrs[0]) {
		t.Fatalf("opcode 0x%04X is marked production-native safe; test expects fallback", opcode)
	}

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.RetPC = 0
	rig.ctx.RetCount = 0xFFFF
	rig.ctx.NeedIOFallback = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	if rig.ctx.NeedIOFallback == 0 {
		t.Fatalf("opcode 0x%04X silently executed natively; want fallback", opcode)
	}
	if got := rig.ctx.RetPC; got != m68kDiffStartPC {
		t.Fatalf("fallback RetPC=0x%08X want instruction PC 0x%08X", got, m68kDiffStartPC)
	}
	if got := rig.ctx.RetCount; got != 0 {
		t.Fatalf("fallback RetCount=%d want 0", got)
	}
	jit.PC = rig.ctx.RetPC
	assertM68KCoreStateEqual(t, jit, want)
}

func TestM68KJIT_Differential_ProductionDBFOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cases := []m68kDiffCase{}
	for _, reg := range []uint16{0, 7} {
		for _, value := range []uint32{0x00000002, 0x00000000} {
			tc := m68kNativeDiffCase(fmt.Sprintf("DBF_D%d_%04X", reg, value&0xFFFF), 0x51C8|reg, 0x0004)
			tc.setup = m68kDiffSetupDataReg(reg, value)
			cases = append(cases, tc)
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingle(t, tc)
		})
	}
}

func TestM68KJIT_Differential_ProductionBranchOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range []m68kDiffCase{
		func() m68kDiffCase {
			tc := m68kNativeDiffCase("BRA_S_forward", 0x6002)
			tc.requireProdSafe = true
			tc.requireNativeRun = true
			return tc
		}(),
		func() m68kDiffCase {
			tc := m68kNativeDiffCase("BRA_W_forward", 0x6000, 0x0004)
			tc.requireProdSafe = true
			tc.requireNativeRun = true
			return tc
		}(),
	} {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialSingleInstruction(t, tc)
		})
	}

	for _, sr := range []uint16{
		0,
		M68K_SR_C,
		M68K_SR_Z,
		M68K_SR_N,
		M68K_SR_V,
		M68K_SR_N | M68K_SR_V,
		M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C,
	} {
		for cond := uint16(2); cond < 16; cond++ {
			tc := m68kNativeDiffCase(fmt.Sprintf("Bcc_S_%X_SR%02X", cond, sr&0x1F), 0x6002|cond<<8)
			tc.requireProdSafe = true
			tc.requireNativeRun = true
			tc.setup = m68kDiffSetupSR(sr)
			t.Run(tc.name, func(t *testing.T) {
				runM68KJITDifferentialSingleInstruction(t, tc)
			})

			tc = m68kNativeDiffCase(fmt.Sprintf("Bcc_W_%X_SR%02X", cond, sr&0x1F), 0x6000|cond<<8, 0x0004)
			tc.requireProdSafe = true
			tc.requireNativeRun = true
			tc.setup = m68kDiffSetupSR(sr)
			t.Run(tc.name, func(t *testing.T) {
				runM68KJITDifferentialSingleInstruction(t, tc)
			})
		}
	}
}

func TestM68KJIT_Differential_ProductionAROSReverseCopyTail(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range []struct {
		name string
		a1   uint32
		d3   uint32
	}{
		{name: "not_done_high", a1: 0xFFFFFFFF, d3: 0xFFFFFFFC},
		{name: "done_high", a1: 0xFFFFFFFD, d3: 0xFFFFFFFC},
		{name: "wrap_to_zero", a1: 0x00000000, d3: 0xFFFFFFFF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialBlock(t, m68kDiffCase{
				name:             tc.name,
				words:            []uint16{0x5389, 0xB689}, // SUBQ.L #1,A1; CMP.L A1,D3
				requireProdSafe:  true,
				requireNativeRun: true,
				setup: func(cpu *M68KCPU) {
					cpu.AddrRegs[1] = tc.a1
					cpu.DataRegs[3] = tc.d3
				},
			}, 2)
		})
	}
}

func TestM68KJIT_Differential_AROSReverseIndexedByteCopyTailBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range []struct {
		name string
		a0   uint32
		a2   uint32
		a1   uint32
		d3   uint32
	}{
		{name: "single", a0: 0x7000, a2: 0x7100, a1: 0xFFFFFFFF, d3: 0xFFFFFFFE},
		{name: "larger_nonoverlap", a0: 0x8000, a2: 0x9000, a1: 0xFFFFFFFF, d3: 0xFFFFFFF0},
		{name: "larger_overlap_dst_above_src", a0: 0x8000, a2: 0x8008, a1: 0xFFFFFFFF, d3: 0xFFFFFFF0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			watch := []m68kDiffMemWatch{}
			for off := uint32(1); off <= 16; off++ {
				watch = append(watch,
					m68kDiffMemWatch{addr: tc.a0 - off, size: M68K_SIZE_BYTE},
					m68kDiffMemWatch{addr: tc.a2 - off, size: M68K_SIZE_BYTE},
				)
			}
			runM68KJITDifferentialBlockDynamicRetire(t, m68kDiffCase{
				name:             tc.name,
				words:            []uint16{0x15B0, 0x9800, 0x9800, 0x5389, 0xB689, 0x66F4},
				requireProdSafe:  false,
				requireNativeRun: true,
				setup: func(cpu *M68KCPU) {
					cpu.AddrRegs[0] = tc.a0
					cpu.AddrRegs[1] = tc.a1
					cpu.AddrRegs[2] = tc.a2
					cpu.DataRegs[3] = tc.d3
					for off := uint32(1); off <= 16; off++ {
						cpu.Write8(tc.a0-off, byte(0xA0+off))
						cpu.Write8(tc.a2-off, byte(0x10+off))
					}
				},
				watch: watch,
			}, 4)
		})
	}
}

func TestM68KJIT_Differential_ProductionAROSStackUnwindPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range []struct {
		name string
		d0   uint32
	}{
		{name: "move_zero_then_lea_a7", d0: 0},
		{name: "move_nonzero_then_lea_a7", d0: 0x80010000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialBlock(t, m68kDiffCase{
				name:             tc.name,
				words:            []uint16{0x2600, 0x4FEF, 0x0010}, // MOVE.L D0,D3; LEA 16(A7),A7
				requireProdSafe:  true,
				requireNativeRun: true,
				setup: func(cpu *M68KCPU) {
					cpu.DataRegs[0] = tc.d0
					cpu.AddrRegs[7] = 0x10000
					for addr := uint32(0x10000); addr < 0x10040; addr += 4 {
						cpu.Write32(addr, 0xCAFE0000|addr)
					}
				},
				watch: []m68kDiffMemWatch{
					{addr: 0x10000, size: M68K_SIZE_LONG},
					{addr: 0x10010, size: M68K_SIZE_LONG},
				},
			}, 2)
		})
	}

	runM68KJITDifferentialSingle(t, m68kDiffCase{
		name:             "LEA_d16A7_A7",
		words:            []uint16{0x4FEF, 0x0010},
		requireProdSafe:  true,
		requireNativeRun: true,
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[7] = 0x10000
		},
	})

	runM68KJITDifferentialSingle(t, m68kDiffCase{
		name:             "PEA_absW_high_A7",
		words:            []uint16{0x4878, 0x0505},
		requireProdSafe:  true,
		requireNativeRun: true,
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[7] = 0x00C34748
			for addr := uint32(0x00C34730); addr < 0x00C34750; addr += 4 {
				cpu.Write32(addr, 0x5A5A0000|addr)
			}
		},
		watch: []m68kDiffMemWatch{
			{addr: 0x00C34744, size: M68K_SIZE_LONG},
			{addr: 0x00C34748, size: M68K_SIZE_LONG},
		},
	})
}

func TestM68KJIT_Differential_FullIndexLEAThenCMPMPostinc(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	runM68KJITDifferentialBlock(t, m68kDiffCase{
		name: "full_index_lea_cmpm_postinc",
		words: []uint16{
			0x41F1, 0x8800, // LEA 0(A1,A0.L),A0
			0xB1C9, // CMPM.B (A1)+,(A0)+
		},
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = 4
			cpu.AddrRegs[1] = 0x3000
			cpu.Write8(0x3000, 0x5A)
			cpu.Write8(0x3004, 0x5A)
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}, 2)
}

func TestM68KJIT_Differential_CMPMPostincThenBNE(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range []struct {
		name string
		src  uint8
		dst  uint8
	}{
		{name: "equal_fallthrough", src: 0x42, dst: 0x42},
		{name: "different_branch", src: 0x42, dst: 0x7A},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialBlockDynamicRetire(t, m68kDiffCase{
				name: "cmpm_bne_" + tc.name,
				words: []uint16{
					0xB1C9, // CMPM.B (A1)+,(A0)+
					0x6604, // BNE.S +4
					0x7001, // MOVEQ #1,D0
					0x4E75, // RTS
					0x7002, // MOVEQ #2,D0
					0x4E75, // RTS
				},
				setup: func(cpu *M68KCPU) {
					cpu.AddrRegs[0] = 0x3100
					cpu.AddrRegs[1] = 0x3200
					cpu.AddrRegs[7] = 0x3F00
					cpu.Write32(cpu.AddrRegs[7], 0x00C0FFEE)
					cpu.Write8(0x3100, tc.dst)
					cpu.Write8(0x3200, tc.src)
				},
				requireProdSafe:  true,
				requireNativeRun: true,
				watch: []m68kDiffMemWatch{
					{addr: 0x3100, size: M68K_SIZE_BYTE},
					{addr: 0x3200, size: M68K_SIZE_BYTE},
				},
			}, 4)
		})
	}
}

func TestM68KJIT_Differential_DnToMemoryEAUsesComputedAddress(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range []struct {
		name string
		op   uint16
	}{
		{name: "ADD_L_D2_d16A2", op: 0xD5AA},
		{name: "OR_L_D2_d16A2", op: 0x85AA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialBlock(t, m68kDiffCase{
				name:  tc.name,
				words: []uint16{tc.op, 0x0010},
				setup: func(cpu *M68KCPU) {
					cpu.AddrRegs[2] = 0x4000
					cpu.DataRegs[2] = 0x00000030
					cpu.Write32(0x4010, 0x00000100)
				},
				watch: []m68kDiffMemWatch{
					{addr: 0x4010, size: M68K_SIZE_LONG},
				},
				requireProdSafe:  true,
				requireNativeRun: true,
			}, 1)
		})
	}
}

func TestM68KJIT_Differential_FPUHelperOpcodes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range m68kFPUDiffCases() {
		t.Run(tc.name, func(t *testing.T) {
			runM68KJITDifferentialFPUHelperSingle(t, tc)
		})
	}
	if os.Getenv("IE_M68K_JIT_68020_FPU_MATRIX") != "" {
		writeM68K68020FPUMatrix(t, "m68k_jit_68020_fpu_coverage_matrix.tsv")
	}
}

func m68kFPUDiffCases() []m68kFPUDiffCase {
	setupRegs := func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(0, 20.0)
		cpu.FPU.SetFP64(1, 10.0)
		cpu.FPU.SetFP64(2, 4.0)
		cpu.FPU.SetFP64(3, 4.0)
		cpu.FPU.FPCR = 0x01020304
		cpu.FPU.FPSR = 0x05060708
		cpu.FPU.FPIAR = 0x090A0B0C
	}
	setupMem := func(cpu *M68KCPU) {
		setupRegs(cpu)
		cpu.AddrRegs[0] = 0x3600
		cpu.Write32(0x3600, 0x11223344)
		cpu.Write32(0x3604, 0x55667788)
		cpu.Write32(0x3608, 0x99AABBCC)
	}

	cases := m68kFPURegisterOpDiffCases()
	cases = append(cases, m68kFPUEAOpDiffCases()...)
	cases = append(cases, m68kFPUStoreDiffCases()...)
	cases = append(cases, m68kFPUContextDiffCases()...)
	cases = append(cases, m68kFPUConditionalDiffCases()...)
	cases = append(cases,
		m68kFPUDiffCase{name: "FMOVECR_PI_FP0", words: []uint16{0xF200, 0x5C00}, setup: setupRegs},
		m68kFPUDiffCase{name: "FMOVE_D_IMM_PI_FP0", words: []uint16{0xF23C, 0x5400, 0x4009, 0x21FB, 0x5444, 0x2D18}, setup: setupRegs},
		m68kFPUDiffCase{name: "FMOVE_CONTROL_TO_MEM_A0", words: []uint16{0xF210, 0xBC00}, setup: setupMem, watch: []m68kDiffMemWatch{
			{addr: 0x3600, size: M68K_SIZE_LONG},
			{addr: 0x3604, size: M68K_SIZE_LONG},
			{addr: 0x3608, size: M68K_SIZE_LONG},
		}},
		m68kFPUDiffCase{name: "FMOVE_MEM_TO_CONTROL_A0", words: []uint16{0xF210, 0x9C00}, setup: setupMem, watch: []m68kDiffMemWatch{
			{addr: 0x3600, size: M68K_SIZE_LONG},
			{addr: 0x3604, size: M68K_SIZE_LONG},
			{addr: 0x3608, size: M68K_SIZE_LONG},
		}},
		m68kFPUDiffCase{name: "FSAVE_D16_A0", words: []uint16{0xF328, 0x006C}, setup: setupMem, watch: []m68kDiffMemWatch{
			{addr: 0x366C, size: M68K_SIZE_LONG},
		}},
	)
	return cases
}

func m68kFPUEAOpDiffCases() []m68kFPUDiffCase {
	setupMem := func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(0, 20.0)
		cpu.FPU.SetFP64(1, 10.0)
		cpu.FPU.SetFP64(2, -3.5)
		cpu.FPU.SetFP64(3, 4.25)
		cpu.FPU.FPCR = 0x01020304
		cpu.FPU.FPSR = 0x05060708
		cpu.FPU.FPIAR = 0x090A0B0C
		cpu.DataRegs[0] = 0
		cpu.AddrRegs[0] = 0x3600
		cpu.AddrRegs[1] = 0x3664
		cpu.AddrRegs[2] = 0x3670
		cpu.Write32(0x3600, 0x0000000A)            // long integer 10
		cpu.Write32(0x3610, math.Float32bits(2.5)) // single 2.5
		cpu.Write16(0x3620, 0xFFF9)                // word integer -7
		cpu.Write32(0x3630, 0x40090000)            // double pi high word
		cpu.Write32(0x3634, 0x00000000)            // double pi low word
		cpu.Write8(0x3640, 0xFD)                   // byte integer -3
		cpu.writeExtendedReal96(0x3650, cpu.FPU.GetExtendedReal(1))
		cpu.Write32(0x3660, 0x0000000B)            // predecrement/postincrement long
		cpu.Write32(0x3670, 0x0000000C)            // indexed long
		cpu.Write32(0x3680, math.Float32bits(3.5)) // absolute/PC single
	}
	setupImmediate := func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(0, 20.0)
		cpu.FPU.SetFP64(1, 10.0)
		cpu.FPU.SetFP64(2, -3.5)
		cpu.FPU.SetFP64(3, 4.25)
		cpu.FPU.FPCR = 0x01020304
		cpu.FPU.FPSR = 0x05060708
		cpu.FPU.FPIAR = 0x090A0B0C
	}

	eaOpcode := func(mode, reg uint16) uint16 { return 0xF200 | mode<<3 | reg }
	eaCmd := func(format, dst, op uint16) uint16 { return 0x4000 | format<<10 | dst<<7 | op }

	var cases []m68kFPUDiffCase
	for _, op := range []struct {
		name string
		code uint16
	}{
		{name: "FMOVE", code: FPU_OP_FMOVE},
		{name: "FADD", code: FPU_OP_FADD},
		{name: "FSUB", code: FPU_OP_FSUB},
		{name: "FMUL", code: FPU_OP_FMUL},
		{name: "FDIV", code: FPU_OP_FDIV},
		{name: "FCMP", code: FPU_OP_FCMP},
	} {
		for _, format := range []struct {
			name string
			code uint16
			disp uint16
		}{
			{name: "L", code: 0, disp: 0x0000},
			{name: "S", code: 1, disp: 0x0010},
			{name: "X", code: 2, disp: 0x0050},
			{name: "W", code: 4, disp: 0x0020},
			{name: "D", code: 5, disp: 0x0030},
			{name: "B", code: 6, disp: 0x0040},
		} {
			cases = append(cases, m68kFPUDiffCase{
				name:  fmt.Sprintf("%s_%s_d16A0_FP0", op.name, format.name),
				words: []uint16{eaOpcode(M68K_AM_AR_DISP, 0), eaCmd(format.code, 0, op.code), format.disp},
				setup: setupMem,
			})
		}
	}
	cases = append(cases,
		m68kFPUDiffCase{name: "FMOVE_L_A0ind_FP0", words: []uint16{eaOpcode(M68K_AM_AR_IND, 0), eaCmd(0, 0, FPU_OP_FMOVE)}, setup: setupMem},
		m68kFPUDiffCase{name: "FADD_L_A0ind_FP0", words: []uint16{eaOpcode(M68K_AM_AR_IND, 0), eaCmd(0, 0, FPU_OP_FADD)}, setup: setupMem},
		m68kFPUDiffCase{name: "FMOVE_L_A1post_FP0", words: []uint16{eaOpcode(M68K_AM_AR_POST, 1), eaCmd(0, 0, FPU_OP_FMOVE)}, setup: setupMem},
		m68kFPUDiffCase{name: "FMOVE_L_A1pre_FP0", words: []uint16{eaOpcode(M68K_AM_AR_PRE, 1), eaCmd(0, 0, FPU_OP_FMOVE)}, setup: setupMem},
		m68kFPUDiffCase{name: "FMOVE_L_d8A2D0_FP0", words: []uint16{eaOpcode(M68K_AM_AR_INDEX, 2), eaCmd(0, 0, FPU_OP_FMOVE), 0x0000}, setup: setupMem},
		m68kFPUDiffCase{name: "FMOVE_S_absW_FP0", words: []uint16{eaOpcode(7, 0), eaCmd(1, 0, FPU_OP_FMOVE), 0x3680}, setup: setupMem},
		m68kFPUDiffCase{name: "FMOVE_S_absL_FP0", words: []uint16{eaOpcode(7, 1), eaCmd(1, 0, FPU_OP_FMOVE), 0x0000, 0x3680}, setup: setupMem},
		m68kFPUDiffCase{name: "FMOVE_L_IMM_FP0", words: []uint16{eaOpcode(7, 4), eaCmd(0, 0, FPU_OP_FMOVE), 0xFFFF, 0xFFF6}, setup: setupImmediate},
		m68kFPUDiffCase{name: "FMOVE_S_IMM_FP0", words: []uint16{eaOpcode(7, 4), eaCmd(1, 0, FPU_OP_FMOVE), uint16(math.Float32bits(2.5) >> 16), uint16(math.Float32bits(2.5))}, setup: setupImmediate},
		m68kFPUDiffCase{name: "FMOVE_X_IMM_FP0", words: []uint16{eaOpcode(7, 4), eaCmd(2, 0, FPU_OP_FMOVE), 0x4000, 0x8000, 0x0000, 0x0000, 0x0000, 0x0000}, setup: setupImmediate},
		m68kFPUDiffCase{name: "FMOVE_W_IMM_FP0", words: []uint16{eaOpcode(7, 4), eaCmd(4, 0, FPU_OP_FMOVE), 0xFFF9}, setup: setupImmediate},
		m68kFPUDiffCase{name: "FMOVE_D_IMM_FP0", words: []uint16{eaOpcode(7, 4), eaCmd(5, 0, FPU_OP_FMOVE), 0x4009, 0x0000, 0x0000, 0x0000}, setup: setupImmediate},
		m68kFPUDiffCase{name: "FMOVE_B_IMM_FP0", words: []uint16{eaOpcode(7, 4), eaCmd(6, 0, FPU_OP_FMOVE), 0x00FD}, setup: setupImmediate},
		m68kFPUDiffCase{name: "FADD_W_IMM_FP0", words: []uint16{eaOpcode(7, 4), eaCmd(4, 0, FPU_OP_FADD), 0x000A}, setup: setupImmediate},
	)
	return cases
}

func m68kFPUStoreDiffCases() []m68kFPUDiffCase {
	setup := func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(0, math.Pi)
		cpu.FPU.SetFP64(1, -7.75)
		cpu.FPU.FPCR = 0x01020304
		cpu.FPU.FPSR = 0x05060708
		cpu.FPU.FPIAR = 0x090A0B0C
		cpu.DataRegs[0] = 0
		cpu.AddrRegs[0] = 0x3700
		cpu.AddrRegs[1] = 0x3784
		cpu.AddrRegs[2] = 0x3790
		for addr := uint32(0x3700); addr < 0x3780; addr += 4 {
			cpu.Write32(addr, 0xA5A50000|addr)
		}
		for addr := uint32(0x3780); addr < 0x37C0; addr += 4 {
			cpu.Write32(addr, 0x5A5A0000|addr)
		}
	}
	opcode := func(mode, reg uint16) uint16 { return 0xF200 | mode<<3 | reg }
	cmd := func(format, src uint16) uint16 { return 0x6000 | format<<10 | src<<7 }

	return []m68kFPUDiffCase{
		{name: "FMOVE_FP0_L_A0ind", words: []uint16{opcode(M68K_AM_AR_IND, 0), cmd(0, 0)}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x3700, size: M68K_SIZE_LONG}}},
		{name: "FMOVE_FP0_L_A1post", words: []uint16{opcode(M68K_AM_AR_POST, 1), cmd(0, 0)}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x3784, size: M68K_SIZE_LONG}}},
		{name: "FMOVE_FP0_L_A1pre", words: []uint16{opcode(M68K_AM_AR_PRE, 1), cmd(0, 0)}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x3780, size: M68K_SIZE_LONG}}},
		{name: "FMOVE_FP0_L_d8A2D0", words: []uint16{opcode(M68K_AM_AR_INDEX, 2), cmd(0, 0), 0x0000}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x3790, size: M68K_SIZE_LONG}}},
		{name: "FMOVE_FP0_S_absW", words: []uint16{opcode(7, 0), cmd(1, 0), 0x37A0}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x37A0, size: M68K_SIZE_LONG}}},
		{name: "FMOVE_FP0_S_absL", words: []uint16{opcode(7, 1), cmd(1, 0), 0x0000, 0x37A4}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x37A4, size: M68K_SIZE_LONG}}},
		{name: "FMOVE_FP0_S_d16A0", words: []uint16{opcode(M68K_AM_AR_DISP, 0), cmd(1, 0), 0x0010}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x3710, size: M68K_SIZE_LONG}}},
		{name: "FMOVE_FP0_X_d16A0", words: []uint16{opcode(M68K_AM_AR_DISP, 0), cmd(2, 0), 0x0020}, setup: setup, watch: []m68kDiffMemWatch{
			{addr: 0x3720, size: M68K_SIZE_LONG},
			{addr: 0x3724, size: M68K_SIZE_LONG},
			{addr: 0x3728, size: M68K_SIZE_LONG},
		}},
		{name: "FMOVE_FP1_W_d16A0", words: []uint16{opcode(M68K_AM_AR_DISP, 0), cmd(4, 1), 0x0030}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x3730, size: M68K_SIZE_WORD}}},
		{name: "FMOVE_FP0_D_d16A0", words: []uint16{opcode(M68K_AM_AR_DISP, 0), cmd(5, 0), 0x0040}, setup: setup, watch: []m68kDiffMemWatch{
			{addr: 0x3740, size: M68K_SIZE_LONG},
			{addr: 0x3744, size: M68K_SIZE_LONG},
		}},
		{name: "FMOVE_FP1_B_d16A0", words: []uint16{opcode(M68K_AM_AR_DISP, 0), cmd(6, 1), 0x0050}, setup: setup, watch: []m68kDiffMemWatch{{addr: 0x3750, size: M68K_SIZE_BYTE}}},
	}
}

func m68kFPUConditionalDiffCases() []m68kFPUDiffCase {
	setupFPSR := func(fpsr uint32) func(*M68KCPU) {
		return func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(0, 1.0)
			cpu.FPU.SetFP64(1, -1.0)
			cpu.FPU.FPCR = 0x01020304
			cpu.FPU.FPSR = fpsr
			cpu.FPU.FPIAR = 0x090A0B0C
			cpu.DataRegs[0] = 0x12340002
			cpu.DataRegs[1] = 0xAABBCCDD
			cpu.AddrRegs[0] = 0x3A00
			cpu.Write8(0x3A00, 0x55)
		}
	}

	return []m68kFPUDiffCase{
		{name: "FBF_W_not_taken", words: []uint16{0xF280, 0x0004}, setup: setupFPSR(0)},
		{name: "FBT_W_taken", words: []uint16{0xF28F, 0x0004}, setup: setupFPSR(0)},
		{name: "FBEQ_W_taken", words: []uint16{0xF281, 0x0004}, setup: setupFPSR(FPU_CC_Z)},
		{name: "FBNE_W_not_taken", words: []uint16{0xF28E, 0x0004}, setup: setupFPSR(FPU_CC_Z)},
		{name: "FBT_L_taken", words: []uint16{0xF2CF, 0x0000, 0x0006}, setup: setupFPSR(0)},
		{name: "FBF_L_not_taken", words: []uint16{0xF2C0, 0x0000, 0x0006}, setup: setupFPSR(0)},
		{name: "FST_D0", words: []uint16{0xF240, 0x000F}, setup: setupFPSR(0)},
		{name: "FSF_D0", words: []uint16{0xF240, 0x0000}, setup: setupFPSR(0)},
		{name: "FSEQ_A0ind", words: []uint16{0xF250, 0x0001}, setup: setupFPSR(FPU_CC_Z), watch: []m68kDiffMemWatch{{addr: 0x3A00, size: M68K_SIZE_BYTE}}},
		{name: "FSNE_A0ind", words: []uint16{0xF250, 0x000E}, setup: setupFPSR(FPU_CC_Z), watch: []m68kDiffMemWatch{{addr: 0x3A00, size: M68K_SIZE_BYTE}}},
		{name: "FDBF_D0_branch", words: []uint16{0xF248, 0x0000, 0x0004}, setup: setupFPSR(0)},
		{name: "FDBT_D0_no_branch", words: []uint16{0xF248, 0x000F, 0x0004}, setup: setupFPSR(0)},
	}
}

func m68kFPUContextDiffCases() []m68kFPUDiffCase {
	watchRange := func(addr uint32, bytes uint32) []m68kDiffMemWatch {
		watch := make([]m68kDiffMemWatch, 0, bytes/4)
		for off := uint32(0); off < bytes; off += 4 {
			watch = append(watch, m68kDiffMemWatch{addr: addr + off, size: M68K_SIZE_LONG})
		}
		return watch
	}
	setupRegs := func(cpu *M68KCPU) {
		values := []float64{math.Pi, -math.E, 0.0, 1.25, -42.0, 1e-20, 256.5, -0.125}
		for i, value := range values {
			cpu.FPU.SetFP64(i, value)
		}
		cpu.FPU.FPCR = 0x01020304
		cpu.FPU.FPSR = 0x05060708
		cpu.FPU.FPIAR = 0x090A0B0C
		cpu.AddrRegs[0] = 0x3800
		cpu.AddrRegs[7] = 0x3900
		for addr := uint32(0x3700); addr < 0x3920; addr += 4 {
			cpu.Write32(addr, 0xA5A50000|addr)
		}
	}
	setupLoadAll := func(cpu *M68KCPU) {
		setupRegs(cpu)
		for i := range 8 {
			cpu.writeExtendedReal96(0x380C+uint32(i*m68kFPUExtendedRealBytes), cpu.FPU.GetExtendedReal(i))
			cpu.FPU.SetFP64(i, 0)
		}
	}
	setupLoadPairPost := func(cpu *M68KCPU) {
		setupRegs(cpu)
		cpu.writeExtendedReal96(0x3900, cpu.FPU.GetExtendedReal(2))
		cpu.writeExtendedReal96(0x3900+m68kFPUExtendedRealBytes, cpu.FPU.GetExtendedReal(3))
		cpu.FPU.SetFP64(2, 0)
		cpu.FPU.SetFP64(3, 0)
	}
	setupFRestore := func(cpu *M68KCPU) {
		setupRegs(cpu)
		cpu.FPU.FPCR = 0x11111111
		cpu.FPU.FPSR = 0x22222222
		cpu.FPU.FPIAR = 0x33333333
		cpu.Write32(0x386C, 0) // NULL frame resets FPU control state.
	}

	return []m68kFPUDiffCase{
		{
			name:  "FMOVEM_STORE_FP0_FP7_d16A0",
			words: []uint16{0xF228, 0xF0FF, 0x000C},
			setup: setupRegs,
			watch: watchRange(0x380C, 8*m68kFPUExtendedRealBytes),
		},
		{
			name:  "FMOVEM_LOAD_d16A0_FP0_FP7",
			words: []uint16{0xF228, 0xD0FF, 0x000C},
			setup: setupLoadAll,
			watch: watchRange(0x380C, 8*m68kFPUExtendedRealBytes),
		},
		{
			name:  "FMOVEM_STORE_FP2_FP3_predecA7",
			words: []uint16{0xF227, 0xE00C},
			setup: setupRegs,
			watch: watchRange(0x3900-2*m68kFPUExtendedRealBytes, 2*m68kFPUExtendedRealBytes),
		},
		{
			name:  "FMOVEM_LOAD_A7post_FP2_FP3",
			words: []uint16{0xF21F, 0xD030},
			setup: setupLoadPairPost,
			watch: watchRange(0x3900, 2*m68kFPUExtendedRealBytes),
		},
		{
			name:  "FRESTORE_NULL_d16A0",
			words: []uint16{0xF368, 0x006C},
			setup: setupFRestore,
			watch: []m68kDiffMemWatch{{addr: 0x386C, size: M68K_SIZE_LONG}},
		},
	}
}

func m68kFPURegisterOpDiffCases() []m68kFPUDiffCase {
	type regOp struct {
		name  string
		op    uint16
		setup func(*M68KCPU)
	}
	setupPair := func(dstReg, srcReg uint16, dst, src float64) func(*M68KCPU) {
		return func(cpu *M68KCPU) {
			values := []float64{20.0, 10.0, -3.5, 4.25, 0.5, -0.25, 16.0, -8.0}
			for i, value := range values {
				cpu.FPU.SetFP64(i, value)
			}
			cpu.FPU.SetFP64(int(dstReg), dst)
			cpu.FPU.SetFP64(int(srcReg), src)
			cpu.FPU.FPCR = 0x01020304
			cpu.FPU.FPSR = 0x05060708
			cpu.FPU.FPIAR = 0x090A0B0C
		}
	}
	ops := []regOp{
		{name: "FMOVE", op: FPU_OP_FMOVE, setup: setupPair(0, 1, 20.0, 10.0)},
		{name: "FINT", op: FPU_OP_FINT, setup: setupPair(0, 1, 20.0, 10.75)},
		{name: "FSINH", op: FPU_OP_FSINH, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FINTRZ", op: FPU_OP_FINTRZ, setup: setupPair(0, 1, 20.0, -10.75)},
		{name: "FSQRT", op: FPU_OP_FSQRT, setup: setupPair(0, 1, 20.0, 16.0)},
		{name: "FLOGNP1", op: FPU_OP_FLOGNP1, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FETOXM1", op: FPU_OP_FETOXM1, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FTANH", op: FPU_OP_FTANH, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FATAN", op: FPU_OP_FATAN, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FASIN", op: FPU_OP_FASIN, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FATANH", op: FPU_OP_FATANH, setup: setupPair(0, 1, 20.0, 0.25)},
		{name: "FSIN", op: FPU_OP_FSIN, setup: setupPair(0, 1, 20.0, math.Pi/2)},
		{name: "FTAN", op: FPU_OP_FTAN, setup: setupPair(0, 1, 20.0, math.Pi/6)},
		{name: "FETOX", op: FPU_OP_FETOX, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FTWOTOX", op: FPU_OP_FTWOTOX, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FTENTOX", op: FPU_OP_FTENTOX, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FLOGN", op: FPU_OP_FLOGN, setup: setupPair(0, 1, 20.0, 2.0)},
		{name: "FLOG10", op: FPU_OP_FLOG10, setup: setupPair(0, 1, 20.0, 100.0)},
		{name: "FLOG2", op: FPU_OP_FLOG2, setup: setupPair(0, 1, 20.0, 8.0)},
		{name: "FABS", op: FPU_OP_FABS, setup: setupPair(0, 1, 20.0, -10.0)},
		{name: "FCOSH", op: FPU_OP_FCOSH, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FNEG", op: FPU_OP_FNEG, setup: setupPair(0, 1, 20.0, 10.0)},
		{name: "FACOS", op: FPU_OP_FACOS, setup: setupPair(0, 1, 20.0, 0.5)},
		{name: "FCOS", op: FPU_OP_FCOS, setup: setupPair(0, 1, 20.0, math.Pi/3)},
		{name: "FGETEXP", op: FPU_OP_FGETEXP, setup: setupPair(0, 1, 20.0, 16.0)},
		{name: "FGETMAN", op: FPU_OP_FGETMAN, setup: setupPair(0, 1, 20.0, 16.0)},
		{name: "FDIV", op: FPU_OP_FDIV, setup: setupPair(0, 1, 20.0, 4.0)},
		{name: "FMOD", op: FPU_OP_FMOD, setup: setupPair(0, 1, 20.0, 6.0)},
		{name: "FADD", op: FPU_OP_FADD, setup: setupPair(0, 1, 20.0, 10.0)},
		{name: "FMUL", op: FPU_OP_FMUL, setup: setupPair(0, 1, 20.0, 4.0)},
		{name: "FSGLDIV", op: FPU_OP_FSGLDIV, setup: setupPair(0, 1, 20.0, 4.0)},
		{name: "FREM", op: FPU_OP_FREM, setup: setupPair(0, 1, 20.0, 6.0)},
		{name: "FSCALE", op: FPU_OP_FSCALE, setup: setupPair(0, 1, 20.0, 2.0)},
		{name: "FSGLMUL", op: FPU_OP_FSGLMUL, setup: setupPair(0, 1, 20.0, 4.0)},
		{name: "FSUB", op: FPU_OP_FSUB, setup: setupPair(0, 1, 20.0, 10.0)},
		{name: "FCMP", op: FPU_OP_FCMP, setup: setupPair(0, 1, 20.0, 10.0)},
		{name: "FTST", op: FPU_OP_FTST, setup: setupPair(0, 1, 20.0, 10.0)},
	}

	pairs := []struct {
		dst uint16
		src uint16
	}{
		{dst: 0, src: 1},
		{dst: 2, src: 3},
		{dst: 7, src: 0},
		{dst: 4, src: 4},
	}
	cases := make([]m68kFPUDiffCase, 0, len(ops)*len(pairs)+4)
	for _, op := range ops {
		for _, pair := range pairs {
			tc := m68kFPUDiffCase{
				name:  fmt.Sprintf("%s_FP%d_FP%d", op.name, pair.src, pair.dst),
				words: []uint16{0xF200, uint16(pair.src<<10 | pair.dst<<7 | op.op)},
				setup: op.setup,
			}
			if pair.dst != 0 || pair.src != 1 {
				tc.setup = setupPair(pair.dst, pair.src, cpuFPUTestDstValue(op.name), cpuFPUTestSrcValue(op.name))
			}
			cases = append(cases, tc)
		}
	}
	cases = append(cases,
		m68kFPUDiffCase{name: "FADD_S_FP1_FP0", words: []uint16{0xF200, uint16(1<<10 | FPU_OP_FADD | 0x40)}, setup: setupPair(0, 1, 20.0, 10.0)},
		m68kFPUDiffCase{name: "FADD_D_FP1_FP0", words: []uint16{0xF200, uint16(1<<10 | FPU_OP_FADD | 0x44)}, setup: setupPair(0, 1, 20.0, 10.0)},
		m68kFPUDiffCase{name: "FMOVE_S_FP1_FP0", words: []uint16{0xF200, uint16(1<<10 | FPU_OP_FMOVE | 0x40)}, setup: setupPair(0, 1, 20.0, math.Pi)},
		m68kFPUDiffCase{name: "FMOVE_D_FP1_FP0", words: []uint16{0xF200, uint16(1<<10 | FPU_OP_FMOVE | 0x44)}, setup: setupPair(0, 1, 20.0, math.Pi)},
	)
	return cases
}

func cpuFPUTestDstValue(op string) float64 {
	switch op {
	case "FMOD", "FREM":
		return 20.0
	case "FSCALE":
		return 20.0
	default:
		return 20.0
	}
}

func cpuFPUTestSrcValue(op string) float64 {
	switch op {
	case "FINT":
		return 10.75
	case "FINTRZ":
		return -10.75
	case "FSQRT", "FGETEXP", "FGETMAN":
		return 16.0
	case "FLOGN":
		return 2.0
	case "FLOG10":
		return 100.0
	case "FLOG2":
		return 8.0
	case "FABS":
		return -10.0
	case "FNEG", "FMOVE", "FADD", "FSUB", "FCMP", "FTST":
		return 10.0
	case "FDIV", "FMUL", "FSGLDIV", "FSGLMUL":
		return 4.0
	case "FMOD", "FREM":
		return 6.0
	case "FSCALE":
		return 2.0
	case "FSIN":
		return math.Pi / 2
	case "FTAN":
		return math.Pi / 6
	case "FCOS":
		return math.Pi / 3
	case "FATANH":
		return 0.25
	default:
		return 0.5
	}
}

func writeM68K68020FPUMatrix(t *testing.T, path string) {
	t.Helper()

	var rows strings.Builder
	rows.WriteString("scope\tfeature\tcase\topcode\tform\tinterpreter_passing\tjit_admitted\tjit_path\tparity_test\n")
	for _, tc := range m68kFPUDiffCases() {
		fmt.Fprintf(&rows, "explicit\tFPU\t%s\t%04X\thelper\ttrue\ttrue\thelper\tTestM68KJIT_Differential_FPUHelperOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kBitFieldDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_ProductionBitFieldOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kMultiplyDivideDiffCases() {
		if !m68kDiffCaseIs68020MultiplyDivide(tc.name) {
			continue
		}
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_ProductionMultiplyDivideOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kSpecialDataDiffCases() {
		if !m68kDiffCaseIs68020SpecialData(tc.name) {
			continue
		}
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_ProductionSpecialDataOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kCHK2CMP2DiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\thelper\ttrue\ttrue\thelper\tTestM68KJIT_Differential_CHK2CMP2HelperOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kCASCAS2DiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\thelper\ttrue\ttrue\thelper\tTestM68KJIT_Differential_CASCAS2HelperOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kMOVESDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\thelper\ttrue\ttrue\thelper\tTestM68KJIT_Differential_MOVESHelperOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kTRAPccDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\thelper\ttrue\ttrue\thelper\tTestM68KJIT_Differential_TRAPccHelperOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kBKPTDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\thelper\ttrue\ttrue\thelper\tTestM68KJIT_Differential_BKPTHelperOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kCALLMDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\thelper\ttrue\ttrue\thelper\tTestM68KJIT_Differential_CALLMHelperOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kRTMDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\thelper\ttrue\ttrue\thelper\tTestM68KJIT_Differential_RTMHelperOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kMOVECDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_MOVECNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kTASDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_TASNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kMOVEPDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_MOVEPNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kNBCDDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_NBCDNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kABCDSBCDDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_ABCDSBCDNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kEXGDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_EXGNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kCHKDiffCases() {
		fmt.Fprintf(&rows, "explicit\t68020\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_CHKNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kUnaryEADiffCases() {
		fmt.Fprintf(&rows, "explicit\tM68K\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_UnaryEANativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kAddressGenerationDiffCases() {
		fmt.Fprintf(&rows, "explicit\tM68K\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_AddressGenerationNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kSccDiffCases() {
		fmt.Fprintf(&rows, "explicit\tM68K\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_SccNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kImmediateDiffCases() {
		fmt.Fprintf(&rows, "explicit\tM68K\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_ImmediateNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kADDQSUBQDiffCases() {
		fmt.Fprintf(&rows, "explicit\tM68K\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_ADDQSUBQNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kMOVEMDiffCases() {
		fmt.Fprintf(&rows, "explicit\tM68K\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_ProductionMOVEMOpcodes\n", tc.name, tc.words[0])
	}
	for _, tc := range m68kExtendedArithmeticDiffCases() {
		fmt.Fprintf(&rows, "explicit\tM68K\t%s\t%04X\tnative\ttrue\ttrue\tnative\tTestM68KJIT_Differential_ExtendedArithmeticNativeOpcodes\n", tc.name, tc.words[0])
	}
	for _, row := range m68kGenerated68020InventoryRows(t) {
		fmt.Fprintf(&rows, "%s\t%s\t%s\t%04X\t%s\t%s\t%t\t%s\t%s\n",
			row.scope, row.feature, row.name, row.opcode, row.form,
			row.interpreterPassing, row.jitAdmitted, row.jitPath, row.parityTest)
	}
	if err := os.WriteFile(path, []byte(rows.String()), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type m68kInventoryRow struct {
	scope              string
	feature            string
	name               string
	opcode             uint16
	form               string
	interpreterPassing string
	jitAdmitted        bool
	jitPath            string
	parityTest         string
}

func m68kGenerated68020InventoryRows(t *testing.T) []m68kInventoryRow {
	t.Helper()

	var rows []m68kInventoryRow
	add := func(feature, name string, opcode uint16, form string, words ...uint16) {
		if len(words) == 0 {
			words = []uint16{opcode}
		}
		rows = append(rows, m68kClassifyGenerated68020InventoryRow(t, feature, name, opcode, form, words))
	}

	for _, size := range []struct {
		name string
		base uint16
	}{
		{name: "B", base: 0x00C0},
		{name: "W", base: 0x02C0},
		{name: "L", base: 0x04C0},
	} {
		for _, kind := range []struct {
			name string
			ext  uint16
		}{
			{name: "CMP2", ext: 0x0000},
			{name: "CHK2", ext: 0x0800},
		} {
			for _, ea := range m68kInventoryMemoryEAForms() {
				opcode := size.base | ea.mode<<3 | ea.reg
				words := []uint16{opcode, kind.ext}
				words = append(words, ea.ext...)
				add("68020", fmt.Sprintf("%s_%s_%s", kind.name, size.name, ea.name), opcode, "generated", words...)
			}
		}
	}

	for _, size := range []struct {
		name string
		base uint16
	}{
		{name: "B", base: 0x0AC0},
		{name: "W", base: 0x0CC0},
		{name: "L", base: 0x0EC0},
	} {
		for _, ea := range m68kInventoryMemoryEAForms() {
			opcode := size.base | ea.mode<<3 | ea.reg
			words := []uint16{opcode, 0x0040}
			words = append(words, ea.ext...)
			add("68020", fmt.Sprintf("CAS_%s_%s", size.name, ea.name), opcode, "generated", words...)
		}
	}
	add("68020", "CAS2_W_generated", 0x0CFC, "generated", 0x0CFC, 0xA040, 0xB082)
	add("68020", "CAS2_L_generated", 0x0EFC, "generated", 0x0EFC, 0xA040, 0xB082)

	for _, size := range []struct {
		name string
		bits uint16
	}{
		{name: "B", bits: 0 << 6},
		{name: "W", bits: 1 << 6},
		{name: "L", bits: 2 << 6},
	} {
		for _, direction := range []struct {
			name string
			ext  uint16
		}{
			{name: "mem_to_D1", ext: 0x1000},
			{name: "D1_to_mem", ext: 0x1800},
		} {
			for _, ea := range m68kInventoryMemoryEAForms() {
				opcode := uint16(0x0E00) | size.bits | ea.mode<<3 | ea.reg
				words := []uint16{opcode, direction.ext}
				words = append(words, ea.ext...)
				add("68020", fmt.Sprintf("MOVES_%s_%s_%s", size.name, direction.name, ea.name), opcode, "generated", words...)
			}
		}
	}

	for _, cond := range []struct {
		name string
		bits uint16
	}{
		{name: "T_taken", bits: 0x0},
		{name: "F_not_taken", bits: 0x1},
		{name: "EQ_taken", bits: 0x7},
		{name: "NE_not_taken", bits: 0x6},
	} {
		for _, form := range []struct {
			name  string
			reg   uint16
			extra []uint16
		}{
			{name: "W", reg: 2, extra: []uint16{0x1234}},
			{name: "L", reg: 3, extra: []uint16{0x1234, 0x5678}},
			{name: "none", reg: 4},
		} {
			opcode := uint16(0x50F8) | cond.bits<<8 | form.reg
			words := []uint16{opcode}
			words = append(words, form.extra...)
			add("68020", fmt.Sprintf("TRAP%s_%s", cond.name, form.name), opcode, "generated", words...)
		}
	}

	for n := uint16(0); n < 8; n++ {
		add("68020", fmt.Sprintf("BKPT_%d", n), 0x4848|n, "generated", 0x4848|n)
	}

	for _, ea := range m68kInventoryMemoryEAForms() {
		opcode := uint16(0x06C0) | ea.mode<<3 | ea.reg
		words := []uint16{opcode, 0x0003}
		words = append(words, ea.ext...)
		add("68020", fmt.Sprintf("CALLM_%s", ea.name), opcode, "generated", words...)
	}

	for reg := uint16(0); reg < 8; reg++ {
		add("68020", fmt.Sprintf("RTM_D%d", reg), 0x06C0|reg, "generated", 0x06C0|reg)
		add("68020", fmt.Sprintf("RTM_A%d", reg), 0x06C8|reg, "generated", 0x06C8|reg)
	}

	for _, creg := range m68kMOVECControlRegs() {
		for _, reg := range []struct {
			name string
			num  uint16
		}{
			{name: "D1", num: 1},
			{name: "A2", num: 0xA},
		} {
			add("68020", fmt.Sprintf("MOVEC_%s_to_%s", creg.name, reg.name), 0x4E7A, "generated", 0x4E7A, reg.num<<12|creg.code)
			add("68020", fmt.Sprintf("MOVEC_%s_to_%s", reg.name, creg.name), 0x4E7B, "generated", 0x4E7B, reg.num<<12|creg.code)
		}
	}

	for _, tc := range m68kTASDiffCases() {
		add("68020", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kMOVEPDiffCases() {
		add("68020", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kNBCDDiffCases() {
		add("68020", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kABCDSBCDDiffCases() {
		add("68020", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kEXGDiffCases() {
		add("68020", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kCHKDiffCases() {
		add("68020", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kUnaryEADiffCases() {
		add("M68K", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kAddressGenerationDiffCases() {
		add("M68K", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kSccDiffCases() {
		add("M68K", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kImmediateDiffCases() {
		add("M68K", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kADDQSUBQDiffCases() {
		add("M68K", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kMOVEMDiffCases() {
		add("M68K", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kExtendedArithmeticDiffCases() {
		add("M68K", tc.name, tc.words[0], "generated", tc.words...)
	}

	for _, tc := range m68kPACKUNPKDiffCases() {
		add("68020", tc.name, tc.words[0], "generated", tc.words...)
	}

	return rows
}

func m68kClassifyGenerated68020InventoryRow(t *testing.T, feature, name string, opcode uint16, form string, words []uint16) m68kInventoryRow {
	t.Helper()

	row := m68kInventoryRow{
		scope:              "generated",
		feature:            feature,
		name:               name,
		opcode:             opcode,
		form:               form,
		interpreterPassing: "not_run",
		jitAdmitted:        false,
		jitPath:            "unsupported",
		parityTest:         "inventory",
	}

	cpu := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(cpu)
	m68kDiffSimpleEASetup(cpu)
	cpu.DataRegs[0] = 0x00000011
	cpu.DataRegs[1] = 0x00000022
	cpu.DataRegs[2] = 0x00000033
	cpu.DataRegs[3] = 0x00000044
	cpu.Write32(0x3200, 0x00000011)
	cpu.Write32(0x3210, 0x00000011)
	cpu.Write32(0x3400, 0x00000011)
	cpu.Write32(0x00036000, 0x00000011)
	if strings.HasPrefix(name, "CAS") {
		m68kDiffCASCAS2Setup(cpu)
	}
	if strings.HasPrefix(name, "MOVES_") {
		m68kDiffMOVESSetup(cpu)
	}
	if strings.HasPrefix(name, "TRAP") {
		sr := uint16(0)
		if strings.Contains(name, "EQ") {
			sr = M68K_SR_Z
		}
		m68kDiffTRAPccSetup(sr)(cpu)
	}
	if strings.HasPrefix(name, "BKPT_") {
		m68kDiffBKPTSetup(cpu)
	}
	if strings.HasPrefix(name, "CALLM_") {
		m68kDiffCALLMSetup(cpu)
	}
	if strings.HasPrefix(name, "RTM_") {
		m68kDiffRTMSetup(cpu)
	}
	if strings.HasPrefix(name, "MOVEC_") {
		m68kDiffMOVECSetup(cpu)
	}
	if strings.HasPrefix(name, "TAS_") {
		m68kDiffTASSetup(cpu)
	}
	if strings.HasPrefix(name, "MOVEP_") {
		m68kDiffMOVEPSetup(cpu)
	}
	if strings.HasPrefix(name, "NBCD_") {
		m68kDiffNBCDSetup(cpu)
	}
	if strings.HasPrefix(name, "ABCD_") || strings.HasPrefix(name, "SBCD_") {
		m68kDiffABCDSBCDSetup(cpu)
	}
	if strings.HasPrefix(name, "EXG_") {
		m68kDiffEXGSetup(cpu)
	}
	if strings.HasPrefix(name, "CHK_W_") || strings.HasPrefix(name, "CHK_L_") {
		m68kDiffCHKSetup(cpu)
	}
	if strings.HasPrefix(name, "NEGX_") ||
		strings.HasPrefix(name, "CLR_") ||
		strings.HasPrefix(name, "NEG_") ||
		strings.HasPrefix(name, "NOT_") ||
		strings.HasPrefix(name, "TST_") {
		m68kDiffUnaryEASetup(cpu)
	}
	if strings.HasPrefix(name, "LEA_") || strings.HasPrefix(name, "PEA_") {
		m68kDiffEffectiveAddressSetup(cpu)
	}
	if strings.HasPrefix(name, "Scc_") {
		sr := uint16(M68K_SR_S)
		if strings.Contains(name, "SR1F") {
			sr |= M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
		}
		m68kDiffSccSetup(sr)(cpu)
	}
	if strings.HasPrefix(name, "ORI_") ||
		strings.HasPrefix(name, "ANDI_") ||
		strings.HasPrefix(name, "SUBI_") ||
		strings.HasPrefix(name, "ADDI_") ||
		strings.HasPrefix(name, "EORI_") ||
		strings.HasPrefix(name, "CMPI_") {
		m68kDiffImmediateSetup(cpu)
	}
	if strings.HasPrefix(name, "ADDQ_") || strings.HasPrefix(name, "SUBQ_") {
		m68kDiffQuickSetup(cpu)
	}
	if strings.HasPrefix(name, "MOVEM_") {
		m68kDiffMOVEMSetup(cpu)
	}
	if strings.HasPrefix(name, "ADDX_") ||
		strings.HasPrefix(name, "SUBX_") ||
		strings.HasPrefix(name, "CMPM_") {
		sr := uint16(M68K_SR_S | M68K_SR_Z)
		if strings.Contains(name, "SR15") || strings.HasPrefix(name, "CMPM_") {
			sr = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_C
		}
		m68kDiffExtendedArithmeticSetup(sr)(cpu)
	}
	m68kDiffWriteProgram(cpu, m68kDiffStartPC, words...)
	expectedPC := m68kDiffStartPC + uint32(len(words)*2)
	if strings.HasPrefix(name, "TRAP") {
		condition := uint8((opcode >> 8) & 0xF)
		if cpu.CheckCondition(condition) {
			expectedPC = 0x00005000
		}
	}
	if strings.HasPrefix(name, "CALLM_") {
		expectedPC = 0x00005400
	}
	if strings.HasPrefix(name, "RTM_") {
		expectedPC = 0x00005600
	}
	if cycles := cpu.StepOne(); cycles != 0 && cpu.PC == expectedPC {
		row.interpreterPassing = "true"
	} else {
		row.interpreterPassing = "false"
	}

	instrs := m68kScanBlock(cpu.memory, m68kDiffStartPC)
	if len(instrs) == 0 {
		row.jitPath = "decode_failed"
		return row
	}
	if instrs[0].opcode != opcode {
		row.jitPath = "decode_diverged"
		return row
	}
	row.jitAdmitted = m68kInstrProductionNativeSafe(&instrs[0])
	if row.jitAdmitted {
		if opcode == 0x0CFC || opcode == 0x0EFC ||
			opcode&0xFFF8 == 0x4848 ||
			opcode&0xFFF0 == 0x06C0 ||
			(opcode&0xFFC0 == 0x06C0 && opcode&0x00C0 == 0x00C0) ||
			(opcode&0xF9C0 == 0x08C0 && opcode&0x0600 != 0) ||
			opcode&0xFF00 == 0x0E00 ||
			(opcode&0xF0F8 == 0x50F8 && opcode&7 >= 2 && opcode&7 <= 4) ||
			opcode&0xF9C0 == 0x00C0 || opcode&0xF000 == 0xF000 {
			row.jitPath = "helper"
		} else {
			row.jitPath = "native"
		}
	}
	return row
}

func m68kInventoryMemoryEAForms() []struct {
	name string
	mode uint16
	reg  uint16
	ext  []uint16
} {
	return []struct {
		name string
		mode uint16
		reg  uint16
		ext  []uint16
	}{
		{name: "A2ind", mode: M68K_AM_AR_IND, reg: 2},
		{name: "d16A2", mode: M68K_AM_AR_DISP, reg: 2, ext: []uint16{0x0010}},
		{name: "absW", mode: 7, reg: 0, ext: []uint16{0x3400}},
		{name: "absL", mode: 7, reg: 1, ext: []uint16{0x0003, 0x6000}},
	}
}

func TestM68KJIT_Differential_MULLDIVLImmediateDispIndexBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		objBase = uint32(0x00020000)
		outBase = uint32(0x00030000)
		stack   = uint32(0x00040000)
	)
	runM68KJITDifferentialBlock(t, m68kDiffCase{
		name: "MULLDIVL_immediate_disp_index_block",
		words: []uint16{
			0x222B, 0x0084, // MOVE.L 132(A3),D1
			0x4C3C, 0x1800, 0x0001, 0x5180, // MULS.L #0x00015180,D1
			0xD081,                 // ADD.L D1,D0
			0x723C,                 // MOVEQ #60,D1
			0x4C2B, 0x1800, 0x0088, // MULS.L 136(A3),D1
			0x2040,         // MOVEA.L D0,A0
			0xD1C1,         // ADDA.L D1,A0
			0x202B, 0x008C, // MOVE.L 140(A3),D0
			0x7432,         // MOVEQ #50,D2
			0x4C42, 0x0800, // DIVSL D2,D0:D0
			0x41F0, 0x0930, // LEA 48(A0,D0.L),A0
			0x3F00,         // MOVE.W D0,-(A7)
			0x2948, 0x001C, // MOVE.L A0,28(A4)
			0x2948, 0x0024, // MOVE.L A0,36(A4)
			0x2948, 0x0014, // MOVE.L A0,20(A4)
			0x4E75, // RTS
		},
		setup: func(cpu *M68KCPU) {
			cpu.DataRegs[0] = 0
			cpu.DataRegs[1] = 0xFFFF0001
			cpu.DataRegs[2] = 0x000080C0
			cpu.AddrRegs[3] = objBase
			cpu.AddrRegs[4] = outBase
			cpu.AddrRegs[7] = stack
			cpu.Write32(objBase+0x84, 1)
			cpu.Write32(objBase+0x88, 0x1234)
			cpu.Write32(objBase+0x8C, 1234)
			cpu.Write32(stack, 0x00005000)
		},
		watch: []m68kDiffMemWatch{
			{addr: outBase + 0x14, size: M68K_SIZE_LONG},
			{addr: outBase + 0x1C, size: M68K_SIZE_LONG},
			{addr: outBase + 0x24, size: M68K_SIZE_LONG},
			{addr: stack - 2, size: M68K_SIZE_WORD},
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}, 14)
}

func TestM68KJIT_Differential_EXTBAndSccCallPrepPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		dataBase = uint32(0x00024000)
		stack    = uint32(0x00028040)
	)
	runM68KJITDifferentialBlock(t, m68kDiffCase{
		name: "EXTB_Scc_call_prep_prefix",
		words: []uint16{
			0x4A82,         // TST.L D2
			0x56C0,         // SNE D0
			0x49C0,         // EXTB.L D0
			0x2204,         // MOVE.L D4,D1
			0xD282,         // ADD.L D2,D1
			0xC081,         // AND.L D1,D0
			0x2F00,         // MOVE.L D0,-(A7)
			0x508F,         // ADDQ.L #8,A7
			0x4281,         // CLR.L D1
			0x1213,         // MOVE.B (A3),D1
			0xE189,         // LSL.L #8,D1
			0x4280,         // CLR.L D0
			0x102B, 0x0001, // MOVE.B 1(A3),D0
			0x2F05, // MOVE.L D5,-(A7)
			0x8280, // OR.L D0,D1
			0x2F01, // MOVE.L D1,-(A7)
			0x2F03, // MOVE.L D3,-(A7)
			0x2F0C, // MOVE.L A4,-(A7)
		},
		setup: func(cpu *M68KCPU) {
			cpu.SR = M68K_SR_S
			cpu.DataRegs[0] = 0x00FDC933
			cpu.DataRegs[1] = 0x00FD7C52
			cpu.DataRegs[2] = 0
			cpu.DataRegs[3] = 0x10
			cpu.DataRegs[4] = 0x10
			cpu.DataRegs[5] = 0x11
			cpu.AddrRegs[3] = dataBase
			cpu.AddrRegs[4] = 0x00E59340
			cpu.AddrRegs[7] = stack
			cpu.Write8(dataBase, 0x12)
			cpu.Write8(dataBase+1, 0x34)
			for off := uint32(0); off < 0x40; off += 4 {
				cpu.Write32(stack-0x40+off, 0xA5A50000|off)
			}
		},
		watch: []m68kDiffMemWatch{
			{addr: stack - 0x20, size: M68K_SIZE_LONG},
			{addr: stack - 0x1C, size: M68K_SIZE_LONG},
			{addr: stack - 0x18, size: M68K_SIZE_LONG},
			{addr: stack - 0x14, size: M68K_SIZE_LONG},
			{addr: stack - 0x10, size: M68K_SIZE_LONG},
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}, 18)
}

func m68kDiffCaseIs68020MultiplyDivide(name string) bool {
	return strings.HasPrefix(name, "MULL_") || strings.HasPrefix(name, "DIVL_")
}

func m68kDiffCaseIs68020SpecialData(name string) bool {
	return strings.HasPrefix(name, "CHK_L_") ||
		strings.HasPrefix(name, "PACK_") ||
		strings.HasPrefix(name, "UNPK_")
}

func m68kNativeDiffCase(name string, words ...uint16) m68kDiffCase {
	return m68kDiffCase{
		name:             name,
		words:            words,
		requireProdSafe:  true,
		requireNativeRun: true,
	}
}

// TestM68KJIT_Differential_MOVEMPredecSaveThenStackArgReload reproduces the AROS
// memset routine prologue at 0x0099460C: MOVEM.L D2/D3/D4/A2,-(A7) then reading
// the call arguments back via d16(A7). The native MOVEM predecrement save must
// leave A7 and the stack such that the subsequent 20(A7)/28(A7) reloads return
// the caller's arguments — a JIT-only crash boots AROS with a garbage memset
// length because A0/A1 come back wrong here.
func TestM68KJIT_Differential_MOVEMPredecSaveThenStackArgReload(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const a7 = uint32(0x10000)
	tc := m68kDiffCase{
		name: "MOVEM_D2D3D4A2_predec_then_20A7_28A7_reload",
		words: []uint16{
			0x48E7, 0x3820, // MOVEM.L D2/D3/D4/A2,-(A7)
			0x202F, 0x0014, // MOVE.L 20(A7),D0
			0x206F, 0x001C, // MOVEA.L 28(A7),A0
			0x6002, // BRA.S *+4 (block terminator)
		},
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[7] = a7
			// After MOVEM saves 16 bytes, A7=a7-16; 20(A7)=a7+4, 28(A7)=a7+12.
			cpu.Write32(a7+4, 0x11112222)  // → D0
			cpu.Write32(a7+12, 0x33334444) // → A0
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}
	runM68KJITDifferentialBlock(t, tc, 4)
}

// TestM68KJIT_Differential_MemsetLoopTerminator isolates the loop terminator of
// the AROS memset longword loop (no back-branch): compute D3 = A0 - A2 + A1, then
// CMP.L D3,D4 and a carry-conditional branch. If D3 or the CMP carry is wrong the
// branch flips, which in the real loop means it never exits.
func TestM68KJIT_Differential_MemsetLoopTerminator(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, sub := range []struct {
		name           string
		a0, a1, a2     uint32
		wantBranchBack bool // remaining = A0-(A2-A1) > 4 → BCS taken
	}{
		{"remaining_16_loop", 0x14, 0x2004, 0x2008, true},
		{"remaining_4_exit", 0x14, 0x2000, 0x2010, false},
		{"remaining_0_exit", 0x14, 0x2000, 0x2014, false},
	} {
		sub := sub
		tc := m68kDiffCase{
			name: "memset_loop_terminator_" + sub.name,
			words: []uint16{
				0x2608, // MOVE.L A0,D3
				0x968A, // SUB.L A2,D3
				0xD689, // ADD.L A1,D3
				0x7804, // MOVEQ #4,D4
				0xB883, // CMP.L D3,D4
				0x6502, // BCS.S *+4 (terminator; taken == "keep looping")
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = sub.a0
				cpu.AddrRegs[1] = sub.a1
				cpu.AddrRegs[2] = sub.a2
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		}
		runM68KJITDifferentialBlock(t, tc, 6)
	}
}

// TestM68KJIT_Differential_MemsetEntryBlock reproduces the full entry block of
// the AROS memset (0x0099460C): MOVEM save, reload the three stack arguments,
// set up A1/D1, then BTST/BEQ on the dest alignment. The JIT-only crash feeds
// the longword loop a garbage A0 (length); this checks the entry block preserves
// the arguments exactly like the interpreter.
func TestM68KJIT_Differential_MemsetEntryBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const a7 = uint32(0x10000)
	tc := m68kDiffCase{
		name: "memset_entry_block",
		words: []uint16{
			0x48E7, 0x3820, // MOVEM.L D2/D3/D4/A2,-(A7)
			0x202F, 0x0014, // MOVE.L 20(A7),D0
			0x242F, 0x0018, // MOVE.L 24(A7),D2
			0x206F, 0x001C, // MOVEA.L 28(A7),A0
			0x2240,         // MOVEA.L D0,A1
			0x2209,         // MOVE.L A1,D1
			0x0801, 0x0000, // BTST #0,D1
			0x671A, // BEQ.S (terminator)
		},
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[7] = a7
			cpu.Write32(a7+4, 0x00999D54)  // 20(A7) → D0/A1 (dest, even)
			cpu.Write32(a7+8, 0x000000AA)  // 24(A7) → D2 (fill byte)
			cpu.Write32(a7+12, 0x00000014) // 28(A7) → A0 (length)
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}
	runM68KJITDifferentialBlock(t, tc, 8)
}

// TestM68KJIT_Differential_MemsetLengthCheckBlock reproduces the 0x00994640 block
// of the AROS memset: MOVEQ #4,D4 ; CMP.L A0,D4 ; BCC. A0 holds the length and
// must survive the An-source CMP unchanged for the longword loop that follows.
func TestM68KJIT_Differential_MemsetLengthCheckBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, a0 := range []uint32{0x14, 0x04, 0x03, 0x100000} {
		a0 := a0
		tc := m68kDiffCase{
			name: fmt.Sprintf("memset_length_check_A0_%X", a0),
			words: []uint16{
				0x7804, // MOVEQ #4,D4
				0xB888, // CMP.L A0,D4
				0x6402, // BCC.S *+4 (terminator)
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = a0
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		}
		runM68KJITDifferentialBlock(t, tc, 3)
	}
}

// TestM68KJIT_Diag_MemsetLoopViaDispatcher drives the self-chaining longword loop
// through the real JIT dispatcher (many iterations across chain edges), which the
// single-callNative differential cannot exercise. With A0=0x14 the interpreter
// fills 16 bytes and stops; if the JIT loop runs away under chaining, A2 overruns.
func TestM68KJIT_Diag_MemsetLoopViaDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	cpu := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	cpu.stackLowerBound = 0x00002000
	cpu.stackUpperBound = uint32(len(cpu.memory))
	// loop body (BCS back to start) then STOP to halt the dispatcher.
	m68kDiffWriteProgram(cpu, m68kDiffStartPC,
		0x24C1, 0x2608, 0x968A, 0xD689, 0x7804, 0xB883, 0x65F2, // loop
		0x4E72, 0x2700, // STOP #$2700
	)
	cpu.DataRegs[1] = 0xAAAAAAAA
	cpu.AddrRegs[0] = 0x14
	cpu.AddrRegs[1] = 0x3000
	cpu.AddrRegs[2] = 0x3000
	cpu.SR = M68K_SR_S
	cpu.m68kJitEnabled = true
	cpu.running.Store(true)

	done := make(chan struct{})
	go func() { cpu.M68KExecuteJIT(); close(done) }()
	// The loop fills 16 bytes (length 0x14, longword chunks while remaining > 4)
	// then falls through to the STOP. The STOP idles without an interrupt source,
	// so stop the dispatcher after it has had ample time to run the loop.
	time.Sleep(300 * time.Millisecond)
	cpu.running.Store(false)
	<-done
	t.Logf("after run: A0=%08X A1=%08X A2=%08X PC=%08X instr=%d", cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.PC, cpu.InstructionCount)
	if cpu.AddrRegs[2] != 0x3010 {
		t.Fatalf("longword loop did not terminate correctly: A2=%08X, want 0x3010 (16 bytes filled for length 0x14)", cpu.AddrRegs[2])
	}
	if cpu.AddrRegs[0] != 0x14 {
		t.Fatalf("length A0 corrupted across loop chaining: A0=%08X, want 0x14", cpu.AddrRegs[0])
	}
}

// TestM68KJIT_Diag_MemsetLoopIRQPreempted drives the long self-chaining fill loop
// through the dispatcher with a level-4 interrupt pending, so it is delivered at
// the first 256-instruction sampling boundary (mid-loop). A trivial RTE handler
// returns to the loop. If the native→exception→RTE handoff for a chained high-RAM
// block corrupts the loop's live registers, A0 (length) / A2 (write pointer) come
// back wrong and the fill diverges — the suspected AROS memset crash mechanism.
func TestM68KJIT_Diag_MemsetLoopIRQPreempted(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const (
		dest = uint32(0x00010000)
		size = uint32(0x4000)
	)
	cpu := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	cpu.stackLowerBound = 0x00002000
	cpu.stackUpperBound = uint32(len(cpu.memory))
	m68kDiffWriteProgram(cpu, m68kDiffStartPC,
		0x24C1, 0x2608, 0x968A, 0xD689, 0x7804, 0xB883, 0x65F2, // longword fill loop
		0x4E72, 0x2700, // STOP #$2700
	)
	cpu.Write16(0x2000, 0x4E73)   // level-4 handler: RTE
	cpu.Write32(28*4, 0x00002000) // autovector level 4 → 0x2000
	cpu.AddrRegs[7] = 0x8000      // supervisor stack for the exception frame
	cpu.DataRegs[1] = 0xAAAAAAAA  // fill pattern
	cpu.AddrRegs[0] = size        // length
	cpu.AddrRegs[1] = dest        // dest base
	cpu.AddrRegs[2] = dest        // write pointer
	cpu.SR = M68K_SR_S            // supervisor, IPL mask 0 (allows level 4)
	cpu.m68kJitEnabled = true
	cpu.running.Store(true)
	cpu.AssertInterrupt(4) // pending before run → delivered at first 256-instr boundary (mid-loop)

	done := make(chan struct{})
	go func() { cpu.M68KExecuteJIT(); close(done) }()
	time.Sleep(400 * time.Millisecond)
	cpu.running.Store(false)
	<-done

	wantA2 := dest + size - 4 // fills until remaining (A0-(A2-A1)) <= 4
	t.Logf("after IRQ-preempted run: A0=%08X A1=%08X A2=%08X PC=%08X instr=%d (want A2~%08X)",
		cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.PC, cpu.InstructionCount, wantA2)
	if cpu.AddrRegs[0] != size {
		t.Fatalf("length A0 corrupted by IRQ during chained native loop: A0=%08X, want %08X", cpu.AddrRegs[0], size)
	}
	if cpu.AddrRegs[2] < dest || cpu.AddrRegs[2] > dest+size {
		t.Fatalf("write pointer A2 corrupted by IRQ during chained native loop: A2=%08X (dest=%08X size=%08X)", cpu.AddrRegs[2], dest, size)
	}
}

// m68kAROSMemsetRoutineWords is the complete AROS fill/verify routine at
// 0x0099460C (RTS-bounded), where the JIT-only AROS boot crash surfaces. All
// branches are PC-relative short branches, so it relocates to any base.
var m68kAROSMemsetRoutineWords = []uint16{
	0x48E7, 0x3820, // MOVEM.L D2/D3/D4/A2,-(A7)
	0x202F, 0x0014, // MOVE.L 20(A7),D0
	0x242F, 0x0018, // MOVE.L 24(A7),D2
	0x206F, 0x001C, // MOVEA.L 28(A7),A0
	0x2240,         // MOVEA.L D0,A1
	0x2209,         // MOVE.L A1,D1
	0x0801, 0x0000, // BTST #0,D1
	0x671A,         // BEQ.S +0x1A
	0xB0FC, 0x0000, // CMPA.W #0,A0
	0x660E,         // BNE.S +0x0E
	0x41F1, 0x8800, // LEA 0(A1,A0.L),A0
	0xB1C9,         // CMPM.L (A1)+,(A0)+
	0x6652,         // BNE.S +0x52
	0x4CDF, 0x041C, // MOVEM.L (A7)+,D2/D3/D4/A2
	0x4E75,         // RTS
	0x12C2,         // MOVE.B D2,(A1)+
	0x5388,         // SUBQ.L #1,A0
	0x60DE,         // BRA.S -0x22
	0x7804,         // MOVEQ #4,D4
	0xB888,         // CMP.L A0,D4
	0x64E6,         // BCC.S -0x1A
	0x7600,         // MOVEQ #0,D3
	0x4603,         // NOT.B D3
	0xC682,         // AND.L D2,D3
	0x2203,         // MOVE.L D3,D1
	0xE189,         // LSL.L #8,D1
	0x8283,         // OR.L D3,D1
	0x2601,         // MOVE.L D1,D3
	0x4843,         // SWAP D3
	0x4243,         // CLR.W D3
	0x8283,         // OR.L D3,D1
	0x2449,         // MOVEA.L A1,A2
	0x24C1,         // MOVE.L D1,(A2)+
	0x2608,         // MOVE.L A0,D3
	0x968A,         // SUB.L A2,D3
	0xD689,         // ADD.L A1,D3
	0x7804,         // MOVEQ #4,D4
	0xB883,         // CMP.L D3,D4
	0x65F2,         // BCS.S -0x0C
	0x2208,         // MOVE.L A0,D1
	0x5B81,         // SUBQ.L #5,D1
	0xE489,         // LSR.L #2,D1
	0x2601,         // MOVE.L D1,D3
	0x5283,         // ADDQ.L #1,D3
	0xD683,         // ADD.L D3,D3
	0xD683,         // ADD.L D3,D3
	0xD3C3,         // ADDA.L D3,A1
	0x4481,         // NEG.L D1
	0xD281,         // ADD.L D1,D1
	0xD281,         // ADD.L D1,D1
	0x41F0, 0x18FC, // LEA -4(A0,D1.L),A0
	0x60A6, // BRA.S -0x58
	0x12C2, // MOVE.B D2,(A1)+
	0x60A6, // BRA.S -0x58
}

// runAROSMemsetRoutine runs the relocated memset routine to completion (until it
// RTSes to the STOP sentinel) under either the interpreter or the JIT dispatcher,
// and returns the filled bytes plus the final length register A0. Inputs are
// passed on the stack exactly as the routine reads them (20/24/28(A7) after its
// entry MOVEM saves 16 bytes).
func runAROSMemsetRoutine(t *testing.T, useJIT bool, dest, fill, length uint32) ([]byte, uint32) {
	t.Helper()
	const (
		base   = uint32(0x1000)
		stopPC = uint32(0x5000)
		a7     = uint32(0x9000)
	)
	cpu := newM68KDiffTestProgramCPU(t, base)
	cpu.stackLowerBound = 0x00002000
	cpu.stackUpperBound = uint32(len(cpu.memory))
	m68kDiffWriteProgram(cpu, base, m68kAROSMemsetRoutineWords...)
	cpu.Write16(stopPC, 0x4E72)
	cpu.Write16(stopPC+2, 0x2700) // STOP #$2700 sentinel
	cpu.AddrRegs[7] = a7
	cpu.Write32(a7, stopPC)    // return address
	cpu.Write32(a7+4, dest)    // 20(A7) → D0/A1 (dest)
	cpu.Write32(a7+8, fill)    // 24(A7) → D2 (fill byte)
	cpu.Write32(a7+12, length) // 28(A7) → A0 (length)
	cpu.SR = M68K_SR_S
	cpu.PC = base

	if useJIT {
		cpu.m68kJitEnabled = true
		cpu.running.Store(true)
		done := make(chan struct{})
		go func() { cpu.M68KExecuteJIT(); close(done) }()
		time.Sleep(300 * time.Millisecond)
		cpu.running.Store(false)
		<-done
	} else {
		for i := 0; i < 200000; i++ {
			if cpu.PC == stopPC {
				break
			}
			if cpu.StepOne() == 0 {
				break
			}
		}
	}
	out := make([]byte, length)
	for i := uint32(0); i < length; i++ {
		out[i] = cpu.Read8(dest + i)
	}
	return out, cpu.AddrRegs[0]
}

// TestM68KJIT_Diag_AROSMemsetEnterAtBuild enters the AROS memset routine at the
// build-pattern PC (0x00994646), exactly as the real boot does after the entry
// ran interpreted: the build+longword-loop+tail then run as one chained native
// block. The A0-tracer showed A0 (length 0x14) getting corrupted inside this very
// block in the boot; this drives it through the real dispatcher entered at the
// same PC with the same register state.
func TestM68KJIT_Diag_AROSMemsetEnterAtBuild(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, length := range []uint32{0x14, 0x40, 0x18} {
		length := length
		t.Run(fmt.Sprintf("len_%X", length), func(t *testing.T) {
			run := func(useJIT bool) (uint32, uint32, []byte) {
				const (
					base       = uint32(0x1000)
					buildPC    = base + 0x3A // 0x00994646 - 0x0099460C
					retPC      = base + 0x2C // routine RTS lands here via the entry frame; unused
					dest       = uint32(0x00999D54)
					a7         = uint32(0x9000)
					stopPC     = uint32(0x5000)
					retToFrame = uint32(0x9000)
				)
				_ = retPC
				cpu := newM68KDiffTestProgramCPU(t, base)
				cpu.stackLowerBound = 0x00002000
				cpu.stackUpperBound = uint32(len(cpu.memory))
				m68kDiffWriteProgram(cpu, base, m68kAROSMemsetRoutineWords...)
				cpu.Write16(stopPC, 0x4E72)
				cpu.Write16(stopPC+2, 0x2700)
				// The routine's epilogue does MOVEM.L (A7)+,D2/D3/D4/A2 then RTS.
				// Provide a frame: saved regs (16 bytes) then a return address.
				cpu.AddrRegs[7] = a7
				cpu.Write32(a7+0, 0x11111111)  // saved D2
				cpu.Write32(a7+4, 0x22222222)  // saved D3
				cpu.Write32(a7+8, 0x33333333)  // saved D4
				cpu.Write32(a7+12, 0x44444444) // saved A2
				cpu.Write32(a7+16, stopPC)     // return address → STOP
				_ = retToFrame
				cpu.AddrRegs[0] = length // A0 = length
				cpu.AddrRegs[1] = dest   // A1 = dest
				cpu.AddrRegs[2] = dest   // A2 = dest (build sets A2=A1 anyway)
				cpu.DataRegs[2] = 0      // D2 = fill byte
				cpu.SR = M68K_SR_S
				cpu.PC = buildPC
				if useJIT {
					cpu.m68kJitEnabled = true
					cpu.running.Store(true)
					done := make(chan struct{})
					go func() { cpu.M68KExecuteJIT(); close(done) }()
					time.Sleep(250 * time.Millisecond)
					cpu.running.Store(false)
					<-done
				} else {
					for i := 0; i < 100000; i++ {
						if cpu.PC == stopPC {
							break
						}
						if cpu.StepOne() == 0 {
							break
						}
					}
				}
				out := make([]byte, length)
				for i := uint32(0); i < length; i++ {
					out[i] = cpu.Read8(dest + i)
				}
				return cpu.AddrRegs[0], cpu.PC, out
			}
			wantA0, wantPC, wantB := run(false)
			gotA0, gotPC, gotB := run(true)
			t.Logf("interp: A0=%08X PC=%08X | jit: A0=%08X PC=%08X", wantA0, wantPC, gotA0, gotPC)
			if wantA0 != gotA0 || !bytesEqual(wantB, gotB) {
				t.Fatalf("memset-enter-at-build diverged: interp A0=%08X bytes=% X | jit A0=%08X bytes=% X",
					wantA0, wantB, gotA0, gotB)
			}
		})
	}
}

// TestM68KJIT_Differential_AROSMemsetRoutineFull runs the entire AROS memset
// routine (entry, alignment, longword fill, tail, RTS) through the dispatcher
// and compares the filled region and final A0 against the interpreter oracle.
func TestM68KJIT_Differential_AROSMemsetRoutineFull(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, sub := range []struct {
		name               string
		dest, fill, length uint32
	}{
		{"even_dest_len20", 0x00010000, 0x000000AA, 0x14},
		{"even_dest_len64", 0x00010000, 0x000000AA, 0x40},
		{"odd_dest_len20", 0x00010001, 0x000000AA, 0x14},
		{"len5", 0x00010000, 0x000000AA, 0x05},
		{"len3", 0x00010000, 0x000000AA, 0x03},
		// Exact AROS-crash inputs: dest on the guest stack just above the
		// routine's own frame, fill byte 0, length 0x14.
		{"crash_stackdest_fill0_len20", 0x00999D54, 0x00000000, 0x14},
		{"crash_stackdest_fill0_len64", 0x00999D54, 0x00000000, 0x40},
	} {
		sub := sub
		t.Run(sub.name, func(t *testing.T) {
			wantBytes, wantA0 := runAROSMemsetRoutine(t, false, sub.dest, sub.fill, sub.length)
			gotBytes, gotA0 := runAROSMemsetRoutine(t, true, sub.dest, sub.fill, sub.length)
			if !bytesEqual(wantBytes, gotBytes) {
				t.Fatalf("filled region mismatch:\n interp=% X\n jit   =% X", wantBytes, gotBytes)
			}
			if wantA0 != gotA0 {
				t.Fatalf("final A0 mismatch: interp=%08X jit=%08X", wantA0, gotA0)
			}
		})
	}
}

// TestM68KJIT_Differential_MemsetTailRecomputesA0 isolates the AROS memset tail
// (0x0099466A): it recomputes A0 from the length via SUBQ/LSR/NEG/ADD and a
// LEA -4(A0,D1.L),A0. The A0-across-IRQ tracer (no interrupt involved) showed A0
// flipping from the correct length 0x14 to 0x5D20556E right here. For length 0x14
// the tail must leave A0 = 4 (then a later LEA turns it into the end pointer).
func TestM68KJIT_Differential_MemsetTailRecomputesA0(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, length := range []uint32{0x14, 0x40, 0x09, 0x05} {
		length := length
		tc := m68kDiffCase{
			name: fmt.Sprintf("memset_tail_len_%X", length),
			words: []uint16{
				0x2208,         // MOVE.L A0,D1
				0x5B81,         // SUBQ.L #5,D1
				0xE489,         // LSR.L #2,D1
				0x2601,         // MOVE.L D1,D3
				0x5283,         // ADDQ.L #1,D3
				0xD683,         // ADD.L D3,D3
				0xD683,         // ADD.L D3,D3
				0xD3C3,         // ADDA.L D3,A1
				0x4481,         // NEG.L D1
				0xD281,         // ADD.L D1,D1
				0xD281,         // ADD.L D1,D1
				0x41F0, 0x18FC, // LEA -4(A0,D1.L),A0
				0x6002, // BRA.S *+4 (block terminator, forward)
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = length     // A0 = length
				cpu.AddrRegs[1] = 0x00999D54 // A1 = dest
				cpu.DataRegs[1] = 0xAAAAAAAA // overwritten immediately
				cpu.DataRegs[3] = 0x12345678
			},
			requireProdSafe:  true,
			requireNativeRun: true,
		}
		runM68KJITDifferentialBlock(t, tc, 13)
	}
}

// TestM68KJIT_Differential_MOVEMPostincRestorePreservesA0 reproduces the AROS
// memset epilogue MOVEM.L (A7)+,D2/D3/D4/A2 (mask 0x041C). It restores only
// D2/D3/D4/A2 and must leave A0 (mapped to host R12) untouched. The A0-across-IRQ
// tracer showed A0 flipping from the correct length 0x14 to garbage exactly at
// this instruction (PC=0x00994636), with no interrupt — i.e. the postinc restore
// clobbers R12.
func TestM68KJIT_Differential_MOVEMPostincRestorePreservesA0(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const a7 = uint32(0x9000)
	tc := m68kDiffCase{
		name: "MOVEM_L_A7postinc_D2D3D4A2_preserves_A0",
		words: []uint16{
			0x4CDF, 0x041C, // MOVEM.L (A7)+,D2/D3/D4/A2
			0x4E71, // NOP (block filler)
		},
		setup: func(cpu *M68KCPU) {
			cpu.AddrRegs[0] = 0x00000014   // A0 = length, must be preserved
			cpu.AddrRegs[7] = a7           //
			cpu.Write32(a7+0, 0x5D20556E)  // → D2
			cpu.Write32(a7+4, 0x57D9F2D4)  // → D3
			cpu.Write32(a7+8, 0x00000004)  // → D4
			cpu.Write32(a7+12, 0xDEADBEEF) // → A2
		},
		requireProdSafe:  true,
		requireNativeRun: true,
	}
	runM68KJITDifferentialBlock(t, tc, 2)
}

func runM68KJITDifferentialBlock(t *testing.T, tc m68kDiffCase, wantInstrs int) {
	t.Helper()
	if len(tc.words) == 0 {
		t.Fatal("empty differential case")
	}

	interp := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(interp)
	if tc.setup != nil {
		tc.setup(interp)
	}
	m68kDiffWriteProgram(interp, m68kDiffStartPC, tc.words...)
	for i := 0; i < wantInstrs; i++ {
		if cycles := interp.StepOne(); cycles == 0 {
			t.Fatalf("interpreter stopped at instruction %d", i)
		}
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	if tc.setup != nil {
		tc.setup(jit)
	}
	m68kDiffWriteProgram(jit, m68kDiffStartPC, tc.words...)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	if len(instrs) == 0 {
		t.Fatal("m68kScanBlock returned no instructions")
	}
	if instrs[len(instrs)-1].opcode&0xFFF0 == 0x4E40 {
		instrs = instrs[:len(instrs)-1]
	}
	if len(instrs) != wantInstrs {
		t.Fatalf("differential block decoded %d native instructions, want %d", len(instrs), wantInstrs)
	}
	if tc.requireProdSafe {
		for i := range instrs {
			if !m68kInstrProductionNativeSafe(&instrs[i]) {
				t.Fatalf("opcode %d 0x%04X is not marked production-native safe", i, instrs[i].opcode)
			}
		}
		if !m68kCanUseProductionNativeBlock(jit.memory, m68kDiffStartPC, instrs) {
			t.Fatalf("block is not admitted by production-native gate")
		}
	}

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	jit.PC = rig.ctx.RetPC
	if tc.requireNativeRun && rig.ctx.NeedIOFallback != 0 {
		t.Fatalf("block requested interpreter fallback instead of native execution")
	}

	if jit.PC != interp.PC {
		t.Fatalf("PC mismatch after native RetCount=%d: got=0x%08X want=0x%08X jit[A1]=0x%08X interp[A1]=0x%08X jit[D3]=0x%08X interp[D3]=0x%08X jit[SR]=0x%04X interp[SR]=0x%04X",
			wantInstrs, jit.PC, interp.PC,
			jit.AddrRegs[1], interp.AddrRegs[1],
			jit.DataRegs[3], interp.DataRegs[3],
			jit.SR, interp.SR)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for _, watch := range tc.watch {
		got := m68kDiffReadMem(jit, watch)
		want := m68kDiffReadMem(interp, watch)
		if got != want {
			t.Fatalf("memory[0x%08X].%s mismatch: got=0x%X want=0x%X",
				watch.addr, m68kDiffSizeName(watch.size), got, want)
		}
	}
}

func runM68KJITDifferentialBlockDynamicRetire(t *testing.T, tc m68kDiffCase, staticInstrs int) {
	t.Helper()
	if len(tc.words) == 0 {
		t.Fatal("empty differential case")
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	if tc.setup != nil {
		tc.setup(jit)
	}
	m68kDiffWriteProgram(jit, m68kDiffStartPC, tc.words...)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	if len(instrs) == 0 {
		t.Fatal("m68kScanBlock returned no instructions")
	}
	if instrs[len(instrs)-1].opcode&0xFFF0 == 0x4E40 {
		instrs = instrs[:len(instrs)-1]
	}
	if len(instrs) != staticInstrs {
		t.Fatalf("differential block decoded %d native instructions, want %d", len(instrs), staticInstrs)
	}
	if tc.requireProdSafe {
		for i := range instrs {
			if !m68kInstrProductionNativeSafe(&instrs[i]) {
				t.Fatalf("opcode %d 0x%04X is not marked production-native safe", i, instrs[i].opcode)
			}
		}
		if !m68kCanUseProductionNativeBlock(jit.memory, m68kDiffStartPC, instrs) {
			t.Fatalf("block is not admitted by production-native gate")
		}
	}

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	jit.PC = rig.ctx.RetPC
	if tc.requireNativeRun && rig.ctx.NeedIOFallback != 0 {
		t.Fatalf("block requested interpreter fallback instead of native execution")
	}
	// Use the same accounting the dispatcher uses: a block that exits via a
	// chain bail (e.g. a loop that hits the interrupt-sample boundary) reports
	// its retired count in ChainCount with RetCount==0, so reading RetCount
	// alone undercounts (or reads 0).
	exitSignal := rig.ctx.NeedIOFallback != 0 || rig.ctx.NativeException != 0 || rig.ctx.NeedInval != 0 || rig.ctx.NeedHelper != m68kJITHelperNone
	retired := int(m68kJITRetiredInstructionCount(rig.ctx.RetCount, rig.ctx.ChainCount, block.instrCount, exitSignal))
	if retired <= 0 {
		t.Fatalf("native block reported invalid retired count: RetCount=%d ChainCount=%d", rig.ctx.RetCount, rig.ctx.ChainCount)
	}

	interp := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(interp)
	if tc.setup != nil {
		tc.setup(interp)
	}
	m68kDiffWriteProgram(interp, m68kDiffStartPC, tc.words...)
	for i := 0; i < retired; i++ {
		if cycles := interp.StepOne(); cycles == 0 {
			t.Fatalf("interpreter stopped at instruction %d/%d", i, retired)
		}
	}

	if jit.PC != interp.PC {
		t.Fatalf("PC mismatch after native RetCount=%d: got=0x%08X want=0x%08X jit[A1]=0x%08X interp[A1]=0x%08X jit[D3]=0x%08X interp[D3]=0x%08X jit[SR]=0x%04X interp[SR]=0x%04X",
			retired, jit.PC, interp.PC,
			jit.AddrRegs[1], interp.AddrRegs[1],
			jit.DataRegs[3], interp.DataRegs[3],
			jit.SR, interp.SR)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for _, watch := range tc.watch {
		got := m68kDiffReadMem(jit, watch)
		want := m68kDiffReadMem(interp, watch)
		if got != want {
			t.Fatalf("memory[0x%08X].%s mismatch: got=0x%X want=0x%X",
				watch.addr, m68kDiffSizeName(watch.size), got, want)
		}
	}
}

func runM68KJITDifferentialSingle(t *testing.T, tc m68kDiffCase) {
	t.Helper()
	if len(tc.words) == 0 {
		t.Fatal("empty differential case")
	}

	interp := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(interp)
	if tc.setup != nil {
		tc.setup(interp)
	}
	m68kDiffWriteProgram(interp, m68kDiffStartPC, tc.words...)

	if cycles := interp.StepOne(); cycles == 0 {
		t.Fatalf("interpreter did not execute first opcode 0x%04X", tc.words[0])
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	if tc.setup != nil {
		tc.setup(jit)
	}
	m68kDiffWriteProgram(jit, m68kDiffStartPC, tc.words...)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	if len(instrs) == 0 {
		t.Fatal("m68kScanBlock returned no instructions")
	}
	if instrs[len(instrs)-1].opcode&0xFFF0 == 0x4E40 {
		instrs = instrs[:len(instrs)-1]
	}
	if len(instrs) != 1 {
		t.Fatalf("differential case decoded %d native instructions, want 1", len(instrs))
	}
	if tc.requireProdSafe && !m68kInstrProductionNativeSafe(&instrs[0]) {
		t.Fatalf("opcode 0x%04X is not marked production-native safe", instrs[0].opcode)
	}

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	jit.PC = rig.ctx.RetPC
	if tc.requireNativeRun && rig.ctx.NeedIOFallback != 0 {
		t.Fatalf("opcode 0x%04X requested interpreter fallback instead of native execution: retPC=0x%08X retCount=%d needIOFallback=%d",
			instrs[0].opcode, rig.ctx.RetPC, rig.ctx.RetCount, rig.ctx.NeedIOFallback)
	}

	assertM68KCoreStateEqual(t, jit, interp)
	for _, watch := range tc.watch {
		got := m68kDiffReadMem(jit, watch)
		want := m68kDiffReadMem(interp, watch)
		if got != want {
			t.Fatalf("memory[0x%08X].%s mismatch: got=0x%X want=0x%X",
				watch.addr, m68kDiffSizeName(watch.size), got, want)
		}
	}
}

func runM68KJITDifferentialFPUHelperSingle(t *testing.T, tc m68kFPUDiffCase) {
	t.Helper()
	if len(tc.words) == 0 {
		t.Fatal("empty FPU differential case")
	}

	interp := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(interp)
	if tc.setup != nil {
		tc.setup(interp)
	}
	m68kDiffWriteProgram(interp, m68kDiffStartPC, tc.words...)
	if cycles := interp.StepOne(); cycles == 0 {
		t.Fatalf("interpreter did not execute first FPU opcode 0x%04X", tc.words[0])
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	if tc.setup != nil {
		tc.setup(jit)
	}
	m68kDiffWriteProgram(jit, m68kDiffStartPC, tc.words...)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	if len(instrs) == 0 {
		t.Fatal("m68kScanBlock returned no instructions")
	}
	if instrs[len(instrs)-1].opcode&0xFFF0 == 0x4E40 {
		instrs = instrs[:len(instrs)-1]
	}
	if len(instrs) != 1 {
		t.Fatalf("FPU differential case decoded %d instructions, want 1", len(instrs))
	}
	if !m68kInstrProductionNativeSafe(&instrs[0]) {
		t.Fatalf("FPU opcode 0x%04X is not production-helper-admitted", instrs[0].opcode)
	}

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.USPPtr = uintptr(unsafe.Pointer(&jit.USP))
	rig.ctx.SSPPtr = uintptr(unsafe.Pointer(&jit.SSP))
	rig.ctx.RetPC = 0
	rig.ctx.RetCount = 0
	rig.ctx.NeedIOFallback = 0
	rig.ctx.NeedHelper = m68kJITHelperNone
	rig.ctx.HelperPC = 0

	// Register-to-register arithmetic ops are now emitted inline (native SSE2)
	// rather than routed through the FPU helper. Detect that and assert the
	// appropriate path: native ops update the FP state directly with no helper
	// request; everything else still drives the helper.
	nativeEligible := false
	if len(tc.words) >= 2 {
		if op, _, _, _, ok := m68kDecodeNativeFPURegToReg(tc.words[0], tc.words[1]); ok {
			nativeEligible = m68kFPUNativeOpEmittable(op)
		}
	}

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	jit.PC = rig.ctx.RetPC
	if rig.ctx.NeedIOFallback != 0 {
		t.Fatalf("FPU opcode 0x%04X requested interpreter fallback", instrs[0].opcode)
	}
	if nativeEligible {
		if rig.ctx.NeedHelper != m68kJITHelperNone {
			t.Fatalf("native FPU opcode 0x%04X unexpectedly requested helper %d", instrs[0].opcode, rig.ctx.NeedHelper)
		}
	} else {
		if rig.ctx.NeedHelper != m68kJITHelperFPU {
			t.Fatalf("FPU opcode 0x%04X requested helper %d, want FPU helper", instrs[0].opcode, rig.ctx.NeedHelper)
		}
		if retired, ok := jit.m68kHandleJITHelper(rig.ctx); !ok || retired != 1 {
			t.Fatalf("FPU helper execution ok=%v retired=%d, want ok=true retired=1", ok, retired)
		}
	}

	assertM68KCoreStateEqual(t, jit, interp)
	assertM68KFPUStateEqual(t, jit, interp)
	for _, watch := range tc.watch {
		got := m68kDiffReadMem(jit, watch)
		want := m68kDiffReadMem(interp, watch)
		if got != want {
			t.Fatalf("memory[0x%08X].%s mismatch: got=0x%X want=0x%X",
				watch.addr, m68kDiffSizeName(watch.size), got, want)
		}
	}
}

func runM68KJITDifferentialHelperSingle(t *testing.T, tc m68kDiffCase, wantHelper uint32) {
	t.Helper()
	if len(tc.words) == 0 {
		t.Fatal("empty differential helper case")
	}

	interp := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(interp)
	if tc.setup != nil {
		tc.setup(interp)
	}
	m68kDiffWriteProgram(interp, m68kDiffStartPC, tc.words...)
	if cycles := interp.StepOne(); cycles == 0 {
		t.Fatalf("interpreter did not execute first opcode 0x%04X", tc.words[0])
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	if tc.setup != nil {
		tc.setup(jit)
	}
	m68kDiffWriteProgram(jit, m68kDiffStartPC, tc.words...)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	if len(instrs) == 0 {
		t.Fatal("m68kScanBlock returned no instructions")
	}
	if instrs[len(instrs)-1].opcode&0xFFF0 == 0x4E40 {
		instrs = instrs[:len(instrs)-1]
	}
	if len(instrs) != 1 {
		t.Fatalf("helper differential case decoded %d instructions, want 1", len(instrs))
	}
	if tc.requireProdSafe && !m68kInstrProductionNativeSafe(&instrs[0]) {
		t.Fatalf("opcode 0x%04X is not marked production-helper safe", instrs[0].opcode)
	}

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.USPPtr = uintptr(unsafe.Pointer(&jit.USP))
	rig.ctx.SSPPtr = uintptr(unsafe.Pointer(&jit.SSP))
	rig.ctx.RetPC = 0
	rig.ctx.RetCount = 0
	rig.ctx.NeedIOFallback = 0
	rig.ctx.NeedHelper = m68kJITHelperNone
	rig.ctx.HelperPC = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	jit.PC = rig.ctx.RetPC
	if rig.ctx.NeedIOFallback != 0 {
		t.Fatalf("opcode 0x%04X requested interpreter fallback instead of helper", instrs[0].opcode)
	}
	if rig.ctx.NeedHelper != wantHelper {
		t.Fatalf("opcode 0x%04X requested helper %d, want %d", instrs[0].opcode, rig.ctx.NeedHelper, wantHelper)
	}
	if retired, ok := jit.m68kHandleJITHelper(rig.ctx); !ok || retired != 1 {
		t.Fatalf("helper execution ok=%v retired=%d, want ok=true retired=1", ok, retired)
	}

	assertM68KCoreStateEqual(t, jit, interp)
	for _, watch := range tc.watch {
		got := m68kDiffReadMem(jit, watch)
		want := m68kDiffReadMem(interp, watch)
		if got != want {
			t.Fatalf("memory[0x%08X].%s mismatch: got=0x%X want=0x%X",
				watch.addr, m68kDiffSizeName(watch.size), got, want)
		}
	}
}

func runM68KJITDifferentialSingleInstruction(t *testing.T, tc m68kDiffCase) {
	t.Helper()
	if len(tc.words) == 0 {
		t.Fatal("empty differential case")
	}

	interp := newM68KDiffTestProgramCPU(t, m68kDiffStartPC)
	m68kDiffSetupCPU(interp)
	if tc.setup != nil {
		tc.setup(interp)
	}
	m68kDiffWriteProgram(interp, m68kDiffStartPC, tc.words...)
	if cycles := interp.StepOne(); cycles == 0 {
		t.Fatalf("interpreter did not execute first opcode 0x%04X", tc.words[0])
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	if tc.setup != nil {
		tc.setup(jit)
	}
	m68kDiffWriteProgram(jit, m68kDiffStartPC, tc.words...)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	if len(instrs) == 0 {
		t.Fatal("m68kScanBlock returned no instructions")
	}
	instrs = instrs[:1]
	if tc.requireProdSafe && !m68kInstrProductionNativeSafe(&instrs[0]) {
		t.Fatalf("opcode 0x%04X is not marked production-native safe", instrs[0].opcode)
	}

	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0

	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	jit.PC = rig.ctx.RetPC
	if tc.requireNativeRun && rig.ctx.NeedIOFallback != 0 {
		t.Fatalf("opcode 0x%04X requested interpreter fallback instead of native execution", instrs[0].opcode)
	}

	assertM68KCoreStateEqual(t, jit, interp)
	for _, watch := range tc.watch {
		got := m68kDiffReadMem(jit, watch)
		want := m68kDiffReadMem(interp, watch)
		if got != want {
			t.Fatalf("memory[0x%08X].%s mismatch: got=0x%X want=0x%X",
				watch.addr, m68kDiffSizeName(watch.size), got, want)
		}
	}
}

func m68kDiffSetupCPU(cpu *M68KCPU) {
	cpu.PC = m68kDiffStartPC
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0x10203040 + uint32(i)*0x11111111
	}
	for i := 0; i < 7; i++ {
		cpu.AddrRegs[i] = 0x3000 + uint32(i)*0x100
	}
	cpu.AddrRegs[7] = 0x10000
	for addr := uint32(0x3000); addr < 0x3900; addr += 4 {
		cpu.Write32(addr, 0xA5000000|addr)
	}
}

func m68kDiffSetupSR(sr uint16) func(*M68KCPU) {
	return func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S | (sr & 0x1F)
	}
}

func m68kDiffSetupDataReg(reg uint16, value uint32) func(*M68KCPU) {
	return func(cpu *M68KCPU) {
		cpu.DataRegs[reg] = value
	}
}

func m68kDiffSimpleEASetup(cpu *M68KCPU) {
	const (
		a2Base   = uint32(0x3200)
		a3Base   = uint32(0x3300)
		absShort = uint32(0x3400)
		absLong  = uint32(0x00036000)
	)
	cpu.AddrRegs[2] = a2Base
	cpu.AddrRegs[3] = a3Base
	cpu.Write32(a2Base-4, 0x0F1E2D3C)
	cpu.Write32(a2Base, 0x01020304)
	cpu.Write32(a2Base+0x10, 0x11223344)
	cpu.Write32(a3Base, 0x55667788)
	cpu.Write32(a3Base+0x14, 0x99AABBCC)
	cpu.Write32(absShort, 0x55667788)
	cpu.Write32(absShort+0x20, 0xA5A5A5A5)
	cpu.Write32(absLong, 0x99AABBCC)
	cpu.Write32(absLong+0x20, 0x5A5A5A5A)
}

func m68kDiffBitFieldSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0xF00F1234 ^ (uint32(i) * 0x01020408)
	}
	cpu.DataRegs[1] = 0x000000FF
	cpu.DataRegs[2] = 0x00000A5A
	cpu.DataRegs[3] = 0x00001234
	cpu.DataRegs[4] = 0x89ABCDEF
	cpu.Write32(0x3200, 0x80FF55AA)
	cpu.Write32(0x3210, 0x7F00AA55)
	cpu.Write32(0x3400, 0xCC330FF0)
	cpu.Write32(0x00036000, 0x55AA8001)
}

func m68kDiffBitFieldWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
	}
}

func m68kDiffCHK2CMP2Setup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	cpu.DataRegs[0] = 0x00000011
	cpu.DataRegs[1] = 0x00000022
	cpu.DataRegs[2] = 0x00000033
	cpu.DataRegs[3] = 0x00000044
	cpu.Write32(0x3200, 0x10203040)
	cpu.Write32(0x3210, 0x10203040)
	cpu.Write32(0x3400, 0x10203040)
	cpu.Write32(0x00036000, 0x10203040)
}

func m68kDiffCHK2CMP2Watch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
	}
}

func m68kDiffCHKSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	cpu.DataRegs[0] = 0x00000022
	cpu.DataRegs[1] = 0x00000100
	cpu.Write32(0x3200, 0x01000100)
	cpu.Write32(0x3210, 0x01000100)
	cpu.Write32(0x3204, 0x01000100)
	cpu.Write32(0x3400, 0x01000100)
	cpu.Write32(0x00036000, 0x01000100)
	cpu.Write32(m68kDiffStartPC+2+0x20, 0x01000100)
}

func m68kDiffUnaryEASetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0x10203040 ^ (uint32(i) * 0x01010101)
	}
	cpu.Write32(0x3200, 0x10203040)
	cpu.Write32(0x3210, 0x11223344)
	cpu.Write32(0x31FC, 0x55667788)
	cpu.Write32(0x3400, 0x99AABBCC)
	cpu.Write32(0x00036000, 0x12345678)
	cpu.Write32(m68kDiffStartPC+2+0x20, 0x89ABCDEF)
	cpu.Write32(m68kDiffStartPC+2+0x04, 0x13579BDF)
}

func m68kDiffSccSetup(sr uint16) func(*M68KCPU) {
	return func(cpu *M68KCPU) {
		m68kDiffSimpleEASetup(cpu)
		cpu.SR = sr
		for i := range cpu.DataRegs {
			cpu.DataRegs[i] = 0xA0B0C000 | uint32(i)
		}
		cpu.DataRegs[0] = 0xA0B00000
		cpu.Write32(0x3200, 0x10203040)
		cpu.Write32(0x3210, 0x11223344)
		cpu.Write32(0x31FC, 0x55667788)
		cpu.Write32(0x3204, 0x99AABBCC)
		cpu.Write32(0x3400, 0xDDEEFF00)
		cpu.Write32(0x00036000, 0x12345678)
	}
}

func m68kDiffImmediateSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0x10203040 ^ (uint32(i) * 0x11111111)
	}
	cpu.DataRegs[0] = 0x10200000
	cpu.Write32(0x3200, 0x10203040)
	cpu.Write32(0x3210, 0x11223344)
	cpu.Write32(0x31FC, 0x55667788)
	cpu.Write32(0x3204, 0x99AABBCC)
	cpu.Write32(0x3400, 0xDDEEFF00)
	cpu.Write32(0x00036000, 0x12345678)
	for _, size := range []int{M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG} {
		immBytes := m68kImmediateBytes(size)
		cpu.Write32(m68kDiffStartPC+2+uint32(immBytes)+0x04, 0x13579BDF)
	}
}

func m68kDiffQuickSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0x10203040 ^ (uint32(i) * 0x01010101)
	}
	cpu.DataRegs[0] = 0x10200000
	cpu.AddrRegs[0] = 0x00003000
	cpu.AddrRegs[2] = 0x00003200
	cpu.AddrRegs[7] = 0x00010000
	cpu.Write32(0x3200, 0x10203040)
	cpu.Write32(0x3210, 0x11223344)
	cpu.Write32(0x31FC, 0x55667788)
	cpu.Write32(0x3204, 0x99AABBCC)
	cpu.Write32(0x3400, 0xDDEEFF00)
	cpu.Write32(0x00036000, 0x12345678)
}

func m68kDiffExtendedArithmeticSetup(sr uint16) func(*M68KCPU) {
	return func(cpu *M68KCPU) {
		m68kDiffSimpleEASetup(cpu)
		cpu.SR = sr
		cpu.DataRegs[0] = 0x00000001
		cpu.DataRegs[1] = 0x00000002
		cpu.DataRegs[2] = 0x00007FFF
		cpu.DataRegs[3] = 0x00008000
		cpu.DataRegs[7] = 0xFFFFFFFF
		cpu.AddrRegs[0] = 0x00003104
		cpu.AddrRegs[1] = 0x00003204
		cpu.AddrRegs[2] = 0x00003204
		cpu.AddrRegs[3] = 0x00003304
		cpu.AddrRegs[7] = 0x00010004
		for addr := uint32(0x3100); addr <= 0x3110; addr += 4 {
			cpu.Write32(addr, 0x01020304)
		}
		for addr := uint32(0x3200); addr <= 0x3210; addr += 4 {
			cpu.Write32(addr, 0x10203040)
		}
		for addr := uint32(0x3300); addr <= 0x3310; addr += 4 {
			cpu.Write32(addr, 0x11223344)
		}
		for addr := uint32(0xFFFC); addr <= 0x10008; addr += 4 {
			cpu.Write32(addr, 0x55667788)
		}
	}
}

func m68kDiffExtendedArithmeticWatch() []m68kDiffMemWatch {
	watch := m68kDiffSimpleEAWatch()
	for addr := uint32(0x3100); addr <= 0x3110; addr += 4 {
		watch = append(watch, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	for addr := uint32(0x3200); addr <= 0x3210; addr += 4 {
		watch = append(watch, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	for addr := uint32(0x3300); addr <= 0x3310; addr += 4 {
		watch = append(watch, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	for addr := uint32(0xFFFC); addr <= 0x10008; addr += 4 {
		watch = append(watch, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	return watch
}

func m68kDiffCHKWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
		{addr: m68kDiffStartPC + 2 + 0x20, size: M68K_SIZE_LONG},
	}
}

func m68kDiffCASCAS2Setup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	cpu.DataRegs[0] = 0x00000011
	cpu.DataRegs[1] = 0x00000022
	cpu.DataRegs[2] = 0x00000033
	cpu.DataRegs[3] = 0x00000044
	cpu.AddrRegs[2] = 0x3200
	cpu.AddrRegs[3] = 0x3300
	cpu.Write32(0x3200, 0x00000011)
	cpu.Write32(0x3210, 0x00000011)
	cpu.Write32(0x3300, 0x00000033)
	cpu.Write32(0x3400, 0x00000011)
	cpu.Write32(0x00036000, 0x00000011)
}

func m68kDiffCASCAS2Watch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3300, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
	}
}

func m68kDiffMOVESSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	cpu.SFC = 1
	cpu.DFC = 5
	cpu.DataRegs[1] = 0x55667788
	cpu.Write32(0x3200, 0x11223344)
	cpu.Write32(0x3210, 0x99AABBCC)
	cpu.Write32(0x3400, 0x01020304)
	cpu.Write32(0x00036000, 0xA1B2C3D4)
}

func m68kDiffMOVESWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
	}
}

func m68kDiffTASSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	cpu.DataRegs[0] = 0x12345600
	cpu.DataRegs[4] = 0x00000000
	cpu.AddrRegs[2] = 0x3200
	cpu.Write8(0x31FF, 0x7E)
	cpu.Write8(0x3200, 0x00)
	cpu.Write8(0x3210, 0x7F)
	cpu.Write8(0x3204, 0x80)
	cpu.Write8(0x3400, 0x01)
	cpu.Write8(0x00036000, 0x40)
}

func m68kDiffTASWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x31FC, size: M68K_SIZE_LONG},
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
	}
}

func m68kDiffMOVEPSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	cpu.DataRegs[0] = 0x10203040
	cpu.DataRegs[1] = 0x21314151
	cpu.DataRegs[7] = 0x8797A7B7
	cpu.AddrRegs[0] = 0x3000
	cpu.AddrRegs[2] = 0x3200
	cpu.AddrRegs[6] = 0x3600
	for _, base := range []uint32{0x3000, 0x3200, 0x3600} {
		for off := uint32(0); off < 0x30; off += 4 {
			cpu.Write32(base-0x10+off, 0x55000000|base|off)
		}
	}
}

func m68kDiffMOVEPWatch() []m68kDiffMemWatch {
	var watch []m68kDiffMemWatch
	for _, base := range []uint32{0x3000, 0x3200, 0x3600} {
		for off := uint32(0); off < 0x30; off += 4 {
			watch = append(watch, m68kDiffMemWatch{addr: base - 0x10 + off, size: M68K_SIZE_LONG})
		}
	}
	return watch
}

func m68kDiffNBCDSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_Z
	cpu.DataRegs[0] = 0x12345600
	cpu.DataRegs[1] = 0xABCDEF45
	cpu.DataRegs[7] = 0x76543299
	cpu.AddrRegs[2] = 0x3200
	cpu.Write8(0x31FF, 0x01)
	cpu.Write8(0x3200, 0x45)
	cpu.Write8(0x3210, 0x99)
	cpu.Write8(0x3204, 0x10)
	cpu.Write8(0x3400, 0x00)
	cpu.Write8(0x00036000, 0x38)
}

func m68kDiffNBCDWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x31FC, size: M68K_SIZE_LONG},
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
	}
}

func m68kDiffABCDSBCDSetup(cpu *M68KCPU) {
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_Z
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0x11111100 | uint32([]uint8{0x45, 0x55, 0x99, 0x01, 0x10, 0x90, 0x09, 0x00}[i])
	}
	for i := range cpu.AddrRegs {
		base := uint32(0x3100 + i*0x100)
		cpu.AddrRegs[i] = base + 1
		cpu.Write32(base-4, 0xB0000000|base)
		cpu.Write32(base, 0xC0000000|base)
		cpu.Write32(base+4, 0xD0000000|base)
		cpu.Write8(base, []uint8{0x45, 0x55, 0x99, 0x01, 0x10, 0x90, 0x09, 0x00}[i])
	}
}

func m68kDiffABCDSBCDWatch() []m68kDiffMemWatch {
	var watch []m68kDiffMemWatch
	for i := 0; i < 8; i++ {
		base := uint32(0x3100 + i*0x100)
		watch = append(watch,
			m68kDiffMemWatch{addr: base - 4, size: M68K_SIZE_LONG},
			m68kDiffMemWatch{addr: base, size: M68K_SIZE_LONG},
			m68kDiffMemWatch{addr: base + 4, size: M68K_SIZE_LONG},
		)
	}
	return watch
}

func m68kDiffEXGSetup(cpu *M68KCPU) {
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	for i := 0; i < 8; i++ {
		cpu.DataRegs[i] = 0xD0000000 | uint32(i)*0x01010101 | uint32(i)
		cpu.AddrRegs[i] = 0xA0000000 | uint32(i)*0x00110111 | uint32(7-i)
	}
	cpu.AddrRegs[7] = 0x10000
}

func m68kDiffTRAPccSetup(sr uint16) func(*M68KCPU) {
	return func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S | (sr & 0x1F)
		cpu.AddrRegs[7] = 0x10000
		cpu.Write32(uint32(M68K_VEC_TRAPV)*4, 0x00005000)
		for addr := uint32(0x0FFE0); addr < 0x10010; addr += 4 {
			cpu.Write32(addr, 0xA55A0000|addr)
		}
	}
}

func m68kDiffTRAPccWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x0FFE0, size: M68K_SIZE_LONG},
		{addr: 0x0FFE4, size: M68K_SIZE_LONG},
		{addr: 0x0FFE8, size: M68K_SIZE_LONG},
		{addr: 0x0FFEC, size: M68K_SIZE_LONG},
		{addr: 0x0FFF0, size: M68K_SIZE_LONG},
		{addr: 0x0FFF4, size: M68K_SIZE_LONG},
		{addr: 0x0FFF8, size: M68K_SIZE_LONG},
		{addr: 0x0FFFC, size: M68K_SIZE_LONG},
		{addr: 0x10000, size: M68K_SIZE_LONG},
		{addr: 0x10004, size: M68K_SIZE_LONG},
	}
}

func m68kDiffBKPTSetup(cpu *M68KCPU) {
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_Z
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(uint32(M68K_VEC_BKPT)*4, 0x00005200)
	for addr := uint32(0x0FFE0); addr < 0x10010; addr += 4 {
		cpu.Write32(addr, 0x5AA50000|addr)
	}
}

func m68kDiffBKPTWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x0FFE0, size: M68K_SIZE_LONG},
		{addr: 0x0FFE4, size: M68K_SIZE_LONG},
		{addr: 0x0FFE8, size: M68K_SIZE_LONG},
		{addr: 0x0FFEC, size: M68K_SIZE_LONG},
		{addr: 0x0FFF0, size: M68K_SIZE_LONG},
		{addr: 0x0FFF4, size: M68K_SIZE_LONG},
		{addr: 0x0FFF8, size: M68K_SIZE_LONG},
		{addr: 0x0FFFC, size: M68K_SIZE_LONG},
		{addr: 0x10000, size: M68K_SIZE_LONG},
		{addr: 0x10004, size: M68K_SIZE_LONG},
	}
}

func m68kDiffCALLMSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_Z
	cpu.AddrRegs[7] = 0x10000
	for _, descAddr := range []uint32{0x3200, 0x3210, 0x3400, 0x00036000} {
		cpu.Write16(descAddr, 0x0001)
		cpu.Write16(descAddr+2, 0xC11D)
		cpu.Write32(descAddr+4, 0x00005400)
	}
	for addr := uint32(0x0FFE0); addr < 0x10010; addr += 4 {
		cpu.Write32(addr, 0xC4110000|addr)
	}
}

func m68kDiffCALLMWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3204, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3214, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x3404, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
		{addr: 0x00036004, size: M68K_SIZE_LONG},
		{addr: 0x0FFE0, size: M68K_SIZE_LONG},
		{addr: 0x0FFE4, size: M68K_SIZE_LONG},
		{addr: 0x0FFE8, size: M68K_SIZE_LONG},
		{addr: 0x0FFEC, size: M68K_SIZE_LONG},
		{addr: 0x0FFF0, size: M68K_SIZE_LONG},
		{addr: 0x0FFF4, size: M68K_SIZE_LONG},
		{addr: 0x0FFF8, size: M68K_SIZE_LONG},
		{addr: 0x0FFFC, size: M68K_SIZE_LONG},
		{addr: 0x10000, size: M68K_SIZE_LONG},
		{addr: 0x10004, size: M68K_SIZE_LONG},
	}
}

func m68kDiffRTMSetup(cpu *M68KCPU) {
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_Z
	cpu.AddrRegs[7] = 0x0FF00
	cpu.Write32(0x0FF00, 0x00003302)
	cpu.Write16(0x0FF04, 0x0003)
	cpu.Write16(0x0FF06, 0x0000)
	cpu.Write32(0x0FF08, 0x00005600)
	for addr := uint32(0x0FF0C); addr < 0x0FF20; addr += 4 {
		cpu.Write32(addr, 0xA7700000|addr)
	}
}

func m68kDiffRTMWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x0FF00, size: M68K_SIZE_LONG},
		{addr: 0x0FF04, size: M68K_SIZE_WORD},
		{addr: 0x0FF06, size: M68K_SIZE_WORD},
		{addr: 0x0FF08, size: M68K_SIZE_LONG},
		{addr: 0x0FF0C, size: M68K_SIZE_LONG},
		{addr: 0x0FF10, size: M68K_SIZE_LONG},
		{addr: 0x0FF14, size: M68K_SIZE_LONG},
		{addr: 0x0FF18, size: M68K_SIZE_LONG},
		{addr: 0x0FF1C, size: M68K_SIZE_LONG},
	}
}

func m68kDiffMOVECSetup(cpu *M68KCPU) {
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_Z
	cpu.SFC = 2
	cpu.DFC = 5
	cpu.CACR = 0x01020304
	cpu.CAAR = 0x11121314
	cpu.USP = 0x00012344
	cpu.VBR = 0x00002000
	cpu.MSP = 0x00013000
	cpu.ISP = 0x00014000
	cpu.DataRegs[1] = 0x76543217
	cpu.AddrRegs[2] = 0x89ABCDEF
}

func m68kDiffMOVECWatch() []m68kDiffMemWatch {
	return nil
}

func m68kDiffSpecialDataSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_Z
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0x00001010 + uint32(i)*0x00001111
	}
	cpu.DataRegs[0] = 0x00000035
	cpu.DataRegs[1] = 0x00000024
	for i := range cpu.AddrRegs {
		cpu.AddrRegs[i] = 0x20020 + uint32(i)*0x20
	}
	for addr := uint32(0x20000); addr < 0x20200; addr += 4 {
		cpu.Write32(addr, 0xA5000000|addr)
	}
}

func m68kDiffSpecialDataWatch() []m68kDiffMemWatch {
	watches := make([]m68kDiffMemWatch, 0, 128)
	for addr := uint32(0x20000); addr < 0x20200; addr += 4 {
		watches = append(watches, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	return watches
}

func m68kDiffEffectiveAddressSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.DataRegs[0] = 4
}

func m68kDiffStackFrameSetup(cpu *M68KCPU) {
	cpu.AddrRegs[7] = 0x10000
	for reg := uint32(0); reg < 7; reg++ {
		base := uint32(0x9000 + reg*0x20)
		cpu.AddrRegs[reg] = base
		cpu.Write32(base, 0xABC00000|reg)
	}
	for addr := uint32(0xFFE0); addr < 0x10004; addr += 4 {
		cpu.Write32(addr, 0x5A000000|addr)
	}
}

func m68kDiffStackFrameWatch() []m68kDiffMemWatch {
	watch := []m68kDiffMemWatch{}
	for addr := uint32(0xFFE0); addr < 0x10004; addr += 4 {
		watch = append(watch, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	for reg := uint32(0); reg < 7; reg++ {
		watch = append(watch, m68kDiffMemWatch{addr: 0x9000 + reg*0x20, size: M68K_SIZE_LONG})
	}
	return watch
}

func m68kDiffMOVEMSetup(cpu *M68KCPU) {
	cpu.DataRegs[0] = 0xAAAAAAAA
	cpu.DataRegs[1] = 0xBBBBBBBB
	cpu.DataRegs[2] = 0xCCCCCCCC
	cpu.AddrRegs[0] = 0x3000
	cpu.AddrRegs[2] = 0xEEEEEEEE
	cpu.AddrRegs[3] = 0x9000
	cpu.AddrRegs[7] = 0x10000
	for addr := uint32(0x3000); addr < 0x3920; addr += 4 {
		cpu.Write32(addr, 0x30000000|addr)
	}
	for addr := uint32(0x8FF0); addr < 0x9018; addr += 4 {
		cpu.Write32(addr, 0x90000000|addr)
	}
	for addr := uint32(0xFFC0); addr < 0x10040; addr += 4 {
		cpu.Write32(addr, 0x77000000|addr)
	}
	cpu.Write32(0x10000, 0x11111111)
	cpu.Write32(0x10004, 0x22222222)
	cpu.Write32(0x10008, 0x33333333)
	cpu.Write16(0x10000, 0xFFFE)
	cpu.Write16(0x10002, 0x8000)
}

func m68kDiffMOVEMWatch() []m68kDiffMemWatch {
	watch := []m68kDiffMemWatch{}
	for addr := uint32(0x3000); addr < 0x3920; addr += 4 {
		watch = append(watch, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	for addr := uint32(0x8FF0); addr < 0x9018; addr += 4 {
		watch = append(watch, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	for addr := uint32(0xFFC0); addr < 0x10040; addr += 4 {
		watch = append(watch, m68kDiffMemWatch{addr: addr, size: M68K_SIZE_LONG})
	}
	return watch
}

func m68kDiffPredecrementMoveSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.DataRegs[0] = 0x89ABCDEF
	cpu.AddrRegs[3] = 0x3300
	cpu.AddrRegs[7] = 0x10000
	for addr := uint32(0x32F8); addr <= 0x3304; addr += 4 {
		cpu.Write32(addr, 0x33000000|addr)
	}
	for addr := uint32(0xFFF8); addr <= 0x10004; addr += 4 {
		cpu.Write32(addr, 0x77000000|addr)
	}
}

func m68kDiffPredecrementMoveWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x32F8, size: M68K_SIZE_LONG},
		{addr: 0x32FC, size: M68K_SIZE_LONG},
		{addr: 0x3300, size: M68K_SIZE_LONG},
		{addr: 0xFFF8, size: M68K_SIZE_LONG},
		{addr: 0xFFFC, size: M68K_SIZE_LONG},
		{addr: 0x10000, size: M68K_SIZE_LONG},
	}
}

func m68kDiffPostincrementMemToMemSetup(cpu *M68KCPU) {
	m68kDiffSimpleEASetup(cpu)
	cpu.AddrRegs[2] = 0x3200
	cpu.AddrRegs[3] = 0x3300
	cpu.Write32(0x3200, 0x89ABCDEF)
	cpu.Write32(0x3204, 0x11223344)
	cpu.Write32(0x3300, 0x55667788)
	cpu.Write32(0x3304, 0x99AABBCC)
}

func m68kDiffPostincrementMemToMemWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3204, size: M68K_SIZE_LONG},
		{addr: 0x3300, size: M68K_SIZE_LONG},
		{addr: 0x3304, size: M68K_SIZE_LONG},
	}
}

func m68kDiffSimpleEAWatch() []m68kDiffMemWatch {
	return []m68kDiffMemWatch{
		{addr: 0x31FC, size: M68K_SIZE_LONG},
		{addr: 0x3200, size: M68K_SIZE_LONG},
		{addr: 0x3210, size: M68K_SIZE_LONG},
		{addr: 0x3300, size: M68K_SIZE_LONG},
		{addr: 0x3314, size: M68K_SIZE_LONG},
		{addr: 0x3400, size: M68K_SIZE_LONG},
		{addr: 0x3420, size: M68K_SIZE_LONG},
		{addr: 0x00036000, size: M68K_SIZE_LONG},
		{addr: 0x00036020, size: M68K_SIZE_LONG},
	}
}

func m68kDiffWriteProgram(cpu *M68KCPU, startPC uint32, words ...uint16) {
	writeM68KWords(cpu, startPC, words...)
	pc := startPC + uint32(len(words))*2
	cpu.Write16(pc, 0x4E40) // TRAP #0 terminator for scanner only.
}

func m68kDiffMoveOpcode(size int, srcMode, srcReg, dstMode, dstReg uint16) uint16 {
	group := uint16(0x1000)
	switch size {
	case M68K_SIZE_BYTE:
		group = 0x1000
	case M68K_SIZE_WORD:
		group = 0x3000
	case M68K_SIZE_LONG:
		group = 0x2000
	default:
		panic(fmt.Sprintf("invalid M68K size %d", size))
	}
	return group | dstReg<<9 | dstMode<<6 | srcMode<<3 | srcReg
}

func m68kDiffImmWords(size int, imm uint32) []uint16 {
	switch size {
	case M68K_SIZE_BYTE:
		return []uint16{uint16(imm & 0xFF)}
	case M68K_SIZE_WORD:
		return []uint16{uint16(imm)}
	case M68K_SIZE_LONG:
		return []uint16{uint16(imm >> 16), uint16(imm)}
	default:
		panic(fmt.Sprintf("invalid M68K size %d", size))
	}
}

func m68kDiffRepresentativeImms(size int) []uint32 {
	switch size {
	case M68K_SIZE_BYTE:
		return []uint32{0x00, 0x01, 0x7F, 0x80, 0xFF}
	case M68K_SIZE_WORD:
		return []uint32{0x0000, 0x0001, 0x7FFF, 0x8000, 0xFFFF}
	case M68K_SIZE_LONG:
		return []uint32{0x00000000, 0x00000001, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF}
	default:
		panic(fmt.Sprintf("invalid M68K size %d", size))
	}
}

func m68kDiffCompareImm(size int) uint32 {
	switch size {
	case M68K_SIZE_BYTE:
		return 0x34
	case M68K_SIZE_WORD:
		return 0x0304
	case M68K_SIZE_LONG:
		return 0x01020304
	default:
		panic(fmt.Sprintf("invalid M68K size %d", size))
	}
}

func m68kDiffReadMem(cpu *M68KCPU, watch m68kDiffMemWatch) uint32 {
	switch watch.size {
	case M68K_SIZE_BYTE:
		return uint32(cpu.Read8(watch.addr))
	case M68K_SIZE_WORD:
		return uint32(cpu.Read16(watch.addr))
	case M68K_SIZE_LONG:
		return cpu.Read32(watch.addr)
	default:
		panic(fmt.Sprintf("invalid M68K watch size %d", watch.size))
	}
}

func m68kDiffSizeName(size int) string {
	switch size {
	case M68K_SIZE_BYTE:
		return "B"
	case M68K_SIZE_WORD:
		return "W"
	case M68K_SIZE_LONG:
		return "L"
	default:
		return fmt.Sprintf("size%d", size)
	}
}
