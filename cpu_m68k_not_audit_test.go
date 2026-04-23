package main

import "testing"

func TestM68KAudit_NotByte_UsesByteSizedNegativeFlag(t *testing.T) {
	cpu := newAuditM68KCPU()
	cpu.PC = M68K_ENTRY_POINT
	cpu.DataRegs[5] = 0x0000007F

	// NOT.B D5
	cpu.Write16(cpu.PC, 0x4605)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if got, want := cpu.DataRegs[5], uint32(0x00000080); got != want {
		t.Fatalf("D5 after NOT.B = %08X, want %08X", got, want)
	}
	if (cpu.SR & M68K_SR_N) == 0 {
		t.Fatal("NOT.B result 0x80 should set N")
	}
	if (cpu.SR & M68K_SR_Z) != 0 {
		t.Fatal("NOT.B result 0x80 should clear Z")
	}
	if (cpu.SR & (M68K_SR_V | M68K_SR_C)) != 0 {
		t.Fatal("NOT must clear V and C")
	}
}
