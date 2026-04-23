package main

import "testing"

func TestM68KAudit_MovemLongSparseMixedSaveRestoreStackFrame(t *testing.T) {
	cpu := newAuditM68KCPU()
	cpu.PC = M68K_ENTRY_POINT
	cpu.AddrRegs[7] = 0x00004000

	cpu.DataRegs[2] = 0x00000004
	cpu.AddrRegs[2] = 0x01DFFFF0
	cpu.AddrRegs[3] = 0x0081AE50
	cpu.AddrRegs[6] = 0x00800518

	cpu.Write16(cpu.PC, 0x48E7)
	cpu.Write16(cpu.PC+2, 0x2032)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if got, want := cpu.AddrRegs[7], uint32(0x00003FF0); got != want {
		t.Fatalf("SP after save = %08X, want %08X", got, want)
	}

	checks := []struct {
		addr uint32
		want uint32
	}{
		{0x00003FF0, 0x00000004},
		{0x00003FF4, 0x01DFFFF0},
		{0x00003FF8, 0x0081AE50},
		{0x00003FFC, 0x00800518},
	}
	for _, check := range checks {
		if got := cpu.Read32(check.addr); got != check.want {
			t.Fatalf("save mem[%08X] = %08X, want %08X", check.addr, got, check.want)
		}
	}

	cpu.DataRegs[2] = 0
	cpu.AddrRegs[2] = 0
	cpu.AddrRegs[3] = 0
	cpu.AddrRegs[6] = 0

	cpu.Write16(cpu.PC, 0x4CDF)
	cpu.Write16(cpu.PC+2, 0x4C04)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if got, want := cpu.AddrRegs[7], uint32(0x00004000); got != want {
		t.Fatalf("SP after restore = %08X, want %08X", got, want)
	}
	if got, want := cpu.DataRegs[2], uint32(0x00000004); got != want {
		t.Fatalf("D2 after restore = %08X, want %08X", got, want)
	}
	if got, want := cpu.AddrRegs[2], uint32(0x01DFFFF0); got != want {
		t.Fatalf("A2 after restore = %08X, want %08X", got, want)
	}
	if got, want := cpu.AddrRegs[3], uint32(0x0081AE50); got != want {
		t.Fatalf("A3 after restore = %08X, want %08X", got, want)
	}
	if got, want := cpu.AddrRegs[6], uint32(0x00800518); got != want {
		t.Fatalf("A6 after restore = %08X, want %08X", got, want)
	}
}

func TestM68KAudit_ExecMovem_MemoryToAddressRegisterPostincrementWord_UpdatesBase(t *testing.T) {
	cpu := newAuditM68KCPU()
	cpu.PC = M68K_ENTRY_POINT + 2
	cpu.AddrRegs[2] = 0x00002000
	cpu.Write16(M68K_ENTRY_POINT+2, 0x0800) // MOVEM register mask: A3 only
	cpu.Write16(0x00002000, 0x1234)

	cpu.ExecMovem(M68K_DIRECTION_EA_TO_REG, 0, M68K_AM_AR_POST, 2)

	if got, want := cpu.AddrRegs[2], uint32(0x00002002); got != want {
		t.Fatalf("A2 after MOVEM.W (A2)+,A3 = %08X, want %08X", got, want)
	}
	if got, want := cpu.AddrRegs[3], uint32(0x00001234); got != want {
		t.Fatalf("A3 after MOVEM.W (A2)+,A3 = %08X, want %08X", got, want)
	}
}

func TestM68KAudit_ExecMovem_MemoryToAddressRegisterPostincrementLong_UpdatesBase(t *testing.T) {
	cpu := newAuditM68KCPU()
	cpu.PC = M68K_ENTRY_POINT + 2
	cpu.AddrRegs[2] = 0x00002000
	cpu.Write16(M68K_ENTRY_POINT+2, 0x0800) // MOVEM register mask: A3 only
	cpu.Write32(0x00002000, 0x12345678)

	cpu.ExecMovem(M68K_DIRECTION_EA_TO_REG, 1, M68K_AM_AR_POST, 2)

	if got, want := cpu.AddrRegs[2], uint32(0x00002004); got != want {
		t.Fatalf("A2 after MOVEM.L (A2)+,A3 = %08X, want %08X", got, want)
	}
	if got, want := cpu.AddrRegs[3], uint32(0x12345678); got != want {
		t.Fatalf("A3 after MOVEM.L (A2)+,A3 = %08X, want %08X", got, want)
	}
}
