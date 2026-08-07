//go:build arm64 && linux

package main

import "testing"

func TestZ80JITARM64NativeNOPMarker(t *testing.T) {
	if !z80JitAvailable {
		t.Fatal("ARM64 Z80 JIT is unavailable")
	}
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x00) // NOP, emitted natively on ARM64
	bus.Write8(0x0101, 0x76) // HALT, explicit helper boundary
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted {
		t.Fatal("ARM64 Z80 JIT did not halt")
	}
	if got := cpu.jitStats.nativeEntries.Load(); got == 0 {
		t.Fatal("ARM64 Z80 JIT did not execute an emitted native block")
	}
	if got := cpu.R & 0x7F; got != 2 {
		t.Fatalf("ARM64 emitted NOP plus HALT R = %02x, want 02", got)
	}
}

func TestZ80JITARM64NativeLDAImmediate(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x3E) // LD A,$42
	bus.Write8(0x0101, 0x42)
	bus.Write8(0x0102, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := cpu.A; got != 0x42 {
		t.Fatalf("ARM64 emitted LD A,n result = %02x, want 42", got)
	}
	if got := cpu.jitStats.nativeEntries.Load(); got == 0 {
		t.Fatal("ARM64 LD A,n did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeLDBImmediate(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	bus.Write8(0x0100, 0x06) // LD B,$24
	bus.Write8(0x0101, 0x24)
	bus.Write8(0x0102, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := cpu.B; got != 0x24 {
		t.Fatalf("ARM64 emitted LD B,n result = %02x, want 24", got)
	}
}

func TestZ80JITARM64NativeAddHLDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x21) // LD HL,$4000
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x40)
	bus.Write8(0x0103, 0x3E) // LD A,$12
	bus.Write8(0x0104, 0x12)
	bus.Write8(0x0105, 0x86) // ADD A,(HL)
	bus.Write8(0x0106, 0x76)
	bus.Write8(0x4000, 0x22)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := cpu.A; got != 0x34 {
		t.Fatalf("ADD A,(HL) = %02x, want 34", got)
	}
	if got := cpu.jitStats.nativeEntries.Load(); got == 0 {
		t.Fatal("ARM64 ADD A,(HL) did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeLoadFromHLDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x21)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x40) // LD HL,$4000
	bus.Write8(0x0103, 0x46)
	bus.Write8(0x0104, 0x76) // LD B,(HL); HALT
	bus.Write8(0x4000, 0x7C)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := cpu.B; got != 0x7C {
		t.Fatalf("LD B,(HL) = %02x, want 7c", got)
	}
	if got := cpu.jitStats.nativeEntries.Load(); got == 0 {
		t.Fatal("ARM64 LD B,(HL) did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeLoadFromBCDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x01)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x40) // LD BC,$4000
	bus.Write8(0x0103, 0x0A)
	bus.Write8(0x0104, 0x76) // LD A,(BC); HALT
	bus.Write8(0x4000, 0x7C)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := cpu.A; got != 0x7C {
		t.Fatalf("LD A,(BC) = %02x, want 7c", got)
	}
	if got := cpu.jitStats.nativeEntries.Load(); got == 0 {
		t.Fatal("ARM64 LD A,(BC) did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeStoreToHLDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x21)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x40) // LD HL,$4000
	bus.Write8(0x0103, 0x06)
	bus.Write8(0x0104, 0x7C) // LD B,$7C
	bus.Write8(0x0105, 0x70)
	bus.Write8(0x0106, 0x76) // LD (HL),B; HALT
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := bus.Read8(0x4000); got != 0x7C {
		t.Fatalf("LD (HL),B wrote %02x, want 7c", got)
	}
	if got := cpu.jitStats.nativeEntries.Load(); got == 0 {
		t.Fatal("ARM64 LD (HL),B did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeStoreImmediateToHLDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x21)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x40) // LD HL,$4000
	bus.Write8(0x0103, 0x36)
	bus.Write8(0x0104, 0x7C)
	bus.Write8(0x0105, 0x76) // LD (HL),$7C; HALT
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := bus.Read8(0x4000); got != 0x7C {
		t.Fatalf("LD (HL),n wrote %02x, want 7c", got)
	}
}

func TestZ80JITARM64NativeStoreToHLInvalidatesCodePage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	// The direct store replaces the following NOP with HALT. The native block
	// must return after committing it, invalidate its source page, then resume.
	bus.Write8(0x0100, 0x21)
	bus.Write8(0x0101, 0x06)
	bus.Write8(0x0102, 0x01) // LD HL,$0106
	bus.Write8(0x0103, 0x06)
	bus.Write8(0x0104, 0x76) // LD B,$76
	bus.Write8(0x0105, 0x70)
	bus.Write8(0x0106, 0x00) // LD (HL),B; NOP
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted {
		t.Fatal("self-modified HALT did not execute")
	}
	if got := bus.Read8(0x0106); got != 0x76 {
		t.Fatalf("code write = %02x, want 76", got)
	}
	if got := cpu.jitStats.invalidations.Load(); got == 0 {
		t.Fatal("code-page store did not invalidate JIT blocks")
	}
}

func TestZ80JITARM64NativePairStoresInvalidateCodePage(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
	}{
		{
			name: "LD (nn),HL",
			code: []byte{0x21, 0x76, 0x76, 0x22, 0x07, 0x01, 0x00, 0x00, 0x00, 0x76},
		},
		{
			name: "PUSH BC",
			code: []byte{0x01, 0x76, 0x76, 0x31, 0x09, 0x01, 0xC5, 0x00, 0x00, 0x76},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			cpu.jitEnabled = true
			cpu.jitPersist = true
			t.Cleanup(cpu.freeZ80JIT)
			for offset, value := range tc.code {
				bus.Write8(0x0100+uint32(offset), value)
			}
			cpu.PC = 0x0100
			cpu.SetRunning(true)
			cpu.ExecuteJITZ80()
			if !cpu.Halted {
				t.Fatal("self-modified HALT did not execute")
			}
			if bus.Read8(0x0107) != 0x76 || bus.Read8(0x0108) != 0x76 {
				t.Fatalf("pair store bytes = %02X %02X, want 76 76", bus.Read8(0x0107), bus.Read8(0x0108))
			}
			if cpu.jitStats.invalidations.Load() == 0 {
				t.Fatal("pair store did not invalidate the compiled code page")
			}
		})
	}
}

func TestZ80JITARM64NativeLoadAbsoluteDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x3A)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x40) // LD A,($4000)
	bus.Write8(0x0103, 0x76)
	bus.Write8(0x4000, 0x7C)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := cpu.A; got != 0x7C {
		t.Fatalf("LD A,(nn) = %02x, want 7c", got)
	}
}

func TestZ80JITARM64NativeLoadHLAbsoluteDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x2A)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x40) // LD HL,($4000)
	bus.Write8(0x0103, 0x76)
	bus.Write8(0x4000, 0x34)
	bus.Write8(0x4001, 0x12)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := uint16(cpu.H)<<8 | uint16(cpu.L); got != 0x1234 {
		t.Fatalf("LD HL,(nn) = %04x, want 1234", got)
	}
}

func TestZ80JITARM64NativeStoreAbsoluteDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x3E)
	bus.Write8(0x0101, 0x7C) // LD A,$7C
	bus.Write8(0x0102, 0x32)
	bus.Write8(0x0103, 0x00)
	bus.Write8(0x0104, 0x40) // LD ($4000),A
	bus.Write8(0x0105, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := bus.Read8(0x4000); got != 0x7C {
		t.Fatalf("LD (nn),A wrote %02x, want 7c", got)
	}
}

func TestZ80JITARM64NativeStoreAbsoluteInvalidatesCodePage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x3E)
	bus.Write8(0x0101, 0x76) // LD A,$76
	bus.Write8(0x0102, 0x32)
	bus.Write8(0x0103, 0x06)
	bus.Write8(0x0104, 0x01) // LD ($0106),A
	bus.Write8(0x0105, 0x00)
	bus.Write8(0x0106, 0x00) // NOP; overwritten NOP
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted {
		t.Fatal("self-modified HALT did not execute")
	}
	if got := bus.Read8(0x0106); got != 0x76 {
		t.Fatalf("absolute code write = %02x, want 76", got)
	}
	if got := cpu.jitStats.invalidations.Load(); got == 0 {
		t.Fatal("absolute code-page store did not invalidate JIT blocks")
	}
}

func TestZ80JITARM64NativeStoreToBCDirectPage(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x01)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x40) // LD BC,$4000
	bus.Write8(0x0103, 0x3E)
	bus.Write8(0x0104, 0x7C) // LD A,$7C
	bus.Write8(0x0105, 0x02)
	bus.Write8(0x0106, 0x76) // LD (BC),A; HALT
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := bus.Read8(0x4000); got != 0x7C {
		t.Fatalf("LD (BC),A wrote %02x, want 7c", got)
	}
}

func TestZ80JITARM64NativeEDLoadIA(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x3E)
	bus.Write8(0x0101, 0x7C) // LD A,$7C
	bus.Write8(0x0102, 0xED)
	bus.Write8(0x0103, 0x47) // LD I,A
	bus.Write8(0x0104, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.I != 0x7C {
		t.Fatalf("LD I,A = %02x, want 7c", cpu.I)
	}
}

func TestZ80JITARM64NativeEDInterruptModes(t *testing.T) {
	for _, tc := range []struct{ opcode, want byte }{{0x46, 0}, {0x56, 1}, {0x5E, 2}} {
		bus := NewMachineBus()
		adapter := NewZ80BusAdapter(bus)
		cpu := NewCPU_Z80(adapter)
		cpu.jitEnabled = true
		cpu.jitPersist = true
		bus.Write8(0x0100, 0xED)
		bus.Write8(0x0101, tc.opcode)
		bus.Write8(0x0102, 0x76)
		cpu.PC = 0x0100
		cpu.SetRunning(true)
		cpu.ExecuteJITZ80()
		cpu.freeZ80JIT()
		if cpu.IM != tc.want {
			t.Fatalf("%02X IM = %d, want %d", tc.opcode, cpu.IM, tc.want)
		}
	}
}

func TestZ80JITARM64NativeEDNEG(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.A = 1
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xED)
	bus.Write8(0x0101, 0x44)
	bus.Write8(0x0102, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.A != 0xFF || cpu.F != byte(z80FlagS|z80FlagX|z80FlagY|z80FlagH|z80FlagN|z80FlagC) {
		t.Fatalf("NEG state A=%02X F=%02X", cpu.A, cpu.F)
	}
}

func TestZ80JITARM64NativeEDLoadAI(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.I, cpu.F, cpu.IFF2 = 0x80, z80FlagC, true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xED)
	bus.Write8(0x0101, 0x57)
	bus.Write8(0x0102, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.A != 0x80 || cpu.F != byte(z80FlagS|z80FlagPV|z80FlagC) {
		t.Fatalf("LD A,I state A=%02X F=%02X", cpu.A, cpu.F)
	}
}

func TestZ80JITARM64NativeIgnoredIndexPrefixNOP(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xDD)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted {
		t.Fatal("DD NOP did not reach HALT")
	}
	if got := cpu.R & 0x7F; got != 3 {
		t.Fatalf("DD NOP plus HALT R = %02X, want 03", got)
	}
}

func TestZ80JITARM64NativeIgnoredIndexPrefixSCF(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xFD)
	bus.Write8(0x0101, 0x37)
	bus.Write8(0x0102, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.F&z80FlagC == 0 {
		t.Fatal("FD SCF did not set carry")
	}
	if got := cpu.R & 0x7F; got != 3 {
		t.Fatalf("FD SCF plus HALT R = %02X, want 03", got)
	}
}

func TestZ80JITARM64NativeIgnoredIndexPrefixLDImm(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xDD)
	bus.Write8(0x0101, 0x3E)
	bus.Write8(0x0102, 0x7C)
	bus.Write8(0x0103, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.A != 0x7C {
		t.Fatalf("DD LD A,n = %02X, want 7C", cpu.A)
	}
	if got := cpu.R & 0x7F; got != 3 {
		t.Fatalf("DD LD A,n plus HALT R = %02X, want 03", got)
	}
}

func TestZ80JITARM64NativeIgnoredIndexPrefixLDReg(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x7C
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xFD)
	bus.Write8(0x0101, 0x78)
	bus.Write8(0x0102, 0x76) // LD A,B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.A != 0x7C {
		t.Fatalf("FD LD A,B = %02X, want 7C", cpu.A)
	}
}

func TestZ80JITARM64NativeIgnoredIndexPrefixIncDec(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x7B
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xDD)
	bus.Write8(0x0101, 0x04)
	bus.Write8(0x0102, 0x76) // INC B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x7C {
		t.Fatalf("DD INC B = %02X, want 7C", cpu.B)
	}
}

func TestZ80JITARM64NativeIgnoredIndexPrefixALUReg(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.A = 0x21
	cpu.B = 0x03
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xFD)
	bus.Write8(0x0101, 0x80)
	bus.Write8(0x0102, 0x76) // ADD A,B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.A != 0x24 {
		t.Fatalf("FD ADD A,B = %02X, want 24", cpu.A)
	}
}

func TestZ80JITARM64NativeIgnoredIndexPrefixPairLoad(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xDD)
	bus.Write8(0x0101, 0x01)
	bus.Write8(0x0102, 0x34)
	bus.Write8(0x0103, 0x12)
	bus.Write8(0x0104, 0x76) // LD BC,$1234
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x12 || cpu.C != 0x34 {
		t.Fatalf("DD LD BC,nn = %02X%02X, want 1234", cpu.B, cpu.C)
	}
}

func TestZ80JITARM64NativeCBSetResRegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x03
	cpu.F = z80FlagC
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x80)
	bus.Write8(0x0102, 0x76) // RES 0,B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x02 || cpu.F != z80FlagC {
		t.Fatalf("CB RES 0,B = B:%02X F:%02X, want 02/01", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeCBBITRegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.H = 0xA8
	cpu.F = z80FlagC
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x7C)
	bus.Write8(0x0102, 0x76) // BIT 7,H
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	want := byte(z80FlagS | z80FlagH | z80FlagX | z80FlagY | z80FlagC)
	if cpu.F != want {
		t.Fatalf("CB BIT 7,H F = %02X, want %02X", cpu.F, want)
	}
}

func TestZ80JITARM64NativeCBBITRegisterClear(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x28
	cpu.F = z80FlagC
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x40)
	bus.Write8(0x0102, 0x76) // BIT 0,B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	want := byte(z80FlagZ | z80FlagPV | z80FlagH | z80FlagX | z80FlagY | z80FlagC)
	if cpu.F != want {
		t.Fatalf("CB BIT 0,B F = %02X, want %02X", cpu.F, want)
	}
}

func TestZ80JITARM64NativeCBRLCRegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x81
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x00)
	bus.Write8(0x0102, 0x76) // RLC B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x03 || cpu.F != z80FlagPV|z80FlagC {
		t.Fatalf("CB RLC B = B:%02X F:%02X, want 03/05", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeCBRRCRegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x03
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x08)
	bus.Write8(0x0102, 0x76) // RRC B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x81 || cpu.F != z80FlagS|z80FlagPV|z80FlagC {
		t.Fatalf("CB RRC B = B:%02X F:%02X, want 81/85", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeCBSLARegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x81
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x20)
	bus.Write8(0x0102, 0x76) // SLA B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x02 || cpu.F != z80FlagC {
		t.Fatalf("CB SLA B = B:%02X F:%02X, want 02/01", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeCBSRLRegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x81
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x38)
	bus.Write8(0x0102, 0x76) // SRL B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x40 || cpu.F != z80FlagC {
		t.Fatalf("CB SRL B = B:%02X F:%02X, want 40/01", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeCBSRARegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x81
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x28)
	bus.Write8(0x0102, 0x76) // SRA B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0xC0 || cpu.F != z80FlagS|z80FlagPV|z80FlagC {
		t.Fatalf("CB SRA B = B:%02X F:%02X, want C0/85", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeCBSLLRegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x80
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x30)
	bus.Write8(0x0102, 0x76) // SLL B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x01 || cpu.F != z80FlagC {
		t.Fatalf("CB SLL B = B:%02X F:%02X, want 01/01", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeCBRLRegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x80
	cpu.F = z80FlagC
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x10)
	bus.Write8(0x0102, 0x76) // RL B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x01 || cpu.F != z80FlagC {
		t.Fatalf("CB RL B = B:%02X F:%02X, want 01/01", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeCBRRRegister(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.B = 0x01
	cpu.F = z80FlagC
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0xCB)
	bus.Write8(0x0101, 0x18)
	bus.Write8(0x0102, 0x76) // RR B
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if cpu.B != 0x80 || cpu.F != z80FlagS|z80FlagC {
		t.Fatalf("CB RR B = B:%02X F:%02X, want 80/81", cpu.B, cpu.F)
	}
}

func TestZ80JITARM64NativeLDRegReg(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	bus.Write8(0x0100, 0x3E) // LD A,$6C (native immediate load)
	bus.Write8(0x0101, 0x6C)
	bus.Write8(0x0102, 0x47) // LD B,A (native register load)
	bus.Write8(0x0103, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if got := cpu.B; got != 0x6C {
		t.Fatalf("ARM64 emitted LD B,A result = %02x, want 6c", got)
	}
}

func TestZ80JITARM64NativeStaticJumps(t *testing.T) {
	for _, program := range []struct {
		name string
		code []byte
	}{
		{"jp", []byte{0xC3, 0x05, 0x01, 0x00, 0x00, 0x3E, 0xA5, 0x76}}, // JP $0105; LD A,$A5; HALT
		{"jr", []byte{0x18, 0x03, 0x00, 0x00, 0x00, 0x3E, 0x5A, 0x76}}, // JR $0105; LD A,$5A; HALT
	} {
		t.Run(program.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			cpu.jitEnabled = true
			cpu.jitPersist = true
			t.Cleanup(cpu.freeZ80JIT)
			for offset, value := range program.code {
				bus.Write8(0x0100+uint32(offset), value)
			}
			cpu.PC = 0x0100
			cpu.SetRunning(true)
			cpu.ExecuteJITZ80()
			if !cpu.Halted || cpu.A != program.code[6] {
				t.Fatalf("ARM64 %s state: halted=%v A=%02X", program.name, cpu.Halted, cpu.A)
			}
			if cpu.jitStats.nativeEntries.Load() == 0 {
				t.Fatalf("ARM64 %s did not execute an emitted native block", program.name)
			}
		})
	}
}

func TestZ80JITARM64NativeIndirectJump(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	cpu.H, cpu.L = 0x01, 0x05
	bus.Write8(0x0100, 0xE9) // JP (HL)
	bus.Write8(0x0105, 0x3E) // LD A,$A5
	bus.Write8(0x0106, 0xA5)
	bus.Write8(0x0107, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted || cpu.A != 0xA5 {
		t.Fatalf("ARM64 JP (HL) state: halted=%v A=%02X", cpu.Halted, cpu.A)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("ARM64 JP (HL) did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativePairLoads(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x21) // LD HL,$1234
	bus.Write8(0x0101, 0x34)
	bus.Write8(0x0102, 0x12)
	bus.Write8(0x0103, 0x23) // INC HL
	bus.Write8(0x0104, 0x2B) // DEC HL
	bus.Write8(0x0105, 0xF9) // LD SP,HL
	bus.Write8(0x0106, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted || cpu.H != 0x12 || cpu.L != 0x34 || cpu.SP != 0x1234 {
		t.Fatalf("ARM64 pair state: halted=%v HL=%02X%02X SP=%04X", cpu.Halted, cpu.H, cpu.L, cpu.SP)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("ARM64 pair forms did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeAddHLPair(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.H, cpu.L, cpu.B, cpu.C, cpu.F = 0x8F, 0xFF, 0x70, 0x01, z80FlagZ|z80FlagPV
	bus.Write8(0x0100, 0x09) // ADD HL,BC
	bus.Write8(0x0101, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted || cpu.H != 0 || cpu.L != 0 || cpu.F != z80FlagZ|z80FlagPV|z80FlagH|z80FlagC {
		t.Fatalf("ARM64 ADD HL,BC state: halted=%v HL=%02X%02X F=%02X", cpu.Halted, cpu.H, cpu.L, cpu.F)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("ARM64 ADD HL,BC did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeRegisterExchange(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.D, cpu.E, cpu.H, cpu.L = 0x56, 0x78, 0x12, 0x34
	bus.Write8(0x0100, 0xEB) // EX DE,HL
	bus.Write8(0x0101, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted || cpu.D != 0x12 || cpu.E != 0x34 || cpu.H != 0x56 || cpu.L != 0x78 {
		t.Fatalf("ARM64 EX DE,HL state: halted=%v DE=%02X%02X HL=%02X%02X", cpu.Halted, cpu.D, cpu.E, cpu.H, cpu.L)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("ARM64 EX DE,HL did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeFlagControls(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.A, cpu.F = 0x28, 0xC5
	bus.Write8(0x0100, 0x2F) // CPL
	bus.Write8(0x0101, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted || cpu.A != 0xD7 || cpu.F != 0xD7 {
		t.Fatalf("ARM64 CPL state: halted=%v A=%02X F=%02X", cpu.Halted, cpu.A, cpu.F)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("ARM64 CPL did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeAccumulatorRotates(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	t.Cleanup(cpu.freeZ80JIT)
	bus.Write8(0x0100, 0x3E) // LD A,$81
	bus.Write8(0x0101, 0x81)
	bus.Write8(0x0102, 0x07) // RLCA
	bus.Write8(0x0103, 0x0F) // RRCA
	bus.Write8(0x0104, 0x17) // RLA
	bus.Write8(0x0105, 0x1F) // RRA
	bus.Write8(0x0106, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	if !cpu.Halted || cpu.A != 0x81 || cpu.F != z80FlagC {
		t.Fatalf("ARM64 accumulator rotates: halted=%v A=%02X F=%02X", cpu.Halted, cpu.A, cpu.F)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("ARM64 accumulator rotates did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeIncDecFlags(t *testing.T) {
	for _, tc := range []struct {
		name       string
		opcode, in byte
		wantValue  byte
		wantFlags  byte
	}{
		{"inc overflow", 0x04, 0x7F, 0x80, 0x95},
		{"dec overflow", 0x05, 0x80, 0x7F, 0x3F},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			cpu.jitEnabled = true
			cpu.B, cpu.F = tc.in, z80FlagC
			bus.Write8(0x0100, tc.opcode)
			bus.Write8(0x0101, 0x76)
			cpu.PC = 0x0100
			cpu.SetRunning(true)
			cpu.ExecuteJITZ80()
			if !cpu.Halted || cpu.B != tc.wantValue || cpu.F != tc.wantFlags {
				t.Fatalf("ARM64 %s state: halted=%v B=%02X F=%02X", tc.name, cpu.Halted, cpu.B, cpu.F)
			}
			if cpu.jitStats.nativeEntries.Load() == 0 {
				t.Fatalf("ARM64 %s did not execute an emitted native block", tc.name)
			}
		})
	}
}

func TestZ80JITARM64NativeLogicalALU(t *testing.T) {
	for _, tc := range []struct {
		name         string
		code         []byte
		a, b         byte
		wantA, wantF byte
	}{
		{"AND register", []byte{0xA0, 0x76}, 0xD3, 0x6A, 0x42, 0x14},
		{"XOR immediate", []byte{0xEE, 0xFF, 0x76}, 0x55, 0, 0xAA, 0xAC},
		{"OR register", []byte{0xB0, 0x76}, 0x40, 0x03, 0x43, 0x00},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			cpu.jitEnabled = true
			cpu.A, cpu.B = tc.a, tc.b
			for offset, value := range tc.code {
				bus.Write8(0x0100+uint32(offset), value)
			}
			cpu.PC = 0x0100
			cpu.SetRunning(true)
			cpu.ExecuteJITZ80()
			if !cpu.Halted || cpu.A != tc.wantA || cpu.F != tc.wantF {
				t.Fatalf("ARM64 %s state: halted=%v A=%02X F=%02X", tc.name, cpu.Halted, cpu.A, cpu.F)
			}
			if cpu.jitStats.nativeEntries.Load() == 0 {
				t.Fatalf("ARM64 %s did not execute an emitted native block", tc.name)
			}
		})
	}
}

func TestZ80JITARM64NativeAddALU(t *testing.T) {
	for _, tc := range []struct {
		name         string
		code         []byte
		a, b, flags  byte
		wantA, wantF byte
	}{
		{"register overflow", []byte{0x80, 0x76}, 0x7F, 0x01, 0, 0x80, z80FlagS | z80FlagH | z80FlagPV},
		{"immediate carry", []byte{0xC6, 0x01, 0x76}, 0xFF, 0, 0, 0, z80FlagZ | z80FlagH | z80FlagC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			cpu.jitEnabled = true
			cpu.A, cpu.B, cpu.F = tc.a, tc.b, tc.flags
			for i, value := range tc.code {
				bus.Write8(0x0100+uint32(i), value)
			}
			cpu.PC = 0x0100
			cpu.SetRunning(true)
			cpu.ExecuteJITZ80()
			if !cpu.Halted || cpu.A != tc.wantA || cpu.F != tc.wantF {
				t.Fatalf("ARM64 ADD state: halted=%v A=%02X F=%02X", cpu.Halted, cpu.A, cpu.F)
			}
			if cpu.jitStats.nativeEntries.Load() == 0 {
				t.Fatal("ARM64 ADD did not execute an emitted native block")
			}
		})
	}
}

func TestZ80JITARM64NativeADCALU(t *testing.T) {
	for _, tc := range []struct {
		name         string
		code         []byte
		a, b         byte
		wantA, wantF byte
	}{
		{"register carry", []byte{0x88, 0x76}, 0xFF, 0x00, 0x00, z80FlagZ | z80FlagH | z80FlagC},
		{"immediate carry", []byte{0xCE, 0x00, 0x76}, 0xFF, 0, 0x00, z80FlagZ | z80FlagH | z80FlagC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			cpu.jitEnabled = true
			cpu.A, cpu.B, cpu.F = tc.a, tc.b, z80FlagC
			for i, value := range tc.code {
				bus.Write8(0x0100+uint32(i), value)
			}
			cpu.PC = 0x0100
			cpu.SetRunning(true)
			cpu.ExecuteJITZ80()
			if !cpu.Halted || cpu.A != tc.wantA || cpu.F != tc.wantF {
				t.Fatalf("ARM64 ADC state: halted=%v A=%02X F=%02X", cpu.Halted, cpu.A, cpu.F)
			}
			if cpu.jitStats.nativeEntries.Load() == 0 {
				t.Fatal("ARM64 ADC did not execute an emitted native block")
			}
		})
	}
}

func TestZ80JITARM64NativeSBCALU(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.A, cpu.B, cpu.F = 0x00, 0x00, z80FlagC
	bus.Write8(0x0100, 0x98) // SBC A,B
	bus.Write8(0x0101, 0x76)
	cpu.PC = 0x0100
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	wantF := byte(z80FlagS | z80FlagX | z80FlagY | z80FlagH | z80FlagN | z80FlagC)
	if !cpu.Halted || cpu.A != 0xFF || cpu.F != wantF {
		t.Fatalf("ARM64 SBC state: halted=%v A=%02X F=%02X", cpu.Halted, cpu.A, cpu.F)
	}
	if cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("ARM64 SBC did not execute an emitted native block")
	}
}

func TestZ80JITARM64NativeSubALU(t *testing.T) {
	for _, tc := range []struct {
		name         string
		code         []byte
		a, b         byte
		wantA, wantF byte
	}{
		{"register overflow", []byte{0x90, 0x76}, 0x80, 0x01, 0x7F, z80FlagX | z80FlagY | z80FlagH | z80FlagPV | z80FlagN},
		{"immediate borrow", []byte{0xD6, 0x01, 0x76}, 0x00, 0, 0xFF, z80FlagS | z80FlagX | z80FlagY | z80FlagH | z80FlagN | z80FlagC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			cpu.jitEnabled = true
			cpu.A, cpu.B = tc.a, tc.b
			for i, value := range tc.code {
				bus.Write8(0x0100+uint32(i), value)
			}
			cpu.PC = 0x0100
			cpu.SetRunning(true)
			cpu.ExecuteJITZ80()
			if !cpu.Halted || cpu.A != tc.wantA || cpu.F != tc.wantF {
				t.Fatalf("ARM64 SUB state: halted=%v A=%02X F=%02X", cpu.Halted, cpu.A, cpu.F)
			}
			if cpu.jitStats.nativeEntries.Load() == 0 {
				t.Fatal("ARM64 SUB did not execute an emitted native block")
			}
		})
	}
}

func TestZ80JITARM64NativeCompareALU(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  []byte
		a, b  byte
		wantF byte
	}{
		{"register overflow", []byte{0xB8, 0x76}, 0x80, 0x01, z80FlagX | z80FlagY | z80FlagH | z80FlagPV | z80FlagN},
		{"immediate borrow", []byte{0xFE, 0x01, 0x76}, 0x00, 0, z80FlagS | z80FlagX | z80FlagY | z80FlagH | z80FlagN | z80FlagC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			cpu.jitEnabled = true
			cpu.A, cpu.B = tc.a, tc.b
			for i, value := range tc.code {
				bus.Write8(0x0100+uint32(i), value)
			}
			cpu.PC = 0x0100
			cpu.SetRunning(true)
			cpu.ExecuteJITZ80()
			if !cpu.Halted || cpu.A != tc.a || cpu.F != tc.wantF {
				t.Fatalf("ARM64 CP state: halted=%v A=%02X F=%02X", cpu.Halted, cpu.A, cpu.F)
			}
			if cpu.jitStats.nativeEntries.Load() == 0 {
				t.Fatal("ARM64 CP did not execute an emitted native block")
			}
		})
	}
}
