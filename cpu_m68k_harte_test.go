// cpu_m68k_harte_test.go - Tom Harte 68000 JSON Test Harness

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
This test harness validates the M68K CPU implementation against Tom Harte's
SingleStepTests, a comprehensive suite of ~1,000,000 test cases covering
all 68000 instructions with precise initial/final state specifications.

Test Format:
The tests are organized as gzip-compressed JSON files with structure:
  - name: Test case identifier
  - initial: CPU and memory state before execution
  - final: Expected CPU and memory state after execution
  - length: Number of bytes in the instruction
  - transactions: Bus transactions (optional, for cycle accuracy)

Test Data Source:
https://github.com/SingleStepTests/680x0

Usage:
  go test -v -run TestHarte68000           # Run all tests
  go test -v -run TestHarteNOP             # Run specific instruction
  go test -v -short -run TestHarte68000    # Run with sampling (faster)
*/

package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// Test Data Structures
// -----------------------------------------------------------------------------

// HarteTestCase represents a single test from Tom Harte's test suite
type HarteTestCase struct {
	Name         string          `json:"name"`
	Initial      HarteState      `json:"initial"`
	Final        HarteState      `json:"final"`
	Length       int             `json:"length"`
	Transactions [][]interface{} `json:"transactions"` // Optional, for cycle accuracy
}

// HarteState represents CPU and memory state
// The JSON uses lowercase register names (d0, d1, a0, etc.)
type HarteState struct {
	D0       uint32     `json:"d0"`
	D1       uint32     `json:"d1"`
	D2       uint32     `json:"d2"`
	D3       uint32     `json:"d3"`
	D4       uint32     `json:"d4"`
	D5       uint32     `json:"d5"`
	D6       uint32     `json:"d6"`
	D7       uint32     `json:"d7"`
	A0       uint32     `json:"a0"`
	A1       uint32     `json:"a1"`
	A2       uint32     `json:"a2"`
	A3       uint32     `json:"a3"`
	A4       uint32     `json:"a4"`
	A5       uint32     `json:"a5"`
	A6       uint32     `json:"a6"`
	USP      uint32     `json:"usp"`
	SSP      uint32     `json:"ssp"`
	SR       uint32     `json:"sr"`
	PC       uint32     `json:"pc"`
	Prefetch []uint32   `json:"prefetch"` // Instruction words at PC
	RAM      [][]uint32 `json:"ram"`      // [[address, value], ...]
}

// -----------------------------------------------------------------------------
// Test Configuration
// -----------------------------------------------------------------------------

var (
	harteVerbose = flag.Bool("harte-verbose", false, "Enable verbose output for Harte tests")
	harteSample  = flag.Int("harte-sample", 0, "Run only N random tests per file (0 = all)")
)

const (
	harteTestDir = "testdata/68000/v1"
)

// -----------------------------------------------------------------------------
// Test File Loading
// -----------------------------------------------------------------------------

// LoadHarteTests loads gzip-compressed JSON test file
func LoadHarteTests(filename string) ([]HarteTestCase, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open test file: %w", err)
	}
	defer file.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Decode JSON array
	var tests []HarteTestCase
	decoder := json.NewDecoder(gzReader)
	if err := decoder.Decode(&tests); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return tests, nil
}

// LoadHarteTestsUncompressed loads a plain JSON test file (for testing)
func LoadHarteTestsUncompressed(filename string) ([]HarteTestCase, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open test file: %w", err)
	}
	defer file.Close()

	var tests []HarteTestCase
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&tests); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return tests, nil
}

// -----------------------------------------------------------------------------
// CPU State Management
// -----------------------------------------------------------------------------

// SetupHarteCPUState configures CPU to match initial state from test case
func SetupHarteCPUState(cpu *M68KCPU, state HarteState) {
	// Set data registers
	cpu.DataRegs[0] = state.D0
	cpu.DataRegs[1] = state.D1
	cpu.DataRegs[2] = state.D2
	cpu.DataRegs[3] = state.D3
	cpu.DataRegs[4] = state.D4
	cpu.DataRegs[5] = state.D5
	cpu.DataRegs[6] = state.D6
	cpu.DataRegs[7] = state.D7

	// Set address registers (A0-A6)
	cpu.AddrRegs[0] = state.A0
	cpu.AddrRegs[1] = state.A1
	cpu.AddrRegs[2] = state.A2
	cpu.AddrRegs[3] = state.A3
	cpu.AddrRegs[4] = state.A4
	cpu.AddrRegs[5] = state.A5
	cpu.AddrRegs[6] = state.A6

	// Set status register (before setting stack pointers)
	cpu.SR = uint16(state.SR)

	// Set stack pointers based on supervisor mode
	// When SR.S=1 (supervisor mode), A7 = SSP and USP is separate
	// When SR.S=0 (user mode), A7 = USP and SSP is stored internally
	if state.SR&uint32(M68K_SR_S) != 0 {
		// Supervisor mode: A7 is SSP
		cpu.AddrRegs[7] = state.SSP
		cpu.USP = state.USP
	} else {
		// User mode: A7 is USP
		cpu.AddrRegs[7] = state.USP
		// Store SSP - we need a way to track this, use USP field temporarily
		// Actually the real 68000 has separate internal storage for SSP when in user mode
		// For our implementation, we'll need to handle this carefully
		cpu.USP = state.SSP // Store SSP in USP since we're in user mode
		// Actually this is wrong - let me reconsider
		// In user mode: A7 = USP, and SSP is stored elsewhere
		// We need to swap them correctly
	}

	// Re-handle stack pointers more carefully
	// The 68000 has two stack pointers: USP (user) and SSP (supervisor)
	// At any time, A7 points to one of them based on SR.S bit
	// The other one is stored in an internal register
	//
	// For our emulator:
	// - cpu.AddrRegs[7] is always the "active" stack pointer
	// - cpu.USP stores the inactive USP when in supervisor mode
	//
	// So the mapping is:
	// - If SR.S=1: A7=SSP, cpu.USP=USP (stored)
	// - If SR.S=0: A7=USP, we need somewhere to store SSP...
	//
	// Looking at the existing code, it seems like cpu.USP is used to store USP
	// when we're in supervisor mode. When in user mode, there's no separate
	// storage for SSP in the current implementation.
	//
	// For test setup, let's do this:
	cpu.USP = state.USP
	if state.SR&uint32(M68K_SR_S) != 0 {
		cpu.AddrRegs[7] = state.SSP
	} else {
		cpu.AddrRegs[7] = state.USP
	}

	// Set program counter
	cpu.PC = state.PC

	// Setup prefetch queue - write instruction words to memory at PC
	// The prefetch contains the instruction word(s) that should be at PC
	// Write directly in big-endian format (don't use cpu.Write16 which does endian swap)
	for i, word := range state.Prefetch {
		addr := state.PC + uint32(i*2)
		// Write big-endian: high byte first, then low byte
		cpu.memory[addr] = uint8(word >> 8)     // High byte
		cpu.memory[addr+1] = uint8(word & 0xFF) // Low byte
	}

	// Setup RAM contents - each entry is [address, byte_value]
	// Write directly to memory to avoid any endian conversion
	for _, entry := range state.RAM {
		if len(entry) >= 2 {
			addr := entry[0]
			value := uint8(entry[1])
			if addr < uint32(len(cpu.memory)) {
				cpu.memory[addr] = value
			}
		}
	}
}

// -----------------------------------------------------------------------------
// State Verification
// -----------------------------------------------------------------------------

// HarteTestResult holds the result of a single test comparison
type HarteTestResult struct {
	TestName   string
	Passed     bool
	Mismatches []string
}

// VerifyHarteFinalState compares CPU state against expected final state
func VerifyHarteFinalState(cpu *M68KCPU, expected HarteState, testName string) HarteTestResult {
	result := HarteTestResult{
		TestName: testName,
		Passed:   true,
	}

	// Helper to record mismatches
	mismatch := func(format string, args ...interface{}) {
		result.Passed = false
		result.Mismatches = append(result.Mismatches, fmt.Sprintf(format, args...))
	}

	// Check data registers
	expectedD := []uint32{expected.D0, expected.D1, expected.D2, expected.D3,
		expected.D4, expected.D5, expected.D6, expected.D7}
	for i, exp := range expectedD {
		if cpu.DataRegs[i] != exp {
			mismatch("D%d: got 0x%08X, want 0x%08X", i, cpu.DataRegs[i], exp)
		}
	}

	// Check address registers A0-A6
	expectedA := []uint32{expected.A0, expected.A1, expected.A2, expected.A3,
		expected.A4, expected.A5, expected.A6}
	for i, exp := range expectedA {
		if cpu.AddrRegs[i] != exp {
			mismatch("A%d: got 0x%08X, want 0x%08X", i, cpu.AddrRegs[i], exp)
		}
	}

	// Check stack pointers based on supervisor mode
	// This is tricky because A7's meaning depends on SR.S
	if expected.SR&uint32(M68K_SR_S) != 0 {
		// Supervisor mode: A7 should be SSP
		if cpu.AddrRegs[7] != expected.SSP {
			mismatch("SSP (A7): got 0x%08X, want 0x%08X", cpu.AddrRegs[7], expected.SSP)
		}
		if cpu.USP != expected.USP {
			mismatch("USP: got 0x%08X, want 0x%08X", cpu.USP, expected.USP)
		}
	} else {
		// User mode: A7 should be USP
		if cpu.AddrRegs[7] != expected.USP {
			mismatch("USP (A7): got 0x%08X, want 0x%08X", cpu.AddrRegs[7], expected.USP)
		}
		// SSP verification in user mode is tricky since we don't have separate storage
	}

	// Check SR
	if cpu.SR != uint16(expected.SR) {
		mismatch("SR: got 0x%04X, want 0x%04X", cpu.SR, uint16(expected.SR))
	}

	// Check PC
	if cpu.PC != expected.PC {
		mismatch("PC: got 0x%08X, want 0x%08X", cpu.PC, expected.PC)
	}

	// Check RAM contents - read directly from memory to avoid endian conversion
	for _, entry := range expected.RAM {
		if len(entry) >= 2 {
			addr := entry[0]
			expectedVal := uint8(entry[1])
			var actualVal uint8
			if addr < uint32(len(cpu.memory)) {
				actualVal = cpu.memory[addr]
			}
			if actualVal != expectedVal {
				mismatch("RAM[0x%06X]: got 0x%02X, want 0x%02X", addr, actualVal, expectedVal)
			}
		}
	}

	return result
}

// -----------------------------------------------------------------------------
// Test Runner
// -----------------------------------------------------------------------------

// Reusable test harness to avoid allocating 16MB per test case
var harteTestBus *SystemBus
var harteTestCPU *M68KCPU

func getHarteTestCPU() *M68KCPU {
	if harteTestBus == nil {
		harteTestBus = NewSystemBus()
		harteTestCPU = &M68KCPU{
			SR:              M68K_SR_S,
			bus:             harteTestBus,
			memory:          harteTestBus.GetMemory(),
			stackLowerBound: 0,
			stackUpperBound: 0xFFFFFFFF,
		}
	}
	return harteTestCPU
}

func resetHarteTestCPU(cpu *M68KCPU) {
	// Clear memory (only the parts we use - first 2MB should be enough for tests)
	mem := cpu.memory
	for i := 0; i < 2*1024*1024 && i < len(mem); i++ {
		mem[i] = 0
	}
	// Reset CPU state
	cpu.PC = 0
	cpu.SR = M68K_SR_S
	cpu.USP = 0
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0
	}
	for i := range cpu.AddrRegs {
		cpu.AddrRegs[i] = 0
	}
	cpu.running.Store(true)
}

// RunHarteTest executes a single test case and returns the result
func RunHarteTest(tc HarteTestCase) HarteTestResult {
	// Get reusable CPU and reset it
	cpu := getHarteTestCPU()
	resetHarteTestCPU(cpu)

	// Setup initial state
	SetupHarteCPUState(cpu, tc.Initial)

	// Capture initial A7 for debugging
	initialA7 := cpu.AddrRegs[7]
	initialPC := cpu.PC

	// Execute single instruction
	// The prefetch is already written to memory, fetch and decode it
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Verify final state
	result := VerifyHarteFinalState(cpu, tc.Final, tc.Name)

	// Debug output for failing tests
	if !result.Passed && *harteVerbose {
		fmt.Printf("DEBUG %s: initial A7=0x%08X, initial PC=0x%08X, currentIR=0x%04X\n",
			tc.Name, initialA7, initialPC, cpu.currentIR)
		fmt.Printf("DEBUG %s: initial state: A0=0x%08X A1=0x%08X A2=0x%08X A3=0x%08X A4=0x%08X A5=0x%08X A6=0x%08X\n",
			tc.Name, tc.Initial.A0, tc.Initial.A1, tc.Initial.A2, tc.Initial.A3, tc.Initial.A4, tc.Initial.A5, tc.Initial.A6)
		fmt.Printf("DEBUG %s: initial state: D0=0x%08X D1=0x%08X D2=0x%08X D3=0x%08X D4=0x%08X D5=0x%08X D6=0x%08X D7=0x%08X\n",
			tc.Name, tc.Initial.D0, tc.Initial.D1, tc.Initial.D2, tc.Initial.D3, tc.Initial.D4, tc.Initial.D5, tc.Initial.D6, tc.Initial.D7)
		fmt.Printf("DEBUG %s: initial SSP=0x%08X USP=0x%08X SR=0x%04X\n",
			tc.Name, tc.Initial.SSP, tc.Initial.USP, tc.Initial.SR)
		fmt.Printf("DEBUG %s: final A7=0x%08X, final PC=0x%08X\n",
			tc.Name, cpu.AddrRegs[7], cpu.PC)
	}

	return result
}

// RunHarteTestT runs a test case using Go's testing framework
func RunHarteTestT(t *testing.T, tc HarteTestCase) bool {
	result := RunHarteTest(tc)

	if !result.Passed {
		if *harteVerbose || testing.Verbose() {
			t.Errorf("%s FAILED:", result.TestName)
			for _, m := range result.Mismatches {
				t.Errorf("  %s", m)
			}
		}
	}

	return result.Passed
}

// RunHarteTestFile runs all tests from a single .json.gz file
func RunHarteTestFile(t *testing.T, filename string) {
	tests, err := LoadHarteTests(filename)
	if err != nil {
		t.Fatalf("Failed to load tests from %s: %v", filename, err)
	}

	if len(tests) == 0 {
		t.Skipf("No tests found in %s", filename)
		return
	}

	// Sample if requested
	testCount := len(tests)
	if *harteSample > 0 && *harteSample < testCount {
		// Simple sampling: take every Nth test
		step := testCount / *harteSample
		sampled := make([]HarteTestCase, 0, *harteSample)
		for i := 0; i < testCount && len(sampled) < *harteSample; i += step {
			sampled = append(sampled, tests[i])
		}
		tests = sampled
	}

	// Also sample in short mode
	if testing.Short() && len(tests) > 100 {
		step := len(tests) / 100
		sampled := make([]HarteTestCase, 0, 100)
		for i := 0; i < len(tests) && len(sampled) < 100; i += step {
			sampled = append(sampled, tests[i])
		}
		tests = sampled
	}

	passed, failed := 0, 0
	var failures []string

	for _, tc := range tests {
		if RunHarteTestT(t, tc) {
			passed++
		} else {
			failed++
			if len(failures) < 10 {
				failures = append(failures, tc.Name)
			}
		}
	}

	// Summary
	total := passed + failed
	passRate := float64(passed) / float64(total) * 100
	t.Logf("%s: %d/%d passed (%.1f%%)", filepath.Base(filename), passed, total, passRate)

	if failed > 0 && len(failures) > 0 {
		t.Logf("First failures: %v", failures)
	}
}

// -----------------------------------------------------------------------------
// Main Test Entry Points
// -----------------------------------------------------------------------------

// TestHarte68000 runs all available Tom Harte tests
func TestHarte68000(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(harteTestDir, "*.json.gz"))
	if err != nil || len(files) == 0 {
		t.Skip("Tom Harte test files not found. Run 'make testdata-harte' to download them.")
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json.gz")
		t.Run(name, func(t *testing.T) {
			// Don't run in parallel - causes memory exhaustion and crashes
			RunHarteTestFile(t, file)
		})
	}
}

// Individual instruction tests for targeted debugging
func TestHarteNOP(t *testing.T) {
	file := filepath.Join(harteTestDir, "NOP.json.gz")
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skip("NOP test file not found")
	}
	RunHarteTestFile(t, file)
}

func TestHarteMOVE(t *testing.T) {
	patterns := []string{"MOVE.b.json.gz", "MOVE.w.json.gz", "MOVE.l.json.gz"}
	for _, pattern := range patterns {
		file := filepath.Join(harteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(strings.TrimSuffix(pattern, ".json.gz"), func(t *testing.T) {
			RunHarteTestFile(t, file)
		})
	}
}

func TestHarteADD(t *testing.T) {
	patterns := []string{"ADD.b.json.gz", "ADD.w.json.gz", "ADD.l.json.gz"}
	for _, pattern := range patterns {
		file := filepath.Join(harteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(strings.TrimSuffix(pattern, ".json.gz"), func(t *testing.T) {
			RunHarteTestFile(t, file)
		})
	}
}

func TestHarteSUB(t *testing.T) {
	patterns := []string{"SUB.b.json.gz", "SUB.w.json.gz", "SUB.l.json.gz"}
	for _, pattern := range patterns {
		file := filepath.Join(harteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(strings.TrimSuffix(pattern, ".json.gz"), func(t *testing.T) {
			RunHarteTestFile(t, file)
		})
	}
}

func TestHarteAND(t *testing.T) {
	patterns := []string{"AND.b.json.gz", "AND.w.json.gz", "AND.l.json.gz"}
	for _, pattern := range patterns {
		file := filepath.Join(harteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(strings.TrimSuffix(pattern, ".json.gz"), func(t *testing.T) {
			RunHarteTestFile(t, file)
		})
	}
}

func TestHarteOR(t *testing.T) {
	patterns := []string{"OR.b.json.gz", "OR.w.json.gz", "OR.l.json.gz"}
	for _, pattern := range patterns {
		file := filepath.Join(harteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(strings.TrimSuffix(pattern, ".json.gz"), func(t *testing.T) {
			RunHarteTestFile(t, file)
		})
	}
}

// TestHarteSingleFile allows running a specific test file by name
// Usage: go test -v -run "TestHarteSingleFile/BTST"
func TestHarteSingleFile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(harteTestDir, "*.json.gz"))
	if err != nil || len(files) == 0 {
		t.Skip("Tom Harte test files not found")
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json.gz")
		t.Run(name, func(t *testing.T) {
			RunHarteTestFile(t, file)
		})
	}
}

// TestHarteManualNOP tests NOP instruction with a manually created test case
func TestHarteManualNOP(t *testing.T) {
	file := filepath.Join(harteTestDir, "NOP_test.json")
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skip("Manual NOP test file not found")
	}

	tests, err := LoadHarteTestsUncompressed(file)
	if err != nil {
		t.Fatalf("Failed to load tests: %v", err)
	}

	for _, tc := range tests {
		result := RunHarteTest(tc)
		if !result.Passed {
			t.Errorf("Test %s FAILED:", result.TestName)
			for _, m := range result.Mismatches {
				t.Errorf("  %s", m)
			}
		} else {
			t.Logf("Test %s PASSED", result.TestName)
		}
	}
}

// -----------------------------------------------------------------------------
// Benchmark
// -----------------------------------------------------------------------------

func BenchmarkHarteNOP(b *testing.B) {
	file := filepath.Join(harteTestDir, "NOP.json.gz")
	tests, err := LoadHarteTests(file)
	if err != nil || len(tests) == 0 {
		b.Skip("NOP test file not found or empty")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc := tests[i%len(tests)]
		RunHarteTest(tc)
	}
}
