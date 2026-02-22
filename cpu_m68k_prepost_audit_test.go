package main

import "testing"

func newAuditM68KCPU() *M68KCPU {
	return NewM68KCPU(NewMachineBus())
}

func writeExtWords(cpu *M68KCPU, words ...uint16) {
	for i, w := range words {
		cpu.Write16(cpu.PC+uint32(i*2), w)
	}
}

func TestM68KAudit_ExecScc_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x2001
		cpu.Write8(0x2000, 0x12)
		cpu.Write8(0x2001, 0x34)

		cpu.ExecScc(0, M68K_AM_AR_PRE, 0) // Condition 0 = true

		if cpu.AddrRegs[0] != 0x2000 {
			t.Fatalf("A0=0x%08X want 0x00002000", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x2000); got != 0xFF {
			t.Fatalf("mem[0x2000]=0x%02X want 0xFF", got)
		}
		if got := cpu.Read8(0x2001); got != 0x34 {
			t.Fatalf("mem[0x2001]=0x%02X want 0x34", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x2001
		cpu.Write8(0x2001, 0x34)

		cpu.ExecScc(0, M68K_AM_AR_POST, 0)

		if cpu.AddrRegs[0] != 0x2002 {
			t.Fatalf("A0=0x%08X want 0x00002002", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x2001); got != 0xFF {
			t.Fatalf("mem[0x2001]=0x%02X want 0xFF", got)
		}
	})
}

func TestM68KAudit_ExecNeg_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3001
		cpu.Write8(0x3000, 0x01)
		cpu.Write8(0x3001, 0x22)

		cpu.ExecNeg(M68K_AM_AR_PRE, 0, M68K_SIZE_BYTE)

		if cpu.AddrRegs[0] != 0x3000 {
			t.Fatalf("A0=0x%08X want 0x00003000", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3000); got != 0xFF {
			t.Fatalf("mem[0x3000]=0x%02X want 0xFF", got)
		}
		if got := cpu.Read8(0x3001); got != 0x22 {
			t.Fatalf("mem[0x3001]=0x%02X want 0x22", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3001
		cpu.Write8(0x3001, 0x01)

		cpu.ExecNeg(M68K_AM_AR_POST, 0, M68K_SIZE_BYTE)

		if cpu.AddrRegs[0] != 0x3002 {
			t.Fatalf("A0=0x%08X want 0x00003002", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3001); got != 0xFF {
			t.Fatalf("mem[0x3001]=0x%02X want 0xFF", got)
		}
	})
}

func TestM68KAudit_ExecNegx_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3101
		cpu.Write8(0x3100, 0x00)
		cpu.Write8(0x3101, 0x22)

		cpu.ExecNegx(M68K_AM_AR_PRE, 0, M68K_SIZE_BYTE)

		if cpu.AddrRegs[0] != 0x3100 {
			t.Fatalf("A0=0x%08X want 0x00003100", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3100); got != 0x00 {
			t.Fatalf("mem[0x3100]=0x%02X want 0x00", got)
		}
		if got := cpu.Read8(0x3101); got != 0x22 {
			t.Fatalf("mem[0x3101]=0x%02X want 0x22", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3101
		cpu.Write8(0x3101, 0x00)

		cpu.ExecNegx(M68K_AM_AR_POST, 0, M68K_SIZE_BYTE)

		if cpu.AddrRegs[0] != 0x3102 {
			t.Fatalf("A0=0x%08X want 0x00003102", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3101); got != 0x00 {
			t.Fatalf("mem[0x3101]=0x%02X want 0x00", got)
		}
	})
}

func TestM68KAudit_ExecNot_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3201
		cpu.Write8(0x3200, 0x0F)
		cpu.Write8(0x3201, 0x22)

		cpu.ExecNot(M68K_AM_AR_PRE, 0, M68K_SIZE_BYTE)

		if cpu.AddrRegs[0] != 0x3200 {
			t.Fatalf("A0=0x%08X want 0x00003200", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3200); got != 0xF0 {
			t.Fatalf("mem[0x3200]=0x%02X want 0xF0", got)
		}
		if got := cpu.Read8(0x3201); got != 0x22 {
			t.Fatalf("mem[0x3201]=0x%02X want 0x22", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3201
		cpu.Write8(0x3201, 0x0F)

		cpu.ExecNot(M68K_AM_AR_POST, 0, M68K_SIZE_BYTE)

		if cpu.AddrRegs[0] != 0x3202 {
			t.Fatalf("A0=0x%08X want 0x00003202", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3201); got != 0xF0 {
			t.Fatalf("mem[0x3201]=0x%02X want 0xF0", got)
		}
	})
}

func TestM68KAudit_ExecNbcd_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3301
		cpu.Write8(0x3300, 0x00)
		cpu.Write8(0x3301, 0x22)

		cpu.ExecNbcd(M68K_AM_AR_PRE, 0)

		if cpu.AddrRegs[0] != 0x3300 {
			t.Fatalf("A0=0x%08X want 0x00003300", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3300); got != 0x00 {
			t.Fatalf("mem[0x3300]=0x%02X want 0x00", got)
		}
		if got := cpu.Read8(0x3301); got != 0x22 {
			t.Fatalf("mem[0x3301]=0x%02X want 0x22", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3301
		cpu.Write8(0x3301, 0x00)

		cpu.ExecNbcd(M68K_AM_AR_POST, 0)

		if cpu.AddrRegs[0] != 0x3302 {
			t.Fatalf("A0=0x%08X want 0x00003302", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3301); got != 0x00 {
			t.Fatalf("mem[0x3301]=0x%02X want 0x00", got)
		}
	})
}

func TestM68KAudit_ExecBitManip_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.DataRegs[1] = 0 // bit 0
		cpu.AddrRegs[0] = 0x3401
		cpu.Write8(0x3400, 0x00)
		cpu.Write8(0x3401, 0x22)

		cpu.ExecBitManip(BSET, 1, M68K_AM_AR_PRE, 0)

		if cpu.AddrRegs[0] != 0x3400 {
			t.Fatalf("A0=0x%08X want 0x00003400", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3400); got != 0x01 {
			t.Fatalf("mem[0x3400]=0x%02X want 0x01", got)
		}
		if got := cpu.Read8(0x3401); got != 0x22 {
			t.Fatalf("mem[0x3401]=0x%02X want 0x22", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.DataRegs[1] = 0 // bit 0
		cpu.AddrRegs[0] = 0x3401
		cpu.Write8(0x3401, 0x00)

		cpu.ExecBitManip(BSET, 1, M68K_AM_AR_POST, 0)

		if cpu.AddrRegs[0] != 0x3402 {
			t.Fatalf("A0=0x%08X want 0x00003402", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3401); got != 0x01 {
			t.Fatalf("mem[0x3401]=0x%02X want 0x01", got)
		}
	})
}

func TestM68KAudit_ExecBitManipImm_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.PC = 0x1000
		writeExtWords(cpu, 0x0000) // bit #0
		cpu.AddrRegs[0] = 0x3501
		cpu.Write8(0x3500, 0x00)
		cpu.Write8(0x3501, 0x22)

		cpu.ExecBitManipImm(BSET, M68K_AM_AR_PRE, 0)

		if cpu.AddrRegs[0] != 0x3500 {
			t.Fatalf("A0=0x%08X want 0x00003500", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3500); got != 0x01 {
			t.Fatalf("mem[0x3500]=0x%02X want 0x01", got)
		}
		if got := cpu.Read8(0x3501); got != 0x22 {
			t.Fatalf("mem[0x3501]=0x%02X want 0x22", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.PC = 0x1000
		writeExtWords(cpu, 0x0000) // bit #0
		cpu.AddrRegs[0] = 0x3501
		cpu.Write8(0x3501, 0x00)

		cpu.ExecBitManipImm(BSET, M68K_AM_AR_POST, 0)

		if cpu.AddrRegs[0] != 0x3502 {
			t.Fatalf("A0=0x%08X want 0x00003502", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3501); got != 0x01 {
			t.Fatalf("mem[0x3501]=0x%02X want 0x01", got)
		}
	})
}

func TestM68KAudit_ExecTas_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3601
		cpu.Write8(0x3600, 0x11)
		cpu.Write8(0x3601, 0x22)

		cpu.ExecTas(M68K_AM_AR_PRE, 0)

		if cpu.AddrRegs[0] != 0x3600 {
			t.Fatalf("A0=0x%08X want 0x00003600", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3600); got != 0x91 {
			t.Fatalf("mem[0x3600]=0x%02X want 0x91", got)
		}
		if got := cpu.Read8(0x3601); got != 0x22 {
			t.Fatalf("mem[0x3601]=0x%02X want 0x22", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3601
		cpu.Write8(0x3601, 0x11)

		cpu.ExecTas(M68K_AM_AR_POST, 0)

		if cpu.AddrRegs[0] != 0x3602 {
			t.Fatalf("A0=0x%08X want 0x00003602", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3601); got != 0x91 {
			t.Fatalf("mem[0x3601]=0x%02X want 0x91", got)
		}
	})
}

func TestM68KAudit_ExecBitField_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.PC = 0x1000
		writeExtWords(cpu, 0x0001) // offset=0, width=1
		cpu.AddrRegs[0] = 0x3701
		cpu.Write8(0x3700, 0x00)
		cpu.Write8(0x3701, 0x22)

		cpu.ExecBitField(BFSET, M68K_AM_AR_PRE, 0)

		if cpu.AddrRegs[0] != 0x3700 {
			t.Fatalf("A0=0x%08X want 0x00003700", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3700); got != 0x80 {
			t.Fatalf("mem[0x3700]=0x%02X want 0x80", got)
		}
		if got := cpu.Read8(0x3701); got != 0x22 {
			t.Fatalf("mem[0x3701]=0x%02X want 0x22", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.PC = 0x1000
		writeExtWords(cpu, 0x0001) // offset=0, width=1
		cpu.AddrRegs[0] = 0x3701
		cpu.Write8(0x3701, 0x00)

		cpu.ExecBitField(BFSET, M68K_AM_AR_POST, 0)

		if cpu.AddrRegs[0] != 0x3702 {
			t.Fatalf("A0=0x%08X want 0x00003702", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3701); got != 0x80 {
			t.Fatalf("mem[0x3701]=0x%02X want 0x80", got)
		}
	})
}

func TestM68KAudit_ExecCas_PrePost(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.PC = 0x1000
		writeExtWords(cpu, uint16((2<<6)|1)) // dc=D1, du=D2
		cpu.DataRegs[1] = 0x22               // compare
		cpu.DataRegs[2] = 0x55               // update
		cpu.AddrRegs[0] = 0x3801
		cpu.Write8(0x3800, 0x22)
		cpu.Write8(0x3801, 0x77)

		cpu.ExecCas(M68K_SIZE_BYTE, M68K_AM_AR_PRE, 0)

		if cpu.AddrRegs[0] != 0x3800 {
			t.Fatalf("A0=0x%08X want 0x00003800", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3800); got != 0x55 {
			t.Fatalf("mem[0x3800]=0x%02X want 0x55", got)
		}
		if got := cpu.Read8(0x3801); got != 0x77 {
			t.Fatalf("mem[0x3801]=0x%02X want 0x77", got)
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.PC = 0x1000
		writeExtWords(cpu, uint16((2<<6)|1)) // dc=D1, du=D2
		cpu.DataRegs[1] = 0x22               // compare
		cpu.DataRegs[2] = 0x55               // update
		cpu.AddrRegs[0] = 0x3801
		cpu.Write8(0x3801, 0x22)

		cpu.ExecCas(M68K_SIZE_BYTE, M68K_AM_AR_POST, 0)

		if cpu.AddrRegs[0] != 0x3802 {
			t.Fatalf("A0=0x%08X want 0x00003802", cpu.AddrRegs[0])
		}
		if got := cpu.Read8(0x3801); got != 0x55 {
			t.Fatalf("mem[0x3801]=0x%02X want 0x55", got)
		}
	})
}

func TestM68KAudit_ExecTst_PrePostSideEffects(t *testing.T) {
	t.Run("predecrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3901
		cpu.Write8(0x3900, 0x00)
		cpu.Write8(0x3901, 0x80)

		cpu.ExecTst(0, M68K_AM_AR_PRE, 0) // byte

		if cpu.AddrRegs[0] != 0x3900 {
			t.Fatalf("A0=0x%08X want 0x00003900", cpu.AddrRegs[0])
		}
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Fatalf("Z flag should be set after reading zero byte")
		}
	})

	t.Run("postincrement", func(t *testing.T) {
		cpu := newAuditM68KCPU()
		cpu.AddrRegs[0] = 0x3901
		cpu.Write8(0x3901, 0x00)

		cpu.ExecTst(0, M68K_AM_AR_POST, 0) // byte

		if cpu.AddrRegs[0] != 0x3902 {
			t.Fatalf("A0=0x%08X want 0x00003902", cpu.AddrRegs[0])
		}
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Fatalf("Z flag should be set after reading zero byte")
		}
	})
}
