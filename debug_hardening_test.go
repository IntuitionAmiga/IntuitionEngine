package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func TestParseCount_DecimalDefault(t *testing.T) {
	got, err := parseCount("10")
	if err != nil || got != 10 {
		t.Fatalf("parseCount decimal = %d, %v; want 10, nil", got, err)
	}
	got, err = parseCount("$10")
	if err != nil || got != 0x10 {
		t.Fatalf("parseCount hex = %d, %v; want 16, nil", got, err)
	}
}

func TestParseCommand_QuotedArgs(t *testing.T) {
	cmd := ParseCommand(`macro name "b $1000 if R1==$2"`)
	if cmd.Name != "macro" || len(cmd.Args) != 2 || cmd.Args[1] != "b $1000 if R1==$2" {
		t.Fatalf("ParseCommand quoted = %#v", cmd)
	}
}

func TestEvalAddress_LeadingMinusUnderflow(t *testing.T) {
	if _, ok := EvalAddress("-1", nil); ok {
		t.Fatal("EvalAddress(-1) succeeded; want underflow failure")
	}
}

func TestParseCondition_WidthSuffix_W_L(t *testing.T) {
	cond, err := ParseCondition("[$1000].W==$1234")
	if err != nil {
		t.Fatal(err)
	}
	if cond.Source != CondSourceMemory || cond.Width != 2 || FormatCondition(cond) != "[$1000].W==$1234" {
		t.Fatalf("word condition = %#v formatted %q", cond, FormatCondition(cond))
	}
	cond, err = ParseCondition("[$1000].L==$12345678")
	if err != nil {
		t.Fatal(err)
	}
	if cond.Width != 4 || FormatCondition(cond) != "[$1000].L==$12345678" {
		t.Fatalf("long condition = %#v formatted %q", cond, FormatCondition(cond))
	}
}

func TestCondition_MemDeref_UsesTargetByteOrder(t *testing.T) {
	bus := NewMachineBus()

	ie32 := NewDebugIE32(NewCPU(bus))
	ie32.WriteMemory(0x1000, []byte{0x34, 0x12, 0x78, 0x56})
	cond, err := ParseCondition("[$1000].W==$1234")
	if err != nil {
		t.Fatal(err)
	}
	if !evaluateCondition(cond, ie32) {
		t.Fatal("IE32 .W memory condition did not use little-endian byte order")
	}
	cond, err = ParseCondition("[$1000].L==$56781234")
	if err != nil {
		t.Fatal(err)
	}
	if !evaluateCondition(cond, ie32) {
		t.Fatal("IE32 .L memory condition did not use little-endian byte order")
	}

	m68k := NewDebugM68K(NewM68KCPU(bus), nil)
	m68k.WriteMemory(0x2000, []byte{0x12, 0x34, 0x56, 0x78})
	cond, err = ParseCondition("[$2000].W==$1234")
	if err != nil {
		t.Fatal(err)
	}
	if !evaluateCondition(cond, m68k) {
		t.Fatal("M68K .W memory condition did not use big-endian byte order")
	}
	cond, err = ParseCondition("[$2000].L==$12345678")
	if err != nil {
		t.Fatal(err)
	}
	if !evaluateCondition(cond, m68k) {
		t.Fatal("M68K .L memory condition did not use big-endian byte order")
	}
}

func TestRestoreSnapshot_RefusesMismatchedCPUType(t *testing.T) {
	bus := NewMachineBus()
	ie64 := NewDebugIE64(NewCPU64(bus))
	snap := TakeSnapshot(ie64)
	m68k := NewDebugM68K(NewM68KCPU(bus), nil)
	if err := RestoreSnapshot(m68k, snap); err == nil {
		t.Fatal("RestoreSnapshot accepted mismatched CPU type")
	}
}

func TestSnapshot_M68K_RoundTripsSSPAndUSP(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	dbg := NewDebugM68K(cpu, nil)
	cpu.USP = 0x12345678
	cpu.SSP = 0x87654321
	snap := TakeSnapshot(dbg)
	cpu.USP = 0
	cpu.SSP = 0
	if err := RestoreSnapshot(dbg, snap); err != nil {
		t.Fatal(err)
	}
	if cpu.USP != 0x12345678 || cpu.SSP != 0x87654321 {
		t.Fatalf("USP/SSP = %08X/%08X", cpu.USP, cpu.SSP)
	}
}

func TestMemoryDump_AddressColumn_IE64Full64(t *testing.T) {
	mon, _ := newTestMonitor()
	mon.ExecuteCommand("m $100000000 1")
	if len(mon.outputLines) == 0 {
		t.Fatal("no output")
	}
	got := mon.outputLines[len(mon.outputLines)-1].Text
	if !strings.HasPrefix(got, "0000000100000000:") {
		t.Fatalf("dump line %q does not show 64-bit address width", got)
	}
}

func TestHunt_RangeShorterThanPattern_NoOp(t *testing.T) {
	mon, _ := newTestMonitor()
	mon.ExecuteCommand("h $1000 $1000 01 02")
	if len(mon.outputLines) == 0 || mon.outputLines[len(mon.outputLines)-1].Text != "Not found" {
		t.Fatalf("hunt output = %#v", mon.outputLines)
	}
}

func TestDebuggableCPU_SnapshotBreakpoint_IE64ValueCopy(t *testing.T) {
	dbg := NewDebugIE64(NewCPU64(NewMachineBus()))
	cond := &BreakpointCondition{Source: CondSourceRegister, RegName: "R1", Op: CondOpEqual, Value: 1}
	dbg.SetConditionalBreakpoint(0x1000, cond)
	snap, ok := dbg.SnapshotBreakpoint(0x1000)
	if !ok || snap.Condition == nil {
		t.Fatalf("snapshot = %#v, %v", snap, ok)
	}
	snap.Condition.Value = 2
	live, _ := dbg.SnapshotBreakpoint(0x1000)
	if live.Condition.Value != 1 {
		t.Fatalf("snapshot mutation changed live condition to %d", live.Condition.Value)
	}
}

func TestIOView_ListIncludesSN76489SysInfoAndVoodooDepth(t *testing.T) {
	names := map[string]bool{}
	for _, name := range listIODevices() {
		names[name] = true
	}
	for _, want := range []string{"sn76489", "sysinfo", "voodoo_depth"} {
		if !names[want] {
			t.Fatalf("listIODevices missing %s: %v", want, listIODevices())
		}
	}
}

func TestBacktraceM68K_FollowsA6LinkChain(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	dbg := NewDebugM68K(cpu, nil)
	cpu.AddrRegs[6] = 0x100
	frame0 := make([]byte, 8)
	binary.BigEndian.PutUint32(frame0[0:4], 0x120)
	binary.BigEndian.PutUint32(frame0[4:8], 0x2000)
	dbg.WriteMemory(0x100, frame0)
	frame1 := make([]byte, 8)
	binary.BigEndian.PutUint32(frame1[0:4], 0)
	binary.BigEndian.PutUint32(frame1[4:8], 0x3000)
	dbg.WriteMemory(0x120, frame1)

	got := backtraceM68K(dbg, 8)
	if len(got) != 2 || got[0] != 0x2000 || got[1] != 0x3000 {
		t.Fatalf("backtraceM68K = %#v", got)
	}
}

func TestDisasm6502_65C02Opcodes(t *testing.T) {
	cases := []struct {
		bytes []byte
		want  string
	}{
		{[]byte{0x80, 0x02}, "BRA"},
		{[]byte{0x64, 0x10}, "STZ $10"},
		{[]byte{0x14, 0x10}, "TRB $10"},
		{[]byte{0x04, 0x10}, "TSB $10"},
		{[]byte{0xDA}, "PHX"},
		{[]byte{0x5A}, "PHY"},
		{[]byte{0xFA}, "PLX"},
		{[]byte{0x7A}, "PLY"},
		{[]byte{0x89, 0x7F}, "BIT #$7F"},
		{[]byte{0x07, 0x20}, "RMB0 $20"},
		{[]byte{0x87, 0x20}, "SMB0 $20"},
		{[]byte{0x0F, 0x20, 0x02}, "BBR0 $20"},
		{[]byte{0x8F, 0x20, 0x02}, "BBS0 $20"},
		{[]byte{0x7C, 0x34, 0x12}, "JMP ($1234,X)"},
	}
	for _, tc := range cases {
		mem := append([]byte{}, tc.bytes...)
		lines := disassemble6502(func(addr uint64, size int) []byte {
			if int(addr) >= len(mem) {
				return nil
			}
			end := min(int(addr)+size, len(mem))
			return mem[int(addr):end]
		}, 0, 1)
		if len(lines) != 1 || !strings.Contains(lines[0].Mnemonic, tc.want) {
			t.Fatalf("% X disassembled as %#v, want %q", tc.bytes, lines, tc.want)
		}
	}
}

func TestDisasm6502_JMPAbsIndexedIndirectHasUnknownTarget(t *testing.T) {
	mem := []byte{0x7C, 0x34, 0x12}
	lines := disassemble6502(func(addr uint64, size int) []byte {
		if int(addr) >= len(mem) {
			return nil
		}
		end := min(int(addr)+size, len(mem))
		return mem[int(addr):end]
	}, 0, 1)
	if len(lines) != 1 {
		t.Fatalf("got lines %#v", lines)
	}
	if !lines[0].IsBranch {
		t.Fatal("JMP ($addr,X) should be marked as a branch")
	}
	if lines[0].BranchTarget != 0 {
		t.Fatalf("BranchTarget = $%04X, want unknown target 0", lines[0].BranchTarget)
	}
}

func TestDisasm6502_JMPIndirectHasUnknownTarget(t *testing.T) {
	mem := []byte{0x6C, 0x34, 0x12}
	lines := disassemble6502(func(addr uint64, size int) []byte {
		if int(addr) >= len(mem) {
			return nil
		}
		end := min(int(addr)+size, len(mem))
		return mem[int(addr):end]
	}, 0, 1)
	if len(lines) != 1 {
		t.Fatalf("got lines %#v", lines)
	}
	if !lines[0].IsBranch {
		t.Fatal("JMP ($addr) should be marked as a branch")
	}
	if lines[0].BranchTarget != 0 {
		t.Fatalf("BranchTarget = $%04X, want unknown target 0", lines[0].BranchTarget)
	}
}

func TestMacro_QuotedSemicolon(t *testing.T) {
	cmds, err := splitSemicolonAware(`r PC "$1000;literal"; s 2`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 || cmds[0] != `r PC "$1000;literal"` || cmds[1] != "s 2" {
		t.Fatalf("splitSemicolonAware = %#v", cmds)
	}
}

func TestScript_CRLFLineEndings(t *testing.T) {
	lines := splitScriptLines("s 1\r\ns 2\rs 3\n")
	if len(lines) < 3 || lines[0] != "s 1" || lines[1] != "s 2" || lines[2] != "s 3" {
		t.Fatalf("splitScriptLines = %#v", lines)
	}
}

func TestDisasmM68K_68020_BFEXTU_DIVSL_LINKL_FullExtension(t *testing.T) {
	mem := []byte{
		0x48, 0x08, 0x00, 0x00, 0x12, 0x34, // LINK.L A0,#$00001234
		0xE9, 0xC0, 0x10, 0x08, // BFEXTU D0 {0:8},D1
		0x4C, 0x40, 0x10, 0x00, // DIVSL D0,D1:D0
		0xE9, 0xC0, 0x18, 0xA5, // BFEXTU D0 {D2:D5},D1
	}
	lines := disassembleM68K(func(addr uint64, size int) []byte {
		if int(addr) >= len(mem) {
			return nil
		}
		end := min(int(addr)+size, len(mem))
		return mem[int(addr):end]
	}, 0, 4)
	if len(lines) != 4 {
		t.Fatalf("got %d lines: %#v", len(lines), lines)
	}
	for i, want := range []string{"LINK.L", "BFEXTU", "DIVSL"} {
		if !strings.Contains(lines[i].Mnemonic, want) {
			t.Fatalf("line %d = %q, want %s", i, lines[i].Mnemonic, want)
		}
	}
	if !strings.Contains(lines[3].Mnemonic, "{D2:D5}") {
		t.Fatalf("dynamic bitfield extension = %q, want dynamic offset/width", lines[3].Mnemonic)
	}
}

func TestFreeze_NoSpinWait_UsesChannel_IE64(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	dbg := NewDebugIE64(cpu)
	dbg.SetBreakpoint(0x1000)
	cpu.PC = 0x2000
	cpu.memory[0x2000] = OP_NOP64
	dbg.Resume()
	done := make(chan struct{})
	start := time.Now()
	go func() {
		dbg.Freeze()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Freeze timed out")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("Freeze used slow wait")
	}
}

func TestFreeze_NoSpinWait_UsesChannel_AllAdapters(t *testing.T) {
	bus := NewMachineBus()
	tests := []struct {
		name string
		cpu  DebuggableCPU
	}{
		{"IE64", NewDebugIE64(NewCPU64(bus))},
		{"IE32", NewDebugIE32(NewCPU(bus))},
		{"6502", NewDebug6502(NewCPU_6502(bus), nil)},
		{"Z80", NewDebugZ80(NewCPU_Z80(&testZ80Bus{}), nil)},
		{"M68K", NewDebugM68K(NewM68KCPU(bus), nil)},
		{"X86", NewDebugX86(NewCPU_X86(&testX86Bus{}), nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stop := make(chan struct{})
			frozen := make(chan struct{})
			switch d := tt.cpu.(type) {
			case *DebugIE64:
				d.trapStop, d.frozenCh = stop, frozen
				d.trapRunning.Store(true)
			case *DebugIE32:
				d.trapStop, d.frozenCh = stop, frozen
				d.trapRunning.Store(true)
			case *Debug6502:
				d.trapStop, d.frozenCh = stop, frozen
				d.trapRunning.Store(true)
			case *DebugZ80:
				d.trapStop, d.frozenCh = stop, frozen
				d.trapRunning.Store(true)
			case *DebugM68K:
				d.trapStop, d.frozenCh = stop, frozen
				d.trapRunning.Store(true)
			case *DebugX86:
				d.trapStop, d.frozenCh = stop, frozen
				d.trapRunning.Store(true)
			default:
				t.Fatalf("unexpected adapter %T", tt.cpu)
			}
			go func() {
				<-stop
				close(frozen)
			}()
			done := make(chan struct{})
			go func() {
				tt.cpu.Freeze()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Freeze timed out")
			}
		})
	}
}

func TestAdapter_NilRunner_FreezeNoOp(t *testing.T) {
	bus := NewMachineBus()
	tests := []struct {
		name string
		cpu  DebuggableCPU
	}{
		{"6502", NewDebug6502(NewCPU_6502(bus), nil)},
		{"Z80", NewDebugZ80(NewCPU_Z80(&testZ80Bus{}), nil)},
		{"M68K", NewDebugM68K(NewM68KCPU(bus), nil)},
		{"X86", NewDebugX86(NewCPU_X86(&testX86Bus{}), nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Freeze panicked: %v", r)
				}
			}()
			tt.cpu.Freeze()
		})
	}
}
