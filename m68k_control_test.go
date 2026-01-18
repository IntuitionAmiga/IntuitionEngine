//go:build m68k_test

package main

import (
	"testing"
)

// =============================================================================
// Control Flow Instructions Test Suite
// Tests: Bcc, BRA, BSR, DBcc, Scc, JMP, JSR, RTS, RTE, RTR, TRAP, TRAPV
// =============================================================================

// Test Bcc (Branch Conditional) instruction with all 16 conditions
func TestBccAllConditions(t *testing.T) {
	// Condition codes for Bcc: condition is encoded in bits 11-8
	// BRA = 0x6000 + (condition << 8) + displacement
	// Displacement 0x08 means branch forward 8 bytes from start of opcode
	// Note: Condition 1 is BSR (not BF) in branch instructions

	tests := []struct {
		name         string
		condition    uint8  // Condition code (0-15)
		srFlags      uint16 // Initial SR flags
		shouldBranch bool
	}{
		// CC_T (0) - Always true (BRA)
		{"BRA_always_branches", M68K_CC_T, 0x0000, true},
		{"BRA_branches_with_flags", M68K_CC_T, M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C, true},

		// Note: CC_F (1) is BSR in Bcc, not a conditional branch - tested separately

		// CC_HI (2) - High: C=0 AND Z=0
		{"BHI_branches_when_C0_Z0", M68K_CC_HI, 0x0000, true},
		{"BHI_no_branch_when_C1", M68K_CC_HI, M68K_SR_C, false},
		{"BHI_no_branch_when_Z1", M68K_CC_HI, M68K_SR_Z, false},
		{"BHI_no_branch_when_C1_Z1", M68K_CC_HI, M68K_SR_C | M68K_SR_Z, false},

		// CC_LS (3) - Low or Same: C=1 OR Z=1
		{"BLS_branches_when_C1", M68K_CC_LS, M68K_SR_C, true},
		{"BLS_branches_when_Z1", M68K_CC_LS, M68K_SR_Z, true},
		{"BLS_branches_when_C1_Z1", M68K_CC_LS, M68K_SR_C | M68K_SR_Z, true},
		{"BLS_no_branch_when_C0_Z0", M68K_CC_LS, 0x0000, false},

		// CC_CC (4) - Carry Clear: C=0
		{"BCC_branches_when_C0", M68K_CC_CC, 0x0000, true},
		{"BCC_no_branch_when_C1", M68K_CC_CC, M68K_SR_C, false},

		// CC_CS (5) - Carry Set: C=1
		{"BCS_branches_when_C1", M68K_CC_CS, M68K_SR_C, true},
		{"BCS_no_branch_when_C0", M68K_CC_CS, 0x0000, false},

		// CC_NE (6) - Not Equal: Z=0
		{"BNE_branches_when_Z0", M68K_CC_NE, 0x0000, true},
		{"BNE_no_branch_when_Z1", M68K_CC_NE, M68K_SR_Z, false},

		// CC_EQ (7) - Equal: Z=1
		{"BEQ_branches_when_Z1", M68K_CC_EQ, M68K_SR_Z, true},
		{"BEQ_no_branch_when_Z0", M68K_CC_EQ, 0x0000, false},

		// CC_VC (8) - Overflow Clear: V=0
		{"BVC_branches_when_V0", M68K_CC_VC, 0x0000, true},
		{"BVC_no_branch_when_V1", M68K_CC_VC, M68K_SR_V, false},

		// CC_VS (9) - Overflow Set: V=1
		{"BVS_branches_when_V1", M68K_CC_VS, M68K_SR_V, true},
		{"BVS_no_branch_when_V0", M68K_CC_VS, 0x0000, false},

		// CC_PL (10) - Plus: N=0
		{"BPL_branches_when_N0", M68K_CC_PL, 0x0000, true},
		{"BPL_no_branch_when_N1", M68K_CC_PL, M68K_SR_N, false},

		// CC_MI (11) - Minus: N=1
		{"BMI_branches_when_N1", M68K_CC_MI, M68K_SR_N, true},
		{"BMI_no_branch_when_N0", M68K_CC_MI, 0x0000, false},

		// CC_GE (12) - Greater or Equal (signed): N=V
		{"BGE_branches_when_N0_V0", M68K_CC_GE, 0x0000, true},
		{"BGE_branches_when_N1_V1", M68K_CC_GE, M68K_SR_N | M68K_SR_V, true},
		{"BGE_no_branch_when_N1_V0", M68K_CC_GE, M68K_SR_N, false},
		{"BGE_no_branch_when_N0_V1", M68K_CC_GE, M68K_SR_V, false},

		// CC_LT (13) - Less Than (signed): N!=V
		{"BLT_branches_when_N1_V0", M68K_CC_LT, M68K_SR_N, true},
		{"BLT_branches_when_N0_V1", M68K_CC_LT, M68K_SR_V, true},
		{"BLT_no_branch_when_N0_V0", M68K_CC_LT, 0x0000, false},
		{"BLT_no_branch_when_N1_V1", M68K_CC_LT, M68K_SR_N | M68K_SR_V, false},

		// CC_GT (14) - Greater Than (signed): Z=0 AND N=V
		{"BGT_branches_when_Z0_N0_V0", M68K_CC_GT, 0x0000, true},
		{"BGT_branches_when_Z0_N1_V1", M68K_CC_GT, M68K_SR_N | M68K_SR_V, true},
		{"BGT_no_branch_when_Z1", M68K_CC_GT, M68K_SR_Z, false},
		{"BGT_no_branch_when_N_neq_V", M68K_CC_GT, M68K_SR_N, false},

		// CC_LE (15) - Less or Equal (signed): Z=1 OR N!=V
		{"BLE_branches_when_Z1", M68K_CC_LE, M68K_SR_Z, true},
		{"BLE_branches_when_N1_V0", M68K_CC_LE, M68K_SR_N, true},
		{"BLE_branches_when_N0_V1", M68K_CC_LE, M68K_SR_V, true},
		{"BLE_no_branch_when_Z0_N_eq_V", M68K_CC_LE, 0x0000, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x1000
			cpu.SR = M68K_SR_S | tc.srFlags // Supervisor mode + test flags

			// Create Bcc opcode: 0x6000 | (condition << 8) | displacement
			// Using displacement 0x08 (8 bytes forward from end of opcode word)
			opcode := uint16(0x6000 | (uint16(tc.condition) << 8) | 0x08)
			cpu.Write16(cpu.PC, opcode)

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			// Check result
			if tc.shouldBranch {
				// PC = 0x1002 (after fetch) - 2 (M68K_WORD_SIZE) + 8 (displacement) = 0x1008
				expectedPC := uint32(0x1008)
				if cpu.PC != expectedPC {
					t.Errorf("Expected PC=0x%08X after branch, got PC=0x%08X", expectedPC, cpu.PC)
				}
			} else {
				// PC should advance past the instruction (2 bytes)
				expectedPC := uint32(0x1002)
				if cpu.PC != expectedPC {
					t.Errorf("Expected PC=0x%08X (no branch), got PC=0x%08X", expectedPC, cpu.PC)
				}
			}
		})
	}
}

// Test BRA with different displacement sizes
func TestBraDisplacementSizes(t *testing.T) {
	// Test byte displacement (positive)
	t.Run("BRA_byte_positive", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		// BRA with byte displacement +16
		cpu.Write16(cpu.PC, 0x6010)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
		// After fetch, PC = 0x1002
		// targetPC = 0x1002 - 2 + 16 = 0x1010
		if cpu.PC != 0x1010 {
			t.Errorf("Expected PC=0x1010, got PC=0x%08X", cpu.PC)
		}
	})

	// Test byte displacement (negative)
	t.Run("BRA_byte_negative", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.PC = 0x1010
		cpu.SR = M68K_SR_S
		// BRA with byte displacement -8 (0xF8)
		cpu.Write16(cpu.PC, 0x60F8)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
		// After fetch, PC = 0x1012
		// targetPC = 0x1012 - 2 + (-8) = 0x1008
		if cpu.PC != 0x1008 {
			t.Errorf("Expected PC=0x1008, got PC=0x%08X", cpu.PC)
		}
	})

	// Test word displacement
	t.Run("BRA_word_displacement", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		// BRA with word displacement: byte = 0x00
		cpu.Write16(cpu.PC, 0x6000)   // BRA.W
		cpu.Write16(cpu.PC+2, 0x0100) // displacement = 256
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
		// After fetch opcode, PC = 0x1002
		// Fetch displacement, PC = 0x1004
		// targetPC = 0x1004 - 2 + 256 = 0x1102
		if cpu.PC != 0x1102 {
			t.Errorf("Expected PC=0x1102, got PC=0x%08X", cpu.PC)
		}
	})
}

// Test BSR (Branch to Subroutine)
func TestBsrSystematic(t *testing.T) {
	t.Run("BSR_forward", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.PC = 0x1000
		cpu.AddrRegs[7] = 0x10000 // Above default stackLowerBound (0x2000)
		cpu.SR = M68K_SR_S

		// BSR with byte displacement +8: opcode = 0x6108
		cpu.Write16(cpu.PC, 0x6108)

		// Execute
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// After fetching opcode, PC = 0x1002
		// BSR pushes PC (0x1002) then branches
		// targetPC = 0x1002 - 2 + 8 = 0x1008
		expectedPC := uint32(0x1008)
		if cpu.PC != expectedPC {
			t.Errorf("Expected PC=0x%08X, got PC=0x%08X", expectedPC, cpu.PC)
		}

		// Check SP was decremented by 4
		expectedSP := uint32(0x0FFFC)
		if cpu.AddrRegs[7] != expectedSP {
			t.Errorf("Expected SP=0x%08X, got SP=0x%08X", expectedSP, cpu.AddrRegs[7])
		}

		// Check return address on stack
		pushedAddr := cpu.Read32(cpu.AddrRegs[7])
		expectedReturnAddr := uint32(0x1002)
		if pushedAddr != expectedReturnAddr {
			t.Errorf("Expected return address 0x%08X on stack, got 0x%08X", expectedReturnAddr, pushedAddr)
		}
	})
}

// Test DBcc (Decrement and Branch on Condition)
func TestDBccSystematic(t *testing.T) {
	// DBRA (DBF) - Always loop (condition F never true)
	t.Run("DBRA_loops_when_count_nonzero", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		cpu.DataRegs[0] = 5 // Count

		// DBRA D0,loop: opcode = 0x51C8, displacement = -4 (0xFFFC)
		cpu.Write16(cpu.PC, 0x51C8)   // DBRA D0 (condition F = 1)
		cpu.Write16(cpu.PC+2, 0xFFFC) // displacement -4

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Condition F is never true, so decrement and branch
		// After fetch opcode, PC = 0x1002
		// After fetch displacement, PC = 0x1004
		// targetPC = 0x1004 - 2 + (-4) = 0x0FFE
		if cpu.PC != 0x0FFE {
			t.Errorf("Expected PC=0x0FFE (branch), got PC=0x%08X", cpu.PC)
		}
		// Counter should be 4 (decremented from 5)
		if cpu.DataRegs[0]&0xFFFF != 4 {
			t.Errorf("Expected D0=4, got D0=%d", cpu.DataRegs[0]&0xFFFF)
		}
	})

	t.Run("DBRA_exits_when_count_minus1", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		cpu.DataRegs[0] = 0 // Count will become -1

		cpu.Write16(cpu.PC, 0x51C8)
		cpu.Write16(cpu.PC+2, 0xFFFC)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Counter becomes -1, so fall through (no branch)
		// PC should be after the instruction: 0x1004
		if cpu.PC != 0x1004 {
			t.Errorf("Expected PC=0x1004 (fall through), got PC=0x%08X", cpu.PC)
		}
		// Counter should be 0xFFFF (-1)
		if cpu.DataRegs[0]&0xFFFF != 0xFFFF {
			t.Errorf("Expected D0=0xFFFF, got D0=0x%04X", cpu.DataRegs[0]&0xFFFF)
		}
	})

	t.Run("DBEQ_exits_when_condition_met", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S | M68K_SR_Z // Z=1 means EQ is true
		cpu.DataRegs[0] = 5

		// DBEQ D0: 0x57C8
		cpu.Write16(cpu.PC, 0x57C8)
		cpu.Write16(cpu.PC+2, 0xFFFC)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Condition EQ is true (Z=1), so exit without decrementing
		if cpu.PC != 0x1004 {
			t.Errorf("Expected PC=0x1004 (condition met), got PC=0x%08X", cpu.PC)
		}
		// Counter should NOT be decremented
		if cpu.DataRegs[0]&0xFFFF != 5 {
			t.Errorf("Expected D0=5 (unchanged), got D0=%d", cpu.DataRegs[0]&0xFFFF)
		}
	})

	t.Run("DBEQ_loops_when_condition_not_met", func(t *testing.T) {
		cpu := setupTestCPU()
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S // Z=0 means EQ is false
		cpu.DataRegs[0] = 5

		cpu.Write16(cpu.PC, 0x57C8)
		cpu.Write16(cpu.PC+2, 0xFFFC)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Condition EQ is false (Z=0), so decrement and branch
		if cpu.PC != 0x0FFE {
			t.Errorf("Expected PC=0x0FFE (branch), got PC=0x%08X", cpu.PC)
		}
		if cpu.DataRegs[0]&0xFFFF != 4 {
			t.Errorf("Expected D0=4 (decremented), got D0=%d", cpu.DataRegs[0]&0xFFFF)
		}
	})
}

// Test Scc (Set Byte on Condition)
func TestSccSystematic(t *testing.T) {
	tests := []struct {
		name      string
		condition uint8
		srFlags   uint16
		expected  uint8 // 0xFF if condition true, 0x00 if false
	}{
		// ST always sets
		{"ST_sets_FF", M68K_CC_T, 0x0000, 0xFF},
		{"ST_sets_FF_with_flags", M68K_CC_T, M68K_SR_N | M68K_SR_Z, 0xFF},

		// SF always clears
		{"SF_sets_00", M68K_CC_F, 0x0000, 0x00},
		{"SF_sets_00_with_flags", M68K_CC_F, M68K_SR_N | M68K_SR_Z, 0x00},

		// SEQ sets when Z=1
		{"SEQ_sets_FF_when_Z1", M68K_CC_EQ, M68K_SR_Z, 0xFF},
		{"SEQ_sets_00_when_Z0", M68K_CC_EQ, 0x0000, 0x00},

		// SNE sets when Z=0
		{"SNE_sets_FF_when_Z0", M68K_CC_NE, 0x0000, 0xFF},
		{"SNE_sets_00_when_Z1", M68K_CC_NE, M68K_SR_Z, 0x00},

		// SCC sets when C=0
		{"SCC_sets_FF_when_C0", M68K_CC_CC, 0x0000, 0xFF},
		{"SCC_sets_00_when_C1", M68K_CC_CC, M68K_SR_C, 0x00},

		// SCS sets when C=1
		{"SCS_sets_FF_when_C1", M68K_CC_CS, M68K_SR_C, 0xFF},
		{"SCS_sets_00_when_C0", M68K_CC_CS, 0x0000, 0x00},

		// SMI sets when N=1
		{"SMI_sets_FF_when_N1", M68K_CC_MI, M68K_SR_N, 0xFF},
		{"SMI_sets_00_when_N0", M68K_CC_MI, 0x0000, 0x00},

		// SPL sets when N=0
		{"SPL_sets_FF_when_N0", M68K_CC_PL, 0x0000, 0xFF},
		{"SPL_sets_00_when_N1", M68K_CC_PL, M68K_SR_N, 0x00},

		// SGE sets when N=V
		{"SGE_sets_FF_when_N0_V0", M68K_CC_GE, 0x0000, 0xFF},
		{"SGE_sets_FF_when_N1_V1", M68K_CC_GE, M68K_SR_N | M68K_SR_V, 0xFF},
		{"SGE_sets_00_when_N_neq_V", M68K_CC_GE, M68K_SR_N, 0x00},

		// SLT sets when N!=V
		{"SLT_sets_FF_when_N_neq_V", M68K_CC_LT, M68K_SR_N, 0xFF},
		{"SLT_sets_00_when_N_eq_V", M68K_CC_LT, 0x0000, 0x00},

		// SGT sets when Z=0 AND N=V
		{"SGT_sets_FF_when_Z0_N_eq_V", M68K_CC_GT, 0x0000, 0xFF},
		{"SGT_sets_00_when_Z1", M68K_CC_GT, M68K_SR_Z, 0x00},
		{"SGT_sets_00_when_N_neq_V", M68K_CC_GT, M68K_SR_N, 0x00},

		// SLE sets when Z=1 OR N!=V
		{"SLE_sets_FF_when_Z1", M68K_CC_LE, M68K_SR_Z, 0xFF},
		{"SLE_sets_FF_when_N_neq_V", M68K_CC_LE, M68K_SR_N, 0xFF},
		{"SLE_sets_00_when_Z0_N_eq_V", M68K_CC_LE, 0x0000, 0x00},

		// SHI sets when C=0 AND Z=0
		{"SHI_sets_FF_when_C0_Z0", M68K_CC_HI, 0x0000, 0xFF},
		{"SHI_sets_00_when_C1", M68K_CC_HI, M68K_SR_C, 0x00},
		{"SHI_sets_00_when_Z1", M68K_CC_HI, M68K_SR_Z, 0x00},

		// SLS sets when C=1 OR Z=1
		{"SLS_sets_FF_when_C1", M68K_CC_LS, M68K_SR_C, 0xFF},
		{"SLS_sets_FF_when_Z1", M68K_CC_LS, M68K_SR_Z, 0xFF},
		{"SLS_sets_00_when_C0_Z0", M68K_CC_LS, 0x0000, 0x00},

		// SVS sets when V=1
		{"SVS_sets_FF_when_V1", M68K_CC_VS, M68K_SR_V, 0xFF},
		{"SVS_sets_00_when_V0", M68K_CC_VS, 0x0000, 0x00},

		// SVC sets when V=0
		{"SVC_sets_FF_when_V0", M68K_CC_VC, 0x0000, 0xFF},
		{"SVC_sets_00_when_V1", M68K_CC_VC, M68K_SR_V, 0x00},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x1000
			cpu.SR = M68K_SR_S | tc.srFlags
			cpu.DataRegs[0] = 0x12345678 // Pre-fill with known value

			// Scc D0 opcode: 0x50C0 | (condition << 8) | reg
			opcode := uint16(0x50C0 | (uint16(tc.condition) << 8) | 0)
			cpu.Write16(cpu.PC, opcode)

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			// Check only low byte was affected
			expected := uint32(0x12345600) | uint32(tc.expected)
			if cpu.DataRegs[0] != expected {
				t.Errorf("Expected D0=0x%08X, got D0=0x%08X", expected, cpu.DataRegs[0])
			}
		})
	}
}

// Test Scc with memory addressing
func TestSccMemory(t *testing.T) {
	cpu := setupTestCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S | M68K_SR_Z // Z=1 for SEQ
	cpu.AddrRegs[0] = 0x2000
	cpu.Write8(0x2000, 0x00)

	// SEQ (A0) opcode: 0x57D0 (Scc with mode=010, reg=0)
	// 0x50C0 | (7 << 8) | (2 << 3) | 0 = 0x57D0
	opcode := uint16(0x50C0 | (M68K_CC_EQ << 8) | (M68K_AM_AR_IND << 3) | 0)
	cpu.Write16(cpu.PC, opcode)

	// Execute
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Check memory was set to 0xFF
	result := cpu.Read8(0x2000)
	if result != 0xFF {
		t.Errorf("Expected memory[0x2000]=0xFF, got 0x%02X", result)
	}
}

// Test JMP (Jump)
func TestJmpSystematic(t *testing.T) {
	tests := []struct {
		name       string
		mode       uint16
		reg        uint16
		setupAddr  uint32
		targetAddr uint32
	}{
		// JMP (An) - Address Register Indirect
		{"JMP_An_indirect", M68K_AM_AR_IND, 0, 0x2000, 0x2000},
		// JMP d(An) - Address Register Indirect with Displacement
		{"JMP_An_disp", M68K_AM_AR_DISP, 0, 0x2000, 0x2010}, // d=16
		// JMP xxx.W - Absolute Short
		{"JMP_abs_short", 7, 0, 0, 0x3000},
		// JMP xxx.L - Absolute Long
		{"JMP_abs_long", 7, 1, 0, 0x40000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x1000
			cpu.SR = M68K_SR_S

			// JMP opcode: 0x4EC0 | (mode << 3) | reg
			opcode := uint16(0x4EC0 | (tc.mode << 3) | tc.reg)
			cpu.Write16(cpu.PC, opcode)

			switch tc.mode {
			case M68K_AM_AR_IND:
				cpu.AddrRegs[tc.reg] = tc.targetAddr
			case M68K_AM_AR_DISP:
				cpu.AddrRegs[tc.reg] = tc.setupAddr
				cpu.Write16(cpu.PC+2, 0x0010) // displacement = 16
			case 7:
				if tc.reg == 0 { // Absolute short
					cpu.Write16(cpu.PC+2, uint16(tc.targetAddr))
				} else { // Absolute long
					cpu.Write32(cpu.PC+2, tc.targetAddr)
				}
			}

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			if cpu.PC != tc.targetAddr {
				t.Errorf("Expected PC=0x%08X, got PC=0x%08X", tc.targetAddr, cpu.PC)
			}
		})
	}
}

// Test JSR (Jump to Subroutine)
func TestJsrSystematic(t *testing.T) {
	tests := []struct {
		name       string
		mode       uint16
		reg        uint16
		targetAddr uint32
		initialSP  uint32
	}{
		{"JSR_An_indirect", M68K_AM_AR_IND, 0, 0x2000, 0x3000},
		{"JSR_abs_short", 7, 0, 0x4000, 0x3000},
		{"JSR_abs_long", 7, 1, 0x50000, 0x3000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x1000
			cpu.SR = M68K_SR_S
			cpu.AddrRegs[7] = tc.initialSP

			// JSR opcode: 0x4E80 | (mode << 3) | reg
			opcode := uint16(0x4E80 | (tc.mode << 3) | tc.reg)
			cpu.Write16(cpu.PC, opcode)

			var expectedReturnPC uint32
			switch tc.mode {
			case M68K_AM_AR_IND:
				cpu.AddrRegs[tc.reg] = tc.targetAddr
				expectedReturnPC = 0x1002
			case 7:
				if tc.reg == 0 { // Absolute short
					cpu.Write16(cpu.PC+2, uint16(tc.targetAddr))
					expectedReturnPC = 0x1004
				} else { // Absolute long
					cpu.Write32(cpu.PC+2, tc.targetAddr)
					expectedReturnPC = 0x1006
				}
			}

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			// Check PC
			if cpu.PC != tc.targetAddr {
				t.Errorf("Expected PC=0x%08X, got PC=0x%08X", tc.targetAddr, cpu.PC)
			}

			// Check return address on stack
			pushedAddr := cpu.Read32(cpu.AddrRegs[7])
			if pushedAddr != expectedReturnPC {
				t.Errorf("Expected return addr 0x%08X on stack, got 0x%08X", expectedReturnPC, pushedAddr)
			}

			// Check SP decremented
			expectedSP := tc.initialSP - 4
			if cpu.AddrRegs[7] != expectedSP {
				t.Errorf("Expected SP=0x%08X, got SP=0x%08X", expectedSP, cpu.AddrRegs[7])
			}
		})
	}
}

// Test RTS (Return from Subroutine)
func TestRtsSystematic(t *testing.T) {
	tests := []struct {
		name      string
		returnPC  uint32
		initialSP uint32
	}{
		{"RTS_normal", 0x1234, 0x2000},
		{"RTS_high_address", 0x80000, 0x3000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x5000
			cpu.SR = M68K_SR_S
			cpu.AddrRegs[7] = tc.initialSP

			// Push return address on stack
			cpu.AddrRegs[7] -= 4
			cpu.Write32(cpu.AddrRegs[7], tc.returnPC)

			// RTS opcode: 0x4E75
			cpu.Write16(cpu.PC, 0x4E75)

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			// Check PC restored
			if cpu.PC != tc.returnPC {
				t.Errorf("Expected PC=0x%08X, got PC=0x%08X", tc.returnPC, cpu.PC)
			}

			// Check SP restored
			if cpu.AddrRegs[7] != tc.initialSP {
				t.Errorf("Expected SP=0x%08X, got SP=0x%08X", tc.initialSP, cpu.AddrRegs[7])
			}
		})
	}
}

// Test RTE (Return from Exception) - privileged instruction
func TestRteSystematic(t *testing.T) {
	tests := []struct {
		name       string
		savedSR    uint16
		savedPC    uint32
		initialSP  uint32
		supervisor bool
	}{
		{"RTE_supervisor_mode", M68K_SR_S | 0x0010, 0x1234, 0x2000, true},
		{"RTE_to_user_mode", 0x0000, 0x5678, 0x2000, true},
		{"RTE_privilege_violation", 0x0000, 0x1234, 0x2000, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x5000
			if tc.supervisor {
				cpu.SR = M68K_SR_S
			} else {
				cpu.SR = 0 // User mode
			}
			cpu.AddrRegs[7] = tc.initialSP

			// Push exception frame: SR, PC, format/offset word
			cpu.AddrRegs[7] -= 2
			cpu.Write16(cpu.AddrRegs[7], 0) // Format 0, vector offset 0
			cpu.AddrRegs[7] -= 4
			cpu.Write32(cpu.AddrRegs[7], tc.savedPC)
			cpu.AddrRegs[7] -= 2
			cpu.Write16(cpu.AddrRegs[7], tc.savedSR)

			stackBeforeRTE := cpu.AddrRegs[7]

			// RTE opcode: 0x4E73
			cpu.Write16(cpu.PC, 0x4E73)

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			if tc.supervisor {
				// Check SR restored
				if cpu.SR != tc.savedSR {
					t.Errorf("Expected SR=0x%04X, got SR=0x%04X", tc.savedSR, cpu.SR)
				}
				// Check PC restored
				if cpu.PC != tc.savedPC {
					t.Errorf("Expected PC=0x%08X, got PC=0x%08X", tc.savedPC, cpu.PC)
				}
				// Check SP restored (popped 8 bytes: SR, PC, format/offset)
				expectedSP := stackBeforeRTE + 8
				if cpu.AddrRegs[7] != expectedSP {
					t.Errorf("Expected SP=0x%08X, got SP=0x%08X", expectedSP, cpu.AddrRegs[7])
				}
			} else {
				// Should have triggered privilege violation
				// PC should not be restored
				if cpu.PC == tc.savedPC {
					t.Errorf("RTE should have caused privilege violation, but PC was restored")
				}
			}
		})
	}
}

// Test RTR (Return and Restore CCR)
func TestRtrSystematic(t *testing.T) {
	tests := []struct {
		name      string
		savedCCR  uint16
		savedPC   uint32
		initialSP uint32
		initialSR uint16
	}{
		{"RTR_restore_CCR", 0x001F, 0x1234, 0x2000, M68K_SR_S}, // All CCR flags set
		{"RTR_clear_CCR", 0x0000, 0x5678, 0x2000, M68K_SR_S | 0x001F},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x5000
			cpu.SR = tc.initialSR
			cpu.AddrRegs[7] = tc.initialSP

			// Push return frame: CCR word then PC
			cpu.AddrRegs[7] -= 4
			cpu.Write32(cpu.AddrRegs[7], tc.savedPC)
			cpu.AddrRegs[7] -= 2
			cpu.Write16(cpu.AddrRegs[7], tc.savedCCR)

			// RTR opcode: 0x4E77
			cpu.Write16(cpu.PC, 0x4E77)

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			// Check CCR restored (only low 5 bits)
			expectedSR := (tc.initialSR & 0xFF00) | (tc.savedCCR & 0x00FF)
			if cpu.SR != expectedSR {
				t.Errorf("Expected SR=0x%04X, got SR=0x%04X", expectedSR, cpu.SR)
			}

			// Check PC restored
			if cpu.PC != tc.savedPC {
				t.Errorf("Expected PC=0x%08X, got PC=0x%08X", tc.savedPC, cpu.PC)
			}

			// Check SP restored
			if cpu.AddrRegs[7] != tc.initialSP {
				t.Errorf("Expected SP=0x%08X, got SP=0x%08X", tc.initialSP, cpu.AddrRegs[7])
			}
		})
	}
}

// Test TRAP instruction
func TestTrapSystematic(t *testing.T) {
	tests := []struct {
		name       string
		trapNum    uint16
		vectorAddr uint32
	}{
		{"TRAP_0", 0, 0x80},   // Vector 32
		{"TRAP_1", 1, 0x84},   // Vector 33
		{"TRAP_15", 15, 0xBC}, // Vector 47
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x1000
			cpu.SR = M68K_SR_S
			cpu.AddrRegs[7] = 0x10000 // Above default stackLowerBound (0x2000)

			// Set up trap handler address in vector table
			handlerAddr := uint32(0x5000 + tc.trapNum*0x100)
			cpu.Write32(tc.vectorAddr, handlerAddr)

			// TRAP #n opcode: 0x4E40 | n
			opcode := uint16(0x4E40 | tc.trapNum)
			cpu.Write16(cpu.PC, opcode)

			initialSP := cpu.AddrRegs[7]
			initialSR := cpu.SR
			initialPC := cpu.PC + 2 // PC after fetching opcode

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			// Check exception frame was pushed
			// Stack should have: SR (2 bytes), PC (4 bytes), format/offset (2 bytes)
			expectedSP := initialSP - 8
			if cpu.AddrRegs[7] != expectedSP {
				t.Errorf("Expected SP=0x%08X after exception, got SP=0x%08X", expectedSP, cpu.AddrRegs[7])
			}

			// Check saved SR on stack
			savedSR := cpu.Read16(cpu.AddrRegs[7])
			if savedSR != initialSR {
				t.Errorf("Expected saved SR=0x%04X, got 0x%04X", initialSR, savedSR)
			}

			// Check saved PC on stack
			savedPC := cpu.Read32(cpu.AddrRegs[7] + 2)
			if savedPC != initialPC {
				t.Errorf("Expected saved PC=0x%08X, got 0x%08X", initialPC, savedPC)
			}

			// Check PC jumped to handler
			if cpu.PC != handlerAddr {
				t.Errorf("Expected PC=0x%08X (handler), got PC=0x%08X", handlerAddr, cpu.PC)
			}
		})
	}
}

// Test TRAPV (Trap on Overflow)
func TestTrapvSystematic(t *testing.T) {
	tests := []struct {
		name       string
		vFlag      bool
		shouldTrap bool
	}{
		{"TRAPV_traps_when_V1", true, true},
		{"TRAPV_no_trap_when_V0", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupTestCPU()
			cpu.PC = 0x1000
			cpu.SR = M68K_SR_S
			if tc.vFlag {
				cpu.SR |= M68K_SR_V
			}
			cpu.AddrRegs[7] = 0x10000 // Above default stackLowerBound (0x2000)

			// Set up TRAPV handler
			handlerAddr := uint32(0x6000)
			cpu.Write32(M68K_VEC_TRAPV*4, handlerAddr)

			// TRAPV opcode: 0x4E76
			cpu.Write16(cpu.PC, 0x4E76)

			initialSP := cpu.AddrRegs[7]

			// Execute
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			if tc.shouldTrap {
				// Check exception was taken
				if cpu.PC != handlerAddr {
					t.Errorf("Expected PC=0x%08X (TRAPV handler), got PC=0x%08X", handlerAddr, cpu.PC)
				}
				// Check stack was pushed
				expectedSP := initialSP - 8
				if cpu.AddrRegs[7] != expectedSP {
					t.Errorf("Expected SP=0x%08X, got SP=0x%08X", expectedSP, cpu.AddrRegs[7])
				}
			} else {
				// Check no exception
				expectedPC := uint32(0x1002)
				if cpu.PC != expectedPC {
					t.Errorf("Expected PC=0x%08X (no trap), got PC=0x%08X", expectedPC, cpu.PC)
				}
				// Check stack unchanged
				if cpu.AddrRegs[7] != initialSP {
					t.Errorf("Expected SP unchanged at 0x%08X, got SP=0x%08X", initialSP, cpu.AddrRegs[7])
				}
			}
		})
	}
}

// Test JSR/RTS sequence (subroutine call and return)
func TestJsrRtsSequence(t *testing.T) {
	cpu := setupTestCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x3000

	// Write subroutine at 0x2000 that just returns
	subroutineAddr := uint32(0x2000)
	cpu.Write16(subroutineAddr, 0x4E75) // RTS

	// JSR to subroutine (absolute long)
	// JSR = 0x4E80 | (mode << 3) | reg = 0x4E80 | (7 << 3) | 1 = 0x4EB9
	cpu.Write16(cpu.PC, 0x4EB9) // JSR xxx.L
	cpu.Write32(cpu.PC+2, subroutineAddr)

	// Execute JSR
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Verify we're at subroutine
	if cpu.PC != subroutineAddr {
		t.Errorf("After JSR: Expected PC=0x%08X, got PC=0x%08X", subroutineAddr, cpu.PC)
	}

	// Return address should be 0x1006 (after JSR instruction)
	returnAddr := cpu.Read32(cpu.AddrRegs[7])
	expectedReturn := uint32(0x1006)
	if returnAddr != expectedReturn {
		t.Errorf("Return address on stack: Expected 0x%08X, got 0x%08X", expectedReturn, returnAddr)
	}

	// Execute RTS
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Verify we returned
	if cpu.PC != expectedReturn {
		t.Errorf("After RTS: Expected PC=0x%08X, got PC=0x%08X", expectedReturn, cpu.PC)
	}

	// Stack should be restored
	if cpu.AddrRegs[7] != 0x3000 {
		t.Errorf("After RTS: Expected SP=0x3000, got SP=0x%08X", cpu.AddrRegs[7])
	}
}

// Test DBcc loop construct
func TestDBccLoopConstruct(t *testing.T) {
	cpu := setupTestCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 3 // Loop 4 times (3, 2, 1, 0, then exit on -1)
	cpu.DataRegs[1] = 0 // Accumulator

	// Loop body: ADD.W #1,D1
	loopStart := uint32(0x1000)
	cpu.Write16(loopStart, 0x5241)   // ADDQ.W #1,D1
	cpu.Write16(loopStart+2, 0x51C8) // DBRA D0,loop (DBF D0,loop)
	cpu.Write16(loopStart+4, 0xFFFC) // displacement -4 (back to loop start)

	// Execute loop
	for i := 0; i < 10; i++ { // Max 10 iterations to prevent infinite loop
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.PC == loopStart+6 { // Exited loop
			break
		}
	}

	// Should have added 4 times (counts 3,2,1,0 then exits)
	if cpu.DataRegs[1] != 4 {
		t.Errorf("Expected D1=4 after loop, got D1=%d", cpu.DataRegs[1])
	}

	// Counter should be 0xFFFF (-1)
	counter := uint16(cpu.DataRegs[0])
	if counter != 0xFFFF {
		t.Errorf("Expected D0=0xFFFF after loop, got D0=0x%04X", counter)
	}
}

// Test conditional branch chain
func TestBccChain(t *testing.T) {
	cpu := setupTestCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	// Test: Branch based on comparison result
	cpu.DataRegs[0] = 5
	cpu.DataRegs[1] = 10

	// CMP.L D0,D1 (compare D1 with D0: D1-D0)
	cpu.Write16(0x1000, 0xB280) // CMP.L D0,D1

	// BLT forward (if D1 < D0, branch)
	cpu.Write16(0x1002, 0x6D06) // BLT.S +6

	// BEQ forward (if D1 == D0, branch)
	cpu.Write16(0x1004, 0x6704) // BEQ.S +4

	// BGT forward (if D1 > D0, should take this one)
	// After fetch at 0x1006, PC = 0x1008, target = 0x1008 - 2 + 4 = 0x100A
	cpu.Write16(0x1006, 0x6E04) // BGT.S +4

	// Target locations (NOP)
	cpu.Write16(0x1008, 0x4E71) // NOP (fall through)
	cpu.Write16(0x100A, 0x4E71) // NOP (BGT target)
	cpu.Write16(0x100C, 0x4E71) // NOP (BEQ target)
	cpu.Write16(0x100E, 0x4E71) // NOP (BLT target)

	// Execute CMP
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Execute BLT (should not branch since D1 > D0)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()
	if cpu.PC != 0x1004 {
		t.Errorf("BLT should not branch when D1>D0, PC=0x%08X", cpu.PC)
	}

	// Execute BEQ (should not branch since D1 != D0)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()
	if cpu.PC != 0x1006 {
		t.Errorf("BEQ should not branch when D1!=D0, PC=0x%08X", cpu.PC)
	}

	// Execute BGT (should branch since D1 > D0)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()
	expectedPC := uint32(0x100A) // 0x1006 + 2 + 2
	if cpu.PC != expectedPC {
		t.Errorf("BGT should branch when D1>D0, expected PC=0x%08X, got PC=0x%08X", expectedPC, cpu.PC)
	}
}
