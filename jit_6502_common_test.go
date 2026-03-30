package main

import (
	"testing"
	"unsafe"
)

// ===========================================================================
// Context Layout Tests
// ===========================================================================

func TestJIT6502_ContextFieldOffsets(t *testing.T) {
	var ctx JIT6502Context
	base := uintptr(unsafe.Pointer(&ctx))

	tests := []struct {
		name     string
		got      uintptr
		expected uintptr
	}{
		{"MemPtr", uintptr(unsafe.Pointer(&ctx.MemPtr)) - base, j65CtxOffMemPtr},
		{"IOBitmapPtr", uintptr(unsafe.Pointer(&ctx.IOBitmapPtr)) - base, j65CtxOffIOBitmapPtr},
		{"CpuPtr", uintptr(unsafe.Pointer(&ctx.CpuPtr)) - base, j65CtxOffCpuPtr},
		{"CodePageBitmapPtr", uintptr(unsafe.Pointer(&ctx.CodePageBitmapPtr)) - base, j65CtxOffCodePageBitmap},
		{"RetCycles", uintptr(unsafe.Pointer(&ctx.RetCycles)) - base, j65CtxOffRetCycles},
		{"NeedBail", uintptr(unsafe.Pointer(&ctx.NeedBail)) - base, j65CtxOffNeedBail},
		{"NeedInval", uintptr(unsafe.Pointer(&ctx.NeedInval)) - base, j65CtxOffNeedInval},
		{"RetPC", uintptr(unsafe.Pointer(&ctx.RetPC)) - base, j65CtxOffRetPC},
		{"RetCount", uintptr(unsafe.Pointer(&ctx.RetCount)) - base, j65CtxOffRetCount},
		{"FastPathLimit", uintptr(unsafe.Pointer(&ctx.FastPathLimit)) - base, j65CtxOffFastPathLimit},
		{"ChainBudget", uintptr(unsafe.Pointer(&ctx.ChainBudget)) - base, j65CtxOffChainBudget},
		{"ChainCount", uintptr(unsafe.Pointer(&ctx.ChainCount)) - base, j65CtxOffChainCount},
		{"InvalPage", uintptr(unsafe.Pointer(&ctx.InvalPage)) - base, j65CtxOffInvalPage},
		{"RTSCache0PC", uintptr(unsafe.Pointer(&ctx.RTSCache0PC)) - base, j65CtxOffRTSCache0PC},
		{"RTSCache0Addr", uintptr(unsafe.Pointer(&ctx.RTSCache0Addr)) - base, j65CtxOffRTSCache0Addr},
		{"RTSCache1PC", uintptr(unsafe.Pointer(&ctx.RTSCache1PC)) - base, j65CtxOffRTSCache1PC},
		{"RTSCache1Addr", uintptr(unsafe.Pointer(&ctx.RTSCache1Addr)) - base, j65CtxOffRTSCache1Addr},
		{"DirectPageBitmapPtr", uintptr(unsafe.Pointer(&ctx.DirectPageBitmapPtr)) - base, j65CtxOffDirectPageBitmapPtr},
	}

	for _, tt := range tests {
		if tt.got != tt.expected {
			t.Errorf("JIT6502Context.%s: offset = %d, want %d", tt.name, tt.got, tt.expected)
		}
	}
}

func TestJIT6502_CPUStructFieldOffsets(t *testing.T) {
	var cpu CPU_6502
	base := uintptr(unsafe.Pointer(&cpu))

	tests := []struct {
		name     string
		got      uintptr
		expected uintptr
	}{
		{"PC", uintptr(unsafe.Pointer(&cpu.PC)) - base, cpu6502OffPC},
		{"SP", uintptr(unsafe.Pointer(&cpu.SP)) - base, cpu6502OffSP},
		{"A", uintptr(unsafe.Pointer(&cpu.A)) - base, cpu6502OffA},
		{"X", uintptr(unsafe.Pointer(&cpu.X)) - base, cpu6502OffX},
		{"Y", uintptr(unsafe.Pointer(&cpu.Y)) - base, cpu6502OffY},
		{"SR", uintptr(unsafe.Pointer(&cpu.SR)) - base, cpu6502OffSR},
	}

	for _, tt := range tests {
		if tt.got != tt.expected {
			t.Errorf("CPU_6502.%s: offset = %d, want %d", tt.name, tt.got, tt.expected)
		}
	}
}

func TestJIT6502_ContextConstruction(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	ctx := newJIT6502Context(cpu)
	if ctx == nil {
		t.Fatal("newJIT6502Context returned nil")
	}

	if ctx.MemPtr == 0 {
		t.Error("MemPtr is zero")
	}
	if ctx.CpuPtr == 0 {
		t.Error("CpuPtr is zero")
	}
	if ctx.IOBitmapPtr == 0 {
		t.Error("IOBitmapPtr is zero")
	}
	if ctx.CodePageBitmapPtr == 0 {
		t.Error("CodePageBitmapPtr is zero")
	}
	if ctx.FastPathLimit != jit6502FastPathLimit {
		t.Errorf("FastPathLimit = 0x%X, want 0x%X", ctx.FastPathLimit, jit6502FastPathLimit)
	}
	if ctx.DirectPageBitmapPtr == 0 {
		t.Error("DirectPageBitmapPtr is zero")
	}
}

// ===========================================================================
// DirectPageBitmap Tests
// ===========================================================================

func TestJIT6502_DirectPageBitmap_Ranges(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)
	cpu.initDirectPageBitmap()

	// Pages $00-$1F should be direct (low RAM)
	for page := 0; page < 0x20; page++ {
		if cpu.directPageBitmap[page] != 0 {
			t.Errorf("page $%02X should be direct (low RAM), got bail", page)
		}
	}

	// Pages $20-$7F should bail (bank windows)
	for page := 0x20; page <= 0x7F; page++ {
		if cpu.directPageBitmap[page] != 1 {
			t.Errorf("page $%02X should bail (bank window), got direct", page)
		}
	}

	// Pages $80-$BF should bail (VRAM window)
	for page := 0x80; page <= 0xBF; page++ {
		if cpu.directPageBitmap[page] != 1 {
			t.Errorf("page $%02X should bail (VRAM window), got direct", page)
		}
	}

	// Pages $C0-$CF should be direct (RAM above VRAM)
	for page := 0xC0; page <= 0xCF; page++ {
		if cpu.directPageBitmap[page] != 0 {
			t.Errorf("page $%02X should be direct (RAM above VRAM), got bail", page)
		}
	}

	// I/O pages (ioTable handlers) should bail
	ioPages := []int{0xD2, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xF7}
	for _, page := range ioPages {
		if cpu.directPageBitmap[page] != 1 {
			t.Errorf("page $%02X should bail (I/O handler), got direct", page)
		}
	}

	// Non-I/O pages in $D0-$EF should be direct
	nonIOPages := []int{0xD0, 0xD1, 0xD3, 0xD9, 0xDA, 0xE0, 0xEF}
	for _, page := range nonIOPages {
		if cpu.directPageBitmap[page] != 0 {
			t.Errorf("page $%02X should be direct (non-I/O), got bail", page)
		}
	}

	// Pages $F0-$FF should bail (I/O translation region)
	for page := 0xF0; page <= 0xFF; page++ {
		if cpu.directPageBitmap[page] != 1 {
			t.Errorf("page $%02X should bail (I/O translation), got direct", page)
		}
	}
}

// ===========================================================================
// Instruction Length Table Tests
// ===========================================================================

func TestJIT6502_InstrLengths_Documented(t *testing.T) {
	// Verify a representative set of documented opcodes
	tests := []struct {
		opcode byte
		name   string
		length byte
	}{
		{0x00, "BRK", 1},
		{0xA9, "LDA imm", 2},
		{0xA5, "LDA zp", 2},
		{0xB5, "LDA zp,X", 2},
		{0xAD, "LDA abs", 3},
		{0xBD, "LDA abs,X", 3},
		{0xB9, "LDA abs,Y", 3},
		{0xA1, "LDA (ind,X)", 2},
		{0xB1, "LDA (ind),Y", 2},
		{0x85, "STA zp", 2},
		{0x8D, "STA abs", 3},
		{0x20, "JSR", 3},
		{0x60, "RTS", 1},
		{0x4C, "JMP abs", 3},
		{0x6C, "JMP ind", 3},
		{0x40, "RTI", 1},
		{0xEA, "NOP", 1},
		{0xD0, "BNE", 2},
		{0xF0, "BEQ", 2},
		{0x48, "PHA", 1},
		{0x68, "PLA", 1},
		{0x08, "PHP", 1},
		{0x28, "PLP", 1},
		{0x0A, "ASL A", 1},
		{0x06, "ASL zp", 2},
		{0x0E, "ASL abs", 3},
		{0x18, "CLC", 1},
		{0x38, "SEC", 1},
		{0xE8, "INX", 1},
		{0xC8, "INY", 1},
		{0xCA, "DEX", 1},
		{0x88, "DEY", 1},
	}

	for _, tt := range tests {
		got := jit6502InstrLengths[tt.opcode]
		if got != tt.length {
			t.Errorf("opcode 0x%02X (%s): length = %d, want %d", tt.opcode, tt.name, got, tt.length)
		}
	}
}

func TestJIT6502_InstrLengths_AllNonZero(t *testing.T) {
	for i := 0; i < 256; i++ {
		if jit6502InstrLengths[i] == 0 {
			t.Errorf("opcode 0x%02X has length 0 (must be 1, 2, or 3)", i)
		}
		if jit6502InstrLengths[i] > 3 {
			t.Errorf("opcode 0x%02X has length %d (must be 1, 2, or 3)", i, jit6502InstrLengths[i])
		}
	}
}

func TestJIT6502_InstrLengths_JAMOpcodes(t *testing.T) {
	// JAM/KIL opcodes are 1 byte (implied addressing)
	jams := []byte{0x02, 0x12, 0x22, 0x32, 0x42, 0x52, 0x62, 0x72, 0x92, 0xB2, 0xD2, 0xF2}
	for _, op := range jams {
		if jit6502InstrLengths[op] != 1 {
			t.Errorf("JAM opcode 0x%02X: length = %d, want 1", op, jit6502InstrLengths[op])
		}
	}
}

// ===========================================================================
// Base Cycle Cost Table Tests
// ===========================================================================

func TestJIT6502_BaseCycles_BranchesAreZero(t *testing.T) {
	// Branch opcodes have 0 base cycles (interpreter's branch() adds them conditionally)
	branches := []byte{0x10, 0x30, 0x50, 0x70, 0x90, 0xB0, 0xD0, 0xF0}
	for _, op := range branches {
		if jit6502BaseCycles[op] != 0 {
			t.Errorf("branch opcode 0x%02X: base cycles = %d, want 0", op, jit6502BaseCycles[op])
		}
	}
}

func TestJIT6502_BaseCycles_JAMsAreZero(t *testing.T) {
	jams := []byte{0x02, 0x12, 0x22, 0x32, 0x42, 0x52, 0x62, 0x72, 0x92, 0xB2, 0xD2, 0xF2}
	for _, op := range jams {
		if jit6502BaseCycles[op] != 0 {
			t.Errorf("JAM opcode 0x%02X: base cycles = %d, want 0", op, jit6502BaseCycles[op])
		}
	}
}

func TestJIT6502_BaseCycles_Representative(t *testing.T) {
	tests := []struct {
		opcode byte
		name   string
		cycles byte
	}{
		{0x00, "BRK", 7},
		{0xA9, "LDA imm", 2},
		{0xA5, "LDA zp", 3},
		{0xAD, "LDA abs", 4},
		{0xBD, "LDA abs,X", 4},
		{0xB9, "LDA abs,Y", 4},
		{0xA1, "LDA (ind,X)", 6},
		{0xB1, "LDA (ind),Y", 5},
		{0x85, "STA zp", 3},
		{0x8D, "STA abs", 4},
		{0x9D, "STA abs,X", 5},
		{0x99, "STA abs,Y", 5},
		{0x91, "STA (ind),Y", 6},
		{0x20, "JSR", 6},
		{0x60, "RTS", 6},
		{0x40, "RTI", 6},
		{0x4C, "JMP abs", 3},
		{0x6C, "JMP ind", 5},
		{0xEA, "NOP", 2},
		{0x48, "PHA", 3},
		{0x68, "PLA", 4},
		{0x08, "PHP", 3},
		{0x28, "PLP", 4},
		{0x18, "CLC", 2},
		{0x38, "SEC", 2},
		{0xE8, "INX", 2},
		{0xC8, "INY", 2},
		{0xCA, "DEX", 2},
		{0x88, "DEY", 2},
		{0x0A, "ASL A", 2},
		{0x06, "ASL zp", 5},
		{0x0E, "ASL abs", 6},
		{0xE6, "INC zp", 5},
		{0xEE, "INC abs", 6},
		{0xC6, "DEC zp", 5},
		{0xCE, "DEC abs", 6},
		{0x69, "ADC imm", 2},
		{0xE9, "SBC imm", 2},
		{0xC9, "CMP imm", 2},
		{0xE0, "CPX imm", 2},
		{0xC0, "CPY imm", 2},
		{0x24, "BIT zp", 3},
		{0x2C, "BIT abs", 4},
	}

	for _, tt := range tests {
		got := jit6502BaseCycles[tt.opcode]
		if got != tt.cycles {
			t.Errorf("opcode 0x%02X (%s): base cycles = %d, want %d", tt.opcode, tt.name, got, tt.cycles)
		}
	}
}

// ===========================================================================
// Compilable Opcode Table Tests
// ===========================================================================

func TestJIT6502_IsCompilable_DocumentedOpcodes(t *testing.T) {
	// All documented opcodes should be compilable except BRK and RTI
	documented := []byte{
		0x01, 0x05, 0x06, 0x08, 0x09, 0x0A, 0x0D, 0x0E, // ORA, ASL, PHP
		0x10, 0x11, 0x15, 0x16, 0x18, 0x19, 0x1D, 0x1E, // BPL, ORA, CLC
		0x20, 0x21, 0x24, 0x25, 0x26, 0x28, 0x29, 0x2A, 0x2C, 0x2D, 0x2E, // JSR, AND, BIT, ROL, PLP
		0x30, 0x31, 0x35, 0x36, 0x38, 0x39, 0x3D, 0x3E, // BMI, AND, SEC
		0x41, 0x45, 0x46, 0x48, 0x49, 0x4A, 0x4C, 0x4D, 0x4E, // EOR, LSR, PHA, JMP
		0x50, 0x51, 0x55, 0x56, 0x58, 0x59, 0x5D, 0x5E, // BVC, EOR, CLI
		0x60, 0x61, 0x65, 0x66, 0x68, 0x69, 0x6A, 0x6C, 0x6D, 0x6E, // RTS, ADC, ROR, PLA, JMP ind
		0x70, 0x71, 0x75, 0x76, 0x78, 0x79, 0x7D, 0x7E, // BVS, ADC, SEI
		0x81, 0x84, 0x85, 0x86, 0x88, 0x8A, 0x8C, 0x8D, 0x8E, // STA, STY, STX, DEY, TXA
		0x90, 0x91, 0x94, 0x95, 0x96, 0x98, 0x99, 0x9A, 0x9D, // BCC, STA, STY, TYA, TXS
		0xA0, 0xA1, 0xA2, 0xA4, 0xA5, 0xA6, 0xA8, 0xA9, 0xAA, 0xAC, 0xAD, 0xAE, // LDY, LDA, LDX, TAY, TAX
		0xB0, 0xB1, 0xB4, 0xB5, 0xB6, 0xB8, 0xB9, 0xBA, 0xBC, 0xBD, 0xBE, // BCS, LDA, LDY, CLV, TSX
		0xC0, 0xC1, 0xC4, 0xC5, 0xC6, 0xC8, 0xC9, 0xCA, 0xCC, 0xCD, 0xCE, // CPY, CMP, DEC, INY, DEX
		0xD0, 0xD1, 0xD5, 0xD6, 0xD8, 0xD9, 0xDD, 0xDE, // BNE, CMP, CLD
		0xE0, 0xE1, 0xE4, 0xE5, 0xE6, 0xE8, 0xE9, 0xEA, 0xEC, 0xED, 0xEE, // CPX, SBC, INC, INX, NOP
		0xF0, 0xF1, 0xF5, 0xF6, 0xF8, 0xF9, 0xFD, 0xFE, // BEQ, SBC, SED
	}

	for _, op := range documented {
		if !jit6502IsCompilable[op] {
			t.Errorf("documented opcode 0x%02X should be compilable", op)
		}
	}
}

func TestJIT6502_IsCompilable_BRKAndRTI_NotCompilable(t *testing.T) {
	if jit6502IsCompilable[0x00] {
		t.Error("BRK (0x00) should not be compilable")
	}
	if jit6502IsCompilable[0x40] {
		t.Error("RTI (0x40) should not be compilable")
	}
}

func TestJIT6502_IsCompilable_UndocumentedNotCompilable(t *testing.T) {
	undocumented := []byte{
		0x03, 0x07, 0x0B, 0x0F, // SLO, ANC
		0x13, 0x17, 0x1A, 0x1B, 0x1C, 0x1F, // SLO, NOP(undoc)
		0x23, 0x27, 0x2B, 0x2F, // RLA, ANC
		0x33, 0x37, 0x3A, 0x3B, 0x3C, 0x3F, // RLA
		0x43, 0x47, 0x4B, 0x4F, // SRE, ALR
		0x53, 0x57, 0x5A, 0x5B, 0x5C, 0x5F, // SRE
		0x63, 0x67, 0x6B, 0x6F, // RRA, ARR
		0x73, 0x77, 0x7A, 0x7B, 0x7C, 0x7F, // RRA
		0x80, 0x82, 0x83, 0x87, 0x89, 0x8B, 0x8F, // SKB, SAX, XAA
		0x93, 0x97, 0x9B, 0x9C, 0x9E, 0x9F, // SHA, SAX, SHS, SHY, SHX
		0xA3, 0xA7, 0xAB, 0xAF, // LAX
		0xB3, 0xB7, 0xBB, 0xBF, // LAX, LAS
		0xC2, 0xC3, 0xC7, 0xCB, 0xCF, // SKB, DCP, SBX
		0xD3, 0xD7, 0xDA, 0xDB, 0xDC, 0xDF, // DCP
		0xE2, 0xE3, 0xE7, 0xEB, 0xEF, // SKB, ISC, USBC
		0xF3, 0xF7, 0xFA, 0xFB, 0xFC, 0xFF, // ISC
	}

	for _, op := range undocumented {
		if jit6502IsCompilable[op] {
			t.Errorf("undocumented opcode 0x%02X should not be compilable", op)
		}
	}
}

// ===========================================================================
// Block Scanner Tests
// ===========================================================================

func TestJIT6502_ScanBlock_LinearToBRK(t *testing.T) {
	// LDA #$42, STA $10, NOP, BRK
	mem := make([]byte, 0x10000)
	mem[0x0600] = 0xA9 // LDA #$42
	mem[0x0601] = 0x42
	mem[0x0602] = 0x85 // STA $10
	mem[0x0603] = 0x10
	mem[0x0604] = 0xEA // NOP
	mem[0x0605] = 0x00 // BRK

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	if len(instrs) != 4 {
		t.Fatalf("expected 4 instructions, got %d", len(instrs))
	}

	if instrs[0].opcode != 0xA9 || instrs[0].operand != 0x42 || instrs[0].length != 2 {
		t.Errorf("instr[0]: opcode=0x%02X operand=0x%04X length=%d", instrs[0].opcode, instrs[0].operand, instrs[0].length)
	}
	if instrs[1].opcode != 0x85 || instrs[1].operand != 0x10 || instrs[1].length != 2 {
		t.Errorf("instr[1]: opcode=0x%02X operand=0x%04X length=%d", instrs[1].opcode, instrs[1].operand, instrs[1].length)
	}
	if instrs[2].opcode != 0xEA || instrs[2].length != 1 {
		t.Errorf("instr[2]: opcode=0x%02X length=%d", instrs[2].opcode, instrs[2].length)
	}
	if instrs[3].opcode != 0x00 || instrs[3].length != 1 {
		t.Errorf("instr[3]: opcode=0x%02X length=%d", instrs[3].opcode, instrs[3].length)
	}
}

func TestJIT6502_ScanBlock_BranchNotTerminator(t *testing.T) {
	// LDA #$01, BNE +2, NOP, BRK
	mem := make([]byte, 0x10000)
	mem[0x0600] = 0xA9 // LDA #$01
	mem[0x0601] = 0x01
	mem[0x0602] = 0xD0 // BNE +2
	mem[0x0603] = 0x02
	mem[0x0604] = 0xEA // NOP (skipped by branch)
	mem[0x0605] = 0xEA // NOP (branch target)
	mem[0x0606] = 0x00 // BRK

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	// BNE should be inside the block, not a terminator
	if len(instrs) != 5 { // LDA, BNE, NOP, NOP, BRK
		t.Fatalf("expected 5 instructions, got %d", len(instrs))
	}
	if instrs[1].opcode != 0xD0 {
		t.Errorf("instr[1] should be BNE (0xD0), got 0x%02X", instrs[1].opcode)
	}
}

func TestJIT6502_ScanBlock_JMPTerminates(t *testing.T) {
	// NOP, JMP $1234, NOP
	mem := make([]byte, 0x10000)
	mem[0x0600] = 0xEA // NOP
	mem[0x0601] = 0x4C // JMP $1234
	mem[0x0602] = 0x34
	mem[0x0603] = 0x12
	mem[0x0604] = 0xEA // NOP (not reached)

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	if len(instrs) != 2 { // NOP, JMP
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}
	if instrs[1].opcode != 0x4C {
		t.Errorf("instr[1] should be JMP, got 0x%02X", instrs[1].opcode)
	}
	if instrs[1].operand != 0x1234 {
		t.Errorf("JMP operand = 0x%04X, want 0x1234", instrs[1].operand)
	}
}

func TestJIT6502_ScanBlock_StopsBeforeUndocumented(t *testing.T) {
	// LDA #$42, LAX $10 (undocumented 0xA7), NOP
	mem := make([]byte, 0x10000)
	mem[0x0600] = 0xA9 // LDA #$42
	mem[0x0601] = 0x42
	mem[0x0602] = 0xA7 // LAX $10 (undocumented — not compilable, not a terminator)
	mem[0x0603] = 0x10
	mem[0x0604] = 0xEA // NOP

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	// Should stop before the undocumented LAX
	if len(instrs) != 1 { // just LDA
		t.Fatalf("expected 1 instruction (stop before undocumented), got %d", len(instrs))
	}
	if instrs[0].opcode != 0xA9 {
		t.Errorf("instr[0] should be LDA, got 0x%02X", instrs[0].opcode)
	}
}

func TestJIT6502_ScanBlock_MaxSize(t *testing.T) {
	mem := make([]byte, 0x10000)
	// Fill with NOPs
	for i := 0; i < 256; i++ {
		mem[0x0600+i] = 0xEA
	}

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	if len(instrs) != jit6502MaxBlockSize {
		t.Errorf("expected max block size %d, got %d", jit6502MaxBlockSize, len(instrs))
	}
}

func TestJIT6502_ScanBlock_PcOffset(t *testing.T) {
	// LDA #$42 (2 bytes), STA $1234 (3 bytes), NOP (1 byte), BRK (1 byte)
	mem := make([]byte, 0x10000)
	mem[0x0600] = 0xA9 // LDA #$42
	mem[0x0601] = 0x42
	mem[0x0602] = 0x8D // STA $1234
	mem[0x0603] = 0x34
	mem[0x0604] = 0x12
	mem[0x0605] = 0xEA // NOP
	mem[0x0606] = 0x00 // BRK

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	if len(instrs) != 4 {
		t.Fatalf("expected 4 instructions, got %d", len(instrs))
	}

	expectedOffsets := []uint16{0, 2, 5, 6}
	for i, instr := range instrs {
		if instr.pcOffset != expectedOffsets[i] {
			t.Errorf("instr[%d]: pcOffset = %d, want %d", i, instr.pcOffset, expectedOffsets[i])
		}
	}
}

// ===========================================================================
// NeedsFallback Tests
// ===========================================================================

func TestJIT6502_NeedsFallback_BRK(t *testing.T) {
	instrs := []JIT6502Instr{{opcode: 0x00}}
	if !jit6502NeedsFallback(instrs) {
		t.Error("BRK should need fallback")
	}
}

func TestJIT6502_NeedsFallback_RTI(t *testing.T) {
	instrs := []JIT6502Instr{{opcode: 0x40}}
	if !jit6502NeedsFallback(instrs) {
		t.Error("RTI should need fallback")
	}
}

func TestJIT6502_NeedsFallback_JAM(t *testing.T) {
	instrs := []JIT6502Instr{{opcode: 0x02}}
	if !jit6502NeedsFallback(instrs) {
		t.Error("JAM should need fallback")
	}
}

func TestJIT6502_NeedsFallback_Undocumented(t *testing.T) {
	instrs := []JIT6502Instr{{opcode: 0xA7}} // LAX zp (undocumented)
	if !jit6502NeedsFallback(instrs) {
		t.Error("undocumented opcode should need fallback")
	}
}

func TestJIT6502_NeedsFallback_NOP(t *testing.T) {
	instrs := []JIT6502Instr{{opcode: 0xEA}}
	if jit6502NeedsFallback(instrs) {
		t.Error("NOP should not need fallback")
	}
}

func TestJIT6502_NeedsFallback_Empty(t *testing.T) {
	if !jit6502NeedsFallback(nil) {
		t.Error("empty block should need fallback")
	}
}

// ===========================================================================
// Backward Branch Detection Tests
// ===========================================================================

func TestJIT6502_DetectBackwardBranch_Yes(t *testing.T) {
	// DEX, BNE -2 (backward to DEX)
	instrs := []JIT6502Instr{
		{opcode: 0xCA, length: 1, pcOffset: 0},                // DEX
		{opcode: 0xD0, length: 2, pcOffset: 1, operand: 0xFC}, // BNE -4 (signed: -4, target = PC+2-4 = PC-2)
	}
	// startPC=0x0600, BNE at offset 1, branchPC = 0x0600+1+2 = 0x0603, target = 0x0603-4 = 0x05FF
	// That's BEFORE startPC, so it's not within the block... Let me fix the test.

	// DEX at 0x0600 (1 byte), BNE at 0x0601 (2 bytes)
	// BNE offset should target 0x0600 (DEX)
	// branchPC = 0x0601 + 2 = 0x0603
	// offset = 0x0600 - 0x0603 = -3 = 0xFD
	instrs[1].operand = 0xFD

	if !jit6502DetectBackwardBranches(instrs, 0x0600) {
		t.Error("should detect backward branch")
	}
}

func TestJIT6502_DetectBackwardBranch_No(t *testing.T) {
	// LDA #$42, BNE +2 (forward)
	instrs := []JIT6502Instr{
		{opcode: 0xA9, length: 2, pcOffset: 0, operand: 0x42},
		{opcode: 0xD0, length: 2, pcOffset: 2, operand: 0x02}, // BNE +2 (forward)
	}
	if jit6502DetectBackwardBranches(instrs, 0x0600) {
		t.Error("forward branch should not be detected as backward")
	}
}

// ===========================================================================
// Block Terminator Tests
// ===========================================================================

func TestJIT6502_IsBlockTerminator(t *testing.T) {
	terminators := []byte{0x4C, 0x6C, 0x20, 0x60, 0x40, 0x00, 0x02, 0x12, 0x22, 0x32, 0x42, 0x52, 0x62, 0x72, 0x92, 0xB2, 0xD2, 0xF2}
	for _, op := range terminators {
		if !jit6502IsBlockTerminator(op) {
			t.Errorf("opcode 0x%02X should be a block terminator", op)
		}
	}

	nonTerminators := []byte{0xEA, 0xA9, 0xD0, 0xF0, 0x90, 0xB0, 0x85, 0xE8}
	for _, op := range nonTerminators {
		if jit6502IsBlockTerminator(op) {
			t.Errorf("opcode 0x%02X should not be a block terminator", op)
		}
	}
}
