package main

import "testing"

func newMoveATestCPU() *M68KCPU {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.Write32(0, M68K_STACK_START)
	cpu.Write32(M68K_RESET_VECTOR, M68K_ENTRY_POINT)
	cpu.PC = M68K_ENTRY_POINT
	cpu.AddrRegs[7] = M68K_STACK_START
	cpu.SSP = M68K_STACK_START
	cpu.USP = M68K_STACK_START
	cpu.stackLowerBound = 0
	cpu.stackUpperBound = M68K_MEMORY_SIZE
	return cpu
}

func TestM68K_MOVEAL_AddressDisplacementToAddressRegister(t *testing.T) {
	cpu := newMoveATestCPU()

	// Mirror the failing AROS pattern:
	//   movea.l 14(a3),a1
	//   movea.l 18(a3),a2
	cpu.AddrRegs[3] = 0x00002000
	cpu.Write32(cpu.AddrRegs[3]+14, 0x0086A010)
	cpu.Write32(cpu.AddrRegs[3]+18, 0x0072BEA0)

	cpu.Write16(cpu.PC+0, 0x226B)
	cpu.Write16(cpu.PC+2, 0x000E)
	cpu.Write16(cpu.PC+4, 0x246B)
	cpu.Write16(cpu.PC+6, 0x0012)

	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()
	if cpu.AddrRegs[1] != 0x0086A010 {
		t.Fatalf("first MOVEA.L wrote A1=%08X, want %08X", cpu.AddrRegs[1], 0x0086A010)
	}
	if cpu.PC != M68K_ENTRY_POINT+4 {
		t.Fatalf("after first MOVEA.L PC=%08X, want %08X", cpu.PC, M68K_ENTRY_POINT+4)
	}

	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()
	if cpu.AddrRegs[2] != 0x0072BEA0 {
		t.Fatalf("second MOVEA.L wrote A2=%08X, want %08X", cpu.AddrRegs[2], 0x0072BEA0)
	}
	if cpu.PC != M68K_ENTRY_POINT+8 {
		t.Fatalf("after second MOVEA.L PC=%08X, want %08X", cpu.PC, M68K_ENTRY_POINT+8)
	}
}
