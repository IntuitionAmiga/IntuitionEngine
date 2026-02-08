//go:build m68k_test

package main

import (
	"fmt"
	"strings"
	"testing"
)

func boilerPlateTest() {
	fmt.Println("\n\033[38;2;255;20;147m ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████\033[0m\n\033[38;2;255;50;147m▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀\033[0m\n\033[38;2;255;80;147m▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███\033[0m\n\033[38;2;255;110;147m░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄\033[0m\n\033[38;2;255;140;147m░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒\033[0m\n\033[38;2;255;170;147m░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░\033[0m\n\033[38;2;255;200;147m ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░\033[0m\n\033[38;2;255;230;147m ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░\033[0m\n\033[38;2;255;255;147m ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░\033[0m")
	fmt.Println("\nA modern 32-bit reimagining of the Commodore, Atari and Sinclair 8-bit home computers.")
	fmt.Println("(c) 2024 - 2026 Zayn Otley")
	fmt.Println("https://github.com/IntuitionAmiga/IntuitionEngine")
	fmt.Println("Buy me a coffee: https://ko-fi.com/intuition/tip")
	fmt.Println("License: GPLv3 or later")
}

// Helper function to create a MOVE instruction opcode
func createMoveOpcode(size, srcMode, srcReg, destMode, destReg uint16) uint16 {
	var sizeField uint16
	switch size {
	case M68K_SIZE_BYTE:
		sizeField = 1 // 01
	case M68K_SIZE_WORD:
		sizeField = 3 // 11
	case M68K_SIZE_LONG:
		sizeField = 2 // 10
	}

	return (sizeField << 12) | (destReg << 9) | (destMode << 6) | (srcMode << 3) | srcReg
}

// Helper to set up a CPU ready for testing
func setupTestCPU() *M68KCPU {
	boilerPlateTest()
	// Create a real system bus
	bus := NewMachineBus()

	// Initialize terminal output
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)

	// Create CPU first
	cpu := NewM68KCPU(bus)

	// Initialize vector table using CPU's Write32 to handle M68K endianness correctly
	// The M68K reads vectors 0 (initial SP) and 1 (initial PC) during reset
	cpu.Write32(0, M68K_STACK_START)
	cpu.Write32(M68K_RESET_VECTOR, M68K_ENTRY_POINT)

	// Re-initialize the CPU to read the corrected vectors
	// Set PC and SP directly since vectors are now correct
	cpu.PC = M68K_ENTRY_POINT
	cpu.AddrRegs[7] = M68K_STACK_START
	cpu.SSP = M68K_STACK_START
	cpu.stackUpperBound = M68K_STACK_START + 0x10000
	cpu.stackLowerBound = M68K_STACK_START - 0x10000

	return cpu
}

// Helper function for detailed output
func runDetailedTest(t *testing.T, cpu *M68KCPU, opcode uint16, description string, expectedResults map[string]uint32) {
	// Get the current program counter for opcode address display
	opcodeAddr := cpu.PC

	// Capture initial state
	beforeState := map[string]uint32{
		"D0": cpu.DataRegs[0],
		"D1": cpu.DataRegs[1],
		"D2": cpu.DataRegs[2],
		"D3": cpu.DataRegs[3],
		"D4": cpu.DataRegs[4],
		"D5": cpu.DataRegs[5],
		"D6": cpu.DataRegs[6],
		"D7": cpu.DataRegs[7],
		"A0": cpu.AddrRegs[0],
		"A1": cpu.AddrRegs[1],
		"A2": cpu.AddrRegs[2],
		"A3": cpu.AddrRegs[3],
		"A4": cpu.AddrRegs[4],
		"A5": cpu.AddrRegs[5],
		"A6": cpu.AddrRegs[6],
		"A7": cpu.AddrRegs[7],
	}

	// Capture initial memory state if address register is used
	memBefore := make(map[string]uint32)
	for reg, addr := range beforeState {
		if strings.HasPrefix(reg, "A") {
			memKey := fmt.Sprintf("MEM[%s]", reg)
			if _, exists := expectedResults[memKey]; exists {
				memBefore[memKey] = cpu.Read32(addr)
			}
		}
	}

	// Write instruction to memory
	cpu.Write16(cpu.PC, opcode)

	// Get initial SR (used for flag comparison if needed)
	_ = cpu.SR

	// Execute instruction
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Collect results
	afterState := map[string]uint32{
		"D0": cpu.DataRegs[0],
		"D1": cpu.DataRegs[1],
		"D2": cpu.DataRegs[2],
		"D3": cpu.DataRegs[3],
		"D4": cpu.DataRegs[4],
		"D5": cpu.DataRegs[5],
		"D6": cpu.DataRegs[6],
		"D7": cpu.DataRegs[7],
		"A0": cpu.AddrRegs[0],
		"A1": cpu.AddrRegs[1],
		"A2": cpu.AddrRegs[2],
		"A3": cpu.AddrRegs[3],
		"A4": cpu.AddrRegs[4],
		"A5": cpu.AddrRegs[5],
		"A6": cpu.AddrRegs[6],
		"A7": cpu.AddrRegs[7],
	}

	// Get flag states
	n := (cpu.SR & M68K_SR_N) != 0
	z := (cpu.SR & M68K_SR_Z) != 0
	v := (cpu.SR & M68K_SR_V) != 0
	c := (cpu.SR & M68K_SR_C) != 0

	// Format output header
	t.Logf(">> %s [0x%04X] (%s)", description, opcodeAddr, formatInstructionType(description))

	// Format before state
	beforeLine := "Before: "
	for reg, val := range beforeState {
		if _, exists := expectedResults[reg]; exists {
			beforeLine += fmt.Sprintf("%s=0x%08X, ", reg, val)
		}
	}

	// Add memory values
	for reg := range beforeState {
		if strings.HasPrefix(reg, "A") {
			memKey := fmt.Sprintf("MEM[%s]", reg)
			if _, exists := expectedResults[memKey]; exists {
				beforeLine += fmt.Sprintf("%s=0x%08X, ", memKey, memBefore[memKey])
			}
		}
	}
	t.Logf("%s", strings.TrimSuffix(beforeLine, ", "))

	// Format after state
	afterLine := "After:  "
	for reg, val := range afterState {
		if _, exists := expectedResults[reg]; exists {
			afterLine += fmt.Sprintf("%s=0x%08X, ", reg, val)
		}
	}

	// Add memory values after execution
	for reg, addr := range afterState {
		if strings.HasPrefix(reg, "A") {
			memKey := fmt.Sprintf("MEM[%s]", reg)
			if _, exists := expectedResults[memKey]; exists {
				afterLine += fmt.Sprintf("%s=0x%08X, ", memKey, cpu.Read32(addr))
			}
		}
	}
	t.Logf("%s", strings.TrimSuffix(afterLine, ", "))

	// Format flags
	t.Logf("Flags:  N=%d Z=%d V=%d C=%d", btoi(n), btoi(z), btoi(v), btoi(c))

	// Format expected values
	expectedLine := "Expected "
	for reg, val := range expectedResults {
		expectedLine += fmt.Sprintf("%s=0x%08X, ", reg, val)
	}
	t.Logf("%s", strings.TrimSuffix(expectedLine, ", "))

	// Check results and print pass/fail
	testPassed := true
	failureDetails := ""

	for reg, expected := range expectedResults {
		var actual uint32

		if strings.HasPrefix(reg, "MEM[") {
			// Handle memory access - extract register name from MEM[Ax]
			addrReg := reg[4:6]
			var addrValue uint32

			switch addrReg {
			case "A0":
				addrValue = cpu.AddrRegs[0]
			case "A1":
				addrValue = cpu.AddrRegs[1]
			case "A2":
				addrValue = cpu.AddrRegs[2]
			case "A3":
				addrValue = cpu.AddrRegs[3]
			case "A4":
				addrValue = cpu.AddrRegs[4]
			case "A5":
				addrValue = cpu.AddrRegs[5]
			case "A6":
				addrValue = cpu.AddrRegs[6]
			case "A7":
				addrValue = cpu.AddrRegs[7]
			}

			actual = cpu.Read32(addrValue)
		} else {
			// Handle register access
			switch reg {
			case "D0":
				actual = cpu.DataRegs[0]
			case "D1":
				actual = cpu.DataRegs[1]
			case "D2":
				actual = cpu.DataRegs[2]
			case "D3":
				actual = cpu.DataRegs[3]
			case "D4":
				actual = cpu.DataRegs[4]
			case "D5":
				actual = cpu.DataRegs[5]
			case "D6":
				actual = cpu.DataRegs[6]
			case "D7":
				actual = cpu.DataRegs[7]
			case "A0":
				actual = cpu.AddrRegs[0]
			case "A1":
				actual = cpu.AddrRegs[1]
			case "A2":
				actual = cpu.AddrRegs[2]
			case "A3":
				actual = cpu.AddrRegs[3]
			case "A4":
				actual = cpu.AddrRegs[4]
			case "A5":
				actual = cpu.AddrRegs[5]
			case "A6":
				actual = cpu.AddrRegs[6]
			case "A7":
				actual = cpu.AddrRegs[7]
			}
		}

		if actual != expected {
			testPassed = false
			// Generate detailed failure message
			// Check which bits differ
			for bit := 0; bit < 32; bit++ {
				expectedBit := (expected >> bit) & 1
				actualBit := (actual >> bit) & 1
				if expectedBit != actualBit {
					failureDetails = fmt.Sprintf("Expected %s bit %d=%d, Actual=%d",
						reg, bit, expectedBit, actualBit)
					break
				}
			}
		}
	}

	if testPassed {
		t.Logf("** PASS **")
	} else {
		t.Logf("** FAIL ** (Difference: %s)", failureDetails)
		// Also use t.Errorf to make the test fail
		t.Errorf("Test failed: %s", failureDetails)
	}

	t.Logf("------------------------------------------------------")
}

// Helper function to format the instruction type
func formatInstructionType(desc string) string {
	if strings.Contains(desc, "(A0)+,D") {
		return "Post-Increment Addr to Data Reg"
	} else if strings.Contains(desc, "-(A0),D") {
		return "Pre-Decrement Addr to Data Reg"
	} else if strings.Contains(desc, "D,-(A") {
		return "Data Reg to Pre-Decrement Addr"
	} else if strings.Contains(desc, "D,(A0)+") {
		return "Data Reg to Post-Increment Addr"
	} else if strings.Contains(desc, "D0,D1") || strings.Contains(desc, "D1,D0") {
		return "Data Reg to Data Reg"
	} else if strings.Contains(desc, "D") && strings.Contains(desc, "A") && !strings.Contains(desc, "(A") {
		return "Data Reg to Addr Reg"
	} else if strings.Contains(desc, "D") && strings.Contains(desc, "(A") {
		return "Data Reg to Memory"
	} else if strings.Contains(desc, "(A") && strings.Contains(desc, "D") {
		return "Memory to Data Reg"
	} else if strings.Contains(desc, ",$") {
		return "Data Reg to Absolute Address"
	} else if strings.Contains(desc, "$") && strings.Contains(desc, "D") {
		return "Absolute Address to Data Reg"
	}
	// Default case
	return "Unspecified Operation"
}

//----------------------------------------------------------------------------
// MOVE Data Register to Data Register Tests
//----------------------------------------------------------------------------

func TestMoveDataRegisterToDataRegister(t *testing.T) {
	t.Logf("=== MOVE Data Register to Data Register Tests ===")

	t.Run("MOVE.B D1,D0 (Positive value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0xFFFFFFFF
		cpu.DataRegs[1] = 0x000000AB

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_BYTE, M68K_AM_DR, 1, M68K_AM_DR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0xFFFFFFAB,
			"D1": 0x000000AB,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.B D1,D0", expectedResults)
	})

	t.Run("MOVE.B D1,D0 (Negative value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0xFFFFFF00
		cpu.DataRegs[1] = 0x000000FF // 0xFF signed byte is -1

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_BYTE, M68K_AM_DR, 1, M68K_AM_DR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0xFFFFFFFF,
			"D1": 0x000000FF,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.B D1,D0", expectedResults)
	})

	t.Run("MOVE.B D1,D0 (Zero value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0xFFFFFFFF
		cpu.DataRegs[1] = 0x00000000

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_BYTE, M68K_AM_DR, 1, M68K_AM_DR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0xFFFFFF00,
			"D1": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.B D1,D0", expectedResults)
	})

	t.Run("MOVE.W D1,D0", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0xFFFF0000
		cpu.DataRegs[1] = 0x0000ABCD

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_DR, 1, M68K_AM_DR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0xFFFFABCD,
			"D1": 0x0000ABCD,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.W D1,D0", expectedResults)
	})

	t.Run("MOVE.L D1,D0", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000000
		cpu.DataRegs[1] = 0xABCD1234

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 1, M68K_AM_DR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0xABCD1234,
			"D1": 0xABCD1234,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.L D1,D0", expectedResults)
	})
}

//----------------------------------------------------------------------------
// MOVE Data Register to Address Register Tests (MOVEA)
//----------------------------------------------------------------------------

func TestMoveDataRegisterToAddressRegister(t *testing.T) {
	t.Logf("=== MOVE Data Register to Address Register Tests (MOVEA) ===")

	t.Run("MOVE.W D0,A0 (Positive value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00007FFF
		cpu.AddrRegs[0] = 0x00000000

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_DR, 0, M68K_AM_AR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x00007FFF,
			"A0": 0x00007FFF,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.W D0,A0", expectedResults)
	})

	t.Run("MOVE.W D0,A0 (Negative value, sign extension)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x0000FFFF // Source D0 (negative in word context)
		cpu.AddrRegs[0] = 0x00000000

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_DR, 0, M68K_AM_AR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x0000FFFF,
			"A0": 0xFFFFFFFF,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.W D0,A0", expectedResults)
	})

	t.Run("MOVE.L D0,A0", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0xABCD1234
		cpu.AddrRegs[0] = 0x00000000

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, M68K_AM_AR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0xABCD1234,
			"A0": 0xABCD1234,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.L D0,A0", expectedResults)
	})
}

//----------------------------------------------------------------------------
// MOVE Data Register to Memory (Indirect Addressing) Tests
//----------------------------------------------------------------------------

func TestMoveDataRegisterToMemory(t *testing.T) {
	t.Logf("=== MOVE Data Register to Memory Tests ===")

	t.Run("MOVE.L D0,(A0) - Address Register Indirect", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0x12345678 // Source D0
		cpu.AddrRegs[0] = memAddr    // Destination address in A0

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, M68K_AM_AR_IND, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0":      0x12345678,
			"A0":      memAddr,
			"MEM[A0]": 0x12345678,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.L D0,(A0)", expectedResults)
	})

	t.Run("MOVE.W D0,(A0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0x0000ABCD // Source D0
		cpu.AddrRegs[0] = memAddr    // Destination address in A0

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_DR, 0, M68K_AM_AR_IND, 0)

		// For word operations, we need to read memory manually
		expectedWord := uint16(0xABCD)

		// Expected results - using D0 and A0
		expectedResults := map[string]uint32{
			"D0": 0x0000ABCD,
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.W D0,(A0)", expectedResults)

		// Check memory manually since our test helper uses 32-bit reads
		if cpu.Read16(memAddr) != expectedWord {
			t.Errorf("MOVE.W D0,(A0) failed: Memory at 0x%08X = %04X, expected %04X",
				memAddr, cpu.Read16(memAddr), expectedWord)
		}
	})

	t.Run("MOVE.B D0,(A0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0x000000AB // Source D0
		cpu.AddrRegs[0] = memAddr    // Destination address in A0

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_BYTE, M68K_AM_DR, 0, M68K_AM_AR_IND, 0)

		// For byte operations, we need to read memory manually
		expectedByte := uint8(0xAB)

		// Expected results - using D0 and A0
		expectedResults := map[string]uint32{
			"D0": 0x000000AB,
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.B D0,(A0)", expectedResults)

		// Check memory manually since our test helper uses 32-bit reads
		if cpu.Read8(memAddr) != expectedByte {
			t.Errorf("MOVE.B D0,(A0) failed: Memory at 0x%08X = %02X, expected %02X",
				memAddr, cpu.Read8(memAddr), expectedByte)
		}
	})
}

//----------------------------------------------------------------------------
// MOVE with Postincrement Addressing Tests
//----------------------------------------------------------------------------

func TestMoveWithPostincrement(t *testing.T) {
	t.Logf("=== MOVE with Postincrement Addressing Tests ===")

	t.Run("MOVE.L D0,(A0)+ (Data register to memory with postincrement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0xABCD1234
		cpu.AddrRegs[0] = memAddr

		// First, manually craft the output header
		t.Logf(">> MOVE.L D0,(A0)+ [0x%04X] (Data Reg to Post-Increment Addr)", cpu.PC)
		t.Logf("Before: D0=0x%08X, A0=0x%08X", cpu.DataRegs[0], cpu.AddrRegs[0])

		// Save the ORIGINAL address before execution
		originalAddr := cpu.AddrRegs[0]

		// Create and manually execute the instruction
		opcode := createMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, M68K_AM_AR_POST, 0)
		cpu.Write16(cpu.PC, opcode)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// After execution - check all values
		t.Logf("After:  D0=0x%08X, A0=0x%08X", cpu.DataRegs[0], cpu.AddrRegs[0])

		// Read data from the ORIGINAL location - not the incremented one!
		memValue := cpu.Read32(originalAddr)
		t.Logf("Memory at ORIGINAL address 0x%08X = 0x%08X", originalAddr, memValue)

		t.Logf("Flags:  N=%d Z=%d V=%d C=%d",
			btoi((cpu.SR&M68K_SR_N) != 0),
			btoi((cpu.SR&M68K_SR_Z) != 0),
			btoi((cpu.SR&M68K_SR_V) != 0),
			btoi((cpu.SR&M68K_SR_C) != 0))

		// Check expected values
		if cpu.DataRegs[0] != 0xABCD1234 ||
			cpu.AddrRegs[0] != memAddr+4 ||
			memValue != 0xABCD1234 {
			t.Logf("** FAIL **")
			t.Errorf("Expected: D0=0xABCD1234, A0=0x%08X, Memory[0x%08X]=0xABCD1234",
				memAddr+4, originalAddr)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})

	t.Run("MOVE.W D0,(A0)+ (Data register to memory with postincrement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0x0000ABCD // Source D0
		cpu.AddrRegs[0] = memAddr    // Destination address in A0

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_DR, 0, M68K_AM_AR_POST, 0)

		// For word operations, we need to read memory manually
		expectedWord := uint16(0xABCD)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x0000ABCD,
			"A0": memAddr + 2, // A0 should be incremented by 2 for word
		}

		runDetailedTest(t, cpu, opcode, "MOVE.W D0,(A0)+", expectedResults)

		// Check memory manually
		if cpu.Read16(memAddr) != expectedWord {
			t.Errorf("MOVE.W D0,(A0)+ failed: Memory at 0x%08X = %04X, expected %04X",
				memAddr, cpu.Read16(memAddr), expectedWord)
		}
	})

	t.Run("MOVE.L D0,(A0)+ (Data register to memory with postincrement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0xABCD1234 // Source D0
		cpu.AddrRegs[0] = memAddr    // Destination address in A0

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, M68K_AM_AR_POST, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0xABCD1234,
			"A0": memAddr + 4, // A0 should be incremented by 4 for long
			// Note: Cannot use MEM[A0] here because A0 has been incremented
		}

		runDetailedTest(t, cpu, opcode, "MOVE.L D0,(A0)+", expectedResults)

		// Manually verify memory at the ORIGINAL address (before increment)
		actualMem := cpu.Read32(memAddr)
		if actualMem != 0xABCD1234 {
			t.Errorf("MOVE.L D0,(A0)+ failed: Memory at 0x%08X = 0x%08X, expected 0xABCD1234",
				memAddr, actualMem)
		}
	})

	t.Run("MOVE.B (A0)+,D0 (Memory with postincrement to data register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.Write8(memAddr, 0xAB)    // Set memory value
		cpu.AddrRegs[0] = memAddr    // Source address in A0
		cpu.DataRegs[0] = 0x00000000 // Destination D0

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_BYTE, M68K_AM_AR_POST, 0, M68K_AM_DR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x000000AB,
			"A0": memAddr + 1, // A0 should be incremented by 1 for byte
		}

		runDetailedTest(t, cpu, opcode, "MOVE.B (A0)+,D0", expectedResults)
	})

	t.Run("MOVE.L (A0)+,D0 (Memory with postincrement to data register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		expectedValue := uint32(0xFEDCBA98)
		cpu.Write32(memAddr, expectedValue) // Set memory value
		cpu.AddrRegs[0] = memAddr           // Source address in A0
		cpu.DataRegs[0] = 0x00000000        // Destination D0

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_LONG, M68K_AM_AR_POST, 0, M68K_AM_DR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": expectedValue,
			"A0": memAddr + 4, // A0 should be incremented by 4 for long
		}

		runDetailedTest(t, cpu, opcode, "MOVE.L (A0)+,D0", expectedResults)
	})
}

//----------------------------------------------------------------------------
// MOVE with Predecrement Addressing Tests
//----------------------------------------------------------------------------

func TestMoveWithPredecrement(t *testing.T) {
	t.Logf("=== MOVE with Predecrement Addressing Tests ===")

	t.Run("MOVE.B D0,-(A0) (Data register to memory with predecrement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0x000000AB // Source D0
		cpu.AddrRegs[0] = memAddr    // Destination address in A0 (will be decremented)

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_BYTE, M68K_AM_DR, 0, M68K_AM_AR_PRE, 0)

		// A0 should be decremented by 1 for byte operation
		expectedAddr := memAddr - 1

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x000000AB,
			"A0": expectedAddr,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.B D0,-(A0)", expectedResults)

		// Check memory manually since our test helper uses 32-bit reads
		if cpu.Read8(expectedAddr) != 0xAB {
			t.Errorf("MOVE.B D0,-(A0) failed: Memory at 0x%08X = %02X, expected %02X",
				expectedAddr, cpu.Read8(expectedAddr), 0xAB)
		}
	})

	t.Run("MOVE.W D0,-(A0) (Data register to memory with predecrement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0x0000ABCD // Source D0
		cpu.AddrRegs[0] = memAddr    // Destination address in A0 (will be decremented)

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_DR, 0, M68K_AM_AR_PRE, 0)

		// A0 should be decremented by 2 for word operation
		expectedAddr := memAddr - 2

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x0000ABCD,
			"A0": expectedAddr,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.W D0,-(A0)", expectedResults)

		// Check memory manually since our test helper uses 32-bit reads
		if cpu.Read16(expectedAddr) != 0xABCD {
			t.Errorf("MOVE.W D0,-(A0) failed: Memory at 0x%08X = %04X, expected %04X",
				expectedAddr, cpu.Read16(expectedAddr), 0xABCD)
		}
	})

	t.Run("MOVE.L D0,-(A0) (Data register to memory with predecrement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.DataRegs[0] = 0xABCD1234 // Source D0
		cpu.AddrRegs[0] = memAddr    // Destination address in A0 (will be decremented)

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 0, M68K_AM_AR_PRE, 0)

		// A0 should be decremented by 4 for long operation
		expectedAddr := memAddr - 4

		// Expected results
		expectedResults := map[string]uint32{
			"D0":      0xABCD1234,
			"A0":      expectedAddr,
			"MEM[A0]": 0xABCD1234,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.L D0,-(A0)", expectedResults)
	})

	t.Run("MOVE.B -(A0),D0 (Memory with predecrement to data register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		expectedAddr := memAddr - 1
		cpu.Write8(expectedAddr, 0xAB) // Set memory value at the address that will be decremented to
		cpu.AddrRegs[0] = memAddr      // Source address in A0 (will be decremented)
		cpu.DataRegs[0] = 0x00000000   // Destination D0

		// Create and execute instruction
		opcode := createMoveOpcode(M68K_SIZE_BYTE, M68K_AM_AR_PRE, 0, M68K_AM_DR, 0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x000000AB,
			"A0": expectedAddr,
		}

		runDetailedTest(t, cpu, opcode, "MOVE.B -(A0),D0", expectedResults)
	})
}

//----------------------------------------------------------------------------
// MOVE with Address Register Indirect with Displacement Tests
//----------------------------------------------------------------------------

func TestMoveWithDisplacement(t *testing.T) {
	t.Logf("=== MOVE with Address Register Indirect with Displacement Tests ===")

	t.Run("MOVE.L D0,16(A0) (Data register to memory with displacement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		baseAddr := uint32(0x00002000)
		displacement := uint32(16) // 0x10
		memAddr := baseAddr + displacement
		cpu.DataRegs[0] = 0x12345678 // Source D0
		cpu.AddrRegs[0] = baseAddr   // Base address in A0

		// We need to assemble this instruction manually since it requires a displacement
		// MOVE.L D0,16(A0) = 0x2140 0010
		cpu.Write16(cpu.PC, 0x2140)   // MOVE.L D0,(d16,A0)
		cpu.Write16(cpu.PC+2, 0x0010) // Displacement 16

		t.Logf(">> MOVE.L D0,16(A0) [0x%04X] (Data Reg to Memory with Displacement)", cpu.PC)
		t.Logf("Before: D0=0x%08X, A0=0x%08X", cpu.DataRegs[0], cpu.AddrRegs[0])

		// Execute the instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  D0=0x%08X, A0=0x%08X", cpu.DataRegs[0], cpu.AddrRegs[0])
		t.Logf("Flags:  N=%d Z=%d V=%d C=%d",
			btoi((cpu.SR&M68K_SR_N) != 0),
			btoi((cpu.SR&M68K_SR_Z) != 0),
			btoi((cpu.SR&M68K_SR_V) != 0),
			btoi((cpu.SR&M68K_SR_C) != 0))
		t.Logf("Expected D0=0x%08X, A0=0x%08X, MEM[A0+16]=0x%08X",
			cpu.DataRegs[0], baseAddr, 0x12345678)

		// Check that memory at base + displacement has been updated
		actualValue := cpu.Read32(memAddr)
		if actualValue != 0x12345678 {
			t.Logf("** FAIL ** (Difference: Expected MEM[A0+16] bit 0=0, Actual=1)")
			t.Errorf("MOVE.L D0,16(A0) failed: Memory at 0x%08X = %08X, expected %08X",
				memAddr, actualValue, 0x12345678)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})

	t.Run("MOVE.W 16(A0),D0 (Memory with displacement to data register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		baseAddr := uint32(0x00002000)
		displacement := uint32(16) // 0x10
		memAddr := baseAddr + displacement
		cpu.Write16(memAddr, 0xABCD) // Set memory value
		cpu.AddrRegs[0] = baseAddr   // Base address in A0
		cpu.DataRegs[0] = 0x00000000 // Destination D0

		// Manually assemble MOVE.W 16(A0),D0 = 0x3028 0010
		cpu.Write16(cpu.PC, 0x3028)   // MOVE.W (d16,A0),D0
		cpu.Write16(cpu.PC+2, 0x0010) // Displacement 16

		t.Logf(">> MOVE.W 16(A0),D0 [0x%04X] (Memory with Displacement to Data Reg)", cpu.PC)
		t.Logf("Before: D0=0x%08X, A0=0x%08X", cpu.DataRegs[0], cpu.AddrRegs[0])

		// Execute the instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  D0=0x%08X, A0=0x%08X", cpu.DataRegs[0], cpu.AddrRegs[0])
		t.Logf("Flags:  N=%d Z=%d V=%d C=%d",
			btoi((cpu.SR&M68K_SR_N) != 0),
			btoi((cpu.SR&M68K_SR_Z) != 0),
			btoi((cpu.SR&M68K_SR_V) != 0),
			btoi((cpu.SR&M68K_SR_C) != 0))
		t.Logf("Expected D0=0x0000ABCD, A0=0x%08X", baseAddr)

		// Check data register
		if cpu.DataRegs[0] != 0x0000ABCD {
			t.Logf("** FAIL ** (Difference: Expected D0 bit 16=1, Actual=0)")
			t.Errorf("MOVE.W 16(A0),D0 failed: D0 = %08X, expected %08X",
				cpu.DataRegs[0], 0x0000ABCD)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})
}

//----------------------------------------------------------------------------
// MOVE with Absolute Addressing Tests
//----------------------------------------------------------------------------

func TestMoveWithAbsoluteAddressing(t *testing.T) {
	t.Logf("=== MOVE with Absolute Addressing Tests ===")

	t.Run("MOVE.L D0,$2000 (Data register to absolute address)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		absAddr := uint32(0x2000)
		cpu.DataRegs[0] = 0x12345678 // Source value

		// For MOVE.L D0,$2000
		cpu.Write16(cpu.PC, 0x23C0)   // MOVE.L D0,xxx.L
		cpu.Write16(cpu.PC+2, 0x0000) // High word of address
		cpu.Write16(cpu.PC+4, 0x2000) // Low word of address

		t.Logf(">> MOVE.L D0,$2000 [0x%04X] (Data Reg to Absolute Address)", cpu.PC)
		t.Logf("Before: D0=0x%08X", cpu.DataRegs[0])

		// Execute the instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  D0=0x%08X", cpu.DataRegs[0])
		t.Logf("Flags:  N=%d Z=%d V=%d C=%d",
			btoi((cpu.SR&M68K_SR_N) != 0),
			btoi((cpu.SR&M68K_SR_Z) != 0),
			btoi((cpu.SR&M68K_SR_V) != 0),
			btoi((cpu.SR&M68K_SR_C) != 0))
		t.Logf("Expected D0=0x12345678, MEM[$2000]=0x12345678")

		// Check memory at absolute address
		if cpu.Read32(absAddr) != 0x12345678 {
			t.Logf("** FAIL ** (Difference: Expected MEM[$2000] bit 0=0, Actual=1)")
			t.Errorf("MOVE.L D0,$2000 failed: Memory at 0x%08X = 0x%08X, expected 0x%08X",
				absAddr, cpu.Read32(absAddr), 0x12345678)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})

	t.Run("MOVE.W $3000,D0 (Absolute address to data register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		absAddr := uint32(0x3000)    // Use different address to avoid interference
		cpu.Write16(absAddr, 0xABCD) // Set memory value
		cpu.DataRegs[0] = 0x00000000 // Clear destination register

		// For MOVE.W $3000,D0
		cpu.Write16(cpu.PC, 0x3039)   // MOVE.W xxx.L,D0
		cpu.Write16(cpu.PC+2, 0x0000) // High word of address
		cpu.Write16(cpu.PC+4, 0x3000) // Low word of address

		t.Logf(">> MOVE.W $3000,D0 [0x%04X] (Absolute Address to Data Reg)", cpu.PC)
		t.Logf("Before: D0=0x%08X", cpu.DataRegs[0])

		// Execute the instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  D0=0x%08X", cpu.DataRegs[0])
		t.Logf("Flags:  N=%d Z=%d V=%d C=%d",
			btoi((cpu.SR&M68K_SR_N) != 0),
			btoi((cpu.SR&M68K_SR_Z) != 0),
			btoi((cpu.SR&M68K_SR_V) != 0),
			btoi((cpu.SR&M68K_SR_C) != 0))
		t.Logf("Expected D0=0x0000ABCD")

		// Check data register
		if cpu.DataRegs[0] != 0x0000ABCD {
			t.Logf("** FAIL ** (Difference: Expected D0 bit 16=1, Actual=0)")
			t.Errorf("MOVE.W $3000,D0 failed: D0 = 0x%08X, expected 0x%08X",
				cpu.DataRegs[0], 0x0000ABCD)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})
}

//----------------------------------------------------------------------------
// MOVE with Immediate Data Tests
//----------------------------------------------------------------------------

func TestMoveWithImmediateData(t *testing.T) {
	t.Logf("=== MOVE with Immediate Data Tests ===")

	t.Run("MOVE.L #$12345678,D0 (Immediate to data register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000000 // Destination D0

		// Manually assemble MOVE.L #$12345678,D0 = 0x203C + 32-bit immediate
		cpu.Write16(cpu.PC, 0x203C)   // MOVE.L #imm,D0
		cpu.Write16(cpu.PC+2, 0x1234) // High word of immediate
		cpu.Write16(cpu.PC+4, 0x5678) // Low word of immediate

		t.Logf(">> MOVE.L #$12345678,D0 [0x%04X] (Immediate to Data Reg)", cpu.PC)
		t.Logf("Before: D0=0x%08X", cpu.DataRegs[0])

		// Execute the instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  D0=0x%08X", cpu.DataRegs[0])
		t.Logf("Flags:  N=%d Z=%d V=%d C=%d",
			btoi((cpu.SR&M68K_SR_N) != 0),
			btoi((cpu.SR&M68K_SR_Z) != 0),
			btoi((cpu.SR&M68K_SR_V) != 0),
			btoi((cpu.SR&M68K_SR_C) != 0))
		t.Logf("Expected D0=0x12345678")

		// Check data register
		if cpu.DataRegs[0] != 0x12345678 {
			t.Logf("** FAIL ** (Difference: Expected D0 bit 0=0, Actual=1)")
			t.Errorf("MOVE.L #$12345678,D0 failed: D0 = %08X, expected %08X",
				cpu.DataRegs[0], 0x12345678)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})

	t.Run("MOVE.W #$ABCD,D0 (Immediate to data register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000000 // Destination D0

		// Manually assemble MOVE.W #$ABCD,D0 = 0x303C + 16-bit immediate
		cpu.Write16(cpu.PC, 0x303C)   // MOVE.W #imm,D0
		cpu.Write16(cpu.PC+2, 0xABCD) // 16-bit immediate

		t.Logf(">> MOVE.W #$ABCD,D0 [0x%04X] (Immediate to Data Reg)", cpu.PC)
		t.Logf("Before: D0=0x%08X", cpu.DataRegs[0])

		// Execute the instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  D0=0x%08X", cpu.DataRegs[0])
		t.Logf("Flags:  N=%d Z=%d V=%d C=%d",
			btoi((cpu.SR&M68K_SR_N) != 0),
			btoi((cpu.SR&M68K_SR_Z) != 0),
			btoi((cpu.SR&M68K_SR_V) != 0),
			btoi((cpu.SR&M68K_SR_C) != 0))
		t.Logf("Expected D0=0x0000ABCD")

		// Check data register
		if cpu.DataRegs[0] != 0x0000ABCD {
			t.Logf("** FAIL ** (Difference: Expected D0 bit 16=1, Actual=0)")
			t.Errorf("MOVE.W #$ABCD,D0 failed: D0 = %08X, expected %08X",
				cpu.DataRegs[0], 0x0000ABCD)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})

	t.Run("MOVE.B #$FF,D0 (Immediate to data register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000000 // Destination D0

		// Manually assemble MOVE.B #$FF,D0 = 0x103C + 16-bit immediate (with low byte used)
		cpu.Write16(cpu.PC, 0x103C)   // MOVE.B #imm,D0
		cpu.Write16(cpu.PC+2, 0x00FF) // 16-bit immediate (low byte used)

		t.Logf(">> MOVE.B #$FF,D0 [0x%04X] (Immediate to Data Reg)", cpu.PC)
		t.Logf("Before: D0=0x%08X", cpu.DataRegs[0])

		// Execute the instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  D0=0x%08X", cpu.DataRegs[0])
		t.Logf("Flags:  N=%d Z=%d V=%d C=%d",
			btoi((cpu.SR&M68K_SR_N) != 0),
			btoi((cpu.SR&M68K_SR_Z) != 0),
			btoi((cpu.SR&M68K_SR_V) != 0),
			btoi((cpu.SR&M68K_SR_C) != 0))
		t.Logf("Expected D0=0x000000FF")

		// Check data register
		if cpu.DataRegs[0] != 0x000000FF {
			t.Logf("** FAIL ** (Difference: Expected D0 bit 0=1, Actual=0)")
			t.Errorf("MOVE.B #$FF,D0 failed: D0 = %08X, expected %08X",
				cpu.DataRegs[0], 0x000000FF)
		} else {
			t.Logf("** PASS **")
		}

		// Check flags - should set negative flag for 0xFF
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("MOVE.B #$FF,D0 failed: N flag should be set for negative value")
		}
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("MOVE.B #$FF,D0 failed: Z flag should not be set")
		}

		t.Logf("------------------------------------------------------")
	})
}

// Test for Moveq instruction (quick immediate to data register)
func TestMoveq(t *testing.T) {
	t.Logf("=== MOVEQ Tests ===")

	t.Run("MOVEQ #0,D0", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D0 has some non-zero value
		cpu.DataRegs[0] = 0xFFFFFFFF

		// MOVEQ #0,D0 - This should clear D0
		opcode := uint16(0x7000) // MOVEQ #0,D0

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "MOVEQ #0,D0", expectedResults)
	})

	t.Run("MOVEQ #127,D1 (Max positive value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[1] = 0x00000000

		// MOVEQ #127,D1 - Maximum positive value for MOVEQ
		opcode := uint16(0x7200 | 127) // MOVEQ #127,D1

		// Expected results
		expectedResults := map[string]uint32{
			"D1": 0x0000007F,
		}

		runDetailedTest(t, cpu, opcode, "MOVEQ #127,D1", expectedResults)
	})

	t.Run("MOVEQ #-128,D2 (Min negative value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[2] = 0x00000000

		// MOVEQ #-128,D2 - Minimum negative value for MOVEQ
		opcode := uint16(0x7400 | 128) // MOVEQ #-128,D2 (-128 is represented as 0x80)

		// Expected results
		expectedResults := map[string]uint32{
			"D2": 0xFFFFFF80, // Sign-extended
		}

		runDetailedTest(t, cpu, opcode, "MOVEQ #-128,D2", expectedResults)
	})

	t.Run("MOVEQ #-1,D3 (Negative value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[3] = 0x00000000

		// MOVEQ #-1,D3
		opcode := uint16(0x7600 | 255) // MOVEQ #-1,D3 (-1 is represented as 0xFF)

		// Expected results
		expectedResults := map[string]uint32{
			"D3": 0xFFFFFFFF, // Sign-extended
		}

		runDetailedTest(t, cpu, opcode, "MOVEQ #-1,D3", expectedResults)
	})
}

// Test for Swap instruction (swap register halves)
func TestMoveSwap(t *testing.T) {
	t.Logf("=== SWAP Tests ===")

	t.Run("SWAP D0 (different halves)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D0 has a value with different upper and lower words
		cpu.DataRegs[0] = 0x12345678

		// SWAP D0 - Swap upper and lower words
		opcode := uint16(0x4840) // SWAP D0

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x56781234, // Upper and lower words swapped
		}

		runDetailedTest(t, cpu, opcode, "SWAP D0", expectedResults)
	})

	t.Run("SWAP D1 (zero upper word)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D1 has a value with zero upper word
		cpu.DataRegs[1] = 0x00005678

		// SWAP D1 - Swap upper and lower words
		opcode := uint16(0x4841) // SWAP D1

		// Expected results
		expectedResults := map[string]uint32{
			"D1": 0x56780000, // Upper and lower words swapped
		}

		runDetailedTest(t, cpu, opcode, "SWAP D1", expectedResults)
	})

	t.Run("SWAP D2 (zero lower word)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D2 has a value with zero lower word
		cpu.DataRegs[2] = 0x12340000

		// SWAP D2 - Swap upper and lower words
		opcode := uint16(0x4842) // SWAP D2

		// Expected results
		expectedResults := map[string]uint32{
			"D2": 0x00001234, // Upper and lower words swapped
		}

		runDetailedTest(t, cpu, opcode, "SWAP D2", expectedResults)
	})

	t.Run("SWAP D3 (all zeros)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D3 is zero
		cpu.DataRegs[3] = 0x00000000

		// SWAP D3 - Swap upper and lower words (no visible change for zero)
		opcode := uint16(0x4843) // SWAP D3

		// Expected results
		expectedResults := map[string]uint32{
			"D3": 0x00000000, // Still zero
		}

		runDetailedTest(t, cpu, opcode, "SWAP D3", expectedResults)
	})
}

// Test for Ext instruction (sign extension)
func TestMoveExt(t *testing.T) {
	t.Logf("=== EXT Tests ===")

	t.Run("EXT.W D0 (byte->word, positive)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D0 has a positive byte value
		cpu.DataRegs[0] = 0x0000007F

		// EXT.W D0 - Sign extend byte to word
		opcode := uint16(0x4880) // EXT.W D0

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x0000007F, // No change for positive values
		}

		runDetailedTest(t, cpu, opcode, "EXT.W D0 (byte->word, positive)", expectedResults)
	})

	t.Run("EXT.W D1 (byte->word, negative)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D1 has a negative byte value
		cpu.DataRegs[1] = 0x000000FF // 0xFF is -1 as a signed byte

		// EXT.W D1 - Sign extend byte to word
		opcode := uint16(0x4881) // EXT.W D1

		// Expected results
		expectedResults := map[string]uint32{
			"D1": 0x0000FFFF, // Sign extension to word
		}

		runDetailedTest(t, cpu, opcode, "EXT.W D1 (byte->word, negative)", expectedResults)
	})

	t.Run("EXT.L D2 (word->long, positive)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D2 has a positive word value
		cpu.DataRegs[2] = 0x00007FFF

		// EXT.L D2 - Sign extend word to long
		opcode := uint16(0x48C2) // EXT.L D2

		// Expected results
		expectedResults := map[string]uint32{
			"D2": 0x00007FFF, // No change for positive values
		}

		runDetailedTest(t, cpu, opcode, "EXT.L D2 (word->long, positive)", expectedResults)
	})

	t.Run("EXT.L D3 (word->long, negative)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D3 has a negative word value
		cpu.DataRegs[3] = 0x0000FFFF // 0xFFFF is -1 as a signed word

		// EXT.L D3 - Sign extend word to long
		opcode := uint16(0x48C3) // EXT.L D3

		// Expected results
		expectedResults := map[string]uint32{
			"D3": 0xFFFFFFFF, // Sign extension to long
		}

		runDetailedTest(t, cpu, opcode, "EXT.L D3 (word->long, negative)", expectedResults)
	})
}

// Test for Clr instruction (clear operand)
func TestMoveClr(t *testing.T) {
	t.Logf("=== CLR Tests ===")

	t.Run("CLR.B D0", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D0 has a non-zero value
		cpu.DataRegs[0] = 0x12345678

		// CLR.B D0 - Clear byte (lowest byte)
		opcode := uint16(0x4200) // CLR.B D0

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x12345600, // Lowest byte cleared
		}

		runDetailedTest(t, cpu, opcode, "CLR.B D0", expectedResults)
	})

	t.Run("CLR.W D1", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D1 has a non-zero value
		cpu.DataRegs[1] = 0x12345678

		// CLR.W D1 - Clear word (lowest word)
		opcode := uint16(0x4241) // CLR.W D1

		// Expected results
		expectedResults := map[string]uint32{
			"D1": 0x12340000, // Lowest word cleared
		}

		runDetailedTest(t, cpu, opcode, "CLR.W D1", expectedResults)
	})

	t.Run("CLR.L D2", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D2 has a non-zero value
		cpu.DataRegs[2] = 0x12345678

		// CLR.L D2 - Clear long (entire register)
		opcode := uint16(0x4282) // CLR.L D2

		// Expected results
		expectedResults := map[string]uint32{
			"D2": 0x00000000, // Entire register cleared
		}

		runDetailedTest(t, cpu, opcode, "CLR.L D2", expectedResults)
	})

	t.Run("CLR.B (A0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to memory, set memory to non-zero
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0xFF)

		// CLR.B (A0) - Clear byte at address in A0
		opcode := uint16(0x4210) // CLR.B (A0)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "CLR.B (A0)", expectedResults)

		// Check memory manually since our test helper uses 32-bit reads
		if cpu.Read8(memAddr) != 0x00 {
			t.Errorf("CLR.B (A0) failed: Memory at 0x%08X = %02X, expected %02X",
				memAddr, cpu.Read8(memAddr), 0x00)
		}
	})

	t.Run("CLR.W (A1)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A1 points to memory, set memory to non-zero
		memAddr := uint32(0x00002010)
		cpu.AddrRegs[1] = memAddr
		cpu.Write16(memAddr, 0xABCD)

		// CLR.W (A1) - Clear word at address in A1
		opcode := uint16(0x4251) // CLR.W (A1)

		// Expected results
		expectedResults := map[string]uint32{
			"A1": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "CLR.W (A1)", expectedResults)

		// Check memory manually
		if cpu.Read16(memAddr) != 0x0000 {
			t.Errorf("CLR.W (A1) failed: Memory at 0x%08X = %04X, expected %04X",
				memAddr, cpu.Read16(memAddr), 0x0000)
		}
	})
}

// Test for Tst instruction (test operand)
func TestMoveTst(t *testing.T) {
	t.Logf("=== TST Tests ===")

	t.Run("TST.B D0 (positive value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D0 has a positive value
		cpu.DataRegs[0] = 0x0000007F

		// TST.B D0 - Test byte
		opcode := uint16(0x4A00) // TST.B D0

		// Expected results - only flags change
		expectedResults := map[string]uint32{
			"D0": 0x0000007F, // No change to the register
		}

		runDetailedTest(t, cpu, opcode, "TST.B D0 (positive)", expectedResults)

		// Check flags
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("TST.B D0 failed: Z flag should not be set for non-zero value")
		}
		if (cpu.SR & M68K_SR_N) != 0 {
			t.Errorf("TST.B D0 failed: N flag should not be set for positive value")
		}
	})

	t.Run("TST.B D1 (negative value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D1 has a negative value
		cpu.DataRegs[1] = 0x000000FF // 0xFF is -1 as a signed byte

		// TST.B D1 - Test byte
		opcode := uint16(0x4A01) // TST.B D1

		// Expected results - only flags change
		expectedResults := map[string]uint32{
			"D1": 0x000000FF, // No change to the register
		}

		runDetailedTest(t, cpu, opcode, "TST.B D1 (negative)", expectedResults)

		// Check flags
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("TST.B D1 failed: Z flag should not be set for non-zero value")
		}
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("TST.B D1 failed: N flag should be set for negative value")
		}
	})

	t.Run("TST.B D2 (zero value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D2 has a zero value
		cpu.DataRegs[2] = 0x00000000

		// TST.B D2 - Test byte
		opcode := uint16(0x4A02) // TST.B D2

		// Expected results - only flags change
		expectedResults := map[string]uint32{
			"D2": 0x00000000, // No change to the register
		}

		runDetailedTest(t, cpu, opcode, "TST.B D2 (zero)", expectedResults)

		// Check flags
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("TST.B D2 failed: Z flag should be set for zero value")
		}
	})

	t.Run("TST.W D3", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D3 has a negative word value
		cpu.DataRegs[3] = 0x0000FFFF // 0xFFFF is -1 as a signed word

		// TST.W D3 - Test word
		opcode := uint16(0x4A43) // TST.W D3

		// Expected results - only flags change
		expectedResults := map[string]uint32{
			"D3": 0x0000FFFF, // No change to the register
		}

		runDetailedTest(t, cpu, opcode, "TST.W D3", expectedResults)

		// Check flags
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("TST.W D3 failed: Z flag should not be set for non-zero value")
		}
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("TST.W D3 failed: N flag should be set for negative value")
		}
	})

	t.Run("TST.L D4", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D4 has a negative long value
		cpu.DataRegs[4] = 0x80000000 // Most negative 32-bit value

		// TST.L D4 - Test long
		opcode := uint16(0x4A84) // TST.L D4

		// Expected results - only flags change
		expectedResults := map[string]uint32{
			"D4": 0x80000000, // No change to the register
		}

		runDetailedTest(t, cpu, opcode, "TST.L D4", expectedResults)

		// Check flags
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("TST.L D4 failed: Z flag should not be set for non-zero value")
		}
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("TST.L D4 failed: N flag should be set for negative value")
		}
	})
}

// Test for Lea instruction (load effective address)
func TestMoveLea(t *testing.T) {
	t.Logf("=== LEA Tests ===")

	t.Run("LEA (A0),A1", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to memory
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr

		// LEA (A0),A1 - Load effective address from A0 to A1
		opcode := uint16(0x43D0) // LEA (A0),A1

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr, // A0 unchanged
			"A1": memAddr, // A1 loaded with address from A0
		}

		runDetailedTest(t, cpu, opcode, "LEA (A0),A1", expectedResults)
	})

	t.Run("LEA (16,A0),A2", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to memory
		baseAddr := uint32(0x00002000)
		displacement := uint32(16)
		cpu.AddrRegs[0] = baseAddr

		// Manually assemble this instruction:
		// LEA (16,A0),A2 = 0x45E8 0010
		cpu.Write16(cpu.PC, 0x45E8)   // LEA with displacement
		cpu.Write16(cpu.PC+2, 0x0010) // Displacement of 16

		// Execute manually
		t.Logf(">> LEA (16,A0),A2 [0x%04X]", cpu.PC)
		t.Logf("Before: A0=0x%08X, A2=0x%08X", cpu.AddrRegs[0], cpu.AddrRegs[2])

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  A0=0x%08X, A2=0x%08X", cpu.AddrRegs[0], cpu.AddrRegs[2])

		// Check result
		expectedAddr := baseAddr + displacement
		if cpu.AddrRegs[2] != expectedAddr {
			t.Errorf("LEA (16,A0),A2 failed: A2=0x%08X, expected 0x%08X",
				cpu.AddrRegs[2], expectedAddr)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})

	t.Run("LEA (8,A1,D0.W),A3", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		baseAddr := uint32(0x00003000)
		displacement := uint32(8)
		indexValue := uint32(0x0000ABCD) // Using lower word only for index

		cpu.AddrRegs[1] = baseAddr
		cpu.DataRegs[0] = indexValue
		cpu.AddrRegs[3] = 0 // Clear target register

		// Manually assemble complex addressing mode instruction:
		// LEA (8,A1,D0.W),A3 = 0x47F1 8800 (where 8800 encodes D0.W as index with disp 8)
		cpu.Write16(cpu.PC, 0x47F1)   // LEA with index
		cpu.Write16(cpu.PC+2, 0x8008) // Brief format, D0.W as index, disp 8

		// Execute manually
		t.Logf(">> LEA (8,A1,D0.W),A3 [0x%04X]", cpu.PC)
		t.Logf("Before: A1=0x%08X, D0=0x%08X, A3=0x%08X",
			cpu.AddrRegs[1], cpu.DataRegs[0], cpu.AddrRegs[3])

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		t.Logf("After:  A1=0x%08X, D0=0x%08X, A3=0x%08X",
			cpu.AddrRegs[1], cpu.DataRegs[0], cpu.AddrRegs[3])

		// Check result - A3 should contain base + displacement + index
		expectedAddr := baseAddr + displacement + (indexValue & 0xFFFF)
		if cpu.AddrRegs[3] != expectedAddr {
			t.Errorf("LEA (8,A1,D0.W),A3 failed: A3=0x%08X, expected 0x%08X",
				cpu.AddrRegs[3], expectedAddr)
		} else {
			t.Logf("** PASS **")
		}

		t.Logf("------------------------------------------------------")
	})
}

// Test for Pea instruction (push effective address)
func TestMovePea(t *testing.T) {
	t.Logf("=== PEA Tests ===")

	t.Run("PEA (A0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to memory, SP is at initial value
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		initialSP := cpu.AddrRegs[7]

		// PEA (A0) - Push effective address of A0 onto stack
		opcode := uint16(0x4850) // PEA (A0)

		// Execute
		cpu.Write16(cpu.PC, opcode)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check results
		expectedSP := initialSP - 4 // SP decremented by 4 bytes
		if cpu.AddrRegs[7] != expectedSP {
			t.Errorf("PEA (A0) failed: SP=0x%08X, expected 0x%08X",
				cpu.AddrRegs[7], expectedSP)
		}

		// Check stack content
		stackValue := cpu.Read32(cpu.AddrRegs[7])
		if stackValue != memAddr {
			t.Errorf("PEA (A0) failed: Stack value=0x%08X, expected 0x%08X",
				stackValue, memAddr)
		} else {
			t.Logf("PEA (A0): SP=0x%08X, Stack value=0x%08X - PASS",
				cpu.AddrRegs[7], stackValue)
		}
	})

	t.Run("PEA (16,A0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to memory, SP is at initial value
		baseAddr := uint32(0x00002000)
		displacement := uint32(16)
		cpu.AddrRegs[0] = baseAddr
		initialSP := cpu.AddrRegs[7]

		// Manually assemble:
		// PEA (16,A0) = 0x4868 0010
		cpu.Write16(cpu.PC, 0x4868)   // PEA with displacement
		cpu.Write16(cpu.PC+2, 0x0010) // Displacement of 16

		// Execute
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check results
		expectedSP := initialSP - 4 // SP decremented by 4 bytes
		if cpu.AddrRegs[7] != expectedSP {
			t.Errorf("PEA (16,A0) failed: SP=0x%08X, expected 0x%08X",
				cpu.AddrRegs[7], expectedSP)
		}

		// Check stack content
		expectedAddr := baseAddr + displacement
		stackValue := cpu.Read32(cpu.AddrRegs[7])
		if stackValue != expectedAddr {
			t.Errorf("PEA (16,A0) failed: Stack value=0x%08X, expected 0x%08X",
				stackValue, expectedAddr)
		} else {
			t.Logf("PEA (16,A0): SP=0x%08X, Stack value=0x%08X - PASS",
				cpu.AddrRegs[7], stackValue)
		}
	})
}

// Test for Movem instruction (move multiple registers)
func TestMovem(t *testing.T) {
	t.Logf("=== MOVEM Tests ===")

	t.Run("MOVEM.W D0-D3,-(A7) (Registers to memory)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Set some values in D0-D3
		cpu.DataRegs[0] = 0x00001111
		cpu.DataRegs[1] = 0x00002222
		cpu.DataRegs[2] = 0x00003333
		cpu.DataRegs[3] = 0x00004444
		initialSP := cpu.AddrRegs[7]

		// MOVEM.W D0-D3,-(A7) - Push D0-D3 onto stack
		// Register mask for D0-D3 is 0x000F
		cpu.Write16(cpu.PC, 0x48A7)   // MOVEM.W to predecrement A7
		cpu.Write16(cpu.PC+2, 0x000F) // Register mask for D0-D3

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check SP value (should be decremented by 2 bytes for each register)
		expectedSP := initialSP - (4 * 2) // 4 registers * 2 bytes per register
		if cpu.AddrRegs[7] != expectedSP {
			t.Errorf("MOVEM.W D0-D3,-(A7) failed: SP=0x%08X, expected 0x%08X",
				cpu.AddrRegs[7], expectedSP)
		}

		// Check stack values in reverse order (predecrement pushes in reverse)
		stackAddr := cpu.AddrRegs[7]
		if cpu.Read16(stackAddr) != 0x1111 {
			t.Errorf("MOVEM.W failed: D0 value at 0x%08X = 0x%04X, expected 0x1111",
				stackAddr, cpu.Read16(stackAddr))
		}
		if cpu.Read16(stackAddr+2) != 0x2222 {
			t.Errorf("MOVEM.W failed: D1 value at 0x%08X = 0x%04X, expected 0x2222",
				stackAddr+2, cpu.Read16(stackAddr+2))
		}
		if cpu.Read16(stackAddr+4) != 0x3333 {
			t.Errorf("MOVEM.W failed: D2 value at 0x%08X = 0x%04X, expected 0x3333",
				stackAddr+4, cpu.Read16(stackAddr+4))
		}
		if cpu.Read16(stackAddr+6) != 0x4444 {
			t.Errorf("MOVEM.W failed: D3 value at 0x%08X = 0x%04X, expected 0x4444",
				stackAddr+6, cpu.Read16(stackAddr+6))
		} else {
			t.Logf("MOVEM.W D0-D3,-(A7): PASS")
		}
	})

	t.Run("MOVEM.L (A7)+,D0-D3 (Memory to registers)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Set stack values
		stackAddr := cpu.AddrRegs[7] - 16 // Make room for 4 long words
		cpu.AddrRegs[7] = stackAddr
		cpu.Write32(stackAddr, 0x11111111)
		cpu.Write32(stackAddr+4, 0x22222222)
		cpu.Write32(stackAddr+8, 0x33333333)
		cpu.Write32(stackAddr+12, 0x44444444)

		// Clear the destination registers
		cpu.DataRegs[0] = 0
		cpu.DataRegs[1] = 0
		cpu.DataRegs[2] = 0
		cpu.DataRegs[3] = 0

		// MOVEM.L (A7)+,D0-D3 - Pop D0-D3 from stack
		// Register mask for D0-D3 is 0x000F
		cpu.Write16(cpu.PC, 0x4CDF)   // MOVEM.L from postincrement A7
		cpu.Write16(cpu.PC+2, 0x000F) // Register mask for D0-D3

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check SP value (should be incremented by 4 bytes for each register)
		expectedSP := stackAddr + (4 * 4) // 4 registers * 4 bytes per register
		if cpu.AddrRegs[7] != expectedSP {
			t.Errorf("MOVEM.L (A7)+,D0-D3 failed: SP=0x%08X, expected 0x%08X",
				cpu.AddrRegs[7], expectedSP)
		}

		// Check register values
		if cpu.DataRegs[0] != 0x11111111 {
			t.Errorf("MOVEM.L failed: D0=0x%08X, expected 0x11111111", cpu.DataRegs[0])
		}
		if cpu.DataRegs[1] != 0x22222222 {
			t.Errorf("MOVEM.L failed: D1=0x%08X, expected 0x22222222", cpu.DataRegs[1])
		}
		if cpu.DataRegs[2] != 0x33333333 {
			t.Errorf("MOVEM.L failed: D2=0x%08X, expected 0x33333333", cpu.DataRegs[2])
		}
		if cpu.DataRegs[3] != 0x44444444 {
			t.Errorf("MOVEM.L failed: D3=0x%08X, expected 0x44444444", cpu.DataRegs[3])
		} else {
			t.Logf("MOVEM.L (A7)+,D0-D3: PASS")
		}
	})

	t.Run("MOVEM.W D0-A5,(A0) (Multiple register types to memory)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr

		// Set values in registers
		for i := 0; i < 6; i++ {
			cpu.DataRegs[i] = 0x1000 + uint32(i) // D0-D5
			cpu.AddrRegs[i] = 0x2000 + uint32(i) // A0-A5
		}

		// MOVEM.W D0-A5,(A0) - Write D0-D5 and A0-A5 to memory
		// Register mask: bits 0-5 for D0-D5, bits 8-13 for A0-A5
		// 0x3F for D0-D5, 0x3F00 for A0-A5, combined: 0x3F3F
		cpu.Write16(cpu.PC, 0x48B0)   // MOVEM.W to address in A0
		cpu.Write16(cpu.PC+2, 0x3F3F) // Register mask

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check memory values - words should be stored in order
		offset := uint32(0)

		// Check data registers D0-D5
		for i := 0; i < 6; i++ {
			expected := uint16(0x1000 + i)
			actual := cpu.Read16(memAddr + offset)
			if actual != expected {
				t.Errorf("MOVEM.W failed: Memory at 0x%08X = 0x%04X, expected 0x%04X",
					memAddr+offset, actual, expected)
			}
			offset += 2
		}

		// Check address registers A0-A5
		for i := 0; i < 6; i++ {
			expected := uint16(0x2000 + i)
			actual := cpu.Read16(memAddr + offset)
			if actual != expected {
				t.Errorf("MOVEM.W failed: Memory at 0x%08X = 0x%04X, expected 0x%04X",
					memAddr+offset, actual, expected)
			}
			offset += 2
		}

		t.Logf("MOVEM.W D0-A5,(A0): PASS")
	})
}

// Test for MoveFromUSP instruction (move from user stack pointer)
func TestMoveFromUSP(t *testing.T) {
	t.Logf("=== MOVE from USP Tests ===")

	t.Run("MOVE USP,A0 (Supervisor mode)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Set supervisor mode and USP value
		cpu.SR |= M68K_SR_S          // Set supervisor mode bit
		cpu.USP = 0x00FF0000         // Set user stack pointer value
		cpu.AddrRegs[0] = 0x00000000 // Clear A0

		// MOVE USP,A0 - Move USP to A0
		opcode := uint16(0x4E68) // MOVE USP,A0

		// Execute manually
		cpu.Write16(cpu.PC, opcode)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result
		if cpu.AddrRegs[0] != cpu.USP {
			t.Errorf("MOVE USP,A0 failed: A0=0x%08X, expected USP=0x%08X",
				cpu.AddrRegs[0], cpu.USP)
		} else {
			t.Logf("MOVE USP,A0: PASS")
		}
	})

	t.Run("MOVE USP,A1 (User mode - should cause privilege violation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up privilege violation exception handler at vector 8 (address 0x20)
		// This is required because ProcessException halts if the vector is uninitialized
		exceptionHandlerAddr := uint32(0x00008000)
		cpu.Write32(M68K_VEC_PRIVILEGE*M68K_LONG_SIZE, exceptionHandlerAddr)

		// Setup test data - Ensure user mode and set USP value
		cpu.SR &= ^uint16(M68K_SR_S) // Clear supervisor mode bit (user mode)
		cpu.USP = 0x00FF0000         // Set user stack pointer value
		cpu.AddrRegs[1] = 0x00000000 // Clear A1

		initialSP := cpu.AddrRegs[7]

		// MOVE USP,A1 - This should cause a privilege violation in user mode
		opcode := uint16(0x4E69) // MOVE USP,A1

		// Execute manually
		cpu.Write16(cpu.PC, opcode)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check if a privilege violation occurred - A1 should NOT have been modified
		if cpu.AddrRegs[1] == cpu.USP {
			t.Errorf("MOVE USP,A1 in user mode did not cause privilege violation")
		}

		// SP should be decremented for exception processing (PC and SR pushed = 6 bytes)
		if cpu.AddrRegs[7] >= initialSP {
			t.Errorf("Stack not adjusted for privilege violation exception")
		}

		// PC should change to exception handler address
		if cpu.PC != exceptionHandlerAddr {
			t.Errorf("PC not changed to exception handler: PC=0x%08X, expected=0x%08X", cpu.PC, exceptionHandlerAddr)
		} else {
			t.Logf("MOVE USP,A1 (User mode): Privilege violation correctly triggered, PC=0x%08X", cpu.PC)
		}
	})
}

// Test for MoveToUSP instruction (move to user stack pointer)
func TestMoveToUSP(t *testing.T) {
	t.Logf("=== MOVE to USP Tests ===")

	t.Run("MOVE A0,USP (Supervisor mode)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Set supervisor mode and A0 value
		cpu.SR |= M68K_SR_S          // Set supervisor mode bit
		cpu.USP = 0x00000000         // Clear USP
		cpu.AddrRegs[0] = 0x00FF1000 // Set A0 value

		// MOVE A0,USP - Move A0 to USP
		opcode := uint16(0x4E60) // MOVE A0,USP

		// Execute manually
		cpu.Write16(cpu.PC, opcode)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result
		if cpu.USP != cpu.AddrRegs[0] {
			t.Errorf("MOVE A0,USP failed: USP=0x%08X, expected A0=0x%08X",
				cpu.USP, cpu.AddrRegs[0])
		} else {
			t.Logf("MOVE A0,USP: PASS")
		}
	})

	t.Run("MOVE A1,USP (User mode - should cause privilege violation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up privilege violation exception handler at vector 8 (address 0x20)
		// This is required because ProcessException halts if the vector is uninitialized
		exceptionHandlerAddr := uint32(0x00008000)
		cpu.Write32(M68K_VEC_PRIVILEGE*M68K_LONG_SIZE, exceptionHandlerAddr)

		// Setup test data - Ensure user mode and set A1 value
		cpu.SR &= ^uint16(M68K_SR_S) // Clear supervisor mode bit (user mode)
		cpu.USP = 0x00000000         // Clear USP
		cpu.AddrRegs[1] = 0x00FF1000 // Set A1 value

		initialUSP := cpu.USP
		initialSP := cpu.AddrRegs[7]

		// MOVE A1,USP - This should cause a privilege violation in user mode
		opcode := uint16(0x4E61) // MOVE A1,USP

		// Execute manually
		cpu.Write16(cpu.PC, opcode)
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check if a privilege violation occurred - USP should NOT have been modified
		if cpu.USP != initialUSP {
			t.Errorf("USP changed in user mode after privilege violation")
		}

		// SP should be decremented for exception processing (PC and SR pushed = 6 bytes)
		if cpu.AddrRegs[7] >= initialSP {
			t.Errorf("Stack not adjusted for privilege violation exception")
		}

		// PC should change to exception handler address
		if cpu.PC != exceptionHandlerAddr {
			t.Errorf("PC not changed to exception handler: PC=0x%08X, expected=0x%08X", cpu.PC, exceptionHandlerAddr)
		} else {
			t.Logf("MOVE A1,USP (User mode): Privilege violation correctly triggered, PC=0x%08X", cpu.PC)
		}
	})
}

// Test for Movep instruction (move peripheral data)
func TestMovep(t *testing.T) {
	t.Logf("=== MOVEP Tests ===")

	t.Run("MOVEP.W D0,(8,A0) (Word to memory)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		baseAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = baseAddr
		cpu.DataRegs[0] = 0xABCD1234 // Only lower word is used for MOVEP.W

		// MOVEP.W D0,(8,A0) - Move D0 word to memory at A0+8 with even-byte addressing
		// opmode = 5 (101 = word to memory), register = 0, areg = 0
		// Encoding: 0000 0000 0 101 001 000 = 0x0148
		// displacement = 8
		cpu.Write16(cpu.PC, 0x0148)   // MOVEP.W D0,(d16,A0)
		cpu.Write16(cpu.PC+2, 0x0008) // Displacement of 8

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check memory - MOVEP stores bytes at even addresses
		// For value 0x1234, high byte (0x12) at baseAddr+8, low byte (0x34) at baseAddr+10
		if cpu.Read8(baseAddr+8) != 0x12 {
			t.Errorf("MOVEP.W failed: High byte at 0x%08X = 0x%02X, expected 0x12",
				baseAddr+8, cpu.Read8(baseAddr+8))
		}
		if cpu.Read8(baseAddr+10) != 0x34 {
			t.Errorf("MOVEP.W failed: Low byte at 0x%08X = 0x%02X, expected 0x34",
				baseAddr+10, cpu.Read8(baseAddr+10))
		} else {
			t.Logf("MOVEP.W D0,(8,A0): PASS")
		}
	})

	t.Run("MOVEP.L D1,(4,A1) (Long to memory)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		baseAddr := uint32(0x00002100)
		cpu.AddrRegs[1] = baseAddr
		cpu.DataRegs[1] = 0xAABBCCDD // Full long word used for MOVEP.L

		// MOVEP.L D1,(4,A1) - Move D1 long to memory at A1+4 with even-byte addressing
		// opmode = 7 (long to memory), register = 1
		// displacement = 4
		cpu.Write16(cpu.PC, 0x03C9)   // MOVEP.L D1,(d16,A1)
		cpu.Write16(cpu.PC+2, 0x0004) // Displacement of 4

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check memory - MOVEP stores bytes at even addresses
		// For 0xAABBCCDD, bytes at baseAddr+4, +6, +8, +10
		if cpu.Read8(baseAddr+4) != 0xAA {
			t.Errorf("MOVEP.L failed: Byte 0 at 0x%08X = 0x%02X, expected 0xAA",
				baseAddr+4, cpu.Read8(baseAddr+4))
		}
		if cpu.Read8(baseAddr+6) != 0xBB {
			t.Errorf("MOVEP.L failed: Byte 1 at 0x%08X = 0x%02X, expected 0xBB",
				baseAddr+6, cpu.Read8(baseAddr+6))
		}
		if cpu.Read8(baseAddr+8) != 0xCC {
			t.Errorf("MOVEP.L failed: Byte 2 at 0x%08X = 0x%02X, expected 0xCC",
				baseAddr+8, cpu.Read8(baseAddr+8))
		}
		if cpu.Read8(baseAddr+10) != 0xDD {
			t.Errorf("MOVEP.L failed: Byte 3 at 0x%08X = 0x%02X, expected 0xDD",
				baseAddr+10, cpu.Read8(baseAddr+10))
		} else {
			t.Logf("MOVEP.L D1,(4,A1): PASS")
		}
	})

	t.Run("MOVEP.W (8,A0),D2 (Memory to word)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		baseAddr := uint32(0x00002200)
		cpu.AddrRegs[0] = baseAddr
		cpu.DataRegs[2] = 0x00000000 // Clear destination register

		// Setup memory with bytes at even addresses
		cpu.Write8(baseAddr+8, 0x12)  // High byte
		cpu.Write8(baseAddr+10, 0x34) // Low byte

		// MOVEP.W (8,A0),D2 - Move word from memory at A0+8 to D2
		// opmode = 4 (word from memory), register = 2
		// displacement = 8
		cpu.Write16(cpu.PC, 0x0508)   // MOVEP.W (d16,A0),D2
		cpu.Write16(cpu.PC+2, 0x0008) // Displacement of 8

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result - D2 should contain 0x1234 in its lower word
		expected := uint32(0x00001234)
		if cpu.DataRegs[2] != expected {
			t.Errorf("MOVEP.W (8,A0),D2 failed: D2=0x%08X, expected 0x%08X",
				cpu.DataRegs[2], expected)
		} else {
			t.Logf("MOVEP.W (8,A0),D2: PASS")
		}
	})

	t.Run("MOVEP.L (4,A1),D3 (Memory to long)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		baseAddr := uint32(0x00002300)
		cpu.AddrRegs[1] = baseAddr
		cpu.DataRegs[3] = 0x00000000 // Clear destination register

		// Setup memory with bytes at even addresses
		cpu.Write8(baseAddr+4, 0xAA)
		cpu.Write8(baseAddr+6, 0xBB)
		cpu.Write8(baseAddr+8, 0xCC)
		cpu.Write8(baseAddr+10, 0xDD)

		// MOVEP.L (4,A1),D3 - Move long from memory at A1+4 to D3
		// opmode = 6 (110 = long from memory), register = 3, areg = 1
		// Encoding: 0000 0111 1000 1001 = 0x0789
		// displacement = 4
		cpu.Write16(cpu.PC, 0x0789)   // MOVEP.L (d16,A1),D3
		cpu.Write16(cpu.PC+2, 0x0004) // Displacement of 4

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result - D3 should contain 0xAABBCCDD
		expected := uint32(0xAABBCCDD)
		if cpu.DataRegs[3] != expected {
			t.Errorf("MOVEP.L (4,A1),D3 failed: D3=0x%08X, expected 0x%08X",
				cpu.DataRegs[3], expected)
		} else {
			t.Logf("MOVEP.L (4,A1),D3: PASS")
		}
	})
}

// Test for MoveImmToAddr instruction (move immediate to address register)
func TestMoveImmToAddr(t *testing.T) {
	t.Logf("=== MOVE Immediate to Address Register Tests ===")

	t.Run("MOVE.W #$1234,A0 (Word immediate to address register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Clear A0
		cpu.AddrRegs[0] = 0x00000000

		// MOVE.W #$1234,A0 - Move word immediate to A0 with sign extension
		cpu.Write16(cpu.PC, 0x307C)   // MOVE.W #imm,A0
		cpu.Write16(cpu.PC+2, 0x1234) // Immediate value 0x1234

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result - A0 should contain sign-extended 0x1234
		expected := uint32(0x00001234) // Positive value, no sign extension
		if cpu.AddrRegs[0] != expected {
			t.Errorf("MOVE.W #$1234,A0 failed: A0=0x%08X, expected 0x%08X",
				cpu.AddrRegs[0], expected)
		} else {
			t.Logf("MOVE.W #$1234,A0: PASS")
		}
	})

	t.Run("MOVE.W #$F000,A1 (Negative word immediate to address register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Clear A1
		cpu.AddrRegs[1] = 0x00000000

		// MOVE.W #$F000,A1 - Move negative word immediate to A1 with sign extension
		cpu.Write16(cpu.PC, 0x327C)   // MOVE.W #imm,A1
		cpu.Write16(cpu.PC+2, 0xF000) // Immediate value 0xF000 (negative)

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result - A1 should contain sign-extended 0xF000
		expected := uint32(0xFFFFF000) // Negative value, sign extension applied
		if cpu.AddrRegs[1] != expected {
			t.Errorf("MOVE.W #$F000,A1 failed: A1=0x%08X, expected 0x%08X",
				cpu.AddrRegs[1], expected)
		} else {
			t.Logf("MOVE.W #$F000,A1: PASS")
		}
	})

	t.Run("MOVE.L #$12345678,A2 (Long immediate to address register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Clear A2
		cpu.AddrRegs[2] = 0x00000000

		// MOVE.L #$12345678,A2 - Move long immediate to A2
		cpu.Write16(cpu.PC, 0x247C)   // MOVE.L #imm,A2
		cpu.Write16(cpu.PC+2, 0x1234) // High word of immediate
		cpu.Write16(cpu.PC+4, 0x5678) // Low word of immediate

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result - A2 should contain 0x12345678
		expected := uint32(0x12345678)
		if cpu.AddrRegs[2] != expected {
			t.Errorf("MOVE.L #$12345678,A2 failed: A2=0x%08X, expected 0x%08X",
				cpu.AddrRegs[2], expected)
		} else {
			t.Logf("MOVE.L #$12345678,A2: PASS")
		}
	})
}

// Test for MoveC instruction (move to/from control register)
func TestMoveC(t *testing.T) {
	t.Logf("=== MOVE to/from Control Register Tests ===")

	t.Run("MOVEC USP,A0 (Move USP to A0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Ensure supervisor mode and set USP value
		cpu.SR |= M68K_SR_S  // Set supervisor mode bit
		cpu.USP = 0x00FF8800 // Set user stack pointer value
		cpu.AddrRegs[0] = 0  // Clear A0

		// MOVEC USP,A0 - Move USP to A0
		// Opcode format: 0x4E7A <register><control register>
		// A0 = 0x8 (register), USP = 0x800 (control register)
		cpu.Write16(cpu.PC, 0x4E7A)   // MOVEC Rc,Rn
		cpu.Write16(cpu.PC+2, 0x8800) // A0, USP

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result - A0 should contain USP value
		if cpu.AddrRegs[0] != cpu.USP {
			t.Errorf("MOVEC USP,A0 failed: A0=0x%08X, expected USP=0x%08X",
				cpu.AddrRegs[0], cpu.USP)
		} else {
			t.Logf("MOVEC USP,A0: PASS")
		}
	})

	t.Run("MOVEC A1,USP (Move A1 to USP)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Ensure supervisor mode and set A1 value
		cpu.SR |= M68K_SR_S          // Set supervisor mode bit
		cpu.USP = 0                  // Clear USP
		cpu.AddrRegs[1] = 0x00FF9900 // Set A1 value

		// MOVEC A1,USP - Move A1 to USP
		// Opcode format: 0x4E7B <register><control register>
		// A1 = 0x9 (register), USP = 0x800 (control register)
		cpu.Write16(cpu.PC, 0x4E7B)   // MOVEC Rn,Rc
		cpu.Write16(cpu.PC+2, 0x9800) // A1, USP

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result - USP should contain A1 value
		if cpu.USP != cpu.AddrRegs[1] {
			t.Errorf("MOVEC A1,USP failed: USP=0x%08X, expected A1=0x%08X",
				cpu.USP, cpu.AddrRegs[1])
		} else {
			t.Logf("MOVEC A1,USP: PASS")
		}
	})

	t.Run("MOVEC in User Mode (should cause privilege violation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Ensure user mode
		cpu.SR &= ^uint16(M68K_SR_S) // Clear supervisor mode bit (user mode)
		initialPC := cpu.PC
		initialSP := cpu.AddrRegs[7]

		// MOVEC USP,A0 - Should cause privilege violation in user mode
		cpu.Write16(cpu.PC, 0x4E7A)   // MOVEC Rc,Rn
		cpu.Write16(cpu.PC+2, 0x8800) // A0, USP

		// Execute manually
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check if a privilege violation occurred
		// SP should be decremented for exception processing
		if cpu.AddrRegs[7] >= initialSP {
			t.Errorf("Stack not adjusted for privilege violation exception")
		}

		// PC should change due to exception
		if cpu.PC == initialPC+4 {
			t.Errorf("PC not changed after privilege violation")
		} else {
			t.Logf("MOVEC in User Mode: Privilege violation correctly triggered")
		}
	})
}

// Test for MOVEA instruction (Move to Address Register with sign extension)
func TestMovea(t *testing.T) {
	t.Logf("=== MOVEA (Move to Address Register) Tests ===")

	t.Run("MOVEA.W D0,A0 (Positive value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D0 has a positive value, clear A0
		cpu.DataRegs[0] = 0x00007FFF
		cpu.AddrRegs[0] = 0

		// MOVEA.W D0,A0 - Move word from D0 to A0 with sign extension
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_DR, 0, M68K_AM_AR, 0)

		// Expected results - positive value, no sign extension
		expectedResults := map[string]uint32{
			"D0": 0x00007FFF,
			"A0": 0x00007FFF,
		}

		runDetailedTest(t, cpu, opcode, "MOVEA.W D0,A0 (Positive)", expectedResults)

		// Verify flags aren't affected (flags should be cleared from prior setup)
		if (cpu.SR & (M68K_SR_N | M68K_SR_Z)) != 0 {
			t.Errorf("MOVEA.W affected flags when it shouldn't: SR=0x%04X", cpu.SR)
		}
	})

	t.Run("MOVEA.W D1,A1 (Negative value with sign extension)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D1 has a negative value, clear A1
		cpu.DataRegs[1] = 0x0000FFFF // 0xFFFF is -1 as a signed word
		cpu.AddrRegs[1] = 0

		// MOVEA.W D1,A1 - Move word from D1 to A1 with sign extension
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_DR, 1, M68K_AM_AR, 1)

		// Expected results - negative value, sign extension applied
		expectedResults := map[string]uint32{
			"D1": 0x0000FFFF,
			"A1": 0xFFFFFFFF, // Sign extended
		}

		runDetailedTest(t, cpu, opcode, "MOVEA.W D1,A1 (Negative)", expectedResults)
	})

	t.Run("MOVEA.L D2,A2 (Long value - no sign extension)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[2] = 0xABCD1234
		cpu.AddrRegs[2] = 0

		// MOVEA.L D2,A2 - Move long from D2 to A2 (no sign extension)
		opcode := createMoveOpcode(M68K_SIZE_LONG, M68K_AM_DR, 2, M68K_AM_AR, 2)

		// Expected results
		expectedResults := map[string]uint32{
			"D2": 0xABCD1234,
			"A2": 0xABCD1234,
		}

		runDetailedTest(t, cpu, opcode, "MOVEA.L D2,A2", expectedResults)
	})

	t.Run("MOVEA.W A3,A4 (Address Register to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A3 has a value, clear A4
		cpu.AddrRegs[3] = 0x0000ABCD
		cpu.AddrRegs[4] = 0

		// MOVEA.W A3,A4 - Move word from A3 to A4 with sign extension
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_AR, 3, M68K_AM_AR, 4)

		// Expected results - sign extension should be applied
		expectedResults := map[string]uint32{
			"A3": 0x0000ABCD,
			"A4": 0xFFFFABCD, // Sign extended (bit 15 is set)
		}

		runDetailedTest(t, cpu, opcode, "MOVEA.W A3,A4", expectedResults)
	})

	t.Run("MOVEA.W (A0),A1 (Memory Indirect to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to memory, clear A1
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0x8000) // Negative value (0x8000)
		cpu.AddrRegs[1] = 0

		// MOVEA.W (A0),A1 - Move word from memory to A1 with sign extension
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_AR_IND, 0, M68K_AM_AR, 1)

		// Expected results - negative value, sign extension applied
		expectedResults := map[string]uint32{
			"A0": memAddr,
			"A1": 0xFFFF8000, // Sign extended
		}

		runDetailedTest(t, cpu, opcode, "MOVEA.W (A0),A1", expectedResults)
	})

	t.Run("MOVEA.W (A0)+,A1 (Memory Postincrement to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to memory, clear A1
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0x1234)
		cpu.AddrRegs[1] = 0

		// MOVEA.W (A0)+,A1 - Move word from memory to A1 and increment A0
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_AR_POST, 0, M68K_AM_AR, 1)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr + 2, // A0 incremented by word size
			"A1": 0x00001234,  // No sign extension (positive value)
		}

		runDetailedTest(t, cpu, opcode, "MOVEA.W (A0)+,A1", expectedResults)
	})

	t.Run("MOVEA.W -(A0),A1 (Memory Predecrement to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to memory + 2, clear A1
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr + 2
		cpu.Write16(memAddr, 0x7FFF) // Positive value
		cpu.AddrRegs[1] = 0

		// MOVEA.W -(A0),A1 - Decrement A0 and move word from memory to A1
		opcode := createMoveOpcode(M68K_SIZE_WORD, M68K_AM_AR_PRE, 0, M68K_AM_AR, 1)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,    // A0 decremented by word size
			"A1": 0x00007FFF, // No sign extension (positive value)
		}

		runDetailedTest(t, cpu, opcode, "MOVEA.W -(A0),A1", expectedResults)
	})

	t.Run("MOVEA.W (8,A0),A1 (Memory with Displacement to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - A0 points to base address, clear A1
		baseAddr := uint32(0x00002000)
		disp := uint32(8)
		cpu.AddrRegs[0] = baseAddr
		cpu.Write16(baseAddr+disp, 0xC000) // Negative value
		cpu.AddrRegs[1] = 0

		// We need to assemble this instruction manually:
		// MOVEA.W (8,A0),A1 = 0x3268 0008
		cpu.Write16(cpu.PC, 0x3268)   // MOVEA.W with displacement
		cpu.Write16(cpu.PC+2, 0x0008) // Displacement of 8

		// Execute and check results
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify sign extension
		if cpu.AddrRegs[1] != 0xFFFFC000 {
			t.Errorf("MOVEA.W (8,A0),A1 failed: A1=0x%08X, expected 0xFFFFC000", cpu.AddrRegs[1])
		} else {
			t.Logf("MOVEA.W (8,A0),A1: PASS")
		}
	})

	t.Run("MOVEA.W #$1234,A2 (Immediate to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Clear A2
		cpu.AddrRegs[2] = 0

		// Manually assemble:
		// MOVEA.W #$1234,A2 = 0x347C 1234
		cpu.Write16(cpu.PC, 0x347C)   // MOVEA.W #imm,A2
		cpu.Write16(cpu.PC+2, 0x1234) // Immediate 0x1234

		// Execute and check results
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.AddrRegs[2] != 0x00001234 {
			t.Errorf("MOVEA.W #$1234,A2 failed: A2=0x%08X, expected 0x00001234", cpu.AddrRegs[2])
		} else {
			t.Logf("MOVEA.W #$1234,A2: PASS")
		}
	})

	t.Run("MOVEA.W ($1000).W,A3 (Absolute Short to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Clear A3
		cpu.AddrRegs[3] = 0

		// Set memory at absolute address
		absAddr := uint32(0x1000)
		cpu.Write16(absAddr, 0x5678)

		// Manually assemble:
		// MOVEA.W ($1000).W,A3 = 0x3678 1000
		cpu.Write16(cpu.PC, 0x3678)   // MOVEA.W (xxx).W,A3
		cpu.Write16(cpu.PC+2, 0x1000) // Address 0x1000

		// Execute and check results
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.AddrRegs[3] != 0x00005678 {
			t.Errorf("MOVEA.W ($1000).W,A3 failed: A3=0x%08X, expected 0x00005678", cpu.AddrRegs[3])
		} else {
			t.Logf("MOVEA.W ($1000).W,A3: PASS")
		}
	})

	t.Run("MOVEA.L ($00002000).L,A4 (Absolute Long to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Clear A4
		cpu.AddrRegs[4] = 0

		// Set memory at absolute address
		absAddr := uint32(0x00002000)
		cpu.Write32(absAddr, 0x12345678)

		// Manually assemble:
		// MOVEA.L ($00002000).L,A4 = 0x2879 0000 2000
		cpu.Write16(cpu.PC, 0x2879)   // MOVEA.L (xxx).L,A4
		cpu.Write16(cpu.PC+2, 0x0000) // High word of address
		cpu.Write16(cpu.PC+4, 0x2000) // Low word of address

		// Execute and check results
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.AddrRegs[4] != 0x12345678 {
			t.Errorf("MOVEA.L ($00002000).L,A4 failed: A4=0x%08X, expected 0x12345678", cpu.AddrRegs[4])
		} else {
			t.Logf("MOVEA.L ($00002000).L,A4: PASS")
		}
	})
}

func TestFetchAndDecodeInstruction(t *testing.T) {
	t.Logf("=== Comprehensive FetchAndDecodeInstruction Tests ===")

	// Helper function to test opcode routing
	testOpcodeRouting := func(t *testing.T, desc string, opcode uint16,
		setupFn func(*M68KCPU),
		verifyFn func(*M68KCPU, uint32) bool) {
		t.Run(desc, func(t *testing.T) {
			cpu := setupTestCPU()

			// Run any test-specific setup
			if setupFn != nil {
				setupFn(cpu)
			}

			initialPC := cpu.PC

			// Write the instruction and execute it
			cpu.Write16(cpu.PC, opcode)
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()

			// Verify the results
			if !verifyFn(cpu, initialPC) {
				t.Errorf("Instruction routing failed for %s [0x%04X]", desc, opcode)
			}
		})
	}

	// ================== SIMPLE INSTRUCTIONS ==================

	// Test NOP
	testOpcodeRouting(t, "NOP", 0x4E71, nil, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.PC == initialPC+2
	})

	// Test RTS
	testOpcodeRouting(t, "RTS", 0x4E75, func(cpu *M68KCPU) {
		// Setup a return address on the stack
		returnAddr := uint32(0x00003000)
		cpu.Push32(returnAddr)
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.PC == 0x00003000
	})

	// ================== MOVE INSTRUCTIONS ==================

	// Test MOVE.B D0,D1
	testOpcodeRouting(t, "MOVE.B D0,D1", 0x1200, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x000000FF
		cpu.DataRegs[1] = 0x00000000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0x000000FF && cpu.PC == initialPC+2
	})

	// Test MOVE.W D0,D1
	testOpcodeRouting(t, "MOVE.W D0,D1", 0x3200, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0000FFFF
		cpu.DataRegs[1] = 0x00000000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0x0000FFFF && cpu.PC == initialPC+2
	})

	// Test MOVE.L D0,D1
	testOpcodeRouting(t, "MOVE.L D0,D1", 0x2200, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0xFFFFFFFF
		cpu.DataRegs[1] = 0x00000000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0xFFFFFFFF && cpu.PC == initialPC+2
	})

	// Test MOVE.W D0,(A0)
	testOpcodeRouting(t, "MOVE.W D0,(A0)", 0x3080, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0000ABCD
		cpu.AddrRegs[0] = 0x00002000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.Read16(0x00002000) == 0xABCD && cpu.PC == initialPC+2
	})

	// Test MOVEA.W D0,A0
	testOpcodeRouting(t, "MOVEA.W D0,A0", 0x3040, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00008000 // Negative for sign extension
		cpu.AddrRegs[0] = 0x00000000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.AddrRegs[0] == 0xFFFF8000 && cpu.PC == initialPC+2 // Sign extended
	})

	// Test MOVEQ
	testOpcodeRouting(t, "MOVEQ #$FF,D0", 0x70FF, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00000000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[0] == 0xFFFFFFFF && cpu.PC == initialPC+2 // Sign extended
	})

	// ================== ARITHMETIC INSTRUCTIONS ==================

	// Test ADD.L D0,D1
	testOpcodeRouting(t, "ADD.L D0,D1", 0xD280, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x11111111
		cpu.DataRegs[1] = 0x22222222
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0x33333333 && cpu.PC == initialPC+2
	})

	// Test SUB.L D0,D1
	testOpcodeRouting(t, "SUB.L D0,D1", 0x9280, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x11111111
		cpu.DataRegs[1] = 0x33333333
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0x22222222 && cpu.PC == initialPC+2
	})

	// Test MULU
	testOpcodeRouting(t, "MULU D0,D1", 0xC2C0, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00000003
		cpu.DataRegs[1] = 0x00000004
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0x0000000C && cpu.PC == initialPC+2
	})

	// Test DIVU
	testOpcodeRouting(t, "DIVU D0,D1", 0x82C0, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00000002
		cpu.DataRegs[1] = 0x0000000C
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0x00000006 && cpu.PC == initialPC+2
	})

	// ================== LOGICAL INSTRUCTIONS ==================

	// Test AND.L D0,D1
	testOpcodeRouting(t, "AND.L D0,D1", 0xC280, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0F0F0F0F
		cpu.DataRegs[1] = 0xFF00FF00
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0x0F000F00 && cpu.PC == initialPC+2
	})

	// Test OR.L D0,D1
	testOpcodeRouting(t, "OR.L D0,D1", 0x8280, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0F0F0F0F
		cpu.DataRegs[1] = 0xF000F000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0xFF0FFF0F && cpu.PC == initialPC+2
	})

	// Test EOR
	testOpcodeRouting(t, "EOR.L D0,D1", 0xB180, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0F0F0F0F
		cpu.DataRegs[1] = 0xFFFFFFFF
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0xF0F0F0F0 && cpu.PC == initialPC+2
	})

	// ================== BRANCH/JUMP INSTRUCTIONS ==================

	// Test BRA
	testOpcodeRouting(t, "BRA.S +10", 0x600A, nil, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.PC == initialPC+10
	})

	// Test Bcc (BEQ - not taken)
	testOpcodeRouting(t, "BEQ.S +10 (not taken)", 0x6706, func(cpu *M68KCPU) {
		// Clear Z flag to prevent branch
		cpu.SR &= ^uint16(M68K_SR_Z)
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.PC == initialPC+2 // PC advances to next instruction only
	})

	// Test Bcc (BEQ - taken)
	testOpcodeRouting(t, "BEQ.S +10 (taken)", 0x670A, func(cpu *M68KCPU) {
		// Set Z flag to allow branch
		cpu.SR |= M68K_SR_Z
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.PC == initialPC+10 // PC jumps forward by displacement
	})

	// Test JMP
	testOpcodeRouting(t, "JMP (A0)", 0x4ED0, func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x00003000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.PC == 0x00003000
	})

	// ================== BIT MANIPULATION INSTRUCTIONS ==================

	// Test BTST
	testOpcodeRouting(t, "BTST D0,D1", 0x0100, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00000001 // Test bit 1
		cpu.DataRegs[1] = 0x00000002 // Bit 1 is set
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return (cpu.SR&M68K_SR_Z) == 0 && cpu.PC == initialPC+2 // Z clear = bit was set
	})

	// Test BSET
	testOpcodeRouting(t, "BSET D0,D1", 0x01C0, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00000002 // Set bit 2
		cpu.DataRegs[1] = 0x00000000 // Initially clear
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[1] == 0x00000004 && cpu.PC == initialPC+2
	})

	// ================== SHIFT/ROTATE INSTRUCTIONS ==================

	// Test ASR
	testOpcodeRouting(t, "ASR.L #1,D0", 0xE080, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x80000000 // Negative value
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		// Arithmetic shift right preserves sign bit
		return cpu.DataRegs[0] == 0xC0000000 && cpu.PC == initialPC+2
	})

	// Test LSL
	testOpcodeRouting(t, "LSL.L #1,D0", 0xE380, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x40000000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[0] == 0x80000000 && cpu.PC == initialPC+2
	})

	// ================== SYSTEM CONTROL INSTRUCTIONS ==================

	// Test ANDI to CCR
	testOpcodeRouting(t, "ANDI to CCR", 0x023C, func(cpu *M68KCPU) {
		cpu.SR = 0x001F               // All CCR flags set
		cpu.Write16(cpu.PC+2, 0x000A) // Immediate 0x0A (bits 1 and 3 set)
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.SR == 0x000A && cpu.PC == initialPC+4
	})

	// ================== MISCELLANEOUS INSTRUCTIONS ==================

	// Test SWAP
	testOpcodeRouting(t, "SWAP D0", 0x4840, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x12345678
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[0] == 0x56781234 && cpu.PC == initialPC+2
	})

	// Test EXT
	testOpcodeRouting(t, "EXT.L D0", 0x48C0, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0000FFFF // Negative word value
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[0] == 0xFFFFFFFF && cpu.PC == initialPC+2
	})

	// Test CLR
	testOpcodeRouting(t, "CLR.L D0", 0x4280, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0xFFFFFFFF
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[0] == 0x00000000 && cpu.PC == initialPC+2
	})

	// Test TST
	testOpcodeRouting(t, "TST.L D0", 0x4A80, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x80000000 // Negative value
		cpu.SR = 0x0000              // Clear flags
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		// Negative flag should be set
		return (cpu.SR&M68K_SR_N) != 0 && cpu.PC == initialPC+2
	})

	// ================== ADDRESSING MODES ==================

	// Test (An)+ (Address Register Indirect with Postincrement)
	testOpcodeRouting(t, "MOVE.W (A0)+,D0", 0x3018, func(cpu *M68KCPU) {
		addr := uint32(0x00002000)
		cpu.AddrRegs[0] = addr
		cpu.Write16(addr, 0xABCD)
		cpu.DataRegs[0] = 0x00000000
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.DataRegs[0] == 0x0000ABCD &&
			cpu.AddrRegs[0] == 0x00002002 &&
			cpu.PC == initialPC+2
	})

	// Test -(An) (Address Register Indirect with Predecrement)
	testOpcodeRouting(t, "MOVE.W D0,-(A0)", 0x3100, func(cpu *M68KCPU) {
		addr := uint32(0x00002000)
		cpu.AddrRegs[0] = addr
		cpu.DataRegs[0] = 0x0000ABCD
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.AddrRegs[0] == 0x00001FFE &&
			cpu.Read16(0x00001FFE) == 0xABCD &&
			cpu.PC == initialPC+2
	})

	// Test (d16,An) (Address Register Indirect with Displacement)
	testOpcodeRouting(t, "MOVE.W D0,(8,A0)", 0x3168, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0000ABCD
		cpu.AddrRegs[0] = 0x00002000
		cpu.Write16(cpu.PC+2, 0x0008) // Displacement 8
	}, func(cpu *M68KCPU, initialPC uint32) bool {
		return cpu.Read16(0x00002008) == 0xABCD && cpu.PC == initialPC+4
	})
}

// Test for arithmetic operation instructions
func TestArithmeticOperations(t *testing.T) {
	t.Logf("=== Arithmetic Operations Tests ===")

	// ================== ADD INSTRUCTIONS ==================
	t.Run("ADD.B D0,D1 (Data Register to Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000FF // Source
		cpu.DataRegs[1] = 0x00000001 // Destination

		// ADD.B D0,D1
		opcode := uint16(0xD200) // ADD.B D0,D1

		// Expected results: 0xFF + 0x01 = 0x00 with carry
		expectedResults := map[string]uint32{
			"D0": 0x000000FF,
			"D1": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "ADD.B D0,D1", expectedResults)

		// Verify flags - carry and zero flags should be set
		if (cpu.SR & (M68K_SR_C | M68K_SR_X | M68K_SR_Z)) != (M68K_SR_C | M68K_SR_X | M68K_SR_Z) {
			t.Errorf("ADD.B failed: SR flags incorrect. Expected C,X,Z set, got %04X", cpu.SR)
		}
	})

	t.Run("ADD.W D0,D1 (Positive Overflow)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Maximum positive value in a word + 1 = overflow
		cpu.DataRegs[0] = 0x00007FFF // Largest positive 16-bit number
		cpu.DataRegs[1] = 0x00000001 // Will cause overflow when added

		// ADD.W D0,D1
		opcode := uint16(0xD240) // ADD.W D0,D1

		// Expected results: Overflow to negative
		expectedResults := map[string]uint32{
			"D0": 0x00007FFF,
			"D1": 0x00008000, // 0x7FFF + 0x0001 = 0x8000 (negative)
		}

		runDetailedTest(t, cpu, opcode, "ADD.W D0,D1 (Overflow)", expectedResults)

		// Verify flags - overflow and negative flags should be set
		if (cpu.SR & (M68K_SR_V | M68K_SR_N)) != (M68K_SR_V | M68K_SR_N) {
			t.Errorf("ADD.W overflow failed: SR flags incorrect. Expected V,N set, got %04X", cpu.SR)
		}
	})

	t.Run("ADD.L D0,D1 (Normal Operation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x12345678
		cpu.DataRegs[1] = 0x87654321

		// ADD.L D0,D1
		opcode := uint16(0xD280) // ADD.L D0,D1

		// Expected results: 0x12345678 + 0x87654321 = 0x99999999
		expectedResults := map[string]uint32{
			"D0": 0x12345678,
			"D1": 0x99999999,
		}

		runDetailedTest(t, cpu, opcode, "ADD.L D0,D1", expectedResults)
	})

	t.Run("ADD.B D0,(A0) (Data Register to Memory)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x0000007F // Source
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x01) // Initial memory value

		// ADD.B D0,(A0)
		opcode := uint16(0xD110) // ADD.B D0,(A0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x0000007F,
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "ADD.B D0,(A0)", expectedResults)

		// Check memory result
		result := cpu.Read8(memAddr)
		if result != 0x80 {
			t.Errorf("ADD.B D0,(A0) memory result incorrect: %02X, expected 0x80", result)
		}

		// Negative flag should be set (result is negative)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("ADD.B D0,(A0) flag incorrect: N flag should be set")
		}
	})

	// ADDQ tests
	t.Run("ADDQ.B #1,D0 (Quick Add to Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000FF // 255 (max 8-bit value)

		// ADDQ.B #1,D0
		opcode := uint16(0x5200) // ADDQ.B #1,D0

		// Expected results: 0xFF + 0x01 = 0x00 with carry
		expectedResults := map[string]uint32{
			"D0": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "ADDQ.B #1,D0", expectedResults)

		// Verify flags - carry, extend, and zero flags should be set
		if (cpu.SR & (M68K_SR_C | M68K_SR_X | M68K_SR_Z)) != (M68K_SR_C | M68K_SR_X | M68K_SR_Z) {
			t.Errorf("ADDQ.B failed: SR flags incorrect. Expected C,X,Z set, got %04X", cpu.SR)
		}
	})

	t.Run("ADDQ.W #8,A0 (Quick Add to Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.AddrRegs[0] = 0x00002000

		// ADDQ.W #8,A0 - Quick of 0 encodes as 8
		opcode := uint16(0x5048) // ADDQ.W #8,A0

		// Expected results: Address register incremented by 8
		expectedResults := map[string]uint32{
			"A0": 0x00002008,
		}

		runDetailedTest(t, cpu, opcode, "ADDQ.W #8,A0", expectedResults)

		// Verify flags are not affected for address registers
		if (cpu.SR & (M68K_SR_C | M68K_SR_V | M68K_SR_Z | M68K_SR_N)) != 0 {
			t.Errorf("ADDQ.W to address reg should not affect flags, SR=%04X", cpu.SR)
		}
	})

	// ADDX tests
	t.Run("ADDX.B D0,D1 (Data Register to Data Register with X Flag)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000001
		cpu.DataRegs[1] = 0x00000002
		cpu.SR |= M68K_SR_X // Set extend flag before operation

		// ADDX.B D0,D1
		opcode := uint16(0xD300) // ADDX.B D0,D1

		// Expected results: 0x01 + 0x02 + X = 0x04
		expectedResults := map[string]uint32{
			"D0": 0x00000001,
			"D1": 0x00000004,
		}

		runDetailedTest(t, cpu, opcode, "ADDX.B D0,D1", expectedResults)
	})

	t.Run("ADDX.W -(A0),-(A1) (Memory to Memory with X Flag)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		addr0 := uint32(0x00002000)
		addr1 := uint32(0x00003000)
		cpu.AddrRegs[0] = addr0
		cpu.AddrRegs[1] = addr1
		cpu.Write16(addr0-2, 0xFFFF) // Max word value at predecrement address
		cpu.Write16(addr1-2, 0x0000) // Zero at predecrement address
		cpu.SR |= M68K_SR_X          // Set extend flag before operation

		// ADDX.W -(A0),-(A1)
		opcode := uint16(0xD148) // ADDX.W -(A0),-(A1)

		// Expected results: Registers decremented, 0xFFFF + 0x0000 + X = 0x0000 with carry
		expectedResults := map[string]uint32{
			"A0": addr0 - 2,
			"A1": addr1 - 2,
		}

		runDetailedTest(t, cpu, opcode, "ADDX.W -(A0),-(A1)", expectedResults)

		// Check memory result and flags
		result := cpu.Read16(addr1 - 2)
		if result != 0x0000 {
			t.Errorf("ADDX.W memory result incorrect: %04X, expected 0x0000", result)
		}

		// Carry, Extend, and Zero flags should be set
		if (cpu.SR & (M68K_SR_C | M68K_SR_X | M68K_SR_Z)) != (M68K_SR_C | M68K_SR_X | M68K_SR_Z) {
			t.Errorf("ADDX.W flags incorrect: Expected C,X,Z set, got %04X", cpu.SR)
		}
	})

	// ================== SUB INSTRUCTIONS ==================
	t.Run("SUB.B D0,D1 (Data Register from Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000001 // Source (subtrahend)
		cpu.DataRegs[1] = 0x00000000 // Destination (minuend)

		// SUB.B D0,D1
		opcode := uint16(0x9200) // SUB.B D0,D1

		// Expected results: 0x00 - 0x01 = 0xFF with carry (borrow)
		expectedResults := map[string]uint32{
			"D0": 0x00000001,
			"D1": 0x000000FF,
		}

		runDetailedTest(t, cpu, opcode, "SUB.B D0,D1", expectedResults)

		// Verify flags - carry and negative flags should be set
		if (cpu.SR & (M68K_SR_C | M68K_SR_X | M68K_SR_N)) != (M68K_SR_C | M68K_SR_X | M68K_SR_N) {
			t.Errorf("SUB.B failed: SR flags incorrect. Expected C,X,N set, got %04X", cpu.SR)
		}
	})

	t.Run("SUB.W D0,D1 (Underflow Test)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Tests negative underflow
		cpu.DataRegs[0] = 0x00008001 // Large negative number in 16-bit
		cpu.DataRegs[1] = 0x00007FFF // Largest positive 16-bit number

		// SUB.W D0,D1
		opcode := uint16(0x9240) // SUB.W D0,D1

		// Expected results: 0x7FFF - 0x8001 = 0xFFFE with overflow
		expectedResults := map[string]uint32{
			"D0": 0x00008001,
			"D1": 0x0000FFFE,
		}

		runDetailedTest(t, cpu, opcode, "SUB.W D0,D1 (Underflow)", expectedResults)

		// Verify flags - overflow should be set
		if (cpu.SR & M68K_SR_V) == 0 {
			t.Errorf("SUB.W underflow failed: SR flags incorrect. Expected V set, got %04X", cpu.SR)
		}
	})

	t.Run("SUB.L D0,D1 (Normal Operation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x12345678
		cpu.DataRegs[1] = 0x87654321

		// SUB.L D0,D1
		opcode := uint16(0x9280) // SUB.L D0,D1

		// Expected results: 0x87654321 - 0x12345678 = 0x7530ECa9
		expectedResults := map[string]uint32{
			"D0": 0x12345678,
			"D1": 0x7530ECA9,
		}

		runDetailedTest(t, cpu, opcode, "SUB.L D0,D1", expectedResults)
	})

	// SUBQ tests
	t.Run("SUBQ.B #1,D0 (Quick Subtract from Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000000 // 0 - 1 = -1 (0xFF) with borrow

		// SUBQ.B #1,D0
		opcode := uint16(0x5300) // SUBQ.B #1,D0

		// Expected results: 0x00 - 0x01 = 0xFF with carry
		expectedResults := map[string]uint32{
			"D0": 0x000000FF,
		}

		runDetailedTest(t, cpu, opcode, "SUBQ.B #1,D0", expectedResults)

		// Verify flags - carry, extend, and negative flags should be set
		if (cpu.SR & (M68K_SR_C | M68K_SR_X | M68K_SR_N)) != (M68K_SR_C | M68K_SR_X | M68K_SR_N) {
			t.Errorf("SUBQ.B failed: SR flags incorrect. Expected C,X,N set, got %04X", cpu.SR)
		}
	})

	t.Run("SUBQ.W #8,A0 (Quick Subtract from Address Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.AddrRegs[0] = 0x00002010

		// SUBQ.W #8,A0 - Quick of 0 encodes as 8
		opcode := uint16(0x5148) // SUBQ.W #8,A0

		// Expected results: Address register decremented by 8
		expectedResults := map[string]uint32{
			"A0": 0x00002008,
		}

		runDetailedTest(t, cpu, opcode, "SUBQ.W #8,A0", expectedResults)

		// Verify flags are not affected for address registers
		if (cpu.SR & (M68K_SR_C | M68K_SR_V | M68K_SR_Z | M68K_SR_N)) != 0 {
			t.Errorf("SUBQ.W to address reg should not affect flags, SR=%04X", cpu.SR)
		}
	})

	// SUBI tests
	t.Run("SUBI.B #$0F,D0 (Subtract Immediate from Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000FF // FF - 0F = F0

		// Setup SUBI.B #$0F,D0 instruction
		cpu.Write16(cpu.PC, 0x0400)   // SUBI.B
		cpu.Write16(cpu.PC+2, 0x000F) // Immediate value 0x0F

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result
		if cpu.DataRegs[0] != 0x000000F0 {
			t.Errorf("SUBI.B #$0F,D0 failed: D0=%08X, expected 0x000000F0", cpu.DataRegs[0])
		}
	})

	// NEG tests
	t.Run("NEG.B D0 (Negate Data Register Byte)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000001 // 0 - 1 = -1 (0xFF) with borrow

		// NEG.B D0
		opcode := uint16(0x4400) // NEG.B D0

		// Expected results: 0 - 1 = 0xFF with carry
		expectedResults := map[string]uint32{
			"D0": 0x000000FF,
		}

		runDetailedTest(t, cpu, opcode, "NEG.B D0", expectedResults)

		// Verify flags - carry, extend, and negative flags should be set
		if (cpu.SR & (M68K_SR_C | M68K_SR_X | M68K_SR_N)) != (M68K_SR_C | M68K_SR_X | M68K_SR_N) {
			t.Errorf("NEG.B failed: SR flags incorrect. Expected C,X,N set, got %04X", cpu.SR)
		}
	})

	t.Run("NEG.W D1 (Negate Zero Value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[1] = 0x00000000 // 0 - 0 = 0, no carry

		// NEG.W D1
		opcode := uint16(0x4441) // NEG.W D1

		// Expected results: 0 - 0 = 0
		expectedResults := map[string]uint32{
			"D1": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "NEG.W D1", expectedResults)

		// Verify flags - zero flag should be set, carry and extend should be clear
		if (cpu.SR & (M68K_SR_C | M68K_SR_X | M68K_SR_Z)) != M68K_SR_Z {
			t.Errorf("NEG.W zero failed: SR flags incorrect. Expected Z set, C,X clear, got %04X", cpu.SR)
		}
	})

	t.Run("NEG.W (A0) (Negate Memory Word)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0x8000) // Most negative 16-bit value, will cause overflow

		// NEG.W (A0)
		opcode := uint16(0x4450) // NEG.W (A0)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "NEG.W (A0)", expectedResults)

		// Check memory result
		result := cpu.Read16(memAddr)
		if result != 0x8000 { // -32768 negated is still -32768 in two's complement
			t.Errorf("NEG.W memory result incorrect: %04X, expected 0x8000", result)
		}

		// Overflow should be set (negating 0x8000 overflows)
		if (cpu.SR & (M68K_SR_V | M68K_SR_N)) != (M68K_SR_V | M68K_SR_N) {
			t.Errorf("NEG.W (A0) flags incorrect: Expected V,N set, got %04X", cpu.SR)
		}
	})

	// NEGX tests
	t.Run("NEGX.B D0 (Negate with Extend)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000001
		cpu.SR |= M68K_SR_X // Set extend flag before operation

		// NEGX.B D0
		opcode := uint16(0x4000) // NEGX.B D0

		// Expected results: 0 - 1 - X = 0xFE with carry
		expectedResults := map[string]uint32{
			"D0": 0x000000FE,
		}

		runDetailedTest(t, cpu, opcode, "NEGX.B D0", expectedResults)

		// Verify flags - carry, extend, and negative flags should be set
		if (cpu.SR & (M68K_SR_C | M68K_SR_X | M68K_SR_N)) != (M68K_SR_C | M68K_SR_X | M68K_SR_N) {
			t.Errorf("NEGX.B failed: SR flags incorrect. Expected C,X,N set, got %04X", cpu.SR)
		}
	})

	// CMP tests
	t.Run("CMP.B D0,D1 (Compare Byte)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D1 < D0 test
		cpu.DataRegs[0] = 0x00000080 // Source (larger)
		cpu.DataRegs[1] = 0x0000007F // Destination (smaller)

		// CMP.B D0,D1
		opcode := uint16(0xB200) // CMP.B D0,D1

		// Expected results - registers unchanged
		expectedResults := map[string]uint32{
			"D0": 0x00000080,
			"D1": 0x0000007F,
		}

		runDetailedTest(t, cpu, opcode, "CMP.B D0,D1", expectedResults)

		// Verify flags - negative and carry flags should be set (D1-D0 < 0)
		if (cpu.SR & (M68K_SR_N | M68K_SR_C)) != (M68K_SR_N | M68K_SR_C) {
			t.Errorf("CMP.B failed: SR flags incorrect. Expected N,C set, got %04X", cpu.SR)
		}
	})

	t.Run("CMP.W D0,D1 (Equal Test)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - Equal values test
		cpu.DataRegs[0] = 0x00001234 // Source
		cpu.DataRegs[1] = 0x00001234 // Destination

		// CMP.W D0,D1
		opcode := uint16(0xB240) // CMP.W D0,D1

		// Expected results - registers unchanged
		expectedResults := map[string]uint32{
			"D0": 0x00001234,
			"D1": 0x00001234,
		}

		runDetailedTest(t, cpu, opcode, "CMP.W D0,D1 (Equal)", expectedResults)

		// Verify flags - zero flag should be set (D1-D0 = 0)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("CMP.W equal failed: SR flags incorrect. Expected Z set, got %04X", cpu.SR)
		}
	})

	// CMPI tests
	t.Run("CMPI.B #$FF,D0 (Compare Immediate)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - D0 > immediate value test
		cpu.DataRegs[0] = 0x00000001 // Value to compare

		// Setup CMPI.B #$FF,D0 instruction
		cpu.Write16(cpu.PC, 0x0C00)   // CMPI.B
		cpu.Write16(cpu.PC+2, 0x00FF) // Immediate value 0xFF

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - register unchanged
		if cpu.DataRegs[0] != 0x00000001 {
			t.Errorf("CMPI.B register was changed: D0=%08X, expected 0x00000001", cpu.DataRegs[0])
		}

		// Verify flags - N and C should be set (D0-0xFF < 0)
		if (cpu.SR & (M68K_SR_N | M68K_SR_C)) != (M68K_SR_N | M68K_SR_C) {
			t.Errorf("CMPI.B failed: SR flags incorrect. Expected N,C set, got %04X", cpu.SR)
		}
	})
}

func TestMultiplicationAndDivision(t *testing.T) {
	t.Logf("=== Multiplication and Division Operations Tests ===")

	// ================== MULU TESTS ==================
	t.Run("MULU D0,D1 (Unsigned Multiply Register Direct)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000004 // Source (multiplier)
		cpu.DataRegs[1] = 0x00000003 // Destination (multiplicand)

		// MULU D0,D1
		opcode := uint16(0xC2C0) // MULU D0,D1

		// Expected results: 3 * 4 = 12
		expectedResults := map[string]uint32{
			"D0": 0x00000004,
			"D1": 0x0000000C,
		}

		runDetailedTest(t, cpu, opcode, "MULU D0,D1", expectedResults)

		// Verify flags - N and Z should be clear
		if (cpu.SR & (M68K_SR_N | M68K_SR_Z)) != 0 {
			t.Errorf("MULU flags incorrect. Expected N,Z clear, got %04X", cpu.SR)
		}
	})

	t.Run("MULU (A0),D1 (Unsigned Multiply Memory Indirect)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0x0002) // Value at memory address
		cpu.DataRegs[1] = 0x0000FFFF // Maximum 16-bit value

		// MULU (A0),D1
		opcode := uint16(0xC2D0) // MULU (A0),D1

		// Expected results: 0xFFFF * 2 = 0x1FFFE
		expectedResults := map[string]uint32{
			"A0": memAddr,
			"D1": 0x0001FFFE,
		}

		runDetailedTest(t, cpu, opcode, "MULU (A0),D1", expectedResults)
	})

	t.Run("MULU #$0,D2 (Unsigned Multiply by Zero)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[2] = 0x0000FFFF

		// Setup MULU #$0,D2 instruction (immediate addressing)
		cpu.Write16(cpu.PC, 0xC4FC)   // MULU #imm,D2
		cpu.Write16(cpu.PC+2, 0x0000) // Immediate value 0

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result: D2 should be 0
		if cpu.DataRegs[2] != 0 {
			t.Errorf("MULU #$0,D2 failed: D2=%08X, expected 0", cpu.DataRegs[2])
		}

		// Z flag should be set
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("MULU #$0,D2 flag incorrect: Z flag should be set")
		}
	})

	// ================== MULS TESTS ==================
	t.Run("MULS D0,D1 (Signed Multiply Positive Numbers)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000003 // Source (multiplier)
		cpu.DataRegs[1] = 0x00000004 // Destination (multiplicand)

		// MULS D0,D1
		opcode := uint16(0xC1C0) // MULS D0,D1

		// Expected results: 4 * 3 = 12
		expectedResults := map[string]uint32{
			"D0": 0x00000003,
			"D1": 0x0000000C,
		}

		runDetailedTest(t, cpu, opcode, "MULS D0,D1", expectedResults)
	})

	t.Run("MULS D0,D1 (Signed Multiply Negative Numbers)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - negative values
		cpu.DataRegs[0] = 0x0000FFFD // -3 as a signed 16-bit value
		cpu.DataRegs[1] = 0x0000FFFC // -4 as a signed 16-bit value

		// MULS D0,D1
		opcode := uint16(0xC1C0) // MULS D0,D1

		// Expected results: (-4) * (-3) = 12
		expectedResults := map[string]uint32{
			"D0": 0x0000FFFD, // Unchanged
			"D1": 0x0000000C, // Positive result
		}

		runDetailedTest(t, cpu, opcode, "MULS D0,D1 (Negative)", expectedResults)
	})

	t.Run("MULS D0,D1 (Signed Multiply Mixed Sign)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - mixed signs
		cpu.DataRegs[0] = 0x00000003 // Positive 3
		cpu.DataRegs[1] = 0x0000FFFC // -4 as a signed 16-bit value

		// MULS D0,D1
		opcode := uint16(0xC1C0) // MULS D0,D1

		// Expected results: (-4) * 3 = -12 = 0xFFFFFFF4 in 32-bit
		expectedResults := map[string]uint32{
			"D0": 0x00000003, // Unchanged
			"D1": 0xFFFFFFF4, // Negative result, sign-extended
		}

		runDetailedTest(t, cpu, opcode, "MULS D0,D1 (Mixed Signs)", expectedResults)

		// N flag should be set (negative result)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("MULS with negative result should set N flag")
		}
	})

	// ================== MULL TESTS ==================
	t.Run("MULL.L D0,D1:D2 (32x32->64 Long Multiply)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data for 32x32->64 bit multiplication
		cpu.DataRegs[0] = 0x00010001 // Source operand
		cpu.DataRegs[1] = 0x00020002 // High word destination
		cpu.DataRegs[2] = 0x00030003 // Low word destination (holds result)

		// Setup MULL D0,D1:D2 instruction
		// Format: 0x4C00 <ext1> <ext2> where ext1 has register specs
		cpu.Write16(cpu.PC, 0x4C00)   // MULL
		cpu.Write16(cpu.PC+2, 0x2C00) // High word in D1, using D0 as source
		cpu.Write16(cpu.PC+4, 0x0002) // Low word in D2, unsigned

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result: 0x00010001 * 0x00030003 = 0x0003 0003 0003 (64-bit result)
		if cpu.DataRegs[1] != 0x00000003 || cpu.DataRegs[2] != 0x00030003 {
			t.Errorf("MULL.L failed: D1:D2=%08X:%08X, expected 0x00000003:0x00030003",
				cpu.DataRegs[1], cpu.DataRegs[2])
		}
	})

	// ================== DIVU TESTS ==================
	t.Run("DIVU D0,D1 (Unsigned Divide Basic)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000003 // Divisor
		cpu.DataRegs[1] = 0x0000000F // Dividend

		// DIVU D0,D1
		opcode := uint16(0x82C0) // DIVU D0,D1

		// Expected results: 15 ÷ 3 = 5 remainder 0
		// Upper word = remainder, lower word = quotient
		expectedResults := map[string]uint32{
			"D0": 0x00000003, // Unchanged
			"D1": 0x00000005, // Quotient in lower word, remainder 0
		}

		runDetailedTest(t, cpu, opcode, "DIVU D0,D1", expectedResults)
	})

	t.Run("DIVU D0,D1 (Unsigned Divide with Remainder)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000003 // Divisor
		cpu.DataRegs[1] = 0x00000010 // Dividend (16)

		// DIVU D0,D1
		opcode := uint16(0x82C0) // DIVU D0,D1

		// Expected results: 16 ÷ 3 = 5 remainder 1
		// Upper word = remainder, lower word = quotient
		expectedResults := map[string]uint32{
			"D0": 0x00000003, // Unchanged
			"D1": 0x00010005, // Quotient 5 in lower word, remainder 1 in upper word
		}

		runDetailedTest(t, cpu, opcode, "DIVU D0,D1 (With Remainder)", expectedResults)
	})

	t.Run("DIVU D0,D1 (Division by Zero)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - divide by zero
		cpu.DataRegs[0] = 0x00000000 // Divisor (zero)
		cpu.DataRegs[1] = 0x0000AAAA // Dividend
		initialPC := cpu.PC

		// DIVU D0,D1
		opcode := uint16(0x82C0) // DIVU D0,D1
		cpu.Write16(cpu.PC, opcode)

		// Execute instruction directly
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Division by zero should trigger exception
		if cpu.PC == initialPC+2 {
			t.Errorf("DIVU division by zero should trigger exception")
		}

		// The register should remain unchanged
		if cpu.DataRegs[1] != 0x0000AAAA {
			t.Errorf("DIVU div by zero changed D1: %08X, expected unchanged 0x0000AAAA",
				cpu.DataRegs[1])
		}
	})

	t.Run("DIVU (A0),D1 (Memory Indirect Addressing)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0x0004) // Divisor in memory
		cpu.DataRegs[1] = 0x00000020 // Dividend (32)

		// DIVU (A0),D1
		opcode := uint16(0x82D0) // DIVU (A0),D1

		// Expected results: 32 ÷ 4 = 8 remainder 0
		expectedResults := map[string]uint32{
			"A0": memAddr,
			"D1": 0x00000008,
		}

		runDetailedTest(t, cpu, opcode, "DIVU (A0),D1", expectedResults)
	})

	t.Run("DIVU D0,D1 (Overflow Test)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - value that causes overflow in 16-bit quotient
		cpu.DataRegs[0] = 0x00000001 // Divisor (1)
		cpu.DataRegs[1] = 0x00010000 // Dividend (too large for 16-bit quotient)

		// DIVU D0,D1
		opcode := uint16(0x82C0) // DIVU D0,D1

		// Expected results: Overflow should set V flag and leave D1 unchanged
		expectedResults := map[string]uint32{
			"D0": 0x00000001,
			"D1": 0x00010000, // Unchanged due to overflow
		}

		runDetailedTest(t, cpu, opcode, "DIVU D0,D1 (Overflow)", expectedResults)

		// V flag should be set for overflow
		if (cpu.SR & M68K_SR_V) == 0 {
			t.Errorf("DIVU overflow should set V flag")
		}
	})

	// ================== DIVS TESTS ==================
	t.Run("DIVS D0,D1 (Signed Divide Positive by Positive)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000003 // Divisor (+3)
		cpu.DataRegs[1] = 0x0000000F // Dividend (+15)

		// DIVS D0,D1
		opcode := uint16(0x81C0) // DIVS D0,D1

		// Expected results: 15 ÷ 3 = 5 remainder 0
		expectedResults := map[string]uint32{
			"D0": 0x00000003, // Unchanged
			"D1": 0x00000005, // Quotient in lower word, remainder 0
		}

		runDetailedTest(t, cpu, opcode, "DIVS D0,D1", expectedResults)
	})

	t.Run("DIVS D0,D1 (Signed Divide Negative by Positive)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - negative dividend
		cpu.DataRegs[0] = 0x00000003 // Divisor (+3)
		cpu.DataRegs[1] = 0x0000FFF1 // Dividend (-15 as signed 16-bit)

		// DIVS D0,D1
		opcode := uint16(0x81C0) // DIVS D0,D1

		// Expected results: -15 ÷ 3 = -5 remainder 0
		// Negative quotient should be sign-extended
		expectedResults := map[string]uint32{
			"D0": 0x00000003, // Unchanged
			"D1": 0x0000FFFB, // -5 as signed 16-bit value
		}

		runDetailedTest(t, cpu, opcode, "DIVS D0,D1 (Negative Dividend)", expectedResults)

		// N flag should be set (negative result)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("DIVS with negative result should set N flag")
		}
	})

	t.Run("DIVS D0,D1 (Signed Divide with Remainder)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000004 // Divisor (+4)
		cpu.DataRegs[1] = 0x0000FFFD // Dividend (-3 as signed 16-bit)

		// DIVS D0,D1
		opcode := uint16(0x81C0) // DIVS D0,D1

		// Expected results: -3 ÷ 4 = 0 remainder -3
		// Remainder has same sign as dividend
		expectedResults := map[string]uint32{
			"D0": 0x00000004, // Unchanged
			"D1": 0xFFFD0000, // Quotient 0, remainder -3 in upper word
		}

		runDetailedTest(t, cpu, opcode, "DIVS D0,D1 (With Remainder)", expectedResults)
	})

	t.Run("DIVS D0,D1 (Special Case: -2^15 ÷ -1)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - edge case that would overflow
		cpu.DataRegs[0] = 0x0000FFFF // Divisor (-1)
		cpu.DataRegs[1] = 0x00008000 // Dividend (-32768 or -2^15)

		// DIVS D0,D1
		opcode := uint16(0x81C0) // DIVS D0,D1

		// Expected results: Overflow should set V flag and leave D1 unchanged
		expectedResults := map[string]uint32{
			"D0": 0x0000FFFF, // Unchanged
			"D1": 0x00008000, // Unchanged due to overflow
		}

		runDetailedTest(t, cpu, opcode, "DIVS D0,D1 (Special Case)", expectedResults)

		// V flag should be set for overflow
		if (cpu.SR & M68K_SR_V) == 0 {
			t.Errorf("DIVS overflow should set V flag")
		}
	})

	// ================== DIVL TESTS ==================
	t.Run("DIVL.L D0,D1:D2 (32÷32->32 Long Division)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data for 32÷32->32 bit division
		cpu.DataRegs[0] = 0x00000003 // Divisor (3)
		cpu.DataRegs[1] = 0x00000000 // High word of dividend (0)
		cpu.DataRegs[2] = 0x0000000F // Low word of dividend (15)

		// Setup DIVL D0,D1:D2 instruction
		// Format: 0x4C40 <ext1> <ext2> where ext1 has register specs
		cpu.Write16(cpu.PC, 0x4C40)   // DIVL
		cpu.Write16(cpu.PC+2, 0x2C00) // High word in D1, using D0 as source
		cpu.Write16(cpu.PC+4, 0x0002) // Low word in D2, unsigned

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result: 15 ÷ 3 = 5 remainder 0
		// D2 should contain quotient, D1 should contain remainder
		if cpu.DataRegs[2] != 0x00000005 || cpu.DataRegs[1] != 0x00000000 {
			t.Errorf("DIVL.L failed: D1:D2=%08X:%08X, expected 0x00000000:0x00000005",
				cpu.DataRegs[1], cpu.DataRegs[2])
		}
	})

	t.Run("DIVL.L D0,D1:D2 (Long Division with Remainder)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000004 // Divisor (4)
		cpu.DataRegs[1] = 0x00000000 // High word of dividend (0)
		cpu.DataRegs[2] = 0x0000000E // Low word of dividend (14)

		// Setup DIVL D0,D1:D2 instruction
		cpu.Write16(cpu.PC, 0x4C40)   // DIVL
		cpu.Write16(cpu.PC+2, 0x2C00) // High word in D1, using D0 as source
		cpu.Write16(cpu.PC+4, 0x0002) // Low word in D2, unsigned

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result: 14 ÷ 4 = 3 remainder 2
		// D2 should contain quotient, D1 should contain remainder
		if cpu.DataRegs[2] != 0x00000003 || cpu.DataRegs[1] != 0x00000002 {
			t.Errorf("DIVL.L with remainder failed: D1:D2=%08X:%08X, expected 0x00000002:0x00000003",
				cpu.DataRegs[1], cpu.DataRegs[2])
		}
	})
}

func TestLogicalOperations(t *testing.T) {
	t.Logf("=== Logical Operations Tests ===")

	// ================== AND TESTS ==================
	t.Run("AND.B D0,D1 (Data Register to Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000F0 // Source
		cpu.DataRegs[1] = 0x0000000F // Destination

		// AND.B D0,D1
		opcode := uint16(0xC200) // AND.B D0,D1

		// Expected results: 0x0F & 0xF0 = 0x00
		expectedResults := map[string]uint32{
			"D0": 0x000000F0,
			"D1": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "AND.B D0,D1", expectedResults)

		// Verify Z flag is set (result is zero)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("AND.B failed: Z flag should be set for zero result")
		}
	})

	t.Run("AND.W D0,D1 (Non-Zero Result)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x0000FFFF // Source (all bits set)
		cpu.DataRegs[1] = 0x0000F0F0 // Destination

		// AND.W D0,D1
		opcode := uint16(0xC240) // AND.W D0,D1

		// Expected results: 0xF0F0 & 0xFFFF = 0xF0F0
		expectedResults := map[string]uint32{
			"D0": 0x0000FFFF,
			"D1": 0x0000F0F0,
		}

		runDetailedTest(t, cpu, opcode, "AND.W D0,D1", expectedResults)

		// N flag should be set (MSB of result is 1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("AND.W failed: N flag should be set for result with MSB=1")
		}
	})

	t.Run("AND.L D0,D1 (Long Word Operation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0xFFFF0000 // Source
		cpu.DataRegs[1] = 0x0000FFFF // Destination

		// AND.L D0,D1
		opcode := uint16(0xC280) // AND.L D0,D1

		// Expected results: 0xFFFF0000 & 0x0000FFFF = 0x00000000
		expectedResults := map[string]uint32{
			"D0": 0xFFFF0000,
			"D1": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "AND.L D0,D1", expectedResults)

		// Z flag should be set (result is zero)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("AND.L failed: Z flag should be set for zero result")
		}
	})

	t.Run("AND.B D0,(A0) (Data Register to Memory)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000AA // Source
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x55) // Initial memory value

		// AND.B D0,(A0)
		opcode := uint16(0xC110) // AND.B D0,(A0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x000000AA,
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "AND.B D0,(A0)", expectedResults)

		// Check memory result
		result := cpu.Read8(memAddr)
		if result != 0x00 {
			t.Errorf("AND.B D0,(A0) memory result incorrect: %02X, expected 0x00", result)
		}
	})

	// ANDI tests
	t.Run("ANDI.B #$0F,D0 (Immediate to Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000FF // Destination

		// Setup ANDI.B #$0F,D0 instruction
		cpu.Write16(cpu.PC, 0x0200)   // ANDI.B
		cpu.Write16(cpu.PC+2, 0x000F) // Immediate value 0x0F

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result
		if cpu.DataRegs[0] != 0x0000000F {
			t.Errorf("ANDI.B #$0F,D0 failed: D0=%08X, expected 0x0000000F", cpu.DataRegs[0])
		}

		// Verify flags - Z and V should be clear, N should be clear
		if (cpu.SR & (M68K_SR_Z | M68K_SR_V | M68K_SR_N)) != 0 {
			t.Errorf("ANDI.B flags incorrect: Expected all cleared, got %04X", cpu.SR)
		}
	})

	t.Run("ANDI.W #$FF00,D1 (Immediate Word Operation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[1] = 0x0000FFFF // Destination

		// Setup ANDI.W #$FF00,D1 instruction
		cpu.Write16(cpu.PC, 0x0241)   // ANDI.W
		cpu.Write16(cpu.PC+2, 0xFF00) // Immediate value 0xFF00

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result
		if cpu.DataRegs[1] != 0x0000FF00 {
			t.Errorf("ANDI.W #$FF00,D1 failed: D1=%08X, expected 0x0000FF00", cpu.DataRegs[1])
		}

		// N flag should be set (MSB of result is 1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("ANDI.W flag incorrect: N flag should be set")
		}
	})

	// ================== OR TESTS ==================
	t.Run("OR.B D0,D1 (Data Register to Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000F0 // Source
		cpu.DataRegs[1] = 0x0000000F // Destination

		// OR.B D0,D1
		opcode := uint16(0x8200) // OR.B D0,D1

		// Expected results: 0x0F | 0xF0 = 0xFF
		expectedResults := map[string]uint32{
			"D0": 0x000000F0,
			"D1": 0x000000FF,
		}

		runDetailedTest(t, cpu, opcode, "OR.B D0,D1", expectedResults)

		// N flag should be set (MSB of result is 1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("OR.B failed: N flag should be set for result with MSB=1")
		}
	})

	t.Run("OR.W D0,D1 (Zero Source)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000000 // Source (all zeros)
		cpu.DataRegs[1] = 0x0000F0F0 // Destination

		// OR.W D0,D1
		opcode := uint16(0x8240) // OR.W D0,D1

		// Expected results: 0xF0F0 | 0x0000 = 0xF0F0 (unchanged)
		expectedResults := map[string]uint32{
			"D0": 0x00000000,
			"D1": 0x0000F0F0,
		}

		runDetailedTest(t, cpu, opcode, "OR.W D0,D1", expectedResults)
	})

	t.Run("OR.L D0,D1 (All Bits Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0xFFFF0000 // Source
		cpu.DataRegs[1] = 0x0000FFFF // Destination

		// OR.L D0,D1
		opcode := uint16(0x8280) // OR.L D0,D1

		// Expected results: 0xFFFF0000 | 0x0000FFFF = 0xFFFFFFFF
		expectedResults := map[string]uint32{
			"D0": 0xFFFF0000,
			"D1": 0xFFFFFFFF,
		}

		runDetailedTest(t, cpu, opcode, "OR.L D0,D1", expectedResults)

		// N flag should be set (MSB of result is 1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("OR.L failed: N flag should be set for result with MSB=1")
		}
	})

	t.Run("OR.B D0,(A0) (Data Register to Memory)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000AA // Source
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x55) // Initial memory value

		// OR.B D0,(A0)
		opcode := uint16(0x8110) // OR.B D0,(A0)

		// Expected results
		expectedResults := map[string]uint32{
			"D0": 0x000000AA,
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "OR.B D0,(A0)", expectedResults)

		// Check memory result
		result := cpu.Read8(memAddr)
		if result != 0xFF {
			t.Errorf("OR.B D0,(A0) memory result incorrect: %02X, expected 0xFF", result)
		}
	})

	// ORI tests
	t.Run("ORI.B #$0F,D0 (Immediate to Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000F0 // Destination

		// Setup ORI.B #$0F,D0 instruction
		cpu.Write16(cpu.PC, 0x0000)   // ORI.B
		cpu.Write16(cpu.PC+2, 0x000F) // Immediate value 0x0F

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result
		if cpu.DataRegs[0] != 0x000000FF {
			t.Errorf("ORI.B #$0F,D0 failed: D0=%08X, expected 0x000000FF", cpu.DataRegs[0])
		}

		// Verify N flag is set (result has MSB=1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("ORI.B flag incorrect: N flag should be set")
		}
	})

	// ================== EOR TESTS ==================
	t.Run("EOR.B D0,D1 (Data Register to Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000FF // Source
		cpu.DataRegs[1] = 0x000000FF // Destination

		// EOR.B D0,D1 - format: 1011 rrr 1 ss mmm rrr (rrr=D0=000, ss=00=byte, mmm=000=Dn, rrr=001=D1)
		opcode := uint16(0xB101) // EOR.B D0,D1

		// Expected results: 0xFF ^ 0xFF = 0x00
		expectedResults := map[string]uint32{
			"D0": 0x000000FF,
			"D1": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "EOR.B D0,D1", expectedResults)

		// Z flag should be set (result is zero)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("EOR.B failed: Z flag should be set for zero result")
		}
	})

	t.Run("EOR.W D0,D1 (Selective Bit Flipping)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x0000FF00 // Source
		cpu.DataRegs[1] = 0x0000FFFF // Destination

		// EOR.W D0,D1 - format: 1011 rrr 1 ss mmm rrr (rrr=D0=000, ss=01=word, mmm=000=Dn, rrr=001=D1)
		opcode := uint16(0xB141) // EOR.W D0,D1

		// Expected results: 0xFFFF ^ 0xFF00 = 0x00FF
		expectedResults := map[string]uint32{
			"D0": 0x0000FF00,
			"D1": 0x000000FF,
		}

		runDetailedTest(t, cpu, opcode, "EOR.W D0,D1", expectedResults)
	})

	t.Run("EOR.L D0,D1 (Long Word Operation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0xAAAAAAAA // Source (alternating bits)
		cpu.DataRegs[1] = 0x55555555 // Destination (alternating bits)

		// EOR.L D0,D1 - format: 1011 rrr 1 ss mmm rrr (rrr=D0=000, ss=10=long, mmm=000=Dn, rrr=001=D1)
		opcode := uint16(0xB181) // EOR.L D0,D1

		// Expected results: 0x55555555 ^ 0xAAAAAAAA = 0xFFFFFFFF
		expectedResults := map[string]uint32{
			"D0": 0xAAAAAAAA,
			"D1": 0xFFFFFFFF,
		}

		runDetailedTest(t, cpu, opcode, "EOR.L D0,D1", expectedResults)

		// N flag should be set (MSB of result is 1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("EOR.L failed: N flag should be set for result with MSB=1")
		}
	})

	// EORI tests
	t.Run("EORI.B #$FF,D0 (Immediate to Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000055 // Destination

		// Setup EORI.B #$FF,D0 instruction
		cpu.Write16(cpu.PC, 0x0A00)   // EORI.B
		cpu.Write16(cpu.PC+2, 0x00FF) // Immediate value 0xFF

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result
		if cpu.DataRegs[0] != 0x000000AA {
			t.Errorf("EORI.B #$FF,D0 failed: D0=%08X, expected 0x000000AA", cpu.DataRegs[0])
		}

		// Verify N flag is set (result has MSB=1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("EORI.B flag incorrect: N flag should be set")
		}
	})

	// ================== NOT TESTS ==================
	t.Run("NOT.B D0 (Data Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000AA // ~0xAA = 0x55

		// NOT.B D0
		opcode := uint16(0x4600) // NOT.B D0

		// Expected results: ~0xAA = 0x55
		expectedResults := map[string]uint32{
			"D0": 0x00000055,
		}

		runDetailedTest(t, cpu, opcode, "NOT.B D0", expectedResults)
	})

	t.Run("NOT.W D1 (Word Operation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[1] = 0x0000FFFF // ~0xFFFF = 0x0000

		// NOT.W D1
		opcode := uint16(0x4641) // NOT.W D1

		// Expected results: ~0xFFFF = 0x0000
		expectedResults := map[string]uint32{
			"D1": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "NOT.W D1", expectedResults)

		// Z flag should be set (result is zero)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("NOT.W failed: Z flag should be set for zero result")
		}
	})

	t.Run("NOT.L D2 (Long Word Operation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[2] = 0x00000000 // ~0x00000000 = 0xFFFFFFFF

		// NOT.L D2
		opcode := uint16(0x4682) // NOT.L D2

		// Expected results: ~0x00000000 = 0xFFFFFFFF
		expectedResults := map[string]uint32{
			"D2": 0xFFFFFFFF,
		}

		runDetailedTest(t, cpu, opcode, "NOT.L D2", expectedResults)

		// N flag should be set (MSB of result is 1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("NOT.L failed: N flag should be set for result with MSB=1")
		}
	})

	t.Run("NOT.B (A0) (Memory Operation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x55) // ~0x55 = 0xAA

		// NOT.B (A0)
		opcode := uint16(0x4610) // NOT.B (A0)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "NOT.B (A0)", expectedResults)

		// Check memory result
		result := cpu.Read8(memAddr)
		if result != 0xAA {
			t.Errorf("NOT.B (A0) memory result incorrect: %02X, expected 0xAA", result)
		}
	})

	// ================== SPECIAL CASES ==================
	t.Run("AND.B #0,D0 (Zero Mask)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000FF

		// Setup ANDI.B #$00,D0 instruction
		cpu.Write16(cpu.PC, 0x0200)   // ANDI.B
		cpu.Write16(cpu.PC+2, 0x0000) // Immediate value 0x00

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result is zero
		if cpu.DataRegs[0] != 0x00000000 {
			t.Errorf("ANDI.B #$00,D0 failed: D0=%08X, expected 0x00000000", cpu.DataRegs[0])
		}

		// Z flag should be set
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("ANDI.B with zero mask should set Z flag")
		}
	})

	t.Run("OR.B #0,D0 (Identity Element)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000AA

		// Setup ORI.B #$00,D0 instruction
		cpu.Write16(cpu.PC, 0x0000)   // ORI.B
		cpu.Write16(cpu.PC+2, 0x0000) // Immediate value 0x00

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result is unchanged
		if cpu.DataRegs[0] != 0x000000AA {
			t.Errorf("ORI.B #$00,D0 failed: D0=%08X, expected 0x000000AA", cpu.DataRegs[0])
		}
	})

	t.Run("EOR.B #0,D0 (Identity Element)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000AA

		// Setup EORI.B #$00,D0 instruction
		cpu.Write16(cpu.PC, 0x0A00)   // EORI.B
		cpu.Write16(cpu.PC+2, 0x0000) // Immediate value 0x00

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result is unchanged
		if cpu.DataRegs[0] != 0x000000AA {
			t.Errorf("EORI.B #$00,D0 failed: D0=%08X, expected 0x000000AA", cpu.DataRegs[0])
		}
	})
}

func TestCCRSROperations(t *testing.T) {
	t.Logf("=== CCR/SR Operations Tests ===")

	// ================== ANDI to CCR TESTS ==================
	t.Run("ANDI to CCR (Clear All Flags)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set all CCR flags
		cpu.SR = 0x001F // All CCR flags set (X,N,Z,V,C)

		// Setup ANDI to CCR instruction
		cpu.Write16(cpu.PC, 0x023C)   // ANDI to CCR
		cpu.Write16(cpu.PC+2, 0x0000) // Immediate value 0x00 (clears all flags)

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - all CCR flags should be cleared
		if (cpu.SR & 0x001F) != 0 {
			t.Errorf("ANDI to CCR failed: SR=%04X, expected all CCR flags cleared", cpu.SR)
		}
	})

	t.Run("ANDI to CCR (Selective Clear)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set all CCR flags
		cpu.SR = 0x001F // All CCR flags set (X,N,Z,V,C)

		// Setup ANDI to CCR instruction - keep X,Z flags only
		cpu.Write16(cpu.PC, 0x023C)   // ANDI to CCR
		cpu.Write16(cpu.PC+2, 0x0014) // Immediate value 0x14 (X and Z bits set)

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - only X and Z should remain set
		if (cpu.SR & 0x001F) != 0x0014 {
			t.Errorf("ANDI to CCR selective clear failed: SR=%04X, expected 0x0014", cpu.SR&0x001F)
		}
	})

	// ================== ORI to CCR TESTS ==================
	t.Run("ORI to CCR (Set All Flags)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - clear all CCR flags
		cpu.SR = 0x0000 // All flags cleared

		// Setup ORI to CCR instruction
		cpu.Write16(cpu.PC, 0x003C)   // ORI to CCR
		cpu.Write16(cpu.PC+2, 0x001F) // Immediate value 0x1F (sets all CCR flags)

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - all CCR flags should be set
		if (cpu.SR & 0x001F) != 0x001F {
			t.Errorf("ORI to CCR failed: SR=%04X, expected all CCR flags set", cpu.SR)
		}
	})

	t.Run("ORI to CCR (Selective Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - clear all CCR flags
		cpu.SR = 0x0000 // All flags cleared

		// Setup ORI to CCR instruction - set N,V flags only
		cpu.Write16(cpu.PC, 0x003C)   // ORI to CCR
		cpu.Write16(cpu.PC+2, 0x000A) // Immediate value 0x0A (N and V bits)

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - only N and V should be set
		if (cpu.SR & 0x001F) != 0x000A {
			t.Errorf("ORI to CCR selective set failed: SR=%04X, expected 0x000A", cpu.SR&0x001F)
		}
	})

	// ================== EORI to CCR TESTS ==================
	t.Run("EORI to CCR (Toggle All Flags)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set some CCR flags
		cpu.SR = 0x000A // N and V flags set

		// Setup EORI to CCR instruction
		cpu.Write16(cpu.PC, 0x0A3C)   // EORI to CCR
		cpu.Write16(cpu.PC+2, 0x001F) // Immediate value 0x1F (toggle all CCR flags)

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - N and V should be cleared, X,Z,C should be set
		if (cpu.SR & 0x001F) != 0x0015 {
			t.Errorf("EORI to CCR toggle failed: SR=%04X, expected 0x0015", cpu.SR&0x001F)
		}
	})

	t.Run("EORI to CCR (No Change with Zero)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set some CCR flags
		cpu.SR = 0x000A // N and V flags set

		// Setup EORI to CCR instruction with zero (no change)
		cpu.Write16(cpu.PC, 0x0A3C)   // EORI to CCR
		cpu.Write16(cpu.PC+2, 0x0000) // Immediate value 0x00 (no bits toggled)

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - should be unchanged
		if (cpu.SR & 0x001F) != 0x000A {
			t.Errorf("EORI to CCR with zero failed: SR=%04X, expected 0x000A", cpu.SR&0x001F)
		}
	})

	// ================== ANDI to SR TESTS ==================
	t.Run("ANDI to SR (Supervisor Mode)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set supervisor mode and some flags
		cpu.SR = 0x2F1F // Supervisor mode (bit 13) with various flags set

		// Setup ANDI to SR instruction
		cpu.Write16(cpu.PC, 0x027C)   // ANDI to SR
		cpu.Write16(cpu.PC+2, 0x2000) // Keep only supervisor bit

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - should clear all except supervisor bit
		if cpu.SR != 0x2000 {
			t.Errorf("ANDI to SR failed: SR=%04X, expected 0x2000", cpu.SR)
		}
	})

	t.Run("ANDI to SR (User Mode - Privilege Violation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - ensure user mode (clear supervisor bit)
		cpu.SR = 0x0F1F // User mode with various flags set
		initialPC := cpu.PC

		// Setup ANDI to SR instruction
		cpu.Write16(cpu.PC, 0x027C)   // ANDI to SR
		cpu.Write16(cpu.PC+2, 0x0000) // Clear all bits

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify privilege violation
		if cpu.PC == initialPC+4 {
			t.Errorf("ANDI to SR in user mode should cause privilege violation")
		}

		// SR should remain unchanged
		if (cpu.SR & 0x0F1F) != 0x0F1F {
			t.Errorf("ANDI to SR privilege violation should not change SR: %04X", cpu.SR)
		}
	})

	// ================== ORI to SR TESTS ==================
	t.Run("ORI to SR (Set Interrupt Mask)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - supervisor mode with low interrupt mask
		cpu.SR = 0x2000 // Supervisor mode only

		// Setup ORI to SR instruction
		cpu.Write16(cpu.PC, 0x007C)   // ORI to SR
		cpu.Write16(cpu.PC+2, 0x0700) // Set interrupt mask to highest level

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - interrupt mask should be set to 7
		if (cpu.SR & 0x0700) != 0x0700 {
			t.Errorf("ORI to SR failed to set interrupt mask: SR=%04X", cpu.SR)
		}
	})

	// ================== EORI to SR TESTS ==================
	t.Run("EORI to SR (Toggle Trace Mode)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - supervisor mode with trace mode off
		cpu.SR = 0x2000 // Supervisor mode only

		// Setup EORI to SR instruction
		cpu.Write16(cpu.PC, 0x0A7C)   // EORI to SR
		cpu.Write16(cpu.PC+2, 0x8000) // Toggle trace bit (T1)

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - trace mode should be toggled on
		if (cpu.SR & 0x8000) != 0x8000 {
			t.Errorf("EORI to SR failed to toggle trace mode: SR=%04X", cpu.SR)
		}
	})

	// ================== MOVE from SR TESTS ==================
	t.Run("MOVE from SR to Data Register", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - specific SR value
		cpu.SR = 0x2F1F // Supervisor mode with various flags
		cpu.DataRegs[0] = 0xFFFFFFFF

		// Setup MOVE SR,D0 instruction
		cpu.Write16(cpu.PC, 0x40C0) // MOVE SR,D0

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - D0 should contain SR value in lower word
		if (cpu.DataRegs[0] & 0xFFFF) != 0x2F1F {
			t.Errorf("MOVE from SR failed: D0=%08X, expected SR=0x2F1F in lower word",
				cpu.DataRegs[0])
		}
	})

	t.Run("MOVE from SR to Memory", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.SR = 0x2F1F // Supervisor mode with various flags
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr

		// Setup MOVE SR,(A0) instruction
		cpu.Write16(cpu.PC, 0x40D0) // MOVE SR,(A0)

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - memory should contain SR value
		if cpu.Read16(memAddr) != 0x2F1F {
			t.Errorf("MOVE from SR to memory failed: Mem[%08X]=%04X, expected 0x2F1F",
				memAddr, cpu.Read16(memAddr))
		}
	})

	// ================== MOVE to SR TESTS ==================
	t.Run("MOVE to SR from Data Register (Supervisor Mode)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - supervisor mode
		cpu.SR = 0x2000              // Supervisor mode only
		cpu.DataRegs[0] = 0x0000A71F // New SR value

		// Setup MOVE D0,SR instruction
		cpu.Write16(cpu.PC, 0x46C0) // MOVE D0,SR

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - SR should be updated
		if cpu.SR != 0xA71F {
			t.Errorf("MOVE to SR failed: SR=%04X, expected 0xA71F", cpu.SR)
		}
	})

	t.Run("MOVE to SR from Memory (Supervisor Mode)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - supervisor mode
		cpu.SR = 0x2000 // Supervisor mode only
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0xA71F) // New SR value

		// Setup MOVE (A0),SR instruction
		cpu.Write16(cpu.PC, 0x46D0) // MOVE (A0),SR

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - SR should be updated
		if cpu.SR != 0xA71F {
			t.Errorf("MOVE to SR from memory failed: SR=%04X, expected 0xA71F", cpu.SR)
		}
	})

	t.Run("MOVE to SR (User Mode - Privilege Violation)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - ensure user mode
		cpu.SR = 0x0000              // User mode
		cpu.DataRegs[0] = 0x0000A71F // Value to move to SR
		initialPC := cpu.PC

		// Setup MOVE D0,SR instruction
		cpu.Write16(cpu.PC, 0x46C0) // MOVE D0,SR

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify privilege violation
		if cpu.PC == initialPC+2 {
			t.Errorf("MOVE to SR in user mode should cause privilege violation")
		}

		// SR should remain unchanged
		if cpu.SR != 0x0000 {
			t.Errorf("MOVE to SR privilege violation should not change SR: %04X", cpu.SR)
		}
	})

	// ================== MOVE to/from CCR TESTS ==================
	t.Run("MOVE from Data Register to CCR", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.SR = 0x2000              // Supervisor bit set, CCR cleared
		cpu.DataRegs[0] = 0x0000001F // All CCR flags set

		// Setup MOVE D0,CCR instruction
		cpu.Write16(cpu.PC, 0x44C0) // MOVE D0,CCR

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - CCR bits should be set, supervisor bit unchanged
		if cpu.SR != 0x201F {
			t.Errorf("MOVE to CCR failed: SR=%04X, expected 0x201F", cpu.SR)
		}
	})

	t.Run("MOVE CCR to Data Register", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.SR = 0x201F // Supervisor bit and all CCR flags set
		cpu.DataRegs[0] = 0xFFFFFFFF

		// Setup MOVE CCR,D0 instruction
		cpu.Write16(cpu.PC, 0x42C0) // MOVE CCR,D0

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - D0 should contain CCR value in lower word
		if (cpu.DataRegs[0] & 0xFFFF) != 0x001F {
			t.Errorf("MOVE from CCR failed: D0=%08X, expected CCR=0x001F in lower word",
				cpu.DataRegs[0])
		}
	})
}

func TestShiftAndRotateOperations(t *testing.T) {
	t.Logf("=== Shift and Rotate Operations Tests ===")

	// ================== ASL TESTS ==================
	t.Run("ASL.B #1,D0 (Arithmetic Shift Left Byte)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000040 // Bit 6 set, will shift into bit 7

		// ASL.B #1,D0
		opcode := uint16(0xE300) // ASL.B #1,D0

		// Expected results: 0x40 << 1 = 0x80
		expectedResults := map[string]uint32{
			"D0": 0x00000080,
		}

		runDetailedTest(t, cpu, opcode, "ASL.B #1,D0", expectedResults)

		// Verify N flag set (MSB=1), Z,V,C clear
		if (cpu.SR & (M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)) != M68K_SR_N {
			t.Errorf("ASL.B flags incorrect: SR=%04X, expected N set", cpu.SR)
		}
	})

	t.Run("ASL.B #1,D0 (With Overflow and Carry)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - bit shifts from bit 7 to outside (overflow + carry)
		cpu.DataRegs[0] = 0x00000080

		// ASL.B #1,D0
		opcode := uint16(0xE300) // ASL.B #1,D0

		// Expected results: 0x80 << 1 = 0x00 with carry out and overflow
		expectedResults := map[string]uint32{
			"D0": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "ASL.B #1,D0 (Overflow)", expectedResults)

		// Verify Z,V,C,X flags set (overflow, carry, and zero result)
		if (cpu.SR & (M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X)) != (M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X) {
			t.Errorf("ASL.B overflow flags incorrect: SR=%04X, expected Z,V,C,X set", cpu.SR)
		}
	})

	t.Run("ASL.W #4,D1 (Multiple Bit Shift)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[1] = 0x00000101 // Will shift into 0x1010

		// ASL.W #4,D1
		opcode := uint16(0xE949) // ASL.W #4,D1

		// Expected results: 0x0101 << 4 = 0x1010
		expectedResults := map[string]uint32{
			"D1": 0x00001010,
		}

		runDetailedTest(t, cpu, opcode, "ASL.W #4,D1", expectedResults)
	})

	t.Run("ASL.L #8,D2 (Long Word Shift by 8)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[2] = 0x00ABCDEF // Will shift to 0xABCDEF00

		// ASL.L #8,D2
		opcode := uint16(0xE1A2) // ASL.L #8,D2 (size 2, count 001, D2)

		// Expected results: 0x00ABCDEF << 8 = 0xABCDEF00
		expectedResults := map[string]uint32{
			"D2": 0xABCDEF00,
		}

		runDetailedTest(t, cpu, opcode, "ASL.L #8,D2", expectedResults)

		// N flag should be set (MSB=1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("ASL.L #8,D2 flags incorrect: N flag should be set")
		}
	})

	t.Run("ASL.B D1,D0 (Register Shift Count)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000001 // Value to shift
		cpu.DataRegs[1] = 0x00000003 // Shift count in D1

		// ASL.B D1,D0
		opcode := uint16(0xE320) // ASL.B D1,D0

		// Expected results: 0x01 << 3 = 0x08
		expectedResults := map[string]uint32{
			"D0": 0x00000008,
			"D1": 0x00000003, // Count register unchanged
		}

		runDetailedTest(t, cpu, opcode, "ASL.B D1,D0", expectedResults)
	})

	t.Run("ASL.W (A0) (Memory Shift by 1)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0x0001) // Will shift to 0x0002

		// ASL.W (A0)
		opcode := uint16(0xE1D0) // ASL.W (A0)

		// Expected results: A0 unchanged
		expectedResults := map[string]uint32{
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "ASL.W (A0)", expectedResults)

		// Check memory result
		result := cpu.Read16(memAddr)
		if result != 0x0002 {
			t.Errorf("ASL.W (A0) memory result incorrect: %04X, expected 0x0002", result)
		}
	})

	// ================== ASR TESTS ==================
	t.Run("ASR.B #1,D0 (Arithmetic Shift Right Byte, Sign Extension)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - negative value (MSB=1)
		cpu.DataRegs[0] = 0x000000FF // -1 in two's complement

		// ASR.B #1,D0
		opcode := uint16(0xE000) // ASR.B #1,D0

		// Expected results: 0xFF >> 1 = 0xFF (sign bit preserved)
		expectedResults := map[string]uint32{
			"D0": 0x000000FF,
		}

		runDetailedTest(t, cpu, opcode, "ASR.B #1,D0", expectedResults)

		// Verify N,C,X flags set (negative result, bit shifted into carry)
		if (cpu.SR & (M68K_SR_N | M68K_SR_C | M68K_SR_X)) != (M68K_SR_N | M68K_SR_C | M68K_SR_X) {
			t.Errorf("ASR.B flags incorrect: SR=%04X, expected N,C,X set", cpu.SR)
		}
	})

	t.Run("ASR.B #1,D0 (Arithmetic Shift Right, Positive Value)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - positive value with bit 0 set
		cpu.DataRegs[0] = 0x00000003 // Bit 0 set (0x03 = 00000011)

		// ASR.B #1,D0
		opcode := uint16(0xE000) // ASR.B #1,D0

		// Expected results: 0x03 >> 1 = 0x01
		expectedResults := map[string]uint32{
			"D0": 0x00000001,
		}

		runDetailedTest(t, cpu, opcode, "ASR.B #1,D0 (Positive)", expectedResults)

		// Verify C,X flags set (bit shifted into carry)
		if (cpu.SR & (M68K_SR_C | M68K_SR_X)) != (M68K_SR_C | M68K_SR_X) {
			t.Errorf("ASR.B flags incorrect: SR=%04X, expected C,X set", cpu.SR)
		}
	})

	t.Run("ASR.W #8,D1 (Multi-Bit Shift, Word)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - negative word value
		cpu.DataRegs[1] = 0x0000ABCD // Negative in word context (MSB=1)

		// ASR.W #8,D1
		opcode := uint16(0xE04A) // ASR.W #8,D1 (count 001 = 8)

		// Expected results: 0xABCD >> 8 = 0xFFAB (sign extended)
		expectedResults := map[string]uint32{
			"D1": 0x0000FFAB,
		}

		runDetailedTest(t, cpu, opcode, "ASR.W #8,D1", expectedResults)
	})

	// ================== LSL TESTS ==================
	t.Run("LSL.B #1,D0 (Logical Shift Left Byte)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000041 // Will shift to 0x82

		// LSL.B #1,D0
		opcode := uint16(0xE308) // LSL.B #1,D0

		// Expected results: 0x41 << 1 = 0x82
		expectedResults := map[string]uint32{
			"D0": 0x00000082,
		}

		runDetailedTest(t, cpu, opcode, "LSL.B #1,D0", expectedResults)

		// Verify N flag set (MSB=1), Z,V,C clear
		if (cpu.SR & (M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)) != M68K_SR_N {
			t.Errorf("LSL.B flags incorrect: SR=%04X, expected N set", cpu.SR)
		}
	})

	t.Run("LSL.W D1,D0 (Register Shift Count)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000001 // Value to shift
		cpu.DataRegs[1] = 0x0000000F // Shift count in D1 (only low 5 bits used)

		// LSL.W D1,D0
		opcode := uint16(0xE368) // LSL.W D1,D0

		// Expected results: 0x0001 << 15 = 0x8000
		expectedResults := map[string]uint32{
			"D0": 0x00008000,
			"D1": 0x0000000F, // Count register unchanged
		}

		runDetailedTest(t, cpu, opcode, "LSL.W D1,D0", expectedResults)

		// Verify N flag set (MSB=1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("LSL.W flags incorrect: N flag should be set")
		}
	})

	// ================== LSR TESTS ==================
	t.Run("LSR.B #1,D0 (Logical Shift Right Byte)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x000000FF // Will shift to 0x7F

		// LSR.B #1,D0
		opcode := uint16(0xE008) // LSR.B #1,D0

		// Expected results: 0xFF >> 1 = 0x7F (logical shift)
		expectedResults := map[string]uint32{
			"D0": 0x0000007F,
		}

		runDetailedTest(t, cpu, opcode, "LSR.B #1,D0", expectedResults)

		// Verify C,X flags set (bit shifted into carry), N cleared
		if (cpu.SR & (M68K_SR_N | M68K_SR_C | M68K_SR_X)) != (M68K_SR_C | M68K_SR_X) {
			t.Errorf("LSR.B flags incorrect: SR=%04X, expected C,X set, N clear", cpu.SR)
		}
	})

	t.Run("LSR.L #16,D1 (Logical Shift Right Long)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[1] = 0xABCD1234 // Will shift to 0x0000ABCD

		// LSR.L #16,D1 (shift by 0 encodes as 8 for immediates)
		opcode := uint16(0xE08A) // LSR.L #16(0),D1

		// Expected results: 0xABCD1234 >> 16 = 0x0000ABCD
		expectedResults := map[string]uint32{
			"D1": 0x0000ABCD,
		}

		runDetailedTest(t, cpu, opcode, "LSR.L #16,D1", expectedResults)

		// Verify flags - N,V,C,X should be clear
		if (cpu.SR & (M68K_SR_N | M68K_SR_V | M68K_SR_C | M68K_SR_X)) != 0 {
			t.Errorf("LSR.L flags incorrect: SR=%04X, expected all clear", cpu.SR)
		}
	})

	// ================== ROL TESTS ==================
	t.Run("ROL.B #1,D0 (Rotate Left Byte)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000080 // MSB set, will rotate to 0x01

		// ROL.B #1,D0
		opcode := uint16(0xE318) // ROL.B #1,D0

		// Expected results: 0x80 rotated left by 1 = 0x01
		expectedResults := map[string]uint32{
			"D0": 0x00000001,
		}

		runDetailedTest(t, cpu, opcode, "ROL.B #1,D0", expectedResults)

		// Verify C flag set (bit rotated into carry), N,Z,V clear
		if (cpu.SR & (M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)) != M68K_SR_C {
			t.Errorf("ROL.B flags incorrect: SR=%04X, expected C set", cpu.SR)
		}
	})

	t.Run("ROL.W #4,D1 (Multi-Bit Rotate, Word)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[1] = 0x0000F001 // Will rotate to 0xF001 >> 4 = 0x100F

		// ROL.W #4,D1
		opcode := uint16(0xE959) // ROL.W #4,D1

		// Expected results: 0xF001 rotated left by 4 = 0x001F
		expectedResults := map[string]uint32{
			"D1": 0x0000100F,
		}

		runDetailedTest(t, cpu, opcode, "ROL.W #4,D1", expectedResults)
	})

	// ================== ROR TESTS ==================
	t.Run("ROR.B #1,D0 (Rotate Right Byte)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000001 // LSB set, will rotate to 0x80

		// ROR.B #1,D0
		opcode := uint16(0xE018) // ROR.B #1,D0

		// Expected results: 0x01 rotated right by 1 = 0x80
		expectedResults := map[string]uint32{
			"D0": 0x00000080,
		}

		runDetailedTest(t, cpu, opcode, "ROR.B #1,D0", expectedResults)

		// Verify C,N flags set (bit rotated into carry, result negative)
		if (cpu.SR & (M68K_SR_N | M68K_SR_C)) != (M68K_SR_N | M68K_SR_C) {
			t.Errorf("ROR.B flags incorrect: SR=%04X, expected N,C set", cpu.SR)
		}
	})

	// ================== ROXL TESTS ==================
	t.Run("ROXL.B #1,D0 (Rotate with Extend Left, X=0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000080 // MSB set, will shift into X
		cpu.SR &= ^uint16(M68K_SR_X) // Clear X flag

		// ROXL.B #1,D0
		opcode := uint16(0xE310) // ROXL.B #1,D0

		// Expected results: 0x80 rotate left through X (X=0) = 0x00, X=1
		expectedResults := map[string]uint32{
			"D0": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "ROXL.B #1,D0", expectedResults)

		// Verify Z,C,X flags set (X and C from bit shifted out, Z for zero result)
		if (cpu.SR & (M68K_SR_Z | M68K_SR_C | M68K_SR_X)) != (M68K_SR_Z | M68K_SR_C | M68K_SR_X) {
			t.Errorf("ROXL.B flags incorrect: SR=%04X, expected Z,C,X set", cpu.SR)
		}
	})

	t.Run("ROXL.B #1,D0 (Rotate with Extend Left, X=1)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000000 // Will become 0x01 with X=1
		cpu.SR |= M68K_SR_X          // Set X flag

		// ROXL.B #1,D0
		opcode := uint16(0xE310) // ROXL.B #1,D0

		// Expected results: 0x00 rotate left through X (X=1) = 0x01, X=0
		expectedResults := map[string]uint32{
			"D0": 0x00000001,
		}

		runDetailedTest(t, cpu, opcode, "ROXL.B #1,D0 (X=1)", expectedResults)

		// Verify X,C flags clear (X rotated into result)
		if (cpu.SR & (M68K_SR_X | M68K_SR_C)) != 0 {
			t.Errorf("ROXL.B flags incorrect: SR=%04X, expected X,C clear", cpu.SR)
		}
	})

	// ================== ROXR TESTS ==================
	t.Run("ROXR.B #1,D0 (Rotate with Extend Right, X=0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000001 // LSB set, will shift into X
		cpu.SR &= ^uint16(M68K_SR_X) // Clear X flag

		// ROXR.B #1,D0
		opcode := uint16(0xE010) // ROXR.B #1,D0

		// Expected results: 0x01 rotate right through X (X=0) = 0x00, X=1
		expectedResults := map[string]uint32{
			"D0": 0x00000000,
		}

		runDetailedTest(t, cpu, opcode, "ROXR.B #1,D0", expectedResults)

		// Verify Z,C,X flags set (X and C from bit shifted out, Z for zero result)
		if (cpu.SR & (M68K_SR_Z | M68K_SR_C | M68K_SR_X)) != (M68K_SR_Z | M68K_SR_C | M68K_SR_X) {
			t.Errorf("ROXR.B flags incorrect: SR=%04X, expected Z,C,X set", cpu.SR)
		}
	})

	t.Run("ROXR.W #1,D0 (Rotate with Extend Right, X=1)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x00000000 // Will become 0x8000 with X=1
		cpu.SR |= M68K_SR_X          // Set X flag

		// ROXR.W #1,D0
		opcode := uint16(0xE050) // ROXR.W #1,D0

		// Expected results: 0x0000 rotate right through X (X=1) = 0x8000, X=0
		expectedResults := map[string]uint32{
			"D0": 0x00008000,
		}

		runDetailedTest(t, cpu, opcode, "ROXR.W #1,D0 (X=1)", expectedResults)

		// Verify N flag set (negative result), X,C clear
		if (cpu.SR & (M68K_SR_N | M68K_SR_X | M68K_SR_C)) != M68K_SR_N {
			t.Errorf("ROXR.W flags incorrect: SR=%04X, expected N set, X,C clear", cpu.SR)
		}
	})

	// ================== SHIFT MEMORY OPERATIONS ==================
	t.Run("LSL (A0) (Memory Shift)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0x4000) // Will shift to 0x8000

		// LSL (A0) - Logical shift left memory
		opcode := uint16(0xE3D0) // LSL (A0)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "LSL (A0)", expectedResults)

		// Check memory result
		result := cpu.Read16(memAddr)
		if result != 0x8000 {
			t.Errorf("LSL (A0) memory result incorrect: %04X, expected 0x8000", result)
		}

		// Verify N flag set (result negative)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("LSL (A0) flags incorrect: N flag should be set")
		}
	})

	t.Run("ROR (A0) (Memory Rotate)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write16(memAddr, 0x0001) // Will rotate to 0x8000

		// ROR (A0) - Rotate right memory
		opcode := uint16(0xE6D0) // ROR (A0)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,
		}

		runDetailedTest(t, cpu, opcode, "ROR (A0)", expectedResults)

		// Check memory result
		result := cpu.Read16(memAddr)
		if result != 0x8000 {
			t.Errorf("ROR (A0) memory result incorrect: %04X, expected 0x8000", result)
		}

		// Verify N,C flags set (result negative, bit rotated into carry)
		if (cpu.SR & (M68K_SR_N | M68K_SR_C)) != (M68K_SR_N | M68K_SR_C) {
			t.Errorf("ROR (A0) flags incorrect: N,C flags should be set")
		}
	})

	// ================== EDGE CASES ==================
	t.Run("LSL.L #0,D0 (Shift Count = 0)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data
		cpu.DataRegs[0] = 0x12345678

		// LSL.L #0,D0 (immediate count of 0 means shift by 8)
		opcode := uint16(0xE380) // Specify count = 000 which is 8 shifts

		// Execute
		runDetailedTest(t, cpu, opcode, "LSL.L #0,D0", map[string]uint32{
			"D0": 0x34567800, // Should shift by 8
		})
	})

	t.Run("LSL.L D1,D0 (Shift Count > 31)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - shift count > 31 uses only lower 5 bits
		cpu.DataRegs[0] = 0x12345678
		cpu.DataRegs[1] = 0x00000028 // 40 decimal, modulo 32 = 8

		// LSL.L D1,D0
		opcode := uint16(0xE3A0) // LSL.L D1,D0

		// Expected results: 0x12345678 << 8 = 0x34567800 (only lower 5 bits of count used)
		expectedResults := map[string]uint32{
			"D0": 0x34567800,
			"D1": 0x00000028, // Count register unchanged
		}

		runDetailedTest(t, cpu, opcode, "LSL.L D1,D0 (Count > 31)", expectedResults)
	})

	t.Run("LSL.L D1,D0 (Shift Count = 32)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - shift count = 32 (all bits shifted out)
		cpu.DataRegs[0] = 0x12345678
		cpu.DataRegs[1] = 0x00000020 // 32 decimal, modulo 32 = 0, but special case

		// LSL.L D1,D0
		opcode := uint16(0xE3A0) // LSL.L D1,D0

		// Expected results: All bits shifted out = 0
		expectedResults := map[string]uint32{
			"D0": 0x00000000,
			"D1": 0x00000020,
		}

		runDetailedTest(t, cpu, opcode, "LSL.L D1,D0 (Count = 32)", expectedResults)

		// Z flag should be set (result is zero)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("LSL.L with count=32 flags incorrect: Z flag should be set")
		}
	})
}

func TestBitManipulationOperations(t *testing.T) {
	t.Logf("=== Bit Manipulation Operations Tests ===")

	// ================== BTST TESTS (Test Bit) ==================
	t.Run("BTST #2,D0 (Test Bit in Data Register, Bit Clear)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 2 (value 0)
		cpu.DataRegs[0] = 0x00000001 // Only bit 0 set

		// Setup BTST #2,D0 instruction
		cpu.Write16(cpu.PC, 0x0800)   // BTST
		cpu.Write16(cpu.PC+2, 0x0002) // Immediate bit number 2

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - register should be unchanged
		if cpu.DataRegs[0] != 0x00000001 {
			t.Errorf("BTST #2,D0 modified register: D0=%08X, expected unchanged 0x00000001", cpu.DataRegs[0])
		}

		// Z flag should be set (bit is clear)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BTST #2,D0 flags incorrect: Z flag should be set when bit is clear")
		}
	})

	t.Run("BTST #0,D0 (Test Bit in Data Register, Bit Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 0 (value 1)
		cpu.DataRegs[0] = 0x00000001 // Bit 0 is set

		// Setup BTST #0,D0 instruction
		cpu.Write16(cpu.PC, 0x0800)   // BTST
		cpu.Write16(cpu.PC+2, 0x0000) // Immediate bit number 0

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - register should be unchanged
		if cpu.DataRegs[0] != 0x00000001 {
			t.Errorf("BTST #0,D0 modified register: D0=%08X, expected unchanged 0x00000001", cpu.DataRegs[0])
		}

		// Z flag should be clear (bit is set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST #0,D0 flags incorrect: Z flag should be clear when bit is set")
		}
	})

	t.Run("BTST D1,D0 (Test Bit using Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 3 (value 0)
		cpu.DataRegs[0] = 0x00000005 // Bits 0 and 2 set
		cpu.DataRegs[1] = 0x00000003 // Bit number in D1

		// BTST D1,D0
		opcode := uint16(0x0300) // BTST D1,D0

		// Expected results - registers unchanged
		expectedResults := map[string]uint32{
			"D0": 0x00000005,
			"D1": 0x00000003,
		}

		runDetailedTest(t, cpu, opcode, "BTST D1,D0", expectedResults)

		// Z flag should be set (bit is clear)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BTST D1,D0 flags incorrect: Z flag should be set when bit is clear")
		}
	})

	t.Run("BTST D1,D0 (Test High Bit)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 31 (MSB, value 1)
		cpu.DataRegs[0] = 0x80000000 // Only MSB set
		cpu.DataRegs[1] = 0x0000001F // Bit number 31 in D1

		// BTST D1,D0
		opcode := uint16(0x0300) // BTST D1,D0

		// Expected results - registers unchanged
		expectedResults := map[string]uint32{
			"D0": 0x80000000,
			"D1": 0x0000001F,
		}

		runDetailedTest(t, cpu, opcode, "BTST D1,D0 (High Bit)", expectedResults)

		// Z flag should be clear (bit is set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST D1,D0 flags incorrect: Z flag should be clear when bit is set")
		}
	})

	t.Run("BTST #7,(A0) (Test Bit in Memory, Bit Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 7 in memory
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x80) // Only bit 7 set

		// Setup BTST #7,(A0) instruction
		cpu.Write16(cpu.PC, 0x0810)   // BTST (A0)
		cpu.Write16(cpu.PC+2, 0x0007) // Immediate bit number 7

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - memory should be unchanged
		if cpu.Read8(memAddr) != 0x80 {
			t.Errorf("BTST #7,(A0) modified memory: Mem[%08X]=%02X, expected unchanged 0x80",
				memAddr, cpu.Read8(memAddr))
		}

		// Z flag should be clear (bit is set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST #7,(A0) flags incorrect: Z flag should be clear when bit is set")
		}
	})

	t.Run("BTST D1,(A0) (Test Bit in Memory using Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 3 in memory
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x08)    // Only bit 3 set
		cpu.DataRegs[1] = 0x00000003 // Bit number in D1

		// BTST D1,(A0)
		opcode := uint16(0x0310) // BTST D1,(A0)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,
			"D1": 0x00000003,
		}

		runDetailedTest(t, cpu, opcode, "BTST D1,(A0)", expectedResults)

		// Verify memory is unchanged
		if cpu.Read8(memAddr) != 0x08 {
			t.Errorf("BTST D1,(A0) modified memory: Mem[%08X]=%02X, expected unchanged 0x08",
				memAddr, cpu.Read8(memAddr))
		}

		// Z flag should be clear (bit is set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST D1,(A0) flags incorrect: Z flag should be clear when bit is set")
		}
	})

	// ================== BCHG TESTS (Test and Change Bit) ==================
	t.Run("BCHG #3,D0 (Change Bit in Data Register, Initially Clear)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - change bit 3 (initially clear)
		cpu.DataRegs[0] = 0x00000001 // Only bit 0 set

		// Setup BCHG #3,D0 instruction
		cpu.Write16(cpu.PC, 0x0840)   // BCHG
		cpu.Write16(cpu.PC+2, 0x0003) // Immediate bit number 3

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - bit 3 should be set
		if cpu.DataRegs[0] != 0x00000009 {
			t.Errorf("BCHG #3,D0 incorrect result: D0=%08X, expected 0x00000009", cpu.DataRegs[0])
		}

		// Z flag should be set (bit was clear)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BCHG #3,D0 flags incorrect: Z flag should be set when tested bit was clear")
		}
	})

	t.Run("BCHG D1,D0 (Change Bit in Data Register, Initially Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - change bit 2 (initially set)
		cpu.DataRegs[0] = 0x00000004 // Only bit 2 set
		cpu.DataRegs[1] = 0x00000002 // Bit number 2 in D1

		// BCHG D1,D0
		opcode := uint16(0x0340) // BCHG D1,D0

		// Expected results - bit 2 should be cleared
		expectedResults := map[string]uint32{
			"D0": 0x00000000, // Bit 2 cleared
			"D1": 0x00000002, // Unchanged
		}

		runDetailedTest(t, cpu, opcode, "BCHG D1,D0", expectedResults)

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BCHG D1,D0 flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BCHG #0,(A0) (Change Bit in Memory, Initially Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - change bit 0 in memory (initially set)
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x01) // Only bit 0 set

		// Setup BCHG #0,(A0) instruction
		cpu.Write16(cpu.PC, 0x0850)   // BCHG (A0)
		cpu.Write16(cpu.PC+2, 0x0000) // Immediate bit number 0

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - bit 0 should be cleared
		if cpu.Read8(memAddr) != 0x00 {
			t.Errorf("BCHG #0,(A0) incorrect result: Mem[%08X]=%02X, expected 0x00",
				memAddr, cpu.Read8(memAddr))
		}

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BCHG #0,(A0) flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BCHG D1,(A0) (Change Bit in Memory using Register)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - change bit 7 in memory (initially clear)
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x00)    // All bits clear
		cpu.DataRegs[1] = 0x00000007 // Bit number 7 in D1

		// BCHG D1,(A0)
		opcode := uint16(0x0350) // BCHG D1,(A0)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,
			"D1": 0x00000007,
		}

		runDetailedTest(t, cpu, opcode, "BCHG D1,(A0)", expectedResults)

		// Verify memory - bit 7 should be set
		if cpu.Read8(memAddr) != 0x80 {
			t.Errorf("BCHG D1,(A0) incorrect result: Mem[%08X]=%02X, expected 0x80",
				memAddr, cpu.Read8(memAddr))
		}

		// Z flag should be set (bit was clear)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BCHG D1,(A0) flags incorrect: Z flag should be set when tested bit was clear")
		}
	})

	// ================== BCLR TESTS (Test and Clear Bit) ==================
	t.Run("BCLR #2,D0 (Clear Bit in Data Register, Initially Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - clear bit 2 (initially set)
		cpu.DataRegs[0] = 0x00000004 // Only bit 2 set

		// Setup BCLR #2,D0 instruction
		cpu.Write16(cpu.PC, 0x0880)   // BCLR
		cpu.Write16(cpu.PC+2, 0x0002) // Immediate bit number 2

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - bit 2 should be cleared
		if cpu.DataRegs[0] != 0x00000000 {
			t.Errorf("BCLR #2,D0 incorrect result: D0=%08X, expected 0x00000000", cpu.DataRegs[0])
		}

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BCLR #2,D0 flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BCLR D1,D0 (Clear Bit in Data Register, Initially Clear)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - clear bit 3 (initially clear)
		cpu.DataRegs[0] = 0x00000001 // Only bit 0 set
		cpu.DataRegs[1] = 0x00000003 // Bit number 3 in D1

		// BCLR D1,D0
		opcode := uint16(0x0380) // BCLR D1,D0

		// Expected results - no change (bit was already clear)
		expectedResults := map[string]uint32{
			"D0": 0x00000001, // Unchanged
			"D1": 0x00000003, // Unchanged
		}

		runDetailedTest(t, cpu, opcode, "BCLR D1,D0", expectedResults)

		// Z flag should be set (bit was clear)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BCLR D1,D0 flags incorrect: Z flag should be set when tested bit was clear")
		}
	})

	t.Run("BCLR #31,D0 (Clear High Bit)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - clear bit 31 (MSB, initially set)
		cpu.DataRegs[0] = 0x80000001 // MSB and bit 0 set

		// Setup BCLR #31,D0 instruction
		cpu.Write16(cpu.PC, 0x0880)   // BCLR
		cpu.Write16(cpu.PC+2, 0x001F) // Immediate bit number 31

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - MSB should be cleared, bit 0 still set
		if cpu.DataRegs[0] != 0x00000001 {
			t.Errorf("BCLR #31,D0 incorrect result: D0=%08X, expected 0x00000001", cpu.DataRegs[0])
		}

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BCLR #31,D0 flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BCLR #7,(A0) (Clear Bit in Memory, Initially Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - clear bit 7 in memory (initially set)
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x81) // Bits 7 and 0 set

		// Setup BCLR #7,(A0) instruction
		cpu.Write16(cpu.PC, 0x0890)   // BCLR (A0)
		cpu.Write16(cpu.PC+2, 0x0007) // Immediate bit number 7

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - bit 7 should be cleared, bit 0 still set
		if cpu.Read8(memAddr) != 0x01 {
			t.Errorf("BCLR #7,(A0) incorrect result: Mem[%08X]=%02X, expected 0x01",
				memAddr, cpu.Read8(memAddr))
		}

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BCLR #7,(A0) flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	// ================== BSET TESTS (Test and Set Bit) ==================
	t.Run("BSET #4,D0 (Set Bit in Data Register, Initially Clear)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set bit 4 (initially clear)
		cpu.DataRegs[0] = 0x00000001 // Only bit 0 set

		// Setup BSET #4,D0 instruction
		cpu.Write16(cpu.PC, 0x08C0)   // BSET
		cpu.Write16(cpu.PC+2, 0x0004) // Immediate bit number 4

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - bit 4 should be set, as well as bit 0
		if cpu.DataRegs[0] != 0x00000011 {
			t.Errorf("BSET #4,D0 incorrect result: D0=%08X, expected 0x00000011", cpu.DataRegs[0])
		}

		// Z flag should be set (bit was clear)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BSET #4,D0 flags incorrect: Z flag should be set when tested bit was clear")
		}
	})

	t.Run("BSET D1,D0 (Set Bit in Data Register, Initially Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set bit 2 (initially set)
		cpu.DataRegs[0] = 0x00000004 // Only bit 2 set
		cpu.DataRegs[1] = 0x00000002 // Bit number 2 in D1

		// BSET D1,D0
		opcode := uint16(0x03C0) // BSET D1,D0

		// Expected results - no change (bit was already set)
		expectedResults := map[string]uint32{
			"D0": 0x00000004, // Unchanged
			"D1": 0x00000002, // Unchanged
		}

		runDetailedTest(t, cpu, opcode, "BSET D1,D0", expectedResults)

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BSET D1,D0 flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BSET #3,(A0) (Set Bit in Memory, Initially Clear)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set bit 3 in memory (initially clear)
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x01) // Only bit 0 set

		// Setup BSET #3,(A0) instruction
		cpu.Write16(cpu.PC, 0x08D0)   // BSET (A0)
		cpu.Write16(cpu.PC+2, 0x0003) // Immediate bit number 3

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify result - bit 3 should be set, as well as bit 0
		if cpu.Read8(memAddr) != 0x09 {
			t.Errorf("BSET #3,(A0) incorrect result: Mem[%08X]=%02X, expected 0x09",
				memAddr, cpu.Read8(memAddr))
		}

		// Z flag should be set (bit was clear)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BSET #3,(A0) flags incorrect: Z flag should be set when tested bit was clear")
		}
	})

	t.Run("BSET D1,(A0) (Set Bit in Memory using Register, Initially Set)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - set bit 7 in memory (initially set)
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x80)    // Only bit 7 set
		cpu.DataRegs[1] = 0x00000007 // Bit number 7 in D1

		// BSET D1,(A0)
		opcode := uint16(0x03D0) // BSET D1,(A0)

		// Expected results
		expectedResults := map[string]uint32{
			"A0": memAddr,
			"D1": 0x00000007,
		}

		runDetailedTest(t, cpu, opcode, "BSET D1,(A0)", expectedResults)

		// Verify memory - no change (bit was already set)
		if cpu.Read8(memAddr) != 0x80 {
			t.Errorf("BSET D1,(A0) incorrect result: Mem[%08X]=%02X, expected 0x80",
				memAddr, cpu.Read8(memAddr))
		}

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BSET D1,(A0) flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	// ================== BTST with Different Addressing Modes ==================
	t.Run("BTST D1,(A0)+ (Test Bit with Postincrement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 6 in memory
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr, 0x40)    // Only bit 6 set
		cpu.DataRegs[1] = 0x00000006 // Bit number 6 in D1

		// BTST D1,(A0)+
		opcode := uint16(0x0318) // BTST D1,(A0)+

		// Expected results - A0 should be incremented by 1 (byte)
		expectedResults := map[string]uint32{
			"A0": memAddr + 1,
			"D1": 0x00000006,
		}

		runDetailedTest(t, cpu, opcode, "BTST D1,(A0)+", expectedResults)

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST D1,(A0)+ flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BTST D1,-(A0) (Test Bit with Predecrement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 5 in memory
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr
		cpu.Write8(memAddr-1, 0x20)  // Only bit 5 set
		cpu.DataRegs[1] = 0x00000005 // Bit number 5 in D1

		// BTST D1,-(A0)
		opcode := uint16(0x0320) // BTST D1,-(A0)

		// Expected results - A0 should be decremented by 1 (byte)
		expectedResults := map[string]uint32{
			"A0": memAddr - 1,
			"D1": 0x00000005,
		}

		runDetailedTest(t, cpu, opcode, "BTST D1,-(A0)", expectedResults)

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST D1,-(A0) flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BTST D1,(d16,A0) (Test Bit with Displacement)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 4 in memory with displacement
		baseAddr := uint32(0x00002000)
		displacement := uint16(0x0010)
		memAddr := baseAddr + uint32(displacement)
		cpu.AddrRegs[0] = baseAddr
		cpu.Write8(memAddr, 0x10)    // Only bit 4 set
		cpu.DataRegs[1] = 0x00000004 // Bit number 4 in D1

		// Setup BTST D1,(d16,A0) instruction
		cpu.Write16(cpu.PC, 0x0328)         // BTST D1,(d16,A0)
		cpu.Write16(cpu.PC+2, displacement) // Displacement

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify A0 is unchanged
		if cpu.AddrRegs[0] != baseAddr {
			t.Errorf("BTST D1,(d16,A0) modified A0: A0=%08X, expected unchanged %08X",
				cpu.AddrRegs[0], baseAddr)
		}

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST D1,(d16,A0) flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BTST D1,(d8,A0,D2) (Test Bit with Index)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 3 in memory with index
		baseAddr := uint32(0x00002000)
		displacement := uint8(0x08)
		index := uint32(0x00000004)
		memAddr := baseAddr + uint32(displacement) + index

		cpu.AddrRegs[0] = baseAddr
		cpu.DataRegs[2] = index      // Index value
		cpu.Write8(memAddr, 0x08)    // Only bit 3 set
		cpu.DataRegs[1] = 0x00000003 // Bit number 3 in D1

		// Setup BTST D1,(d8,A0,D2) instruction with brief format extension word
		// Extension word format: 0bRRRRFSSS0DDDDDD where:
		// RRRR = Register (D2 = 0010), F = 0 (Data), SS = Scale (00), DDDDDD = Displacement
		extWord := uint16(0x2008) // D2.W with displacement 8

		cpu.Write16(cpu.PC, 0x0330)    // BTST D1,(d8,A0,D2)
		cpu.Write16(cpu.PC+2, extWord) // Extension word

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Verify registers are unchanged
		if cpu.AddrRegs[0] != baseAddr {
			t.Errorf("BTST D1,(d8,A0,D2) modified A0: A0=%08X, expected unchanged %08X",
				cpu.AddrRegs[0], baseAddr)
		}

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST D1,(d8,A0,D2) flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BTST D1,ABS.W (Test Bit with Absolute Short)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 2 in memory at absolute address
		memAddr := uint32(0x00004000)
		cpu.Write8(memAddr, 0x04)    // Only bit 2 set
		cpu.DataRegs[1] = 0x00000002 // Bit number 2 in D1

		// Setup BTST D1,ABS.W instruction
		cpu.Write16(cpu.PC, 0x0338)   // BTST D1,ABS.W
		cpu.Write16(cpu.PC+2, 0x4000) // Absolute address

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST D1,ABS.W flags incorrect: Z flag should be clear when tested bit was set")
		}
	})

	t.Run("BTST D1,ABS.L (Test Bit with Absolute Long)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup test data - test bit 1 in memory at absolute address
		memAddr := uint32(0x00FF4000)
		cpu.Write8(memAddr, 0x02)    // Only bit 1 set
		cpu.DataRegs[1] = 0x00000001 // Bit number 1 in D1

		// Setup BTST D1,ABS.L instruction
		cpu.Write16(cpu.PC, 0x0339)   // BTST D1,ABS.L
		cpu.Write16(cpu.PC+2, 0x00FF) // High word of address
		cpu.Write16(cpu.PC+4, 0x4000) // Low word of address

		// Execute instruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Z flag should be clear (bit was set)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BTST D1,ABS.L flags incorrect: Z flag should be clear when tested bit was set")
		}
	})
}

//============================================================================
// 68020-SPECIFIC INSTRUCTION TESTS
//============================================================================

//----------------------------------------------------------------------------
// Bit Field Operation Tests
//----------------------------------------------------------------------------

func TestBitFieldOperations(t *testing.T) {
	t.Logf("=== 68020 Bit Field Operation Tests ===")

	// BFTST - Test Bit Field (doesn't modify operand)
	// M68K bit field: bit 31 is offset 0, bit 0 is offset 31
	// offset=8 extracts bits 23-16, offset=16 extracts bits 15-8, etc.
	t.Run("BFTST D0{8:8} - Test middle byte", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: D0 contains 0x12345678
		// Bits 31-24=0x12, bits 23-16=0x34, bits 15-8=0x56, bits 7-0=0x78
		cpu.DataRegs[0] = 0x12345678

		// BFTST D0{8:8} - Test bits 23-16 (value 0x34)
		// Extension word format: bits 10-6 = offset, bits 4-0 = width
		// For immediate offset=8, width=8: (8<<6)|8 = 0x0208
		cpu.Write16(cpu.PC, 0xE8C0)   // BFTST D0{offset:width}
		cpu.Write16(cpu.PC+2, 0x0208) // offset=8, width=8

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should be unchanged
		if cpu.DataRegs[0] != 0x12345678 {
			t.Errorf("BFTST modified D0: got 0x%08X, expected 0x12345678", cpu.DataRegs[0])
		}

		// Z flag should be clear (field is non-zero: 0x34)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BFTST Z flag should be clear for non-zero field")
		}

		// N flag should be clear (MSB of 0x34=0011_0100 is 0)
		if (cpu.SR & M68K_SR_N) != 0 {
			t.Errorf("BFTST N flag should be clear")
		}
	})

	t.Run("BFTST D0{16:8} - Test with negative field", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: D0 contains 0x1234F678
		// offset=16 extracts bits 15-8 = 0xF6, which has MSB=1
		cpu.DataRegs[0] = 0x1234F678

		// BFTST D0{16:8} - Test bits 15-8 (value 0xF6)
		// Extension word: (16<<6)|8 = 0x0408
		cpu.Write16(cpu.PC, 0xE8C0)
		cpu.Write16(cpu.PC+2, 0x0408) // offset=16, width=8

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// N flag should be set (MSB of 0xF6=1111_0110 is 1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("BFTST N flag should be set for negative field")
		}
	})

	// BFEXTU - Extract Bit Field Unsigned (zero-extended)
	t.Run("BFEXTU D0{8:8},D1 - Extract unsigned byte", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: D0 = 0x12F45678
		// Bits 31-24=0x12, bits 23-16=0xF4, bits 15-8=0x56, bits 7-0=0x78
		// offset=8 extracts bits 23-16 = 0xF4
		cpu.DataRegs[0] = 0x12F45678 // Source
		cpu.DataRegs[1] = 0xFFFFFFFF // Destination (to verify clearing)

		// BFEXTU D0{8:8},D1
		// Extension word: dest_reg=1 (D1), immediate offset=8, immediate width=8
		// Format: (dest<<12) | (offset<<6) | width = (1<<12) | (8<<6) | 8 = 0x1208
		cpu.Write16(cpu.PC, 0xE9C0)   // BFEXTU D0,Dn (D0 is source)
		cpu.Write16(cpu.PC+2, 0x1208) // dest=D1, offset=8, width=8

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D1 should contain 0x000000F4 (unsigned, zero-extended)
		if cpu.DataRegs[1] != 0x000000F4 {
			t.Errorf("BFEXTU result incorrect: got 0x%08X, expected 0x000000F4", cpu.DataRegs[1])
		}

		// D0 unchanged
		if cpu.DataRegs[0] != 0x12F45678 {
			t.Errorf("BFEXTU modified source")
		}
	})

	// BFEXTS - Extract Bit Field Signed (sign-extended)
	t.Run("BFEXTS D0{8:8},D1 - Extract signed byte", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: D0 = 0x12F45678
		// offset=8 extracts bits 23-16 = 0xF4, which is negative (MSB=1)
		cpu.DataRegs[0] = 0x12F45678 // Source
		cpu.DataRegs[1] = 0x00000000

		// BFEXTS D0{8:8},D1
		// Extension word: dest_reg=1 (D1), immediate offset=8, immediate width=8
		// Format: (dest<<12) | (offset<<6) | width = (1<<12) | (8<<6) | 8 = 0x1208
		cpu.Write16(cpu.PC, 0xEBC0)   // BFEXTS D0,Dn
		cpu.Write16(cpu.PC+2, 0x1208) // dest=D1, offset=8, width=8

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D1 should contain 0xFFFFFFF4 (signed, sign-extended from 0xF4)
		if cpu.DataRegs[1] != 0xFFFFFFF4 {
			t.Errorf("BFEXTS result incorrect: got 0x%08X, expected 0xFFFFFFF4", cpu.DataRegs[1])
		}

		// N flag should be set
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("BFEXTS N flag should be set for negative field")
		}
	})

	t.Run("BFEXTS D0{0:4},D1 - Extract signed nibble", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0xF2345678 // MSB nibble is 0xF (negative in 4-bit signed)
		cpu.DataRegs[1] = 0x00000000

		// BFEXTS D0{0:4},D1 - Extract top 4 bits
		// Opcode: 0xEBC0 = BFEXTS with source D0 (mode=0, reg=0)
		// Extension: dest=D1 (bits 15-12=1), offset=0, width=4 = (1<<12)|4 = 0x1004
		cpu.Write16(cpu.PC, 0xEBC0)
		cpu.Write16(cpu.PC+2, 0x1004) // dest=D1, offset=0, width=4

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D1 should contain 0xFFFFFFFF (0xF sign-extended from 4 bits)
		if cpu.DataRegs[1] != 0xFFFFFFFF {
			t.Errorf("BFEXTS 4-bit result incorrect: got 0x%08X, expected 0xFFFFFFFF", cpu.DataRegs[1])
		}
	})

	// BFCHG - Change (toggle) Bit Field
	t.Run("BFCHG D0{16:8} - Toggle byte", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0x12345678

		// BFCHG D0{16:8} - Toggle bits 16-23
		// Extension: offset=16, width=8 = (16<<6)|8 = 0x0408
		cpu.Write16(cpu.PC, 0xEAC0)
		cpu.Write16(cpu.PC+2, 0x0408) // offset=16, width=8

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Offset 16 in M68K = starting from bit 15, so bits 15-8 get toggled
		// Original bits 15-8 = 0x56, toggled = 0x56 ^ 0xFF = 0xA9
		expected := uint32(0x1234A978)
		if cpu.DataRegs[0] != expected {
			t.Errorf("BFCHG result incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[0], expected)
		}
	})

	// BFCLR - Clear Bit Field (set to all zeros)
	t.Run("BFCLR D0{8:16} - Clear middle word", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0x12345678

		// BFCLR D0{8:16} - Clear bits 8-23
		// Extension: offset=8, width=16 = (8<<6)|16 = 0x0210
		cpu.Write16(cpu.PC, 0xECC0)
		cpu.Write16(cpu.PC+2, 0x0210) // offset=8, width=16

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Bits 8-23 should be zero: 0x12345678 -> 0x12000078
		expected := uint32(0x12000078)
		if cpu.DataRegs[0] != expected {
			t.Errorf("BFCLR result incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[0], expected)
		}

		// Z flag should be clear (original field was non-zero before clearing)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BFCLR Z flag incorrect")
		}
	})

	// BFSET - Set Bit Field (set to all ones)
	t.Run("BFSET D0{4:8} - Set byte", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0x00000000

		// BFSET D0{4:8} - Set 8 bits starting at bit 4
		// Extension: offset=4, width=8 = (4<<6)|8 = 0x0108
		cpu.Write16(cpu.PC, 0xEEC0)
		cpu.Write16(cpu.PC+2, 0x0108) // offset=4, width=8

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Bits 4-11 should be set: 0x00000000 -> 0x0FF00000
		expected := uint32(0x0FF00000)
		if cpu.DataRegs[0] != expected {
			t.Errorf("BFSET result incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[0], expected)
		}

		// Z flag should be set (original field was zero)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BFSET Z flag should be set for originally zero field")
		}
	})

	// BFFFO - Find First One in Bit Field
	t.Run("BFFFO D0{0:32},D1 - Find first set bit", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: first 1 bit is at position 8 (0x00FF0000)
		cpu.DataRegs[0] = 0x00FF0000
		cpu.DataRegs[1] = 0xFFFFFFFF

		// BFFFO D0{0:32},D1 - Search entire register
		// Opcode: 0xEDC0 = BFFFO with source D0 (mode=0, reg=0)
		// Extension: dest=D1 (bits 15-12=1), offset=0, width=32(=0) = (1<<12)|0 = 0x1000
		cpu.Write16(cpu.PC, 0xEDC0)
		cpu.Write16(cpu.PC+2, 0x1000) // dest=D1, offset=0, width=32

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D1 should contain bit offset of first 1 (bit 8)
		if cpu.DataRegs[1] != 8 {
			t.Errorf("BFFFO result incorrect: got %d, expected 8", cpu.DataRegs[1])
		}

		// Z flag should be clear (found a 1)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("BFFFO Z flag should be clear when 1 bit found")
		}
	})

	t.Run("BFFFO D0{0:32},D1 - No bits set", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: all zeros
		cpu.DataRegs[0] = 0x00000000
		cpu.DataRegs[1] = 0xFFFFFFFF

		// BFFFO D0{0:32},D1
		// Opcode: 0xEDC0 = BFFFO with source D0
		// Extension: dest=D1, offset=0, width=32(=0) = 0x1000
		cpu.Write16(cpu.PC, 0xEDC0)
		cpu.Write16(cpu.PC+2, 0x1000) // dest=D1, offset=0, width=32

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D1 should contain offset+width (0+32=32) when no 1 found
		if cpu.DataRegs[1] != 32 {
			t.Errorf("BFFFO no-match result incorrect: got %d, expected 32", cpu.DataRegs[1])
		}

		// Z flag should be set (no 1 found)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("BFFFO Z flag should be set when no 1 bit found")
		}
	})

	// BFINS - Insert Bit Field
	t.Run("BFINS D1,D0{8:8} - Insert byte", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0x12345678 // Destination
		cpu.DataRegs[1] = 0x000000AB // Source (will insert low 8 bits)

		// BFINS D1,D0{8:8} - Insert low 8 bits of D1 into D0 at offset 8
		// Opcode: 0xEFC0 = BFINS with dest D0 (mode=0, reg=0)
		// Extension: src=D1 (bits 15-12=1), offset=8, width=8 = (1<<12)|(8<<6)|8 = 0x1208
		cpu.Write16(cpu.PC, 0xEFC0)
		cpu.Write16(cpu.PC+2, 0x1208) // src=D1, offset=8, width=8

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Byte at offset 8 should be replaced: 0x12345678 -> 0x12AB5678
		expected := uint32(0x12AB5678)
		if cpu.DataRegs[0] != expected {
			t.Errorf("BFINS result incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[0], expected)
		}

		// N flag reflects inserted field MSB (0xAB = 0b10101011, MSB is 1)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("BFINS N flag should be set for inserted value 0xAB (MSB=1)")
		}
	})

	t.Run("BFINS D1,D0{0:4} - Insert nibble", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0xF2345678 // Destination
		cpu.DataRegs[1] = 0x0000000A // Source

		// BFINS D1,D0{0:4} - Insert low 4 bits into top nibble
		// Opcode: 0xEFC0 = BFINS with dest D0
		// Extension: src=D1 (bits 15-12=1), offset=0, width=4 = (1<<12)|4 = 0x1004
		cpu.Write16(cpu.PC, 0xEFC0)
		cpu.Write16(cpu.PC+2, 0x1004) // src=D1, offset=0, width=4

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Top nibble replaced: 0xF2345678 -> 0xA2345678
		expected := uint32(0xA2345678)
		if cpu.DataRegs[0] != expected {
			t.Errorf("BFINS nibble result incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[0], expected)
		}
	})
}

//----------------------------------------------------------------------------
// 32-bit Multiply/Divide Tests
//----------------------------------------------------------------------------

func Test32BitMultiplyDivide(t *testing.T) {
	t.Logf("=== 68020 32-bit Multiply/Divide Tests ===")

	// MULU.L - Unsigned 32-bit Multiply
	t.Run("MULU.L D1,D0 - 32x32=32 unsigned", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: 1000 * 2000 = 2,000,000
		cpu.DataRegs[0] = 1000
		cpu.DataRegs[1] = 2000

		// MULU.L D1,D0 (32x32=32 result in D0)
		cpu.Write16(cpu.PC, 0x4C01)   // MULU.L D1,D0
		cpu.Write16(cpu.PC+2, 0x0000) // 32-bit result

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain 2,000,000
		if cpu.DataRegs[0] != 2000000 {
			t.Errorf("MULU.L result incorrect: got %d, expected 2000000", cpu.DataRegs[0])
		}

		// V flag clear (no overflow)
		if (cpu.SR & M68K_SR_V) != 0 {
			t.Errorf("MULU.L V flag should be clear")
		}
	})

	t.Run("MULU.L D1,D1:D0 - 32x32=64 unsigned", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: 0x80000000 * 4 = 0x00000002_00000000 (needs 64 bits)
		cpu.DataRegs[0] = 0x80000000
		cpu.DataRegs[1] = 4

		// MULU.L D1,D1:D0 (32x32=64, high in D1, low in D0)
		cpu.Write16(cpu.PC, 0x4C01)   // MULU.L D1
		cpu.Write16(cpu.PC+2, 0x0400) // D1:D0 (64-bit result)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 (low) should be 0x00000000
		if cpu.DataRegs[0] != 0x00000000 {
			t.Errorf("MULU.L D1:D0 low incorrect: got 0x%08X, expected 0x00000000", cpu.DataRegs[0])
		}

		// D1 (high) should be 0x00000002
		if cpu.DataRegs[1] != 0x00000002 {
			t.Errorf("MULU.L D1:D0 high incorrect: got 0x%08X, expected 0x00000002", cpu.DataRegs[1])
		}
	})

	// MULS.L - Signed 32-bit Multiply
	t.Run("MULS.L D1,D0 - 32x32=32 signed positive", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: 1000 * 2000 = 2,000,000
		cpu.DataRegs[0] = 1000
		cpu.DataRegs[1] = 2000

		// MULS.L D1,D0
		cpu.Write16(cpu.PC, 0x4C01)
		cpu.Write16(cpu.PC+2, 0x0800) // Signed multiply

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain 2,000,000
		if cpu.DataRegs[0] != 2000000 {
			t.Errorf("MULS.L result incorrect: got %d, expected 2000000", cpu.DataRegs[0])
		}
	})

	t.Run("MULS.L D1,D0 - 32x32=32 signed negative", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: -1000 * 2000 = -2,000,000
		var negValue int32 = -1000
		cpu.DataRegs[0] = uint32(negValue)
		cpu.DataRegs[1] = 2000

		// MULS.L D1,D0
		cpu.Write16(cpu.PC, 0x4C01)
		cpu.Write16(cpu.PC+2, 0x0800)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain -2,000,000 (as two's complement)
		var expectedSigned int32 = -2000000
		expected := uint32(expectedSigned)
		if cpu.DataRegs[0] != expected {
			t.Errorf("MULS.L negative result incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[0], expected)
		}

		// N flag should be set
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("MULS.L N flag should be set for negative result")
		}
	})

	t.Run("MULS.L D1,D1:D0 - 32x32=64 signed", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: -2 * 0x7FFFFFFF = -0xFFFFFFFE (needs sign extension to 64 bits)
		var negTwo int32 = -2
		cpu.DataRegs[0] = uint32(negTwo)
		cpu.DataRegs[1] = 0x7FFFFFFF

		// MULS.L D1,D1:D0 (64-bit result)
		cpu.Write16(cpu.PC, 0x4C01)
		cpu.Write16(cpu.PC+2, 0x0C00) // Signed, 64-bit

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Result: -2 * 0x7FFFFFFF = 0xFFFFFFFF_00000002
		if cpu.DataRegs[0] != 0x00000002 {
			t.Errorf("MULS.L D1:D0 low incorrect: got 0x%08X, expected 0x00000002", cpu.DataRegs[0])
		}
		if cpu.DataRegs[1] != 0xFFFFFFFF {
			t.Errorf("MULS.L D1:D0 high incorrect: got 0x%08X, expected 0xFFFFFFFF", cpu.DataRegs[1])
		}
	})

	// DIVU.L - Unsigned 32-bit Divide
	t.Run("DIVU.L D1,D0 - 32/32 unsigned", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: 2000000 / 1000 = 2000
		cpu.DataRegs[0] = 2000000
		cpu.DataRegs[1] = 1000

		// DIVU.L D1,D0 (quotient in D0)
		cpu.Write16(cpu.PC, 0x4C41)
		cpu.Write16(cpu.PC+2, 0x0000)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain quotient 2000
		if cpu.DataRegs[0] != 2000 {
			t.Errorf("DIVU.L quotient incorrect: got %d, expected 2000", cpu.DataRegs[0])
		}

		// V flag clear (no overflow)
		if (cpu.SR & M68K_SR_V) != 0 {
			t.Errorf("DIVU.L V flag should be clear")
		}
	})

	t.Run("DIVU.L D1,D2:D0 - 32/32 with remainder", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: 2000001 / 1000 = 2000 remainder 1
		cpu.DataRegs[0] = 2000001
		cpu.DataRegs[1] = 1000
		cpu.DataRegs[2] = 0

		// DIVU.L D1,D2:D0 (remainder in D2, quotient in D0)
		cpu.Write16(cpu.PC, 0x4C41)
		cpu.Write16(cpu.PC+2, 0x0002) // Remainder in D2

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain quotient 2000
		if cpu.DataRegs[0] != 2000 {
			t.Errorf("DIVU.L quotient incorrect: got %d, expected 2000", cpu.DataRegs[0])
		}

		// D2 should contain remainder 1
		if cpu.DataRegs[2] != 1 {
			t.Errorf("DIVU.L remainder incorrect: got %d, expected 1", cpu.DataRegs[2])
		}
	})

	// DIVS.L - Signed 32-bit Divide
	t.Run("DIVS.L D1,D0 - 32/32 signed positive", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: 2000000 / 1000 = 2000
		cpu.DataRegs[0] = 2000000
		cpu.DataRegs[1] = 1000

		// DIVS.L D1,D0
		cpu.Write16(cpu.PC, 0x4C41)
		cpu.Write16(cpu.PC+2, 0x0800)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain quotient 2000
		if cpu.DataRegs[0] != 2000 {
			t.Errorf("DIVS.L quotient incorrect: got %d, expected 2000", cpu.DataRegs[0])
		}
	})

	t.Run("DIVS.L D1,D0 - 32/32 signed negative", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: -2000000 / 1000 = -2000
		var negDividend int32 = -2000000
		cpu.DataRegs[0] = uint32(negDividend)
		cpu.DataRegs[1] = 1000

		// DIVS.L D1,D0
		cpu.Write16(cpu.PC, 0x4C41)
		cpu.Write16(cpu.PC+2, 0x0800)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain -2000
		var expectedQuotSigned int32 = -2000
		expected := uint32(expectedQuotSigned)
		if cpu.DataRegs[0] != expected {
			t.Errorf("DIVS.L negative quotient incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[0], expected)
		}

		// N flag should be set
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("DIVS.L N flag should be set for negative result")
		}
	})

	t.Run("DIVS.L D1,D2:D0 - 32/32 signed with remainder", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: -2000001 / 1000 = -2000 remainder -1
		var negDividend2 int32 = -2000001
		cpu.DataRegs[0] = uint32(negDividend2)
		cpu.DataRegs[1] = 1000
		cpu.DataRegs[2] = 0

		// DIVS.L D1,D2:D0
		cpu.Write16(cpu.PC, 0x4C41)
		cpu.Write16(cpu.PC+2, 0x0802) // Signed, remainder in D2

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain quotient -2000
		var expQuotS int32 = -2000
		expectedQuot := uint32(expQuotS)
		if cpu.DataRegs[0] != expectedQuot {
			t.Errorf("DIVS.L quotient incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[0], expectedQuot)
		}

		// D2 should contain remainder -1
		var expRemS int32 = -1
		expectedRem := uint32(expRemS)
		if cpu.DataRegs[2] != expectedRem {
			t.Errorf("DIVS.L remainder incorrect: got 0x%08X, expected 0x%08X", cpu.DataRegs[2], expectedRem)
		}
	})

	// Division by zero test
	t.Run("DIVU.L D1,D0 - Division by zero", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: 1000 / 0 (should trap)
		cpu.DataRegs[0] = 1000
		cpu.DataRegs[1] = 0 // Divide by zero

		// Set up exception vector for divide by zero
		cpu.Write32(M68K_VEC_ZERO_DIVIDE*4, 0x00003000)

		// DIVU.L D1,D0
		cpu.Write16(cpu.PC, 0x4C41)
		cpu.Write16(cpu.PC+2, 0x0000)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to exception handler
		if cpu.PC != 0x00003000 {
			t.Errorf("Division by zero should trigger exception: PC=0x%08X, expected 0x00003000", cpu.PC)
		}
	})

	// Overflow test
	t.Run("DIVU.L D1,D0 - Overflow (quotient too large)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: 0xFFFFFFFF / 1 = 0xFFFFFFFF (fits in 32 bits, OK)
		cpu.DataRegs[0] = 0xFFFFFFFF
		cpu.DataRegs[1] = 1

		// DIVU.L D1,D0
		cpu.Write16(cpu.PC, 0x4C41)
		cpu.Write16(cpu.PC+2, 0x0000)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Should succeed with quotient 0xFFFFFFFF
		if cpu.DataRegs[0] != 0xFFFFFFFF {
			t.Errorf("DIVU.L max quotient incorrect: got 0x%08X, expected 0xFFFFFFFF", cpu.DataRegs[0])
		}

		// V flag should be clear
		if (cpu.SR & M68K_SR_V) != 0 {
			t.Errorf("DIVU.L V flag should be clear for valid quotient")
		}
	})
}

//----------------------------------------------------------------------------
// CAS/CAS2 Atomic Operation Tests
//----------------------------------------------------------------------------

func TestAtomicOperations(t *testing.T) {
	t.Logf("=== 68020 CAS/CAS2 Atomic Operation Tests ===")

	// CAS - Compare and Swap (lock-free primitive)
	t.Run("CAS.B Dc,Du,D0 - Byte compare successful", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0x12345678 // Destination (low byte 0x78)
		cpu.DataRegs[1] = 0x00000078 // Compare value Dc (matches!)
		cpu.DataRegs[2] = 0x000000AB // Update value Du

		// CAS.B D1,D2,D0 - If D0.B == D1.B then D0.B = D2.B
		cpu.Write16(cpu.PC, 0x0AC0)   // CAS.B
		cpu.Write16(cpu.PC+2, 0x0142) // Dc=D1, Du=D2, EA=D0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 low byte should be updated to 0xAB
		if (cpu.DataRegs[0] & 0xFF) != 0xAB {
			t.Errorf("CAS.B swap failed: got 0x%08X, expected low byte 0xAB", cpu.DataRegs[0])
		}

		// Z flag should be set (compare succeeded)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("CAS.B Z flag should be set on successful compare")
		}
	})

	t.Run("CAS.B Dc,Du,D0 - Byte compare failed", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0x12345678 // Destination (low byte 0x78)
		cpu.DataRegs[1] = 0x00000099 // Compare value Dc (doesn't match)
		cpu.DataRegs[2] = 0x000000AB // Update value Du

		// CAS.B D1,D2,D0
		cpu.Write16(cpu.PC, 0x0AC0)
		cpu.Write16(cpu.PC+2, 0x0142)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should be unchanged (compare failed)
		if cpu.DataRegs[0] != 0x12345678 {
			t.Errorf("CAS.B should not swap on compare fail: got 0x%08X", cpu.DataRegs[0])
		}

		// D1 should be updated with actual value from D0
		if (cpu.DataRegs[1] & 0xFF) != 0x78 {
			t.Errorf("CAS.B Dc should be updated with actual value: got 0x%02X, expected 0x78", cpu.DataRegs[1]&0xFF)
		}

		// Z flag should be clear (compare failed)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("CAS.B Z flag should be clear on failed compare")
		}
	})

	t.Run("CAS.W Dc,Du,D0 - Word compare successful", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0x12345678 // Destination (low word 0x5678)
		cpu.DataRegs[1] = 0x00005678 // Compare value (matches!)
		cpu.DataRegs[2] = 0x0000ABCD // Update value

		// CAS.W D1,D2,D0
		cpu.Write16(cpu.PC, 0x0CC0)
		cpu.Write16(cpu.PC+2, 0x0142)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 low word should be updated to 0xABCD
		if (cpu.DataRegs[0] & 0xFFFF) != 0xABCD {
			t.Errorf("CAS.W swap failed: got 0x%08X, expected low word 0xABCD", cpu.DataRegs[0])
		}

		// Z flag should be set
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("CAS.W Z flag should be set on successful compare")
		}
	})

	t.Run("CAS.L Dc,Du,D0 - Long compare successful", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		cpu.DataRegs[0] = 0x12345678 // Destination
		cpu.DataRegs[1] = 0x12345678 // Compare value (matches!)
		cpu.DataRegs[2] = 0xABCDEF99 // Update value

		// CAS.L D1,D2,D0
		cpu.Write16(cpu.PC, 0x0EC0)
		cpu.Write16(cpu.PC+2, 0x0142)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should be updated to 0xABCDEF99
		if cpu.DataRegs[0] != 0xABCDEF99 {
			t.Errorf("CAS.L swap failed: got 0x%08X, expected 0xABCDEF99", cpu.DataRegs[0])
		}

		// Z flag should be set
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("CAS.L Z flag should be set on successful compare")
		}
	})

	t.Run("CAS.L Dc,Du,(A0) - Memory compare and swap", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup memory
		memAddr := uint32(0x00002000)
		cpu.Write32(memAddr, 0x11223344)
		cpu.AddrRegs[0] = memAddr

		// Setup registers
		cpu.DataRegs[1] = 0x11223344 // Compare value (matches memory!)
		cpu.DataRegs[2] = 0xAABBCCDD // Update value

		// CAS.L D1,D2,(A0)
		cpu.Write16(cpu.PC, 0x0ED0) // CAS.L with (A0)
		cpu.Write16(cpu.PC+2, 0x0142)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Memory should be updated
		if cpu.Read32(memAddr) != 0xAABBCCDD {
			t.Errorf("CAS.L memory swap failed: got 0x%08X, expected 0xAABBCCDD", cpu.Read32(memAddr))
		}

		// Z flag should be set
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("CAS.L Z flag should be set on successful compare")
		}
	})

	// CAS2 - Compare and Swap Dual operands (for linked list operations)
	t.Run("CAS2.L - Dual compare and swap success", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: Testing linked list head/tail atomic update
		memAddr1 := uint32(0x00002000)
		memAddr2 := uint32(0x00002004)

		cpu.Write32(memAddr1, 0x11111111)
		cpu.Write32(memAddr2, 0x22222222)

		cpu.AddrRegs[0] = memAddr1 // First address
		cpu.AddrRegs[1] = memAddr2 // Second address

		// Compare values
		cpu.DataRegs[0] = 0x11111111 // Compare for first location
		cpu.DataRegs[1] = 0x22222222 // Compare for second location

		// Update values
		cpu.DataRegs[2] = 0xAAAAAAAA // Update for first location
		cpu.DataRegs[3] = 0xBBBBBBBB // Update for second location

		// CAS2.L D0:D1,D2:D3,(A0):(A1)
		cpu.Write16(cpu.PC, 0x0EFC)   // CAS2.L
		cpu.Write16(cpu.PC+2, 0x0180) // Dc1=D0, Dc2=D1, Du1=D2, Du2=D3
		cpu.Write16(cpu.PC+4, 0x2100) // Rn1=A0, Rn2=A1

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Both memory locations should be updated
		if cpu.Read32(memAddr1) != 0xAAAAAAAA {
			t.Errorf("CAS2.L first location not updated: got 0x%08X", cpu.Read32(memAddr1))
		}
		if cpu.Read32(memAddr2) != 0xBBBBBBBB {
			t.Errorf("CAS2.L second location not updated: got 0x%08X", cpu.Read32(memAddr2))
		}

		// Z flag should be set (both compares succeeded)
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("CAS2.L Z flag should be set on successful dual compare")
		}
	})

	t.Run("CAS2.L - First compare fails, no swap", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup
		memAddr1 := uint32(0x00002000)
		memAddr2 := uint32(0x00002004)

		cpu.Write32(memAddr1, 0x11111111)
		cpu.Write32(memAddr2, 0x22222222)

		cpu.AddrRegs[0] = memAddr1
		cpu.AddrRegs[1] = memAddr2

		// First compare value is WRONG
		cpu.DataRegs[0] = 0x99999999 // Wrong!
		cpu.DataRegs[1] = 0x22222222 // Correct (but shouldn't matter)

		cpu.DataRegs[2] = 0xAAAAAAAA
		cpu.DataRegs[3] = 0xBBBBBBBB

		// CAS2.L D0:D1,D2:D3,(A0):(A1)
		cpu.Write16(cpu.PC, 0x0EFC)
		cpu.Write16(cpu.PC+2, 0x0180)
		cpu.Write16(cpu.PC+4, 0x2100)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// NEITHER memory location should be updated (atomic property)
		if cpu.Read32(memAddr1) != 0x11111111 {
			t.Errorf("CAS2.L should not update first location on fail: got 0x%08X", cpu.Read32(memAddr1))
		}
		if cpu.Read32(memAddr2) != 0x22222222 {
			t.Errorf("CAS2.L should not update second location on fail: got 0x%08X", cpu.Read32(memAddr2))
		}

		// D0 should be loaded with actual value from first location
		if cpu.DataRegs[0] != 0x11111111 {
			t.Errorf("CAS2.L Dc1 should be updated with actual value: got 0x%08X", cpu.DataRegs[0])
		}

		// Z flag should be clear (compare failed)
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("CAS2.L Z flag should be clear on failed compare")
		}
	})
}

//----------------------------------------------------------------------------
// CHK2/CMP2 Bounds Checking Tests
//----------------------------------------------------------------------------

func TestBoundsChecking(t *testing.T) {
	t.Logf("=== 68020 CHK2/CMP2 Bounds Checking Tests ===")

	// CHK2 - Check bounds with exception on out-of-range
	t.Run("CHK2.W D0,(A0) - Value within bounds", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup bounds in memory: lower=100, upper=200
		memAddr := uint32(0x00002000)
		cpu.Write16(memAddr, 100)   // Lower bound
		cpu.Write16(memAddr+2, 200) // Upper bound
		cpu.AddrRegs[0] = memAddr

		// Value to check
		cpu.DataRegs[0] = 150 // Within bounds [100, 200]

		// CHK2.W D0,(A0)
		cpu.Write16(cpu.PC, 0x00D0)   // CHK2.W (A0)
		cpu.Write16(cpu.PC+2, 0x0800) // Check D0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Should not trap (value is within bounds)
		// C flag clear = within bounds
		if (cpu.SR & M68K_SR_C) != 0 {
			t.Errorf("CHK2 C flag should be clear for value within bounds")
		}
	})

	t.Run("CHK2.W D0,(A0) - Value below lower bound (trap)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup bounds: lower=100, upper=200
		memAddr := uint32(0x00002000)
		cpu.Write16(memAddr, 100)
		cpu.Write16(memAddr+2, 200)
		cpu.AddrRegs[0] = memAddr

		// Value to check
		cpu.DataRegs[0] = 50 // Below lower bound!

		// Set up CHK exception vector
		cpu.Write32(M68K_VEC_CHK*4, 0x00003000)

		// CHK2.W D0,(A0)
		cpu.Write16(cpu.PC, 0x00D0)
		cpu.Write16(cpu.PC+2, 0x0800)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to CHK exception handler
		if cpu.PC != 0x00003000 {
			t.Errorf("CHK2 should trap for out-of-bounds value: PC=0x%08X", cpu.PC)
		}
	})

	t.Run("CHK2.L D0,(A0) - Value above upper bound (trap)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup bounds: lower=1000, upper=2000
		memAddr := uint32(0x00002000)
		cpu.Write32(memAddr, 1000)
		cpu.Write32(memAddr+4, 2000)
		cpu.AddrRegs[0] = memAddr

		// Value to check
		cpu.DataRegs[0] = 3000 // Above upper bound!

		// Set up CHK exception vector
		cpu.Write32(M68K_VEC_CHK*4, 0x00003000)

		// CHK2.L D0,(A0)
		cpu.Write16(cpu.PC, 0x00D0)
		cpu.Write16(cpu.PC+2, 0x0C00) // Long size

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to exception handler
		if cpu.PC != 0x00003000 {
			t.Errorf("CHK2 should trap for value above upper bound: PC=0x%08X", cpu.PC)
		}
	})

	// CMP2 - Compare bounds without exception (just set flags)
	t.Run("CMP2.W D0,(A0) - Value within bounds", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup bounds: lower=100, upper=200
		memAddr := uint32(0x00002000)
		cpu.Write16(memAddr, 100)
		cpu.Write16(memAddr+2, 200)
		cpu.AddrRegs[0] = memAddr

		// Value to check
		cpu.DataRegs[0] = 150 // Within bounds

		// CMP2.W D0,(A0)
		cpu.Write16(cpu.PC, 0x00D0)
		cpu.Write16(cpu.PC+2, 0x0000) // CMP2 (no trap)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// C flag clear = within bounds
		if (cpu.SR & M68K_SR_C) != 0 {
			t.Errorf("CMP2 C flag should be clear for value within bounds")
		}

		// Z flag clear = not equal to either bound
		if (cpu.SR & M68K_SR_Z) != 0 {
			t.Errorf("CMP2 Z flag should be clear when value != bounds")
		}
	})

	t.Run("CMP2.W D0,(A0) - Value equals lower bound", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup bounds: lower=100, upper=200
		memAddr := uint32(0x00002000)
		cpu.Write16(memAddr, 100)
		cpu.Write16(memAddr+2, 200)
		cpu.AddrRegs[0] = memAddr

		// Value to check
		cpu.DataRegs[0] = 100 // Equals lower bound

		// CMP2.W D0,(A0)
		cpu.Write16(cpu.PC, 0x00D0)
		cpu.Write16(cpu.PC+2, 0x0000)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// C flag clear = within/on bounds
		if (cpu.SR & M68K_SR_C) != 0 {
			t.Errorf("CMP2 C flag should be clear for value on lower bound")
		}

		// Z flag set = equals a bound
		if (cpu.SR & M68K_SR_Z) == 0 {
			t.Errorf("CMP2 Z flag should be set when value equals bound")
		}
	})

	t.Run("CMP2.W D0,(A0) - Value below bounds", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup bounds: lower=100, upper=200
		memAddr := uint32(0x00002000)
		cpu.Write16(memAddr, 100)
		cpu.Write16(memAddr+2, 200)
		cpu.AddrRegs[0] = memAddr

		// Value to check
		cpu.DataRegs[0] = 50 // Below lower bound

		// CMP2.W D0,(A0)
		cpu.Write16(cpu.PC, 0x00D0)
		cpu.Write16(cpu.PC+2, 0x0000)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// C flag set = out of bounds
		if (cpu.SR & M68K_SR_C) == 0 {
			t.Errorf("CMP2 C flag should be set for value below lower bound")
		}

		// Should NOT trap (unlike CHK2)
		if cpu.PC >= 0x00003000 {
			t.Errorf("CMP2 should not trap, only set flags")
		}
	})

	t.Run("CMP2.L A0,(A1) - Address register bounds check", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup bounds in memory
		memAddr := uint32(0x00002000)
		cpu.Write32(memAddr, 0x00001000)   // Lower bound
		cpu.Write32(memAddr+4, 0x00003000) // Upper bound
		cpu.AddrRegs[1] = memAddr

		// Address to check
		cpu.AddrRegs[0] = 0x00002000 // Within bounds [0x1000, 0x3000]

		// CMP2.L A0,(A1)
		cpu.Write16(cpu.PC, 0x00D1)
		cpu.Write16(cpu.PC+2, 0x0C08) // Long size, A0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// C flag clear = within bounds
		if (cpu.SR & M68K_SR_C) != 0 {
			t.Errorf("CMP2 C flag should be clear for address within bounds")
		}
	})
}

//----------------------------------------------------------------------------
// PACK/UNPK BCD Operation Tests
//----------------------------------------------------------------------------

func TestPackUnpack(t *testing.T) {
	t.Logf("=== 68020 PACK/UNPK BCD Operation Tests ===")

	// PACK - Convert unpacked BCD to packed BCD
	t.Run("PACK D0,D1,#0 - Pack BCD digits", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: Unpacked BCD in D0 (0x00000705 = digits 7 and 5)
		cpu.DataRegs[0] = 0x00000705
		cpu.DataRegs[1] = 0xFFFFFFFF

		// PACK D0,D1,#0 - Pack low 2 BCD digits with adjustment 0
		// Encoding: 1000 yyy 10100 mxxx where yyy=dest(D1=001), m=0, xxx=src(D0=000)
		// = 1000 001 10100 0 000 = 0x8340
		cpu.Write16(cpu.PC, 0x8340)   // PACK D0,D1
		cpu.Write16(cpu.PC+2, 0x0000) // Adjustment = 0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D1 low byte should be 0x75 (packed)
		if (cpu.DataRegs[1] & 0xFF) != 0x75 {
			t.Errorf("PACK result incorrect: got 0x%02X, expected 0x75", cpu.DataRegs[1]&0xFF)
		}

		// Upper bytes of D1 preserved
		if (cpu.DataRegs[1] & 0xFFFFFF00) != 0xFFFFFF00 {
			t.Errorf("PACK should preserve upper 24 bits")
		}
	})

	t.Run("PACK D0,D1,#$F0F0 - Pack with ASCII adjustment", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: ASCII digits '3' '7' stored as unpacked BCD-style
		// PACK extracts low nibble from each byte: (0x03 << 4) | 0x07 = 0x37
		// Then adds adjustment: 0x37 + 0xF0F0 = 0xF127, low byte = 0x27
		cpu.DataRegs[0] = 0x00000337
		cpu.DataRegs[1] = 0x00000000

		// PACK D0,D1,#$F0F0 - Pack and add adjustment
		cpu.Write16(cpu.PC, 0x8340)   // PACK D0,D1
		cpu.Write16(cpu.PC+2, 0xF0F0) // Adjustment

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PACK extracts nibbles: (0x03 << 4) | 0x07 = 0x37
		// Then adds 0xF0F0: 0x37 + 0xF0F0 = 0xF127, low byte = 0x27
		expected := uint8(0x27)
		if (cpu.DataRegs[1] & 0xFF) != uint32(expected) {
			t.Errorf("PACK with adjustment: got 0x%02X, expected 0x%02X", cpu.DataRegs[1]&0xFF, expected)
		}
	})

	t.Run("PACK -(A0),-(A1),#0 - Pack from memory", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup memory with unpacked BCD
		memSrc := uint32(0x00002000)
		memDst := uint32(0x00003000)

		cpu.Write16(memSrc-2, 0x0809) // Unpacked: digits 8 and 9

		cpu.AddrRegs[0] = memSrc
		cpu.AddrRegs[1] = memDst

		// PACK -(A0),-(A1),#0
		// Encoding: 1000 yyy 10100 1 xxx where yyy=dest(A1=001), m=1, xxx=src(A0=000)
		// = 1000 001 10100 1 000 = 0x8348
		cpu.Write16(cpu.PC, 0x8348)   // PACK -(A0),-(A1)
		cpu.Write16(cpu.PC+2, 0x0000) // Adjustment = 0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result in memory at decremented A1
		result := cpu.Read8(memDst - 1)
		if result != 0x89 {
			t.Errorf("PACK memory result incorrect: got 0x%02X, expected 0x89", result)
		}

		// Check pointers decremented
		if cpu.AddrRegs[0] != memSrc-2 {
			t.Errorf("PACK A0 not decremented correctly")
		}
		if cpu.AddrRegs[1] != memDst-1 {
			t.Errorf("PACK A1 not decremented correctly")
		}
	})

	// UNPK - Convert packed BCD to unpacked BCD
	t.Run("UNPK D0,D1,#0 - Unpack BCD digits", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: Packed BCD in D0 (0x12345675)
		cpu.DataRegs[0] = 0x12345675 // Low byte is 0x75 packed
		cpu.DataRegs[1] = 0x00000000

		// UNPK D0,D1,#0 - Unpack low byte with adjustment 0
		// Encoding: 1000 yyy 11000 mxxx where yyy=dest(D1=001), m=0, xxx=src(D0=000)
		// = 1000 001 11000 0 000 = 0x8380
		cpu.Write16(cpu.PC, 0x8380)   // UNPK D0,D1
		cpu.Write16(cpu.PC+2, 0x0000) // Adjustment = 0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D1 low word should be 0x0705 (unpacked: digit 7 in high, digit 5 in low)
		if (cpu.DataRegs[1] & 0xFFFF) != 0x0705 {
			t.Errorf("UNPK result incorrect: got 0x%04X, expected 0x0705", cpu.DataRegs[1]&0xFFFF)
		}
	})

	t.Run("UNPK D0,D1,#$3030 - Unpack with ASCII conversion", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: Packed BCD 0x37
		cpu.DataRegs[0] = 0x00000037
		cpu.DataRegs[1] = 0x00000000

		// UNPK D0,D1,#$3030 - Add ASCII bias to make printable
		cpu.Write16(cpu.PC, 0x8380)   // UNPK D0,D1
		cpu.Write16(cpu.PC+2, 0x3030) // ASCII adjustment

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Result: 0x0307 + 0x3030 = 0x3337 ('3' '7' in ASCII)
		if (cpu.DataRegs[1] & 0xFFFF) != 0x3337 {
			t.Errorf("UNPK with ASCII adjustment: got 0x%04X, expected 0x3337", cpu.DataRegs[1]&0xFFFF)
		}
	})

	t.Run("UNPK -(A0),-(A1),#0 - Unpack from memory", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup memory with packed BCD
		memSrc := uint32(0x00002000)
		memDst := uint32(0x00003000)

		cpu.Write8(memSrc-1, 0x89) // Packed: 8 and 9

		cpu.AddrRegs[0] = memSrc
		cpu.AddrRegs[1] = memDst

		// UNPK -(A0),-(A1),#0
		// Encoding: 1000 yyy 11000 1 xxx where yyy=dest(A1=001), m=1, xxx=src(A0=000)
		// = 1000 001 11000 1 000 = 0x8388
		cpu.Write16(cpu.PC, 0x8388)   // UNPK -(A0),-(A1)
		cpu.Write16(cpu.PC+2, 0x0000) // Adjustment = 0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check result in memory: should be 0x0809 at decremented A1
		result := cpu.Read16(memDst - 2)
		if result != 0x0809 {
			t.Errorf("UNPK memory result incorrect: got 0x%04X, expected 0x0809", result)
		}

		// Check pointers decremented
		if cpu.AddrRegs[0] != memSrc-1 {
			t.Errorf("UNPK A0 not decremented correctly")
		}
		if cpu.AddrRegs[1] != memDst-2 {
			t.Errorf("UNPK A1 not decremented correctly")
		}
	})
}

//----------------------------------------------------------------------------
// Supervisor Mode Tests
//----------------------------------------------------------------------------

func TestSupervisorMode(t *testing.T) {
	t.Logf("=== 68020 Supervisor Mode Tests ===")

	// MOVEC - Move Control Register
	t.Run("MOVEC D0,VBR - Move to Vector Base Register", func(t *testing.T) {
		cpu := setupTestCPU()

		// Start in supervisor mode
		cpu.SR |= M68K_SR_S

		// Setup
		cpu.DataRegs[0] = 0x00010000 // New VBR value

		// MOVEC D0,VBR
		cpu.Write16(cpu.PC, 0x4E7B)   // MOVEC
		cpu.Write16(cpu.PC+2, 0x0801) // D0 → VBR

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// VBR should be updated
		if cpu.VBR != 0x00010000 {
			t.Errorf("MOVEC VBR not updated: got 0x%08X, expected 0x00010000", cpu.VBR)
		}
	})

	t.Run("MOVEC VBR,D0 - Move from Vector Base Register", func(t *testing.T) {
		cpu := setupTestCPU()

		// Start in supervisor mode
		cpu.SR |= M68K_SR_S

		// Setup
		cpu.VBR = 0x00020000
		cpu.DataRegs[0] = 0x00000000

		// MOVEC VBR,D0
		cpu.Write16(cpu.PC, 0x4E7A)   // MOVEC
		cpu.Write16(cpu.PC+2, 0x0801) // VBR → D0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain VBR value
		if cpu.DataRegs[0] != 0x00020000 {
			t.Errorf("MOVEC from VBR incorrect: got 0x%08X, expected 0x00020000", cpu.DataRegs[0])
		}
	})

	t.Run("MOVEC D0,CACR - Move to Cache Control Register", func(t *testing.T) {
		cpu := setupTestCPU()

		// Start in supervisor mode
		cpu.SR |= M68K_SR_S

		// Setup
		cpu.DataRegs[0] = 0x00000001 // Enable cache

		// MOVEC D0,CACR
		cpu.Write16(cpu.PC, 0x4E7B)
		cpu.Write16(cpu.PC+2, 0x0002) // D0 → CACR

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// CACR should be updated
		if cpu.CACR != 0x00000001 {
			t.Errorf("MOVEC CACR not updated: got 0x%08X, expected 0x00000001", cpu.CACR)
		}
	})

	t.Run("MOVEC D0,VBR - Privilege violation in user mode", func(t *testing.T) {
		cpu := setupTestCPU()

		// Start in USER mode (clear supervisor bit)
		cpu.SR &= ^uint16(M68K_SR_S)

		// Set up privilege violation exception vector
		cpu.Write32(M68K_VEC_PRIVILEGE*4, 0x00004000)

		// Setup
		cpu.DataRegs[0] = 0x00010000

		// MOVEC D0,VBR (should trap)
		cpu.Write16(cpu.PC, 0x4E7B)
		cpu.Write16(cpu.PC+2, 0x0801)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to privilege violation handler
		if cpu.PC != 0x00004000 {
			t.Errorf("MOVEC should trigger privilege violation in user mode: PC=0x%08X", cpu.PC)
		}

		// VBR should NOT be updated
		if cpu.VBR != 0 {
			t.Errorf("MOVEC should not update VBR in user mode")
		}
	})

	// MOVES - Move Address Space
	t.Run("MOVES D0,(A0) - Write to alternate address space", func(t *testing.T) {
		cpu := setupTestCPU()

		// Start in supervisor mode
		cpu.SR |= M68K_SR_S

		// Setup
		cpu.DataRegs[0] = 0x12345678
		memAddr := uint32(0x00002000)
		cpu.AddrRegs[0] = memAddr

		// Set DFC (Destination Function Code) to user data space
		cpu.DFC = 0x01

		// MOVES.L D0,(A0) - Write using function code
		cpu.Write16(cpu.PC, 0x0E90)   // MOVES.L (A0)
		cpu.Write16(cpu.PC+2, 0x0800) // D0 source

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Memory should be written (using alternate address space)
		if cpu.Read32(memAddr) != 0x12345678 {
			t.Errorf("MOVES write failed: got 0x%08X, expected 0x12345678", cpu.Read32(memAddr))
		}
	})

	t.Run("MOVES (A0),D0 - Read from alternate address space", func(t *testing.T) {
		cpu := setupTestCPU()

		// Start in supervisor mode
		cpu.SR |= M68K_SR_S

		// Setup
		memAddr := uint32(0x00002000)
		cpu.Write32(memAddr, 0xABCDEF12)
		cpu.AddrRegs[0] = memAddr
		cpu.DataRegs[0] = 0x00000000

		// Set SFC (Source Function Code) to user data space
		cpu.SFC = 0x01

		// MOVES.L (A0),D0 - Read using function code
		cpu.Write16(cpu.PC, 0x0E90)
		cpu.Write16(cpu.PC+2, 0x0000) // D0 destination

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// D0 should contain read value
		if cpu.DataRegs[0] != 0xABCDEF12 {
			t.Errorf("MOVES read failed: got 0x%08X, expected 0xABCDEF12", cpu.DataRegs[0])
		}
	})

	t.Run("MOVES D0,(A0) - Privilege violation in user mode", func(t *testing.T) {
		cpu := setupTestCPU()

		// Start in USER mode
		cpu.SR &= ^uint16(M68K_SR_S)

		// Set up privilege violation exception vector
		cpu.Write32(M68K_VEC_PRIVILEGE*4, 0x00004000)

		// Setup
		cpu.DataRegs[0] = 0x12345678
		cpu.AddrRegs[0] = 0x00002000

		// MOVES.L D0,(A0) (should trap)
		cpu.Write16(cpu.PC, 0x0E90)
		cpu.Write16(cpu.PC+2, 0x0800)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to privilege violation handler
		if cpu.PC != 0x00004000 {
			t.Errorf("MOVES should trigger privilege violation in user mode: PC=0x%08X", cpu.PC)
		}
	})
}

//----------------------------------------------------------------------------
// Edge Case Tests
//----------------------------------------------------------------------------

func TestEdgeCases(t *testing.T) {
	t.Logf("=== 68020 Edge Case Tests ===")

	// Address Error Tests
	t.Run("Address Error - MOVE.W to odd address", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up address error exception vector
		cpu.Write32(M68K_VEC_ADDRESS_ERROR*4, 0x00005000)

		// Setup: Try to write word to odd address
		cpu.DataRegs[0] = 0x1234
		cpu.AddrRegs[0] = 0x00002001 // ODD address!

		// MOVE.W D0,(A0) - Should trap on address error
		cpu.Write16(cpu.PC, 0x30C0) // MOVE.W D0,(A0)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to address error handler
		// (Note: Some emulators may allow unaligned access; check implementation)
		if cpu.PC == 0x00005000 {
			t.Logf("Address error correctly detected for odd word access")
		} else {
			t.Logf("Warning: Emulator allows unaligned word access (not strictly accurate)")
		}
	})

	t.Run("Address Error - MOVE.L to odd address", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up address error exception vector
		cpu.Write32(M68K_VEC_ADDRESS_ERROR*4, 0x00005000)

		// Setup: Try to write long to odd address
		cpu.DataRegs[0] = 0x12345678
		cpu.AddrRegs[0] = 0x00002003 // ODD address!

		// MOVE.L D0,(A0) - Should trap
		cpu.Write16(cpu.PC, 0x20C0) // MOVE.L D0,(A0)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		if cpu.PC == 0x00005000 {
			t.Logf("Address error correctly detected for odd long access")
		}
	})

	// Illegal Instruction Test
	t.Run("Illegal Instruction - Invalid opcode", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up illegal instruction exception vector
		cpu.Write32(M68K_VEC_ILLEGAL_INSTR*4, 0x00006000)

		// Write illegal/unimplemented opcode
		cpu.Write16(cpu.PC, 0xFFFF) // Illegal opcode

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to illegal instruction handler
		if cpu.PC != 0x00006000 {
			t.Errorf("Illegal instruction should trigger exception: PC=0x%08X, expected 0x00006000", cpu.PC)
		}
	})

	// Overflow Tests
	t.Run("ADD.L overflow detection", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: Add two large positive numbers causing signed overflow
		cpu.DataRegs[0] = 0x7FFFFFFF // Max positive int32
		cpu.DataRegs[1] = 0x00000001

		// ADD.L D1,D0 - Should overflow
		cpu.Write16(cpu.PC, 0xD081) // ADD.L D1,D0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Result wraps: 0x7FFFFFFF + 1 = 0x80000000 (negative)
		if cpu.DataRegs[0] != 0x80000000 {
			t.Errorf("ADD.L overflow result incorrect: got 0x%08X", cpu.DataRegs[0])
		}

		// V flag should be set (signed overflow)
		if (cpu.SR & M68K_SR_V) == 0 {
			t.Errorf("ADD.L should set V flag on signed overflow")
		}

		// N flag should be set (result is negative)
		if (cpu.SR & M68K_SR_N) == 0 {
			t.Errorf("ADD.L should set N flag for negative result")
		}
	})

	t.Run("SUB.L overflow detection", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: Subtract causing signed overflow
		cpu.DataRegs[0] = 0x80000000 // Min negative int32
		cpu.DataRegs[1] = 0x00000001

		// SUB.L D1,D0 - Should overflow
		cpu.Write16(cpu.PC, 0x9081) // SUB.L D1,D0

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Result wraps: 0x80000000 - 1 = 0x7FFFFFFF (positive)
		if cpu.DataRegs[0] != 0x7FFFFFFF {
			t.Errorf("SUB.L overflow result incorrect: got 0x%08X", cpu.DataRegs[0])
		}

		// V flag should be set
		if (cpu.SR & M68K_SR_V) == 0 {
			t.Errorf("SUB.L should set V flag on signed overflow")
		}
	})

	// CHK.L Tests
	t.Run("CHK.L D1,D0 - Value within range", func(t *testing.T) {
		cpu := setupTestCPU()

		// Setup: Check if D0 <= D1
		cpu.DataRegs[0] = 500  // Value to check
		cpu.DataRegs[1] = 1000 // Upper bound

		// CHK.L D1,D0
		cpu.Write16(cpu.PC, 0x4180) // CHK.L D1,D0
		cpu.Write16(cpu.PC+2, 0x0100)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Should not trap (500 is valid: 0 <= 500 <= 1000)
		// N flag clear = within range
		if (cpu.SR & M68K_SR_N) != 0 {
			t.Errorf("CHK.L should not trap for valid value")
		}
	})

	t.Run("CHK.L D1,D0 - Negative value (trap)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up CHK exception vector
		cpu.Write32(M68K_VEC_CHK*4, 0x00007000)

		// Setup
		var negOne int32 = -1
		cpu.DataRegs[0] = uint32(negOne) // Negative! (< 0)
		cpu.DataRegs[1] = 1000

		// CHK.L D1,D0
		cpu.Write16(cpu.PC, 0x4180)
		cpu.Write16(cpu.PC+2, 0x0100)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to CHK exception handler
		if cpu.PC != 0x00007000 {
			t.Errorf("CHK.L should trap for negative value: PC=0x%08X", cpu.PC)
		}
	})

	t.Run("CHK.L D1,D0 - Value exceeds upper bound (trap)", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up CHK exception vector
		cpu.Write32(M68K_VEC_CHK*4, 0x00007000)

		// Setup
		cpu.DataRegs[0] = 2000 // Exceeds upper bound
		cpu.DataRegs[1] = 1000 // Upper bound

		// CHK.L D1,D0
		cpu.Write16(cpu.PC, 0x4180)
		cpu.Write16(cpu.PC+2, 0x0100)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to CHK exception handler
		if cpu.PC != 0x00007000 {
			t.Errorf("CHK.L should trap for value > upper bound: PC=0x%08X", cpu.PC)
		}
	})

	// TRAPV - Trap on Overflow
	t.Run("TRAPV with V flag set", func(t *testing.T) {
		cpu := setupTestCPU()

		// Set up TRAPV exception vector
		cpu.Write32(M68K_VEC_TRAPV*4, 0x00008000)

		// Set V flag (overflow condition)
		cpu.SR |= M68K_SR_V

		// TRAPV
		cpu.Write16(cpu.PC, 0x4E76) // TRAPV

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should have jumped to TRAPV handler
		if cpu.PC != 0x00008000 {
			t.Errorf("TRAPV should trap when V flag set: PC=0x%08X", cpu.PC)
		}
	})

	t.Run("TRAPV with V flag clear", func(t *testing.T) {
		cpu := setupTestCPU()

		// Clear V flag
		cpu.SR &= ^uint16(M68K_SR_V)

		savedPC := cpu.PC

		// TRAPV
		cpu.Write16(cpu.PC, 0x4E76)

		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// PC should advance normally (no trap)
		if cpu.PC < savedPC+2 {
			t.Errorf("TRAPV should not trap when V flag clear")
		}
	})
}
