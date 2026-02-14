package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Address parsing
// ---------------------------------------------------------------------------

func TestAddressParsing(t *testing.T) {
	tests := []struct {
		input string
		want  uint64
		ok    bool
	}{
		{"$1000", 0x1000, true},
		{"0x1000", 0x1000, true},
		{"1000", 0x1000, true},
		{"#4096", 4096, true},
		{"$DEAD", 0xDEAD, true},
		{"0XBEEF", 0xBEEF, true},
		{"FF", 0xFF, true},
		{"#0", 0, true},
		{"$0", 0, true},
		{"", 0, false},
	}

	for _, tt := range tests {
		got, ok := ParseAddress(tt.input)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("ParseAddress(%q) = (%X, %v), want (%X, %v)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

// ---------------------------------------------------------------------------
// Command parsing
// ---------------------------------------------------------------------------

func TestCommandParsing(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs []string
	}{
		{"r pc 1000", "r", []string{"pc", "1000"}},
		{"d", "d", nil},
		{"  m  $1000  8  ", "m", []string{"$1000", "8"}},
		{"s", "s", nil},
		{"g $2000", "g", []string{"$2000"}},
		{"", "", nil},
		{"cpu ie64", "cpu", []string{"ie64"}},
	}

	for _, tt := range tests {
		cmd := ParseCommand(tt.input)
		if cmd.Name != tt.wantName {
			t.Errorf("ParseCommand(%q).Name = %q, want %q", tt.input, cmd.Name, tt.wantName)
		}
		if len(cmd.Args) != len(tt.wantArgs) {
			t.Errorf("ParseCommand(%q).Args = %v, want %v", tt.input, cmd.Args, tt.wantArgs)
		}
	}
}

// ---------------------------------------------------------------------------
// Monitor activate/deactivate
// ---------------------------------------------------------------------------

func newTestMonitor() (*MachineMonitor, *CPU64) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)
	return mon, cpu
}

func TestMonitorActivateDeactivate(t *testing.T) {
	mon, _ := newTestMonitor()

	if mon.IsActive() {
		t.Fatal("Monitor should start inactive")
	}

	mon.Activate()
	if !mon.IsActive() {
		t.Fatal("Monitor should be active after Activate()")
	}

	mon.Deactivate()
	if mon.IsActive() {
		t.Fatal("Monitor should be inactive after Deactivate()")
	}
}

// ---------------------------------------------------------------------------
// Freeze/Resume
// ---------------------------------------------------------------------------

func TestMonitorFreezeResume(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Write a NOP loop so Execute() doesn't immediately crash
	// NOP = 0xE0, rest zeros
	for addr := PROG_START; addr < PROG_START+80; addr += 8 {
		cpu.memory[addr] = OP_NOP64
	}
	// BRA back to start at end
	cpu.memory[PROG_START+80] = OP_BRA
	// imm32 offset = -(80+8) = -88 as int32
	offset := int32(-88)
	uoff := uint32(offset)
	cpu.memory[PROG_START+84] = byte(uoff)
	cpu.memory[PROG_START+85] = byte(uoff >> 8)
	cpu.memory[PROG_START+86] = byte(uoff >> 16)
	cpu.memory[PROG_START+87] = byte(uoff >> 24)

	cpu.StartExecution()

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)

	// CPU should be running
	if !adapter.IsRunning() {
		t.Fatal("CPU should be running after StartExecution()")
	}

	mon.Activate()

	// CPU should be frozen after activation
	if adapter.IsRunning() {
		t.Fatal("CPU should be frozen after monitor activation")
	}

	mon.Deactivate()

	// CPU should be running again
	if !adapter.IsRunning() {
		t.Fatal("CPU should be running after monitor deactivation")
	}

	// Clean up
	cpu.Stop()
}

// ---------------------------------------------------------------------------
// IE64 Disassemble
// ---------------------------------------------------------------------------

func TestIE64Disassemble(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)

	// Write move.l r1, #$42 at PROG_START
	// opcode=0x01, byte1: rd=1(<<3)=0x08 | size=L(2<<1)=0x04 | xbit=1 = 0x0D
	cpu.memory[PROG_START] = OP_MOVE             // 0x01
	cpu.memory[PROG_START+1] = (1<<3 | 2<<1 | 1) // rd=1, size=L, xbit=1
	cpu.memory[PROG_START+2] = 0                 // rs=0
	cpu.memory[PROG_START+3] = 0                 // rt=0
	cpu.memory[PROG_START+4] = 0x42              // imm32 lo
	cpu.memory[PROG_START+5] = 0
	cpu.memory[PROG_START+6] = 0
	cpu.memory[PROG_START+7] = 0

	adapter := NewDebugIE64(cpu)
	lines := adapter.Disassemble(uint64(PROG_START), 1)

	if len(lines) != 1 {
		t.Fatalf("Expected 1 disassembled line, got %d", len(lines))
	}
	if lines[0].Size != 8 {
		t.Errorf("Expected size 8, got %d", lines[0].Size)
	}
	if !strings.Contains(lines[0].Mnemonic, "move") {
		t.Errorf("Expected mnemonic containing 'move', got %q", lines[0].Mnemonic)
	}
	if !strings.Contains(lines[0].Mnemonic, "r1") {
		t.Errorf("Expected mnemonic containing 'r1', got %q", lines[0].Mnemonic)
	}
}

// ---------------------------------------------------------------------------
// IE64 Registers
// ---------------------------------------------------------------------------

func TestIE64Registers(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)
	cpu.regs[1] = 0xDEADBEEF
	cpu.regs[5] = 0x1234

	adapter := NewDebugIE64(cpu)
	regs := adapter.GetRegisters()

	// Should have R0-R30 + SP + PC = 33
	if len(regs) != 33 {
		t.Fatalf("Expected 33 registers, got %d", len(regs))
	}

	// Check R1
	val, ok := adapter.GetRegister("r1")
	if !ok || val != 0xDEADBEEF {
		t.Errorf("GetRegister(R1) = (%X, %v), want (DEADBEEF, true)", val, ok)
	}

	// Check PC
	val, ok = adapter.GetRegister("PC")
	if !ok {
		t.Error("GetRegister(PC) should return ok=true")
	}
	if val != PROG_START {
		t.Errorf("GetRegister(PC) = %X, want %X", val, PROG_START)
	}

	// Check SP (R31)
	val, ok = adapter.GetRegister("SP")
	if !ok {
		t.Error("GetRegister(SP) should return ok=true")
	}
}

// ---------------------------------------------------------------------------
// IE64 StepOne
// ---------------------------------------------------------------------------

func TestIE64StepOne(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)

	// move.l r1, #42
	cpu.memory[PROG_START] = OP_MOVE
	cpu.memory[PROG_START+1] = (1<<3 | 2<<1 | 1) // rd=1, size=L, xbit=1
	cpu.memory[PROG_START+4] = 42

	adapter := NewDebugIE64(cpu)
	cycles := adapter.Step()

	if cycles != 1 {
		t.Errorf("Step() returned %d cycles, expected 1", cycles)
	}
	if cpu.regs[1] != 42 {
		t.Errorf("After step, R1 = %d, expected 42", cpu.regs[1])
	}
	if cpu.PC != PROG_START+8 {
		t.Errorf("After step, PC = %X, expected %X", cpu.PC, PROG_START+8)
	}
}

// ---------------------------------------------------------------------------
// IE64 SetRegister
// ---------------------------------------------------------------------------

func TestIE64SetRegister(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)

	adapter := NewDebugIE64(cpu)
	ok := adapter.SetRegister("pc", 0x2000)
	if !ok {
		t.Fatal("SetRegister(pc) should succeed")
	}
	if cpu.PC != 0x2000 {
		t.Errorf("PC = %X, want 2000", cpu.PC)
	}

	ok = adapter.SetRegister("r0", 42)
	if ok {
		t.Fatal("SetRegister(r0) should fail (R0 hardwired to zero)")
	}
}

// ---------------------------------------------------------------------------
// IE64 ReadWriteMemory
// ---------------------------------------------------------------------------

func TestIE64ReadWriteMemory(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)

	adapter := NewDebugIE64(cpu)
	adapter.WriteMemory(0x5000, []byte{0xDE, 0xAD, 0xBE, 0xEF})

	data := adapter.ReadMemory(0x5000, 4)
	if len(data) != 4 {
		t.Fatalf("ReadMemory returned %d bytes, expected 4", len(data))
	}
	if data[0] != 0xDE || data[1] != 0xAD || data[2] != 0xBE || data[3] != 0xEF {
		t.Errorf("ReadMemory returned %X, expected DEADBEEF", data)
	}
}

// ---------------------------------------------------------------------------
// Command: register display
// ---------------------------------------------------------------------------

func TestCommandRegisterDisplay(t *testing.T) {
	mon, cpu := newTestMonitor()
	cpu.regs[1] = 0x42

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("r")
	lines := mon.outputLines
	mon.mu.Unlock()

	if len(lines) == 0 {
		t.Fatal("Expected output from 'r' command")
	}

	// Check that register values appear in output
	found := false
	for _, line := range lines {
		if strings.Contains(line.Text, "R1") && strings.Contains(line.Text, "42") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Register R1=42 not found in output")
	}
}

// ---------------------------------------------------------------------------
// Command: disassemble
// ---------------------------------------------------------------------------

func TestCommandDisassemble(t *testing.T) {
	mon, cpu := newTestMonitor()

	// Write a NOP at PC
	cpu.memory[PROG_START] = OP_NOP64

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("d")
	lines := mon.outputLines
	mon.mu.Unlock()

	if len(lines) == 0 {
		t.Fatal("Expected output from 'd' command")
	}
}

// ---------------------------------------------------------------------------
// Command: memory dump
// ---------------------------------------------------------------------------

func TestCommandMemoryDump(t *testing.T) {
	mon, cpu := newTestMonitor()
	cpu.memory[0x100] = 0x42

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("m 100 1")
	lines := mon.outputLines
	mon.mu.Unlock()

	if len(lines) == 0 {
		t.Fatal("Expected output from 'm' command")
	}
	// Check hex dump contains 42
	found := false
	for _, line := range lines {
		if strings.Contains(line.Text, "42") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Memory dump should contain 42")
	}
}

// ---------------------------------------------------------------------------
// Command: step
// ---------------------------------------------------------------------------

func TestCommandStep(t *testing.T) {
	mon, cpu := newTestMonitor()

	// Write move.l r1, #42 at PC
	cpu.memory[PROG_START] = OP_MOVE
	cpu.memory[PROG_START+1] = (1<<3 | 2<<1 | 1)
	cpu.memory[PROG_START+4] = 42

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("s")
	mon.mu.Unlock()

	if cpu.regs[1] != 42 {
		t.Errorf("After step, R1 = %d, expected 42", cpu.regs[1])
	}
	if cpu.PC != PROG_START+8 {
		t.Errorf("After step, PC = %X, expected %X", cpu.PC, PROG_START+8)
	}
}

// ---------------------------------------------------------------------------
// Command: go
// ---------------------------------------------------------------------------

func TestCommandGo(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.mu.Lock()
	exit := mon.ExecuteCommand("g")
	mon.mu.Unlock()

	if !exit {
		t.Error("'g' command should signal exit")
	}
}

// ---------------------------------------------------------------------------
// Command: exit
// ---------------------------------------------------------------------------

func TestCommandExit(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.mu.Lock()
	exit := mon.ExecuteCommand("x")
	mon.mu.Unlock()

	if !exit {
		t.Error("'x' command should signal exit")
	}
}

// ---------------------------------------------------------------------------
// Command: help
// ---------------------------------------------------------------------------

func TestCommandHelp(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("?")
	lines := mon.outputLines
	mon.mu.Unlock()

	if len(lines) == 0 {
		t.Fatal("Expected help text output")
	}
}

// ---------------------------------------------------------------------------
// Command: cpu list
// ---------------------------------------------------------------------------

func TestCommandCpuList(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("cpu")
	lines := mon.outputLines
	mon.mu.Unlock()

	if len(lines) == 0 {
		t.Fatal("Expected CPU list output")
	}

	found := false
	for _, line := range lines {
		if strings.Contains(line.Text, "IE64") {
			found = true
			break
		}
	}
	if !found {
		t.Error("CPU list should contain 'IE64'")
	}
}

// ---------------------------------------------------------------------------
// Breakpoint set/clear/list
// ---------------------------------------------------------------------------

func TestBreakpointSetClearList(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.mu.Lock()
	mon.ExecuteCommand("b $1000")
	mon.mu.Unlock()

	entry := mon.FocusedCPU()
	if entry == nil {
		t.Fatal("No focused CPU")
	}
	bps := entry.CPU.ListBreakpoints()
	if len(bps) != 1 || bps[0] != 0x1000 {
		t.Errorf("Expected breakpoint at $1000, got %v", bps)
	}

	mon.mu.Lock()
	mon.ExecuteCommand("bc $1000")
	mon.mu.Unlock()

	bps = entry.CPU.ListBreakpoints()
	if len(bps) != 0 {
		t.Errorf("Expected no breakpoints after clear, got %v", bps)
	}

	// Set multiple and clear all
	mon.mu.Lock()
	mon.ExecuteCommand("b $1000")
	mon.ExecuteCommand("b $2000")
	mon.ExecuteCommand("bc *")
	mon.mu.Unlock()

	bps = entry.CPU.ListBreakpoints()
	if len(bps) != 0 {
		t.Errorf("Expected no breakpoints after bc *, got %v", bps)
	}
}

// ---------------------------------------------------------------------------
// Memory fill
// ---------------------------------------------------------------------------

func TestMemoryFill(t *testing.T) {
	mon, cpu := newTestMonitor()

	mon.mu.Lock()
	mon.ExecuteCommand("f 1000 100F 42")
	mon.mu.Unlock()

	for i := range 16 {
		if cpu.memory[0x1000+i] != 0x42 {
			t.Errorf("memory[%X] = %02X, expected 42", 0x1000+i, cpu.memory[0x1000+i])
		}
	}
}

// ---------------------------------------------------------------------------
// Memory write
// ---------------------------------------------------------------------------

func TestMemoryWrite(t *testing.T) {
	mon, cpu := newTestMonitor()

	mon.mu.Lock()
	mon.ExecuteCommand("w 1000 DE AD BE EF")
	mon.mu.Unlock()

	if cpu.memory[0x1000] != 0xDE || cpu.memory[0x1001] != 0xAD ||
		cpu.memory[0x1002] != 0xBE || cpu.memory[0x1003] != 0xEF {
		t.Errorf("memory at $1000 = %02X %02X %02X %02X, expected DE AD BE EF",
			cpu.memory[0x1000], cpu.memory[0x1001], cpu.memory[0x1002], cpu.memory[0x1003])
	}
}

// ---------------------------------------------------------------------------
// Memory hunt
// ---------------------------------------------------------------------------

func TestMemoryHunt(t *testing.T) {
	mon, cpu := newTestMonitor()

	cpu.memory[0x2000] = 0xDE
	cpu.memory[0x2001] = 0xAD

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("h 0 FFFF DE AD")
	lines := mon.outputLines
	mon.mu.Unlock()

	found := false
	for _, line := range lines {
		if strings.Contains(line.Text, "2000") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Hunt should find pattern at $2000")
	}
}

// ---------------------------------------------------------------------------
// Memory compare
// ---------------------------------------------------------------------------

func TestMemoryCompare(t *testing.T) {
	mon, cpu := newTestMonitor()

	// Same data
	cpu.memory[0x1000] = 0x42
	cpu.memory[0x2000] = 0x42

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("c 1000 100F 2000")
	lines := mon.outputLines
	mon.mu.Unlock()

	// Should have some output (differences at bytes 1-15 which are both 0)
	_ = lines
}

// ---------------------------------------------------------------------------
// Memory transfer
// ---------------------------------------------------------------------------

func TestMemoryTransfer(t *testing.T) {
	mon, cpu := newTestMonitor()

	cpu.memory[0x1000] = 0xDE
	cpu.memory[0x1001] = 0xAD

	mon.mu.Lock()
	mon.ExecuteCommand("t 1000 1001 2000")
	mon.mu.Unlock()

	if cpu.memory[0x2000] != 0xDE || cpu.memory[0x2001] != 0xAD {
		t.Errorf("Transfer failed: memory at $2000 = %02X %02X", cpu.memory[0x2000], cpu.memory[0x2001])
	}
}

// ---------------------------------------------------------------------------
// Stable CPU IDs
// ---------------------------------------------------------------------------

func TestStableCPUIDs(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)

	cpu1 := NewCPU64(bus)
	cpu1.running.Store(false)
	cpu2 := NewCPU64(bus)
	cpu2.running.Store(false)
	cpu3 := NewCPU64(bus)
	cpu3.running.Store(false)

	id0 := mon.RegisterCPU("IE64", NewDebugIE64(cpu1))
	id1 := mon.RegisterCPU("Z80", NewDebugIE64(cpu2)) // using IE64 adapter for simplicity
	id2 := mon.RegisterCPU("6502", NewDebugIE64(cpu3))

	if id0 != 0 || id1 != 1 || id2 != 2 {
		t.Errorf("IDs = %d, %d, %d; expected 0, 1, 2", id0, id1, id2)
	}

	// Unregister id:1
	mon.UnregisterCPU(1)

	// Register another — should get id:3 (never reuse)
	cpu4 := NewCPU64(bus)
	cpu4.running.Store(false)
	id3 := mon.RegisterCPU("M68K", NewDebugIE64(cpu4))
	if id3 != 3 {
		t.Errorf("New CPU got id:%d, expected 3", id3)
	}
}

// ---------------------------------------------------------------------------
// Command history
// ---------------------------------------------------------------------------

func TestCommandHistory(t *testing.T) {
	mon, _ := newTestMonitor()

	mon.mu.Lock()
	mon.ExecuteCommand("r")
	mon.ExecuteCommand("d")
	mon.ExecuteCommand("m 0")
	mon.mu.Unlock()

	if len(mon.history) != 3 {
		t.Errorf("Expected 3 history entries, got %d", len(mon.history))
	}
	if mon.history[0] != "r" || mon.history[1] != "d" || mon.history[2] != "m 0" {
		t.Errorf("History = %v", mon.history)
	}
}

// ===========================================================================
// Phase 2: IE32 Adapter + Disassembler
// ===========================================================================

func TestIE32Disassemble(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	// Write a LOAD A, #$42 instruction at address 0
	// IE32 instructions are 8 bytes: opcode, reg, addrMode, pad, operand[4]
	cpu.memory[0] = LOAD           // opcode
	cpu.memory[1] = 0              // reg A (index 0)
	cpu.memory[2] = ADDR_IMMEDIATE // immediate mode
	cpu.memory[3] = 0              // pad
	cpu.memory[4] = 0x42           // operand lo
	cpu.memory[5] = 0
	cpu.memory[6] = 0
	cpu.memory[7] = 0

	adapter := NewDebugIE32(cpu)
	lines := adapter.Disassemble(0, 1)

	if len(lines) != 1 {
		t.Fatalf("Expected 1 disassembled line, got %d", len(lines))
	}
	if lines[0].Size != 8 {
		t.Errorf("Expected size 8, got %d", lines[0].Size)
	}
	if !strings.Contains(lines[0].Mnemonic, "LOAD") {
		t.Errorf("Expected mnemonic containing 'LOAD', got %q", lines[0].Mnemonic)
	}
}

func TestIE32Registers(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	cpu.A = 0x42
	cpu.X = 0x1234

	adapter := NewDebugIE32(cpu)
	regs := adapter.GetRegisters()

	// Should have 16 GP + SP + PC = 18
	if len(regs) != 18 {
		t.Fatalf("Expected 18 registers, got %d", len(regs))
	}

	val, ok := adapter.GetRegister("A")
	if !ok || val != 0x42 {
		t.Errorf("GetRegister(A) = (%X, %v), want (42, true)", val, ok)
	}

	val, ok = adapter.GetRegister("X")
	if !ok || val != 0x1234 {
		t.Errorf("GetRegister(X) = (%X, %v), want (1234, true)", val, ok)
	}

	val, ok = adapter.GetRegister("PC")
	if !ok {
		t.Error("GetRegister(PC) should return ok=true")
	}
}

func TestIE32StepOne(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	// LDA #42 at PC=0
	cpu.PC = 0
	cpu.memory[0] = LDA
	cpu.memory[1] = 0 // reg index (A)
	cpu.memory[2] = ADDR_IMMEDIATE
	cpu.memory[4] = 42

	adapter := NewDebugIE32(cpu)
	cycles := adapter.Step()

	if cycles == 0 {
		t.Error("Step() returned 0 cycles")
	}
	if cpu.A != 42 {
		t.Errorf("After step, A = %d, expected 42", cpu.A)
	}
	if cpu.PC != 8 {
		t.Errorf("After step, PC = %X, expected 8", cpu.PC)
	}
}

// ===========================================================================
// Phase 2: 6502 Adapter + Disassembler
// ===========================================================================

func TestDisassemble6502(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)

	mem := bus.GetMemory()
	// LDA #$42 at address $0600
	mem[0x0600] = 0xA9 // LDA immediate
	mem[0x0601] = 0x42

	// STA $1234 at $0602
	mem[0x0602] = 0x8D // STA absolute
	mem[0x0603] = 0x34
	mem[0x0604] = 0x12

	adapter := NewDebug6502(cpu, nil)
	lines := adapter.Disassemble(0x0600, 2)

	if len(lines) != 2 {
		t.Fatalf("Expected 2 disassembled lines, got %d", len(lines))
	}

	// Check LDA #$42
	if lines[0].Size != 2 {
		t.Errorf("LDA #imm size = %d, expected 2", lines[0].Size)
	}
	if !strings.Contains(lines[0].Mnemonic, "LDA") {
		t.Errorf("Expected mnemonic containing 'LDA', got %q", lines[0].Mnemonic)
	}
	if !strings.Contains(lines[0].Mnemonic, "42") {
		t.Errorf("Expected mnemonic containing '42', got %q", lines[0].Mnemonic)
	}

	// Check STA $1234
	if lines[1].Size != 3 {
		t.Errorf("STA abs size = %d, expected 3", lines[1].Size)
	}
	if !strings.Contains(lines[1].Mnemonic, "STA") {
		t.Errorf("Expected mnemonic containing 'STA', got %q", lines[1].Mnemonic)
	}
	if !strings.Contains(lines[1].Mnemonic, "1234") {
		t.Errorf("Expected mnemonic containing '1234', got %q", lines[1].Mnemonic)
	}
}

func TestCPU6502Registers(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.A = 0x42
	cpu.X = 0x10
	cpu.Y = 0x20
	cpu.SP = 0xFD
	cpu.PC = 0x0600

	adapter := NewDebug6502(cpu, nil)

	regs := adapter.GetRegisters()
	// A, X, Y, SP, PC, SR = 6
	if len(regs) != 6 {
		t.Fatalf("Expected 6 registers, got %d", len(regs))
	}

	val, ok := adapter.GetRegister("A")
	if !ok || val != 0x42 {
		t.Errorf("GetRegister(A) = (%X, %v), want (42, true)", val, ok)
	}

	val, ok = adapter.GetRegister("PC")
	if !ok || val != 0x0600 {
		t.Errorf("GetRegister(PC) = (%X, %v), want (600, true)", val, ok)
	}
}

func TestCPU6502Step(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.rdyLine.Store(true)

	mem := bus.GetMemory()
	// LDA #$42
	mem[0x0600] = 0xA9
	mem[0x0601] = 0x42

	adapter := NewDebug6502(cpu, nil)
	cycles := adapter.Step()

	if cycles == 0 {
		t.Error("Step() returned 0 cycles")
	}
	if cpu.A != 0x42 {
		t.Errorf("After step, A = %X, expected 42", cpu.A)
	}
}

// ===========================================================================
// Phase 2: Z80 Adapter + Disassembler
// ===========================================================================

type testZ80Bus struct {
	mem   [0x10000]byte
	io    [0x10000]byte
	ticks uint64
}

func (b *testZ80Bus) Read(addr uint16) byte         { return b.mem[addr] }
func (b *testZ80Bus) Write(addr uint16, value byte) { b.mem[addr] = value }
func (b *testZ80Bus) In(port uint16) byte           { return b.io[port] }
func (b *testZ80Bus) Out(port uint16, value byte)   { b.io[port] = value }
func (b *testZ80Bus) Tick(cycles int)               { b.ticks += uint64(cycles) }

func TestZ80Disassemble(t *testing.T) {
	bus := &testZ80Bus{}
	cpu := NewCPU_Z80(bus)

	// LD A, $42
	bus.mem[0x0000] = 0x3E // LD A, n
	bus.mem[0x0001] = 0x42

	// NOP
	bus.mem[0x0002] = 0x00

	// CB prefix: BIT 3, A
	bus.mem[0x0003] = 0xCB
	bus.mem[0x0004] = 0x5F // BIT 3, A

	adapter := NewDebugZ80(cpu, nil)
	lines := adapter.Disassemble(0, 3)

	if len(lines) != 3 {
		t.Fatalf("Expected 3 disassembled lines, got %d", len(lines))
	}

	// LD A, $42
	if lines[0].Size != 2 {
		t.Errorf("LD A,n size = %d, expected 2", lines[0].Size)
	}
	if !strings.Contains(lines[0].Mnemonic, "LD A") {
		t.Errorf("Expected 'LD A', got %q", lines[0].Mnemonic)
	}

	// NOP
	if lines[1].Mnemonic != "NOP" {
		t.Errorf("Expected 'NOP', got %q", lines[1].Mnemonic)
	}

	// BIT 3, A
	if lines[2].Size != 2 {
		t.Errorf("BIT 3,A size = %d, expected 2", lines[2].Size)
	}
	if !strings.Contains(lines[2].Mnemonic, "BIT 3") {
		t.Errorf("Expected 'BIT 3', got %q", lines[2].Mnemonic)
	}
}

func TestZ80Registers(t *testing.T) {
	bus := &testZ80Bus{}
	cpu := NewCPU_Z80(bus)
	cpu.A = 0x42
	cpu.B = 0x10
	cpu.IX = 0x1234

	adapter := NewDebugZ80(cpu, nil)
	regs := adapter.GetRegisters()

	// 16 main + shadow, IX, IY, SP, PC, I, R, IM = 23
	if len(regs) != 23 {
		t.Fatalf("Expected 23 registers, got %d", len(regs))
	}

	val, ok := adapter.GetRegister("A")
	if !ok || val != 0x42 {
		t.Errorf("GetRegister(A) = (%X, %v), want (42, true)", val, ok)
	}

	val, ok = adapter.GetRegister("IX")
	if !ok || val != 0x1234 {
		t.Errorf("GetRegister(IX) = (%X, %v), want (1234, true)", val, ok)
	}
}

func TestZ80Step(t *testing.T) {
	bus := &testZ80Bus{}
	cpu := NewCPU_Z80(bus)
	cpu.PC = 0

	// LD A, $42
	bus.mem[0x0000] = 0x3E
	bus.mem[0x0001] = 0x42

	adapter := NewDebugZ80(cpu, nil)
	cycles := adapter.Step()

	if cycles == 0 {
		t.Error("Step() returned 0 cycles")
	}
	if cpu.A != 0x42 {
		t.Errorf("After step, A = %X, expected 42", cpu.A)
	}
}

// ===========================================================================
// Phase 2: M68K Adapter + Disassembler
// ===========================================================================

func TestM68KDisassemble(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	// NOP = 0x4E71 (big-endian)
	cpu.memory[0x1000] = 0x4E
	cpu.memory[0x1001] = 0x71

	// MOVEQ #42, D0 = 0x702A
	cpu.memory[0x1002] = 0x70
	cpu.memory[0x1003] = 0x2A

	adapter := NewDebugM68K(cpu, nil)
	lines := adapter.Disassemble(0x1000, 2)

	if len(lines) != 2 {
		t.Fatalf("Expected 2 disassembled lines, got %d", len(lines))
	}

	if lines[0].Mnemonic != "NOP" {
		t.Errorf("Expected 'NOP', got %q", lines[0].Mnemonic)
	}
	if lines[0].Size != 2 {
		t.Errorf("NOP size = %d, expected 2", lines[0].Size)
	}

	if !strings.Contains(lines[1].Mnemonic, "MOVEQ") {
		t.Errorf("Expected 'MOVEQ', got %q", lines[1].Mnemonic)
	}
	if !strings.Contains(lines[1].Mnemonic, "D0") {
		t.Errorf("Expected 'D0' in mnemonic, got %q", lines[1].Mnemonic)
	}
}

func TestM68KRegisters(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.DataRegs[0] = 0xDEADBEEF
	cpu.AddrRegs[7] = 0x9F000
	cpu.PC = 0x1000

	adapter := NewDebugM68K(cpu, nil)
	regs := adapter.GetRegisters()

	// D0-D7 + A0-A7 + PC + SR + USP = 19
	if len(regs) != 19 {
		t.Fatalf("Expected 19 registers, got %d", len(regs))
	}

	val, ok := adapter.GetRegister("D0")
	if !ok || val != 0xDEADBEEF {
		t.Errorf("GetRegister(D0) = (%X, %v), want (DEADBEEF, true)", val, ok)
	}

	val, ok = adapter.GetRegister("A7")
	if !ok || val != 0x9F000 {
		t.Errorf("GetRegister(A7) = (%X, %v), want (9F000, true)", val, ok)
	}

	val, ok = adapter.GetRegister("PC")
	if !ok || val != 0x1000 {
		t.Errorf("GetRegister(PC) = (%X, %v), want (1000, true)", val, ok)
	}
}

func TestM68KStepOne(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	// MOVEQ #42, D0 = 0x702A (big-endian)
	cpu.PC = 0x1000
	cpu.memory[0x1000] = 0x70
	cpu.memory[0x1001] = 0x2A

	adapter := NewDebugM68K(cpu, nil)
	cycles := adapter.Step()

	if cycles == 0 {
		t.Error("Step() returned 0 cycles")
	}
	if cpu.DataRegs[0] != 42 {
		t.Errorf("After step, D0 = %d, expected 42", cpu.DataRegs[0])
	}
}

// ===========================================================================
// Phase 2: X86 Adapter + Disassembler
// ===========================================================================

type testX86Bus struct {
	mem [0x100000]byte
	io  [0x10000]byte
}

func (b *testX86Bus) Read(addr uint32) byte         { return b.mem[addr&0xFFFFF] }
func (b *testX86Bus) Write(addr uint32, value byte) { b.mem[addr&0xFFFFF] = value }
func (b *testX86Bus) In(port uint16) byte           { return b.io[port] }
func (b *testX86Bus) Out(port uint16, value byte)   { b.io[port] = value }
func (b *testX86Bus) Tick(cycles int)               {}

func TestX86Disassemble(t *testing.T) {
	bus := &testX86Bus{}
	cpu := NewCPU_X86(bus)

	// NOP
	bus.mem[0x1000] = 0x90

	// MOV EAX, 0x00000042
	bus.mem[0x1001] = 0xB8
	bus.mem[0x1002] = 0x42
	bus.mem[0x1003] = 0x00
	bus.mem[0x1004] = 0x00
	bus.mem[0x1005] = 0x00

	adapter := NewDebugX86(cpu, nil)
	lines := adapter.Disassemble(0x1000, 2)

	if len(lines) != 2 {
		t.Fatalf("Expected 2 disassembled lines, got %d", len(lines))
	}

	if lines[0].Mnemonic != "NOP" {
		t.Errorf("Expected 'NOP', got %q", lines[0].Mnemonic)
	}
	if lines[0].Size != 1 {
		t.Errorf("NOP size = %d, expected 1", lines[0].Size)
	}

	if !strings.Contains(lines[1].Mnemonic, "MOV") {
		t.Errorf("Expected 'MOV', got %q", lines[1].Mnemonic)
	}
	if !strings.Contains(lines[1].Mnemonic, "EAX") {
		t.Errorf("Expected 'EAX' in mnemonic, got %q", lines[1].Mnemonic)
	}
}

func TestX86Registers(t *testing.T) {
	bus := &testX86Bus{}
	cpu := NewCPU_X86(bus)
	cpu.EAX = 0xDEADBEEF
	cpu.EBX = 0x1234
	cpu.EIP = 0x1000

	adapter := NewDebugX86(cpu, nil)
	regs := adapter.GetRegisters()

	// EAX-EDI (8) + EIP + FLAGS + CS-GS (6) = 16
	if len(regs) != 16 {
		t.Fatalf("Expected 16 registers, got %d", len(regs))
	}

	val, ok := adapter.GetRegister("EAX")
	if !ok || val != 0xDEADBEEF {
		t.Errorf("GetRegister(EAX) = (%X, %v), want (DEADBEEF, true)", val, ok)
	}

	val, ok = adapter.GetRegister("EIP")
	if !ok || val != 0x1000 {
		t.Errorf("GetRegister(EIP) = (%X, %v), want (1000, true)", val, ok)
	}
}

func TestX86Step(t *testing.T) {
	bus := &testX86Bus{}
	cpu := NewCPU_X86(bus)
	cpu.EIP = 0x1000

	// MOV EAX, 0x42
	bus.mem[0x1000] = 0xB8
	bus.mem[0x1001] = 0x42
	bus.mem[0x1002] = 0x00
	bus.mem[0x1003] = 0x00
	bus.mem[0x1004] = 0x00

	adapter := NewDebugX86(cpu, nil)
	cycles := adapter.Step()

	if cycles == 0 {
		t.Error("Step() returned 0 cycles")
	}
	if cpu.EAX != 0x42 {
		t.Errorf("After step, EAX = %X, expected 42", cpu.EAX)
	}
}

// ===========================================================================
// Phase 2: CPU Switching + Multi-CPU
// ===========================================================================

func TestCPUSwitching(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)

	cpu1 := NewCPU64(bus)
	cpu1.running.Store(false)
	cpu2 := NewCPU(bus)

	id0 := mon.RegisterCPU("IE64", NewDebugIE64(cpu1))
	id1 := mon.RegisterCPU("IE32", NewDebugIE32(cpu2))

	// Focus should start at first registered CPU
	focused := mon.FocusedCPU()
	if focused == nil || focused.ID != id0 {
		t.Fatalf("Expected focused CPU id=%d, got %v", id0, focused)
	}

	// Switch by ID
	mon.mu.Lock()
	mon.ExecuteCommand("cpu 1")
	mon.mu.Unlock()

	focused = mon.FocusedCPU()
	if focused == nil || focused.ID != id1 {
		t.Errorf("After 'cpu 1', focused CPU id=%d, expected %d", focused.ID, id1)
	}

	// Switch by label
	mon.mu.Lock()
	mon.ExecuteCommand("cpu ie64")
	mon.mu.Unlock()

	focused = mon.FocusedCPU()
	if focused == nil || focused.ID != id0 {
		t.Errorf("After 'cpu ie64', focused CPU id=%d, expected %d", focused.ID, id0)
	}
}

func TestMultiCPUFreeze(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)

	cpu1 := NewCPU64(bus)
	cpu1.running.Store(false)
	cpu2 := NewCPU(bus)
	cpu2.running.Store(false)

	a1 := NewDebugIE64(cpu1)
	a2 := NewDebugIE32(cpu2)

	mon.RegisterCPU("IE64", a1)
	mon.RegisterCPU("IE32", a2)

	// Both should not be running initially
	if a1.IsRunning() {
		t.Error("IE64 should not be running")
	}
	if a2.IsRunning() {
		t.Error("IE32 should not be running")
	}
}

// ===========================================================================
// Phase 3: Breakpoint Runtime Trap
// ===========================================================================

func TestBreakpointRuntimeTrap(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Write a NOP loop: 10 NOPs + BRA back
	for i := range uint32(10) {
		cpu.memory[PROG_START+i*8] = OP_NOP64
	}
	// BRA back to start
	cpu.memory[PROG_START+80] = OP_BRA
	offset := int32(-88)
	uoff := uint32(offset)
	cpu.memory[PROG_START+84] = byte(uoff)
	cpu.memory[PROG_START+85] = byte(uoff >> 8)
	cpu.memory[PROG_START+86] = byte(uoff >> 16)
	cpu.memory[PROG_START+87] = byte(uoff >> 24)

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)

	// Set breakpoint at 3rd instruction
	bpAddr := uint64(PROG_START + 16)
	adapter.SetBreakpoint(bpAddr)

	// Resume CPU (should enter trap mode due to breakpoint)
	adapter.Resume()

	// Wait for breakpoint event with timeout
	select {
	case ev := <-mon.breakpointChan:
		if ev.Address != bpAddr {
			t.Errorf("Breakpoint event address = %X, expected %X", ev.Address, bpAddr)
		}
		if ev.CPUID != 0 {
			t.Errorf("Breakpoint event CPUID = %d, expected 0", ev.CPUID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for breakpoint event")
		adapter.Freeze()
	}
}

func TestBreakpointAutoActivation(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Write NOPs + BRA loop
	for i := range uint32(10) {
		cpu.memory[PROG_START+i*8] = OP_NOP64
	}
	cpu.memory[PROG_START+80] = OP_BRA
	offset := int32(-88)
	uoff := uint32(offset)
	cpu.memory[PROG_START+84] = byte(uoff)
	cpu.memory[PROG_START+85] = byte(uoff >> 8)
	cpu.memory[PROG_START+86] = byte(uoff >> 16)
	cpu.memory[PROG_START+87] = byte(uoff >> 24)

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)
	mon.StartBreakpointListener()

	// Set breakpoint at 3rd instruction
	bpAddr := uint64(PROG_START + 16)
	adapter.SetBreakpoint(bpAddr)

	// Resume CPU
	adapter.Resume()

	// Wait for monitor to auto-activate
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mon.IsActive() {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if !mon.IsActive() {
		t.Fatal("Monitor should auto-activate on breakpoint hit")
		adapter.Freeze()
	}

	// Check that BREAK message appears in output
	mon.mu.Lock()
	found := false
	for _, line := range mon.outputLines {
		if strings.Contains(line.Text, "BREAK") {
			found = true
			break
		}
	}
	mon.mu.Unlock()

	if !found {
		t.Error("Expected 'BREAK' message in monitor output")
	}
}

// ===========================================================================
// Phase 3: Freeze/Thaw Commands
// ===========================================================================

func TestFreezeThawCPU(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)

	cpu1 := NewCPU64(bus)
	cpu1.running.Store(false)
	cpu2 := NewCPU(bus)
	cpu2.running.Store(false)

	a1 := NewDebugIE64(cpu1)
	a2 := NewDebugIE32(cpu2)

	mon.RegisterCPU("IE64", a1)
	mon.RegisterCPU("IE32", a2)

	// Test freeze by label
	mon.mu.Lock()
	mon.ExecuteCommand("freeze ie64")
	mon.mu.Unlock()
	// IE64 is already not running, so no change expected
	if a1.IsRunning() {
		t.Error("IE64 should still not be running after freeze")
	}

	// Test thaw by ID
	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("thaw 0")
	out := mon.outputLines
	mon.mu.Unlock()

	// Should have output message
	found := false
	for _, line := range out {
		if strings.Contains(line.Text, "thaw") || strings.Contains(line.Text, "Thaw") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected thaw message in output")
	}
}

func TestFreezeThawAll(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)

	cpu1 := NewCPU64(bus)
	cpu1.running.Store(false)
	cpu2 := NewCPU(bus)
	cpu2.running.Store(false)

	a1 := NewDebugIE64(cpu1)
	a2 := NewDebugIE32(cpu2)

	mon.RegisterCPU("IE64", a1)
	mon.RegisterCPU("IE32", a2)

	// Freeze all
	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("freeze *")
	out := mon.outputLines
	mon.mu.Unlock()

	found := false
	for _, line := range out {
		if strings.Contains(line.Text, "All") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'All CPUs frozen' message")
	}

	// Thaw all
	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("thaw *")
	out = mon.outputLines
	mon.mu.Unlock()

	found = false
	for _, line := range out {
		if strings.Contains(line.Text, "All") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'All CPUs thawed' message")
	}
}

func TestOutputScrollback(t *testing.T) {
	mon, _ := newTestMonitor()

	// Add enough output to test scrolling
	mon.mu.Lock()
	for i := range 50 {
		mon.appendOutput(fmt.Sprintf("Line %d", i), colorWhite)
	}
	mon.mu.Unlock()

	if len(mon.outputLines) != 50 {
		t.Errorf("Expected 50 output lines, got %d", len(mon.outputLines))
	}

	// Verify scroll offset starts at 0
	if mon.scrollOffset != 0 {
		t.Errorf("Expected scrollOffset 0, got %d", mon.scrollOffset)
	}
}

// ===========================================================================
// Phase 4: Register Change Highlighting + Auto-disassemble
// ===========================================================================

func TestRegisterChangeHighlight(t *testing.T) {
	mon, cpu := newTestMonitor()

	// Write move.l r1, #42 at PC
	cpu.memory[PROG_START] = OP_MOVE
	cpu.memory[PROG_START+1] = (1<<3 | 2<<1 | 1) // rd=1, size=L, xbit=1
	cpu.memory[PROG_START+4] = 42

	// Save initial regs
	mon.mu.Lock()
	mon.saveCurrentRegs()
	mon.outputLines = nil
	mon.ExecuteCommand("s")
	lines := mon.outputLines
	mon.mu.Unlock()

	// Should have output showing R1 changed
	foundChange := false
	for _, line := range lines {
		if strings.Contains(line.Text, "R1") && line.Color == colorGreen {
			foundChange = true
			break
		}
	}
	if !foundChange {
		t.Error("Expected R1 change highlighted in green after step")
	}
}

func TestAutoDisassembleAfterStep(t *testing.T) {
	mon, cpu := newTestMonitor()

	// Write move.l r1, #42 at PC
	cpu.memory[PROG_START] = OP_MOVE
	cpu.memory[PROG_START+1] = (1<<3 | 2<<1 | 1)
	cpu.memory[PROG_START+4] = 42
	// Write NOP at next instruction
	cpu.memory[PROG_START+8] = OP_NOP64

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("s")
	lines := mon.outputLines
	mon.mu.Unlock()

	// Last output line should contain disassembly (nop or similar)
	foundDisasm := false
	for _, line := range lines {
		if strings.Contains(line.Text, fmt.Sprintf("%X", PROG_START+8)) {
			foundDisasm = true
			break
		}
	}
	if !foundDisasm {
		t.Error("Expected auto-disassembly of next instruction after step")
	}
}

func TestCycleCountDisplay(t *testing.T) {
	mon, cpu := newTestMonitor()

	// Write NOP
	cpu.memory[PROG_START] = OP_NOP64

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("s")
	lines := mon.outputLines
	mon.mu.Unlock()

	// Should show cycle count
	found := false
	for _, line := range lines {
		if strings.Contains(line.Text, "cycle") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected cycle count in step output")
	}
}

// ---------------------------------------------------------------------------
// Phase 4: Coprocessor Discovery
// ---------------------------------------------------------------------------

func TestCoprocessorDiscovery(t *testing.T) {
	bus := NewMachineBus()

	// Create a coprocessor manager
	coprocMgr := NewCoprocessorManager(bus, "/tmp")

	// Create an IE32 CPU at worker region and wrap in debug adapter
	cpu := NewCPU(bus)
	cpu.running.Store(false) // don't actually run
	cpu.PC = WORKER_IE32_BASE

	// Manually install a worker with debugCPU
	coprocMgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType:  EXEC_TYPE_IE32,
		stop:     func() {},
		done:     make(chan struct{}),
		loadBase: WORKER_IE32_BASE,
		loadEnd:  WORKER_IE32_END,
		debugCPU: NewDebugIE32(cpu),
	}

	// Verify GetActiveWorkers returns the worker
	workers := coprocMgr.GetActiveWorkers()
	if len(workers) != 1 {
		t.Fatalf("Expected 1 active worker, got %d", len(workers))
	}
	if workers[0].Label != "coproc:IE32" {
		t.Errorf("Expected label 'coproc:IE32', got %q", workers[0].Label)
	}
	if workers[0].CPUType != EXEC_TYPE_IE32 {
		t.Errorf("Expected cpuType %d, got %d", EXEC_TYPE_IE32, workers[0].CPUType)
	}

	// Create monitor with coprocMgr and verify cpu command lists coprocessor
	mon := NewMachineMonitor(bus)
	mon.coprocMgr = coprocMgr
	coprocMgr.monitor = mon

	// Register a primary CPU
	cpu64 := NewCPU64(bus)
	dbg := NewDebugIE64(cpu64)
	mon.RegisterCPU("IE64", dbg)

	// Register the worker with the monitor (as auto-registration would do)
	if workers[0].CPU != nil {
		mon.RegisterCPU(workers[0].Label, workers[0].CPU)
	}

	// Activate monitor
	mon.mu.Lock()
	mon.state = MonitorActive
	mon.mu.Unlock()

	// Run cpu command (no lock needed — ExecuteCommand takes lock internally in cmdCPU)
	mon.mu.Lock()
	mon.outputLines = nil
	mon.cmdCPU(MonitorCommand{Name: "cpu"})
	lines := make([]OutputLine, len(mon.outputLines))
	copy(lines, mon.outputLines)
	mon.mu.Unlock()

	// Should have at least 2 lines: one for IE64, one for coproc:IE32
	if len(lines) < 2 {
		t.Fatalf("Expected at least 2 output lines, got %d", len(lines))
	}

	foundPrimary := false
	foundCoproc := false
	for _, line := range lines {
		if strings.Contains(line.Text, "IE64") {
			foundPrimary = true
		}
		if strings.Contains(line.Text, "coproc:IE32") {
			foundCoproc = true
		}
	}
	if !foundPrimary {
		t.Error("Expected primary CPU IE64 in cpu list")
	}
	if !foundCoproc {
		t.Error("Expected coprocessor coproc:IE32 in cpu list")
	}
}

// ---------------------------------------------------------------------------
// Breakpoint: step to breakpoint
// ---------------------------------------------------------------------------

func TestBreakpointStepHit(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Write 5 NOPs starting at PROG_START (each 8 bytes)
	for i := range uint32(5) {
		cpu.memory[PROG_START+i*8] = OP_NOP64
	}

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)

	// Set breakpoint at 3rd instruction (offset 16)
	bpAddr := uint64(PROG_START + 16)
	adapter.SetBreakpoint(bpAddr)

	// Step through instructions manually — breakpoint should not prevent
	// stepping (breakpoints are checked by trapLoop, not Step).
	// Step 1: NOP at PROG_START
	adapter.Step()
	if cpu.PC != PROG_START+8 {
		t.Fatalf("After step 1, PC = %X, expected %X", cpu.PC, PROG_START+8)
	}

	// Step 2: NOP at PROG_START+8
	adapter.Step()
	if cpu.PC != PROG_START+16 {
		t.Fatalf("After step 2, PC = %X, expected %X", cpu.PC, PROG_START+16)
	}

	// Now PC is at the breakpoint address — verify the breakpoint exists
	if !adapter.HasBreakpoint(bpAddr) {
		t.Fatal("Expected breakpoint at target address")
	}

	// Step again — Step() doesn't check breakpoints, so it should execute
	adapter.Step()
	if cpu.PC != PROG_START+24 {
		t.Fatalf("After step 3, PC = %X, expected %X", cpu.PC, PROG_START+24)
	}
}

// ---------------------------------------------------------------------------
// Breakpoint: concurrent set/clear while CPU runs in trap mode
// ---------------------------------------------------------------------------

func TestBreakpointConcurrency(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Write a NOP loop: 10 NOPs + BRA back to start
	for i := range uint32(10) {
		cpu.memory[PROG_START+i*8] = OP_NOP64
	}
	cpu.memory[PROG_START+80] = OP_BRA
	offset := int32(-88)
	uoff := uint32(offset)
	cpu.memory[PROG_START+84] = byte(uoff)
	cpu.memory[PROG_START+85] = byte(uoff >> 8)
	cpu.memory[PROG_START+86] = byte(uoff >> 16)
	cpu.memory[PROG_START+87] = byte(uoff >> 24)

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)

	// Set an initial breakpoint far away so trap mode is active but won't fire
	adapter.SetBreakpoint(0xFFFFFF)

	// Resume CPU — enters trap mode because breakpoints exist
	adapter.Resume()

	// Concurrently set and clear breakpoints while CPU is running
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 100 {
			addr := uint64(0x80000 + i)
			adapter.SetBreakpoint(addr)
			adapter.ClearBreakpoint(addr)
		}
	}()

	// Wait for concurrent operations to complete
	select {
	case <-done:
		// Success — no data race or deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout: concurrent breakpoint operations deadlocked")
	}

	// Stop the CPU
	adapter.Freeze()

	// Verify the original breakpoint is still set
	if !adapter.HasBreakpoint(0xFFFFFF) {
		t.Error("Expected far-away breakpoint to still be set")
	}

	// Clean up
	adapter.ClearAllBreakpoints()
}

// ===========================================================================
// Regression: ResetCPUs stops trap-mode goroutines before CPU recreation
// ===========================================================================

func TestResetCPUsStopsTrapMode(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Write a NOP loop: 10 NOPs + BRA back
	for i := range uint32(10) {
		cpu.memory[PROG_START+i*8] = OP_NOP64
	}
	cpu.memory[PROG_START+80] = OP_BRA
	offset := int32(-88)
	uoff := uint32(offset)
	cpu.memory[PROG_START+84] = byte(uoff)
	cpu.memory[PROG_START+85] = byte(uoff >> 8)
	cpu.memory[PROG_START+86] = byte(uoff >> 16)
	cpu.memory[PROG_START+87] = byte(uoff >> 24)

	mon := NewMachineMonitor(bus)
	adapter := NewDebugIE64(cpu)
	mon.RegisterCPU("IE64", adapter)

	// Enter trap mode: set far breakpoint, resume
	adapter.SetBreakpoint(0xFFFFFF)
	adapter.Resume()

	// Wait for the trap loop to start
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if adapter.trapRunning.Load() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !adapter.trapRunning.Load() {
		t.Fatal("Adapter should be in trap mode")
	}

	// ResetCPUs must stop the trap goroutine before clearing the CPU map
	mon.ResetCPUs()

	if adapter.trapRunning.Load() {
		t.Error("trapRunning should be false after ResetCPUs")
	}
	if adapter.IsRunning() {
		t.Error("IsRunning() should be false after ResetCPUs")
	}
}

// ===========================================================================
// Regression: breakpoint auto-activation preserves frozen CPU state
// ===========================================================================

func TestBreakpointAutoActivationPreservesFrozenState(t *testing.T) {
	bus := NewMachineBus()

	// CPU 0: intentionally frozen (never started)
	cpu0 := NewCPU64(bus)
	cpu0.running.Store(false)

	// CPU 1: will run and hit breakpoint
	cpu1 := NewCPU64(bus)
	for i := range uint32(10) {
		cpu1.memory[PROG_START+i*8] = OP_NOP64
	}
	cpu1.memory[PROG_START+80] = OP_BRA
	offset := int32(-88)
	uoff := uint32(offset)
	cpu1.memory[PROG_START+84] = byte(uoff)
	cpu1.memory[PROG_START+85] = byte(uoff >> 8)
	cpu1.memory[PROG_START+86] = byte(uoff >> 16)
	cpu1.memory[PROG_START+87] = byte(uoff >> 24)

	mon := NewMachineMonitor(bus)
	adapter0 := NewDebugIE64(cpu0)
	adapter1 := NewDebugIE64(cpu1)
	mon.RegisterCPU("IE64-main", adapter0)
	mon.RegisterCPU("IE64-coproc", adapter1)
	mon.StartBreakpointListener()

	// Set breakpoint on CPU 1 at 3rd instruction
	bpAddr := uint64(PROG_START + 16)
	adapter1.SetBreakpoint(bpAddr)

	// Resume CPU 1 (enters trap mode, CPU 0 stays frozen)
	adapter1.Resume()

	// Wait for auto-activation
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mon.IsActive() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !mon.IsActive() {
		adapter1.Freeze()
		t.Fatal("Monitor should auto-activate on breakpoint hit")
	}

	// Clear breakpoints so Deactivate's Resume won't re-trigger immediately
	adapter1.ClearAllBreakpoints()

	// Deactivate — should only resume CPUs that were genuinely running
	mon.Deactivate()

	// CPU 0 was frozen before the breakpoint — it must NOT be resumed
	if adapter0.IsRunning() {
		t.Error("CPU 0 should remain frozen — it was not running before the breakpoint")
	}
}

// ===========================================================================
// Regression: Z80 trap loop clears CPU Running flag on all exit paths
// ===========================================================================

func TestZ80TrapLoopClearsRunning(t *testing.T) {
	bus := &testZ80Bus{}
	cpu := NewCPU_Z80(bus)

	bpChan := make(chan BreakpointEvent, 1)
	adapter := NewDebugZ80(cpu, nil)
	adapter.SetBreakpointChannel(bpChan, 0)

	// Sub-test 1: breakpoint hit must clear Running
	t.Run("breakpoint_exit", func(t *testing.T) {
		cpu.PC = 0
		// Memory is zero-initialized = all NOPs (0x00)
		adapter.SetBreakpoint(5)
		adapter.Resume()

		select {
		case <-bpChan:
		case <-time.After(2 * time.Second):
			adapter.Freeze()
			t.Fatal("Timeout waiting for breakpoint")
		}

		if cpu.Running() {
			t.Error("CPU.Running() should be false after breakpoint hit")
		}
		if adapter.trapRunning.Load() {
			t.Error("trapRunning should be false after breakpoint hit")
		}
		adapter.ClearAllBreakpoints()
	})

	// Sub-test 2: Freeze must clear Running
	t.Run("freeze_exit", func(t *testing.T) {
		cpu.PC = 0
		// JR -2: infinite loop at address 0 so breakpoint is never reached
		bus.mem[0] = 0x18 // JR
		bus.mem[1] = 0xFE // offset -2

		adapter.SetBreakpoint(0xFFFF)
		adapter.Resume()

		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if adapter.trapRunning.Load() {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if !adapter.trapRunning.Load() {
			t.Fatal("Trap loop should have started")
		}

		adapter.Freeze()

		if cpu.Running() {
			t.Error("CPU.Running() should be false after Freeze")
		}
		if adapter.trapRunning.Load() {
			t.Error("trapRunning should be false after Freeze")
		}
		adapter.ClearAllBreakpoints()
		bus.mem[0] = 0x00
		bus.mem[1] = 0x00
	})
}

// ===========================================================================
// Audio Freeze/Thaw
// ===========================================================================

func TestSoundChipFreezeBlocksOutput(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}
	chip.enabled.Store(true)

	// Generate a sample with audio enabled — should produce some output
	// (may be zero if no channels configured, but the path runs)
	_ = chip.ReadSample()

	// Freeze audio
	chip.audioFrozen.Store(true)

	// ReadSample must return exactly 0 when frozen
	sample := chip.ReadSample()
	if sample != 0 {
		t.Errorf("ReadSample while frozen = %f, want 0", sample)
	}
}

type mockTicker struct {
	ticked bool
}

func (m *mockTicker) TickSample() { m.ticked = true }

func TestSoundChipFreezeSkipsTicker(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}

	ticker := &mockTicker{}
	chip.SetSampleTicker(ticker)
	chip.audioFrozen.Store(true)

	_ = chip.ReadSample()

	if ticker.ticked {
		t.Error("Ticker should NOT be called when audio is frozen")
	}
}

func TestSoundChipThawRestoresOutput(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}

	ticker := &mockTicker{}
	chip.SetSampleTicker(ticker)

	// Freeze then thaw
	chip.audioFrozen.Store(true)
	_ = chip.ReadSample()
	if ticker.ticked {
		t.Fatal("Ticker should not tick while frozen")
	}

	chip.audioFrozen.Store(false)
	_ = chip.ReadSample()
	if !ticker.ticked {
		t.Error("Ticker should tick after thaw")
	}
}

func TestSoundChipResetClearsFrozen(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}

	chip.audioFrozen.Store(true)
	chip.Reset()

	if chip.audioFrozen.Load() {
		t.Error("audioFrozen should be false after Reset()")
	}
}

func TestMonitorFaCommand(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}

	mon, _ := newTestMonitor()
	mon.soundChip = chip

	mon.mu.Lock()
	mon.ExecuteCommand("fa")
	mon.mu.Unlock()

	if !chip.audioFrozen.Load() {
		t.Error("audioFrozen should be true after 'fa' command")
	}
}

func TestMonitorTaCommand(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}

	mon, _ := newTestMonitor()
	mon.soundChip = chip
	chip.audioFrozen.Store(true)

	mon.mu.Lock()
	mon.ExecuteCommand("ta")
	mon.mu.Unlock()

	if chip.audioFrozen.Load() {
		t.Error("audioFrozen should be false after 'ta' command")
	}
}

// ===========================================================================
// Worker Control Model (Step 2)
// ===========================================================================

func TestCoprocWorkerPauseUnpause(t *testing.T) {
	bus := NewMachineBus()
	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := createIE32Worker(bus, code)
	if err != nil {
		t.Fatalf("createIE32Worker: %v", err)
	}

	// Give it a moment to start running
	time.Sleep(10 * time.Millisecond)

	// Pause
	worker.Pause()
	worker.mu.Lock()
	frozen := worker.frozen
	worker.mu.Unlock()
	if !frozen {
		t.Error("Worker should be frozen after Pause()")
	}

	// Unpause
	worker.Unpause()
	worker.mu.Lock()
	frozen = worker.frozen
	worker.mu.Unlock()
	if frozen {
		t.Error("Worker should not be frozen after Unpause()")
	}

	// Give it time to start running again
	time.Sleep(10 * time.Millisecond)
	select {
	case <-worker.done:
		t.Fatal("Worker should be running after Unpause()")
	default:
	}

	// Clean up
	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop in time")
	}
}

func TestCoprocWorkerPauseTimeout(t *testing.T) {
	// Create a worker with stopCPU that does nothing (simulating stuck CPU)
	done := make(chan struct{})
	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_IE32,
		monitorID: -1,
		stopCPU:   func() {},            // no-op — won't actually stop
		execCPU:   func() { select {} }, // blocks forever
		done:      done,
		stop:      func() {},
	}

	// Launch the "stuck" goroutine
	go func() {
		defer close(done)
		worker.execCPU()
	}()

	// Pause should return within ~2s timeout without hanging
	start := time.Now()
	worker.Pause()
	elapsed := time.Since(start)

	if elapsed > 4*time.Second {
		t.Fatalf("Pause() took too long: %v", elapsed)
	}

	// frozen should be false since it timed out
	worker.mu.Lock()
	frozen := worker.frozen
	worker.mu.Unlock()
	if frozen {
		t.Error("frozen should be false on timeout (goroutine still alive)")
	}
}

func TestCoprocWorkerFreezeViaAdapterZ80(t *testing.T) {
	bus := NewMachineBus()
	code := []byte{0x18, 0xFE} // JR -2 (infinite loop)
	worker, err := createZ80Worker(bus, code)
	if err != nil {
		t.Fatalf("createZ80Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Freeze via the debug adapter — should not panic
	worker.debugCPU.Freeze()

	// Worker should be frozen
	worker.mu.Lock()
	frozen := worker.frozen
	worker.mu.Unlock()
	if !frozen {
		t.Error("Worker should be frozen after adapter.Freeze()")
	}
}

func TestCoprocWorkerResumeViaAdapterZ80(t *testing.T) {
	bus := NewMachineBus()
	code := []byte{0x18, 0xFE} // JR -2 (infinite loop)
	worker, err := createZ80Worker(bus, code)
	if err != nil {
		t.Fatalf("createZ80Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Freeze
	worker.debugCPU.Freeze()
	worker.mu.Lock()
	frozen := worker.frozen
	worker.mu.Unlock()
	if !frozen {
		t.Fatal("Worker should be frozen")
	}

	// Resume via adapter
	worker.debugCPU.Resume()
	time.Sleep(10 * time.Millisecond)

	// Should be running again
	worker.mu.Lock()
	frozen = worker.frozen
	worker.mu.Unlock()
	if frozen {
		t.Error("Worker should not be frozen after adapter.Resume()")
	}

	// Clean up
	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop")
	}
}

func TestCoprocWorkerFreezeResume6502(t *testing.T) {
	bus := NewMachineBus()
	code := []byte{0x4C, 0x00, 0x00} // JMP $0000
	worker, err := create6502Worker(bus, code)
	if err != nil {
		t.Fatalf("create6502Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	worker.debugCPU.Freeze()
	worker.mu.Lock()
	if !worker.frozen {
		worker.mu.Unlock()
		t.Fatal("Worker should be frozen")
	}
	worker.mu.Unlock()

	worker.debugCPU.Resume()
	time.Sleep(10 * time.Millisecond)

	worker.mu.Lock()
	if worker.frozen {
		worker.mu.Unlock()
		t.Error("Worker should not be frozen after Resume()")
	}
	worker.mu.Unlock()

	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop")
	}
}

func TestCoprocWorkerFreezeResumeM68K(t *testing.T) {
	bus := NewMachineBus()
	// M68K BRA.S -2: store as 0xFE60 (byte-swapped)
	code := []byte{0xFE, 0x60}
	worker, err := createM68KWorker(bus, code)
	if err != nil {
		t.Fatalf("createM68KWorker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	worker.debugCPU.Freeze()
	worker.mu.Lock()
	if !worker.frozen {
		worker.mu.Unlock()
		t.Fatal("Worker should be frozen")
	}
	worker.mu.Unlock()

	worker.debugCPU.Resume()
	time.Sleep(10 * time.Millisecond)

	worker.mu.Lock()
	if worker.frozen {
		worker.mu.Unlock()
		t.Error("Worker should not be frozen after Resume()")
	}
	worker.mu.Unlock()

	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop")
	}
}

func TestCoprocWorkerFreezeResumeX86(t *testing.T) {
	bus := NewMachineBus()
	code := []byte{0xEB, 0xFE} // JMP short -2
	worker, err := createX86Worker(bus, code)
	if err != nil {
		t.Fatalf("createX86Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	worker.debugCPU.Freeze()
	worker.mu.Lock()
	if !worker.frozen {
		worker.mu.Unlock()
		t.Fatal("Worker should be frozen")
	}
	worker.mu.Unlock()

	worker.debugCPU.Resume()
	time.Sleep(10 * time.Millisecond)

	worker.mu.Lock()
	if worker.frozen {
		worker.mu.Unlock()
		t.Error("Worker should not be frozen after Resume()")
	}
	worker.mu.Unlock()

	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop")
	}
}

func TestCoprocWorkerFreezeResumeIE32(t *testing.T) {
	bus := NewMachineBus()
	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := createIE32Worker(bus, code)
	if err != nil {
		t.Fatalf("createIE32Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	worker.debugCPU.Freeze()
	worker.mu.Lock()
	if !worker.frozen {
		worker.mu.Unlock()
		t.Fatal("Worker should be frozen")
	}
	worker.mu.Unlock()

	worker.debugCPU.Resume()
	time.Sleep(10 * time.Millisecond)

	worker.mu.Lock()
	if worker.frozen {
		worker.mu.Unlock()
		t.Error("Worker should not be frozen after Resume()")
	}
	worker.mu.Unlock()

	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop")
	}
}

func TestCoprocWorkerMonitorIDInitNegOne(t *testing.T) {
	bus := NewMachineBus()
	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := createIE32Worker(bus, code)
	if err != nil {
		t.Fatalf("createIE32Worker: %v", err)
	}
	defer func() {
		worker.stop()
		<-worker.done
	}()

	if worker.monitorID != -1 {
		t.Errorf("monitorID = %d, want -1", worker.monitorID)
	}
}

// ===========================================================================
// Auto-Registration (Step 3)
// ===========================================================================

func TestCoprocWorkerRegistersOnStart(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
	if err != nil {
		t.Fatalf("createWorkerAndRegister: %v", err)
	}
	defer func() {
		worker.stopCPU()
		<-worker.done
	}()

	if worker.monitorID < 0 {
		t.Fatal("Worker should have a non-negative monitorID after registration")
	}

	// Verify the monitor has an entry with the right label
	mon.mu.Lock()
	entry, ok := mon.cpus[worker.monitorID]
	mon.mu.Unlock()
	if !ok {
		t.Fatal("Monitor should have an entry for the worker's monitorID")
	}
	if entry.Label != "coproc:IE32" {
		t.Errorf("Label = %q, want %q", entry.Label, "coproc:IE32")
	}
}

func TestCoprocWorkerUnregistersOnStop(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
	if err != nil {
		t.Fatalf("createWorkerAndRegister: %v", err)
	}
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = worker
	mgr.mu.Unlock()

	monID := worker.monitorID
	if monID < 0 {
		t.Fatal("Worker should be registered")
	}

	// Stop via stopWorkerAndUnregister
	mgr.stopWorkerAndUnregister(EXEC_TYPE_IE32, worker)

	// Verify monitor entry removed
	mon.mu.Lock()
	_, ok := mon.cpus[monID]
	mon.mu.Unlock()
	if ok {
		t.Error("Monitor entry should be removed after stop")
	}
}

func TestCoprocWorkerReplaceUnregisters(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	w1, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
	if err != nil {
		t.Fatalf("first createWorkerAndRegister: %v", err)
	}
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = w1
	mgr.mu.Unlock()
	oldID := w1.monitorID

	// Create a second worker of the same type — should unregister the old one
	w2, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
	if err != nil {
		t.Fatalf("second createWorkerAndRegister: %v", err)
	}

	// Stop old worker
	mgr.stopWorkerAndUnregister(EXEC_TYPE_IE32, w1)
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = w2
	mgr.mu.Unlock()

	// Old monitor entry should be gone
	mon.mu.Lock()
	_, oldOK := mon.cpus[oldID]
	_, newOK := mon.cpus[w2.monitorID]
	mon.mu.Unlock()
	if oldOK {
		t.Error("Old monitor entry should be removed")
	}
	if !newOK {
		t.Error("New monitor entry should exist")
	}

	// Clean up
	w2.stopCPU()
	<-w2.done
}

func TestCoprocMonitorIDInitNegOne2(t *testing.T) {
	bus := NewMachineBus()
	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := createIE32Worker(bus, code)
	if err != nil {
		t.Fatalf("createIE32Worker: %v", err)
	}
	defer func() {
		worker.stop()
		<-worker.done
	}()
	if worker.monitorID != -1 {
		t.Errorf("monitorID = %d, want -1", worker.monitorID)
	}
}

func TestCoprocUnregisterGuardNegOne(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := createIE32Worker(bus, code)
	if err != nil {
		t.Fatalf("createIE32Worker: %v", err)
	}
	// monitorID is -1 (not registered)
	// stopWorkerAndUnregister should not panic
	mgr.stopWorkerAndUnregister(EXEC_TYPE_IE32, worker)
}

func TestCoprocStopAllUnregisters(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	w1, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
	if err != nil {
		t.Fatalf("createWorkerAndRegister: %v", err)
	}
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = w1
	mgr.mu.Unlock()

	monID := w1.monitorID
	if monID < 0 {
		t.Fatal("Worker should be registered")
	}

	mgr.StopAll()

	// Verify monitor entry removed
	mon.mu.Lock()
	_, ok := mon.cpus[monID]
	mon.mu.Unlock()
	if ok {
		t.Error("Monitor entry should be removed after StopAll")
	}
}

func TestCoprocPauseTimeoutNoFrozen(t *testing.T) {
	// Worker with no-op stopCPU (simulates stuck CPU)
	done := make(chan struct{})
	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_IE32,
		monitorID: -1,
		stopCPU:   func() {},
		execCPU:   func() { select {} },
		done:      done,
		stop:      func() {},
	}
	go func() {
		defer close(done)
		worker.execCPU()
	}()

	worker.Pause() // times out after 2s

	worker.mu.Lock()
	frozen := worker.frozen
	worker.mu.Unlock()
	if frozen {
		t.Error("frozen should be false after Pause timeout")
	}

	// Subsequent Unpause should be no-op (no duplicate goroutine)
	worker.Unpause()
	worker.mu.Lock()
	frozen = worker.frozen
	worker.mu.Unlock()
	if frozen {
		t.Error("frozen should still be false after Unpause on non-frozen worker")
	}
}

// ===========================================================================
// TrapLoop + Worker Interaction (Step 2 remaining)
// ===========================================================================

func TestCoprocWorkerTrapLoopWhileFrozen(t *testing.T) {
	// Create a Z80 worker with a tight loop (JR -2 = 0x18 0xFE)
	bus := NewMachineBus()
	code := []byte{0x18, 0xFE} // JR -2
	worker, err := createZ80Worker(bus, code)
	if err != nil {
		t.Fatalf("createZ80Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Freeze the worker via the adapter
	worker.debugCPU.Freeze()
	worker.mu.Lock()
	if !worker.frozen {
		worker.mu.Unlock()
		t.Fatal("Worker should be frozen after Freeze()")
	}
	worker.mu.Unlock()

	// Set a breakpoint at address 0 (the loop address)
	worker.debugCPU.SetBreakpoint(0)

	// Resume — should launch trapLoop, NOT the worker goroutine.
	// Worker stays frozen because trapLoop drives execution directly.
	worker.debugCPU.Resume()

	// Give trapLoop time to hit the breakpoint
	time.Sleep(50 * time.Millisecond)

	// Worker should still be frozen (trapLoop manages CPU, not the worker goroutine)
	worker.mu.Lock()
	frozen := worker.frozen
	worker.mu.Unlock()
	if !frozen {
		t.Error("Worker should remain frozen=true while trapLoop is driving execution")
	}

	// trapLoop should have exited after hitting the breakpoint
	time.Sleep(50 * time.Millisecond)
	if worker.debugCPU.IsRunning() {
		t.Error("CPU should not be running after breakpoint hit in trapLoop")
	}

	// Clean up: clear breakpoints, resume without breakpoints (uses worker goroutine)
	worker.debugCPU.ClearAllBreakpoints()
	worker.debugCPU.Resume()
	time.Sleep(10 * time.Millisecond)

	worker.mu.Lock()
	frozen = worker.frozen
	worker.mu.Unlock()
	if frozen {
		t.Error("Worker should be unfrozen after Resume() without breakpoints")
	}

	// Final cleanup
	worker.debugCPU.Freeze()
	worker.stopCPU()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop in time")
	}
}

func TestCoprocWorkerStopDuringTrapLoop(t *testing.T) {
	// Create a Z80 worker with a tight loop
	bus := NewMachineBus()
	code := []byte{0x18, 0xFE} // JR -2
	worker, err := createZ80Worker(bus, code)
	if err != nil {
		t.Fatalf("createZ80Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Freeze the worker
	worker.debugCPU.Freeze()

	// Set breakpoint at an address the CPU will never reach (far away)
	worker.debugCPU.SetBreakpoint(0x1000)

	// Resume with breakpoints — trapLoop drives execution
	worker.debugCPU.Resume()

	// Give trapLoop time to start stepping
	time.Sleep(20 * time.Millisecond)

	// Verify trapLoop is running
	if !worker.debugCPU.IsRunning() {
		t.Fatal("CPU should be running via trapLoop")
	}

	// Freeze the adapter — this should close trapStop and wait for trapLoop to exit
	worker.debugCPU.Freeze()

	// After Freeze, trapLoop should have exited
	if worker.debugCPU.IsRunning() {
		t.Error("CPU should not be running after Freeze() during trapLoop")
	}

	// Worker is still frozen — clean up by stopping CPU
	worker.stopCPU()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop")
	}
}

// ===========================================================================
// Auto-Registration Remaining Tests (Step 3 remaining)
// ===========================================================================

func TestCoprocMonitorIDWriteUnderMu(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
	if err != nil {
		t.Fatalf("createWorkerAndRegister: %v", err)
	}
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = worker
	mgr.mu.Unlock()
	defer func() {
		mgr.StopAll()
	}()

	// Verify monitorID is assigned (not -1) and visible via GetActiveWorkers
	workers := mgr.GetActiveWorkers()
	if len(workers) == 0 {
		t.Fatal("Expected at least 1 active worker")
	}
	found := false
	for _, w := range workers {
		if w.CPUType == EXEC_TYPE_IE32 {
			found = true
		}
	}
	if !found {
		t.Error("Expected IE32 worker in active workers list")
	}

	if worker.monitorID < 0 {
		t.Error("monitorID should have been assigned a non-negative value")
	}
}

func TestCoprocRegistrationRace(t *testing.T) {
	// This test verifies that if a worker is replaced during the unlock window
	// in cmdStart, the stale monitor entry gets cleaned up properly.
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))

	// Create first worker and register
	w1, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
	if err != nil {
		t.Fatalf("first createWorkerAndRegister: %v", err)
	}
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = w1
	mgr.mu.Unlock()

	w1ID := w1.monitorID
	if w1ID < 0 {
		t.Fatal("First worker should be registered")
	}

	// Create second worker and register (replacing first)
	w2, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
	if err != nil {
		t.Fatalf("second createWorkerAndRegister: %v", err)
	}

	// Stop and unregister old worker
	mgr.stopWorkerAndUnregister(EXEC_TYPE_IE32, w1)
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = w2
	mgr.mu.Unlock()

	// Old monitor entry should be cleaned up
	mon.mu.Lock()
	_, oldExists := mon.cpus[w1ID]
	_, newExists := mon.cpus[w2.monitorID]
	mon.mu.Unlock()

	if oldExists {
		t.Error("Stale monitor entry for first worker should be removed")
	}
	if !newExists {
		t.Error("New monitor entry for second worker should exist")
	}

	// Clean up
	mgr.StopAll()
}

func TestCoprocNoDeadlock(t *testing.T) {
	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	// Register a primary CPU
	cpu64 := NewCPU64(bus)
	dbg := NewDebugIE64(cpu64)
	mon.RegisterCPU("IE64", dbg)

	mon.mu.Lock()
	mon.state = MonitorActive
	mon.mu.Unlock()

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Loop: start and stop workers rapidly
		for range 10 {
			w, err := mgr.createWorkerAndRegister(EXEC_TYPE_IE32, code)
			if err != nil {
				continue
			}
			mgr.mu.Lock()
			mgr.workers[EXEC_TYPE_IE32] = w
			mgr.mu.Unlock()

			// Small delay to let things settle
			time.Sleep(time.Millisecond)

			mgr.stopWorkerAndUnregister(EXEC_TYPE_IE32, w)
			mgr.mu.Lock()
			mgr.workers[EXEC_TYPE_IE32] = nil
			mgr.mu.Unlock()
		}
	}()

	// Concurrently run cpu command to list CPUs
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		for range 20 {
			mon.mu.Lock()
			mon.outputLines = nil
			mon.cmdCPU(MonitorCommand{Name: "cpu"})
			mon.mu.Unlock()
			time.Sleep(500 * time.Microsecond)
		}
	}()

	// Both goroutines must finish within timeout — no deadlock
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected: start/stop loop didn't finish")
	}
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("Deadlock detected: cpu command loop didn't finish")
	}
}

func TestCoprocStopAllTimeout(t *testing.T) {
	// Create a worker with a stuck CPU (ignores stop signal)
	done := make(chan struct{})
	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_IE32,
		monitorID: -1,
		stopCPU:   func() {},            // no-op — won't actually stop
		execCPU:   func() { select {} }, // blocks forever
		done:      done,
		stop:      func() {},
	}
	go func() {
		defer close(done)
		worker.execCPU()
	}()

	bus := NewMachineBus()
	mon := NewMachineMonitor(bus)
	mgr := NewCoprocessorManager(bus, ".")
	mgr.monitor = mon

	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = worker
	mgr.mu.Unlock()

	// StopAll should return within reasonable time (2s timeout per worker + margin)
	start := time.Now()
	mgr.StopAll()
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("StopAll took too long: %v (expected < 5s)", elapsed)
	}
}

// ===========================================================================
// X86 EFLAGS Register Naming (Step 4)
// ===========================================================================

func TestX86RegisterEFLAGS(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.Flags = 0x202
	dbg := NewDebugX86(cpu, nil)

	val, ok := dbg.GetRegister("EFLAGS")
	if !ok {
		t.Fatal("GetRegister('EFLAGS') should return ok=true")
	}
	if val != 0x202 {
		t.Errorf("EFLAGS = 0x%X, want 0x202", val)
	}
}

func TestX86RegisterFLAGSCompat(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.Flags = 0x246
	dbg := NewDebugX86(cpu, nil)

	val, ok := dbg.GetRegister("FLAGS")
	if !ok {
		t.Fatal("GetRegister('FLAGS') should still work for backward compatibility")
	}
	if val != 0x246 {
		t.Errorf("FLAGS = 0x%X, want 0x246", val)
	}
}

func TestX86SetRegisterEFLAGS(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	dbg := NewDebugX86(cpu, nil)

	ok := dbg.SetRegister("EFLAGS", 0x42)
	if !ok {
		t.Fatal("SetRegister('EFLAGS') should return true")
	}
	if cpu.Flags != 0x42 {
		t.Errorf("Flags = 0x%X, want 0x42", cpu.Flags)
	}
}

func TestX86SetRegisterFLAGSCompat(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	dbg := NewDebugX86(cpu, nil)

	ok := dbg.SetRegister("FLAGS", 0x99)
	if !ok {
		t.Fatal("SetRegister('FLAGS') should return true")
	}
	if cpu.Flags != 0x99 {
		t.Errorf("Flags = 0x%X, want 0x99", cpu.Flags)
	}
}

func TestX86RegisterDisplayName(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	dbg := NewDebugX86(cpu, nil)

	regs := dbg.GetRegisters()
	for _, r := range regs {
		if r.Group == "flags" {
			if r.Name != "EFLAGS" {
				t.Errorf("Display name = %q, want %q", r.Name, "EFLAGS")
			}
			return
		}
	}
	t.Fatal("No register with group 'flags' found")
}

func TestMonitorFaWithoutSoundChip(t *testing.T) {
	mon, _ := newTestMonitor()
	// No soundChip wired

	mon.mu.Lock()
	mon.outputLines = nil
	mon.ExecuteCommand("fa")
	lines := mon.outputLines
	mon.mu.Unlock()

	found := false
	for _, line := range lines {
		if strings.Contains(line.Text, "No sound chip") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error message when no soundChip wired")
	}
}
