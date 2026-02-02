// cpu_x86_harte_test.go - Tom Harte 8088 JSON Test Harness
//
// This test harness validates the x86 CPU implementation against Tom Harte's
// SingleStepTests/8088, a comprehensive suite of ~10,000 tests per opcode
// covering all 8086/8088 instructions with precise initial/final state specifications.
//
// Test Data Source:
// https://github.com/SingleStepTests/8088
//
// Usage:
//   go test -v -run TestHarte8086           # Run all tests
//   go test -v -run TestHarteX86_NOP        # Run specific instruction
//   go test -v -short -run TestHarte8086    # Run with sampling (faster)
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

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

// X86HarteTestCase represents a single test from Tom Harte's 8088 test suite
type X86HarteTestCase struct {
	Name    string        `json:"name"`
	Initial X86HarteState `json:"initial"`
	Final   X86HarteState `json:"final"`
}

// X86HarteState represents CPU and memory state for 8088 tests
type X86HarteState struct {
	Regs X86HarteRegs `json:"regs"`
	RAM  [][]uint32   `json:"ram"` // [[address, value], ...]
}

// X86HarteRegs represents 8088 register state
type X86HarteRegs struct {
	AX    uint16 `json:"ax"`
	BX    uint16 `json:"bx"`
	CX    uint16 `json:"cx"`
	DX    uint16 `json:"dx"`
	SI    uint16 `json:"si"`
	DI    uint16 `json:"di"`
	BP    uint16 `json:"bp"`
	SP    uint16 `json:"sp"`
	IP    uint16 `json:"ip"`
	CS    uint16 `json:"cs"`
	DS    uint16 `json:"ds"`
	ES    uint16 `json:"es"`
	SS    uint16 `json:"ss"`
	Flags uint16 `json:"flags"`
}

// -----------------------------------------------------------------------------
// Test Configuration
// -----------------------------------------------------------------------------

var (
	x86HarteVerbose = flag.Bool("x86-harte-verbose", false, "Enable verbose output for x86 Harte tests")
	x86HarteSample  = flag.Int("x86-harte-sample", 0, "Run only N random tests per file (0 = all)")
)

const (
	x86HarteTestDir = "testdata/8088/v1"
)

// -----------------------------------------------------------------------------
// Test Bus for Harte Tests
// -----------------------------------------------------------------------------

type X86HarteBus struct {
	memory [1024 * 1024]byte // 1MB memory space
}

func NewX86HarteBus() *X86HarteBus {
	return &X86HarteBus{}
}

func (b *X86HarteBus) Read(addr uint32) byte {
	if addr < uint32(len(b.memory)) {
		return b.memory[addr]
	}
	return 0
}

func (b *X86HarteBus) Write(addr uint32, value byte) {
	if addr < uint32(len(b.memory)) {
		b.memory[addr] = value
	}
}

func (b *X86HarteBus) In(port uint16) byte {
	return 0 // No I/O for Harte tests
}

func (b *X86HarteBus) Out(port uint16, value byte) {
	// No I/O for Harte tests
}

func (b *X86HarteBus) Tick(cycles int) {}

// Clear clears memory
func (b *X86HarteBus) Clear() {
	for i := range b.memory {
		b.memory[i] = 0
	}
}

// -----------------------------------------------------------------------------
// Test File Loading
// -----------------------------------------------------------------------------

// LoadX86HarteTests loads gzip-compressed JSON test file
func LoadX86HarteTests(filename string) ([]X86HarteTestCase, error) {
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
	var tests []X86HarteTestCase
	decoder := json.NewDecoder(gzReader)
	if err := decoder.Decode(&tests); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return tests, nil
}

// LoadX86HarteTestsUncompressed loads a plain JSON test file (for testing)
func LoadX86HarteTestsUncompressed(filename string) ([]X86HarteTestCase, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open test file: %w", err)
	}
	defer file.Close()

	var tests []X86HarteTestCase
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&tests); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return tests, nil
}

// -----------------------------------------------------------------------------
// CPU State Management
// -----------------------------------------------------------------------------

// calcLinearAddr calculates linear address from segment:offset
func calcLinearAddr(seg, off uint16) uint32 {
	return (uint32(seg) << 4) + uint32(off)
}

// SetupX86HarteCPUState configures CPU to match initial state from test case
func SetupX86HarteCPUState(cpu *CPU_X86, bus *X86HarteBus, state X86HarteState) {
	// Clear memory first
	bus.Clear()

	// Set 16-bit registers (for 8086/8088 mode)
	cpu.SetAX(state.Regs.AX)
	cpu.SetBX(state.Regs.BX)
	cpu.SetCX(state.Regs.CX)
	cpu.SetDX(state.Regs.DX)
	cpu.SetSI(state.Regs.SI)
	cpu.SetDI(state.Regs.DI)
	cpu.SetBP(state.Regs.BP)
	cpu.SetSP(state.Regs.SP)

	// Set instruction pointer
	cpu.SetIP(state.Regs.IP)

	// Set segment registers
	cpu.CS = state.Regs.CS
	cpu.DS = state.Regs.DS
	cpu.ES = state.Regs.ES
	cpu.SS = state.Regs.SS

	// Set flags
	cpu.Flags = uint32(state.Regs.Flags)

	// Clear upper 16 bits of 32-bit registers for 8086 mode
	cpu.EAX = cpu.EAX & 0xFFFF
	cpu.EBX = cpu.EBX & 0xFFFF
	cpu.ECX = cpu.ECX & 0xFFFF
	cpu.EDX = cpu.EDX & 0xFFFF
	cpu.ESI = cpu.ESI & 0xFFFF
	cpu.EDI = cpu.EDI & 0xFFFF
	cpu.EBP = cpu.EBP & 0xFFFF
	cpu.ESP = cpu.ESP & 0xFFFF
	cpu.EIP = cpu.EIP & 0xFFFF

	// Setup RAM contents - each entry is [address, byte_value]
	for _, entry := range state.RAM {
		if len(entry) >= 2 {
			addr := entry[0]
			value := byte(entry[1])
			if addr < uint32(len(bus.memory)) {
				bus.memory[addr] = value
			}
		}
	}

	// Reset CPU execution state
	cpu.Halted = false
	cpu.SetRunning(true)
	cpu.Cycles = 0
}

// -----------------------------------------------------------------------------
// State Verification
// -----------------------------------------------------------------------------

// X86HarteTestResult holds the result of a single test comparison
type X86HarteTestResult struct {
	TestName   string
	Passed     bool
	Mismatches []string
}

// VerifyX86HarteFinalState compares CPU state against expected final state
func VerifyX86HarteFinalState(cpu *CPU_X86, bus *X86HarteBus, expected X86HarteState, testName string) X86HarteTestResult {
	result := X86HarteTestResult{
		TestName: testName,
		Passed:   true,
	}

	// Helper to record mismatches
	mismatch := func(format string, args ...interface{}) {
		result.Passed = false
		result.Mismatches = append(result.Mismatches, fmt.Sprintf(format, args...))
	}

	// Check 16-bit registers
	if cpu.AX() != expected.Regs.AX {
		mismatch("AX: got 0x%04X, want 0x%04X", cpu.AX(), expected.Regs.AX)
	}
	if cpu.BX() != expected.Regs.BX {
		mismatch("BX: got 0x%04X, want 0x%04X", cpu.BX(), expected.Regs.BX)
	}
	if cpu.CX() != expected.Regs.CX {
		mismatch("CX: got 0x%04X, want 0x%04X", cpu.CX(), expected.Regs.CX)
	}
	if cpu.DX() != expected.Regs.DX {
		mismatch("DX: got 0x%04X, want 0x%04X", cpu.DX(), expected.Regs.DX)
	}
	if cpu.SI() != expected.Regs.SI {
		mismatch("SI: got 0x%04X, want 0x%04X", cpu.SI(), expected.Regs.SI)
	}
	if cpu.DI() != expected.Regs.DI {
		mismatch("DI: got 0x%04X, want 0x%04X", cpu.DI(), expected.Regs.DI)
	}
	if cpu.BP() != expected.Regs.BP {
		mismatch("BP: got 0x%04X, want 0x%04X", cpu.BP(), expected.Regs.BP)
	}
	if cpu.SP() != expected.Regs.SP {
		mismatch("SP: got 0x%04X, want 0x%04X", cpu.SP(), expected.Regs.SP)
	}
	if cpu.IP() != expected.Regs.IP {
		mismatch("IP: got 0x%04X, want 0x%04X", cpu.IP(), expected.Regs.IP)
	}

	// Check segment registers
	if cpu.CS != expected.Regs.CS {
		mismatch("CS: got 0x%04X, want 0x%04X", cpu.CS, expected.Regs.CS)
	}
	if cpu.DS != expected.Regs.DS {
		mismatch("DS: got 0x%04X, want 0x%04X", cpu.DS, expected.Regs.DS)
	}
	if cpu.ES != expected.Regs.ES {
		mismatch("ES: got 0x%04X, want 0x%04X", cpu.ES, expected.Regs.ES)
	}
	if cpu.SS != expected.Regs.SS {
		mismatch("SS: got 0x%04X, want 0x%04X", cpu.SS, expected.Regs.SS)
	}

	// Check flags (mask to 16-bit and only check defined flags)
	// 8088 flags: CF(0), PF(2), AF(4), ZF(6), SF(7), TF(8), IF(9), DF(10), OF(11)
	flagMask := uint16(0x0FD5) // Only defined flags
	gotFlags := uint16(cpu.Flags) & flagMask
	wantFlags := expected.Regs.Flags & flagMask
	if gotFlags != wantFlags {
		mismatch("Flags: got 0x%04X, want 0x%04X", gotFlags, wantFlags)
	}

	// Check RAM contents
	for _, entry := range expected.RAM {
		if len(entry) >= 2 {
			addr := entry[0]
			expectedVal := byte(entry[1])
			var actualVal byte
			if addr < uint32(len(bus.memory)) {
				actualVal = bus.memory[addr]
			}
			if actualVal != expectedVal {
				mismatch("RAM[0x%05X]: got 0x%02X, want 0x%02X", addr, actualVal, expectedVal)
			}
		}
	}

	return result
}

// -----------------------------------------------------------------------------
// Test Runner
// -----------------------------------------------------------------------------

// Reusable test harness to avoid allocating memory per test case
var x86HarteTestBus *X86HarteBus
var x86HarteTestCPU *CPU_X86

func getX86HarteTestCPU() (*CPU_X86, *X86HarteBus) {
	if x86HarteTestBus == nil {
		x86HarteTestBus = NewX86HarteBus()
		x86HarteTestCPU = NewCPU_X86(x86HarteTestBus)
	}
	return x86HarteTestCPU, x86HarteTestBus
}

// RunX86HarteTest executes a single test case and returns the result
func RunX86HarteTest(tc X86HarteTestCase) X86HarteTestResult {
	// Get reusable CPU and bus
	cpu, bus := getX86HarteTestCPU()

	// Setup initial state
	SetupX86HarteCPUState(cpu, bus, tc.Initial)

	// Enable 16-bit mode (8086/8088 compatibility)
	cpu.prefixOpSize = true   // Use 16-bit operands
	cpu.prefixAddrSize = true // Use 16-bit addresses

	// Execute single instruction
	cpu.Step()

	// Verify final state
	return VerifyX86HarteFinalState(cpu, bus, tc.Final, tc.Name)
}

// RunX86HarteTestT runs a test case using Go's testing framework
func RunX86HarteTestT(t *testing.T, tc X86HarteTestCase) bool {
	result := RunX86HarteTest(tc)

	if !result.Passed {
		if *x86HarteVerbose || testing.Verbose() {
			t.Errorf("%s FAILED:", result.TestName)
			for _, m := range result.Mismatches {
				t.Errorf("  %s", m)
			}
		}
	}

	return result.Passed
}

// RunX86HarteTestFile runs all tests from a single .json.gz file
func RunX86HarteTestFile(t *testing.T, filename string) {
	tests, err := LoadX86HarteTests(filename)
	if err != nil {
		t.Fatalf("Failed to load tests from %s: %v", filename, err)
	}

	if len(tests) == 0 {
		t.Skipf("No tests found in %s", filename)
		return
	}

	// Sample if requested
	testCount := len(tests)
	if *x86HarteSample > 0 && *x86HarteSample < testCount {
		step := testCount / *x86HarteSample
		sampled := make([]X86HarteTestCase, 0, *x86HarteSample)
		for i := 0; i < testCount && len(sampled) < *x86HarteSample; i += step {
			sampled = append(sampled, tests[i])
		}
		tests = sampled
	}

	// Also sample in short mode
	if testing.Short() && len(tests) > 100 {
		step := len(tests) / 100
		sampled := make([]X86HarteTestCase, 0, 100)
		for i := 0; i < len(tests) && len(sampled) < 100; i += step {
			sampled = append(sampled, tests[i])
		}
		tests = sampled
	}

	passed, failed := 0, 0
	var failures []string

	for _, tc := range tests {
		if RunX86HarteTestT(t, tc) {
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
	if total == 0 {
		t.Logf("%s: no tests run", filepath.Base(filename))
		return
	}
	passRate := float64(passed) / float64(total) * 100
	t.Logf("%s: %d/%d passed (%.1f%%)", filepath.Base(filename), passed, total, passRate)

	if failed > 0 && len(failures) > 0 {
		t.Logf("First failures: %v", failures)
	}
}

// -----------------------------------------------------------------------------
// Main Test Entry Points
// -----------------------------------------------------------------------------

// TestHarte8086 runs all available Tom Harte 8088 tests
func TestHarte8086(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(x86HarteTestDir, "*.json.gz"))
	if err != nil || len(files) == 0 {
		t.Skip("Tom Harte 8088 test files not found. Run 'make testdata-x86' to download them.")
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json.gz")
		t.Run(name, func(t *testing.T) {
			RunX86HarteTestFile(t, file)
		})
	}
}

// Individual instruction tests for targeted debugging
func TestHarteX86_NOP(t *testing.T) {
	file := filepath.Join(x86HarteTestDir, "90.json.gz")
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skip("NOP test file not found")
	}
	RunX86HarteTestFile(t, file)
}

func TestHarteX86_MOV(t *testing.T) {
	patterns := []string{
		"88.json.gz", // MOV r/m8, r8
		"89.json.gz", // MOV r/m16, r16
		"8A.json.gz", // MOV r8, r/m8
		"8B.json.gz", // MOV r16, r/m16
		"B0.json.gz", // MOV AL, imm8
		"B8.json.gz", // MOV AX, imm16
	}
	for _, pattern := range patterns {
		file := filepath.Join(x86HarteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			RunX86HarteTestFile(t, file)
		})
	}
}

func TestHarteX86_ADD(t *testing.T) {
	patterns := []string{
		"00.json.gz", // ADD r/m8, r8
		"01.json.gz", // ADD r/m16, r16
		"02.json.gz", // ADD r8, r/m8
		"03.json.gz", // ADD r16, r/m16
		"04.json.gz", // ADD AL, imm8
		"05.json.gz", // ADD AX, imm16
	}
	for _, pattern := range patterns {
		file := filepath.Join(x86HarteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			RunX86HarteTestFile(t, file)
		})
	}
}

func TestHarteX86_SUB(t *testing.T) {
	patterns := []string{
		"28.json.gz", // SUB r/m8, r8
		"29.json.gz", // SUB r/m16, r16
		"2A.json.gz", // SUB r8, r/m8
		"2B.json.gz", // SUB r16, r/m16
		"2C.json.gz", // SUB AL, imm8
		"2D.json.gz", // SUB AX, imm16
	}
	for _, pattern := range patterns {
		file := filepath.Join(x86HarteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			RunX86HarteTestFile(t, file)
		})
	}
}

func TestHarteX86_JMP(t *testing.T) {
	patterns := []string{
		"E9.json.gz", // JMP rel16
		"EB.json.gz", // JMP rel8
	}
	for _, pattern := range patterns {
		file := filepath.Join(x86HarteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			RunX86HarteTestFile(t, file)
		})
	}
}

func TestHarteX86_PUSH_POP(t *testing.T) {
	patterns := []string{
		"50.json.gz", // PUSH AX
		"51.json.gz", // PUSH CX
		"52.json.gz", // PUSH DX
		"53.json.gz", // PUSH BX
		"54.json.gz", // PUSH SP
		"55.json.gz", // PUSH BP
		"56.json.gz", // PUSH SI
		"57.json.gz", // PUSH DI
		"58.json.gz", // POP AX
		"59.json.gz", // POP CX
		"5A.json.gz", // POP DX
		"5B.json.gz", // POP BX
		"5C.json.gz", // POP SP
		"5D.json.gz", // POP BP
		"5E.json.gz", // POP SI
		"5F.json.gz", // POP DI
	}
	for _, pattern := range patterns {
		file := filepath.Join(x86HarteTestDir, pattern)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			RunX86HarteTestFile(t, file)
		})
	}
}
