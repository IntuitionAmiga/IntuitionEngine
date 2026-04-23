package main

import "testing"

func TestM68KAudit_StepOne_MullLong_PostincrementSourceAdvancesBase(t *testing.T) {
	cpu := newAuditM68KCPU()
	cpu.PC = M68K_ENTRY_POINT
	cpu.SR = M68K_SR_S | 0x0700
	cpu.AddrRegs[2] = 0x00002000
	cpu.AddrRegs[7] = 0x00004000

	// MULL.L (A2)+,<ext=0x0800>. With D0 initially zero, the result stays zero.
	// The regression is that the long source operand must still postincrement A2 by 4.
	cpu.Write16(cpu.PC, 0x4C1A)
	cpu.Write16(cpu.PC+2, 0x0800)
	cpu.Write32(0x00002000, 0x12345678)

	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if got, want := cpu.PC, uint32(M68K_ENTRY_POINT+4); got != want {
		t.Fatalf("PC after MULL.L = %08X, want %08X", got, want)
	}
	if got, want := cpu.AddrRegs[2], uint32(0x00002004); got != want {
		t.Fatalf("A2 after MULL.L (A2)+ = %08X, want %08X", got, want)
	}
	if got := cpu.DataRegs[0]; got != 0 {
		t.Fatalf("D0 after zero multiplicand MULL.L = %08X, want 00000000", got)
	}
}
