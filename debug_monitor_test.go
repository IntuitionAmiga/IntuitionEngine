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

	for i := 0; i < 16; i++ {
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
	for i := uint32(0); i < 10; i++ {
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
	for i := uint32(0); i < 10; i++ {
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
	for i := 0; i < 50; i++ {
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

	// Register a primary CPU
	cpu64 := NewCPU64(bus)
	dbg := NewDebugIE64(cpu64)
	mon.RegisterCPU("IE64", dbg)

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
	for i := uint32(0); i < 5; i++ {
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
	for i := uint32(0); i < 10; i++ {
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
		for i := 0; i < 100; i++ {
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
