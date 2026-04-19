// jit_exec_test.go - JIT dispatcher integration tests

//go:build (amd64 && (linux || windows)) || (arm64 && (linux || windows))

package main

import (
	"testing"
	"time"
)

// runJITProgram loads instructions, enables JIT, runs until halt or timeout.
func runJITProgram(t *testing.T, instructions ...[]byte) *CPU64 {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true

	offset := uint32(PROG_START)
	for _, instr := range instructions {
		copy(cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}
	// Add HALT
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("JIT execution timed out")
	}
	return cpu
}

// runInterpreterProgram loads instructions, runs with interpreter until halt or timeout.
func runInterpreterProgram(t *testing.T, instructions ...[]byte) *CPU64 {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	offset := uint32(PROG_START)
	for _, instr := range instructions {
		copy(cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	done := make(chan struct{})
	go func() {
		cpu.Execute()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("interpreter execution timed out")
	}
	return cpu
}

func TestJIT_SingleALU(t *testing.T) {
	// MOVE.Q R1, #100; ADD.Q R1, R1, #50; HALT
	cpu := runJITProgram(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 50),
	)
	if cpu.regs[1] != 150 {
		t.Fatalf("R1 = %d, want 150", cpu.regs[1])
	}
}

func TestJIT_MultiBlock(t *testing.T) {
	// Block 1: MOVE R1, #10; BRA +16 (skip MOVE R1, #99)
	// Block 2: ADD R1, R1, #5; HALT (at PROG_START+24)
	// The BRA skips over the MOVE #99 so R1 = 10 + 5 = 15
	cpu := runJITProgram(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 10), // PROG_START+0
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 16),            // PROG_START+8, target=+24
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 99), // PROG_START+16 (skipped)
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 5),   // PROG_START+24 (block 2)
	)
	// HALT at PROG_START+32 (added by helper)
	if cpu.regs[1] != 15 {
		t.Fatalf("R1 = %d, want 15", cpu.regs[1])
	}
}

func TestJIT_FallbackToInterpreter(t *testing.T) {
	// NOP followed by a HALT — the NOP block compiles, then HALT uses interpreter fallback
	cpu := runJITProgram(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42),
		ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0),
	)
	if cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", cpu.regs[1])
	}
}

func TestJIT_Reset(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true

	// Write a simple program
	offset := uint32(PROG_START)
	copy(cpu.memory[offset:], ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42))
	offset += IE64_INSTR_SIZE
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Run to populate cache
	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("timeout")
	}

	if cpu.regs[1] != 42 {
		t.Fatalf("first run: R1 = %d, want 42", cpu.regs[1])
	}

	// Reset clears JIT cache
	cpu.Reset()
	if cpu.jitCache != nil && cpu.jitCache.Get(PROG_START) != nil {
		t.Fatal("JIT cache should be empty after Reset")
	}
}

func TestJIT_vs_Interpreter_Dispatcher_ALU(t *testing.T) {
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 200),
		ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 0, 1, 2, 0),  // R3 = R1 + R2 = 300
		ie64Instr(OP_SUB, 4, IE64_SIZE_Q, 0, 2, 1, 0),  // R4 = R2 - R1 = 100
		ie64Instr(OP_MULU, 5, IE64_SIZE_Q, 1, 1, 0, 3), // R5 = R1 * 3 = 300
	}

	jitCPU := runJITProgram(t, instrs...)
	interpCPU := runInterpreterProgram(t, instrs...)

	for i := 0; i < 32; i++ {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Errorf("R%d: JIT=0x%X, Interp=0x%X", i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
}

func TestJIT_vs_Interpreter_Dispatcher_Memory(t *testing.T) {
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xBEEF),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0x4000), // address
		ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 2, 0, 0),     // mem[0x4000] = 0xBEEF
		ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 1, 2, 0, 0),      // R3 = mem[0x4000]
	}

	jitCPU := runJITProgram(t, instrs...)
	interpCPU := runInterpreterProgram(t, instrs...)

	for i := 0; i < 32; i++ {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Errorf("R%d: JIT=0x%X, Interp=0x%X", i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
}

func TestJIT_vs_Interpreter_Dispatcher_Branches(t *testing.T) {
	// Program: R1 = 10, R2 = 10, if R1 == R2 then R3 = 1 else R3 = 2
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 10),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 10),
		ie64Instr(OP_BEQ, 0, IE64_SIZE_Q, 0, 1, 2, 16), // BEQ Rs=R1, Rt=R2, skip 2 instrs
		ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 2), // not taken
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 8),            // skip over next
		ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 1), // taken path
	}

	jitCPU := runJITProgram(t, instrs...)
	interpCPU := runInterpreterProgram(t, instrs...)

	if jitCPU.regs[3] != interpCPU.regs[3] {
		t.Errorf("R3: JIT=%d, Interp=%d", jitCPU.regs[3], interpCPU.regs[3])
	}
	if jitCPU.regs[3] != 1 {
		t.Errorf("R3 = %d, want 1 (BEQ taken)", jitCPU.regs[3])
	}
}

func TestJIT_vs_Interpreter_Dispatcher_Subroutines(t *testing.T) {
	// Program: JSR to subroutine that sets R1=42, RTS back, set R2=99
	// Layout: +0=JSR, +8=MOVE R2, +16=BRA, +24=MOVE R1, +32=RTS, +40=HALT
	// Branch semantics: target = PC + displacement (no implicit +8)
	instrs := [][]byte{
		ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 24),          // JSR: PC=+0, target=+24 (MOVE R1)
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 99), // +8: after return: R2=99
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 24),            // +16: target=+40 (HALT)
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42), // +24: subroutine body
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),           // +32: RTS → returns to +8
	}

	jitCPU := runJITProgram(t, instrs...)
	interpCPU := runInterpreterProgram(t, instrs...)

	for i := 0; i < 32; i++ {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Errorf("R%d: JIT=0x%X, Interp=0x%X", i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
	if jitCPU.regs[1] != 42 {
		t.Errorf("R1 = %d, want 42", jitCPU.regs[1])
	}
	if jitCPU.regs[2] != 99 {
		t.Errorf("R2 = %d, want 99", jitCPU.regs[2])
	}
}

func TestJIT_TightLoop(t *testing.T) {
	// R1 = 1000, loop: R1 = R1 - 1, BNE R1, R0, -8 (loop back to SUB)
	neg8 := int32(-8)
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 1000),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),            // R1 -= 1
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, uint32(neg8)), // if R1 != 0 goto SUB
	}

	cpu := runJITProgram(t, instrs...)
	if cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 after tight loop", cpu.regs[1])
	}
}

func TestJIT_IOFallback_MMIO(t *testing.T) {
	// Register a custom MMIO region and verify that JIT STORE/LOAD
	// bail to the interpreter and trigger the I/O callbacks.
	const testIOAddr = 0xF0700 // Terminal MMIO range
	bus := NewMachineBus()

	var writeVal uint32
	var writeCalled bool
	bus.MapIO(testIOAddr, testIOAddr+4,
		func(addr uint32) uint32 {
			return 0x1234
		},
		func(addr uint32, value uint32) {
			writeCalled = true
			writeVal = value
		},
	)

	cpu := NewCPU64(bus)
	cpu.jitEnabled = true

	// Program: MOVE R1, #0x42; MOVE R2, #testIOAddr; STORE.L R1, 0(R2); LOAD.L R3, 0(R2); HALT
	offset := uint32(PROG_START)
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x42),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, uint32(testIOAddr)),
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 1, 2, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 1, 2, 0, 0),
	}
	for _, instr := range instrs {
		copy(cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("JIT execution timed out")
	}

	if !writeCalled {
		t.Fatal("MMIO write callback was not called")
	}
	if writeVal != 0x42 {
		t.Errorf("MMIO write value = 0x%X, want 0x42", writeVal)
	}
	if cpu.regs[3] != 0x1234 {
		t.Errorf("MMIO read: R3 = 0x%X, want 0x1234", cpu.regs[3])
	}
}

func TestJIT_VRAM_DirectAccess(t *testing.T) {
	// VRAM (0x100000+) is above IO_REGION_START but has no I/O mappings,
	// so the JIT should use direct memory access (not bail to interpreter).
	const vramAddr = 0x100000
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, uint32(vramAddr)),
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 1, 2, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 1, 2, 0, 0),
	}

	cpu := runJITProgram(t, instrs...)
	if cpu.regs[3] != 0xDEAD {
		t.Errorf("VRAM round-trip: R3 = 0x%X, want 0xDEAD", cpu.regs[3])
	}
}

func TestJIT_SelfModifyingCode(t *testing.T) {
	// Write to the code region should set NeedInval and invalidate the cache.
	// Program: MOVE R1, #0; MOVE R2, #PROG_START; STORE.L R1, 0(R2); HALT
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, PROG_START),
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 1, 2, 0, 0),
	}

	cpu := runJITProgram(t, instrs...)
	// After the STORE to code region, the cache should have been invalidated.
	// The important thing is that the program completes without crashing.
	_ = cpu
}

func TestJIT_vs_Interpreter_BackwardBranch(t *testing.T) {
	// Loop program: R1=1000, R2=0; loop: R2+=R1; R1-=1; BNE back to ADD.
	// Result: R1=0, R2=sum(1..1000)=500500.
	neg16 := uint32(0xFFFFFFF0) // -16 as uint32
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 1000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 0, 2, 1, 0),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, neg16),
	}

	jitCPU := runJITProgram(t, instrs...)
	interpCPU := runInterpreterProgram(t, instrs...)

	for i := 0; i < 32; i++ {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Errorf("R%d: JIT=0x%X, Interp=0x%X", i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
	if jitCPU.regs[1] != 0 {
		t.Errorf("R1 = %d, want 0", jitCPU.regs[1])
	}
	if jitCPU.regs[2] != 500500 {
		t.Errorf("R2 = %d, want 500500", jitCPU.regs[2])
	}
}

// TestJIT_FCVTFI_Bail verifies that FCVTFI (float→int) correctly bails to
// the interpreter through the full dispatcher loop. On x86-64 this exercises
// the NeedIOFallback handoff; on ARM64 it runs natively but should produce
// the same result.
func TestJIT_FCVTFI_Bail(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true

	// Set F2 = 42.0 (0x42280000)
	cpu.FPU.FPRegs[2] = 0x42280000

	// Program: FCVTFI R1, F2; HALT
	// FCVTFI is the first instruction — on x86-64 this hits needsFallback=false
	// (only HALT/WAIT/RTI trigger needsFallback at block entry), so the JIT
	// compiles it but emits a bail. The dispatcher re-executes via interpretOne().
	offset := uint32(PROG_START)
	copy(cpu.memory[offset:], ie64Instr(OP_FCVTFI, 1, 0, 0, 2, 0, 0))
	offset += 8
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("timed out")
	}

	if cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", cpu.regs[1])
	}
}

// TestJIT_FCVTFI_Saturate verifies FCVTFI overflow saturation through the
// dispatcher. Positive overflow should produce MaxInt32 (2147483647).
func TestJIT_FCVTFI_Saturate(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true

	// F2 = 1e20 (way above MaxInt32) → bit pattern 0x60AD78EC
	cpu.FPU.FPRegs[2] = 0x60AD78EC

	offset := uint32(PROG_START)
	copy(cpu.memory[offset:], ie64Instr(OP_FCVTFI, 1, 0, 0, 2, 0, 0))
	offset += 8
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("timed out")
	}

	// Interpreter saturates positive overflow to MaxInt32
	want := uint64(0x7FFFFFFF) // MaxInt32
	if cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%X, want 0x%X (MaxInt32 saturation)", cpu.regs[1], want)
	}

	// IO exception flag should be set
	if cpu.FPU.FPSR&IE64_FPU_EX_IO == 0 {
		t.Fatal("FPSR IO exception flag should be set after overflow")
	}
}

// TestJIT_FINT_Bail verifies that FINT (round to integer) correctly bails
// to the interpreter and produces the right result through the dispatcher.
func TestJIT_FINT_Bail(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true

	// F2 = 2.7 (0x402CCCCD), FPCR mode 0 = nearest → F1 = 3.0 (0x40400000)
	cpu.FPU.FPRegs[2] = 0x402CCCCD
	cpu.FPU.FPCR = 0 // nearest rounding

	offset := uint32(PROG_START)
	copy(cpu.memory[offset:], ie64Instr(OP_FINT, 1, 0, 0, 2, 0, 0))
	offset += 8
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("timed out")
	}

	if cpu.FPU.FPRegs[1] != 0x40400000 {
		t.Fatalf("F1 = 0x%08X, want 0x40400000 (3.0, nearest rounding)", cpu.FPU.FPRegs[1])
	}
}
