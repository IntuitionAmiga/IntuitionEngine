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
			if !cpu.FPU.FPRegs[i].IsZero() {
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
		cpu.FPU.FPRegs[0] = ExtendedRealFromFloat64(1.5)
		cpu.FPU.FPRegs[1] = ExtendedRealFromFloat64(2.5)

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

		result := cpu.FPU.FPRegs[0].ToFloat64()
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

		result := cpu.FPU.FPRegs[0].ToFloat64()
		if math.Abs(result-math.Pi) > 1e-15 {
			t.Errorf("FMOVECR pi: expected %v, got %v", math.Pi, result)
		}
	})

	t.Run("FMOVE_register_to_register", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		// Set FP1 to a value
		cpu.FPU.FPRegs[1] = ExtendedRealFromFloat64(42.5)

		// FMOVE FP1,FP0
		// Command word: src=1 (bits 12-10), dst=0 (bits 9-7), op=FMOVE(0x00)
		// Binary: 0000 0100 0000 0000 = 0x0400
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0400)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.FPRegs[0].ToFloat64()
		if result != 42.5 {
			t.Errorf("FMOVE FP1,FP0: expected 42.5, got %v", result)
		}
	})

	t.Run("FMUL_multiplies_registers", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.FPRegs[0] = ExtendedRealFromFloat64(3.0)
		cpu.FPU.FPRegs[2] = ExtendedRealFromFloat64(4.0)

		// FMUL FP2,FP0 - FP0 = FP0 * FP2
		// Command word: src=2 (bits 12-10 = 010), dst=0 (bits 9-7 = 000), op=FMUL(0x23)
		// Binary: 0000 1000 0010 0011 = 0x0823
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0823)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.FPRegs[0].ToFloat64()
		if result != 12.0 {
			t.Errorf("FMUL FP2,FP0: expected 12.0, got %v", result)
		}
	})

	t.Run("FDIV_divides_registers", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.FPRegs[0] = ExtendedRealFromFloat64(20.0)
		cpu.FPU.FPRegs[3] = ExtendedRealFromFloat64(4.0)

		// FDIV FP3,FP0 - FP0 = FP0 / FP3
		// Command word: src=3 (bits 12-10 = 011), dst=0 (bits 9-7 = 000), op=FDIV(0x20)
		// Binary: 0000 1100 0010 0000 = 0x0C20
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0C20)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.FPRegs[0].ToFloat64()
		if result != 5.0 {
			t.Errorf("FDIV FP3,FP0: expected 5.0, got %v", result)
		}
	})

	t.Run("FSQRT_computes_square_root", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.FPRegs[0] = ExtendedRealFromFloat64(16.0)

		// FSQRT FP0,FP0
		// Command word: R/M=0, src=000, dst=000, op=FSQRT(0x04)
		// 0 000 000 0 000 0100 = 0x0004
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x0004)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.FPRegs[0].ToFloat64()
		if result != 4.0 {
			t.Errorf("FSQRT FP0: expected 4.0, got %v", result)
		}
	})

	t.Run("FSIN_computes_sine", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.FPRegs[0] = ExtendedRealFromFloat64(math.Pi / 2)

		// FSIN FP0,FP0
		// Command word: R/M=0, src=000, dst=000, op=FSIN(0x0E)
		// 0 000 000 0 000 1110 = 0x000E
		cpu.Write16(cpu.PC, 0xF200)
		cpu.Write16(cpu.PC+2, 0x000E)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		result := cpu.FPU.FPRegs[0].ToFloat64()
		if math.Abs(result-1.0) > 1e-14 {
			t.Errorf("FSIN(pi/2): expected 1.0, got %v", result)
		}
	})

	t.Run("FCMP_sets_condition_codes", func(t *testing.T) {
		cpu := setupFPUTestCPU()
		cpu.PC = 0x1000

		cpu.FPU.FPRegs[0] = ExtendedRealFromFloat64(5.0)
		cpu.FPU.FPRegs[1] = ExtendedRealFromFloat64(10.0)

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

			cpu.FPU.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

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

	cpu.FPU.FPRegs[0] = ExtendedRealFromFloat64(1.0)
	cpu.FPU.FPRegs[1] = ExtendedRealFromFloat64(2.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}
