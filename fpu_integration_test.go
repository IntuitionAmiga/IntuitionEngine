package main

import (
	"math"
	"testing"
)

// =============================================================================
// FPU Integration Tests - Verifies F-line decoder and CPU/FPU interaction
// =============================================================================

func setupFPUTestCPU() *M68KCPU {
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, M68K_STACK_START)
	bus.Write32(M68K_RESET_VECTOR, M68K_ENTRY_POINT)
	return NewM68KCPU(bus)
}

func TestFPU_CpuIntegration(t *testing.T) {
	t.Run("FPU_is_initialized", func(t *testing.T) {
		cpu := setupFPUTestCPU()

		if cpu.FPU == nil {
			t.Fatal("FPU should be initialized on CPU creation")
		}

		// Verify FPU registers are zero-initialized
		for i := range 8 {
			if cpu.FPU.GetFP64(i) != 0 {
				t.Errorf("FP%d should be zero on init", i)
			}
		}
	})
}

func TestFPU_FlineDecoder(t *testing.T) {
	t.Run("F_line_opcode_routes_to_FPU", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		// Load FP0 with a value manually
		cpu.FPU.SetFP64(0, 1.5)
		cpu.FPU.SetFP64(1, 2.5)

		// FADD FP1,FP0 - should add FP1 to FP0
		// F-line opcode: 1111 001 0 00 000 000 = 0xF200
		// Command word encoding (bits 12-10=src, 9-7=dst, 6-0=op):
		//   src=1 (FP1): bits 12-10 = 001
		//   dst=0 (FP0): bits 9-7 = 000
		//   op=0x22 (FADD): bits 6-0 = 0100010
		// Binary: 0000 0100 0010 0010 = 0x0422
		cpu.Write16(cpu.PC, 0xF200)   // F-line opcode
		cpu.Write16(cpu.PC+2, 0x0422) // Command word: FADD FP1,FP0

		// Fetch and decode
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.GetFP64(0)
		expected := 4.0 // 1.5 + 2.5

		if math.Abs(result-expected) > 1e-10 {
			t.Errorf("FADD FP1,FP0: expected %v, got %v", expected, result)
		}
	})

	t.Run("FMOVECR_loads_pi", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		// FMOVECR #$00,FP0 - load Pi into FP0
		// F-line opcode: 0xF200
		// Command word for FMOVECR: 0x5C00 | (dst << 7) | romAddr
		// dst=0 (bits 9-7 = 000), romAddr=0x00 (Pi)
		// 0101 1100 0000 0000 = 0x5C00
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x5C00)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.GetFP64(0)
		if math.Abs(result-math.Pi) > 1e-15 {
			t.Errorf("FMOVECR pi: expected %v, got %v", math.Pi, result)
		}
	})

	t.Run("FMOVE_register_to_register", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		// Set FP1 to a value
		cpu.FPU.SetFP64(1, 42.5)

		// FMOVE FP1,FP0
		// Command word: src=1 (bits 12-10), dst=0 (bits 9-7), op=FMOVE(0x00)
		// Binary: 0000 0100 0000 0000 = 0x0400
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0400)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.GetFP64(0)
		if result != 42.5 {
			t.Errorf("FMOVE FP1,FP0: expected 42.5, got %v", result)
		}
	})

	t.Run("FMUL_multiplies_registers", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.SetFP64(0, 3.0)
		cpu.FPU.SetFP64(2, 4.0)

		// FMUL FP2,FP0 - FP0 = FP0 * FP2
		// Command word: src=2 (bits 12-10 = 010), dst=0 (bits 9-7 = 000), op=FMUL(0x23)
		// Binary: 0000 1000 0010 0011 = 0x0823
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0823)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.GetFP64(0)
		if result != 12.0 {
			t.Errorf("FMUL FP2,FP0: expected 12.0, got %v", result)
		}
	})

	t.Run("FDIV_divides_registers", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.SetFP64(0, 20.0)
		cpu.FPU.SetFP64(3, 4.0)

		// FDIV FP3,FP0 - FP0 = FP0 / FP3
		// Command word: src=3 (bits 12-10 = 011), dst=0 (bits 9-7 = 000), op=FDIV(0x20)
		// Binary: 0000 1100 0010 0000 = 0x0C20
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0C20)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.GetFP64(0)
		if result != 5.0 {
			t.Errorf("FDIV FP3,FP0: expected 5.0, got %v", result)
		}
	})

	t.Run("FSQRT_computes_square_root", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.SetFP64(0, 16.0)

		// FSQRT FP0,FP0
		// Command word: R/M=0, src=000, dst=000, op=FSQRT(0x04)
		// 0 000 000 0 000 0100 = 0x0004
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0004)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.GetFP64(0)
		if result != 4.0 {
			t.Errorf("FSQRT FP0: expected 4.0, got %v", result)
		}
	})

	t.Run("FSIN_computes_sine", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.SetFP64(0, math.Pi/2)

		// FSIN FP0,FP0
		// Command word: R/M=0, src=000, dst=000, op=FSIN(0x0E)
		// 0 000 000 0 000 1110 = 0x000E
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x000E)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.GetFP64(0)
		if math.Abs(result-1.0) > 1e-14 {
			t.Errorf("FSIN(pi/2): expected 1.0, got %v", result)
		}
	})

	t.Run("FCMP_sets_condition_codes", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.SetFP64(0, 5.0)
		cpu.FPU.SetFP64(1, 10.0)

		// FCMP FP1,FP0 - compare FP0 with FP1
		// Command word: src=1 (bits 12-10 = 001), dst=0 (bits 9-7 = 000), op=FCMP(0x38)
		// Binary: 0000 0100 0011 1000 = 0x0438
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0438)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// FP0 (5.0) < FP1 (10.0), so N flag should be set
		if !cpu.FPU.GetConditionN() {
			t.Error("FCMP should set N when FP0 < FP1")
		}
		if cpu.FPU.GetConditionZ() {
			t.Error("FCMP should not set Z when FP0 != FP1")
		}
	})
}

func TestFPU_NoFPU_TriggersLineF(t *testing.T) {
	t.Run("F_line_without_FPU_triggers_exception", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.FPU = nil // Remove FPU
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S // Supervisor mode

		// Set up Line F exception vector
		cpu.Write32(M68K_VEC_LINE_F*4, 0x2000) // Vector points to 0x2000

		// Write an F-line opcode
		cpu.Write16(cpu.PC, 0xF200)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should now point to the exception handler
		if cpu.PC != 0x2000 {
			t.Errorf("Expected PC to be exception vector (0x2000), got 0x%08X", cpu.PC)
		}
	})
}

func TestFPU_ConditionCodes(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		expectN   bool
		expectZ   bool
		expectI   bool
		expectNAN bool
	}{
		{"positive", 42.0, false, false, false, false},
		{"negative", -42.0, true, false, false, false},
		{"zero", 0.0, false, true, false, false},
		{"infinity", math.Inf(1), false, false, true, false},
		{"neg_infinity", math.Inf(-1), true, false, true, false},
		{"NaN", math.NaN(), false, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu := setupFPUTestCPU()
			cpu.PC = 0x1000

			cpu.FPU.SetFP64(0, tt.value)

			// FTST FP0
			// Command word: R/M=0, src=000, dst=000, op=FTST(0x3A)
			// 0 000 000 0 011 1010 = 0x003A
			cpu.Write16(cpu.PC, 0xF200)
			cpu.Write16(cpu.PC+2, 0x003A)

			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			if cpu.FPU.GetConditionN() != tt.expectN {
				t.Errorf("N flag: expected %v, got %v", tt.expectN, cpu.FPU.GetConditionN())
			}
			if cpu.FPU.GetConditionZ() != tt.expectZ {
				t.Errorf("Z flag: expected %v, got %v", tt.expectZ, cpu.FPU.GetConditionZ())
			}
			if cpu.FPU.GetConditionI() != tt.expectI {
				t.Errorf("I flag: expected %v, got %v", tt.expectI, cpu.FPU.GetConditionI())
			}
			if cpu.FPU.GetConditionNAN() != tt.expectNAN {
				t.Errorf("NAN flag: expected %v, got %v", tt.expectNAN, cpu.FPU.GetConditionNAN())
			}
		})
	}
}

// Benchmark FPU instruction decode + execute through CPU
func BenchmarkFPU_FlineDecodeExecute(b *testing.B) {
	cpu := setupFPUTestCPU()
	cpu.PC = 0x1000

	// FADD FP1,FP0
	cpu.Write16(0x1000, 0xF200)
	cpu.Write16(0x1002, 0x0822)

	cpu.FPU.SetFP64(0, 1.0)
	cpu.FPU.SetFP64(1, 2.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

func TestFPU_MemToReg_ExtendedFormat(t *testing.T) {
	cpu := setupFPUTestCPU()
	cpu.AddrRegs[0] = 0x3000
	want := ExtendedRealFromFloat64(math.Pi)
	cpu.writeExtendedReal96(cpu.AddrRegs[0], want)
	opcode := uint16((2 << 3) | 0) // (A0)
	cmdWord := uint16((2 << 10) | (1 << 7))
	cpu.execFPUMemToReg(opcode, cmdWord)
	if got := cpu.FPU.GetFP64(1); math.Abs(got-math.Pi) > 1e-15 {
		t.Fatalf("mem->reg extended got %v want %v", got, math.Pi)
	}
}

func TestFPU_RegToMem_ExtendedFormat(t *testing.T) {
	cpu := setupFPUTestCPU()
	cpu.AddrRegs[0] = 0x3200
	cpu.FPU.SetFP64(2, -math.E)
	opcode := uint16((2 << 3) | 0) // (A0)
	cmdWord := uint16((2 << 10) | (2 << 7))
	cpu.execFPURegToMem(opcode, cmdWord)
	ext := cpu.readExtendedReal96(cpu.AddrRegs[0])
	if got := ext.ToFloat64(); math.Abs(got-(-math.E)) > 1e-15 {
		t.Fatalf("reg->mem extended got %v want %v", got, -math.E)
	}
}

func TestFPU_FMOVEM_ExtendedRoundTrip(t *testing.T) {
	cpu := setupFPUTestCPU()
	cpu.AddrRegs[0] = 0x3400
	values := [8]float64{math.Pi, -math.E, 0, math.Inf(1), math.Inf(-1), 1.25, -42.0, 1e-20}
	for i := range 8 {
		cpu.FPU.SetFP64(i, values[i])
	}

	opcode := uint16((2 << 3) | 0) // (A0)
	cpu.execFMOVEM(opcode, 0x00FF) // to memory, FP0-FP7
	for i := range 8 {
		cpu.FPU.SetFP64(i, 0)
	}
	cpu.execFMOVEM(opcode, 0x20FF) // from memory, FP0-FP7

	for i := range 8 {
		got := cpu.FPU.GetFP64(i)
		want := values[i]
		if math.IsInf(want, 0) {
			if !math.IsInf(got, 0) || (math.Signbit(got) != math.Signbit(want)) {
				t.Fatalf("FP%d round-trip got %v want %v", i, got, want)
			}
			continue
		}
		if math.Abs(got-want) > 1e-15 {
			t.Fatalf("FP%d round-trip got %v want %v", i, got, want)
		}
	}
}

func TestFPU_IllegalOpcodeRaisesLineF(t *testing.T) {
	cpu := setupFPUTestCPU()
	cpu.SR = M68K_SR_S
	cpu.Write32(M68K_VEC_LINE_F*4, 0x4000)

	invalidOps := make([]uint16, 0, 128)
	valid := map[uint16]bool{
		0x00: true, 0x01: true, 0x02: true, 0x03: true, 0x04: true, 0x09: true, 0x0A: true,
		0x0C: true, 0x0D: true, 0x0E: true, 0x0F: true, 0x10: true, 0x11: true, 0x12: true,
		0x14: true, 0x15: true, 0x16: true, 0x18: true, 0x19: true, 0x1A: true, 0x1C: true,
		0x1D: true, 0x1E: true, 0x1F: true, 0x20: true, 0x21: true, 0x22: true, 0x23: true,
		0x24: true, 0x25: true, 0x26: true, 0x27: true, 0x28: true, 0x38: true, 0x3A: true,
	}
	for op := uint16(0); op < 128; op++ {
		if !valid[op] {
			invalidOps = append(invalidOps, op)
		}
	}

	for _, op := range invalidOps {
		cpu.PC = 0x2000
		cpu.execFPUGeneral(op)
		if cpu.PC != 0x4000 {
			t.Fatalf("op 0x%02X did not raise LINE_F", op)
		}
	}
}

func TestFPU_FMOVECR_AfterJumpTable(t *testing.T) {
	cpu := setupFPUTestCPU()
	cases := []struct {
		rom  uint8
		want float64
	}{
		{0x00, math.Pi},
		{0x0C, math.E},
		{0x30, math.Ln2},
		{0x31, math.Ln10},
	}
	for i, tc := range cases {
		cmdWord := uint16(0x5C00 | (uint16(i&7) << 7) | uint16(tc.rom))
		cpu.execFPUGeneral(cmdWord)
		if got := cpu.FPU.GetFP64(i & 7); math.Abs(got-tc.want) > 1e-15 {
			t.Fatalf("rom 0x%02X got %v want %v", tc.rom, got, tc.want)
		}
	}
}

func BenchmarkFPU_MemOp_FADD(b *testing.B) {
	cpu := setupFPUTestCPU()
	cpu.AddrRegs[0] = 0x3600
	bits := math.Float64bits(1.5)
	cpu.Write32(cpu.AddrRegs[0], uint32(bits>>32))
	cpu.Write32(cpu.AddrRegs[0]+4, uint32(bits))
	opcode := uint16((2 << 3) | 0)                 // (A0)
	cmdWord := uint16((5 << 10) | (0 << 7) | 0x22) // double src, dst FP0, FADD
	cpu.FPU.SetFP64(0, 2.5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.execFPUMemOp(opcode, cmdWord)
	}
}

func BenchmarkFPU_FMOVEM_RoundTrip(b *testing.B) {
	cpu := setupFPUTestCPU()
	cpu.AddrRegs[0] = 0x3800
	for i := range 8 {
		cpu.FPU.SetFP64(i, float64(i)+0.25)
	}
	opcode := uint16((2 << 3) | 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.execFMOVEM(opcode, 0x00FF)
		cpu.execFMOVEM(opcode, 0x20FF)
	}
}
