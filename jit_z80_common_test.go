// jit_z80_common_test.go - Z80 JIT infrastructure tests: field offsets, scanner, tables

//go:build (amd64 || arm64) && linux

package main

import (
	"testing"
	"unsafe"
)

// ===========================================================================
// Context Layout Tests
// ===========================================================================

func TestZ80JITContext_FieldOffsets(t *testing.T) {
	var ctx Z80JITContext
	base := uintptr(unsafe.Pointer(&ctx))

	tests := []struct {
		name     string
		got      uintptr
		expected uintptr
	}{
		{"MemPtr", uintptr(unsafe.Pointer(&ctx.MemPtr)) - base, jzCtxOffMemPtr},
		{"CpuPtr", uintptr(unsafe.Pointer(&ctx.CpuPtr)) - base, jzCtxOffCpuPtr},
		{"DirectPageBitmapPtr", uintptr(unsafe.Pointer(&ctx.DirectPageBitmapPtr)) - base, jzCtxOffDirectPageBitmapPtr},
		{"CodePageBitmapPtr", uintptr(unsafe.Pointer(&ctx.CodePageBitmapPtr)) - base, jzCtxOffCodePageBitmapPtr},
		{"RetCycles", uintptr(unsafe.Pointer(&ctx.RetCycles)) - base, jzCtxOffRetCycles},
		{"NeedBail", uintptr(unsafe.Pointer(&ctx.NeedBail)) - base, jzCtxOffNeedBail},
		{"NeedInval", uintptr(unsafe.Pointer(&ctx.NeedInval)) - base, jzCtxOffNeedInval},
		{"RetPC", uintptr(unsafe.Pointer(&ctx.RetPC)) - base, jzCtxOffRetPC},
		{"RetCount", uintptr(unsafe.Pointer(&ctx.RetCount)) - base, jzCtxOffRetCount},
		{"ChainBudget", uintptr(unsafe.Pointer(&ctx.ChainBudget)) - base, jzCtxOffChainBudget},
		{"ChainCount", uintptr(unsafe.Pointer(&ctx.ChainCount)) - base, jzCtxOffChainCount},
		{"InvalPage", uintptr(unsafe.Pointer(&ctx.InvalPage)) - base, jzCtxOffInvalPage},
		{"RTSCache0PC", uintptr(unsafe.Pointer(&ctx.RTSCache0PC)) - base, jzCtxOffRTSCache0PC},
		{"RTSCache0Addr", uintptr(unsafe.Pointer(&ctx.RTSCache0Addr)) - base, jzCtxOffRTSCache0Addr},
		{"RTSCache1PC", uintptr(unsafe.Pointer(&ctx.RTSCache1PC)) - base, jzCtxOffRTSCache1PC},
		{"RTSCache1Addr", uintptr(unsafe.Pointer(&ctx.RTSCache1Addr)) - base, jzCtxOffRTSCache1Addr},
		{"ParityTablePtr", uintptr(unsafe.Pointer(&ctx.ParityTablePtr)) - base, jzCtxOffParityTablePtr},
		{"DAATablePtr", uintptr(unsafe.Pointer(&ctx.DAATablePtr)) - base, jzCtxOffDAATablePtr},
		{"ChainCycles", uintptr(unsafe.Pointer(&ctx.ChainCycles)) - base, jzCtxOffChainCycles},
		{"ChainRIncrements", uintptr(unsafe.Pointer(&ctx.ChainRIncrements)) - base, jzCtxOffChainRIncrements},
		{"CycleBudget", uintptr(unsafe.Pointer(&ctx.CycleBudget)) - base, jzCtxOffCycleBudget},
	}

	for _, tt := range tests {
		if tt.got != tt.expected {
			t.Errorf("Z80JITContext.%s: offset = %d, want %d", tt.name, tt.got, tt.expected)
		}
	}
}

func TestZ80JIT_CPUStructFieldOffsets(t *testing.T) {
	var cpu CPU_Z80
	base := uintptr(unsafe.Pointer(&cpu))

	tests := []struct {
		name     string
		got      uintptr
		expected uintptr
	}{
		{"A", uintptr(unsafe.Pointer(&cpu.A)) - base, cpuZ80OffA},
		{"F", uintptr(unsafe.Pointer(&cpu.F)) - base, cpuZ80OffF},
		{"B", uintptr(unsafe.Pointer(&cpu.B)) - base, cpuZ80OffB},
		{"C", uintptr(unsafe.Pointer(&cpu.C)) - base, cpuZ80OffC},
		{"D", uintptr(unsafe.Pointer(&cpu.D)) - base, cpuZ80OffD},
		{"E", uintptr(unsafe.Pointer(&cpu.E)) - base, cpuZ80OffE},
		{"H", uintptr(unsafe.Pointer(&cpu.H)) - base, cpuZ80OffH},
		{"L", uintptr(unsafe.Pointer(&cpu.L)) - base, cpuZ80OffL},
		{"A2", uintptr(unsafe.Pointer(&cpu.A2)) - base, cpuZ80OffA2},
		{"F2", uintptr(unsafe.Pointer(&cpu.F2)) - base, cpuZ80OffF2},
		{"B2", uintptr(unsafe.Pointer(&cpu.B2)) - base, cpuZ80OffB2},
		{"C2", uintptr(unsafe.Pointer(&cpu.C2)) - base, cpuZ80OffC2},
		{"D2", uintptr(unsafe.Pointer(&cpu.D2)) - base, cpuZ80OffD2},
		{"E2", uintptr(unsafe.Pointer(&cpu.E2)) - base, cpuZ80OffE2},
		{"H2", uintptr(unsafe.Pointer(&cpu.H2)) - base, cpuZ80OffH2},
		{"L2", uintptr(unsafe.Pointer(&cpu.L2)) - base, cpuZ80OffL2},
		{"IX", uintptr(unsafe.Pointer(&cpu.IX)) - base, cpuZ80OffIX},
		{"IY", uintptr(unsafe.Pointer(&cpu.IY)) - base, cpuZ80OffIY},
		{"SP", uintptr(unsafe.Pointer(&cpu.SP)) - base, cpuZ80OffSP},
		{"PC", uintptr(unsafe.Pointer(&cpu.PC)) - base, cpuZ80OffPC},
		{"I", uintptr(unsafe.Pointer(&cpu.I)) - base, cpuZ80OffI},
		{"R", uintptr(unsafe.Pointer(&cpu.R)) - base, cpuZ80OffR},
		{"IM", uintptr(unsafe.Pointer(&cpu.IM)) - base, cpuZ80OffIM},
		{"WZ", uintptr(unsafe.Pointer(&cpu.WZ)) - base, cpuZ80OffWZ},
		{"IFF1", uintptr(unsafe.Pointer(&cpu.IFF1)) - base, cpuZ80OffIFF1},
		{"IFF2", uintptr(unsafe.Pointer(&cpu.IFF2)) - base, cpuZ80OffIFF2},
		{"Halted", uintptr(unsafe.Pointer(&cpu.Halted)) - base, cpuZ80OffHalted},
		{"Cycles", uintptr(unsafe.Pointer(&cpu.Cycles)) - base, cpuZ80OffCycles},
		{"iffDelay", uintptr(unsafe.Pointer(&cpu.iffDelay)) - base, cpuZ80OffIffDelay},
	}

	for _, tt := range tests {
		if tt.got != tt.expected {
			t.Errorf("CPU_Z80.%s: offset = %d, want %d", tt.name, tt.got, tt.expected)
		}
	}
}

// ===========================================================================
// Context Construction Tests
// ===========================================================================

func TestZ80JIT_ContextConstruction(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)

	ctx := newZ80JITContext(cpu, adapter)
	if ctx == nil {
		t.Fatal("newZ80JITContext returned nil")
	}
	if ctx.MemPtr == 0 {
		t.Error("MemPtr is zero")
	}
	if ctx.CpuPtr == 0 {
		t.Error("CpuPtr is zero")
	}
	if ctx.DirectPageBitmapPtr == 0 {
		t.Error("DirectPageBitmapPtr is zero")
	}
	if ctx.CodePageBitmapPtr == 0 {
		t.Error("CodePageBitmapPtr is zero")
	}
	if ctx.ParityTablePtr == 0 {
		t.Error("ParityTablePtr is zero")
	}
	if ctx.DAATablePtr == 0 {
		t.Error("DAATablePtr is zero")
	}
}

// ===========================================================================
// Parity Table Tests
// ===========================================================================

func TestZ80JIT_ParityTable(t *testing.T) {
	tests := []struct {
		value    byte
		expected byte // 0x04 = even parity, 0 = odd parity
	}{
		{0x00, 0x04}, // 0 bits set = even
		{0x01, 0x00}, // 1 bit set = odd
		{0x03, 0x04}, // 2 bits set = even
		{0x07, 0x00}, // 3 bits set = odd
		{0x55, 0x04}, // 4 bits set = even
		{0xAA, 0x04}, // 4 bits set = even
		{0xFF, 0x04}, // 8 bits set = even
		{0x80, 0x00}, // 1 bit set = odd
		{0xFE, 0x00}, // 7 bits set = odd
	}

	for _, tt := range tests {
		got := z80ParityTable[tt.value]
		if got != tt.expected {
			t.Errorf("parityTable[0x%02X] = 0x%02X, want 0x%02X", tt.value, got, tt.expected)
		}
	}
}

// ===========================================================================
// Direct Page Bitmap Tests
// ===========================================================================

func TestZ80JIT_DirectPageBitmap(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.initDirectPageBitmapZ80(adapter)

	// Pages $00-$1F should be direct (flat memory)
	for page := 0; page < 0x20; page++ {
		if cpu.directPageBitmap[page] != 0 {
			t.Errorf("page $%02X: expected direct (0), got %d", page, cpu.directPageBitmap[page])
		}
	}

	// Pages $20-$7F should be non-direct (bank windows)
	for page := 0x20; page < 0x80; page++ {
		if cpu.directPageBitmap[page] != 1 {
			t.Errorf("page $%02X: expected non-direct (1), got %d", page, cpu.directPageBitmap[page])
		}
	}

	// Pages $80-$BF should be non-direct (VRAM window)
	for page := 0x80; page < 0xC0; page++ {
		if cpu.directPageBitmap[page] != 1 {
			t.Errorf("page $%02X: expected non-direct (1), got %d", page, cpu.directPageBitmap[page])
		}
	}

	// Pages $C0-$EF should be direct (flat memory above VRAM, below I/O)
	for page := 0xC0; page < 0xF0; page++ {
		if cpu.directPageBitmap[page] != 0 {
			t.Errorf("page $%02X: expected direct (0), got %d", page, cpu.directPageBitmap[page])
		}
	}

	// Pages $F0-$FF should be non-direct (I/O translation)
	for page := 0xF0; page <= 0xFF; page++ {
		if cpu.directPageBitmap[page] != 1 {
			t.Errorf("page $%02X: expected non-direct (1), got %d", page, cpu.directPageBitmap[page])
		}
	}
}

// ===========================================================================
// Block Scanner Tests
// ===========================================================================

func newZ80JITScanTestMem(size int) []byte {
	return make([]byte, size)
}

func TestZ80JIT_ScanBlock_NOP_HALT(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0x00     // NOP
	mem[0x0001] = 0x76     // HALT
	var directBM [256]byte // all zeros = all direct

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)

	// HALT is a fallback instruction, so scanner should return just NOP
	// (HALT can't be JIT compiled, and NOP isn't a terminator so the block
	// stops before HALT)
	if len(instrs) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instrs))
	}
	if instrs[0].opcode != 0x00 {
		t.Errorf("instr[0].opcode = 0x%02X, want 0x00 (NOP)", instrs[0].opcode)
	}
}

func TestZ80JIT_ScanBlock_LDRegReg(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0x78 // LD A,B
	mem[0x0001] = 0x4A // LD C,D
	mem[0x0002] = 0xC9 // RET
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(instrs))
	}
	if instrs[0].opcode != 0x78 || instrs[0].length != 1 {
		t.Errorf("instr[0]: opcode=0x%02X len=%d, want 0x78 len=1", instrs[0].opcode, instrs[0].length)
	}
	if instrs[1].opcode != 0x4A || instrs[1].length != 1 {
		t.Errorf("instr[1]: opcode=0x%02X len=%d, want 0x4A len=1", instrs[1].opcode, instrs[1].length)
	}
	if instrs[2].opcode != 0xC9 || instrs[2].length != 1 {
		t.Errorf("instr[2]: opcode=0x%02X len=%d, want 0xC9 len=1", instrs[2].opcode, instrs[2].length)
	}
}

func TestZ80JIT_ScanBlock_CBPrefix(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0xCB // CB prefix
	mem[0x0001] = 0x00 // RLC B
	mem[0x0002] = 0xCB // CB prefix
	mem[0x0003] = 0x01 // RLC C
	mem[0x0004] = 0xC9 // RET
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(instrs))
	}
	if instrs[0].prefix != z80JITPrefixCB || instrs[0].opcode != 0x00 {
		t.Errorf("instr[0]: prefix=0x%02X opcode=0x%02X, want CB 0x00", instrs[0].prefix, instrs[0].opcode)
	}
	if instrs[0].length != 2 {
		t.Errorf("instr[0].length = %d, want 2", instrs[0].length)
	}
	if instrs[0].rIncrements != 2 {
		t.Errorf("instr[0].rIncrements = %d, want 2", instrs[0].rIncrements)
	}
}

func TestZ80JIT_ScanBlock_DDPrefix(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0xDD // DD prefix
	mem[0x0001] = 0x7E // LD A,(IX+d)
	mem[0x0002] = 0x05 // displacement = +5
	mem[0x0003] = 0xC9 // RET
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}
	if instrs[0].prefix != z80JITPrefixDD || instrs[0].opcode != 0x7E {
		t.Errorf("instr[0]: prefix=0x%02X opcode=0x%02X, want DD 0x7E", instrs[0].prefix, instrs[0].opcode)
	}
	if instrs[0].length != 3 {
		t.Errorf("instr[0].length = %d, want 3", instrs[0].length)
	}
	if instrs[0].displacement != 5 {
		t.Errorf("instr[0].displacement = %d, want 5", instrs[0].displacement)
	}
	if instrs[0].rIncrements != 2 {
		t.Errorf("instr[0].rIncrements = %d, want 2", instrs[0].rIncrements)
	}
}

func TestZ80JIT_ScanBlock_IOBail(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0xDB // IN A,(n) — fallback
	mem[0x0001] = 0x42 // port number
	mem[0x0002] = 0x00 // NOP
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	// First instruction is fallback, so scanner returns empty block
	if len(instrs) != 0 {
		t.Fatalf("expected 0 instructions (first is fallback), got %d", len(instrs))
	}
}

func TestZ80JIT_ScanBlock_JPTerminator(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0x00 // NOP
	mem[0x0001] = 0xC3 // JP nn
	mem[0x0002] = 0x34 // low byte
	mem[0x0003] = 0x12 // high byte
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}
	if instrs[1].opcode != 0xC3 {
		t.Errorf("instr[1].opcode = 0x%02X, want 0xC3 (JP)", instrs[1].opcode)
	}
	if instrs[1].operand != 0x1234 {
		t.Errorf("instr[1].operand = 0x%04X, want 0x1234", instrs[1].operand)
	}
}

func TestZ80JIT_ScanBlock_MaxBlockSize(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	// Fill with NOPs (not terminators), then a RET
	for i := 0; i < 200; i++ {
		mem[i] = 0x00 // NOP
	}
	mem[200] = 0xC9 // RET
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) != z80JITMaxBlockSize {
		t.Fatalf("expected %d instructions (max block), got %d", z80JITMaxBlockSize, len(instrs))
	}
}

func TestZ80JIT_InstrLength(t *testing.T) {
	tests := []struct {
		name   string
		bytes  []byte
		length byte
	}{
		{"NOP", []byte{0x00}, 1},
		{"LD A,n", []byte{0x3E, 0x42}, 2},
		{"LD HL,nn", []byte{0x21, 0x34, 0x12}, 3},
		{"CB RLC B", []byte{0xCB, 0x00}, 2},
		{"DD LD A,(IX+d)", []byte{0xDD, 0x7E, 0x05}, 3},
		// ED LD (nn),BC and DDCB not tested here — they're not JIT-compilable
		// and the scanner now correctly rejects them (z80JITCanEmit=false).
	}

	for _, tt := range tests {
		mem := newZ80JITScanTestMem(0x10000)
		copy(mem[0:], tt.bytes)
		// Add a RET after to ensure we don't scan past
		mem[len(tt.bytes)] = 0xC9
		var directBM [256]byte

		instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
		if len(instrs) < 1 {
			t.Errorf("%s: scanner returned 0 instructions", tt.name)
			continue
		}
		if instrs[0].length != tt.length {
			t.Errorf("%s: length = %d, want %d", tt.name, instrs[0].length, tt.length)
		}
	}
}

func TestZ80JIT_NeedsFallback(t *testing.T) {
	tests := []struct {
		name   string
		instr  JITZ80Instr
		expect bool
	}{
		{"IN A,(n)", JITZ80Instr{prefix: z80JITPrefixNone, opcode: 0xDB}, true},
		{"OUT (n),A", JITZ80Instr{prefix: z80JITPrefixNone, opcode: 0xD3}, true},
		{"HALT", JITZ80Instr{prefix: z80JITPrefixNone, opcode: 0x76}, true},
		{"EX (SP),HL", JITZ80Instr{prefix: z80JITPrefixNone, opcode: 0xE3}, true},
		{"DAA", JITZ80Instr{prefix: z80JITPrefixNone, opcode: 0x27}, false}, // now handled via lookup table
		{"NOP", JITZ80Instr{prefix: z80JITPrefixNone, opcode: 0x00}, false},
		{"LD A,B", JITZ80Instr{prefix: z80JITPrefixNone, opcode: 0x78}, false},
		{"ADD A,n", JITZ80Instr{prefix: z80JITPrefixNone, opcode: 0xC6}, false},
		{"ED RLD", JITZ80Instr{prefix: z80JITPrefixED, opcode: 0x6F}, true},
		{"ED INI", JITZ80Instr{prefix: z80JITPrefixED, opcode: 0xA2}, true},
		{"ED OUTI", JITZ80Instr{prefix: z80JITPrefixED, opcode: 0xA3}, true},
		{"ED IN B,(C)", JITZ80Instr{prefix: z80JITPrefixED, opcode: 0x40}, true},
		{"ED LD A,R", JITZ80Instr{prefix: z80JITPrefixED, opcode: 0x57}, true},
		{"ED NEG", JITZ80Instr{prefix: z80JITPrefixED, opcode: 0x44}, false},
		{"DD EX (SP),IX", JITZ80Instr{prefix: z80JITPrefixDD, opcode: 0xE3}, true},
		{"FD EX (SP),IY", JITZ80Instr{prefix: z80JITPrefixFD, opcode: 0xE3}, true},
	}

	for _, tt := range tests {
		got := z80JITNeedsFallback(&tt.instr)
		if got != tt.expect {
			t.Errorf("%s: needsFallback = %v, want %v", tt.name, got, tt.expect)
		}
	}
}

func TestZ80JIT_ScanBlock_EI_IsTerminator(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0x00 // NOP
	mem[0x0001] = 0xFB // EI — should be block terminator
	mem[0x0002] = 0x00 // NOP (should not be in block)
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions (NOP + EI terminator), got %d", len(instrs))
	}
	if instrs[1].opcode != 0xFB {
		t.Errorf("instr[1].opcode = 0x%02X, want 0xFB (EI)", instrs[1].opcode)
	}
}

func TestZ80JIT_ScanBlock_DI_IsTerminator(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0x00 // NOP
	mem[0x0001] = 0xF3 // DI — should be block terminator
	mem[0x0002] = 0x00 // NOP (should not be in block)
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions (NOP + DI terminator), got %d", len(instrs))
	}
	if instrs[1].opcode != 0xF3 {
		t.Errorf("instr[1].opcode = 0x%02X, want 0xF3 (DI)", instrs[1].opcode)
	}
}

func TestZ80JIT_ScanBlock_RIncrements(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	// NOP (rInc=1), CB RLC B (rInc=2), DD LD A,(IX+0) (rInc=2), RET (rInc=1)
	mem[0x0000] = 0x00 // NOP
	mem[0x0001] = 0xCB // CB prefix
	mem[0x0002] = 0x00 // RLC B
	mem[0x0003] = 0xDD // DD prefix
	mem[0x0004] = 0x7E // LD A,(IX+d)
	mem[0x0005] = 0x00 // displacement = 0
	mem[0x0006] = 0xC9 // RET
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) != 4 {
		t.Fatalf("expected 4 instructions, got %d", len(instrs))
	}

	expected := []byte{1, 2, 2, 1}
	for i, exp := range expected {
		if instrs[i].rIncrements != exp {
			t.Errorf("instr[%d].rIncrements = %d, want %d", i, instrs[i].rIncrements, exp)
		}
	}

	// Total R increments for block
	totalR := 0
	for _, instr := range instrs {
		totalR += int(instr.rIncrements)
	}
	if totalR != 6 {
		t.Errorf("total R increments = %d, want 6", totalR)
	}
}

func TestZ80JIT_ScanBlock_DDCB_RIncrements(t *testing.T) {
	// DDCB instructions are not JIT-compilable, so the scanner rejects them.
	// Test that the scanner correctly returns empty when DDCB is the first instruction.
	mem := newZ80JITScanTestMem(0x10000)
	mem[0x0000] = 0xDD // DD prefix
	mem[0x0001] = 0xCB // CB sub-prefix
	mem[0x0002] = 0x05 // displacement = +5
	mem[0x0003] = 0x06 // RLC (IX+d)
	mem[0x0004] = 0xC9 // RET
	var directBM [256]byte

	instrs := z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	// DDCB is now compilable → scanner returns 2 instructions (DDCB + RET)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions (DDCB + RET), got %d", len(instrs))
	}
	if instrs[0].rIncrements != 3 {
		t.Errorf("DDCB instr rIncrements = %d, want 3", instrs[0].rIncrements)
	}
	if instrs[0].length != 4 {
		t.Errorf("DDCB instr length = %d, want 4", instrs[0].length)
	}

	// Test R increments via a compilable prefix (DD LD A,(IX+d), rInc=2)
	mem[0x0000] = 0xDD // DD prefix
	mem[0x0001] = 0x7E // LD A,(IX+d)
	mem[0x0002] = 0x05 // displacement = +5
	mem[0x0003] = 0xC9 // RET

	instrs = z80JITScanBlock(mem, 0x0000, len(mem), &directBM)
	if len(instrs) < 1 {
		t.Fatalf("expected >=1 instructions, got %d", len(instrs))
	}
	if instrs[0].rIncrements != 2 {
		t.Errorf("DD instr rIncrements = %d, want 2", instrs[0].rIncrements)
	}
}

func TestZ80JIT_ScanBlock_PageBoundarySafety(t *testing.T) {
	mem := newZ80JITScanTestMem(0x10000)
	// Place NOPs near the end of page $00 (which is direct)
	// Page $20 is non-direct (bank window)
	startPC := uint16(0x1FFD) // 3 bytes before page $20
	mem[0x1FFD] = 0x00        // NOP
	mem[0x1FFE] = 0x00        // NOP
	mem[0x1FFF] = 0x21        // LD HL,nn (3 bytes — would cross into page $20)
	mem[0x2000] = 0x34
	mem[0x2001] = 0x12
	mem[0x2002] = 0xC9 // RET

	var directBM [256]byte
	// Mark page $20 as non-direct
	directBM[0x20] = 1

	instrs := z80JITScanBlock(mem, startPC, len(mem), &directBM)
	// Should get 2 NOPs, then stop before LD HL,nn which crosses into non-direct page
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions (stop before page boundary), got %d", len(instrs))
	}
}

func TestZ80JIT_PeepholeFlags(t *testing.T) {
	// Block: LD A,0x42; ADD A,B; ADD A,C; JP Z,nn
	// First ADD's flags are overwritten by second ADD → flagsNeeded[1]=false
	// Second ADD's flags ARE consumed by JP Z → flagsNeeded[2]=true
	instrs := []JITZ80Instr{
		{prefix: z80JITPrefixNone, opcode: 0x3E, length: 2},                  // LD A,n (no flags)
		{prefix: z80JITPrefixNone, opcode: 0x80, length: 1},                  // ADD A,B (produces flags)
		{prefix: z80JITPrefixNone, opcode: 0x81, length: 1},                  // ADD A,C (produces flags)
		{prefix: z80JITPrefixNone, opcode: 0xCA, operand: 0x1234, length: 3}, // JP Z,nn (consumes flags)
	}

	flagsNeeded := z80PeepholeFlags(instrs)

	if flagsNeeded[0] != 0 {
		t.Error("instr 0 (LD A,n) should not have flagsNeeded")
	}
	if flagsNeeded[1] != 0 {
		t.Error("instr 1 (ADD A,B) flagsNeeded should be 0 (overwritten by ADD A,C)")
	}
	if flagsNeeded[2] == 0 {
		t.Error("instr 2 (ADD A,C) flagsNeeded should be nonzero (consumed by JP Z)")
	}
	if flagsNeeded[3] != 0 {
		t.Error("instr 3 (JP Z) should not have flagsNeeded (doesn't produce flags)")
	}
}

func TestZ80JIT_PeepholeFlags_AllNeeded(t *testing.T) {
	// Block: INC A; JR NZ,e — flags from INC consumed by JR NZ
	instrs := []JITZ80Instr{
		{prefix: z80JITPrefixNone, opcode: 0x3C, length: 1},                // INC A
		{prefix: z80JITPrefixNone, opcode: 0x20, operand: 0x05, length: 2}, // JR NZ
	}

	flagsNeeded := z80PeepholeFlags(instrs)

	if flagsNeeded[0] == 0 {
		t.Error("INC A flagsNeeded should be nonzero (consumed by JR NZ)")
	}
}

func TestZ80JIT_PeepholeFlags_NoneNeeded(t *testing.T) {
	// Block: ADD A,B; LD C,D; XOR A — all flags from ADD are dead (XOR overwrites)
	instrs := []JITZ80Instr{
		{prefix: z80JITPrefixNone, opcode: 0x80, length: 1}, // ADD A,B
		{prefix: z80JITPrefixNone, opcode: 0x4A, length: 1}, // LD C,D (no flags)
		{prefix: z80JITPrefixNone, opcode: 0xAF, length: 1}, // XOR A (produces flags)
	}

	flagsNeeded := z80PeepholeFlags(instrs)

	if flagsNeeded[0] != 0 {
		t.Error("ADD A,B flagsNeeded should be 0 (XOR A overwrites before any consumer)")
	}
	// XOR A is last — conservatively flagsNeeded=nonzero (exec loop may check F)
	if flagsNeeded[2] == 0 {
		t.Error("XOR A flagsNeeded should be nonzero (last instruction, conservative)")
	}
}
