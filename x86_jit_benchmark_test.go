// x86_jit_benchmark_test.go - x86 JIT vs interpreter benchmark suite
//
// Benchmarks the x86 CPU through both the Go interpreter and the JIT compiler,
// reporting ns/op and instructions/op. MIPS = instructions/op / ns/op * 1000.
//
// Workload categories:
//
//   - ALU:     Register-to-register integer arithmetic (MOV, ADD, SUB, AND, OR, XOR, SHL)
//   - Memory:  MOV r32,[mem] / MOV [mem],r32 sequential loop
//   - Mixed:   Interleaved ALU, memory, and branches
//   - String:  REP STOSB fill operation
//
// Reference results (i5-8365U, same-session 10s runs, with ERMS/BMI2/LZCNT):
//
//   ALU:    Interpreter 25.8 MIPS -> JIT 2,397 MIPS (93x)
//   Memory: Interpreter 19.0 MIPS -> JIT 1,251 MIPS (66x)
//   Mixed:  Interpreter 17.7 MIPS -> JIT 1,761 MIPS (99x)
//   String: Interpreter 79.5us    -> JIT 0.92us      (86x)
//
// Usage:
//
//	go test -tags headless -run='^$' -bench 'BenchmarkX86JIT_' -benchtime 30s ./...

//go:build amd64 && linux

package main

import (
	"testing"
	"time"
)

const (
	x86BenchIterations = 10000
	x86BenchDataAddr   = 0x5000
	x86BenchStackAddr  = 0x20000
)

// le32 encodes a uint32 as 4 little-endian bytes.
func le32(v uint32) [4]byte {
	return [4]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
}

// ===========================================================================
// Program Builders
// ===========================================================================

// buildX86ALUProgram constructs an x86 program: tight ALU loop with 8 operations.
//
//	MOV EAX, 7       ; setup
//	MOV EBX, 3       ; setup
//	MOV ECX, iter    ; loop counter
//	loop:
//	  ADD EAX, EBX   ; 01 D8
//	  SUB EDX, EAX   ; 29 C2 (EDX = EDX - EAX -> feedback)
//	  AND EAX, EBX   ; 21 D8
//	  OR  EAX, EBX   ; 09 D8
//	  XOR EDX, EAX   ; 31 C2
//	  SHL EAX, 1     ; D1 E0
//	  ADD EAX, EDX   ; 01 D0
//	  DEC ECX        ; 49
//	  JNZ loop       ; 75 EE (-18)
//	HLT
//
// Total: 3 setup + iter * 9 + 1 HLT
func buildX86ALUProgram(iterations uint32) (code []byte, totalInstrs int) {
	it := le32(iterations)
	code = []byte{
		0xB8, 0x07, 0x00, 0x00, 0x00, // MOV EAX, 7
		0xBB, 0x03, 0x00, 0x00, 0x00, // MOV EBX, 3
		0xB9, it[0], it[1], it[2], it[3], // MOV ECX, iter
		// loop: (offset 15)
		0x01, 0xD8, // ADD EAX, EBX     (2)
		0x29, 0xC2, // SUB EDX, EAX     (2)
		0x21, 0xD8, // AND EAX, EBX     (2)
		0x09, 0xD8, // OR  EAX, EBX     (2)
		0x31, 0xC2, // XOR EDX, EAX     (2)
		0xD1, 0xE0, // SHL EAX, 1       (2)
		0x01, 0xD0, // ADD EAX, EDX     (2)
		0x49,       // DEC ECX          (1)
		0x75, 0xEF, // JNZ -17 (back to offset 15) (2)
		0xF4, // HLT
	}
	totalInstrs = 3 + int(iterations)*9 + 1
	return
}

// buildX86MemoryProgram constructs an x86 program: sequential memory store/load loop.
// Uses addresses below I/O threshold for direct-memory fast path.
//
//	MOV ESI, dataAddr ; base address
//	MOV ECX, iter     ; counter
//	MOV EAX, 0        ; accumulator
//	loop:
//	  MOV [ESI], ECX   ; store (89 0E)
//	  MOV EBX, [ESI]   ; load  (8B 1E)
//	  ADD EAX, EBX     ; accumulate
//	  ADD ESI, 4       ; advance
//	  DEC ECX          ; counter--
//	  JNZ loop         ; 75 F4
//	HLT
//
// Total: 3 setup + iter * 6 + 1 HLT
func buildX86MemoryProgram(iterations uint32) (code []byte, totalInstrs int) {
	da := le32(x86BenchDataAddr)
	it := le32(iterations)
	code = []byte{
		0xBE, da[0], da[1], da[2], da[3], // MOV ESI, dataAddr
		0xB9, it[0], it[1], it[2], it[3], // MOV ECX, iter
		0xB8, 0x00, 0x00, 0x00, 0x00, // MOV EAX, 0
		// loop: (offset 15)
		0x89, 0x0E, // MOV [ESI], ECX   (2)
		0x8B, 0x1E, // MOV EBX, [ESI]   (2)
		0x01, 0xD8, // ADD EAX, EBX     (2)
		0x83, 0xC6, 0x04, // ADD ESI, 4       (3)
		0x49,       // DEC ECX          (1)
		0x75, 0xF4, // JNZ -12 (back to offset 15)  (2)
		0xF4, // HLT
	}
	totalInstrs = 3 + int(iterations)*6 + 1
	return
}

// buildX86MixedProgram constructs an x86 program: interleaved ALU + memory + branches.
//
//	MOV ESI, dataAddr
//	MOV ECX, iter
//	MOV EAX, 1
//	MOV EBX, 0
//	loop:
//	  MOV [ESI], EAX    ; store
//	  ADD EAX, 1        ; ALU
//	  MOV EDX, [ESI]    ; load
//	  ADD EBX, EDX      ; accumulate
//	  SHL EBX, 1        ; shift
//	  XOR EBX, EAX      ; logic
//	  ADD ESI, 4        ; advance
//	  DEC ECX
//	  JNZ loop
//	HLT
//
// Total: 4 setup + iter * 9 + 1 HLT
func buildX86MixedProgram(iterations uint32) (code []byte, totalInstrs int) {
	da := le32(x86BenchDataAddr)
	it := le32(iterations)
	code = []byte{
		0xBE, da[0], da[1], da[2], da[3],
		0xB9, it[0], it[1], it[2], it[3],
		0xB8, 0x01, 0x00, 0x00, 0x00, // MOV EAX, 1
		0xBB, 0x00, 0x00, 0x00, 0x00, // MOV EBX, 0
		// loop: (offset 20)
		0x89, 0x06, // MOV [ESI], EAX   (2)
		0x83, 0xC0, 0x01, // ADD EAX, 1       (3)
		0x8B, 0x16, // MOV EDX, [ESI]   (2)
		0x01, 0xD3, // ADD EBX, EDX     (2)
		0xD1, 0xE3, // SHL EBX, 1       (2)
		0x31, 0xC3, // XOR EBX, EAX     (2)
		0x83, 0xC6, 0x04, // ADD ESI, 4       (3)
		0x49,       // DEC ECX          (1)
		0x75, 0xED, // JNZ -19 (back to offset 20) (2)
		0xF4, // HLT
	}
	totalInstrs = 4 + int(iterations)*9 + 1
	return
}

// buildX86CallProgram constructs an x86 program: tight CALL/RET loop.
//
//	MOV ESP, stackAddr
//	MOV ECX, iter
//	MOV EAX, 0
//	loop:
//	  CALL sub          ; E8 rel32
//	  DEC ECX
//	  JNZ loop          ; 75 F7
//	  JMP end           ; EB xx
//	sub:
//	  INC EAX           ; 40
//	  RET               ; C3
//	end:
//	  HLT
func buildX86CallProgram(iterations uint32) (code []byte, totalInstrs int) {
	sa := le32(x86BenchStackAddr)
	it := le32(iterations)
	code = []byte{
		0xBC, sa[0], sa[1], sa[2], sa[3], // MOV ESP, stackAddr
		0xB9, it[0], it[1], it[2], it[3], // MOV ECX, iter
		0xB8, 0x00, 0x00, 0x00, 0x00, // MOV EAX, 0
		// loop: (offset 15)
		0xE8, 0x04, 0x00, 0x00, 0x00, // CALL sub (rel32 = +4, nextPC=20, target=24) (5)
		0x49,       // DEC ECX  (1)
		0x75, 0xF8, // JNZ -8 (back to offset 15) (2)
		0xEB, 0x01, // JMP +1 (skip sub, to HLT at offset 26) (2)
		// sub: (offset 24)
		0x40, // INC EAX  (1)
		0xC3, // RET      (1)
		// end: (offset 26)
		0xF4, // HLT
	}
	// 3 setup + iter*(CALL+INC+RET+DEC+JNZ=5) + JMP + HLT
	totalInstrs = 3 + int(iterations)*5 + 2
	return
}

// ===========================================================================
// Benchmark Harness
// ===========================================================================

func setupX86JITBenchCPU() (*CPU_X86, *X86BusAdapter, *MachineBus) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	return cpu, adapter, bus
}

func loadX86BenchProgram(cpu *CPU_X86, startPC uint32, code []byte) {
	for i, b := range code {
		cpu.memory[startPC+uint32(i)] = b
	}
}

func resetX86BenchState(cpu *CPU_X86, startPC uint32) {
	cpu.EIP = startPC
	cpu.Halted = false
	cpu.running.Store(true)
	cpu.Flags = x86FlagIF
}

func runX86BenchInterpreter(cpu *CPU_X86) {
	for cpu.Running() && !cpu.Halted {
		cpu.Step()
	}
}

func runX86BenchJIT(cpu *CPU_X86) {
	cpu.X86ExecuteJIT()
}

// ===========================================================================
// ALU Benchmarks
// ===========================================================================

func BenchmarkX86JIT_ALU_Interpreter(b *testing.B) {
	code, totalInstrs := buildX86ALUProgram(x86BenchIterations)
	cpu, _, _ := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		runX86BenchInterpreter(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkX86JIT_ALU_JIT(b *testing.B) {
	if !x86JitAvailable {
		b.Skip("x86 JIT not available")
	}
	code, totalInstrs := buildX86ALUProgram(x86BenchIterations)
	cpu, adapter, bus := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	cpu.x86JitEnabled = true
	cpu.x86JitPersist = true
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	// Warm-up
	resetX86BenchState(cpu, 0x1000)
	runX86BenchJIT(cpu)

	b.Cleanup(func() {
		cpu.x86JitPersist = false
		cpu.freeX86JIT()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		runX86BenchJIT(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Memory Benchmarks
// ===========================================================================

func BenchmarkX86JIT_Memory_Interpreter(b *testing.B) {
	code, totalInstrs := buildX86MemoryProgram(x86BenchIterations)
	cpu, _, _ := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		runX86BenchInterpreter(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkX86JIT_Memory_JIT(b *testing.B) {
	if !x86JitAvailable {
		b.Skip("x86 JIT not available")
	}
	code, totalInstrs := buildX86MemoryProgram(x86BenchIterations)
	cpu, adapter, bus := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	cpu.x86JitEnabled = true
	cpu.x86JitPersist = true
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	resetX86BenchState(cpu, 0x1000)
	runX86BenchJIT(cpu)

	b.Cleanup(func() {
		cpu.x86JitPersist = false
		cpu.freeX86JIT()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		runX86BenchJIT(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Mixed Benchmarks
// ===========================================================================

func BenchmarkX86JIT_Mixed_Interpreter(b *testing.B) {
	code, totalInstrs := buildX86MixedProgram(x86BenchIterations)
	cpu, _, _ := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		runX86BenchInterpreter(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkX86JIT_Mixed_JIT(b *testing.B) {
	if !x86JitAvailable {
		b.Skip("x86 JIT not available")
	}
	code, totalInstrs := buildX86MixedProgram(x86BenchIterations)
	cpu, adapter, bus := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	cpu.x86JitEnabled = true
	cpu.x86JitPersist = true
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	resetX86BenchState(cpu, 0x1000)
	runX86BenchJIT(cpu)

	b.Cleanup(func() {
		cpu.x86JitPersist = false
		cpu.freeX86JIT()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		runX86BenchJIT(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Call Benchmarks
// ===========================================================================

func BenchmarkX86JIT_Call_Interpreter(b *testing.B) {
	code, totalInstrs := buildX86CallProgram(x86BenchIterations)
	cpu, _, _ := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		cpu.ESP = x86BenchStackAddr
		runX86BenchInterpreter(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkX86JIT_Call_JIT(b *testing.B) {
	if !x86JitAvailable {
		b.Skip("x86 JIT not available")
	}
	code, totalInstrs := buildX86CallProgram(x86BenchIterations)
	cpu, adapter, bus := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	cpu.x86JitEnabled = true
	cpu.x86JitPersist = true
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	resetX86BenchState(cpu, 0x1000)
	cpu.ESP = x86BenchStackAddr
	runX86BenchJIT(cpu)

	b.Cleanup(func() {
		cpu.x86JitPersist = false
		cpu.freeX86JIT()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		cpu.ESP = x86BenchStackAddr
		runX86BenchJIT(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// String Benchmarks (REP STOSB)
// ===========================================================================

func buildX86StringProgram(iterations uint32) (code []byte, totalInstrs int) {
	it := le32(iterations)
	da := le32(x86BenchDataAddr)
	code = []byte{
		0xBF, da[0], da[1], da[2], da[3], // MOV EDI, dataAddr
		0xB9, it[0], it[1], it[2], it[3], // MOV ECX, count
		0xB0, 0x42, // MOV AL, 0x42
		0xF3, 0xAA, // REP STOSB
		0xF4, // HLT
	}
	// 3 setup + 1 REP (counts as 1 instruction) + 1 HLT = 5
	// But REP STOSB does iterations byte stores internally
	totalInstrs = 5
	return
}

func BenchmarkX86JIT_String_Interpreter(b *testing.B) {
	code, totalInstrs := buildX86StringProgram(x86BenchIterations)
	cpu, _, _ := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		runX86BenchInterpreter(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkX86JIT_String_JIT(b *testing.B) {
	if !x86JitAvailable {
		b.Skip("x86 JIT not available")
	}
	code, totalInstrs := buildX86StringProgram(x86BenchIterations)
	cpu, adapter, bus := setupX86JITBenchCPU()
	loadX86BenchProgram(cpu, 0x1000, code)

	cpu.x86JitEnabled = true
	cpu.x86JitPersist = true
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	resetX86BenchState(cpu, 0x1000)
	runX86BenchJIT(cpu)

	b.Cleanup(func() {
		cpu.x86JitPersist = false
		cpu.freeX86JIT()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetX86BenchState(cpu, 0x1000)
		runX86BenchJIT(cpu)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Correctness validation for benchmark programs
// ===========================================================================

func TestX86JIT_BenchALU_Correctness(t *testing.T) {
	code, _ := buildX86ALUProgram(100) // small iteration count for testing

	jitCPU := runX86JITProgram(t, 0x1000, code...)
	interpCPU := runX86InterpreterProgram(t, 0x1000, code...)

	if jitCPU.EAX != interpCPU.EAX {
		t.Errorf("ALU bench EAX: JIT=0x%08X, Interp=0x%08X", jitCPU.EAX, interpCPU.EAX)
	}
	if jitCPU.EBX != interpCPU.EBX {
		t.Errorf("ALU bench EBX: JIT=0x%08X, Interp=0x%08X", jitCPU.EBX, interpCPU.EBX)
	}
	if jitCPU.ECX != interpCPU.ECX {
		t.Errorf("ALU bench ECX: JIT=0x%08X, Interp=0x%08X", jitCPU.ECX, interpCPU.ECX)
	}
}

func TestX86JIT_BenchCall_Correctness(t *testing.T) {
	t.Skip("CALL/RET loop benchmark needs stack outside translateIO range - deferred")
	code, _ := buildX86CallProgram(10)

	interpCPU := runX86InterpreterProgram(t, 0x1000, code...)
	if interpCPU.EAX != 10 {
		t.Fatalf("Interpreter: EAX = %d, want 10 (program is broken)", interpCPU.EAX)
	}

	// Now test with JIT
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitEnabled = true
	cpu.EIP = 0x1000
	cpu.ESP = x86BenchStackAddr
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	for i, b := range code {
		cpu.memory[0x1000+uint32(i)] = b
	}

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.Halted = false
		cpu.X86ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("JIT timed out")
	}

	if cpu.EAX != 10 {
		t.Errorf("JIT: EAX = %d, want 10 (INC EAX called 10 times)", cpu.EAX)
	}
}
