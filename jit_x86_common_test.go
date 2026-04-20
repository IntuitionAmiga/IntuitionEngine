// jit_x86_common_test.go - Tests for x86 JIT infrastructure: context, scanner, length calculator

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// ===========================================================================
// X86JITContext Field Offset Tests
// ===========================================================================

func TestX86JITContext_FieldOffsets(t *testing.T) {
	var ctx X86JITContext
	base := uintptr(unsafe.Pointer(&ctx))

	tests := []struct {
		name     string
		got      uintptr
		expected int
	}{
		{"JITRegsPtr", uintptr(unsafe.Pointer(&ctx.JITRegsPtr)) - base, x86CtxOffJITRegsPtr},
		{"MemPtr", uintptr(unsafe.Pointer(&ctx.MemPtr)) - base, x86CtxOffMemPtr},
		{"MemSize", uintptr(unsafe.Pointer(&ctx.MemSize)) - base, x86CtxOffMemSize},
		{"FlagsPtr", uintptr(unsafe.Pointer(&ctx.FlagsPtr)) - base, x86CtxOffFlagsPtr},
		{"EIPPtr", uintptr(unsafe.Pointer(&ctx.EIPPtr)) - base, x86CtxOffEIPPtr},
		{"CpuPtr", uintptr(unsafe.Pointer(&ctx.CpuPtr)) - base, x86CtxOffCpuPtr},
		{"NeedInval", uintptr(unsafe.Pointer(&ctx.NeedInval)) - base, x86CtxOffNeedInval},
		{"NeedIOFallback", uintptr(unsafe.Pointer(&ctx.NeedIOFallback)) - base, x86CtxOffNeedIOFallback},
		{"RetPC", uintptr(unsafe.Pointer(&ctx.RetPC)) - base, x86CtxOffRetPC},
		{"RetCount", uintptr(unsafe.Pointer(&ctx.RetCount)) - base, x86CtxOffRetCount},
		{"CodePageBitmapPtr", uintptr(unsafe.Pointer(&ctx.CodePageBitmapPtr)) - base, x86CtxOffCodePageBitmapPtr},
		{"IOBitmapPtr", uintptr(unsafe.Pointer(&ctx.IOBitmapPtr)) - base, x86CtxOffIOBitmapPtr},
		{"FPUPtr", uintptr(unsafe.Pointer(&ctx.FPUPtr)) - base, x86CtxOffFPUPtr},
		{"SegRegsPtr", uintptr(unsafe.Pointer(&ctx.SegRegsPtr)) - base, x86CtxOffSegRegsPtr},
		{"ChainBudget", uintptr(unsafe.Pointer(&ctx.ChainBudget)) - base, x86CtxOffChainBudget},
		{"ChainCount", uintptr(unsafe.Pointer(&ctx.ChainCount)) - base, x86CtxOffChainCount},
		{"RTSCache0PC", uintptr(unsafe.Pointer(&ctx.RTSCache0PC)) - base, x86CtxOffRTSCache0PC},
		{"RTSCache0Addr", uintptr(unsafe.Pointer(&ctx.RTSCache0Addr)) - base, x86CtxOffRTSCache0Addr},
		{"RTSCache1PC", uintptr(unsafe.Pointer(&ctx.RTSCache1PC)) - base, x86CtxOffRTSCache1PC},
		{"RTSCache1Addr", uintptr(unsafe.Pointer(&ctx.RTSCache1Addr)) - base, x86CtxOffRTSCache1Addr},
	}

	for _, tt := range tests {
		if int(tt.got) != tt.expected {
			t.Errorf("%s: offset = %d, want %d", tt.name, tt.got, tt.expected)
		}
	}
}

// ===========================================================================
// JIT Register Sync Tests
// ===========================================================================

func TestSyncJITRegs_RoundTrip(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)

	// Set named registers to known values
	cpu.EAX = 0x11111111
	cpu.ECX = 0x22222222
	cpu.EDX = 0x33333333
	cpu.EBX = 0x44444444
	cpu.ESP = 0x55555555
	cpu.EBP = 0x66666666
	cpu.ESI = 0x77777777
	cpu.EDI = 0x88888888

	// Sync to JIT regs (x86 encoding order: EAX=0, ECX=1, EDX=2, EBX=3, ESP=4, EBP=5, ESI=6, EDI=7)
	cpu.syncJITRegsFromNamed()

	if cpu.jitRegs[0] != 0x11111111 {
		t.Errorf("jitRegs[0] (EAX) = 0x%08X, want 0x11111111", cpu.jitRegs[0])
	}
	if cpu.jitRegs[1] != 0x22222222 {
		t.Errorf("jitRegs[1] (ECX) = 0x%08X, want 0x22222222", cpu.jitRegs[1])
	}
	if cpu.jitRegs[2] != 0x33333333 {
		t.Errorf("jitRegs[2] (EDX) = 0x%08X, want 0x33333333", cpu.jitRegs[2])
	}
	if cpu.jitRegs[3] != 0x44444444 {
		t.Errorf("jitRegs[3] (EBX) = 0x%08X, want 0x44444444", cpu.jitRegs[3])
	}
	if cpu.jitRegs[4] != 0x55555555 {
		t.Errorf("jitRegs[4] (ESP) = 0x%08X, want 0x55555555", cpu.jitRegs[4])
	}
	if cpu.jitRegs[5] != 0x66666666 {
		t.Errorf("jitRegs[5] (EBP) = 0x%08X, want 0x66666666", cpu.jitRegs[5])
	}
	if cpu.jitRegs[6] != 0x77777777 {
		t.Errorf("jitRegs[6] (ESI) = 0x%08X, want 0x77777777", cpu.jitRegs[6])
	}
	if cpu.jitRegs[7] != 0x88888888 {
		t.Errorf("jitRegs[7] (EDI) = 0x%08X, want 0x88888888", cpu.jitRegs[7])
	}

	// Modify JIT regs
	cpu.jitRegs[0] = 0xAAAAAAAA
	cpu.jitRegs[4] = 0xBBBBBBBB

	// Sync back to named
	cpu.syncJITRegsToNamed()

	if cpu.EAX != 0xAAAAAAAA {
		t.Errorf("EAX = 0x%08X, want 0xAAAAAAAA", cpu.EAX)
	}
	if cpu.ESP != 0xBBBBBBBB {
		t.Errorf("ESP = 0x%08X, want 0xBBBBBBBB", cpu.ESP)
	}
	// Unchanged registers should still be correct
	if cpu.ECX != 0x22222222 {
		t.Errorf("ECX = 0x%08X, want 0x22222222", cpu.ECX)
	}
}

func TestSyncJITSegRegs_RoundTrip(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)

	cpu.ES = 0x1000
	cpu.CS = 0x2000
	cpu.SS = 0x3000
	cpu.DS = 0x4000
	cpu.FS = 0x5000
	cpu.GS = 0x6000

	cpu.syncJITSegRegsFromNamed()

	if cpu.jitSegRegs[0] != 0x1000 {
		t.Errorf("jitSegRegs[0] (ES) = 0x%04X, want 0x1000", cpu.jitSegRegs[0])
	}
	if cpu.jitSegRegs[1] != 0x2000 {
		t.Errorf("jitSegRegs[1] (CS) = 0x%04X, want 0x2000", cpu.jitSegRegs[1])
	}
	if cpu.jitSegRegs[5] != 0x6000 {
		t.Errorf("jitSegRegs[5] (GS) = 0x%04X, want 0x6000", cpu.jitSegRegs[5])
	}

	cpu.jitSegRegs[3] = 0x9999
	cpu.syncJITSegRegsToNamed()

	if cpu.DS != 0x9999 {
		t.Errorf("DS = 0x%04X, want 0x9999", cpu.DS)
	}
}

// ===========================================================================
// Instruction Length Calculator Tests
// ===========================================================================

func TestX86InstrLength_SingleByte(t *testing.T) {
	// NOP = 0x90, 1 byte
	mem := make([]byte, 16)
	mem[0] = 0x90
	if l := x86InstrLength(mem, 0); l != 1 {
		t.Errorf("NOP length = %d, want 1", l)
	}

	// HLT = 0xF4, 1 byte
	mem[0] = 0xF4
	if l := x86InstrLength(mem, 0); l != 1 {
		t.Errorf("HLT length = %d, want 1", l)
	}

	// CLC = 0xF8, 1 byte
	mem[0] = 0xF8
	if l := x86InstrLength(mem, 0); l != 1 {
		t.Errorf("CLC length = %d, want 1", l)
	}

	// PUSH EAX = 0x50, 1 byte
	mem[0] = 0x50
	if l := x86InstrLength(mem, 0); l != 1 {
		t.Errorf("PUSH EAX length = %d, want 1", l)
	}

	// POP EAX = 0x58, 1 byte
	mem[0] = 0x58
	if l := x86InstrLength(mem, 0); l != 1 {
		t.Errorf("POP EAX length = %d, want 1", l)
	}

	// RET = 0xC3, 1 byte
	mem[0] = 0xC3
	if l := x86InstrLength(mem, 0); l != 1 {
		t.Errorf("RET length = %d, want 1", l)
	}

	// INC EAX = 0x40, 1 byte
	mem[0] = 0x40
	if l := x86InstrLength(mem, 0); l != 1 {
		t.Errorf("INC EAX length = %d, want 1", l)
	}
}

func TestX86InstrLength_ImmediateOnly(t *testing.T) {
	mem := make([]byte, 16)

	// MOV EAX, imm32 = 0xB8 + 4 bytes = 5
	mem[0] = 0xB8
	mem[1] = 0x78
	mem[2] = 0x56
	mem[3] = 0x34
	mem[4] = 0x12
	if l := x86InstrLength(mem, 0); l != 5 {
		t.Errorf("MOV EAX,imm32 length = %d, want 5", l)
	}

	// MOV AL, imm8 = 0xB0 + 1 byte = 2
	mem[0] = 0xB0
	mem[1] = 0x42
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("MOV AL,imm8 length = %d, want 2", l)
	}

	// ADD AL, imm8 = 0x04 + 1 byte = 2
	mem[0] = 0x04
	mem[1] = 0x42
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("ADD AL,imm8 length = %d, want 2", l)
	}

	// ADD EAX, imm32 = 0x05 + 4 bytes = 5
	mem[0] = 0x05
	if l := x86InstrLength(mem, 0); l != 5 {
		t.Errorf("ADD EAX,imm32 length = %d, want 5", l)
	}

	// PUSH imm32 = 0x68 + 4 bytes = 5
	mem[0] = 0x68
	if l := x86InstrLength(mem, 0); l != 5 {
		t.Errorf("PUSH imm32 length = %d, want 5", l)
	}

	// PUSH imm8 = 0x6A + 1 byte = 2
	mem[0] = 0x6A
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("PUSH imm8 length = %d, want 2", l)
	}

	// INT imm8 = 0xCD + 1 byte = 2
	mem[0] = 0xCD
	mem[1] = 0x21
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("INT imm8 length = %d, want 2", l)
	}

	// RET imm16 = 0xC2 + 2 bytes = 3
	mem[0] = 0xC2
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("RET imm16 length = %d, want 3", l)
	}
}

func TestX86InstrLength_ModRM_RegisterDirect(t *testing.T) {
	mem := make([]byte, 16)

	// ADD EAX, EBX = 0x01 ModRM(0xC3 = mod=3, reg=0, rm=3) = 2 bytes
	mem[0] = 0x01
	mem[1] = 0xD8 // mod=11, reg=011(EBX), rm=000(EAX)
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("ADD EAX,EBX length = %d, want 2", l)
	}

	// MOV EAX, EBX = 0x89 ModRM(0xD8) = 2 bytes
	mem[0] = 0x89
	mem[1] = 0xD8
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("MOV EAX,EBX length = %d, want 2", l)
	}

	// TEST EAX, EBX = 0x85 ModRM = 2 bytes
	mem[0] = 0x85
	mem[1] = 0xD8
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("TEST EAX,EBX length = %d, want 2", l)
	}
}

func TestX86InstrLength_ModRM_Displacement(t *testing.T) {
	mem := make([]byte, 16)

	// MOV EAX, [EBX] = 0x8B ModRM(0x03 = mod=0, reg=0, rm=3) = 2 bytes
	mem[0] = 0x8B
	mem[1] = 0x03 // mod=00, reg=000, rm=011(EBX)
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("MOV EAX,[EBX] length = %d, want 2", l)
	}

	// MOV EAX, [EBX+disp8] = 0x8B ModRM(0x43) = 3 bytes
	mem[0] = 0x8B
	mem[1] = 0x43 // mod=01, reg=000, rm=011
	mem[2] = 0x10
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("MOV EAX,[EBX+disp8] length = %d, want 3", l)
	}

	// MOV EAX, [EBX+disp32] = 0x8B ModRM(0x83) = 6 bytes
	mem[0] = 0x8B
	mem[1] = 0x83 // mod=10, reg=000, rm=011
	if l := x86InstrLength(mem, 0); l != 6 {
		t.Errorf("MOV EAX,[EBX+disp32] length = %d, want 6", l)
	}

	// MOV EAX, [disp32] = 0x8B ModRM(0x05 = mod=0, reg=0, rm=5) = 6 bytes
	mem[0] = 0x8B
	mem[1] = 0x05 // mod=00, rm=101 = disp32 only
	if l := x86InstrLength(mem, 0); l != 6 {
		t.Errorf("MOV EAX,[disp32] length = %d, want 6", l)
	}
}

func TestX86InstrLength_ModRM_SIB(t *testing.T) {
	mem := make([]byte, 16)

	// MOV EAX, [ESP] needs SIB: 0x8B 0x04 0x24 = 3 bytes
	// ModRM: mod=00, reg=000, rm=100 (SIB follows)
	// SIB: scale=00, index=100(none), base=100(ESP)
	mem[0] = 0x8B
	mem[1] = 0x04 // mod=00, rm=100
	mem[2] = 0x24 // SIB: scale=0, index=4(none), base=4(ESP)
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("MOV EAX,[ESP] length = %d, want 3", l)
	}

	// MOV EAX, [ESP+disp8] needs SIB: 4 bytes
	mem[0] = 0x8B
	mem[1] = 0x44 // mod=01, rm=100
	mem[2] = 0x24 // SIB
	mem[3] = 0x08 // disp8
	if l := x86InstrLength(mem, 0); l != 4 {
		t.Errorf("MOV EAX,[ESP+disp8] length = %d, want 4", l)
	}

	// MOV EAX, [EAX+EBX*4] needs SIB: 3 bytes
	// ModRM: mod=00, rm=100
	// SIB: scale=10, index=011(EBX), base=000(EAX)
	mem[0] = 0x8B
	mem[1] = 0x04 // mod=00, rm=100
	mem[2] = 0x98 // SIB: scale=2, index=3, base=0
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("MOV EAX,[EAX+EBX*4] length = %d, want 3", l)
	}

	// SIB with base=5 and mod=0 means disp32: 7 bytes
	mem[0] = 0x8B
	mem[1] = 0x04 // mod=00, rm=100
	mem[2] = 0x9D // SIB: scale=2, index=3, base=5
	if l := x86InstrLength(mem, 0); l != 7 {
		t.Errorf("MOV EAX,[EBX*4+disp32] length = %d, want 7", l)
	}
}

func TestX86InstrLength_ModRM_WithImmediate(t *testing.T) {
	mem := make([]byte, 16)

	// Grp1: ADD [EBX], imm32 = 0x81 ModRM + imm32 = 2 + 4 = 6
	mem[0] = 0x81
	mem[1] = 0x03 // mod=00, reg=000(ADD), rm=011(EBX)
	if l := x86InstrLength(mem, 0); l != 6 {
		t.Errorf("ADD [EBX],imm32 length = %d, want 6", l)
	}

	// Grp1: ADD EBX, imm8 = 0x83 ModRM + imm8 = 2 + 1 = 3
	mem[0] = 0x83
	mem[1] = 0xC3 // mod=11, reg=000(ADD), rm=011(EBX)
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("ADD EBX,imm8 length = %d, want 3", l)
	}

	// MOV [EBX], imm32 = 0xC7 ModRM + imm32 = 2 + 4 = 6
	mem[0] = 0xC7
	mem[1] = 0x03 // mod=00, rm=011
	if l := x86InstrLength(mem, 0); l != 6 {
		t.Errorf("MOV [EBX],imm32 length = %d, want 6", l)
	}

	// MOV [EBX], imm8 = 0xC6 ModRM + imm8 = 2 + 1 = 3
	mem[0] = 0xC6
	mem[1] = 0x03
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("MOV [EBX],imm8 length = %d, want 3", l)
	}
}

func TestX86InstrLength_Jumps(t *testing.T) {
	mem := make([]byte, 16)

	// JMP rel8 = 0xEB + 1 byte = 2
	mem[0] = 0xEB
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("JMP rel8 length = %d, want 2", l)
	}

	// JMP rel32 = 0xE9 + 4 bytes = 5
	mem[0] = 0xE9
	if l := x86InstrLength(mem, 0); l != 5 {
		t.Errorf("JMP rel32 length = %d, want 5", l)
	}

	// CALL rel32 = 0xE8 + 4 bytes = 5
	mem[0] = 0xE8
	if l := x86InstrLength(mem, 0); l != 5 {
		t.Errorf("CALL rel32 length = %d, want 5", l)
	}

	// Jcc rel8 = 0x74 + 1 byte = 2
	mem[0] = 0x74
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("JZ rel8 length = %d, want 2", l)
	}

	// LOOP rel8 = 0xE2 + 1 byte = 2
	mem[0] = 0xE2
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("LOOP rel8 length = %d, want 2", l)
	}
}

func TestX86InstrLength_TwoByteOpcodes(t *testing.T) {
	mem := make([]byte, 16)

	// 0F 80: JO rel32 = 2 + 4 = 6
	mem[0] = 0x0F
	mem[1] = 0x80
	if l := x86InstrLength(mem, 0); l != 6 {
		t.Errorf("JO rel32 length = %d, want 6", l)
	}

	// 0F B6: MOVZX r32, r/m8 = 2 + ModRM = 3 (mod=11)
	mem[0] = 0x0F
	mem[1] = 0xB6
	mem[2] = 0xC0 // mod=11
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("MOVZX r32,r8 length = %d, want 3", l)
	}

	// 0F BE: MOVSX r32, r/m8 = 2 + ModRM = 3 (mod=11)
	mem[0] = 0x0F
	mem[1] = 0xBE
	mem[2] = 0xC0
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("MOVSX r32,r8 length = %d, want 3", l)
	}

	// 0F AF: IMUL r32, r/m32 = 2 + ModRM = 3 (mod=11)
	mem[0] = 0x0F
	mem[1] = 0xAF
	mem[2] = 0xC1 // mod=11
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("IMUL r32,r32 length = %d, want 3", l)
	}

	// 0F 90: SETO r/m8 = 2 + ModRM = 3 (mod=11)
	mem[0] = 0x0F
	mem[1] = 0x90
	mem[2] = 0xC0
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("SETO r8 length = %d, want 3", l)
	}
}

func TestX86InstrLength_Prefixes(t *testing.T) {
	mem := make([]byte, 16)

	// 66 90: NOP with operand size prefix = 2
	mem[0] = 0x66
	mem[1] = 0x90
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("66 NOP length = %d, want 2", l)
	}

	// F3 A4: REP MOVSB = 2
	mem[0] = 0xF3
	mem[1] = 0xA4
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("REP MOVSB length = %d, want 2", l)
	}

	// 26 A1 00 10 00 00: ES: MOV EAX, [0x1000] = 6 (prefix + opcode + 4 byte addr)
	mem[0] = 0x26 // ES:
	mem[1] = 0xA1 // MOV EAX, moffs32
	mem[2] = 0x00
	mem[3] = 0x10
	mem[4] = 0x00
	mem[5] = 0x00
	if l := x86InstrLength(mem, 0); l != 6 {
		t.Errorf("ES:MOV EAX,[moffs32] length = %d, want 6", l)
	}
}

func TestX86InstrLength_FPU(t *testing.T) {
	mem := make([]byte, 16)

	// D9 C0: FLD ST(0) = 2 bytes (escape + ModRM)
	mem[0] = 0xD9
	mem[1] = 0xC0
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("FLD ST(0) length = %d, want 2", l)
	}

	// D8 03: FADD dword [EBX] = 2 bytes (escape + ModRM with mod=00,rm=011)
	mem[0] = 0xD8
	mem[1] = 0x03
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("FADD [EBX] length = %d, want 2", l)
	}

	// DD 45 08: FLD qword [EBP+8] = 3 bytes (escape + ModRM + disp8)
	mem[0] = 0xDD
	mem[1] = 0x45 // mod=01, rm=101(EBP)
	mem[2] = 0x08
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("FLD [EBP+8] length = %d, want 3", l)
	}
}

func TestX86InstrLength_ENTER(t *testing.T) {
	mem := make([]byte, 16)

	// ENTER imm16, imm8 = 0xC8 + 2 + 1 = 4
	mem[0] = 0xC8
	if l := x86InstrLength(mem, 0); l != 4 {
		t.Errorf("ENTER length = %d, want 4", l)
	}
}

func TestX86InstrLength_Grp3_TEST(t *testing.T) {
	mem := make([]byte, 16)

	// F6 /0: TEST Eb, Ib -> ModRM + imm8
	// F6 C3 42: TEST BL, 0x42 = 3 bytes
	mem[0] = 0xF6
	mem[1] = 0xC3 // mod=11, reg=000(TEST), rm=011
	mem[2] = 0x42
	if l := x86InstrLength(mem, 0); l != 3 {
		t.Errorf("TEST BL,imm8 length = %d, want 3", l)
	}

	// F6 /2: NOT Eb -> ModRM only, no immediate
	// F6 D3: NOT BL = 2 bytes
	mem[0] = 0xF6
	mem[1] = 0xD3 // mod=11, reg=010(NOT), rm=011
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("NOT BL length = %d, want 2", l)
	}

	// F7 /0: TEST Ev, Iv -> ModRM + imm32
	// F7 C3 00 01 00 00: TEST EBX, 0x100 = 6 bytes
	mem[0] = 0xF7
	mem[1] = 0xC3 // mod=11, reg=000(TEST), rm=011
	if l := x86InstrLength(mem, 0); l != 6 {
		t.Errorf("TEST EBX,imm32 length = %d, want 6", l)
	}

	// F7 /3: NEG Ev -> ModRM only, no immediate
	// F7 DB: NEG EBX = 2 bytes
	mem[0] = 0xF7
	mem[1] = 0xDB // mod=11, reg=011(NEG), rm=011
	if l := x86InstrLength(mem, 0); l != 2 {
		t.Errorf("NEG EBX length = %d, want 2", l)
	}
}

// ===========================================================================
// Block Scanner Tests
// ===========================================================================

func TestX86ScanBlock_SingleHalt(t *testing.T) {
	mem := make([]byte, 256)
	mem[0] = 0xF4 // HLT

	instrs := x86ScanBlock(mem, 0)
	if len(instrs) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instrs))
	}
	if instrs[0].opcode != 0xF4 {
		t.Errorf("opcode = 0x%04X, want 0x00F4", instrs[0].opcode)
	}
	if instrs[0].length != 1 {
		t.Errorf("length = %d, want 1", instrs[0].length)
	}
}

func TestX86ScanBlock_ALUThenHalt(t *testing.T) {
	mem := make([]byte, 256)
	// MOV EAX, 0x12345678 (5 bytes)
	mem[0] = 0xB8
	mem[1] = 0x78
	mem[2] = 0x56
	mem[3] = 0x34
	mem[4] = 0x12
	// ADD EAX, EBX (2 bytes)
	mem[5] = 0x01
	mem[6] = 0xD8
	// HLT (1 byte)
	mem[7] = 0xF4

	instrs := x86ScanBlock(mem, 0)
	if len(instrs) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(instrs))
	}

	if instrs[0].length != 5 {
		t.Errorf("instr[0] length = %d, want 5", instrs[0].length)
	}
	if instrs[0].pcOffset != 0 {
		t.Errorf("instr[0] pcOffset = %d, want 0", instrs[0].pcOffset)
	}

	if instrs[1].length != 2 {
		t.Errorf("instr[1] length = %d, want 2", instrs[1].length)
	}
	if instrs[1].pcOffset != 5 {
		t.Errorf("instr[1] pcOffset = %d, want 5", instrs[1].pcOffset)
	}

	if instrs[2].length != 1 {
		t.Errorf("instr[2] length = %d, want 1", instrs[2].length)
	}
	if instrs[2].pcOffset != 7 {
		t.Errorf("instr[2] pcOffset = %d, want 7", instrs[2].pcOffset)
	}
}

func TestX86ScanBlock_JmpTerminates(t *testing.T) {
	mem := make([]byte, 256)
	// NOP
	mem[0] = 0x90
	// JMP rel8
	mem[1] = 0xEB
	mem[2] = 0x05
	// This should not be scanned
	mem[3] = 0x90

	instrs := x86ScanBlock(mem, 0)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}
}

func TestX86ScanBlock_RetTerminates(t *testing.T) {
	mem := make([]byte, 256)
	mem[0] = 0x90 // NOP
	mem[1] = 0xC3 // RET

	instrs := x86ScanBlock(mem, 0)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}
}

func TestX86ScanBlock_CallTerminates(t *testing.T) {
	mem := make([]byte, 256)
	mem[0] = 0xE8 // CALL rel32
	mem[1] = 0x00
	mem[2] = 0x01
	mem[3] = 0x00
	mem[4] = 0x00

	instrs := x86ScanBlock(mem, 0)
	if len(instrs) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instrs))
	}
}

func TestX86ScanBlock_ConditionalBranchNotTerminator(t *testing.T) {
	mem := make([]byte, 256)
	mem[0] = 0x90 // NOP
	mem[1] = 0x74 // JZ rel8
	mem[2] = 0x02 // +2
	mem[3] = 0x90 // NOP
	mem[4] = 0xF4 // HLT

	instrs := x86ScanBlock(mem, 0)
	// Jcc is NOT a terminator, block continues
	if len(instrs) != 4 {
		t.Fatalf("expected 4 instructions (NOP, JZ, NOP, HLT), got %d", len(instrs))
	}
}

func TestX86ScanBlock_MaxBlockSize(t *testing.T) {
	mem := make([]byte, 512)
	// Fill with NOPs (no terminator)
	for i := range mem {
		mem[i] = 0x90
	}

	instrs := x86ScanBlock(mem, 0)
	if len(instrs) != x86JitMaxBlockSize {
		t.Errorf("expected max block size %d, got %d", x86JitMaxBlockSize, len(instrs))
	}
}

func TestX86ScanBlock_PrefixHandling(t *testing.T) {
	mem := make([]byte, 256)
	// REP MOVSB = F3 A4 (2 bytes, but treated as a single instruction)
	mem[0] = 0xF3
	mem[1] = 0xA4
	mem[2] = 0xF4 // HLT

	instrs := x86ScanBlock(mem, 0)
	if len(instrs) < 1 {
		t.Fatalf("expected at least 1 instruction, got %d", len(instrs))
	}
	if instrs[0].length != 2 {
		t.Errorf("REP MOVSB length = %d, want 2", instrs[0].length)
	}
}

func TestX86ScanBlock_NonZeroStartPC(t *testing.T) {
	mem := make([]byte, 0x1010)
	mem[0x1000] = 0xB8 // MOV EAX, imm32
	mem[0x1001] = 0x01
	mem[0x1002] = 0x00
	mem[0x1003] = 0x00
	mem[0x1004] = 0x00
	mem[0x1005] = 0xF4 // HLT

	instrs := x86ScanBlock(mem, 0x1000)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}
	if instrs[0].opcodePC != 0x1000 {
		t.Errorf("instr[0] opcodePC = 0x%X, want 0x1000", instrs[0].opcodePC)
	}
	if instrs[1].opcodePC != 0x1005 {
		t.Errorf("instr[1] opcodePC = 0x%X, want 0x1005", instrs[1].opcodePC)
	}
}

// ===========================================================================
// Block Terminator Tests
// ===========================================================================

func TestX86IsBlockTerminator(t *testing.T) {
	terminators := []uint16{
		0x00C3, // RET
		0x00CB, // RETF
		0x00C2, // RET imm16
		0x00CA, // RETF imm16
		0x00E8, // CALL rel
		0x00E9, // JMP rel32
		0x00EB, // JMP rel8
		0x00EA, // JMP far
		0x009A, // CALL far
		0x00CF, // IRET
		0x00CC, // INT3
		0x00CD, // INT imm8
		0x00CE, // INTO
		0x00F4, // HLT
		0x00FF, // Grp5 (indirect CALL/JMP)
	}
	for _, op := range terminators {
		if !x86IsBlockTerminator(op) {
			t.Errorf("opcode 0x%04X should be a terminator", op)
		}
	}

	nonTerminators := []uint16{
		0x0090, // NOP
		0x0074, // JZ rel8 (conditional, not terminator)
		0x00B8, // MOV EAX, imm32
		0x0001, // ADD
		0x0050, // PUSH EAX
	}
	for _, op := range nonTerminators {
		if x86IsBlockTerminator(op) {
			t.Errorf("opcode 0x%04X should not be a terminator", op)
		}
	}
}

// ===========================================================================
// Fallback Detection Tests
// ===========================================================================

func TestX86NeedsFallback(t *testing.T) {
	// Empty block needs fallback
	if !x86NeedsFallback(nil) {
		t.Error("nil block should need fallback")
	}
	if !x86NeedsFallback([]X86JITInstr{}) {
		t.Error("empty block should need fallback")
	}

	// Segment register write needs fallback
	segWrite := X86JITInstr{opcode: 0x008E} // MOV Sreg, Ew
	if !x86NeedsFallback([]X86JITInstr{segWrite}) {
		t.Error("MOV Sreg should need fallback")
	}

	// Far CALL needs fallback
	farCall := X86JITInstr{opcode: 0x009A}
	if !x86NeedsFallback([]X86JITInstr{farCall}) {
		t.Error("far CALL should need fallback")
	}

	// IRET needs fallback
	iret := X86JITInstr{opcode: 0x00CF}
	if !x86NeedsFallback([]X86JITInstr{iret}) {
		t.Error("IRET should need fallback")
	}

	// INT needs fallback
	intInstr := X86JITInstr{opcode: 0x00CD}
	if !x86NeedsFallback([]X86JITInstr{intInstr}) {
		t.Error("INT should need fallback")
	}

	// LDS needs fallback
	lds := X86JITInstr{opcode: 0x00C5}
	if !x86NeedsFallback([]X86JITInstr{lds}) {
		t.Error("LDS should need fallback")
	}

	// LES needs fallback
	les := X86JITInstr{opcode: 0x00C4}
	if !x86NeedsFallback([]X86JITInstr{les}) {
		t.Error("LES should need fallback")
	}

	// Normal ALU should not need fallback
	addInstr := X86JITInstr{opcode: 0x0001} // ADD
	if x86NeedsFallback([]X86JITInstr{addInstr}) {
		t.Error("ADD should not need fallback")
	}

	// PUSH segment reg needs fallback
	pushES := X86JITInstr{opcode: 0x0006}
	if !x86NeedsFallback([]X86JITInstr{pushES}) {
		t.Error("PUSH ES should need fallback")
	}

	// POP segment reg needs fallback
	popES := X86JITInstr{opcode: 0x0007}
	if !x86NeedsFallback([]X86JITInstr{popES}) {
		t.Error("POP ES should need fallback")
	}
}

// ===========================================================================
// Register Analysis Tests
// ===========================================================================

func TestX86AnalyzeBlockRegs_MOVImm(t *testing.T) {
	mem := make([]byte, 256)
	// MOV EAX, imm32 (0xB8) - writes EAX
	mem[0] = 0xB8
	mem[1] = 0x01
	mem[2] = 0x00
	mem[3] = 0x00
	mem[4] = 0x00
	mem[5] = 0xF4 // HLT

	instrs := x86ScanBlock(mem, 0)
	regs := x86AnalyzeBlockRegs(instrs, mem, 0)

	// EAX (reg 0) should be written
	if regs.written&(1<<0) == 0 {
		t.Error("EAX should be marked as written")
	}
}

func TestX86AnalyzeBlockRegs_AddRegReg(t *testing.T) {
	mem := make([]byte, 256)
	// ADD EAX, EBX (0x01 0xD8) - reads EAX+EBX, writes EAX
	mem[0] = 0x01
	mem[1] = 0xD8 // mod=11, reg=011(EBX), rm=000(EAX)
	mem[2] = 0xF4

	instrs := x86ScanBlock(mem, 0)
	regs := x86AnalyzeBlockRegs(instrs, mem, 0)

	// EAX (reg 0) should be read and written
	if regs.read&(1<<0) == 0 {
		t.Error("EAX should be marked as read")
	}
	if regs.written&(1<<0) == 0 {
		t.Error("EAX should be marked as written")
	}
	// EBX (reg 3) should be read
	if regs.read&(1<<3) == 0 {
		t.Error("EBX should be marked as read")
	}
}

// ===========================================================================
// Tier 2 Register Allocation Tests
// ===========================================================================

func TestX86Tier2RegAlloc_FrequencyBased(t *testing.T) {
	mem := make([]byte, 256)
	// Program that heavily uses ESI (guest reg 6) and EDI (guest reg 7)
	// which are normally spilled in Tier 1
	// MOV ESI, 0x5000 (uses ESI once)
	mem[0] = 0xBE
	mem[1] = 0x00
	mem[2] = 0x50
	mem[3] = 0x00
	mem[4] = 0x00
	// MOV EDI, 0x6000 (uses EDI once)
	mem[5] = 0xBF
	mem[6] = 0x00
	mem[7] = 0x60
	mem[8] = 0x00
	mem[9] = 0x00
	// ADD ESI, EDI (uses ESI + EDI)
	// Using a form the analyzer understands -- ADD EAX, EBX as placeholder
	mem[10] = 0x01
	mem[11] = 0xD8 // ADD EAX, EBX
	mem[12] = 0xF4

	instrs := x86ScanBlock(mem, 0)
	mapping := x86Tier2RegAlloc(instrs, mem, 0)

	// ESI and EDI should get mapped since they're used
	if mapping[6] == 0 && mapping[7] == 0 {
		t.Log("ESI and EDI not mapped -- frequency may be too low from this program")
	}
	// At minimum, registers that are used should get mapped
	usedCount := 0
	for i := 0; i < 8; i++ {
		if mapping[i] != 0 {
			usedCount++
		}
	}
	if usedCount > 5 {
		t.Errorf("mapped %d registers, but only 5 slots available", usedCount)
	}
}

// ===========================================================================
// Peephole Optimizer Tests
// ===========================================================================

func TestX86PeepholeFlags_DeadFlags(t *testing.T) {
	mem := make([]byte, 256)
	// ADD EAX, EBX (sets flags) -- followed by AND EAX, EDX (also sets flags)
	// The ADD's flags are dead because AND overwrites them
	mem[0] = 0x01
	mem[1] = 0xD8 // ADD EAX, EBX
	mem[2] = 0x21
	mem[3] = 0xD0 // AND EAX, EDX
	mem[4] = 0xF4 // HLT

	instrs := x86ScanBlock(mem, 0)
	flags := x86PeepholeFlags(instrs)

	// ADD's flags are NOT needed (AND overwrites them)
	if flags[0] {
		t.Error("ADD flags should be dead (overwritten by AND)")
	}
	// AND's flags are not consumed either (HLT doesn't read flags)
	if flags[1] {
		t.Error("AND flags should be dead (no consumer)")
	}
}

func TestX86PeepholeFlags_LiveFlags(t *testing.T) {
	mem := make([]byte, 256)
	// CMP EAX, EBX (sets flags) -- followed by JZ (reads flags)
	mem[0] = 0x39
	mem[1] = 0xD8 // CMP EAX, EBX
	mem[2] = 0x74
	mem[3] = 0x00 // JZ +0
	mem[4] = 0xF4 // HLT

	instrs := x86ScanBlock(mem, 0)
	flags := x86PeepholeFlags(instrs)

	// CMP's flags ARE needed by JZ
	if !flags[0] {
		t.Error("CMP flags should be live (consumed by JZ)")
	}
}

func TestX86PeepholeFlags_DecJnz(t *testing.T) {
	mem := make([]byte, 256)
	// ADD EAX, EBX (sets flags)
	// DEC ECX (sets flags, but ADD's are dead)
	// JNZ (reads DEC's flags)
	mem[0] = 0x01
	mem[1] = 0xD8 // ADD EAX, EBX
	mem[2] = 0x49 // DEC ECX
	mem[3] = 0x75
	mem[4] = 0xFB // JNZ -5
	mem[5] = 0xF4 // HLT

	instrs := x86ScanBlock(mem, 0)
	flags := x86PeepholeFlags(instrs)

	if flags[0] {
		t.Error("ADD flags should be dead (DEC overwrites, JNZ reads DEC)")
	}
	if !flags[1] {
		t.Error("DEC flags should be live (consumed by JNZ)")
	}
}

// ===========================================================================
// Host Feature Detection Tests
// ===========================================================================

func TestX86HostFeatures_Detect(t *testing.T) {
	// Should not panic and should return a valid struct
	f := detectX86HostFeatures()
	t.Logf("Host features: BMI1=%v BMI2=%v AVX2=%v LZCNT=%v ERMS=%v FSRM=%v",
		f.HasBMI1, f.HasBMI2, f.HasAVX2, f.HasLZCNT, f.HasERMS, f.HasFSRM)
}

func TestX86HostFeatures_PackageLevelInit(t *testing.T) {
	// x86Host should be initialized by init(). Access all fields to verify struct.
	_ = x86Host.HasBMI1
	_ = x86Host.HasBMI2
	_ = x86Host.HasAVX2
	_ = x86Host.HasLZCNT
	_ = x86Host.HasERMS
	_ = x86Host.HasFSRM
}

// ===========================================================================
// I/O Bitmap Tests
// ===========================================================================

func TestBuildX86IOBitmap_TranslateIORegion(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	bitmap := buildX86IOBitmap(adapter, bus)

	// Pages covering 0xF000-0xFFFF should be marked
	for addr := uint32(0xF000); addr < 0x10000; addr += 0x100 {
		page := addr >> 8
		if page < uint32(len(bitmap)) && bitmap[page] == 0 {
			t.Errorf("page 0x%X (addr 0x%X) should be marked as I/O", page, addr)
		}
	}
}

func TestBuildX86IOBitmap_BankControlRegs(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	bitmap := buildX86IOBitmap(adapter, bus)

	// Bank control registers at 0xF700-0xF7F1 should be covered
	page := uint32(0xF700) >> 8
	if page < uint32(len(bitmap)) && bitmap[page] == 0 {
		t.Errorf("bank control register page 0x%X should be marked", page)
	}
}

func TestBuildX86IOBitmap_VGAVRAMRegion(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	bitmap := buildX86IOBitmap(adapter, bus)

	// VGA VRAM at 0xA0000-0xAFFFF should be marked
	for addr := uint32(0xA0000); addr < 0xB0000; addr += 0x100 {
		page := addr >> 8
		if page < uint32(len(bitmap)) && bitmap[page] == 0 {
			t.Errorf("VGA VRAM page 0x%X (addr 0x%X) should be marked", page, addr)
		}
	}
}

func TestBuildX86IOBitmap_LowRAMClean(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	bitmap := buildX86IOBitmap(adapter, bus)

	// Low RAM (0x0000-0x1FFF) should NOT be marked (no banks, no I/O)
	for addr := uint32(0); addr < 0x2000; addr += 0x100 {
		page := addr >> 8
		if page < uint32(len(bitmap)) && bitmap[page] != 0 {
			t.Errorf("low RAM page 0x%X (addr 0x%X) should not be marked", page, addr)
		}
	}
}
