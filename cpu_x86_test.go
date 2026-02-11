// cpu_x86_test.go - x86 CPU Unit Tests
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import (
	"testing"
)

// TestBus provides a simple bus implementation for testing
type TestX86Bus struct {
	memory [1024 * 1024]byte // 1MB test memory
	ports  [65536]byte       // Port I/O space
}

func NewTestX86Bus() *TestX86Bus {
	return &TestX86Bus{}
}

func (b *TestX86Bus) Read(addr uint32) byte {
	if addr < uint32(len(b.memory)) {
		return b.memory[addr]
	}
	return 0
}

func (b *TestX86Bus) Write(addr uint32, value byte) {
	if addr < uint32(len(b.memory)) {
		b.memory[addr] = value
	}
}

func (b *TestX86Bus) In(port uint16) byte {
	return b.ports[port]
}

func (b *TestX86Bus) Out(port uint16, value byte) {
	b.ports[port] = value
}

func (b *TestX86Bus) Tick(cycles int) {}

// =============================================================================
// Register Access Tests
// =============================================================================

func TestX86_RegisterAccess(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// Test EAX register parts
	cpu.EAX = 0x12345678
	if cpu.AX() != 0x5678 {
		t.Errorf("AX: got 0x%04X, want 0x5678", cpu.AX())
	}
	if cpu.AL() != 0x78 {
		t.Errorf("AL: got 0x%02X, want 0x78", cpu.AL())
	}
	if cpu.AH() != 0x56 {
		t.Errorf("AH: got 0x%02X, want 0x56", cpu.AH())
	}

	// Test setting parts
	cpu.SetAL(0xAB)
	if cpu.EAX != 0x123456AB {
		t.Errorf("SetAL: EAX got 0x%08X, want 0x123456AB", cpu.EAX)
	}

	cpu.SetAH(0xCD)
	if cpu.EAX != 0x1234CDAB {
		t.Errorf("SetAH: EAX got 0x%08X, want 0x1234CDAB", cpu.EAX)
	}

	cpu.SetAX(0x9999)
	if cpu.EAX != 0x12349999 {
		t.Errorf("SetAX: EAX got 0x%08X, want 0x12349999", cpu.EAX)
	}

	// Test register access by index
	cpu.EBX = 0xAABBCCDD
	if cpu.getReg32(3) != 0xAABBCCDD {
		t.Errorf("getReg32(3): got 0x%08X, want 0xAABBCCDD", cpu.getReg32(3))
	}
	if cpu.getReg16(3) != 0xCCDD {
		t.Errorf("getReg16(3): got 0x%04X, want 0xCCDD", cpu.getReg16(3))
	}
	if cpu.getReg8(3) != 0xDD { // BL
		t.Errorf("getReg8(3): got 0x%02X, want 0xDD", cpu.getReg8(3))
	}
	if cpu.getReg8(7) != 0xCC { // BH
		t.Errorf("getReg8(7): got 0x%02X, want 0xCC", cpu.getReg8(7))
	}
}

// =============================================================================
// Flag Tests
// =============================================================================

func TestX86_Flags(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// Test setting individual flags
	cpu.setFlag(x86FlagCF, true)
	if !cpu.CF() {
		t.Error("CF should be set")
	}

	cpu.setFlag(x86FlagZF, true)
	if !cpu.ZF() {
		t.Error("ZF should be set")
	}

	cpu.setFlag(x86FlagCF, false)
	if cpu.CF() {
		t.Error("CF should be clear")
	}

	// Test parity calculation
	if !parity(0x00) {
		t.Error("parity(0x00) should be even")
	}
	if parity(0x01) {
		t.Error("parity(0x01) should be odd")
	}
	if !parity(0x03) { // Two bits set = even
		t.Error("parity(0x03) should be even")
	}
}

// =============================================================================
// Basic Instruction Tests
// =============================================================================

func TestX86_NOP(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	bus.memory[0] = 0x90 // NOP
	bus.memory[1] = 0xF4 // HLT
	cpu.EIP = 0

	cpu.Step()
	if cpu.EIP != 1 {
		t.Errorf("EIP after NOP: got 0x%08X, want 0x00000001", cpu.EIP)
	}
}

func TestX86_MOV_reg_imm(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// MOV EAX, 0x12345678
	bus.memory[0] = 0xB8 // MOV EAX, imm32
	bus.memory[1] = 0x78
	bus.memory[2] = 0x56
	bus.memory[3] = 0x34
	bus.memory[4] = 0x12
	cpu.EIP = 0

	cpu.Step()
	if cpu.EAX != 0x12345678 {
		t.Errorf("MOV EAX, imm32: got 0x%08X, want 0x12345678", cpu.EAX)
	}
	if cpu.EIP != 5 {
		t.Errorf("EIP after MOV: got 0x%08X, want 0x00000005", cpu.EIP)
	}
}

func TestX86_MOV_r8_imm8(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// MOV AL, 0xAB
	bus.memory[0] = 0xB0 // MOV AL, imm8
	bus.memory[1] = 0xAB
	cpu.EIP = 0

	cpu.Step()
	if cpu.AL() != 0xAB {
		t.Errorf("MOV AL, imm8: got 0x%02X, want 0xAB", cpu.AL())
	}
}

func TestX86_ADD(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// ADD AL, 0x10
	cpu.SetAL(0x20)
	bus.memory[0] = 0x04 // ADD AL, imm8
	bus.memory[1] = 0x10
	cpu.EIP = 0

	cpu.Step()
	if cpu.AL() != 0x30 {
		t.Errorf("ADD AL, imm8: got 0x%02X, want 0x30", cpu.AL())
	}
	if cpu.ZF() {
		t.Error("ZF should be clear")
	}
	if cpu.CF() {
		t.Error("CF should be clear")
	}
}

func TestX86_ADD_overflow(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// ADD AL, 0x10 with overflow
	cpu.SetAL(0xF0)
	bus.memory[0] = 0x04 // ADD AL, imm8
	bus.memory[1] = 0x20
	cpu.EIP = 0

	cpu.Step()
	if cpu.AL() != 0x10 {
		t.Errorf("ADD AL with overflow: got 0x%02X, want 0x10", cpu.AL())
	}
	if !cpu.CF() {
		t.Error("CF should be set on overflow")
	}
}

func TestX86_SUB(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// SUB AL, 0x10
	cpu.SetAL(0x30)
	bus.memory[0] = 0x2C // SUB AL, imm8
	bus.memory[1] = 0x10
	cpu.EIP = 0

	cpu.Step()
	if cpu.AL() != 0x20 {
		t.Errorf("SUB AL, imm8: got 0x%02X, want 0x20", cpu.AL())
	}
}

func TestX86_CMP_zero(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// CMP AL, AL (should set ZF)
	cpu.SetAL(0x42)
	bus.memory[0] = 0x3C // CMP AL, imm8
	bus.memory[1] = 0x42
	cpu.EIP = 0

	cpu.Step()
	if !cpu.ZF() {
		t.Error("ZF should be set when comparing equal values")
	}
	if cpu.CF() {
		t.Error("CF should be clear when comparing equal values")
	}
}

func TestX86_XOR_self(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// XOR EAX, EAX (common idiom for zeroing a register)
	cpu.EAX = 0x12345678
	bus.memory[0] = 0x31 // XOR r/m32, r32
	bus.memory[1] = 0xC0 // ModRM: EAX, EAX
	cpu.EIP = 0

	cpu.Step()
	if cpu.EAX != 0 {
		t.Errorf("XOR EAX, EAX: got 0x%08X, want 0x00000000", cpu.EAX)
	}
	if !cpu.ZF() {
		t.Error("ZF should be set after XOR to zero")
	}
}

func TestX86_PUSH_POP(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.ESP = 0x1000
	cpu.EAX = 0xDEADBEEF

	// PUSH EAX
	bus.memory[0] = 0x50 // PUSH EAX
	cpu.EIP = 0
	cpu.Step()

	if cpu.ESP != 0x0FFC {
		t.Errorf("ESP after PUSH: got 0x%08X, want 0x00000FFC", cpu.ESP)
	}

	// POP EBX
	cpu.EBX = 0
	bus.memory[1] = 0x5B // POP EBX
	cpu.Step()

	if cpu.EBX != 0xDEADBEEF {
		t.Errorf("EBX after POP: got 0x%08X, want 0xDEADBEEF", cpu.EBX)
	}
	if cpu.ESP != 0x1000 {
		t.Errorf("ESP after POP: got 0x%08X, want 0x00001000", cpu.ESP)
	}
}

func TestX86_JMP_rel8(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// JMP +5
	bus.memory[0] = 0xEB // JMP rel8
	bus.memory[1] = 0x05 // +5
	cpu.EIP = 0

	cpu.Step()
	if cpu.EIP != 7 { // 2 (instruction length) + 5 (offset)
		t.Errorf("EIP after JMP: got 0x%08X, want 0x00000007", cpu.EIP)
	}
}

func TestX86_JMP_rel8_backward(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// JMP -5 (from address 0x100)
	bus.memory[0x100] = 0xEB // JMP rel8
	bus.memory[0x101] = 0xFB // -5 (signed)
	cpu.EIP = 0x100

	cpu.Step()
	if cpu.EIP != 0xFD { // 0x102 - 5 = 0xFD
		t.Errorf("EIP after backward JMP: got 0x%08X, want 0x000000FD", cpu.EIP)
	}
}

func TestX86_JZ_taken(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.setFlag(x86FlagZF, true)
	bus.memory[0] = 0x74 // JZ rel8
	bus.memory[1] = 0x10 // +16
	cpu.EIP = 0

	cpu.Step()
	if cpu.EIP != 0x12 { // 2 + 16
		t.Errorf("EIP after JZ (taken): got 0x%08X, want 0x00000012", cpu.EIP)
	}
}

func TestX86_JZ_not_taken(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.setFlag(x86FlagZF, false)
	bus.memory[0] = 0x74 // JZ rel8
	bus.memory[1] = 0x10 // +16
	cpu.EIP = 0

	cpu.Step()
	if cpu.EIP != 2 { // Just instruction length, branch not taken
		t.Errorf("EIP after JZ (not taken): got 0x%08X, want 0x00000002", cpu.EIP)
	}
}

func TestX86_CALL_RET(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.ESP = 0x1000

	// CALL +10
	bus.memory[0] = 0xE8 // CALL rel32
	bus.memory[1] = 0x0A
	bus.memory[2] = 0x00
	bus.memory[3] = 0x00
	bus.memory[4] = 0x00
	cpu.EIP = 0

	cpu.Step()
	if cpu.EIP != 0x0F { // 5 + 10
		t.Errorf("EIP after CALL: got 0x%08X, want 0x0000000F", cpu.EIP)
	}
	if cpu.ESP != 0x0FFC {
		t.Errorf("ESP after CALL: got 0x%08X, want 0x00000FFC", cpu.ESP)
	}

	// RET
	bus.memory[0x0F] = 0xC3 // RET
	cpu.Step()

	if cpu.EIP != 5 { // Return address
		t.Errorf("EIP after RET: got 0x%08X, want 0x00000005", cpu.EIP)
	}
}

func TestX86_LOOP(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.ECX = 3
	// LOOP -2 (infinite loop if we didn't decrement)
	bus.memory[0] = 0xE2 // LOOP rel8
	bus.memory[1] = 0xFE // -2
	cpu.EIP = 0

	// First iteration
	cpu.Step()
	if cpu.ECX != 2 {
		t.Errorf("ECX after first LOOP: got %d, want 2", cpu.ECX)
	}
	if cpu.EIP != 0 { // Should jump back
		t.Errorf("EIP after first LOOP: got 0x%08X, want 0x00000000", cpu.EIP)
	}

	// Continue until ECX = 0
	cpu.Step()
	cpu.Step() // ECX should be 0 now, should fall through

	if cpu.ECX != 0 {
		t.Errorf("ECX should be 0, got %d", cpu.ECX)
	}
	if cpu.EIP != 2 { // Should have fallen through
		t.Errorf("EIP after LOOP exit: got 0x%08X, want 0x00000002", cpu.EIP)
	}
}

func TestX86_IN_OUT(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// OUT 0x80, AL
	cpu.SetAL(0x42)
	bus.memory[0] = 0xE6 // OUT imm8, AL
	bus.memory[1] = 0x80
	cpu.EIP = 0

	cpu.Step()
	if bus.ports[0x80] != 0x42 {
		t.Errorf("Port 0x80 after OUT: got 0x%02X, want 0x42", bus.ports[0x80])
	}

	// IN AL, 0x80
	bus.ports[0x80] = 0xAB
	cpu.SetAL(0)
	bus.memory[2] = 0xE4 // IN AL, imm8
	bus.memory[3] = 0x80
	cpu.Step()

	if cpu.AL() != 0xAB {
		t.Errorf("AL after IN: got 0x%02X, want 0xAB", cpu.AL())
	}
}

func TestX86_SHL(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.SetAL(0x01)
	// SHL AL, 1
	bus.memory[0] = 0xD0 // Grp2 Eb, 1
	bus.memory[1] = 0xE0 // ModRM: SHL AL
	cpu.EIP = 0

	cpu.Step()
	if cpu.AL() != 0x02 {
		t.Errorf("SHL AL, 1: got 0x%02X, want 0x02", cpu.AL())
	}
}

func TestX86_SHR(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.SetAL(0x80)
	// SHR AL, 1
	bus.memory[0] = 0xD0 // Grp2 Eb, 1
	bus.memory[1] = 0xE8 // ModRM: SHR AL
	cpu.EIP = 0

	cpu.Step()
	if cpu.AL() != 0x40 {
		t.Errorf("SHR AL, 1: got 0x%02X, want 0x40", cpu.AL())
	}
}

func TestX86_LEA(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.EBX = 0x1000
	cpu.ESI = 0x0100
	// LEA EAX, [EBX+ESI]
	bus.memory[0] = 0x8D // LEA
	bus.memory[1] = 0x04 // ModRM
	bus.memory[2] = 0x33 // SIB: EBX + ESI
	cpu.EIP = 0

	cpu.Step()
	if cpu.EAX != 0x1100 {
		t.Errorf("LEA EAX, [EBX+ESI]: got 0x%08X, want 0x00001100", cpu.EAX)
	}
}

func TestX86_INC_DEC(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.EAX = 0x10
	// INC EAX
	bus.memory[0] = 0x40 // INC EAX
	cpu.EIP = 0

	cpu.Step()
	if cpu.EAX != 0x11 {
		t.Errorf("INC EAX: got 0x%08X, want 0x00000011", cpu.EAX)
	}

	// DEC EAX
	bus.memory[1] = 0x48 // DEC EAX
	cpu.Step()
	if cpu.EAX != 0x10 {
		t.Errorf("DEC EAX: got 0x%08X, want 0x00000010", cpu.EAX)
	}
}

func TestX86_MOVS(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// Setup source data
	bus.memory[0x1000] = 0x11
	bus.memory[0x1001] = 0x22
	bus.memory[0x1002] = 0x33
	bus.memory[0x1003] = 0x44

	cpu.ESI = 0x1000
	cpu.EDI = 0x2000
	cpu.setFlag(x86FlagDF, false) // Forward direction

	// MOVSD (move dword)
	bus.memory[0] = 0xA5 // MOVSW/MOVSD
	cpu.EIP = 0

	cpu.Step()

	// Check destination
	if bus.memory[0x2000] != 0x11 || bus.memory[0x2001] != 0x22 ||
		bus.memory[0x2002] != 0x33 || bus.memory[0x2003] != 0x44 {
		t.Error("MOVSD: data not copied correctly")
	}

	// Check pointers advanced
	if cpu.ESI != 0x1004 {
		t.Errorf("ESI after MOVSD: got 0x%08X, want 0x00001004", cpu.ESI)
	}
	if cpu.EDI != 0x2004 {
		t.Errorf("EDI after MOVSD: got 0x%08X, want 0x00002004", cpu.EDI)
	}
}

func TestX86_STOS(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.EAX = 0xDEADBEEF
	cpu.EDI = 0x2000
	cpu.setFlag(x86FlagDF, false)

	// STOSD
	bus.memory[0] = 0xAB // STOSW/STOSD
	cpu.EIP = 0

	cpu.Step()

	if bus.memory[0x2000] != 0xEF || bus.memory[0x2001] != 0xBE ||
		bus.memory[0x2002] != 0xAD || bus.memory[0x2003] != 0xDE {
		t.Error("STOSD: data not stored correctly")
	}
}

func TestX86_REP_STOSB(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.SetAL(0xFF)
	cpu.EDI = 0x2000
	cpu.ECX = 4
	cpu.setFlag(x86FlagDF, false)

	// REP STOSB
	bus.memory[0] = 0xF3 // REP
	bus.memory[1] = 0xAA // STOSB
	cpu.EIP = 0

	cpu.Step()

	// Check all bytes filled
	for i := range uint32(4) {
		if bus.memory[0x2000+i] != 0xFF {
			t.Errorf("REP STOSB: memory[0x%X] = 0x%02X, want 0xFF", 0x2000+i, bus.memory[0x2000+i])
		}
	}

	// Check ECX is 0
	if cpu.ECX != 0 {
		t.Errorf("ECX after REP STOSB: got %d, want 0", cpu.ECX)
	}
}

func TestX86_MUL(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.SetAL(0x10)
	cpu.SetBL(0x10)
	// MUL BL
	bus.memory[0] = 0xF6 // Grp3 Eb
	bus.memory[1] = 0xE3 // MUL BL
	cpu.EIP = 0

	cpu.Step()
	if cpu.AX() != 0x0100 {
		t.Errorf("MUL BL: AX got 0x%04X, want 0x0100", cpu.AX())
	}
}

func TestX86_DIV(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	cpu.SetAX(0x0064) // 100
	cpu.SetCL(0x0A)   // 10
	// DIV CL
	bus.memory[0] = 0xF6 // Grp3 Eb
	bus.memory[1] = 0xF1 // DIV CL
	cpu.EIP = 0

	cpu.Step()
	if cpu.AL() != 0x0A { // Quotient
		t.Errorf("DIV CL: AL (quotient) got 0x%02X, want 0x0A", cpu.AL())
	}
	if cpu.AH() != 0x00 { // Remainder
		t.Errorf("DIV CL: AH (remainder) got 0x%02X, want 0x00", cpu.AH())
	}
}

// =============================================================================
// Flag Instruction Tests
// =============================================================================

func TestX86_CLC_STC_CMC(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// CLC
	cpu.setFlag(x86FlagCF, true)
	bus.memory[0] = 0xF8 // CLC
	cpu.EIP = 0
	cpu.Step()
	if cpu.CF() {
		t.Error("CLC: CF should be clear")
	}

	// STC
	bus.memory[1] = 0xF9 // STC
	cpu.Step()
	if !cpu.CF() {
		t.Error("STC: CF should be set")
	}

	// CMC
	bus.memory[2] = 0xF5 // CMC
	cpu.Step()
	if cpu.CF() {
		t.Error("CMC: CF should be clear (complement)")
	}
}

func TestX86_CLD_STD(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// CLD
	cpu.setFlag(x86FlagDF, true)
	bus.memory[0] = 0xFC // CLD
	cpu.EIP = 0
	cpu.Step()
	if cpu.DF() {
		t.Error("CLD: DF should be clear")
	}

	// STD
	bus.memory[1] = 0xFD // STD
	cpu.Step()
	if !cpu.DF() {
		t.Error("STD: DF should be set")
	}
}
