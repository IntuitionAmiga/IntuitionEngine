//go:build arm64 && linux

package main

import (
	"testing"
	"unsafe"
)

// TestJIT6502_ARM64_EmitterReturnEncoding guards the native trampoline
// boundary independently of execution. A block that falls through instead of
// returning to jitCall corrupts the g0 stack under QEMU.
func TestJIT6502_ARM64_EmitterReturnEncoding(t *testing.T) {
	execMem, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("allocate executable memory: %v", err)
	}
	defer execMem.Free()
	block, err := compileBlock6502([]JIT6502Instr{{opcode: 0xEA, length: 1}}, 0x0600, execMem, nil)
	if err != nil {
		t.Fatalf("compile NOP: %v", err)
	}
	if block.execSize < 4 {
		t.Fatalf("NOP block too short: %d", block.execSize)
	}
	words := unsafe.Slice((*uint32)(unsafe.Pointer(block.execAddr)), block.execSize/4)
	if got, want := words[len(words)-1], arm64RET(); got != want {
		t.Fatalf("native NOP terminator=%#08x, want RET %#08x", got, want)
	}
}

func TestJIT6502_ARM64_NativeReturnABI(t *testing.T) {
	execMem, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("allocate executable memory: %v", err)
	}
	defer execMem.Free()
	block, err := compileBlock6502([]JIT6502Instr{{opcode: 0xEA, length: 1}}, 0x0600, execMem, nil)
	if err != nil {
		t.Fatalf("compile NOP: %v", err)
	}
	ctx := &JIT6502Context{}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := ctx.RetPC, uint32(0x0601); got != want {
		t.Fatalf("return PC=%#x, want %#x", got, want)
	}
	if got, want := ctx.RetCount, uint32(1); got != want {
		t.Fatalf("return count=%d, want %d", got, want)
	}
}

// TestJIT6502_ARM64_NativeDEYReturnABI isolates the opcode path which writes
// both CPU state and the dispatcher return contract. It catches an emitter
// fall-through without involving the cache or interpreter.
func TestJIT6502_ARM64_NativeDEYReturnABI(t *testing.T) {
	execMem, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("allocate executable memory: %v", err)
	}
	defer execMem.Free()
	block, err := compileBlock6502([]JIT6502Instr{{opcode: 0x88, length: 1}}, 0x0600, execMem, nil)
	if err != nil {
		t.Fatalf("compile DEY: %v", err)
	}
	cpu := &CPU_6502{Y: 1, SR: UNUSED_FLAG}
	ctx := &JIT6502Context{CpuPtr: uintptr(unsafe.Pointer(cpu)), NZTablePtr: uintptr(unsafe.Pointer(&nzTable[0]))}
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if got, want := cpu.Y, byte(0); got != want {
		t.Fatalf("Y=%#x, want %#x", got, want)
	}
	if got, want := ctx.RetPC, uint32(0x0601); got != want {
		t.Fatalf("return PC=%#x, want %#x", got, want)
	}
	if got, want := ctx.RetCount, uint32(1); got != want {
		t.Fatalf("return count=%d, want %d", got, want)
	}
}

// TestJIT6502_ARM64_NativeNOPProvenance is deliberately run under QEMU in the
// parity target. It proves that the public ARM64 dispatcher reaches an emitted
// ARM64 block, rather than passing by interpreter-only execution.
func TestJIT6502_ARM64_NativeNOPProvenance(t *testing.T) {
	if !jit6502Available {
		t.Fatal("Linux ARM64 6502 JIT dispatcher is unavailable")
	}
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	bus.Write8(0x0600, 0xEA) // NOP: direct ARM64 lowering
	bus.Write8(0x0601, haltOpcode)
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()

	if cpu.Running() {
		t.Fatal("program did not reach JAM halt")
	}
	if got := cpu.jit6502StatsSnapshot().nativeEntries; got == 0 {
		t.Fatal("ARM64 dispatcher did not execute an emitted native block")
	}
	if got, want := cpu.Cycles, uint64(2); got != want {
		t.Fatalf("NOP cycles=%d, want %d", got, want)
	}
}

func TestJIT6502_ARM64_BoundedStraightLineChain(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	program := []byte{
		0xEA,       // NOP, patched to the next NOP after the first lap
		0xEA,       // NOP
		0x4C, 0, 6, // JMP $0600
	}
	for i, b := range program {
		bus.Write8(0x0600+uint32(i), b)
	}
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.jitTestStopAfter = 5
	cpu.ExecuteJIT6502()
	if got, want := cpu.jitTestRetired, uint64(5); got != want {
		t.Fatalf("retired=%d, want %d", got, want)
	}
	if got, want := cpu.PC, uint16(0x0602); got != want {
		t.Fatalf("PC=%04X, want %04X after chained NOP pair", got, want)
	}
	if got, want := cpu.Cycles, uint64(11); got != want {
		t.Fatalf("cycles=%d, want %d", got, want)
	}
	if got := cpu.jit6502StatsSnapshot().chainExits; got == 0 {
		t.Fatal("straight-line native blocks did not use the bounded chain path")
	}
}

// TestJIT6502_ARM64_CachedSuccessorChain compiles the successor before its
// predecessor. The latter must patch its outbound slot immediately; ARM64
// slots hold a complete B instruction, not an x86 rel32 displacement.
func TestJIT6502_ARM64_CachedSuccessorChain(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	for i, value := range []byte{0xEA, 0xEA, haltOpcode} {
		bus.Write8(0x0600+uint32(i), value)
	}
	cpu.PC = 0x0601 // compile and retain the successor first
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	defer func() {
		cpu.jitPersist = false
		cpu.freeJIT6502()
	}()
	cpu.jitTestStopAfter = 1
	cpu.ExecuteJIT6502()
	if cpu.jitCache.Get(0x0601) == nil {
		t.Fatal("successor block was not cached")
	}

	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.jitTestStopAfter = 3
	cpu.ExecuteJIT6502()
	if got, want := cpu.jitTestRetired, uint64(3); got != want {
		t.Fatalf("retired=%d, want %d", got, want)
	}
	if got, want := cpu.PC, uint16(0x0602); got != want {
		t.Fatalf("PC=%04X, want %04X after patched successor chain", got, want)
	}
	if got := cpu.jit6502StatsSnapshot().chainExits; got == 0 {
		t.Fatal("cached successor did not execute through a patched ARM64 chain")
	}
}

func TestJIT6502_ARM64_NativeImmediateLoad(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	bus.Write8(0x0600, 0xA9) // LDA #$80
	bus.Write8(0x0601, 0x80)
	bus.Write8(0x0602, haltOpcode)
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()
	if got, want := cpu.A, byte(0x80); got != want {
		t.Fatalf("A=%02X, want %02X", got, want)
	}
	if cpu.SR&NEGATIVE_FLAG == 0 || cpu.SR&ZERO_FLAG != 0 {
		t.Fatalf("SR=%02X, expected N set and Z clear", cpu.SR)
	}
	if got := cpu.jit6502StatsSnapshot().nativeEntries; got == 0 {
		t.Fatal("LDA immediate did not execute through ARM64 native code")
	}
}

func TestJIT6502_ARM64_NativeRegisterAndFlagForms(t *testing.T) {
	program := []byte{
		0xA9, 0x00, // LDA #0
		0xAA, // TAX
		0x9A, // TXS (must preserve N/Z)
		0xBA, // TSX
		0xE8, // INX
		0xCA, // DEX
		0x38, // SEC
		haltOpcode,
	}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, b := range program {
			bus.Write8(0x0600+uint32(i), b)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	for interp.Running() {
		interp.Step()
	}
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if got := jit.jit6502StatsSnapshot().nativeEntries; got < 7 {
		t.Fatalf("native entries=%d, want at least 7", got)
	}
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT state PC=%04X A=%02X X=%02X Y=%02X SR=%02X cycles=%d; interpreter PC=%04X A=%02X X=%02X Y=%02X SR=%02X cycles=%d",
			jit.PC, jit.A, jit.X, jit.Y, jit.SR, jit.Cycles, interp.PC, interp.A, interp.X, interp.Y, interp.SR, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeDirectRAMLoad(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	bus.Write8(0x0010, 0x7F)
	bus.Write8(0x0600, 0xA5) // LDA $10
	bus.Write8(0x0601, 0x10)
	bus.Write8(0x0602, haltOpcode)
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()
	if got, want := cpu.A, byte(0x7F); got != want {
		t.Fatalf("A=%02X, want %02X", got, want)
	}
	if cpu.SR&(ZERO_FLAG|NEGATIVE_FLAG) != 0 {
		t.Fatalf("SR=%02X, expected N/Z clear", cpu.SR)
	}
	if got := cpu.jit6502StatsSnapshot().nativeEntries; got == 0 {
		t.Fatal("direct-RAM load did not execute through ARM64 native code")
	}
}

func TestJIT6502_ARM64_MappedLoadBailsBeforeSideEffect(t *testing.T) {
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0x0010, 0x0010, func(uint32) uint32 {
		reads++
		return 0x42
	}, nil)
	cpu := NewCPU_6502(bus)
	bus.Write8(0x0600, 0xA5) // LDA $10
	bus.Write8(0x0601, 0x10)
	bus.Write8(0x0602, haltOpcode)
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()
	if got, want := reads, 1; got != want {
		t.Fatalf("mapped read count=%d, want %d", got, want)
	}
	if got, want := cpu.A, byte(0x42); got != want {
		t.Fatalf("A=%02X, want mapped value %02X", got, want)
	}
}

func TestJIT6502_ARM64_NativeDirectStoreInvalidates(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	bus.Write8(0x0600, 0xA9) // LDA #$42
	bus.Write8(0x0601, 0x42)
	bus.Write8(0x0602, 0x85) // STA $10
	bus.Write8(0x0603, 0x10)
	bus.Write8(0x0604, haltOpcode)
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()
	if got, want := bus.Read8(0x0010), byte(0x42); got != want {
		t.Fatalf("memory[$10]=%02X, want %02X", got, want)
	}
	if got := cpu.jit6502StatsSnapshot().invalidations; got == 0 {
		t.Fatal("native store did not request dispatcher invalidation")
	}
}

func TestJIT6502_ARM64_NativeAbsoluteJump(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	bus.Write8(0x0600, 0x4C) // JMP $0604
	bus.Write8(0x0601, 0x04)
	bus.Write8(0x0602, 0x06)
	bus.Write8(0x0603, 0xEA) // skipped NOP
	bus.Write8(0x0604, haltOpcode)
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()
	if got, want := cpu.Cycles, uint64(3); got != want {
		t.Fatalf("cycles=%d, want %d", got, want)
	}
	if got := cpu.jit6502StatsSnapshot().nativeEntries; got == 0 {
		t.Fatal("absolute jump did not execute through ARM64 native code")
	}
}

func TestJIT6502_ARM64_NativeBranchesMatchInterpreter(t *testing.T) {
	// BNE skips a NOP when Z is clear; the second BNE falls through when the
	// preceding LDA sets Z. This exercises both arms of the emitted branch.
	program := []byte{0xD0, 0x01, 0xEA, 0xA9, 0x00, 0xD0, 0x01, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
	if got := jit.jit6502StatsSnapshot().nativeEntries; got < 3 {
		t.Fatalf("native entries=%d, want branch execution through ARM64", got)
	}
}

func TestJIT6502_ARM64_NativeDecimalImmediate(t *testing.T) {
	program := []byte{0xF8, 0xA9, 0x45, 0x38, 0x69, 0x55, haltOpcode} // SED; LDA #45; SEC; ADC #55
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeBinaryImmediate(t *testing.T) {
	program := []byte{0xA9, 0x45, 0x38, 0x69, 0x55, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
	if got := jit.jit6502StatsSnapshot().nativeEntries; got < 3 {
		t.Fatalf("native entries=%d, want binary ADC native execution", got)
	}
}

func TestJIT6502_ARM64_NativeLogicalDirectRAM(t *testing.T) {
	program := []byte{
		0xA9, 0xF0, // LDA #$F0
		0x25, 0x10, // AND $10
		0x05, 0x11, // ORA $11
		0x49, 0x0F, // EOR #$0F
		haltOpcode,
	}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0xCC)
		bus.Write8(0x0011, 0x03)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeBITDirectRAM(t *testing.T) {
	program := []byte{0xA9, 0x40, 0x24, 0x10, 0x2C, 0x11, 0x00, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0xC0) // V/N from memory; A&value is non-zero.
		bus.Write8(0x0011, 0x80) // N from memory; A&value is zero.
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeStackFormsWrapAndPreserveStatus(t *testing.T) {
	program := []byte{0xA9, 0x80, 0x48, 0xA9, 0x00, 0x68, 0x08, 0x28, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SP = 0x00 // PHA must write $0100 then wrap to $FF.
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.SP != interp.SP || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0100) != interpBus.Read8(0x0100) {
		t.Fatalf("JIT A=%02X SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X; interpreter A=%02X SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X",
			jit.A, jit.SP, jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0100), interp.A, interp.SP, interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0100))
	}
}

func TestJIT6502_ARM64_NativeAccumulatorShifts(t *testing.T) {
	program := []byte{0xA9, 0x81, 0x38, 0x0A, 0x2A, 0x38, 0x6A, 0x4A, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeImmediateCompares(t *testing.T) {
	program := []byte{0xA9, 0x10, 0xA2, 0x80, 0xA0, 0x10, 0xC9, 0x10, 0xE0, 0x81, 0xC0, 0x11, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeDirectINCDEC(t *testing.T) {
	program := []byte{0xE6, 0x10, 0xCE, 0x11, 0x00, 0xA2, 0x02, 0xF6, 0xFF, 0xD6, 0xFE, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0xFF)
		bus.Write8(0x0011, 0x00)
		bus.Write8(0x0001, 0xFF)
		bus.Write8(0x0000, 0x00)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x10) != interpBus.Read8(0x10) || jitBus.Read8(0x11) != interpBus.Read8(0x11) || jitBus.Read8(0x00) != interpBus.Read8(0x00) || jitBus.Read8(0x01) != interpBus.Read8(0x01) {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d mem=%02X/%02X/%02X/%02X; interpreter SR=%02X PC=%04X cycles=%d mem=%02X/%02X/%02X/%02X",
			jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x10), jitBus.Read8(0x11), jitBus.Read8(0x00), jitBus.Read8(0x01), interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x10), interpBus.Read8(0x11), interpBus.Read8(0x00), interpBus.Read8(0x01))
	}
}

func TestJIT6502_ARM64_NativeZeroPageIndexedLogic(t *testing.T) {
	program := []byte{0xA2, 0x02, 0xA9, 0xF0, 0x35, 0xFE, 0x15, 0xFD, 0x55, 0xFC, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0xCC)
		bus.Write8(0x00FF, 0x03)
		bus.Write8(0x00FE, 0x0F)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeJSRRTS(t *testing.T) {
	program := []byte{0x20, 0x06, 0x06, haltOpcode, 0xEA, 0xEA, 0x60}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SP = 0
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.SP != interp.SP || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0100) != interpBus.Read8(0x0100) || jitBus.Read8(0x01FF) != interpBus.Read8(0x01FF) {
		t.Fatalf("JIT SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X/%02X; interpreter SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X/%02X",
			jit.SP, jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0100), jitBus.Read8(0x01FF), interp.SP, interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0100), interpBus.Read8(0x01FF))
	}
}

func TestJIT6502_ARM64_NativeJMPIndirectPageWrap(t *testing.T) {
	program := []byte{0x6C, 0xFF, 0x10, 0xEA, 0xEA, 0xEA, 0xEA, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x10FF, 0x07)
		bus.Write8(0x1000, 0x06) // NMOS wrap source, not $1100.
		bus.Write8(0x1100, 0x05)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.PC != interp.PC || jit.Cycles != interp.Cycles || jit.Running() != interp.Running() {
		t.Fatalf("JIT PC=%04X cycles=%d running=%v; interpreter PC=%04X cycles=%d running=%v",
			jit.PC, jit.Cycles, jit.Running(), interp.PC, interp.Cycles, interp.Running())
	}
}

func TestJIT6502_ARM64_NativeRTI(t *testing.T) {
	program := []byte{0x40, 0xEA, 0xEA, 0xEA, 0xEA, 0xEA, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x01FD, BREAK_FLAG|CARRY_FLAG)
		bus.Write8(0x01FE, 0x06)
		bus.Write8(0x01FF, 0x06)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC, cpu.SP = 0x0600, 0xFC
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.SP != interp.SP || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jit.Running() != interp.Running() {
		t.Fatalf("JIT SP=%02X SR=%02X PC=%04X cycles=%d running=%v; interpreter SP=%02X SR=%02X PC=%04X cycles=%d running=%v",
			jit.SP, jit.SR, jit.PC, jit.Cycles, jit.Running(), interp.SP, interp.SR, interp.PC, interp.Cycles, interp.Running())
	}
}

func TestJIT6502_ARM64_NativeBRK(t *testing.T) {
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(IRQ_VECTOR, 0x06)
		bus.Write8(IRQ_VECTOR+1, 0x06)
		bus.Write8(0x0600, 0x00)
		bus.Write8(0x0601, 0xEA)
		bus.Write8(0x0606, haltOpcode)
		cpu := NewCPU_6502(bus)
		cpu.PC, cpu.SP = 0x0600, 0x00
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.SP != interp.SP || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jit.Running() != interp.Running() || jitBus.Read8(0x0100) != interpBus.Read8(0x0100) || jitBus.Read8(0x01FF) != interpBus.Read8(0x01FF) || jitBus.Read8(0x01FE) != interpBus.Read8(0x01FE) {
		t.Fatalf("JIT SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X/%02X/%02X; interpreter SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X/%02X/%02X",
			jit.SP, jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0100), jitBus.Read8(0x01FF), jitBus.Read8(0x01FE), interp.SP, interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0100), interpBus.Read8(0x01FF), interpBus.Read8(0x01FE))
	}
}

func TestJIT6502_ARM64_NativeAbsoluteIndexedStore(t *testing.T) {
	program := []byte{0xA9, 0x5A, 0xA2, 0x02, 0x9D, 0xFE, 0x01, 0xA0, 0x03, 0x99, 0xFD, 0x01, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X PC=%04X cycles=%d mem=%02X; interpreter A=%02X X=%02X Y=%02X PC=%04X cycles=%d mem=%02X",
			jit.A, jit.X, jit.Y, jit.PC, jit.Cycles, jitBus.Read8(0x0200), interp.A, interp.X, interp.Y, interp.PC, interp.Cycles, interpBus.Read8(0x0200))
	}
}

func TestJIT6502_ARM64_NativeIndirectStores(t *testing.T) {
	program := []byte{0xA9, 0x5A, 0xA2, 0x02, 0x81, 0xFE, 0xA0, 0x03, 0x91, 0x10, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x00)
		bus.Write8(0x0001, 0x02)
		bus.Write8(0x0010, 0xFD)
		bus.Write8(0x0011, 0x01)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X PC=%04X cycles=%d mem=%02X; interpreter A=%02X X=%02X Y=%02X PC=%04X cycles=%d mem=%02X",
			jit.A, jit.X, jit.Y, jit.PC, jit.Cycles, jitBus.Read8(0x0200), interp.A, interp.X, interp.Y, interp.PC, interp.Cycles, interpBus.Read8(0x0200))
	}
}

func TestJIT6502_ARM64_NativeIndirectLoads(t *testing.T) {
	program := []byte{0xA2, 0x02, 0xA1, 0xFE, 0xA0, 0x03, 0xB1, 0x10, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x00)
		bus.Write8(0x0001, 0x02)
		bus.Write8(0x0010, 0xFD)
		bus.Write8(0x0011, 0x01)
		bus.Write8(0x0200, 0x80)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeIndirectLogic(t *testing.T) {
	program := []byte{0xA9, 0xF0, 0xA2, 0x02, 0x01, 0xFE, 0x21, 0xFE, 0x41, 0xFE, 0xA0, 0x03, 0x11, 0x10, 0x31, 0x10, 0x51, 0x10, 0xA9, 0x55, 0xC1, 0xFE, 0xA9, 0x54, 0xD1, 0x10, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x00)
		bus.Write8(0x0001, 0x02)
		bus.Write8(0x0010, 0xFD)
		bus.Write8(0x0011, 0x01)
		bus.Write8(0x0200, 0x0F)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	_, interp := newCPU()
	interp.Execute()
	_, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeAbsoluteIndexedLoad(t *testing.T) {
	program := []byte{0xA2, 0x02, 0xBD, 0xFE, 0x01, 0xA0, 0x03, 0xBE, 0xFD, 0x01, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x80)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeDirectCompares(t *testing.T) {
	program := []byte{0xA9, 0x10, 0xA2, 0x80, 0xA0, 0x10, 0xC5, 0x10, 0xEC, 0x11, 0x00, 0xCC, 0x12, 0x00, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0x10)
		bus.Write8(0x0011, 0x81)
		bus.Write8(0x0012, 0x11)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeDirectArithmetic(t *testing.T) {
	program := []byte{0xA9, 0x45, 0x38, 0x65, 0x10, 0xF8, 0xED, 0x11, 0x00, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0x55)
		bus.Write8(0x0011, 0x01)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeIndirectArithmetic(t *testing.T) {
	program := []byte{0xA2, 0x02, 0xA0, 0x03, 0xA9, 0x45, 0x38, 0x61, 0xFE, 0xF8, 0xA9, 0x45, 0x38, 0x71, 0x10, 0xA9, 0x45, 0x38, 0xE1, 0xFE, 0xD8, 0xA9, 0x45, 0x38, 0xF1, 0x10, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x00)
		bus.Write8(0x0001, 0x02)
		bus.Write8(0x0010, 0xFD)
		bus.Write8(0x0011, 0x01)
		bus.Write8(0x0200, 0x55)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeZeroPageIndexedArithmetic(t *testing.T) {
	program := []byte{0xA2, 0x02, 0xA9, 0x45, 0x38, 0x75, 0xFE, 0xF8, 0xA9, 0x45, 0x38, 0xF5, 0xFE, 0xA9, 0x54, 0xD5, 0xFE, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x55)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeAbsoluteIndexedArithmetic(t *testing.T) {
	program := []byte{0xA2, 0x02, 0xA0, 0x03, 0xA9, 0x45, 0x38, 0x7D, 0xFE, 0x01, 0xF8, 0xA9, 0x45, 0x38, 0x79, 0xFD, 0x01, 0xA9, 0x45, 0x38, 0xFD, 0xFE, 0x01, 0xD8, 0xA9, 0x45, 0x38, 0xF9, 0xFD, 0x01, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x55)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeAbsoluteIndexedLogic(t *testing.T) {
	program := []byte{0xA2, 0x02, 0xA0, 0x03, 0xA9, 0xF0, 0x1D, 0xFE, 0x01, 0x3D, 0xFE, 0x01, 0x5D, 0xFE, 0x01, 0x19, 0xFD, 0x01, 0x39, 0xFD, 0x01, 0x59, 0xFD, 0x01, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x0F)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d", jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeAbsoluteIndexedCompare(t *testing.T) {
	// Both operands cross from $01FE into $0200, exercising CMP's extra cycle
	// as well as the carry-set and carry-clear paths.
	program := []byte{0xA2, 0x02, 0xA0, 0x03, 0xA9, 0x55, 0xDD, 0xFE, 0x01, 0xD9, 0xFD, 0x01, haltOpcode}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x55)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d", jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeDirectShifts(t *testing.T) {
	program := []byte{0x06, 0x10, 0x46, 0x11, 0x38, 0x26, 0x12, 0x6E, 0x13, 0x00, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0x80)
		bus.Write8(0x0011, 0x01)
		bus.Write8(0x0012, 0x80)
		bus.Write8(0x0013, 0x01)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	for _, address := range []uint32{0x10, 0x11, 0x12, 0x13} {
		if got, want := jitBus.Read8(address), interpBus.Read8(address); got != want {
			t.Fatalf("memory[$%04X]=%02X, want %02X", address, got, want)
		}
	}
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d; interpreter SR=%02X PC=%04X cycles=%d", jit.SR, jit.PC, jit.Cycles, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeZeroPageIndexedShifts(t *testing.T) {
	program := []byte{0xA2, 0x02, 0x16, 0xFE, 0x38, 0x36, 0xFF, 0x56, 0xFE, 0x38, 0x76, 0xFF, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x80)
		bus.Write8(0x0001, 0x80)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0) != interpBus.Read8(0) || jitBus.Read8(1) != interpBus.Read8(1) {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d mem=%02X/%02X; interpreter SR=%02X PC=%04X cycles=%d mem=%02X/%02X", jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0), jitBus.Read8(1), interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0), interpBus.Read8(1))
	}
}

func TestJIT6502_ARM64_NativeAbsoluteIndexedINCDEC(t *testing.T) {
	program := []byte{0xA2, 0x02, 0xFE, 0xFE, 0x01, 0xDE, 0xFE, 0x01, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0xFF)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d mem=%02X; interpreter SR=%02X PC=%04X cycles=%d mem=%02X", jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0200), interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0200))
	}
}

func TestJIT6502_ARM64_NativeAbsoluteIndexedShifts(t *testing.T) {
	program := []byte{0xA2, 0x02, 0x1E, 0xFE, 0x01, 0x38, 0x3E, 0xFF, 0x01, 0x5E, 0xFE, 0x01, 0x38, 0x7E, 0xFF, 0x01, haltOpcode}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x80)
		bus.Write8(0x0201, 0x80)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) || jitBus.Read8(0x0201) != interpBus.Read8(0x0201) {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d mem=%02X/%02X; interpreter SR=%02X PC=%04X cycles=%d mem=%02X/%02X", jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0200), jitBus.Read8(0x0201), interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0200), interpBus.Read8(0x0201))
	}
}

func TestJIT6502_ARM64_NativeZeroPageIndexedLoads(t *testing.T) {
	program := []byte{
		0xA2, 0x02, // LDX #2
		0xB5, 0xFF, // LDA $FF,X -> $01
		0xA0, 0x03, // LDY #3
		0xB6, 0xFE, // LDX $FE,Y -> $01
		haltOpcode,
	}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0001, 0x5A)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT PC=%04X A=%02X X=%02X Y=%02X SR=%02X cycles=%d; interpreter PC=%04X A=%02X X=%02X Y=%02X SR=%02X cycles=%d",
			jit.PC, jit.A, jit.X, jit.Y, jit.SR, jit.Cycles, interp.PC, interp.A, interp.X, interp.Y, interp.SR, interp.Cycles)
	}
}

func TestJIT6502_ARM64_NativeZeroPageIndexedStore(t *testing.T) {
	program := []byte{
		0xA9, 0xA5, // LDA #$A5
		0xA2, 0x02, // LDX #2
		0x95, 0xFF, // STA $FF,X -> $01
		haltOpcode,
	}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.ExecuteJIT6502()
	if got, want := jitBus.Read8(0x0001), interpBus.Read8(0x0001); got != want {
		t.Fatalf("indexed store=%02X, want %02X", got, want)
	}
	if jit.Cycles != interp.Cycles || jit.PC != interp.PC {
		t.Fatalf("JIT PC=%04X cycles=%d; interpreter PC=%04X cycles=%d", jit.PC, jit.Cycles, interp.PC, interp.Cycles)
	}
}

func TestJIT6502_ARM64_ContextABI(t *testing.T) {
	ctx := &JIT6502Context{}
	if got, want := j65CtxOffBackendMarker, 144; got != want {
		t.Fatalf("backend marker offset=%d, want %d", got, want)
	}
	ctx.BackendMarker = p65ARM64BackendMarker
	if ctx.BackendMarker != p65ARM64BackendMarker {
		t.Fatal("ARM64 provenance marker did not round-trip")
	}
}
