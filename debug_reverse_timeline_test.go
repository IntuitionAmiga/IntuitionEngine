package main

import (
	"strings"
	"testing"
)

func TestReverseContinue_RestoresWholeMachineSnapshot(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(DEFAULT_MEMORY_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	cpu.PC = 0x1000
	cpu.regs[1] = 0xCAFE
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))
	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	mon.wholeHistory = append(mon.wholeHistory, snap)

	cpu.PC = 0x2000
	cpu.regs[1] = 0
	_, out := mon.ExecuteCommandResult("rg")
	if !strings.Contains(uxOutputText(out), "Reverse continue") {
		t.Fatalf("rg output = %#v", out)
	}
	if cpu.PC != 0x1000 || cpu.regs[1] != 0xCAFE {
		t.Fatalf("restored pc=$%X r1=$%X", cpu.PC, cpu.regs[1])
	}
}

func TestReverseContinue_ReplaysFromPriorSnapshotToBoundary(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(DEFAULT_MEMORY_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	cpu.PC = PROG_START
	cpu.memory[PROG_START] = OP_NOP64
	cpu.memory[PROG_START+8] = OP_NOP64
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))

	mon.recordWholeMachineHistory()
	if _, err := safeDebugStep(NewDebugIE64(cpu)); err != nil {
		t.Fatal(err)
	}
	mon.handleBreakpointHit(BreakpointEvent{CPUID: 0, Address: cpu.PC})
	if len(mon.timelineEvents) == 0 || mon.timelineEvents[len(mon.timelineEvents)-1].SnapshotID == 0 {
		t.Fatalf("breakpoint event did not record a whole-machine boundary snapshot")
	}
	if _, err := safeDebugStep(NewDebugIE64(cpu)); err != nil {
		t.Fatal(err)
	}
	if cpu.PC != PROG_START+16 {
		t.Fatalf("setup PC=$%X, want $%X", cpu.PC, PROG_START+16)
	}

	_, out := mon.ExecuteCommandResult("rg")
	text := uxOutputText(out)
	if !strings.Contains(text, "replayed 1 instruction") {
		t.Fatalf("rg output = %#v, want replay path", out)
	}
	if cpu.PC != PROG_START+8 {
		t.Fatalf("PC=$%X, want replayed boundary $%X", cpu.PC, PROG_START+8)
	}
}

func TestReverseRunUntil_StopsAtMatchingSnapshot(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))

	cpu.PC = 0x1000
	first, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	cpu.PC = 0x2000
	second, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	cpu.PC = 0x3000
	mon.wholeHistory = append(mon.wholeHistory, first, second)

	_, out := mon.ExecuteCommandResult("rt PC==$1000")
	if !strings.Contains(uxOutputText(out), "Reverse run-until") {
		t.Fatalf("rt output = %#v", out)
	}
	if cpu.PC != 0x1000 {
		t.Fatalf("PC=$%X, want $1000", cpu.PC)
	}
}

func TestTimeline_ShowsAccessEvents(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	cpu.PC = 0x1234
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))
	mon.access.EnableHistory(4)
	mon.access.OnRead(0, 0x44, 1)

	_, out := mon.ExecuteCommandResult("tl 1")
	if !strings.Contains(uxOutputText(out), "read $44") {
		t.Fatalf("tl output = %#v", out)
	}
	_, out = mon.ExecuteCommandResult("history horizon")
	if !strings.Contains(uxOutputText(out), "Whole-machine reverse horizon") {
		t.Fatalf("history output = %#v", out)
	}
}

func TestTimelineBack_RestoresPreviousWholeMachineSnapshot(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	cpu.PC = 0x1000
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))
	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	mon.wholeHistory = append(mon.wholeHistory, snap)

	cpu.PC = 0x2000
	_, out := mon.ExecuteCommandResult("tl back")
	if !strings.Contains(uxOutputText(out), "Reverse continue") {
		t.Fatalf("tl back output = %#v", out)
	}
	if cpu.PC != 0x1000 {
		t.Fatalf("PC=$%X, want $1000", cpu.PC)
	}
}

func TestTimeline_MergesInstructionAndFaultEvents(t *testing.T) {
	mon, cpu := newTestMonitor()
	cpu.memory[PROG_START] = OP_NOP64
	mon.ExecuteCommand("tracering on 4")
	mon.ExecuteCommand("s")
	mon.handleBreakpointHit(BreakpointEvent{CPUID: 0, Address: 0xCAFE, IsFault: true, FaultKind: "ie64.illegal"})

	_, out := mon.ExecuteCommandResult("tl 8")
	text := uxOutputText(out)
	if !strings.Contains(text, "instr cpu=0") || !strings.Contains(text, "fault cpu=0") {
		t.Fatalf("tl output = %#v", out)
	}
}

func TestReverseRunUntil_NoMatchPreservesCurrentState(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))

	cpu.PC = 0x1000
	snap, err := TakeWholeMachineSnapshot(mon)
	if err != nil {
		t.Fatal(err)
	}
	mon.wholeHistory = append(mon.wholeHistory, snap)
	cpu.PC = 0x3000

	_, out := mon.ExecuteCommandResult("rt PC==$9999")
	if !strings.Contains(uxOutputText(out), "No earlier whole-machine snapshot matched") {
		t.Fatalf("rt output = %#v", out)
	}
	if cpu.PC != 0x3000 {
		t.Fatalf("PC=$%X, want current state $3000 after no-match", cpu.PC)
	}
	if len(mon.wholeHistory) != 1 {
		t.Fatalf("whole history length = %d, want preserved history", len(mon.wholeHistory))
	}
	_, out = mon.ExecuteCommandResult("rg")
	if !strings.Contains(uxOutputText(out), "Reverse continue") {
		t.Fatalf("rg after failed rt output = %#v", out)
	}
	if cpu.PC != 0x1000 {
		t.Fatalf("rg after failed rt PC=$%X, want $1000", cpu.PC)
	}
}

func TestTimeline_RecentMonitorEventNotDroppedAfterManyAccessEvents(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))
	mon.access.EnableHistory(64)
	for i := 0; i < 20; i++ {
		mon.access.OnRead(0, uint64(i), 1)
	}
	mon.recordTimelineEventLocked("fault", 0, 0xCAFE, "recent fault")

	_, out := mon.ExecuteCommandResult("tl 1")
	text := uxOutputText(out)
	if !strings.Contains(text, "recent fault") {
		t.Fatalf("tl output = %#v, want recent monitor event", out)
	}
}

func TestTimeline_RecentAccessEventNotHiddenByOlderMonitorEvent(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	mon := NewMachineMonitor(bus)
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))
	mon.access.EnableHistory(64)

	mon.recordTimelineEventLocked("fault", 0, 0xCAFE, "older fault")
	mon.access.OnRead(0, 0x44, 1)

	_, out := mon.ExecuteCommandResult("tl 1")
	text := uxOutputText(out)
	if !strings.Contains(text, "read $44") {
		t.Fatalf("tl output = %#v, want recent access event", out)
	}
	if strings.Contains(text, "older fault") {
		t.Fatalf("tl output = %#v, older monitor event should not hide newer access", out)
	}
}
